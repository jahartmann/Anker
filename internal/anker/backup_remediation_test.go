package anker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type unfixedRemediationCollector struct{ fixtureCollector }

func (c unfixedRemediationCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	col, err := c.fixtureCollector.Collect(ctx, h, dest)
	col.Warnings = []string{"Cannot capture /" + escapedMountPath + ": invalid path"}
	return col, err
}
func remediationOldBackup(t *testing.T, s *Service, hostID string) Backup {
	t.Helper()
	s.Collector = unfixedRemediationCollector{}
	b, err := s.CreateBackup(context.Background(), hostID)
	if err != nil || b.Status != "partial" {
		t.Fatal(b, err)
	}
	return b
}
func TestRemediationBatchRecapturesAndVerifiesWithoutReenrollment(t *testing.T) {
	s := testService(t)
	old := remediationOldBackup(t, s, "host1")
	before, _ := s.Host("host1")
	manifestPath := filepath.Join(s.backupDir(old), "manifest.json")
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	s.Collector = legacySystemdCollector{}
	defer s.StopJobs(context.Background())
	request := RemediationRequest{RequestID: "one-click", Sources: []RemediationSource{{HostID: "host1", BackupID: old.ID}}}
	run, err := s.StartBackupRemediation("admin", request)
	if err != nil || len(run.Targets) != 1 || run.Targets[0].State != "queued" {
		t.Fatal(run, err)
	}
	job := waitReliabilityJob(t, s, run.Targets[0].JobID, finishedJob)
	b, err := s.Backup(job.ResultID)
	if err != nil || job.State != "successful" || b.Status != "successful" || b.VerifiedAt == "" || b.VerificationAttempt == nil || b.VerificationAttempt.State != "ok" {
		t.Fatal(job, b, err)
	}
	if last := job.Stages[len(job.Stages)-1]; last.ID != "completion" || last.State != "successful" {
		t.Fatal(job.Stages)
	}
	after, _ := s.Host("host1")
	before.Inventory, after.Inventory = nil, nil
	before.LastProbe, after.LastProbe = "", ""
	before.ProbeError, after.ProbeError = "", ""
	if !reflect.DeepEqual(before, after) {
		t.Fatal("host registration or profile changed")
	}
	unchanged, err := os.ReadFile(manifestPath)
	oldAgain, _ := s.Backup(old.ID)
	if err != nil || !bytes.Equal(original, unchanged) || oldAgain.Status != "partial" || len(oldAgain.Warnings) != 1 {
		t.Fatal("old evidence changed", err)
	}
	replay, err := s.StartBackupRemediation("admin", request)
	jobs, _ := s.Jobs()
	backups, _ := s.ListBackups("host1")
	if err != nil || replay.Targets[0].JobID != job.ID || len(jobs) != 1 || len(backups) != 2 {
		t.Fatal("replay captured twice", replay, err, len(jobs), len(backups))
	}
	plan, err := s.BackupRemediationPlan()
	if err != nil || len(plan.Targets) != 0 {
		t.Fatal("completed host still targeted", plan, err)
	}
}
func TestRemediationPlanUsesLatestEvidenceAndSkipsBusyPausedAndUnknown(t *testing.T) {
	s := testService(t)
	remediationOldBackup(t, s, "host1")
	template, _ := s.Host("host1")
	for _, id := range []string{"paused", "busy", "unknown", "fixed"} {
		h := template
		h.ID = id
		h.Name = id
		h.Enabled = id != "paused"
		if err := s.SaveHost(h); err != nil {
			t.Fatal(err)
		}
	}
	paused := remediationOldBackup(t, s, "paused")
	busy := remediationOldBackup(t, s, "busy")
	unknown := remediationOldBackup(t, s, "unknown")
	unknown.Warnings = []string{"Cannot capture /etc/private.conf: permission denied"}
	if err := s.Store.Put("backups", unknown.ID, unknown); err != nil {
		t.Fatal(err)
	}
	remediationOldBackup(t, s, "fixed")
	s.Collector = fixtureCollector{}
	if _, err := s.CreateBackup(context.Background(), "fixed"); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Put("jobs", "active-backup", Job{ID: "active-backup", HostID: "busy", Kind: "backup", State: "running"}); err != nil {
		t.Fatal(err)
	}
	plan, err := s.BackupRemediationPlan()
	if err != nil || len(plan.Targets) != 3 || plan.OtherHosts != 1 {
		t.Fatal(plan, err)
	}
	states := map[string]string{}
	for _, target := range plan.Targets {
		states[target.HostID] = target.State
	}
	if states["host1"] != "ready" || states["paused"] != "paused" || states["busy"] != "busy" {
		t.Fatal(states)
	}
	request := RemediationRequest{RequestID: "skips", Sources: []RemediationSource{{HostID: "host1", BackupID: ID()}, {HostID: "paused", BackupID: paused.ID}, {HostID: "busy", BackupID: busy.ID}, {HostID: "unknown", BackupID: unknown.ID}}}
	run, err := s.StartBackupRemediation("admin", request)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"skipped", "paused", "busy", "skipped"} {
		if run.Targets[i].State != want {
			t.Fatal(run)
		}
	}
	jobs, _ := s.Jobs()
	if len(jobs) != 1 {
		t.Fatal("unexpected jobs", jobs)
	}
	if err := s.Store.Delete("jobs", "active-backup"); err != nil {
		t.Fatal(err)
	}
	request = RemediationRequest{RequestID: "paused-explicit", IncludePaused: true, Sources: []RemediationSource{{HostID: "paused", BackupID: paused.ID}}}
	s.Collector = legacySystemdCollector{}
	defer s.StopJobs(context.Background())
	run, err = s.StartBackupRemediation("admin", request)
	if err != nil || run.Targets[0].State != "queued" {
		t.Fatal(run, err)
	}
	waitReliabilityJob(t, s, run.Targets[0].JobID, finishedJob)
	host, _ := s.Host("paused")
	if host.Enabled {
		t.Fatal("paused schedule was enabled")
	}
}
func TestRemediationConcurrentRequestsAndCrashReceiptNeverDuplicate(t *testing.T) {
	s := testService(t)
	old := remediationOldBackup(t, s, "host1")
	s.Collector = legacySystemdCollector{}
	defer s.StopJobs(context.Background())
	request := RemediationRequest{RequestID: "same-request", Sources: []RemediationSource{{HostID: "host1", BackupID: old.ID}}}
	var wg sync.WaitGroup
	replies := make(chan RemediationRun, 6)
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := s.StartBackupRemediation("admin", request); replies <- r; errs <- e }()
	}
	wg.Wait()
	close(replies)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var saved RemediationRun
	for reply := range replies {
		if saved.ID != "" && reply.Targets[0].JobID != saved.Targets[0].JobID {
			t.Fatal("different job IDs")
		}
		saved = reply
	}
	waitReliabilityJob(t, s, saved.Targets[0].JobID, finishedJob)
	// Simulate death after the durable job insert but before receipt update.
	saved.Targets[0].State = "pending"
	saved.Targets[0].JobID = ""
	if err := s.Store.Put("backup_remediation", saved.ID, saved); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.StartBackupRemediation("admin", request)
	jobs, _ := s.Jobs()
	if err != nil || len(jobs) != 1 || resumed.Targets[0].JobID != jobs[0].ID || resumed.Targets[0].State != "queued" {
		t.Fatal(resumed, err, jobs)
	}
	changed := request
	changed.IncludePaused = true
	if _, err := s.StartBackupRemediation("admin", changed); err == nil {
		t.Fatal("idempotency key accepted changed request")
	}
	if _, err := s.BackupRemediationRun("another-user", request.RequestID); err == nil {
		t.Fatal("another user accessed the receipt")
	}
}
func TestRemediationPreservesRealWarningsAndDoesNotRepeatCapture(t *testing.T) {
	s := testService(t)
	old := remediationOldBackup(t, s, "host1")
	defer s.StopJobs(context.Background())
	run, err := s.StartBackupRemediation("admin", RemediationRequest{RequestID: "no-evidence", Sources: []RemediationSource{{HostID: "host1", BackupID: old.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	job := waitReliabilityJob(t, s, run.Targets[0].JobID, finishedJob)
	b, err := s.Backup(job.ResultID)
	if err != nil || job.State != "failed" || job.Attempts != 1 || b.Status != "partial" || len(b.Warnings) != 1 || b.VerificationAttempt == nil || b.VerificationAttempt.State != "ok" {
		t.Fatal(job, b, err)
	}
	if len(job.Stages) < 2 || job.Stages[len(job.Stages)-2].ID != "integrity" || job.Stages[len(job.Stages)-2].State != "successful" || job.Stages[len(job.Stages)-1].ID != "completion" || job.Stages[len(job.Stages)-1].State != "failed" {
		t.Fatal("valid byte check incorrectly shown as failed", job.Stages)
	}
	backups, _ := s.ListBackups("host1")
	if len(backups) != 2 {
		t.Fatal("failure caused repeated capture")
	}
}
func TestRemediationHTTPPermissionAndSafePreview(t *testing.T) {
	s := testService(t)
	old := remediationOldBackup(t, s, "host1")
	defer s.StopJobs(context.Background())
	auth := NewAuth(s.Store)
	err := auth.CreateUser("reader", "long-test-password", "reader", false)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := auth.Login("reader", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(RemediationRequest{RequestID: "reader-request", Sources: []RemediationSource{{HostID: "host1", BackupID: old.ID}}})
	r := httptest.NewRequest("POST", "/api/backup-remediation", bytes.NewReader(body))
	r.Header.Set("X-Anker-Request", "1")
	r.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
	w := httptest.NewRecorder()
	Handler(s, auth, false).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	r = httptest.NewRequest("GET", "/api/backup-remediation", nil)
	w = httptest.NewRecorder()
	Handler(s, auth, true).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "systemd_path") || strings.Contains(w.Body.String(), "key_path") || strings.Contains(w.Body.String(), escapedMountPath) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, request := range []RemediationRequest{{RequestID: "bad", Sources: nil}, {RequestID: "duplicate", Sources: []RemediationSource{{HostID: "host1", BackupID: old.ID}, {HostID: "host1", BackupID: old.ID}}}} {
		if _, err := s.StartBackupRemediation("admin", request); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	jobs, _ := s.Jobs()
	if len(jobs) != 0 {
		t.Fatal("invalid or reader request started jobs")
	}
	s.Collector = legacySystemdCollector{}
	body, _ = json.Marshal(RemediationRequest{RequestID: "admin-request", Sources: []RemediationSource{{HostID: "host1", BackupID: old.ID}}})
	r = httptest.NewRequest("POST", "/api/backup-remediation", bytes.NewReader(body))
	r.Header.Set("X-Anker-Request", "1")
	w = httptest.NewRecorder()
	Handler(s, auth, true).ServeHTTP(w, r)
	var run RemediationRun
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &run) != nil || len(run.Targets) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	waitReliabilityJob(t, s, run.Targets[0].JobID, finishedJob)
	r = httptest.NewRequest("GET", "/api/backup-remediation/admin-request", nil)
	w = httptest.NewRecorder()
	Handler(s, auth, true).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), run.ID) {
		t.Fatal(w.Code, w.Body.String())
	}

}

func TestRemediationAdmissionRechecksFreshEvidenceUnderJobLock(t *testing.T) {
	s := testService(t)
	old := remediationOldBackup(t, s, "host1")
	s.Collector = fixtureCollector{}
	if _, err := s.CreateBackup(context.Background(), "host1"); err != nil {
		t.Fatal(err)
	}
	_, err := s.queueOperation("host1", "backup", nil, "", "", "guarded-job", s.remediationRunner("host1"), s.remediationAdmission(RemediationSource{HostID: "host1", BackupID: old.ID}, false))
	if !errors.Is(err, errRemediationChanged) {
		t.Fatal("stale preview admitted", err)
	}
	host, _ := s.Host("host1")
	host.Enabled = false
	if err := s.SaveHost(host); err != nil {
		t.Fatal(err)
	}
	_, err = s.queueOperation("host1", "backup", nil, "", "", "guarded-paused", s.remediationRunner("host1"), s.remediationAdmission(RemediationSource{HostID: "host1", BackupID: old.ID}, false))
	if !errors.Is(err, errRemediationPaused) {
		t.Fatal("newly paused host admitted", err)
	}
	jobs, _ := s.Jobs()
	if len(jobs) != 0 {
		t.Fatal("admission failure created work", jobs)
	}
}

func TestRemediationRejectsStoredSelectionMismatchBeforeAdmission(t *testing.T) {
	s := testService(t)
	old := remediationOldBackup(t, s, "host1")
	request := RemediationRequest{RequestID: "damaged-receipt", Sources: []RemediationSource{{HostID: "host1", BackupID: old.ID}}}
	run := RemediationRun{ID: remediationRunID("admin", request.RequestID), ActorID: "admin", Request: request, Targets: []RemediationTarget{{RemediationSource: RemediationSource{HostID: "another-host", BackupID: old.ID}, State: "pending"}}}
	if err := s.Store.Put("backup_remediation", run.ID, run); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartBackupRemediation("admin", request); err == nil {
		t.Fatal("mismatched persistent selection accepted")
	}
	jobs, _ := s.Jobs()
	if len(jobs) != 0 {
		t.Fatal("malformed receipt created work")
	}
}

func TestDisasterIsolationClosesStagesAndRevokesPendingRemediation(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	source := RemediationSource{HostID: "host1", BackupID: "old-backup"}
	request := RemediationRequest{RequestID: "before-import", IncludePaused: true, Sources: []RemediationSource{source}}
	run := RemediationRun{ID: remediationRunID("admin", request.RequestID), ActorID: "admin", Request: request, Targets: []RemediationTarget{{RemediationSource: source, State: "pending"}}}
	job := Job{ID: "imported-worker", Kind: "backup", HostID: "host1", State: "running", RetryAt: "2099-01-01T00:00:00Z", Stages: []JobStage{{ID: "collect", State: "running", Attempt: 1}}}
	pkg := &disasterPackage{}
	for bucket, value := range map[string]any{"backup_remediation": run, "jobs": job} {
		id := run.ID
		if bucket == "jobs" {
			id = job.ID
		}
		raw, _ := json.Marshal(value)
		if bucket == "jobs" {
			var object map[string]any
			if err := json.Unmarshal(raw, &object); err != nil {
				t.Fatal(err)
			}
			object["stages"].([]any)[0].(map[string]any)["future_evidence"] = "preserve"
			value = object
			raw, _ = json.Marshal(value)
		}
		if err := store.Put(bucket, id, value); err != nil {
			t.Fatal(err)
		}
		pkg.Records = append(pkg.Records, disasterRecord{Bucket: bucket, ID: id, Value: raw, SHA256: Hash(raw)})
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := disasterIsolate(pkg, root, root); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var saved RemediationRun
	var stopped Job
	if err := store.Get("backup_remediation", run.ID, &saved); err != nil {
		t.Fatal(err)
	}
	if err := store.Get("jobs", job.ID, &stopped); err != nil {
		t.Fatal(err)
	}
	var preserved json.RawMessage
	if err := store.Get("jobs", job.ID, &preserved); err != nil || !bytes.Contains(preserved, []byte(`"future_evidence":"preserve"`)) {
		t.Fatal("future stage evidence lost", err, string(preserved))
	}
	if saved.Targets[0].State != "skipped" || stopped.State != "interrupted" || stopped.RetryAt != "" || stopped.Stages[0].State != "interrupted" {
		t.Fatal(saved, stopped)
	}
}
