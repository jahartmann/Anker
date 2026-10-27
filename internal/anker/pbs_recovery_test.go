package anker

import (
	"encoding/json"
	"strings"
	"testing"
)

func pbsPolicySnapshot() RecoverySnapshot {
	config := []byte("datastore: main\n path /mnt/target\n backend type=local\n maintenance-mode type=read-only\n gc-schedule daily\n\ndatastore: extra\n path /mnt/extra\n")
	return RecoverySnapshot{Files: []RecoveryReadFile{{Path: pbsConfigRoot + "datastore.cfg", State: "read", SHA256: Hash(config), Content: config}}}
}
func TestPBSPreparationPreservesBackingAndDisablesDataActions(t *testing.T) {
	snapshot := pbsPolicySnapshot()
	source := []byte("datastore: main\n path /mnt/source\n backend type=s3,client=old,bucket=old\n maintenance-mode type=offline\n gc-schedule hourly\n prune-schedule hourly\n verify-new true\n gc-on-unmount true\n keep-last 5\n")
	prepared, err := preparePBSConfig(pbsConfigRoot+"datastore.cfg", source, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	text := string(prepared)
	for _, want := range []string{"path /mnt/target", "backend type=local", "maintenance-mode type=read-only", "datastore: extra", "keep-last 5"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	for _, forbidden := range []string{"/mnt/source", "type=s3", "gc-schedule", "prune-schedule", "verify-new", "gc-on-unmount"} {
		if strings.Contains(text, forbidden) {
			t.Fatal(text)
		}
	}
	if err := validatePBSPrepared(pbsConfigRoot+"datastore.cfg", prepared, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := validatePBSPrepared(pbsConfigRoot+"datastore.cfg", source, snapshot); err == nil {
		t.Fatal("raw datastore relocation accepted")
	}
	for _, tc := range []struct{ file, source, absent, present string }{
		{"sync.cfg", "sync: one\n store main\n remote-store other\n schedule daily\n run-on-mount true\n unmount-on-done true\n group-filter type:vm\n group-filter type:ct\n", "schedule", "store main"},
		{"prune.cfg", "prune: one\n store main\n schedule daily\n disable false\n", "disable false", "disable true"},
		{"verification.cfg", "verification: one\n store main\n schedule daily\n", "schedule", "store main"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			prepared, err := preparePBSConfig(pbsConfigRoot+tc.file, []byte(tc.source), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(prepared), tc.absent) || !strings.Contains(string(prepared), tc.present) {
				t.Fatal(string(prepared))
			}
			if err := validatePBSPrepared(pbsConfigRoot+tc.file, prepared, snapshot); err != nil {
				t.Fatal(err)
			}
			if err := validatePBSPrepared(pbsConfigRoot+tc.file, []byte(tc.source), snapshot); err == nil {
				t.Fatal("active source jobs accepted")
			}
		})
	}
}
func TestPBSMissingDatastoresAndInvalidSyntaxDoNotBecomeWritable(t *testing.T) {
	for _, data := range []string{"datastore: missing\n path /mnt/new\n", "datastore: main\n path /mnt/a\n path /mnt/b\n", "arbitrary: syntax\n"} {
		if _, err := preparePBSConfig(pbsConfigRoot+"datastore.cfg", []byte(data), pbsPolicySnapshot()); err == nil {
			t.Fatal(data)
		}
	}
	if _, err := preparePBSConfig(pbsConfigRoot+"sync.cfg", []byte("sync: one\n store missing\n"), pbsPolicySnapshot()); err == nil {
		t.Fatal("unknown datastore accepted")
	}
}
func TestPBSRestoreScopeAndVersionChecks(t *testing.T) {
	inv := Inventory{HostType: "pbs", PBSVersion: "4.0"}
	manifest := Manifest{Inventory: inv, Entries: []Entry{{Path: "etc/hosts", Type: "file"}, {Path: pbsConfigRoot + "remote.cfg", Type: "file"}}}
	for _, request := range []PlanRequest{{Scenario: "host"}, {Scenario: "files", Files: []string{"etc/hosts"}}, {Scenario: "files", RemoveFiles: []string{pbsConfigRoot + "remote.cfg"}}} {
		if _, err := scenarioSelection(manifest, request); err == nil {
			t.Fatal(request)
		}
	}
	if _, err := scenarioSelection(manifest, PlanRequest{Scenario: "files", Files: []string{pbsConfigRoot + "remote.cfg"}}); err != nil {
		t.Fatal(err)
	}
	if recoveryVersionsSupported(inv, Inventory{HostType: "pbs", PBSVersion: "3.4"}) || recoveryVersionsSupported(inv, Inventory{PVEVersion: "4.0"}) {
		t.Fatal("unsupported versions accepted")
	}
	target := Inventory{HostType: "pbs", PBSVersion: "4.1", Details: map[string]json.RawMessage{"capabilities": json.RawMessage(`{"pbs_restore_protocol":1}`), "pbs_services": json.RawMessage(`{"proxmox-backup":"inactive","proxmox-backup-proxy":"inactive"}`), "pbs_datastore_identity": json.RawMessage(`{}`), "command_results": json.RawMessage(`{"pbs_version":{"state":"ok"},"pbs_services":{"state":"ok"},"pbs_datastore_identity":{"state":"ok"},"file_inventory":{"state":"ok"},"users":{"state":"ok"},"groups":{"state":"ok"}}`)}}
	if blockers := pbsRecoveryBlockers(target); len(blockers) > 0 {
		t.Fatal(blockers)
	}
	target.Details["pbs_services"] = json.RawMessage(`{"proxmox-backup":"active","proxmox-backup-proxy":"inactive"}`)
	if len(pbsRecoveryBlockers(target)) == 0 {
		t.Fatal("running services accepted")
	}
}
