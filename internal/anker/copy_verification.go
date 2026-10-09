package anker

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Produced only by the local verifier. No caller-supplied assertions or paths are
// persisted. This proves bytes at a point in time, never hardware isolation.
type CopyVerificationReceipt struct {
	Version       int    `json:"version"`
	BackupID      string `json:"backup_id"`
	ManifestSHA   string `json:"manifest_sha"`
	State         string `json:"state"`
	AttemptAt     string `json:"attempt_at"`
	VerifiedAt    string `json:"verified_at,omitempty"`
	Provenance    string `json:"provenance"`
	FailureDomain string `json:"failure_domain"`
}

// VerifyIndependentCopy accepts the expanded backup directory containing
// manifest.json, files/ and artifacts. Source archives are not expanded or changed.
func (s *Service) VerifyIndependentCopy(id, copyRoot string) (CopyVerificationReceipt, error) {
	release, err := s.beginIntegrityOperation()
	if err != nil {
		return CopyVerificationReceipt{}, err
	}
	defer release()
	lock := s.backupLock(id)
	lock.Lock()
	defer lock.Unlock()
	b, err := s.Backup(id)
	if err != nil {
		return CopyVerificationReceipt{}, err
	}
	receipt := CopyVerificationReceipt{Version: 1, BackupID: b.ID, ManifestSHA: b.ManifestSHA, State: "failed", AttemptAt: now(), Provenance: "local-byte-verifier-v1", FailureDomain: "unknown"}
	err = s.checkIndependentCopy(b, copyRoot)
	if err == nil {
		receipt.State = "ok"
		receipt.VerifiedAt = now()
	} else if verificationDeferred(err) {
		receipt.State = "deferred"
	}
	b.CopyVerification = &receipt
	if storeErr := s.Store.Put("backups", b.ID, b); storeErr != nil {
		return CopyVerificationReceipt{}, storeErr
	}
	if err != nil {
		if receipt.State == "deferred" {
			return receipt, errors.New("Kopie wegen Speicher- oder E/A-Problem nicht geprüft; Nachweis zurückgestellt")
		}
		return receipt, errors.New("Kopie nicht bestätigt: separate vollständige Ablage, Manifest, Dateien und Links prüfen")
	}
	return receipt, nil
}
func (s *Service) checkIndependentCopy(b Backup, copyRoot string) error {
	if !filepath.IsAbs(copyRoot) {
		return errors.New("absoluter Kopiepfad erforderlich")
	}
	copyRoot = filepath.Clean(copyRoot)
	info, err := os.Lstat(copyRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Kopiewurzel muss echtes Verzeichnis sein")
	}
	root, err := filepath.EvalSymlinks(copyRoot)
	if err != nil {
		return err
	}
	source, err := filepath.EvalSymlinks(s.backupDir(b))
	if err != nil {
		return err
	}
	within := func(a, b string) bool {
		rel, e := filepath.Rel(a, b)
		return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	if os.SameFile(info, sourceInfo) || within(source, root) || within(root, source) {
		return errors.New("Kopie überlappt Quelle")
	}
	if _, err = s.Manifest(b.ID); err != nil {
		return err
	}
	// A canonical pathname does not pin its ancestors. Anchor both inventory and
	// reads to the opened directory, and prove it is the object approved above.
	handle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer handle.Close()
	opened, err := handle.Lstat(".")
	if err != nil {
		return err
	}
	if !opened.IsDir() || !os.SameFile(info, opened) || os.SameFile(sourceInfo, opened) {
		return errors.New("Kopiewurzel während Prüfung ersetzt")
	}
	// Reject every linked/special inode, including unlisted extras and renamed hardlinks.
	// Collected symlink metadata is stored as ordinary bytes, never live filesystem links.
	snapshot := map[string]os.FileInfo{}
	err = fs.WalkDir(handle.FS(), ".", func(p string, _ fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if len(snapshot) >= 100000 {
			return errors.New("Kopie überschreitet Dateilimit")
		}
		st, e := handle.Lstat(p)
		if e != nil {
			return e
		}
		if !st.IsDir() && !st.Mode().IsRegular() {
			return errors.New("Link oder Sonderdatei in Kopie")
		}
		if stat, ok := st.Sys().(*syscall.Stat_t); !st.IsDir() && (!ok || stat.Nlink != 1) {
			return errors.New("Mehrfach verknüpfte Kopiedatei")
		}
		snapshot[p] = st
		return nil
	})
	if err != nil {
		return err
	}
	// O_NOFOLLOW and inode comparisons prevent replacing a checked file by a
	// live link between descriptor-anchored inventory and byte verification.
	read := func(p string) ([]byte, error) {
		if err := ValidPath(p); err != nil {
			return nil, err
		}
		before, ok := snapshot[p]
		if !ok || !before.Mode().IsRegular() {
			return nil, errors.New("Kopiedatei fehlt")
		}
		f, e := handle.OpenFile(p, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		st, e := f.Stat()
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() || !os.SameFile(before, st) || st.Size() > 64<<20 {
			return nil, errors.New("Kopiedatei wurde ersetzt oder ist zu groß")
		}
		data, e := io.ReadAll(io.LimitReader(f, (64<<20)+1))
		if e != nil {
			return nil, e
		}
		after, e := f.Stat()
		if e != nil {
			return nil, e
		}
		if !unchangedCopyFile(before, after) || int64(len(data)) != after.Size() {
			return nil, errors.New("Kopiedatei während Prüfung verändert")
		}
		return data, nil
	}
	if err = verifyRecordBytes(b, read); err != nil {
		return err
	}
	for p, before := range snapshot {
		after, e := handle.Lstat(p)
		if e != nil {
			return e
		}
		if !unchangedCopyFile(before, after) {
			return errors.New("Kopie während Prüfung verändert")
		}
	}
	// Fail closed if the selected pathname was redirected after the descriptor
	// was opened, even when the pinned original copy itself stayed unchanged.
	namedRoot, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !unchangedCopyFile(opened, namedRoot) || os.SameFile(sourceInfo, namedRoot) {
		return errors.New("Kopiewurzel während Prüfung ersetzt")
	}
	return nil
}
func unchangedCopyFile(a, b os.FileInfo) bool {
	if !os.SameFile(a, b) || a.Mode() != b.Mode() || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) {
		return false
	}
	if !b.IsDir() {
		st, ok := b.Sys().(*syscall.Stat_t)
		return ok && st.Nlink == 1
	}
	return true
}
