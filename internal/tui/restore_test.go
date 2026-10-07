package tui

import (
	"anker/internal/anker"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Recovery must remain reachable through the form, with its backend scenario
// intact; adding a label alone or silently falling back to migration is wrong.
func TestRestoreScenariosReachableThroughKeyboard(t *testing.T) {
	for _, tt := range []struct{ scenario, label string }{
		{"files", "Einzelne Dateien"},
		{"standalone", "Host nach Totalausfall"},
		{"migration", "Andere Hardware"},
		{"version", "Versionswechsel"},
		{"cluster-node", "Ersatznode im Cluster"},
		{"cluster-disaster", "Vollständiger Clusterverlust"},
		{"topology", "Standalone / Cluster wechseln"},
	} {
		t.Run(tt.scenario, func(t *testing.T) {
			m := New(nil)
			m.height = 40
			m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "backup"}}}
			m.openForm("plan", nil)
			m.form.focus = 2
			for i := 0; i < 7 && m.form.fields[2].value != tt.scenario; i++ {
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
				m = next.(Model)
			}
			if m.form.fields[2].value != tt.scenario {
				t.Fatal("scenario cannot be chosen with the keyboard")
			}
			if !strings.Contains(m.View(), tt.label) {
				t.Fatalf("scenario lacks understandable label: %s", m.View())
			}
			if tt.scenario != "files" && (!strings.Contains(m.View(), "manuell") || !strings.Contains(m.View(), "Export")) {
				t.Fatal("whole recovery must explain manual execution and export before submission")
			}
			m.form.fields[3].value = "etc/sysctl.d/99-anker.conf"
			acceptInspectionForTest(&m)
			m, _ = key(m, "ctrl+s")
			if m.pending == nil {
				t.Fatal("valid guided plan did not reach review")
			}
			if p, ok := m.pending.input.(anker.PlanRequest); !ok || p.Scenario != tt.scenario {
				t.Fatal("review does not preserve selected backend scenario")
			}
		})
	}
}

// The first/last path is 1,599 arrow presses apart. Direct input must work after
// the picker has loaded, remain visible, and survive polling and resizing.
func TestRestoreLargeFileListAllowsDirectPathEntry(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "backup"}}}
	m.openForm("plan", nil)
	items := make([]any, 1600)
	for i := range items {
		items[i] = map[string]any{"path": fmt.Sprintf("etc/config-%04d", i), "type": "file", "secret": false}
	}
	items[1599] = map[string]any{"path": "etc/sysctl.d/99-anker.conf", "type": "file", "secret": false}
	m.handleResult(commandResult{data: items, purpose: "planFiles:backup"})
	m.form.focus = 3
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = next.(Model)
	m, _ = key(m, "etc/sysctl.d/99-anker.conf")
	if m.form.fields[3].value != "etc/sysctl.d/99-anker.conf" || !strings.Contains(m.View(), "etc/sysctl.d/99-anker.conf") {
		t.Fatal("loaded file picker prevents direct path input or hides the input")
	}
	next, _ = m.Update(loaded{data: m.data, revision: m.revision})
	m = next.(Model)
	next, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)
	acceptInspectionForTest(&m)
	m, _ = key(m, "ctrl+s")
	if m.pending == nil {
		t.Fatal("entered path disappeared before review")
	}
	p := m.pending.input.(anker.PlanRequest)
	if len(p.Files) != 1 || p.Files[0] != "etc/sysctl.d/99-anker.conf" {
		t.Fatalf("wrong files in reviewed plan: %v", p.Files)
	}
}

func TestRestoreDirectPathEntryKeepsValidationAndPickerSelection(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "backup"}}}
	m.openForm("plan", nil)
	m.handleResult(commandResult{data: []any{map[string]any{"path": "etc/test.conf", "type": "file"}}, purpose: "planFiles:backup"})
	m.form.focus = 3
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = next.(Model)
	m, _ = key(m, "../etc/passwd")
	if m.form.fields[3].value != "../etc/passwd" {
		t.Fatal("direct editing mode did not accept the path to validate")
	}
	m, _ = key(m, "ctrl+s")
	if m.pending != nil || m.err == "" {
		t.Fatal("direct entry bypassed relative path validation")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	acceptInspectionForTest(&m)
	m, _ = key(m, "ctrl+s")
	if m.pending == nil || m.pending.input.(anker.PlanRequest).Files[0] != "etc/test.conf" {
		t.Fatal("returning to list mode broke existing file selection")
	}
}

func TestDirectRestorePathCreatesReviewedPlanAgainstService(t *testing.T) {
	m, _ := connected(t)
	m.switchTab(1)
	m, cmd := key(m, "r")
	m = complete(t, m, cmd)
	m.form.focus = 3
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = next.(Model)
	m, _ = key(m, "etc/sysctl.d/99-anker.conf")
	m, cmd = key(m, "ctrl+s")
	if m.pending != nil || cmd == nil {
		t.Fatal("direct path must be inspected without redundant read-only approval")
	}
	m = complete(t, m, cmd)
	m, _ = key(m, "ctrl+s")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	p := object(m.data["preview"])
	steps := list(p["steps"])
	if len(steps) != 1 || str(object(steps[0]), "path") != "etc/sysctl.d/99-anker.conf" {
		t.Fatal("direct input did not produce a plan for the requested file")
	}
	if m.pending != nil || str(p, "state") != "ready" {
		t.Fatal("creating the plan must not approve its execution")
	}
}

func TestGuidedClusterRecoveryRemainsManualAgainstService(t *testing.T) {
	m, svc := connected(t)
	svc.Demo = false
	m.switchTab(1)
	m, cmd := key(m, "r")
	m = complete(t, m, cmd)
	m.form.focus = 2
	for i := 0; i < 7 && m.form.fields[2].value != "cluster-disaster"; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = next.(Model)
	}
	if m.form.fields[2].value != "cluster-disaster" {
		t.Fatal("guided cluster recovery not reachable")
	}
	m.form.fields[5].value = "ja"
	m.form.fields[6].value = "ja"
	m, cmd = key(m, "ctrl+s")
	m = complete(t, m, cmd)
	inspection := recoveryInspection(m.form)
	if ports := list(inspection["ports"]); len(ports) > 0 {
		m.form.fields[4].value = str(object(ports[0]), "name") + "=" + str(object(list(inspection["target_ports"])[0]), "name")
	}
	m.form.fields[5].value, m.form.fields[6].value = "ja", "ja"
	m, _ = key(m, "ctrl+s")
	m, cmd = key(m, "y")
	m = complete(t, m, cmd)
	p := object(m.data["preview"])
	if str(p, "scenario") != "cluster-disaster" || (str(p, "state") != "manual" && str(p, "state") != "blocked") {
		t.Fatal("real-host cluster recovery must stay manual or blocked")
	}
	m, cmd = key(m, "a")
	if cmd != nil || m.pending != nil || m.err == "" {
		t.Fatal("guided scenario offered automatic whole-cluster recovery")
	}
}

func TestRestoreIdentityChangeClearsPreviousSafeguards(t *testing.T) {
	for _, tt := range []struct {
		name  string
		focus int
	}{
		{"source backup", 0},
		{"target host", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := New(nil)
			m.data = map[string]any{
				"hosts":   []any{map[string]any{"id": "target-a"}, map[string]any{"id": "target-b"}},
				"backups": []any{map[string]any{"id": "source-a"}, map[string]any{"id": "source-b"}},
			}
			m.openForm("plan", nil)
			m.form.fields[3].value = "etc/test.conf"
			m.form.fields[4].value = "eno1=ens3"
			m.form.fields[5].value = "ja"
			m.form.fields[6].value = "ja"
			m.form.focus = tt.focus
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
			m = next.(Model)
			if m.form.fields[tt.focus].value != []string{"source-b", "target-b"}[tt.focus] {
				t.Fatal("keyboard did not change recovery identity")
			}
			if m.form.fields[4].value != "" || m.form.fields[5].value != "nein" || m.form.fields[6].value != "nein" {
				t.Fatal("port mapping or confirmation silently migrated to another source/target")
			}
			if tt.focus == 0 && m.form.fields[3].value != "" {
				t.Fatal("paths from the previous backup remain selected")
			}
			if tt.focus == 1 && m.form.fields[3].value != "etc/test.conf" {
				t.Fatal("target change discarded unchanged backup file selection")
			}
		})
	}
}

func TestRestoreUnchangedIdentityPreservesDraft(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "target"}}, "backups": []any{map[string]any{"id": "source"}}}
	m.openForm("plan", nil)
	m.form.fields[3].value = "etc/test.conf"
	m.form.fields[4].value = "eno1=ens3"
	m.form.fields[5].value = "ja"
	m.form.fields[6].value = "ja"
	for _, focus := range []int{0, 1} {
		m.form.focus = focus
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = next.(Model)
		if m.form.fields[3].value != "etc/test.conf" || m.form.fields[4].value != "eno1=ens3" || m.form.fields[5].value != "ja" || m.form.fields[6].value != "ja" {
			t.Fatal("cycling a single unchanged source/target discarded the draft")
		}
	}
}

func acceptInspectionForTest(m *Model) {
	m.handleResult(commandResult{purpose: recoveryPurpose(m.form), data: map[string]any{"ports": []any{}, "storage": []any{}}})
}
