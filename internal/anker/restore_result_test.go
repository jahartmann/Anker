package anker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type recoveryResultCollector struct {
	result         ApplyResult
	applyErr       error
	status         ApplyResult
	statusErr      error
	rollbackResult ApplyResult
	rollbacks      int
}

func recoveryTestInventory() Inventory {
	return Inventory{Hostname: "pve-test", PVEVersion: "8.4", Details: map[string]json.RawMessage{
		"file_hashes":   json.RawMessage(`{}`),
		"file_metadata": json.RawMessage(`{}`),
		"users":         json.RawMessage(`{"0":"root"}`),
		"groups":        json.RawMessage(`{"0":"root"}`),
		"capabilities":  json.RawMessage(`{"restore_protocol":2,"journal":true}`),
	}}
}
func (c *recoveryResultCollector) Probe(context.Context, Host) (Inventory, error) {
	return recoveryTestInventory(), nil
}
func (c *recoveryResultCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	if err := os.MkdirAll(filepath.Join(dest, "files/etc"), 0700); err != nil {
		return Collection{}, err
	}
	if err := os.WriteFile(filepath.Join(dest, "files/etc/app.conf"), []byte("enabled = true\n"), 0600); err != nil {
		return Collection{}, err
	}
	return Collection{Inventory: recoveryTestInventory(), Entries: []Entry{{Path: "etc/app.conf", Type: "file", Mode: 0600, UID: 0, GID: 0}}}, nil
}
func (c *recoveryResultCollector) Apply(context.Context, Host, Plan, string) (ApplyResult, error) {
	return c.result, c.applyErr
}
func (c *recoveryResultCollector) RestoreStatus(context.Context, Host, Plan) (ApplyResult, error) {
	return c.status, c.statusErr
}
func (c *recoveryResultCollector) Rollback(context.Context, Host, Plan, string) (ApplyResult, error) {
	c.rollbacks++
	return c.rollbackResult, nil
}
func recoveryReadyPlan(t *testing.T, c *recoveryResultCollector) (*Service, Plan) {
	t.Helper()
	s := testService(t)
	s.Collector = c
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "host1", Scenario: "files", Files: []string{"etc/app.conf"}})
	if err != nil || p.State != "ready" {
		t.Fatalf("plan: %+v %v", p, err)
	}
	return s, p
}

func TestReconcileConfirmsAppliedHostWithoutRepeatingRestore(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	p.State = "interrupted"
	s.savePlan(p)
	c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}, RollbackPath: "/host/rollback/" + p.ID}
	p, err := s.ReconcilePlan(context.Background(), p.ID)
	if err != nil || p.State != "checks_pending" || p.Result == nil || p.Result.OperationID != p.ID {
		t.Fatalf("reconcile: %+v %v", p, err)
	}
}

func TestReconcileRejectsAnotherOperationJournal(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	p.State = "failed"
	s.savePlan(p)
	c.status = ApplyResult{State: "applied", OperationID: "another-plan", Applied: []string{"etc/app.conf"}}
	if _, err := s.ReconcilePlan(context.Background(), p.ID); err == nil {
		t.Fatal("wrong operation accepted")
	}
	stored, _ := s.Plan(p.ID)
	if stored.State != "failed" || stored.Result != nil {
		t.Fatalf("wrong journal persisted: %+v", stored)
	}
}

func TestRollbackRequiresConfirmationAndFreshHostJournal(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	p.State = "checks_pending"
	s.savePlan(p)
	c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
	c.rollbackResult = ApplyResult{State: "rolled_back", OperationID: p.ID}
	if _, err := s.RollbackPlan(context.Background(), p.ID, "wrong"); err == nil || c.rollbacks != 0 {
		t.Fatal("wrong confirmation accepted")
	}
	c.statusErr = errors.New("host not reachable")
	if _, err := s.RollbackPlan(context.Background(), p.ID, p.ID); err == nil || c.rollbacks != 0 {
		t.Fatal("uninspected rollback accepted")
	}
	c.statusErr = nil
	p, err := s.RollbackPlan(context.Background(), p.ID, p.ID)
	if err != nil || p.State != "rolled_back" || c.rollbacks != 1 {
		t.Fatalf("rollback: %+v %v", p, err)
	}
}

func TestFailedApplyRetainsKnownRolledBackState(t *testing.T) {
	c := &recoveryResultCollector{result: ApplyResult{State: "rolled_back"}, applyErr: errors.New("disk full")}
	s, p := recoveryReadyPlan(t, c)
	c.result.OperationID = p.ID
	p, err := s.ApplyPlan(context.Background(), p.ID, p.ID)
	if err == nil || p.State != "rolled_back" || p.Result == nil {
		t.Fatalf("rollback result hidden: %+v %v", p, err)
	}
}

func TestRestorePreservesHostEvidenceOnFailure(t *testing.T) {
	s := testService(t)
	c := &recoveryResultCollector{result: ApplyResult{Applied: []string{"etc/app.conf"}, RollbackPath: "/var/lib/anker-host/rollback/test"}, applyErr: errors.New("second write failed")}
	s.Collector = c
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "host1", Scenario: "files", Files: []string{"etc/app.conf"}})
	if err != nil || p.State != "ready" {
		t.Fatalf("ready plan: %s %v", p.State, err)
	}
	p, err = s.ApplyPlan(context.Background(), p.ID, p.ID)
	if err == nil {
		t.Fatal("write error lost")
	}
	if p.Result == nil || len(p.Result.Applied) != 1 || p.Result.RollbackPath == "" {
		t.Fatalf("host evidence lost: %+v", p)
	}
	persisted, readErr := s.Plan(p.ID)
	if readErr != nil || persisted.Result == nil {
		t.Fatalf("host evidence not persistent: %+v %v", persisted, readErr)
	}
}
