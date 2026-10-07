package tui

import (
	"anker/internal/client"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type loaded struct {
	data     map[string]any
	err      error
	revision int
}
type commandResult struct {
	data    any
	err     error
	purpose string
}
type tick time.Time

type action struct {
	trust                                                    *trustedHost
	challengeLabel                                           string
	title, summary, method, path, output, challenge, purpose string
	input                                                    any
	overwrite                                                bool
}
type field struct {
	label, value, hint string
	secret, required   bool
	choices            []string
	multi              bool
	editing            bool
	option             int
}
type form struct {
	kind, title string
	fields      []field
	focus       int
	original    map[string]any
	generation  int
}
type Model struct {
	client                                    *client.Client
	rows, ids                                 []string
	cursor, offset, tab, width, height        int
	data                                      map[string]any
	detail, detailTitle, err, loadErr, notice string
	detailOffset                              int
	loading, busy                             bool
	revision                                  int
	formGeneration                            int
	form                                      *form
	pending                                   *action
	confirmYes                                bool
	confirmation                              string
	users                                     bool
}

var tabs = []string{"1 Hosts", "2 Sicherungen", "3 Wiederherstellung", "4 Aufträge", "5 Einstellungen"}
var dataKeys = []string{"hosts", "backups", "plans", "jobs"}

func New(c *client.Client) Model {
	return Model{client: c, width: 100, height: 30, data: map[string]any{}, loading: c != nil}
}
func nextTick() tea.Cmd { return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return tick(t) }) }
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return tea.Batch(m.load(), nextTick())
}
func (m Model) load() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		var d map[string]any
		err := m.client.Call(ctx, "GET", "status", nil, &d)
		return loaded{data: d, err: err, revision: m.revision}
	}
}
func (m *Model) request(a action) tea.Cmd {
	if m.busy || m.client == nil {
		if m.client == nil {
			m.err = "Kein Dienst verbunden."
		}
		return nil
	}
	m.busy = true
	m.revision++
	if a.path == "updates/install" {
		delete(m.data, "preview")
	}
	m.err = ""
	m.notice = a.title + " …"
	c := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		var result any
		var err error
		if a.trust != nil {
			result, err = executeTrust(ctx, *a.trust)
		} else if a.output != "" {
			err = download(c, ctx, a.path, a.output, a.overwrite)
			result = map[string]any{"datei": a.output}
		} else {
			err = c.Call(ctx, a.method, a.path, a.input, &result)
		}
		return commandResult{result, err, a.purpose}
	}
}
func (m *Model) confirm(a action) {
	if m.busy {
		return
	}
	m.pending = &a
	m.confirmYes = false
	m.confirmation = ""
	m.err = ""
}
func (m Model) selected() map[string]any {
	if m.detail != "" && recordDetail(m.detailTitle) {
		return object(m.data["detailRecord"])
	}
	key := "users"
	if m.tab < 4 {
		key = dataKeys[m.tab]
	}
	items, _ := m.data[key].([]any)
	if m.cursor >= 0 && m.cursor < len(items) {
		v, _ := items[m.cursor].(map[string]any)
		return v
	}
	return nil
}
func (m *Model) buildRows() {
	previous := ""
	if m.cursor >= 0 && m.cursor < len(m.ids) {
		previous = m.ids[m.cursor]
	}
	m.rows = nil
	m.ids = nil
	key := "users"
	if m.tab < 4 {
		key = dataKeys[m.tab]
	}
	items, _ := m.data[key].([]any)
	if m.tab < 4 || m.users {
		for _, raw := range items {
			v, _ := raw.(map[string]any)
			if v == nil {
				continue
			}
			m.ids = append(m.ids, str(v, "id"))
			var row string
			switch m.tab {
			case 0:
				state := "aktiv"
				if !boolean(v, "enabled") {
					state = "pausiert"
				}
				if str(v, "probe_error") != "" {
					state = "Prüffehler"
				}
				row = fmt.Sprintf("%-22s %-23s %-12s %s", single(str(v, "name")), single(str(v, "address")), state, single(str(v, "group")))
			case 1:
				row = fmt.Sprintf("%-22s %-21s %-12s %s", single(str(v, "host_name")), date(str(v, "created_at")), str(v, "status"), size(number(v, "size")))
			case 2:
				row = fmt.Sprintf("%-26s %-12s %-14s %s", str(v, "id"), str(v, "scenario"), str(v, "state"), m.hostName(str(v, "target_id")))
			case 3:
				row = fmt.Sprintf("%-15s %-22s %-12s %s", str(v, "kind"), m.hostName(str(v, "host_id")), str(v, "state"), date(str(v, "created_at")))
			case 4:
				row = fmt.Sprintf("%-26s %-14s %s", str(v, "name"), str(v, "role"), yesNo(!boolean(v, "disabled")))
			}
			m.rows = append(m.rows, single(row))
		}
	} else {
		m.ids = []string{"schedule", "system", "tls", "storage", "updates", "users"}
		m.rows = []string{"Zeitplan & Aufbewahrung     Zeiten, Parallelität und Rotation", "Systemübersicht              Dienstzustand und Diagnose", "TLS-Zertifikat               Status, Erneuerung und Automatik", "Speicher                     Kapazität und Datenträger", "Updates                      Quelle einrichten, prüfen, installieren", "Benutzer                     Konten, Rollen und Passwörter"}
	}
	if previous != "" {
		for i, id := range m.ids {
			if id == previous {
				m.cursor = i
				break
			}
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
	m.ensureVisible()
}
func (m Model) hostName(id string) string {
	items, _ := m.data["hosts"].([]any)
	for _, raw := range items {
		v, _ := raw.(map[string]any)
		if str(v, "id") == id {
			return single(str(v, "name"))
		}
	}
	return single(id)
}
func (m Model) backupName(id string) string {
	for _, raw := range list(m.data["backups"]) {
		v := object(raw)
		if str(v, "id") == id {
			return single(str(v, "host_name")) + " · " + date(str(v, "created_at")) + " · " + id
		}
	}
	return single(id)
}
func (m *Model) switchTab(tab int) {
	m.tab = tab
	m.users = false
	m.cursor = 0
	m.offset = 0
	m.detail = ""
	m.detailOffset = 0
	m.buildRows()
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(1, v.Width)
		m.height = max(1, v.Height)
	case loaded:
		m.loading = false
		if v.revision != m.revision {
			if !m.busy && m.client != nil {
				m.loading = true
				return m, m.load()
			}
			return m, nil
		}
		if v.err != nil {
			m.loadErr = v.err.Error()
		} else {
			m.data = mergeData(m.data, v.data)
			m.loadErr = ""
			m.buildRows()
		}
	case commandResult:
		if m.enrollmentResult(v) {
			if v.err == nil && strings.HasPrefix(v.purpose, "hostEnroll:") && m.form == nil && m.client != nil {
				m.loading = true
				return m, m.load()
			}
			return m, nil
		}
		m.busy = false
		m.notice = ""
		if v.err != nil {
			m.err = v.err.Error()
			return m, nil
		}
		m.err = ""
		m.revision++
		m.handleResult(v)
		if v.purpose == "usersChanged" {
			return m, m.request(action{title: "Benutzer laden", method: "GET", path: "users", purpose: "users"})
		}
		if !m.loading && m.client != nil {
			m.loading = true
			return m, m.load()
		}
	case tick:
		if !m.loading && !m.busy && m.client != nil {
			m.loading = true
			return m, tea.Batch(m.load(), nextTick())
		}
		return m, nextTick()
	case tea.MouseMsg:
		if m.busy {
			return m, nil
		}
		if v.Button == tea.MouseButtonWheelUp {
			return m, m.move(-1)
		}
		if v.Button == tea.MouseButtonWheelDown {
			return m, m.move(1)
		}
		if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
			if m.pending != nil {
				if v.Y == m.height-3 {
					m.confirmYes = v.X >= m.width/2
					if m.confirmYes {
						return m, m.acceptConfirmation()
					}
					if !m.confirmYes {
						m.cancelConfirmation()
					}
				}
			} else if m.form != nil {
				start := m.formStart()
				index := start + (v.Y-5)/3
				if v.Y >= 5 && index >= 0 && index < len(m.form.fields) && v.Y < m.height-4 {
					m.form.focus = index
					if (v.Y-5)%3 == 1 && len(m.form.fields[index].choices) > 0 {
						return m, m.formKey(tea.KeyMsg{Type: tea.KeySpace})
					}
				}
				if v.Y == m.height-3 {
					return m, m.submitForm()
				}
			} else if m.detail != "" { // Detail actions stay visible and are also reachable with keys.
				if v.Y == m.height-3 {
					return m, m.detailMouse(v.X)
				}
			} else if v.Y == 2 {
				x := 0
				for i, label := range m.tabLabels() {
					end := x + ansi.StringWidth(label) + 3
					if v.X >= x && v.X < end {
						m.switchTab(i)
						break
					}
					x = end
				}
			} else if v.Y >= 4 && v.Y < 4+m.capacity() {
				index := m.offset + v.Y - 4
				if index >= 0 && index < len(m.rows) {
					if m.cursor == index {
						return m, m.listKey("enter")
					}
					m.cursor = index
				}
			} else if v.Y == m.height-3 {
				return m, m.actionMouse(v.X)
			}
		}
	case tea.KeyMsg:
		if v.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.pending != nil {
			return m, m.confirmKey(v)
		}
		if m.form != nil {
			return m, m.formKey(v)
		}
		if m.detail != "" {
			switch v.String() {
			case "esc":
				if !m.busy {
					m.detail = ""
					m.detailOffset = 0
				}
			case "q":
				return m, tea.Quit
			case "up", "k":
				m.detailOffset = max(0, m.detailOffset-1)
			case "down", "j":
				m.detailOffset++
			case "pgdown":
				m.detailOffset += m.bodyCapacity()
			case "pgup":
				m.detailOffset = max(0, m.detailOffset-m.bodyCapacity())
			case "home":
				m.detailOffset = 0
			default:
				return m, m.detailKey(v.String())
			}
			m.clampDetail()
			return m, nil
		}
		switch v.String() {
		case "q":
			return m, tea.Quit
		case "down", "j":
			return m, m.move(1)
		case "up", "k":
			return m, m.move(-1)
		case "pgdown":
			return m, m.move(m.capacity())
		case "pgup":
			return m, m.move(-m.capacity())
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = max(0, len(m.rows)-1)
		case "tab":
			if !m.busy {
				m.switchTab((m.tab + 1) % 5)
			}
		case "shift+tab":
			if !m.busy {
				m.switchTab((m.tab + 4) % 5)
			}
		case "1", "2", "3", "4", "5":
			if !m.busy {
				m.switchTab(int(v.String()[0] - '1'))
			}
		case "esc":
			if m.users {
				m.users = false
				m.cursor = 0
				m.buildRows()
			}
		default:
			return m, m.listKey(v.String())
		}
	}
	m.ensureVisible()
	return m, nil
}
func (m *Model) move(delta int) tea.Cmd {
	if m.pending != nil {
		return nil
	}
	if m.form != nil {
		m.form.focus = max(0, min(len(m.form.fields)-1, m.form.focus+delta))
		return nil
	}
	if m.detail != "" {
		m.detailOffset = max(0, m.detailOffset+delta)
		m.clampDetail()
		return nil
	}
	m.cursor = max(0, min(len(m.rows)-1, m.cursor+delta))
	m.ensureVisible()
	return nil
}
func (m *Model) confirmKey(k tea.KeyMsg) tea.Cmd {
	if m.busy {
		return nil
	}
	switch k.String() {
	case "esc":
		m.cancelConfirmation()
	case "n":
		if m.pending.challenge == "" {
			m.cancelConfirmation()
		} else {
			m.confirmation += "n"
			m.confirmYes = true
		}
	case "left", "right", "tab", "shift+tab":
		m.confirmYes = !m.confirmYes
	case "y":
		if m.pending.challenge == "" {
			m.confirmYes = true
			return m.acceptConfirmation()
		} else {
			m.confirmation += "y"
			m.confirmYes = true
		}
	case "enter":
		return m.acceptConfirmation()
	case "backspace":
		rr := []rune(m.confirmation)
		if len(rr) > 0 {
			m.confirmation = string(rr[:len(rr)-1])
		}
	default:
		if m.pending.challenge != "" && k.Type == tea.KeyRunes && len(m.confirmation) < 4096 {
			m.confirmation += single(string(k.Runes))
			m.confirmYes = true
		}
	}
	return nil
}
func (m *Model) acceptConfirmation() tea.Cmd {
	if !m.confirmYes {
		m.cancelConfirmation()
		return nil
	}
	if m.pending.challenge != "" && m.confirmation != m.pending.challenge {
		label := m.pending.challengeLabel
		if label == "" {
			label = "Plan-ID"
		}
		m.err = "Zur Bestätigung " + label + " vollständig eingeben."
		return nil
	}
	a := *m.pending
	if strings.HasPrefix(a.purpose, "hostEnroll:") {
		if m.form == nil || a.purpose != enrollmentPurpose("hostEnroll", m.form) || m.form.fields[2].value == "" {
			m.cancelConfirmation()
			m.err = "SSH-Zugang erneut eingeben und Identität neu prüfen."
			return nil
		}
		input := clone(object(a.input))
		input["password"] = m.form.fields[2].value
		a.input = input
		m.clearEnrollmentSecret()
	}
	m.pending = nil
	m.confirmation = ""
	return m.request(a)
}
func (m *Model) handleResult(v commandResult) {
	if strings.HasPrefix(v.purpose, "planFiles:") {
		if m.form != nil && m.form.kind == "plan" && m.form.fields[0].value == strings.TrimPrefix(v.purpose, "planFiles:") {
			entry := &m.form.fields[3]
			entry.choices = nil
			entry.option = 0
			for _, raw := range list(v.data) {
				item := object(raw)
				path := str(item, "path")
				if path != "" && str(item, "type") == "file" {
					entry.choices = append(entry.choices, path)
				}
			}
			entry.multi = true
			entry.hint = "←/→ Pfad · Leertaste wählen · Ctrl+F Liste neu laden"
		}
		return
	}
	if strings.HasPrefix(v.purpose, "form:") {
		m.openForm(strings.TrimPrefix(v.purpose, "form:"), object(v.data))
		return
	}
	switch v.purpose {
	case "updateSource":
		cfg := object(v.data)
		repo := str(cfg, "official_repository")
		fingerprint := str(cfg, "official_fingerprint")
		if repo == "" || fingerprint == "" {
			m.err = "Der Dienst liefert keinen verifizierbaren offiziellen Schlüssel."
			return
		}
		m.confirm(action{title: "Offizielle Updatequelle einrichten", summary: "Quelle: " + repo + "\nVertrauter Ed25519-Fingerabdruck: " + fingerprint + "\nBisherige Quelle: " + str(cfg, "repository") + "\nDieser mit Anker gelieferte Schlüssel wird künftig zur Releaseprüfung verwendet.", method: "POST", path: "updates/configure", input: map[string]any{"repository": repo, "public_key": "", "confirmed": true}, purpose: "updateConfigure"})
	case "users":
		m.data["users"] = v.data
		m.users = true
		m.cursor = 0
		m.offset = 0
		m.detail = ""
		m.buildRows()
	case "plan":
		m.form = nil
		p := object(v.data)
		m.data["preview"] = p
		m.showDetail("Wiederherstellungsplan", planText(p, *m))
		m.tab = 2
		m.users = false
		m.cursor = 0
		m.offset = 0
		m.buildRows()
	case "storagePlan":
		m.form = nil
		m.data["growthPlan"] = v.data
		m.showDetail("Speicherplan", describe(v.data))
	case "storageGrow":
		m.form = nil
		delete(m.data, "growthPlan")
		m.showDetail("Speichererweiterung", describe(v.data))
	case "trust":
		m.form = nil
		m.detail = ""
		m.notice = str(object(v.data), "message")
	case "tls", "storage", "updates", "system":
		m.form = nil
		m.data[v.purpose] = v.data
		m.showDetail(map[string]string{"tls": "TLS-Zertifikat", "storage": "Speicher", "updates": "Updates", "system": "Systemübersicht"}[v.purpose], describe(v.data))
	case "files":
		m.showDetail("Gesicherte Dateien", describe(v.data))
	default:
		m.form = nil
		if recordDetail(m.detailTitle) || v.purpose == "apply" {
			m.detail = ""
			m.detailOffset = 0
		}
		if v.purpose == "apply" {
			delete(m.data, "preview")
		}
		m.notice = "Erfolgreich abgeschlossen."
		if v.purpose == "usersChanged" {
			m.users = true
			m.detail = ""
			return
		}
		if p := object(v.data); str(p, "id") != "" {
			m.notice = "Auftrag / Ergebnis: " + str(p, "id") + " · " + str(p, "state")
		}
		if v.purpose == "export" {
			m.notice = "Export gespeichert: " + str(object(v.data), "datei")
		}
		if v.purpose == "updateConfigure" {
			m.data["updates"] = v.data
			m.showDetail("Updatequelle eingerichtet", describe(v.data))
		}
	}
}
func (m *Model) showDetail(title, text string) {
	m.detailTitle = title
	m.detail = text
	m.detailOffset = 0
}
func (m Model) capacity() int     { return max(1, m.height-10) }
func (m Model) bodyCapacity() int { return max(1, m.height-10) }
func (m *Model) ensureVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.capacity() {
		m.offset = m.cursor - m.capacity() + 1
	}
	m.offset = max(0, m.offset)
}
func (m *Model) clampDetail() {
	m.detailOffset = min(m.detailOffset, max(0, len(m.detailLines())-max(1, m.bodyCapacity()-1)))
}
func clean(s string) string {
	// Strip escape sequences before controls; otherwise CSI suffixes look like data.
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.Is(unicode.Cf, r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, s)
}
func single(s string) string { return strings.Join(strings.Fields(clean(s)), " ") }
func str(v map[string]any, k string) string {
	if s, ok := v[k].(string); ok {
		if len(s) > 4096 {
			return s[:4096]
		}
		return s
	}
	return ""
}
func boolean(v map[string]any, k string) bool { b, _ := v[k].(bool); return b }
func number(v map[string]any, k string) int   { f, _ := v[k].(float64); return int(f) }
func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}
func mergeData(old, next map[string]any) map[string]any {
	d := map[string]any{}
	for k, v := range old {
		d[k] = v
	}
	for k, v := range next {
		d[k] = v
	}
	return d
}
func yesNo(b bool) string {
	if b {
		return "ja"
	}
	return "nein"
}
func date(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return single(s)
	}
	return t.Local().Format("02.01.2006 15:04")
}
func size(n int) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	}
	if n >= 1<<20 {
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d B", n)
}
func segment(s string) string { return url.PathEscape(s) }
func Run(c *client.Client) error {
	_, err := tea.NewProgram(New(c), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

var strong = lipgloss.NewStyle().Bold(true)
