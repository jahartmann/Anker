package anker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type RecoveryPort struct {
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	PCI       string `json:"pci"`
	Reason    string `json:"reason"`
	Suggested string `json:"suggested"`
}
type RecoveryStorage struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Reason    string `json:"reason"`
	Suggested string `json:"suggested"`
}
type RecoveryInspection struct {
	Source                Inventory         `json:"source"`
	Target                Inventory         `json:"target"`
	Ports                 []RecoveryPort    `json:"ports"`
	TargetPorts           []Interface       `json:"target_ports"`
	Storage               []RecoveryStorage `json:"storage"`
	TargetStorage         []RecoveryStorage `json:"target_storage"`
	Blockers              []string          `json:"blockers"`
	Warnings              []string          `json:"warnings"`
	Manual                []string          `json:"manual"`
	RequiresConsole       bool              `json:"requires_console"`
	RequiresSourceOffline bool              `json:"requires_source_offline"`
	Automatic             bool              `json:"automatic"`
}

func recoverySelection(m Manifest, r PlanRequest) (map[string]bool, error) {
	switch r.Scenario {
	case "files", "standalone", "migration", "cluster-node", "cluster-disaster", "version", "topology":
	default:
		return nil, errors.New("unbekanntes Wiederherstellungsszenario")
	}
	selected := map[string]bool{}
	for _, p := range r.Files {
		if err := ValidPath(p); err != nil {
			return nil, err
		}
		selected[p] = true
	}
	if r.Scenario == "files" && len(selected) == 0 {
		return nil, errors.New("mindestens eine Datei auswählen")
	}
	if len(selected) == 0 {
		for _, e := range m.Entries {
			if e.Type != "directory" {
				selected[e.Path] = true
			}
		}
	}
	available := map[string]bool{}
	for _, e := range m.Entries {
		if e.Type != "directory" {
			available[e.Path] = true
		}
	}
	for p := range selected {
		if !available[p] {
			return nil, errors.New("Dateiauswahl enthält nicht gesicherte Dateien")
		}
	}
	return selected, nil
}
func (s *Service) InspectRecovery(ctx context.Context, r PlanRequest) (RecoveryInspection, error) {
	release, err := s.readableLease(r.BackupID)
	if err != nil {
		return RecoveryInspection{}, err
	}
	defer release()
	if err = s.checkBackupUnlocked(r.BackupID); err != nil {
		return RecoveryInspection{}, err
	}
	b, err := s.Backup(r.BackupID)
	if err != nil {
		return RecoveryInspection{}, err
	}
	m, err := s.Manifest(b.ID)
	if err != nil {
		return RecoveryInspection{}, err
	}
	if _, err = recoverySelection(m, r); err != nil {
		return RecoveryInspection{}, err
	}
	h, err := s.Host(r.TargetID)
	if err != nil {
		return RecoveryInspection{}, err
	}
	target, err := s.Collector.Probe(ctx, h)
	if err != nil {
		return RecoveryInspection{}, err
	}
	target.Fingerprint = Fingerprint(target)
	return s.analyzeRecovery(b, m, target, r)
}

var physicalName = regexp.MustCompile(`^(?:eth[0-9]+|en[a-zA-Z0-9_-]+)$`)

func physicalPort(i Interface) bool {
	// Explicit classification takes precedence over legacy names, even for an eno-looking VM NIC.
	if i.Type != "" {
		return i.Type == "physical" && i.Name != "lo"
	}
	if i.Physical {
		return i.Name != "lo"
	}
	if strings.Contains(i.Name, ".") || strings.Contains(i.Name, ":") {
		return false
	}
	for _, pre := range []string{"lo", "vmbr", "bond", "veth", "tap", "fwbr", "fwln", "fwpr", "tun", "vxlan", "dummy", "docker", "br"} {
		if strings.HasPrefix(i.Name, pre) {
			return false
		}
	}
	return (i.PCI != "" && i.PCI != "device") || physicalName.MatchString(i.Name)
}
func virtualName(name string) bool {
	for _, pre := range []string{"lo", "vmbr", "bond", "veth", "tap", "fwbr", "fwln", "fwpr", "tun", "vxlan", "dummy", "br"} {
		if strings.HasPrefix(name, pre) {
			return true
		}
	}
	return false
}
func physicalPorts(i Inventory) []Interface {
	out := []Interface{}
	for _, p := range i.Interfaces {
		if physicalPort(p) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}
func suggestedPort(p Interface, source, target Inventory, sameHost bool) string {
	candidates := []string{}
	for _, t := range physicalPorts(target) {
		if p.MAC != "" && t.MAC != "" && !strings.EqualFold(p.MAC, t.MAC) {
			continue
		}
		if (p.MAC != "" && strings.EqualFold(p.MAC, t.MAC)) || (p.PCI != "" && p.PCI == t.PCI) || (sameHost && source.Hostname != "" && source.Hostname == target.Hostname && p.Name == t.Name) {
			candidates = append(candidates, t.Name)
		}
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}
func appendUnique(items []string, value string) []string {
	for _, x := range items {
		if x == value {
			return items
		}
	}
	return append(items, value)
}

// parseNetwork follows interface declarations, bridge/bond members and VLAN parents.
// Shell hooks and unknown topology directives cannot be safely interpreted as port mapping.
func networkReferences(files map[string]string, source Inventory) ([]string, []string) {
	refs := map[string]bool{}
	deps := map[string][]string{}
	issues := []string{}
	known := map[string]Interface{}
	for _, p := range source.Interfaces {
		known[p.Name] = p
	}
	for path, data := range files {
		current := ""
		for _, line := range strings.Split(data, "\n") {
			line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			key := f[0]
			switch key {
			case "iface":
				if len(f) < 4 {
					issues = appendUnique(issues, "Ungültige Netzwerkdeklaration in "+path)
					continue
				}
				current = f[1]
				refs[current] = true
			case "auto", "allow-hotplug", "allow-auto":
				for _, n := range f[1:] {
					refs[n] = true
				}
			case "bridge-ports", "bond-slaves", "bond-ports", "vlan-raw-device":
				for _, n := range f[1:] {
					if n != "none" {
						refs[n] = true
						deps[current] = append(deps[current], n)
					}
				}
			case "source", "source-directory", "pre-up", "up", "post-up", "pre-down", "down", "post-down":
				issues = appendUnique(issues, "Netzwerkdirektive "+key+" in "+path+" erfordert manuelle Prüfung")
			default:
				allowed := map[string]bool{"address": true, "gateway": true, "netmask": true, "broadcast": true, "mtu": true, "dns-nameservers": true, "dns-search": true, "bridge-stp": true, "bridge-fd": true, "bridge-vlan-aware": true, "bridge-vids": true, "bridge-pvid": true, "bond-mode": true, "bond-miimon": true, "bond-xmit-hash-policy": true, "bond-primary": true, "bond-lacp-rate": true, "bond-min-links": true, "bond-updelay": true, "bond-downdelay": true, "vlan-id": true, "hwaddress": true, "metric": true, "pointopoint": true, "accept_ra": true, "autoconf": true}
				if !allowed[key] {
					issues = appendUnique(issues, "Nicht unterstützte Netzwerkdirektive "+key+" in "+path+" erfordert manuelle Prüfung")
				}
			}
		}
	}
	out := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string)
	visit = func(n string) {
		if visited[n] {
			return
		}
		visited[n] = true
		if colon := strings.LastIndex(n, ":"); colon > 0 {
			if _, err := strconv.Atoi(n[colon+1:]); err == nil {
				visit(n[:colon])
				return
			}
		}
		if dot := strings.LastIndex(n, "."); dot > 0 {
			if _, err := strconv.Atoi(n[dot+1:]); err == nil {
				visit(n[:dot])
				return
			}
		}
		if p, ok := known[n]; ok {
			if physicalPort(p) {
				out[n] = true
				return
			}
			if p.Type != "" && !virtualName(n) {
				issues = appendUnique(issues, "Referenz "+n+" ist kein physischer Netzwerkport")
			}
		}
		for _, child := range deps[n] {
			visit(child)
		}
		if _, ok := known[n]; !ok && !virtualName(n) && len(deps[n]) == 0 {
			issues = appendUnique(issues, "Unbekannter physischer Netzwerkport "+n+"; Quellinventar und Config manuell prüfen")
		}
	}
	for n := range refs {
		visit(n)
	}
	names := []string{}
	for n := range out {
		names = append(names, n)
	}
	sort.Strings(names)
	sort.Strings(issues)
	return names, issues
}
func storageDefinitions(data string) []RecoveryStorage {
	out := []RecoveryStorage{}
	var cur *RecoveryStorage
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if strings.HasSuffix(f[0], ":") && len(f) == 2 {
			out = append(out, RecoveryStorage{ID: f[1], Kind: strings.TrimSuffix(f[0], ":"), Reason: "Konfigurierte Storage-ID; Bestand und Zugriff manuell prüfen"})
			cur = &out[len(out)-1]
			continue
		}
		if cur != nil && len(f) > 1 {
			switch f[0] {
			case "path", "pool", "vgname", "server", "monhost":
				if cur.Path != "" {
					cur.Path += "; "
				}
				cur.Path += strings.Join(f[1:], " ")
			}
		}
	}
	return out
}

var volumeReference = regexp.MustCompile(`(?:^|[\s,=])([a-zA-Z][a-zA-Z0-9_.-]*):[^\s,]+`)

func targetStorages(i Inventory) []RecoveryStorage {
	out := []RecoveryStorage{}
	var text string
	if json.Unmarshal(i.Details["storage_config"], &text) == nil {
		return storageDefinitions(text)
	}
	if json.Unmarshal(i.Details["storage"], &text) == nil {
		for _, line := range strings.Split(text, "\n") {
			f := strings.Fields(line)
			if len(f) > 1 && f[0] != "Name" {
				out = append(out, RecoveryStorage{ID: f[0], Kind: f[1], Reason: "Vorhandene Ziel-Storage-ID; manuelle Bestandsprüfung"})
			}
		}
	}
	return out
}
func identityNames(i Inventory, key string) map[string]string {
	out := map[string]string{}
	json.Unmarshal(i.Details[key], &out)
	return out
}
func fileManualReason(e Entry, source, target Inventory) string {
	if e.Type != "file" || protectedPath(e.Path) || len(e.XAttrs) > 0 {
		return "Hardware-, Identitäts-, Cluster- oder Metadatenbestandteil gezielt manuell übernehmen"
	}
	for _, key := range []string{"users", "groups"} {
		id := e.UID
		if key == "groups" {
			id = e.GID
		}
		a, b := identityNames(source, key), identityNames(target, key)
		name := a[strconv.Itoa(id)]
		if name == "" || name != b[strconv.Itoa(id)] {
			return "Benutzer-/Gruppenidentität der numerischen UID/GID fehlt oder weicht ab; manuell zuordnen"
		}
	}
	var meta map[string]json.RawMessage
	if json.Unmarshal(target.Details["file_metadata"], &meta) != nil || meta == nil {
		return "Zielmetadaten fehlen; Eigentümer, Rechte und ACLs/xattrs manuell prüfen"
	}
	hash := fileHashes(target)[e.Path]
	if hash == "unreadable" || hash == "too-large" || hash == "unsupported" || strings.HasPrefix(hash, "symlink:") {
		return "Zielinhalt ist nicht sicher lesbar oder kein gewöhnliches File; manuell prüfen"
	}
	if hash != "" && hash != "missing" {
		raw, ok := meta[e.Path]
		if !ok {
			return "Zielmetadaten fehlen; manuell prüfen"
		}
		var m struct {
			Type    string            `json:"type"`
			Mode    *uint32           `json:"mode"`
			UID     *int              `json:"uid"`
			GID     *int              `json:"gid"`
			XAttrs  map[string]string `json:"xattrs"`
			Warning string            `json:"metadata_warning"`
		}
		if json.Unmarshal(raw, &m) != nil || m.Type != "file" || m.Mode == nil || m.UID == nil || m.GID == nil || m.XAttrs == nil || len(m.XAttrs) > 0 || m.Warning != "" {
			return "Zielmetadaten/ACLs/xattrs sind unvollständig oder erfordern manuelle Erhaltung"
		}
	}
	return ""
}
func requiredCommandFailures(i Inventory, keys []string, strict bool) []string {
	var results map[string]struct {
		State string `json:"state"`
		Error string `json:"error"`
	}
	if json.Unmarshal(i.Details["command_results"], &results) != nil {
		if strict {
			return []string{"Ziel liefert keine nachvollziehbaren Inventarabfragen; Hosthelfer erneut anbinden"}
		}
		return nil
	}
	out := []string{}
	for _, key := range keys {
		result, ok := results[key]
		if key == "cluster" && i.ClusterID == "" && ok && result.State == "not_applicable" {
			continue
		}
		if !ok || result.State != "ok" {
			out = append(out, "Erforderliche Ziel-Inventarabfrage "+key+" ist unbekannt oder fehlgeschlagen")
		}
	}
	return out
}
func (s *Service) analyzeRecovery(b Backup, m Manifest, target Inventory, r PlanRequest) (RecoveryInspection, error) {
	selected, err := recoverySelection(m, r)
	if err != nil {
		return RecoveryInspection{}, err
	}
	i := RecoveryInspection{Source: m.Inventory, Target: target, Ports: []RecoveryPort{}, TargetPorts: physicalPorts(target), Storage: []RecoveryStorage{}, TargetStorage: []RecoveryStorage{}, Blockers: []string{}, Warnings: []string{}, Manual: []string{}, RequiresSourceOffline: r.Scenario != "files", Automatic: true}
	network := map[string]string{}
	definitions := []RecoveryStorage{}
	usedStorage := map[string]string{}
	selectedStorage := false
	hasAutomaticFiles := false
	for _, e := range m.Entries {
		// storage.cfg is read only to resolve selected guest references, never creates unrelated requirements.
		if e.Path == "etc/pve/storage.cfg" && e.Type == "file" {
			data, _, readErr := s.readFileUnlocked(b.ID, e.Path, 64<<20)
			if readErr != nil {
				return i, readErr
			}
			definitions = storageDefinitions(string(data))
		}
		if !selected[e.Path] {
			continue
		}
		reason := fileManualReason(e, m.Inventory, target)
		if reason != "" {
			i.Automatic = false
			i.Manual = appendUnique(i.Manual, e.Path+": "+reason)
		} else if e.Type == "file" && !strings.HasPrefix(e.Path, "etc/network/") {
			hasAutomaticFiles = true
		}
		if e.Type != "file" {
			continue
		}
		if strings.HasPrefix(e.Path, "etc/network/") || strings.HasPrefix(e.Path, "etc/pve/") || e.Path == "etc/fstab" {
			data, _, readErr := s.readFileUnlocked(b.ID, e.Path, 64<<20)
			if readErr != nil {
				return i, readErr
			}
			if strings.HasPrefix(e.Path, "etc/network/") {
				network[e.Path] = string(data)
				i.RequiresConsole = true
				i.Automatic = false
			}
			if e.Path == "etc/pve/storage.cfg" {
				selectedStorage = true
			}
			if strings.Contains(e.Path, "/qemu-server/") || strings.Contains(e.Path, "/lxc/") {
				for _, match := range volumeReference.FindAllStringSubmatch(string(data), -1) {
					usedStorage[match[1]] = e.Path
				}
			}
			if e.Path == "etc/fstab" {
				i.Manual = appendUnique(i.Manual, "Mounts und UUIDs aus /etc/fstab gegen vorhandenen Zielbestand prüfen; keine Datenträgerformatierung")
			}
		}
	}
	if hasAutomaticFiles && target.Details["file_hashes"] == nil {
		i.Blockers = append(i.Blockers, "Ziel liefert keine Dateiprüfsummen für sichere Vorbedingungen")
	}
	names, issues := networkReferences(network, m.Inventory)
	i.Blockers = append(i.Blockers, issues...)
	for _, name := range names {
		for _, p := range m.Inventory.Interfaces {
			if p.Name == name {
				i.Ports = append(i.Ports, RecoveryPort{Name: p.Name, MAC: p.MAC, PCI: p.PCI, Reason: "Von ausgewählter Netzwerkkonfiguration referenzierter physischer Port", Suggested: suggestedPort(p, m.Inventory, target, b.HostID == r.TargetID)})
			}
		}
	}
	if len(network) > 0 {
		i.Manual = appendUnique(i.Manual, "Netzwerkdateien werden vorbereitet; Aktivierung über Konsole bleibt manuell, weil kein unabhängiger lokaler Rückfallwächter vorhanden ist")
	}
	for _, d := range definitions {
		if selectedStorage || usedStorage[d.ID] != "" {
			i.Storage = append(i.Storage, d)
			delete(usedStorage, d.ID)
		}
	}
	for id, path := range usedStorage {
		i.Storage = append(i.Storage, RecoveryStorage{ID: id, Kind: "volume", Path: path, Reason: "Gast referenziert Storage-ID; Definition oder Zielzugriff manuell prüfen"})
	}
	sort.Slice(i.Storage, func(a, b int) bool { return i.Storage[a].ID < i.Storage[b].ID })
	if len(i.Storage) > 0 {
		i.TargetStorage = targetStorages(target)
		i.Automatic = false
		i.Manual = appendUnique(i.Manual, "Storage-Zuordnungen sind manuelle Entscheidungen; Pools, Volumes, Mounts und Datenbestand erhalten, keine automatische Formatierung oder Umbenennung")
	}
	if r.Scenario == "cluster-node" {
		i.Manual = appendUnique(i.Manual, "Vorhandene gemeinsame Storage- und Clusterkonfiguration des lebenden Clusters erhalten; gesicherte Storage-IDs dienen nur zum Abgleich beim kontrollierten Node-Beitritt")
	}
	if r.Scenario != "files" {
		i.Automatic = false
		i.Manual = appendUnique(i.Manual, "Gesamtszenario erfordert manuellen Wiederaufbau und geprüfte Hostidentität")
	}
	if m.Status != "successful" {
		i.Blockers = append(i.Blockers, "Sicherung hat Pflichtlücken; nur manueller Export ist erlaubt")
	}
	if versionMajor(m.Inventory.PVEVersion) != versionMajor(target.PVEVersion) || !(versionMajor(target.PVEVersion) == "8" || versionMajor(target.PVEVersion) == "9") {
		i.Blockers = append(i.Blockers, "Quell-/Zielversion ist nicht für automatische Übernahme freigegeben")
	}
	_, strict := s.Collector.(SSHCollector)
	if strict {
		var caps struct {
			Protocol int `json:"restore_protocol"`
		}
		json.Unmarshal(target.Details["capabilities"], &caps)
		if caps.Protocol != 2 {
			i.Blockers = append(i.Blockers, "Hosthelfer unterstützt Wiederherstellungsprotokoll 2 noch nicht; Ziel erneut anbinden")
		}
	}
	keys := []string{"pve_version"}
	if strict {
		keys = append(keys, "file_inventory", "users", "groups")
	}
	if len(network) > 0 {
		keys = append(keys, "interfaces", "addresses", "routes")
	}
	if len(i.Storage) > 0 {
		keys = append(keys, "storage")
	}
	if r.Scenario != "files" {
		keys = append(keys, "disks", "packages")
		if r.Scenario == "cluster-node" || r.Scenario == "cluster-disaster" || r.Scenario == "topology" {
			keys = append(keys, "cluster")
		}
	}
	i.Blockers = append(i.Blockers, requiredCommandFailures(target, keys, strict)...)
	if len(i.Blockers) > 0 {
		i.Automatic = false
	}
	return i, nil
}
func validateRecoveryMapping(i RecoveryInspection, r PlanRequest) (map[string]string, []string, []string) {
	resolved := map[string]string{}
	blockers := []string{}
	manual := []string{}
	needed := map[string]bool{}
	targets := map[string]bool{}
	used := map[string]string{}
	for _, p := range i.TargetPorts {
		targets[p.Name] = true
	}
	for _, p := range i.Ports {
		needed[p.Name] = true
		name := r.Mapping.Interfaces[p.Name]
		if name == "" {
			name = p.Suggested
		}
		if !targets[name] {
			blockers = append(blockers, "Netzwerkport "+p.Name+" muss einem vorhandenen physischen Zielport zugeordnet werden")
			continue
		}
		if other := used[name]; other != "" {
			blockers = append(blockers, "Zielport "+name+" ist mehrfach zugeordnet: "+other+" und "+p.Name)
			continue
		}
		used[name] = p.Name
		resolved[p.Name] = name
	}
	for name := range r.Mapping.Interfaces {
		if !needed[name] {
			blockers = append(blockers, "Nicht benötigte oder unbekannte Netzwerkzuordnung: "+name)
		}
	}
	storage := map[string]bool{}
	for _, d := range i.Storage {
		storage[d.ID] = true
	}
	for id, value := range r.Mapping.Storage {
		if !storage[id] || strings.TrimSpace(value) == "" {
			blockers = append(blockers, "Unbekannte oder leere Storage-Entscheidung: "+id)
			continue
		}
		manual = append(manual, fmt.Sprintf("Storage %s → %s: manuelle Entscheidung; wird nicht automatisch umgesetzt", id, value))
	}
	if r.Mapping.Hostname != "" {
		manual = append(manual, "Hostidentität "+i.Source.Hostname+" → "+r.Mapping.Hostname+": manuelle Entscheidung; Configs und Clusteridentität gezielt prüfen und manuell umsetzen")
	}
	if r.Mapping.Address != "" {
		manual = append(manual, "Gewünschte Zieladresse "+r.Mapping.Address+": manuell konfigurieren und Erreichbarkeit prüfen")
	}
	sort.Strings(blockers)
	sort.Strings(manual)
	return resolved, blockers, manual
}
