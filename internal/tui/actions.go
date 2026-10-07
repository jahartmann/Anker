package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

type teaCmd = tea.Cmd

type hotAction struct{ key, label string }

func (m Model) listActions() []hotAction {
	switch m.tab {
	case 0:
		if m.width <= 90 {
			return []hotAction{{"a", "Hinzufügen"}, {"m", "Manuell"}, {"e", "Edit"}, {"v", "SSH"}, {"p", "Pause"}, {"h", "Key"}, {"t", "Test"}, {"b", "Sichern"}}
		}
		return []hotAction{{"a", "Hinzufügen"}, {"m", "Manuell"}, {"e", "Bearbeiten"}, {"v", "Anbinden"}, {"p", "Pause"}, {"h", "Schlüssel"}, {"t", "Prüfen"}, {"b", "Sichern"}}
	case 1:
		return []hotAction{{"v", "Prüfen"}, {"x", "Export"}, {"r", "Wiederherstellen"}, {"f", "Dateien"}}
	case 2:
		return []hotAction{{"a", "Plan erstellen"}, {"x", "Export"}}
	case 3:
		return []hotAction{{"c", "Abbrechen"}, {"r", "Wiederholen"}}
	case 4:
		if m.users {
			return []hotAction{{"a", "Hinzufügen"}, {"e", "Rolle"}, {"p", "Passwort"}, {"d", "Sperren"}, {"s", "Sitzungen"}}
		}
		return []hotAction{{"enter", "Öffnen"}}
	}
	return nil
}
func (m Model) detailActions() []hotAction {
	switch m.detailTitle {
	case "Hostdetails", "Sicherungsdetails", "Auftragsdetails", "Benutzerdetails":
		return m.listActions()
	case "Wiederherstellungsplan":
		return []hotAction{{"a", "Ausführen"}, {"x", "Export"}}
	case "Speicher":
		if storageManual(object(m.data["storage"])) {
			return []hotAction{{"r", "Erweiterungsstatus"}}
		}
		return []hotAction{{"g", "Erweiterung planen"}, {"r", "Erweiterungsstatus"}}
	case "Speicherplan":
		if boolean(object(m.data["growthPlan"]), "can_grow") && !storageManual(object(m.data["growthPlan"])) {
			return []hotAction{{"g", "Erweitern"}}
		}
		return nil
	case "Speichererweiterung":
		return []hotAction{{"r", "Status aktualisieren"}}
	case "TLS-Zertifikat":
		return []hotAction{{"r", "Erneuern"}, {"e", "Automatik"}, {"x", "Zertifikat exportieren"}}
	case "Updates", "Updatequelle eingerichtet":
		return []hotAction{{"o", "Offizielle Quelle"}, {"c", "Prüfen"}, {"i", "Installieren"}, {"r", "Status"}}
	}
	return nil
}
func hotText(actions []hotAction) string {
	var parts []string
	for _, a := range actions {
		parts = append(parts, a.key+" "+a.label)
	}
	return strings.Join(parts, "   ")
}
func clickKey(actions []hotAction, x int) string {
	offset := 0
	for _, a := range actions {
		end := offset + ansi.StringWidth(a.key+" "+a.label) + 3
		if x >= offset && x < end {
			return a.key
		}
		offset = end
	}
	return ""
}
func (m *Model) actionMouse(x int) teaCmd { return m.listKey(clickKey(m.listActions(), x)) }
func (m *Model) detailMouse(x int) teaCmd { return m.detailKey(clickKey(m.detailActions(), x)) }

func (m *Model) listKey(key string) teaCmd {
	if m.busy {
		return nil
	}
	v := m.selected()
	id := str(v, "id")
	path := ""
	if m.tab < 4 {
		path = dataKeys[m.tab] + "/" + segment(id)
	}
	if key == "enter" {
		if m.tab == 4 && !m.users {
			if m.cursor >= len(m.ids) {
				return nil
			}
			switch m.ids[m.cursor] {
			case "schedule":
				return m.request(action{title: "Einstellungen laden", method: "GET", path: "settings", purpose: "form:settings"})
			case "system":
				return m.request(action{title: "Diagnose laden", method: "GET", path: "doctor", purpose: "system"})
			case "tls":
				return m.request(action{title: "Zertifikat laden", method: "GET", path: "tls", purpose: "tls"})
			case "storage":
				return m.request(action{title: "Speicher laden", method: "GET", path: "storage", purpose: "storage"})
			case "updates":
				return m.request(action{title: "Updates laden", method: "GET", path: "updates", purpose: "updates"})
			case "users":
				return m.request(action{title: "Benutzer laden", method: "GET", path: "users", purpose: "users"})
			}
		}
		if v != nil {
			m.data["detailRecord"] = clone(v)
			if m.tab == 2 {
				m.data["preview"] = v
				m.showDetail("Wiederherstellungsplan", planText(v, *m))
			} else {
				m.showDetail([]string{"Hostdetails", "Sicherungsdetails", "Wiederherstellungsplan", "Auftragsdetails", "Benutzerdetails"}[m.tab], describe(v))
			}
		}
		return nil
	}
	switch m.tab {
	case 0:
		if key == "a" {
			m.openForm("hostConnect", nil)
			return nil
		}
		if key == "m" {
			m.openForm("host", nil)
			return nil
		}
		if v == nil {
			return nil
		}
		switch key {
		case "v":
			m.openForm("hostConnect", v)
		case "e":
			m.openForm("host", v)
		case "b":
			m.confirm(action{title: "Sicherung starten", summary: "Host: " + str(v, "name") + "\nEine neue Sicherung wird als Auftrag eingeplant.", method: "POST", path: path + "/backup", input: map[string]any{}})
		case "h":
			if boolean(m.data, "demo") {
				m.err = "SSH-Vertrauen ist in der Demo ausgeschaltet."
				return nil
			}
			if path := str(v, "known_hosts_path"); path != "" && path != "/etc/anker/known_hosts" {
				m.err = "Geführtes SSH-Vertrauen verwendet /etc/anker/known_hosts. Eigene Vertrauensdateien separat verwalten."
				return nil
			}
			m.openForm("trust", v)
		case "t":
			m.confirm(action{title: "SSH-Verbindung prüfen", summary: "Host: " + str(v, "name") + " · " + str(v, "address") + "\nDer Dienst prüft den hinterlegten SSH-Zugang und erfasst das Inventar.", method: "POST", path: path + "/probe", input: map[string]any{}})
		case "p":
			h := clone(v)
			h["enabled"] = !boolean(v, "enabled")
			m.confirm(action{title: "Zeitplan ändern", summary: "Host: " + str(v, "name") + "\nZeitplan aktiv: " + yesNo(!boolean(v, "enabled")), method: "POST", path: "hosts", input: h})
		}
	case 1:
		if v == nil {
			return nil
		}
		switch key {
		case "v":
			m.confirm(action{title: "Sicherung prüfen", summary: "Sicherung: " + id + "\nDateien und Prüfsummen werden durch den Dienst geprüft.", method: "POST", path: path + "/verify", input: map[string]any{}})
		case "x":
			m.openExport(path+"/download", "anker-"+id+".tar")
		case "r":
			m.openForm("plan", map[string]any{"backup_id": id})
			return m.planFiles()
		case "f":
			return m.request(action{title: "Dateien laden", method: "GET", path: path + "/files", purpose: "files"})
		}
	case 2:
		if key == "a" {
			m.openForm("plan", nil)
			return m.planFiles()
		}
		if key == "x" && v != nil {
			m.openExport(path+"/download", "anker-plan-"+id+".tar")
		}
	case 3:
		if v == nil {
			return nil
		}
		if key == "c" || key == "r" {
			state := str(v, "state")
			if key == "c" && state != "queued" && state != "running" {
				m.err = "Nur wartende oder laufende Aufträge können abgebrochen werden."
				return nil
			}
			if key == "r" && state != "failed" && state != "cancelled" && state != "interrupted" && state != "successful" {
				m.err = "Nur beendete Aufträge können wiederholt werden."
				return nil
			}
			if key == "r" && str(v, "kind") == "restore" {
				m.err = "Wiederherstellung benötigt einen frisch geprüften Plan mit neuer Bestätigung."
				return nil
			}
			operation, title := "cancel", "Auftrag abbrechen"
			if key == "r" {
				operation, title = "retry", "Auftrag wiederholen"
			}
			m.confirm(action{title: title, summary: "Auftrag: " + id + "\nHost: " + m.hostName(str(v, "host_id")) + "\nArt: " + str(v, "kind") + " · Zustand: " + state, method: "POST", path: path + "/" + operation, input: map[string]any{}})
		}
	case 4:
		if !m.users {
			return nil
		}
		if key == "a" {
			m.openForm("user", nil)
			return nil
		}
		if v == nil {
			return nil
		}
		switch key {
		case "e":
			m.openForm("userEdit", v)
		case "p":
			m.openForm("password", v)
		case "d":
			m.confirm(action{title: "Benutzerzugang ändern", summary: "Benutzer: " + str(v, "name") + "\nGesperrt: " + yesNo(!boolean(v, "disabled")), method: "PUT", path: "users/" + segment(id), input: map[string]any{"role": str(v, "role"), "secrets": boolean(v, "secrets"), "disabled": !boolean(v, "disabled")}, purpose: "usersChanged"})
		case "s":
			m.confirm(action{title: "Sitzungen widerrufen", summary: "Alle Web-Sitzungen von " + str(v, "name") + " werden beendet.", method: "DELETE", path: "users/" + segment(id) + "/sessions", purpose: "usersChanged"})
		}
	}
	return nil
}
func (m *Model) detailKey(key string) teaCmd {
	if m.busy {
		return nil
	}
	switch m.detailTitle {
	case "Hostdetails", "Sicherungsdetails", "Auftragsdetails", "Benutzerdetails":
		return m.listKey(key)
	case "Wiederherstellungsplan":
		p := object(m.data["preview"])
		id := str(p, "id")
		if key == "x" {
			m.openExport("plans/"+segment(id)+"/download", "anker-plan-"+id+".tar")
		}
		if key == "a" {
			blockers, _ := p["blockers"].([]any)
			if str(p, "state") != "ready" || len(blockers) > 0 {
				m.err = "Dieser Plan ist nicht ausführbar. Sperren und Zustand im Plan prüfen."
				return nil
			}
			m.confirm(action{title: "Wiederherstellung ausführen", summary: planText(p, *m) + "\nDer Dienst prüft Quelle und Ziel erneut. Dateien auf dem Ziel werden geändert.", method: "POST", path: "plans/" + segment(id) + "/apply", input: map[string]string{"confirmation": id}, challenge: id, purpose: "apply"})
		}
	case "Speicher":
		if key == "r" {
			return m.request(action{title: "Speicherstatus laden", method: "GET", path: "storage/state", purpose: "storageGrow"})
		}
		if key == "g" {
			report := object(m.data["storage"])
			if storageManual(report) {
				m.err = "Diese Umgebung unterstützt nur manuelle Speichererweiterung; Hinweise beachten."
				return nil
			}
			m.openForm("storage", report)
		}
	case "Speichererweiterung":
		if key == "r" {
			return m.request(action{title: "Speicherstatus laden", method: "GET", path: "storage/state", purpose: "storageGrow"})
		}
	case "Speicherplan":
		if key == "g" {
			p := object(m.data["growthPlan"])
			if !boolean(p, "can_grow") || str(p, "id") == "" || str(p, "mount") == "" || storageManual(p) {
				m.err = "Dieser Speicherplan ist nur manuell ausführbar."
				return nil
			}
			m.confirm(action{title: "Dateisystem erweitern", summary: describe(p) + "\nDer geprüfte Datenträger und das Dateisystem werden geändert. Hinweise vollständig prüfen.", method: "POST", path: "storage/grow", input: map[string]string{"volume_id": str(p, "volume_id"), "plan_id": str(p, "id"), "confirmation": str(p, "mount")}, challenge: str(p, "mount"), challengeLabel: "Mountpoint", purpose: "storageGrow"})
		}
	case "TLS-Zertifikat":
		switch key {
		case "r":
			m.confirm(action{title: "TLS-Zertifikat erneuern", summary: "Das Serverzertifikat wird ersetzt. Die Herkunft und Gültigkeit prüft der Updater.", method: "POST", path: "tls/renew", input: map[string]any{}, purpose: "tls"})
		case "e":
			m.openForm("tls", object(m.data["tls"]))
		case "x":
			m.openExport("tls/certificate", "anker-server.crt")
		}
	case "Updates", "Updatequelle eingerichtet":
		switch key {
		case "r":
			return m.request(action{title: "Updatezustand laden", method: "GET", path: "updates", purpose: "updates"})
		case "o":
			return m.request(action{title: "Vertrauensquelle laden", method: "GET", path: "updates/configuration", purpose: "updateSource"})
		case "c":
			return m.request(action{title: "Signiertes Release prüfen", method: "POST", path: "updates/check", input: map[string]any{}, purpose: "updates"})
		case "i":
			state := object(m.data["updates"])
			release := object(state["available"])
			version := str(release, "version")
			if version == "" {
				m.err = "Zuerst nach einem signierten Update suchen."
				return nil
			}
			if boolean(state, "busy") {
				m.err = "Ein Update läuft bereits."
				return nil
			}
			m.confirm(action{title: "Anker aktualisieren", summary: fmt.Sprintf("Quelle: %s\nVersion: %s → %s\nSignatur und Sicherung werden geprüft. Der Dienst startet anschließend neu; diese Verbindung kann kurz unterbrochen werden.", str(state, "repository"), str(state, "current"), version), method: "POST", path: "updates/install", input: map[string]string{"version": version}, purpose: "updates"})
		}
	}
	return nil
}
func clone(v map[string]any) map[string]any {
	d := map[string]any{}
	for k, value := range v {
		d[k] = value
	}
	return d
}

func (m *Model) planFiles() tea.Cmd {
	if m.form == nil || m.form.kind != "plan" || m.form.fields[0].value == "" {
		return nil
	}
	id := m.form.fields[0].value
	return m.request(action{title: "Dateiauswahl laden", method: "GET", path: "backups/" + segment(id) + "/files", purpose: "planFiles:" + id})
}

func recordDetail(title string) bool {
	switch title {
	case "Hostdetails", "Sicherungsdetails", "Auftragsdetails", "Benutzerdetails":
		return true
	}
	return false
}

func storageManual(v map[string]any) bool {
	environment := str(v, "environment")
	return boolean(v, "demo") || strings.HasPrefix(environment, "container:") || (environment != "physical" && !strings.HasPrefix(environment, "vm:"))
}
