package anker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataModesCannotBeMixed(t *testing.T) {
	for _, demo := range []bool{false, true} {
		root := t.TempDir()
		if err := EnsureDataMode(root, demo); err != nil {
			t.Fatal(err)
		}
		if err := EnsureDataMode(root, demo); err != nil {
			t.Fatal("cannot restart same mode", err)
		}
		if err := EnsureDataMode(root, !demo); err == nil {
			t.Fatal("mode changed")
		}
		child := filepath.Join(root, "nested")
		if err := os.Mkdir(child, 0700); err != nil {
			t.Fatal(err)
		}
		if err := EnsureDataMode(child, !demo); err == nil {
			t.Fatal("mixed nested directory")
		}
	}
}

func TestProductionCannotEncloseExistingDemo(t *testing.T) {
	root := t.TempDir()
	demo := filepath.Join(root, "demo")
	if err := os.Mkdir(demo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDataMode(demo, true); err != nil {
		t.Fatal(err)
	}
	unlock, err := InstanceLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureDataMode(root, false); err == nil {
		unlock()
		t.Fatal("production claimed an existing demo's parent")
	}
	unlock()
	if err := EnsureDataMode(demo, true); err != nil {
		t.Fatal("existing demo cannot restart", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".anker-mode")); !os.IsNotExist(err) {
		t.Fatal("parent was marked despite overlap", err)
	}
}

func TestProductionCanUseFilesystemRootWithUnreadableLostFound(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires an unprivileged process to reproduce directory permissions")
	}
	root := t.TempDir()
	lostFound := filepath.Join(root, "lost+found")
	if err := os.Mkdir(lostFound, 0700); err != nil {
		t.Fatal(err)
	}
	recovered := filepath.Join(lostFound, "recovered-file")
	if err := os.WriteFile(recovered, []byte("recovered filesystem data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lostFound, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(lostFound, 0700) })
	if _, err := os.ReadDir(lostFound); !os.IsPermission(err) {
		t.Fatalf("fixture must be unreadable: %v", err)
	}
	if err := EnsureDataMode(root, false); err != nil {
		t.Fatal("filesystem recovery directory prevented production initialization", err)
	}
	mode, err := dataMode(root)
	if err != nil || mode != "production\n" {
		t.Fatal("production mode not persisted", mode, err)
	}
	info, err := os.Stat(lostFound)
	if err != nil || info.Mode().Perm() != 0000 {
		t.Fatal("filesystem recovery directory permissions changed", err)
	}
	if err = os.Chmod(lostFound, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(recovered)
	if err != nil || string(data) != "recovered filesystem data" {
		t.Fatal("recovered filesystem data changed", err)
	}
}

func TestProductionStillRejectsDemoInsideNestedLostFound(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "other", "lost+found")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".anker-mode"), []byte("demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDataMode(root, false); err == nil {
		t.Fatal("nested directory name bypassed demo isolation")
	}
}

func TestProductionStillRejectsUnreadableApplicationDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires an unprivileged process to reproduce directory permissions")
	}
	root := t.TempDir()
	private := filepath.Join(root, "unknown-data")
	if err := os.Mkdir(private, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(private, 0700) })
	if err := EnsureDataMode(root, false); !os.IsPermission(err) {
		t.Fatal("unreadable application data silently accepted", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".anker-mode")); !os.IsNotExist(err) {
		t.Fatal("failed scan marked the directory as production", err)
	}
}

func TestProductionIgnoresModeFilesInsideBackupAndPlanPayloads(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"hosts/h/backups/b/files/etc/.anker-mode", "plans/p/prepared-files/etc/.anker-mode"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("demo\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := EnsureDataMode(root, false); err != nil {
		t.Fatal("saved payload confused with active demo", err)
	}
}

func TestDemoCannotEnterUnmarkedProduction(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	demo := filepath.Join(root, "hosts/h/backups/b/files/demo")
	if err := os.MkdirAll(demo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDataMode(demo, true); err == nil {
		t.Fatal("demo entered legacy production data")
	}
}

func TestDemoRejectsExistingUnmarkedData(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "catalog.db"), []byte("existing data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDataMode(root, true); err == nil {
		t.Fatal("existing catalog adopted as demo")
	}
	data, err := os.ReadFile(filepath.Join(root, "catalog.db"))
	if err != nil || string(data) != "existing data" {
		t.Fatal("existing data changed", err)
	}
}

func TestInvalidDataModeFailsClosed(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		root := t.TempDir()
		marker := filepath.Join(root, ".anker-mode")
		if corrupt {
			if err := os.WriteFile(marker, []byte("unknown\n"), 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			target := filepath.Join(t.TempDir(), "mode")
			if err := os.WriteFile(target, []byte("demo\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, marker); err != nil {
				t.Fatal(err)
			}
		}
		for _, demo := range []bool{false, true} {
			if err := EnsureDataMode(root, demo); err == nil {
				t.Fatal("invalid marker accepted")
			}
		}
	}
}

func TestSeedDemoRequiresExplicitDemoMode(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := NewService(root, store, DemoCollector{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SeedDemo(NewAuth(store)); err == nil {
		t.Fatal("unmarked service seeded")
	}
	hosts, err := s.Hosts()
	if err != nil || len(hosts) != 0 {
		t.Fatal("demo hosts added", err)
	}
	users, err := records[User](store, "users")
	if err != nil || len(users) != 0 {
		t.Fatal("demo account added", err)
	}
}
