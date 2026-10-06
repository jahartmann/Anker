package anker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestLateScheduleNeverFallsIntoUnreachableNextDay(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	h.Schedule = "23:59"
	s.SaveHost(h)
	defer s.StopJobs(context.Background())
	loc, _ := time.LoadLocation("Europe/Berlin")
	if err := NewScheduler(s).Tick(time.Date(2026, 10, 5, 23, 59, 59, 0, loc)); err != nil {
		t.Fatal(err)
	}
	jobs, _ := s.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("late backup permanently skipped: %d jobs", len(jobs))
	}
}

type editedDuringBackup struct {
	fixtureCollector
	entered, resume chan struct{}
}

func (c editedDuringBackup) Collect(ctx context.Context, h Host, d string) (Collection, error) {
	close(c.entered)
	<-c.resume
	return c.fixtureCollector.Collect(ctx, h, d)
}
func TestBackupNeverOverwritesConcurrentHostEdit(t *testing.T) {
	s := testService(t)
	c := editedDuringBackup{entered: make(chan struct{}), resume: make(chan struct{})}
	s.Collector = c
	done := make(chan error, 1)
	go func() { _, err := s.CreateBackup(context.Background(), "host1"); done <- err }()
	<-c.entered
	h, _ := s.Host("host1")
	h.Group = "new group"
	h.Enabled = false
	if err := s.SaveHost(h); err != nil {
		t.Fatal(err)
	}
	close(c.resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	h, _ = s.Host("host1")
	if h.Group != "new group" || h.Enabled {
		t.Fatalf("operator edit lost: %+v", h)
	}
}
func TestSavingHostDoesNotResetLatestProbe(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	s.Collector = fixtureCollector{}
	if _, err := s.CreateBackup(context.Background(), h.ID); err != nil {
		t.Fatal(err)
	}
	h.Group = "from old open form"
	if err := s.SaveHost(h); err != nil {
		t.Fatal(err)
	}
	h, _ = s.Host(h.ID)
	if h.Inventory == nil || h.LastProbe == "" {
		t.Fatal("old form erased latest inventory")
	}
}
func TestPlanExportRejectsTamperedPreparedFiles(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	s.Collector = targetCollector{inv: Inventory{PVEVersion: "8.4", Interfaces: []Interface{{Name: "eno1"}}, Details: map[string]json.RawMessage{"file_hashes": json.RawMessage(`{}`)}}}
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "host1", Scenario: "files", Files: []string{"etc/network/interfaces"}, ConsoleConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "plans", p.ID, "prepared-files/etc/network/interfaces"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := s.ExportPlan(p.ID, &out); err == nil {
		t.Fatal("corrupt prepared config downloaded")
	}
	if out.Len() != 0 {
		t.Fatal("export started before validation")
	}
}
func TestUnarchiveRejectsCorruptExistingFiles(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	if err := s.ArchiveBackup(b.ID); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.backupDir(b), "files/etc/network/interfaces")
	os.MkdirAll(filepath.Dir(p), 0700)
	os.WriteFile(p, []byte("crash leftover"), 0600)
	if err := s.EnsureReadable(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyBackup(b.ID); err != nil {
		t.Fatalf("validated archive was replaced by corrupt leftover: %v", err)
	}
}
func TestArchiveDoesNotRetainDuplicateAfterUnpacking(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	if err := s.ArchiveBackup(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureReadable(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.backupDir(b), "archive.tar.gz")); !os.IsNotExist(err) {
		t.Fatal("duplicate archive silently consumes space after unpacking")
	}
}
func TestFileDownloadPreservesRawBytesAndRejectsCorruption(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	handler := Handler(s, NewAuth(s.Store), true)
	url := "/api/backups/" + b.ID + "/file-download?path=etc/network/interfaces"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	data, _, _ := s.ReadFile(b.ID, "etc/network/interfaces")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data) || w.Header().Get("Content-Disposition") == "" {
		t.Fatalf("individual download: %d %s", w.Code, w.Body.String())
	}
	os.WriteFile(filepath.Join(s.backupDir(b), "files/etc/network/interfaces"), []byte("corrupt"), 0600)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	if w.Code == 200 || w.Header().Get("Content-Disposition") != "" {
		t.Fatal("corrupt download accepted")
	}
}
func TestUnknownBackupHealthCannotRemainSuccessful(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	os.WriteFile(filepath.Join(s.backupDir(b), "files/etc/network/interfaces"), []byte("corrupt"), 0600)
	if err := s.VerifyBackup(b.ID); err == nil {
		t.Fatal("missing checksum rejection")
	}
	got, _ := s.Backup(b.ID)
	if got.Status == "successful" {
		t.Fatal("known corrupt backup still shown as secured")
	}
}

func TestLowSpaceStopsUnpackingBeforeCreatingFiles(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	if err := s.ArchiveBackup(b.ID); err != nil {
		t.Fatal(err)
	}
	s.diskUsage = func() (DiskUsage, error) { return DiskUsage{Total: 1 << 30, Free: 1, UsedPercent: 99}, nil }
	if err := s.EnsureReadable(b.ID); err == nil {
		t.Fatal("unpacking started on full disk")
	}
	if _, err := os.Stat(filepath.Join(s.backupDir(b), "files")); !os.IsNotExist(err) {
		t.Fatal("partial tree on full disk")
	}
	if _, err := os.Stat(filepath.Join(s.backupDir(b), "archive.tar.gz")); err != nil {
		t.Fatal("archive lost on full disk")
	}
}
func TestReaderFileDownloadsRespectContentDetectedSecrets(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	a := NewAuth(s.Store)
	a.CreateUser("reader", "long-test-password", "reader", false)
	token, _, _ := a.Login("reader", "long-test-password")
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/backups/"+b.ID+"/file-download?path=etc/network/interfaces", nil)
		r.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
		w := httptest.NewRecorder()
		Handler(s, a, false).ServeHTTP(w, r)
		return w
	}
	if w := request(); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	data := []byte("wpa-psk = private-secret\n")
	os.WriteFile(filepath.Join(s.backupDir(b), "files/etc/network/interfaces"), data, 0600)
	m, _ := s.Manifest(b.ID)
	m.Entries[0].SHA256 = Hash(data)
	m.Entries[0].Size = int64(len(data))
	m.Entries[0].Secret = false
	writeJSON(filepath.Join(s.backupDir(b), "manifest.json"), m)
	raw, _ := os.ReadFile(filepath.Join(s.backupDir(b), "manifest.json"))
	b.ManifestSHA = Hash(raw)
	s.Store.Put("backups", b.ID, b)
	if w := request(); w.Code != 403 || bytes.Contains(w.Body.Bytes(), []byte("private-secret")) {
		t.Fatal("secret download leaked", w.Code, w.Body.String())
	}
}
func TestLargeSingleFileDownloadDoesNotNeedPreview(t *testing.T) {
	s := testService(t)
	s.Collector = extendedCollector{large: true}
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	Handler(s, NewAuth(s.Store), true).ServeHTTP(w, httptest.NewRequest("GET", "/api/backups/"+b.ID+"/file-download?path=etc/aaa.conf", nil))
	if w.Code != 200 || w.Body.Len() != 9<<20 {
		t.Fatal(w.Code, w.Body.Len())
	}
}
func TestCancelledQueuedJobNeverStartsCollector(t *testing.T) {
	s := testService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	j := Job{ID: "cancelled", State: "queued", Kind: "backup"}
	s.Store.Put("jobs", j.ID, j)
	s.executeJob(ctx, j, func(context.Context) (string, error) { called = true; return "", nil })
	if called {
		t.Fatal("cancelled collector started")
	}
	var got Job
	s.Store.Get("jobs", j.ID, &got)
	if got.State != "cancelled" {
		t.Fatal(got)
	}
}
func TestNinetyDayRetentionKeepsLatestPinAndPlanSource(t *testing.T) {
	s := testService(t)
	at := time.Date(2027, 1, 5, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 90; i++ {
		copy, err := s.CreateBackup(context.Background(), "host1")
		if err != nil {
			t.Fatal(err)
		}
		copy.CreatedAt = at.Add(-time.Duration(i) * 24 * time.Hour).Format(time.RFC3339Nano)
		copy.Pinned = i == 89
		s.Store.Put("backups", copy.ID, copy)
		if i == 88 {
			s.Store.Put("plans", "protected", Plan{ID: "protected", BackupID: copy.ID})
		}
	}
	v, _ := s.Settings()
	v.Daily = 7
	v.Weekly = 3
	v.Monthly = 2
	v.ArchiveDays = 0
	s.SaveSettings(v)
	if err := s.Maintain(at); err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListBackups("host1")
	if len(all) > 15 {
		t.Fatal("retention never prunes", len(all))
	}
	var pinned, referenced, latest bool
	for _, v := range all {
		pinned = pinned || v.Pinned
		referenced = referenced || v.CreatedAt == at.Add(-88*24*time.Hour).Format(time.RFC3339Nano)
		latest = latest || v.CreatedAt == at.Format(time.RFC3339Nano)
	}
	if !pinned || !referenced || !latest {
		t.Fatal("required recovery source pruned", pinned, referenced, latest)
	}
}
func TestExpiredSessionsAreEvictedDuringLogin(t *testing.T) {
	s := testService(t)
	a := NewAuth(s.Store)
	a.CreateUser("reader", "long-test-password", "reader", false)
	s.Store.Put("sessions", "old", SessionInfo{UserID: "reader", Expires: time.Now().Add(-time.Hour)})
	if _, _, err := a.Login("reader", "long-test-password"); err != nil {
		t.Fatal(err)
	}
	var old SessionInfo
	if err := s.Store.Get("sessions", "old", &old); err == nil {
		t.Fatal("expired sessions accumulate over months")
	}
}
func TestOverdueNotificationSurvivesRestartAndRecovers(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	var messages atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { messages.Add(1); w.WriteHeader(204) }))
	defer server.Close()
	v, _ := s.Settings()
	v.Webhook = server.URL
	s.SaveSettings(v)
	at := time.Now().Add(48 * time.Hour)
	if err := s.CheckStaleBackups(at); err != nil {
		t.Fatal(err)
	}
	s2, err := NewService(s.Root, s.Store, s.Collector)
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.CheckStaleBackups(at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if messages.Load() != 1 {
		t.Fatal("duplicate/missing overdue warning", messages.Load())
	}
	b.CreatedAt = at.Format(time.RFC3339Nano)
	s.Store.Put("backups", b.ID, b)
	if err := s2.CheckStaleBackups(at.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if messages.Load() != 2 {
		t.Fatal("recovery notification missing", messages.Load())
	}
}
func TestCrashAfterUnpackingNeedsNoSecondFullCopy(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	data, _, _ := s.ReadFile(b.ID, "etc/network/interfaces")
	if err := s.ArchiveBackup(b.ID); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.backupDir(b), "files/etc/network/interfaces")
	os.MkdirAll(filepath.Dir(p), 0700)
	os.WriteFile(p, data, 0600)
	s.diskUsage = func() (DiskUsage, error) { return DiskUsage{Total: 1 << 30, Free: 1, UsedPercent: 99}, nil }
	if err := s.EnsureReadable(b.ID); err != nil {
		t.Fatal("valid expanded tree unnecessarily unpacked again", err)
	}
	if err := s.VerifyBackup(b.ID); err != nil {
		t.Fatal(err)
	}
}
func TestRunningAttemptIsVisibleBeforeCollectorStarts(t *testing.T) {
	s := testService(t)
	j := Job{ID: ID(), State: "queued", Kind: "backup"}
	s.Store.Put("jobs", j.ID, j)
	s.executeJob(context.Background(), j, func(context.Context) (string, error) {
		var got Job
		s.Store.Get("jobs", j.ID, &got)
		if got.Attempts != 1 || got.State != "running" {
			t.Errorf("running status misleading: %+v", got)
		}
		return "", nil
	})
}
func TestReindexRecoversValidArchiveDespitePartialExpandedTree(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	if err := s.ArchiveBackup(b.ID); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(s.backupDir(b), "files/etc/network/interfaces")
	os.MkdirAll(filepath.Dir(p), 0700)
	os.WriteFile(p, []byte("crash"), 0600)
	s.Store.Delete("backups", b.ID)
	if _, err := s.Reindex(); err != nil {
		t.Fatal("valid archive cannot rebuild lost catalog", err)
	}
	if err := s.EnsureReadable(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyBackup(b.ID); err != nil {
		t.Fatal(err)
	}
}
func TestHelperRequestLimitBlocksOversizedPlanBeforeApply(t *testing.T) {
	s := testService(t)
	s.Collector = extendedCollector{large: true}
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	s.Collector = targetCollector{inv: Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"file_hashes": json.RawMessage(`{}`)}}}
	// Inventory plus base64 file data must fit the fixed host-helper protocol.
	padding := bytes.Repeat([]byte("x"), 16<<20)
	inv := s.Collector.(targetCollector).inv
	inv.Details["packages"], _ = json.Marshal(string(padding))
	s.Collector = targetCollector{inv: inv}
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "host1", Scenario: "files", Files: []string{"etc/aaa.conf", "etc/zzz.conf"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.State == "ready" {
		t.Fatal("plan ready even though helper will reject request size")
	}
}
func TestStartupCleansOnlyUnpublishedStagingArtifacts(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	for _, name := range []string{ID(), "archive-crash.gz", "archive-crash"} {
		os.MkdirAll(filepath.Join(s.Root, "staging", name), 0700)
	}
	if err := s.RecoverStaging(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(s.Root, "staging"))
	if len(entries) != 0 {
		t.Fatal("crash staging grows without cleanup", len(entries))
	}
	if err := s.VerifyBackup(b.ID); err != nil {
		t.Fatal("published backup removed", err)
	}
}
func TestNotificationTestCannotSucceedWithoutDestination(t *testing.T) {
	s := testService(t)
	settings, _ := s.Settings()
	if err := s.SendNotification(settings, "test"); err == nil {
		t.Fatal("notification test claims success but sends nothing")
	}
}
func TestSettingsRejectInvalidNotificationDestinations(t *testing.T) {
	s := testService(t)
	settings, _ := s.Settings()
	settings.Webhook = "ftp://example.invalid"
	if err := s.SaveSettings(settings); err == nil {
		t.Fatal("unsupported webhook accepted")
	}
	settings.Webhook = ""
	settings.SMTPServer = "mail.invalid:465"
	settings.MailTo = "recipient@example.invalid"
	settings.MailFrom = "bad\r\nheader"
	if err := s.SaveSettings(settings); err == nil {
		t.Fatal("invalid sender accepted")
	}
}
func TestCorruptSettingsNeverResetRetentionSilently(t *testing.T) {
	s := testService(t)
	if _, err := s.Store.db.Exec(`UPDATE records SET value=? WHERE bucket='settings'`, []byte(`broken JSON`)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(s.Root, s.Store, s.Collector); err == nil {
		t.Fatal("corrupt retention settings silently replaced by defaults")
	}
}
func TestChangedHostAddressNeverReceivesOldProbeInventory(t *testing.T) {
	s := testService(t)
	c := editedDuringBackup{entered: make(chan struct{}), resume: make(chan struct{})}
	s.Collector = c
	done := make(chan error, 1)
	go func() { _, err := s.CreateBackup(context.Background(), "host1"); done <- err }()
	<-c.entered
	h, _ := s.Host("host1")
	h.Address = "192.0.2.9"
	s.SaveHost(h)
	close(c.resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	h, _ = s.Host("host1")
	if h.Inventory != nil || h.LastProbe != "" {
		t.Fatal("old machine inventory assigned to new SSH target")
	}
}
func TestSchedulerNeverHidesMaintenanceErrors(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	h.Enabled = false
	s.SaveHost(h)
	s.Store.Put("plans", "broken", map[string]any{"steps": "invalid"})
	if err := NewScheduler(s).Tick(time.Now()); err == nil {
		t.Fatal("retention/catalog error silently ignored")
	}
}
