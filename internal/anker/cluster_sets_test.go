package anker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type setCollector struct {
	fixtureCollector
	mode string
}

func (c setCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	if c.mode == "missing" && h.ID == "node2" {
		return Collection{}, errors.New("private-token-do-not-project")
	}
	v, e := c.fixtureCollector.Collect(ctx, h, dest)
	if e != nil {
		return v, e
	}
	v.Inventory.ClusterID = "cluster1"
	v.Inventory.Hostname = h.Name
	if c.mode == "identity-unknown" {
		v.Inventory.ClusterID = ""
	}
	members := "nodelist {\n node {\n name: pve-test\n nodeid: 1\n }\n node {\n name: pve-two\n nodeid: 2\n }\n}\n"
	if c.mode == "drift" && h.ID == "node2" {
		members = strings.ReplaceAll(members, "pve-two", "unexpected-node")
	}
	data := "dir: local\n path /var/lib/vz\n"
	if c.mode == "different" && h.ID == "node2" {
		data += " content images\n"
	}
	for p, s := range map[string]string{"etc/pve/corosync.conf": members, "etc/pve/storage.cfg": data, "etc/pve/priv/token": "private-token-do-not-project"} {
		os.MkdirAll(filepath.Dir(filepath.Join(dest, "files", p)), 0700)
		os.WriteFile(filepath.Join(dest, "files", p), []byte(s), 0600)
		v.Entries = append(v.Entries, Entry{Path: p, Type: "file", Mode: 0640})
	}
	os.MkdirAll(filepath.Join(dest, "recovery"), 0700)
	os.WriteFile(filepath.Join(dest, "recovery/config.db"), []byte("snapshot"), 0600)
	if c.mode == "unknown" {
		v.Entries = v.Entries[:1]
	}
	return v, nil
}
func setService(t *testing.T, mode string) *Service {
	s := testService(t)
	h, _ := s.Host("host1")
	h.ClusterID = "cluster1"
	s.SaveHost(h)
	if err := s.SaveHost(Host{ID: "node2", Name: "pve-two", Address: "192.0.2.2", ClusterID: "cluster1"}); err != nil {
		t.Fatal(err)
	}
	settings, _ := s.Settings()
	settings.Retries = 0
	settings.Parallel = 1
	s.Store.Put("settings", "main", settings)
	s.Collector = setCollector{mode: mode}
	return s
}
func awaitSet(t *testing.T, s *Service, id string) ClusterBackupSet {
	t.Helper()
	for i := 0; i < 250; i++ {
		v, e := s.ClusterSet(id)
		if e != nil {
			t.Fatal(e)
		}
		if v.State != "capturing" {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("capture did not finish")
	return ClusterBackupSet{}
}
func TestClusterSetCaptureEvidence(t *testing.T) {
	for _, tc := range []struct{ mode, state string }{{"", "complete"}, {"missing", "incomplete"}, {"drift", "divergent"}, {"different", "divergent"}, {"unknown", "incomplete"}, {"identity-unknown", "incomplete"}} {
		t.Run(tc.mode, func(t *testing.T) {
			s := setService(t, tc.mode)
			v, err := s.QueueClusterSet("cluster1")
			if err != nil {
				t.Fatal(err)
			}
			v = awaitSet(t, s, v.ID)
			if v.State != tc.state {
				t.Fatalf("want %s got %+v", tc.state, v)
			}
			if v.Atomic || v.Purpose != "manualSourcePlanning" {
				t.Fatalf("unsafe scope %+v", v)
			}
			if tc.mode == "" {
				for _, m := range v.Members {
					if m.BackupID == "" || m.SharedSHA256 == "" || m.StartedAt == "" || m.CompletedAt == "" {
						t.Fatalf("missing actual capture evidence %+v", m)
					}
				}
			}
			raw, _ := json.Marshal(publicClusterSet(v))
			if strings.Contains(string(raw), "private-token") || strings.Contains(string(raw), "shared_sha256") {
				t.Fatalf("secret evidence projected %s", raw)
			}
		})
	}
}
func TestClusterSetDoesNotAdoptStaleBackupOrSuccessfulJob(t *testing.T) {
	s := setService(t, "")
	old, err := s.CreateBackup(t.Context(), "node2")
	if err != nil {
		t.Fatal(err)
	}
	s.Collector = setCollector{mode: "missing"}
	v, err := s.QueueClusterSet("cluster1")
	if err != nil {
		t.Fatal(err)
	}
	v = awaitSet(t, s, v.ID)
	for _, m := range v.Members {
		if m.BackupID == old.ID {
			t.Fatal("adopted stale backup")
		}
	}
	// A successful job without capture evidence cannot fill a missing member.
	for _, m := range v.Members {
		if m.HostID == "node2" {
			j, _ := s.Job(m.JobID)
			j.State = "successful"
			j.ResultID = old.ID
			s.Store.Put("jobs", j.ID, j)
		}
	}
	v, _ = s.ClusterSet(v.ID)
	if v.State != "incomplete" {
		t.Fatalf("job status substituted for evidence %+v", v)
	}
}
func TestClusterSetInterruptedAndAdmission(t *testing.T) {
	s := setService(t, "")
	s.maintenance = true
	v, err := s.QueueClusterSet("cluster1")
	if err != nil {
		t.Fatal(err)
	}
	v, _ = s.ClusterSet(v.ID)
	if v.State != "incomplete" {
		t.Fatalf("admission bypass %+v", v)
	}
	s.maintenance = false
	v.Members[0].State = "queued"
	v.Members[0].JobID = "interrupted-job"
	v.Members[0].BackupID = ""
	s.Store.Put("cluster_sets", v.ID, v)
	s.Store.Put("jobs", "interrupted-job", Job{ID: "interrupted-job", HostID: "host1", Kind: "backup", State: "running", SourceSetID: v.ID})
	if err = s.RecoverJobs(); err != nil {
		t.Fatal(err)
	}
	v, _ = s.ClusterSet(v.ID)
	if v.State != "incomplete" || v.Members[0].State != "interrupted" {
		t.Fatalf("interruption not retained %+v", v)
	}
}

type gatedSetCollector struct {
	setCollector
	entered chan string
	release chan struct{}
}

func (c gatedSetCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	c.entered <- h.ID
	select {
	case <-ctx.Done():
		return Collection{}, ctx.Err()
	case <-c.release:
	}
	return c.setCollector.Collect(ctx, h, dest)
}
func TestClusterSetUsesExistingExclusionAndParallelBudget(t *testing.T) {
	for _, preexisting := range []bool{false, true} {
		t.Run(fmt.Sprint(preexisting), func(t *testing.T) {
			s := setService(t, "")
			entered := make(chan string, 5)
			release := make(chan struct{})
			s.Collector = gatedSetCollector{setCollector: setCollector{}, entered: entered, release: release}
			if preexisting {
				if _, e := s.QueueBackup("host1"); e != nil {
					t.Fatal(e)
				}
				select {
				case <-entered:
				case <-time.After(2 * time.Second):
					t.Fatal("existing job not started")
				}
			}
			v, e := s.QueueClusterSet("cluster1")
			if e != nil {
				t.Fatal(e)
			}
			if !preexisting {
				select {
				case <-entered:
				case <-time.After(2 * time.Second):
					t.Fatal("set not started")
				}
			}
			select {
			case <-entered:
				t.Fatal("parallel budget bypassed")
			case <-time.After(150 * time.Millisecond):
			}
			if preexisting && (v.Members[0].HostID != "host1" || v.Members[0].JobID != "" || v.Members[0].State != "not_admitted") {
				t.Fatalf("old job adopted %+v", v)
			}
			close(release)
			v = awaitSet(t, s, v.ID)
			if preexisting && v.State != "incomplete" {
				t.Fatal("excluded member became complete", v)
			}
			if !preexisting && (v.State != "complete" || v.StartSkewMS < 100) {
				t.Fatalf("capture/skew missing %+v", v)
			}
		})
	}
}
func TestClusterSetHTTPProjectionAndMethods(t *testing.T) {
	s := setService(t, "")
	v, e := s.QueueClusterSet("cluster1")
	if e != nil {
		t.Fatal(e)
	}
	v = awaitSet(t, s, v.ID)
	for _, path := range []string{"/api/cluster-sets", "/api/cluster-sets/" + v.ID, "/api/cluster-sets/" + v.ID + "/export"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		if err := handleAPI(s, nil, User{Role: "reader"}, w, r); err != nil {
			t.Fatal(err)
		}
		body := w.Body.String()
		if !strings.Contains(body, "manualSourcePlanning") || strings.Contains(body, "shared_sha256") || strings.Contains(body, "private-token") {
			t.Fatal("unsafe projection", body)
		}
	}
	for _, role := range []string{"reader", "admin"} {
		r := httptest.NewRequest("POST", "/api/cluster-sets/"+v.ID+"/export", nil)
		w := httptest.NewRecorder()
		if e := handleAPI(s, nil, User{Role: role}, w, r); e == nil {
			t.Fatal("unsafe mutation")
		}
	}
	r := httptest.NewRequest("POST", "/api/cluster-sets", strings.NewReader(`{"cluster_id":"cluster1"}`))
	if e := handleAPI(s, nil, User{Role: "reader"}, httptest.NewRecorder(), r); e == nil {
		t.Fatal("reader captured")
	}
}
func TestClusterSetCatalogColdRoundTrip(t *testing.T) {
	s := setService(t, "")
	v, e := s.QueueClusterSet("cluster1")
	if e != nil {
		t.Fatal(e)
	}
	v = awaitSet(t, s, v.ID)
	// Cold boundary: every admitted worker must have fully exited.
	for i := 0; i < 200; i++ {
		s.mu.Lock()
		n := len(s.cancels)
		s.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if e = EnsureDataMode(s.Root, false); e != nil {
		t.Fatal(e)
	}
	var before ClusterBackupSet
	s.Store.Get("cluster_sets", v.ID, &before)
	var bundle bytes.Buffer
	if e = ExportDisaster(s.Root, "", []byte("cluster-set-test-pass"), &bundle); e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(t.TempDir(), "imported")
	if _, e = ImportDisaster(&bundle, dest, []byte("cluster-set-test-pass")); e != nil {
		t.Fatal(e)
	}
	store, e := OpenStore(filepath.Join(dest, "catalog.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	var after ClusterBackupSet
	if e = store.Get("cluster_sets", v.ID, &after); e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("historical evidence lost", e)
	}
	for _, m := range after.Members {
		if _, e = os.Stat(filepath.Join(dest, "hosts", m.HostID, "backups", m.BackupID, "manifest.json")); e != nil {
			t.Fatal("referenced bytes omitted", e)
		}
	}
	if _, e = os.Stat(filepath.Join(dest, disasterIsolation)); e != nil {
		t.Fatal(e)
	}
	if _, e = NewService(dest, store, fixtureCollector{}); e == nil {
		t.Fatal("isolated import started")
	}
}

func TestClusterMemberParserRejectsAmbiguousNodeNames(t *testing.T) {
	config := "nodelist {\n node {\n name: pve-test\n name: pve-two\n }\n node {\n nodeid: 2\n }\n}\n"
	if names := clusterMemberNames(config); len(names) != 0 {
		t.Fatalf("missing per-node identity accepted: %v", names)
	}
}

func TestClusterMemberParserIgnoresCommentBraces(t *testing.T) {
	config := "nodelist {\n # example: node { name: ignored }\n node {\n name: pve-test\n }\n node {\n name: pve-two\n }\n}\n"
	want := []string{"pve-test", "pve-two"}
	if got := clusterMemberNames(config); !reflect.DeepEqual(got, want) {
		t.Fatalf("comments changed captured membership: %v", got)
	}
}

func TestClusterSetParallelResultsNeverLoseMembers(t *testing.T) {
	s := setService(t, "")
	settings, _ := s.Settings()
	settings.Parallel = 2
	s.Store.Put("settings", "main", settings)
	entered := make(chan string, 2)
	release := make(chan struct{})
	s.Collector = gatedSetCollector{setCollector: setCollector{}, entered: entered, release: release}
	v, e := s.QueueClusterSet("cluster1")
	if e != nil {
		t.Fatal(e)
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("parallel capture did not start")
		}
	}
	close(release)
	v = awaitSet(t, s, v.ID)
	if v.State != "complete" || len(v.Members) != 2 {
		t.Fatalf("lost capture results %+v", v)
	}
	seen := map[string]bool{}
	for _, m := range v.Members {
		if m.BackupID == "" || seen[m.BackupID] {
			t.Fatal("missing/duplicate result", m)
		}
		seen[m.BackupID] = true
		j, e := s.Job(m.JobID)
		if e != nil || j.SourceSetID != v.ID || j.HostID != m.HostID || j.ResultID != m.BackupID {
			t.Fatal("capture provenance lost", m, j, e)
		}
	}
}
