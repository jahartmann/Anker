package updater

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Subprocesses leave the same on-disk states a process exit can leave, without systemd or root.
func TestUpdaterHelperProcess(t *testing.T) {
	action := os.Getenv("ANKER_TEST_HELPER_ACTION")
	if action == "" {
		return
	}
	root := os.Getenv("ANKER_TEST_HELPER_ROOT")
	if action == "wal" {
		db, err := sql.Open("sqlite", filepath.Join(root, "catalog.db"))
		if err != nil {
			os.Exit(2)
		}
		if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; CREATE TABLE example(value TEXT); INSERT INTO example VALUES('before');`); err != nil {
			os.Exit(3)
		}
		os.Exit(0) // Deliberately no Close: committed changes remain in WAL.
	}
	i := &Installer{Binary: filepath.Join(root, "bin", "anker"), Data: filepath.Join(root, "data"), StateDir: filepath.Join(root, "state"), Control: &fakeControl{}}
	if err := i.Recover(context.Background()); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}
func TestKnownUpdaterStartsWhenCandidateCannotExecute(t *testing.T) {
	i, _, _, _ := fixture(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(filepath.Dir(i.Binary), "anker-updater")
	if err = copyFile(self, helper, 0755, -1, -1); err != nil {
		t.Fatal(err)
	}
	copyFile(i.Binary, filepath.Join(i.StateDir, "previous"), 0755, -1, -1)
	copyFile(filepath.Join(i.Data, "catalog.db"), filepath.Join(i.StateDir, "catalog.db"), 0600, -1, -1)
	if err = i.writeJournal(journal{Version: "v0.2.0", Previous: "0.1.0", Snapshot: true, UID: os.Getuid(), GID: os.Getgid()}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(i.Binary, []byte("this is a broken candidate executable"), 0755)
	if err = exec.Command(i.Binary, "version").Run(); err == nil {
		t.Fatal("candidate unexpectedly executes")
	}
	cmd := exec.Command(helper, "-test.run=^TestUpdaterHelperProcess$")
	cmd.Env = append(os.Environ(), "ANKER_TEST_HELPER_ACTION=recover", "ANKER_TEST_HELPER_ROOT="+filepath.Dir(i.StateDir))
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	b, _ := os.ReadFile(i.Binary)
	if string(b) != "old binary" {
		t.Fatal("old helper did not recover candidate")
	}
	unit, err := os.ReadFile("../../deploy/anker-updater.service")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), "ExecStart="+HelperPath+" updater-serve") {
		t.Fatal("systemd would start the candidate instead of the recovery helper")
	}
}
func TestRecoveryRestoresCommittedSQLiteWAL(t *testing.T) {
	i, _, _, _ := fixture(t)
	os.Remove(filepath.Join(i.Data, "catalog.db"))
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run=^TestUpdaterHelperProcess$")
	cmd.Env = append(os.Environ(), "ANKER_TEST_HELPER_ACTION=wal", "ANKER_TEST_HELPER_ROOT="+i.Data)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	for _, name := range []string{"catalog.db", "catalog.db-wal"} {
		if err = copyFile(filepath.Join(i.Data, name), filepath.Join(i.StateDir, name), 0600, -1, -1); err != nil {
			t.Fatal(err)
		}
	}
	copyFile(i.Binary, filepath.Join(i.StateDir, "previous"), 0755, -1, -1)
	db, err := sql.Open("sqlite", filepath.Join(i.Data, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE example SET value='after'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err = i.writeJournal(journal{Version: "v0.2.0", Previous: "0.1.0", Snapshot: true, WAL: true, UID: os.Getuid(), GID: os.Getgid()}); err != nil {
		t.Fatal(err)
	}
	if err = i.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", filepath.Join(i.Data, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err = db.QueryRow(`SELECT value FROM example`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "before" {
		t.Fatal("committed catalog state in WAL lost", value)
	}
}
