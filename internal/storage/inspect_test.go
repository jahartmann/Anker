package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDeviceMapperAliasResolvesToTheSameBlockDevice(t *testing.T) {
	dir := t.TempDir()
	device, alias := filepath.Join(dir, "dm-0"), filepath.Join(dir, "mapper-volume")
	if err := os.WriteFile(device, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(device, alias); err != nil {
		t.Fatal(err)
	}
	d, ok := FindDevice([]Device{{Name: alias, UUID: "same-filesystem"}}, device)
	if !ok || d.UUID != "same-filesystem" {
		t.Fatal("mapper alias lost filesystem identity", d, ok)
	}
}

func TestMountInventoryDeduplicatesAnkerAndSystemAndSkipsVirtualFiles(t *testing.T) {
	raw := `21 1 8:1 / / ro,relatime - ext4 /dev/vda1 rw
22 21 8:1 /srv/anker /srv/anker rw,relatime - ext4 /dev/vda1 rw
23 21 0:5 / /proc rw - proc proc rw
24 21 8:2 / /mnt/backup\040disk rw - xfs /dev/vdb1 rw
25 21 0:6 / /run rw - tmpfs tmpfs rw
26 21 8:1 /etc/hosts /etc/hosts rw - ext4 /dev/vda1 rw
`
	stat := func(path string) (Stats, error) {
		if path == "/etc/hosts" {
			return Stats{}, fmt.Errorf("file bind must not be statted")
		}
		return Stats{Total: 1000, Free: 400, Available: 350, Inodes: 100, InodesFree: 10}, nil
	}
	volumes, warnings, err := VolumesFromMounts(raw, "/srv/anker", stat)
	if err != nil || len(warnings) != 0 || len(volumes) != 2 {
		t.Fatalf("inventory: %+v warnings=%v err=%v", volumes, warnings, err)
	}
	v := volumes[0]
	if !v.IsData || !v.IsSystem || v.Used != 600 || v.Reserved != 50 || v.Available != 350 || v.InodesUsed != 90 || v.ReadOnly || len(v.Paths) != 2 {
		t.Fatalf("wrong capacity / shared filesystem: %+v", v)
	}
	if volumes[1].Mount != "/mnt/backup disk" {
		t.Fatalf("escaped mount not decoded: %+v", volumes[1])
	}
}

func TestInventoryShowsFailedVolumesWithoutPretendingTheyAreEmpty(t *testing.T) {
	raw := `21 1 8:1 / / rw - ext4 /dev/vda1 rw
22 21 8:2 / /srv/anker rw - ext4 /dev/vdb1 rw
`
	volumes, _, err := VolumesFromMounts(raw, "/srv/anker", func(path string) (Stats, error) {
		if path == "/srv/anker" {
			return Stats{}, fmt.Errorf("permission denied")
		}
		return Stats{Total: 1000, Free: 400, Available: 350}, nil
	})
	if err != nil || len(volumes) != 2 || volumes[1].Error == "" || volumes[1].UsedPercent != 0 {
		t.Fatalf("missing failed volume: %+v %v", volumes, err)
	}
	if _, _, err = VolumesFromMounts("invalid mount info", "/srv/anker", nil); err == nil {
		t.Fatal("malformed mount list accepted")
	}
}

func TestBackupSubmountKeepsItsOwnCapacity(t *testing.T) {
	raw := `21 1 8:1 / / rw - ext4 /dev/vda1 rw
22 21 8:2 / /srv/anker/hosts rw - ext4 /dev/vdb1 rw
`
	volumes, _, err := VolumesFromMounts(raw, "/srv/anker", func(path string) (Stats, error) {
		if path == "/srv/anker/hosts" {
			return Stats{Total: 2000, Free: 1000, Available: 900}, nil
		}
		return Stats{Total: 1000, Free: 400, Available: 350}, nil
	})
	if err != nil || len(volumes) != 2 || !volumes[1].IsData || volumes[1].Total != 2000 {
		t.Fatal("backup mount inherited root capacity", volumes, err)
	}
}

func TestBackupBindAliasUsesItsOwnMountForMeasurement(t *testing.T) {
	raw := `21 1 8:1 / / rw - ext4 /dev/vda1 rw
22 21 8:2 / /mnt/backup rw - xfs /dev/vdb1 rw
23 21 8:2 /hosts /srv/anker/hosts rw - xfs /dev/vdb1 rw
`
	volumes, _, err := VolumesFromMounts(raw, "/srv/anker", func(path string) (Stats, error) {
		if path == "/srv/anker/hosts" {
			return Stats{Total: 5000, Free: 3000, Available: 3000}, nil
		}
		return Stats{Total: 1000, Free: 400, Available: 350}, nil
	})
	if err != nil || len(volumes) != 2 || volumes[1].Mount != "/srv/anker/hosts" || volumes[1].Total != 5000 {
		t.Fatal("bind alias measured the wrong filesystem", volumes, err)
	}
}

func TestInventoryDoesNotReportAnOvermountedFilesystem(t *testing.T) {
	raw := `21 1 8:1 / / rw - ext4 /dev/vda1 rw
22 21 8:2 / / rw - ext4 /dev/vdb1 rw
`
	volumes, _, err := VolumesFromMounts(raw, "/srv/anker", func(string) (Stats, error) { return Stats{Total: 1000, Free: 500, Available: 450}, nil })
	if err != nil || len(volumes) != 1 || volumes[0].Source != "/dev/vdb1" {
		t.Fatal("hidden filesystem advertised", volumes, err)
	}
}
