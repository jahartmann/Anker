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

Standard: 30 Tagesstände, 12 Wochenstände, 12 Monatsstände; Archivierung nach 90 Tagen. Geschützte Stände, in Plänen referenzierte Sicherungen, unvollständige Stände und der letzte erfolgreiche Stand sind ausgenommen. Tägliche Wartung läuft nur im Produktionsmodus. Regelmäßige Stände prüfen und Pflichtlücken beheben; dauerhaft unvollständige Sicherungen können sonst viel Platz belegen.

Vor neuen Sicherungen wird der freie Platz geprüft. Automatische Archivierung wird ab 90 % Belegung gestoppt, um Platz für wesentliche Sicherungen zu lassen. Manuelle Archivierung verlangt freien Arbeitsraum und prüft ihr Ergebnis vor dem Entfernen lesbarer Dateien. Auch Entpacken benötigt Platz. Eine bereits vollständig entpackte, geprüfte Sicherung wird nach einem Abbruch ohne weitere Kopie übernommen. Beschädigte Restdateien werden erst ersetzt, nachdem das Archiv geprüft wurde. Nach erfolgreichem Entpacken wird die doppelte Archivkopie entfernt. Dateigröße maximal 64 MiB, Übertragung/Archivinhalt maximal 2 GiB, maximal 100.000 Archiveinträge. Die Textvorschau ist auf 8 MiB begrenzt; größere Configs können direkt einzeln heruntergeladen werden. Automatische Übernahmen müssen einschließlich Base64 und Zielinventar ins 32-MiB-Hostprotokoll passen; zu große Auswahlen werden schon bei der Planung blockiert und können auf kleinere Pläne verteilt oder manuell exportiert werden. Ein Einzeldateidownload liefert Originalbytes; Symlinks werden als Linkzieltext mit `.symlink.txt` ausgegeben. Originalmetadaten und tatsächliche Links stehen im vollständigen TAR-Export.

Die tägliche Wartung prüft den neuesten vollständigen Stand je Host, bevor die Aufbewahrung einen letzten erfolgreichen Stand auswählt. Prüfsummenfehler erscheinen als „Beschädigt“ und schließen diesen Stand als erfolgreichen Sicherungsnachweis aus. Ältere Stände bleiben über die Prüffunktion prüfbar. Unvollständige und beschädigte Stände werden vorsorglich nicht automatisch gelöscht. Beschädigte Betriebseinstellungen führen zu einem erklärten Startfehler; Aufbewahrungswerte werden nicht still durch Defaults ersetzt.

Katalog und Sicherungsordner sind unterschiedliche Ebenen: `anker reindex` kann vorhandene, geprüfte Manifeste wieder einlesen. Bekannte Manifestprüfsummen und Schutzmarkierungen werden erhalten. Bei einem vollständig verlorenen Katalog müssen Hostzugänge und Benutzer erneut eingerichtet werden. Sicherungsordner und Recoveryanleitungen bleiben unabhängig lesbar. Katalog, SSH-Schlüssel, TLS-Dateien und den Anker-Server selbst im vorhandenen betrieblichen Sicherungskonzept berücksichtigen.

## Speicheranzeige und Erweiterung

**Einstellungen → Speicher** überwacht ausschließlich den Anker-Server mit seinen sichtbaren Laufwerken und Backup-Mounts. System und Ablage auf demselben Dateisystem werden nicht doppelt gezählt. Bindmounts zeigen die Kapazität ihres eigenen Dateisystems. „Verfügbar“ ist der Platz für den Dienstbenutzer; reservierte Blöcke und Inodes erscheinen separat. Containerquoten und das zugrunde liegende Proxmox-Poolvolumen sind unterschiedliche Ebenen. Die Anzeige kann aus dem Container keine freien Poolressourcen zusagen.

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
