package anker

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copyFixture(t *testing.T, s *Service, b Backup) string {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir(s.backupDir(b), func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(s.backupDir(b), p)
		target := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(target, data, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func verifyCopyRequest(t *testing.T, s *Service, id, root string, local bool) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"copy_root": root})
	r := httptest.NewRequest("POST", "/api/local-copy-verification/"+id, strings.NewReader(string(data)))
	r.Header.Set("X-Anker-Request", "1")
	w := httptest.NewRecorder()
	Handler(s, NewAuth(s.Store), local).ServeHTTP(w, r)
	return w
}
func TestIndependentCopyChecksActualBytesAndBindsReceipt(t *testing.T) {
	s := testService(t)
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	root := copyFixture(t, s, b)
	response := verifyCopyRequest(t, s, b.ID, root, true)
	if response.Code != 200 {
		t.Fatal("valid actual copy rejected", response.Code, response.Body.String())
	}
	var receipt map[string]any
	json.Unmarshal(response.Body.Bytes(), &receipt)
	if receipt["backup_id"] != b.ID || receipt["manifest_sha"] != b.ManifestSHA || receipt["verified_at"] == "" || receipt["provenance"] != "local-byte-verifier-v1" || receipt["failure_domain"] != "unknown" {
		t.Fatal("unbound or misleading receipt", receipt)
	}
	r, e := s.BackupReadiness(b.ID)
	if e != nil || readinessState(r.Items, "independent_copy") != "ok" {
		t.Fatal("trusted receipt not integrated", r, e)
	}
	bytes, _ := json.Marshal(r)
	if strings.Contains(string(bytes), root) {
		t.Fatal("private copy root leaked")
	}
	// A new valid catalog/manifest pair must invalidate the old receipt.
	manifestPath := filepath.Join(s.backupDir(b), "manifest.json")
	data, _ := os.ReadFile(manifestPath)
	data = append(data, '\n')
	os.WriteFile(manifestPath, data, 0600)
	b, _ = s.Backup(b.ID)
	b.ManifestSHA = Hash(data)
	s.Store.Put("backups", b.ID, b)
	r, e = s.BackupReadiness(b.ID)
	if e != nil || readinessState(r.Items, "independent_copy") != "stale" {
		t.Fatal("old receipt accepted for changed manifest", r, e)
	}
}
func TestIndependentCopyRejectsAliasesMissingTamperedAndLinkedData(t *testing.T) {
	for _, kind := range []string{"same", "nested", "manifest", "missing", "bytes", "root-symlink", "root-symlink-slash", "entry-symlink", "hardlink", "renamed-hardlink"} {
		t.Run(kind, func(t *testing.T) {
			s := testService(t)
			b, e := s.CreateBackup(context.Background(), "host1")
			if e != nil {
				t.Fatal(e)
			}
			root := copyFixture(t, s, b)
			p := filepath.Join(root, "files/etc/network/interfaces")
			source := filepath.Join(s.backupDir(b), "files/etc/network/interfaces")
			switch kind {
			case "same":
				root = s.backupDir(b)
			case "nested":
				root = filepath.Join(s.backupDir(b), "copy")
				os.Mkdir(root, 0700)
			case "manifest":
				os.WriteFile(filepath.Join(root, "manifest.json"), []byte("{}"), 0600)
			case "missing":
				os.Remove(p)
			case "bytes":
				os.WriteFile(p, []byte("corrupt"), 0600)
			case "root-symlink", "root-symlink-slash":
				alias := filepath.Join(t.TempDir(), "alias")
				os.Symlink(root, alias)
				root = alias
				if kind == "root-symlink-slash" {
					root += "/"
				}
			case "entry-symlink":
				os.Remove(p)
				os.Symlink(source, p)
			case "hardlink":
				os.Remove(p)
				os.Link(source, p)
			case "renamed-hardlink":
				os.Link(source, filepath.Join(root, "unlisted"))
			}
			response := verifyCopyRequest(t, s, b.ID, root, true)
			if response.Code == 200 {
				t.Fatal("unsafe copy accepted", kind)
			}
			b2, _ := s.Backup(b.ID)
			if b2.Status != "successful" || b2.ManifestSHA != b.ManifestSHA {
				t.Fatal("copy failure changed source", b2)
			}
			r, e := s.BackupReadiness(b.ID)
			if e != nil || readinessState(r.Items, "independent_copy") == "ok" {
				t.Fatal("failed copy claimed verified", r, e)
			}
		})
	}
}
func TestIndependentCopyWebCannotSelectPath(t *testing.T) {
	s := testService(t)
	w := verifyCopyRequest(t, s, "anything", "/private/copy", false)
	if w.Code != 404 {
		t.Fatal("local path route exposed over web", w.Code)
	}
}

func TestIndependentCopyFailureReplacesOldReceiptAndPreservesArchivedSource(t *testing.T) {
	s := testService(t)
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	root := copyFixture(t, s, b)
	if e = s.ArchiveBackup(b.ID); e != nil {
		t.Fatal(e)
	}
	if w := verifyCopyRequest(t, s, b.ID, root, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	after, _ := s.Backup(b.ID)
	if !after.Archived {
		t.Fatal("copy verifier expanded source")
	}
	if _, e = os.Stat(filepath.Join(s.backupDir(b), "files")); !os.IsNotExist(e) {
		t.Fatal("source staging mutated", e)
	}
	os.Remove(filepath.Join(root, "WIEDERHERSTELLUNG.md"))
	if w := verifyCopyRequest(t, s, b.ID, root, true); w.Code == 200 {
		t.Fatal("missing artifact accepted")
	}
	r, e := s.BackupReadiness(b.ID)
	if e != nil || readinessState(r.Items, "independent_copy") != "failed" {
		t.Fatal("old success survived failed recheck", r, e)
	}
}
func TestIndependentCopyHonorsMaintenanceAndRejectsCallerProof(t *testing.T) {
	s := testService(t)
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	root := copyFixture(t, s, b)
	if e = s.PrepareUpdate(); e != nil {
		t.Fatal(e)
	}
	if w := verifyCopyRequest(t, s, b.ID, root, true); w.Code != 503 {
		t.Fatal("copy admitted during maintenance", w.Code)
	}
	s.ReleaseUpdate()
	r := httptest.NewRequest("POST", "/api/local-copy-verification/"+b.ID, strings.NewReader(`{"copy_root":"/somewhere","verified_at":"2030-01-01","state":"ok"}`))
	r.Header.Set("X-Anker-Request", "1")
	w := httptest.NewRecorder()
	Handler(s, NewAuth(s.Store), true).ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatal("caller proof accepted")
	}
}

func TestIndependentCopyReceiptSurvivesReindexOfSameManifest(t *testing.T) {
	s := testService(t)
	b, e := s.CreateBackup(context.Background(), "host1")
	if e != nil {
		t.Fatal(e)
	}
	root := copyFixture(t, s, b)
	if w := verifyCopyRequest(t, s, b.ID, root, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, e = s.Reindex(); e != nil {
		t.Fatal(e)
	}
	r, e := s.BackupReadiness(b.ID)
	if e != nil || readinessState(r.Items, "independent_copy") != "ok" {
		t.Fatal("same-manifest reindex lost trusted copy receipt", r, e)
	}
}
