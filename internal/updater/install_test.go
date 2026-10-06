package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeControl struct {
	busy, fail    bool
	starts, stops int
	version       string
}

func (f *fakeControl) Prepare(context.Context) (string, error) {
	if f.busy {
		return "", errors.New("busy")
	}
	return "0.1.0", nil
}
func (f *fakeControl) Release(context.Context) error { return nil }
func (f *fakeControl) Stop(context.Context) error    { f.stops++; return nil }
func (f *fakeControl) Start(context.Context) error   { f.starts++; return nil }
func (f *fakeControl) Health(_ context.Context, want string) error {
	if f.fail && want == "v0.2.0" {
		return errors.New("failed startup")
	}
	return nil
}
func fixture(t *testing.T) (*Installer, *fakeControl, Release, []byte) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "anker")
	data := filepath.Join(root, "data")
	state := filepath.Join(root, "state")
	os.MkdirAll(filepath.Dir(bin), 0700)
	os.MkdirAll(data, 0700)
	os.MkdirAll(state, 0700)
	os.WriteFile(bin, []byte("old binary"), 0755)
	os.WriteFile(filepath.Join(data, "catalog.db"), []byte("old catalog"), 0600)
	content := []byte("new binary")
	sum := sha256.Sum256(content)
	r := Release{Version: "v0.2.0", Artifact: Artifact{Name: "anker-linux-amd64", Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}}
	ctl := &fakeControl{}
	return &Installer{Binary: bin, Data: data, StateDir: state, Control: ctl}, ctl, r, content
}
func TestInstallAndRollback(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "startup failure"}[failure], func(t *testing.T) {
			i, c, r, b := fixture(t)
			c.fail = failure
			err := i.Install(context.Background(), r, b)
			if failure != (err != nil) {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(i.Binary)
			want := "new binary"
			if failure {
				want = "old binary"
			}
			if string(got) != want {
				t.Fatal("binary", string(got))
			}
			if c.stops == 0 || c.starts == 0 {
				t.Fatal("service not controlled")
			}
		})
	}
}
func TestInstallRefusesCorruptDownloadAndBusyService(t *testing.T) {
	i, c, r, b := fixture(t)
	if err := i.Install(context.Background(), r, []byte("tampered")); err == nil {
		t.Fatal("corrupt binary accepted")
	}
	if c.stops != 0 {
		t.Fatal("stopped before verification")
	}
	c.busy = true
	if err := i.Install(context.Background(), r, b); err == nil {
		t.Fatal("busy service stopped")
	}
	if c.stops != 0 {
		t.Fatal("busy service stopped")
	}
}
func TestInterruptedUpdateRestoresCatalogAndBinary(t *testing.T) {
	i, c, _, _ := fixture(t)
	if err := copyFile(i.Binary, filepath.Join(i.StateDir, "previous"), 0755, -1, -1); err != nil {
		t.Fatal(err)
	}
	copyFile(filepath.Join(i.Data, "catalog.db"), filepath.Join(i.StateDir, "catalog.db"), 0600, -1, -1)
	j := journal{Version: "v0.2.0", Previous: "0.1.0", Snapshot: true, UID: os.Getuid(), GID: os.Getgid()}
	if err := i.writeJournal(j); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(i.Binary, []byte("broken new version"), 0755)
	os.WriteFile(filepath.Join(i.Data, "catalog.db"), []byte("new catalog"), 0600)
	if err := i.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(i.Binary)
	db, _ := os.ReadFile(filepath.Join(i.Data, "catalog.db"))
	if string(b) != "old binary" || string(db) != "old catalog" || c.starts == 0 {
		t.Fatal("interrupted update not recovered")
	}
}

func TestLowSpaceDoesNotInterruptService(t *testing.T) {
	i, c, r, b := fixture(t)
	i.spaceCheck = func(string, int64) error { return errors.New("insufficient disk space") }
	if err := i.Install(context.Background(), r, b); err == nil {
		t.Fatal("low space accepted")
	}
	if c.stops != 0 || c.starts != 0 {
		t.Fatal("service interrupted before disk-space check")
	}
}
