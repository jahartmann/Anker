package tui

import (
	"anker/internal/anker"
	"anker/internal/client"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func recoveryIdentity(f *form) string {
	return anker.Hash([]byte(strings.Join([]string{f.fields[0].value, f.fields[1].value, f.fields[2].value, f.fields[3].value, recoveryStorageIdentity(f)}, "\x00")))
}
func typedRecoveryInspection(raw map[string]any) anker.RecoveryInspection {
	data, _ := json.Marshal(raw)
	var result anker.RecoveryInspection
	json.Unmarshal(data, &result)
	return result
}
func recoveryFileDecisions(f *form) map[string]anker.FileDecision {
	decisions, _ := client.ParseRecoveryDecisions(strings.Split(f.fields[10].value, ","))
	return decisions
}
func recoveryReferenceNeeded(raw map[string]any, decisions map[string]anker.FileDecision) bool {
	paths := []string{}
	for _, p := range list(raw["paths"]) {
		paths = append(paths, fmt.Sprint(p))
	}
	return client.RecoveryReferenceNeeded(paths, decisions)
}
func recoveryStorageIdentity(f *form) string {
	storage, err := parseRecoveryPairs(f.fields[7].value, "Storage")
	if err != nil {
		return f.fields[7].value
	}
	if raw := object(f.original["inspection"]); raw != nil {
		storage = client.RelevantRecoveryMapping(anker.Mapping{Storage: storage}, typedRecoveryInspection(raw), recoveryFileDecisions(f)).Storage
	}
	data, _ := json.Marshal(storage)
	return string(data)
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
				if !hasRelevantRecoveryReference(i, "ports", f) {
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
				if !hasRelevantRecoveryReference(i, "storage", f) {
					continue
				}
			case 8, 9:
				if i == nil || f.fields[2].value == "files" {
					continue
				}
			case 11:
				if i == nil || len(relevantRecoveryStanzas(f, i)) == 0 {
					continue
				}
			case 12, 13, 14:
				if i == nil || f.fields[11].value == "" {
					continue
				}
			case 10:
				if i == nil {
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
	if strings.HasPrefix(v.purpose, "recoveryComparison:") {
		if str(object(m.data["preview"]), "id") != strings.TrimPrefix(v.purpose, "recoveryComparison:") {
			return true
		}
		m.form = nil
		m.showDetail("Dateivergleich", recoveryComparisonText(object(v.data)))
		return true
	}
	if v.purpose == "recoveryCheck" {
		m.form = nil
		p := object(v.data)
		m.data["preview"] = p
		m.showDetail("Wiederherstellungsplan", planText(p, *m))
		return true
	}
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
	clearChangedTargetChoices(f, object(f.original["inspection"]), i)
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
	if f.fields[7].value == "" {
		f.fields[7].value = strings.Join(storage, ",")
	}
	f.fields[7].hint = "Benötigt: " + strings.Join(storageNames, ", ") + " · Ziele: " + strings.Join(targets, ", ") + " · ID=Ziel oder ID=manual; keine automatische Umsetzung"
	if f.fields[8].value == "" {
		f.fields[8].value = str(object(i["target"]), "hostname")
	}
	updateRecoveryNetworkChoices(f, i)
	f.title = "Ziel geprüft · Zuordnungen und manuelle Entscheidungen"
	f.focus = 4
	if len(m.visibleFormFields()) <= 4 {
		f.focus = len(f.fields)
	} else {
		f.focus = m.visibleFormFields()[4]
	}
	m.notice = "Ziel frisch geprüft. Zuordnungen ansehen und Plan erstellen."
	if recoveryInspection(f) == nil {
		m.notice = "Storage-Entscheidungen vorbelegt. Mit Ctrl+S an eine neue Zielaufnahme binden."
	}
	return true
}

func clearChangedTargetChoices(f *form, before, after map[string]any) {
	if before == nil || len(f.fields) <= 10 {
		return
	}
	evidence := func(inspection map[string]any) map[string]string {
		out := map[string]string{}
		for _, raw := range list(inspection["files"]) {
			file := object(raw)
			data, _ := json.Marshal(file)
			out[str(file, "path")] = anker.Hash(data)
		}
		return out
	}
	old, current := evidence(before), evidence(after)
	if len(f.fields) > 14 && !equalRecoveryEvidence(old, current) {
		for n := 11; n <= 14; n++ {
			f.fields[n].value = ""
		}
	}
	retained := []string{}
	for _, value := range strings.Split(f.fields[10].value, ",") {
		pair := strings.Split(value, "=")
		if len(pair) == 2 && (strings.TrimSpace(pair[1]) == "keep" || strings.TrimSpace(pair[1]) == "prepared") && old[strings.TrimSpace(pair[0])] != current[strings.TrimSpace(pair[0])] {
			continue
		}
		if value != "" {
			retained = append(retained, value)
		}
	}
	f.fields[10].value = strings.Join(retained, ",")
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
	for _, index := range []int{4, 7, 8, 9, 11, 12, 13, 14} {
		f.fields[index].value = ""
	}
	f.fields[5].value, f.fields[6].value = "nein", "nein"
	f.title = "Wiederherstellung planen"
	if len(f.fields) > 10 {
		f.fields[10].value = ""
	}
}

// Presentation only: prerequisites and permission to confirm remain service rules.
func orderedRecoveryTasks(raw []any) []map[string]any {
	pending := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		pending = append(pending, object(item))
	}
	out := []map[string]any{}
	seen := map[string]bool{}
	for len(pending) > 0 {
		progress := false
		for index, task := range pending {
			ready := true
			for _, prerequisite := range list(task["requires"]) {
				if !seen[fmt.Sprint(prerequisite)] {
					ready = false
				}
			}
			if !ready {
				continue
			}
			out = append(out, task)
			seen[str(task, "id")] = true
			pending = append(pending[:index], pending[index+1:]...)
			progress = true
			break
		}
		if !progress {
			out = append(out, pending...)
			break
		}
	}
	return out
}

func recoveryComparisonText(c map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s → %s\nVergleichszustand: %s\n%s\n", str(c, "path"), str(c, "before_label"), str(c, "after_label"), str(c, "state"), str(c, "error"))
	if boolean(c, "masked") || str(c, "state") == "masked" || str(c, "state") == "unavailable" {
		return clean(b.String())
	}
	for _, raw := range list(c["metadata_changes"]) {
		change := object(raw)
		fmt.Fprintf(&b, "Metadaten %s: %v → %v\n", str(change, "field"), change["before"], change["after"])
	}
	for _, raw := range list(c["hunks"]) {
		hunk := object(raw)
		fmt.Fprintf(&b, "@@ %d → %d @@\n", number(hunk, "before_start"), number(hunk, "after_start"))
		for _, rawLine := range list(hunk["lines"]) {
			line := object(rawLine)
			prefix := " "
			switch str(line, "kind") {
			case "add":
				prefix = "+"
			case "remove":
				prefix = "-"
			}
			text, _ := line["text"].(string)
			fmt.Fprintf(&b, "%4d %4d %s%s", number(line, "before_line"), number(line, "after_line"), prefix, text)
			if !strings.HasSuffix(text, "\n") {
				b.WriteString("\n  [Keine abschließende neue Zeile]\n")
			}
		}
	}
	return clean(b.String())
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

func hasRelevantRecoveryReference(i map[string]any, key string, f *form) bool {
	for _, raw := range list(i[key]) {
		if recoveryReferenceNeeded(object(raw), recoveryFileDecisions(f)) {
			return true
		}
	}
	return false
}
func equalRecoveryEvidence(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for p, v := range a {
		if b[p] != v {
			return false
		}
	}
	return true
}

func relevantRecoveryStanzas(f *form, i map[string]any) []string {
	keys := []string{}
	decisions := recoveryFileDecisions(f)
	for _, raw := range list(i["network_stanzas"]) {
		stanza := object(raw)
		if decisions[str(stanza, "path")] != anker.DecisionKeep {
			keys = append(keys, str(stanza, "key"))
		}
	}
	return keys
}
func updateRecoveryNetworkChoices(f *form, i map[string]any) {
	if len(f.fields) <= 14 {
		return
	}
	choices := append([]string{""}, relevantRecoveryStanzas(f, i)...)
	f.fields[11].choices = choices
	for _, choice := range choices {
		if choice == f.fields[11].value {
			return
		}
	}
	for n := 11; n <= 14; n++ {
		f.fields[n].value = ""
	}
}
