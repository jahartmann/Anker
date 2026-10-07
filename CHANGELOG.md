# Änderungen

## 0.2.7

- Dateidialog passt Liste und Vorschau an die verfügbare Bildschirmhöhe an. Download und Wiederherstellungsaktionen bleiben auch auf kleineren Laptop- und Mobilansichten direkt sichtbar.

## 0.2.6

- Ankersymbol im Browser-Tab und einheitliche kleine Wortmarke; der Tabtitel folgt der geöffneten Ansicht. Das Symbol bleibt auf hellen und dunklen Browserflächen erkennbar.
- Übersicht mit getrennten Kennzahlen für aktive Hosts, aktuelle Sicherungen, Handlungsbedarf und aktive Aufträge. Pausierte Hosts und der ausdrücklich deaktivierte Demozeitplan bleiben sichtbar.
- Dezente Akzente für Navigation, Register, Fokus und Dateiauswahl; kompakte Gruppenkennzeichnung und einheitliche Statuskapseln. Echte Fehler unterscheiden sich von Hinweisen.
- Klarere Abschnittsabstände, besser lesbare sekundäre Texte und kurze Dialogübergänge, die bei reduzierter Bewegung deaktiviert bleiben. Die bestehende ruhige Tabellenbasis bleibt erhalten.

## 0.2.5

- Wiederherstellung mit Quellhostfilter, durchsuchbarer Dateiauswahl und festen Seiten für Dateien, Pläne und Planschritte. Auswahlen bleiben beim Suchen und Blättern erhalten; neu geprüfte Zielports sind über „Zuordnungen anpassen“ direkt erreichbar.
- Quell-/Zielwechsel setzt Konsolen-/Isolationsbestätigungen und Netzwerkzuordnungen zurück. Planstatusfilter bleiben auch bei laufenden Statusänderungen sichtbar. Hostliste mit Seitenwechsel, Handlungsbedarfsfilter und übersichtlichen mobilen Filtern.
- Terminal bietet sämtliche Wiederherstellungsszenarien und direkte Dateipfadeingabe mit `Ctrl+E`. Vollständige Szenarien werden ausdrücklich als manuell geführt erklärt.
- Planexport enthält jetzt Originaldateien mit gesicherten Rechten, Eigentümern, Linkzielen und erweiterten Metadaten, auch aus archivierten Sicherungen. Vorbereitete Dateien, Integritäts- und Zugriffsprüfungen bleiben erhalten.
- README und Implementierungsübersicht unterscheiden umgesetzte Betriebsfunktionen von offenen Recovery-Regeln und realen Laborabnahmen.

## 0.2.4

- Die Weboberfläche lädt nach einem bestätigten Update automatisch neu, auch nach einem Seitenwechsel oder vorübergehendem Verbindungsabbruch. Fehlgeschlagene Updates erzeugen keine Reload-Schleife.
- Laufende und wartende Aufträge sind getrennt vom Verlauf sichtbar, mit Host, tatsächlicher Laufzeit und Versuch. Aufträge und Sicherungen haben Filter und feste Seiten statt wachsender Listen.
- Sicherungsdateien über Ordner, Breadcrumbs und eine Suche über alle Pfade durchsuchen; 25 oder 50 Einträge pro Seite. Vorschau, geschützte Inhalte, Download und Wiederherstellung bleiben verfügbar.
- Inventar mit kompakten Systemangaben, durchsuchbaren Netzwerk-/Datenträgertabellen und aufklappbaren Details. Zurückhaltende Statusfarben und einheitliche Listensteuerung.
- Aufbewahrung erklärt die Tages-, Wochen- und Monatsregeln je Host. Archivierung und Überfälligkeitsgrenze sind getrennt; automatische Archivierung lässt sich ausschalten.
- Neue Sicherungen erkennen die Proxmox-RNG-Quelle `/dev/urandom` und nachweislich fehlende optionale Vim-Includes korrekt. Die zentrale Prüfung berücksichtigt vorhandene Hosthelfer; echte fehlende Dateien bleiben Hinweise. Vorhandene Sicherungsmanifeste bleiben unverändert.

## 0.2.2

- Hosts direkt im Web oder Terminal mit Adresse, SSH-Benutzer und einmaligem Passwort anbinden. Nach Fingerprintbestätigung installiert Anker den Helfer und getrennte Sicherungs-/Wiederherstellungszugänge; gespeichert wird erst nach erfolgreichen Schlüsselprüfungen.
- Bestehende Profile und fremde autorisierte Schlüssel bei erneuter Einrichtung erhalten. Passwort bleibt flüchtig; bekannte Schlüsselwechsel, konkurrierende Hostaufträge und Updates werden abgefangen. Manuelle Einrichtung bleibt verfügbar.
- Terminal: `a` bindet automatisch an, `m` legt manuell an und `v` richtet den ausgewählten Host erneut ein. README und Handbuch erklären beide Wege.

## 0.2.0

- Updatequelle direkt im Web einrichten: mitgelieferte Anker-Quelle, Fingerprint und bestätigter Wechsel; bestehende Signatur-, Start- und Rückfallprüfung bleiben erhalten.
- Geführte Terminalverwaltung mit Formularen und bestätigten Aktionen für Hosts, Sicherungen, Wiederherstellung, Aufträge, Zeitplan, Benutzer und Systemfunktionen, einschließlich SSH-Fingerprintprüfung und Speichererweiterung. `anker` öffnet sie im Terminal; `anker -help` und die installierte Manpage erklären den Betrieb.

- TLS-Ordnerrechte unabhängig von der Installer-umask setzen und bei erneuter Einrichtung reparieren. Der Webdienst kann seine Zertifikate lesen; private Schlüssel bleiben vor anderen Benutzern geschützt.

- Installation direkt nach dem Clone: fehlende Werkzeuge automatisch installieren und Anker bauen. Dasselbe Skript erkennt bestehende Installationen und aktualisiert Hauptprogramm und Systemhelfer mit Sicherung, Startprüfung und Rückfall.

- Wiederholte Terminaleinrichtung mit „Fortsetzen“ oder „Neu konfigurieren“. Vorhandene Ports, Zertifikate, Benutzer und SSH-Schlüssel bleiben erhalten; unterbrochene Konfigurationsänderungen werden beim nächsten Einrichtungsaufruf aus einem geschützten Journal zurückgenommen.

- Terminaleinrichtung mit klaren Abschnitten, kompakter Konfigurationsübersicht und animierter Dienstprüfung; ruhige Ausgabe für einfache Terminals und Logs.

- Ersteinrichtung auf einem direkt eingebundenen Dateisystem funktioniert auch mit root-eigenem `lost+found`; dessen Rechte und Inhalte bleiben unverändert.

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
