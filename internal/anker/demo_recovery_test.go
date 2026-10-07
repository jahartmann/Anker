package anker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitDemoStillSupportsReviewedOrdinaryFileRestore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".anker-mode"), []byte("demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := NewService(root, store, DemoCollector{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SeedDemo(NewAuth(store)); err != nil {
		t.Fatal(err)
	}
	backups, err := s.ListBackups("")
	if err != nil {
		t.Fatal(err)
	}
	var backup Backup
	for _, b := range backups {
		if b.HostID == "demo1" {
			backup = b
			break
		}
	}
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: backup.ID, TargetID: "demo1", Scenario: "files", Files: []string{"etc/sysctl.d/99-anker.conf"}})
	if err != nil || p.State != "ready" {
		t.Fatalf("demo plan no longer usable: %s manual=%v err=%v", p.State, p.Manual, err)
	}
	var users map[string]string
	if json.Unmarshal(p.Source.Details["users"], &users) != nil || users["0"] != "root" {
		t.Fatal("demo namespace identities missing")
	}
	p, err = s.ApplyPlan(context.Background(), p.ID, p.ID)
	if err != nil || p.State != "demo_applied" {
		t.Fatalf("demo apply: %+v %v", p, err)
	}
}

func TestRollbackMissingHostConfirmationStaysInterrupted(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	p.State = "checks_pending"
	s.savePlan(p)
	c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
	c.rollbackResult = ApplyResult{}
	p, err := s.RollbackPlan(context.Background(), p.ID, p.ID)
	if err == nil || p.State != "interrupted" {
		t.Fatalf("unconfirmed rollback treated as success: %+v %v", p, err)
	}
}
