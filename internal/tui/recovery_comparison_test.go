package tui

import (
	"anker/internal/anker"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Catches lost explicit choices/snapshot on review-back, and a source diff
// substituted for the saved plan's captured-target comparison.
func TestTUIRecoveryPreservesDraftAndShowsTargetDiff(t *testing.T) {
	m, _ := connected(t)
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/sysctl.d/99-anker.conf"
	m, cmd := key(m, "ctrl+s")
	m = complete(t, m, cmd)
	if len(m.form.fields) < 11 {
		t.Fatal("no explicit per-file decision input")
	}
	m.form.fields[10].value = "etc/sysctl.d/99-anker.conf=backup"
	snapshot := str(recoveryInspection(m.form), "snapshot_id")
	m, _ = key(m, "ctrl+s")
	if m.pending == nil {
		t.Fatal(m.err)
	}
	p := m.pending.input.(anker.PlanRequest)
	if snapshot == "" || p.SnapshotID != snapshot || p.Decisions["etc/sysctl.d/99-anker.conf"] != anker.DecisionBackup {
		t.Fatalf("choices/evidence lost: %+v", p)
	}
	m, _ = key(m, "esc")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 48, Height: 18})
	m = next.(Model)
	if m.form.fields[10].value != "etc/sysctl.d/99-anker.conf=backup" || str(recoveryInspection(m.form), "snapshot_id") != snapshot {
		t.Fatal("back/resize lost draft")
	}
	m, _ = key(m, "ctrl+s")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	m, _ = key(m, "c")
	if m.form == nil || m.form.kind != "recoveryCompare" || m.form.fields[1].value != "target" {
		t.Fatal("target comparison unavailable")
	}
	m, cmd = key(m, "ctrl+s")
	if m.pending != nil {
		t.Fatal("read comparison requested write approval")
	}
	m = complete(t, m, cmd)
	if m.detailTitle != "Dateivergleich" || !strings.Contains(m.detail, "Ziel") || !strings.Contains(m.detail, "Vorbereitet") {
		t.Fatalf("missing actual target/prepared labels: %s", m.detail)
	}
	m, _ = key(m, "esc")
	if m.detailTitle != "Wiederherstellungsplan" || m.detail == "" {
		t.Fatal("comparison back lost saved plan")
	}
}

func TestTUIProtectedDecisionCannotEnableApply(t *testing.T) {
	m, s := connected(t)
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/network/interfaces"
	m, cmd := key(m, "ctrl+s")
	m = complete(t, m, cmd)
	if len(m.form.fields) < 11 {
		t.Fatal("no explicit per-file choice")
	}
	m.form.fields[10].value = "etc/network/interfaces=prepared"
	i := recoveryInspection(m.form)
	ports := []string{}
	for _, raw := range list(i["ports"]) {
		ports = append(ports, str(object(raw), "name")+"="+str(object(list(i["target_ports"])[0]), "name"))
	}
	m.form.fields[4].value = strings.Join(ports, ",")
	m.form.fields[5].value = "ja"
	m, _ = key(m, "ctrl+s")
	if m.pending == nil {
		t.Fatal(m.err)
	}
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	p, err := s.Plan(str(object(m.data["preview"]), "id"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].Action != "manual" {
		t.Fatal("protected decision bypassed server policy")
	}
	m, cmd = key(m, "a")
	if cmd != nil || m.pending != nil || m.err == "" {
		t.Fatal("protected plan offered execution")
	}
}

// The same saved Plan is served to web's detail request and CLI, while TUI
// displays actual service actions/blockers rather than a separate release rule.
func TestTUIRecoveryPlanMatchesCLIAndHTTP(t *testing.T) {
	m, s := connected(t)
	m.openForm("plan", nil)
	m.form.fields[2].value = "host"
	m.form.fields[3].value = "etc/network/interfaces"
	m, cmd := key(m, "ctrl+s")
	m = complete(t, m, cmd)
	m.form.fields[4].value = "eno1=eno1"
	m.form.fields[10].value = "etc/network/interfaces=backup"
	// Deliberately retain missing source isolation to compare real blockers.
	m, _ = key(m, "ctrl+s")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	id := str(object(m.data["preview"]), "id")
	var webPlan anker.Plan
	if err := m.client.Call(t.Context(), "GET", "plans/"+id, nil, &webPlan); err != nil {
		t.Fatal(err)
	}
	if len(webPlan.Blockers) == 0 || webPlan.State != "blocked" {
		t.Fatal("parity fixture must exercise nonempty backend blockers", webPlan.State, webPlan.Blockers)
	}
	var out bytes.Buffer
	c := *m.client
	c.Out = &out
	if err := c.Run(t.Context(), []string{"restore", "list"}); err != nil {
		t.Fatal(err)
	}
	var cliPlans []anker.Plan
	if err := json.Unmarshal(out.Bytes(), &cliPlans); err != nil {
		t.Fatal(err)
	}
	if len(cliPlans) != 1 || !reflect.DeepEqual(webPlan.Steps, cliPlans[0].Steps) || !reflect.DeepEqual(webPlan.Blockers, cliPlans[0].Blockers) || webPlan.ID != cliPlans[0].ID || webPlan.State != cliPlans[0].State {
		t.Fatal("CLI and web endpoint disagree about the identical plan")
	}
	stored, _ := s.Plan(id)
	if stored.Steps[0].Action != "manual" || !strings.Contains(m.detail, "etc/network/interfaces · manual") || str(object(m.data["preview"]), "state") != webPlan.State {
		t.Fatal("TUI changed the shared file action or state")
	}
	for _, blocker := range webPlan.Blockers {
		if !strings.Contains(m.detail, blocker) {
			t.Fatal("TUI omitted service blocker", blocker)
		}
	}
	for _, action := range m.detailActions() {
		if action.key == "a" {
			t.Fatal("TUI released a manual plan")
		}
	}
	out.Reset()
	if err := c.Run(t.Context(), []string{"restore", "compare", id, "--path", "etc/network/interfaces", "--basis", "target"}); err != nil {
		t.Fatal(err)
	}
	var cliComparison anker.FileComparison
	if err := json.Unmarshal(out.Bytes(), &cliComparison); err != nil {
		t.Fatal(err)
	}
	var webComparison anker.FileComparison
	if err := c.Call(t.Context(), "GET", "plans/"+id+"/comparison?path=etc%2Fnetwork%2Finterfaces&basis=target", nil, &webComparison); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cliComparison, webComparison) {
		t.Fatal("CLI comparison differs from web endpoint")
	}
}

func TestTUIRecoveryChecksUseTrustedActorAndPreserveMachineResult(t *testing.T) {
	m, s := connected(t)
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/sysctl.d/99-anker.conf"
	m, cmd := key(m, "ctrl+s")
	m = complete(t, m, cmd)
	m, _ = key(m, "ctrl+s")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	p, _ := s.Plan(str(object(m.data["preview"]), "id"))
	p.State, p.Result = "checks_pending", &anker.ApplyResult{State: "applied", OperationID: p.ID}
	// Reverse the API fixture order: display/selection must follow dependencies.
	p.Tasks = []anker.RecoveryTask{
		{ID: "check-reboot", Kind: "manual-check", Title: "Neustart prüfen", State: "pending", Requires: []string{"check-reachability"}},
		{ID: "check-reachability", Kind: "manual-check", Title: "Erreichbarkeit prüfen", State: "pending", Requires: []string{"check-services"}},
		{ID: "check-services", Kind: "manual-check", Title: "Dienste prüfen", State: "pending"},
	}
	data, _ := json.Marshal(p)
	if err := os.WriteFile(filepath.Join(s.Root, "plans", p.ID, "plan.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Put("plans", p.ID, p); err != nil {
		t.Fatal(err)
	}
	var public map[string]any
	if err := m.client.Call(t.Context(), "GET", "plans/"+p.ID, nil, &public); err != nil {
		t.Fatal(err)
	}
	m.data["preview"] = public
	m.showDetail("Wiederherstellungsplan", planText(public, m))
	if strings.Index(m.detail, "check-services ·") > strings.Index(m.detail, "check-reachability ·") || strings.Index(m.detail, "check-reachability ·") > strings.Index(m.detail, "check-reboot ·") {
		t.Fatal("checks are not dependency ordered")
	}
	for _, id := range []string{"check-services", "check-reachability", "check-reboot"} {
		m, _ = key(m, "d")
		if m.form == nil || m.form.fields[0].value != id {
			t.Fatalf("wrong next task: %v", m.form)
		}
		m.form.focus = 1
		m, _ = key(m, "Prüfung an der Konsole dokumentiert")
		m, _ = key(m, "ctrl+s")
		if m.pending == nil {
			t.Fatal(m.err)
		}
		payload := m.pending.input.(map[string]string)
		if len(payload) != 1 || payload["evidence"] == "" {
			t.Fatal("adapter supplied an actor or omitted evidence")
		}
		m, _ = key(m, "esc")
		if m.form.fields[1].value == "" {
			t.Fatal("back lost check evidence draft")
		}
		m, _ = key(m, "ctrl+s")
		m, cmd = key(m, "y")
		m = complete(t, m, cmd)
	}
	stored, _ := s.Plan(p.ID)
	for _, task := range stored.Tasks {
		if task.State != "user_confirmed" || task.ConfirmedBy != "local" || task.ConfirmedAt == "" {
			t.Fatal("trusted identity/provenance lost", task)
		}
	}
	if stored.State != "checks_pending" || stored.Result.RebootVerified || !strings.Contains(m.detail, "Maschineller Neustartnachweis: fehlt") {
		t.Fatal("manual confirmation became machine proof")
	}
}

func TestTUIRecoveryLegacyPlanRemainsReadableWithoutApply(t *testing.T) {
	m := New(nil)
	p := map[string]any{"id": "old", "scenario": "migration", "state": "ready", "steps": []any{map[string]any{"path": "etc/a", "action": "apply", "diff": "private legacy content"}}}
	m.data["preview"] = p
	m.showDetail("Wiederherstellungsplan", planText(p, m))
	if !strings.Contains(m.detail, "Andere Hardware") || !strings.Contains(m.detail, "Legacy-Plan") || strings.Contains(m.detail, "private legacy content") {
		t.Fatal("legacy reading/private comparison contract lost")
	}
	m, cmd := key(m, "a")
	if cmd != nil || m.pending != nil || m.err == "" {
		t.Fatal("legacy plan offered new execution")
	}
	// The service accepts exactly schema 2, not arbitrary future schemas.
	p["schema_version"] = float64(3)
	m, cmd = key(m, "a")
	if cmd != nil || m.pending != nil {
		t.Fatal("unsupported future schema offered execution")
	}
	for _, action := range m.detailActions() {
		if action.key == "a" {
			t.Fatal("unsupported schema advertised execution")
		}
	}
}

func TestTUIRecoveryStorageChoiceRequiresFreshSnapshot(t *testing.T) {
	m, _ := connected(t)
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/pve/storage.cfg,etc/sysctl.d/99-anker.conf"
	m, cmd := key(m, "ctrl+s")
	m = complete(t, m, cmd)
	if recoveryInspection(m.form) == nil {
		m, cmd = key(m, "ctrl+s")
		m = complete(t, m, cmd)
	}
	first := str(recoveryInspection(m.form), "snapshot_id")
	m.form.fields[7].value = "local=local,local-lvm=manual"
	m.form.fields[10].value = "etc/sysctl.d/99-anker.conf=backup"
	m, cmd = key(m, "ctrl+s")
	if cmd == nil || m.pending != nil {
		t.Fatal("changed storage choice reused old captured selection")
	}
	m = complete(t, m, cmd)
	second := str(recoveryInspection(m.form), "snapshot_id")
	if second == "" || second == first {
		t.Fatal("storage change did not acquire a new target snapshot")
	}
	m, _ = key(m, "ctrl+s")
	if m.pending == nil {
		t.Fatal(m.err)
	}
	p := m.pending.input.(anker.PlanRequest)
	if p.SnapshotID != second || p.Decisions["etc/sysctl.d/99-anker.conf"] != anker.DecisionBackup || p.Mapping.Storage["local"] != "local" {
		t.Fatal("recapture lost choice or submitted old evidence", p)
	}
}

func TestTUIRecoveryRecaptureInvalidatesTargetDependentChoices(t *testing.T) {
	for _, choice := range []string{"keep", "prepared", "backup", "manual"} {
		t.Run(choice, func(t *testing.T) {
			m, s := connected(t)
			m.openForm("plan", nil)
			m.form.fields[3].value = "etc/sysctl.d/99-anker.conf"
			m, cmd := key(m, "ctrl+s")
			m = complete(t, m, cmd)
			first := str(recoveryInspection(m.form), "snapshot_id")
			m.form.fields[10].value = "etc/sysctl.d/99-anker.conf=" + choice
			if err := os.WriteFile(filepath.Join(s.Root, "demo-hosts", m.form.fields[1].value, "files/etc/sysctl.d/99-anker.conf"), []byte("net.ipv4.ip_forward = 0\n# changed target\n"), 0600); err != nil {
				t.Fatal(err)
			}
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
			m = next.(Model)
			if cmd == nil || m.pending != nil {
				t.Fatal("draft cannot recapture target without approval or destroying input")
			}
			m = complete(t, m, cmd)
			if str(recoveryInspection(m.form), "snapshot_id") == first || m.form.fields[3].value != "etc/sysctl.d/99-anker.conf" {
				t.Fatal("fresh target was not captured or source draft lost")
			}
			if choice == "keep" || choice == "prepared" {
				if m.form.fields[10].value != "" {
					t.Fatal("changed target evidence retained dependent approval")
				}
			} else if m.form.fields[10].value != "etc/sysctl.d/99-anker.conf="+choice {
				t.Fatal("target refresh discarded source/manual choice")
			}
		})
	}
}

func TestTUIKeptReferencesBesideWritableFile(t *testing.T) {
	m, s := connected(t)
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/network/interfaces,etc/pve/storage.cfg,etc/sysctl.d/99-anker.conf"
	m, cmd := key(m, "ctrl+s")
	m = complete(t, m, cmd)
	m.form.fields[10].value = "etc/network/interfaces=keep,etc/pve/storage.cfg=keep,etc/sysctl.d/99-anker.conf=backup"
	// Kept dependencies need no target mappings, even though inspection included them.
	m.form.fields[4].value = ""
	m.form.fields[7].value = ""
	m, cmd = key(m, "ctrl+s")
	if cmd != nil {
		m = complete(t, m, cmd)
		m, _ = key(m, "ctrl+s")
	}
	if m.pending == nil {
		t.Fatalf("kept dependencies prevent plan: %s", m.err)
	}
	p := m.pending.input.(anker.PlanRequest)
	if len(p.Mapping.Interfaces) != 0 || len(p.Mapping.Storage) != 0 {
		t.Fatalf("kept-only mappings retained: %+v", p.Mapping)
	}
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	saved, err := s.Plan(str(object(m.data["preview"]), "id"))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Blockers) > 0 {
		t.Fatal(saved.Blockers)
	}
}

func TestTUITypedNetworkPreparationStaysManual(t *testing.T) {
	m, s := connected(t)
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/network/interfaces"
	m, cmd := key(m, "ctrl+s")
	m = complete(t, m, cmd)
	if len(m.form.fields) < 15 {
		t.Fatal("typed stanza controls missing")
	}
	found := false
	for _, key := range m.form.fields[11].choices {
		if key == "vmbr0/inet" {
			found = true
		}
	}
	if !found {
		t.Fatal("captured source stanza unavailable")
	}
	m.form.fields[10].value = "etc/network/interfaces=prepared"
	m.form.fields[11].value = "vmbr0/inet"
	m.form.fields[12].value = "198.51.100.9/24"
	m.form.fields[13].value = "198.51.100.1"
	m.form.fields[14].value = "192.0.2.53,2001:db8::53"
	m.form.fields[5].value = "ja"
	i := recoveryInspection(m.form)
	ports := []string{}
	for _, raw := range list(i["ports"]) {
		ports = append(ports, str(object(raw), "name")+"="+str(object(list(i["target_ports"])[0]), "name"))
	}
	m.form.fields[4].value = strings.Join(ports, ",")
	m, _ = key(m, "ctrl+s")
	if m.pending == nil {
		t.Fatal(m.err)
	}
	r := m.pending.input.(anker.PlanRequest)
	if r.Mapping.Network["vmbr0/inet"].AddressCIDR != "198.51.100.9/24" {
		t.Fatal("typed choice dropped")
	}
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	p, err := s.Plan(str(object(m.data["preview"]), "id"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].Action != "manual" || len(p.Steps[0].Effects) == 0 {
		t.Fatalf("network authority/effects: %+v", p.Steps)
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, "plans", p.ID, "prepared-files", "etc/network/interfaces"))
	if err != nil || !strings.Contains(string(raw), "address 198.51.100.9/24") {
		t.Fatalf("prepared bytes: %s %v", raw, err)
	}
	if len(p.Mapping.Network["vmbr0/inet"].DNSServers) != 2 {
		t.Fatal("DNS intent lost")
	}
	if !strings.Contains(string(raw), "iface lo inet loopback") {
		t.Fatal("unselected stanza changed")
	}
}
