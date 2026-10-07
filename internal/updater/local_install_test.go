package updater

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalInstallationKeepsHelperAndMainTogether(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failure], func(t *testing.T) {
			i, c, r, b := fixture(t)
			c.fail = failure
			i.Helper = filepath.Join(filepath.Dir(i.Binary), "helper")
			os.WriteFile(i.Helper, []byte("old helper"), 0755)
			err := i.Install(context.Background(), r, b)
			if (err != nil) != failure {
				t.Fatal(err)
			}
			main, _ := os.ReadFile(i.Binary)
			helper, _ := os.ReadFile(i.Helper)
			if failure {
				if string(main) != "old binary" || string(helper) != "old helper" {
					t.Fatal("paired rollback failed")
				}
			} else if string(main) != "new binary" || string(helper) != "new binary" {
				t.Fatal("paired install failed")
			}
		})
	}
}

func TestLocalInstallationBeforeSetupDoesNotCreateCatalog(t *testing.T) {
	i, _, r, b := fixture(t)
	os.Remove(filepath.Join(i.Data, "catalog.db"))
	i.AllowMissingCatalog = true
	i.Offline = true
	if err := i.Install(context.Background(), r, b); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(i.Data, "catalog.db")); !os.IsNotExist(err) {
		t.Fatal("catalog created before setup", err)
	}
}

func TestLocalHelperReadinessFailureRollsBackWholePair(t *testing.T) {
	i, _, r, b := fixture(t)
	i.Helper = filepath.Join(filepath.Dir(i.Binary), "helper")
	os.WriteFile(i.Helper, []byte("old helper"), 0755)
	i.Coordinated = true
	i.AfterHealth = func(context.Context) error { return errors.New("helper did not start") }
	if err := i.Install(context.Background(), r, b); err == nil {
		t.Fatal("helper readiness failure accepted")
	}
	main, _ := os.ReadFile(i.Binary)
	helper, _ := os.ReadFile(i.Helper)
	if string(main) != "old binary" || string(helper) != "old helper" {
		t.Fatal("readiness failure did not roll back both programs")
	}
}

func TestActiveLocalCoordinatorDefersBootRecoveryUntilCrash(t *testing.T) {
	i, c, _, _ := fixture(t)
	i.Helper = filepath.Join(filepath.Dir(i.Binary), "helper")
	i.LocalLockPath = filepath.Join(t.TempDir(), "local.lock")
	lock, err := os.OpenFile(i.LocalLockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	copyFile(i.Binary, filepath.Join(i.StateDir, "previous"), 0755, -1, -1)
	os.WriteFile(filepath.Join(i.StateDir, "previous-helper"), []byte("old helper"), 0755)
	os.WriteFile(i.Helper, []byte("new helper"), 0755)
	os.WriteFile(i.Binary, []byte("new binary"), 0755)
	if err = i.writeJournal(journal{Version: "v0.2.0", Previous: "0.1.0", Helper: true, Coordinated: true}); err != nil {
		t.Fatal(err)
	}
	if err = i.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.stops != 0 {
		t.Fatal("live coordinator interrupted by helper startup")
	}
	if _, err = os.Stat(filepath.Join(i.StateDir, "pending.json")); err != nil {
		t.Fatal("active journal cleared")
	}
	lock.Close() // Simulate coordinator exit/crash.
	if err = i.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	main, _ := os.ReadFile(i.Binary)
	helper, _ := os.ReadFile(i.Helper)
	if string(main) != "old binary" || string(helper) != "old helper" || !i.recoveredHelper {
		t.Fatal("crash recovery did not restore and request execution of the old helper")
	}
}
