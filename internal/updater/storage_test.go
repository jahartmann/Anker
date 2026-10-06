package updater

import (
	"anker/internal/storage"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func growthFixture(t *testing.T) (*StorageManager, *int64) {
	t.Helper()
	dir := t.TempDir()
	size := int64(100 << 20)
	m := &StorageManager{ConfigDir: dir, StateDir: dir, ownerUID: uint32(os.Getuid())}
	m.inspect = func() (storage.Report, error) {
		return storage.Report{Environment: "vm:kvm", DataPath: "/srv/anker", Volumes: []storage.Volume{{ID: "data", IsData: true, Mount: "/srv/anker", Source: "/dev/vdb1", FSType: "ext4", UUID: "fixture", Total: size - 10<<20, Available: 50 << 20}}, Devices: []storage.Device{{Name: "/dev/vdb1", Type: "part", UUID: "fixture", Size: 200 << 20}}}, nil
	}
	m.verifyDevice = func(string, string, int64) error { return nil }
	m.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "dumpe2fs":
			return []byte("Block count: " + strconv.FormatInt(size/4096, 10) + "\nBlock size: 4096\n"), nil
		case "resize2fs":
			if len(args) != 1 || args[0] != "/dev/vdb1" {
				t.Fatal("unexpected mutation", args)
			}
			size = 200 << 20
			return []byte("filesystem grown"), nil
		}
		return nil, errors.New("unexpected command " + name)
	}
	return m, &size
}

func TestStorageGrowthRejectsForgedPlansAndRequiresExactMountConfirmation(t *testing.T) {
	m, size := growthFixture(t)
	p, err := m.Plan("data")
	if err != nil || !p.CanGrow {
		t.Fatal(p, err)
	}
	for _, c := range []struct{ id, confirmation string }{{"forged", "/srv/anker"}, {p.ID, "yes"}} {
		if _, _, err := m.Begin("data", c.id, c.confirmation); err == nil {
			t.Fatal("unreviewed device mutation accepted", c)
		}
	}
	if *size != 100<<20 {
		t.Fatal("invalid request changed filesystem")
	}
	*size = 150 << 20
	if _, _, err := m.Begin("data", p.ID, "/srv/anker"); err == nil {
		t.Fatal("stale geometry plan accepted")
	}
	if _, err = m.Plan("/dev/sda"); err == nil {
		t.Fatal("caller supplied arbitrary device")
	}
}

func TestStorageGrowthPersistsResultAndReleasesSetupLock(t *testing.T) {
	m, _ := growthFixture(t)
	p, _ := m.Plan("data")
	state, complete, err := m.Begin("data", p.ID, "/srv/anker")
	if err != nil || state.Status != "running" {
		t.Fatal(state, err)
	}
	if _, _, err = m.Begin("data", p.ID, "/srv/anker"); err == nil {
		t.Fatal("concurrent grow accepted")
	}
	recovered := &StorageManager{ConfigDir: m.ConfigDir, StateDir: m.StateDir, ownerUID: uint32(os.Getuid())}
	if err = recovered.Load(); err != nil || recovered.Status().Status != "interrupted" {
		t.Fatal("missing interrupted journal", recovered.Status(), err)
	}
	complete()
	if m.Status().Status != "successful" || m.Status().AfterBytes != 200<<20 {
		t.Fatal("growth verification not persisted", m.Status())
	}
	if err = recovered.Load(); err != nil || recovered.Status().Status != "successful" {
		t.Fatal("success lost", recovered.Status(), err)
	}
	p, err = m.Plan("data")
	if err != nil || p.CanGrow {
		t.Fatal("already allocated full filesystem still offered", p, err)
	}
	unlock, err := (&TLSManager{Dir: m.ConfigDir, ownerUID: uint32(os.Getuid())}).lock()
	if err != nil {
		t.Fatal("setup lock stuck", err)
	}
	unlock()
}

func TestStorageGrowthRefusesContainerReadonlyMissingDeviceAndCommandFailure(t *testing.T) {
	for _, kind := range []string{"container", "readonly", "device", "unknown", "no-space", "other-volume"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := growthFixture(t)
			inspect := m.inspect
			m.inspect = func() (storage.Report, error) {
				r, e := inspect()
				switch kind {
				case "container":
					r.Environment = "container:lxc"
				case "unknown":
					r.Environment = "unknown"
				case "readonly":
					r.Volumes[0].ReadOnly = true
				case "device":
					r.Devices = nil
				case "no-space":
					r.Devices[0].Size = 100 << 20
				case "other-volume":
					r.Volumes[0].IsData = false
				}
				return r, e
			}
			p, err := m.Plan("data")
			if err == nil && p.CanGrow {
				t.Fatal("unsafe operation offered", kind, p)
			}
		})
	}
	m, _ := growthFixture(t)
	p, _ := m.Plan("data")
	run := m.run
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "resize2fs" {
			return []byte("kernel refused"), errors.New("failed")
		}
		return run(ctx, name, args...)
	}
	_, complete, err := m.Begin("data", p.ID, "/srv/anker")
	if err != nil {
		t.Fatal(err)
	}
	complete()
	if m.Status().Status != "failed" || !strings.Contains(m.Status().Message, "kernel refused") {
		t.Fatal("command failure hidden", m.Status())
	}
	if _, err = os.Stat(filepath.Join(m.StateDir, "storage-operation.json")); err != nil {
		t.Fatal("missing durable result", err)
	}
}

func TestStorageGrowthRechecksDeviceImmediatelyBeforeMutation(t *testing.T) {
	m, size := growthFixture(t)
	p, _ := m.Plan("data")
	_, complete, err := m.Begin("data", p.ID, "/srv/anker")
	if err != nil {
		t.Fatal(err)
	}
	inspect := m.inspect
	m.inspect = func() (storage.Report, error) { r, e := inspect(); r.Volumes[0].UUID = "replaced-device"; return r, e }
	complete()
	if *size != 100<<20 || m.Status().Status != "failed" {
		t.Fatal("device changed after confirmation but was still modified", *size, m.Status())
	}
}

func TestStorageJournalFailureDoesNotPretendAnOperationIsRunning(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission denial requires non-root")
	}
	m, _ := growthFixture(t)
	m.StateDir = filepath.Join(m.ConfigDir, "state")
	os.Mkdir(m.StateDir, 0500)
	defer os.Chmod(m.StateDir, 0700)
	p, _ := m.Plan("data")
	if _, _, err := m.Begin("data", p.ID, "/srv/anker"); err == nil {
		t.Fatal("mutation began without durable journal")
	}
	if m.Status().Status == "running" {
		t.Fatal("nonexistent operation shown as running")
	}
}

func TestStorageGrowthRequiresStableFilesystemIdentity(t *testing.T) {
	m, size := growthFixture(t)
	inspect := m.inspect
	m.inspect = func() (storage.Report, error) {
		r, e := inspect()
		r.Volumes[0].UUID = ""
		r.Devices[0].UUID = ""
		return r, e
	}
	p, err := m.Plan("data")
	if err != nil || p.CanGrow {
		t.Fatal("missing identity offered automatic mutation", p, err)
	}
	if *size != 100<<20 {
		t.Fatal("planning changed filesystem")
	}
}

func TestStorageXFSBindAliasKeepsTheBackupTargetThroughoutGrowth(t *testing.T) {
	m, size := growthFixture(t)
	inspect := m.inspect
	m.inspect = func() (storage.Report, error) {
		r, e := inspect()
		r.Volumes[0].Mount = "/srv/anker/hosts"
		r.Volumes[0].Paths = []string{"/mnt/backup", "/srv/anker/hosts"}
		r.Volumes[0].FSType = "xfs"
		return r, e
	}
	m.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "xfs_info":
			if len(args) != 1 || args[0] != "/srv/anker/hosts" {
				t.Fatal("geometry addressed another filesystem", args)
			}
			return []byte("data     =                       bsize=4096   blocks=" + strconv.FormatInt(*size/4096, 10) + ", imaxpct=25\n"), nil
		case "xfs_growfs":
			if len(args) != 2 || args[0] != "-d" || args[1] != "/srv/anker/hosts" {
				t.Fatal("growth addressed another filesystem", args)
			}
			*size = 200 << 20
			return nil, nil
		}
		return nil, errors.New("unexpected command " + name)
	}
	p, err := m.Plan("data")
	if err != nil || !p.CanGrow || p.Mount != "/srv/anker/hosts" {
		t.Fatal("wrong backup target", p, err)
	}
	_, complete, err := m.Begin("data", p.ID, "/srv/anker/hosts")
	if err != nil {
		t.Fatal(err)
	}
	complete()
	if m.Status().Status != "successful" {
		t.Fatal(m.Status())
	}
}
