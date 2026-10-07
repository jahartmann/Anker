# Implementierung

## Abgleich mit dem Konzept vom 7. Oktober 2026

Die Grundlage für den täglichen Betrieb ist umgesetzt. Die ursprünglich geplante umfassende Wiederherstellung ist noch nicht vollständig umgesetzt oder an echten Proxmox-Hosts abgenommen. Die folgende Tabelle trennt vorhandene Funktionen von offenen Arbeitspaketen; die späteren Abschnitte dokumentieren frühere Prüfstände.

| Bereich | Vorhanden | Noch offen oder begrenzt |
|---|---|---|
| Zentrale Hostverwaltung | Stabile Host-IDs, Gruppen, tägliche Zeitpläne, Aufträge und begrenzte Parallelität | Rund 40 Hosts bisher mit synthetischen Daten geprüft |
| Cluster | Node-Konfigurationen und Clusterinventar werden erfasst | Eigener Clusterkatalog, abgestimmte Stände mehrerer Nodes und koordinierter Cluster-Restore |
| Konfiguration und Geheimnisse | `/etc`, `/usr/local`, pmxcfs-Datenbanksnapshot, Originaldateien, Metadaten und Abhängigkeitshinweise | Zusätzliche Pfade werden im root-eigenen Hostprofil eingerichtet; das zentrale Feld prüft Vollständigkeit. Inventarabfragen melden Erfolg, nicht anwendbar oder Fehler; fehlende Pflichtabfragen sperren die Planung. Versionsprüfung des Decoders und echte pmxcfs-Abnahme fehlen |
| Ablage, Archiv und Integrität | Lesbare Stände, Prüfsummen, Archivierung, erneutes Öffnen, Wiederindexierung | Reale Infrastruktur-, Platzmangel- und Stromausfallabnahme |
| Downloads | Einzeldatei, vollständiger Stand und Planpaket mit vorbereiteten Dateien sowie `original/original-files/` und Originalmetadaten | Einzeldateien bis 64 MiB; kein eigener Download beliebiger Dateiauswahlen; vorhandene Pläne können eine ältere Anleitung enthalten |
| Einzeldatei-Restore | Geprüfter Plan, Inhalts-/Metadaten-/Identitätsprüfung, dauerhaftes Hostjournal, kontrollierte Rücksetzung und Statusabgleich | Freigegebene Dateitypen; Links, Extended Attributes und sensible Systemkonfigurationen manuell. Abweichende Benutzer-/Gruppenidentitäten bleiben manuell; Dienst- und Rebootabnahme fehlen |
| Neue Hardware und vollständiger Host | Szenario, Netzwerkportzuordnung, Voraussetzungen und manuelle Pläne/Exporte | Regelwerk für Storage, Hostidentität, Boot und Gesamtausführung. Storage-/Hostname-/Adressentscheidungen werden manuell dokumentiert, nicht automatisch angewendet |
| Netzwerk-Rückweg | Nur benötigte physische Ports, Abhängigkeiten für Bridges/Bonds/VLANs/Aliase, manuell vorbereitete Netzwerkdateien | Kein lokaler Wächter mit automatischer Rücksetzung nach ausbleibender Netzwerkbestätigung; Automatisches Schreiben/Aktivieren gesperrt; Erreichbarkeit und Dienste nach manueller Übernahme prüfen |
| Cluster- und Versionswechsel | Szenarien und manuelle Voraussetzungen | Validierte Ablaufregeln und automatisierte Ausführung |
| Web, Terminal und CLI | Gemeinsamer Dienst; Hostanbindung, Sicherungen, Pläne, Aufträge und Systemverwaltung | Terminal ohne Sicherungsvergleich und Inhaltsvorschau; lokale Socketverwaltung hat administrative Rechte und verwendet keine Webbenutzersitzung |
| SSH-Einrichtung | Benutzer/Passwort, bestätigter Fingerprint, begrenzte getrennte Schlüssel und Helferinstallation | Hosthelfer werden nicht automatisch durch ein zentrales OTA-Update ersetzt |
| SFTP | Optionales Skript für einen lesenden Exportzugang | Einrichtung und persistente Mounts durch Administrator; keine benutzerbezogene Filterung je Host, reale SFTP-Abnahme offen |
| Benutzer und Anmeldung | Persistierte Anmeldung, Rollen, Sperren, Sitzungswiderruf, 30 Tage Standard und einstellbare Passwortlänge | Vorhanden und mit Fehlerfällen geprüft |
| TLS | Eigene Zertifikate, automatische Erneuerung, Einstellungen und Übernahme ohne Dienstneustart | Browservertrauen bleibt bei selbstsignierten Zertifikaten manuell |
| Installation und Updates | Clone/Installer, Wiederholung und Abbruchbehandlung, signierte Pakete, Katalog-/Programm-Rückfall, automatischer Webreload | Dienstneustarts unter Linux geprüft; physischer Reboot und Stromverlust nicht abgenommen |
| Speicher | Belegung, Verlauf, bedingte Prognose, begrenzte Erweiterung vorhandener ext4-/XFS-Kapazität | LXC-Zuweisung, Partitionierung, LVM und Laufwerksumzug manuell; echte ext4-Prüfung vorhanden, weitere Plattformabnahmen offen |
| Anker-Betriebskonfiguration | Katalog, Einstellungen und `/etc/anker` sind über die Dateisystem-/Betriebsanleitung sicherbar | Kein eigener vollständiger Export-/Importassistent für die zentrale Anker-Installation |
| Produktion und Verteilung | Leerer Produktionsstart, ausdrücklich getrennte Demo, öffentliches GitHub und MIT | Vorhanden und geprüft |

Für den nächsten Ausbau haben Wiederherstellung und die Vollständigkeit des Inventars Vorrang: Zieländerungen vollständig inhaltlich vergleichen und die Standalone-/Hardwarefälle im isolierten Proxmox-Labor abnehmen. Danach folgen Storage-/Identitätsregeln und koordinierte Clusterabläufe. Ein allgemeiner Status „automatisch vollständig wiederherstellbar“ wird bis zu dieser Abnahme nicht vergeben.

Die Bedienungsprüfung dieses Stands korrigiert erhaltene Bestätigungen nach Quell-/Zielwechsel, lange Wiederherstellungslisten, fehlende Zielportauswahl nach einer frischen Prüfung und unvollständige Originalmetadaten im Planexport. Im Terminal sind sämtliche Szenarien auswählbar; `Ctrl+E` wechselt zwischen Dateiliste und direkter Pfadeingabe.

Anker besteht aus einem Go-Dienst mit eingebetteter React-Oberfläche, SQLite-Katalog, einem festen Python-Helfer auf dem Proxmox-Host, einem Unix-Socket-CLI und einer Bubble-Tea-Terminaloberfläche. Alle lokalen Demoabläufe sind isoliert; es wurde kein Produktionshost kontaktiert.

Die ursprüngliche visuelle Richtung wurde nach Nutzerfeedback ersetzt. Referenz: `docs/design/anker-hosts-concept.png`. Die Umsetzung verwendet einen weißen Inhaltsbereich, eine hellgraue Navigation ohne dekorative Symbole, schlichte Tabellen, eine gemeinsame Typografie-/Abstandsskala und zurückhaltende Dialoge. Statuszahlen kommen aus dem Dienst. Suchzustände, Fokus, Formularfehler, Mobilnavigation und Secretvorschauen sind eigene Zustände desselben Systems.

Abweichungen vom Konzept: `tar.gz` statt `tar.zst` für portable Standardwerkzeuge; manuell geführte Gesamtrecovery bis zur realen Laborabnahme; keine automatische Entscheidung über Disk-/Storage-Neuanlage; Secretfreigabe separat von Rollen. Terminalformulare können über den gemeinsamen Befehlseingang bedient werden. Details und Grenzen stehen in `SUPPORT.md` und `OPERATIONS.md`.

Abschluss: Go-Tests einschließlich Race Detector und Vet, 129 Go-Testfunktionen (davon eine gesonderte Linux/systemd-Integration), zehn Python-Helfertests und sieben Installerprüfungen, 33 Browserabläufe, Typecheck/Webbuild sowie Linux-Crossbuild für amd64 und arm64. Der unabhängige Gesamtprüfer fand Fehler; die Korrekturen und zugehörigen Regressionen stehen in `REVIEW.md`. Desktop, Dialoge und Mobilansicht wurden zusätzlich im eingebauten Browser geprüft. Langzeit-/SFTP-/Hardwareabnahme wird durch lokale Fixturetests nicht ersetzt.

Bedienungsprüfung vom 06.10.2026: vorausgewählte Einzeldatei in Wiederherstellungsplänen, wiederholbare Ladefehler, vollständig sichtbare Aktionsmenüs, mobile Sicherungsaktionen ohne horizontales Scrollen und sichtbarer Speicherzustand mit Schutz offener Einstellungen. Zwölf zusätzliche Browserregressionen decken die gefundenen Fehler ab. Bestehende Schutzgrenzen der Wiederherstellung bleiben erhalten.

## Anmeldung und Release-Verteilung — 6. Oktober 2026

Pflichtanmeldung mit persistenten, gehashten Sitzungsschlüsseln; Standardlaufzeit 30 Tage. Benutzer können gesperrt, bearbeitet und gelöscht werden. Passwort- und Rechteänderungen sowie expliziter Sitzungswiderruf werden im Katalog atomar gespeichert. Der letzte aktive Administrator und der eigene Administratorzugang bleiben geschützt.

Der Updateweg nutzt signierte GitHub-Releases und einen separaten root-eigenen Updater. Binärdatei und Katalog/WAL werden gesichert; Fehler beim Start und offene Updatejournale führen zum Rückfall. Die bewährte Updater-Binärdatei bleibt bis zum erfolgreichen Start der neuen Hauptversion erhalten. Web-Port und TLS-Dateien werden geprüft, bevor der lokale Dienst Bereitschaft meldet.

Eine unabhängige Prüfung fand drei Start-/Unterbrechungsfälle, die korrigiert und durch zusätzliche Prüfungen abgedeckt wurden. Keine verbleibenden konkreten P1/P2-Befunde im Nachreview. Verifiziert: vollständige lokale Go-/Browser-/Python-Prüfungen, signierte Release-Pakete für beide Linux-Architekturen und deren Paketprüfung mit OpenSSL. Der Quellcode ist öffentlich auf GitHub veröffentlicht. Der echte Linux-/systemd-Prüflauf ist bestanden; die physische Reboot-/Stromausfallabnahme bleibt offen. Die lokalen Pakete wurden mit einem verworfenen Testschlüssel erstellt und sind keine produktiven Releases.

## Geführte Installation und Neustartprüfung

Ein verifizierter Release-Installer übernimmt Download, Paketprüfung und die interaktive Ersteinrichtung. Benutzer und private SSH-Schlüssel bleiben bei Wiederholung erhalten; fehlende öffentliche Schlüssel und Dateirechte werden repariert. Die Ersteinrichtung schließt konkurrierende Updates aus. Neue TLS-Dateien und Token werden vor der Konfigurationsumschaltung in eigenen Dateien gespeichert. Die Erstinstallation hat einen dauerhaften Marker und übernimmt das Hauptprogramm zuletzt.

Die Wartungsfreigabe wartet auf den laufenden Dienst, bevor sie den dauerhaften Marker entfernt. Eine fehlgeschlagene Socketverbindung bleibt dadurch wiederholbar. Ein zusätzlicher GitHub-Prüflauf verwendet echte Linux-/systemd-Dienste für Einrichtung, Programmwechsel, fehlerhafte Kandidaten, Katalogrückfall, Journal-Wiederanlauf und Dienstneustarts. Private GitHub-Releases werden über einen eigenen Lese-Token unterstützt; ein Ende-zu-Ende-Test an einem privaten Repository steht noch aus. Ein Dienstneustart ersetzt keinen physischen Reboot-/Stromausfalltest.

Linux-Nachweis vom 06.10.2026: [GitHub-Prüflauf](https://github.com/jahartmann/Anker/actions/runs/37441649633). Interaktive Erst- und Wiederholeinrichtung, Reparatur einer unvollständigen Installation, unveränderte SSH-Schlüssel, echte systemd-Programmwechsel, Rückfall bei nicht ausführbarem Kandidaten, Journal-Wiederherstellung mit und ohne Katalogsnapshot, gespeicherte Anmeldung und Dienstneustarts sind bestanden.

## Produktion und optionale Demo

Die Standardinstallation enthält keine Beispielhosts, Sicherungen, Aufträge, Pläne oder Demobenutzer. Ein leerer Produktionsstart und die Anmeldung mit einem eigenen Administrator werden zusätzlich im Browser geprüft. Der bekannte Demozugang wird im Produktionsbetrieb abgewiesen.

Nur der ausdrückliche Befehl `demo` erzeugt Beispieldaten. Die persistente Betriebsart sperrt das Vermischen der Datenverzeichnisse bereits vor dem Öffnen des Katalogs; unmarkierte frühere Demoordner werden nicht als Produktion übernommen. Demo-Webzugriff bleibt lokal, der Socket im eigenen Ordner. Regressionen decken Moduswechsel, belegte Ordner, fehlerhafte Marker, Umgebungsvariablen und Pfadverweise ab. Release-Pakete enthalten keine Testfixtures oder Designentwürfe.

## Vereinfachte Einrichtung und Konfiguration

Der Assistent erkennt vorhandene Administratoren lesend und fragt Zugangsdaten nur bei der Ersteinrichtung ab. Die Passwortabfrage erfolgt vor dem Stoppen der Dienste. Bereits vertraute Updatequelle und Signierschlüssel werden automatisch übernommen; Änderungen bleiben über `setup --updates` erreichbar. HTTPS kann ein eigenes Zertifikat für DNS oder IP erzeugen oder vorhandene CA-Dateien importieren. Adresse, Gültigkeit und Schlüsselpaar werden geprüft; Wiederholung erhält die gültige Identität. Eigenes TLS gilt ein Jahr; die optionale automatische Erneuerung und ihre Grenzen sind im folgenden Abschnitt beschrieben. SSH-Tunnel bleibt ohne zusätzliche TLS-Einrichtung möglich.

Neue Hosts verwenden die eingerichteten Sicherungs-/known_hosts-Pfade. Der Hostinstaller übernimmt öffentliche Schlüssel mit festem Helferkommando. Wiederholte Einzelrollenänderungen prüfen auch die bereits installierten Schlüssel der anderen Rolle unter gemeinsamer Sperre. `host trust` schreibt nur einen unabhängig bestätigten SSH-Hostschlüssel; gemeinsam genutzte Aliase und Authority-Einträge erfordern eine manuelle Trennung. Lokale Regressionen prüfen fehlende TLS-Dateien, Zertifikatsidentität, Schlüsseltrennung und atomare Hostschlüsseleinträge. Der Linux-Prüfablauf wurde um echte HTTPS-Anmeldung und wiederholte TLS-Einrichtung erweitert.

## Automatische Zertifikatserneuerung

Die root-eigene Zertifikatsverwaltung verwendet die bestehende Updater-Grenze, auch ohne GitHub-Updatequelle. Automatik prüft beim Start und alle sechs Stunden; 30 Tage Vorlauf sind für neue selbst erzeugte Zertifikate voreingestellt, 7 bis 90 Tage sind konfigurierbar. Nur vom Assistenten ausdrücklich registrierte Zertifikate mit passendem Public-Key-Hash und festen verwalteten Pfaden werden erneuert. Importierte Zertifikate bleiben unverändert, einschließlich selbstsignierter Importe.

Web, CLI und Terminalbefehle zeigen Ablauf/Fingerprint, erlauben öffentliche Downloads und administrative Erneuerung. Das Zertifikat wird nach vorheriger Prüfung atomar ersetzt; derselbe private Schlüssel verhindert einen inkonsistenten Paarwechsel. Vorheriges öffentliches Zertifikat und Erneuerungszustand bleiben gespeichert. Der Weblistener übernimmt gültige neue Paare für DNS- und IP-Verbindungen ohne Neustart und behält bei defekten Dateien das letzte gültige Paar. Setup und Updates schließen konkurrierende Änderungen aus. Die Demo kann keine Produktivzertifikate bearbeiten.

Browservertrauen bleibt separat: selbstsignierte Zertifikate können nach Erneuerung eine neue manuelle Freigabe brauchen. Die Erweiterung erzeugt keine eigene CA und verspricht keine warnungsfreie Erneuerung auf den Clients. Der Linux-Prüfablauf testet ältere Unit-Konfigurationen, fehlende Updatequelle, manuelle und automatische Erneuerung, unveränderten Schlüssel/Prozess, HTTPS sowie persistierte Einstellungen.

## Speicherverwaltung für Anker

Der neue administrative Speicherbereich zeigt tatsächliche Dateisystemwerte, Inodes, gemeinsame Mounts, Backup-Bindmounts und bis zu 90 Tage stündlicher Messung. Prognosen verwenden belastbare Tagestrends über maximal 14 Tage; unzureichende, veraltete oder schwankende Messungen erzeugen kein Fülldatum. Messungen laufen getrennt vom Backup-Zeitplan, blockierte Dateisystemabfragen werden begrenzt.

Die bestehende root-Grenze erlaubt ausschließlich eine bestätigte ext4-/XFS-Dateisystemerweiterung auf bereits zugewiesenem Geräteplatz. UUID, Mount-/Gerätezuordnung und Geometrie werden vor der Änderung erneut geprüft. Dauerhaftes Operationsjournal, gemeinsame Setup-/Update-Sperren und unterbrochene Zustände verhindern einen falschen Erfolgsstatus oder automatischen Wiederholungsversuch. Container bleiben bei Hostanweisungen; Laufwerksumzug, Formatierung und Partition-/LVM-Änderungen sind manuell. Anleitungen lassen sich herunterladen.

Die unabhängige Prüfung fand eine falsche Bind-Alias-Zuordnung und eine fehlende Identitätsanforderung. Beide sind korrigiert und durch Regressionen abgedeckt. Lokal bestanden: vollständige Go-Prüfung mit Race Detector, Go Vet, Webbuild und 39 Browserabläufe. Eine echte ext4-Erweiterung über den systemd-Hilfsdienst ist zusätzlich in der isolierten [Linux-CI bestanden](https://github.com/jahartmann/Anker/actions/runs/37463965688): 64 auf 128 MiB, vorhandene Datei und laufender Webprozess bleiben erhalten. Die Oberfläche wurde auf Desktop und Mobilansicht geprüft; die letzte Diagrammkorrektur und der Erhalt gültiger Messpunkte bei vorübergehendem Mountausfall sind nachgeprüft. Reales Proxmox-LXC, LVM, XFS und Umzug sind noch nicht abgenommen.

## Aufträge, Zeitplan und CRUD

Anker verwendet einen täglichen internen Scheduler mit minütlicher Prüfung. Einstellungen und Hostformular sind direkt mit dessen Katalog verbunden; keine zusätzliche Crontab ist nötig. Zeitplan und Tagesmarker werden atomar eingeplant. Host-/Einstellungsänderungen und die Zulassung wartender Aufträge sind synchronisiert. Fehlerhafte Katalogwerte erzeugen keinen stillen Lauf mit Ersatzwerten.

Web und CLI zeigen Auftragsdetails mit Auslöser und tatsächlichem Start, erlauben kooperativen Abbruch und ausdrücklich erneutes Starten von Sicherung/Hostprüfung. Administratives Entfernen ist auf abgeschlossene Historieneinträge begrenzt und erhält Sicherungen sowie Tagesmarker. Wiederherstellungen durchlaufen erneut die Planprüfung. Regressionen prüfen Berechtigungen, Schreibfehler, Parallelitätsänderung, pausierten/eigenen Zeitplan, Neustart und konkurrierende Hostlöschung. Browserabläufe prüfen tatsächliche API-Verbindungen, Ergebnisse und geschützte UI-Zustände; verspätete Antworten können aktuellere Zustände nicht überschreiben. Die unabhängige Prüfung hat zwei zusätzliche Konkurrenz-/Darstellungsfehler gefunden; beide sind mit zuerst fehlschlagenden Regressionen korrigiert.

Reale SSH-Ausführung, Proxmox-Hostwirkungen und Stromverlust bleiben getrennte Abnahmefälle. Der tägliche Scheduler bildet keine frei editierbaren Cronausdrücke oder beliebige Intervalle ab.

Abschließender lokaler Nachweis: 46 Browserabläufe, vollständige Go-Suite mit Race Detector, Go Vet, zehn Python-Helfertests und sieben Installerprüfungen. Webbuild und Linux-Crossbuild sind bestanden. Die unabhängige Codeprüfung hat nach Korrektur der gefundenen Konkurrenzfehler keine offenen Befunde.
