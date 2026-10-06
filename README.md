# Anker

Anker sichert die **Konfiguration von Proxmox-Hosts** zentral über SSH. Die Sicherungen bleiben als lesbare Ordner verfügbar. Weboberfläche, Terminaloberfläche und Befehle verwenden denselben Dienst.

VM- und Containerdaten gehören weiterhin in den Proxmox Backup Server. Anker sichert Hostkonfigurationen, keine kompletten Festplattenimages. Für den zentralen Dienst einen eigenen Linux-Server oder eine VM mit systemd verwenden.

## Installation

Ein produktiver signierter Release ist noch nicht veröffentlicht. Bis dahin aus dem Quellcode bauen. Dafür werden Go 1.27.1+, Node.js 22.12+, npm und make benötigt:

```sh
git clone --depth 1 https://github.com/jahartmann/Anker.git
cd Anker
make linux
sudo ./scripts/install-server.sh
```

Der Installer wählt amd64 oder arm64 und öffnet die Einrichtung. Auf dem Server werden systemd, Python 3, OpenSSH und `runuser` benötigt. Go und Node.js werden nur zum Bauen verwendet. Der Build kann auch auf einem anderen Rechner erfolgen; anschließend den Projektordner einschließlich `bin/anker-linux-*` auf den Server übertragen und dort den Installer ausführen.

Sobald ein signierter Release bereitsteht, geht es ohne Compiler auf dem Server:

```sh
sudo python3 scripts/install-release.py --public-key /pfad/zur/geprüften/public.pem
```

Das Skript erkennt die Architektur, lädt das Paket, prüft Signatur und SHA-256 und startet dieselbe Einrichtung. Es benötigt zusätzlich OpenSSL 3. Den öffentlichen Signierschlüssel vor der ersten Verwendung unabhängig prüfen. Details zu privaten Repositories und Release-Erstellung stehen in [UPDATES.md](docs/UPDATES.md).

## Erste Einrichtung

Der Assistent fragt nur nach den Angaben, die er nicht selbst bestimmen kann:

1. Administratorname und ein eigenes Passwort mit Wiederholung.
2. Webzugriff: direkt über LAN/VPN mit HTTPS oder über einen SSH-Tunnel.
3. Bei HTTPS: DNS-Name oder IP für den Browser und die Zertifikatswahl.
4. Die angezeigte Konfiguration bestätigen.

Anker legt Benutzer und Ordner an, setzt die Dateirechte, erzeugt getrennte SSH-Schlüssel für Sicherung und Wiederherstellung und startet die Dienste. Eine bereits bekannte Updatequelle wird übernommen. Bei einem geprüften Release wird dessen öffentlicher Signierschlüssel automatisch verwendet. Für ein öffentliches Repository ist kein GitHub-Token nötig.

Die normale Installation startet leer. Es gibt keine Beispielhosts, Backups, Aufträge oder Demobenutzer. Voreingestellt sind tägliche Sicherungen ab 02:00 Uhr in Europe/Berlin, vier parallele Aufträge sowie 30 Tages-, 12 Wochen- und 12 Monatsstände. Zusätzliche Benutzer, SMTP und Webhooks können später in der Oberfläche eingerichtet werden.

Einrichtung wiederholen oder nach einem Abbruch fortsetzen:

```sh
sudo anker setup
```

Bestehende Benutzer und private SSH-Schlüssel bleiben erhalten. Der Assistent fragt nicht nochmals nach dem Administratorpasswort. Ein gültiges vorhandenes TLS-Zertifikat kann ohne erneute Dateiauswahl behalten werden. Für eine andere Updatequelle oder einen privaten GitHub-Zugang ausdrücklich `sudo anker setup --updates` verwenden.

`--no-setup` am Installer installiert nur die Dateien. Für einen regulären ersten Start danach `sudo anker setup` ausführen. `anker init` ist der kleinere Weg zur reinen Benutzeranlage; er ersetzt keine vollständige Einrichtung und überschreibt keine vorhandenen Zugänge. Für Automatisierung akzeptiert `init` die Umgebungsvariable `ANKER_INITIAL_PASSWORD`, kein Passwortargument.

## Webzugriff und TLS

| Zugriff | Was einzurichten ist |
| --- | --- |
| LAN/VPN, mehrere Nutzer | Im Assistenten HTTPS wählen. DNS-Name oder feste IP angeben. Anker bindet beim ersten Einrichten an Port 8087. |
| Nur über SSH-Tunnel | Im Assistenten SSH-Tunnel wählen. Anker bleibt auf `127.0.0.1:8087`; kein zusätzliches TLS-Zertifikat nötig. |
| Eigene interne CA oder vorhandenes Zertifikat | HTTPS wählen und Zertifikat samt Kette und passendem privatem Schlüssel importieren. Der Assistent prüft Adresse, Gültigkeit und Schlüsselpaar. |

Für LAN/VPN kann Anker selbst ein Zertifikat erzeugen. Dafür sind weder Domainregistrierung noch ein öffentlicher ACME-Dienst erforderlich. Die Verbindung ist verschlüsselt; das Zertifikat ist zunächst **nicht vom Browser vertraut**. Den am Server angezeigten SHA256-Fingerprint mit dem Browser vergleichen, bevor eine Ausnahme bestätigt wird. Für verwaltete Clients ein Zertifikat eurer bereits vertrauten internen CA verwenden.

Das automatisch erzeugte Zertifikat gilt ein Jahr. Der Assistent zeigt das Ablaufdatum. Es wird nicht im Hintergrund verlängert: vor Ablauf `sudo anker setup` öffnen und „automatisch erzeugen“ wählen. Innerhalb der letzten 30 Tage wird dann ein neues Zertifikat erzeugt; bei der Browserprüfung den neuen Fingerprint verwenden. Wiederholte Einrichtung behält ein noch ausreichend gültiges Zertifikat mit gleicher Adresse bei.

HTTPS schützt Anmeldung, Sitzung und heruntergeladene Konfigurationen. Ein VPN ersetzt die Absicherung der Webverbindung auf den beteiligten Rechnern und Netzabschnitten nicht automatisch. Deshalb bleibt direkter Webzugriff im Assistenten bei HTTPS. Hintergrund: [OWASP TLS](https://cheatsheetseries.owasp.org/cheatsheets/Transport_Layer_Security_Cheat_Sheet.html).

Für den SSH-Tunnel auf dem Arbeitsplatz:

```sh
ssh -N -L 8087:127.0.0.1:8087 BENUTZER@ANKER-SERVER
```

Danach `http://127.0.0.1:8087` öffnen. Die Netzwerkverbindung wird bereits durch SSH verschlüsselt. Den Tunnel für die Nutzung offen lassen. Ein Reverse Proxy ist für diese Wege nicht erforderlich; Proxy-Terminierung ist in dieser Fassung kein getesteter Installationsweg.

## Proxmox-Hosts anbinden

Auf jedem Host werden der Helfer und ein eingeschränkter SSH-Zugang eingerichtet. Dafür das Projekt- oder Release-Paket und **nur den öffentlichen** Sicherungsschlüssel auf den Host übertragen. Der öffentliche Schlüssel liegt auf dem Anker-Server unter `/etc/anker/keys/backup.pub`.

Auf dem Proxmox-Host:

```sh
sudo ./scripts/install-host.sh --backup-key /pfad/backup.pub
```

Das Skript installiert Helfer, Benutzer, sudo-Regel und den eingeschränkten Schlüsseleintrag gemeinsam. Manuelles Bearbeiten von `authorized_keys` entfällt. Wiederholung erzeugt keine doppelten Schlüssel; vorhandene Profile und andere Schlüssel bleiben erhalten. Der Zugang kann nur den festen Sicherungshelfer ausführen, keine freie Shell.

Das Skript zeigt den **Ed25519-SSH-Hostfingerprint**. Diesen über die Proxmox-Konsole oder eine schon vertrauenswürdige Administrationsverbindung ablesen. Auf dem Anker-Server:

```sh
sudo anker host trust HOSTADRESSE --fingerprint SHA256:FINGERPRINT
```

Bei einem anderen SSH-Port zusätzlich `--port PORT` angeben. Anker fragt den Schlüssel ab, vergleicht ihn mit dem angegebenen Fingerprint und schreibt ihn erst bei Übereinstimmung nach `/etc/anker/known_hosts`. Ein ungeprüftes `ssh-keyscan` oder abgeschaltete Hostprüfung gehört nicht zum Einrichtungsweg. Bereits vorhandene vertrauenswürdige Hosteinträge können weiterhin verwendet werden. [OpenSSH-Hostprüfung](https://man.openbsd.org/ssh_config#StrictHostKeyChecking).

Danach in der Weboberfläche „Host hinzufügen“ öffnen: Hostname und Adresse reichen für den Standardzugang. Schlüsselpfad, Benutzer, Port und bekannte Hostschlüssel sind bereits gesetzt. „Verbindung prüfen“, anschließend „Jetzt sichern“. Bei Fehlern den Auftrag und die Pflichtlücken ansehen. Gruppe, eigener Zeitplan und zusätzliche Pfade sind optional.

Alternativ im Terminal:

```sh
sudo anker host add --name HOSTNAME --address HOSTADRESSE
sudo anker host list
sudo anker host probe HOST-ID
sudo anker backup run HOST-ID
```

Für abweichende SSH-Zugänge die erweiterten Hosteinstellungen beziehungsweise `--key` und `--known-hosts` verwenden.

### Optional: Einzeldateien zurückspielen

Für Planung, Download und manuelle Wiederherstellung ist kein privilegierter Restorezugang nötig. Nur für die automatische Einzeldateiübernahme zusätzlich `/etc/anker/keys/restore.pub` auf den Zielhost übertragen:

```sh
sudo ./scripts/install-host.sh --backup-key /pfad/backup.pub --restore-key /pfad/restore.pub
```

`--restore-key` richtet den separaten Benutzer `anker-restore` mit dem festen Wiederherstellungshelfer ein. Die beiden Schlüssel müssen verschieden sein. In den erweiterten Hosteinstellungen den Wiederherstellungsschlüssel `/etc/anker/keys/restore` hinterlegen; der Benutzer ist bereits vorbelegt. Der normale Sicherungsschlüssel bleibt auf Lesezugriff beschränkt.

Passwortlose Synchronisation verwendet **SSH-Schlüssel**. SSH-Zertifikate oder eine zusätzliche SSH-CA sind dafür nicht erforderlich. TLS-Zertifikate sichern ausschließlich den Webzugriff.

Das Hostprofil `/etc/anker-host.json` sichert standardmäßig `/etc` und `/usr/local`. Zusätzliche Konfiguration beispielsweise unter `/opt` ausdrücklich im Profil und als Pflichtpfad in Anker ergänzen. Das Profil muss root gehören und darf nicht von anderen beschreibbar sein. VM-Datenträger und große Anwendungsdaten nicht in das Configprofil aufnehmen.

## Einrichtung im Überblick

| Schritt | Vereinfachung oder Grund für die manuelle Angabe |
| --- | --- |
| Server installieren | Ein Installer wählt die Architektur und öffnet den Assistenten. Benutzer, Ordner, Rechte und Dienste werden eingerichtet. |
| Zugang anlegen | Ein eigener Administrator; kein Standardpasswort. Vorhandene Zugänge werden bei Wiederholung erkannt. |
| Webzugriff | HTTPS mit erzeugtem Zertifikat oder vorhandener CA; alternativ SSH-Tunnel ohne TLS-Einrichtung. Adresse und erstes Vertrauen kann Anker nicht sicher erraten. |
| Updates | Bereits geprüfter Schlüssel und Repository werden übernommen. Keine wiederholten Fragen; öffentliche Downloads brauchen keinen Token. |
| Hostzugang | Helferinstallation und eingeschränkter Schlüsseleintrag in einem Aufruf. Standardpfade sind im Interface und in der CLI hinterlegt. |
| Hostidentität | Ein unabhängig geprüfter Fingerprint; Eintrag und Dateirechte übernimmt `host trust`. |
| Betrieb | Gemeinsamer Zeitplan, Aufbewahrung und 30-Tage-Anmeldung sind vorbelegt. Benachrichtigungen und Restorezugänge nur bei Bedarf einrichten. |

## Anmeldung und Benutzer

Die Weboberfläche verlangt eine Anmeldung. Standardmäßig gilt sie 30 Tage, einstellbar von 1 bis 365 Tagen für neue Sitzungen. Anmeldung bleibt über Dienstneustarts erhalten. Abmelden, Passwort- oder Rechteänderungen sowie Sperren und Löschen beenden die betroffenen Sitzungen.

Unter „Einstellungen → Zugriff“ weitere Benutzer anlegen:

- **Lesen:** Hosts, Aufträge und freigegebene Dateien ansehen und herunterladen.
- **Wiederherstellung:** Zusätzlich Sicherungen starten und Wiederherstellungen ausführen.
- **Administrator:** Zusätzlich Benutzer, Einstellungen und Updates verwalten.

Secrets und vollständige Exporte brauchen eine eigene Freigabe. Der eigene Administrator und der letzte aktive Administrator sind gegen Aussperren geschützt. Der lokale Unix-Socket erlaubt root und dem Dienstbenutzer Administration ohne Webpasswort; er darf nicht über das Netz freigegeben werden. Passwort zurücksetzen: [Betrieb](docs/OPERATIONS.md).

## Bedienung und Sicherungsformat

Hosts verwalten, Sicherungen prüfen, vergleichen, schützen, archivieren oder herunterladen. „Herunterladen“ liefert den vollständigen Stand mit Manifest, Inventar, Anleitung und Originaldateien. Einzeldateien und vorbereitete Wiederherstellungspläne lassen sich ebenfalls direkt herunterladen.

```text
/srv/anker/
  .anker-mode
  catalog.db
  hosts/<host-id>/backups/<backup-id>/
    files/etc/...
    recovery/config.db
    inventory/host.json
    manifest.json
    checksums.sha256
    WIEDERHERSTELLUNG.md
  plans/<plan-id>/
    prepared-files/...
    plan.json
    mapping.json
    WIEDERHERSTELLUNG.md
```

Dateien liegen mit `0600`, Ordner mit `0700` vor. Originalrechte und weitere Metadaten stehen im Manifest und im vollständigen TAR-Export. Ein Exportarchiv nicht ungeprüft in `/` entpacken. Archivierung komprimiert ältere Dateiordner; Manifeste und Anleitungen bleiben lesbar.

Im Terminal `sudo anker tui` öffnen. Tab oder 1–5 wechseln die Bereiche, Pfeile/Enter öffnen Einträge, `b` startet eine Sicherung, `v` prüft einen Stand, `/` öffnet den Befehlseingang und `q` beendet. `anker help` zeigt die Befehle.

Updates: „Einstellungen → System“ oder `sudo anker update check`, danach `sudo anker update install`. Signatur und SHA-256 werden vor der Installation geprüft. Laufende Sicherungen und Wiederherstellungen blockieren das Update. Bei fehlgeschlagenem Start stellt der Updater die vorherige Version und den Katalog wieder her. [Einrichtung und Rückfall](docs/UPDATES.md).

## Wiederherstellung und Grenzen

Gesamtrecovery, Hardwaremigration und Cluster-/Versionswechsel sind derzeit **manuell geführte Pläne**. Automatische Gesamtausführung bleibt gesperrt, bis die Proxmox-, Hardware- und Clusterfälle im Labor geprüft sind. Reboot, Storage, Quorum, HA und PBS-Erreichbarkeit separat bestätigen.

[Recovery](docs/RECOVERY.md), [Betrieb und optionaler SFTP-Export](docs/OPERATIONS.md), [geprüfter Umfang](docs/SUPPORT.md). Planexporte bleiben ohne laufenden Anker-Dienst verwendbar. SFTP kann bestehende Ordner ohne zweite Kopie bereitstellen.

## Optionale lokale Demo

Nur der ausdrückliche Befehl `demo` erzeugt Beispieldaten:

```sh
make build
./bin/anker --data ./var/demo demo
```

Auf `http://127.0.0.1:8087` mit `demo` / `anker-demo-2026` anmelden. Mit `Ctrl+C` beenden. Die Demo kontaktiert keine echten Hosts und bleibt auf Loopback beschränkt. Ohne `--data` verwendet sie `./var/demo`, unabhängig von `ANKER_DATA`.

Beim ersten Start muss der Demoordner leer sein. `.anker-mode` hält die Betriebsart fest. Demo und Produktion dürfen sich nicht überlappen; `/srv/anker` und seine Eltern- und Unterverzeichnisse sind für Demo gesperrt. Alte unmarkierte Demoordner bleiben erhalten; stattdessen einen neuen Demoordner wählen. Den Marker nicht zum Wechseln der Betriebsart entfernen oder ändern.

## Entwicklung und Lizenz

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s host -p 'test_*.py'
python3 -m unittest discover -s scripts -p 'test_*.py'
cd web && npm ci && npm run build && npx playwright install chromium && npm test
```

Browserprüfungen verwenden getrennte temporäre Ordner für Demo und leere Produktion. Linux-/systemd-Prüfungen testen den echten Installer, Terminaleinrichtung, HTTPS-Anmeldung, Updates und Wiederanlauf in einer isolierten CI-VM. Tests und Designentwürfe werden nicht in Release-Pakete aufgenommen.

[MIT-Lizenz](LICENSE), [Beiträge](CONTRIBUTING.md), [Sicherheitsmeldungen](SECURITY.md). Schreibrechte am öffentlichen Originalrepository erhalten nur freigegebene Maintainer. Eigene Forks und Änderungen sind unter MIT erlaubt.
