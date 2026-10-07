package anker

import "fmt"

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
