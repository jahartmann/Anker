# Änderungen

## 0.2.0

- Anmeldung bleibt über Dienstneustarts und Updates erhalten. Sitzungen laufen nach 30 Tagen ab; die Dauer ist einstellbar.
- Benutzer lassen sich sperren, entsperren, bearbeiten und löschen. Passwort- und Rechteänderungen beenden bestehende Sitzungen. Der letzte aktive Administrator bleibt geschützt.
- Signierte GitHub-Releases für Linux amd64 und arm64. Updates sind über die Weboberfläche und `anker update` möglich.
- Der Updater prüft Signatur, Plattform, Dateigröße und SHA-256. Vor dem Austausch sichert er Programm und Katalog. Ein fehlgeschlagener Start löst einen Rückfall aus.
- Aktive Sicherungen, Wiederherstellungen und Aufbewahrungsprüfungen sperren die Updateinstallation.

## 0.1.0

Erster Entwicklungsstand mit zentraler Sicherung von Proxmox-Hostkonfigurationen, lesbarer Ablage, Weboberfläche, Shelloberfläche, Datei-Downloads und geführter Wiederherstellungsplanung.
