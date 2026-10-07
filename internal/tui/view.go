package tui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"sort"
	"strings"
)

func (m Model) View() string {
	width := max(1, m.width)
	fit := func(s string) string { return ansi.Truncate(s, width, "…") }
	if m.height < 12 || width < 28 {
		return fit("Anker · Terminal vergrößern") + "\n" + fit("Mindestens 28 × 12 · Ctrl+C beendet")
	}
	names := m.tabLabels()
	names[m.tab] = accent.Bold(true).Underline(true).Render(names[m.tab])
	state := "Verbunden"
	if m.loading {
		state = "Aktualisieren …"
	}
	if m.loadErr != "" {
		state = "Dienst nicht erreichbar"
	}
	if boolean(m.data, "demo") {
		state += " · Demo"
	}
	lines := []string{accent.Bold(true).Render("Anker  /  Terminalverwaltung"), fit(state + "   ·   " + fmt.Sprintf("%d Hosts · %d Sicherungen · %d Aufträge", len(list(m.data["hosts"])), len(list(m.data["backups"])), len(list(m.data["jobs"])))), fit(strings.Join(names, "   ")), ""}
	body := []string{}
	helper := "↑/↓ Auswahl · Enter Details · Tab Ansicht · q Ende"
	actions := hotText(m.listActions())
	hint := "Maus: Ansicht, Zeile oder Aktion anklicken · Mausrad scrollt"
	switch {
	case m.pending != nil:
		a := m.pending
		body = append(body, strong.Render(clean(a.title)), "")
		available := max(1, m.bodyCapacity()-5)
		summary := wrap(clean(a.summary), width)
		if len(summary) > available {
			summary = append(summary[:max(1, available-1)], "… Vorschau mit Esc erneut ansehen")
		}
		body = append(body, summary...)
		if a.challenge != "" {
			body = append(body, "", challengeName(a)+" zur Bestätigung:", fit(clean(m.confirmation)+"▏"))
			helper = challengeName(a) + " vollständig eingeben · Enter bestätigen · Esc zurück"
		} else {
			helper = "←/→ Auswahl · Enter bestätigen · y Ja · Esc zurück"
		}
		left, right := "[ Abbrechen ]", "[ Bestätigen ]"
		if m.confirmYes {
			right = strong.Render(right)
		} else {
			left = strong.Render(left)
		}
		gap := max(2, width/2-ansi.StringWidth(left))
		actions = left + strings.Repeat(" ", gap) + right
		hint = "Die Änderung startet erst nach Ihrer Bestätigung."
	case m.form != nil:
		f := m.form
		body = append(body, strong.Render(clean(f.title)))
		start := m.formStart()
		count := max(1, (m.bodyCapacity()-1)/3)
		positions := m.visibleFormFields()
		for position := start; position < len(positions) && position < start+count; position++ {
			i := positions[position]
			e := f.fields[i]
			value := clean(e.value)
			if e.secret {
				value = strings.Repeat("•", min(40, len([]rune(e.value))))
			}
			if f.kind == "storage" && i == 0 {
				for _, raw := range list(f.original["volumes"]) {
					v := object(raw)
					if str(v, "id") == e.value {
						value = str(v, "mount") + " · " + str(v, "fs_type") + " · " + str(v, "source")
					}
				}
			}
			if e.label == "Rolle" {
				switch e.value {
				case "reader":
					value = "Lesen (reader)"
				case "restore":
					value = "Wiederherstellen (restore)"
				case "admin":
					value = "Administrator (admin)"
				}
			}
			if f.kind == "plan" && i == 0 {
				value = m.backupName(e.value)
			}
			if f.kind == "plan" && i == 1 {
				value = m.hostName(e.value) + " · " + e.value
			}
			if f.kind == "plan" && i == 2 {
				value = restoreScenarioLabel(e.value)
			}
			fieldHint := e.hint
			if e.multi {
				fieldHint = "←/→ Pfad · Leertaste wählen · Ctrl+E Eingabe · Ctrl+F neu laden"
				if e.editing {
					fieldHint = "Relative Pfade, kommagetrennt · Ctrl+E Liste · Ctrl+F neu laden"
				}
			}
			if e.multi && len(e.choices) > 0 && !e.editing {
				chosen := e.choices[e.option]
				check := "[ ]"
				for _, p := range strings.Split(e.value, ",") {
					if p == chosen {
						check = "[✓]"
					}
				}
				value = "‹ " + check + " " + chosen + " ›"
			} else if len(e.choices) > 0 && !e.editing {
				value = "‹ " + value + " ›"
			}
			prefix := "  "
			if i == f.focus {
				prefix = "› "
				value = strong.Render(value + "▏")
			}
			body = append(body, fit(prefix+e.label), fit("  "+value), fit("  "+clean(fieldHint)))
		}
		helper = "Tab/↑/↓ Feld · Ctrl+U leeren · ←/→ Auswahl · Esc zurück"
		actions = "[ Prüfen & weiter ]   Ctrl+S"
		if f.focus == len(f.fields) {
			actions = strong.Render(actions)
		}
		visiblePosition := len(positions)
		for position, index := range positions {
			if index == f.focus {
				visiblePosition = position + 1
				break
			}
		}
		hint = fmt.Sprintf("Feld %d/%d · Enter nächstes Feld; auf Weiter: Enter", visiblePosition, len(positions))
		if f.kind == "plan" && recoveryInspection(f) != nil {
			inspection := recoveryInspection(f)
			if boolean(inspection, "requires_console") {
				hint = "Netzwerk vorbereitet · Aktivierung manuell über Konsole; lokaler Rückfallwächter fehlt."
			}
		}
		if f.kind == "plan" && f.fields[2].value != "files" {
			hint = "Reale Hosts: manuell geführt · Export; keine automatische Gesamtausführung."
		}
	case m.detail != "":
		body = append(body, strong.Render(clean(m.detailTitle)))
		detail := m.detailLines()
		start := min(m.detailOffset, max(0, len(detail)-max(1, m.bodyCapacity()-1)))
		limit := max(1, m.bodyCapacity()-1)
		for i := start; i < len(detail) && i < start+limit; i++ {
			body = append(body, detail[i])
		}
		helper = "↑/↓ oder Mausrad scrollen · PgUp/PgDown · Esc zurück · q Ende"
		actions = hotText(m.detailActions())
		hint = fmt.Sprintf("Zeilen %d–%d von %d", start+1, min(len(detail), start+limit), len(detail))
	default:
		for i := m.offset; i < len(m.rows) && i < m.offset+m.capacity(); i++ {
			prefix := "  "
			row := single(m.rows[i])
			if i == m.cursor {
				prefix = "› "
				row = accent.Bold(true).Render(row)
			}
			body = append(body, fit(prefix+row))
		}
		if len(m.rows) == 0 {
			body = append(body, "Noch keine Einträge.")
			if m.tab == 0 {
				body = append(body, "a Hinzufügen öffnet die Hosteinrichtung.")
			}
		}
		if m.users {
			helper = "↑/↓ Auswahl · Enter Details · Esc Einstellungen · q Ende"
		}
	}
	if len(body) > m.bodyCapacity() {
		body = body[:m.bodyCapacity()]
	}
	for _, line := range body {
		lines = append(lines, fit(line))
	}
	for len(lines) < m.height-6 {
		lines = append(lines, "")
	}
	status := m.notice
	if m.err != "" {
		status = "Fehler: " + single(m.err)
	} else if m.loadErr != "" {
		status = "Verbindung: " + single(m.loadErr)
	}
	if m.busy {
		status = "Bitte warten · " + m.notice
		actions = "Aktion läuft …"
		hint = "Ctrl+C beendet die Terminaloberfläche."
	}
	lines = append(lines, "", fit(statusStyle(m.err != "" || m.loadErr != "").Render(single(status))), fit(muted.Render(helper)), fit(accent.Render(actions)), fit(muted.Render(hint)), "")
	return strings.Join(lines, "\n")
}
func (m Model) formStart() int {
	if m.form == nil {
		return 0
	}
	count := max(1, (m.bodyCapacity()-1)/3)
	positions := m.visibleFormFields()
	focus := len(positions) - 1
	for index, value := range positions {
		if value == m.form.focus {
			focus = index
			break
		}
	}
	return max(0, focus-count+1)
}
func (m Model) detailLines() []string { return wrap(clean(m.detail), max(1, m.width)) }
func wrap(s string, width int) []string {
	lines := []string{}
	for _, line := range strings.Split(s, "\n") {
		lines = append(lines, strings.Split(ansi.Hardwrap(line, max(1, width), true), "\n")...)
	}
	return lines
}

var labels = map[string]string{
	"environment":          "Umgebung",
	"volumes":              "Dateisysteme",
	"data_path":            "Datenverzeichnis",
	"can_grow":             "Automatisch erweiterbar",
	"steps":                "Schritte",
	"explanation":          "Erläuterung",
	"command":              "Geprüfter Vorgang",
	"device_bytes":         "Datenträgergröße",
	"filesystem_bytes":     "Dateisystemgröße",
	"source":               "Quelle",
	"total":                "Gesamt",
	"used":                 "Belegt",
	"available":            "Verfügbar",
	"fs_type":              "Dateisystemtyp",
	"read_only":            "Schreibgeschützt",
	"id":                   "ID",
	"name":                 "Name",
	"address":              "Adresse",
	"group":                "Gruppe",
	"cluster_id":           "Cluster",
	"ssh_user":             "SSH-Benutzer",
	"ssh_port":             "SSH-Port",
	"key_path":             "Sicherungsschlüssel",
	"restore_key_path":     "Restore-Schlüssel",
	"restore_ssh_user":     "Restore-Benutzer",
	"known_hosts_path":     "Known Hosts",
	"enabled":              "Aktiv",
	"schedule":             "Uhrzeit",
	"last_probe":           "Letzte Prüfung",
	"probe_error":          "Prüffehler",
	"inventory":            "Inventar",
	"host_id":              "Host-ID",
	"host_name":            "Host",
	"created_at":           "Erstellt",
	"started_at":           "Gestartet",
	"finished_at":          "Beendet",
	"verified_at":          "Geprüft",
	"verification_error":   "Prüffehler",
	"status":               "Status",
	"state":                "Zustand",
	"size":                 "Größe",
	"files":                "Dateien",
	"pinned":               "Angeheftet",
	"archived":             "Archiviert",
	"warnings":             "Hinweise",
	"kind":                 "Art",
	"attempts":             "Versuche",
	"error":                "Fehler",
	"result_id":            "Ergebnis-ID",
	"trigger":              "Auslöser",
	"role":                 "Rolle",
	"secrets":              "Geheimnisse lesen",
	"disabled":             "Gesperrt",
	"timezone":             "Zeitzone",
	"parallel":             "Parallele Aufträge",
	"daily":                "Tägliche Stände",
	"weekly":               "Wöchentliche Stände",
	"monthly":              "Monatliche Stände",
	"archive_days":         "Archivierung nach Tagen",
	"stale_hours":          "Veraltet nach Stunden",
	"current":              "Installierte Version",
	"version":              "Version",
	"repository":           "Quelle",
	"configured":           "Eingerichtet",
	"busy":                 "Läuft",
	"message":              "Hinweis",
	"checked_at":           "Letzte Updateprüfung",
	"updated_at":           "Letztes Update",
	"fingerprint":          "Fingerabdruck",
	"automatic":            "Automatische Erneuerung",
	"managed":              "Verwaltet",
	"expires_at":           "Gültig bis",
	"days_remaining":       "Verbleibende Tage",
	"renew_before_days":    "Vorlauf in Tagen",
	"names":                "DNS-Namen",
	"path":                 "Pfad",
	"type":                 "Typ",
	"hostname":             "Hostname",
	"pve_version":          "Proxmox-Version",
	"debian":               "Debian",
	"kernel":               "Kernel",
	"interfaces":           "Netzwerkports",
	"disks":                "Datenträger",
	"mount":                "Mountpoint",
	"free_bytes":           "Freier Speicher",
	"total_bytes":          "Gesamtspeicher",
	"used_bytes":           "Belegt",
	"notification_health":  "Benachrichtigungen",
	"scheduler_health":     "Zeitplan",
	"maintenance_health":   "Wartung",
	"official":             "Offizielle Quelle",
	"official_repository":  "Offizielle Quelle",
	"official_fingerprint": "Offizieller Fingerabdruck",
	"has_token":            "GitHub-Zugang konfiguriert",
}

func label(key string) string {
	if s := labels[key]; s != "" {
		return s
	}
	return strings.ReplaceAll(key, "_", " ")
}
func describe(v any) string {
	var lines []string
	var visit func(any, string, int)
	visit = func(value any, prefix string, depth int) {
		if len(lines) > 1200 {
			return
		}
		switch x := value.(type) {
		case map[string]any:
			if depth > 4 {
				lines = append(lines, prefix+"…")
				return
			}
			keys := make([]string, 0, len(x))
			for k := range x {
				if k != "details" && k != "password" && k != "smtp_password" && k != "password_hash" && k != "public_key" && k != "official_public_key" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				switch x[k].(type) {
				case map[string]any, []any:
					lines = append(lines, prefix+label(k)+":")
					visit(x[k], prefix+"  ", depth+1)
				default:
					lines = append(lines, prefix+label(k)+": "+displayValue(k, x[k]))
				}
			}
		case []any:
			if len(x) == 0 {
				lines = append(lines, prefix+"—")
			}
			for _, item := range x {
				visit(item, prefix, depth+1)
				lines = append(lines, "")
			}
		default:
			lines = append(lines, prefix+displayValue("", x))
		}
	}
	visit(v, "", 0)
	if len(lines) > 1200 {
		lines = append(lines[:1200], "… weitere Einträge über CLI abrufen")
	}
	return strings.Join(lines, "\n")
}
func displayValue(key string, v any) string {
	switch x := v.(type) {
	case nil:
		return "—"
	case bool:
		return yesNo(x)
	case string:
		if len(x) > 12000 {
			x = x[:12000] + "…"
		}
		if strings.HasSuffix(key, "_at") || key == "last_probe" {
			return date(x)
		}
		if x == "" {
			return "—"
		}
		return clean(x)
	case float64:
		if strings.HasSuffix(key, "bytes") || key == "size" || key == "total" || key == "used" || key == "available" {
			return size(int(x))
		}
		return fmt.Sprintf("%g", x)
	default:
		return single(fmt.Sprint(x))
	}
}
func restoreScenarioLabel(scenario string) string {
	if label := restoreScenarioLabels[scenario]; label != "" {
		return label
	}
	return scenario
}

var restoreScenarioLabels = map[string]string{
	"files":            "Einzelne Dateien",
	"standalone":       "Host nach Totalausfall",
	"migration":        "Andere Hardware",
	"version":          "Versionswechsel",
	"cluster-node":     "Ersatznode im Cluster",
	"cluster-disaster": "Vollständiger Clusterverlust",
	"topology":         "Standalone / Cluster wechseln",
}

func planText(p map[string]any, m Model) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Plan-ID: %s\nZustand: %s\nSicherung: %s\nZiel: %s (%s)\nSzenario: %s\n", str(p, "id"), str(p, "state"), str(p, "backup_id"), m.hostName(str(p, "target_id")), str(p, "target_id"), restoreScenarioLabel(str(p, "scenario")))
	if len(list(p["blockers"])) > 0 {
		b.WriteString("\nGESPERRT – vor Ausführung beheben:\n")
		for _, v := range list(p["blockers"]) {
			fmt.Fprintf(&b, "• %v\n", v)
		}
	} else {
		b.WriteString("\nKeine Plansperren. Der Dienst prüft das Ziel vor der Ausführung erneut.\n")
	}
	if result := object(p["result"]); result != nil {
		fmt.Fprintf(&b, "\nHostprotokoll: %s · Recovery-ID: %s\nRollbackpfad: %s\n%s\n", str(result, "state"), str(result, "operation_id"), str(result, "rollback_path"), str(result, "error"))
	}
	if recoveryMayHaveJournal(p) {
		b.WriteString("Hostprotokoll mit r abgleichen; keine Dateiübernahme wiederholen. Kontrollierte Rücksetzung mit b und ausdrücklicher Plan-ID. Dienste und Neustart bleiben separat zu prüfen.\n")
	}
	b.WriteString("\nGeplante Dateiänderungen:\n")
	for _, raw := range list(p["steps"]) {
		s := object(raw)
		fmt.Fprintf(&b, "• %s · %s\n  %s\n", str(s, "path"), str(s, "action"), str(s, "reason"))
		if str(s, "diff") != "" {
			b.WriteString(clean(str(s, "diff")) + "\n")
		}
		if boolean(s, "secret") {
			b.WriteString("  Inhalt geschützt; keine Geheimnisse in der Vorschau.\n")
		}
	}
	if len(list(p["manual"])) > 0 {
		b.WriteString("\nManuell zu prüfen:\n")
		for _, v := range list(p["manual"]) {
			fmt.Fprintf(&b, "• %v\n", v)
		}
	}
	if len(list(p["steps"])) == 0 {
		b.WriteString("  Keine automatisch ausführbaren Änderungen.\n")
	}
	return clean(b.String())
}

func (m Model) tabLabels() []string {
	if m.width < 100 {
		return []string{"1 Hosts", "2 Backups", "3 Restore", "4 Jobs", "5 System"}
	}
	return append([]string(nil), tabs...)
}

var accent = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "75"})
var muted = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})

func statusStyle(failed bool) lipgloss.Style {
	if failed {
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"})
	}
	return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "114"})
}

func challengeName(a *action) string {
	if a.challengeLabel != "" {
		return a.challengeLabel
	}
	return "Plan-ID"
}
