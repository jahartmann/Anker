package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func enrollmentPurpose(phase string, f *form) string {
	return phase + ":" + strconv.Itoa(f.generation)
}

func (m *Model) clearEnrollmentSecret() {
	if m.form != nil && m.form.kind == "hostConnect" {
		m.form.fields[2].value = ""
	}
}

func (m *Model) cancelConfirmation() {
	if m.pending != nil && strings.HasPrefix(m.pending.purpose, "hostEnroll:") {
		m.clearEnrollmentSecret()
	}
	m.pending = nil
	m.confirmation = ""
}

func (m *Model) inspectEnrollment() tea.Cmd {
	f := m.form
	if boolean(m.data, "demo") {
		m.clearEnrollmentSecret()
		m.err = "Automatische SSH-Anbindung ist in der Demo ausgeschaltet. Mit m einen Host manuell anlegen."
		return nil
	}
	address := strings.TrimSpace(f.fields[0].value)
	port, err := strconv.Atoi(strings.TrimSpace(f.fields[3].value))
	// Share the address and port validation used by manual host trust.
	if err == nil {
		err = validateTrust(trustedHost{address: address, port: port, fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"})
	}
	if err != nil {
		m.err = "DNS-Name oder IP ohne Protokoll und SSH-Port zwischen 1 und 65535 angeben."
		return nil
	}
	return m.request(action{title: "SSH-Identität prüfen", method: "POST", path: "hosts/connection/inspect", input: map[string]any{"address": address, "port": port}, purpose: enrollmentPurpose("hostInspect", f)})
}

// Enrollment responses belong to the particular draft that started them. Polls
// and resize events keep the draft; a reopened form cannot inherit old trust.
func (m *Model) enrollmentResult(v commandResult) bool {
	inspect := strings.HasPrefix(v.purpose, "hostInspect:")
	enroll := strings.HasPrefix(v.purpose, "hostEnroll:")
	if !inspect && !enroll {
		return false
	}
	phase := "hostEnroll"
	if inspect {
		phase = "hostInspect"
	}
	f := m.form
	if f == nil || f.kind != "hostConnect" || v.purpose != enrollmentPurpose(phase, f) {
		return true
	}
	m.busy = false
	m.notice = ""
	if v.err != nil {
		m.clearEnrollmentSecret()
		// Remote error text may contain banners or echoed bootstrap credentials.
		// Only the local phase and safe retry instructions reach the terminal.
		if inspect {
			m.err = "SSH-Identität konnte nicht geprüft werden. Adresse, Port und Dienstverbindung prüfen; SSH-Zugang erneut eingeben."
		} else {
			m.err = "SSH-Einrichtung fehlgeschlagen. SSH-Zugang und root-/sudo-Rechte prüfen und erneut versuchen."
		}
		return true
	}
	m.err = ""
	m.revision++
	if enroll {
		m.clearEnrollmentSecret()
		m.form = nil
		m.detail = ""
		m.notice = "Host angebunden; beide Schlüsselzugänge und Inventar geprüft."
		return true
	}
	identity := object(v.data)
	address := strings.TrimSpace(f.fields[0].value)
	port, _ := strconv.Atoi(strings.TrimSpace(f.fields[3].value))
	fingerprint := str(identity, "fingerprint")
	if boolean(identity, "changed") {
		m.clearEnrollmentSecret()
		m.err = "Bekannter SSH-Hostschlüssel wurde geändert. Identität auf der Proxmox-Konsole prüfen und über h Schlüssel ausdrücklich aktualisieren."
		return true
	}
	if str(identity, "address") != address || number(identity, "port") != port || (str(identity, "key_type") != "ssh-ed25519" && str(identity, "key_type") != "ed25519") || validateTrust(trustedHost{address: address, port: port, fingerprint: fingerprint}) != nil {
		m.clearEnrollmentSecret()
		m.err = "Der Dienst liefert keine gültige SSH-Identität für diesen Host. Erneut prüfen."
		return true
	}
	host := clone(f.original)
	// Enrollment carries operator metadata only; the helper returns fresh probes.
	delete(host, "inventory")
	delete(host, "last_probe")
	delete(host, "probe_error")
	host["address"] = address
	host["ssh_port"] = port
	host["name"] = strings.TrimSpace(f.fields[4].value)
	host["group"] = strings.TrimSpace(f.fields[5].value)
	host["cluster_id"] = strings.TrimSpace(f.fields[6].value)
	host["ssh_user"] = "anker"
	host["restore_ssh_user"] = "anker-restore"
	host["key_path"] = "/etc/anker/keys/backup"
	host["restore_key_path"] = "/etc/anker/keys/restore"
	host["known_hosts_path"] = "/etc/anker/known_hosts"
	if str(f.original, "id") == "" {
		host["enabled"] = true
	}
	username := strings.TrimSpace(f.fields[1].value)
	state := "Unbekannter Schlüssel: auf der Proxmox-Konsole vergleichen."
	if boolean(identity, "known") {
		state = "Bekannter Schlüssel stimmt überein."
	}
	m.confirm(action{title: "SSH-Identität bestätigen", method: "POST", path: "hosts/connection/enroll", purpose: enrollmentPurpose("hostEnroll", f), input: map[string]any{"host": host, "username": username, "fingerprint": fingerprint, "confirmed": true}, summary: fmt.Sprintf("Host: %s:%d · Zugang: %s\nEd25519-Fingerprint:\n%s\n%s\nKonsole: ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub\nBestätigen installiert Helfer und beide eingeschränkten Zugänge.", address, port, username, fingerprint, state)})
	return true
}
