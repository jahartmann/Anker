# Unterstützungsstand

| Bereich | Implementiert | Lokal geprüft | Reales Proxmox-Labor |
|---|---|---|---|
| Lesbare versionierte Sicherungen und Prüfsummen | Ja | Go-Tests, Demo | Offen |
| SQLite-Snapshot und pmxcfs-Dekodierung | Ja | Python-Fixture mit konsistentem Baum | Offen |
| SSH mit geprüftem Hostschlüssel und festem Helfer | Ja | Echte SSH-Verbindungen und Helferinstallation auf isoliertem Debian-13-Ziel mit PVE-Kommandofixture, root/sudo, Rollen und Wiederanbindung; Go-/Browser-/PTY-Prüfungen | Reales PVE-Labor offen |
| Einzeldateiübernahme mit Driftprüfung und Rollbackkopie | Ja | Go-/Python-Tests, Demo-Browserablauf | Offen |
| Netzwerkportzuordnung | Ja | Transformation und fehlende Zuordnung | Verkabelung/Erreichbarkeit offen |
| Neue Hardware, Cluster, Versionen, Topologie | Manuelle Pläne und Exporte | Entscheidungsregeln und Dokumente | Offen; automatische Gesamtausführung gesperrt |
| Web, CLI und Terminal | Ja | Build, Go-Tests, Browserabläufe | Betrieb offen |
| Aufbewahrung, Archivierung, Scheduler | Ja | Go-Tests einschließlich DST und Archivprüfung | Langzeitbetrieb offen |
| Linux-/systemd-Serverinstallation und Updates | Geführte Einrichtung und signierter Updater | Echte systemd-Dienste in isolierter GitHub-Linux-VM | Physischer Reboot/Stromausfall offen |
| SFTP-Installation | Skripte und Anleitung | Syntax | Reale Ausführung offen |
| Speicher von Anker und seinen Backup-Laufwerken | Anzeige, Verlauf, bedingte Prognose, begrenzte ext4-/XFS-Erweiterung und manuelle Assistenten | Inventar/Bindmounts, Prognose, Gerätewechsel, Journal, API/CLI und Browser; echte ext4-Erweiterung über systemd bestanden | LXC-/LVM-/XFS-/Migrationsabnahme offen |

Der Versionsrahmen für automatische Einzeldateipläne umfasst gleiche Proxmox-Major-Versionen 8 oder 9. Das ist eine Entscheidungsregel, keine Zertifizierung jedes Minorstands. Hardware-, Cluster- und Versionskombinationen müssen vor Freigabe separat dokumentiert werden.

Erforderliche Abnahme: mindestens ein Standalone-Host, ein Node im gesunden Cluster und ein vollständiger Clusterverlust im isolierten Netz; neue Hardware mit anderen NIC-Namen und Diskkennungen; PBS-Verbindung und Gastrestores; Neustart; Wiederherstellung ohne Anker über SFTP; unterbrochene Übertragung und Übernahme; knappes Dateisystem; Linux-Servicehärtung und effektive SSH-Berechtigungen.

Grenzen: keine Gastdaten, kein Image-/Bootloaderrestore, keine automatische Quorumkorrektur, kein Ceph-Wiederaufbau, kein Proxmox-Versionsupgrader, kein automatisch bestätigter Reboot, keine atomische Gesamtrückspielung und keine automatisierte Metadatenübernahme für Extended Attributes/Links. Diese Bereiche erscheinen als manuelle Schritte und bleiben exportierbar.

## Prüfung typischer Fehler nach drei Monaten Betrieb

Prüfstand vom 05.10.2026: simulierte Zeitpunkte und Fehler, keine behaupteten drei Monate Produktionsbetrieb. Die Tests erzeugen 90 tatsächliche lokale Sicherungsstände für die Aufbewahrung sowie 3.600 synthetische Listeneinträge für die Browserprüfung. Alle Hostzugriffe bleiben Fixtures. Die vorherige Gesamtprüfung und ihre Befunde stehen zusätzlich in `REVIEW.md`.

| Nutzerproblem / Fehlerfall | Umgesetzte Korrektur oder bestätigte Absicherung |
|---|---|
| Downloads sind versteckt; einzelne Dateien fehlen | Direkte Downloads in der Sicherungsliste, unveränderte Einzeldateien bis 64 MiB und vollständige Planexports. |
| Fehlender Zugriff oder beschädigter Export verlässt das Interface | Downloadvorprüfung zeigt den Fehler im Interface; fehlerhafte Streams brechen ab und erhalten keinen irreführenden JSON-Anhang. |
| Geschützte Dateien könnten ohne Freigabe heruntergeladen werden | Server prüft auch inhaltsbasierte Secretklassifizierung, nicht nur das Manifestflag. Reader können allgemeine Dateien herunterladen; Vollstand-/Planexports erfordern Secretfreigabe. |
| Große Datei hat keine Vorschau und ist dadurch unzugänglich | Download unabhängig vom 8-MiB-Vorschaulimit; Originalbytes und Prüfsumme bleiben erhalten. |
| Langsame Dateiantwort überschreibt die neue Auswahl | Antwort wird nur für die weiterhin aktuelle Auswahl übernommen. |
| Dienst/VPN fällt aus, alte grüne Daten bleiben stehen | Dauerhafte Verbindungs-/Datenstandmeldung; neue fehlgeschlagene Backupversuche erscheinen als Handlungsbedarf. Pausierte Hosts sind separat erkennbar. |
| Sitzung läuft während einer Aktion ab | Rückkehr zur Anmeldung bei jeder 401-Antwort, offene Dateidialoge und sensible Ansichten werden geschlossen. Abgelaufene serverseitige Sitzungen werden aufgeräumt. |
| Zeitplan kurz vor Mitternacht fällt nie an | Lastversatz auf den letzten Zeitpunkt desselben Tages begrenzt; DST-/Tagesmarker weiterhin geprüft. |
| Laufende Sicherung überschreibt eine Änderung im Hostformular | Inventaraktualisierung übernimmt nur Telemetrie in die aktuellen Einstellungen. Veraltete Formulare löschen keine neuen Inventardaten. |
| Während der Sicherung wird die Zieladresse geändert | Inventar des alten SSH-Ziels wird dem neuen Ziel nicht zugeordnet; bisherige Sicherung bleibt als Originalstand erhalten. |
| Beschädigte Betriebseinstellungen ersetzen die Aufbewahrung durch Defaults | Erklärter Startfehler statt stiller Rücksetzung. |
| Bekannter Prüfsummenfehler bleibt grün | Beschädigten Stand dauerhaft markieren; Wartung prüft den neuesten vollständigen Stand und kann auf ältere vollständige Stände zurückfallen. |
| Vorbereitete Datei oder Planmetadaten werden verändert | Export und Ausführung prüfen vorbereitete Hashes, Plan, Mapping und Quellmanifest vor Nutzung. |
| Nach einem Abbruch existieren Archiv und teilweise entpackte Dateien | Nur geprüften Baum übernehmen; sonst aus dem geprüften Archiv ersetzen. Reindex kann einen gültigen Archivstand trotz beschädigter Restdateien wiederfinden. |
| Wieder geöffnetes Archiv belegt dauerhaft doppelt Platz | Nach erfolgreicher Veröffentlichung der lesbaren Dateien doppelte Archivkopie entfernen. Vollständig entpackte Restbäume brauchen keine zweite Arbeitskopie. |
| Entpacken beginnt bei zu wenig Platz | Vorprüfung vor Extraktion; vorhandenes Archiv bleibt unangetastet. Tatsächliches ENOSPC ist weiterhin abhängig vom Linux-Dateisystemlabor. |
| Temporäre Dateien wachsen nach Abstürzen immer weiter | Unveröffentlichte Stagingreste unter exklusiver Dienstsperre beim Start aufräumen. |
| Abgebrochener Auftrag startet trotzdem; Anzeige bleibt bei Versuch 0 | Vor Hostaufruf Abbruch prüfen und aktuellen Versuch persistieren. |
| Aufbewahrung löscht einen benötigten Wiederherstellungsstand | 90-Stände-Test erhält neuesten Stand, Schutzmarkierung und Planreferenz. Erneute Referenzprüfung unter Sperre; Katalogfehler brechen Löschung ab. |
| Überfällige Sicherung wird nur im Browser bemerkt | Dauerhafte Überfälligkeits-/Erholungsmeldung bei eingerichtetem Ziel; Neustart erzeugt keine doppelte Vorfallmeldung. |
| Wartung scheitert, aber der Zeitplan verschluckt den Fehler | Fehler werden zurückgegeben, im Dienstlog erfasst und für Administratoren dauerhaft im Interface angezeigt. |
| Nachrichtentest meldet Erfolg, obwohl nichts gesendet wurde | Ziel erforderlich, Adressen vorab validiert, Zustellfehler dauerhaft für Administratoren sichtbar. |
| Dateiauswahl überschreitet das Helferprotokoll | Größe bereits bei Planung inklusive Base64/Inventar abschätzen und Ausführung blockieren. Einzeldateipläne aufteilen oder manuell exportieren. |
| Stromverlust lässt eine Änderung ohne dauerhaft gespeicherte Rollbackkopie zurück | Rollbackdateien, Metadaten und Verzeichnisse vor erster Änderung synchronisieren; anschließend Zieldatei und Elternverzeichnis synchronisieren. Reihenfolge im Helfertest geprüft, echter Stromverlust bleibt Laborfall. |
| Tausende Sicherungen/Aufträge machen die Tabelle unhandlich | Anzeige in 50er-Schritten; Statusantworten übertragen keine vollständigen Inventardetails jedes alten Plans. Vollständige Pläne werden beim Öffnen neu gelesen. Entfernte Hosts bleiben im Sicherungsfilter erreichbar. |

Aktueller lokaler Nachweis: 129 Go-Testfunktionen (eine Linux/systemd-Integration wird gesondert ausgeführt) mit Race Detector, Go Vet, zehn Python-Helfertests und sieben Installerprüfungen, 33 Browserabläufe, TypeScript-/Webbuild und Linux-Crossbuild für amd64/arm64. Browser: Desktop 1440×1000 und Mobilansicht 390×844; Downloads als Vollstand, Einzeldatei und Plan tatsächlich geprüft. Im eingebauten Browser wurde zusätzlich eine Einzeldatei heruntergeladen und die Oberfläche visuell kontrolliert.

Diese Prüfung kann unbekannte Kombinationen und echte Infrastrukturfehler nicht vollständig ausschließen. Noch offen sind reale SSH-/sudo-/SFTP-Installation, ein erzwungen voller Datenträger, physischer Stromverlust, Neustart-/Dienstprüfung nach Restore, Disk-/Storage-Migration, abweichende Hardware sowie isolierte Cluster-/Quorum-/HA-/Ceph-Szenarien. Gesamtrecovery bleibt bis zu dieser Abnahme manuell geführt und automatisch gesperrt.

## Ergänzende Bedienungsprüfung vom 06.10.2026

| Beobachteter Fehler | Korrektur |
|---|---|
| Dateiauswahl geht beim Wechsel zur Wiederherstellung verloren | Ausgewählte Datei ausdrücklich übernehmen; erneuter Einstieg erzeugt eine frische Anfrage. |
| Ladefehler sehen wie ein leerer oder dauerhaft ladender Dialog aus | Fehlermeldung und Wiederholen in Dateiliste, Vorschau, Wiederherstellung und Einstellungen. |
| Menüs werden am Tabellenrand abgeschnitten | Außerhalb des scrollenden Tabellenbereichs positionieren; Escape, Pfeiltasten und Fokus-Rückgabe. Fenstergrößenwechsel schließt das Menü ohne Laufzeitfehler. |
| Downloads und Dateien liegen mobil außerhalb des sichtbaren Bereichs | Sicherungszeilen mit sämtlichen Aktionen untereinander; mobile Dialogaktionen über die volle Breite. |
| Gleicher Stand lässt sich nach Änderung der Quelle mit sich selbst vergleichen | Identisches Vergleichsziel zurücksetzen und Vergleich sperren; Änderungen deutsch beschriften. |
| Offene Einstellungen gehen beim Navigieren verloren | Speicherzustand anzeigen, unverändertes Speichern sperren, Seitenwechsel und Browser-Neuladen mit offenen Änderungen schützen. Verbindungstest erst nach Speichern. |
| Pausierte Hosts werden als vollständig gesichert dargestellt | Gesonderte Zustände für pausierte Zeitpläne und noch nicht eingerichtete Hosts. |
| Laufende Planerstellung lässt sich schließen oder erneut starten | Dialog und Formulare während der Anfrage sperren; Fehler im bestehenden Formular anzeigen. |
| Abmelden bei Netzfehler erzeugt einen unbehandelten Fehler | Sitzungsansicht behalten und Verbindungsfehler anzeigen. |

Zwölf zusätzliche Browserregressionen wurden zunächst gegen den alten Stand mit dem jeweiligen Fehler beobachtet und nach der Korrektur erfolgreich ausgeführt. Visuelle Prüfung auf Desktop und bei 390×844 Pixeln; keine Behauptung einer vollständigen WCAG-Abnahme oder einer realen Hardwareabnahme.

## Anmeldung und Updates

| Fall | Verhalten und Nachweis |
|---|---|
| Neustart oder Update während einer gültigen Anmeldung | Gehashte Sitzungen im Katalog, mit echtem Schließen und Wiederöffnen des Speichers geprüft. |
| Passwort, Rolle oder Secretfreigabe geändert | Betroffene Sitzungen werden zusammen mit der Änderung atomar widerrufen. |
| Zwei Administratoren gleichzeitig herabgestuft | Einer bleibt aktiv; nebenläufiger Test vorhanden. |
| Manipuliertes oder falsch zugeordnetes Release | Signatur, Tag, Format, Plattform, Größe und SHA-256 werden vor dem Austausch geprüft. |
| Update bei aktivem Auftrag | Installation wird abgewiesen. Neue Aufträge und automatische Wartung bleiben in der Startprüfung gesperrt. |
| Neue Binärdatei lässt sich nicht ausführen | Separate bekannte Updater-Binärdatei bleibt startfähig; Wiederherstellung aus einem neuen Testprozess geprüft. |
| Abgebrochener Katalogaustausch mit SQLite-WAL | Wiederherstellung übernimmt auch im WAL festgeschriebene Änderungen; subprocessbasierter SQLite-Test. |
| Web-Port belegt oder TLS-Dateien ungültig | Start meldet keine lokale Bereitschaft, bevor TCP-Bindung und Zertifikatprüfung erfolgreich waren. |

Der neue Linux/systemd-Prüflauf ist gesondert vom lokalen Nachweis; ein echter Reboot-/Stromausfalltest bleibt offen. Anleitung und bekannte Grenzen stehen in [UPDATES.md](UPDATES.md). Der Quellcode ist öffentlich auf GitHub veröffentlicht. Signierte Produktivreleases sind mit der im Projekt hinterlegten öffentlichen Signieridentität veröffentlicht; [Releases](https://github.com/jahartmann/Anker/releases).

Linux/systemd-Nachweis vom 06.10.2026: [Prüflauf](https://github.com/jahartmann/Anker/actions/runs/37441649633). Der geführte Installer einschließlich wiederholter Einrichtung, Programmwechsel mit erhaltener Anmeldung, fehlerhafter Kandidat, unterbrochene Journale mit und ohne Katalogsnapshot sowie echte Dienstneustarts wurden erfolgreich ausgeführt. Dies ist kein physischer Reboot-/Stromausfalltest und keine Proxmox-Hostabnahme.

## Speicherprüfung vom 06.10.2026

Neue Regressionen prüfen gemeinsame Dateisysteme, eingebundene Backup-Unterordner und Bind-Aliase, unerreichbare Mounts, fehlende Geräteidentität, Gerätetausch zwischen Bestätigung und Ausführung, abgelehnte/geänderte Pläne, fehlgeschlagene Journalpersistenz, unterbrochene Vorgänge sowie Container-/Schreibschutzgrenzen. XFS-Geometrie und Befehlsziel sind mit Fixtures geprüft. Browserabläufe decken Demoisolation, Verbrauch/Verlauf, Bestätigung, LXC-Befehl, fehlenden Hilfsdienst, Aktualisierungs-/Pollingfehler und Anleitungsdownload ab.

Die echte ext4-Erweiterung auf einem ausschließlich für den Test angelegten Loopgerät ist im [Linux-Prüflauf](https://github.com/jahartmann/Anker/actions/runs/37463965688) bestanden: 64 auf 128 MiB über den gehärteten systemd-Hilfsdienst, vorhandene Datei und Webdienstprozess bleiben erhalten. Derselbe Lauf besteht die vollständigen Go-/Python-/Browserprüfungen sowie bestehende Installations-, TLS- und Updatefälle. Weder Produktionsdatenträger noch Proxmox-Hosts werden dabei verwendet. Reales XFS, LXC-Quoten, LVM, Stromverlust und Laufwerksumzug bleiben eigene Abnahmefälle.

Die abschließende lokale Prüfung umfasst 39 Browserabläufe und die vollständige Go-Suite mit Race Detector und Vet, zehn Python-Helfertests und sieben Installerprüfungen. Nach einer kleinen Diagrammkorrektur wurden die sechs Speicherabläufe erneut bestanden. Desktop 1440×1000, Mobilansicht 390×844 und Erweiterungsdialog sind visuell geprüft; kein horizontaler Seitenüberlauf oder Browserlaufzeitfehler. Ein vorübergehend fehlgeschlagener Mount erhält die vorherigen gültigen Messpunkte und ergänzt keine erfundenen Werte.

## Aufträge und Zeitplanprüfung vom 06.10.2026

| Gefundener Fehler | Korrektur und Regression |
|---|---|
| Aufträge starten trotz fehlgeschlagenem Tagesmarker oder verschwinden für diesen Tag bei fehlgeschlagenem Job-Insert | Auftrag und Marker in einer Transaktion; Fehler an beiden Schreibstellen gezielt ausgelöst und nächsten Tick geprüft. |
| Defekte Marker werden ignoriert | Kein Lauf für den betroffenen Host; Scheduler-/Wartungsfehler im Dienstlog und Interface. |
| Wartende Aufträge verwenden alte Parallelität | Einstellungen vor jeder Zulassung neu lesen und mit Einstellungsänderungen synchronisieren. |
| Ungültige Einstellungen ergeben einen falschen Erfolg | Worker beendet sich mit erklärtem Fehler vor dem Hostzugriff, auch bei ungültiger Wiederholungszahl. |
| Host verschwindet zwischen Auftragsprüfung und Start | Entfernen und neue Einplanung gemeinsam sperren; unlesbare Auftragsliste verhindert Entfernen. |
| Hostzeitplanänderung überschneidet sich mit Einplanung | Hoständerung und Zulassung gemeinsam ordnen. |
| Historieneinträge haben keinen Detail-/Löschpfad | Detail-API, Webdialog und CLI; aktive Einträge schützen, Sicherung und Tagesmarker nach Löschung prüfen. |
| Erneuter Start könnte einen alten Restore ungeprüft wiederholen | Nur Sicherung/Hostprüfung wiederholen; Restore über neuen geprüften Plan. Leser dürfen keine Aufträge verändern, Historienlöschung nur durch Admin. |
| Abbruch wird mehrfach gesendet oder schon als beendet angezeigt | Knopf während Anfrage und nach Annahme sperren; Endstatus kommt vom Worker. |
| Verzögerte Listenantwort oder alte Liste verdeckt aktuellen Zustand | Alte Statusantworten verwerfen und den fortgeschrittenen Auftragszustand zwischen Detail- und Listenantwort erhalten. |

Geprüft sind die echten API-Verbindungen für Einstellungen/Host-CRUD/Scheduler und Auftragsaktionen, Neustart ohne doppelten Tageslauf, Datenbankfehler sowie Browseraktionen. Die Browserprüfung nutzt einen isolierten Demo- und einen leeren Produktionsdienst; UI-Fehlerfälle werden zusätzlich gezielt simuliert. Die Go-Integration verwendet einen lokalen Test-Collector statt realer SSH-Hosts. Dies ersetzt keine Abnahme mit euren Hosts oder einen physischen Reboot-/Stromausfalltest.

Abschließender lokaler Nachweis: 46 Browserabläufe, vollständige Go-Suite mit Race Detector, Go Vet, zehn Python-Helfertests und sieben Installerprüfungen. Webbuild und Linux-Crossbuild sind bestanden. Die unabhängige Codeprüfung hat nach Korrektur der gefundenen Konkurrenzfehler keine offenen Befunde.

## Automatische Hostanbindung vom 07.10.2026

[Linux-Prüflauf](https://github.com/jahartmann/Anker/actions/runs/37597961722): Anbindung über die laufende Web-API und den root-Dienst an einen echten OpenSSH-Server im isolierten Debian-13-Container. Falsche Passwörter registrieren keinen Host und ändern keine Vertrauensdatei. Root-Anbindung installiert fehlendes sudo, beide Rollen werden mit getrennten Schlüsseln geprüft, erneute Anbindung über Passwort-sudo behält Host-ID, Profile und fremde autorisierte Schlüssel. Die Backuprolle kann keine Schreiboperation ausführen. Vorhandene zentrale Schlüssel bleiben im Besitz des Dienstbenutzers mit 0600; Verzeichnisse und verwaltete Vertrauensdatei bleiben root-kontrolliert. Passwortfreiheit im Katalog nachgewiesen.

Der Zielcontainer verwendet eine ausdrücklich gefälschte `pveversion` zur Prüfung des Installations-/SSH-Wegs, keine reale Proxmox-Installation. SSH-Identitätswechsel, PAM-Passwortprompt, Abbruch, Ausgabegrenzen und unsichere Schlüsseldateien haben zusätzliche Go-Regressionen. 55 Browserfälle und echte Terminal-PTY-Prüfung bestanden.
