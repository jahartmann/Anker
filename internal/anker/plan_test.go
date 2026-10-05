package anker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type targetCollector struct {
	fixtureCollector
	inv Inventory
}

func (c targetCollector) Probe(ctx context.Context, h Host) (Inventory, error) {
	if h.ID == "target" {
		return c.inv, nil
	}
	return c.fixtureCollector.Probe(ctx, h)
}
func setupPlan(t *testing.T) (*Service, Backup) {
	s := testService(t)
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	s.SaveHost(Host{ID: "target", Name: "new-pve", Address: "192.0.2.20", SSHUser: "anker", SSHPort: 22})
	s.Collector = targetCollector{inv: Inventory{Hostname: "new-pve", PVEVersion: "8.4", Interfaces: []Interface{{Name: "ens3"}}, Details: map[string]json.RawMessage{"file_hashes": json.RawMessage(`{}`)}}}
	return s, b
}
func TestPlanMapsPhysicalNetworkPort(t *testing.T) {
	s, b := setupPlan(t)
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "migration", Mapping: Mapping{Interfaces: map[string]string{"eno1": "ens3"}}, ConsoleConfirmed: true, SourceOffline: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.Root, "plans", p.ID, "prepared-files/etc/network/interfaces"))
	if err != nil || !strings.Contains(string(data), "bridge-ports ens3") {
		t.Fatalf("%s %v", data, err)
	}
}
func TestPlanBlocksUnmappedPort(t *testing.T) {
	s, b := setupPlan(t)
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "migration"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Blockers) == 0 {
		t.Fatal("ambiguous mapping accepted")
	}
}
func TestVersionChangeRequiresManualRules(t *testing.T) {
	s, b := setupPlan(t)
	c := s.Collector.(targetCollector)
	c.inv.PVEVersion = "9.1"
	s.Collector = c
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "version", ConsoleConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Manual) == 0 || p.State == "ready" {
		t.Fatal("untested version change auto-approved")
	}
}
func TestApplyRejectsWrongConfirmationAndDrift(t *testing.T) {
	s, b := setupPlan(t)
	p, _ := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces"}, Mapping: Mapping{Interfaces: map[string]string{"eno1": "ens3"}}, ConsoleConfirmed: true})
	if _, err := s.ApplyPlan(context.Background(), p.ID, "wrong"); err == nil {
		t.Fatal("no confirmation required")
	}
	c := s.Collector.(targetCollector)
	c.inv.Hostname = "changed"
	s.Collector = c
	if _, err := s.ApplyPlan(context.Background(), p.ID, p.ID); err == nil {
		t.Fatal("target drift accepted")
	}
}

func TestFingerprintIgnoresActivityButDetectsConfigDrift(t *testing.T) {
	a := Inventory{Hostname: "pve", Details: map[string]json.RawMessage{"cluster": json.RawMessage(`"Date: now"`), "file_hashes": json.RawMessage(`{"etc/a":"one"}`)}}
	b := Inventory{Hostname: "pve", Details: map[string]json.RawMessage{"cluster": json.RawMessage(`"Date: later"`), "file_hashes": json.RawMessage(`{"etc/a":"one"}`)}}
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("volatile cluster status invalidates plan")
	}
	b.Details["file_hashes"] = json.RawMessage(`{"etc/a":"two"}`)
	if Fingerprint(a) == Fingerprint(b) {
		t.Fatal("configuration drift ignored")
	}
}
