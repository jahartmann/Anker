package anker

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func restorePreviewRequest(t *testing.T, s *Service, source, baseline Backup, expectedSource, expectedBaseline string) *httptest.ResponseRecorder {
	t.Helper()
	query := url.Values{"baseline": {baseline.ID}, "setting": {"ipv4-forwarding"}, "expected_source": {expectedSource}, "expected_baseline": {expectedBaseline}}
	return comparisonRequest(t, s, "GET", "/api/backups/"+source.ID+"/change-restore-preview?"+query.Encode(), nil, &User{Role: "reader"})
}
func TestChangeRestorePreviewReversesValuesAndIncludesWholeFileCollateral(t *testing.T) {
	old := "net.ipv4.ip_forward=1\nvm.swappiness=60\ncustom.setting=old\n"
	recent := "net.ipv4.ip_forward=0\nvm.swappiness=10\ncustom.setting=new\n"
	s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": old}, map[string]string{"etc/sysctl.d/a.conf": recent})
	plansBefore, _ := records[Plan](s.Store, "plans")
	jobsBefore, _ := s.Jobs()
	for range 2 {
		w := restorePreviewRequest(t, s, a, b, "1", "0")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var preview ChangeRestorePreview
		if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if preview.Source.ID != a.ID || preview.Baseline.ID != b.ID || preview.Basis != "saved-backup" || preview.Selected.Before.Value != "0" || preview.Selected.After.Value != "1" {
			t.Fatal(preview)
		}
		if len(preview.Additional) != 1 || preview.Additional[0].Setting.ID != "swappiness" || preview.Additional[0].Before.Value != "10" || preview.Additional[0].After.Value != "60" {
			t.Fatal(preview.Additional)
		}
		if len(preview.Files) != 1 || preview.Files[0].Action != "replace" {
			t.Fatal(preview.Files)
		}
		raw, _ := json.Marshal(preview.Files[0].Comparison)
		if !strings.Contains(string(raw), "custom.setting=old") || !strings.Contains(string(raw), "vm.swappiness=60") {
			t.Fatal(string(raw))
		}
	}
	plansAfter, _ := records[Plan](s.Store, "plans")
	jobsAfter, _ := s.Jobs()
	if len(plansBefore) != len(plansAfter) || len(jobsBefore) != len(jobsAfter) {
		t.Fatal("preview created an operation")
	}
	for _, test := range []struct {
		b    Backup
		text string
	}{{a, old}, {b, recent}} {
		content, _, err := s.ReadFile(test.b.ID, "etc/sysctl.d/a.conf")
		if err != nil || string(content) != test.text {
			t.Fatal("preview changed files", err)
		}
	}
}
func TestChangeRestorePreviewTracksMovedDeclarationsWithoutHidingDeletion(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/old.conf": "net.ipv4.ip_forward=1"}, map[string]string{"etc/sysctl.d/new.conf": "net.ipv4.ip_forward=0"})
	w := restorePreviewRequest(t, s, a, b, "1", "0")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var preview ChangeRestorePreview
	json.Unmarshal(w.Body.Bytes(), &preview)
	actions := map[string]string{}
	for _, f := range preview.Files {
		actions[f.Path] = f.Action
	}
	if actions["etc/sysctl.d/old.conf"] != "create" || actions["etc/sysctl.d/new.conf"] != "remove" {
		t.Fatal(actions)
	}
}
func TestChangeRestorePreviewRejectsStaleCorruptProtectedOrCrossNodeEvidence(t *testing.T) {
	for _, scenario := range []string{"stale", "corrupt", "protected", "partial", "cross-node", "reverse", "equal-time"} {
		t.Run(scenario, func(t *testing.T) {
			s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": "net.ipv4.ip_forward=1"}, map[string]string{"etc/sysctl.d/a.conf": "net.ipv4.ip_forward=0"})
			desired, observed := "1", "0"
			code := 409
			switch scenario {
			case "stale":
				desired = "0"
			case "corrupt":
				os.WriteFile(filepath.Join(s.backupDir(a), "files/etc/sysctl.d/a.conf"), []byte("corrupt"), 0600)
			case "protected":
				comparisonManifestFixture(t, s, a, func(e *Entry) { e.Secret = true })
			case "partial":
				b.Status = "partial"
				s.Store.Put("backups", b.ID, b)
			case "cross-node":
				b.HostID = "target"
				s.Store.Put("backups", b.ID, b)
				code = 400
			case "reverse":
				a, b = b, a
				desired, observed = "0", "1"
			case "equal-time":
				b.CreatedAt = a.CreatedAt
				s.Store.Put("backups", b.ID, b)
			}
			w := restorePreviewRequest(t, s, a, b, desired, observed)
			if w.Code != code {
				t.Fatal(w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), `"files"`) {
				t.Fatal("partial result escaped", w.Body.String())
			}
		})
	}
}
func TestChangeRestorePreviewRetainsReadAuthorizationAndRejectsWrites(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": "net.ipv4.ip_forward=1"}, map[string]string{"etc/sysctl.d/a.conf": "net.ipv4.ip_forward=0"})
	endpoint := "/api/backups/" + a.ID + "/change-restore-preview?baseline=" + b.ID + "&setting=ipv4-forwarding&expected_source=1&expected_baseline=0"
	for _, test := range []struct {
		method string
		user   User
		code   int
	}{{"GET", User{}, 403}, {"POST", User{Role: "admin"}, 405}, {"GET", User{Role: "reader", Secrets: false}, 200}} {
		w := comparisonRequest(t, s, test.method, endpoint, nil, &test.user)
		if w.Code != test.code {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
