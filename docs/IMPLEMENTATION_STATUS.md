# Implementierung

Anker besteht aus einem Go-Dienst mit eingebetteter React-Oberfläche, SQLite-Katalog, einem festen Python-Helfer auf dem Proxmox-Host, einem Unix-Socket-CLI und einer Bubble-Tea-Terminaloberfläche. Alle lokalen Demoabläufe sind isoliert; es wurde kein Produktionshost kontaktiert.

Die ursprüngliche visuelle Richtung wurde nach Nutzerfeedback ersetzt. Referenz: `docs/design/anker-hosts-concept.png`. Die Umsetzung verwendet einen weißen Inhaltsbereich, eine hellgraue Navigation ohne dekorative Symbole, schlichte Tabellen, eine gemeinsame Typografie-/Abstandsskala und zurückhaltende Dialoge. Statuszahlen kommen aus dem Dienst. Suchzustände, Fokus, Formularfehler, Mobilnavigation und Secretvorschauen sind eigene Zustände desselben Systems.

Abweichungen vom Konzept: `tar.gz` statt `tar.zst` für portable Standardwerkzeuge; manuell geführte Gesamtrecovery bis zur realen Laborabnahme; keine automatische Entscheidung über Disk-/Storage-Neuanlage; Secretfreigabe separat von Rollen. Terminalformulare können über den gemeinsamen Befehlseingang bedient werden. Details und Grenzen stehen in `SUPPORT.md` und `OPERATIONS.md`.

Abschluss: Go-Tests einschließlich Race Detector und Vet, 99 Go-Testfunktionen (davon eine gesonderte Linux/systemd-Integration), zehn Python-Helfertests und drei Installerprüfungen, 30 Browserabläufe, Typecheck/Webbuild sowie Linux-Crossbuild für amd64 und arm64. Der unabhängige Gesamtprüfer fand Fehler; die Korrekturen und zugehörigen Regressionen stehen in `REVIEW.md`. Desktop, Dialoge und Mobilansicht wurden zusätzlich im eingebauten Browser geprüft. Langzeit-/SFTP-/Hardwareabnahme wird durch lokale Fixturetests nicht ersetzt.

Bedienungsprüfung vom 06.10.2026: vorausgewählte Einzeldatei in Wiederherstellungsplänen, wiederholbare Ladefehler, vollständig sichtbare Aktionsmenüs, mobile Sicherungsaktionen ohne horizontales Scrollen und sichtbarer Speicherzustand mit Schutz offener Einstellungen. Zwölf zusätzliche Browserregressionen decken die gefundenen Fehler ab. Bestehende Schutzgrenzen der Wiederherstellung bleiben erhalten.

## Anmeldung und Release-Verteilung — 6. Oktober 2026

Pflichtanmeldung mit persistenten, gehashten Sitzungsschlüsseln; Standardlaufzeit 30 Tage. Benutzer können gesperrt, bearbeitet und gelöscht werden. Passwort- und Rechteänderungen sowie expliziter Sitzungswiderruf werden im Katalog atomar gespeichert. Der letzte aktive Administrator und der eigene Administratorzugang bleiben geschützt.

Der Updateweg nutzt signierte GitHub-Releases und einen separaten root-eigenen Updater. Binärdatei und Katalog/WAL werden gesichert; Fehler beim Start und offene Updatejournale führen zum Rückfall. Die bewährte Updater-Binärdatei bleibt bis zum erfolgreichen Start der neuen Hauptversion erhalten. Web-Port und TLS-Dateien werden geprüft, bevor der lokale Dienst Bereitschaft meldet.

Eine unabhängige Prüfung fand drei Start-/Unterbrechungsfälle, die korrigiert und durch zusätzliche Prüfungen abgedeckt wurden. Keine verbleibenden konkreten P1/P2-Befunde im Nachreview. Verifiziert: vollständige lokale Go-/Browser-/Python-Prüfungen, signierte Release-Pakete für beide Linux-Architekturen und deren Paketprüfung mit OpenSSL. Der Quellcode ist öffentlich auf GitHub veröffentlicht. Der echte Linux-/systemd-Prüflauf ist bestanden; die physische Reboot-/Stromausfallabnahme bleibt offen. Die lokalen Pakete wurden mit einem verworfenen Testschlüssel erstellt und sind keine produktiven Releases.

## Geführte Installation und Neustartprüfung

Ein verifizierter Release-Installer übernimmt Download, Paketprüfung und die interaktive Ersteinrichtung. Benutzer und private SSH-Schlüssel bleiben bei Wiederholung erhalten; fehlende öffentliche Schlüssel und Dateirechte werden repariert. Die Ersteinrichtung schließt konkurrierende Updates aus. Neue TLS-Dateien und Token werden vor der Konfigurationsumschaltung in eigenen Dateien gespeichert. Die Erstinstallation hat einen dauerhaften Marker und übernimmt das Hauptprogramm zuletzt.

Die Wartungsfreigabe wartet auf den laufenden Dienst, bevor sie den dauerhaften Marker entfernt. Eine fehlgeschlagene Socketverbindung bleibt dadurch wiederholbar. Ein zusätzlicher GitHub-Prüflauf verwendet echte Linux-/systemd-Dienste für Einrichtung, Programmwechsel, fehlerhafte Kandidaten, Katalogrückfall, Journal-Wiederanlauf und Dienstneustarts. Private GitHub-Releases werden über einen eigenen Lese-Token unterstützt; ein Ende-zu-Ende-Test an einem privaten Repository steht noch aus. Ein Dienstneustart ersetzt keinen physischen Reboot-/Stromausfalltest.

Linux-Nachweis vom 06.10.2026: [GitHub-Prüflauf](https://github.com/jahartmann/Anker/actions/runs/37441649633). Interaktive Erst- und Wiederholeinrichtung, Reparatur einer unvollständigen Installation, unveränderte SSH-Schlüssel, echte systemd-Programmwechsel, Rückfall bei nicht ausführbarem Kandidaten, Journal-Wiederherstellung mit und ohne Katalogsnapshot, gespeicherte Anmeldung und Dienstneustarts sind bestanden.
