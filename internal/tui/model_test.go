package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func TestKeyboardAndMouseNavigation(t *testing.T) {
	m := New(nil)
	m.rows = []string{"one", "two", "three"}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	v := next.(Model)
	if v.cursor != 1 {
		t.Fatal(v.cursor)
	}
	next, _ = v.Update(tea.MouseMsg{Y: 6, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	v = next.(Model)
	if v.cursor != 2 {
		t.Fatal(v.cursor)
	}
}

func TestFortyHostsScrollIntoView(t *testing.T) {
	m := New(nil)
	m.height = 20
	for i := 0; i < 40; i++ {
		m.rows = append(m.rows, fmt.Sprintf("host-%02d", i))
	}
	for i := 0; i < 39; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(Model)
	}
	if !strings.Contains(m.View(), "host-39") || strings.Contains(m.View(), "host-00") {
		t.Fatal("selected host is invisible")
	}
	next, _ := m.Update(tea.MouseMsg{Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if next.(Model).cursor != m.offset {
		t.Fatal("mouse offset ignored")
	}
}

func TestAddHostOpensGuidedFormAndSurvivesRefresh(t *testing.T) {
	m := New(nil)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(Model)
	if cmd != nil || !strings.Contains(m.View(), "Host hinzufügen") || !strings.Contains(m.View(), "SSH-Port") {
		t.Fatal("add must open a guided host form")
	}
	next, _ = m.Update(loaded{data: map[string]any{"hosts": []any{map[string]any{"id": "new", "name": "server"}}}})
	if !strings.Contains(next.(Model).View(), "Host hinzufügen") {
		t.Fatal("refresh dismissed form")
	}
}

func TestBackupRequiresExplicitConfirmation(t *testing.T) {
	m := New(nil)
	m.data = map[string]any{"hosts": []any{map[string]any{"id": "host1", "name": "server", "address": "10.0.0.1"}}}
	m.buildRows()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if cmd != nil || !strings.Contains(next.(Model).View(), "Sicherung starten") || !strings.Contains(next.(Model).View(), "Abbrechen") {
		t.Fatal("backup must first show confirmation")
	}
}

func TestUntrustedTerminalControlsCannotEscapeText(t *testing.T) {
	got := clean("ok\x1b[2J\x9b31m\u202eevil\x7f")
	if strings.ContainsAny(got, "\x1b\x9b\u202e\x7f") {
		t.Fatalf("unsafe controls: %q", got)
	}
}
