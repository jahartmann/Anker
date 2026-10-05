package tui

import (
	tea "github.com/charmbracelet/bubbletea"
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
	next, _ = v.Update(tea.MouseMsg{Y: 7, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	v = next.(Model)
	if v.cursor != 2 {
		t.Fatal(v.cursor)
	}
}
