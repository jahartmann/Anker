# Wiederherstellung

## Einzeldateien

Sicherung prüfen, Zielhost und Dateien auswählen, Zielinventar lesen lassen. Netzwerkports bei anderer Hardware zuordnen; für Netzwerkänderungen Konsolenzugang bestätigen. Der Plan speichert Zielprüfsummen und eine vorbereitete Kopie. Vor Ausführung werden Zielinventar und Dateiinhalte erneut geprüft. Die exakte Plan-ID bestätigt die Übernahme. Ein geänderter Zielzustand verlangt einen neuen Plan.

Normale Configdateien unter `/etc` und `/usr/local` können atomisch pro Datei übernommen werden. Identität, Accounts, Bootloader, Paketquellen, Clusterzustand, systemd-Dienste, symbolische Links und Dateien mit Extended Attributes bleiben manuelle Schritte. Es gibt keine atomische Transaktion über alle Dateien. Bei einem Fehler kann ein Teil bereits übernommen sein; Rollbackpfad und tatsächlichen Zustand prüfen.

Vorherige Inhalte werden auf dem Ziel unter `/var/lib/anker-host/rollback/<zeit-id>/` abgelegt. `before.json` dokumentiert, ob Dateien existierten und ihre Originalmetadaten. Neue Dateien müssen beim Rollback gegebenenfalls entfernt werden. Eine automatische Rücknahme, Dienstaktivierung oder ein Neustart wird nicht behauptet. Nach Übernahme Konfigurationssyntax, Dienstfunktion, Netzwerk, Storage, PBS und Reboot gesondert prüfen.

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
