package anker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTrackedSettingParsers(t *testing.T) {
	tests := []struct{ id, text, value, state string }{
		{"ipv4-forwarding", "# note\nnet.ipv4.ip_forward = 1 # enabled\n", "1", "value"},
		{"ipv4-forwarding", "-net/ipv4/ip_forward=0;note\n", "0", "value"},
		{"ipv4-forwarding", "net.ipv4.ip_forward=1\nnet.ipv4.ip_forward=0\n", "", "ambiguous"},
		{"ipv4-forwarding", "net.ipv4.ip_forward=invalid", "", "unsupported"},
		{"ipv4-forwarding", "net.ipv6.conf.all.forwarding=1", "", "not_recorded"},
		{"swappiness", "vm.swappiness=060", "60", "value"},
		{"swappiness", "vm.swappiness=201", "", "unsupported"},
		{"hostname", "pve-berlin-01\n", "pve-berlin-01", "value"},
		{"hostname", "first\nsecond\n", "", "ambiguous"},
		{"hostname", "bad value", "", "unsupported"},
		{"dns-servers", "# comment\nnameserver 192.0.2.1\nnameserver 2001:db8::1\nsearch example.org", "192.0.2.1 · 2001:db8::1", "value"},
		{"dns-servers", "nameserver untrusted.example", "", "unsupported"},
	}
	for _, test := range tests {
		t.Run(test.id+test.text, func(t *testing.T) {
			var setting TrackedSetting
			for _, s := range trackedSettings {
				if s.ID == test.id {
					setting = s
				}
			}
			v, state := parseTrackedSetting(setting, test.text)
			if v != test.value || state != test.state {
				t.Fatalf("%q %q, wanted %q %q", v, state, test.value, test.state)
			}
		})
	}
}
func historyRequest(t *testing.T, s *Service, query string) SettingHistory {
	t.Helper()
	w := comparisonRequest(t, s, "GET", "/api/hosts/host1/setting-history?"+query, nil, &User{Role: "reader"})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var h SettingHistory
	if err := json.Unmarshal(w.Body.Bytes(), &h); err != nil {
		t.Fatal(err)
	}
	return h
}
func TestSettingHistoryReadsSafeValuesAndPreservesEvidenceGaps(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": "net.ipv4.ip_forward=1\n"}, map[string]string{"etc/sysctl.d/a.conf": "net.ipv4.ip_forward=0\n"})
	h := historyRequest(t, s, "setting=ipv4-forwarding")
	if len(h.Observations) != 2 {
		t.Fatal(h)
	}
	values := map[string]string{}
	for _, o := range h.Observations {
		values[o.BackupID] = o.Value
		if o.State != "value" || len(o.Sources) != 1 {
			t.Fatal(o)
		}
	}
	if values[a.ID] != "1" || values[b.ID] != "0" {
		t.Fatal(values)
	}
	if err := os.WriteFile(filepath.Join(s.backupDir(a), "files/etc/sysctl.d/a.conf"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	h = historyRequest(t, s, "setting=ipv4-forwarding")
	for _, o := range h.Observations {
		if o.BackupID == a.ID && (o.State != "unavailable" || o.Value != "" || len(o.Sources) != 0) {
			t.Fatal("corruption became a value", o)
		}
	}
	b.VerificationError = "failed"
	if err := s.Store.Put("backups", b.ID, b); err != nil {
		t.Fatal(err)
	}
	h = historyRequest(t, s, "setting=ipv4-forwarding")
	for _, o := range h.Observations {
		if o.BackupID == b.ID && o.State != "unreliable" {
			t.Fatal(o)
		}
	}
}
func TestSettingHistoryConflictingProtectedMissingAndBudget(t *testing.T) {
	for _, test := range []struct {
		name  string
		files map[string]string
		state string
	}{
		{"conflict", map[string]string{"etc/sysctl.d/b.conf": "net.ipv4.ip_forward=1", "etc/sysctl.d/a.conf": "net.ipv4.ip_forward=0"}, "ambiguous"},
		{"same declarations", map[string]string{"etc/sysctl.d/b.conf": "net.ipv4.ip_forward=1", "etc/sysctl.d/a.conf": "net.ipv4.ip_forward=1"}, "value"},
		{"protected", map[string]string{"etc/sysctl.d/a.conf": "net.ipv4.ip_forward=1\npassword=do-not-leak"}, "protected"},
		{"missing declaration", map[string]string{"etc/sysctl.d/a.conf": "# none\n"}, "not_recorded"},
		{"missing file", map[string]string{"etc/hosts": "localhost"}, "not_recorded"},
		{"oversize", map[string]string{"etc/sysctl.d/a.conf": strings.Repeat("#", 65537)}, "limited"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := recoverySetup(t, test.files, Inventory{}, Inventory{})
			h := historyRequest(t, s, "setting=ipv4-forwarding")
			o := h.Observations[0]
			if o.State != test.state {
				t.Fatal(o)
			}
			if o.State != "value" && (o.Value != "" || len(o.Sources) != 0) {
				t.Fatal("non-value observation leaked evidence", o)
			}
		})
	}
}
func TestSettingHistoryPaginationNodeScopeAndInvalidDates(t *testing.T) {
	s, b := recoverySetup(t, map[string]string{"etc/hostname": "node-a\n"}, Inventory{}, Inventory{})
	// Catalog copies use distinct IDs only to test pagination; unreadable files must
	// remain observations instead of disappearing from chronology.
	for i := 0; i < 23; i++ {
		copy := b
		copy.ID = fmt.Sprintf("history-%02d", i)
		copy.CreatedAt = time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC).Format(time.RFC3339)
		if err := s.Store.Put("backups", copy.ID, copy); err != nil {
			t.Fatal(err)
		}
	}
	foreign := b
	foreign.ID = "foreign-history"
	foreign.HostID = "target"
	if err := s.Store.Put("backups", foreign.ID, foreign); err != nil {
		t.Fatal(err)
	}
	invalid := b
	invalid.ID = "invalid-history"
	invalid.CreatedAt = "bad-date"
	if err := s.Store.Put("backups", invalid.ID, invalid); err != nil {
		t.Fatal(err)
	}
	h := historyRequest(t, s, "setting=hostname")
	if len(h.Observations) != 20 || h.Next == "" || h.Undated != 1 {
		t.Fatal(h)
	}
	next := historyRequest(t, s, "setting=hostname&before="+h.Next)
	if len(next.Observations) != 4 || next.Next != "" {
		t.Fatal(next)
	}
	ids := map[string]bool{}
	for _, o := range append(h.Observations, next.Observations...) {
		if ids[o.BackupID] || o.BackupID == foreign.ID || o.BackupID == invalid.ID {
			t.Fatal(o)
		}
		ids[o.BackupID] = true
	}
	w := comparisonRequest(t, s, "GET", "/api/hosts/host1/setting-history?before="+foreign.ID, nil, &User{Role: "reader"})
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.settingHistory(ctx, "host1", trackedSettings[0], ""); err == nil {
		t.Fatal("cancelled traversal continued")
	}
}
func TestSettingHistoryAuthAndAllowlist(t *testing.T) {
	s := testService(t)
	for _, test := range []struct {
		method, url string
		user        User
		status      int
	}{
		{"GET", "/api/hosts/host1/setting-history?setting=../../etc/shadow", User{Role: "reader"}, 400},
		{"GET", "/api/hosts/missing/setting-history", User{Role: "reader"}, 404},
		{"POST", "/api/hosts/host1/setting-history", User{Role: "reader"}, 405},
		{"GET", "/api/hosts/host1/setting-history", User{}, 403},
	} {
		w := comparisonRequest(t, s, test.method, test.url, nil, &test.user)
		if w.Code != test.status {
			t.Fatalf("%s %s: %d %s", test.method, test.url, w.Code, w.Body.String())
		}
	}
}
func TestSettingHistoryFlagsEqualTimestamps(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/hostname": "one"}, map[string]string{"etc/hostname": "two"})
	b.CreatedAt = a.CreatedAt
	if err := s.Store.Put("backups", b.ID, b); err != nil {
		t.Fatal(err)
	}
	h := historyRequest(t, s, "setting=hostname")
	for _, o := range h.Observations {
		if !o.OrderUncertain {
			t.Fatal("invented ordering", o)
		}
	}
}

func TestSettingSnapshotReadsExactlyTheSelectedBackup(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/hostname": "before-node", "etc/sysctl.d/a.conf": "net.ipv4.ip_forward=1"}, map[string]string{"etc/hostname": "after-node", "etc/sysctl.d/a.conf": "net.ipv4.ip_forward=0"})
	for _, test := range []struct {
		b                 Backup
		hostname, forward string
	}{{a, "before-node", "1"}, {b, "after-node", "0"}} {
		w := comparisonRequest(t, s, "GET", "/api/backups/"+test.b.ID+"/setting-snapshot", nil, &User{Role: "reader"})
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var result struct {
			BackupID string `json:"backup_id"`
			Settings []struct {
				Setting     TrackedSetting     `json:"setting"`
				Observation SettingObservation `json:"observation"`
			} `json:"settings"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.BackupID != test.b.ID || len(result.Settings) != len(trackedSettings) {
			t.Fatal(result)
		}
		for _, item := range result.Settings {
			o := item.Observation
			if o.BackupID != test.b.ID {
				t.Fatal("mixed backup evidence", o)
			}
			if item.Setting.ID == "hostname" && o.Value != test.hostname {
				t.Fatal(o)
			}
			if item.Setting.ID == "ipv4-forwarding" && o.Value != test.forward {
				t.Fatal(o)
			}
		}
	}
}
func TestSettingSnapshotPreservesProtectedAndCorruptStates(t *testing.T) {
	s, a, b := comparisonPair(t, map[string]string{"etc/sysctl.d/a.conf": "net.ipv4.ip_forward=1\npassword=do-not-leak"}, map[string]string{"etc/sysctl.d/a.conf": "net.ipv4.ip_forward=0"})
	if err := os.WriteFile(filepath.Join(s.backupDir(b), "files/etc/sysctl.d/a.conf"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ id, state string }{{a.ID, "protected"}, {b.ID, "unavailable"}} {
		w := comparisonRequest(t, s, "GET", "/api/backups/"+test.id+"/setting-snapshot", nil, &User{Role: "reader", Secrets: true})
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"`+test.state+`"`) || strings.Contains(w.Body.String(), "do-not-leak") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, test := range []struct {
		method, id string
		user       User
		status     int
	}{{"POST", a.ID, User{Role: "reader"}, 405}, {"GET", "missing", User{Role: "reader"}, 404}, {"GET", a.ID, User{}, 403}} {
		w := comparisonRequest(t, s, test.method, "/api/backups/"+test.id+"/setting-snapshot", nil, &test.user)
		if w.Code != test.status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
