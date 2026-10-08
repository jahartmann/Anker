package anker

import (
	"encoding/json"
	"strings"
	"testing"
)

func preparationSnapshot() RecoverySnapshot {
	cfg := []byte("lvmthin: new\n vgname data\n thinpool guests\n content images,rootdir\n")
	return RecoverySnapshot{Source: Inventory{Hostname: "old", Interfaces: []Interface{{Name: "eno1", Type: "physical"}, {Name: "eno2", Type: "physical"}}}, SourceStorageConfig: []byte("lvmthin: old\n vgname data\n thinpool guests\n content rootdir,images\n"), Target: Inventory{Hostname: "target", Interfaces: []Interface{{Name: "ens3", Type: "physical"}, {Name: "ens4", Type: "physical"}, {Name: "vmbr0", Type: "bridge"}}, Details: map[string]json.RawMessage{"command_results": json.RawMessage(`{"file_inventory":{"state":"ok"}}`), "file_hashes": json.RawMessage(`{}`)}}, Files: []RecoveryReadFile{{Path: "etc/pve/storage.cfg", State: "read", Content: cfg, SHA256: Hash(cfg)}}, StorageVolumes: map[string]RecoveryVolumeResult{"new": {State: "ok", Volumes: []string{"new:vm-100-disk-0"}}}}
}
func prepareForTest(t *testing.T, path, input string, snapshot RecoverySnapshot, mapping Mapping) PreparedRecoveryFile {
	t.Helper()
	p, err := prepareRecoveryFile(Entry{Path: path, Type: "file"}, []byte(input), snapshot, mapping, "host")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func hasPreparationTask(p PreparedRecoveryFile, text string) bool {
	for _, task := range p.ManualTasks {
		if strings.Contains(task.Instruction, text) || strings.Contains(task.Kind, text) {
			return true
		}
	}
	return false
}
func TestStorageIDRequiresMatchingBacking(t *testing.T) {
	s := preparationSnapshot()
	s.SourceStorageConfig = []byte("lvmthin: new\n vgname other\n thinpool guests\n content images,rootdir\n")
	p := prepareForTest(t, "etc/pve/qemu-server/100.conf", "scsi0: new:vm-100-disk-0,size=8G\n", s, Mapping{})
	if !hasPreparationTask(p, "backing") {
		t.Fatalf("same ID with different VG accepted: %+v", p)
	}
	i := RecoveryInspection{Storage: storageDefinitions("lvm: local\n vgname one\n"), TargetStorage: storageDefinitions("lvm: target\n vgname two\n")}
	_, blocked, _ := validateRecoveryMapping(i, PlanRequest{Mapping: Mapping{Storage: map[string]string{"local": "invented"}}})
	if len(blocked) == 0 {
		t.Fatal("unverified TargetID accepted")
	}
}
func TestGuestMappingPreservesVolumeAndOptions(t *testing.T) {
	input := "# old: keep\nscsi0: old:vm-100-disk-0,size=8G\ndescription: old: untouched\n"
	p := prepareForTest(t, "etc/pve/qemu-server/100.conf", input, preparationSnapshot(), Mapping{Storage: map[string]string{"old": "new"}})
	want := "# old: keep\nscsi0: new:vm-100-disk-0,size=8G\ndescription: old: untouched\n"
	if string(p.Content) != want || len(p.Effects) != 1 || p.Effects[0].Field != "scsi0" || p.Effects[0].Before != "old:vm-100-disk-0" || p.Effects[0].After != "new:vm-100-disk-0" {
		t.Fatalf("unsafe guest preparation: %+v %s", p, p.Content)
	}
}
func TestGuestCollisionAndMissingVolumeRemainManual(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*RecoverySnapshot)
		task  string
	}{
		{"collision", func(s *RecoverySnapshot) {
			s.Target.Details["file_hashes"] = json.RawMessage(`{"etc/pve/nodes/another/lxc/100.conf":"abc"}`)
		}, "guest-collision"},
		{"missing", func(s *RecoverySnapshot) {
			s.StorageVolumes["new"] = RecoveryVolumeResult{State: "ok", Volumes: []string{}}
		}, "volume"},
		{"unknown", func(s *RecoverySnapshot) { s.StorageVolumes = nil }, "volume"},
		{"backing unknown", func(s *RecoverySnapshot) { s.SourceStorageConfig = nil }, "backing"},
		{"content unknown", func(s *RecoverySnapshot) {
			s.Files[0].Content = []byte("lvmthin: new\n vgname data\n thinpool guests\n")
			s.Files[0].SHA256 = Hash(s.Files[0].Content)
		}, "storage-content"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := preparationSnapshot()
			tc.alter(&s)
			p := prepareForTest(t, "etc/pve/qemu-server/100.conf", "scsi0: old:vm-100-disk-0,size=8G\n", s, Mapping{Storage: map[string]string{"old": "new"}})
			if !hasPreparationTask(p, tc.task) {
				t.Fatalf("missing task %s: %+v", tc.task, p)
			}
		})
	}
}
func TestNetworkPreparationChangesOnlySelectedStanza(t *testing.T) {
	input := "auto vmbr0 vmbr1\niface vmbr0 inet static\n address 192.0.2.1/24\n gateway 192.0.2.254\n dns-nameservers 192.0.2.53\niface vmbr1 inet static\n address 192.0.2.1/24\n # 192.0.2.1 is duplicated intentionally\n"
	p := prepareForTest(t, "etc/network/interfaces", input, preparationSnapshot(), Mapping{Network: map[string]RecoveryNetworkMapping{"vmbr0": {AddressCIDR: "198.51.100.1/24", Gateway: "198.51.100.254", DNSServers: []string{"2001:db8::53"}}}})
	want := "auto vmbr0 vmbr1\niface vmbr0 inet static\n address 198.51.100.1/24\n gateway 198.51.100.254\n dns-nameservers 2001:db8::53\niface vmbr1 inet static\n address 192.0.2.1/24\n # 192.0.2.1 is duplicated intentionally\n"
	if string(p.Content) != want || len(p.Effects) != 3 {
		t.Fatalf("selected stanza not isolated: %s %+v", p.Content, p.Effects)
	}
}
func TestHostnamePreparationAvoidsGlobalReplacement(t *testing.T) {
	s := preparationSnapshot()
	input := "127.0.0.1 localhost\n192.0.2.1 old.example.org old # old comment\n192.0.2.2 old-storage old.example.net\n"
	p := prepareForTest(t, "etc/hosts", input, s, Mapping{Hostname: "new"})
	if string(p.Content) != "127.0.0.1 localhost\n192.0.2.1 new.example.org new # old comment\n192.0.2.2 old-storage old.example.net\n" || len(p.Effects) != 2 {
		t.Fatalf("hostname replacement too broad or absent: %s %+v", p.Content, p)
	}
	p = prepareForTest(t, "etc/hostname", "old\n", s, Mapping{Hostname: "new"})
	if string(p.Content) != "new\n" {
		t.Fatal("hostname not prepared")
	}
}
func TestRootBootAndSwapMountsPreserved(t *testing.T) {
	input := "UUID=root / ext4 defaults 0 1\nUUID=boot /boot ext4 defaults 0 2\nUUID=swap none swap sw 0 0\n/dev/sdb /srv/data ext4 defaults 0 2\n"
	p := prepareForTest(t, "etc/fstab", input, preparationSnapshot(), Mapping{Storage: map[string]string{"old": "new"}})
	if string(p.Content) != input || len(p.Effects) != 0 || !hasPreparationTask(p, "mount") {
		t.Fatalf("unsafe mount changes: %+v", p)
	}
}

func TestGuestPreparationSupportedFieldsAndDependencies(t *testing.T) {
	for _, tc := range []struct{ name, path, input, volume, want, task string }{
		{"unused", "qemu-server", "unused7: old:vm-100-disk-0\n", "vm-100-disk-0", "unused7: new:vm-100-disk-0\n", ""},
		{"iso", "qemu-server", "ide2: old:iso/install.iso,media=cdrom\n", "iso/install.iso", "ide2: new:iso/install.iso,media=cdrom\n", ""},
		{"cdrom", "qemu-server", "ide2: none,media=cdrom\n", "", "ide2: none,media=cdrom\n", ""},
		{"rootfs", "lxc", "rootfs: old:subvol-100-disk-0,size=8G\n", "subvol-100-disk-0", "rootfs: new:subvol-100-disk-0,size=8G\n", ""},
		{"mp", "lxc", "mp0: old:subvol-100-disk-0,mp=/data,backup=1\n", "subvol-100-disk-0", "mp0: new:subvol-100-disk-0,mp=/data,backup=1\n", ""},
		{"bind mount", "lxc", "mp1: /srv/data,mp=/data\n", "", "mp1: /srv/data,mp=/data\n", "volume"},
		{"hookscript", "qemu-server", "hookscript: old:snippets/hook.pl\n", "snippets/hook.pl", "hookscript: new:snippets/hook.pl\n", "hook"},
		{"pci", "qemu-server", "hostpci0: 0000:01:00.0\n", "", "hostpci0: 0000:01:00.0\n", "pci"},
		{"unknown", "qemu-server", "custom-volume: old:vm-100-disk-0\n", "vm-100-disk-0", "custom-volume: old:vm-100-disk-0\n", "guest-syntax"},
		{"bridge", "qemu-server", "net0: virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr9\n", "", "net0: virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr9\n", "bridge"},
		{"guest-volume mismatch", "qemu-server", "scsi0: old:vm-200-disk-0\n", "vm-200-disk-0", "scsi0: old:vm-200-disk-0\n", "guest-volume"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := preparationSnapshot()
			s.SourceStorageConfig = []byte("dir: old\n path /srv/data\n content images,rootdir,iso,snippets\n")
			cfg := []byte("dir: new\n path /srv/data\n content images,rootdir,iso,snippets\n")
			s.Files[0].Content = cfg
			s.Files[0].SHA256 = Hash(cfg)
			s.StorageVolumes["new"] = RecoveryVolumeResult{State: "ok", Volumes: []string{"new:" + tc.volume}}
			p := prepareForTest(t, "etc/pve/"+tc.path+"/100.conf", tc.input, s, Mapping{Storage: map[string]string{"old": "new"}})
			if string(p.Content) != tc.want || (tc.task != "" && !hasPreparationTask(p, tc.task)) {
				t.Fatalf("result %s tasks %+v", p.Content, p.ManualTasks)
			}
		})
	}
}
func TestStorageBackingKindsAndSyntax(t *testing.T) {
	for _, tc := range []struct {
		name, a, b string
		equal      bool
	}{
		{"NFS same", "nfs: a\n server nas\n export /data\n content images\n", "nfs: b\n server nas\n export /data\n content images,iso\n", true},
		{"NFS other export", "nfs: a\n server nas\n export /one\n", "nfs: a\n server nas\n export /two\n", false},
		{"PBS namespace", "pbs: a\n server nas\n datastore backups\n namespace one\n password private\n", "pbs: a\n server nas\n datastore backups\n namespace two\n", false},
		{"PBS same", "pbs: a\n server nas\n datastore backups\n namespace one\n", "pbs: b\n server nas\n datastore backups\n namespace one\n", true},
		{"unknown syntax", "dir: a\n path /data\n mystery yes\n", "dir: b\n path /data\n mystery yes\n", false},
		{"duplicate ID", "dir: a\n path /data\ndir: a\n path /data\n", "dir: b\n path /data\n", false},
		{"duplicate backend field", "dir: a\n path /data\n path /other\n", "dir: b\n path /data\n", false},
		{"missing backing", "lvmthin: a\n vgname data\n", "lvmthin: b\n vgname data\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := storageDefinitions(tc.a), storageDefinitions(tc.b)
			if matchingStorageBacking(a[0], b[0]) != tc.equal {
				t.Fatalf("unexpected identity result: %+v %+v", a, b)
			}
			raw, _ := json.Marshal(a)
			if strings.Contains(string(raw), "private") {
				t.Fatal("credential leaked")
			}
		})
	}
}
func TestNetworkPreparationVariantsAndRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		mapping           Mapping
		task              string
	}{
		{"bond primary", "iface bond0 inet manual\n bond-slaves eno1 eno2\n bond-primary eno2\n", "iface bond0 inet manual\n bond-slaves ens3 ens4\n bond-primary ens4\n", Mapping{Interfaces: map[string]string{"eno1": "ens3", "eno2": "ens4"}}, ""},
		{"vlan alias", "auto eno1.100:2\niface eno1.100:2 inet static\n address 192.0.2.2/24\n", "auto ens3.100:2\niface ens3.100:2 inet static\n address 192.0.2.2/24\n", Mapping{Interfaces: map[string]string{"eno1": "ens3"}}, ""},
		{"IPv6", "iface vmbr0 inet6 static\n address 2001:db8::1/64\n gateway 2001:db8::ff\n", "iface vmbr0 inet6 static\n address 2001:db8:1::1/64\n gateway 2001:db8:1::ff\n", Mapping{Network: map[string]RecoveryNetworkMapping{"vmbr0": {AddressCIDR: "2001:db8:1::1/64", Gateway: "2001:db8:1::ff"}}}, ""},
		{"invalid family", "iface vmbr0 inet static\n address 192.0.2.1/24\n", "iface vmbr0 inet static\n address 192.0.2.1/24\n", Mapping{Network: map[string]RecoveryNetworkMapping{"vmbr0": {AddressCIDR: "192.0.2.2/24", Gateway: "2001:db8::1"}}}, "network-value"},
		{"invalid cidr", "iface vmbr0 inet static\n address 192.0.2.1/24\n", "iface vmbr0 inet static\n address 192.0.2.1/24\n", Mapping{Network: map[string]RecoveryNetworkMapping{"vmbr0": {AddressCIDR: "192.0.2.2/99"}}}, "network-value"},
		{"port collision", "iface bond0 inet manual\n bond-slaves eno1 eno2\n", "iface bond0 inet manual\n bond-slaves eno1 eno2\n", Mapping{Interfaces: map[string]string{"eno1": "ens3", "eno2": "ens3"}}, "network"},
		{"hooks and includes", "source /etc/network/interfaces.d/*\niface vmbr0 inet manual\n post-up logger eno1\n mystery foo\n", "source /etc/network/interfaces.d/*\niface vmbr0 inet manual\n post-up logger eno1\n mystery foo\n", Mapping{}, "network"},
		{"duplicate stanza", "iface vmbr0 inet static\n address 192.0.2.1/24\niface vmbr0 inet static\n address 192.0.2.2/24\n", "iface vmbr0 inet static\n address 192.0.2.1/24\niface vmbr0 inet static\n address 192.0.2.2/24\n", Mapping{Network: map[string]RecoveryNetworkMapping{"vmbr0": {AddressCIDR: "192.0.2.9/24"}}}, "network-ambiguous"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := prepareForTest(t, "etc/network/interfaces", tc.input, preparationSnapshot(), tc.mapping)
			if string(p.Content) != tc.want || (tc.task != "" && !hasPreparationTask(p, tc.task)) {
				t.Fatalf("result %s tasks %+v", p.Content, p.ManualTasks)
			}
		})
	}
}
func TestPreparationMissingSourceAndManualChoices(t *testing.T) {
	s := preparationSnapshot()
	s.Source = Inventory{}
	p := prepareForTest(t, "etc/hosts", "192.0.2.1 old\n", s, Mapping{Hostname: "new"})
	if string(p.Content) != "192.0.2.1 old\n" || !hasPreparationTask(p, "hostname") {
		t.Fatal("guessed source identity")
	}
	p = prepareForTest(t, "etc/pve/qemu-server/100.conf", "scsi0: old:vm-100-disk-0\n", s, Mapping{Storage: map[string]string{"old": "manual"}})
	if string(p.Content) != "scsi0: old:vm-100-disk-0\n" || !hasPreparationTask(p, "storage") {
		t.Fatal("manual choice ignored")
	}
}

func TestPreparationKeepsAmbiguousGuestSectionsAndNetworkFamiliesManual(t *testing.T) {
	input := "[snapshot-one]\nscsi0: old:vm-100-disk-0\n"
	p := prepareForTest(t, "etc/pve/qemu-server/100.conf", input, preparationSnapshot(), Mapping{Storage: map[string]string{"old": "new"}})
	if string(p.Content) != input || !hasPreparationTask(p, "guest-syntax") {
		t.Fatal("unsupported guest section rewritten")
	}
	input = "iface vmbr0 inet static\n dns-nameservers 192.0.2.53\niface vmbr0 inet6 static\n dns-nameservers 2001:db8::53\n"
	p = prepareForTest(t, "etc/network/interfaces", input, preparationSnapshot(), Mapping{Network: map[string]RecoveryNetworkMapping{"vmbr0": {DNSServers: []string{"198.51.100.53"}}}})
	if string(p.Content) != input || !hasPreparationTask(p, "network-ambiguous") {
		t.Fatal("ambiguous dual-stack stanza partially rewritten")
	}
}
func TestPreparationNodeQualifiedGuestDestinationRemainsExplicit(t *testing.T) {
	p := prepareForTest(t, "etc/pve/nodes/old/qemu-server/100.conf", "name: guest\n", preparationSnapshot(), Mapping{})
	if p.Destination != "etc/pve/nodes/old/qemu-server/100.conf" || !hasPreparationTask(p, "guest-node") {
		t.Fatal("node relocation silently inferred or ignored")
	}
}
