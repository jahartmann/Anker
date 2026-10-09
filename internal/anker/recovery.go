package anker

import (
	"fmt"
	"strings"
)

func recoveryGuide(m Manifest) string {
	return fmt.Sprintf(`# Anker Wiederherstellung

Host: %s · Sicherung: %s · Proxmox: %s
Status: %s. Erfassung: %s bis %s.

## Dateien und Prüfung

Dieses Paket ist ohne Anker lesbar. Prüfen Sie vor Verwendung im Exportwurzelverzeichnis mit sha256sum -c checksums.sha256. manifest.json enthält Originalrechte, numerische UID/GID, Symlinkziele, verfügbare xattrs und Warnungen. files/ ist eine geschützte Ansicht; Symlinks sind dort als Linkzieltext abgelegt. Das tar-Exportpaket enthält zusätzlich original-files/ mit ursprünglichen Modi und echten Links. Nur ausgewählte Inhalte auf ein geprüftes Ziel kopieren; nicht das gesamte Archiv direkt nach / entpacken.

## Wiederaufbau

1. Passende Proxmox-Version frisch installieren; Root-Dateisystem, Bootloader und Hardwaretreiber des Ziels erhalten.
2. inventory/host.json lesen. Netzwerkkarten nach Funktion zuordnen. Storage-Pools, Mounts, UUIDs und Gerätekennungen am Ziel prüfen. VM-Daten aus vorhandenen Storages beziehungsweise PBS separat bereitstellen.
3. Hostname und IP bewusst festlegen. Der bisherige Host darf nicht gleichzeitig dieselbe Identität verwenden. SSH-Hostschlüssel und maschinenspezifische Zertifikate passend neu erzeugen oder gezielt übernehmen.
4. Configs gezielt vorbereiten. Passwd/shadow, fstab, Boot-Konfiguration, Paketquellen und PCI-Adressen nicht pauschal übernehmen. UID/GID-Zuordnung prüfen. Metadaten aus manifest.json nur nach Prüfung zurücksetzen.
5. Vor Änderungen die Zielkonfiguration sichern. Netzwerkänderungen ausschließlich mit Konsolenzugang vornehmen. Dienste einzeln prüfen und erst nach einem erfolgreichen Neustart den Wiederaufbau abschließen.

## Cluster

Ein Ersatznode in einem gesunden Cluster übernimmt den aktuellen gemeinsamen Zustand des Clusters. Alte config.db nicht in diesen Cluster einsetzen. Entfernung eines ausgefallenen Nodes und Wiederbeitritt benötigen den zur Version passenden Proxmox-Ablauf. Clusterbeitritt überschreibt /etc/pve.

Bei vollständigem Clusterverlust in isolierter Umgebung beginnen, einen geeigneten gemeinsamen Stand wählen, aktive alte Nodes ausschließen und Quorum/HA/Storage prüfen. Datenbank-Recovery nur bei gestopptem pve-cluster und anhand der passenden offiziellen Anleitung. config.db ist kein Datenbackup für Gäste oder Ceph.

## Versionswechsel

Quell- und Zielversion samt Debian-Basis vergleichen. Bei anderer Hauptversion Konfiguration anhand geprüfter Regeln anpassen; keine pauschale Datenbankübernahme. Paket-Upgrades und Repository-Änderungen sind ein gesonderter Ablauf. Ohne getestete Kombination ist die Wiederherstellung manuell zu prüfen.

## Automatische Einzeldateien und Unterbrechung

Freigegebene gewöhnliche Dateien benötigen Restore-Protokoll 2 und erfasste Benutzer-/Gruppenidentitäten. Netzwerk, SSH-/sudo-/PAM-Zugang, Hosthelfer, Cluster und hardwareabhängige Configs bleiben manuell. Vor einer Übernahme andere Schreiber und betroffene Dienste anhalten. Anker sperrt nur eigene Helferprozesse, keine fremden Programme.

Das Zieljournal liegt unter /var/lib/anker-host/rollback/<Plan-ID>/journal.json. Nach Abbruch Hostzustand über Anker abgleichen; eine ungeklärte Übernahme nicht wiederholen. Rücksetzung separat mit Plan-ID bestätigen. Nur eigene unveränderte Nachherzustände werden zurückgesetzt, Fremdänderungen bleiben erhalten. Originale und Journal bis zur Betriebsabnahme behalten. Dateiübernahme und Rücksetzung bestätigen keinen Reboot oder vollständigen Hostbetrieb.

## Einschränkungen

Warnungen: %v
Konsistenz: %s
Ein Live-Dateibaum außerhalb der SQLite-Sicherung ist kein global atomarer Hostzustand. Für Migrationen den finalen Stand im Änderungsstopp sichern.
`, m.Inventory.Hostname, m.ID, m.Inventory.PVEVersion, m.Status, m.CreatedAt, m.CompletedAt, m.Warnings, m.Consistency)
}

func evidenceLabel(s string) string {
	if s == "" {
		return "unbekannt"
	}
	return s
}

// The dossier is rebuilt on every export from the current private plan and
// original manifest. It never promotes human confirmations to machine evidence.
func planScenarioDossier(p Plan, m Manifest) string {
	var b strings.Builder
	b.WriteString("\n## Reihenfolge für dieses Szenario\n\n")
	switch p.Scenario {
	case "cluster-node":
		b.WriteString("1. Gesunden, isolierten Testcluster und ausgeschlossenen alten Node nachweisen.\n2. Frisches Ziel und aktuelle gemeinsame Clusterkonfiguration prüfen. Keine alte config.db einsetzen.\n3. HALT vor Entfernung/Beitritt, Quorum, Ceph und HA: eigener versionsgerechter manueller Ablauf erforderlich.\n4. Erst nach Freigabe ausgewählte lokale Dateien übernehmen; Gäste und Dienste separat prüfen.\n")
	case "cluster-disaster":
		b.WriteString("1. Vollständige Isolation und Abwesenheit aktiver alter Nodes belegen.\n2. Geeigneten gemeinsamen Sicherungsstand sowie Gast-/PBS-/Storage-Abhängigkeiten bestimmen.\n3. HALT vor config.db-Rekonstruktion, Join, Quorum, Ceph oder HA. Nur operatorgeführte Rekonstruktion anhand passender PVE-Anleitung.\n4. Gemeinsamen Zustand und danach jeden Node, Gäste und Dienste einzeln prüfen.\n")
	case "host", "migration":
		b.WriteString("1. Alten Host isolieren; neues PVE mit eigener Boot-/Rootinstallation bereitstellen.\n2. Abweichende NIC-Namen nach physischer Funktion, Disk-/Storage-Backing und externe Gast-/PBS-Abhängigkeiten prüfen.\n3. HALT bei unbekannter Identität, Backing oder fehlenden Gästen; keine Datenträger oder Pools automatisch erzeugen.\n4. Ziel-vorher sichern; einzelne vorbereitete Dateien und manuelle Schritte getrennt freigeben.\n")
	default:
		b.WriteString("1. Quelle, Zielidentität und ausgewählte Einzeldateien erneut vergleichen.\n2. Ziel-vorher und Rückweg prüfen; betroffene Dienste und fremde Schreiber kontrollieren.\n3. HALT vor Übernahme: separate Plan-ID-Bestätigung erforderlich.\n4. Journal abgleichen; Rücksetzung nur nach eigener Bestätigung und ohne Fremddrift.\n")
	}
	b.WriteString("\nFür alle Szenarien: Bei Unterbrechung keinen blinden zweiten Apply starten. Bei Platzmangel stoppen, Quelle unverändert erhalten. Drift erfordert neue Prüfung und Entscheidungen. Neustart ausschließlich durch Operator; anschließend frischen planbezogenen Maschinennachweis einschließlich Maschinenidentität und Boot-Baseline verlangen. Diese Prüffähigkeit ist bis Task 8/Integration Task 12 unvollständig; fehlende Nachweise bleiben offen. Reale PVE-Laborabnahme wurde nicht ausgeführt.\n")
	b.WriteString(fmt.Sprintf("\n## Abhängigkeiten und Herkunft\n\nQuelle: Sicherung %s, Host %s, Erfassung %s bis %s. Ziel: %s, Aufnahme %s, Erfassung %s. Inventar-Fingerprints: Quelle %s · Ziel %s.\n", p.BackupID, m.HostID, evidenceLabel(m.CreatedAt), evidenceLabel(m.CompletedAt), p.TargetID, evidenceLabel(p.TargetSnapshotID), evidenceLabel(p.Target.CapturedAt), evidenceLabel(p.Source.Fingerprint), evidenceLabel(p.Target.Fingerprint)))
	if len(m.Dependencies) == 0 {
		b.WriteString("\nKeine strukturierten Quellabhängigkeiten erfasst: unbekannt, nicht vollständig belegt.\n")
	}
	for _, d := range m.Dependencies {
		fmt.Fprintf(&b, "- Referenz %s · Rolle: Konfigurationsabhängigkeit · Ursprung %s · Status %s · %s\n", d.Path, evidenceLabel(d.Source), evidenceLabel(d.State), d.Reason)
	}
	b.WriteString("\nExterne Referenzen vor Nutzung im separaten Laborprotokoll ergänzen: Gast-/PBS-Sicherung, Verschlüsselungsschlüssel-Verwahrort, Storage-/Disk-Backing, Netz-/Konsolenzugang, PVE-Installationsmedium und Build/Helfer. Je Referenz Rolle, Ort/ID (ohne Zugangsdaten), verantwortliche Person, Prüftermin und Status angeben. Unbekannte Orte, fehlende Artefakte oder unklare Rechte lassen das Szenario unvollständig. Keine Passwörter, Schlüssel oder privaten Config-Inhalte in das Protokoll kopieren.\n")
	return b.String()
}
