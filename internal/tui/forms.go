package tui

import (
	"anker/internal/anker"
	"anker/internal/client"
	"context"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func textField(label, value, hint string, required bool) field {
	return field{label: label, value: value, hint: hint, required: required}
}
func choiceField(label, value string, choices ...string) field {
	if value == "" && len(choices) > 0 {
		value = choices[0]
	}
	return field{label: label, value: value, choices: choices, required: true, hint: "←/→ oder Leertaste wählen"}
}
func intValue(v map[string]any, key string, fallback int) string {
	n := number(v, key)
	if _, exists := v[key]; !exists {
		n = fallback
	}
	return strconv.Itoa(n)
}
func defaultText(v map[string]any, key, fallback string) string {
	if s := str(v, key); s != "" {
		return s
	}
	return fallback
}
func boolValue(v map[string]any, key string, fallback bool) string {
	b := fallback
	if _, ok := v[key]; ok {
		b = boolean(v, key)
	}
	return yesNo(b)
}
func (m *Model) openForm(kind string, v map[string]any) {
	f := &form{kind: kind, original: clone(v)}
	switch kind {
	case "host":
		restoreKey, restoreUser := str(v, "restore_key_path"), str(v, "restore_ssh_user")
		if str(v, "id") == "" {
			restoreKey = "/etc/anker/keys/restore"
			restoreUser = "anker-restore"
		}
		f.title = "Host hinzufügen"
		if str(v, "id") != "" {
			f.title = "Host bearbeiten"
		}
		f.fields = []field{
			textField("Name", str(v, "name"), "Hostname des Servers", true),
			textField("Adresse", str(v, "address"), "IP-Adresse oder DNS-Name", true),
			textField("Gruppe", str(v, "group"), "Optional", false),
			textField("Cluster", str(v, "cluster_id"), "Optional", false),
			textField("SSH-Benutzer", defaultText(v, "ssh_user", "anker"), "Lesender Sicherungszugang", true),
			textField("SSH-Port", intValue(v, "ssh_port", 22), "1–65535", true),
			textField("Sicherungsschlüssel", defaultText(v, "key_path", "/etc/anker/keys/backup"), "Pfad auf dem Anker-Server", true),
			textField("Known Hosts", defaultText(v, "known_hosts_path", "/etc/anker/known_hosts"), "Vorher vertrauenswürdig hinterlegte Hostschlüssel", true),
			textField("Restore-Benutzer", restoreUser, "Separater schreibender Zugang", false),
			textField("Restore-Schlüssel", restoreKey, "Getrennt vom Sicherungsschlüssel", false),
			textField("Uhrzeit", str(v, "schedule"), "HH:MM; leer übernimmt den globalen Zeitplan", false),
			choiceField("Zeitplan aktiv", boolValue(v, "enabled", true), "ja", "nein"),
		}
	case "plan":
		f.title = "Wiederherstellung planen"
		var backups, hosts []string
		for _, raw := range list(m.data["backups"]) {
			backups = append(backups, str(object(raw), "id"))
		}
		for _, raw := range list(m.data["hosts"]) {
			hosts = append(hosts, str(object(raw), "id"))
		}
		f.fields = []field{
			choiceField("Sicherung", str(v, "backup_id"), backups...),
			choiceField("Zielhost", str(v, "target_id"), hosts...),
			choiceField("Szenario", defaultText(v, "scenario", "files"), "files", "migration"),
			textField("Dateien", str(v, "files"), "Relative Pfade, kommagetrennt (für files erforderlich)", false),
			textField("Netzwerkports", str(v, "ports"), "Migration: eno1=ens3,eno2=ens4", false),
			choiceField("Konsolenzugang bestätigt", "nein", "nein", "ja"),
			choiceField("Quellhost ausgeschaltet / isoliert", "nein", "nein", "ja"),
		}
	case "settings":
		f.title = "Zeitplan & Aufbewahrung"
		f.fields = []field{
			textField("Zeitzone", str(v, "timezone"), "IANA, z.B. Europe/Berlin oder UTC", true),
			textField("Uhrzeit", str(v, "schedule"), "Täglich HH:MM", true),
			textField("Parallele Aufträge", intValue(v, "parallel", 2), "1–16", true),
			textField("Wiederholungen", intValue(v, "retries", 2), "0–5", true),
			textField("Tägliche Sicherungen", intValue(v, "daily", 7), "Anzahl aufzubewahrender Stände", true),
			textField("Wöchentliche Sicherungen", intValue(v, "weekly", 4), "Anzahl aufzubewahrender Stände", true),
			textField("Monatliche Sicherungen", intValue(v, "monthly", 12), "Anzahl aufzubewahrender Stände", true),
			textField("Archiv nach Tagen", intValue(v, "archive_days", 90), "Schwelle für Archivierung", true),
			textField("Veraltet nach Stunden", intValue(v, "stale_hours", 26), "Überwachung der Sicherungsfrische", true),
		}
	case "storage":
		f.title = "Dateisystem für Erweiterung wählen"
		choices := []string{}
		for _, raw := range list(v["volumes"]) {
			volume := object(raw)
			if str(volume, "id") != "" {
				choices = append(choices, str(volume, "id"))
			}
		}
		f.fields = []field{choiceField("Dateisystem", "", choices...)}
	case "trust":
		f.title = "SSH-Hostschlüssel vertrauen"
		f.fields = []field{textField("Adresse", str(v, "address"), "Hostadresse; root auf dem Anker-Server erforderlich", true), textField("SSH-Port", intValue(v, "ssh_port", 22), "1–65535", true), textField("SHA256-Fingerprint", "", "Unabhängig auf der Proxmox-Konsole prüfen", true)}
	case "tls":
		f.title = "TLS-Automatik einstellen"
		f.fields = []field{choiceField("Automatisch erneuern", boolValue(v, "automatic", true), "ja", "nein"), textField("Vorlauf in Tagen", intValue(v, "renew_before_days", 30), "7–90 Tage", true)}
	case "user":
		f.title = "Benutzer hinzufügen"
		f.fields = []field{textField("Name", "", "Neues Webkonto", true), choiceField("Rolle", "reader", "reader", "restore", "admin"), choiceField("Geheimnisse lesen", "nein", "nein", "ja"), {label: "Passwort", secret: true, required: true, hint: "Verdeckte Eingabe; Passwortregeln des Dienstes gelten"}, {label: "Passwort wiederholen", secret: true, required: true}}
	case "userEdit":
		f.title = "Benutzerrechte ändern"
		f.fields = []field{choiceField("Rolle", str(v, "role"), "reader", "restore", "admin"), choiceField("Geheimnisse lesen", boolValue(v, "secrets", false), "nein", "ja")}
	case "password":
		f.title = "Passwort ändern · " + single(str(v, "name"))
		f.fields = []field{{label: "Neues Passwort", secret: true, required: true, hint: "Verdeckte Eingabe"}, {label: "Passwort wiederholen", secret: true, required: true}}
	}
	m.form = f
	m.pending = nil
	m.err = ""
}
func (m *Model) openExport(path, name string) {
	m.form = &form{kind: "export", title: "Lokal exportieren", original: map[string]any{"path": path}, fields: []field{textField("Zieldatei", defaultExport(name), "Pfad auf dem Rechner, auf dem anker läuft (bei SSH: Server)", true), choiceField("Vorhandene Datei ersetzen", "nein", "nein", "ja")}}
	m.err = ""
}
func (m *Model) formKey(k tea.KeyMsg) tea.Cmd {
	if m.busy {
		return nil
	}
	f := m.form
	switch k.String() {
	case "esc":
		for i := range f.fields {
			f.fields[i].value = ""
		}
		m.form = nil
		m.err = ""
		return nil
	case "tab", "down":
		f.focus = (f.focus + 1) % (len(f.fields) + 1)
	case "shift+tab", "up":
		f.focus = (f.focus + len(f.fields)) % (len(f.fields) + 1)
	case "ctrl+f":
		return m.planFiles()
	case "ctrl+s":
		return m.submitForm()
	case "enter":
		if f.focus == len(f.fields) {
			return m.submitForm()
		}
		f.focus++
	default:
		if f.focus >= len(f.fields) {
			return nil
		}
		entry := &f.fields[f.focus]
		if len(entry.choices) > 0 && entry.multi {
			switch k.String() {
			case "left":
				entry.option = (entry.option + len(entry.choices) - 1) % len(entry.choices)
			case "right":
				entry.option = (entry.option + 1) % len(entry.choices)
			case " ":
				path := entry.choices[entry.option]
				paths := []string{}
				removed := false
				for _, chosen := range strings.Split(entry.value, ",") {
					if chosen == path {
						removed = true
					} else if chosen != "" {
						paths = append(paths, chosen)
					}
				}
				if !removed {
					paths = append(paths, path)
				}
				entry.value = strings.Join(paths, ",")
			case "ctrl+u":
				entry.value = ""
			}
			return nil
		}
		if len(entry.choices) > 0 {
			if k.String() == "right" || k.String() == "left" || k.String() == " " {
				idx := 0
				for i, value := range entry.choices {
					if value == entry.value {
						idx = i
						break
					}
				}
				delta := 1
				if k.String() == "left" {
					delta = -1
				}
				entry.value = entry.choices[(idx+delta+len(entry.choices))%len(entry.choices)]
				if f.kind == "plan" && f.focus == 0 {
					f.fields[3].value = ""
					f.fields[3].choices = nil
					return m.planFiles()
				}
			}
			return nil
		}
		switch k.Type {
		case tea.KeyBackspace:
			rr := []rune(entry.value)
			if len(rr) > 0 {
				entry.value = string(rr[:len(rr)-1])
			}
		case tea.KeyCtrlU:
			entry.value = ""
		case tea.KeyRunes:
			if len(entry.value)+len(string(k.Runes)) <= 4096 {
				entry.value += singleInput(string(k.Runes))
			}
		case tea.KeySpace:
			if len(entry.value) < 4096 {
				entry.value += " "
			}
		}
	}
	return nil
}
func singleInput(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(clean(s), "\n", ""), "\t", "")
}
func (m *Model) submitForm() tea.Cmd {
	if m.busy || m.form == nil {
		return nil
	}
	f := m.form
	for i, e := range f.fields {
		if e.required && strings.TrimSpace(e.value) == "" {
			m.err = e.label + " ist erforderlich."
			f.focus = i
			return nil
		}
	}
	value := func(i int) string { return strings.TrimSpace(f.fields[i].value) }
	n := func(i int) (int, error) { return strconv.Atoi(value(i)) }
	a := action{method: "POST", input: map[string]any{}}
	switch f.kind {
	case "host":
		port, err := n(5)
		if err != nil || port < 1 || port > 65535 {
			m.err = "SSH-Port muss zwischen 1 und 65535 liegen."
			f.focus = 5
			return nil
		}
		if value(10) != "" {
			if _, err := time.Parse("15:04", value(10)); err != nil {
				m.err = "Uhrzeit muss HH:MM sein."
				f.focus = 10
				return nil
			}
		}
		h := clone(f.original)
		keys := []string{"name", "address", "group", "cluster_id", "ssh_user", "", "key_path", "known_hosts_path", "restore_ssh_user", "restore_key_path", "schedule"}
		for i, k := range keys {
			if k != "" {
				h[k] = value(i)
			}
		}
		h["ssh_port"] = port
		h["enabled"] = value(11) == "ja"
		a.title = "Host speichern"
		a.path = "hosts"
		a.input = h
		a.summary = fmt.Sprintf("Host: %s · %s\nSSH: %s:%d\nZeitplan aktiv: %s", value(0), value(1), value(4), port, value(11))
	case "settings":
		if _, err := time.LoadLocation(value(0)); err != nil {
			m.err = "Unbekannte Zeitzone."
			f.focus = 0
			return nil
		}
		if _, err := time.Parse("15:04", value(1)); err != nil {
			m.err = "Uhrzeit muss HH:MM sein."
			f.focus = 1
			return nil
		}
		s := clone(f.original)
		s["timezone"] = value(0)
		s["schedule"] = value(1)
		keys := []string{"parallel", "retries", "daily", "weekly", "monthly", "archive_days", "stale_hours"}
		for i, key := range keys {
			v, err := n(i + 2)
			if err != nil || v < 0 {
				m.err = f.fields[i+2].label + ": positive ganze Zahl erforderlich."
				f.focus = i + 2
				return nil
			}
			s[key] = v
		}
		a.title = "Zeitplan speichern"
		a.method = "PUT"
		a.path = "settings"
		a.input = s
		a.summary = formSummary(f)
	case "plan":
		p := anker.PlanRequest{BackupID: value(0), TargetID: value(1), Scenario: value(2), ConsoleConfirmed: value(5) == "ja", SourceOffline: value(6) == "ja", Mapping: anker.Mapping{Interfaces: map[string]string{}}}
		if value(3) != "" {
			for _, path := range strings.Split(value(3), ",") {
				path = strings.TrimSpace(path)
				if err := anker.ValidPath(path); err != nil {
					m.err = err.Error()
					f.focus = 3
					return nil
				}
				p.Files = append(p.Files, path)
			}
		}
		if p.Scenario == "files" && len(p.Files) == 0 {
			m.err = "Mindestens einen relativen Dateipfad auswählen."
			f.focus = 3
			return nil
		}
		if value(4) != "" {
			for _, pair := range strings.Split(value(4), ",") {
				port := strings.Split(strings.TrimSpace(pair), "=")
				if len(port) != 2 || strings.TrimSpace(port[0]) == "" || strings.TrimSpace(port[1]) == "" {
					m.err = "Netzwerkports als alt=neu angeben."
					f.focus = 4
					return nil
				}
				p.Mapping.Interfaces[strings.TrimSpace(port[0])] = strings.TrimSpace(port[1])
			}
		}
		a.title = "Wiederherstellungsplan erstellen"
		a.summary = "Sicherung: " + p.BackupID + "\nZiel: " + m.hostName(p.TargetID) + "\nSzenario: " + p.Scenario + "\nDer Dienst erstellt und prüft die Vorschau. Zur Ausführung ist eine separate Bestätigung nötig."
		a.path = "plans"
		a.input = p
		a.purpose = "plan"
	case "export":
		path, err := filepath.Abs(value(0))
		if err != nil {
			m.err = err.Error()
			return nil
		}
		overwrite := value(1) == "ja"
		if err := checkOutput(path, overwrite); err != nil {
			m.err = err.Error()
			f.focus = 0
			return nil
		}
		a.title = "Export speichern"
		a.summary = "Lokale Zieldatei: " + path + "\nVorhandene Datei ersetzen: " + yesNo(overwrite)
		a.path = str(f.original, "path")
		a.output = path
		a.overwrite = overwrite
		a.purpose = "export"
	case "storage":
		if storageManual(f.original) {
			m.err = "Diese Umgebung erfordert manuelle Speichererweiterung."
			return nil
		}
		volume := map[string]any{}
		for _, raw := range list(f.original["volumes"]) {
			v := object(raw)
			if str(v, "id") == value(0) {
				volume = v
				break
			}
		}
		if str(volume, "id") == "" || boolean(volume, "read_only") || str(volume, "error") != "" {
			m.err = "Dieses Dateisystem ist nicht zur Erweiterungsprüfung freigegeben."
			return nil
		}
		a.title = "Speicherplan prüfen"
		a.summary = "Dateisystem: " + str(volume, "mount") + "\nQuelle: " + str(volume, "source") + "\nDer Dienst prüft den Datenträger. Die Erweiterung benötigt eine weitere Bestätigung."
		a.path = "storage/plan"
		a.input = map[string]string{"volume_id": value(0)}
		a.purpose = "storagePlan"
	case "trust":
		port, err := n(1)
		if err != nil {
			m.err = "SSH-Port ungültig."
			f.focus = 1
			return nil
		}
		h := trustedHost{address: value(0), port: port, fingerprint: value(2)}
		if err := validateTrust(h); err != nil {
			m.err = err.Error()
			f.focus = 2
			return nil
		}
		a.title = "SSH-Hostschlüssel speichern"
		a.summary = "Adresse: " + h.address + "\nPort: " + strconv.Itoa(h.port) + "\nUnabhängig geprüfter Fingerprint: " + h.fingerprint + "\nDer bestehende Anker-Helfer vergleicht den Ed25519-Schlüssel und speichert ihn in /etc/anker/known_hosts."
		a.trust = &h
		a.purpose = "trust"
	case "tls":
		days, err := n(1)
		if err != nil || days < 7 || days > 90 {
			m.err = "Vorlauf muss 7–90 Tage betragen."
			f.focus = 1
			return nil
		}
		a.title = "TLS-Automatik speichern"
		a.path = "tls"
		a.input = map[string]any{"automatic": value(0) == "ja", "renew_before_days": days}
		a.summary = formSummary(f)
		a.purpose = "tls"
	case "user":
		if f.fields[3].value != f.fields[4].value {
			m.err = "Passwörter stimmen nicht überein."
			f.focus = 4
			return nil
		}
		a.title = "Benutzer anlegen"
		a.path = "users"
		a.input = map[string]any{"name": value(0), "role": value(1), "secrets": value(2) == "ja", "password": f.fields[3].value}
		a.summary = formSummary(f)
		a.purpose = "usersChanged"
	case "userEdit":
		a.title = "Benutzerrechte speichern"
		a.method = "PUT"
		a.path = "users/" + segment(str(f.original, "id"))
		a.input = map[string]any{"role": value(0), "secrets": value(1) == "ja", "disabled": boolean(f.original, "disabled")}
		a.summary = "Benutzer: " + str(f.original, "name") + "\n" + formSummary(f)
		a.purpose = "usersChanged"
	case "password":
		if f.fields[0].value != f.fields[1].value {
			m.err = "Passwörter stimmen nicht überein."
			f.focus = 1
			return nil
		}
		a.title = "Passwort ändern"
		a.path = "users/" + segment(str(f.original, "id")) + "/password"
		a.input = map[string]string{"password": f.fields[0].value}
		a.summary = "Neues Passwort für " + str(f.original, "name") + " speichern."
		a.purpose = "usersChanged"
	}
	m.confirm(a)
	return nil
}
func formSummary(f *form) string {
	var lines []string
	for _, e := range f.fields {
		if !e.secret {
			lines = append(lines, e.label+": "+single(e.value))
		}
	}
	return strings.Join(lines, "\n")
}
func checkOutput(path string, overwrite bool) error {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("Exportziel muss eine reguläre Datei sein; Links und Verzeichnisse sind nicht erlaubt")
	}
	if !overwrite {
		return errors.New("Zieldatei existiert bereits; anderen Pfad wählen oder Ersetzen ausdrücklich aktivieren")
	}
	return nil
}
func download(c *client.Client, ctx context.Context, path, output string, overwrite bool) error {
	if err := checkOutput(output, overwrite); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(output), ".anker-tui-export-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	f.Close()
	defer os.Remove(temp)
	if err = c.Download(ctx, path, temp); err != nil {
		return err
	}
	if !overwrite {
		return os.Link(temp, output)
	} // Atomic no-replace, including files created during transfer.
	if err = checkOutput(output, true); err != nil {
		return err
	}
	return os.Rename(temp, output)
}
func list(v any) []any { a, _ := v.([]any); return a }

func defaultExport(name string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "anker-exports", name)
	}
	return ""
}
