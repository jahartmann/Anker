package tui

import (
	"anker/internal/anker"
	"fmt"
	"strconv"
	"strings"
)

func recoveryIdentity(f *form) string {
	return anker.Hash([]byte(strings.Join([]string{f.fields[0].value, f.fields[1].value, f.fields[2].value, f.fields[3].value}, "\x00")))
}
func recoveryPurpose(f *form) string {
	return "recoveryInspect:" + strconv.Itoa(f.generation) + ":" + recoveryIdentity(f)
}
func recoveryInspection(f *form) map[string]any {
	if f == nil || f.kind != "plan" || str(f.original, "inspection_key") != recoveryIdentity(f) {
		return nil
	}
	return object(f.original["inspection"])
}
func (m Model) visibleFormFields() []int {
	out := []int{}
	if m.form == nil {
		return out
	}
	f := m.form
	i := recoveryInspection(f)
	for index := range f.fields {
		if f.kind == "plan" && index >= 4 {
			switch index {
			case 4:
				if len(list(i["ports"])) == 0 {
					continue
				}
			case 5:
				if !boolean(i, "requires_console") {
					continue
				}
			case 6:
				if !boolean(i, "requires_source_offline") {
					continue
				}
			case 7:
				if len(list(i["storage"])) == 0 {
					continue
				}
			case 8, 9:
				if i == nil || f.fields[2].value == "files" {
					continue
				}
			}
		}
		out = append(out, index)
	}
	return out
}
func (m *Model) nextFormField(delta int) {
	positions := append(m.visibleFormFields(), len(m.form.fields))
	position := 0
	for index, value := range positions {
		if value == m.form.focus {
			position = index
			break
		}
	}
	m.form.focus = positions[(position+delta+len(positions))%len(positions)]
}
func (m *Model) receiveRecoveryInspection(v commandResult) bool {
	if !strings.HasPrefix(v.purpose, "recoveryInspect:") {
		return false
	}
	f := m.form
	if f == nil || f.kind != "plan" || v.purpose != recoveryPurpose(f) {
		return true
	}
	i := object(v.data)
	if i == nil {
		m.err = "Zielprüfung lieferte keine verwertbaren Daten."
		return true
	}
	f.original["inspection_key"] = recoveryIdentity(f)
	f.original["inspection"] = i
	m.pending = nil
	ports, sourcePorts, targetPorts := []string{}, []string{}, []string{}
	for _, raw := range list(i["ports"]) {
		p := object(raw)
		sourcePorts = append(sourcePorts, str(p, "name"))
		if str(p, "suggested") != "" {
			ports = append(ports, str(p, "name")+"="+str(p, "suggested"))
		}
	}
	for _, raw := range list(i["target_ports"]) {
		targetPorts = append(targetPorts, str(object(raw), "name"))
	}
	if f.fields[4].value == "" {
		f.fields[4].value = strings.Join(ports, ",")
	}
	f.fields[4].hint = "Benötigt: " + strings.Join(sourcePorts, ", ") + " · Ziele: " + strings.Join(targetPorts, ", ") + " · alt=neu"
	storage, storageNames, targets := []string{}, []string{}, []string{}
	for _, raw := range list(i["storage"]) {
		p := object(raw)
		storage = append(storage, str(p, "id")+"=manual")
		storageNames = append(storageNames, str(p, "id")+" ("+str(p, "path")+")")
	}
	for _, raw := range list(i["target_storage"]) {
		targets = append(targets, str(object(raw), "id"))
	}
	f.fields[7].value = strings.Join(storage, ",")
	f.fields[7].hint = "Benötigt: " + strings.Join(storageNames, ", ") + " · Ziele: " + strings.Join(targets, ", ") + " · ID=Ziel oder ID=manual; keine automatische Umsetzung"
	f.fields[8].value = str(object(i["target"]), "hostname")
	f.title = "Ziel geprüft · Zuordnungen und manuelle Entscheidungen"
	f.focus = 4
	if len(m.visibleFormFields()) <= 4 {
		f.focus = len(f.fields)
	} else {
		f.focus = m.visibleFormFields()[4]
	}
	m.notice = "Ziel frisch geprüft. Zuordnungen ansehen und Plan erstellen."
	return true
}
func parseRecoveryPairs(value, label string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range strings.Split(value, ",") {
		if strings.TrimSpace(pair) == "" {
			continue
		}
		p := strings.SplitN(pair, "=", 2)
		if len(p) != 2 || strings.TrimSpace(p[0]) == "" || strings.TrimSpace(p[1]) == "" {
			return nil, fmt.Errorf("%s als alt=neu angeben", label)
		}
		key := strings.TrimSpace(p[0])
		if _, ok := out[key]; ok {
			return nil, fmt.Errorf("%s ist mehrfach angegeben", key)
		}
		out[key] = strings.TrimSpace(p[1])
	}
	return out, nil
}
func recoveryMayHaveJournal(p map[string]any) bool {
	switch str(p, "state") {
	case "failed", "interrupted", "applying", "rolling_back", "rollback_conflict", "checks_pending", "rolled_back":
		return true
	}
	return false
}

func clearRecoveryDecisions(f *form) {
	delete(f.original, "inspection")
	delete(f.original, "inspection_key")
	for _, index := range []int{4, 7, 8, 9} {
		f.fields[index].value = ""
	}
	f.fields[5].value, f.fields[6].value = "nein", "nein"
	f.title = "Wiederherstellung planen"
}

func (m *Model) refreshRecoveryPreview() {
	if m.detailTitle != "Wiederherstellungsplan" || m.pending != nil {
		return
	}
	id := str(object(m.data["preview"]), "id")
	for _, raw := range list(m.data["plans"]) {
		p := object(raw)
		if id != "" && str(p, "id") == id {
			m.data["preview"] = p
			m.detail = planText(p, *m)
			return
		}
	}
}
