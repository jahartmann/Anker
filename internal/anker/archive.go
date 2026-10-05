package anker

import (
	"archive/tar"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func ExtractTar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	seen := map[string]bool{}
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p := strings.TrimSuffix(h.Name, "/")
		if seen[p] {
			return errors.New("doppelter Archivpfad")
		}
		seen[p] = true
		if len(seen) > 100000 || h.Size < 0 || h.Size > 64<<20 || total+h.Size > 2<<30 {
			return errors.New("Archiv überschreitet Erfassungslimit")
		}
		total += h.Size
		full, err := safeJoin(dest, p)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(full, 0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err = os.MkdirAll(filepath.Dir(full), 0700); err != nil {
				return err
			}
			f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			_, err = io.CopyN(f, tr, h.Size)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("Archivlinks und Sonderdateien sind nicht erlaubt")
		}
	}
}
func tarTree(tw *tar.Writer, root string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "archive.tar.gz" || strings.HasPrefix(rel, ".write-") {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return errors.New("unerwarteter Symlink in Sicherungsablage")
		}
		h, err := tar.FileInfoHeader(st, "")
		if err != nil {
			return err
		}
		h.Name = rel
		h.Mode = 0600
		if st.IsDir() {
			h.Mode = 0700
		}
		if err = tw.WriteHeader(h); err != nil {
			return err
		}
		if st.Mode().IsRegular() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tw, f)
			f.Close()
			return copyErr
		}
		return nil
	})
}
func (s *Service) ExportBackup(id string, w io.Writer) error {
	if err := s.EnsureReadable(id); err != nil {
		return err
	}
	if err := s.VerifyBackup(id); err != nil {
		return err
	}
	b, _ := s.Backup(id)
	m, err := s.Manifest(id)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(w)
	if err = tarTree(tw, s.backupDir(b)); err != nil {
		tw.Close()
		return err
	}
	for _, e := range m.Entries {
		h := &tar.Header{Name: "original-files/" + e.Path, Mode: int64(e.Mode), Uid: e.UID, Gid: e.GID, ModTime: unixTime(e.MTime)}
		h.Xattrs = map[string]string{}
		for name, encoded := range e.XAttrs {
			value, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				tw.Close()
				return fmt.Errorf("ungültiges Extended Attribute %s: %w", name, err)
			}
			h.Xattrs[name] = string(value)
		}
		switch e.Type {
		case "directory":
			h.Typeflag = tar.TypeDir
		case "symlink":
			h.Typeflag = tar.TypeSymlink
			h.Linkname = e.Link
		case "file":
			h.Typeflag = tar.TypeReg
			h.Size = e.Size
		default:
			continue
		}
		if err = tw.WriteHeader(h); err != nil {
			tw.Close()
			return err
		}
		if e.Type == "file" {
			p, err := safeJoin(filepath.Join(s.backupDir(b), "files"), e.Path)
			if err != nil {
				tw.Close()
				return err
			}
			f, err := os.Open(p)
			if err != nil {
				tw.Close()
				return err
			}
			_, err = io.Copy(tw, f)
			f.Close()
			if err != nil {
				tw.Close()
				return err
			}
		}
	}
	return tw.Close()
}
func (s *Service) verifyAt(id, root string) error {
	b, err := s.Backup(id)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return err
	}
	if Hash(data) != b.ManifestSHA {
		return errors.New("Archivmanifest verändert")
	}
	m, err := s.Manifest(id)
	if err != nil {
		return err
	}
	for _, e := range m.Entries {
		if e.Type == "directory" {
			continue
		}
		p, err := safeJoin(filepath.Join(root, "files"), e.Path)
		if err != nil {
			return err
		}
		d, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if Hash(d) != e.SHA256 {
			return fmt.Errorf("beschädigt: %s", e.Path)
		}
	}
	for p, hash := range m.Artifacts {
		full, err := safeJoin(root, p)
		if err != nil {
			return err
		}
		d, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if Hash(d) != hash {
			return fmt.Errorf("beschädigt: %s", p)
		}
	}
	return nil
}
func (s *Service) extractArchive(b Backup) (string, error) {
	temp, err := os.MkdirTemp(filepath.Join(s.Root, "staging"), "archive-")
	if err != nil {
		return "", err
	}
	f, err := os.Open(filepath.Join(s.backupDir(b), "archive.tar.gz"))
	if err != nil {
		os.RemoveAll(temp)
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err == nil {
		err = ExtractTar(gz, temp)
		gz.Close()
	}
	if err == nil {
		err = s.verifyAt(b.ID, temp)
	}
	if err != nil {
		os.RemoveAll(temp)
		return "", err
	}
	return temp, nil
}
func (s *Service) ArchiveBackup(id string) error {
	if err := s.CheckSpace(64 << 20); err != nil {
		return err
	}
	if err := s.acquire("backup:" + id); err != nil {
		return err
	}
	defer s.release("backup:" + id)
	b, err := s.Backup(id)
	if err != nil {
		return err
	}
	if b.Archived {
		return nil
	}
	if b.Pinned {
		return errors.New("markierte Sicherung wird nicht archiviert")
	}
	if err = s.VerifyBackup(id); err != nil {
		return err
	}
	p := filepath.Join(s.backupDir(b), "archive.tar.gz")
	temp, err := os.CreateTemp(filepath.Join(s.Root, "staging"), "archive-*.gz")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	gz := gzip.NewWriter(temp)
	tw := tar.NewWriter(gz)
	err = tarTree(tw, s.backupDir(b))
	if e := tw.Close(); err == nil {
		err = e
	}
	if e := gz.Close(); err == nil {
		err = e
	}
	if e := temp.Sync(); err == nil {
		err = e
	}
	temp.Close()
	if err != nil {
		return err
	}
	if err = os.Rename(temp.Name(), p); err != nil {
		return err
	}
	check, err := s.extractArchive(b)
	if err != nil {
		os.Remove(p)
		return err
	}
	os.RemoveAll(check)
	b.Archived = true
	if err = s.Store.Put("backups", id, b); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.backupDir(b), "files"))
}
func (s *Service) EnsureReadable(id string) error {
	b, err := s.Backup(id)
	if err != nil {
		return err
	}
	if !b.Archived {
		return nil
	}
	if err = s.acquire("backup:" + id); err != nil {
		return err
	}
	defer s.release("backup:" + id)
	temp, err := s.extractArchive(b)
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if _, err = os.Stat(filepath.Join(s.backupDir(b), "files")); os.IsNotExist(err) {
		if err = os.Rename(filepath.Join(temp, "files"), filepath.Join(s.backupDir(b), "files")); err != nil {
			return err
		}
	}
	b.Archived = false
	return s.Store.Put("backups", id, b)
}

type FileDiff struct {
	Path   string `json:"path"`
	Change string `json:"change"`
	Secret bool   `json:"secret"`
	Diff   string `json:"diff,omitempty"`
}

func (s *Service) DiffBackups(from, to, p string) ([]FileDiff, error) {
	a, err := s.Manifest(from)
	if err != nil {
		return nil, err
	}
	b, err := s.Manifest(to)
	if err != nil {
		return nil, err
	}
	left := map[string]Entry{}
	right := map[string]Entry{}
	paths := map[string]bool{}
	for _, e := range a.Entries {
		left[e.Path] = e
		paths[e.Path] = true
	}
	for _, e := range b.Entries {
		right[e.Path] = e
		paths[e.Path] = true
	}
	out := []FileDiff{}
	for path := range paths {
		if p != "" && p != path {
			continue
		}
		a, okA := left[path]
		b, okB := right[path]
		if okA && okB && a.SHA256 == b.SHA256 && a.Mode == b.Mode && a.Link == b.Link {
			continue
		}
		kind := "changed"
		if !okA {
			kind = "added"
		}
		if !okB {
			kind = "removed"
		}
		out = append(out, FileDiff{Path: path, Change: kind, Secret: a.Secret || b.Secret})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func unixTime(v int64) time.Time { return time.Unix(v, 0) }
