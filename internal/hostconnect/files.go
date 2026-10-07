package hostconnect

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/sys/unix"
)

const maxTrust = 1 << 20

func (m Manager) directory() string {
	if m.Dir == "" {
		return "/etc/anker"
	}
	return filepath.Clean(m.Dir)
}

// Open each component without following links, then retain the directory descriptor
// for all subsequent operations. Requests never supply filesystem paths.
func (m Manager) openDir() (*os.File, error) {
	path := m.directory()
	if !filepath.IsAbs(path) {
		return nil, errors.New("SSH-Konfiguration benötigt einen absoluten Pfad")
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	f := os.NewFile(uintptr(fd), path)
	if e = m.check(f, true, false); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func (m Manager) check(f *os.File, dir, private bool) error {
	var st unix.Stat_t
	if e := unix.Fstat(int(f.Fd()), &st); e != nil {
		return e
	}
	return m.checkMetadata(uint32(st.Mode), int(st.Uid), int(st.Gid), dir, private)
}

func (m Manager) checkMetadata(mode uint32, uid, gid int, dir, private bool) error {
	kind := uint32(unix.S_IFREG)
	if dir {
		kind = unix.S_IFDIR
	}
	ownerAllowed := uid == m.OwnerUID || (private && !dir && m.KeyOwnerUID > 0 && uid == m.KeyOwnerUID)
	if mode&unix.S_IFMT != kind || !ownerAllowed || gid != m.GroupGID || mode&0022 != 0 || (private && mode&0007 != 0) {
		return errors.New("SSH-Datei hat unsichere Eigentümer oder Rechte")
	}
	return nil
}
func (m Manager) readAt(dir *os.File, name string, limit int64, private bool) ([]byte, error) {
	fd, e := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	if e = m.check(f, false, private); e != nil {
		return nil, e
	}
	st, e := f.Stat()
	if e != nil || st.Size() > limit {
		return nil, errors.New("SSH-Datei ist zu groß")
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, errors.New("SSH-Datei kann nicht gelesen werden")
	}
	return b, nil
}
func (m Manager) keys(dir *os.File) (ssh.Signer, ssh.Signer, error) {
	fd, e := unix.Openat(int(dir.Fd()), "keys", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, nil, e
	}
	d := os.NewFile(uintptr(fd), "keys")
	defer d.Close()
	if e = m.check(d, true, false); e != nil {
		return nil, nil, e
	}
	read := func(name string) (ssh.Signer, error) {
		b, e := m.readAt(d, name, 16384, true)
		if e != nil {
			return nil, e
		}
		s, e := ssh.ParsePrivateKey(b)
		if e != nil || s.PublicKey().Type() != ssh.KeyAlgoED25519 {
			return nil, errors.New("Geschützter Ed25519-Rollenschlüssel fehlt oder ist ungültig")
		}
		return s, nil
	}
	backup, e := read("backup")
	if e != nil {
		return nil, nil, e
	}
	restore, e := read("restore")
	if e != nil {
		return nil, nil, e
	}
	if bytes.Equal(backup.PublicKey().Marshal(), restore.PublicKey().Marshal()) {
		return nil, nil, errors.New("Rollen benötigen verschiedene Schlüssel")
	}
	return backup, restore, nil
}
func (m Manager) readTrust(dir *os.File) ([]byte, error) {
	b, e := m.readAt(dir, "known_hosts", maxTrust, false)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	return b, e
}
func trustCallback(b []byte) (ssh.HostKeyCallback, error) {
	f, e := os.CreateTemp("", "anker-known-hosts-*")
	if e != nil {
		return nil, e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, e = f.Write(b); e != nil {
		return nil, e
	}
	return knownhosts.New(f.Name())
}
func identityState(b []byte, address string, key ssh.PublicKey) (bool, bool, error) {
	callback, e := trustCallback(b)
	if e != nil {
		return false, false, e
	}
	e = callback(address, &net.TCPAddr{}, key)
	if e == nil {
		return true, false, nil
	}
	var mismatch *knownhosts.KeyError
	if errors.As(e, &mismatch) {
		return len(mismatch.Want) > 0, len(mismatch.Want) > 0, nil
	}
	var revoked *knownhosts.RevokedError
	if errors.As(e, &revoked) {
		return true, true, nil
	}
	return false, false, e
}
func (m Manager) saveTrust(dir *os.File, address string, key ssh.PublicKey) error {
	fd, e := unix.Openat(int(dir.Fd()), "known_hosts.lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	lock := os.NewFile(uintptr(fd), "known_hosts.lock")
	defer lock.Close()
	var st unix.Stat_t
	if e = unix.Fstat(fd, &st); e != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || int(st.Uid) != m.OwnerUID || st.Mode&0022 != 0 {
		return errors.New("Ungültige SSH-Sperrdatei")
	}
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		return errors.New("SSH-Vertrauen wird gerade bearbeitet")
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	old, e := m.readTrust(dir)
	if e != nil {
		return e
	}
	known, changed, e := identityState(old, address, key)
	if e != nil {
		return e
	}
	if changed {
		return errors.New("Bekannter SSH-Hostschlüssel wurde geändert")
	}
	if known {
		return nil
	}
	if len(old) > 0 && old[len(old)-1] != '\n' {
		old = append(old, '\n')
	}
	old = append(old, []byte(knownhosts.Line([]string{address}, key)+"\n")...)
	if len(old) > maxTrust {
		return errors.New("SSH-Vertrauensdatei ist zu groß")
	}
	// A random O_EXCL file in the retained protected directory is atomically promoted.
	random := make([]byte, 16)
	if _, e = io.ReadFull(rand.Reader, random); e != nil {
		return e
	}
	name := ".known-hosts-" + hex.EncodeToString(random)
	fd, e = unix.Openat(int(dir.Fd()), name, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0640)
	if e != nil {
		return e
	}
	out := os.NewFile(uintptr(fd), name)
	defer out.Close()
	defer unix.Unlinkat(int(dir.Fd()), name, 0)
	if e = unix.Fchown(fd, m.OwnerUID, m.GroupGID); e != nil {
		return e
	}
	if e = out.Chmod(0640); e != nil {
		return e
	}
	if _, e = out.Write(old); e != nil {
		return e
	}
	if e = out.Sync(); e != nil {
		return e
	}
	if e = unix.Renameat(int(dir.Fd()), name, int(dir.Fd()), "known_hosts"); e != nil {
		return e
	}
	return dir.Sync()
}
