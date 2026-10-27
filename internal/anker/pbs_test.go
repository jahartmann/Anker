package anker

import (
	"anker/internal/hostconnect"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type pbsCollector struct {
	fixtureCollector
	content string
}

func (c pbsCollector) Probe(context.Context, Host) (Inventory, error) {
	return Inventory{HostType: "pbs", PBSVersion: "4.0.1-1", Hostname: "pbs-one", Details: map[string]json.RawMessage{
		"command_results": json.RawMessage(`{"pbs_version":{"state":"ok"},"users":{"state":"ok"},"groups":{"state":"ok"},"file_inventory":{"state":"ok"}}`),
		"users":           json.RawMessage(`{"0":"root"}`), "groups": json.RawMessage(`{"0":"root"}`),
	}}, nil
}
func (c pbsCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	inv, _ := c.Probe(ctx, h)
	if err := os.MkdirAll(filepath.Join(dest, "files/etc/proxmox-backup"), 0700); err != nil {
		return Collection{}, err
	}
	if err := os.WriteFile(filepath.Join(dest, "files/etc/proxmox-backup/remote.cfg"), []byte(c.content), 0600); err != nil {
		return Collection{}, err
	}
	return Collection{Inventory: inv, Entries: []Entry{
		{Path: "etc/proxmox-backup", Type: "directory", Mode: 0750},
		{Path: "etc/proxmox-backup/remote.cfg", Type: "file", Mode: 0640},
	}, Profile: &CollectionProfile{Version: 1, Roots: []string{"/etc/proxmox-backup"}, DependencyEntryLimit: 10000, DependencyFileLimit: 1048576, DependencyReferenceLimit: 4096, DependencyScanComplete: true}}, nil
}
func TestPBSBackupVerifyCompareAndRecoveryBoundary(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	s.Collector = pbsCollector{content: "remote: other\n password first-encoded-value\n"}
	first, err := s.CreateBackup(ctx, "host1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "successful" || first.HostType != "pbs" {
		t.Fatal(first)
	}
	if err = s.VerifyBackup(first.ID); err != nil {
		t.Fatal(err)
	}
	m, err := s.Manifest(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m.Inventory.Platform() != "pbs" || strings.Contains(m.Consistency, "SQLite") || !m.Entries[1].Secret {
		t.Fatal(m)
	}
	if _, ok := m.Artifacts["recovery/config.db"]; ok {
		t.Fatal("PBS claims PVE database")
	}
	guide, err := os.ReadFile(filepath.Join(s.backupDir(first), "WIEDERHERSTELLUNG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(guide), "Datastore-Inhalte sind ausgeschlossen") || strings.Contains(string(guide), "pve-cluster") {
		t.Fatal(string(guide))
	}
	ready, err := s.BackupReadiness(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range ready.Items {
		if item.ID == "query.pbs_version" {
			found = item.State == "ok"
		}
		if item.ID == "query.pve_version" {
			t.Fatal("PBS requires PVE version")
		}
	}
	if !found {
		t.Fatal(ready)
	}
	s.Collector = pbsCollector{content: "remote: other\n password second-encoded-value\n"}
	second, err := s.CreateBackup(ctx, "host1")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.DiffBackups(first.ID, second.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, row := range rows {
		if row.Path == "etc/proxmox-backup/remote.cfg" {
			found = true
			if !row.Secret {
				t.Fatal("PBS secret exposed")
			}
		}
	}
	if !found {
		t.Fatal(rows)
	}
	comparison, err := s.CompareBackupFile(first.ID, second.ID, "etc/proxmox-backup/remote.cfg", false)
	if err != nil {
		t.Fatal(err)
	}
	if !comparison.Masked || len(comparison.Hunks) != 0 {
		t.Fatal(comparison)
	}
	comparison, err = s.CompareBackupFile(first.ID, second.ID, "etc/proxmox-backup/remote.cfg", true)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Masked || !comparison.ContentChanged || len(comparison.Hunks) == 0 {
		t.Fatal(comparison)
	}
	request := PlanRequest{BackupID: first.ID, TargetID: "host1", Scenario: "files", Files: []string{"etc/proxmox-backup/remote.cfg"}}
	if plan, err := s.CreatePlan(ctx, request); err != nil || plan.State != "blocked" {
		t.Fatalf("old PBS helper was not blocked: %s %v", plan.State, err)
	}
	if inspection, err := s.InspectRecovery(ctx, request); err != nil || len(inspection.Blockers) == 0 {
		t.Fatalf("old PBS helper was not blocked: %+v %v", inspection, err)
	}
	os.WriteFile(filepath.Join(s.backupDir(first), "files/etc/proxmox-backup/remote.cfg"), []byte("tampered"), 0600)
	if err = s.VerifyBackup(first.ID); err == nil {
		t.Fatal("tampered PBS backup accepted")
	}
}
func TestPublicPBSInventoryKeepsOnlyTypedSummaryAndQueryStates(t *testing.T) {
	inv := Inventory{HostType: "pbs", PBSVersion: "4.0", PBSDatastores: []PBSDatastore{{Name: "main", Path: "/mnt/backup"}}, PBSJobs: []PBSJob{{Kind: "Sync", Name: "daily", Store: "main", Schedule: "daily"}}, Details: map[string]json.RawMessage{
		"command_results":   json.RawMessage(`{"pbs_version":{"state":"ok"},"pbs_datastores":{"state":"ok"}}`),
		"pbs_remote_config": json.RawMessage(`"password=private"`),
	}}
	public := redactRecovery(inv)
	if public.Details != nil || public.QueryStates["pbs_version"].State != "ok" || len(public.PBSDatastores) != 1 || len(public.PBSJobs) != 1 {
		t.Fatal(public)
	}
}

func TestPBSEnrollmentAcceptsRecognizedInventory(t *testing.T) {
	s := testService(t)
	s.hostConnector = &connectionFixture{enroll: func(in hostconnect.EnrollRequest) (hostconnect.Enrollment, error) {
		return hostconnect.Enrollment{Address: in.Address, Port: 22, Fingerprint: in.Fingerprint, BackupUser: "anker", RestoreUser: "anker-restore", BackupKeyPath: "/etc/anker/keys/backup", RestoreKeyPath: "/etc/anker/keys/restore", KnownHostsPath: "/etc/anker/known_hosts", Inventory: json.RawMessage(`{"hostname":"pbs-one","host_type":"pbs","pbs_version":"4.0.1-1"}`)}, nil
	}}
	w := connectionRequest(s, "enroll", connectBody)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var h Host
	json.Unmarshal(w.Body.Bytes(), &h)
	if h.Inventory == nil || h.Inventory.Platform() != "pbs" || h.Name != "pbs-one" {
		t.Fatal(h)
	}
	saved, err := s.Host(h.ID)
	if err != nil || saved.Inventory.Platform() != "pbs" {
		t.Fatal(saved, err)
	}
}

func TestPBSRestoreTargetRejected(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	b, err := s.CreateBackup(ctx, "host1")
	if err != nil {
		t.Fatal(err)
	}
	s.Collector = pbsCollector{}
	request := PlanRequest{BackupID: b.ID, TargetID: "host1", Scenario: "files", Files: []string{"etc/network/interfaces"}}
	if _, err = s.InspectRecovery(ctx, request); err == nil || !strings.Contains(err.Error(), "PBS") {
		t.Fatalf("PVE restore onto PBS was accepted: %v", err)
	}
	if _, err = s.CreatePlan(ctx, request); err == nil || !strings.Contains(err.Error(), "PBS") {
		t.Fatalf("PVE plan onto PBS was accepted: %v", err)
	}
}
