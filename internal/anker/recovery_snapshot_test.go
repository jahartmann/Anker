package anker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type snapshotAPI interface {
	CaptureRecovery(context.Context, PlanRequest) (RecoveryInspection, error)
	LoadRecoverySnapshot(string, PlanRequest) (RecoverySnapshot, error)
	ReadSnapshotFile(string, string) (RecoveryReadFile, error)
}
type snapshotCollector struct {
	targetCollector
	result RecoveryReadResult
}

func (c snapshotCollector) ReadRecoveryFiles(_ context.Context, _ Host, paths, ids []string) (RecoveryReadResult, error) {
	return c.result, nil
}
func snapshotSetup(t *testing.T) (*Service, snapshotAPI, PlanRequest) {
	t.Helper()
	path := "etc/sysctl.d/a.conf"
	content := []byte("old\n")
	meta := Entry{Path: path, Type: "file", Mode: 0644, UID: 0, GID: 0, Size: 4, SHA256: Hash(content), XAttrs: map[string]string{}}
	hashes, _ := json.Marshal(map[string]string{path: Hash(content), "etc/pve/storage.cfg": "missing"})
	metadata := json.RawMessage(`{"etc/sysctl.d/a.conf":{"path":"etc/sysctl.d/a.conf","type":"file","mode":420,"uid":0,"gid":0,"size":4,"xattrs":{}}}`)
	target := Inventory{Hostname: "target", PVEVersion: "8.4", Details: map[string]json.RawMessage{"file_hashes": hashes, "file_metadata": metadata, "command_results": json.RawMessage(`{"file_inventory":{"state":"ok"}}`)}}
	s, b := recoverySetup(t, map[string]string{path: "new\n", "etc/hosts": "127.0.0.1 localhost\n"}, Inventory{PVEVersion: "8.4"}, target)
	s.Collector = snapshotCollector{targetCollector: targetCollector{inv: target}, result: RecoveryReadResult{Files: []RecoveryReadFile{{Path: path, State: "read", SHA256: Hash(content), Metadata: &meta, Content: content}, {Path: "etc/pve/storage.cfg", State: "missing"}}, StorageVolumes: map[string]RecoveryVolumeResult{}}}
	api, ok := any(s).(snapshotAPI)
	if !ok {
		t.Fatal("private recovery snapshots missing")
	}
	return s, api, PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{path}}
}
func TestRecoverySnapshotRejectsChangedContent(t *testing.T) {
	s, api, r := snapshotSetup(t)
	c := s.Collector.(snapshotCollector)
	c.result.Files[0].Content = []byte("changed\n")
	c.result.Files[0].SHA256 = Hash(c.result.Files[0].Content)
	c.result.Files[0].Metadata.SHA256 = c.result.Files[0].SHA256
	c.result.Files[0].Metadata.Size = int64(len(c.result.Files[0].Content))
	s.Collector = c
	i, err := api.CaptureRecovery(t.Context(), r)
	if err == nil || i.SnapshotID != "" {
		t.Fatal("changed read published snapshot")
	}
}
func TestRecoverySnapshotBindsConnectionAndSelection(t *testing.T) {
	s, api, r := snapshotSetup(t)
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	if i.SnapshotID == "" {
		t.Fatal("no snapshot")
	}
	if _, err = api.LoadRecoverySnapshot(i.SnapshotID, r); err != nil {
		t.Fatal(err)
	}
	changed := r
	changed.Files = []string{"etc/hosts"}
	if _, err = api.LoadRecoverySnapshot(i.SnapshotID, changed); err == nil {
		t.Fatal("changed selection accepted")
	}
	changed = r
	changed.TargetID = "host1"
	if _, err = api.LoadRecoverySnapshot(i.SnapshotID, changed); err == nil {
		t.Fatal("changed target accepted")
	}
	h, _ := s.Host("target")
	h.Address = "192.0.2.99"
	if err = s.SaveHost(h); err != nil {
		t.Fatal(err)
	}
	if _, err = api.LoadRecoverySnapshot(i.SnapshotID, r); err == nil {
		t.Fatal("changed endpoint accepted")
	}
	if _, err = api.ReadSnapshotFile(i.SnapshotID, r.Files[0]); err == nil {
		t.Fatal("read ignored changed endpoint")
	}
}
func TestRecoverySnapshotTamperingAndExpiry(t *testing.T) {
	s, api, r := snapshotSetup(t)
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	f, err := api.ReadSnapshotFile(i.SnapshotID, r.Files[0])
	if err != nil || string(f.Content) != "old\n" {
		t.Fatalf("read: %+v %v", f, err)
	}
	dir := filepath.Join(s.Root, "recovery-inspections", i.SnapshotID)
	for _, p := range []string{dir, filepath.Join(dir, "snapshot.json"), filepath.Join(dir, "files", r.Files[0])} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0600)
		if st.IsDir() {
			want = 0700
		}
		if st.Mode().Perm() != want {
			t.Fatalf("mode %s: %o", p, st.Mode().Perm())
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "files", r.Files[0]), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = api.ReadSnapshotFile(i.SnapshotID, r.Files[0]); err == nil {
		t.Fatal("tampered content trusted")
	}
	// Expiry is anchored independently from the file so editing either cannot extend it.
	var record recoverySnapshotRecord
	if err = s.Store.Get("recovery-inspections", i.SnapshotID, &record); err != nil {
		t.Fatal(err)
	}
	record.ExpiresAt = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	s.Store.Put("recovery-inspections", i.SnapshotID, record)
	if _, err = api.LoadRecoverySnapshot(i.SnapshotID, r); err == nil {
		t.Fatal("expired snapshot trusted")
	}
}
func TestOldReaderNeverInventsTargetContent(t *testing.T) {
	s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "backup"}, Inventory{PVEVersion: "8.4"}, Inventory{PVEVersion: "8.4"})
	i, err := s.InspectRecovery(t.Context(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/a.conf"}})
	if err != nil {
		t.Fatal(err)
	}
	if i.ComparisonComplete || i.SnapshotID != "" || i.Automatic || !strings.Contains(strings.Join(i.Warnings, " "), "Inhaltsvergleich nicht verfügbar") {
		t.Fatalf("old capability invented evidence: %+v", i)
	}
}
func TestRecoverySnapshotStoragePrivacy(t *testing.T) {
	s, api, r := snapshotSetup(t)
	c := s.Collector.(snapshotCollector)
	content := []byte("pbs: archive\n server pbs.example\n password top-secret\n token credential\n")
	c.result.Files[1] = RecoveryReadFile{Path: "etc/pve/storage.cfg", State: "read", SHA256: Hash(content), Content: content, Metadata: &Entry{Path: "etc/pve/storage.cfg", Type: "file", Mode: 0600, Size: int64(len(content)), SHA256: Hash(content), XAttrs: map[string]string{}}}
	hashes := fileHashes(c.inv)
	hashes["etc/pve/storage.cfg"] = Hash(content)
	c.inv.Details["file_hashes"], _ = json.Marshal(hashes)
	var meta map[string]map[string]any
	json.Unmarshal(c.inv.Details["file_metadata"], &meta)
	meta["etc/pve/storage.cfg"] = map[string]any{"type": "file", "mode": 384, "uid": 0, "gid": 0, "xattrs": map[string]string{}}
	c.inv.Details["file_metadata"], _ = json.Marshal(meta)
	c.inv.Details["storage_config"], _ = json.Marshal(string(content))
	s.Collector = c
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := json.Marshal(i)
	if strings.Contains(string(view), "top-secret") || strings.Contains(string(view), "credential") {
		t.Fatalf("private storage leaked: %s", view)
	}
	f, err := api.ReadSnapshotFile(i.SnapshotID, "etc/pve/storage.cfg")
	if err != nil || string(f.Content) != string(content) {
		t.Fatalf("private storage lost: %v", err)
	}
	encoded, _ := json.Marshal(f)
	if strings.Contains(string(encoded), "content") {
		t.Fatal("read model serialized private content")
	}
	legacy, _ := json.Marshal(Plan{Target: c.inv})
	if strings.Contains(string(legacy), "top-secret") {
		t.Fatal("legacy inventory leaked raw storage")
	}
}

func TestRecoverySnapshotIncompleteInventoryDoesNotClaimMissing(t *testing.T) {
	s, api, r := snapshotSetup(t)
	c := s.Collector.(snapshotCollector)
	c.inv.Details["command_results"] = json.RawMessage(`{"file_inventory":{"state":"failed"}}`)
	c.result.Files[0] = RecoveryReadFile{Path: r.Files[0], State: "unreadable"}
	s.Collector = c
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	if i.ComparisonComplete || i.Automatic {
		t.Fatal("incomplete target trusted")
	}
	f, err := api.ReadSnapshotFile(i.SnapshotID, "etc/pve/storage.cfg")
	if err != nil {
		t.Fatal(err)
	}
	if f.State == "missing" || len(f.Content) > 0 {
		t.Fatalf("invented absence: %+v", f)
	}
}
func TestRecoverySnapshotRejectsMetadataDriftAndManifestTamper(t *testing.T) {
	s, api, r := snapshotSetup(t)
	c := s.Collector.(snapshotCollector)
	c.result.Files[0].Metadata.Mode = 0600
	s.Collector = c
	if _, err := api.CaptureRecovery(t.Context(), r); err == nil {
		t.Fatal("metadata drift accepted")
	}
	c.result.Files[0].Metadata.Mode = 0644
	s.Collector = c
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.Root, "recovery-inspections", i.SnapshotID, "snapshot.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"scenario":"files"`, `"scenario":"host"`, 1))
	os.WriteFile(p, data, 0600)
	if _, err = api.ReadSnapshotFile(i.SnapshotID, r.Files[0]); err == nil {
		t.Fatal("tampered metadata trusted")
	}
}
func TestRecoverySnapshotHostKeyBindingAndCleanup(t *testing.T) {
	s, api, r := snapshotSetup(t)
	h, _ := s.Host("target")
	h.KnownHostsPath = filepath.Join(t.TempDir(), "known")
	os.WriteFile(h.KnownHostsPath, []byte("key-a"), 0600)
	s.SaveHost(h)
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(h.KnownHostsPath, []byte("key-b"), 0600)
	if _, err = api.ReadSnapshotFile(i.SnapshotID, r.Files[0]); err == nil {
		t.Fatal("host key change accepted")
	}
	copy := filepath.Join(s.Root, "plans", "independent", "target-files", r.Files[0])
	if err = atomicWrite(copy, []byte("copied"), 0600); err != nil {
		t.Fatal(err)
	}
	var record recoverySnapshotRecord
	s.Store.Get("recovery-inspections", i.SnapshotID, &record)
	record.ExpiresAt = time.Now().Add(-time.Second).Format(time.RFC3339Nano)
	s.Store.Put("recovery-inspections", i.SnapshotID, record)
	fresh, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.Root, "recovery-inspections", i.SnapshotID)); !os.IsNotExist(err) {
		t.Fatalf("expired snapshot retained: %v", err)
	}
	if _, err = api.LoadRecoverySnapshot(fresh.SnapshotID, r); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(copy); err != nil || string(data) != "copied" {
		t.Fatal("cleanup removed plan copy")
	}
}
func TestRecoverySnapshotDemoUsesLocalContents(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	root := filepath.Join(s.Root, "demo-hosts", h.ID)
	inv := Inventory{PVEVersion: "8.4", Hostname: h.Name}
	if err := writeJSON(filepath.Join(root, "inventory.json"), inv); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(root, "files/etc/hosts"), []byte("localhost\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reader, ok := any(DemoCollector{Root: s.Root}).(recoveryReadAPI)
	if !ok {
		t.Fatal("demo recovery reader missing")
	}
	result, err := reader.ReadRecoveryFiles(t.Context(), h, []string{"etc/hosts", "etc/missing"}, []string{"local"})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Files[0].Content) != "localhost\n" || result.Files[1].State != "missing" || result.StorageVolumes["local"].State != "unknown" {
		t.Fatalf("wrong demo evidence: %+v", result)
	}
	if err = os.Symlink("/etc/hosts", filepath.Join(root, "files/etc/link")); err != nil {
		t.Fatal(err)
	}
	result, err = reader.ReadRecoveryFiles(t.Context(), h, []string{"etc/link"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files[0].State != "unsupported" || result.Files[0].Content != nil {
		t.Fatal("demo followed symlink")
	}
}

func TestRecoverySnapshotMalformedInventoryDoesNotProveAbsence(t *testing.T) {
	s, api, r := snapshotSetup(t)
	c := s.Collector.(snapshotCollector)
	c.inv.Details["file_hashes"] = json.RawMessage(`null`)
	c.result.Files[0] = RecoveryReadFile{Path: r.Files[0], State: "missing"}
	s.Collector = c
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	if i.ComparisonComplete {
		t.Fatal("malformed inventory asserted complete absence")
	}
}

type keyChangingCollector struct {
	snapshotCollector
	path string
}

func (c keyChangingCollector) Probe(ctx context.Context, h Host) (Inventory, error) {
	if err := os.WriteFile(c.path, []byte("changed-during-probe"), 0600); err != nil {
		return Inventory{}, err
	}
	return c.snapshotCollector.Probe(ctx, h)
}
func TestRecoverySnapshotRejectsHostKeyChangeDuringProbe(t *testing.T) {
	s, api, r := snapshotSetup(t)
	h, _ := s.Host("target")
	h.KnownHostsPath = filepath.Join(t.TempDir(), "known")
	os.WriteFile(h.KnownHostsPath, []byte("original"), 0600)
	s.SaveHost(h)
	s.Collector = keyChangingCollector{snapshotCollector: s.Collector.(snapshotCollector), path: h.KnownHostsPath}
	if i, err := api.CaptureRecovery(t.Context(), r); err == nil || i.SnapshotID != "" {
		t.Fatal("host key changed during probe accepted")
	}
}
func TestRecoverySnapshotBindsStorageQueryMapping(t *testing.T) {
	_, api, r := snapshotSetup(t)
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.Mapping.Storage = map[string]string{"old": "new"}
	if _, err = api.LoadRecoverySnapshot(i.SnapshotID, r); err == nil {
		t.Fatal("snapshot reused for different storage query mapping")
	}
}
func TestRecoveryInventorySerializationKeepsLegacyFingerprint(t *testing.T) {
	// Legacy inventory JSON stays readable; raw credentials do not survive public serialization.
	raw := `{"hostname":"target","pve_version":"8.4","debian":"","kernel":"","boot_mode":"","cluster_id":"","quorate":false,"interfaces":null,"disks":null,"details":{"file_hashes":{"etc/a":"old"},"storage_config":"pbs: archive\n server pbs.example\n password top-secret\n"},"captured_at":"","fingerprint":""}`
	var inv Inventory
	if err := json.Unmarshal([]byte(raw), &inv); err != nil {
		t.Fatal(err)
	}
	stable := `{"hostname":"target","pve_version":"8.4","debian":"","kernel":"","boot_mode":"","cluster_id":"","quorate":false,"interfaces":null,"disks":null,"details":{"file_hashes":{"etc/a":"old"}},"captured_at":"","fingerprint":""}`
	before := Fingerprint(inv)
	if before != Hash([]byte(stable)) {
		t.Fatal("legacy fingerprint material changed")
	}
	public, err := json.Marshal(Plan{Target: inv, Source: inv})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), "top-secret") || !strings.Contains(string(public), "storage_definitions") {
		t.Fatalf("bad public inventory: %s", public)
	}
	var roundtrip Plan
	if err = json.Unmarshal(public, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if Fingerprint(roundtrip.Target) != before {
		t.Fatal("redaction changed fingerprint")
	}
	if inv.Details["storage_config"] == nil {
		t.Fatal("serialization mutated internal evidence")
	}
	inv.Details["file_hashes"] = json.RawMessage(`{"etc/a":"new"}`)
	if Fingerprint(inv) == before {
		t.Fatal("content drift no longer changes fingerprint")
	}
}

func TestRecoverySnapshotInspectionExposesStatesWithoutContents(t *testing.T) {
	_, api, r := snapshotSetup(t)
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(i)
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Files          []map[string]any                `json:"files"`
		StorageVolumes map[string]RecoveryVolumeResult `json:"storage_volumes"`
	}
	if err = json.Unmarshal(encoded, &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Files) != 2 || view.StorageVolumes == nil {
		t.Fatal("inspection hides independent file/storage states")
	}
	for _, f := range view.Files {
		if _, exists := f["content"]; exists {
			t.Fatal("private content in inspection")
		}
	}
}

func TestRecoverySnapshotSourceContextVerifiedAndPrivate(t *testing.T) {
	s, api, r := snapshotSetup(t)
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := api.LoadRecoverySnapshot(i.SnapshotID, r)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Source.PVEVersion != "8.4" || snap.SourceStorageConfig != nil {
		t.Fatalf("missing source context or absent config invented: %+v", snap.Source)
	}
	snap.Source.Hostname = "private-source-hostname"
	snap.SourceStorageConfig = []byte("private-source-storage")
	raw, _ := json.Marshal(snap)
	if strings.Contains(string(raw), "private-source-") {
		t.Fatal("ephemeral source context serialized")
	}
	b, err := s.Backup(r.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.backupDir(b), "files", r.Files[0]), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = api.LoadRecoverySnapshot(i.SnapshotID, r); err == nil {
		t.Fatal("tampered source backup accepted for preparation")
	}
}

func TestRecoverySnapshotLoadsAuthoritativeSourceStorage(t *testing.T) {
	for _, config := range []string{"", "dir: source\n path /srv/source\n password private-source-secret\n"} {
		t.Run(map[bool]string{true: "empty", false: "present"}[config == ""], func(t *testing.T) {
			s, _, r := snapshotSetup(t)
			target := s.Collector.(snapshotCollector)
			s.Collector = recoveryFixture{content: map[string]string{r.Files[0]: "new\n", "etc/pve/storage.cfg": config}, source: Inventory{Hostname: "source", PVEVersion: "8.4"}}
			b, err := s.CreateBackup(t.Context(), "host1")
			if err != nil {
				t.Fatal(err)
			}
			r.BackupID = b.ID
			s.Collector = target
			i, err := s.CaptureRecovery(t.Context(), r)
			if err != nil {
				t.Fatal(err)
			}
			snap, err := s.LoadRecoverySnapshot(i.SnapshotID, r)
			if err != nil {
				t.Fatal(err)
			}
			if snap.SourceStorageConfig == nil || string(snap.SourceStorageConfig) != config || snap.Source.Hostname != "source" {
				t.Fatal("authoritative source config not loaded (empty must differ from absent)")
			}
			raw, err := os.ReadFile(filepath.Join(s.Root, "recovery-inspections", i.SnapshotID, "snapshot.json"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "private-source-secret") {
				t.Fatal("source credentials in snapshot metadata")
			}
			if err = os.WriteFile(filepath.Join(s.backupDir(b), "files/etc/pve/storage.cfg"), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = s.LoadRecoverySnapshot(i.SnapshotID, r); err == nil {
				t.Fatal("tampered source storage accepted")
			}
		})
	}
}
