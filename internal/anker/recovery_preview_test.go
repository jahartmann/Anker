package anker

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func draftPreviewInput(t *testing.T, s *Service, r PlanRequest) map[string]any {
	t.Helper()
	i, err := s.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.SnapshotID = i.SnapshotID
	raw, _ := json.Marshal(r)
	var in map[string]any
	json.Unmarshal(raw, &in)
	b, _ := s.Backup(r.BackupID)
	in["manifest_sha"], in["path"], in["basis"] = b.ManifestSHA, r.Files[0], "source-target"
	return in
}
func draftPreview(t *testing.T, s *Service, in map[string]any) map[string]any {
	t.Helper()
	w := comparisonRequest(t, s, "POST", "/api/plans/preview", in, nil)
	if w.Code != 200 {
		t.Fatalf("preview %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestDraftPreviewBeforeDecisionAndNoPlan(t *testing.T) {
	s, _, r := snapshotSetup(t)
	in := draftPreviewInput(t, s, r)
	s.Collector = nil // A preview must not probe, collect or apply on the target.
	out := draftPreview(t, s, in)
	c := out["comparison"].(map[string]any)
	raw, _ := json.Marshal(c)
	if c["state"] != "different" || !strings.Contains(string(raw), "old\\n") || !strings.Contains(string(raw), "new\\n") {
		t.Fatal(string(raw))
	}
	if out["snapshot_id"] != in["snapshot_id"] || out["manifest_sha"] != in["manifest_sha"] {
		t.Fatal(out)
	}
	plans, _ := s.Plans()
	if len(plans) != 0 {
		t.Fatal("preview created a plan")
	}
	if files, err := os.ReadDir(filepath.Join(s.Root, "plans")); err != nil || len(files) != 0 {
		t.Fatal("preview wrote plan files", files, err)
	}
}
func TestDraftPreviewRejectsChangedBindings(t *testing.T) {
	for _, kind := range []string{"path", "selection", "hash", "snapshot", "target", "mapping", "connection", "contents", "permissions", "source", "snapshot-manifest", "source-replaced", "network-key", "storage"} {
		t.Run(kind, func(t *testing.T) {
			s, _, r := snapshotSetup(t)
			in := draftPreviewInput(t, s, r)
			switch kind {
			case "snapshot-manifest":
				os.WriteFile(filepath.Join(s.Root, "recovery-inspections", in["snapshot_id"].(string), "snapshot.json"), []byte(`{"files":[]}`), 0600)
			case "source-replaced":
				b, _ := s.Backup(r.BackupID)
				m, _ := s.Manifest(b.ID)
				m.Inventory.Hostname = "changed-source"
				raw, _ := json.Marshal(m)
				os.WriteFile(filepath.Join(s.backupDir(b), "manifest.json"), raw, 0600)
				b.ManifestSHA = Hash(raw)
				s.Store.Put("backups", b.ID, b)
				in["manifest_sha"] = b.ManifestSHA
			case "network-key":
				in["mapping"] = map[string]any{"network": map[string]any{"unknown": map[string]string{"address_cidr": "192.0.2.4/24"}}}
			case "storage":
				in["mapping"] = map[string]any{"storage": map[string]string{"unknown": "target"}}
			case "path":
				in["path"] = "../etc/shadow"
			case "selection":
				in["path"] = "etc/hosts"
			case "hash":
				in["manifest_sha"] = "wrong"
			case "snapshot":
				in["snapshot_id"] = ""
			case "target":
				in["target_id"] = "other"
			case "mapping":
				in["mapping"] = map[string]any{"interfaces": map[string]string{"bogus": "eno9"}}
			case "connection":
				h, _ := s.Host(r.TargetID)
				h.Address = "192.0.2.10"
				s.SaveHost(h)
			case "contents":
				os.WriteFile(filepath.Join(s.Root, "recovery-inspections", in["snapshot_id"].(string), "files", r.Files[0]), []byte("tampered"), 0600)
			case "permissions":
				os.Chmod(filepath.Join(s.Root, "recovery-inspections", in["snapshot_id"].(string), "snapshot.json"), 0644)
			case "source":
				b, _ := s.Backup(r.BackupID)
				os.WriteFile(filepath.Join(s.backupDir(b), "files", r.Files[0]), []byte("tampered"), 0600)
			}
			w := comparisonRequest(t, s, "POST", "/api/plans/preview", in, nil)
			if w.Code == 200 {
				t.Fatal("unsafe preview accepted", w.Body.String())
			}
		})
	}
}
func TestDraftPreviewSecretPermissionAndHiddenSide(t *testing.T) {
	s, _, r := snapshotSetup(t)
	c := s.Collector.(snapshotCollector)
	f := &c.result.Files[0]
	f.Content = []byte("password=private-value\n")
	f.SHA256 = Hash(f.Content)
	f.Metadata.SHA256 = f.SHA256
	f.Metadata.Size = int64(len(f.Content))
	hashes := fileHashes(c.inv)
	hashes[r.Files[0]] = f.SHA256
	c.inv.Details["file_hashes"], _ = json.Marshal(hashes)
	s.Collector = c
	in := draftPreviewInput(t, s, r)
	in["basis"] = "source"
	out := draftPreview(t, s, in)
	raw, _ := json.Marshal(out)
	if out["comparison"].(map[string]any)["state"] != "masked" || strings.Contains(string(raw), "private-value") {
		t.Fatal(string(raw))
	}
	for _, u := range []User{{ID: "reader", Role: "reader", Secrets: true}, {ID: "restore", Role: "restore"}} {
		in["reveal"] = true
		w := comparisonRequest(t, s, "POST", "/api/plans/preview", in, &u)
		if w.Code != http.StatusForbidden {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	in["basis"] = "source-target"
	out = draftPreview(t, s, in)
	raw, _ = json.Marshal(out)
	if !strings.Contains(string(raw), "private-value") {
		t.Fatal("authorized explicit disclosure unavailable", string(raw))
	}
}
func TestDraftPreviewIdenticalAndUnknownMetadata(t *testing.T) {
	for _, kind := range []string{"identical", "mode", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			s, _, r := snapshotSetup(t)
			c := s.Collector.(snapshotCollector)
			f := &c.result.Files[0]
			f.Content = []byte("new\n")
			f.SHA256 = Hash(f.Content)
			f.Metadata.SHA256 = f.SHA256
			f.Metadata.Mode = 0600
			if kind == "mode" {
				f.Metadata.Mode = 0644
			}
			hashes := fileHashes(c.inv)
			hashes[r.Files[0]] = f.SHA256
			c.inv.Details["file_hashes"], _ = json.Marshal(hashes)
			c.inv.Details["file_metadata"], _ = json.Marshal(map[string]any{r.Files[0]: map[string]any{"type": "file", "mode": f.Metadata.Mode, "uid": 0, "gid": 0, "xattrs": map[string]string{}}})
			if kind == "unknown" {
				f.State = "unreadable"
				f.Metadata = nil
				f.Content = nil
				f.SHA256 = ""
			}
			s.Collector = c
			in := draftPreviewInput(t, s, r)
			out := draftPreview(t, s, in)
			action := out["original_action"].(map[string]any)["action"]
			if kind == "identical" && action != "keep" {
				t.Fatal(out)
			}
			if kind != "identical" && action == "keep" {
				t.Fatal("unproven equality", out)
			}
			if kind == "unknown" && (action != "manual" || out["comparison"].(map[string]any)["state"] != "unavailable") {
				t.Fatal(out)
			}
		})
	}
}
func TestDraftPreviewPreparedMatchesSavedPlan(t *testing.T) {
	source := Inventory{Hostname: "old-host", PVEVersion: "8.4"}
	target := Inventory{Hostname: "target", PVEVersion: "8.4", Details: map[string]json.RawMessage{"file_hashes": json.RawMessage(`{"etc/hostname":"missing","etc/pve/storage.cfg":"missing"}`), "command_results": json.RawMessage(`{"file_inventory":{"state":"ok"}}`)}}
	s, b := recoverySetup(t, map[string]string{"etc/hostname": "old-host\n"}, source, target)
	s.Collector = snapshotCollector{targetCollector: targetCollector{inv: target}, result: RecoveryReadResult{Files: []RecoveryReadFile{{Path: "etc/hostname", State: "missing"}, {Path: "etc/pve/storage.cfg", State: "missing"}}, StorageVolumes: map[string]RecoveryVolumeResult{}}}
	r := PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/hostname"}, Mapping: Mapping{Hostname: "new-host"}}
	in := draftPreviewInput(t, s, r)
	in["basis"] = "source"
	out := draftPreview(t, s, in)
	raw, _ := json.Marshal(out["comparison"])
	if !strings.Contains(string(raw), "new-host\\n") || !strings.Contains(string(raw), "old-host\\n") {
		t.Fatal(string(raw))
	}
	r.SnapshotID = in["snapshot_id"].(string)
	r.Decisions = map[string]FileDecision{"etc/hostname": DecisionPrepared}
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.ComparePlanFile(p.ID, "etc/hostname", "source", false)
	if err != nil {
		t.Fatal(err)
	}
	savedRaw, _ := json.Marshal(saved)
	if string(savedRaw) != string(raw) { // JSON map ordering differs; compare decoded values.
		var a, b any
		json.Unmarshal(savedRaw, &a)
		json.Unmarshal(raw, &b)
		aa, _ := json.Marshal(a)
		bb, _ := json.Marshal(b)
		if string(aa) != string(bb) {
			t.Fatalf("draft %s saved %s", raw, savedRaw)
		}
	}
	if out["prepared_action"].(map[string]any)["action"] != p.Steps[0].Action {
		t.Fatal(out, p.Steps[0])
	}
}
