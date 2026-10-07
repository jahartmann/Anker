package tui

import (
	"anker/internal/client"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const enrollmentFingerprint = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const enrollmentPassword = "never-show-bootstrap-password"

func enrollmentFixture(t *testing.T, changed, fail bool) (Model, *[]map[string]any) {
	t.Helper()
	requests := []map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		json.NewDecoder(r.Body).Decode(&body)
		body["request_path"] = r.URL.Path
		encoded, _ := json.Marshal(body)
		recorded := map[string]any{}
		json.Unmarshal(encoded, &recorded)
		requests = append(requests, recorded)
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]any{"error": "remote banner " + enrollmentPassword})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/inspect") {
			json.NewEncoder(w).Encode(map[string]any{"address": "192.0.2.42", "port": 22, "fingerprint": enrollmentFingerprint, "key_type": "ssh-ed25519", "known": changed, "changed": changed})
		} else {
			host := object(body["host"])
			if str(host, "id") == "" {
				host["id"] = "connected"
			}
			host["inventory"] = map[string]any{"hostname": "remote"}
			json.NewEncoder(w).Encode(host)
		}
	}))
	t.Cleanup(server.Close)
	return New(&client.Client{HTTP: server.Client(), Base: server.URL}), &requests
}

func enrollmentDraft(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = key(m, "a")
	if m.form == nil || m.form.kind != "hostConnect" {
		t.Fatal("add did not open automatic SSH onboarding")
	}
	m.form.fields[0].value = "192.0.2.42"
	m.form.fields[2].value = enrollmentPassword
	return m
}

func TestHostEnrollmentInspectsWithoutPasswordAndRequiresFingerprintApproval(t *testing.T) {
	m, requests := enrollmentFixture(t, false, false)
	m = enrollmentDraft(t, m)
	if m.form.fields[1].value != "root" || !m.form.fields[2].secret || strings.Contains(m.View(), enrollmentPassword) {
		t.Fatal("bootstrap defaults or password masking missing")
	}
	m, inspect := key(m, "ctrl+s")
	if inspect == nil || !m.busy || m.pending != nil {
		t.Fatal("inspection must run before approval")
	}
	m, duplicate := key(m, "ctrl+s")
	if duplicate != nil {
		t.Fatal("busy inspection submitted twice")
	}
	m = complete(t, m, inspect)
	if len(*requests) != 1 || (*requests)[0]["request_path"] != "/api/hosts/connection/inspect" || len((*requests)[0]) != 3 {
		t.Fatalf("inspection exposed bootstrap credentials: %#v", *requests)
	}
	if m.pending == nil || !strings.Contains(m.View(), enrollmentFingerprint) || !strings.Contains(m.View(), "root") || !strings.Contains(m.View(), "Konsole") || strings.Contains(m.View(), enrollmentPassword) {
		t.Fatal("missing safe fingerprint confirmation")
	}
	if strings.Contains(describe(m.pending.input), enrollmentPassword) {
		t.Fatal("confirmation stores password")
	}
	m, enroll := key(m, "y")
	m = complete(t, m, enroll)
	if len(*requests) != 2 {
		t.Fatal("enrollment request missing")
	}
	input := (*requests)[1]
	host := object(input["host"])
	if str(host, "name") != "" {
		t.Fatal("blank optional name must use the verified remote hostname")
	}
	if input["request_path"] != "/api/hosts/connection/enroll" || input["password"] != enrollmentPassword || input["username"] != "root" || input["confirmed"] != true || input["fingerprint"] != enrollmentFingerprint {
		t.Fatalf("enrollment protocol incorrect: %#v", input)
	}
	if str(host, "ssh_user") != "anker" || str(host, "restore_ssh_user") != "anker-restore" || str(host, "key_path") != "/etc/anker/keys/backup" || str(host, "restore_key_path") != "/etc/anker/keys/restore" || host["username"] != nil || host["password"] != nil {
		t.Fatal("bootstrap credentials entered persistent host")
	}
	if m.form != nil || strings.Contains(m.View(), enrollmentPassword) {
		t.Fatal("completed enrollment retained password")
	}
}

func TestHostEnrollmentChangedIdentityAndFailureClearSecret(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		changed, fail bool
	}{{"changed", true, false}, {"failure", false, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			m, requests := enrollmentFixture(t, scenario.changed, scenario.fail)
			m = enrollmentDraft(t, m)
			m, inspect := key(m, "ctrl+s")
			next, _ := m.Update(inspect())
			m = next.(Model)
			if m.pending != nil || m.err == "" || len(*requests) != 1 {
				t.Fatal("unsafe inspection offered enrollment")
			}
			if m.form.fields[2].value != "" || strings.Contains(m.View(), enrollmentPassword) {
				t.Fatal("failed inspection retained or revealed password")
			}
		})
	}
}

func TestHostEnrollmentCancellationStaleResponseAndDraftSurviveRefresh(t *testing.T) {
	m, _ := enrollmentFixture(t, false, false)
	m = enrollmentDraft(t, m)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)
	next, _ = m.Update(loaded{data: map[string]any{"hosts": []any{}}, revision: m.revision})
	m = next.(Model)
	if m.form.fields[2].value != enrollmentPassword || m.form.fields[0].value != "192.0.2.42" {
		t.Fatal("refresh or resize discarded draft")
	}
	m, inspect := key(m, "ctrl+s")
	response := inspect()
	m.busy = false
	m, _ = key(m, "esc")
	m = enrollmentDraft(t, m)
	next, _ = m.Update(response)
	m = next.(Model)
	if m.pending != nil {
		t.Fatal("old inspection attached to a reopened form")
	}
	m, inspect = key(m, "ctrl+s")
	m = complete(t, m, inspect)
	m, _ = key(m, "esc")
	if m.pending != nil || m.form.fields[2].value != "" {
		t.Fatal("back from approval retained bootstrap password")
	}
}

func TestHostEnrollmentDemoNeverContactsNetworkAndManualRemainsAvailable(t *testing.T) {
	m, requests := enrollmentFixture(t, false, false)
	m.data["demo"] = true
	m = enrollmentDraft(t, m)
	m, cmd := key(m, "ctrl+s")
	if cmd != nil || m.err == "" || len(*requests) != 0 {
		t.Fatal("demo attempted real enrollment")
	}
	m, _ = key(m, "esc")
	m, _ = key(m, "m")
	if m.form == nil || m.form.kind != "host" {
		t.Fatal("manual host creation unavailable")
	}
}

func TestHostEnrollmentFailureAfterApprovalCannotEchoPassword(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			json.NewEncoder(w).Encode(map[string]any{"address": "192.0.2.42", "port": 22, "fingerprint": enrollmentFingerprint, "key_type": "ssh-ed25519", "known": true, "changed": false})
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{"error": "remote echoed " + enrollmentPassword})
	}))
	defer server.Close()
	m := enrollmentDraft(t, New(&client.Client{HTTP: server.Client(), Base: server.URL}))
	m, inspect := key(m, "ctrl+s")
	m = complete(t, m, inspect)
	m, enroll := key(m, "y")
	if m.form.fields[2].value != "" {
		t.Fatal("running enrollment retains form password")
	}
	next, _ := m.Update(enroll())
	m = next.(Model)
	if m.err == "" || m.form == nil || strings.Contains(m.View(), enrollmentPassword) || m.form.fields[2].value != "" {
		t.Fatal("enrollment failure leaked credentials or discarded safe draft")
	}
}

func TestSelectedHostEnrollmentKeepsScheduleAndCompactMouseActions(t *testing.T) {
	m, requests := enrollmentFixture(t, false, false)
	m.width = 80
	m.height = 24
	m.data["hosts"] = []any{map[string]any{"id": "existing", "name": "existing server", "address": "192.0.2.42", "ssh_port": float64(22), "schedule": "23:15", "enabled": false, "ssh_user": "old-user", "key_path": "/old/key", "last_probe": "2026-10-07T12:00:00Z", "probe_error": "old error", "inventory": map[string]any{"details": map[string]any{"large": strings.Repeat("x", 70<<10)}}}}
	m.buildRows()
	if !strings.Contains(m.View(), "b Sichern") {
		t.Fatal("host action bar clipped at 80 columns")
	}
	next, _ := m.Update(tea.MouseMsg{X: 38, Y: 21, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if m.form == nil || m.form.kind != "hostConnect" || m.form.fields[1].value != "root" {
		t.Fatal("selected host mouse onboarding unavailable")
	}
	m.form.fields[2].value = enrollmentPassword
	m, inspect := key(m, "ctrl+s")
	m = complete(t, m, inspect)
	next, enroll := m.Update(tea.MouseMsg{X: 60, Y: 21, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = complete(t, next.(Model), enroll)
	host := object((*requests)[1]["host"])
	encoded, _ := json.Marshal((*requests)[1])
	if len(encoded) >= 64<<10 {
		t.Fatal("re-enrollment metadata exceeds bounded request size")
	}
	if host["inventory"] != nil || host["last_probe"] != nil || host["probe_error"] != nil {
		t.Fatal("re-enrollment sends probe artifacts beyond the bounded request size")
	}
	if object(object(m.data["hosts"].([]any)[0])["inventory"])["details"] == nil {
		t.Fatal("re-enrollment changed the displayed host inventory")
	}
	if host["id"] != "existing" || host["schedule"] != "23:15" || host["enabled"] != false || host["ssh_user"] != "anker" {
		t.Fatal("automatic re-enrollment discarded host identity or schedule")
	}
}
