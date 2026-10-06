# Betrieb

## Produktionsbetrieb

Installation und Einrichtung starten ohne Beispieldaten. Der Linux-Dienst verwendet `/srv/anker` im Produktionsbetrieb. Hosts und weitere Benutzer werden ausdrücklich angelegt. Die optionale Demo wird nur über `anker demo` in einem eigenen, zunächst leeren Datenordner gestartet.

Die Datei `.anker-mode` hält die Betriebsart fest. Sie gehört zusammen mit dem Katalog und den übrigen Anker-Dateien in die Sicherung des zentralen Servers. Nicht löschen oder umschreiben, um eine Demo in Produktion umzuwandeln. Anker verweigert gemischte Verzeichnisse und frühere unmarkierte Demoordner; dafür einen getrennten, leeren Ordner verwenden.

## Zeitplan und Aufträge

Anker startet pro aktiviertem Host einmal täglich. Ein eigener Hostzeitplan überschreibt den gemeinsamen Zeitplan. Ein deterministischer Versatz von weniger als einer Stunde verteilt die Last. Zeitzone und Tagesmarker verhindern doppelte Läufe beim Sommerzeitwechsel. Nach Start wird ein an diesem Tag bereits fälliger Lauf nachgeholt. Der Dienst muss laufen; dies ist kein externer Cronjob. Der Versatz endet spätestens um 23:59 desselben Tages, damit späte Zeitpläne nicht dauerhaft ausfallen. Beim Wiederanlauf werden unveröffentlichte temporäre Erfassungs-/Archivdateien im reservierten `staging`-Ordner entfernt; veröffentlichte Sicherungen bleiben erhalten. Bereits abgebrochene Aufträge starten keine Hostoperation mehr, und laufende Versuche werden vor ihrem Aufruf im Katalog sichtbar.

Standard: 4 parallele Aufträge, 3 Wiederholungen für fehlgeschlagene SSH-Abfragen und Sicherungen. Wiederherstellungen werden nicht automatisch wiederholt. Je Host ist nur ein aktiver Auftrag zulässig. Abbruch ist kooperativ; bei unterbrochener Übernahme den tatsächlichen Zielzustand und das Rollbackverzeichnis prüfen. Der Dienst beendet Aufträge vor dem Schließen des Katalogs. Wartungsfehler werden im Dienstlog erfasst und im Interface für Administratoren angezeigt; ein fehlgeschlagener Wartungslauf wird nicht als erledigt markiert.

## Aufbewahrung und Speicher

Standard: 30 Tagesstände, 12 Wochenstände, 12 Monatsstände; Archivierung nach 90 Tagen. Geschützte Stände, in Plänen referenzierte Sicherungen, unvollständige Stände und der letzte erfolgreiche Stand sind ausgenommen. Tägliche Wartung läuft nur im Produktionsmodus. Regelmäßige Stände prüfen und Pflichtlücken beheben; dauerhaft unvollständige Sicherungen können sonst viel Platz belegen.

Vor neuen Sicherungen wird der freie Platz geprüft. Automatische Archivierung wird ab 90 % Belegung gestoppt, um Platz für wesentliche Sicherungen zu lassen. Manuelle Archivierung verlangt freien Arbeitsraum und prüft ihr Ergebnis vor dem Entfernen lesbarer Dateien. Auch Entpacken benötigt Platz. Eine bereits vollständig entpackte, geprüfte Sicherung wird nach einem Abbruch ohne weitere Kopie übernommen. Beschädigte Restdateien werden erst ersetzt, nachdem das Archiv geprüft wurde. Nach erfolgreichem Entpacken wird die doppelte Archivkopie entfernt. Dateigröße maximal 64 MiB, Übertragung/Archivinhalt maximal 2 GiB, maximal 100.000 Archiveinträge. Die Textvorschau ist auf 8 MiB begrenzt; größere Configs können direkt einzeln heruntergeladen werden. Automatische Übernahmen müssen einschließlich Base64 und Zielinventar ins 32-MiB-Hostprotokoll passen; zu große Auswahlen werden schon bei der Planung blockiert und können auf kleinere Pläne verteilt oder manuell exportiert werden. Ein Einzeldateidownload liefert Originalbytes; Symlinks werden als Linkzieltext mit `.symlink.txt` ausgegeben. Originalmetadaten und tatsächliche Links stehen im vollständigen TAR-Export.

Die tägliche Wartung prüft den neuesten vollständigen Stand je Host, bevor die Aufbewahrung einen letzten erfolgreichen Stand auswählt. Prüfsummenfehler erscheinen als „Beschädigt“ und schließen diesen Stand als erfolgreichen Sicherungsnachweis aus. Ältere Stände bleiben über die Prüffunktion prüfbar. Unvollständige und beschädigte Stände werden vorsorglich nicht automatisch gelöscht. Beschädigte Betriebseinstellungen führen zu einem erklärten Startfehler; Aufbewahrungswerte werden nicht still durch Defaults ersetzt.

Katalog und Sicherungsordner sind unterschiedliche Ebenen: `anker reindex` kann vorhandene, geprüfte Manifeste wieder einlesen. Bekannte Manifestprüfsummen und Schutzmarkierungen werden erhalten. Bei einem vollständig verlorenen Katalog müssen Hostzugänge und Benutzer erneut eingerichtet werden. Sicherungsordner und Recoveryanleitungen bleiben unabhängig lesbar. Katalog, SSH-Schlüssel, TLS-Dateien und den Anker-Server selbst im vorhandenen betrieblichen Sicherungskonzept berücksichtigen.

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

Sitzungen sind 30 Tage gültig, konfigurierbar von 1 bis 365 Tagen für neue Anmeldungen. Tokens werden nur gehasht im Katalog gespeichert. Passwort- oder Rechteänderungen sowie Sperren und Löschen widerrufen vorhandene Sitzungen; ein Dienstneustart erhält gültige Sitzungen. Benutzerverwaltung und Sitzungswiderruf stehen unter „Einstellungen → Zugriff“.

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
