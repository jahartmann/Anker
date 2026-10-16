# Betrieb

## Produktionsbetrieb

Installation und Einrichtung starten ohne Beispieldaten. Der Linux-Dienst verwendet `/srv/anker` im Produktionsbetrieb. Hosts und weitere Benutzer werden ausdrücklich angelegt. Die optionale Demo wird nur über `anker demo` in einem eigenen, zunächst leeren Datenordner gestartet.

Die Datei `.anker-mode` hält die Betriebsart fest. Sie gehört zusammen mit dem Katalog und den übrigen Anker-Dateien in die Sicherung des zentralen Servers. Nicht löschen oder umschreiben, um eine Demo in Produktion umzuwandeln. Anker verweigert gemischte Verzeichnisse und frühere unmarkierte Demoordner; dafür einen getrennten, leeren Ordner verwenden.

## Hostzugang einrichten

**Hosts → Host hinzufügen** beziehungsweise `a` im Terminal fragt Adresse, SSH-Benutzer und einmaliges Passwort ab. Den angezeigten Ed25519-Fingerprint unabhängig an der Proxmox-Konsole vergleichen und bestätigen. Erst danach meldet sich Anker an, installiert den eingebetteten Hosthelfer und hinterlegt getrennte öffentliche Schlüssel für `anker` und `anker-restore`. Zwei neue Verbindungen ohne Passwort prüfen beide eingeschränkten Zugänge. Anschließend werden Identität und Hostinventar gespeichert.

Das SSH-Passwort wird nicht gespeichert. Die bestehenden privaten Schlüssel unter `/etc/anker/keys` gehören dem Dienstbenutzer und bleiben mit 0600 geschützt; Verzeichnisse und verwaltete `known_hosts` bleiben root-kontrolliert. Der Bootstrap-Benutzer braucht root-Rechte oder sudo mit demselben Passwort beziehungsweise ohne Passwort. Fehlt sudo, versucht Anker die Installation über die vorhandenen APT-Quellen. Für interaktive MFA oder eigene Schlüsselverwaltung die manuelle Methode verwenden.

Bestehende Hosts mit **Verbindung einrichten** beziehungsweise `v` erneut anbinden. Der Installer erhält vorhandene Profile und andere Schlüssel. Nach einem Fehler den Zugang prüfen und wiederholen; ein teilweise eingerichteter, sicherer Zugang wird nicht entfernt. Geänderte bekannte Hostschlüssel zuerst unabhängig prüfen und ausdrücklich mit `anker host trust` übernehmen. Aktive Aufträge auf diesem Host verhindern eine erneute Anbindung; während der Anbindung sind neue Aufträge für ihn und Updates gesperrt.

## Zeitplan und Aufträge

Anker startet pro aktiviertem Host einmal täglich. Ein eigener Hostzeitplan überschreibt den gemeinsamen Zeitplan. Ein deterministischer Versatz von weniger als einer Stunde verteilt die Last. Zeitzone und Tagesmarker verhindern doppelte Läufe beim Sommerzeitwechsel. Nach Start wird ein an diesem Tag bereits fälliger Lauf nachgeholt. Der Dienst muss laufen; dies ist kein externer Cronjob. Der Versatz endet spätestens um 23:59 desselben Tages, damit späte Zeitpläne nicht dauerhaft ausfallen. Beim Wiederanlauf werden unveröffentlichte temporäre Erfassungs-/Archivdateien im reservierten `staging`-Ordner entfernt; veröffentlichte Sicherungen bleiben erhalten. Bereits abgebrochene Aufträge starten keine Hostoperation mehr, und laufende Versuche werden vor ihrem Aufruf im Katalog sichtbar.

Standard: 4 parallele Aufträge, 3 Wiederholungen für fehlgeschlagene SSH-Abfragen und Sicherungen. Wiederherstellungen werden nicht automatisch wiederholt. Je Host ist nur ein aktiver Auftrag zulässig. Abbruch ist kooperativ; bei unterbrochener Übernahme den tatsächlichen Zielzustand und das Rollbackverzeichnis prüfen. Der Dienst beendet Aufträge vor dem Schließen des Katalogs. Wartungsfehler werden im Dienstlog erfasst und im Interface für Administratoren angezeigt; ein fehlgeschlagener Wartungslauf wird nicht als erledigt markiert.

Der Dienst prüft den täglichen Zeitplan beim Start und danach jede Minute. Einstellungen und Hostformular schreiben denselben Katalog, den der Scheduler liest. Zeitplanänderungen gelten für noch nicht eingeplante Läufe. Ein neues Parallelitätslimit gilt für wartende Aufträge; laufende Aufträge werden nicht dafür beendet. Ein abgeschlossener oder fehlgeschlagener eingeplanter Lauf bleibt für seinen lokalen Kalendertag verbucht. Eine ausdrückliche Wiederholung ist ein neuer manueller Auftrag. Verpasste frühere Kalendertage werden nicht nachgeholt.

Auftrag und Tagesmarker werden in einer SQLite-Transaktion gespeichert. Misslingt einer der Schreibvorgänge, wird keiner veröffentlicht; der nächste Tick kann erneut einplanen. Ein unlesbarer Marker blockiert die Einplanung für den betroffenen Host, ohne andere Hosts stillzulegen. Fehler beim Lesen/Schreiben von Tages- oder Wartungsmarkern werden zurückgegeben und als Zeitplan-/Wartungsfehler im Interface sichtbar. Den Katalog nicht zur Fehlerbehebung löschen.

In **Aufträge → Details** stehen Erstellzeit, tatsächlicher Start, Auslöser, Ende, Versuche und Fehler. Ältere Einträge ohne Start-/Auslöserfeld zeigen „Nicht erfasst“. Sicherungen sind über ihr Ergebnis erreichbar. Abbruch bedeutet eine Anforderung an den Worker; erst dessen Endzustand bestätigt das Ende. Fehlerhafte oder abgebrochene Sicherungen/Hostprüfungen lassen sich erneut starten. Wiederherstellungen benötigen einen neuen geprüften Plan und dessen Bestätigung. Administratoren können abgeschlossene Einträge entfernen. Aktive Einträge und noch auslaufende Worker sind geschützt; Backup-Dateien und Tagesmarker werden nie über diese Aktion gelöscht. Ein Host lässt sich während aktiver Aufträge ebenfalls nicht entfernen. Historische Auftragseinträge sind keine editierbaren Zeitpläne.

CLI: `anker jobs`, `anker job show ID`, `anker job cancel ID`, `anker job retry ID`, `anker job remove ID`. Webaktionen und CLI benutzen dieselbe API. Leser dürfen Aufträge ansehen; die Rolle Wiederherstellung darf starten/abbrechen, die Historie löschen darf nur ein Administrator.

## Aufbewahrung und Speicher

Die Erfassungsprüfung unterscheidet fehlende Dateien von Laufzeitgeräten. Eine Proxmox-VM mit `rng0: source=/dev/urandom` benötigt keine Sicherung des Geräts. Der standardmäßige Vim-Include `/etc/vim/vimrc.local` ist nur dann keine Lücke, wenn seine optionale Bedingung und das Fehlen der Datei nachgewiesen sind. Echte fehlende Secrets, Hooks und nicht lesbare Dateien bleiben Hinweise. Ab 0.2.3 werden diese beiden Fehlmeldungen auch bei neuen Sicherungen mit vorhandenen Hosthelfern abgeglichen; dafür müssen nicht alle Hosts erneut angebunden werden. Unklare Fälle bleiben vorsorglich unvollständig. Historische Manifeste und ihre damaligen Hinweise bleiben erhalten; nach dem Update einen neuen Stand erstellen.

Standard: 30 Tagesstände, 12 Wochenstände, 12 Monatsstände; Archivierung nach 90 Tagen. Geschützte Stände, in Plänen referenzierte Sicherungen, unvollständige Stände und der letzte erfolgreiche Stand sind ausgenommen. Tägliche Wartung läuft nur im Produktionsmodus. Regelmäßige Stände prüfen und Pflichtlücken beheben; dauerhaft unvollständige Sicherungen können sonst viel Platz belegen.

Vor neuen Sicherungen wird der freie Platz geprüft. Automatische Archivierung wird ab 90 % Belegung gestoppt, um Platz für wesentliche Sicherungen zu lassen. Manuelle Archivierung verlangt freien Arbeitsraum und prüft ihr Ergebnis vor dem Entfernen lesbarer Dateien. Auch Entpacken benötigt Platz. Eine bereits vollständig entpackte, geprüfte Sicherung wird nach einem Abbruch ohne weitere Kopie übernommen. Beschädigte Restdateien werden erst ersetzt, nachdem das Archiv geprüft wurde. Nach erfolgreichem Entpacken wird die doppelte Archivkopie entfernt. Dateigröße maximal 64 MiB, Übertragung/Archivinhalt maximal 2 GiB, maximal 100.000 Archiveinträge. Die Textvorschau ist auf 8 MiB begrenzt; größere Configs können direkt einzeln heruntergeladen werden. Automatische Übernahmen müssen einschließlich Base64 und Zielinventar ins 32-MiB-Hostprotokoll passen; zu große Auswahlen werden schon bei der Planung blockiert und können auf kleinere Pläne verteilt oder manuell exportiert werden. Ein Einzeldateidownload liefert Originalbytes; Symlinks werden als Linkzieltext mit `.symlink.txt` ausgegeben. Originalmetadaten und tatsächliche Links stehen im vollständigen TAR-Export.

Die tägliche Wartung prüft den neuesten vollständigen Stand je Host, bevor die Aufbewahrung einen letzten erfolgreichen Stand auswählt. Prüfsummenfehler erscheinen als „Beschädigt“ und schließen diesen Stand als erfolgreichen Sicherungsnachweis aus. Ältere Stände bleiben über die Prüffunktion prüfbar. Unvollständige und beschädigte Stände werden vorsorglich nicht automatisch gelöscht. Beschädigte Betriebseinstellungen führen zu einem erklärten Startfehler; Aufbewahrungswerte werden nicht still durch Defaults ersetzt.

Katalog und Sicherungsordner sind unterschiedliche Ebenen: `anker reindex` kann vorhandene, geprüfte Manifeste wieder einlesen. Bekannte Manifestprüfsummen und Schutzmarkierungen werden erhalten. Bei einem vollständig verlorenen Katalog müssen Hostzugänge und Benutzer erneut eingerichtet werden. Sicherungsordner und Recoveryanleitungen bleiben unabhängig lesbar. Katalog, SSH-Schlüssel, TLS-Dateien und den Anker-Server selbst im vorhandenen betrieblichen Sicherungskonzept berücksichtigen.

## Speicheranzeige und Erweiterung

**Einstellungen → Betrieb → Speicher** überwacht ausschließlich den Anker-Server mit seinen sichtbaren Laufwerken und Backup-Mounts. System und Ablage auf demselben Dateisystem werden nicht doppelt gezählt. Bindmounts zeigen die Kapazität ihres eigenen Dateisystems. „Verfügbar“ ist der Platz für den Dienstbenutzer; reservierte Blöcke und Inodes erscheinen separat. Containerquoten und das zugrunde liegende Proxmox-Poolvolumen sind unterschiedliche Ebenen. Die Anzeige kann aus dem Container keine freien Poolressourcen zusagen.

Die Messung läuft unabhängig vom Backup-Zeitplan. Nicht erreichbare Dateisysteme erhalten einen Fehler statt einer Nullbelegung; blockierte Dateisystemabfragen sind zeitlich begrenzt und werden nicht unbegrenzt neu gestartet. Bis zu 90 Tage stündlicher Verlauf liegen im Katalog. Die Prognose verlangt mindestens 24 Messungen über 72 Stunden und vier Kalendertage, verwendet Tagesmediane der letzten 14 Tage und unterdrückt ungleichmäßige, rückläufige oder über sechs Stunden alte Trends. Sie ist eine Schätzung bei unverändertem Nettozuwachs, keine Kapazitätsgarantie. Aufbewahrung und neue Hosts können den Trend ändern. Nach Kapazitätswechsel beginnt eine neue Messreihe.

Die automatische Erweiterung betrifft nur ext4/XFS auf `/`, `/srv/anker` oder einem darin eingebundenen Backup-Dateisystem, sofern ein lokales Blockgerät, passende UUID und freier bereits zugewiesener Geräteplatz eindeutig bestätigt sind. Mount und Quelle müssen dasselbe Dateisystem bezeichnen. Container bekommen eine Anleitung für den Host. Partition, PV, LV und virtuellen Datenträger zuvor passend zum tatsächlichen Aufbau erweitern; Anker führt dafür keine pauschalen Änderungen aus. Werkzeuge: [resize2fs](https://man7.org/linux/man-pages/man8/resize2fs.8.html), [xfs_growfs](https://man7.org/linux/man-pages/man8/xfs_growfs.8.html).

Vor Ausführung Plan laden und den angezeigten Mountpoint exakt bestätigen. Die letzte Prüfung verwirft eine veränderte Identität oder Geometrie. Der Hilfsdienst führt ausschließlich den festen Dateisystembefehl aus; Web und CLI übergeben keine Gerätepfade oder Shellbefehle. Die Operation wird vor dem Start root-eigen unter `/var/lib/anker-updater/storage-operation.json` gespeichert. Updates, TLS-Änderungen und Einrichtung sind währenddessen gesperrt. Der Vorgang läuft beim Schließen des Browsers weiter. Ist die Statusverbindung unterbrochen, kann die Erweiterung weiterhin laufen; erst Status und tatsächliche Größe prüfen.

Nach einem Hilfsdienst-/Containerneustart mit offenem Journal erscheint „unterbrochen“. Kein automatischer Wiederholungsversuch. Mit `sudo anker storage state`, `sudo anker storage status` und `journalctl -u anker-updater` prüfen; danach einen neuen Plan laden. Ein fehlerhaftes Operationsjournal sperrt Speicherwerkzeuge, ohne Zertifikatsverwaltung und Updateprüfung pauschal stillzulegen. Das Journal nicht als vermeintliche Reparatur löschen.

### Backup-Ablage auf ein neues Laufwerk umziehen

Im Assistenten **Neues Laufwerk einbinden** ein bereits bewusst eingerichtetes ext4-/XFS-Gerät wählen. Die Anleitung kann als Textdatei heruntergeladen werden. Ein unformatiertes oder nicht eindeutig erkanntes Gerät erhält keine Formatierungsbefehle. Im Container den zusätzlichen Speicher zuerst über die Containerverwaltung bereitstellen.

Vor Beginn laufende Updates und Erweiterungen ausschließen, genug Platz am Ziel prüfen und die bestehenden Anker-Dateien sichern. Jeder Schritt wird einzeln vom Administrator ausgeführt:

1. Geräteidentität prüfen, vorübergehend unter `/mnt/anker-new` einbinden und die tatsächlich gemountete UUID vergleichen. Das Ziel muss bis auf `lost+found` leer sein.
2. Vorhandene SFTP-Bindmounts lösen; sie würden sonst weiterhin den alten Datenbestand zeigen. Anker und Updater stoppen und `/srv/anker/` mit `rsync -aHAX --numeric-ids` kopieren. Bei einem Kopierfehler nicht umschalten.
3. Den bestehenden `/srv/anker`-Eintrag in `/etc/fstab` gezielt ersetzen oder ergänzen. Vorherige Einstellung für einen Rückfall festhalten. Das alte Dateisystem beziehungsweise der alte Ordner bleibt erhalten.
4. Temporären und gegebenenfalls bisherigen Ablage-Mount lösen. Das neue Dateisystem unter `/srv/anker` mounten und seine UUID prüfen, erst danach die Dienste starten. Scheitert die Prüfung, die Dienste gestoppt lassen und den Mount korrigieren oder zur alten Zuordnung zurückkehren.
5. Anmeldung, gespeicherte Stände und eine neue Sicherung prüfen. SFTP-Bindmounts vom neuen Bestand neu einbinden und read-only prüfen. Erst anschließend eine Bereinigung des alten Bestands planen.

Dieser Umzug ist keine automatisch ausgeführte Migration. Verschachtelte Backup-Mounts und eigene SFTP-/fstab-Abhängigkeiten gesondert berücksichtigen. Reale LXC-, LVM-, XFS- und Migrationsabnahme steht noch aus; siehe [SUPPORT.md](SUPPORT.md).

## Benachrichtigungen

SMTP verwendet TLS; alternativ Webhook. Gemeldet werden fehlgeschlagene Sicherungsaufträge und Erholungen. Aktivierte Hosts ohne aktuellen vollständigen Stand lösen zusätzlich eine Überfälligkeitsmeldung aus. Der Zustand wird dauerhaft gespeichert: Ein Neustart erzeugt keine erneute Warnung für denselben Vorfall. Nach einer aktuellen vollständigen Sicherung folgt eine Erholungsmeldung. Fehlgeschlagene Überfälligkeitsmeldungen werden frühestens nach einer Stunde erneut versucht. Ohne eingerichtetes Ziel erfolgt keine automatische Meldung.

Ungültige Zieladressen werden beim Speichern abgewiesen; ein manueller Test ohne Ziel ist ein Fehler. Testnachricht erst nach Speichern der Verbindung verwenden. Der letzte Zustellfehler erscheint für Administratoren dauerhaft im Interface, bis eine Zustellung oder ein Test erfolgreich ist. Keine Secretinhalte in Meldungen aufnehmen.

## Optionaler SFTP-Export ohne zweite Kopie

Auf dem Anker-Server `sudo ./scripts/setup-sftp-export.sh /pfad/zu/export-key.pub` ausführen. Das Skript erstellt einen separaten SFTP-Account mit der numerischen Anker-UID, einen root-eigenen Chroot und read-only Bind-Mounts von `hosts` und `plans`. `internal-sftp -R` verhindert Schreibbefehle zusätzlich. Katalog, Benutzercredentials und SSH-Schlüssel werden nicht eingebunden. Der Zugriff umfasst ausdrücklich alle Secrets in den exportierten Sicherungen.

Vor Aktivierung die effektive OpenSSH-Konfiguration prüfen:

```sh
sudo sshd -t
sudo sshd -T -C user=anker-export,host=anker-server,addr=192.0.2.1
```

Erst danach SSH neu laden; vorhandenen administrativen Zugang behalten. Bind-Mounts sind zunächst bis zum Neustart aktiv. Für dauerhaften Betrieb folgende Einträge in `/etc/fstab` aufnehmen und Mounts nach `/srv/anker` starten lassen:

```text
/srv/anker/hosts /srv/anker-sftp/hosts none bind,ro,nodev,nosuid,noexec 0 0
/srv/anker/plans /srv/anker-sftp/plans none bind,ro,nodev,nosuid,noexec 0 0
```

Nach jedem Neustart mit `findmnt /srv/anker-sftp/hosts /srv/anker-sftp/plans` read-only prüfen. Die doppelte numerische UID ist für den Zugriff auf `0600`-Originale nötig; der Exportaccount darf keine Shell, Portweiterleitung oder sonstigen Dienstzugang erhalten. Nur ein geprüftes dediziertes Schlüsselpaar verwenden. SFTP-Installationsschritte benötigen reale Linux-/OpenSSH-Abnahme; sie wurden lokal auf macOS nicht angewendet.

Archivierte Sicherungen enthalten weiterhin Manifest und `archive.tar.gz`. Für lesbare Einzeldateien kann Anker die Sicherung automatisch wieder öffnen oder ein Administrator das Archiv separat in einen neuen Arbeitsordner entpacken.

## Zugänge

Sitzungen sind 30 Tage gültig, konfigurierbar von 1 bis 365 Tagen für neue Anmeldungen. Tokens werden nur gehasht im Katalog gespeichert. Passwort- oder Rechteänderungen sowie Sperren und Löschen widerrufen vorhandene Sitzungen; ein Dienstneustart erhält gültige Sitzungen. Benutzerverwaltung und Sitzungswiderruf stehen unter „Einstellungen → Zugriff“. Dort ist auch die Passwort-Mindestlänge einstellbar: standardmäßig acht Zeichen, zulässiger Bereich 8 bis 128. Die Grenze gilt für neue oder neu gesetzte Passwörter, nicht rückwirkend bei der Anmeldung. „Aktionen → Passwort ändern“ erlaubt auch den eigenen Administratorzugang; dieser Passwortwechsel beendet dessen bestehende Sitzungen. Web und Terminal prüfen dieselbe gespeicherte Vorgabe.

Webrollen: `reader`, `restore`, `admin`; Secretfreigabe wird separat vergeben. Reader können keine Mutationen ausführen. Originalexports erfordern Secretfreigabe. Vorschauen bleiben auch für Administratoren zunächst verdeckt. Unbekannte Configpfade werden vorsorglich als geschützt behandelt; nur bekannte allgemeine Systemdateien sind ohne Secretfreigabe lesbar. Das Anzeigen geschützter Inhalte wird protokolliert. Unix-Socketzugang erlaubt volle Administration; Dateirechte sind deshalb eine Berechtigungsgrenze.

Das Dateisystem enthält absichtlich nutzbare Originale. Vertraulichkeit des Speichers braucht verschlüsselte Serverdatenträger, kontrollierten Serverzugang und die bestehenden betrieblichen Sicherungen. Anker verwaltet keine Festplattenverschlüsselung. Kein HTTP auf einer ungeschützten Netzadresse betreiben; der Dienst verlangt dort standardmäßig TLS.

Referenzen auf Proxmox-Hooks sowie explizite Config-/Schlüsselpfade werden begrenzt gescannt. Nicht erfasste Referenzen erscheinen als Pflichtlücken. Anwendungsspezifische indirekte Referenzen und große/binäre Configs müssen zusätzlich im root-eigenen Hostprofil angegeben und manuell geprüft werden. Metadaten-/ACL-Lesefehler erzeugen ebenfalls eine unvollständige Sicherung.

Passwort zurücksetzen (lokal als root, nicht in die Befehlszeile schreiben):

```sh
read -rs -p 'Neues Passwort: ' ANKER_USER_PASSWORD; echo
export ANKER_USER_PASSWORD
sudo --preserve-env=ANKER_USER_PASSWORD anker user password admin
unset ANKER_USER_PASSWORD
```

## Programmupdates

Signierte GitHub-Releases, getrennte Updateberechtigung und automatische Wiederherstellung von Binärdatei und Katalog: [UPDATES.md](UPDATES.md).

## Kaltes, verschlüsseltes Disaster-Paket

`disaster` arbeitet lokal ohne laufenden Unix-Socket. Es sichert die Anker-Instanz
(Katalog, lokale Sicherungen und private Recovery-Nachweise), nicht den gesamten
Server und keine Proxmox-VMs. Der Import startet keinen Dienst, Updater oder Restore.
Eine echte Wiederherstellungsabnahme auf isoliertem PVE bleibt separat erforderlich.

### Export und Prüfung bei gestopptem Dienst

Dienst **und Updater** stoppen; auch Einrichtung, Installer und sonstige Schreiber
müssen beendet sein. Beispiel für eine installierte Linux-Instanz:

```sh
sudo systemctl stop anker.service anker-updater.service
sudo anker --data /srv/anker disaster export \
  --config-root /etc/anker --output /sicherer-datentraeger/anker.disaster \
  --passphrase-file /geschuetzt/disaster-passphrase
sudo anker disaster inspect --input /sicherer-datentraeger/anker.disaster \
  --passphrase-file /geschuetzt/disaster-passphrase
```

Die Passphrase-Datei ist eine reguläre Datei ohne Symlink, höchstens 4097 Bytes,
mit ausschließlich Eigentümerrechten (beispielsweise `0600`). Eine abschließende
LF-Zeile wird entfernt; die Passphrase enthält 1–4096 Bytes. Eine lange zufällige
Passphrase getrennt vom Paket verwahren. Ohne `--passphrase-file` fragt Anker sie
verdeckt am Terminal ab; Pipes und Passphrasen als CLI-Argument sind nicht erlaubt.
Der Export überschreibt keine vorhandene Paketdatei. `inspect` authentifiziert
und prüft Inventar sowie enthaltene Sicherungs-/Planbytes in einem privaten temporären
Verzeichnis. Es zeigt Version, Modus, Zähler, Quellen-IDs/Host-IDs, bekannte Manifestdigests,
Abdeckungszustände und allgemeine Migrationsvoraussetzungen; keine Zugangsdaten,
Originalpfade oder privaten Dateiinhalte. Export, Inspect und Import prüfen enthaltenes Material.

### Installation und Sicherungsdaten getrennt bewahren

Neue Exporte verwenden Nutzlastversion 2. Für jede katalogisierte Sicherung enthält
`sources` genau einen Eintrag mit `backup_id`, `host_id`, `manifest_sha` und `state`:

- `included`: Das Paket enthält die tatsächlichen Manifest-/Datei-/Artefaktbytes
  beziehungsweise das geprüfte Archiv. Export, Inspect und Import prüfen diese Bytes.
- `missing`: Das gesamte lokale Sicherungsverzeichnis fehlt. Der Katalog und vorhandene
  private Plan-, Zielaufnahme-, Rezept- und Prüfhistorie bleiben enthalten. Ein bekanntes
  Manifestdigest ist nur eine erwartete Bindung; ein leerer Digest bedeutet unbekannt.
- `separate`: Bewusst ausgelassene Sicherungsdaten. Beim ersten Split-Export werden
  vorhandene lokale Quellen einzeln vollständig geprüft, aber ihre Bytes nicht eingepackt.
  Nach Import/Reexport bleibt diese offene Einstufung bei weiter fehlenden Daten erhalten.
  Sie bestätigt weder eine verfügbare externe Kopie noch eine aktuelle Integritätsprüfung.

Für viele aufbewahrte Sicherungen mit getrenntem Datenträger:

```sh
sudo anker --data /srv/anker disaster export --backup-data separate \
  --config-root /etc/anker --output /sicherer-datentraeger/anker-installation.disaster \
  --passphrase-file /geschuetzt/disaster-passphrase
```

Der Standard `--backup-data include` nimmt alle vorhandenen Quellen auf; ganz fehlende
Quellen bleiben ausdrücklich offen. Ein vorhandenes leeres Verzeichnis, fehlendes
Manifest, beschädigte Dateien, Links oder unsichere private Nachweise führen in beiden
Modi zum Abbruch. Solche Fehler werden nie in `missing` umgedeutet. Der Split-Modus
prüft jeweils eine Quelle innerhalb der unten genannten Grenzen; die gesamte Menge
separater Sicherungen muss nicht in das 256-MiB-Paket passen. Er kopiert keine Daten auf
einen anderen Datenträger und folgt keinen externen Katalogpfaden.

Nach dem isolierten Import stehen dieselben Quellenbindungen im privaten
`disaster-migration.json` und im internen Migrationskatalog. Für `missing`/`separate`
bleiben tatsächliche Bytevollständigkeit, Verfügbarkeit und physische Unabhängigkeit
unbekannt. Alte Prüfzeiten sind historische Angaben, keine Importprüfung ausgelassener
Bytes. Fehlende Daten separat beschaffen, vor Ort gegen Backup-ID, Host-ID und den
bekannten Manifestdigest prüfen und als vollständiges reguläres Verzeichnis unter
`hosts/HOST/backups/BACKUP` bereitstellen; keine bestehenden Daten überschreiben und
keine Symlinks einsetzen. Erst nach Prüfung aller Migrationsvoraussetzungen aktivieren,
Quellen mit `anker backup verify BACKUP` prüfen und neue Recovery-Pläne erstellen.
Alte Pläne und Zielaufnahmen bleiben auch nach Nachlieferung dauerhaft widerrufen.

### Isolierter Import und bewusste Aktivierung

Das Elternverzeichnis muss als echtes Verzeichnis bereits existieren. In diesem Verzeichnis `pwd -P` verwenden und den neuen Zielnamen an diesen kanonischen Pfad anhängen. Unter macOS ist `/private/tmp` der reale Elternpfad, `/tmp` ein abgewiesener Symlink. Die Beschränkung bleibt bestehen. Das Ziel darf **noch nicht
existieren**, auch nicht als leeres Verzeichnis oder Symlink. Es darf nicht
innerhalb einer vorhandenen Anker-Instanz liegen:

```sh
sudo anker disaster import --input /sicherer-datentraeger/anker.disaster \
  --destination /srv/anker-isoliert --passphrase-file /geschuetzt/disaster-passphrase
```

Anker prüft zuerst Authentifizierung, Inventar, Originalmanifeste, Sicherungsdaten,
private Pläne und Zielnachweise in einem privaten temporären Verzeichnis. Erst
nach erfolgreicher Prüfung wird eine vollständig geschriebene Ablage atomar und
ohne Überschreiben veröffentlicht. Verzeichnisse erhalten `0700`, Dateien `0600`.
Import unter dem künftigen Dienstkonto ausführen oder anschließend Eigentümer der
neuen Ablage bewusst an dieses Konto übertragen. UID/GID und Originalmodi in
Sicherungsmanifesten werden dabei unverändert erhalten.

Die Datei `.disaster-isolated` verhindert Dienststart, Demo-Start und Initialisierung.
Alle Hosts sind deaktiviert; Sitzungen sind gelöscht; wartende/laufende Aufträge
sind `interrupted`. Bestehende Plan- und Zielaufnahme-Dateien bleiben unverändert
lesbare Belege. Separate Katalogsperren verhindern die Wiederverwendung alter
Pläne für Apply, Reconcile und Rollback sowie alter Zielaufnahmen für neue Pläne,
auch nach Aktivierung. Neue Zielprüfung und neuer Plan sind erforderlich.

Vor Aktivierung auf dem **isolierten** Rechner:

1. `disaster-migration.json` lokal unter Eigentümerrechten lesen. Dort stehen
   ursprüngliche Daten-/Konfigurationswurzeln und konkrete externe Referenzen.
   Fehlende Quellhosts werden offline weder kontaktiert noch als verfügbar bestätigt.
2. App-eigene Schlüssel, `known_hosts`, TLS-Dateien und Konfiguration prüfen.
   Material aus `--config-root` liegt unter `disaster-config/` in Quarantäne.
   Die Hostfelder für app-eigene Key-/Trust-Pfade sind auf die neue Ablage umgestellt.
   Externe absolute Pfade werden niemals verfolgt oder automatisch umgeschrieben:
   externe Schlüssel/TLS-Zertifikate separat sicher übertragen, Rechte setzen und
   SSH-Fingerprints über einen unabhängigen Kanal kontrollieren. Ein Schlüssel
   ersetzt keine erneute Vertrauensprüfung am neuen Einsatzort.
3. `service.env` **nicht ungeprüft sourcen**. Für den späteren Dienst eine neue
   Konfiguration mit dem neuen `ANKER_DATA`, einem isolierten Listen-/Socketpfad
   und geprüften `ANKER_TLS_CERT`/`ANKER_TLS_KEY` erstellen. Quarantänisierte
   `service.env`, `update.json` (einschließlich `token_file`) und TLS-Konfiguration
   behalten ihre ursprünglichen Bytes und Pfade; erforderliche Pfade bewusst in
   einer neuen aktiven Konfiguration anpassen. Den Updater separat neu einrichten;
   keine alten Pending-/Installationsdateien übernehmen. Benachrichtigungsziele
   und Zeitpläne vor Netzwerkfreigabe prüfen; Einstellungen wurden erhalten.
4. Erst nach dieser lokalen Prüfung die Aktivierung ausdrücklich vornehmen,
   beispielsweise `sudo rm -- /srv/anker-isoliert/.disaster-isolated`. Dieser
   einzelne Schritt hebt ausschließlich die Startsperre auf. Er startet nichts.
   Für die erste Prüfung Anker manuell mit `--data /srv/anker-isoliert` und eigener
   Loopback-Adresse starten; den erhaltenen Modus beachten (`serve` für production,
   `demo` nur für eine ausdrücklich getrennte Demo). Hosts erst nach Prüfung ihrer
   Adressen/Zugänge einzeln aktivieren. Dienste erst anschließend bewusst einrichten.

### Weitere Disaster-Exporte nach einem Import

Ein normaler Folgeexport mit `--data NEUER_ROOT` nimmt die erlaubten Dateien unter
`NEUER_ROOT/disaster-config/` automatisch wieder auf. Ein erneutes `--config-root`
ist dafür nicht erforderlich. Beispiel bei weiterhin gestoppter Instanz:

```sh
sudo anker --data /srv/anker-isoliert disaster export \
  --output /sicherer-datentraeger/anker-zweite-generation.disaster \
  --passphrase-file /geschuetzt/disaster-passphrase
```

Wird `--config-root` ausdrücklich auf genau dieselbe Quarantäne gesetzt, erkennt
Anker deren geöffnete kanonische Verzeichnisidentität und erfasst sie nur einmal.
Andere überlappende Daten-/Konfigurationswurzeln werden abgelehnt. Auch eine andere,
separate Konfigurationswurzel wird bei bereits vorhandenen Quarantänedateien
abgelehnt: unterschiedliche Schlüssel und Konfigurationen werden nicht still
zusammengeführt oder überschrieben.

Wenn nach Aktivierung beispielsweise neue aktive Dateien unter `/etc/anker`
entstanden sind, historische Quarantäne unverändert aufbewahren. Die aktiven
Schlüssel/Trust-Dateien nach Prüfung unter neuen eindeutigen Namen in
`NEUER_ROOT/keys/active/`, TLS-Dateien in `NEUER_ROOT/tls/active/` und aktive
Anker-Konfiguration in `NEUER_ROOT/config/` kopieren (Verzeichnisse `0700`, Dateien
`0600`, passender Eigentümer). Aktive Host-Key-/Trust-Pfade und die neu geprüfte
Dienst-/TLS-/Token-Konfiguration bewusst auf diese Kopien umstellen; anschließend
bei gestoppten Schreibern ohne `--config-root` exportieren. So enthält der normale
Datenroot-Export sowohl die unveränderten historischen Quarantänedateien als auch
das aktuelle app-eigene Material. Historische `service.env`-/JSON-Dateien behalten
alte Pfade; deren Referenzen können weiterhin als manuell zu prüfende historische
Migrationshinweise erscheinen. Sie werden nicht automatisch aktiv verwendet.

Für jede explizit geöffnete Daten-/Konfigurationswurzel merkt sich das Paket die
angegebene und die dazu geprüfte kanonische Schreibweise, beispielsweise
`/var/...` und `/private/var/...`. Ein app-eigener Key-/Trust-Pfad unter einer dieser
beiden Schreibweisen wird beim Import auf die neue lokale Kopie umgestellt.
Andere, ausschließlich im Katalog genannte Symlink-Aliase werden nicht aufgelöst
und bleiben externe Migrationsreferenzen. Alle Quarantänebytes, deaktivierten
Hosts und dauerhaften Sperren alter Pläne/Zielaufnahmen bleiben auch nach
weiteren Export-/Importgenerationen erhalten.

Der Import ist kein Nachweis einer unabhängigen Zweitkopie oder erfolgreicher
Hostwiederherstellung. Vorhandene Verifikationsdaten und Copy-Receipts behalten
Datum, Herkunft und ursprüngliche Quellenbindung; Export/Import erzeugt keine
neuen Backup-Verifikationsbelege und bestätigt keine Fehlerdomänentrennung.

### Format 1, Umfang und Grenzen

Das binäre Format beginnt mit `ANKER-DISASTER`, NUL und Versionsbyte `0x01`,
anschließend 16 zufälligen Salt-Bytes und 12 zufälligen Nonce-Bytes. scrypt mit
festem `N=32768`, `r=8`, `p=1` erzeugt einen 32-Byte-Schlüssel. AES-256-GCM schützt
JSON-Nutzlast und gesamten Header als Associated Data; der Tag ist 16 Bytes lang.
KDF-Parameter können vom Paket nicht erhöht werden. Ein falsches Passwort,
veränderte Daten oder eine unbekannte Version werden abgelehnt.

Die verschlüsselte JSON-Nutzlast enthält `version`, `created_at`, `source_root`,
optional `config_root`, `source_canonical_root`, `config_canonical_root`,
`mode`, `records`, `files`, `references` sowie ab Nutzlastversion 2 `sources`.
Der verschlüsselte äußere Header bleibt Version 1; neue Leser unterstützen vollständige
alte Nutzlasten der Version 1. Alte Binaries können Version 2 nicht lesen. Die optionalen kanonischen Wurzeln
stammen ausschließlich aus identitätsgeprüften, explizit geöffneten Verzeichnissen;
ältere Pakete ohne diese Zusatzfelder bleiben lesbar. Jeder Katalogeintrag
enthält `bucket`, `id`, seine **unveränderten Rohbytes** als Base64-`value` und
`sha256`. Dadurch bleiben getrennte Credentials, JSON-ausgeblendete/private Felder
und unbekannte Zusatzfelder erhalten. Jeder reguläre Dateieintrag enthält einen
normalisierten relativen `path`, Base64-`data` und `sha256`. Doppelte Pfade/Datensätze,
Traversal, absolute Paketpfade, Datei-/Verzeichniskollisionen, Sonderdateien,
Symlinks und mehrfach verknüpfte Quelldateien werden abgewiesen. Private Plan- und
Zielnachweise müssen schon an der Quelle Dateien `0600`/Verzeichnisse `0700` haben.

Unter dem Datenroot werden `.anker-mode`, `hosts/` (einschließlich vollständiger
Sicherungsdaten oder Archive), `plans/`, `recovery-inspections/`, `recovery-recipes/`, `clusters/`,
`keys/`, `ssh/`, `tls/`, `known_hosts`, `service.env` und erlaubte Anker-Konfiguration
unter `config/` und `disaster-config/` aufgenommen. `--config-root` ist eine explizite zusätzliche Quelle;
zugelassen sind `keys/`, `tls/`, `known_hosts`, `service.env`, `tls-renewal.json`,
`update.json`, `setup-complete`, `release-public.key`, `release.pub`,
`release-key.pub` und reguläre `github-token-*`-Dateien. Die Tooling-Binaries,
Systemd-Units, Logs, Exporte, Staging-Verzeichnisse, Demo-Quellhosts, Sockets und
Updater-/Setup-/Installations-Pendingzustände gehören nicht zum Paket. Externe
Backup-Symlinks werden nicht verfolgt. Ganz fehlende Sicherungsverzeichnisse werden
im Quelleninventar als offen erfasst. Referenzierte, fehlende lokale Schlüssel
oder teilweise vorhandene/beschädigte Sicherungs-/Plan-/Zieldaten führen zum Abbruch; externe
Credential-/Konfigurationspfade werden stattdessen als Migrationspflicht aufgeführt.

Die vorhandene `InstanceLock` schließt laufende Anker-Instanzen aus. Der Katalog
wird samt vorhandenem SQLite-WAL descriptorgebunden in eine private Kopie gelesen,
dort read-only mit `integrity_check` geöffnet und als Rohdatensätze aufgenommen.
Die Quelle erhält kein Schema-Upgrade und keinen erzwungenen Checkpoint. Alle
anderen Schreiber müssen ebenfalls gestoppt sein; die Sperre ersetzt keine
Dateisystem-Snapshotgarantie gegen fremde Prozesse. Dateioperationen verwenden
geöffnete Verzeichnisidentitäten, `O_NOFOLLOW`, Inventarprüfungen und abschließende
Identitätsprüfungen. Der finale Import verwendet descriptorgebundenes Staging
und Linux `RENAME_NOREPLACE` beziehungsweise macOS `RENAME_EXCL`.

Grenzen: 64 MiB je Quelldatei und je DB-/WAL-Datei, 64 MiB rohe Katalogwerte,
256 MiB Dateinutzdaten, höchstens 50.000 Quelldateien/-verzeichniseinträge und
50.000 Katalogdatensätze, Verzeichnistiefe bis 101 Komponenten; maximal 384 MiB
verschlüsseltes Paket einschließlich Base64-/JSON-Aufwand. Sicherungsarchive
werden zusätzlich auf insgesamt 256 MiB entpackte Nutzdaten und 50.000 Tar-Einträge
begrenzt. Das Format wird im Speicher verarbeitet; genügend RAM und privaten
Temporärspeicher sowie Platz für Prüfung und Zielkopie einplanen. Im Split-Modus gelten
die Datei-/Daten-/Archivgrenzen je einzeln geprüfter Quelle und separat für das
Installationspaket; Quelleninventar und Katalog bleiben auf jeweils 50.000 Einträge begrenzt.
Einzelne größere Quellen benötigen einen gesonderten, offline geprüften Migrationsweg. Temporäre Klartexte
werden mit privaten Rechten erzeugt und regulär entfernt, nicht kryptografisch
vom Datenträger gelöscht; für sensible Systeme verschlüsselten Temporärspeicher
verwenden. Alte temporäre Stagingreste nach einem Prozess-/Stromausfall nur bei
weiterhin gestopptem Dienst prüfen und entfernen.

### Historischen Recovery-Vorgang am ursprünglichen Ziel klären

`anker restore reconcile PLAN` und die separat bestätigte Rücksetzung prüfen die
private gespeicherte v2-Zielaufnahme und ihre SSH-Verbindungsbindung bei jedem
Journalzugriff. Adresse, Port, Sicherungsrolle, Schlüsselbezug und SSH-Vertrauen müssen
zur Aufnahme passen. Änderungen während der Abfrage werden verworfen; unmittelbar
vor einer Rücksetzung wird erneut geprüft. Eine während der Rücksetzung geänderte
Bindung lässt das Ergebnis unterbrochen, ohne automatischen Wiederholungsversuch.
Eine verfügbare/gesunde Quellsicherung ist dafür nicht erforderlich: Rücksetzung
betrifft das vorhandene Journal am ursprünglichen Ziel. Bei Legacy-Plänen oder
fehlender/defekter Zielbindung an der ursprünglichen, unabhängig identifizierten
Zielkonsole Journal und Drift manuell prüfen. Keine neue Adresse in einen historischen
Plan einsetzen und keine blinde zweite Übernahme starten. Importierte Pläne bleiben gesperrt.
