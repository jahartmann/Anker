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
