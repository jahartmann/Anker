package anker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func rotationBackups(t *testing.T, s *Service, count int) []Backup {
	t.Helper()
	settings, _ := s.Settings()
	settings.ArchiveDays = 0
	s.SaveSettings(settings)
	var out []Backup
	for i := 0; i < count; i++ {
		b, e := s.CreateBackup(context.Background(), "host1")
		if e != nil {
			t.Fatal(e)
		}
		b.CreatedAt = time.Now().Add(time.Duration(i-count) * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
		b.VerifiedAt = ""
		if e = s.Store.Put("backups", b.ID, b); e != nil {
			t.Fatal(e)
		}
		out = append(out, b)
	}
	return out
}

func TestIntegrityRotationDetectsOlderPinnedAndBoundsChecks(t *testing.T) {
	s := testService(t)
	bs := rotationBackups(t, s, 4)
	bs[0].Pinned = true
	s.Store.Put("backups", bs[0].ID, bs[0])
	if e := os.WriteFile(filepath.Join(s.backupDir(bs[0]), "files/etc/network/interfaces"), []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Store.db.Exec(`CREATE TABLE verification_writes(id TEXT); CREATE TRIGGER count_verification AFTER UPDATE ON records WHEN NEW.bucket='backups' BEGIN INSERT INTO verification_writes VALUES(NEW.id); END`); e != nil {
		t.Fatal(e)
	}
	if e := s.Maintain(time.Now()); e != nil {
		t.Fatal(e)
	}
	old, _ := s.Backup(bs[0].ID)
	if old.Status != "damaged" {
		t.Fatalf("older protected corruption missed: %+v", old)
	}
	var n int
	s.Store.db.QueryRow(`SELECT count(*) FROM verification_writes`).Scan(&n)
	if n != 2 {
		t.Fatalf("want newest plus one older check; got %d writes", n)
	}
	for _, b := range bs[1:3] {
		got, _ := s.Backup(b.ID)
		if got.VerifiedAt != "" {
			t.Fatal("unbounded verification", got.ID)
		}
	}
}

func TestIntegrityRotationPersistsAttemptsAndDoesNotDoubleCheckNewest(t *testing.T) {
	s := testService(t)
	bs := rotationBackups(t, s, 3)
	for i := 0; i < 2; i++ {
		bs[i].Pinned = true
		s.Store.Put("backups", bs[i].ID, bs[i])
	}
	if e := s.Maintain(time.Now()); e != nil {
		t.Fatal(e)
	}
	first, _ := s.Backup(bs[0].ID)
	second, _ := s.Backup(bs[1].ID)
	if (first.VerifiedAt == "") == (second.VerifiedAt == "") {
		t.Fatal("expected exactly one older checked", first, second)
	}
	restarted, e := NewService(s.Root, s.Store, fixtureCollector{})
	if e != nil {
		t.Fatal(e)
	}
	if e = restarted.Maintain(time.Now().Add(24 * time.Hour)); e != nil {
		t.Fatal(e)
	}
	for _, b := range bs {
		got, _ := s.Backup(b.ID)
		if got.VerifiedAt == "" {
			t.Fatal("rotation did not advance across restart", got.ID)
		}
	}
}

func TestIntegritySpaceAndIOFailureAreDeferred(t *testing.T) {
	for _, diskErr := range []error{nil, syscall.EIO, syscall.EROFS, syscall.EDQUOT} {
		t.Run(func() string {
			if diskErr == nil {
				return "space"
			}
			return diskErr.Error()
		}(), func(t *testing.T) {
			s := testService(t)
			b := rotationBackups(t, s, 1)[0]
			if e := s.ArchiveBackup(b.ID); e != nil {
				t.Fatal(e)
			}
			before, _ := s.Backup(b.ID)
			s.diskUsage = func() (DiskUsage, error) { return DiskUsage{Free: 0}, diskErr }
			if e := s.VerifyBackup(b.ID); e == nil {
				t.Fatal("environment failure did not defer verification")
			}
			after, _ := s.Backup(b.ID)
			if after.Status != "successful" || after.VerificationError != "" || after.VerifiedAt != before.VerifiedAt {
				t.Fatal("environment failure marked corruption/success", after)
			}
		})
	}
}

func TestIntegrityIncidentAndRecoveryNotificationsAreDeduplicated(t *testing.T) {
	s := testService(t)
	b := rotationBackups(t, s, 1)[0]
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	settings, _ := s.Settings()
	settings.Webhook = server.URL
	s.SaveSettings(settings)
	p := filepath.Join(s.backupDir(b), "files/etc/network/interfaces")
	data, _ := os.ReadFile(p)
	os.WriteFile(p, []byte("bad"), 0600)
	for i := 0; i < 2; i++ {
		if e := s.VerifyBackup(b.ID); e == nil {
			t.Fatal("corruption missed")
		}
	}
	restarted, e := NewService(s.Root, s.Store, fixtureCollector{})
	if e != nil {
		t.Fatal(e)
	}
	restarted.VerifyBackup(b.ID)
	if calls.Load() != 1 {
		t.Fatal("incident should notify once across restart", calls.Load())
	}
	os.WriteFile(p, data, 0600)
	for i := 0; i < 2; i++ {
		if e := restarted.VerifyBackup(b.ID); e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("recovery should notify once", calls.Load())
	}
}

func TestIntegrityDeferredAttemptIsVisibleWithoutReplacingSuccessfulVerdict(t *testing.T) {
	s := testService(t)
	b := rotationBackups(t, s, 1)[0]
	if e := s.ArchiveBackup(b.ID); e != nil {
		t.Fatal(e)
	}
	s.diskUsage = func() (DiskUsage, error) { return DiskUsage{Free: 0}, nil }
	if s.VerifyBackup(b.ID) == nil {
		t.Fatal("low space accepted")
	}
	r, e := s.BackupReadiness(b.ID)
	if e != nil {
		t.Fatal(e)
	}
	if readinessState(r.Items, "verification") != "unknown" {
		t.Fatal("deferred attempt shown as success or corruption", r.Items)
	}
}

func TestIntegrityDeferredProtectedRotationAdvancesAcrossRestart(t *testing.T) {
	s := testService(t)
	bs := rotationBackups(t, s, 3)
	for i := 0; i < 2; i++ {
		if e := s.ArchiveBackup(bs[i].ID); e != nil {
			t.Fatal(e)
		}
		bs[i], _ = s.Backup(bs[i].ID)
		bs[i].Pinned = true
		s.Store.Put("backups", bs[i].ID, bs[i])
	}
	s.diskUsage = func() (DiskUsage, error) { return DiskUsage{Free: 0}, nil }
	if e := s.Maintain(time.Now()); e != nil {
		t.Fatal(e)
	}
	var first integrityAttempt
	s.Store.Get("integrity_attempts", bs[0].ID, &first)
	if first.State != "deferred" {
		t.Fatal("older pinned archive did not defer", first)
	}
	restarted, e := NewService(s.Root, s.Store, fixtureCollector{})
	if e != nil {
		t.Fatal(e)
	}
	restarted.diskUsage = s.diskUsage
	if e = restarted.Maintain(time.Now().Add(24 * time.Hour)); e != nil {
		t.Fatal(e)
	}
	var second integrityAttempt
	s.Store.Get("integrity_attempts", bs[1].ID, &second)
	if second.State != "deferred" {
		t.Fatal("deferred older monopolized next run", second)
	}
}

func TestIntegrityRefusesSymlinkFilesRoot(t *testing.T) {
	s := testService(t)
	b := rotationBackups(t, s, 1)[0]
	files := filepath.Join(s.backupDir(b), "files")
	moved := filepath.Join(t.TempDir(), "files")
	if e := os.Rename(files, moved); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(moved, files); e != nil {
		t.Fatal(e)
	}
	if e := s.VerifyBackup(b.ID); e == nil {
		t.Fatal("live symlink root accepted as source tree")
	}
}

func TestIntegritySweepReachesUnprotectedBeforeRepeatingProtected(t *testing.T) {
	s := testService(t)
	bs := rotationBackups(t, s, 4)
	bs[0].Pinned = true
	s.Store.Put("backups", bs[0].ID, bs[0])
	if e := s.Maintain(time.Now()); e != nil {
		t.Fatal(e)
	}
	pinned, _ := s.Backup(bs[0].ID)
	if pinned.VerifiedAt == "" {
		t.Fatal("protected source did not lead sweep")
	}
	restarted, e := NewService(s.Root, s.Store, fixtureCollector{})
	if e != nil {
		t.Fatal(e)
	}
	for i := 1; i <= 2; i++ {
		if e = restarted.Maintain(time.Now().Add(time.Duration(i) * 24 * time.Hour)); e != nil {
			t.Fatal(e)
		}
		got, _ := s.Backup(bs[i].ID)
		if got.VerifiedAt == "" {
			t.Fatal("protected repeats starved unprotected source", bs[i].ID)
		}
	}
	after, _ := s.Backup(bs[0].ID)
	if after.VerifiedAt != pinned.VerifiedAt {
		t.Fatal("protected repeated before complete sweep")
	}
	if e = restarted.Maintain(time.Now().Add(72 * time.Hour)); e != nil {
		t.Fatal(e)
	}
	after, _ = s.Backup(bs[0].ID)
	if after.VerifiedAt == pinned.VerifiedAt {
		t.Fatal("protected did not lead next sweep")
	}
}

func TestIntegrityNotificationFailureDoesNotRejectRecoveredBytes(t *testing.T) {
	s := testService(t)
	b := rotationBackups(t, s, 1)[0]
	var unavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable.Load() {
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	settings, _ := s.Settings()
	settings.Webhook = server.URL
	s.SaveSettings(settings)
	p := filepath.Join(s.backupDir(b), "files/etc/network/interfaces")
	data, _ := os.ReadFile(p)
	os.WriteFile(p, []byte("bad"), 0600)
	if s.VerifyBackup(b.ID) == nil {
		t.Fatal("corruption missed")
	}
	os.WriteFile(p, data, 0600)
	unavailable.Store(true)
	if e := s.VerifyBackup(b.ID); e != nil {
		t.Fatal("notification transport blocked verified bytes", e)
	}
	got, _ := s.Backup(b.ID)
	if got.Status != "successful" || got.VerificationError != "" {
		t.Fatal("notification error marked source corrupt", got)
	}
	var health map[string]string
	s.Store.Get("health", "notifications", &health)
	if health["error"] == "" {
		t.Fatal("notification delivery failure invisible")
	}
}

func TestIntegrityFailureClassification(t *testing.T) {
	for _, code := range []syscall.Errno{syscall.ENOSPC, syscall.EACCES, syscall.EPERM, syscall.EIO, syscall.EROFS, syscall.EDQUOT, syscall.ENOMEM, syscall.EMFILE, syscall.ETIMEDOUT} {
		if !verificationDeferred(&os.PathError{Op: "read", Path: "private", Err: code}) {
			t.Errorf("environment error classified as corrupt: %v", code)
		}
	}
	for _, code := range []syscall.Errno{syscall.ENOENT, syscall.ENOTDIR, syscall.EISDIR, syscall.ELOOP} {
		if verificationDeferred(&os.PathError{Op: "read", Path: "private", Err: code}) {
			t.Errorf("missing/invalid content deferred: %v", code)
		}
	}
}
