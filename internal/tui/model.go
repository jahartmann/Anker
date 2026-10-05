package tui

import (
	"anker/internal/client"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"time"
)

type loaded struct {
	data map[string]any
	err  error
}
type commandResult struct {
	text string
	err  error
}
type tick time.Time
type Model struct {
	client  *client.Client
	rows    []string
	ids     []string
	cursor  int
	tab     int
	width   int
	height  int
	data    map[string]any
	input   string
	editing bool
	detail  string
	err     string
}

func New(c *client.Client) Model {
	return Model{client: c, width: 100, height: 30, rows: []string{}, data: map[string]any{}}
}
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return tea.Batch(m.load(), tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tick(t) }))
}
func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		var d map[string]any
		err := m.client.Call(context.Background(), "GET", "status", nil, &d)
		return loaded{d, err}
	}
}
func (m Model) execute(cmd string) tea.Cmd {
	return func() tea.Msg {
		var b bytes.Buffer
		c := *m.client
		c.Out = &b
		err := c.Run(context.Background(), strings.Fields(cmd))
		return commandResult{b.String(), err}
	}
}
func (m *Model) buildRows() {
	m.rows = nil
	m.ids = nil
	key := []string{"hosts", "backups", "plans", "jobs", "settings"}[m.tab]
	items, _ := m.data[key].([]any)
	for _, raw := range items {
		v, _ := raw.(map[string]any)
		id, _ := v["id"].(string)
		m.ids = append(m.ids, id)
		switch m.tab {
		case 0:
			m.rows = append(m.rows, fmt.Sprintf("%-24v %-20v %v", v["name"], v["address"], v["group"]))
		case 1:
			m.rows = append(m.rows, fmt.Sprintf("%-25v %-23v %v", v["host_name"], v["created_at"], v["status"]))
		case 2:
			m.rows = append(m.rows, fmt.Sprintf("%-28v %-20v %v", v["id"], v["scenario"], v["state"]))
		case 3:
			m.rows = append(m.rows, fmt.Sprintf("%-24v %-20v %v", v["kind"], v["host_id"], v["state"]))
		}
	}
	if m.tab == 4 {
		m.rows = []string{"Einstellungen über / settings show anzeigen und / settings save DATEI.json ändern.", "Benutzer: / user list · / user add NAME ROLE", "Alle Befehle: / help"}
		m.ids = nil
	}
	if m.cursor >= len(m.rows) {
		m.cursor = 0
	}
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
	case loaded:
		if v.err != nil {
			m.err = v.err.Error()
		} else {
			m.data = v.data
			m.err = ""
			m.buildRows()
		}
	case commandResult:
		if v.err != nil {
			m.err = v.err.Error()
		} else {
			m.detail = v.text
			m.err = ""
		}
		return m, m.load()
	case tick:
		return m, tea.Batch(m.load(), tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tick(t) }))
	case tea.MouseMsg:
		if !m.editing && v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
			index := v.Y - 5
			if index >= 0 && index < len(m.rows) {
				m.cursor = index
			}
		}
	case tea.KeyMsg:
		if m.editing {
			switch v.Type {
			case tea.KeyEsc:
				m.editing = false
				m.input = ""
			case tea.KeyEnter:
				m.editing = false
				cmd := m.input
				m.input = ""
				if m.client != nil {
					return m, m.execute(cmd)
				}
			case tea.KeyBackspace:
				rr := []rune(m.input)
				if len(rr) > 0 {
					m.input = string(rr[:len(rr)-1])
				}
			case tea.KeyRunes:
				m.input += string(v.Runes)
			case tea.KeySpace:
				m.input += " "
			}
			return m, nil
		}
		switch v.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "tab":
			m.tab = (m.tab + 1) % 5
			m.cursor = 0
			m.detail = ""
			m.buildRows()
		case "1", "2", "3", "4", "5":
			m.tab = int(v.String()[0] - '1')
			m.cursor = 0
			m.detail = ""
			m.buildRows()
		case "/", ":":
			m.editing = true
		case "esc":
			m.detail = ""
		case "enter":
			if m.cursor < len(m.ids) {
				key := []string{"hosts", "backups", "plans", "jobs"}[m.tab]
				items, _ := m.data[key].([]any)
				if m.cursor < len(items) {
					b, _ := json.MarshalIndent(items[m.cursor], "", "  ")
					m.detail = string(b)
				}
			}
		case "b":
			if m.tab == 0 && m.cursor < len(m.ids) && m.client != nil {
				return m, m.execute("backup run " + m.ids[m.cursor])
			}
		case "v":
			if m.tab == 1 && m.cursor < len(m.ids) && m.client != nil {
				return m, m.execute("backup verify " + m.ids[m.cursor])
			}
		case "a":
			m.editing = true
			m.input = "host add --name  --address "
		}
	}
	return m, nil
}
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}
func (m Model) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("Anker")
	tabs := []string{"1 Hosts", "2 Sicherungen", "3 Wiederherstellung", "4 Aufträge", "5 Einstellungen"}
	tabs[m.tab] = lipgloss.NewStyle().Bold(true).Underline(true).Render(tabs[m.tab])
	var b strings.Builder
	b.WriteString(title + "\n\n" + strings.Join(tabs, "   ") + "\n\n")
	for i, row := range m.rows {
		prefix := "  "
		if i == m.cursor {
			prefix = "› "
		}
		if i > m.height-12 {
			break
		}
		if i == m.cursor {
			row = lipgloss.NewStyle().Bold(true).Render(clean(row))
		}
		b.WriteString(prefix + row + "\n")
	}
	if len(m.rows) == 0 {
		b.WriteString("  Noch keine Einträge. / help zeigt verfügbare Befehle.\n")
	}
	if m.detail != "" {
		text := clean(m.detail)
		lines := strings.Split(text, "\n")
		limit := m.height - len(m.rows) - 10
		if limit < 4 {
			limit = 4
		}
		if len(lines) > limit {
			lines = lines[:limit]
			lines = append(lines, "… vollständige Ausgabe über CLI abrufen")
		}
		b.WriteString("\n" + strings.Join(lines, "\n") + "\n")
	}
	if m.err != "" {
		b.WriteString("\nFehler: " + clean(m.err) + "\n")
	}
	if m.editing {
		b.WriteString("\n/ " + m.input + "▏\n")
	} else {
		b.WriteString("\n↑/↓ auswählen · Enter Details · b sichern · v prüfen · / Befehl · Tab Ansicht · q Ende\n")
	}
	return b.String()
}
func Run(c *client.Client) error {
	_, err := tea.NewProgram(New(c), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
