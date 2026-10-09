package anker

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisasterMissingSourcePreservesPrivateHistory(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	if err := EnsureDataMode(s.Root, false); err != nil {
		t.Fatal(err)
	}
	b, _ := s.Backup(p.BackupID)
	os.RemoveAll(s.backupDir(b))
	var bundle bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("test-pass"), &bundle); err != nil {
		t.Fatal("missing source must not block installation recovery", err)
	}
	info, err := InspectDisaster(bytes.NewReader(bundle.Bytes()), []byte("test-pass"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(info)
	var view map[string]any
	json.Unmarshal(raw, &view)
	sources, ok := view["sources"].([]any)
	if !ok || len(sources) != 1 {
		t.Fatalf("explicit source inventory missing: %s", raw)
	}
	source := sources[0].(map[string]any)
	if source["state"] != "missing" || source["backup_id"] != b.ID || source["host_id"] != b.HostID || source["manifest_sha"] != b.ManifestSHA {
		t.Fatalf("source binding: %s", raw)
	}
	dest := filepath.Join(t.TempDir(), "restored")
	if _, err = ImportDisaster(bytes.NewReader(bundle.Bytes()), dest, []byte("test-pass")); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(dest, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	imported := &Service{Root: dest, Store: store}
	if err = imported.verifyPlanFiles(p); err != nil {
		t.Fatal("private history lost", err)
	}
	if _, err = imported.ReconcilePlan(context.Background(), p.ID); err == nil {
		t.Fatal("old plan regained authority")
	}
	if _, err = imported.RollbackPlan(context.Background(), p.ID, p.ID); err == nil {
		t.Fatal("old rollback regained authority")
	}
	var again bytes.Buffer
	if err = ExportDisaster(dest, "", []byte("test-pass"), &again); err != nil {
		t.Fatal("missing source reexport", err)
	}
}

func TestDisasterSeparateSourceRoundTrip(t *testing.T) {
	s, id := disasterFixture(t)
	b, _ := s.Backup(id)
	var bundle bytes.Buffer
	if err := ExportDisasterWithBackupData(s.Root, "", "separate", []byte("pass"), &bundle); err != nil {
		t.Fatal(err)
	}
	p, err := disasterUnseal(bytes.NewReader(bundle.Bytes()), []byte("pass"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Sources) != 1 || p.Sources[0].State != "separate" || p.Sources[0].ManifestSHA != b.ManifestSHA {
		t.Fatalf("binding: %+v", p.Sources)
	}
	for _, f := range p.Files {
		if strings.HasPrefix(f.Path, p.Sources[0].prefix()+"/") {
			t.Fatal("split package includes backup bytes")
		}
	}
	info, err := InspectDisaster(bytes.NewReader(bundle.Bytes()), []byte("pass"))
	if err != nil || info.SeparateSources != 1 || info.IncludedSources != 0 {
		t.Fatalf("summary %+v %v", info, err)
	}
	dest := filepath.Join(t.TempDir(), "new")
	if _, err = ImportDisaster(bytes.NewReader(bundle.Bytes()), dest, []byte("pass")); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(dest, "disaster-migration.json"))
	if err != nil || !bytes.Contains(report, []byte(`"separate"`)) {
		t.Fatal("migration coverage missing", err)
	}
	store, err := OpenStore(filepath.Join(dest, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var got Backup
	if err = store.Get("backups", id, &got); err != nil || got.ManifestSHA != b.ManifestSHA {
		t.Fatal("catalog lost", err)
	}
	var cred map[string]string
	store.Get("credentials", "admin", &cred)
	if cred["hash"] != "raw-secret-hash" {
		t.Fatal("credential lost")
	}
	var host Host
	store.Get("hosts", b.HostID, &host)
	if host.Enabled {
		t.Fatal("host active")
	}
	var job Job
	store.Get("jobs", "old-job", &job)
	if job.State != "interrupted" {
		t.Fatal("job active")
	}
	var session any
	if store.Get("sessions", "session-secret", &session) == nil {
		t.Fatal("session alive")
	}
	var again bytes.Buffer
	if err = ExportDisaster(dest, "", []byte("pass"), &again); err != nil {
		t.Fatal(err)
	}
	info, err = InspectDisaster(&again, []byte("pass"))
	if err != nil || info.SeparateSources != 1 {
		t.Fatal("separate state lost at reexport", err)
	}
}
func TestDisasterPartialPresentNeverBecomesAbsence(t *testing.T) {
	for _, mode := range []string{"include", "separate"} {
		for _, damage := range []string{"empty-directory", "missing-manifest", "bad-content", "link"} {
			t.Run(mode+"/"+damage, func(t *testing.T) {
				s, id := disasterFixture(t)
				b, _ := s.Backup(id)
				dir := s.backupDir(b)
				switch damage {
				case "empty-directory":
					os.RemoveAll(dir)
					os.Mkdir(dir, 0700)
				case "missing-manifest":
					os.Remove(filepath.Join(dir, "manifest.json"))
				case "bad-content":
					os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0600)
				case "link":
					outside := filepath.Join(t.TempDir(), "data")
					os.Rename(dir, outside)
					os.Symlink(outside, dir)
				}
				var out bytes.Buffer
				if err := ExportDisasterWithBackupData(s.Root, "", mode, []byte("pass"), &out); err == nil || out.Len() != 0 {
					t.Fatal("damaged present source exported", err)
				}
			})
		}
	}
}
func TestDisasterSourceInventoryRejectsForgery(t *testing.T) {
	s, _ := disasterFixture(t)
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	for _, damage := range []string{"host", "id", "digest", "duplicate", "missing-entry", "partial-missing", "partial-separate", "missing-included", "invented-state", "changed-included-bytes"} {
		t.Run(damage, func(t *testing.T) {
			p, err := disasterUnseal(bytes.NewReader(out.Bytes()), []byte("pass"))
			if err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "host":
				p.Sources[0].HostID = "other"
			case "id":
				p.Sources[0].BackupID = "other"
			case "digest":
				p.Sources[0].ManifestSHA = Hash([]byte("other"))
			case "duplicate":
				p.Sources = append(p.Sources, p.Sources[0])
			case "missing-entry":
				p.Sources = nil
			case "partial-missing":
				p.Sources[0].State = "missing"
			case "partial-separate":
				p.Sources[0].State = "separate"
			case "invented-state":
				p.Sources[0].State = "verified"
			case "missing-included":
				kept := []disasterFile{}
				for _, f := range p.Files {
					if !strings.HasPrefix(f.Path, p.Sources[0].prefix()+"/") {
						kept = append(kept, f)
					}
				}
				p.Files = kept
			case "changed-included-bytes":
				for i, f := range p.Files {
					if strings.Contains(f.Path, "/files/") {
						p.Files[i].Data = []byte("changed")
						p.Files[i].SHA256 = Hash(p.Files[i].Data)
						break
					}
				}
			}
			var changed bytes.Buffer
			if err = disasterSeal(p, []byte("pass"), &changed); err != nil {
				t.Fatal(err)
			}
			if _, err = InspectDisaster(bytes.NewReader(changed.Bytes()), []byte("pass")); err == nil {
				t.Fatal("forged inventory passed inspect")
			}
			dest := filepath.Join(t.TempDir(), "new")
			if _, err = ImportDisaster(&changed, dest, []byte("pass")); err == nil {
				t.Fatal("forged inventory imported")
			}
			if _, err = os.Stat(dest); !os.IsNotExist(err) {
				t.Fatal("invalid destination published")
			}
		})
	}
}
func TestDisasterLegacyCompletePackageStillReadable(t *testing.T) {
	s, _ := disasterFixture(t)
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	p, err := disasterUnseal(&out, []byte("pass"))
	if err != nil {
		t.Fatal(err)
	}
	p.Version = 1
	p.Sources = nil
	out.Reset()
	if err = disasterSeal(p, []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	if _, err = InspectDisaster(bytes.NewReader(out.Bytes()), []byte("pass")); err != nil {
		t.Fatal(err)
	}
	if _, err = ImportDisaster(&out, filepath.Join(t.TempDir(), "new"), []byte("pass")); err != nil {
		t.Fatal(err)
	}
}

func TestDisasterMissingSourcePreservesRecipeAndInspection(t *testing.T) {
	for _, mode := range []string{"missing", "separate"} {
		t.Run(mode, func(t *testing.T) {
			s, id, _, original := recipeDisasterFixture(t)
			var record recoveryRecipeRecord
			s.Store.Get("recovery-recipes", id, &record)
			plan, err := s.Plan(record.OriginPlanID)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := s.Backup(plan.BackupID)
			backupMode := "separate"
			if mode == "missing" {
				os.RemoveAll(s.backupDir(b))
				backupMode = "include"
			}
			var out bytes.Buffer
			if err = ExportDisasterWithBackupData(s.Root, "", backupMode, []byte("pass"), &out); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "new")
			if _, err = ImportDisaster(&out, dest, []byte("pass")); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(dest, "recovery-recipes", id+".json"))
			if err != nil || !bytes.Equal(raw, original) {
				t.Fatal("recipe history changed", err)
			}
			store, err := OpenStore(filepath.Join(dest, "catalog.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			var proof recoveryRecipePrivate
			json.Unmarshal(raw, &proof)
			var revoked any
			if err = store.Get("disaster-revoked-inspections", proof.OriginSnapshotID, &revoked); err != nil {
				t.Fatal("inspection not revoked", err)
			}
			imported := &Service{Root: dest, Store: store}
			if err = imported.verifyPlanFiles(plan); err != nil {
				t.Fatal(err)
			}
			if err = imported.disasterPlanAllowed(plan.ID); err == nil {
				t.Fatal("plan authority not revoked")
			}
			var again bytes.Buffer
			if err = ExportDisaster(dest, "", []byte("pass"), &again); err != nil {
				t.Fatal("history reexport", err)
			}
		})
	}
}

func TestDisasterMissingSourceCannotHideUnsafePrivateHistory(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	EnsureDataMode(s.Root, false)
	b, _ := s.Backup(p.BackupID)
	os.RemoveAll(s.backupDir(b))
	if err := os.Chmod(filepath.Join(s.Root, "plans", p.ID, "target-snapshot.json"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err == nil {
		t.Fatal("missing source bypassed private evidence permissions")
	}
}
func TestDisasterUnknownManifestRemainsUnknown(t *testing.T) {
	s, id := disasterFixture(t)
	b, _ := s.Backup(id)
	os.RemoveAll(s.backupDir(b))
	b.ManifestSHA = ""
	s.Store.Put("backups", id, b)
	var out bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("pass"), &out); err != nil {
		t.Fatal(err)
	}
	info, err := InspectDisaster(&out, []byte("pass"))
	if err != nil || info.Sources[0].ManifestSHA != "" || info.Sources[0].State != "missing" {
		t.Fatal("missing proof invented", info, err)
	}
	b.ManifestSHA = "not-a-digest-private-material"
	s.Store.Put("backups", id, b)
	out.Reset()
	if err = ExportDisaster(s.Root, "", []byte("pass"), &out); err == nil {
		t.Fatal("non-digest catalog data admitted to public inspect inventory")
	}
}
