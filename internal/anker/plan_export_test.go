package anker

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type planExportCollector struct {
	fixtureCollector
	originalXAttrs map[string]string
}

func (c planExportCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	col, err := c.fixtureCollector.Collect(ctx, h, dest)
	if err != nil {
		return col, err
	}
	col.Entries[0].UID = 1234
	col.Entries[0].GID = 2345
	col.Entries[0].MTime = 1700000000
	col.Entries[0].XAttrs = map[string]string{"user.note": "b3JpZ2luYWw="}
	if c.originalXAttrs != nil {
		col.Entries[0].XAttrs = c.originalXAttrs
	}
	if err = os.WriteFile(filepath.Join(dest, "files/etc/example.conf"), []byte("enabled=yes\n"), 0600); err != nil {
		return col, err
	}
	col.Entries = append(col.Entries,
		Entry{Path: "etc/example.conf", Type: "file", Mode: 0644, UID: 0, GID: 0},
		Entry{Path: "etc/network", Type: "directory", Mode: 0750, UID: 1234, GID: 2345, MTime: 1700000000},
		Entry{Path: "etc/network-link", Type: "symlink", Link: "network/interfaces", Mode: 0777, UID: 1234, GID: 2345, MTime: 1700000000},
	)
	return col, nil
}

// A plan package must support manual recovery with the same original metadata
// and links as a backup package, even when its source was archived.
func TestPlanExportContainsOriginalMetadataAndLinks(t *testing.T) {
	for _, archived := range []bool{false, true} {
		name := "readable"
		if archived {
			name = "archived"
		}
		t.Run(name, func(t *testing.T) {
			s := testService(t)
			s.Collector = planExportCollector{}
			b, err := s.CreateBackup(context.Background(), "host1")
			if err != nil {
				t.Fatal(err)
			}
			originalManifest, err := os.ReadFile(filepath.Join(s.backupDir(b), "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err = s.SaveHost(Host{ID: "target", Name: "new-pve", Address: "192.0.2.20", SSHUser: "anker", SSHPort: 22}); err != nil {
				t.Fatal(err)
			}
			s.Collector = targetCollector{inv: Inventory{Hostname: "new-pve", PVEVersion: "8.4", Interfaces: []Interface{{Name: "ens3"}}}}
			p, err := s.CreatePlan(context.Background(), PlanRequest{BackupID: b.ID, TargetID: "target", Scenario: "migration", Mapping: Mapping{Interfaces: map[string]string{"eno1": "ens3"}}, ConsoleConfirmed: true, SourceOffline: true})
			if err != nil {
				t.Fatal(err)
			}
			if archived {
				if err = s.ArchiveBackup(b.ID); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			if err = s.ExportPlan(p.ID, &out); err != nil {
				t.Fatal(err)
			}
			headers := map[string]tar.Header{}
			contents := map[string][]byte{}
			r := tar.NewReader(&out)
			for {
				h, readErr := r.Next()
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					t.Fatal(readErr)
				}
				headers[h.Name] = *h
				contents[h.Name], readErr = io.ReadAll(r)
				if readErr != nil {
					t.Fatal(readErr)
				}
			}
			for _, path := range []string{"package-index.json", "package-checksums.sha256", "plan.json", "mapping.json", "WIEDERHERSTELLUNG.md", "prepared-files/etc/example.conf", "original/manifest.json", "original/checksums.sha256", "original/WIEDERHERSTELLUNG.md", "original/original-files/etc/network/interfaces", "original/original-files/etc/network", "original/original-files/etc/network-link"} {
				if _, ok := headers[path]; !ok {
					t.Errorf("plan export is missing %s", path)
				}
			}
			if !bytes.Equal(contents["prepared-files/etc/example.conf"], []byte("enabled=yes\n")) {
				t.Error("prepared config content was lost")
			}
			file := headers["original/original-files/etc/network/interfaces"]
			if file.Typeflag != tar.TypeReg || file.Mode != 0640 || file.Uid != 1234 || file.Gid != 2345 || file.ModTime.Unix() != 1700000000 || file.Xattrs["user.note"] != "original" {
				t.Errorf("original file metadata was lost: %+v", file)
			}
			if !bytes.Equal(contents["original/original-files/etc/network/interfaces"], []byte("auto vmbr0\niface vmbr0 inet static\n  bridge-ports eno1\n")) {
				t.Error("original content was replaced by prepared content")
			}
			dir := headers["original/original-files/etc/network"]
			if dir.Typeflag != tar.TypeDir || dir.Mode != 0750 || dir.Uid != 1234 || dir.Gid != 2345 {
				t.Errorf("original directory metadata was lost: %+v", dir)
			}
			link := headers["original/original-files/etc/network-link"]
			if link.Typeflag != tar.TypeSymlink || link.Linkname != "network/interfaces" || link.Uid != 1234 || link.Gid != 2345 {
				t.Errorf("original symlink was not reconstructed: %+v", link)
			}
			afterManifest, err := os.ReadFile(filepath.Join(s.backupDir(b), "manifest.json"))
			if err != nil || !bytes.Equal(afterManifest, originalManifest) {
				t.Fatal("export changed the source manifest", err)
			}
			st, err := os.Lstat(filepath.Join(s.backupDir(b), "files/etc/network-link"))
			if err != nil || !st.Mode().IsRegular() {
				t.Fatal("export changed the inert source link", err)
			}
		})
	}
}

func TestPlanGuideIncludesHumanEvidenceAndBindings(t *testing.T) {
	p := Plan{ID: "plan-7", BackupID: "backup-7", TargetID: "target-7", CreatedAt: "2026-10-09T08:00:00Z", Scenario: "cluster-disaster", Tasks: []RecoveryTask{{ID: "check", Title: "Isolation", State: "user_confirmed", ConfirmedBy: "operator-7", ConfirmedAt: "2026-10-09T09:00:00Z", Evidence: "Kabel getrennt.\nKonsole geprüft."}}}
	guide := planGuide(p, Manifest{})
	for _, want := range []string{"Kabel getrennt.\nKonsole geprüft.", "operator-7", "2026-10-09T09:00:00Z", "backup-7", "target-7", "2026-10-09T08:00:00Z", "Menschliche Angabe", "Abhängigkeiten", "Quorum"} {
		if !strings.Contains(guide, want) {
			t.Errorf("readable evidence/dossier missing %q", want)
		}
	}
}

func TestFreshGuideDescribesCurrentVerification(t *testing.T) {
	c := &recoveryResultCollector{}
	s, p := recoveryReadyPlan(t, c)
	var buf bytes.Buffer
	if err := s.ExportPlan(p.ID, &buf); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(&buf)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "WIEDERHERSTELLUNG.md" {
			raw, _ := io.ReadAll(tr)
			for _, required := range []string{"anker restore verify PLAN", "gültiger Baseline", "Maschinenidentität", "Reale PVE-Laborabnahme wurde nicht ausgeführt"} {
				if !strings.Contains(string(raw), required) {
					t.Fatalf("current guide lacks %q", required)
				}
			}
			if strings.Contains(string(raw), "bis Task 8/Integration Task 12 unvollständig") {
				t.Fatal("fresh export still marks current verification capability as unimplemented until Task 8/12")
			}
			return
		}
	}
	t.Fatal("guide missing")
}
