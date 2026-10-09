package anker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func exportedPlanPackage(t *testing.T) []byte {
	t.Helper()
	s := testService(t)
	s.Collector = planExportCollector{}
	b, err := s.CreateBackup(context.Background(), "host1")
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
	var out bytes.Buffer
	if err = s.ExportPlan(p.ID, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func rewritePackage(t *testing.T, raw []byte, edit func(*tar.Header, []byte) (*tar.Header, []byte), extra ...*tar.Header) []byte {
	t.Helper()
	var out bytes.Buffer
	w := tar.NewWriter(&out)
	r := tar.NewReader(bytes.NewReader(raw))
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		h, data = edit(h, data)
		if h == nil {
			continue
		}
		h.Size = int64(len(data))
		if err = w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	for _, h := range extra {
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func TestOfflinePlanPackageVerification(t *testing.T) {
	raw := exportedPlanPackage(t)
	result, err := VerifyPlanPackage(bytes.NewReader(raw))
	if err != nil || result.Coverage != "complete" || result.TrustedOrigin || result.RecoveryVerified {
		t.Fatalf("valid package: %+v %v", result, err)
	}
	identity := func(h *tar.Header, b []byte) (*tar.Header, []byte) { return h, b }
	for _, name := range []string{"plan.json", "mapping.json", "manifest.json", "prepared-files/etc/example.conf", "original/inventory/host.json", "original/original-files/etc/network/interfaces", "package-index.json", "package-checksums.sha256"} {
		t.Run("corrupt_"+name, func(t *testing.T) {
			found := false
			bad := rewritePackage(t, raw, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
				if h.Name == name {
					found = true
					b = append(b, '!')
				}
				return h, b
			})
			if !found {
				t.Fatal("missing fixture")
			}
			if _, err := VerifyPlanPackage(bytes.NewReader(bad)); err == nil {
				t.Fatal("corruption accepted")
			}
		})
		t.Run("missing_"+name, func(t *testing.T) {
			bad := rewritePackage(t, raw, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
				if h.Name == name {
					return nil, nil
				}
				return h, b
			})
			if _, err := VerifyPlanPackage(bytes.NewReader(bad)); err == nil {
				t.Fatal("missing entry accepted")
			}
		})
	}
	for _, h := range []*tar.Header{{Name: "../escape", Typeflag: tar.TypeReg}, {Name: "/absolute", Typeflag: tar.TypeReg}, {Name: "a\\b", Typeflag: tar.TypeReg}, {Name: "unsafe\nname", Typeflag: tar.TypeReg}, {Name: "sparse", Typeflag: tar.TypeGNUSparse}, {Name: "plan.json", Typeflag: tar.TypeReg}, {Name: "hard", Typeflag: tar.TypeLink, Linkname: "plan.json"}, {Name: "original/original-files/etc/network-link/child", Typeflag: tar.TypeReg}, {Name: "plan.json/child", Typeflag: tar.TypeReg}} {
		t.Run("unsafe_"+h.Name, func(t *testing.T) {
			if _, err := VerifyPlanPackage(bytes.NewReader(rewritePackage(t, raw, identity, h))); err == nil {
				t.Fatal("unsafe entry accepted")
			}
		})
	}
	bad := rewritePackage(t, raw, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
		if h.Name == "original/original-files/etc/network-link" {
			h.Linkname = "/changed"
		}
		return h, b
	})
	if _, err := VerifyPlanPackage(bytes.NewReader(bad)); err == nil {
		t.Fatal("symlink metadata tampering accepted")
	}
	if _, err := verifyPlanPackage(bytes.NewReader(raw), 2, 1<<20); err == nil {
		t.Fatal("entry bound ignored")
	}
	if _, err := verifyPlanPackage(bytes.NewReader(raw), 50000, 50); err == nil {
		t.Fatal("byte bound ignored")
	}
	if _, err := VerifyPlanPackage(bytes.NewReader(raw[:len(raw)/2])); err == nil {
		t.Fatal("truncated package accepted")
	}

	if _, err := VerifyPlanPackage(bytes.NewReader(append(append([]byte{}, raw...), []byte("appended data")...))); err == nil {
		t.Fatal("trailing payload accepted")
	}
	bad = rewritePackage(t, raw, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
		if h.Name == "original/original-files/etc/network/interfaces" {
			h.Mode = 0777
		}
		return h, b
	})
	if _, err := VerifyPlanPackage(bytes.NewReader(bad)); err == nil {
		t.Fatal("mode tampering accepted")
	}
	legacy := rewritePackage(t, raw, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
		if h.Name == "package-index.json" || h.Name == "package-checksums.sha256" {
			return nil, nil
		}
		return h, b
	})
	result, err = VerifyPlanPackage(bytes.NewReader(legacy))
	if err != nil || result.Coverage != "limited" || result.TrustedOrigin || result.RecoveryVerified {
		t.Fatalf("legacy: %+v %v", result, err)
	}
	bad = rewritePackage(t, legacy, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
		if h.Name == "original/files/etc/example.conf" {
			b = []byte("corrupt")
		}
		return h, b
	})
	if _, err := VerifyPlanPackage(bytes.NewReader(bad)); err == nil {
		t.Fatal("corrupt legacy source accepted")
	}
}

func TestOfflinePlanPackageTargetBeforeAndFreshGuide(t *testing.T) {
	s, _, r := snapshotSetup(t)
	p, err := s.CreatePlan(t.Context(), requestDecision(t, r, r.Files[0], "backup"))
	if err != nil {
		t.Fatal(err)
	}
	p.Tasks = append(p.Tasks, RecoveryTask{ID: "late", Evidence: "Nachträglich dokumentierte Aussage", ConfirmedBy: "operator", ConfirmedAt: "2026-10-09T10:00:00Z"})
	if err = writeJSON(filepath.Join(s.Root, "plans", p.ID, "plan.json"), p); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.Put("plans", p.ID, p); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(s.Root, "plans", p.ID, "WIEDERHERSTELLUNG.md"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = s.ExportPlan(p.ID, &out); err != nil {
		t.Fatal(err)
	}
	raw := out.Bytes()
	if result, err := VerifyPlanPackage(bytes.NewReader(raw)); err != nil || result.Coverage != "complete" {
		t.Fatalf("snapshot package: %+v %v", result, err)
	}
	for _, name := range []string{"target-snapshot.json", "target-before/" + r.Files[0]} {
		found := false
		bad := rewritePackage(t, raw, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
			if h.Name == name {
				found = true
				b = append(b, '!')
			}
			return h, b
		})
		if !found {
			t.Fatal("target fixture missing", name)
		}
		if _, err = VerifyPlanPackage(bytes.NewReader(bad)); err == nil {
			t.Fatal("target evidence corruption accepted")
		}
	}
	found := false
	rewritePackage(t, raw, func(h *tar.Header, b []byte) (*tar.Header, []byte) {
		if h.Name == "WIEDERHERSTELLUNG.md" {
			found = strings.Contains(string(b), "Nachträglich dokumentierte Aussage")
		}
		return h, b
	})
	if !found {
		t.Fatal("export used stale readable evidence")
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, "plans", p.ID, "WIEDERHERSTELLUNG.md"))
	if !bytes.Equal(before, after) {
		t.Fatal("read-only export rewrote saved guide")
	}
}

func TestPackageIndexMatchesTarTimestampRounding(t *testing.T) {
	var out bytes.Buffer
	w := &indexedPlanTar{Writer: tar.NewWriter(&out)}
	if err := w.WriteHeader(&tar.Header{Name: "example", Typeflag: tar.TypeReg, Mode: 0600, ModTime: time.Unix(1700000000, 900000000)}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := tar.NewReader(&out)
	h, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if h.ModTime.Unix() != 1700000001 {
		t.Fatalf("unexpected archive time: %v", h.ModTime)
	}
	h, err = r.Next()
	if err != nil || h.Name != "package-index.json" {
		t.Fatal("missing index", err)
	}
	var index packageIndex
	if err = json.NewDecoder(r).Decode(&index); err != nil {
		t.Fatal(err)
	}
	if len(index.Entries) != 1 || index.Entries[0].MTime != 1700000001 {
		t.Fatalf("index lost serialized timestamp: %+v", index)
	}
}
