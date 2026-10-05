package anker

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestExportContainsOriginalMetadataAndGuide(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	var buf bytes.Buffer
	if err := s.ExportBackup(b.ID, &buf); err != nil {
		t.Fatal(err)
	}
	r := tar.NewReader(&buf)
	found := map[string]bool{}
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		found[h.Name] = true
		if h.Name == "original-files/etc/network/interfaces" && h.Mode != 0640 {
			t.Fatal(h.Mode)
		}
	}
	for _, p := range []string{"manifest.json", "WIEDERHERSTELLUNG.md", "original-files/etc/network/interfaces"} {
		if !found[p] {
			t.Fatal(p)
		}
	}
}
func TestArchiveVerifiesAndReopens(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	if err := s.ArchiveBackup(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyBackup(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureReadable(b.ID); err != nil {
		t.Fatal(err)
	}
	data, _, err := s.ReadFile(b.ID, "etc/network/interfaces")
	if err != nil || !bytes.Contains(data, []byte("eno1")) {
		t.Fatalf("%s %v", data, err)
	}
}
func TestExtractRejectsTraversalAndLinks(t *testing.T) {
	for _, h := range []*tar.Header{{Name: "../outside", Mode: 0600, Size: 1, Typeflag: tar.TypeReg}, {Name: "link", Linkname: "/etc", Typeflag: tar.TypeSymlink}} {
		var b bytes.Buffer
		w := tar.NewWriter(&b)
		w.WriteHeader(h)
		if h.Size == 1 {
			w.Write([]byte("x"))
		}
		w.Close()
		if ExtractTar(&b, t.TempDir()) == nil {
			t.Fatal("unsafe tar accepted")
		}
	}
}
func TestCorruptArchiveDoesNotReplaceBackup(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	s.ArchiveBackup(b.ID)
	os.WriteFile(filepath.Join(s.backupDir(b), "archive.tar.gz"), []byte("bad"), 0600)
	if s.EnsureReadable(b.ID) == nil {
		t.Fatal("bad archive accepted")
	}
	if _, err := s.Manifest(b.ID); err != nil {
		t.Fatal("manifest damaged", err)
	}
}
func TestDiffShowsChangedFile(t *testing.T) {
	s := testService(t)
	a, _ := s.CreateBackup(context.Background(), "host1")
	b, _ := s.CreateBackup(context.Background(), "host1")
	d, err := s.DiffBackups(a.ID, b.ID, "")
	if err != nil || len(d) != 0 {
		t.Fatalf("%v %v", d, err)
	}
}
