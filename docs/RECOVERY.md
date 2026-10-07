# Wiederherstellung

## Einzeldateien

1. Sicherung, Zielhost und Dateien wählen. **Ziel prüfen** liest das aktuelle Ziel, ohne einen Plan anzulegen oder Dateien zu ändern.
2. Nur benötigte physische Ports und referenzierte Storage-IDs zuordnen. Bridges, Bonds, VLANs und Interface-Aliase werden über ihre Portabhängigkeiten betrachtet; unbeteiligte Adapter und Partitionen erscheinen nicht als Entscheidungen. Vorschläge anhand von MAC/PCI prüfen. Verkabelung wird nicht automatisch erkannt.
3. **Plan prüfen** wiederholt die Zielprüfung. Netzwerkdateien werden angepasst und für die manuelle Übernahme vorbereitet. Storage, gewünschter Hostname und Adresse dokumentieren manuelle Entscheidungen; Anker baut keine Datenträger um und ersetzt diese Werte nicht pauschal in Configs.
4. Freigegebene gewöhnliche Dateien nach Eingabe der exakten Plan-ID übernehmen. Der Plan prüft Inhalt, Rechte und Benutzer-/Gruppenidentität. Ein geänderter Zielstand verlangt einen neuen Plan.

Accounts, Identität, Boot, Cluster, Paketquellen, Netzwerk, Zugangs- und Anker-Helferkonfiguration, Links sowie Dateien mit erweiterten Metadaten bleiben manuell. **Netzwerk wird nicht automatisch geschrieben oder aktiviert.** Ein unabhängiger lokaler Rückfallwächter ist noch nicht abgenommen; die Konsolenbestätigung hebt diese Grenze nicht auf.

Automatische Übernahmen brauchen Hosthelfer mit Restore-Protokoll 2. Nach einem zentralen Update bei vorhandenen Hosts einmal **Hosts → Verbindung neu einrichten** und danach einen neuen Backupstand erstellen. Alte Stände bleiben exportierbar; fehlende Eigentümeridentitäten werden nicht geraten. Das SSH-Nachrichtenformat bleibt Version 1.

## Unterbrechung und Rücksetzung

Der Host sichert Originale und schreibt ein synchronisiertes Journal unter `/var/lib/anker-host/rollback/<Plan-ID>/journal.json`. Eine lokale Exklusivsperre verhindert parallele Helfermutationen. Jede Datei wird unmittelbar vor Austausch und danach nach Inhalt und Metadaten geprüft. Betroffene Dienste und andere Schreiber für die Übernahme anhalten: Ankers Sperre kann fremde Programme nicht sperren, und mehrere Dateien bilden keine globale Transaktion.

Bei einem gewöhnlichen Schreibfehler setzt der Helfer eigene bereits geänderte Dateien kontrolliert zurück. Zwischenzeitliche Fremdänderungen bleiben erhalten und erscheinen als Rücksetzungskonflikt. Bei Verbindungsabbruch oder Prozessende keine zweite Übernahme starten. **Hostzustand abgleichen** liest nur das Journal. Ein vollständig fehlendes Journal wird als ungeklärt erklärt; ein vorhandenes beschädigtes Journal blockiert weitere Schreiboperationen.

**Dateien zurücksetzen** verlangt die Plan-ID erneut. Die Rücksetzung prüft für jede Datei den eigenen Nachherzustand oder den bereits wiederhergestellten Vorherzustand. Fremde Inhalte oder Rechte werden nicht überschrieben. Originalkopien und Protokoll erst nach bestätigter Betriebsprüfung entfernen. Alte Rollbackordner ohne Journal müssen vor einer neuen automatischen Übernahme manuell geprüft werden; nicht ungeprüft löschen.

„Dateien übernommen“ bedeutet: Dateischritte bestätigt. Syntax, Dienstfunktion, Netzwerk, Storage, PBS und Neustart bleiben zu prüfen. Eine erfolgreiche Rücksetzung bestätigt ebenfalls keinen vollständigen Hostbetrieb.

## Hostausfall oder neue Hardware

1. Alten Host ausschalten oder isolieren, Konsolenzugang sicherstellen.
2. Passende Proxmox-Zielversion frisch installieren. Bootloader, Rootdateisystem und Hardwaretreiber der Zielinstallation behalten.
3. Ziel über den festen SSH-Helfer inventarisieren. Sicherung und geeignetes Szenario auswählen.
4. Ports nach MAC/PCI und tatsächlicher Verkabelung zuordnen. Bridges, Bonds, VLANs, MTU, Management-IP und Routing prüfen. Anker ersetzt zugeordnete Interface-Tokens im vorbereiteten Netzwerkteil, entscheidet jedoch keine physische Verkabelung.
5. Storage-IDs, Disk-UUIDs, ZFS-Pools, LVM, Ceph und Mounts gezielt prüfen. Originale stehen im Export zur Verfügung; Anker legt keine Pools oder Gastdatenträger an. Storage-/Diskumbauten werden manuell dokumentiert.
6. Hardwareabhängige Configs und Identität anhand der Planhinweise übernehmen. Netzwerk erst mit Konsole und einem Rückweg aktivieren.
7. PBS-Anbindung herstellen, vorhandene VM-/Containerbackups separat zurückspielen, Funktionen und Neustart prüfen.

Gesamtszenarien bleiben auf realen Hosts manuell geführt. Auch eine erfolgreiche Dateiübernahme ist keine bestätigte vollständige Wiederherstellung.

## Cluster und Versionswechsel

Ein Node in einem gesunden Cluster wird nach dem passenden offiziellen Verfahren ersetzt und tritt dem bestehenden Cluster bei. Niemals eine alte `config.db` über den laufenden gesunden Cluster kopieren. Bei Gesamtausfall zuerst ein isoliertes Recovery-Netz und einen geeigneten gemeinsamen Stand auswählen. Quorum, Corosync, HA, Ceph, Zertifikate und Nodeidentitäten erfordern einen geprüften Ablauf; Anker erzwingt kein Quorum und startet keinen Cluster automatisch.

Versionsmigration verlangt einen unterstützten Installations-/Upgradepfad. Ein älteres `/etc` darf nicht komplett über eine neue Major-Version gelegt werden. Anker sperrt automatische Übernahme über Major-Grenzen und unbekannte Versionen. Auch gleiche Major-Versionen benötigen eine reale Laborabnahme.

## Ohne laufenden Anker-Dienst

Sicherungsordner per SFTP kopieren oder TAR exportieren. In einem isolierten Arbeitsordner Prüfsummen prüfen:

```sh
cd /pfad/zur/sicherung
sha256sum -c checksums.sha256
```

`WIEDERHERSTELLUNG.md`, `inventory/host.json` und `manifest.json` lesen. Der rohe Baum `files/` enthält Links als Text; `manifest.json` hält Typ und Linkziel. Vollständige TAR-Exporte enthalten `original-files/` mit echten Links und Originalmodi; zuerst vollständig im isolierten Ordner prüfen. Extended Attributes stehen base64-kodiert im Manifest und zusätzlich im PAX-TAR-Export. Für vertrauliche Dateien müssen Eigentümer und Rechte vor Aktivierung stimmen.

`recovery/config.db` ist ein über die SQLite-Backup-API erzeugter Datenbankstand. `files/etc/pve/` wurde daraus dekodiert und dient dem Lesen und gezielten Extrahieren. Eine Rückspielung der Datenbank ist ein eigener manuell geprüfter Recoveryablauf. VM-/Containerdefinitionen im Baum sind Konfiguration, keine Sicherung ihrer Datenträger.
