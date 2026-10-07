# Webupdates und Terminalverwaltung

> Umsetzung gemäß dem ausdrücklich bestätigten Nutzerauftrag: einfache Webupdates, Handbuch und geführte SSH-Verwaltung. Parallel getrennte Dateibereiche bearbeiten, danach gemeinsam prüfen.

**Ziel:** Updatequelle über Web einrichten und geprüfte Releases installieren; mit `anker` eine geführte Terminaloberfläche öffnen; `anker -help` und `man anker` dokumentieren den Betrieb.

**Architektur:** Bestehenden root-eigenen Updatehelfer und gemeinsame Dienst-API erweitern. Der Anker-Schlüssel wird mit der vertrauten Anwendung geliefert; eigene Quellen benötigen einen ausdrücklich bestätigten öffentlichen Schlüssel. Keine ungeprüften Binärdateien installieren. Bubble Tea bleibt die Terminalbibliothek.

## Vorgaben

- Kein privater Signierschlüssel, GitHub-Token oder Beispielkonto in Git.
- Updatekonfiguration nur für Administratoren, mit Herkunftsprüfung, Validierung, fester Konfigurationsdatei und Sperre gegen parallele Einrichtung/Updates.
- Änderungen sofort ohne Dienstneustart anwenden; Fehler behalten die vorherige Quelle. Demobetrieb kann keine Systemkonfiguration ändern.
- Bestehende Sicherungen, Benutzer, TLS-/SSH-Schlüssel und Sitzungen erhalten.
- Terminal: Maus und Tastatur, kompakte Ansichten, klare Bestätigungen, keine rohe JSON-Bedienung oder notwendige Befehlszeile für die Kernaktionen. Fenstergröße und Fehler berücksichtigen.

## Aufgaben

- [x] Updatequelle: GET/POST über Administrator-API und root-Helfer, offizieller eingebetteter Schlüssel, Fingerprintübersicht, Webformular und Bestätigung. Regressionen für Rolle, CSRF, falsche Schlüssel, Parallelität, Persistenz und Demo. Bestehende Updateprüfung/-installation unverändert signiert.
- [x] Terminal: vorhandene fünf Ansichten in geführte Verwaltung ausbauen; Hosts anbinden/prüfen/pausieren, Sicherungen starten/prüfen/exportieren, Wiederherstellung planen/bestätigen, Aufträge abbrechen/wiederholen, Zeitplan/Zertifikat/Speicher/Updateverwaltung. Zustandsstabilität während Polling, Bestätigung vor Änderungen, geschützte Passworteingabe falls Benutzerverwaltung enthalten. Tests mit realer Dienst-API sowie PTY.
- [x] Handbuch: `-h`, `-help`, `--help`, `help` mit Erfolgscode und hilfreicher Bedienung, `anker` standardmäßig TUI auf TTY und Hilfe bei umgeleiteter Ausgabe; Manpage installieren und paketieren, fehlendes man unter Debian automatisch installieren. Regressionen für Hilfe ohne Dienst und TTY-Start.
- [ ] Releasequelle abrunden: echtes Signierschlüsselpaar geschützt außerhalb Git speichern; öffentlichen Schlüssel mitliefern; signiertes Release und künftiges Signing prüfen. GitHub-Zugangsmöglichkeiten lesen, keine Zugangsdaten ausgeben. Fehlende GitHub-Adminrechte für Signing-Secrets sind ein konkreter externer Blocker, kein Grund die Anwendung unfertig zu lassen.
- [ ] Gemeinsame Go-/Python-/Browser-/Linux-Dienstprüfung, unabhängige Review, Readme aktualisieren und geprüften Stand veröffentlichen.

## Reviewfokus

Quelle wird bei laufendem Update geändert; fehlerhafte Konfigurationsdatei; verspätete UI-Antwort überschreibt neuere Auswahl; Update-Dienststart ohne Releasekonfiguration; Terminal-Resize/Maus während Formular oder Bestätigung; nach Update ist ein alter Plan versehentlich noch ausführbar; Zertifikatverlängerung bei nicht eingerichteten Releases.
