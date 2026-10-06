# Implementierung

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
