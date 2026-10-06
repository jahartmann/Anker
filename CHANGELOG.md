# Änderungen

## 0.2.0

- Passwortvorgabe standardmäßig acht Zeichen, administrativ auf 8 bis 128 Zeichen einstellbar. Einheitliche Prüfung in Ersteinrichtung, Web und CLI; bestehende Zugänge bleiben bei Erhöhung nutzbar.

- Aufträge mit Detailansicht, tatsächlichem Start, manuellem/automatischem Auslöser und direktem Sicherungszugriff. Abbruch, erneuter Start von Sicherung/Hostprüfung und administratives Entfernen abgeschlossener Einträge über Web/CLI.
- Tagesmarker und Einplanung atomar speichern, defekte Katalogwerte melden, geänderte Parallelität für wartende Aufträge übernehmen und Hostlöschung gegen gleichzeitig gestartete Aufträge schützen. Schedulerfehler bleiben sichtbar; verspätete Statusantworten überschreiben keine neueren Zustände.

- Speicherbereich für Anker und seine Backup-Laufwerke: Belegung, verfügbarer Platz, Inodes, echter Verlauf und bedingte Kapazitätsprognose. Bestätigte Dateisystemerweiterung für bereits zugewiesenen ext4-/XFS-Platz; LXC-Anleitung und herunterladbare Hinweise zum Einbinden neuer Laufwerke. Keine Formatierung oder automatische Partition-/LVM-Änderung.

- Webzertifikate automatisch und ohne Dienstneustart erneuern; Ablauf, Fingerprint, öffentlicher Download und administrativer Vorlauf in Einstellungen/CLI. Importierte Zertifikate bleiben unangetastet; selbstsignierte Zertifikate können eine neue Browserfreigabe benötigen.

- Kürzere Ersteinrichtung: eigene Zugangsdaten vor dem Dienstwechsel, bestehende Administratoren erkannt, bekannte Updatequellen automatisch übernommen.
- HTTPS-Zertifikat für private DNS-Namen oder IPs direkt im Assistenten erzeugen oder eigene Zertifikate importieren. Gültigkeit und Adresse werden geprüft; eine gültige bestehende TLS-Identität bleibt erhalten.
- Hostinstallation übernimmt eingeschränkte öffentliche SSH-Schlüssel in einem Aufruf. `anker host trust` prüft unabhängig angegebene Fingerprints und pflegt `known_hosts`; Hostformular und CLI verwenden die erzeugten SSH-Pfade automatisch.

- Produktionsinstallation startet ohne Demodaten. Demo nur durch ausdrücklichen Befehl, mit dauerhaft getrenntem Datenordner, lokalem Webzugriff und eigenem Socket. Release-Pakete enthalten keine Testfixtures oder Designbeispiele.

- Anmeldung bleibt über Dienstneustarts und Updates erhalten. Sitzungen laufen nach 30 Tagen ab; die Dauer ist einstellbar.
- Benutzer lassen sich sperren, entsperren, bearbeiten und löschen. Passwort- und Rechteänderungen beenden bestehende Sitzungen. Der letzte aktive Administrator bleibt geschützt.
- Signierte GitHub-Releases für Linux amd64 und arm64. Updates sind über die Weboberfläche und `anker update` möglich.
- Der Updater prüft Signatur, Plattform, Dateigröße und SHA-256. Vor dem Austausch sichert er Programm und Katalog. Ein fehlgeschlagener Start löst einen Rückfall aus.
- Aktive Sicherungen, Wiederherstellungen und Aufbewahrungsprüfungen sperren die Updateinstallation.
- Geführte Servereinrichtung mit verdecktem Passwort, TLS-Prüfung, getrennten SSH-Schlüsseln und wiederholbarer Ersteinrichtung. Verifizierter Release-Installer, auch für private GitHub-Repositories.
- Die Wartungsfreigabe nach einem Update wartet auf den laufenden Dienst; ein noch nicht erreichbarer Socket beim Booten verliert den Wiederholungsmarker nicht.
- Linux/systemd-Prüfung für Erstinstallation, Dienstwechsel, fehlerhafte Programme und unterbrochene Updatejournale im GitHub-Workflow.

## 0.1.0

Erster Entwicklungsstand mit zentraler Sicherung von Proxmox-Hostkonfigurationen, lesbarer Ablage, Weboberfläche, Shelloberfläche, Datei-Downloads und geführter Wiederherstellungsplanung.
