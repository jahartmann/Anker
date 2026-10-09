package anker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func disasterFixture(t *testing.T) (*Service, string) {
	t.Helper()
	s := testService(t)
	if err := EnsureDataMode(s.Root, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Put("credentials", "admin", map[string]string{"hash": "raw-secret-hash", "future": "preserve-me"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Put("sessions", "session-secret", map[string]string{"user_id": "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Put("jobs", "old-job", Job{ID: "old-job", State: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Put("settings", "main", Settings{Timezone: "Europe/Berlin", SMTPPassword: "mail-secret"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"keys/backup", "keys/restore", "known_hosts", "service.env"} {
		if err := atomicWrite(filepath.Join(s.Root, name), []byte("private-material"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h, _ := s.Host("host1")
	h.KeyPath = filepath.Join(s.Root, "keys/backup")
	h.KnownHostsPath = filepath.Join(s.Root, "known_hosts")
	h.RestoreKeyPath = filepath.Join(s.Root, "keys/restore")
	h.RestoreSSHUser = "root"
	if err := s.Store.Put("hosts", h.ID, h); err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateBackup(context.Background(), h.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Test WAL consistency: connection deliberately remains open with committed WAL.
	if st, err := os.Stat(filepath.Join(s.Root, "catalog.db-wal")); err != nil || st.Size() == 0 {
		t.Fatal("fixture did not leave committed WAL", err)
	}
	return s, b.ID
}

func TestDisasterRoundTripColdWALAndIsolation(t *testing.T) {
	s, id := disasterFixture(t)
	original, _ := os.ReadFile(filepath.Join(s.backupDir(Backup{ID: id, HostID: "host1"}), "manifest.json"))
	var bundle bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("test-passphrase"), &bundle); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bundle.Bytes(), []byte("raw-secret")) {
		t.Fatal("plaintext secret")
	}
	info, err := InspectDisaster(bytes.NewReader(bundle.Bytes()), []byte("test-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != 1 || info.Mode != "production" || info.Backups != 1 {
		t.Fatalf("summary: %+v", info)
	}
	data, _ := json.Marshal(info)
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), s.Root) {
		t.Fatal("inspect leaks")
	}
	dest := filepath.Join(t.TempDir(), "imported")
	if _, err = ImportDisaster(bytes.NewReader(bundle.Bytes()), dest, []byte("test-passphrase")); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(dest, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var cred map[string]string
	if err = store.Get("credentials", "admin", &cred); err != nil || cred["hash"] != "raw-secret-hash" || cred["future"] != "preserve-me" {
		t.Fatalf("raw credentials lost: %v %+v", err, cred)
	}
	var settings Settings
	store.Get("settings", "main", &settings)
	if settings.SMTPPassword != "mail-secret" {
		t.Fatal("settings lost")
	}
	var sess any
	if store.Get("sessions", "session-secret", &sess) == nil {
		t.Fatal("session retained")
	}
	var job Job
	store.Get("jobs", "old-job", &job)
	if job.State != "interrupted" {
		t.Fatalf("resumable job: %+v", job)
	}
	dest, _ = filepath.EvalSymlinks(dest)
	var h Host
	store.Get("hosts", "host1", &h)
	if h.Enabled || h.KeyPath != filepath.Join(dest, "keys/backup") {
		t.Fatalf("host isolation/remap %+v", h)
	}
	if _, err = NewService(dest, store, fixtureCollector{}); err == nil {
		t.Fatal("isolated import starts")
	}
	got, err := os.ReadFile(filepath.Join(dest, "hosts/host1/backups", id, "manifest.json"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("original manifest changed", err)
	}
	for _, name := range []string{"keys/backup", "keys/restore", "known_hosts", "service.env"} {
		st, err := os.Stat(filepath.Join(dest, name))
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatal("missing/unprotected material", name)
		}
	}
}

func TestDisasterRejectsWrongPassTamperActiveAndUnsafe(t *testing.T) {
	s, _ := disasterFixture(t)
	var b bytes.Buffer
	lock, err := InstanceLock(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if ExportDisaster(s.Root, "", []byte("passphrase"), &b) == nil {
		t.Fatal("active export")
	}
	lock()
	if err = ExportDisaster(s.Root, "", []byte("passphrase"), &b); err != nil {
		t.Fatal(err)
	}
	if _, err = InspectDisaster(bytes.NewReader(b.Bytes()), []byte("wrong")); err == nil {
		t.Fatal("wrong password")
	}
	bad := append([]byte(nil), b.Bytes()...)
	bad[len(bad)-1] ^= 1
	if _, err = InspectDisaster(bytes.NewReader(bad), []byte("passphrase")); err == nil {
		t.Fatal("tamper")
	}
	dest := t.TempDir()
	os.WriteFile(filepath.Join(dest, "sentinel"), []byte("keep"), 0600)
	if _, err = ImportDisaster(bytes.NewReader(b.Bytes()), dest, []byte("passphrase")); err == nil {
		t.Fatal("existing destination")
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "sentinel")); string(got) != "keep" {
		t.Fatal("overwrite")
	}
	if err = os.Symlink("/etc/passwd", filepath.Join(s.Root, "keys", "evil")); err != nil {
		t.Fatal(err)
	}
	b.Reset()
	if ExportDisaster(s.Root, "", []byte("passphrase"), &b) == nil {
		t.Fatal("symlink exported")
	}
}

func TestDisasterRejectsMissingAndRelativeCredentials(t *testing.T) {
	for _, key := range []string{"../outside", "keys/backup", "/missing/external/key"} {
		t.Run(key, func(t *testing.T) {
			s, _ := disasterFixture(t)
			h, _ := s.Host("host1")
			h.KeyPath = key
			s.Store.Put("hosts", h.ID, h)
			var b bytes.Buffer
			err := ExportDisaster(s.Root, "", []byte("passphrase"), &b)
			if strings.HasPrefix(key, "/") {
				if err != nil {
					t.Fatal(err)
				}
				info, e := InspectDisaster(&b, []byte("passphrase"))
				if e != nil || info.ExternalReferences == 0 {
					t.Fatal("external prerequisite not classified", e)
				}
			} else if err == nil {
				t.Fatal("relative credential accepted")
			}
		})
	}
	s, _ := disasterFixture(t)
	os.Remove(filepath.Join(s.Root, "keys/backup"))
	var b bytes.Buffer
	if ExportDisaster(s.Root, "", []byte("passphrase"), &b) == nil {
		t.Fatal("missing local key accepted")
	}
}

func TestDisasterRetainsRawCatalogWhitespaceAndDatedReceipt(t *testing.T) {
	s, id := disasterFixture(t)
	raw := []byte("{\n  \"hash\": \"omitted-credential\", \"PasswordHash\": \"future-field\"\n}")
	if _, err := s.Store.db.Exec("UPDATE records SET value=? WHERE bucket='credentials' AND id='admin'", raw); err != nil {
		t.Fatal(err)
	}
	b, _ := s.Backup(id)
	b.VerifiedAt = "2001-02-03T04:05:06Z"
	b.CopyVerification = &CopyVerificationReceipt{Version: 1, BackupID: id, ManifestSHA: b.ManifestSHA, State: "ok", VerifiedAt: "2001-02-03T04:05:06Z", Provenance: "local-byte-verifier-v1", FailureDomain: "unknown"}
	s.Store.Put("backups", id, b)
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "new")
	if _, err := ImportDisaster(&out, dest, []byte("pass")); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(dest, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var got []byte
	if err = store.db.QueryRow("SELECT value FROM records WHERE bucket='credentials' AND id='admin'").Scan(&got); err != nil || !bytes.Equal(raw, got) {
		t.Fatalf("raw credential changed: %q %v", got, err)
	}
	var imported Backup
	store.Get("backups", id, &imported)
	if imported.VerifiedAt != b.VerifiedAt || imported.CopyVerification.VerifiedAt != b.CopyVerification.VerifiedAt {
		t.Fatal("manufactured verification date")
	}
}
func TestDisasterRejectsAuthenticatedUnsafeInventory(t *testing.T) {
	s, _ := disasterFixture(t)
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"traversal", "absolute", "duplicate", "checksum", "special", "missing-backup", "missing-key"} {
		t.Run(kind, func(t *testing.T) {
			p, err := disasterUnseal(bytes.NewReader(out.Bytes()), []byte("pass"))
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "traversal":
				p.Files[0].Path = "../outside"
			case "absolute":
				p.Files[0].Path = "/outside"
			case "duplicate":
				p.Files = append(p.Files, p.Files[0])
			case "checksum":
				p.Files[0].SHA256 = "false"
			case "special":
				p.Files[0].Path = "update-pending.json"
			case "missing-backup":
				for i, f := range p.Files {
					if strings.HasSuffix(f.Path, "manifest.json") {
						p.Files = append(p.Files[:i], p.Files[i+1:]...)
						break
					}
				}
			case "missing-key":
				for i, f := range p.Files {
					if f.Path == "keys/backup" {
						p.Files = append(p.Files[:i], p.Files[i+1:]...)
						break
					}
				}
			}
			var changed bytes.Buffer
			if err = disasterSeal(p, []byte("pass"), &changed); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "new")
			if _, err = ImportDisaster(&changed, dest, []byte("pass")); err == nil {
				t.Fatal("unsafe package imported")
			}
			if _, err = os.Lstat(dest); !os.IsNotExist(err) {
				t.Fatal("failed import published")
			}
		})
	}
}
func TestDisasterPrivateEvidencePreservedButAuthorityRevoked(t *testing.T) {
	s, _, request := snapshotSetup(t)
	if err := EnsureDataMode(s.Root, false); err != nil {
		t.Fatal(err)
	}
	plan, err := s.CreatePlan(t.Context(), requestDecision(t, request, request.Files[0], "backup"))
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(s.Root, "plans", plan.ID, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "new")
	if _, err = ImportDisaster(&out, dest, []byte("pass")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dest, "plans", plan.ID, "plan.json"))
	if !bytes.Equal(original, got) {
		t.Fatal("private plan changed")
	}
	store, err := OpenStore(filepath.Join(dest, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	os.Remove(filepath.Join(dest, disasterIsolation))
	service, err := NewService(dest, store, s.Collector)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.verifyPlanFiles(plan); err != nil {
		t.Fatal("private evidence broken", err)
	}
	if err = service.disasterPlanAllowed(plan.ID); err == nil {
		t.Fatal("old plan authority reused")
	}
	if _, err = service.ApplyPlan(t.Context(), plan.ID, plan.ID); err == nil || !strings.Contains(err.Error(), "Importierter Plan") {
		t.Fatal("old apply not blocked", err)
	}
	if _, err = service.ReconcilePlan(t.Context(), plan.ID); err == nil || !strings.Contains(err.Error(), "Importierter Plan") {
		t.Fatal("old reconcile not blocked", err)
	}
	if _, err = service.RollbackPlan(t.Context(), plan.ID, plan.ID); err == nil || !strings.Contains(err.Error(), "Importierter Plan") {
		t.Fatal("old rollback not blocked", err)
	}
	// Even unchanged external host paths must not authorize reuse of old inspections.
	if _, err = service.loadRecoverySnapshotLocked(plan.TargetSnapshotID); err == nil || !strings.Contains(err.Error(), "Disaster") {
		t.Fatal("old inspection authority reused", err)
	}
	os.Chmod(filepath.Join(s.Root, "plans", plan.ID, "plan.json"), 0644)
	out.Reset()
	if ExportDisaster(s.Root, "", []byte("pass"), &out) == nil {
		t.Fatal("unsafe private evidence permissions washed")
	}
}

func TestDisasterConfigQuarantinedAndExternalReferencesListed(t *testing.T) {
	s, _ := disasterFixture(t)
	config := t.TempDir()
	for name, content := range map[string]string{"keys/backup": "ssh-private", "known_hosts": "host-trust", "tls/server.key": "tls-private", "service.env": "ANKER_DATA=" + s.Root + "\nANKER_TLS_KEY=" + filepath.Join(config, "tls/server.key") + "\nANKER_TLS_CERT=/external/server.crt\n", "setup-pending.json": "must-not-restore", "update-pending.json": "must-not-restore"} {
		if err := atomicWrite(filepath.Join(config, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h, _ := s.Host("host1")
	h.KeyPath = filepath.Join(config, "keys/backup")
	h.KnownHostsPath = filepath.Join(config, "known_hosts")
	s.Store.Put("hosts", h.ID, h)
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, config, []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "new")
	if _, err := ImportDisaster(&out, dest, []byte("pass")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"disaster-config/keys/backup", "disaster-config/known_hosts", "disaster-config/tls/server.key", "disaster-config/service.env"} {
		if _, err := os.Stat(filepath.Join(dest, path)); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"disaster-config/setup-pending.json", "disaster-config/update-pending.json"} {
		if _, err := os.Stat(filepath.Join(dest, path)); !os.IsNotExist(err) {
			t.Fatal("pending work imported")
		}
	}
	report, err := os.ReadFile(filepath.Join(dest, "disaster-migration.json"))
	if err != nil || !bytes.Contains(report, []byte("/external/server.crt")) {
		t.Fatal("external migration reference missing", err)
	}
}
func TestDisasterRejectsHardlinkAndMissingPrivateSnapshot(t *testing.T) {
	s, _ := disasterFixture(t)
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("outside"), 0600)
	os.Link(outside, filepath.Join(s.Root, "keys/linked"))
	var out bytes.Buffer
	if ExportDisaster(s.Root, "", []byte("pass"), &out) == nil {
		t.Fatal("hardlinked external material included")
	}
	s, _, request := snapshotSetup(t)
	EnsureDataMode(s.Root, false)
	plan, err := s.CreatePlan(t.Context(), requestDecision(t, request, request.Files[0], "backup"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Root, "recovery-inspections", plan.TargetSnapshotID, "files", request.Files[0])
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if ExportDisaster(s.Root, "", []byte("pass"), &out) == nil {
		t.Fatal("missing private target file accepted")
	}
}
func TestDisasterPinnedDirectoryRejectsAncestorSwap(t *testing.T) {
	parent := t.TempDir()
	source := filepath.Join(parent, "outer", "root")
	os.MkdirAll(source, 0700)
	d, err := openDisasterDir(source)
	if err != nil {
		t.Fatal(err)
	}
	defer d.f.Close()
	os.Rename(filepath.Join(parent, "outer"), filepath.Join(parent, "original"))
	os.MkdirAll(source, 0700)
	if d.check() == nil {
		t.Fatal("swapped ancestor accepted")
	}
	// Publication must fail even for an existing empty directory.
	fd, err := openDisasterDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.f.Close()
	os.Mkdir(filepath.Join(parent, "from"), 0700)
	os.Mkdir(filepath.Join(parent, "to"), 0700)
	if disasterPublish(int(fd.f.Fd()), "from", "to") == nil {
		t.Fatal("atomic publish replaced empty destination")
	}
}

func TestDisasterOriginalHostConfigIsNotInstallationReference(t *testing.T) {
	s, _ := disasterFixture(t)
	if err := atomicWrite(filepath.Join(s.Root, "hosts/evidence/service.env"), []byte("HOST_PATH="+filepath.Join(s.Root, "not-an-app-key")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal("host original treated as app configuration", err)
	}
}
func TestDisasterRejectsArchiveTraversalAndBombHeader(t *testing.T) {
	for _, h := range []*tar.Header{{Name: "../escape", Typeflag: tar.TypeReg, Mode: 0600}, {Name: "oversized", Typeflag: tar.TypeReg, Size: 65 << 20, Mode: 0600}, {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0600}} {
		var out bytes.Buffer
		gz := gzip.NewWriter(&out)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		tw.Close()
		gz.Close()
		p := &disasterPackage{Files: []disasterFile{{Path: "hosts/h/backups/b/archive.tar.gz", Data: out.Bytes()}}}
		if disasterCheckArchives(p) == nil {
			t.Fatal("unsafe archive accepted", h.Name)
		}
	}
}

func TestDisasterRejectsNullHostAndJobRecords(t *testing.T) {
	s, _ := disasterFixture(t)
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	for _, bucket := range []string{"hosts", "jobs"} {
		t.Run(bucket, func(t *testing.T) {
			p, err := disasterUnseal(bytes.NewReader(out.Bytes()), []byte("pass"))
			if err != nil {
				t.Fatal(err)
			}
			for i := range p.Records {
				if p.Records[i].Bucket == bucket {
					p.Records[i].Value = []byte("null")
					p.Records[i].SHA256 = Hash(p.Records[i].Value)
				}
			}
			p.References = nil
			if err = disasterReferences(p); err != nil {
				return
			}
			var bad bytes.Buffer
			disasterSeal(p, []byte("pass"), &bad)
			if _, err = ImportDisaster(&bad, filepath.Join(t.TempDir(), "new"), []byte("pass")); err == nil {
				t.Fatal("null catalog record accepted")
			}
		})
	}
}

func TestDisasterImportRejectsNestedExistingInstance(t *testing.T) {
	s, _ := disasterFixture(t)
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	lock, err := InstanceLock(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock()
	dest := filepath.Join(s.Root, "nested")
	if _, err = ImportDisaster(&out, dest, []byte("pass")); err == nil {
		t.Fatal("nested live-instance import accepted")
	}
	if _, err = os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("nested destination published")
	}
}

func TestDisasterArchivedBackupAndDemoModeRoundTrip(t *testing.T) {
	s, id := disasterFixture(t)
	if err := s.ArchiveBackup(id); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, ".anker-mode"), []byte("demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "new")
	info, err := ImportDisaster(&out, dest, []byte("pass"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode != "demo" {
		t.Fatal("demo provenance lost")
	}
	store, err := OpenStore(filepath.Join(dest, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var backup Backup
	if err = store.Get("backups", id, &backup); err != nil || !backup.Archived {
		t.Fatal("archive record lost", err)
	}
	imported := &Service{Root: dest, Store: store}
	if err = imported.checkBackupUnlocked(id); err != nil {
		t.Fatal("archive no longer readable", err)
	}
}
