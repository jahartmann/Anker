package anker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recoveryFixture struct {
	fixtureCollector
	content map[string]string
	source  Inventory
}

func (c recoveryFixture) Collect(ctx context.Context, h Host, d string) (Collection, error) {
	entries := []Entry{}
	for p, data := range c.content {
		full := filepath.Join(d, "files", p)
		os.MkdirAll(filepath.Dir(full), 0700)
		os.WriteFile(full, []byte(data), 0600)
		entries = append(entries, Entry{Path: p, Type: "file", Mode: 0600})
	}
	return Collection{Inventory: c.source, Entries: entries, Warnings: []string{}}, nil
}
func recoverySetup(t *testing.T, content map[string]string, source, target Inventory) (*Service, Backup) {
	t.Helper()
	s := testService(t)
	s.Collector = recoveryFixture{content: content, source: source}
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	s.SaveHost(Host{ID: "target", Name: "target", Address: "192.0.2.2"})
	s.Collector = targetCollector{inv: target}
	return s, b
}
func TestRecoveryInspectionOnlySelectedRequirements(t *testing.T) {
	s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "vm.swappiness=10\n", "etc/network/interfaces": "iface eno1 inet manual\n", "etc/pve/storage.cfg": "zfspool: local-zfs\n pool rpool/data\n"}, Inventory{PVEVersion: "8.4", Interfaces: []Interface{{Name: "eno1"}}}, Inventory{PVEVersion: "8.4", Interfaces: []Interface{{Name: "ens3"}}})
	i, e := s.InspectRecovery(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/a.conf"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(i.Ports) != 0 || len(i.Storage) != 0 || i.RequiresConsole {
		t.Fatalf("irrelevant requirements: %+v", i)
	}
	after, _ := s.Backup(b.ID)
	if after.VerifiedAt != b.VerifiedAt {
		t.Fatal("inspection mutated backup verification state")
	}
	plans, _ := s.Plans()
	if len(plans) != 0 {
		t.Fatal("inspection created plan")
	}
}
func TestRecoveryPortDependenciesAndConservativeSuggestions(t *testing.T) {
	source := Inventory{PVEVersion: "8.4", Hostname: "old", Interfaces: []Interface{{Name: "eno1", MAC: "aa"}, {Name: "eno2", MAC: "bb"}, {Name: "eno9"}, {Name: "tap9"}, {Name: "bond0"}}}
	target := Inventory{PVEVersion: "8.4", Hostname: "new", Interfaces: []Interface{{Name: "ens3", MAC: "aa"}, {Name: "eno2", MAC: "different"}, {Name: "eno7", Type: "virtual"}, {Name: "veth0"}, {Name: "lo"}}}
	data := "auto vmbr0\niface vmbr0 inet static\n bridge-ports bond0.100\niface bond0 inet manual\n bond-slaves eno1 eno2\niface bond0.100 inet manual\n vlan-raw-device bond0\n"
	s, b := recoverySetup(t, map[string]string{"etc/network/interfaces": data}, source, target)
	i, e := s.InspectRecovery(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(i.Ports) != 2 || len(i.TargetPorts) != 2 {
		t.Fatalf("wrong physical requirements: %+v", i)
	}
	if i.Ports[0].Suggested != "ens3" || i.Ports[1].Suggested != "" {
		t.Fatalf("guessed mapping: %+v", i.Ports)
	}
	if !i.RequiresConsole || i.Automatic {
		t.Fatal("network restore marked automatic")
	}
}
func TestRecoveryRejectsDuplicateAndUnknownNetworkMapping(t *testing.T) {
	s, b := recoverySetup(t, map[string]string{"etc/network/interfaces": "iface bond0 inet manual\n bond-slaves eno1 eno2\n"}, Inventory{PVEVersion: "8.4", Interfaces: []Interface{{Name: "eno1"}, {Name: "eno2"}}}, Inventory{PVEVersion: "8.4", Interfaces: []Interface{{Name: "ens3"}}})
	p, e := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces"}, ConsoleConfirmed: true, Mapping: Mapping{Interfaces: map[string]string{"eno1": "ens3", "eno2": "ens3"}}})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Blockers) == 0 {
		t.Fatal("duplicate target mapping accepted")
	}
}
func TestRecoveryStorageUsesConfigReferences(t *testing.T) {
	s, b := recoverySetup(t, map[string]string{"etc/pve/storage.cfg": "zfspool: local-zfs\n pool rpool/data\ndir: archive\n path /srv/archive\n", "etc/pve/qemu-server/100.conf": "scsi0: local-zfs:vm-100-disk-0,size=8G\n"}, Inventory{PVEVersion: "8.4", Disks: []Disk{{Name: "sda"}, {Name: "sda1"}}}, Inventory{PVEVersion: "8.4"})
	i, e := s.InspectRecovery(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/pve/qemu-server/100.conf"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(i.Storage) != 1 || i.Storage[0].ID != "local-zfs" || i.Storage[0].Path != "rpool/data" {
		t.Fatalf("storage requirements: %+v", i.Storage)
	}
}
func TestRecoveryManualPreparedNetworkIsVerified(t *testing.T) {
	s, b := setupPlan(t)
	p, e := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces"}, ConsoleConfirmed: true, Mapping: Mapping{Interfaces: map[string]string{"eno1": "ens3"}}})
	if e != nil {
		t.Fatal(e)
	}
	if p.Steps[0].Action != "manual" || p.Steps[0].PreparedSHA == "" {
		t.Fatalf("unsafe network step: %+v", p.Steps)
	}
	os.WriteFile(filepath.Join(s.Root, "plans", p.ID, "prepared-files/etc/network/interfaces"), []byte("tampered"), 0600)
	if e = s.verifyPlanFiles(p); e == nil {
		t.Fatal("manual prepared file tampering ignored")
	}
}
func TestRecoveryIdentityAndTargetMetadataGate(t *testing.T) {
	source := Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"users": json.RawMessage(`{"0":"root"}`), "groups": json.RawMessage(`{"0":"root"}`)}}
	target := source
	target.Details = map[string]json.RawMessage{"users": json.RawMessage(`{"0":"other"}`), "groups": json.RawMessage(`{"0":"root"}`), "file_hashes": json.RawMessage(`{}`), "file_metadata": json.RawMessage(`{}`)}
	s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "vm.swappiness=10\n"}, source, target)
	p, e := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/a.conf"}})
	if e != nil {
		t.Fatal(e)
	}
	if p.Steps[0].Action != "manual" || !strings.Contains(p.Steps[0].Reason, "identität") {
		t.Fatalf("identity mismatch accepted: %+v", p.Steps)
	}
}

func TestRecoveryUnknownPhysicalAndUnsupportedNetworkHooksBlock(t *testing.T) {
	s, b := recoverySetup(t, map[string]string{"etc/network/interfaces": "iface vmbr0 inet static\n bridge-ports eth99\n post-up ip link set eth99 up\n"}, Inventory{PVEVersion: "8.4"}, Inventory{PVEVersion: "8.4"})
	i, e := s.InspectRecovery(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(i.Blockers) != 2 {
		t.Fatalf("unrecognized topology was accepted: %+v", i)
	}
}
func TestRecoveryRequiredInventoryFailureOnlyForRelevantSelection(t *testing.T) {
	source := Inventory{PVEVersion: "8.4", Interfaces: []Interface{{Name: "eno1"}}}
	target := Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"command_results": json.RawMessage(`{"pve_version":{"state":"ok"},"interfaces":{"state":"failed"},"addresses":{"state":"ok"},"routes":{"state":"ok"}}`)}}
	s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "vm.swappiness=10\n", "etc/network/interfaces": "iface eno1 inet manual\n"}, source, target)
	i, e := s.InspectRecovery(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/a.conf"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(i.Blockers) != 0 {
		t.Fatalf("irrelevant query failure blocks ordinary file: %+v", i.Blockers)
	}
	i, e = s.InspectRecovery(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(i.Blockers) != 1 || !strings.Contains(i.Blockers[0], "interfaces") {
		t.Fatalf("failed relevant query ignored: %+v", i.Blockers)
	}
}
func TestRecoveryIdentityMetadataAndFingerprint(t *testing.T) {
	source := Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"users": json.RawMessage(`{"0":"root"}`), "groups": json.RawMessage(`{"0":"root"}`)}}
	target := Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"users": json.RawMessage(`{"0":"root"}`), "groups": json.RawMessage(`{"0":"root"}`), "file_hashes": json.RawMessage(`{"etc/sysctl.d/a.conf":"old"}`), "file_metadata": json.RawMessage(`{"etc/sysctl.d/a.conf":{"type":"file","uid":0,"gid":0,"mode":384,"xattrs":{}}}`)}}
	s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "vm.swappiness=10\n"}, source, target)
	p, e := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/a.conf"}})
	if e != nil {
		t.Fatal(e)
	}
	if p.State != "ready" || p.Steps[0].Action != "apply" {
		t.Fatalf("valid safe ordinary file not ready: %+v", p)
	}
	before := Fingerprint(target)
	target.Details["file_metadata"] = json.RawMessage(`{"etc/sysctl.d/a.conf":{"type":"file","uid":99,"gid":0,"mode":384,"xattrs":{}}}`)
	if before == Fingerprint(target) {
		t.Fatal("unchanged bytes with changed metadata did not invalidate fingerprint")
	}
}
func TestRecoveryMappingDecisionsAreNeverSilentlyIgnored(t *testing.T) {
	s, b := recoverySetup(t, map[string]string{"etc/pve/storage.cfg": "zfspool: local-zfs\n pool rpool/data\n"}, Inventory{PVEVersion: "8.4"}, Inventory{PVEVersion: "8.4"})
	p, e := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/pve/storage.cfg"}, Mapping: Mapping{Storage: map[string]string{"local-zfs": "manual"}, Hostname: "new-node", Address: "192.0.2.4"}})
	if e != nil {
		t.Fatal(e)
	}
	joined := strings.Join(p.Manual, "\n")
	for _, decision := range []string{"local-zfs", "manual", "new-node", "192.0.2.4"} {
		if !strings.Contains(joined, decision) {
			t.Fatalf("decision disappeared: %s: %s", decision, joined)
		}
	}
}
func TestRecoveryAllWholeScenariosRemainManual(t *testing.T) {
	for _, scenario := range []string{"standalone", "migration", "cluster-node", "cluster-disaster", "version", "topology"} {
		t.Run(scenario, func(t *testing.T) {
			s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "vm.swappiness=10\n"}, Inventory{PVEVersion: "8.4"}, Inventory{PVEVersion: "8.4"})
			i, e := s.InspectRecovery(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: scenario})
			if e != nil {
				t.Fatal(e)
			}
			if i.Automatic || !i.RequiresSourceOffline {
				t.Fatalf("whole scenario auto approved: %+v", i)
			}
		})
	}
}

func TestRecoverySameNameOnDifferentHostDoesNotSuggest(t *testing.T) {
	source := Inventory{Hostname: "pve", PVEVersion: "8.4", Interfaces: []Interface{{Name: "eno1"}}}
	target := Inventory{Hostname: "pve", PVEVersion: "8.4", Interfaces: []Interface{{Name: "eno1"}}}
	s, b := recoverySetup(t, map[string]string{"etc/network/interfaces": "iface eno1 inet manual\n"}, source, target)
	i, e := s.InspectRecovery(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/network/interfaces"}})
	if e != nil {
		t.Fatal(e)
	}
	if len(i.Ports) != 1 || i.Ports[0].Suggested != "" {
		t.Fatalf("hostname/name guessed identity across hardware: %+v", i.Ports)
	}
}

func TestRecoveryMissingTargetHashesNeverReportedAutomatic(t *testing.T) {
	identities := map[string]json.RawMessage{"users": json.RawMessage(`{"0":"root"}`), "groups": json.RawMessage(`{"0":"root"}`), "file_metadata": json.RawMessage(`{}`)}
	s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "vm.swappiness=10\n"}, Inventory{PVEVersion: "8.4", Details: identities}, Inventory{PVEVersion: "8.4", Details: identities})
	i, e := s.InspectRecovery(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/a.conf"}})
	if e != nil {
		t.Fatal(e)
	}
	if i.Automatic || len(i.Blockers) == 0 {
		t.Fatalf("unknown content preconditions reported automatic: %+v", i)
	}
}
func TestRecoverySSHRequiresProtocol2AndRecordedQueries(t *testing.T) {
	ids := map[string]json.RawMessage{"users": json.RawMessage(`{"0":"root"}`), "groups": json.RawMessage(`{"0":"root"}`), "file_metadata": json.RawMessage(`{}`), "file_hashes": json.RawMessage(`{}`), "capabilities": json.RawMessage(`{"restore_protocol":1}`)}
	s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "vm.swappiness=10\n"}, Inventory{PVEVersion: "8.4", Details: ids}, Inventory{PVEVersion: "8.4", Details: ids})
	m, _ := s.Manifest(b.ID)
	target := s.Collector.(targetCollector).inv
	s.Collector = SSHCollector{}
	i, e := s.analyzeRecovery(b, m, target, PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/a.conf"}})
	if e != nil {
		t.Fatal(e)
	}
	if i.Automatic || len(i.Blockers) != 2 {
		t.Fatalf("legacy production helper accepted: %+v", i)
	}
}

func TestRecoveryNullMetadataAndUnreadableTargetRemainManual(t *testing.T) {
	source := Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"users": json.RawMessage(`{"0":"root"}`), "groups": json.RawMessage(`{"0":"root"}`)}}
	for _, metadata := range []string{`null`, `{"etc/sysctl.d/a.conf":{"type":"file","mode":384,"uid":0,"gid":0,"xattrs":{}}}`} {
		target := Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"users": source.Details["users"], "groups": source.Details["groups"], "file_metadata": json.RawMessage(metadata), "file_hashes": json.RawMessage(`{"etc/sysctl.d/a.conf":"unreadable"}`)}}
		s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "vm.swappiness=10\n"}, source, target)
		p, e := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "files", Files: []string{"etc/sysctl.d/a.conf"}})
		if e != nil {
			t.Fatal(e)
		}
		if p.Steps[0].Action != "manual" {
			t.Fatalf("unknown metadata/content became apply: %+v", p.Steps)
		}
	}
}

func TestRecoveryKnownNICReplacementDoesNotSuggestByNameOrSlot(t *testing.T) {
	p := Interface{Name: "eno1", MAC: "aa", PCI: "0000:01:00.0"}
	target := Inventory{Hostname: "pve", Interfaces: []Interface{{Name: "eno1", MAC: "bb", PCI: "0000:01:00.0"}}}
	if got := suggestedPort(p, Inventory{Hostname: "pve"}, target, true); got != "" {
		t.Fatalf("replacement hardware suggested by name/slot: %q", got)
	}
}

func TestRecoveryFreshClusterTargetAllowsNotApplicableButLiveFailuresBlock(t *testing.T) {
	for _, scenario := range []string{"cluster-node", "cluster-disaster", "topology"} {
		t.Run(scenario, func(t *testing.T) {
			target := Inventory{PVEVersion: "8.4", Details: map[string]json.RawMessage{"command_results": json.RawMessage(`{"pve_version":{"state":"ok"},"disks":{"state":"ok"},"packages":{"state":"ok"},"cluster":{"state":"not_applicable"}}`)}}
			s, b := recoverySetup(t, map[string]string{"etc/sysctl.d/a.conf": "vm.swappiness=10\n"}, Inventory{PVEVersion: "8.4"}, target)
			r := PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: scenario}
			i, e := s.InspectRecovery(context.Background(), r)
			if e != nil {
				t.Fatal(e)
			}
			if len(i.Blockers) != 0 {
				t.Fatalf("fresh isolated target blocked: %+v", i.Blockers)
			}
			target.ClusterID = "live-cluster"
			target.Details["command_results"] = json.RawMessage(`{"pve_version":{"state":"ok"},"disks":{"state":"ok"},"packages":{"state":"ok"},"cluster":{"state":"failed"}}`)
			s.Collector = targetCollector{inv: target}
			i, e = s.InspectRecovery(context.Background(), r)
			if e != nil {
				t.Fatal(e)
			}
			if len(i.Blockers) != 1 || !strings.Contains(i.Blockers[0], "cluster") {
				t.Fatalf("unknown live cluster state ignored: %+v", i.Blockers)
			}
		})
	}
}

func TestRecoveryControlAndAccessFilesRemainManual(t *testing.T) {
	for _, path := range []string{"usr/local/lib/anker/anker-host", "etc/anker-host.json", "etc/sudoers", "etc/sudoers.d/anker-restore", "etc/ssh/sshd_config", "etc/pam.d/sshd", "etc/nsswitch.conf"} {
		if !protectedPath(path) {
			t.Errorf("recovery/access control file allowed automatic restore: %s", path)
		}
	}
}

func TestRecoveryInterfaceAliasesFollowPhysicalPortMapping(t *testing.T) {
	source := Inventory{Interfaces: []Interface{{Name: "eno1", Type: "physical", Physical: true}}}
	raw := "auto eno1:1\niface eno1:1 inet static\n address 192.0.2.2/24\niface eno1.100:2 inet static\n address 192.0.2.3/24\n"
	refs, issues := networkReferences(map[string]string{"etc/network/interfaces": raw}, source)
	if len(issues) > 0 || len(refs) != 1 || refs[0] != "eno1" {
		t.Fatalf("aliases are not a single physical decision: %v %v", refs, issues)
	}
	prepared, _ := mapNetwork(raw, source, Inventory{}, Mapping{Interfaces: map[string]string{"eno1": "ens3"}})
	if strings.Contains(prepared, "eno1") || !strings.Contains(prepared, "ens3:1") || !strings.Contains(prepared, "ens3.100:2") {
		t.Fatalf("aliases were not prepared consistently: %s", prepared)
	}
}

func TestNetworkMappingDoesNotRewriteDNSCommentsOrHookCommands(t *testing.T) {
	raw := "# eno1 is the management uplink\nauto eno1 eno1.100\niface eno1 inet manual\niface vmbr0 inet static\n bridge-ports eno1.100\n dns-search eno1.example.com\n post-up logger eno1\n"
	prepared, _ := mapNetwork(raw, Inventory{}, Inventory{}, Mapping{Interfaces: map[string]string{"eno1": "ens3"}})
	for _, unchanged := range []string{"# eno1 is the management uplink", "dns-search eno1.example.com", "post-up logger eno1"} {
		if !strings.Contains(prepared, unchanged) {
			t.Errorf("non-port content rewritten: %s", prepared)
		}
	}
	if !strings.Contains(prepared, "auto ens3 ens3.100") || !strings.Contains(prepared, "bridge-ports ens3.100") {
		t.Fatal("port references not prepared")
	}
}
