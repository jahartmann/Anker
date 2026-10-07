package tui

import (
	"anker/internal/anker"
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
	"time"
)

func TestTerminalRestoreInspectsBeforePlan(t *testing.T) {
	m, _ := connected(t)
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/sysctl.d/99-anker.conf"
	m, cmd := key(m, "ctrl+s")
	if m.pending != nil || cmd == nil {
		t.Fatal("read-only inspection requires redundant approval or was skipped")
	}
	m = complete(t, m, cmd)
	if m.form == nil || m.pending != nil || object(m.data["preview"])["id"] != nil {
		t.Fatal("inspection unexpectedly created plan")
	}
	if strings.Contains(m.View(), "Netzwerkports") || strings.Contains(m.View(), "Storage (manuell)") {
		t.Fatal("ordinary file exposes irrelevant mappings")
	}
	m, _ = key(m, "ctrl+s")
	if m.pending == nil || m.pending.path != "plans" {
		t.Fatal("inspected selection cannot create reviewed plan")
	}
}
func TestTerminalRecoveryInspectionShowsOnlyRelevantDecisions(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "backup"}}}
	m.height = 60
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/network/interfaces"
	purpose := recoveryPurpose(m.form)
	m.handleResult(commandResult{purpose: purpose, data: map[string]any{"ports": []any{map[string]any{"name": "eno1", "suggested": "ens3"}}, "target_ports": []any{map[string]any{"name": "ens3"}}, "storage": []any{}, "requires_console": true, "requires_source_offline": false, "automatic": false, "manual": []any{"Netzwerkaktivierung bleibt manuell"}}})
	if m.form.fields[4].value != "eno1=ens3" || !strings.Contains(m.View(), "Netzwerkports") || !strings.Contains(m.View(), "manuell") || strings.Contains(m.View(), "Quellhost ausgeschaltet") {
		t.Fatalf("wrong decisions: %s", m.View())
	}
}
func TestTerminalLateInspectionDoesNotReplaceChangedSelection(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "backup"}}}
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/network/interfaces"
	purpose := recoveryPurpose(m.form)
	m.form.fields[3].value = "etc/sysctl.d/a.conf"
	m.pending = nil
	m.handleResult(commandResult{purpose: purpose, data: map[string]any{"ports": []any{map[string]any{"name": "eno1", "suggested": "ens3"}}}})
	if m.form.fields[4].value != "" {
		t.Fatal("late network result replaced new file selection")
	}
}
func TestTerminalRecoveryStatusAndRollbackRequireExplicitPlanID(t *testing.T) {
	m, _ := connected(t)
	m.data["preview"] = map[string]any{"id": "recovery-id", "state": "interrupted", "result": map[string]any{"operation_id": "recovery-id", "state": "writing", "error": "connection lost"}}
	m.showDetail("Wiederherstellungsplan", planText(object(m.data["preview"]), m))
	if !strings.Contains(m.detail, "connection lost") || !strings.Contains(m.detail, "recovery-id") {
		t.Fatal("host recovery evidence hidden")
	}
	m, cmd := key(m, "r")
	if cmd == nil || !m.busy {
		t.Fatal("host journal reconciliation unavailable")
	}
	m.busy = false
	m, cmd = key(m, "b")
	if cmd != nil || m.pending == nil || m.pending.challenge != "recovery-id" || m.pending.path != "plans/recovery-id/rollback" {
		t.Fatal("rollback lacks separate typed confirmation")
	}
	if _, ok := m.pending.input.(map[string]string); !ok {
		t.Fatal("rollback confirmation payload missing")
	}
}
func TestTerminalRecoveryStorageAndIdentityAreManualPlanDecisions(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "backup"}}}
	m.height = 70
	m.openForm("plan", nil)
	m.form.fields[2].value = "migration"
	purpose := recoveryPurpose(m.form)
	m.handleResult(commandResult{purpose: purpose, data: map[string]any{"source": map[string]any{"hostname": "old"}, "target": map[string]any{"hostname": "new"}, "ports": []any{}, "storage": []any{map[string]any{"id": "local-zfs", "path": "rpool/data"}}, "target_storage": []any{}, "requires_source_offline": true, "manual": []any{"Storage bleibt manuell"}}})
	m.form.fields[6].value = "ja"
	m, _ = key(m, "ctrl+s")
	if m.pending == nil || m.pending.path != "plans" {
		t.Fatal("manual mapping cannot reach reviewed plan")
	}
	p := m.pending.input.(anker.PlanRequest)
	if p.Mapping.Storage["local-zfs"] != "manual" || p.Mapping.Hostname != "new" {
		t.Fatalf("manual decisions lost: %+v", p.Mapping)
	}
}

func TestTerminalFileSelectionChangeClearsPreviousInspectionDecisions(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "backup"}}}
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/network/interfaces"
	m.form.fields[4].value = "eno1=ens3"
	m.form.fields[5].value = "ja"
	m.form.focus = 3
	m, _ = key(m, "x")
	if m.form.fields[4].value != "" || m.form.fields[5].value != "nein" {
		t.Fatal("old port mapping/console confirmation remained after selection changed")
	}
}

type terminalJournalCollector struct{ anker.DemoCollector }

func (c terminalJournalCollector) RestoreStatus(_ context.Context, _ anker.Host, p anker.Plan) (anker.ApplyResult, error) {
	return anker.ApplyResult{State: "applied", OperationID: p.ID, Applied: []string{"etc/test.conf"}, RollbackPath: "/var/lib/anker/rollback/" + p.ID}, nil
}
func (c terminalJournalCollector) Rollback(_ context.Context, _ anker.Host, p anker.Plan, confirm string) (anker.ApplyResult, error) {
	if confirm != p.ID {
		return anker.ApplyResult{}, fmt.Errorf("wrong confirmation")
	}
	return anker.ApplyResult{State: "rolled_back", OperationID: p.ID, Applied: []string{}, RollbackPath: "/var/lib/anker/rollback/" + p.ID}, nil
}
func TestTerminalReconcileAndConfirmedRollbackAgainstService(t *testing.T) {
	m, s := connected(t)
	hosts, _ := s.Hosts()
	p := anker.Plan{ID: "recover-terminal", TargetID: hosts[0].ID, Scenario: "files", State: "interrupted", Steps: []anker.Step{{Path: "etc/test.conf", Action: "apply"}}}
	if e := s.Store.Put("plans", p.ID, p); e != nil {
		t.Fatal(e)
	}
	s.Collector = terminalJournalCollector{anker.DemoCollector{Root: s.Root}}
	m.data["preview"] = map[string]any{"id": p.ID, "state": "interrupted"}
	m.showDetail("Wiederherstellungsplan", planText(object(m.data["preview"]), m))
	m, cmd := key(m, "r")
	m = complete(t, m, cmd)
	if str(object(m.data["preview"]), "state") != "checks_pending" || !strings.Contains(m.detail, p.ID) {
		t.Fatal("host evidence/state not displayed after reconciliation")
	}
	m, _ = key(m, "b")
	m, cmd = key(m, "enter")
	if cmd != nil {
		t.Fatal("rollback ran without typed plan ID")
	}
	m, _ = key(m, "b")
	m, _ = key(m, p.ID)
	m, cmd = key(m, "enter")
	m = complete(t, m, cmd)
	if str(object(m.data["preview"]), "id") != p.ID || str(object(m.data["preview"]), "state") != "rolling_back" {
		t.Fatal("queued rollback lost plan identity or claimed premature completion")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stored, _ := s.Plan(p.ID)
		if stored.State == "rolled_back" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	next, _ := m.Update(m.load()())
	m = next.(Model)
	if str(object(m.data["preview"]), "state") != "rolled_back" {
		t.Fatal("controlled rollback result not refreshed from service")
	}
}

func TestTerminalRecoveryMouseWheelSkipsHiddenMappingFields(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "backup"}}}
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/sysctl.d/a.conf"
	acceptInspectionForTest(&m)
	m.form.focus = 3
	next, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = next.(Model)
	if m.form.focus != len(m.form.fields) {
		t.Fatalf("wheel reached hidden recovery field %d", m.form.focus)
	}
	m, _ = key(m, "eno1=ens3")
	if m.form.fields[4].value != "" {
		t.Fatal("mouse wheel allowed editing an invisible port mapping")
	}
	next, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	m = next.(Model)
	if m.form.focus != 3 {
		t.Fatal("wheel cannot return to visible file selection")
	}
}
