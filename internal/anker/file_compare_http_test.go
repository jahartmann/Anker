package anker

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func comparisonRequest(t *testing.T, s *Service, method, path string, body any, user *User) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	r.Header.Set("X-Anker-Request", "1")
	w := httptest.NewRecorder()
	if user == nil {
		Handler(s, NewAuth(s.Store), true).ServeHTTP(w, r)
	} else if err := handleAPI(s, NewAuth(s.Store), *user, w, r); err != nil {
		jsonError(w, err)
	}
	return w
}
func comparisonPair(t *testing.T, before, after map[string]string) (*Service, Backup, Backup) {
	t.Helper()
	s, a := recoverySetup(t, before, Inventory{}, Inventory{})
	s.Collector = recoveryFixture{content: after}
	b, err := s.CreateBackup(t.Context(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	return s, a, b
}
func comparisonJSON(t *testing.T, s *Service, a, b Backup, path string) map[string]any {
	t.Helper()
	w := comparisonRequest(t, s, "POST", "/api/backups/compare/file", map[string]any{"from": a.ID, "to": b.ID, "path": path}, nil)
	if w.Code != 200 {
		t.Fatalf("comparison %d: %s", w.Code, w.Body.String())
	}
	var v map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestComparisonShowsContextAndSeparateMetadata(t *testing.T) {
	before := strings.Repeat("same\n", 100) + "old\n" + strings.Repeat("same\n", 100)
	after := strings.Repeat("same\n", 100) + "new\n" + strings.Repeat("same\n", 100)
	s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": before}, map[string]string{"etc/sysctl.d/a.conf": after})
	v := comparisonJSON(t, s, a, b, "etc/sysctl.d/a.conf")
	h := v["hunks"].([]any)
	if len(h) != 1 {
		t.Fatal(v)
	}
	lines := h[0].(map[string]any)["lines"].([]any)
	if len(lines) != 8 {
		t.Fatalf("want 6 context + 2 edits: %v", v)
	}
}
func TestComparisonNeverTreatsReadFailureAsMissing(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": "old"}, map[string]string{"etc/sysctl.d/a.conf": "new"})
	if err := os.Remove(filepath.Join(s.backupDir(a), "files/etc/sysctl.d/a.conf")); err != nil {
		t.Fatal(err)
	}
	v := comparisonJSON(t, s, a, b, "etc/sysctl.d/a.conf")
	if v["state"] != "unavailable" {
		t.Fatal(v)
	}
	w := comparisonRequest(t, s, "POST", "/api/backups/diff", map[string]string{"from": a.ID, "to": b.ID, "path": "etc/sysctl.d/a.conf"}, nil)
	if w.Code == 200 {
		t.Fatal("legacy diff swallowed missing physical file", w.Body.String())
	}
}
func TestComparisonMasksEitherSideSecret(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		before, after := "password=do-not-leak\n", "safe\n"
		if reverse {
			before, after = after, before
		}
		s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": before}, map[string]string{"etc/sysctl.d/a.conf": after})
		v := comparisonJSON(t, s, a, b, "etc/sysctl.d/a.conf")
		if v["state"] != "masked" || v["masked"] != true {
			t.Fatal(v)
		}
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), "do-not-leak") {
			t.Fatal(string(raw))
		}
	}
}
func TestComparisonLimitsLongAndBinaryFiles(t *testing.T) {
	for _, tc := range []struct{ data, state string }{{strings.Repeat("x", (1<<20)+1), "too_large"}, {strings.Repeat("x\n", 10001), "too_large"}, {"abc\x00def", "binary"}} {
		s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": "old"}, map[string]string{"etc/sysctl.d/a.conf": tc.data})
		v := comparisonJSON(t, s, a, b, "etc/sysctl.d/a.conf")
		if v["state"] != tc.state {
			t.Fatal(v)
		}
	}
}
func TestCompareSummaryDoesNotLoadFileContent(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": "old"}, map[string]string{"etc/sysctl.d/a.conf": "new"})
	os.Remove(filepath.Join(s.backupDir(a), "files/etc/sysctl.d/a.conf"))
	os.Remove(filepath.Join(s.backupDir(b), "files/etc/sysctl.d/a.conf"))
	w := comparisonRequest(t, s, "POST", "/api/backups/compare", map[string]string{"from": a.ID, "to": b.ID}, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "etc/sysctl.d/a.conf") {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"hunks":`) || strings.Contains(w.Body.String(), `"content":`) {
		t.Fatal(w.Body.String())
	}
}
func TestPlanComparisonRequiresRestoreRole(t *testing.T) {
	s, _, r := snapshotSetup(t)
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	w := comparisonRequest(t, s, "GET", "/api/plans/"+p.ID+"/comparison?path="+r.Files[0]+"&basis=target", nil, &User{ID: "reader", Role: "reader", Secrets: true})
	if w.Code != http.StatusForbidden {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestRecoveryListsNeverExposeStorageCredentials(t *testing.T) {
	s, _, r := snapshotSetup(t)
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	p.Source.Details = map[string]json.RawMessage{"storage_config": json.RawMessage(`"password super-sensitive"`)}
	p.Target.Details = p.Source.Details
	p.Steps[0].Effects = []RecoveryEffect{{Path: r.Files[0], Field: "password", Before: "super-sensitive", After: "replacement"}}
	p.Tasks = []RecoveryTask{{ID: "legacy", Instruction: "password super-sensitive", Evidence: "token super-sensitive"}}
	if err = s.Store.Put("plans", p.ID, p); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/status", "/api/plans", "/api/plans/" + p.ID} {
		w := comparisonRequest(t, s, "GET", path, nil, nil)
		if w.Code != 200 || strings.Contains(w.Body.String(), "super-sensitive") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
}

func TestPlanComparisonBasesAndTargetSecrets(t *testing.T) {
	for _, secret := range []bool{false, true} {
		s, _, r := snapshotSetup(t)
		if secret {
			c := s.Collector.(snapshotCollector)
			f := &c.result.Files[0]
			f.Content = []byte("password target-only-secret\n")
			f.SHA256 = Hash(f.Content)
			f.Metadata.SHA256 = f.SHA256
			f.Metadata.Size = int64(len(f.Content))
			hashes := fileHashes(c.inv)
			hashes[r.Files[0]] = f.SHA256
			c.inv.Details["file_hashes"], _ = json.Marshal(hashes)
			s.Collector = c
		}
		p, err := s.CreatePlan(t.Context(), r)
		if err != nil {
			t.Fatal(err)
		}
		for _, basis := range []string{"target", "source", "source-target"} {
			path := "/api/plans/" + p.ID + "/comparison?path=" + r.Files[0] + "&basis=" + basis
			w := comparisonRequest(t, s, "GET", path, nil, &User{ID: "restore-user", Role: "restore"})
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var v map[string]any
			json.Unmarshal(w.Body.Bytes(), &v)
			if secret {
				if v["masked"] != true || strings.Contains(w.Body.String(), "target-only-secret") {
					t.Fatal(w.Body.String())
				}
			} else if basis == "source" && v["state"] != "identical" {
				t.Fatal(w.Body.String())
			}
			w = comparisonRequest(t, s, "GET", path+"&reveal=1", nil, &User{ID: "restore-user", Role: "restore"})
			if w.Code != 403 {
				t.Fatal("reveal without rights", w.Code)
			}
			w = comparisonRequest(t, s, "GET", path+"&reveal=1", nil, &User{ID: "restore-user", Role: "restore", Secrets: true})
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if secret && basis != "source" && !strings.Contains(w.Body.String(), "target-only-secret") {
				t.Fatal("authorized reveal missing", w.Body.String())
			}
		}
		w := comparisonRequest(t, s, "GET", "/api/plans/"+p.ID+"/download", nil, &User{Role: "restore"})
		if w.Code != 403 {
			t.Fatal("full export lost unconditional secrets gate", w.Code)
		}
	}
}
func TestRecoveryConfirmationBindsAuthenticatedActor(t *testing.T) {
	s, _, r := snapshotSetup(t)
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	p.Blockers = nil
	p.Tasks = []RecoveryTask{{ID: "review", Kind: "manual-check", Title: "Review", State: "pending"}}
	if err = s.savePlan(p); err != nil {
		t.Fatal(err)
	}
	url := "/api/plans/" + p.ID + "/checks/review"
	w := comparisonRequest(t, s, "POST", url, map[string]string{"evidence": "checked"}, &User{ID: "reader", Role: "reader"})
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = comparisonRequest(t, s, "POST", url, map[string]string{"evidence": "checked", "user_id": "impostor"}, &User{ID: "trusted", Role: "restore"})
	if w.Code != 400 {
		t.Fatal("body identity accepted", w.Code, w.Body.String())
	}
	w = comparisonRequest(t, s, "POST", url, map[string]string{"evidence": "checked"}, &User{ID: "trusted", Role: "restore"})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	p, err = s.Plan(p.ID)
	if err != nil || p.Tasks[0].ConfirmedBy != "trusted" || p.Tasks[0].State != "user_confirmed" {
		t.Fatal(p.Tasks, err)
	}
}
func TestRecoveryStatusAndInspectionSanitizeInventoryAndErrors(t *testing.T) {
	s, _, r := snapshotSetup(t)
	h, _ := s.Host("host1")
	h.Inventory = &Inventory{Details: map[string]json.RawMessage{"storage_config": json.RawMessage(`"password secret-from-storage"`)}}
	h.ProbeError = "remote failed: https://user:secret-from-storage@example.invalid/"
	if err := s.Store.Put("hosts", h.ID, h); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/api/status", "/api/hosts", "/api/hosts/host1"} {
		w := comparisonRequest(t, s, "GET", route, nil, nil)
		if w.Code != 200 || strings.Contains(w.Body.String(), "secret-from-storage") {
			t.Fatal(route, w.Code, w.Body.String())
		}
	}
	c := s.Collector.(snapshotCollector)
	c.inv.Details["storage_config"] = json.RawMessage(`"password secret-from-storage"`)
	s.Collector = c
	w := comparisonRequest(t, s, "POST", "/api/plans/inspect", r, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-from-storage") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestComparisonReaderSessionAndExplicitReveal(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/hosts": "password=classified"}, map[string]string{"etc/hosts": "safe"})
	auth := NewAuth(s.Store)
	if err := auth.CreateUser("reader", "long-test-password", "reader", false); err != nil {
		t.Fatal(err)
	}
	token, _, err := auth.Login("reader", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"compare", "compare/file"} {
		raw, _ := json.Marshal(map[string]string{"from": a.ID, "to": b.ID, "path": "etc/hosts"})
		r := httptest.NewRequest("POST", "/api/backups/"+route, strings.NewReader(string(raw)))
		r.Header.Set("X-Anker-Request", "1")
		r.AddCookie(&http.Cookie{Name: "anker_session", Value: token})
		w := httptest.NewRecorder()
		Handler(s, auth, false).ServeHTTP(w, r)
		if w.Code != 200 || strings.Contains(w.Body.String(), "classified") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := comparisonRequest(t, s, "POST", "/api/backups/compare/file", map[string]any{"from": a.ID, "to": b.ID, "path": "etc/hosts", "reveal": true}, &User{ID: "reader", Role: "reader"})
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = comparisonRequest(t, s, "POST", "/api/backups/compare/file", map[string]any{"from": a.ID, "to": b.ID, "path": "etc/hosts", "reveal": true}, &User{ID: "reader", Role: "reader", Secrets: true})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "classified") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestRecoveryRedactionPreservesStorageContentKinds(t *testing.T) {
	v := redactRecovery(RecoveryInspection{TargetStorage: []RecoveryStorage{{ID: "local", Content: []string{"images", "rootdir"}}}})
	if len(v.TargetStorage) != 1 || len(v.TargetStorage[0].Content) != 2 {
		t.Fatal("storage capability lost", v.TargetStorage)
	}
}
func TestPlanComparisonPreparedTooLarge(t *testing.T) {
	s, _, r := snapshotSetup(t)
	target := s.Collector
	s.Collector = recoveryFixture{content: map[string]string{r.Files[0]: strings.Repeat("x", (1<<20)+1)}, source: Inventory{PVEVersion: "8.4"}}
	b, err := s.CreateBackup(t.Context(), "host1")
	if err != nil {
		t.Fatal(err)
	}
	r.BackupID = b.ID
	s.Collector = target
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	w := comparisonRequest(t, s, "GET", "/api/plans/"+p.ID+"/comparison?path="+r.Files[0]+"&basis=target", nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"too_large"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestPlanComparisonUnreadableHiddenSideFailsClosed(t *testing.T) {
	s, _, r := snapshotSetup(t)
	c := s.Collector.(snapshotCollector)
	c.result.Files[0] = RecoveryReadFile{Path: r.Files[0], State: "unreadable"}
	s.Collector = c
	p, err := s.CreatePlan(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	w := comparisonRequest(t, s, "GET", "/api/plans/"+p.ID+"/comparison?path="+r.Files[0]+"&basis=source", nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"unavailable"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

// Produce internally consistent persisted manifest fixtures with metadata that
// the content-only collector cannot set. Bodies stay exactly as collected.
func comparisonManifestFixture(t *testing.T, s *Service, b Backup, edit func(*Entry)) {
	t.Helper()
	m, err := s.Manifest(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range m.Entries {
		edit(&m.Entries[i])
	}
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
func TestComparisonXAttrHTTPRevealGate(t *testing.T) {
	for _, value := range []string{base64.StdEncoding.EncodeToString([]byte("password=classified")), "invalid-base64!"} {
		for _, left := range []bool{true, false} {
			s, a, b := comparisonPair(t, map[string]string{"etc/hosts": "old"}, map[string]string{"etc/hosts": "new"})
			selected := b
			if left {
				selected = a
			}
			comparisonManifestFixture(t, s, selected, func(e *Entry) { e.XAttrs = map[string]string{"user.comment": value} })
			body := map[string]any{"from": a.ID, "to": b.ID, "path": "etc/hosts"}
			w := comparisonRequest(t, s, "POST", "/api/backups/compare/file", body, &User{Role: "reader", Secrets: true})
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"masked":true`) || strings.Contains(w.Body.String(), value) {
				t.Fatal("default xattr exposure", w.Code, w.Body.String())
			}
			body["reveal"] = true
			w = comparisonRequest(t, s, "POST", "/api/backups/compare/file", body, &User{Role: "reader"})
			if w.Code != 403 {
				t.Fatal(w.Code, w.Body.String())
			}
			w = comparisonRequest(t, s, "POST", "/api/backups/compare/file", body, &User{Role: "reader", Secrets: true})
			if w.Code != 200 || !strings.Contains(w.Body.String(), value) || !strings.Contains(w.Body.String(), `"field":"xattrs"`) {
				t.Fatal(w.Code, w.Body.String())
			}
		}
	}
}
func TestCompareSummaryClassifiesWithoutBodies(t *testing.T) {
	before := map[string]string{}
	after := map[string]string{}
	for _, name := range []string{"content", "metadata", "mtime", "mixed", "unchanged"} {
		before["etc/sysctl.d/"+name] = "old"
		after["etc/sysctl.d/"+name] = "old"
	}
	after["etc/sysctl.d/content"] = "new"
	after["etc/sysctl.d/mixed"] = "new"
	before["etc/sysctl.d/removed"] = ""
	after["etc/sysctl.d/added"] = ""
	s, a, b := comparisonPair(t, before, after)
	comparisonManifestFixture(t, s, b, func(e *Entry) {
		switch e.Path {
		case "etc/sysctl.d/metadata":
			e.Mode = 0644
		case "etc/sysctl.d/mtime":
			e.MTime = 123
		case "etc/sysctl.d/mixed":
			e.Mode = 0644
			e.MTime = 123
		}
	})
	for _, backup := range []Backup{a, b} {
		if err := os.RemoveAll(filepath.Join(s.backupDir(backup), "files")); err != nil {
			t.Fatal(err)
		}
	}
	w := comparisonRequest(t, s, "POST", "/api/backups/compare", map[string]string{"from": a.ID, "to": b.ID}, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		change                         string
		content, metadata, mtime, only bool
	}{
		"content": {"changed", true, false, false, false}, "metadata": {"changed", false, true, false, false}, "mtime": {"changed", false, false, true, true}, "mixed": {"changed", true, true, true, false}, "added": {"added", true, true, false, false}, "removed": {"removed", true, true, false, false},
	}
	if len(rows) != len(want) {
		t.Fatal(rows)
	}
	for _, row := range rows {
		name := strings.TrimPrefix(row["path"].(string), "etc/sysctl.d/")
		expect, ok := want[name]
		if !ok || row["change"] != expect.change || row["content_changed"] != expect.content || row["metadata_changed"] != expect.metadata || row["mtime_changed"] != expect.mtime || row["mtime_only"] != expect.only {
			t.Fatalf("classification %s: %+v", name, row)
		}
		for _, key := range []string{"diff", "content", "hunks", "metadata_changes", "before", "after"} {
			if _, ok := row[key]; ok {
				t.Fatalf("summary value leak %s: %+v", key, row)
			}
		}
	}
}
func TestRecoveryRedactionMasksEncodedXAttrCredentials(t *testing.T) {
	value := base64.StdEncoding.EncodeToString([]byte("password=classified"))
	v := redactRecovery(RecoveryInspection{Files: []RecoveryReadFile{{Path: "etc/hosts", State: "read", Metadata: &Entry{Path: "etc/hosts", XAttrs: map[string]string{"user.comment": value}}}}})
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), value) {
		t.Fatal(string(raw))
	}
}
func TestPlanComparisonMasksDecodedTargetXAttrs(t *testing.T) {
	for _, value := range []string{base64.StdEncoding.EncodeToString([]byte("password=classified")), "invalid-base64!"} {
		s, _, r := snapshotSetup(t)
		c := s.Collector.(snapshotCollector)
		c.result.Files[0].Metadata.XAttrs = map[string]string{"user.comment": value}
		c.inv.Details["file_metadata"], _ = json.Marshal(map[string]*Entry{r.Files[0]: c.result.Files[0].Metadata})
		s.Collector = c
		p, err := s.CreatePlan(t.Context(), r)
		if value == "invalid-base64!" {
			if err == nil {
				t.Fatal("invalid target xattr evidence accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		path := "/api/plans/" + p.ID + "/comparison?path=" + r.Files[0] + "&basis=source"
		w := comparisonRequest(t, s, "GET", path, nil, &User{Role: "restore", Secrets: true})
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"masked":true`) || strings.Contains(w.Body.String(), value) {
			t.Fatal("hidden target xattr escaped", w.Code, w.Body.String())
		}
		path = "/api/plans/" + p.ID + "/comparison?path=" + r.Files[0] + "&basis=target&reveal=1"
		w = comparisonRequest(t, s, "GET", path, nil, &User{Role: "restore", Secrets: true})
		if w.Code != 200 || !strings.Contains(w.Body.String(), value) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
