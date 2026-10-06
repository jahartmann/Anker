# Implementierung

Anker besteht aus einem Go-Dienst mit eingebetteter React-Oberfläche, SQLite-Katalog, einem festen Python-Helfer auf dem Proxmox-Host, einem Unix-Socket-CLI und einer Bubble-Tea-Terminaloberfläche. Alle lokalen Demoabläufe sind isoliert; es wurde kein Produktionshost kontaktiert.

Die ursprüngliche visuelle Richtung wurde nach Nutzerfeedback ersetzt. Referenz: `docs/design/anker-hosts-concept.png`. Die Umsetzung verwendet einen weißen Inhaltsbereich, eine hellgraue Navigation ohne dekorative Symbole, schlichte Tabellen, eine gemeinsame Typografie-/Abstandsskala und zurückhaltende Dialoge. Statuszahlen kommen aus dem Dienst. Suchzustände, Fokus, Formularfehler, Mobilnavigation und Secretvorschauen sind eigene Zustände desselben Systems.

Abweichungen vom Konzept: `tar.gz` statt `tar.zst` für portable Standardwerkzeuge; manuell geführte Gesamtrecovery bis zur realen Laborabnahme; keine automatische Entscheidung über Disk-/Storage-Neuanlage; Secretfreigabe separat von Rollen. Terminalformulare können über den gemeinsamen Befehlseingang bedient werden. Details und Grenzen stehen in `SUPPORT.md` und `OPERATIONS.md`.

Abschluss: Go-Tests einschließlich Race Detector und Vet, 69 Go-Tests, zehn Python-Helfertests, 27 Browserabläufe, Typecheck/Webbuild sowie Linux-Crossbuild für amd64 und arm64. Der unabhängige Gesamtprüfer fand Fehler; die Korrekturen und zugehörigen Regressionen stehen in `REVIEW.md`. Desktop, Dialoge und Mobilansicht wurden zusätzlich im eingebauten Browser geprüft. Langzeit-/SFTP-/Hardwareabnahme wird durch lokale Fixturetests nicht ersetzt.

Bedienungsprüfung vom 06.10.2026: vorausgewählte Einzeldatei in Wiederherstellungsplänen, wiederholbare Ladefehler, vollständig sichtbare Aktionsmenüs, mobile Sicherungsaktionen ohne horizontales Scrollen und sichtbarer Speicherzustand mit Schutz offener Einstellungen. Zwölf zusätzliche Browserregressionen decken die gefundenen Fehler ab. Bestehende Schutzgrenzen der Wiederherstellung bleiben erhalten.
