package anker

import (
	"fmt"
	"strings"
)

func recoveryGuide(m Manifest) string {
	if m.Inventory.Platform() == "pbs" {
		return fmt.Sprintf(`# Anker PBS-Konfigurationsbackup

Host: %s · Sicherung: %s · PBS: %s
Status: %s. Erfassung: %s bis %s.

Dieses Paket enthält Konfigurationen und Systeminventar. Datastore-Inhalte sind ausgeschlossen und müssen separat gesichert und bereitgestellt werden. Clientseitige Verschlüsselungsschlüssel sind separat aufzubewahren.

Vor Verwendung im Exportwurzelverzeichnis sha256sum -c checksums.sha256 ausführen. manifest.json dokumentiert Originalrechte, UID/GID, Links und xattrs. files/ enthält Links als Zieltext; original-files/ im tar-Export enthält echte Links und Originalmodi. Das Archiv nicht pauschal nach / entpacken.

Bei aus dem Inventar ergänzten Links kennzeichnet capture_origin=inventory_symlink die Herkunft. mtime_unknown im Manifest und ANKER.mtime_unknown im tar-PAX-Eintrag bedeuten: Die ursprüngliche Änderungszeit ist unbekannt; der Zeitwert 0 ist ein Platzhalter und darf nicht als Originalzeit zurückgesetzt werden.

Geführte PBS-Konfigurationswiederherstellung erfolgt als Dateiauswahl mit Zielaufnahme, Vorschau, Plan-ID-Bestätigung und Rücksetzjournal. Beide PBS-Dienste vor Zielprüfung, Übernahme und Rücksetzung stoppen. Datastore-Anbindungen werden aus dem Ziel erhalten und zurückgespielte Jobs in der aufbereiteten Fassung deaktiviert. Anker startet keine Dienste und keine Jobs. Alle bestehenden Jobregeln vor späterem manuellem Dienststart separat prüfen. Eine passende PBS-Version frisch installieren, Datastore-Mounts und vorhandene Daten unabhängig prüfen und nur gezielt Konfigurationen aus /etc/proxmox-backup und dem Systemprofil übernehmen. Zugangsdaten, Zertifikate und Dateieigentümer gesondert prüfen. Vor Änderungen Zielkonfiguration sichern und laufende Jobs anhalten.

Die Live-Erfassung ist kein global atomarer Zustand. Für einen finalen Stand Konfigurationsänderungen pausieren.

Warnungen: %v
Konsistenz: %s
`, m.Inventory.Hostname, m.ID, m.Inventory.PBSVersion, m.Status, m.CreatedAt, m.CompletedAt, m.Warnings, m.Consistency)
	}
	return fmt.Sprintf(`# Anker Wiederherstellung

Host: %s · Sicherung: %s · Proxmox: %s
Status: %s. Erfassung: %s bis %s.

## Dateien und Prüfung

Dieses Paket ist ohne Anker lesbar. Prüfen Sie vor Verwendung im Exportwurzelverzeichnis mit sha256sum -c checksums.sha256. manifest.json enthält Originalrechte, numerische UID/GID, Symlinkziele, verfügbare xattrs und Warnungen. files/ ist eine geschützte Ansicht; Symlinks sind dort als Linkzieltext abgelegt. Das tar-Exportpaket enthält zusätzlich original-files/ mit ursprünglichen Modi und echten Links. Nur ausgewählte Inhalte auf ein geprüftes Ziel kopieren; nicht das gesamte Archiv direkt nach / entpacken.

Bei aus dem Inventar ergänzten Links kennzeichnet capture_origin=inventory_symlink die Herkunft. mtime_unknown im Manifest und ANKER.mtime_unknown im tar-PAX-Eintrag bedeuten: Die ursprüngliche Änderungszeit ist unbekannt; der Zeitwert 0 ist ein Platzhalter und darf nicht als Originalzeit zurückgesetzt werden.

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
	if p.Source.Platform() == "pbs" {
		return fmt.Sprintf("\n## PBS-Konfiguration: Reihenfolge und Grenzen\n\n1. PBS-Ziel mit passender Hauptversion und aktuellem Hosthelfer anbinden. Bestehende Datastores unabhängig einbinden; keine Dateninitialisierung.\n2. `systemctl stop proxmox-backup-proxy proxmox-backup` am Ziel ausführen. Dienste, Datastore-Verzeichnisse, Chunk-Verzeichnisidentitäten und Mounts frisch prüfen.\n3. Ausgewählte Dateien vergleichen. Für Datastore- und Jobdefinitionen aufbereitete Fassungen mit erhaltenem Ziel-Backing und deaktivierten automatischen Auslösern verwenden.\n4. Übernahme separat mit Plan-ID bestätigen; Zieljournal und Dateiergebnis abgleichen. Anker startet keine Dienste oder Jobs und berührt keine Datastore-Inhalte.\n5. Vor späterem manuellem Dienststart alle bestehenden Jobregeln, Zugänge und Zertifikate separat prüfen; zurückgespielte Jobs bleiben deaktiviert. Für Rücksetzung Dienste erneut stoppen. Bei Fremddrift oder geänderter Datastore-Identität manuell prüfen.\n\nQuelle: Sicherung %s, Manifestbezug im Planpaket; Ziel %s, Aufnahme %s. Quelle und Datastore-Daten bleiben getrennte Sicherungen. Dieser Konfigurationsnachweis bestätigt weder die Datenintegrität eines Datastores noch dessen Betriebsbereitschaft. Reale PBS-Laborabnahme steht aus.\n", p.BackupID, p.TargetID, p.TargetSnapshotID)
	}
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
	b.WriteString("\nFür alle Szenarien: Bei Unterbrechung keinen blinden zweiten Apply starten. Bei Platzmangel stoppen, Quelle unverändert erhalten. Drift erfordert neue Prüfung und Entscheidungen. Neustart ausschließlich durch Operator; anschließend frischen planbezogenen Maschinennachweis einschließlich Maschinenidentität und Boot-Baseline verlangen. Mit Anker: `anker restore verify PLAN` (PLAN durch die Plan-ID ersetzen) führt ausschließlich feste lesende Abfragen aus. Erforderlich sind unveränderte Quelle, privater Zielnachweis und gespeicherte Zielverbindung. Maschinenidentität muss zur privaten Vorher-Abfrage passen; eine Boot-ID-Änderung zählt nur mit gültiger Baseline. Fehlende Baseline oder unbekannte Identität bleiben offen; Gast- und Gesamtrecovery werden dadurch nicht bewiesen. Ohne Anker die Nachweise im Paket lesen und die Zielkonsole manuell prüfen. Reale PVE-Laborabnahme wurde nicht ausgeführt.\n")
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
