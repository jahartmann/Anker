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
	for _, pre := range []string{"etc/pve/", "etc/corosync/", "etc/ssh/ssh_host_", "etc/apt/", "etc/default/grub", "etc/kernel/", "etc/modprobe.d/", "etc/udev/", "etc/systemd/system/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	for _, v := range []string{"etc/passwd", "etc/shadow", "etc/group", "etc/gshadow", "etc/fstab", "etc/machine-id", "etc/hostname", "etc/hosts"} {
		if p == v {
			return true
		}
	}
	return !(strings.HasPrefix(p, "etc/") || strings.HasPrefix(p, "usr/local/"))
}

var interfaceToken = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9_.:-]*`)

func mapNetwork(data string, source, target Inventory, m Mapping) (string, []string) {
	targetNames := map[string]bool{}
	for _, i := range target.Interfaces {
		targetNames[i.Name] = true
	}
	missing := []string{}
	oldNames := map[string]string{}
	for _, i := range source.Interfaces {
		if i.Name == "lo" || strings.HasPrefix(i.Name, "vmbr") || strings.HasPrefix(i.Name, "bond") || strings.Contains(i.Name, ".") {
			continue
		}
		if !strings.Contains(data, i.Name) {
			continue
		}
		newName := m.Interfaces[i.Name]
		if newName == "" && targetNames[i.Name] {
			newName = i.Name
		}
		if !targetNames[newName] {
			missing = append(missing, "Netzwerkport "+i.Name+" muss einem vorhandenen Zielport zugeordnet werden")
			continue
		}
		oldNames[i.Name] = newName
	}
	result := interfaceToken.ReplaceAllStringFunc(data, func(token string) string {
		if v, ok := oldNames[token]; ok {
			return v
		}
		for old, newName := range oldNames {
			if strings.HasPrefix(token, old+".") {
				return newName + strings.TrimPrefix(token, old)
			}
		}
		return token
	})
	return result, missing
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
	if err := s.EnsureReadable(r.BackupID); err != nil {
		return Plan{}, err
	}
	if err := s.VerifyBackup(r.BackupID); err != nil {
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
	p := Plan{ID: ID(), BackupID: b.ID, TargetID: h.ID, Scenario: r.Scenario, CreatedAt: now(), State: "ready", Source: m.Inventory, Target: target, Mapping: r.Mapping, Steps: []Step{}, Manual: []string{}, Blockers: []string{}}
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
	selected := map[string]bool{}
	for _, file := range r.Files {
		if err = ValidPath(file); err != nil {
			return p, err
		}
		selected[file] = true
	}
	if r.Scenario == "files" && len(selected) == 0 {
		return p, errors.New("mindestens eine Datei auswählen")
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
	for _, e := range m.Entries {
		if e.Type == "directory" {
			continue
		}
		if len(selected) > 0 && !selected[e.Path] {
			continue
		}
		delete(selected, e.Path)
		step := Step{Path: e.Path, Action: "apply", Secret: e.Secret}
		data, _, readErr := s.ReadFile(b.ID, e.Path)
		if readErr != nil {
			return p, readErr
		}
		prepared := data
		if e.Type != "file" || protectedPath(e.Path) || len(e.XAttrs) > 0 {
			step.Action = "manual"
			step.Reason = "Hardware-, Identitäts-, Cluster- oder Metadatenbestandteil gezielt manuell übernehmen"
		}
		if strings.HasPrefix(e.Path, "etc/network/") {
			mapped, missing := mapNetwork(string(data), m.Inventory, target, r.Mapping)
			prepared = []byte(mapped)
			p.Blockers = append(p.Blockers, missing...)
			if !r.ConsoleConfirmed {
				p.Blockers = append(p.Blockers, "Für Netzwerkänderungen Konsolenzugang bestätigen")
			}
			step.Reason = "Portzuordnung geprüft; Aktivierung und Erreichbarkeit über Konsole prüfen"
		}
		if step.Action == "apply" {
			if target.Details["file_hashes"] == nil {
				p.Blockers = append(p.Blockers, "Ziel liefert keine Dateiprüfsummen für sichere Vorbedingungen")
			}
			step.BeforeSHA = targetHashes[e.Path]
			if step.BeforeSHA == "" {
				step.BeforeSHA = "missing"
			}
			step.PreparedSHA = Hash(prepared)
			step.Diff = lineDiff(string(data), string(prepared))
			full, err := safeJoin(filepath.Join(root, "prepared-files"), e.Path)
			if err != nil {
				return p, err
			}
			if err = atomicWrite(full, prepared, 0600); err != nil {
				return p, err
			}
			count++
		}
		p.Steps = append(p.Steps, step)
	}
	if len(selected) > 0 {
		return p, errors.New("Dateiauswahl enthält nicht gesicherte Dateien")
	}
	sort.Slice(p.Steps, func(i, j int) bool { return p.Steps[i].Path < p.Steps[j].Path })
	if count == 0 {
		p.State = "manual"
	}
	if r.Scenario != "files" && !s.Demo {
		p.State = "manual"
		p.Manual = append(p.Manual, "Dieses Gesamtszenario hat noch keinen realen Hardware-/Cluster-Labortest. Vorbereitete Dateien und manuelle Anleitung exportieren; automatische Gesamtausführung bleibt gesperrt.")
	}
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
	if err = s.savePlan(p); err != nil {
		return p, err
	}
	if err = writeJSON(filepath.Join(root, "mapping.json"), p.Mapping); err != nil {
		return p, err
	}
	if err = atomicWrite(filepath.Join(root, "WIEDERHERSTELLUNG.md"), []byte(planGuide(p, m)), 0600); err != nil {
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
	b.WriteString("Originaldateien stehen im original/-Verzeichnis des vollständigen Planexports. prepared-files/ enthält ausschließlich vorbereitete Configs. Vor Nutzung Target neu prüfen; mapping.json und plan.json dokumentieren Entscheidungen und Vorbedingungen.\n\n## Haltepunkte\n\n")
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
	b.WriteString("\n" + recoveryGuide(m))
	return b.String()
}
func (s *Service) ExportPlan(id string, w io.Writer) error {
	p, err := s.Plan(id)
	if err != nil {
		return err
	}
	if err = s.EnsureReadable(p.BackupID); err != nil {
		return err
	}
	if err = s.VerifyBackup(p.BackupID); err != nil {
		return err
	}
	tw := tar.NewWriter(w)
	root := filepath.Join(s.Root, "plans", id)
	if err = tarTree(tw, root); err != nil {
		tw.Close()
		return err
	}
	b, _ := s.Backup(p.BackupID)
	if err = tarPrefix(tw, s.backupDir(b), "original/"); err != nil {
		tw.Close()
		return err
	}
	return tw.Close()
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
