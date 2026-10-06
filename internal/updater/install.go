package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type Control interface {
	Prepare(context.Context) (string, error)
	Release(context.Context) error
	Stop(context.Context) error
	Start(context.Context) error
	Health(context.Context, string) error
}
type Installer struct {
	Binary, Data, StateDir string
	Control                Control
	spaceCheck             func(string, int64) error
}
type journal struct {
	Version  string
	Previous string
	Snapshot bool
	WAL      bool
	UID, GID int
}

func syncDir(p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func atomic(p string, b []byte, mode os.FileMode, uid, gid int) error {
	f, err := os.CreateTemp(filepath.Dir(p), ".anker-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = f.Chmod(mode)
	if err == nil && uid >= 0 {
		err = f.Chown(uid, gid)
	}
	if err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), p)
	}
	if err == nil {
		err = syncDir(filepath.Dir(p))
	}
	return err
}
func copyFile(from, to string, mode os.FileMode, uid, gid int) error {
	// Do not follow user-controlled catalog symlinks while running as root.
	fd, err := unix.Open(from, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), from)
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("Updatequelle ist keine reguläre Datei")
	}
	dest, err := os.CreateTemp(filepath.Dir(to), ".anker-copy-*")
	if err != nil {
		return err
	}
	defer os.Remove(dest.Name())
	err = dest.Chmod(mode)
	if err == nil && uid >= 0 {
		err = dest.Chown(uid, gid)
	}
	if err == nil {
		_, err = io.Copy(dest, f)
	}
	if err == nil {
		err = dest.Sync()
	}
	cerr := dest.Close()
	if err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(dest.Name(), to)
	}
	if err == nil {
		err = syncDir(filepath.Dir(to))
	}
	return err
}
func (i *Installer) writeJournal(j journal) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return atomic(filepath.Join(i.StateDir, "pending.json"), b, 0600, -1, -1)
}
func (i *Installer) clearJournal() error {
	if err := os.Remove(filepath.Join(i.StateDir, "pending.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDir(i.StateDir)
}
func (i *Installer) Install(ctx context.Context, r Release, content []byte) error {
	sum := sha256.Sum256(content)
	if int64(len(content)) != r.Artifact.Size || hex.EncodeToString(sum[:]) != r.Artifact.SHA256 {
		return errors.New("Prüfsumme oder Größe der Binärdatei stimmt nicht")
	}
	if _, err := os.Stat(filepath.Join(i.StateDir, "pending.json")); err == nil {
		return errors.New("Unterbrochenes Update muss zuerst wiederhergestellt werden")
	} else if !os.IsNotExist(err) {
		return err
	}
	// Keep room for staging, snapshots and a second atomic write during rollback.
	required := int64(len(content)) + (64 << 20)
	for _, name := range []string{i.Binary, filepath.Join(i.Data, "catalog.db"), filepath.Join(i.Data, "catalog.db-wal")} {
		st, err := os.Lstat(name)
		if os.IsNotExist(err) && strings.HasSuffix(name, "-wal") {
			continue
		}
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			return errors.New("Updatequelle ist keine reguläre Datei")
		}
		required += 2 * st.Size()
	}
	check := i.spaceCheck
	if check == nil {
		check = checkSpace
	}
	for _, dir := range []string{filepath.Dir(i.Binary), i.Data, i.StateDir} {
		if err := check(dir, required); err != nil {
			return err
		}
	}
	// Stage on the same filesystem as the destination before interrupting the service.
	staged := i.Binary + ".next"
	if err := atomic(staged, content, 0755, -1, -1); err != nil {
		return err
	}
	defer os.Remove(staged)
	if err := copyFile(i.Binary, filepath.Join(i.StateDir, "previous"), 0755, -1, -1); err != nil {
		return err
	}
	previous, err := i.Control.Prepare(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if _, err := os.Stat(filepath.Join(i.StateDir, "pending.json")); os.IsNotExist(err) {
			i.Control.Release(context.Background())
		}
	}()
	j := journal{Version: r.Version, Previous: previous, UID: -1, GID: -1}
	if err = i.writeJournal(j); err != nil {
		return err
	}
	if err = i.Control.Stop(ctx); err != nil { // Recover even if systemd reports a timeout after stopping the process.
		recovery := i.Recover(context.Background())
		return errors.Join(err, recovery)
	}
	fail := func(cause error) error {
		os.Remove(staged)
		if recovery := i.Recover(context.Background()); recovery != nil {
			return fmt.Errorf("Update fehlgeschlagen; automatische Wiederherstellung fehlgeschlagen: %v; %w", recovery, cause)
		}
		return fmt.Errorf("Update fehlgeschlagen; vorherige Version wiederhergestellt: %w", cause)
	}
	st, err := os.Lstat(filepath.Join(i.Data, "catalog.db"))
	if err != nil {
		return fail(err)
	}
	if !st.Mode().IsRegular() {
		return fail(errors.New("Katalog ist keine reguläre Datei"))
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		j.UID, j.GID = int(sys.Uid), int(sys.Gid)
	}
	if err = copyFile(filepath.Join(i.Data, "catalog.db"), filepath.Join(i.StateDir, "catalog.db"), 0600, -1, -1); err != nil {
		return fail(err)
	}
	wal := filepath.Join(i.Data, "catalog.db-wal")
	if _, err = os.Lstat(wal); err == nil {
		if err = copyFile(wal, filepath.Join(i.StateDir, "catalog.db-wal"), 0600, -1, -1); err != nil {
			return fail(err)
		}
		j.WAL = true
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	j.Snapshot = true
	if err = i.writeJournal(j); err != nil {
		return fail(err)
	}
	if err = os.Rename(staged, i.Binary); err != nil {
		return fail(err)
	}
	if err = syncDir(filepath.Dir(i.Binary)); err != nil {
		return fail(err)
	}
	if err = i.Control.Start(ctx); err != nil {
		return fail(err)
	}
	if err = i.Control.Health(ctx, r.Version); err != nil {
		return fail(err)
	}
	if err = i.clearJournal(); err != nil {
		return fail(err)
	}
	return nil
}
func (i *Installer) Recover(ctx context.Context) error {
	raw, err := os.ReadFile(filepath.Join(i.StateDir, "pending.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var j journal
	if err = json.Unmarshal(raw, &j); err != nil {
		return err
	}
	if err = i.Control.Stop(ctx); err != nil {
		return err
	}
	if err = copyFile(filepath.Join(i.StateDir, "previous"), i.Binary, 0755, -1, -1); err != nil {
		return err
	}
	if j.Snapshot {
		if err = copyFile(filepath.Join(i.StateDir, "catalog.db"), filepath.Join(i.Data, "catalog.db"), 0600, j.UID, j.GID); err != nil {
			return err
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if err = os.Remove(filepath.Join(i.Data, "catalog.db"+suffix)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if j.WAL {
			if err = copyFile(filepath.Join(i.StateDir, "catalog.db-wal"), filepath.Join(i.Data, "catalog.db-wal"), 0600, j.UID, j.GID); err != nil {
				return err
			}
		}
		if err = syncDir(i.Data); err != nil {
			return err
		}
	}
	if err = i.Control.Start(ctx); err != nil {
		return err
	}
	if err = i.Control.Health(ctx, j.Previous); err != nil {
		return err
	}
	return i.clearJournal()
}

func checkSpace(path string, required int64) error {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return err
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if required < 0 || available < uint64(required) {
		return errors.New("Nicht genug freier Speicher für Update und automatischen Rückfall")
	}
	return nil
}
