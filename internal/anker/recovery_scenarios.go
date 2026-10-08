package anker

import (
	"errors"
	"sort"
	"strings"
)

// recoverySituation normalizes the presentation without changing saved scenario IDs.
func recoverySituation(scenario string) (RecoverySituation, error) {
	switch scenario {
	case "files", "host", "cluster-node", "cluster-disaster":
		return RecoverySituation(scenario), nil
	case "standalone", "migration", "version", "topology":
		return "host", nil
	default:
		return "", errors.New("unbekanntes Wiederherstellungsszenario")
	}
}

// scenarioSelection defines references to prepare, not permission to write them.
// Protected explicit files and local node references still require manual handling.
func scenarioSelection(m Manifest, r PlanRequest) (RecoverySelection, error) {
	selection := RecoverySelection{Paths: []string{}, Preserved: []RecoveryPreservation{}}
	situation, err := recoverySituation(r.Scenario)
	if err != nil {
		return selection, err
	}
	available := map[string]bool{}
	for _, e := range m.Entries {
		if e.Type != "directory" {
			available[e.Path] = true
		}
	}
	requested := map[string]bool{}
	for _, p := range r.Files {
		if err := ValidPath(p); err != nil {
			return selection, err
		}
		if !available[p] {
			return selection, errors.New("Dateiauswahl enthält nicht gesicherte Dateien")
		}
		requested[p] = true
	}
	if situation == "files" && len(requested) == 0 {
		return selection, errors.New("mindestens eine Datei auswählen")
	}
	for p := range available {
		reason := ""
		if situation != "files" {
			reason = scenarioPreservation(p, m.Inventory.Hostname, r.Scenario, situation)
		}
		if reason == "" && len(requested) > 0 && !requested[p] {
			reason = "Nicht ausgewählte Datei; Zielstand erhalten"
		}
		if reason != "" {
			selection.Preserved = append(selection.Preserved, RecoveryPreservation{Path: p, Reason: reason})
		} else {
			selection.Paths = append(selection.Paths, p)
		}
	}
	sort.Strings(selection.Paths)
	sort.Slice(selection.Preserved, func(i, j int) bool { return selection.Preserved[i].Path < selection.Preserved[j].Path })
	return selection, nil
}

func scenarioPreservation(p, sourceNode, scenario string, situation RecoverySituation) string {
	switch p {
	case "etc/passwd", "etc/shadow", "etc/group", "etc/gshadow", "etc/subuid", "etc/subgid":
		return "Benutzer- und Gruppenidentität der Zielinstallation erhalten"
	case "etc/fstab", "etc/crypttab", "etc/machine-id", "var/lib/dbus/machine-id":
		return "Root-/Boot-/Swap-Zuordnung und Maschinenidentität der Zielinstallation erhalten"
	case "etc/sudoers", "etc/anker-host.json", "etc/nsswitch.conf":
		return "Funktionierenden SSH- und Wiederherstellungszugang der Zielinstallation erhalten"
	case "etc/modules":
		return "Bootloader, Zieltreiber und gerätespezifische Zielkonfiguration erhalten"
	}
	for _, prefix := range []string{"boot/", "etc/default/grub", "etc/grub.d/", "etc/kernel/", "etc/initramfs-tools/", "etc/modprobe.d/", "etc/modules-load.d/", "etc/udev/", "etc/zfs/"} {
		if strings.HasPrefix(p, prefix) {
			return "Bootloader, Zieltreiber und gerätespezifische Zielkonfiguration erhalten"
		}
	}
	for _, prefix := range []string{"etc/ssh/", "etc/sudoers.d/", "etc/pam.d/", "usr/local/lib/anker/"} {
		if strings.HasPrefix(p, prefix) {
			return "Funktionierenden SSH- und Wiederherstellungszugang der Zielinstallation erhalten"
		}
	}
	if strings.Contains(p, "/.ssh/") {
		return "SSH-Schlüssel und Zugangsidentität der Zielinstallation erhalten"
	}
	if (scenario == "version" || scenario == "topology") && (strings.HasPrefix(p, "etc/apt/") || p == "etc/os-release" || p == "etc/debian_version") {
		return "Paketquellen und versionsabhängige Zielkonfiguration erhalten; unterstützten Migrationspfad separat prüfen"
	}
	if situation == "cluster-node" {
		if strings.HasPrefix(p, "etc/pve/nodes/") {
			parts := strings.Split(p, "/")
			if sourceNode != "" && len(parts) > 4 && parts[3] == sourceNode {
				return ""
			}
			return "Fremde oder nicht eindeutig zugeordnete Nodekonfiguration des lebenden Clusters erhalten"
		}
		// These pmxcfs aliases refer to the source node and remain manual references.
		for _, prefix := range []string{"etc/pve/local/", "etc/pve/qemu-server/", "etc/pve/lxc/"} {
			if strings.HasPrefix(p, prefix) {
				return ""
			}
		}
		for _, prefix := range []string{"etc/pve/", "etc/corosync/", "etc/ceph/", "var/lib/pve-cluster/"} {
			if strings.HasPrefix(p, prefix) {
				return "Gemeinsame Cluster-, HA-, Corosync- und Storagekonfiguration des lebenden Clusters erhalten"
			}
		}
	}
	return ""
}
