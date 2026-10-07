package tui

import (
	"anker/internal/anker"
	"anker/internal/client"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func key(m Model, key string) (Model, tea.Cmd) {
	var k tea.KeyMsg
	switch key {
	case "enter":
		k.Type = tea.KeyEnter
	case "esc":
		k.Type = tea.KeyEsc
	case "tab":
		k.Type = tea.KeyTab
	case "ctrl+s":
		k.Type = tea.KeyCtrlS
	default:
		k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, cmd := m.Update(k)
	return next.(Model), cmd
}
func complete(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected an API operation")
	}
	msg := cmd()
	if r, ok := msg.(commandResult); ok && r.err != nil {
		t.Fatal(r.err)
	}
	next, _ := m.Update(msg)
	return next.(Model)
}
func connected(t *testing.T) (Model, *anker.Service) {
	t.Helper()
	root := t.TempDir()
	if err := anker.EnsureDataMode(root, true); err != nil {
		t.Fatal(err)
	}
	store, err := anker.OpenStore(filepath.Join(root, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := anker.NewService(root, store, anker.DemoCollector{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	svc.Demo = true
	auth := anker.NewAuth(store)
	if err := svc.SeedDemo(auth); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(anker.Handler(svc, auth, true))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		svc.StopJobs(ctx)
		server.Close()
		store.Close()
	})
	c := &client.Client{HTTP: server.Client(), Base: server.URL}
	m := New(c)
	return complete(t, m, m.load()), svc
}

// This fails if the form drops existing SSH properties or writes before approval.
func TestGuidedHostCreateEditPauseAgainstService(t *testing.T) {
	m, svc := connected(t)
	m, _ = key(m, "m")
	m, _ = key(m, "edge-host")
	m, _ = key(m, "tab")
	m, _ = key(m, "192.0.2.42")
	m, cmd := key(m, "ctrl+s")
	if cmd != nil || m.pending == nil {
		t.Fatal("submit bypassed confirmation")
	}
	hosts, _ := svc.Hosts()
	for _, h := range hosts {
		if h.Name == "edge-host" {
			t.Fatal("host saved before approval")
		}
	}
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	hosts, _ = svc.Hosts()
	var h anker.Host
	for _, v := range hosts {
		if v.Name == "edge-host" {
			h = v
		}
	}
	if h.Address != "192.0.2.42" || !h.Enabled || h.SSHPort != 22 || h.KnownHostsPath != "/etc/anker/known_hosts" {
		t.Fatalf("saved host: %+v", h)
	}
	m = complete(t, m, m.load())
	for i, id := range m.ids {
		if id == h.ID {
			m.cursor = i
		}
	}
	m, _ = key(m, "e")
	m.form.fields[2].value = "Datacenter"
	m, _ = key(m, "ctrl+s")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	saved, _ := svc.Host(h.ID)
	if saved.Group != "Datacenter" || saved.KeyPath != h.KeyPath {
		t.Fatalf("edit lost host settings: %+v", saved)
	}
	m = complete(t, m, m.load())
	m, _ = key(m, "p")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	saved, _ = svc.Host(h.ID)
	if saved.Enabled {
		t.Fatal("pause did not reach service")
	}
}

// A stale status refresh or failed validation must not destroy entered passwords.
func TestHiddenPasswordsAndFormErrorsAgainstService(t *testing.T) {
	m, svc := connected(t)
	m.switchTab(4)
	m.users = true
	m.buildRows()
	m, _ = key(m, "a")
	m.form.fields[0].value = "terminal-user"
	m.form.fields[3].value = "unique-terminal-password"
	m.form.fields[4].value = "wrong"
	m, _ = key(m, "ctrl+s")
	if m.pending != nil || m.err == "" || strings.Contains(m.View(), "unique-terminal-password") {
		t.Fatal("password confirmation or masking failed")
	}
	m.form.fields[4].value = m.form.fields[3].value
	m, _ = key(m, "ctrl+s")
	if strings.Contains(m.View(), "unique-terminal-password") {
		t.Fatal("confirmation leaks password")
	}
	m, cmd := key(m, "y")
	m = complete(t, m, cmd)
	var u anker.User
	if err := svc.Store.Get("users", "terminal-user", &u); err != nil {
		t.Fatal(err)
	}
	if u.Role != "reader" {
		t.Fatal("wrong default role")
	}
}

// Whole settings saves must preserve fields outside the guided editor.
func TestGuidedSettingsPreserveNotificationAndPasswordPolicy(t *testing.T) {
	m, svc := connected(t)
	settings, _ := svc.Settings()
	settings.Webhook = "https://example.org/notify"
	settings.PasswordMinLength = 18
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	m.switchTab(4)
	m, cmd := key(m, "enter")
	m = complete(t, m, cmd)
	m.form.fields[0].value = "UTC"
	m.form.fields[1].value = "03:15"
	m.form.fields[2].value = "6"
	m, _ = key(m, "ctrl+s")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	saved, _ := svc.Settings()
	if saved.Timezone != "UTC" || saved.Schedule != "03:15" || saved.Parallel != 6 || saved.Webhook != settings.Webhook || saved.PasswordMinLength != 18 {
		t.Fatalf("settings save: %+v", saved)
	}
}

// Plan creation uses structured inputs, readable preview and a separate typed approval.
func TestRestorePlanAndExplicitApplyAgainstService(t *testing.T) {
	m, svc := connected(t)
	m.switchTab(1)
	backup := m.selected()
	if backup == nil {
		t.Fatal("demo has no backups")
	}
	m, cmd := key(m, "r")
	m = complete(t, m, cmd)
	m.form.fields[3].value = "etc/sysctl.d/99-anker.conf"
	m, cmd = key(m, "ctrl+s")
	m = complete(t, m, cmd)
	m, _ = key(m, "ctrl+s")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	p := object(m.data["preview"])
	id := str(p, "id")
	if id == "" || m.detail == "" || strings.Contains(m.detail, "\"steps\"") {
		t.Fatal("missing readable plan")
	}
	if str(p, "state") != "ready" {
		t.Fatalf("unexpected demo plan: %s", m.detail)
	}
	m, _ = key(m, "a")
	if m.pending == nil || m.pending.challenge != id {
		t.Fatal("apply has no typed confirmation")
	}
	m, cmd = key(m, "enter")
	if cmd != nil {
		t.Fatal("default confirmation applied plan")
	}
	// Re-open after default cancel, enter exact ID and approve.
	m, _ = key(m, "a")
	m, _ = key(m, id)
	m, cmd = key(m, "enter")
	m = complete(t, m, cmd)
	jobs, _ := svc.Jobs()
	found := false
	for _, j := range jobs {
		if j.Kind == "restore" {
			found = true
		}
	}
	if !found {
		t.Fatal("restore did not reach queue")
	}
}

// A refresh can reorder records while the selected ID and modal remain stable.
func TestRefreshSelectionAndConfirmationStayStable(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "a", "name": "first"}, map[string]any{"id": "b", "name": "second"}}}
	m.buildRows()
	m.cursor = 1
	m, _ = key(m, "b")
	next, _ := m.Update(loaded{data: map[string]any{"hosts": []any{map[string]any{"id": "b", "name": "second"}, map[string]any{"id": "a", "name": "first"}}}})
	m = next.(Model)
	if m.cursor != 0 || m.pending == nil || m.pending.path != "hosts/b/backup" {
		t.Fatal("refresh changed selected confirmation")
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)
	if m.pending == nil {
		t.Fatal("resize dismissed confirmation")
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if ansi.StringWidth(line) > 80 {
			t.Fatal("line exceeds viewport")
		}
	}
}

// No-replace export prevents a concurrent file creation from being overwritten.
func TestExportNoOverwriteAndSymlinkGuard(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "backup.tar")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/backups/b/download" {
			t.Errorf("wrong export path: %s", r.URL.Path)
		}
		os.WriteFile(target, []byte("existing"), 0600)
		w.Write([]byte("archive"))
	}))
	defer server.Close()
	c := &client.Client{HTTP: server.Client(), Base: server.URL}
	if err := download(c, context.Background(), "backups/b/download", target, false); err == nil {
		t.Fatal("concurrent existing file was replaced")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "existing" {
		t.Fatal("existing content changed")
	}
	if err := download(c, context.Background(), "backups/b/download", target, true); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(target)
	if string(b) != "archive" {
		t.Fatal("confirmed export failed")
	}
	link := filepath.Join(dir, "link.tar")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if checkOutput(link, true) == nil {
		t.Fatal("symlink target accepted")
	}
}

// Bounded and single-flight polling keeps an unreachable daemon from queuing work.
func TestPollingCoalescesAndErrorsKeepForm(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":"unavailable"}`, 503)
	}))
	defer server.Close()
	m := New(&client.Client{HTTP: server.Client(), Base: server.URL})
	m.loading = false
	next, cmd := m.Update(tick(time.Now()))
	m = next.(Model)
	if cmd == nil || !m.loading {
		t.Fatal("poll did not start")
	}
	next, second := m.Update(tick(time.Now()))
	m = next.(Model)
	if second == nil || !m.loading {
		t.Fatal("timer stopped")
	}
	// Second tick command is a timer, not another HTTP request; execute the pending load.
	m, _ = key(m, "a")
	next, _ = m.Update(m.load()())
	m = next.(Model)
	if calls.Load() != 1 || m.loadErr == "" || m.form == nil {
		t.Fatal("failed refresh dismissed form or duplicated calls")
	}
	next, _ = m.Update(commandResult{err: errors.New("denied")})
	if next.(Model).form == nil {
		t.Fatal("operation failure discarded form")
	}
}

// The actual boundary receives port maps and both migration safeguards as booleans.
func TestMigrationInputsAndUpdateTrustConfirmation(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"repository": "custom/repo", "official_repository": "jahartmann/Anker", "official_fingerprint": "SHA256:official"})
			return
		}
		if r.Header.Get("X-Anker-Request") != "1" {
			t.Error("missing origin header")
		}
		json.NewDecoder(r.Body).Decode(&captured)
		json.NewEncoder(w).Encode(map[string]any{"id": "abcd", "state": "blocked", "blockers": []string{"source check"}})
	}))
	defer server.Close()
	m := New(&client.Client{HTTP: server.Client(), Base: server.URL})
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "backup"}}}
	m.openForm("plan", nil)
	m.form.fields[2].value = "migration"
	m.form.fields[4].value = "eno1=ens3"
	m.form.fields[5].value = "ja"
	m.form.fields[6].value = "ja"
	m, cmd := key(m, "ctrl+s")
	m = complete(t, m, cmd)
	m, _ = key(m, "ctrl+s")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	if captured["console_confirmed"] != true || captured["source_offline"] != true || object(object(captured["mapping"])["interfaces"])["eno1"] != "ens3" {
		t.Fatalf("migration payload: %#v", captured)
	}
	m, _ = key(m, "a")
	if m.pending != nil {
		t.Fatal("blocked plan offered apply")
	}
	m.showDetail("Updates", "Updates")
	m, cmd = key(m, "o")
	m = complete(t, m, cmd)
	if m.pending == nil || !strings.Contains(m.pending.summary, "SHA256:official") || !strings.Contains(m.pending.summary, "custom/repo") {
		t.Fatal("source trust change lacks informed confirmation")
	}
	if m.pending.path != "updates/configure" {
		t.Fatal("wrong trust endpoint")
	}
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	if captured["repository"] != "jahartmann/Anker" || captured["public_key"] != "" || captured["confirmed"] != true {
		t.Fatalf("source configuration payload: %#v", captured)
	}
}

func TestRestoreFilePickerLoadsActualBackupPaths(t *testing.T) {
	m, _ := connected(t)
	m.switchTab(1)
	m, cmd := key(m, "r")
	if cmd == nil {
		t.Fatal("restore should load a file picker")
	}
	m = complete(t, m, cmd)
	if m.form == nil || len(m.form.fields[3].choices) == 0 {
		t.Fatal("restore paths missing")
	}
	found := false
	for _, path := range m.form.fields[3].choices {
		if path == "etc/sysctl.d/99-anker.conf" {
			found = true
		}
	}
	if !found {
		t.Fatal("service file list not selectable")
	}
}

func TestDetailScrollShowsLastLineAndRowsStaySingleLine(t *testing.T) {
	m := New(nil)
	m.width = 80
	m.height = 24
	m.showDetail("Details", strings.Repeat("filler\n", 40)+"last line")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = next.(Model)
	for i := 0; i < 4; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		m = next.(Model)
	}
	if !strings.Contains(m.View(), "last line") {
		t.Fatal("last detail line unreachable")
	}
	m.detail = ""
	m.data = map[string]any{"backups": []any{map[string]any{"id": "b", "host_name": "server", "status": "bad\nrow"}}}
	m.switchTab(1)
	if len(strings.Split(m.View(), "\n")) > 24 {
		t.Fatal("untrusted newline added screen rows")
	}
}

func TestDetailsActionsKeepOriginalObjectAfterRemoval(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"backups": []any{map[string]any{"id": "original", "host_name": "old"}, map[string]any{"id": "other", "host_name": "new"}}}
	m.switchTab(1)
	m, _ = key(m, "enter")
	next, _ := m.Update(loaded{data: map[string]any{"backups": []any{map[string]any{"id": "other", "host_name": "new"}}}})
	m = next.(Model)
	m, _ = key(m, "v")
	if m.pending == nil || m.pending.path != "backups/original/verify" {
		t.Fatal("detail action changed to a different backup after refresh")
	}
}

func TestFinishedBackupCanBeRepeatedAndBusyConfirmationCannotResubmit(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"jobs": []any{map[string]any{"id": "done", "kind": "backup", "state": "successful"}}}
	m.switchTab(3)
	m, _ = key(m, "r")
	if m.pending == nil || m.pending.path != "jobs/done/retry" {
		t.Fatal("backend-supported completed backup cannot be repeated")
	}
	m.busy = true
	m, _ = key(m, "y")
	if m.pending == nil {
		t.Fatal("busy UI processed repeated approval")
	}
}

type gatedDemo struct {
	anker.DemoCollector
	release <-chan struct{}
}

func (c gatedDemo) Probe(ctx context.Context, h anker.Host) (anker.Inventory, error) {
	select {
	case <-ctx.Done():
		return anker.Inventory{}, ctx.Err()
	case <-c.release:
		return c.DemoCollector.Probe(ctx, h)
	}
}
func waitJob(t *testing.T, s *anker.Service, id, state string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j, err := s.Job(id)
		if err == nil && j.State == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := s.Job(id)
	t.Fatalf("job %s did not become %s: %+v", id, state, j)
}

// Cancellation and retry must target real queued workers rather than editing local rows.
func TestProbeCancelAndRetryAgainstService(t *testing.T) {
	m, svc := connected(t)
	release := make(chan struct{})
	svc.Collector = gatedDemo{DemoCollector: svc.Collector.(anker.DemoCollector), release: release}
	defer close(release)
	m, _ = key(m, "t")
	m, cmd := key(m, "y")
	if cmd == nil {
		t.Fatal("probe not queued")
	}
	result := cmd().(commandResult)
	if result.err != nil {
		t.Fatal(result.err)
	}
	id := str(object(result.data), "id")
	next, _ := m.Update(result)
	m = next.(Model)
	waitJob(t, svc, id, "running")
	m = complete(t, m, m.load())
	m.switchTab(3)
	for i, v := range m.ids {
		if v == id {
			m.cursor = i
		}
	}
	m, _ = key(m, "c")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	waitJob(t, svc, id, "cancelled")
	m = complete(t, m, m.load())
	for i, v := range m.ids {
		if v == id {
			m.cursor = i
		}
	}
	m, _ = key(m, "r")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	jobs, _ := svc.Jobs()
	found := false
	for _, j := range jobs {
		if j.ID != id && j.Kind == "probe" {
			found = true
		}
	}
	if !found {
		t.Fatal("retry did not queue another probe")
	}
}

func TestHostEditPreservesAbsentRestoreCredentials(t *testing.T) {
	m := New(nil)
	m.openForm("host", map[string]any{"id": "legacy", "name": "server", "address": "192.0.2.1", "ssh_port": float64(22), "ssh_user": "anker", "key_path": "/keys/backup", "known_hosts_path": "/keys/known", "enabled": true})
	m, _ = key(m, "ctrl+s")
	if m.pending == nil {
		t.Fatal("host editor failed")
	}
	if str(object(m.pending.input), "restore_key_path") != "" {
		t.Fatal("unrelated edit unexpectedly enabled a restore key")
	}
}

func TestStatusStartedBeforeActionCannotOverwriteNewerState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"hosts": []any{map[string]any{"id": "h", "name": "outdated"}}})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"id": "job", "state": "queued"})
		}
	}))
	defer server.Close()
	m := New(&client.Client{HTTP: server.Client(), Base: server.URL})
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "h", "name": "current"}}}
	m.buildRows()
	oldLoad := m.load()
	operation := m.request(action{title: "Sichern", method: "POST", path: "hosts/h/backup", input: map[string]any{}})
	m = complete(t, m, operation)
	next, cmd := m.Update(oldLoad())
	m = next.(Model)
	if strings.Contains(m.View(), "outdated") || cmd == nil {
		t.Fatal("older status response replaced state after a completed action")
	}
}

func TestUserRoleChoicesCreateValidRestoreAccount(t *testing.T) {
	m, svc := connected(t)
	m.switchTab(4)
	m.users = true
	m.openForm("user", nil)
	m.form.fields[0].value = "restore-terminal"
	m.form.fields[3].value = "long-terminal-password"
	m.form.fields[4].value = "long-terminal-password"
	m.form.focus = 1
	m, _ = key(m, "right")
	m, _ = key(m, "ctrl+s")
	m, cmd := key(m, "y")
	m = complete(t, m, cmd)
	var user anker.User
	if err := svc.Store.Get("users", "restore-terminal", &user); err != nil {
		t.Fatal(err)
	}
	if user.Role != "restore" {
		t.Fatalf("wrong restore role: %+v", user)
	}
}

func TestBusyMouseCannotNavigateBehindPendingRequest(t *testing.T) {
	m := New(nil)
	m.switchTab(4)
	m.busy = true
	next, _ := m.Update(tea.MouseMsg{X: 2, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if next.(Model).tab != 4 {
		t.Fatal("mouse changed views while pending request was loading a form")
	}
}

func TestHostTrustGuidedFingerprintValidation(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "h", "name": "server", "address": "192.0.2.1", "ssh_port": float64(22), "known_hosts_path": "/etc/anker/known_hosts"}}}
	m.buildRows()
	m, _ = key(m, "h")
	if m.form == nil {
		t.Fatal("missing guided SSH trust form")
	}
	m.form.fields[2].value = "SHA256:invalid"
	m, _ = key(m, "ctrl+s")
	if m.pending != nil || m.err == "" {
		t.Fatal("invalid SSH fingerprint offered trust approval")
	}
	m.form.fields[2].value = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	m, _ = key(m, "ctrl+s")
	if m.pending == nil || !strings.Contains(m.pending.summary, "Unabhängig") {
		t.Fatal("SSH trust lacks independent fingerprint confirmation")
	}
}

func TestStorageGrowthRequiresPlanAndTypedMountConfirmation(t *testing.T) {
	var paths []string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		json.NewDecoder(r.Body).Decode(&payload)
		if r.URL.Path == "/api/storage/plan" {
			json.NewEncoder(w).Encode(map[string]any{"id": "checked-plan", "volume_id": "data", "mount": "/srv/anker", "can_grow": true, "message": "Datenträger wurde erweitert", "environment": "vm:kvm", "steps": []any{map[string]any{"title": "Dateisystem erweitern", "explanation": "Geprüfter ext4-Vorgang"}}})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"status": "running"})
		}
	}))
	defer server.Close()
	m := New(&client.Client{HTTP: server.Client(), Base: server.URL})
	m.data["storage"] = map[string]any{"environment": "vm:kvm", "volumes": []any{map[string]any{"id": "data", "mount": "/srv/anker", "fs_type": "ext4", "source": "/dev/vdb1"}}}
	m.showDetail("Speicher", "Speicher")
	m, _ = key(m, "g")
	if m.form == nil {
		t.Fatal("no guided volume selection")
	}
	m, _ = key(m, "ctrl+s")
	m, cmd := key(m, "y")
	m = complete(t, m, cmd)
	if len(paths) != 1 || paths[0] != "/api/storage/plan" || payload["volume_id"] != "data" {
		t.Fatalf("wrong storage plan request: %v %#v", paths, payload)
	}
	if !strings.Contains(m.detail, "Geprüfter ext4-Vorgang") {
		t.Fatal("missing real plan preview")
	}
	m, _ = key(m, "g")
	if m.pending == nil || m.pending.challenge != "/srv/anker" {
		t.Fatal("growth lacks mount confirmation")
	}
	m, cmd = key(m, "enter")
	if cmd != nil {
		t.Fatal("growth started without typed mount")
	}
	m, _ = key(m, "g")
	m, _ = key(m, "/srv/anker")
	m, cmd = key(m, "enter")
	m = complete(t, m, cmd)
	if len(paths) != 2 || paths[1] != "/api/storage/grow" || payload["plan_id"] != "checked-plan" || payload["confirmation"] != "/srv/anker" || payload["volume_id"] != "data" {
		t.Fatalf("unsafe growth request: %v %#v", paths, payload)
	}
}

func TestStorageManualAndContainerNeverOfferGrowth(t *testing.T) {
	m := New(nil)
	m.data["storage"] = map[string]any{"environment": "container:lxc", "volumes": []any{map[string]any{"id": "data", "mount": "/srv/anker"}}}
	m.showDetail("Speicher", "LXC-Warnung")
	m, cmd := key(m, "g")
	if cmd != nil || m.form != nil || m.pending != nil {
		t.Fatal("LXC offered storage modification")
	}
	m.data["growthPlan"] = map[string]any{"id": "manual", "mount": "/srv/anker", "can_grow": false}
	m.showDetail("Speicherplan", "Manuelle Erweiterung erforderlich")
	m, cmd = key(m, "g")
	if cmd != nil || m.pending != nil {
		t.Fatal("manual storage plan offered growth")
	}
}

func TestSSHTrustHasFixedArgumentsAndCustomPathGuard(t *testing.T) {
	host := trustedHost{address: "192.0.2.1", port: 2222, fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	if err := validateTrust(host); err != nil {
		t.Fatal(err)
	}
	args := trustArgs(host)
	want := []string{"host", "trust", "192.0.2.1", "--port", "2222", "--fingerprint", "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	if len(args) != len(want) {
		t.Fatal(args)
	}
	for i, v := range args {
		if v != want[i] {
			t.Fatalf("fixed helper arguments: %v", args)
		}
	}
	host.address = "host; touch /tmp/unsafe"
	if validateTrust(host) == nil {
		t.Fatal("invalid address reached helper")
	}
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "h", "known_hosts_path": "/custom/trust"}}}
	m.buildRows()
	m, cmd := key(m, "h")
	if cmd != nil || m.form != nil || m.err == "" {
		t.Fatal("custom trust file changed by fixed-path helper")
	}
	m.data["demo"] = true
	m.data["hosts"] = []any{map[string]any{"id": "h", "known_hosts_path": "/etc/anker/known_hosts"}}
	m, _ = key(m, "h")
	if m.form != nil {
		t.Fatal("demo offered real SSH trust mutation")
	}
	var output boundedOutput
	data := strings.Repeat("a", 100000)
	n, err := output.Write([]byte(data))
	if err != nil || n != 100000 || len(output.data) != 64<<10 {
		t.Fatal("helper output not bounded")
	}
}

func TestTypedApprovalCanBeConfirmedWithMouse(t *testing.T) {
	m := New(&client.Client{})
	m.width = 80
	m.height = 24
	m.confirm(action{title: "Dateisystem erweitern", path: "storage/grow", method: "POST", challenge: "/srv/anker", input: map[string]any{}})
	m, _ = key(m, "/srv/anker")
	next, cmd := m.Update(tea.MouseMsg{X: 60, Y: 21, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil || !next.(Model).busy {
		t.Fatal("mouse confirmation ignored correctly typed mount")
	}
}

func TestAsyncStorageAndUpdateStatusCanBeRefreshedInPlace(t *testing.T) {
	for _, scenario := range []struct{ title, path string }{{"Speicher", "/api/storage/state"}, {"Speichererweiterung", "/api/storage/state"}, {"Updates", "/api/updates"}} {
		t.Run(scenario.title, func(t *testing.T) {
			state := "failed"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != scenario.path || r.Method != "GET" {
					t.Errorf("status request: %s %s", r.Method, r.URL.Path)
				}
				json.NewEncoder(w).Encode(map[string]any{"status": state, "message": "latest operation outcome", "busy": false})
			}))
			defer server.Close()
			m := New(&client.Client{HTTP: server.Client(), Base: server.URL})
			m.showDetail(scenario.title, "running")
			m, cmd := key(m, "r")
			if cmd == nil {
				t.Fatal("no in-place status refresh")
			}
			m = complete(t, m, cmd)
			if !strings.Contains(m.detail, "failed") || !strings.Contains(m.detail, "latest operation outcome") {
				t.Fatal("latest failure missing")
			}
			state = "succeeded"
			m, cmd = key(m, "r")
			m = complete(t, m, cmd)
			if !strings.Contains(m.detail, "succeeded") {
				t.Fatal("completion not shown")
			}
		})
	}
}

func TestStorageSettingsEntryLoadsVolumeOverviewBeforePlanning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/storage" {
			t.Errorf("overview should load volumes: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"environment": "vm:kvm", "volumes": []any{map[string]any{"id": "data", "mount": "/srv/anker"}}})
	}))
	defer server.Close()
	m := New(&client.Client{HTTP: server.Client(), Base: server.URL})
	m.switchTab(4)
	m.cursor = 3
	m, cmd := key(m, "enter")
	m = complete(t, m, cmd)
	if m.detailTitle != "Speicher" || len(list(object(m.data["storage"])["volumes"])) != 1 {
		t.Fatal("volume overview unavailable before planning")
	}
}
