package anker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type readinessCollector struct {
	fixtureCollector
	profile      *CollectionProfile
	dependencies []DependencyEvidence
	queries      map[string]string
	payloads     bool
}

type readinessLinkCollector struct{ fixtureCollector }

func (c readinessLinkCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	result, err := c.fixtureCollector.Collect(ctx, h, dest)
	if err != nil {
		return result, err
	}
	result.Profile = &CollectionProfile{Version: 1, Roots: []string{"/etc"}, DependencyEntryLimit: 10000, DependencyFileLimit: 1048576, DependencyReferenceLimit: 4096, DependencyScanComplete: true}
	result.Entries = append(result.Entries, Entry{Path: "etc/network/current", Type: "symlink", Link: "/opt/external-target", Mode: 0777})
	return result, nil
}

func (c readinessCollector) Collect(ctx context.Context, h Host, dest string) (Collection, error) {
	result, err := c.fixtureCollector.Collect(ctx, h, dest)
	if err != nil {
		return result, err
	}
	result.Profile = c.profile
	result.Dependencies = c.dependencies
	if c.queries != nil {
		states := map[string]map[string]string{}
		for key, state := range c.queries {
			states[key] = map[string]string{"state": state}
		}
		data, _ := json.Marshal(states)
		result.Inventory.Details = map[string]json.RawMessage{"command_results": data}
		if c.payloads {
			for _, name := range []string{"users", "groups", "file_hashes", "file_metadata"} {
				result.Inventory.Details[name] = json.RawMessage(`{}`)
			}
		}
	}
	return result, nil
}

func readinessState(items []ReadinessItem, id string) string {
	for _, item := range items {
		if item.ID == id {
			return item.State
		}
	}
	return "absent"
}

func TestReadinessKeepsMissingProofSeparateAndUsesCapturedAuthority(t *testing.T) {
	s := testService(t)
	h, _ := s.Host("host1")
	h.ExtraPaths = []string{"/opt/required"}
	if err := s.SaveHost(h); err != nil {
		t.Fatal(err)
	}
	s.Collector = readinessCollector{
		profile:      &CollectionProfile{Version: 1, Roots: []string{"/etc", "/usr/local"}, DependencyEntryLimit: 10000, DependencyFileLimit: 1048576, DependencyReferenceLimit: 4096, DependencyScanComplete: true},
		dependencies: []DependencyEvidence{{Path: "/opt/secret.key", Source: "etc/app.conf", State: "missing", Reason: "outside_capture"}},
		queries:      map[string]string{"pve_version": "ok", "users": "failed", "groups": "ok", "file_inventory": "failed"},
	}
	b, err := s.CreateBackup(context.Background(), h.ID)
	if err != nil {
		t.Fatal(err)
	}
	h.ExtraPaths = nil
	if err := s.SaveHost(h); err != nil {
		t.Fatal(err)
	}
	r, err := s.BackupReadiness(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"query.users", "query.file_inventory"} {
		if got := readinessState(r.Items, id); got != "failed" {
			t.Fatalf("%s=%s", id, got)
		}
	}
	if got := readinessState(r.Items, "helper"); got != "unknown" {
		t.Fatalf("old helper=%s", got)
	}
	if got := readinessState(r.Items, "verification"); got != "unknown" {
		t.Fatalf("unverified=%s", got)
	}
	for _, id := range []string{"independent_copy", "offline_drill"} {
		if got := readinessState(r.Items, id); got != "unknown" {
			t.Fatalf("%s=%s", id, got)
		}
	}
	if len(r.Coverage) != 3 || r.Coverage[2].Path != "/opt/required" || r.Coverage[2].State != "missing" {
		t.Fatalf("coverage=%+v", r.Coverage)
	}
	if len(r.Dependencies) != 1 || r.Dependencies[0].State != "missing" {
		t.Fatalf("dependencies=%+v", r.Dependencies)
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "fully_recoverable") {
		t.Fatal("overall claim", string(data))
	}
}

func TestReadinessLegacyUnknownAndFailedVerification(t *testing.T) {
	s := testService(t)
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.BackupReadiness(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := readinessState(r.Items, "profile"); got != "unknown" {
		t.Fatalf("legacy profile=%s", got)
	}
	if got := readinessState(r.Items, "query.users"); got != "unknown" {
		t.Fatalf("legacy users=%s", got)
	}
	b.VerifiedAt = now()
	b.VerificationError = "password=private-value"
	b.Status = "damaged"
	if err := s.Store.Put("backups", b.ID, b); err != nil {
		t.Fatal(err)
	}
	r, err = s.BackupReadiness(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := readinessState(r.Items, "verification"); got != "failed" {
		t.Fatalf("failed verification=%s", got)
	}
	for _, item := range r.Items {
		if strings.Contains(item.Detail, "private-value") {
			t.Fatal("verification secret leaked")
		}
	}
	if err := os.WriteFile(filepath.Join(s.backupDir(b), "manifest.json"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err = s.BackupReadiness(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := readinessState(r.Items, "manifest"); got != "failed" {
		t.Fatalf("tampered manifest=%s", got)
	}
	if len(r.Coverage) != 0 || len(r.Dependencies) != 0 {
		t.Fatal("unvalidated evidence displayed")
	}
}

func TestReadinessReaderProjectionHidesDiagnostics(t *testing.T) {
	s := testService(t)
	s.Collector = readinessCollector{profile: &CollectionProfile{Version: 1, Roots: []string{"/etc"}, DependencyEntryLimit: 10000, DependencyFileLimit: 1048576, DependencyReferenceLimit: 4096, DependencyScanComplete: true}, queries: map[string]string{"users": "failed"}, dependencies: []DependencyEvidence{{Path: "/opt/secret", Source: "etc/app.conf", State: "missing", Reason: "password=private-value"}}}
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	a := NewAuth(s.Store)
	a.CreateUser("reader", "long-test-password", "reader", false)
	token, _, _ := a.Login("reader", "long-test-password")
	req := httptest.NewRequest("GET", "/api/backups/"+b.ID+"/readiness", nil)
	req.Header.Set("X-Anker-Request", "1")
	req.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
	w := httptest.NewRecorder()
	Handler(s, a, false).ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "query.users") || strings.Contains(w.Body.String(), "private-value") || strings.Contains(w.Body.String(), "command_results") {
		t.Fatalf("response %d: %s", w.Code, w.Body.String())
	}
}

func TestReadinessShowsUnresolvedLinkTarget(t *testing.T) {
	s := testService(t)
	s.Collector = readinessLinkCollector{}
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.BackupReadiness(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range r.Dependencies {
		if d.Source == "etc/network/current" && d.Path == "/opt/external-target" && d.State == "missing" && d.Reason == "link_target_uncaptured" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing link evidence: %+v", r.Dependencies)
	}
}

func TestReadinessRequiresQueryPayloadBesideSuccessState(t *testing.T) {
	s := testService(t)
	s.Collector = readinessCollector{queries: map[string]string{"users": "ok", "groups": "ok", "file_inventory": "ok"}}
	b, err := s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.BackupReadiness(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"query.users", "query.groups", "query.file_inventory"} {
		if got := readinessState(r.Items, id); got != "unknown" {
			t.Fatalf("%s without payload=%s", id, got)
		}
	}
	s.Collector = readinessCollector{queries: map[string]string{"users": "ok", "groups": "ok", "file_inventory": "ok"}, payloads: true}
	b, err = s.CreateBackup(context.Background(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.BackupReadiness(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"query.users", "query.groups", "query.file_inventory"} {
		if got := readinessState(r.Items, id); got != "ok" {
			t.Fatalf("%s with payload=%s", id, got)
		}
	}
}
