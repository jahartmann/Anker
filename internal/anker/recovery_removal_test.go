package anker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func removalFixture(t *testing.T) (*Service, PlanRequest, string) {
	t.Helper()
	s, r, root := localRecoveryHelper(t)
	path := "etc/sysctl.d/later.conf"
	if err := os.WriteFile(filepath.Join(root, path), []byte("net.ipv4.ip_forward=1\n"), 0640); err != nil {
		t.Fatal(err)
	}
	r.Files = nil
	r.RemoveFiles = []string{path}
	r.Decisions = map[string]FileDecision{path: DecisionRemove}
	r.RemovalsConfirmed = true
	return s, r, root
}
func TestRemovalRealHelperRoundTrip(t *testing.T) {
	s, r, root := removalFixture(t)
	i, err := s.InspectRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.SnapshotID = i.SnapshotID
	preview, err := s.PreviewRecoveryFile(RecoveryPreviewRequest{PlanRequest: PlanRequest{BackupID: r.BackupID, TargetID: r.TargetID, Scenario: r.Scenario, RemoveFiles: r.RemoveFiles, SnapshotID: r.SnapshotID}, ManifestSHA: i.ManifestSHA, Path: r.RemoveFiles[0], Basis: "target"})
	if err != nil || preview.OriginalAction.Action != "remove" || len(preview.Comparison.Hunks) == 0 {
		t.Fatalf("preview %+v %v", preview, err)
	}
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil || p.State != "ready" || len(p.Steps) != 1 || p.Steps[0].Action != "remove" {
		t.Fatalf("plan %+v %v", p, err)
	}
	if err = s.verifyPlanFiles(p); err != nil {
		t.Fatal(err)
	}
	comparison, err := s.ComparePlanFile(p.ID, r.RemoveFiles[0], "target", false)
	if err != nil || len(comparison.Hunks) == 0 {
		t.Fatalf("comparison %+v %v", comparison, err)
	}
	p, err = s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, r.RemoveFiles[0])); !os.IsNotExist(err) {
		t.Fatal("file not removed", err)
	}
	if _, err = s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
		t.Fatal("duplicate execution accepted")
	}
	p, err = s.RollbackPlan(t.Context(), p.ID, p.ID)
	if err != nil || p.State != "rolled_back" {
		t.Fatalf("rollback %+v %v", p, err)
	}
	data, err := os.ReadFile(filepath.Join(root, r.RemoveFiles[0]))
	if err != nil || string(data) != "net.ipv4.ip_forward=1\n" {
		t.Fatal("original not restored", err)
	}
	st, _ := os.Stat(filepath.Join(root, r.RemoveFiles[0]))
	if st.Mode().Perm() != 0640 {
		t.Fatal("mode not restored")
	}
}
func TestRemovalRequiresConfirmationAndBoundAbsence(t *testing.T) {
	s, r, _ := removalFixture(t)
	unconfirmed := r
	unconfirmed.RemovalsConfirmed = false
	if _, err := s.CreatePlan(t.Context(), unconfirmed); err == nil {
		t.Fatal("unconfirmed removal accepted")
	}
	original := r
	original.RemoveFiles = []string{"etc/sysctl.d/a.conf"}
	original.Decisions = map[string]FileDecision{original.RemoveFiles[0]: DecisionRemove}
	if _, err := s.InspectRecovery(t.Context(), original); err == nil {
		t.Fatal("source file accepted as absent")
	}
	for _, path := range []string{"../etc/foreign", "etc/hostname", "etc/pve/storage.cfg", "etc/ssh/sshd_config", "etc/secret.key"} {
		bad := r
		bad.RemoveFiles = []string{path}
		if _, err := s.InspectRecovery(t.Context(), bad); err == nil {
			t.Fatalf("protected removal allowed %s", path)
		}
	}
	i, err := s.InspectRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.SnapshotID = i.SnapshotID
	changed := r
	changed.RemoveFiles = []string{"etc/sysctl.d/other.conf"}
	if _, err := s.CreatePlan(t.Context(), changed); err == nil {
		t.Fatal("changed removal scope reused snapshot")
	}
	keep := r
	keep.RemovalsConfirmed = false
	keep.Decisions = map[string]FileDecision{r.RemoveFiles[0]: DecisionKeep}
	p, err := s.CreatePlan(t.Context(), keep)
	if err != nil || p.Steps[0].Action != "keep" {
		t.Fatalf("keep not supported %+v %v", p, err)
	}
}
func TestRemovalRejectsTargetDriftAndKeepsForeignRecreation(t *testing.T) {
	s, r, root := removalFixture(t)
	p := localPlan(t, s, r)
	path := filepath.Join(root, r.RemoveFiles[0])
	if err := os.WriteFile(path, []byte("foreign change\n"), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyPlan(t.Context(), p.ID, p.ID); err == nil {
		t.Fatal("drift deleted")
	}
	p = localPlan(t, s, r)
	p, err := s.ApplyPlan(t.Context(), p.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("foreign recreation\n"), 0644); err != nil {
		t.Fatal(err)
	}
	p, err = s.RollbackPlan(t.Context(), p.ID, p.ID)
	if err == nil || p.State != "rollback_conflict" {
		t.Fatalf("conflict %+v %v", p, err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "foreign recreation\n" {
		t.Fatal("foreign file overwritten")
	}
}

func TestRemovalRejectsUnsafeTargetEvidenceAndOldHelper(t *testing.T) {
	s, r, _ := removalFixture(t)
	i, err := s.CaptureRecovery(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.SnapshotID = i.SnapshotID
	snapshot, err := s.loadRecoverySnapshotUnlocked(i.SnapshotID, r)
	if err != nil {
		t.Fatal(err)
	}
	var before RecoveryReadFile
	for _, f := range snapshot.Files {
		if f.Path == r.RemoveFiles[0] {
			before = f
		}
	}
	if reason := removalReason(before.Path, before, snapshot.Target); reason != "" {
		t.Fatal(reason)
	}
	target := snapshot.Target
	target.Details = map[string]json.RawMessage{}
	for k, v := range snapshot.Target.Details {
		target.Details[k] = v
	}
	target.Details["capabilities"] = json.RawMessage(`{"restore_protocol":2,"journal":true}`)
	if removalReason(before.Path, before, target) == "" {
		t.Fatal("old helper accepted")
	}
	bad := before
	bad.State = "unavailable"
	if removalReason(before.Path, bad, snapshot.Target) == "" {
		t.Fatal("unreadable file accepted")
	}
	bad = before
	copy := *bad.Metadata
	bad.Metadata = &copy
	bad.Metadata.XAttrs = map[string]string{"user.test": "YQ=="}
	if removalReason(before.Path, bad, snapshot.Target) == "" {
		t.Fatal("xattrs accepted")
	}
	bad = before
	bad.Content = []byte("password=private-secret\n")
	bad.SHA256 = Hash(bad.Content)
	if removalReason(before.Path, bad, snapshot.Target) == "" {
		t.Fatal("secret content accepted")
	}
}
