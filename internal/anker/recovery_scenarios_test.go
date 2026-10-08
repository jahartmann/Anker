package anker

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func scenarioManifest() Manifest {
	return Manifest{Inventory: Inventory{Hostname: "old"}, Entries: []Entry{
		{Path: "etc/pve/storage.cfg", Type: "file"},
		{Path: "etc/pve/nodes/old/host.fw", Type: "file"},
		{Path: "etc/pve/nodes/other/qemu-server/100.conf", Type: "file"},
		{Path: "etc/fstab", Type: "file"},
		{Path: "etc/ssh/ssh_host_ed25519_key", Type: "file"},
		{Path: "etc/sysctl.d/a.conf", Type: "file"},
		{Path: "etc/sysctl.d", Type: "directory"},
	}}
}

func TestClusterNodePreservesSharedAndForeignNodeFiles(t *testing.T) {
	selected, err := recoverySelection(scenarioManifest(), PlanRequest{Scenario: "cluster-node"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"etc/pve/storage.cfg", "etc/pve/nodes/other/qemu-server/100.conf", "etc/fstab", "etc/ssh/ssh_host_ed25519_key"} {
		if selected[p] {
			t.Errorf("preserved target path selected: %s", p)
		}
	}
	for _, p := range []string{"etc/pve/nodes/old/host.fw", "etc/sysctl.d/a.conf"} {
		if !selected[p] {
			t.Errorf("local recovery reference missing: %s", p)
		}
	}
}

func TestClusterNodeSelectionReportsPreservationAndManualReferences(t *testing.T) {
	m := scenarioManifest()
	for _, p := range []string{"etc/corosync/corosync.conf", "var/lib/pve-cluster/config.db", "etc/ceph/ceph.conf", "etc/pve/nodes/old-other/host.fw"} {
		m.Entries = append(m.Entries, Entry{Path: p, Type: "file"})
	}
	selection, err := scenarioSelection(m, PlanRequest{Scenario: "cluster-node"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selection.Paths, []string{"etc/pve/nodes/old/host.fw", "etc/sysctl.d/a.conf"}) {
		t.Fatalf("wrong cluster replacement references: %v", selection.Paths)
	}
	if len(selection.Preserved) != 8 {
		t.Fatalf("preserved paths not reported: %+v", selection.Preserved)
	}
	preservedPaths := []string{}
	for _, p := range selection.Preserved {
		preservedPaths = append(preservedPaths, p.Path)
		if p.Reason == "" {
			t.Errorf("missing preservation reason for %s", p.Path)
		}
	}
	if !reflect.DeepEqual(preservedPaths, []string{"etc/ceph/ceph.conf", "etc/corosync/corosync.conf", "etc/fstab", "etc/pve/nodes/old-other/host.fw", "etc/pve/nodes/other/qemu-server/100.conf", "etc/pve/storage.cfg", "etc/ssh/ssh_host_ed25519_key", "var/lib/pve-cluster/config.db"}) {
		t.Fatalf("wrong or unstable preservation list: %v", preservedPaths)
	}
	if fileManualReason(Entry{Path: "etc/pve/nodes/old/host.fw", Type: "file"}, Inventory{}, Inventory{}) == "" {
		t.Fatal("local node reference gained automatic permission")
	}
	m.Inventory.Hostname = ""
	selection, err = scenarioSelection(m, PlanRequest{Scenario: "cluster-node"})
	if err != nil || !reflect.DeepEqual(selection.Paths, []string{"etc/sysctl.d/a.conf"}) {
		t.Fatalf("unknown source node selected: %+v, %v", selection, err)
	}
}

func TestLegacyRecoverySituationsAndSelectionValidation(t *testing.T) {
	for scenario, want := range map[string]RecoverySituation{"files": "files", "host": "host", "cluster-node": "cluster-node", "cluster-disaster": "cluster-disaster", "standalone": "host", "migration": "host", "version": "host", "topology": "host"} {
		got, err := recoverySituation(scenario)
		if err != nil || got != want {
			t.Errorf("%s resolves to %q, %v; want %q", scenario, got, err, want)
		}
	}
	for _, r := range []PlanRequest{
		{Scenario: "unknown"}, {Scenario: "files"},
		{Scenario: "files", Files: []string{"../etc/fstab"}},
		{Scenario: "files", Files: []string{"etc/missing"}},
		{Scenario: "files", Files: []string{"etc/sysctl.d"}},
		{Scenario: "host", Files: []string{"etc/missing"}},
	} {
		if _, err := scenarioSelection(scenarioManifest(), r); err == nil {
			t.Errorf("invalid request accepted: %+v", r)
		}
	}
	for _, scenario := range []string{"host", "cluster-node"} {
		selection, err := scenarioSelection(scenarioManifest(), PlanRequest{Scenario: scenario, Files: []string{"etc/fstab", "etc/fstab"}})
		if err != nil || len(selection.Paths) != 0 || len(selection.Preserved) != 6 {
			t.Errorf("explicit whole-host selection bypassed preservation: %+v, %v", selection, err)
		}
	}
}

func TestFreshHostPreservesBootAndAccessIdentity(t *testing.T) {
	for _, scenario := range []string{"host", "standalone", "migration", "version", "topology", "cluster-node", "cluster-disaster"} {
		t.Run(scenario, func(t *testing.T) {
			m := scenarioManifest()
			for _, p := range []string{"etc/passwd", "etc/shadow", "etc/group", "etc/gshadow", "etc/machine-id", "var/lib/dbus/machine-id", "etc/crypttab", "etc/default/grub", "etc/kernel/cmdline", "etc/modules", "etc/modules-load.d/driver.conf", "boot/efi/EFI/proxmox/grubx64.efi", "root/.ssh/authorized_keys", "etc/sudoers.d/anker-restore", "etc/anker-host.json", "usr/local/lib/anker/anker-host", "etc/pam.d/sshd", "etc/nsswitch.conf"} {
				m.Entries = append(m.Entries, Entry{Path: p, Type: "file"})
			}
			selected, err := recoverySelection(m, PlanRequest{Scenario: scenario})
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range m.Entries {
				if e.Type == "directory" || strings.HasPrefix(e.Path, "etc/pve/") || e.Path == "etc/sysctl.d/a.conf" {
					continue
				}
				if selected[e.Path] {
					t.Errorf("target boot/access/account identity selected: %s", e.Path)
				}
			}
			if !selected["etc/sysctl.d/a.conf"] {
				t.Fatal("ordinary configuration missing")
			}
		})
	}
}

func TestExplicitFilesKeepProtectedRules(t *testing.T) {
	files := []string{"etc/pve/storage.cfg", "etc/fstab", "etc/ssh/ssh_host_ed25519_key"}
	selected, err := recoverySelection(scenarioManifest(), PlanRequest{Scenario: "files", Files: files})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != len(files) {
		t.Fatalf("explicit selection changed: %v", selected)
	}
	for _, path := range files {
		if !selected[path] || fileManualReason(Entry{Path: path, Type: "file"}, Inventory{}, Inventory{}) == "" {
			t.Errorf("protected explicit file lost manual restriction: %s", path)
		}
	}
	s, b := recoverySetup(t, map[string]string{"etc/pve/storage.cfg": "dir: archive\n path /srv/archive\n", "etc/fstab": "UUID=source / ext4 defaults 0 1\n", "etc/ssh/ssh_host_ed25519_key": "private host key\n"}, Inventory{PVEVersion: "8.4"}, Inventory{PVEVersion: "8.4"})
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: files})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 3 {
		t.Fatalf("protected selections were removed from manual plan: %+v", p.Steps)
	}
	for _, step := range p.Steps {
		if step.Action != "manual" {
			t.Errorf("explicit protected file gained write permission: %+v", step)
		}
	}
}

func TestLegacyRecoveryScenariosRemainReadable(t *testing.T) {
	for _, scenario := range []string{"standalone", "migration", "version", "topology"} {
		t.Run(scenario, func(t *testing.T) {
			selected, err := recoverySelection(scenarioManifest(), PlanRequest{Scenario: scenario})
			if err != nil {
				t.Fatal(err)
			}
			if selected["etc/fstab"] || selected["etc/ssh/ssh_host_ed25519_key"] {
				t.Fatalf("legacy whole-host scenario loses preservation: %v", selected)
			}
			s := testService(t)
			original := Plan{ID: "legacy", Scenario: scenario, State: "manual", Steps: []Step{{Path: "etc/fstab", Action: "manual"}}}
			if err := s.Store.Put("plans", original.ID, original); err != nil {
				t.Fatal(err)
			}
			loaded, err := s.Plan(original.ID)
			if err != nil || !reflect.DeepEqual(loaded, original) {
				t.Fatalf("legacy plan changed on read: %+v, %v", loaded, err)
			}
			if !strings.Contains(planGuide(loaded, Manifest{}), "Szenario "+scenario) {
				t.Fatal("legacy scenario missing from exported guide")
			}
		})
	}
}

func TestLegacyRecoveryVersionAndTopologyPreserveTargetPackageSources(t *testing.T) {
	m := Manifest{Entries: []Entry{{Path: "etc/apt/sources.list", Type: "file"}, {Path: "etc/os-release", Type: "file"}, {Path: "etc/debian_version", Type: "file"}, {Path: "etc/sysctl.d/a.conf", Type: "file"}}}
	for _, scenario := range []string{"version", "topology"} {
		selection, err := scenarioSelection(m, PlanRequest{Scenario: scenario})
		if err != nil || !reflect.DeepEqual(selection.Paths, []string{"etc/sysctl.d/a.conf"}) || len(selection.Preserved) != 3 {
			t.Errorf("%s selected target package/version state: %+v, %v", scenario, selection, err)
		}
	}
}

func TestClusterNodePlanKeepsSharedStateAndLocalFilesManual(t *testing.T) {
	content := map[string]string{
		"etc/pve/storage.cfg":                      "dir: archive\n path /srv/archive\n",
		"etc/pve/nodes/old/host.fw":                "[OPTIONS]\nenable: 1\n",
		"etc/pve/nodes/other/qemu-server/100.conf": "scsi0: archive:vm-100-disk-0\n",
	}
	s, b := recoverySetup(t, content, Inventory{Hostname: "old", PVEVersion: "8.4"}, Inventory{PVEVersion: "8.4"})
	r := PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "cluster-node", SourceOffline: true}
	i, err := s.InspectRecovery(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Preserved) != 2 || len(i.Storage) != 0 || i.Automatic {
		t.Fatalf("shared/foreign state leaked into inspection requirements: %+v", i)
	}
	p, err := s.CreatePlan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Path != "etc/pve/nodes/old/host.fw" || p.Steps[0].Action != "manual" {
		t.Fatalf("cluster plan gained unsafe shared/node writes: %+v", p.Steps)
	}
}

func TestFreshHostPlanPreservesIdentityWithoutAutomaticApproval(t *testing.T) {
	source := Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"users": json.RawMessage(`{"0":"root"}`), "groups": json.RawMessage(`{"0":"root"}`)}}
	target := Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"users": json.RawMessage(`{"0":"root"}`), "groups": json.RawMessage(`{"0":"root"}`), "file_hashes": json.RawMessage(`{}`), "file_metadata": json.RawMessage(`{}`)}}
	s, b := recoverySetup(t, map[string]string{"etc/fstab": "UUID=source / ext4 defaults 0 1\n", "etc/modules": "source_driver\n", "etc/modules-load.d/driver.conf": "source_storage_driver\n", "etc/sysctl.d/a.conf": "vm.swappiness=10\n"}, source, target)
	p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "host", SourceOffline: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Path != "etc/sysctl.d/a.conf" || p.Steps[0].Action != "apply" {
		t.Fatalf("preserved identity selected or ordinary file lacks write prerequisites: %+v", p.Steps)
	}
	if p.State != "manual" {
		t.Fatalf("whole-host production scenario lacks manual approval guard: %s", p.State)
	}
	if !strings.Contains(strings.Join(p.Manual, "\n"), "etc/fstab") {
		t.Fatal("preservation reason missing from legacy plan guidance")
	}
	for _, path := range []string{"etc/modules", "etc/modules-load.d/driver.conf"} {
		if !strings.Contains(strings.Join(p.Manual, "\n"), path) {
			t.Errorf("target driver preservation reason missing: %s", path)
		}
	}
}
