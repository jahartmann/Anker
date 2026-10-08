package anker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// JSON exercises the client contract while allowing RED before additive types exist.
func requestDecision(t *testing.T, r PlanRequest, path, decision string) PlanRequest {
	t.Helper()
	raw, _ := json.Marshal(r)
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	fields["decisions"] = map[string]string{path: decision}
	raw, _ = json.Marshal(fields)
	json.Unmarshal(raw, &r)
	return r
}
func TestKeepAndIdenticalFilesNeverReachApply(t *testing.T) {
	for _, kind := range []string{"keep", "identical", "mtime-only", "mode-change"} {
		t.Run(kind, func(t *testing.T) {
			s, _, r := snapshotSetup(t)
			c := s.Collector.(snapshotCollector)
			if kind != "keep" {
				c.result.Files[0].Content = []byte("new\n")
				c.result.Files[0].SHA256 = Hash(c.result.Files[0].Content)
				c.result.Files[0].Metadata.SHA256 = c.result.Files[0].SHA256
				c.result.Files[0].Metadata.Mode = 0600
				if kind == "mtime-only" {
					c.result.Files[0].Metadata.MTime = 12345678
				}
				if kind == "mode-change" {
					c.result.Files[0].Metadata.Mode = 0644
				}
				hashes := fileHashes(c.inv)
				hashes[r.Files[0]] = c.result.Files[0].SHA256
				c.inv.Details["file_hashes"], _ = json.Marshal(hashes)
				c.inv.Details["file_metadata"], _ = json.Marshal(map[string]any{r.Files[0]: map[string]any{"type": "file", "mode": c.result.Files[0].Metadata.Mode, "uid": 0, "gid": 0, "xattrs": map[string]string{}}})
				s.Collector = c
			}
			decision := "backup"
			if kind == "keep" {
				decision = "keep"
			}
			p, err := s.CreatePlan(t.Context(), requestDecision(t, r, r.Files[0], decision))
			if err != nil {
				t.Fatal(err)
			}
			if kind == "mode-change" {
				if p.Steps[0].Action == "keep" {
					t.Fatal("metadata change incorrectly identical")
				}
				return
			}
			if kind == "keep" && p.Steps[0].ComparisonState != "different" {
				t.Fatal("available keep comparison hidden", p.Steps[0])
			}
			if p.Steps[0].Action != "keep" {
				t.Fatalf("expected keep: %+v", p.Steps[0])
			}
			if _, err = os.Stat(filepath.Join(s.Root, "plans", p.ID, "prepared-files", r.Files[0])); !os.IsNotExist(err) {
				t.Fatal("keep was prepared", err)
			}
			if _, err = s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
				t.Fatal("all-keep plan reached apply")
			}
		})
	}
}
func TestPlanRejectsSnapshotConnectionDrift(t *testing.T) {
	s, api, r := snapshotSetup(t)
	i, err := api.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.SnapshotID = i.SnapshotID
	h, _ := s.Host(r.TargetID)
	h.Address = "192.0.2.99"
	s.SaveHost(h)
	if _, err = s.CreatePlan(t.Context(), r); err == nil {
		t.Fatal("stale connection snapshot accepted")
	}
}
func TestProtectedDecisionCannotEscalate(t *testing.T) {
	s, b := setupPlan(t)
	r := PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces"}, ConsoleConfirmed: true, Mapping: Mapping{Interfaces: map[string]string{"eno1": "ens3"}}}
	p, err := s.CreatePlan(t.Context(), requestDecision(t, r, r.Files[0], "backup"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].Action != "manual" {
		t.Fatal("protected decision escalated")
	}
	data, err := os.ReadFile(filepath.Join(s.Root, "plans", p.ID, "prepared-files", r.Files[0]))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("ens3")) {
		t.Fatal("backup decision unexpectedly transformed original")
	}
}
func TestLegacyPlanReadableButNewApplyNeedsComparison(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	raw, _ := json.Marshal(p)
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	delete(fields, "schema_version")
	raw, _ = json.Marshal(fields)
	json.Unmarshal(raw, &p)
	// Reset through a fresh value because absent JSON fields retain old values.
	var legacy Plan
	json.Unmarshal(raw, &legacy)
	p = legacy
	s.savePlan(p)
	if _, err := s.Plan(p.ID); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := s.ExportPlan(p.ID, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
		t.Fatal("legacy plan got new apply approval")
	}
	if _, err := s.QueueApply(p.ID, p.ID); err == nil {
		t.Fatal("legacy plan got queued approval")
	}
}
func TestManualChecksDoNotClaimRebootProof(t *testing.T) {
	s, _, r := snapshotSetup(t)
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	api, ok := any(s).(interface {
		ConfirmRecoveryTask(context.Context, string, string, string, string) (Plan, error)
	})
	if !ok {
		t.Fatal("manual task confirmation missing")
	}
	if _, err = api.ConfirmRecoveryTask(t.Context(), p.ID, "unknown", "user1", "done"); err == nil {
		t.Fatal("unauthenticated unknown task accepted")
	}
}

func TestPlanCopiedEvidenceSurvivesInspectionAndRejectsTamper(t *testing.T) {
	for _, part := range []string{"none", "content", "manifest", "missing"} {
		t.Run(part, func(t *testing.T) {
			s, _, r := snapshotSetup(t)
			p, err := s.CreatePlan(t.Context(), r)
			if err != nil {
				t.Fatal(err)
			}
			os.RemoveAll(filepath.Join(s.Root, "recovery-inspections", p.TargetSnapshotID))
			root := filepath.Join(s.Root, "plans", p.ID)
			switch part {
			case "content":
				os.WriteFile(filepath.Join(root, "target-files", r.Files[0]), []byte("bad"), 0600)
			case "manifest":
				os.WriteFile(filepath.Join(root, "target-snapshot.json"), []byte("{}"), 0600)
			case "missing":
				os.Remove(filepath.Join(root, "target-files", r.Files[0]))
			}
			var out bytes.Buffer
			err = s.ExportPlan(p.ID, &out)
			if part == "none" && err != nil {
				t.Fatal("lost private copy", err)
			}
			if part != "none" && (err == nil || out.Len() != 0) {
				t.Fatal("tampered evidence exported", err, out.Len())
			}
		})
	}
}
func TestManualChecksRequireActorDependenciesAndPreserveMachineResult(t *testing.T) {
	s, p := recoveryReadyPlan(t, &recoveryResultCollector{})
	p.State = "checks_pending"
	p.Result = &ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
	if err := s.savePlan(p); err != nil {
		t.Fatal(err)
	}
	var err error
	ctx := WithRecoveryActor(t.Context(), "user1")
	if _, err = s.ConfirmRecoveryTask(ctx, p.ID, "check-services", "other", "verified"); err == nil {
		t.Fatal("actor mismatch accepted")
	}
	if _, err = s.ConfirmRecoveryTask(ctx, p.ID, "check-reboot", "user1", "verified"); err == nil {
		t.Fatal("prerequisites skipped")
	}
	for _, id := range []string{"check-services", "check-reachability", "check-reboot"} {
		p, err = s.ConfirmRecoveryTask(ctx, p.ID, id, "user1", "Prüfung vor Ort dokumentiert")
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, task := range p.Tasks {
		if task.State != "user_confirmed" || task.ConfirmedBy != "user1" || task.ConfirmedAt == "" {
			t.Fatal("missing attestation", task)
		}
	}
	guide := planArchive(t, s, p.ID)["WIEDERHERSTELLUNG.md"]
	if !bytes.Contains(guide, []byte("vom Benutzer bestätigt")) || !bytes.Contains(guide, []byte("user1")) {
		t.Fatal("export loses actor attribution for confirmed manual tasks")
	}
	if p.State != "checks_pending" || p.Result == nil || p.Result.RebootVerified {
		t.Fatal("manual statement invented machine result", p)
	}
	p.Blockers = []string{"unresolved"}
	p.Tasks[0].State = "pending"
	s.savePlan(p)
	if _, err = s.ConfirmRecoveryTask(ctx, p.ID, "check-services", "user1", "verified"); err == nil {
		t.Fatal("blocker overridden")
	}
}
func TestPlanDefaultRequiresExplicitDecision(t *testing.T) {
	s, p := recoveryReadyPlan(t, &recoveryResultCollector{})
	r := PlanRequest{BackupID: p.BackupID, TargetID: p.TargetID, Scenario: "files", Files: []string{"etc/app.conf"}}
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	if p.State == "ready" || p.Steps[0].Action == "apply" {
		t.Fatal("new snapshot granted implicit write")
	}
}

type decisionCollector struct {
	targetCollector
	result         RecoveryReadResult
	writes         []string
	receivedTarget Inventory
	onProbe        func()
}

func (c *decisionCollector) Probe(ctx context.Context, h Host) (Inventory, error) {
	inv, err := c.targetCollector.Probe(ctx, h)
	if c.onProbe != nil {
		c.onProbe()
	}
	return inv, err
}
func (c *decisionCollector) ReadRecoveryFiles(_ context.Context, _ Host, paths, ids []string) (RecoveryReadResult, error) {
	result := RecoveryReadResult{Files: []RecoveryReadFile{}, StorageVolumes: map[string]RecoveryVolumeResult{}}
	for _, path := range paths {
		f := RecoveryReadFile{Path: path, State: "missing"}
		for _, got := range c.result.Files {
			if got.Path == path {
				f = got
			}
		}
		result.Files = append(result.Files, f)
	}
	return result, nil
}
func (c *decisionCollector) Apply(_ context.Context, _ Host, p Plan, _ string) (ApplyResult, error) {
	c.receivedTarget = p.Target
	for _, step := range p.Steps {
		if step.Action == "apply" {
			c.writes = append(c.writes, step.Path)
		}
	}
	return ApplyResult{State: "applied", OperationID: p.ID, Applied: append([]string{}, c.writes...)}, nil
}
func decisionTarget(files map[string]string) (*decisionCollector, Inventory) {
	c := &decisionCollector{result: RecoveryReadResult{Files: []RecoveryReadFile{}}}
	inv := recoveryTestInventory()
	hashes := map[string]string{}
	meta := map[string]any{}
	for path, data := range files {
		hash := Hash([]byte(data))
		hashes[path] = hash
		e := Entry{Path: path, Type: "file", Mode: 0600, Size: int64(len(data)), SHA256: hash, XAttrs: map[string]string{}}
		c.result.Files = append(c.result.Files, RecoveryReadFile{Path: path, State: "read", SHA256: hash, Content: []byte(data), Metadata: &e})
		meta[path] = map[string]any{"type": "file", "mode": 0600, "uid": 0, "gid": 0, "xattrs": map[string]string{}}
	}
	inv.Details["file_hashes"], _ = json.Marshal(hashes)
	inv.Details["file_metadata"], _ = json.Marshal(meta)
	c.inv = inv
	return c, inv
}
func TestKeepDriftDoesNotBlockOtherExplicitWrite(t *testing.T) {
	for _, drift := range []string{"keep", "active", "metadata", "connection", "connection-during-probe", "evidence"} {
		t.Run(drift, func(t *testing.T) {
			c, inv := decisionTarget(map[string]string{"etc/sysctl.d/active.conf": "old\n", "etc/sysctl.d/keep.conf": "target\n"})
			s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/active.conf": "new\n", "etc/sysctl.d/keep.conf": "backup\n"}, recoveryTestInventory(), inv)
			s.Collector = c
			p, err := s.CreatePlan(t.Context(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/active.conf", "etc/sysctl.d/keep.conf"}, Decisions: map[string]FileDecision{"etc/sysctl.d/active.conf": DecisionBackup, "etc/sysctl.d/keep.conf": DecisionKeep}})
			if err != nil || p.State != "ready" {
				t.Fatalf("plan %s %v %+v", p.State, err, p.Blockers)
			}
			if _, err = os.Stat(filepath.Join(s.Root, "plans", p.ID, "prepared-files/etc/sysctl.d/keep.conf")); !os.IsNotExist(err) {
				t.Fatal("kept file prepared")
			}
			hashes := fileHashes(c.inv)
			switch drift {
			case "keep":
				hashes["etc/sysctl.d/keep.conf"] = Hash([]byte("later"))
				c.inv.Details["file_hashes"], _ = json.Marshal(hashes)
			case "active":
				hashes["etc/sysctl.d/active.conf"] = Hash([]byte("later"))
				c.inv.Details["file_hashes"], _ = json.Marshal(hashes)
			case "metadata":
				c.inv.Details["file_metadata"] = json.RawMessage(`{"etc/sysctl.d/active.conf":{"type":"file","mode":420,"uid":0,"gid":0,"xattrs":{}}}`)
			case "connection":
				h, _ := s.Host("target")
				h.Address = "192.0.2.9"
				s.SaveHost(h)
			case "connection-during-probe":
				c.onProbe = func() {
					h, _ := s.Host("target")
					h.Address = "192.0.2.8"
					if err := s.SaveHost(h); err != nil {
						t.Fatal(err)
					}
				}
			case "evidence":
				os.WriteFile(filepath.Join(s.Root, "plans", p.ID, "target-files/etc/sysctl.d/keep.conf"), []byte("changed"), 0600)
			}
			_, err = s.ApplyPlan(t.Context(), p.ID, p.ID)
			if drift == "keep" {
				if err != nil || len(c.writes) != 1 || c.writes[0] != "etc/sysctl.d/active.conf" {
					t.Fatalf("keep blocked/written: %v %v", c.writes, err)
				}
				if fileHashes(c.receivedTarget)["etc/sysctl.d/keep.conf"] != Hash([]byte("later")) {
					t.Fatal("helper did not receive fresh safety inventory")
				}
			} else if err == nil || len(c.writes) != 0 {
				t.Fatalf("drift reached host writes: %s %v", drift, err)
			}
		})
	}
}
func TestPlanSecretDetectionIncludesTargetContent(t *testing.T) {
	c, inv := decisionTarget(map[string]string{"etc/sysctl.d/a.conf": "password=target-only-secret\n"})
	s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "safe\n"}, recoveryTestInventory(), inv)
	s.Collector = c
	p, err := s.CreatePlan(t.Context(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/a.conf"}, Decisions: map[string]FileDecision{"etc/sysctl.d/a.conf": DecisionBackup}})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Steps[0].Secret {
		t.Fatal("target secret flag missing")
	}
	secret, err := s.PlanContainsSecrets(p.ID)
	if err != nil || !secret {
		t.Fatal("target-only secret ignored", err)
	}
	raw, _ := os.ReadFile(filepath.Join(s.Root, "plans", p.ID, "plan.json"))
	if bytes.Contains(raw, []byte("target-only-secret")) {
		t.Fatal("raw target leaked to plan metadata")
	}
	os.Remove(filepath.Join(s.Root, "plans", p.ID, "target-files/etc/sysctl.d/a.conf"))
	if _, err = s.PlanContainsSecrets(p.ID); err == nil {
		t.Fatal("secret permission probe ignored damaged evidence")
	}
}
func TestManualChecksRejectUnknownKindsAndPrematureOperationChecks(t *testing.T) {
	s, p := recoveryReadyPlan(t, &recoveryResultCollector{})
	ctx := WithRecoveryActor(t.Context(), "user")
	if _, err := s.ConfirmRecoveryTask(ctx, p.ID, "check-services", "user", "done"); err == nil {
		t.Fatal("operation check before automatic apply accepted")
	}
	p.State = "manual"
	p.Tasks = []RecoveryTask{{ID: "unknown", Kind: "future-machine-evidence", State: "pending"}}
	s.savePlan(p)
	if _, err := s.ConfirmRecoveryTask(ctx, p.ID, "unknown", "user", "done"); err == nil {
		t.Fatal("unknown task kind user-confirmable")
	}
}

func planArchive(t *testing.T, s *Service, id string) map[string][]byte {
	t.Helper()
	var b bytes.Buffer
	if err := s.ExportPlan(id, &b); err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	r := tar.NewReader(&b)
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		out[h.Name] = data
	}
	return out
}
func TestClusterPlanPreservesSharedFilesInExport(t *testing.T) {
	c, inv := decisionTarget(map[string]string{"etc/pve/storage.cfg": "dir: live\n path /live\n"})
	source := recoveryTestInventory()
	source.Hostname = "old"
	s, b := recoverySetup(t, map[string]string{"etc/pve/storage.cfg": "dir: backup\n path /backup\n", "etc/pve/nodes/old/host.fw": "[OPTIONS]\nenable: 1\n"}, source, inv)
	s.Collector = c
	p, err := s.CreatePlan(t.Context(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "cluster-node", SourceOffline: true})
	if err != nil {
		t.Fatal(err)
	}
	files := planArchive(t, s, p.ID)
	if _, ok := files["prepared-files/etc/pve/storage.cfg"]; ok {
		t.Fatal("shared cluster file prepared")
	}
	if string(files["target-before/etc/pve/storage.cfg"]) != "dir: live\n path /live\n" {
		t.Fatal("live cluster evidence missing")
	}
	if string(files["original/original-files/etc/pve/storage.cfg"]) != "dir: backup\n path /backup\n" {
		t.Fatal("original excluded source lost")
	}
	if !strings.Contains(string(files["WIEDERHERSTELLUNG.md"]), "Gemeinsame Cluster-") {
		t.Fatal("preservation reason missing")
	}
}
func TestPlanCopiesSupplementalTopologyPrivately(t *testing.T) {
	c, inv := decisionTarget(map[string]string{"etc/network/interfaces": "auto lo\niface lo inet loopback\n", "etc/network/interfaces.d/existing": "iface vmbr0 inet manual\n bridge-ports ens3\n"})
	inv.Interfaces = []Interface{{Name: "ens3", Type: "physical"}}
	c.inv = inv
	source := recoveryTestInventory()
	source.Interfaces = []Interface{{Name: "eno1", Type: "physical"}}
	s, b := recoverySetup(t, map[string]string{"etc/network/interfaces.d/new": "iface vmbr1 inet manual\n bridge-ports eno1\n"}, source, inv)
	s.Collector = c
	p, err := s.CreatePlan(t.Context(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces.d/new"}, Decisions: map[string]FileDecision{"etc/network/interfaces.d/new": DecisionPrepared}, Mapping: Mapping{Interfaces: map[string]string{"eno1": "ens3"}}, ConsoleConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(s.Root, "recovery-inspections", p.TargetSnapshotID))
	files := planArchive(t, s, p.ID)
	if string(files["target-before/etc/network/interfaces.d/existing"]) != "iface vmbr0 inet manual\n bridge-ports ens3\n" {
		t.Fatal("supplemental topology lost")
	}
	for _, name := range []string{"target-files/etc/network/interfaces.d/existing", "target-files/etc/network/interfaces", "target-snapshot.json"} {
		st, err := os.Stat(filepath.Join(s.Root, "plans", p.ID, name))
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatal("private evidence missing/unprotected", name, err)
		}
	}
	os.Remove(filepath.Join(s.Root, "plans", p.ID, "target-files/etc/network/interfaces.d/existing"))
	var out bytes.Buffer
	if err = s.ExportPlan(p.ID, &out); err == nil {
		t.Fatal("missing supplemental evidence accepted")
	}
}

func TestPlanRejectsSupplementalTopologyDrift(t *testing.T) {
	c, inv := decisionTarget(map[string]string{"etc/network/interfaces": "auto lo\niface lo inet loopback\n", "etc/network/interfaces.d/existing": "iface vmbr0 inet manual\n"})
	s, b := recoverySetup(t, map[string]string{"etc/network/interfaces": "auto lo\niface lo inet loopback\n"}, recoveryTestInventory(), inv)
	s.Collector = c
	r := PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces"}, Decisions: map[string]FileDecision{"etc/network/interfaces": DecisionPrepared}}
	i, err := s.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.SnapshotID = i.SnapshotID
	hashes := fileHashes(c.inv)
	hashes["etc/network/interfaces.d/existing"] = Hash([]byte("iface changed inet manual\n"))
	c.inv.Details["file_hashes"], _ = json.Marshal(hashes)
	if _, err = s.CreatePlan(t.Context(), r); err == nil {
		t.Fatal("changed supplemental topology used for preparation")
	}
}

func TestKeepProtectedFileDoesNotRequireWriteMapping(t *testing.T) {
	c, inv := decisionTarget(map[string]string{"etc/network/interfaces": "iface ens3 inet manual\n"})
	inv.Interfaces = []Interface{{Name: "ens3", Type: "physical"}}
	c.inv = inv
	source := recoveryTestInventory()
	source.Interfaces = []Interface{{Name: "eno1", Type: "physical"}}
	s, b := recoverySetup(t, map[string]string{"etc/network/interfaces": "iface eno1 inet manual\n", "etc/sysctl.d/a.conf": "new\n"}, source, inv)
	s.Collector = c
	p, err := s.CreatePlan(t.Context(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces", "etc/sysctl.d/a.conf"}, Decisions: map[string]FileDecision{"etc/network/interfaces": DecisionKeep, "etc/sysctl.d/a.conf": DecisionBackup}})
	if err != nil || p.State != "ready" {
		t.Fatalf("kept protected path requires write mapping: %s %v %v", p.State, p.Blockers, err)
	}
}
