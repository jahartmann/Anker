# Implementierung

Anker besteht aus einem Go-Dienst mit eingebetteter React-Oberfläche, SQLite-Katalog, einem festen Python-Helfer auf dem Proxmox-Host, einem Unix-Socket-CLI und einer Bubble-Tea-Terminaloberfläche. Alle lokalen Demoabläufe sind isoliert; es wurde kein Produktionshost kontaktiert.

Die ursprüngliche visuelle Richtung wurde nach Nutzerfeedback ersetzt. Referenz: `docs/design/anker-hosts-concept.png`. Die Umsetzung verwendet einen weißen Inhaltsbereich, eine hellgraue Navigation ohne dekorative Symbole, schlichte Tabellen, eine gemeinsame Typografie-/Abstandsskala und zurückhaltende Dialoge. Statuszahlen kommen aus dem Dienst. Suchzustände, Fokus, Formularfehler, Mobilnavigation und Secretvorschauen sind eigene Zustände desselben Systems.

Abweichungen vom Konzept: `tar.gz` statt `tar.zst` für portable Standardwerkzeuge; manuell geführte Gesamtrecovery bis zur realen Laborabnahme; keine automatische Entscheidung über Disk-/Storage-Neuanlage; Secretfreigabe separat von Rollen. Terminalformulare können über den gemeinsamen Befehlseingang bedient werden. Details und Grenzen stehen in `SUPPORT.md` und `OPERATIONS.md`.

Tests, Browserprüfung und unabhängige Gesamtprüfung werden im Abschluss dieser Implementierung dokumentiert. Automatische Langzeit-/SFTP-/Hardwareabnahme wird durch lokale Fixturetests nicht ersetzt.
