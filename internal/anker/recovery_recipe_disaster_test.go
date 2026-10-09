package anker

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func recipeDisasterFixture(t *testing.T) (*Service, string, string, []byte) {
	t.Helper()
	s, _, id := recipeSetup(t)
	host, err := s.Host("target")
	if err != nil {
		t.Fatal(err)
	}
	host.Enabled = true
	if err = s.Store.Put("hosts", host.ID, host); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDataMode(s.Root, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Root, "recovery-recipes", id+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, id, path, raw
}
func TestRecipeDisasterRoundTripPreservesPrivateIntentAndIsolation(t *testing.T) {
	s, id, path, original := recipeDisasterFixture(t)
	var before recoveryRecipeRecord
	if err := s.Store.Get("recovery-recipes", id, &before); err != nil {
		t.Fatal(err)
	}
	var bundle bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("recipe-test-pass"), &bundle); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "imported")
	if _, err := ImportDisaster(&bundle, dest, []byte("recipe-test-pass")); err != nil {
		t.Fatal(err)
	}
	importedPath := filepath.Join(dest, "recovery-recipes", id+".json")
	actual, err := os.ReadFile(importedPath)
	if err != nil {
		t.Fatalf("successful cold roundtrip lost private recipe %s: %v", path, err)
	}
	if !bytes.Equal(original, actual) {
		t.Fatal("private recipe bytes changed")
	}
	for name, want := range map[string]os.FileMode{filepath.Dir(importedPath): 0700, importedPath: 0600} {
		st, err := os.Stat(name)
		if err != nil || st.Mode().Perm() != want {
			t.Fatal("private recipe permissions", name, err)
		}
	}
	store, err := OpenStore(filepath.Join(dest, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var after recoveryRecipeRecord
	if err = store.Get("recovery-recipes", id, &after); err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("review metadata/catalog binding changed", before, after, err)
	}
	// Inspect isolated evidence directly, never remove isolation or activate it.
	var host Host
	if err = store.Get("hosts", "target", &host); err != nil || host.Enabled {
		t.Fatal("imported host active", err)
	}
	if _, err = os.Stat(filepath.Join(dest, disasterIsolation)); err != nil {
		t.Fatal("isolation marker lost", err)
	}
	if _, err = NewService(dest, store, fixtureCollector{}); err == nil {
		t.Fatal("isolated service started")
	}
	var revoked any
	if err = store.Get("disaster-revoked-plans", before.OriginPlanID, &revoked); err != nil {
		t.Fatal("historical plan not revoked", err)
	}
	var private recoveryRecipePrivate
	json.Unmarshal(original, &private)
	if err = store.Get("disaster-revoked-inspections", private.OriginSnapshotID, &revoked); err != nil {
		t.Fatal("historical inspection not revoked", err)
	}
	imported := &Service{Root: dest, Store: store}
	preview, err := imported.PreviewRecoveryRecipe(id, PlanRequest{SnapshotID: private.OriginSnapshotID})
	if err != nil || preview.Applicable || len(preview.Mismatches) == 0 {
		t.Fatal("historical recipe reused old authority", preview, err)
	}
}
func TestRecipeDisasterExportRejectsInvalidMaterialBeforeWriting(t *testing.T) {
	for _, kind := range []string{"missing", "tampered", "catalog-id", "bound-id", "catalog-metadata", "version", "file-permissions", "directory-permissions"} {
		t.Run(kind, func(t *testing.T) {
			s, id, path, raw := recipeDisasterFixture(t)
			var rec recoveryRecipeRecord
			s.Store.Get("recovery-recipes", id, &rec)
			switch kind {
			case "missing":
				os.Remove(path)
			case "tampered":
				os.WriteFile(path, append(raw, ' '), 0600)
			case "catalog-id":
				rec.ID = ID()
				s.Store.Put("recovery-recipes", id, rec)
			case "bound-id":
				var private recoveryRecipePrivate
				json.Unmarshal(raw, &private)
				private.ID = ID()
				rec.ID = private.ID
				raw, _ = json.Marshal(private)
				rec.SHA256 = Hash(raw)
				os.WriteFile(path, raw, 0600)
				s.Store.Put("recovery-recipes", id, rec)
			case "catalog-metadata":
				rec.ReviewedBy = "different-reviewer"
				s.Store.Put("recovery-recipes", id, rec)
			case "version":
				var private recoveryRecipePrivate
				json.Unmarshal(raw, &private)
				private.Version = 2
				raw, _ = json.Marshal(private)
				os.WriteFile(path, raw, 0600)
				rec.Version = 2
				rec.SHA256 = Hash(raw)
				s.Store.Put("recovery-recipes", id, rec)
			case "file-permissions":
				os.Chmod(path, 0644)
			case "directory-permissions":
				os.Chmod(filepath.Dir(path), 0755)
			}
			var bundle bytes.Buffer
			if err := ExportDisaster(s.Root, "", []byte("recipe-test-pass"), &bundle); err == nil {
				t.Fatal("invalid recipe exported", kind)
			}
			if bundle.Len() != 0 {
				t.Fatal("invalid export wrote package bytes")
			}
		})
	}
}
func TestRecipeDisasterImportRejectsMissingOrMismatchedMaterialBeforePublication(t *testing.T) {
	s, id, _, original := recipeDisasterFixture(t)
	var bundle bytes.Buffer
	if err := ExportDisaster(s.Root, "", []byte("recipe-test-pass"), &bundle); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing", "tampered", "catalog-id", "bound-id", "catalog-metadata", "private-id", "version"} {
		t.Run(kind, func(t *testing.T) {
			p, err := disasterUnseal(bytes.NewReader(bundle.Bytes()), []byte("recipe-test-pass"))
			if err != nil {
				t.Fatal(err)
			}
			recipePath := "recovery-recipes/" + id + ".json"
			// Explicitly install baseline bytes so each preflight case exercises semantic
			// validation even against the old exporter that omitted this directory.
			kept := []disasterFile{}
			for _, f := range p.Files {
				if f.Path != recipePath {
					kept = append(kept, f)
				}
			}
			p.Files = kept
			raw := append([]byte{}, original...)
			var private recoveryRecipePrivate
			json.Unmarshal(raw, &private)
			for i, r := range p.Records {
				if r.Bucket != "recovery-recipes" || r.ID != id {
					continue
				}
				var rec recoveryRecipeRecord
				json.Unmarshal(r.Value, &rec)
				switch kind {
				case "tampered":
					raw = append(raw, ' ')
				case "catalog-id":
					rec.ID = ID()
				case "bound-id":
					private.ID = ID()
					rec.ID = private.ID
					raw, _ = json.Marshal(private)
					rec.SHA256 = Hash(raw)
				case "catalog-metadata":
					rec.Label = "different-label"
				case "private-id":
					private.ID = ID()
					raw, _ = json.Marshal(private)
					rec.SHA256 = Hash(raw)
				case "version":
					private.Version = 2
					raw, _ = json.Marshal(private)
					rec.Version = 2
					rec.SHA256 = Hash(raw)
				}
				p.Records[i].Value, _ = json.Marshal(rec)
				p.Records[i].SHA256 = Hash(p.Records[i].Value)
			}
			if kind != "missing" {
				p.Files = append(p.Files, disasterFile{Path: recipePath, Data: raw, SHA256: Hash(raw)})
			}
			var changed bytes.Buffer
			if err = disasterSeal(p, []byte("recipe-test-pass"), &changed); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "imported")
			if _, err = ImportDisaster(&changed, dest, []byte("recipe-test-pass")); err == nil {
				t.Fatal("invalid recipe imported", kind)
			}
			if _, err = os.Lstat(dest); !os.IsNotExist(err) {
				t.Fatal("invalid import published destination", err)
			}
		})
	}
}
