package anker

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

func (s *Service) Plan(id string) (Plan, error) {
	var p Plan
	if !validID(id) {
		return p, errors.New("ungültige Plan-ID")
	}
	err := s.Store.Get("plans", id, &p)
	return p, err
}
func (s *Service) Plans() ([]Plan, error) { return records[Plan](s.Store, "plans") }
func versionMajor(v string) string        { return strings.Split(strings.Split(v, "-")[0], ".")[0] }
func fileHashes(i Inventory) map[string]string {
	out := map[string]string{}
	if raw := i.Details["file_hashes"]; raw != nil {
		json.Unmarshal(raw, &out)
	}
	return out
}
func protectedPath(p string) bool {
	for _, pre := range []string{"etc/pve/", "etc/corosync/", "etc/ssh/", "etc/sudoers.d/", "etc/pam.d/", "usr/local/lib/anker/", "etc/network/", "etc/apt/", "etc/default/grub", "etc/kernel/", "etc/modprobe.d/", "etc/udev/", "etc/systemd/system/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	for _, v := range []string{"etc/passwd", "etc/shadow", "etc/group", "etc/gshadow", "etc/fstab", "etc/machine-id", "etc/hostname", "etc/hosts", "etc/sudoers", "etc/anker-host.json", "etc/nsswitch.conf"} {
		if p == v {
			return true
		}
	}
	return !(strings.HasPrefix(p, "etc/") || strings.HasPrefix(p, "usr/local/"))
}

var interfaceToken = regexp.MustCompile(`\S+`)
var interfaceSuffix = regexp.MustCompile(`^(?:\.[0-9]+)*(?::[0-9]+)?$`)

func mapNetwork(data string, source, target Inventory, m Mapping) (string, []string) {
	lines := strings.Split(data, "\n")
	for index, line := range lines {
		code, comment := line, ""
		if hash := strings.Index(line, "#"); hash >= 0 {
			code, comment = line[:hash], line[hash:]
		}
		fields := strings.Fields(code)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "iface", "auto", "allow-hotplug", "allow-auto", "bridge-ports", "bond-slaves", "bond-ports", "bond-primary", "vlan-raw-device":
		default:
			continue
		}
		position := -1
		code = interfaceToken.ReplaceAllStringFunc(code, func(token string) string {
			position++
			if position == 0 || (fields[0] == "iface" && position != 1) {
				return token
			}
			if value, ok := m.Interfaces[token]; ok {
				return value
			}
			longest, replacement := "", token
			for old, newName := range m.Interfaces {
				if strings.HasPrefix(token, old) && len(old) > len(longest) {
					suffix := strings.TrimPrefix(token, old)
					if suffix != "" && interfaceSuffix.MatchString(suffix) {
						longest = old
						replacement = newName + suffix
					}
				}
			}
			return replacement
		})
		lines[index] = code + comment
	}
	return strings.Join(lines, "\n"), []string{}
}
func lineDiff(before, after string) string {
	if before == after {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- Original\n+++ Vorbereitet\n")
	for _, l := range strings.Split(strings.TrimSuffix(before, "\n"), "\n") {
		b.WriteString("-" + l + "\n")
	}
	for _, l := range strings.Split(strings.TrimSuffix(after, "\n"), "\n") {
		b.WriteString("+" + l + "\n")
	}
	return b.String()
}
func (s *Service) CreatePlan(ctx context.Context, r PlanRequest) (Plan, error) {
	release, err := s.readableLease(r.BackupID)
	if err != nil {
		return Plan{}, err
	}
	defer release()
	if err := s.verifyBackupUnlocked(r.BackupID); err != nil {
		return Plan{}, err
	}
	b, err := s.Backup(r.BackupID)
	if err != nil {
		return Plan{}, err
	}
	m, err := s.Manifest(b.ID)
	if err != nil {
		return Plan{}, err
	}
	h, err := s.Host(r.TargetID)
	if err != nil {
		return Plan{}, err
	}
	allowed := map[string]bool{"files": true, "standalone": true, "migration": true, "cluster-node": true, "cluster-disaster": true, "version": true, "topology": true}
	if !allowed[r.Scenario] {
		return Plan{}, errors.New("unbekanntes Wiederherstellungsszenario")
	}
	target, err := s.Collector.Probe(ctx, h)
	if err != nil {
		return Plan{}, err
	}
	target.Fingerprint = Fingerprint(target)
	inspection, err := s.analyzeRecovery(b, m, target, r)
	if err != nil {
		return Plan{}, err
	}
	resolvedPorts, mappingBlockers, mappingManual := validateRecoveryMapping(inspection, r)
	p := Plan{ID: ID(), BackupID: b.ID, TargetID: h.ID, Scenario: r.Scenario, CreatedAt: now(), State: "ready", Source: m.Inventory, Target: target, Mapping: r.Mapping, Steps: []Step{}, Manual: []string{}, Blockers: []string{}}
	p.Blockers = append(p.Blockers, inspection.Blockers...)
	p.Blockers = append(p.Blockers, mappingBlockers...)
	p.Manual = append(p.Manual, inspection.Manual...)
	p.Manual = append(p.Manual, mappingManual...)
	p.Mapping.Interfaces = resolvedPorts
	if _, real := s.Collector.(SSHCollector); real {
		if _, authErr := RestoreSSHArgs(h); authErr != nil {
			p.Blockers = append(p.Blockers, "Für die Ausführung ist ein separat berechtigter Wiederherstellungszugang erforderlich")
		}
	}
	if m.Status != "successful" {
		p.Blockers = append(p.Blockers, "Sicherung hat Pflichtlücken; nur manueller Export ist erlaubt")
	}
	if versionMajor(m.Inventory.PVEVersion) != versionMajor(target.PVEVersion) || !(versionMajor(target.PVEVersion) == "8" || versionMajor(target.PVEVersion) == "9") {
		p.Blockers = append(p.Blockers, "Quell-/Zielversion ist nicht für automatische Übernahme freigegeben")
		p.Manual = append(p.Manual, "Neue Zielversion frisch installieren; Paketquellen und Configs nach dem offiziellen Versionspfad prüfen.")
	}
	if r.Scenario != "files" {
		p.Manual = append(p.Manual, "Root-Dateisystem und Bootloader der Zielinstallation erhalten; Hostidentität, Storage und Gäste separat prüfen.")
		if !r.SourceOffline {
			p.Blockers = append(p.Blockers, "Vor Aktivierung einer übernommenen Identität muss der alte Host ausgeschaltet oder isoliert sein")
		}
	}
	switch r.Scenario {
	case "cluster-node":
		p.Manual = append(p.Manual, "Lebenden Clusterzustand erhalten. Alten Node kontrolliert entfernen, Ziel vorbereiten und gemäß passender Proxmox-Anleitung beitreten. config.db nicht pauschal übernehmen.")
	case "cluster-disaster":
		p.Manual = append(p.Manual, "Isoliertes Recovery-Netz verwenden; geeigneten gemeinsamen Stand wählen. Quorum, HA, Ceph und Storage im Laborverfahren prüfen; keine automatische Quorum-Erzwingung.")
	case "topology":
		p.Manual = append(p.Manual, "Topologiewechsel erfordert neu geprüfte Clusteridentität und getrennte Node-/Cluster-Konfiguration.")
	case "version":
		p.Manual = append(p.Manual, "Versionsmigration und In-place-Upgrade sind unterschiedliche Abläufe; den offiziell unterstützten Pfad und Vorprüfungen verwenden.")
	}
	selected, err := recoverySelection(m, r)
	if err != nil {
		return p, err
	}
	root := filepath.Join(s.Root, "plans", p.ID)
	if err = os.MkdirAll(filepath.Join(root, "prepared-files"), 0700); err != nil {
		return p, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(root)
		}
	}()
	targetHashes := fileHashes(target)
	count := 0
	targetData, _ := json.Marshal(target)
	requestBytes := len(targetData) + (1 << 20)
	for _, e := range m.Entries {
		if e.Type == "directory" {
			continue
		}
		if !selected[e.Path] {
			continue
		}
		delete(selected, e.Path)
		step := Step{Path: e.Path, Action: "apply", Secret: e.Secret}
		data, fileEntry, readErr := s.readFileUnlocked(b.ID, e.Path, 64<<20)
		if readErr != nil {
			return p, readErr
		}
		step.Secret = step.Secret || fileEntry.Secret
		prepared := data
		if reason := fileManualReason(e, m.Inventory, target); reason != "" {
			step.Action = "manual"
			step.Reason = reason
		}
		if strings.HasPrefix(e.Path, "etc/network/") {
			mapped, missing := mapNetwork(string(data), m.Inventory, target, Mapping{Interfaces: resolvedPorts})
			prepared = []byte(mapped)
			p.Blockers = append(p.Blockers, missing...)
			if !r.ConsoleConfirmed {
				p.Blockers = append(p.Blockers, "Für Netzwerkänderungen Konsolenzugang bestätigen")
			}
			step.Action = "manual"
			step.Reason = "Netzwerkdatei vorbereitet; Aktivierung manuell über Konsole, lokaler Rückfallwächter fehlt"
		}
		if step.Action == "apply" {
			requestBytes += 4*((len(prepared)+2)/3) + 4096
			if target.Details["file_hashes"] == nil {
				p.Blockers = append(p.Blockers, "Ziel liefert keine Dateiprüfsummen für sichere Vorbedingungen")
			}
			step.BeforeSHA = targetHashes[e.Path]
			if step.BeforeSHA == "" {
				step.BeforeSHA = "missing"
			}
			count++
		}
		if e.Type == "file" {
			step.PreparedSHA = Hash(prepared)
			step.Diff = lineDiff(string(data), string(prepared))
			full, err := safeJoin(filepath.Join(root, "prepared-files"), e.Path)
			if err != nil {
				return p, err
			}
			if err = atomicWrite(full, prepared, 0600); err != nil {
				return p, err
			}
		}
		p.Steps = append(p.Steps, step)
	}
	if len(selected) > 0 {
		return p, errors.New("Dateiauswahl enthält nicht gesicherte Dateien")
	}
	if requestBytes > 32<<20 {
		p.Blockers = append(p.Blockers, "Dateiauswahl überschreitet das 32-MiB-Limit des Hostprotokolls. Kleinere Einzeldateipläne erstellen oder Dateien manuell herunterladen und übernehmen.")
	}
	sort.Slice(p.Steps, func(i, j int) bool { return p.Steps[i].Path < p.Steps[j].Path })
	if count == 0 {
		p.State = "manual"
	}
	if r.Scenario != "files" && !s.Demo {
		p.State = "manual"
		p.Manual = append(p.Manual, "Dieses Gesamtszenario hat noch keinen realen Hardware-/Cluster-Labortest. Vorbereitete Dateien und manuelle Anleitung exportieren; automatische Gesamtausführung bleibt gesperrt.")
	}
	uniqueBlockers := []string{}
	for _, blocker := range p.Blockers {
		uniqueBlockers = appendUnique(uniqueBlockers, blocker)
	}
	p.Blockers = uniqueBlockers
	if len(p.Blockers) > 0 {
		p.State = "blocked"
	}
	data, err := os.ReadFile(filepath.Join(s.backupDir(b), "manifest.json"))
	if err != nil {
		return p, err
	}
	if err = atomicWrite(filepath.Join(root, "manifest.json"), data, 0600); err != nil {
		return p, err
	}
	if err = writeJSON(filepath.Join(root, "plan.json"), p); err != nil {
		return p, err
	}
	if err = writeJSON(filepath.Join(root, "mapping.json"), p.Mapping); err != nil {
		return p, err
	}
	if err = atomicWrite(filepath.Join(root, "WIEDERHERSTELLUNG.md"), []byte(planGuide(p, m)), 0600); err != nil {
		return p, err
	}
	if err = syncTree(root); err != nil {
		return p, err
	}
	if err = syncDir(filepath.Dir(root)); err != nil {
		return p, err
	}
	if err = s.Store.Put("plans", p.ID, p); err != nil {
		return p, err
	}
	cleanup = false
	return p, nil
}
func (s *Service) savePlan(p Plan) error {
	if err := writeJSON(filepath.Join(s.Root, "plans", p.ID, "plan.json"), p); err != nil {
		return err
	}
	return s.Store.Put("plans", p.ID, p)
}
func planGuide(p Plan, m Manifest) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Zielbezogene Wiederherstellung\n\nPlan %s · Szenario %s · Status %s\n\nQuelle %s (%s) → Ziel %s (%s)\n\n", p.ID, p.Scenario, p.State, p.Source.Hostname, p.Source.PVEVersion, p.Target.Hostname, p.Target.PVEVersion))
	b.WriteString("Originaldateien stehen im original/-Verzeichnis des vollständigen Planexports. prepared-files/ enthält ausschließlich vorbereitete Configs. Vor Nutzung Target neu prüfen; mapping.json und plan.json dokumentieren Entscheidungen und Vorbedingungen.\n\n## Ausführung und Rücksetzung\n\nBetroffene Dienste und fremde Schreiber vor Übernahme anhalten. Der Host speichert journal.json unter /var/lib/anker-host/rollback/" + p.ID + "/. Nach Abbruch Hostzustand abgleichen und nicht blind erneut anwenden. Rücksetzung benötigt eine separate Plan-ID-Bestätigung und erhält Fremdänderungen. Netzwerkdateien nur manuell mit Konsolenzugang und geprüftem Rückweg übernehmen.\n\n## Haltepunkte\n\n")
	for _, x := range p.Blockers {
		b.WriteString("- " + x + "\n")
	}
	for _, x := range p.Manual {
		b.WriteString("- " + x + "\n")
	}
	b.WriteString("\n## Dateien\n\n")
	for _, x := range p.Steps {
		b.WriteString(fmt.Sprintf("- %s: %s. %s\n", x.Path, x.Action, x.Reason))
	}
	b.WriteString("\nDie folgende Anleitung bezieht sich auf original/: dort checksums.sha256 prüfen und original-files/ für Dateien mit Originalmetadaten verwenden.\n\n" + recoveryGuide(m))
	return b.String()
}
func (s *Service) ExportPlan(id string, w io.Writer) error {
	lock := s.planLock(id)
	lock.RLock()
	defer lock.RUnlock()
	p, err := s.Plan(id)
	if err != nil {
		return err
	}
	release, err := s.readableLease(p.BackupID)
	if err != nil {
		return err
	}
	defer release()
	if err = s.verifyBackupUnlocked(p.BackupID); err != nil {
		return err
	}
	if err = s.verifyPlanFiles(p); err != nil {
		return err
	}
	b, err := s.Backup(p.BackupID)
	if err != nil {
		return err
	}
	m, err := s.Manifest(p.BackupID)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(w)
	root := filepath.Join(s.Root, "plans", id)
	if err = tarTree(tw, root); err != nil {
		tw.Close()
		return err
	}
	if err = tarPrefix(tw, s.backupDir(b), "original/"); err != nil {
		tw.Close()
		return err
	}
	if err = s.tarOriginalFiles(tw, b, m, "original/"); err != nil {
		tw.Close()
		return err
	}
	return tw.Close()
}
func (s *Service) verifyPlanFiles(p Plan) error {
	root := filepath.Join(s.Root, "plans", p.ID)
	for name, expected := range map[string]any{"plan.json": p, "mapping.json": p.Mapping} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		want, _ := json.Marshal(expected)
		var actual, original any
		if json.Unmarshal(data, &actual) != nil || json.Unmarshal(want, &original) != nil || !reflect.DeepEqual(actual, original) {
			return fmt.Errorf("Planbestandteil wurde verändert: %s", name)
		}
	}
	b, err := s.Backup(p.BackupID)
	if err != nil {
		return err
	}
	manifest, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return err
	}
	if Hash(manifest) != b.ManifestSHA {
		return errors.New("Planmanifest wurde verändert")
	}
	for _, step := range p.Steps {
		if step.PreparedSHA == "" {
			continue
		}
		file, err := safeJoin(filepath.Join(root, "prepared-files"), step.Path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if Hash(data) != step.PreparedSHA {
			return fmt.Errorf("Vorbereitete Datei wurde verändert: %s", step.Path)
		}
	}
	return nil
}
func tarPrefix(w *tar.Writer, root, prefix string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "archive.tar.gz" {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe export link")
		}
		h, err := tar.FileInfoHeader(st, "")
		if err != nil {
			return err
		}
		h.Name = prefix + filepath.ToSlash(rel)
		if err = w.WriteHeader(h); err != nil {
			return err
		}
		if st.Mode().IsRegular() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, f)
			f.Close()
			return err
		}
		return nil
	})
}
