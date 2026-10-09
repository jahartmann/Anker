package anker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProtectedReferenceBindsOneNamedStandPerHost(t *testing.T) {
	s := testService(t)
	a, _ := s.CreateBackup(context.Background(), "host1")
	b, _ := s.CreateBackup(context.Background(), "host1")
	if _, err := s.SaveProtectedReference("host1", a.ID, "Vor Umbau", "Geprüfte Basis", nil, "operator"); err != nil {
		t.Fatal(err)
	}
	ref, err := s.ProtectedReference("host1")
	if err != nil || ref.State != "valid" || ref.Name != "Vor Umbau" || ref.BackupID != a.ID || ref.Drill != nil {
		t.Fatal(ref, err)
	}
	if _, err = s.SaveProtectedReference("host1", b.ID, "Nach Umbau", "Neue Basis", nil, "operator"); err != nil {
		t.Fatal(err)
	}
	ref, _ = s.ProtectedReference("host1")
	if ref.BackupID != b.ID || ref.Name != "Nach Umbau" || ref.Drill != nil {
		t.Fatal(ref)
	}
	if err = s.SaveHost(Host{ID: "other", Name: "other", Address: "192.0.2.2"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveProtectedReference("other", a.ID, "Falsch", "", nil, "operator"); err == nil {
		t.Fatal("cross-host reference accepted")
	}
}

func TestProtectedReferenceSurvivesRetention(t *testing.T) {
	s := testService(t)
	a, _ := s.CreateBackup(context.Background(), "host1")
	b, _ := s.CreateBackup(context.Background(), "host1")
	a.CreatedAt = "2026-01-01T00:00:00Z"
	b.CreatedAt = "2026-10-09T00:00:00Z"
	s.Store.Put("backups", a.ID, a)
	s.Store.Put("backups", b.ID, b)
	if _, err := s.SaveProtectedReference("host1", a.ID, "Basis", "", nil, "alice"); err != nil {
		t.Fatal(err)
	}
	settings, _ := s.Settings()
	settings.Daily, settings.Weekly, settings.Monthly, settings.ArchiveDays = 1, 1, 1, 0
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := s.Maintain(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Backup(a.ID); err != nil {
		t.Fatal("named reference pruned", err)
	}
}

func TestProtectedReferenceRoutesEnforceRoleAndExposeSummary(t *testing.T) {
	s := testService(t)
	a, _ := s.CreateBackup(context.Background(), "host1")
	b, _ := s.CreateBackup(context.Background(), "host1")
	path := "/api/hosts/host1/reference"
	body := map[string]any{"backup_id": a.ID, "name": "Basis", "reason": "Vor Änderung", "drill": map[string]string{"tested_at": "2026-10-09T10:00:00Z", "outcome": "passed", "evidence": "Laborbericht"}}
	reader := &User{ID: "reader", Role: "reader"}
	if w := comparisonRequest(t, s, "PUT", path, body, reader); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := comparisonRequest(t, s, "PUT", path, body, &User{ID: "alice", Name: "alice", Role: "restore"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"actor":"alice"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = comparisonRequest(t, s, "GET", path, nil, reader)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Basis"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = comparisonRequest(t, s, "GET", "/api/status", nil, reader)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"references":{"host1":`) || !strings.Contains(w.Body.String(), `"evidence_revision":`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = comparisonRequest(t, s, "GET", path+"/compare?to="+b.ID, nil, reader)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"to_id":"`+b.ID+`"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestProtectedReferenceDrillIsHumanAndSourceBound(t *testing.T) {
	s := testService(t)
	a, _ := s.CreateBackup(context.Background(), "host1")
	drill := &ManualDrill{TestedAt: "2026-10-09T10:00:00Z", Outcome: "passed", Evidence: "Isolierte Probe laut Laborprotokoll"}
	ref, err := s.SaveProtectedReference("host1", a.ID, "Basis", "", drill, "alice")
	if err != nil || ref.Drill == nil || ref.Drill.Actor != "alice" || ref.Drill.RecordedAt == "" || ref.Drill.ManifestSHA != a.ManifestSHA {
		t.Fatal(ref, err)
	}
	updated, err := s.SaveProtectedReference("host1", a.ID, "Basis neu", "", nil, "bob")
	if err != nil || updated.Drill == nil || updated.Drill.Actor != "alice" || updated.Drill.RecordedAt != ref.Drill.RecordedAt {
		t.Fatal("label edit changed existing testimony", updated, err)
	}
	r, err := s.BackupReadiness(a.ID)
	if err != nil || readinessState(r.Items, "offline_drill") != "ok" {
		t.Fatal(r, err)
	}
	if _, err = s.ClearProtectedReferenceDrill("host1", "alice"); err != nil {
		t.Fatal(err)
	}
	r, _ = s.BackupReadiness(a.ID)
	if readinessState(r.Items, "offline_drill") != "unknown" {
		t.Fatal("removed testimony remained current")
	}
}

func TestProtectedReferenceMissingAndDamagedStayUnresolved(t *testing.T) {
	s := testService(t)
	a, _ := s.CreateBackup(context.Background(), "host1")
	if _, err := s.SaveProtectedReference("host1", a.ID, "Basis", "", nil, "alice"); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(s.backupDir(a), "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	ref, _ := s.ProtectedReference("host1")
	if ref.State != "unresolved" {
		t.Fatal("corrupt reference resolved", ref)
	}
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	ref, _ = s.ProtectedReference("host1")
	if ref.State != "unresolved" {
		t.Fatal(ref)
	}
	if _, err := s.CompareProtectedReference("host1", a.ID); err == nil {
		t.Fatal("unresolved reference compared")
	}
	if err := s.Store.Delete("backups", a.ID); err != nil {
		t.Fatal(err)
	}
	ref, _ = s.ProtectedReference("host1")
	if ref.State != "unresolved" {
		t.Fatal(ref)
	}
}

func TestProtectedReferenceSummarySeparatesMTimeNoise(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/a": "same", "etc/b": "same", "etc/c": "same"}, map[string]string{"etc/a": "same", "etc/b": "changed", "etc/c": "same"})
	ma, _ := s.Manifest(a.ID)
	mb, _ := s.Manifest(b.ID)
	for i := range mb.Entries {
		if mb.Entries[i].Path == "etc/a" {
			for _, original := range ma.Entries {
				if original.Path == "etc/a" {
					mb.Entries[i].MTime = original.MTime + 10
				}
			}
		}
		if mb.Entries[i].Path == "etc/c" {
			mb.Entries[i].Mode = 0640
		}
	}
	writeTestManifest(t, s, b, mb)
	if _, err := s.SaveProtectedReference("host1", a.ID, "Basis", "", nil, "alice"); err != nil {
		t.Fatal(err)
	}
	summary, err := s.CompareProtectedReference("host1", b.ID)
	if err != nil || summary.Changed != 2 || summary.MTimeOnly != 1 || summary.Content != 1 || summary.Metadata != 1 {
		t.Fatal(summary, err)
	}
}

func writeTestManifest(t *testing.T, s *Service, b Backup, m Manifest) {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.backupDir(b), "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	b.ManifestSHA = Hash(raw)
	if err = s.Store.Put("backups", b.ID, b); err != nil {
		t.Fatal(err)
	}
}
