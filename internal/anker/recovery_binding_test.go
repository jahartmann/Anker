package anker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type bindingJournalCollector struct {
	*recoveryResultCollector
	read     func()
	rollback func()
	reads    int
}

func (c *bindingJournalCollector) RestoreStatus(ctx context.Context, h Host, p Plan) (ApplyResult, error) {
	c.reads++
	if c.read != nil {
		c.read()
	}
	return c.recoveryResultCollector.RestoreStatus(ctx, h, p)
}
func (c *bindingJournalCollector) Rollback(ctx context.Context, h Host, p Plan, id string) (ApplyResult, error) {
	if c.rollback != nil {
		c.rollback()
	}
	return c.recoveryResultCollector.Rollback(ctx, h, p, id)
}

func TestHistoricalRecoveryTargetBinding(t *testing.T) {
	for _, operation := range []string{"reconcile", "rollback"} {
		for _, change := range []string{"address", "trust", "during-read", "snapshot", "legacy"} {
			t.Run(operation+"/"+change, func(t *testing.T) {
				c := &recoveryResultCollector{}
				s, p := recoveryReadyPlan(t, c)
				p.State = "interrupted"
				if change == "legacy" {
					p.SchemaVersion = 1
				}
				if err := s.savePlan(p); err != nil {
					t.Fatal(err)
				}
				hook := &bindingJournalCollector{recoveryResultCollector: c}
				s.Collector = hook
				c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
				c.rollbackResult = ApplyResult{State: "rolled_back", OperationID: p.ID}
				mutate := func() {
					h, e := s.Host(p.TargetID)
					if e != nil {
						t.Fatal(e)
					}
					h.Address = "other-target"
					if e = s.Store.Put("hosts", h.ID, h); e != nil {
						t.Fatal(e)
					}
				}
				switch change {
				case "address":
					mutate()
				case "trust":
					h, _ := s.Host(p.TargetID)
					h.KnownHostsPath = filepath.Join(t.TempDir(), "known_hosts")
					os.WriteFile(h.KnownHostsPath, []byte("changed trust"), 0600)
					s.Store.Put("hosts", h.ID, h)
				case "during-read":
					hook.read = mutate
				case "snapshot":
					os.WriteFile(filepath.Join(s.Root, "plans", p.ID, "target-snapshot.json"), []byte("{}"), 0600)
				}
				var err error
				if operation == "reconcile" {
					_, err = s.ReconcilePlan(context.Background(), p.ID)
				} else {
					_, err = s.RollbackPlan(context.Background(), p.ID, p.ID)
				}
				if err == nil || c.rollbacks != 0 {
					t.Fatalf("changed/missing target binding accepted: err=%v writes=%d", err, c.rollbacks)
				}
				if change != "during-read" && hook.reads != 0 {
					t.Fatal("unbound journal read")
				}
				stored, _ := s.Plan(p.ID)
				if stored.State != p.State || stored.Result != nil {
					t.Fatal("unbound evidence replaced saved operation")
				}
			})
		}
	}
}
func TestHistoricalRecoveryWithoutSourceBytes(t *testing.T) {
	for _, operation := range []string{"reconcile", "rollback"} {
		t.Run(operation, func(t *testing.T) {
			c := &recoveryResultCollector{}
			s, p := recoveryReadyPlan(t, c)
			p.State = "interrupted"
			s.savePlan(p)
			b, _ := s.Backup(p.BackupID)
			if err := os.RemoveAll(s.backupDir(b)); err != nil {
				t.Fatal(err)
			}
			c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
			c.rollbackResult = ApplyResult{State: "rolled_back", OperationID: p.ID}
			var got Plan
			var err error
			if operation == "reconcile" {
				got, err = s.ReconcilePlan(context.Background(), p.ID)
			} else {
				got, err = s.RollbackPlan(context.Background(), p.ID, p.ID)
			}
			if err != nil || !strings.Contains("checks_pending rolled_back", got.State) {
				t.Fatalf("historical recovery requires source bytes: %s %v", got.State, err)
			}
		})
	}
}
func TestHistoricalRollbackConnectionChangesDuringMutation(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	p.State = "interrupted"
	s.savePlan(p)
	c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
	c.rollbackResult = ApplyResult{State: "rolled_back", OperationID: p.ID}
	hook := &bindingJournalCollector{recoveryResultCollector: c, rollback: func() { h, _ := s.Host(p.TargetID); h.Address = "other-target"; s.Store.Put("hosts", h.ID, h) }}
	s.Collector = hook
	got, err := s.RollbackPlan(context.Background(), p.ID, p.ID)
	if err == nil || got.State != "interrupted" || got.Result == nil || got.Result.State == "rolled_back" {
		t.Fatalf("changed connection granted success: %+v %v", got, err)
	}
}

func TestHistoricalRecoveryTrustBytesChanged(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(fmt.Sprint(during), func(t *testing.T) {
			c := &recoveryResultCollector{}
			s, p := recoveryReadyPlan(t, c)
			h, _ := s.Host(p.TargetID)
			h.KnownHostsPath = filepath.Join(t.TempDir(), "known_hosts")
			os.WriteFile(h.KnownHostsPath, []byte("original trust"), 0600)
			s.Store.Put("hosts", h.ID, h)
			p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: p.BackupID, TargetID: p.TargetID, Scenario: "files", Files: []string{"etc/app.conf"}, Decisions: map[string]FileDecision{"etc/app.conf": DecisionBackup}})
			if err != nil {
				t.Fatal(err)
			}
			p.State = "interrupted"
			s.savePlan(p)
			c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
			c.rollbackResult = ApplyResult{State: "rolled_back", OperationID: p.ID}
			hook := &bindingJournalCollector{recoveryResultCollector: c}
			s.Collector = hook
			change := func() {
				if err := os.WriteFile(h.KnownHostsPath, []byte("different trust"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if during {
				hook.read = change
			} else {
				change()
			}
			if _, err = s.RollbackPlan(context.Background(), p.ID, p.ID); err == nil || c.rollbacks != 0 {
				t.Fatal("trust-byte change allowed mutation", err)
			}
		})
	}
}

func TestHistoricalRecoverySourceChangeDoesNotRebindTarget(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	p.State = "interrupted"
	s.savePlan(p)
	b, _ := s.Backup(p.BackupID)
	b.ManifestSHA = Hash([]byte("changed source"))
	s.Store.Put("backups", b.ID, b)
	c.status = ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/app.conf"}}
	c.rollbackResult = ApplyResult{State: "rolled_back", OperationID: p.ID}
	if got, err := s.RollbackPlan(context.Background(), p.ID, p.ID); err != nil || got.State != "rolled_back" {
		t.Fatal("source change prevented original target rollback", err)
	}
}
