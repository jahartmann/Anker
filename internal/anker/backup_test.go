package anker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fixtureCollector struct{ partial bool }

func (f fixtureCollector) Probe(_ context.Context, h Host) (Inventory, error) {
	return Inventory{Hostname: h.Name, PVEVersion: "8.4", Interfaces: []Interface{{Name: "eno1", MAC: "aa:bb"}}}, nil
}
func (f fixtureCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	inv, _ := f.Probe(ctx, h)
	os.MkdirAll(filepath.Join(dest, "files/etc/network"), 0700)
	os.WriteFile(filepath.Join(dest, "files/etc/network/interfaces"), []byte("auto vmbr0\niface vmbr0 inet static\n  bridge-ports eno1\n"), 0600)
	entries := []Entry{{Path: "etc/network/interfaces", Type: "file", Mode: 0640, UID: 0, GID: 0}}
	warnings := []string{}
	if f.partial {
		warnings = append(warnings, "Pflichtpfad /etc/pve fehlt")
	}
	return Collection{Inventory: inv, Entries: entries, Warnings: warnings}, nil
}
func (f fixtureCollector) Apply(context.Context, Host, Plan, string) (ApplyResult, error) {
	return ApplyResult{Applied: []string{"etc/network/interfaces"}}, nil
}
func testService(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	store, err := OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	svc, err := NewService(root, store, fixtureCollector{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveHost(Host{ID: "host1", Name: "pve-test", Address: "192.0.2.1", SSHUser: "anker", SSHPort: 22, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return svc
}
func TestBackupPreservesMetadataAndPublishes(t *testing.T) {
	s := testService(t)
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "successful" {
		t.Fatalf("status %s", b.Status)
	}
	m, err := s.Manifest(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 1 || m.Entries[0].Mode != 0640 || len(m.Entries[0].SHA256) != 64 {
		t.Fatalf("manifest: %+v", m)
	}
	if err := s.VerifyBackup(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.backupDir(b), "WIEDERHERSTELLUNG.md")); err != nil {
		t.Fatal(err)
	}
}
func TestBackupDetectsTampering(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	os.WriteFile(filepath.Join(s.backupDir(b), "files/etc/network/interfaces"), []byte("changed"), 0600)
	if s.VerifyBackup(b.ID) == nil {
		t.Fatal("damaged backup accepted")
	}
}
func TestMissingRequiredFilesIsPartial(t *testing.T) {
	s := testService(t)
	s.Collector = fixtureCollector{partial: true}
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "partial" {
		t.Fatal(b.Status)
	}
}
func TestBackupRejectsTraversal(t *testing.T) {
	for _, p := range []string{"../etc/passwd", "/etc/passwd", "etc/../../secret", "a\\..\\b", ""} {
		if ValidPath(p) == nil {
			t.Fatalf("accepted %q", p)
		}
	}
}
func TestStoreSurvivesReopen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "catalog.db")
	s, err := OpenStore(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Put("hosts", "h", Host{ID: "h", Name: "test"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenStore(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var h Host
	if err = s.Get("hosts", "h", &h); err != nil || h.Name != "test" {
		t.Fatalf("%+v %v", h, err)
	}
}

func TestExtraRequiredPathProducesPartialBackup(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	h.ExtraPaths = []string{"/opt/application"}
	s.SaveHost(h)
	b, err := s.CreateBackup(context.Background(), h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "partial" || len(b.Warnings) == 0 {
		t.Fatal("missing required path accepted", b)
	}
}
func TestReindexPreservesPinAndRejectsChangedManifest(t *testing.T) {
	s := testService(t)
	b, _ := s.CreateBackup(context.Background(), "host1")
	b.Pinned = true
	s.Store.Put("backups", b.ID, b)
	if _, err := s.Reindex(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Backup(b.ID)
	if !got.Pinned {
		t.Fatal("pin lost during reindex")
	}
	p := filepath.Join(s.backupDir(b), "manifest.json")
	data, _ := os.ReadFile(p)
	os.WriteFile(p, append(data, ' '), 0600)
	if _, err := s.Reindex(); err == nil {
		t.Fatal("catalog trust silently replaced")
	}
}
