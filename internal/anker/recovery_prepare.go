package anker

import (
	"fmt"
	"net/netip"
	"path"
	"regexp"
	"sort"
	"strings"
)

// prepareRecoveryFile is pure. Source context must come from LoadRecoverySnapshot;
// callers must keep Content private and treat ManualTasks as unresolved dependencies.
func prepareRecoveryFile(e Entry, source []byte, snapshot RecoverySnapshot, mapping Mapping, scenario string) (PreparedRecoveryFile, error) {
	p := PreparedRecoveryFile{Content: append([]byte(nil), source...), Destination: e.Path, Effects: []RecoveryEffect{}, ManualTasks: []RecoveryTask{}}
	if err := ValidPath(e.Path); err != nil {
		return p, err
	}
	if e.Type != "file" {
		p.task("file-type", e.Path, "Dateityp und Metadaten manuell übernehmen")
		return p, nil
	}
	switch {
	case strings.HasPrefix(e.Path, "etc/network/"):
		prepareNetwork(&p, snapshot, mapping)
	case guestConfigPath(e.Path):
		prepareGuest(&p, snapshot, mapping)
	case e.Path == "etc/hostname" || e.Path == "etc/hosts":
		prepareHostname(&p, snapshot, mapping)
	case e.Path == "etc/fstab":
		p.task("mount", e.Path, "Quelldatei nur als manuelle Referenz verwenden: Ziel-Root-, Boot- und Swap-Mounts erhalten; vor Übernahme jeden weiteren Mount samt UUID und vorhandenem Zielgerät prüfen. Keine Formatierung oder pauschale UUID-Ersetzung.")
	case e.Path == "etc/pve/storage.cfg":
		p.task("storage", e.Path, "Zielkonfiguration erhalten; Typ, Backend, Nodes, Content und Zugriff jeder Storage-ID manuell prüfen. Keine Anlage, Umbenennung oder Datenverschiebung.")
	}
	if mapping.Address != "" {
		p.task("legacy-address", e.Path, "Freie Legacy-Adresse erfordert eine ausgewählte Netzwerk-Stanza und explizite CIDR/Gateway/DNS-Entscheidung.")
	}
	if scenario == "cluster-node" && strings.HasPrefix(e.Path, "etc/pve/") {
		p.task("cluster", e.Path, "Aktuellen gemeinsamen Clusterbestand erhalten; keine Übernahme vor kontrolliertem Node-Beitritt.")
	}
	return p, nil
}
func (p *PreparedRecoveryFile) task(kind, field, instruction string) {
	id := Hash([]byte(p.Destination + "\x00" + kind + "\x00" + field + "\x00" + instruction))[:20]
	for _, t := range p.ManualTasks {
		if t.ID == id {
			return
		}
	}
	p.ManualTasks = append(p.ManualTasks, RecoveryTask{ID: id, Kind: kind, Title: field, Instruction: p.Destination + ": " + instruction, Requires: []string{}, State: "pending"})
}
func (p *PreparedRecoveryFile) effect(field, before, after, reason string) {
	if before != after {
		p.Effects = append(p.Effects, RecoveryEffect{Path: p.Destination, Field: field, Before: before, After: after, Reason: reason})
	}
}
func codeComment(line string) (string, string) {
	if n := strings.IndexByte(line, '#'); n >= 0 {
		return line[:n], line[n:]
	}
	return line, ""
}
func replaceTokenAt(line string, n int, value string) string {
	indices := interfaceToken.FindAllStringIndex(line, -1)
	if n >= len(indices) {
		return line
	}
	at := indices[n]
	return line[:at[0]] + value + line[at[1]:]
}
func guestConfigPath(p string) bool {
	return strings.HasPrefix(p, "etc/pve/") && strings.HasSuffix(p, ".conf") && (strings.Contains(p, "/qemu-server/") || strings.Contains(p, "/lxc/"))
}

var guestVolumeField = regexp.MustCompile(`^(?:(?:scsi|sata|ide|virtio|unused|mp)[0-9]+|rootfs|efidisk0|tpmstate0|hookscript)$`)
var guestOwnedVolume = regexp.MustCompile(`^(?:vm|subvol)-([0-9]+)-`)
var guestNetField = regexp.MustCompile(`^net[0-9]+$`)
var guestPCIField = regexp.MustCompile(`^(?:hostpci|usb|dev)[0-9]+$`)

func guestStorageReferences(data string) []string {
	refs := []string{}
	for _, line := range strings.Split(data, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || !guestVolumeField.MatchString(key) {
			continue
		}
		value = strings.TrimSpace(value)
		first := strings.SplitN(value, ",", 2)[0]
		first = strings.TrimPrefix(first, "file=")
		if id, _, ok := strings.Cut(first, ":"); ok && recoveryStorageID.MatchString(id) {
			refs = appendUnique(refs, id)
		}
	}
	sort.Strings(refs)
	return refs
}
func snapshotStorage(s RecoverySnapshot) []RecoveryStorage {
	for _, f := range s.Files {
		if f.Path == "etc/pve/storage.cfg" && f.State == "read" && Hash(f.Content) == f.SHA256 {
			return storageDefinitions(string(f.Content))
		}
	}
	return nil
}
func findStorage(defs []RecoveryStorage, id string) (RecoveryStorage, bool) {
	for _, d := range defs {
		if d.ID == id {
			return d, true
		}
	}
	return RecoveryStorage{}, false
}
func prepareGuest(p *PreparedRecoveryFile, s RecoverySnapshot, m Mapping) {
	guestID := strings.TrimSuffix(path.Base(p.Destination), ".conf")
	if strings.HasPrefix(p.Destination, "etc/pve/nodes/") {
		parts := strings.Split(p.Destination, "/")
		if len(parts) > 4 && parts[3] != s.Target.Hostname {
			p.task("guest-node", guestID, "Quellpfad gehört zu Node "+parts[3]+". Nach kontrolliertem Zielbeitritt Gast-ID prüfen und Zielpfad unter dem tatsächlichen Zielnode manuell festlegen; Originalpfad nicht ungeprüft installieren.")
		}
	}

	if !inventoryComplete(s.Target) {
		p.task("guest-inventory", guestID, "Zielinventar der Gäste ist unbekannt; Gast-ID und MAC-Kollisionen prüfen.")
	}
	for target, hash := range fileHashes(s.Target) {
		if guestConfigPath(target) && path.Base(target) == guestID+".conf" && hash != "missing" {
			p.task("guest-collision", guestID, "Gast-ID "+guestID+" ist im Zielbestand belegt oder nicht sicher lesbar ("+target+"); Übernahme blockiert.")
		}
	}
	sourceDefs, targetDefs := storageDefinitions(string(s.SourceStorageConfig)), snapshotStorage(s)
	for _, line := range strings.Split(string(p.Content), "\n") {
		code, _ := codeComment(line)
		if strings.HasPrefix(strings.TrimSpace(code), "[") {
			p.task("guest-syntax", guestID, "Gastkonfiguration enthält Snapshot-/Abschnittssyntax; aktive Konfiguration und Snapshotreferenzen getrennt manuell prüfen.")
			return
		}
	}

	lines := strings.Split(string(p.Content), "\n")
	for index, line := range lines {
		code, _ := codeComment(line)
		key, value, ok := strings.Cut(strings.TrimSpace(code), ":")
		if strings.TrimSpace(code) == "" {
			continue
		}
		if !ok {
			p.task("guest-syntax", fmt.Sprint(index+1), "Unbekannte Gastsyntax in Zeile "+fmt.Sprint(index+1)+" manuell prüfen.")
			continue
		}
		value = strings.TrimSpace(value)
		if guestNetField.MatchString(key) {
			bridge := ""
			for _, part := range strings.Split(value, ",") {
				if b, ok := strings.CutPrefix(part, "bridge="); ok {
					bridge = b
				}
			}
			found := false
			for _, nic := range s.Target.Interfaces {
				if nic.Name == bridge {
					found = true
				}
			}
			if bridge != "" && !found {
				p.task("bridge", key, "Bridge "+bridge+" fehlt oder ist unbekannt; vor Gaststart prüfen.")
			}
			p.task("guest-mac", key, "Gast-MAC und Netzwerkzuordnung gegen sämtliche Zielgäste auf Kollision prüfen.")
		}
		if guestPCIField.MatchString(key) {
			p.task("pci", key, "PCI-/USB-/Gerätezuordnung "+key+" am Ziel auf Verfügbarkeit und Kollision prüfen.")
		}
		if !guestVolumeField.MatchString(key) {
			if !guestNetField.MatchString(key) && !guestPCIField.MatchString(key) && !knownGuestScalar(key) {
				p.task("guest-syntax", key, "Nicht unterstütztes Gastfeld "+key+" manuell prüfen.")
			}
			continue
		}
		if key == "hookscript" {
			p.task("hook", key, "Hookscript-Inhalt, Berechtigungen und Nebenwirkungen vor Übernahme manuell prüfen.")
		}
		first := strings.SplitN(value, ",", 2)[0]
		prefix := ""
		if strings.HasPrefix(first, "file=") {
			prefix = "file="
			first = strings.TrimPrefix(first, prefix)
		}
		if first == "none" || first == "cdrom" {
			continue
		}
		id, volume, ok := strings.Cut(first, ":")
		if !ok || !recoveryStorageID.MatchString(id) || volume == "" {
			p.task("volume", key, "Geräte-/Bind-Mount- oder unbekannte Volumereferenz in "+key+" am Ziel manuell prüfen.")
			continue
		}
		if owner := guestOwnedVolume.FindStringSubmatch(volume); len(owner) > 1 && owner[1] != guestID {
			p.task("guest-volume", key, "Volume gehört zu Gast-ID "+owner[1]+" statt "+guestID+"; Kollision bzw. beabsichtigte Freigabe manuell prüfen.")
			continue
		}
		targetID := m.Storage[id]
		if targetID == "manual" {
			p.task("storage", key, "Storage "+id+" wurde als manuelle Entscheidung markiert.")
			continue
		}
		if targetID == "" {
			targetID = id
		}
		src, srcOK := findStorage(sourceDefs, id)
		dst, dstOK := findStorage(targetDefs, targetID)
		if !dstOK {
			p.task("storage", key, "Ziel-Storage-ID "+targetID+" ist nicht durch die Zielaufnahme belegt.")
			continue
		}
		if !srcOK || !matchingStorageBacking(src, dst) {
			p.task("backing", key, "Storage-Backing für "+id+" → "+targetID+" stimmt nicht nachweislich überein; Pool/Pfad/VG/Server/Export und separat zurückgespielte Daten prüfen.")
			continue
		}
		if dst.Kind == "pbs" {
			p.task("pbs", key, "PBS-Sicherung separat wiederherstellen und konkretes Gastvolume prüfen; PBS belegt kein startfähiges Volume.")
			continue
		}
		required := "images"
		if strings.Contains(p.Destination, "/lxc/") {
			required = "rootdir"
		}
		if strings.Contains(value, "media=cdrom") {
			required = "iso"
		}
		if key == "hookscript" {
			required = "snippets"
		}
		if !containsString(dst.Content, required) {
			p.task("storage-content", key, "Ziel-Storage belegt den benötigten Content "+required+" nicht.")
			continue
		}
		if len(dst.Nodes) > 0 && !containsString(dst.Nodes, s.Target.Hostname) {
			p.task("storage-nodes", key, "Zielhost ist für Storage "+targetID+" nicht freigegeben.")
			continue
		}
		ref := targetID + ":" + volume
		inventory := s.StorageVolumes[targetID]
		if inventory.State != "ok" || !containsString(inventory.Volumes, ref) {
			p.task("volume", key, "Volume "+ref+" fehlt oder ist nicht nachgewiesen; vorhandenen Bestand bzw. separate Wiederherstellung prüfen.")
			continue
		}
		if id != targetID {
			start := strings.Index(line, ":") + 1
			tail := line[start:]
			offset := strings.Index(tail, prefix+first)
			if offset >= 0 {
				offset += start + len(prefix)
				lines[index] = line[:offset] + ref + line[offset+len(first):]
				p.effect(key, first, ref, "Gleicher geprüfter Storage-Backing und vorhandene konkrete Volume-ID")
			}
		}
	}
	p.Content = []byte(strings.Join(lines, "\n"))
}
func containsString(values []string, search string) bool {
	for _, v := range values {
		if v == search {
			return true
		}
	}
	return false
}
func knownGuestScalar(key string) bool {
	return containsString(strings.Fields("name hostname description memory balloon cores sockets cpu numa ostype boot bootdisk bios machine agent onboot startup tags protection serial0 vga tablet keyboard localtime freeze lock template smbios1 vmgenid arch ostemplate swap unprivileged features cmode console tty searchdomain nameserver"), key)
}

func validateNetworkValue(v RecoveryNetworkMapping) error {
	var address netip.Prefix
	if v.AddressCIDR != "" {
		var err error
		address, err = netip.ParsePrefix(v.AddressCIDR)
		if err != nil || address.Addr().IsUnspecified() || address.Addr().IsMulticast() {
			return fmt.Errorf("ungültige Adresse/CIDR")
		}
	}
	if v.Gateway != "" {
		gateway, err := netip.ParseAddr(v.Gateway)
		if err != nil || gateway.Zone() != "" || gateway.IsUnspecified() || gateway.IsMulticast() {
			return fmt.Errorf("ungültiges Gateway")
		}
		if address.IsValid() && gateway.Is4() != address.Addr().Is4() {
			return fmt.Errorf("Gateway und Adresse benötigen dieselbe IP-Familie")
		}
	}
	for _, dns := range v.DNSServers {
		ip, err := netip.ParseAddr(dns)
		if err != nil || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("ungültiger DNS-Server")
		}
	}
	return nil
}
func prepareNetwork(p *PreparedRecoveryFile, s RecoverySnapshot, m Mapping) {
	original := string(p.Content)
	refs, issues := networkReferences(map[string]string{p.Destination: original}, s.Source)
	inspection := RecoveryInspection{TargetPorts: physicalPorts(s.Target)}
	for _, ref := range refs {
		inspection.Ports = append(inspection.Ports, RecoveryPort{Name: ref})
	}
	_, blockers, _ := validateRecoveryMapping(inspection, PlanRequest{Mapping: Mapping{Interfaces: m.Interfaces}})
	for _, issue := range append(issues, blockers...) {
		p.task("network", p.Destination, issue)
	}
	mapped := original
	if len(blockers) == 0 {
		mapped, _ = mapNetwork(original, s.Source, s.Target, m)
	}
	beforeLines, lines := strings.Split(original, "\n"), strings.Split(mapped, "\n")
	for i := range lines {
		if lines[i] != beforeLines[i] {
			p.effect(fmt.Sprintf("interface:%d", i+1), beforeLines[i], lines[i], "Explizite Zuordnung zu vorhandenem physischem Zielport")
		}
	}
	// Keys select the SOURCE stanza name. Optional /inet or /inet6 disambiguates dual stack.
	// Count source declarations before changing any value, including duplicate same-family stanzas.
	declarations := map[string]int{}
	for _, line := range beforeLines {
		c, _ := codeComment(line)
		f := strings.Fields(c)
		if len(f) == 4 && f[0] == "iface" {
			declarations[f[1]+"/"+f[2]]++
			declarations[f[1]]++
		}
	}
	used := map[string]bool{}
	for start := 0; start < len(lines); start++ {
		code, _ := codeComment(beforeLines[start])
		fields := strings.Fields(code)
		if len(fields) != 4 || fields[0] != "iface" {
			continue
		}
		name, family := fields[1], fields[2]
		key := name + "/" + family
		v, selected := m.Network[key]
		if !selected {
			key = name
			v, selected = m.Network[key]
		}
		if !selected {
			continue
		}
		end := start + 1
		for end < len(lines) {
			next, _ := codeComment(beforeLines[end])
			f := strings.Fields(next)
			if len(f) > 0 && (f[0] == "iface" || f[0] == "auto" || strings.HasPrefix(f[0], "allow-") || f[0] == "source" || f[0] == "source-directory") {
				break
			}
			end++
		}
		if err := validateNetworkValue(v); err != nil {
			p.task("network-value", key, err.Error())
			used[key] = true
			continue
		}
		ip4 := family == "inet"
		if family != "inet" && family != "inet6" {
			p.task("network-syntax", key, "Unbekannte IP-Familie manuell prüfen.")
			continue
		}
		if v.AddressCIDR != "" {
			addr, _ := netip.ParsePrefix(v.AddressCIDR)
			if addr.Addr().Is4() != ip4 {
				continue
			}
		}
		if v.Gateway != "" {
			addr, _ := netip.ParseAddr(v.Gateway)
			if addr.Is4() != ip4 {
				continue
			}
		}
		if key == name && v.AddressCIDR == "" && v.Gateway == "" && declarations[name] > 1 {
			p.task("network-ambiguous", key, "Mehrere IP-Familien verwenden denselben Stanzanamen; explizit /inet oder /inet6 auswählen.")
			used[key] = true
			continue
		}
		if declarations[name+"/"+family] > 1 {
			p.task("network-ambiguous", key, "Mehrfach deklarierte ausgewählte Stanza; Adresse/Gateway/DNS manuell zuordnen.")
			used[key] = true
			continue
		}
		if used[key] {
			p.task("network-ambiguous", key, "Mehrfach deklarierte ausgewählte Stanza; Adresse/Gateway/DNS manuell zuordnen.")
			continue
		}
		used[key] = true
		if fields[3] != "static" {
			p.task("network-method", key, "Nur vorhandene statische Stanzas vorbereiten; Methode manuell ändern.")
			continue
		}
		values := map[string]string{"address": v.AddressCIDR, "gateway": v.Gateway, "dns-nameservers": strings.Join(v.DNSServers, " ")}
		counts := map[string]int{}
		for i := start + 1; i < end; i++ {
			c, _ := codeComment(lines[i])
			f := strings.Fields(c)
			if len(f) > 0 {
				counts[f[0]]++
			}
		}
		for _, field := range []string{"address", "gateway", "dns-nameservers"} {
			value := values[field]
			if value == "" {
				continue
			}
			if counts[field] != 1 {
				p.task("network-field", key+"."+field, "Fehlendes oder mehrfaches Feld "+field+" in "+key+" manuell vorbereiten.")
				continue
			}
			if field == "address" && counts["netmask"] > 0 {
				p.task("network-netmask", key, "Separates netmask-Feld mit CIDR manuell konsolidieren.")
				continue
			}
			for i := start + 1; i < end; i++ {
				c, comment := codeComment(lines[i])
				f := strings.Fields(c)
				if len(f) < 2 || f[0] != field {
					continue
				}
				tokens := interfaceToken.FindAllStringIndex(c, -1)
				before := strings.Join(f[1:], " ")
				lines[i] = c[:tokens[1][0]] + value + c[tokens[len(tokens)-1][1]:] + comment
				p.effect(key+"."+field, before, value, "Explizit ausgewählte Netzwerk-Stanza")
			}
		}
	}
	keys := []string{}
	for key := range m.Network {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !used[key] {
			p.task("network-stanza", key, "Ausgewählte Stanza "+key+" fehlt oder passt nicht zur IP-Familie.")
		}
	}
	p.Content = []byte(strings.Join(lines, "\n"))
	p.task("network-activation", p.Destination, "Vorbereitete Netzwerkdatei über Konsole prüfen und manuell aktivieren; aktive SSH-Verbindung erhalten.")
}

var hostnameLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func validRecoveryHostname(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if !hostnameLabel.MatchString(label) {
			return false
		}
	}
	return true
}
func prepareHostname(p *PreparedRecoveryFile, s RecoverySnapshot, m Mapping) {
	if m.Hostname == "" {
		return
	}
	if !validRecoveryHostname(m.Hostname) || !validRecoveryHostname(s.Source.Hostname) {
		p.task("hostname", p.Destination, "Quellhostname fehlt oder gewünschter Hostname ist ungültig; Identität manuell prüfen.")
		return
	}
	old, newName := s.Source.Hostname, m.Hostname
	if p.Destination == "etc/hostname" {
		if strings.TrimSpace(string(p.Content)) != old {
			p.task("hostname", p.Destination, "Gesicherter Hostname weicht vom Quellinventar ab; manuell prüfen.")
			return
		}
		raw := string(p.Content)
		p.Content = []byte(strings.Replace(raw, old, newName, 1))
		p.effect("hostname", old, newName, "Verifizierter Quellhostname und expliziter Zielname")
	} else {
		lines := strings.Split(string(p.Content), "\n")
		selected := -1
		for i, line := range lines {
			code, _ := codeComment(line)
			fields := strings.Fields(code)
			if len(fields) < 2 {
				continue
			}
			if _, err := netip.ParseAddr(fields[0]); err != nil {
				continue
			}
			for _, token := range fields[1:] {
				if token == old {
					if selected >= 0 && selected != i {
						p.task("hostname", p.Destination, "Quellhostname steht in mehreren hosts-Zeilen; Zeile manuell auswählen.")
						return
					}
					selected = i
					break
				}
			}
		}
		if selected < 0 {
			p.task("hostname", p.Destination, "Keine eindeutige hosts-Zeile mit exaktem Quellhostname; Zeile manuell auswählen.")
			return
		}
		code, comment := codeComment(lines[selected])
		fields := strings.Fields(code)
		for i := len(fields) - 1; i >= 1; i-- {
			before := fields[i]
			after := before
			if before == old {
				after = newName
			} else if !strings.Contains(old, ".") && strings.HasPrefix(before, old+".") {
				if strings.Contains(newName, ".") {
					after = newName
				} else {
					after = newName + strings.TrimPrefix(before, old)
				}
			}
			if after != before {
				code = replaceTokenAt(code, i, after)
				p.effect("hosts.hostname", before, after, "Hostnametoken der eindeutig ausgewählten hosts-Zeile")
			}
		}
		lines[selected] = code + comment
		p.Content = []byte(strings.Join(lines, "\n"))
	}
	p.task("hostname-identity", p.Destination, "DNS und lokale Hostidentität manuell prüfen; Clusteridentität separat behandeln.")
}
