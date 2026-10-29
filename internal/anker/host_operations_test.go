package anker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func operationRequest(t *testing.T, s *Service, action string) HostOperationRequest {
	t.Helper()
	h, e := s.Host("host1")
	if e != nil {
		t.Fatal(e)
	}
	return HostOperationRequest{RequestID: ID(), Action: action, Targets: []HostOperationTarget{{HostID: h.ID, Revision: hostRevision(h)}}}
}
func TestHostOperationsMaintenanceExpiryAndReplay(t *testing.T) {
	s := testService(t)
	before, _ := s.Host("host1")
	req := operationRequest(t, s, "maintenance")
	req.Until = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	run, e := s.StartHostOperation("admin", req)
	if e != nil || run.Targets[0].State != "applied" {
		t.Fatal(run, e)
	}
	h, _ := s.Host("host1")
	if h.Enabled != before.Enabled || h.Schedule != before.Schedule || !hostMaintenance(h, time.Now()) || hostMaintenance(h, time.Now().Add(2*time.Hour)) {
		t.Fatal("maintenance destroyed schedule or did not expire", h)
	}
	replay, e := s.StartHostOperation("admin", req)
	if e != nil || replay.ID != run.ID || replay.Targets[0].State != "applied" {
		t.Fatal(replay, e)
	}
	// Replays remain valid after expiry; they must never reapply stale settings.
	var stored HostOperationRun
	s.Store.Get("host_operations", run.ID, &stored)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	stored.Request.Until = past
	s.Store.Put("host_operations", run.ID, stored)
	req.Until = past
	replay, e = s.StartHostOperation("admin", req)
	if e != nil || replay.Targets[0].State != "applied" {
		t.Fatal(replay, e)
	}
	current, _ := s.Host("host1")
	if current.MaintenanceUntil != h.MaintenanceUntil {
		t.Fatal("replay changed maintenance")
	}
	before.Name = "host-renamed"
	if e = s.SaveHost(before); e != nil {
		t.Fatal(e)
	}
	h, _ = s.Host("host1")
	if h.MaintenanceUntil != reqUntil(run) {
		t.Fatal("stale editor erased maintenance")
	}
}
func reqUntil(run HostOperationRun) string { return run.Request.Until }
func TestHostOperationsStaleSelectionPartialAndNonceIsolation(t *testing.T) {
	s := testService(t)
	req := operationRequest(t, s, "group")
	req.Value = "CP Weida"
	h, _ := s.Host("host1")
	h.Name = "changed"
	s.SaveHost(h)
	run, e := s.StartHostOperation("admin", req)
	if e != nil || run.Targets[0].State != "skipped" {
		t.Fatal(run, e)
	}
	h, _ = s.Host("host1")
	if h.Group == req.Value {
		t.Fatal("stale host overwritten")
	}
	req.Value = "FZ"
	if _, e = s.StartHostOperation("admin", req); e == nil {
		t.Fatal("nonce reused with changed request")
	}
	if _, e = s.HostOperationRun("other", req.RequestID); e == nil {
		t.Fatal("other actor obtained receipt")
	}
	req = operationRequest(t, s, "drift_schedule")
	req.Minutes = 30
	run, e = s.StartHostOperation("admin", req)
	if e != nil || run.Targets[0].State != "applied" {
		t.Fatal(run, e)
	}
	hosts, _ := s.Hosts()
	if len(hosts) != 1 || hosts[0].ConfigRevision == "" || hosts[0].DriftMinutes != 30 {
		t.Fatal(hosts)
	}
}
func TestHostOperationBackupVerifyIdempotent(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	req := operationRequest(t, s, "backup")
	run, e := s.StartHostOperation("admin", req)
	if e != nil || run.Targets[0].JobID == "" {
		t.Fatal(run, e)
	}
	job := waitReliabilityJob(t, s, run.Targets[0].JobID, finishedJob)
	b, e := s.Backup(job.ResultID)
	if e != nil || b.VerifiedAt == "" || job.Kind != "backup_verify" {
		t.Fatal(job, b, e)
	}
	again, e := s.StartHostOperation("admin", req)
	jobs, _ := s.Jobs()
	if e != nil || again.Targets[0].JobID != job.ID || len(jobs) != 1 {
		t.Fatal(again, e, len(jobs))
	}
}
func TestHostDriftRequiresNewCompleteObservationAndScopesPBS(t *testing.T) {
	s := testService(t)
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	m, e := s.Manifest(b.ID)
	if e != nil {
		t.Fatal(e)
	}
	h, _ := s.Host("host1")
	inv := m.Inventory
	inv.CapturedAt = time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	inv.Details = map[string]json.RawMessage{}
	hashes := map[string]string{}
	for _, entry := range m.Entries {
		if entry.Type != "directory" {
			hashes[entry.Path] = entry.SHA256
			if entry.Type == "symlink" {
				hashes[entry.Path] = "symlink:" + entry.Link
			}
		}
	}
	changedPath := ""
	for _, entry := range m.Entries {
		if entry.Type == "file" && strings.HasPrefix(entry.Path, "etc/") {
			changedPath = entry.Path
			hashes[entry.Path] = Hash([]byte("changed"))
			break
		}
	}
	hashes["etc/new-config"] = Hash([]byte("new"))
	inv.Details["file_hashes"], _ = json.Marshal(hashes)
	inv.Details["command_results"] = json.RawMessage(`{"file_inventory":{"state":"ok"}}`)
	h.Inventory = &inv
	h.LastProbe = now()
	s.Store.Put("hosts", h.ID, h)
	report, e := s.HostDrift(h.ID)
	if e != nil || len(report.Files) != 1 || report.Files[0].Path != changedPath {
		t.Fatal(report, e, m.Profile)
	}
	inv.Details["command_results"] = json.RawMessage(`{"file_inventory":{"state":"failed"}}`)
	h.Inventory = &inv
	s.Store.Put("hosts", h.ID, h)
	report, e = s.HostDrift(h.ID)
	if e != nil || report.State != "unknown" {
		t.Fatal(report, e)
	}
	inv.Details["command_results"] = json.RawMessage(`{"file_inventory":{"state":"ok"}}`)
	inv.CapturedAt = m.CompletedAt
	h.Inventory = &inv
	s.Store.Put("hosts", h.ID, h)
	report, e = s.HostDrift(h.ID)
	if e != nil || report.State != "unknown" {
		t.Fatal("backup inventory mistaken for live check", report, e)
	}
	if !excludedDriftPath("etc/data/.chunks/aa", Inventory{}) || !excludedDriftPath("etc/datastore/vm/100", Inventory{PBSDatastores: []PBSDatastore{{Path: "/etc/datastore"}}}) || excludedDriftPath("etc/proxmox-backup/datastore.cfg", Inventory{}) {
		t.Fatal("PBS boundary incorrect")
	}
}
func TestIncidentAcknowledgementAndNewEvidenceLifecycle(t *testing.T) {
	s := testService(t)
	old := remediationOldBackup(t, s, "host1")
	list, e := s.Incidents()
	if e != nil || len(list) != 1 || list[0].State != "new" {
		t.Fatal(list, e)
	}
	id := list[0].ID
	ack, e := s.AcknowledgeIncident(id, "admin", true)
	if e != nil || ack.State != "acknowledged" {
		t.Fatal(ack, e)
	}
	h, _ := s.Host("host1")
	h.Enabled = false
	s.SaveHost(h)
	list, e = s.Incidents()
	if e != nil || list[0].State != "acknowledged" || !list[0].Hosts[0].Muted {
		t.Fatal("pausing marked fixed", list, e)
	}
	// Later good evidence closes the incident, keeping the original stand intact.
	s.Collector = fixtureCollector{}
	b, e := s.CreateBackup(context.Background(), h.ID)
	if e != nil {
		t.Fatal(e)
	}
	list, e = s.Incidents()
	if e != nil || list[0].State != "resolved" {
		t.Fatal(list, e, b)
	}
	original, _ := s.Backup(old.ID)
	if original.Status != "partial" {
		t.Fatal("old evidence changed")
	}
	remediationOldBackup(t, s, h.ID)
	list, e = s.Incidents()
	if e != nil || list[0].State != "reopened" || list[0].ReopenCount != 1 || list[0].AcknowledgedBy != "" {
		t.Fatal(list, e)
	}
}
func TestOperationsHTTPRequiresAdminForSettings(t *testing.T) {
	s := testService(t)
	req := operationRequest(t, s, "pause")
	raw, _ := json.Marshal(req)
	r := httptest.NewRequest("POST", "/api/operations/hosts", strings.NewReader(string(raw)))
	_, e := s.operationsHTTP(User{ID: "operator", Role: "restore"}, httptest.NewRecorder(), r)
	if e == nil {
		t.Fatal("restore role changed settings")
	}
	r = httptest.NewRequest("GET", "/api/operations/helpers", nil)
	w := httptest.NewRecorder()
	handled, e := s.operationsHTTP(User{ID: "reader", Role: "reader"}, w, r)
	if !handled || e != nil || w.Code != http.StatusOK {
		t.Fatal(handled, e, w.Code)
	}
}

func TestQueuedAutomaticDriftHonorsMaintenanceBeforeStarting(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	settings, _ := s.Settings()
	settings.Parallel = 1
	s.Store.Put("settings", "main", settings)
	s.Store.Put("jobs", "busy-other", Job{ID: "busy-other", HostID: "other", Kind: "probe", State: "running"})
	h, _ := s.Host("host1")
	h.DriftMinutes = 15
	h.LastProbe = ""
	s.Store.Put("hosts", h.ID, h)
	job, e := s.queueDriftDue(h, time.Now())
	if e != nil || job.Trigger != "scheduled" {
		t.Fatal(job, e)
	}
	req := operationRequest(t, s, "maintenance")
	req.Until = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	if _, e = s.StartHostOperation("admin", req); e != nil {
		t.Fatal(e)
	}
	s.Store.Delete("jobs", "busy-other")
	done := waitReliabilityJob(t, s, job.ID, finishedJob)
	if done.State != "cancelled" || done.StartedAt != "" {
		t.Fatal("automatic probe started during maintenance", done)
	}
	h, _ = s.Host(h.ID)
	if _, e = s.queueDriftDue(h, time.Now()); e != errJobNotDue {
		t.Fatal("maintenance admitted automatic work", e)
	}
	manual, e := s.QueueDrift(h.ID)
	if e != nil {
		t.Fatal("manual read must remain possible", e)
	}
	done = waitReliabilityJob(t, s, manual.ID, finishedJob)
	if done.State != "successful" {
		t.Fatal(done)
	}
}
func TestIncidentUsesCompletionEvidenceRatherThanCaptureStart(t *testing.T) {
	s := testService(t)
	remediationOldBackup(t, s, "host1")
	list, _ := s.Incidents()
	old := list[0]
	s.Collector = fixtureCollector{}
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	m, _ := s.Manifest(b.ID)
	start, _ := time.Parse(time.RFC3339Nano, m.CreatedAt)
	end, _ := time.Parse(time.RFC3339Nano, m.CompletedAt)
	old.LastSeen = start.Add(end.Sub(start) / 2).UTC().Format(time.RFC3339Nano)
	s.Store.Put("incidents", old.ID, old)
	list, e = s.Incidents()
	if e != nil || list[0].State != "resolved" {
		t.Fatal("completed capture was not accepted as newer evidence", list, e)
	}
}
func TestDisasterImportRevokesPendingHostSettings(t *testing.T) {
	root := t.TempDir()
	store, e := OpenStore(filepath.Join(root, "catalog.db"))
	if e != nil {
		t.Fatal(e)
	}
	req := HostOperationRequest{RequestID: "imported-batch", Action: "resume", Targets: []HostOperationTarget{{HostID: "host1", Revision: strings.Repeat("a", 64)}}}
	run := HostOperationRun{ID: hostOperationID("admin", req.RequestID), ActorID: "admin", Request: req, Targets: []HostOperationTarget{{HostID: "host1", Revision: strings.Repeat("a", 64), State: "pending"}}}
	store.Put("host_operations", run.ID, run)
	raw, _ := json.Marshal(run)
	store.Close()
	pkg := &disasterPackage{Records: []disasterRecord{{Bucket: "host_operations", ID: run.ID, Value: raw, SHA256: Hash(raw)}}}
	if e = disasterIsolate(pkg, root, root); e != nil {
		t.Fatal(e)
	}
	store, e = OpenStore(filepath.Join(root, "catalog.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	var got HostOperationRun
	store.Get("host_operations", run.ID, &got)
	if got.Targets[0].State != "skipped" {
		t.Fatal("imported authorization still pending", got)
	}
}

func TestBatchReplayDoesNotRecaptureAfterCompletedJobWasRemoved(t *testing.T) {
	s := testService(t)
	defer s.StopJobs(context.Background())
	req := operationRequest(t, s, "backup")
	run, e := s.StartHostOperation("admin", req)
	if e != nil {
		t.Fatal(e)
	}
	job := waitReliabilityJob(t, s, run.Targets[0].JobID, finishedJob)
	// Simulate the durable receipt still pending after a lost checkpoint.
	run.Targets[0].State = "pending"
	run.Targets[0].JobID = ""
	s.Store.Put("host_operations", run.ID, run)
	// Wait for worker teardown, as in the normal guarded history action.
	for deadline := time.Now().Add(time.Second); ; {
		e = s.DeleteJob(job.ID)
		if e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(e)
		}
		time.Sleep(time.Millisecond)
	}
	again, e := s.StartHostOperation("admin", req)
	backups, _ := s.ListBackups("host1")
	jobs, _ := s.Jobs()
	if e != nil || again.Targets[0].State != "skipped" || len(backups) != 1 || len(jobs) != 0 {
		t.Fatal("history deletion caused replay", again, e, len(backups), len(jobs))
	}
}

type scopedDriftCollector struct{ fixtureCollector }

func (c scopedDriftCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	col, e := c.fixtureCollector.Collect(ctx, h, dest)
	col.Profile = &CollectionProfile{Version: 1, Roots: []string{"/etc"}, DependencyScanComplete: true}
	col.Entries = append(col.Entries, Entry{Path: "etc/config-link", Type: "symlink", Link: "network/interfaces", Mode: 0777})
	return col, e
}
func TestLiveDriftNormalizesLinksAndKeepsUnknownHashesOutOfChangeCounts(t *testing.T) {
	s := testService(t)
	s.Collector = scopedDriftCollector{}
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	m, _ := s.Manifest(b.ID)
	h, _ := s.Host("host1")
	inv := m.Inventory
	inv.CapturedAt = time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	hashes := map[string]string{"etc/config-link": "symlink:network/interfaces", "etc/new.conf": Hash([]byte("new")), "etc/unreadable.conf": "unreadable", "var/lib/guest.img": Hash([]byte("data")), "etc/.chunks/payload": Hash([]byte("chunks"))}
	raw, _ := json.Marshal(hashes)
	inv.Details = map[string]json.RawMessage{"file_hashes": raw, "command_results": json.RawMessage(`{"file_inventory":{"state":"ok"}}`)}
	h.Inventory = &inv
	h.LastProbe = now()
	s.Store.Put("hosts", h.ID, h)
	r, e := s.HostDrift(h.ID)
	if e != nil || r.State != "partial" || r.Unknown != 1 || len(r.Files) != 2 {
		t.Fatal(r, e)
	}
	states := map[string]string{}
	for _, f := range r.Files {
		states[f.Path] = f.State
	}
	if states["etc/network/interfaces"] != "removed" || states["etc/new.conf"] != "added" {
		t.Fatal(states)
	}
	inv.Hostname = "other-host"
	h.Inventory = &inv
	s.Store.Put("hosts", h.ID, h)
	r, e = s.HostDrift(h.ID)
	if e != nil || r.State != "unknown" || len(r.Files) != 0 {
		t.Fatal("source identity was ignored", r, e)
	}
}
