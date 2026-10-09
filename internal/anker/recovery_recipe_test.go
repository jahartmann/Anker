package anker

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func recipeSetup(t *testing.T) (*Service, PlanRequest, string) {
	t.Helper()
	s, _, r := snapshotSetup(t)
	c := s.Collector.(snapshotCollector)
	c.inv.Details["machine_identity_sha256"] = json.RawMessage(`"` + strings.Repeat("a", 64) + `"`)
	c.inv.Details["command_results"] = json.RawMessage(`{"file_inventory":{"state":"ok"},"machine_identity":{"state":"ok"},"interfaces":{"state":"ok"},"pve_version":{"state":"ok"},"cluster":{"state":"not_applicable"}}`)
	c.inv.Interfaces = []Interface{{Name: "eno1", MAC: "00:11:22:33:44:55", PCI: "0000:01:00.0", Type: "physical"}}
	s.Collector = c
	r.Mapping.Hostname = "reviewed-target"
	r.ConsoleConfirmed, r.SourceOffline = true, true
	r.Decisions = map[string]FileDecision{r.Files[0]: DecisionBackup}
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	w := comparisonRequest(t, s, "POST", "/api/recovery-recipes", map[string]any{"plan_id": p.ID, "label": "Geprüfter Zielhost", "reviewed": true}, nil)
	if w.Code != 200 {
		t.Fatalf("save reviewed recipe: %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	if out["reviewed_at"] == "" || out["reviewed_by"] == "" || out["origin_plan_id"] != p.ID || out["version"] != float64(1) {
		t.Fatal(out)
	}
	return s, r, out["id"].(string)
}
func TestRecipeFreshApplicabilityAndInvalidation(t *testing.T) {
	for _, change := range []string{"unchanged", "identity", "missing-proof", "same-name", "nic-name", "mac", "pci", "version", "kernel", "disk", "mount", "private-query", "topology", "connection", "source", "permissions", "expired"} {
		t.Run(change, func(t *testing.T) {
			s, r, id := recipeSetup(t)
			c := s.Collector.(snapshotCollector)
			switch change {
			case "identity", "same-name":
				c.inv.Details["machine_identity_sha256"] = json.RawMessage(`"` + strings.Repeat("b", 64) + `"`)
			case "missing-proof":
				delete(c.inv.Details, "machine_identity_sha256")
			case "nic-name":
				c.inv.Interfaces[0].Name = "eno2"
			case "mac":
				c.inv.Interfaces[0].MAC = "00:11:22:33:44:66"
			case "pci":
				c.inv.Interfaces[0].PCI = "0000:02:00.0"
			case "kernel":
				c.inv.Kernel = "changed-kernel"
			case "disk":
				c.inv.Disks = []Disk{{Name: "sda1", ID: "different", UUID: "new-uuid", Mount: "/var/lib/vz"}}
			case "mount":
				c.inv.Disks = []Disk{{Name: "sda1", ID: "disk", UUID: "uuid", Mount: "/other"}}
			case "private-query":
				c.inv.Details["command_results"] = json.RawMessage(`{"file_inventory":{"state":"ok"},"machine_identity":{"state":"ok"},"pve_version":{"state":"failed"}}`)
			case "version":
				c.inv.PVEVersion = "9.0"
			case "topology":
				c.inv.ClusterID = "new-cluster"
			case "connection":
				h, _ := s.Host(r.TargetID)
				h.KeyPath = "changed-read-key"
				s.SaveHost(h)
			}
			s.Collector = c
			i, err := s.CaptureRecovery(t.Context(), r)
			if err != nil {
				t.Fatal(err)
			}
			r.SnapshotID = i.SnapshotID
			switch change {
			case "source":
				b, _ := s.Backup(r.BackupID)
				os.WriteFile(filepath.Join(s.backupDir(b), "files", r.Files[0]), []byte("tampered"), 0600)
			case "permissions":
				os.Chmod(filepath.Join(s.Root, "recovery-inspections", i.SnapshotID, "snapshot.json"), 0644)
			case "expired":
				var rec recoverySnapshotRecord
				s.Store.Get("recovery-inspections", i.SnapshotID, &rec)
				rec.ExpiresAt = "2000-01-01T00:00:00Z"
				s.Store.Put("recovery-inspections", i.SnapshotID, rec)
			}
			w := comparisonRequest(t, s, "POST", "/api/recovery-recipes/preview", map[string]any{"recipe_id": id, "request": r}, nil)
			if w.Code != 200 {
				t.Fatalf("preview %d: %s", w.Code, w.Body.String())
			}
			var out struct {
				Applicable  bool     `json:"applicable"`
				Suggestions Mapping  `json:"suggestions"`
				Mismatches  []string `json:"mismatches"`
			}
			json.Unmarshal(w.Body.Bytes(), &out)
			if change == "unchanged" {
				if !out.Applicable || out.Suggestions.Hostname != "reviewed-target" {
					t.Fatal(w.Body.String())
				}
			} else if out.Applicable || out.Suggestions.Hostname != "" || len(out.Mismatches) == 0 {
				t.Fatal("stale intent suggested", w.Body.String())
			}
			for _, forbidden := range []string{"console_confirmed", "source_offline", "decisions", "verification_attempts", "tasks"} {
				if strings.Contains(w.Body.String(), `"`+forbidden+`"`) {
					t.Fatal("authorization leaked", w.Body.String())
				}
			}
		})
	}
}
func TestRecipeRequiresExplicitReviewAndRestorePermission(t *testing.T) {
	s, _, r := snapshotSetup(t)
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	w := comparisonRequest(t, s, "POST", "/api/recovery-recipes", map[string]any{"plan_id": p.ID, "label": "Review fehlt"}, nil)
	if w.Code == 200 {
		t.Fatal("unreviewed saved")
	}
	for _, endpoint := range []string{"/api/recovery-recipes", "/api/recovery-recipes/preview"} {
		w = comparisonRequest(t, s, "POST", endpoint, map[string]any{}, &User{ID: "reader", Role: "reader"})
		if w.Code != http.StatusForbidden {
			t.Fatalf("reader %s: %d", endpoint, w.Code)
		}
	}
}

func TestRecipeStorageConstraintsAndPrivatePermissions(t *testing.T) {
	for _, change := range []string{"backing", "volume", "missing-volume", "recipe-permissions", "forged-public-proof", "old-snapshot"} {
		t.Run(change, func(t *testing.T) {
			s, r, id := recipeSetup(t)
			var rec recoveryRecipeRecord
			if err := s.Store.Get("recovery-recipes", id, &rec); err != nil {
				t.Fatal(err)
			}
			p, err := s.Plan(rec.OriginPlanID)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := s.loadPlanSnapshot(p)
			if err != nil {
				t.Fatal(err)
			}
			// Compare actual private evidence, even when names and IDs are unchanged.
			switch change {
			case "backing":
				snapshot.Files = append(snapshot.Files, RecoveryReadFile{Path: "etc/pve/storage.cfg", State: "read", SHA256: Hash([]byte("dir: local\n path /different-backing\n"))})
			case "volume":
				snapshot.StorageVolumes = map[string]RecoveryVolumeResult{"local": {State: "ok", Volumes: []string{"local:vm-100-disk-1"}}}
			case "missing-volume":
				missing := recipeProof(snapshot, Mapping{Storage: map[string]string{"local": "local"}})
				if len(missing) < 2 {
					t.Fatal("missing storage proofs accepted", missing)
				}
				return
			case "recipe-permissions":
				os.Chmod(filepath.Join(s.Root, "recovery-recipes", id+".json"), 0644)
				w := comparisonRequest(t, s, "POST", "/api/recovery-recipes/preview", map[string]any{"recipe_id": id, "request": r}, nil)
				if w.Code == 200 {
					t.Fatal("public recipe file accepted")
				}
				return
			case "forged-public-proof":
				delete(snapshot.Target.Details, "machine_identity_sha256")
				snapshot.Target.QueryStates = map[string]InventoryQueryState{"machine_identity": {State: "ok"}}
				if len(recipeProof(snapshot, Mapping{})) == 0 {
					t.Fatal("public query states granted identity")
				}
				return
			case "old-snapshot":
				r.SnapshotID = p.TargetSnapshotID
				w := comparisonRequest(t, s, "POST", "/api/recovery-recipes/preview", map[string]any{"recipe_id": id, "request": r}, nil)
				var out RecoveryRecipePreview
				json.Unmarshal(w.Body.Bytes(), &out)
				if w.Code != 200 || out.Applicable || len(out.Mismatches) == 0 {
					t.Fatal("origin snapshot reused", w.Body.String())
				}
				return
			}
			original, err := s.loadPlanSnapshot(p)
			if err != nil {
				t.Fatal(err)
			}
			if recipeHash(recipeConstraints(snapshot)) == recipeHash(recipeConstraints(original)) {
				t.Fatal("changed storage/topology not bound")
			}
		})
	}
}

func TestRecipeRejectsIncompleteStorageBacking(t *testing.T) {
	s, _, id := recipeSetup(t)
	var rec recoveryRecipeRecord
	s.Store.Get("recovery-recipes", id, &rec)
	p, _ := s.Plan(rec.OriginPlanID)
	snapshot, err := s.loadPlanSnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Files = append(snapshot.Files, RecoveryReadFile{Path: "etc/pve/storage.cfg", State: "read", Content: []byte("nfs: same-name\n server 192.0.2.1\n")})
	snapshot.StorageVolumes = map[string]RecoveryVolumeResult{"same-name": {State: "ok", Volumes: []string{}}}
	if len(recipeProof(snapshot, Mapping{Storage: map[string]string{"source": "same-name"}})) == 0 {
		t.Fatal("partial NFS backing without export accepted")
	}
}

func TestRecipeRequiresPrivateVersionAndTopologyProof(t *testing.T) {
	s, _, id := recipeSetup(t)
	var rec recoveryRecipeRecord
	s.Store.Get("recovery-recipes", id, &rec)
	p, _ := s.Plan(rec.OriginPlanID)
	snapshot, err := s.loadPlanSnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Target.Details["command_results"] = json.RawMessage(`{"machine_identity":{"state":"ok"},"file_inventory":{"state":"ok"}}`)
	if len(recipeProof(snapshot, Mapping{})) < 2 {
		t.Fatal("version/topology strings without query proof accepted")
	}
}

func TestRecipeStoresIntentButNeverUpgradesMissingOriginProof(t *testing.T) {
	s, _, r := snapshotSetup(t)
	r.Mapping.Hostname = "reviewed"
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveRecoveryRecipe(p.ID, "Unvollständiger Nachweis", "reviewer", true)
	if err != nil {
		t.Fatalf("reviewed intent cannot be retained: %v", err)
	}
	i, err := s.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.SnapshotID = i.SnapshotID
	out, err := s.PreviewRecoveryRecipe(saved.ID, r)
	if err != nil || out.Applicable || len(out.Mismatches) == 0 {
		t.Fatal("missing origin proof upgraded", out, err)
	}
}
func TestRecipeBindsObservedDisksAndRequiresMountIdentity(t *testing.T) {
	snapshot := RecoverySnapshot{Target: Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"machine_identity_sha256": json.RawMessage(`"` + strings.Repeat("a", 64) + `"`), "command_results": json.RawMessage(`{"machine_identity":{"state":"ok"},"pve_version":{"state":"ok"},"cluster":{"state":"not_applicable"},"disks":{"state":"ok"}}`)}}, Files: []RecoveryReadFile{{Path: "etc/pve/storage.cfg", State: "read", Content: []byte("dir: local\n path /var/lib/vz\n")}}, StorageVolumes: map[string]RecoveryVolumeResult{"local": {State: "ok", Volumes: []string{}}}}
	mapping := Mapping{Storage: map[string]string{"local": "local"}}
	if len(recipeProof(snapshot, mapping)) == 0 {
		t.Fatal("config path without mount identity accepted")
	}
	snapshot.Target.Disks = []Disk{{Name: "sda1", ID: "disk-serial", UUID: "filesystem-uuid", Mount: "/var/lib/vz", Type: "part", Filesystem: "ext4"}}
	if missing := recipeProof(snapshot, mapping); len(missing) > 0 {
		t.Fatal("observed directory backing rejected", missing)
	}
	before := recipeHash(recipeConstraints(snapshot))
	snapshot.Target.Disks[0].UUID = "different-filesystem"
	if recipeHash(recipeConstraints(snapshot)) == before {
		t.Fatal("same storage ID/path with different actual backing accepted")
	}
}

func TestRecipePrivateRecordAndPublicProjectionNeverCopyAuthorization(t *testing.T) {
	s, _, id := recipeSetup(t)
	raw, err := readSnapshotBytes(filepath.Join(s.Root, "recovery-recipes", id+".json"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"console_confirmed", "source_offline", "decisions", "verification_attempts", "tasks", "result"} {
		if strings.Contains(string(raw), `"`+forbidden+`"`) {
			t.Fatal("authorization stored", forbidden)
		}
	}
	w := comparisonRequest(t, s, "GET", "/api/recovery-recipes", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, private := range []string{"constraints", "mapping", "manifest_sha", "machine_identity_sha256", "command_results", "backup_id"} {
		if strings.Contains(w.Body.String(), `"`+private+`"`) {
			t.Fatal("private recipe material exposed", w.Body.String())
		}
	}
	var row recoveryRecipeRecord
	s.Store.Get("recovery-recipes", id, &row)
	for _, field := range []string{"mapping", "reviewed_by", "reviewed_at"} {
		input := map[string]any{"plan_id": row.OriginPlanID, "label": "Forged", "reviewed": true, field: "forged"}
		w := comparisonRequest(t, s, "POST", "/api/recovery-recipes", input, nil)
		if w.Code == 200 {
			t.Fatal("client supplied review/intent authority accepted", field)
		}
	}
}

func TestRecipeNetworkTopologyMustBeObserved(t *testing.T) {
	s, _, id := recipeSetup(t)
	var rec recoveryRecipeRecord
	s.Store.Get("recovery-recipes", id, &rec)
	p, _ := s.Plan(rec.OriginPlanID)
	snapshot, err := s.loadPlanSnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	mapping := Mapping{Interfaces: map[string]string{"source": "eno1"}}
	if len(recipeProof(snapshot, mapping)) == 0 {
		t.Fatal("network mapping without captured topology accepted")
	}
	snapshot.Files = append(snapshot.Files, RecoveryReadFile{Path: "etc/network/interfaces", State: "unreadable"})
	if len(recipeProof(snapshot, mapping)) == 0 {
		t.Fatal("unreadable topology accepted")
	}
	snapshot.Files[len(snapshot.Files)-1].State = "read"
	if missing := recipeProof(snapshot, mapping); len(missing) != 0 {
		t.Fatal("observed topology rejected", missing)
	}
}
