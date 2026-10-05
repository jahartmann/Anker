# Anker

Anker sichert die **Konfiguration von Proxmox-Hosts** zentral über SSH. Weboberfläche, Terminaloberfläche und Befehle greifen auf denselben Dienst zu. Sicherungen bleiben als lesbare, geschützte Ordner verfügbar; Wiederherstellungspläne enthalten Originale, Zielinventar, Portzuordnungen und eine unabhängige Anleitung.

VM- und Containerdaten gehören weiterhin in den Proxmox Backup Server. Anker ist kein Festplattenimage und ersetzt keine Sicherung der Gastdaten.

## Lokal ansehen

```sh
make build
./bin/anker --data ./var/demo demo
```

Öffnen: http://127.0.0.1:8087 — lokale Demo mit `demo` / `anker-demo-2026`. Sie arbeitet ausschließlich mit Dateien unter `var/demo`, ohne echte Hosts zu kontaktieren. Mit `Ctrl+C` beenden.

In einem zweiten Terminal:

```sh
./bin/anker --data ./var/demo status
./bin/anker --data ./var/demo tui
./bin/anker --data ./var/demo help
```

Voraussetzungen für den Build: Go 1.27 oder neuer, Node.js 22.12+ und npm. Die Weboberfläche wird in das Binary eingebettet. Der Linux-Dienst braucht kein Node.js. `make linux` baut amd64 und arm64 ohne CGo. Die Linux-Installationsskripte hier wurden **nicht auf diesem Mac ausgeführt**.

## Linux-Server installieren

```sh
make linux
sudo ./scripts/install-server.sh ./bin/anker-linux-amd64
sudo -u anker /usr/local/bin/anker --data /srv/anker init
```

`init` verlangt `ANKER_INITIAL_PASSWORD` mit mindestens 12 Zeichen. Passwort verdeckt einlesen und nur für diesen Aufruf bereitstellen, beispielsweise in Bash:

```sh
read -rs -p 'Administratorpasswort: ' ANKER_PASSWORD; echo
sudo -u anker env ANKER_INITIAL_PASSWORD="$ANKER_PASSWORD" /usr/local/bin/anker --data /srv/anker init
unset ANKER_PASSWORD
sudo systemctl enable --now anker
```

Der Administrator heißt standardmäßig `admin`. Der Dienst bindet standardmäßig nur `127.0.0.1:8087`. Für einen Zugriff über SSH-Tunnel:

```sh
ssh -L 8087:127.0.0.1:8087 admin@anker-server
```

Direkter LAN-/VPN-Zugriff verlangt TLS. In `/etc/anker/service.env` die Listen-Adresse setzen und eine systemd-Override-Datei mit `ExecStart=` und einem neuen `ExecStart` samt `--tls-cert /etc/anker/tls/server.crt --tls-key /etc/anker/tls/server.key` anlegen. Der Dienstbenutzer braucht Leserechte auf diese Dateien. Ein vom Browser vertrauenswürdiges Zertifikat verwenden; Anker kann TLS selbst terminieren. Eine Reverse-Proxy-Terminierung erfordert passende Cookie-/Proxy-Konfiguration und ist in dieser Fassung kein fertig getesteter Installationsweg.

## Proxmox-Hosts anbinden

1. Auf jedem Host `sudo ./scripts/install-host.sh` ausführen. Es installiert den root-eigenen Python-Helfer mit festem Protokoll und einer engen sudo-Regel.
2. Auf dem Anker-Server einen eigenen Ed25519-Schlüssel erzeugen. Privaten Schlüssel unter `/etc/anker/keys` mit Eigentümer `anker`, Rechten `0600` und einem root-eigenen Elternordner ablegen.
3. Den öffentlichen Schlüssel auf dem Host in `/var/lib/anker-ssh/.ssh/authorized_keys` mit folgender Beschränkung eintragen:

```text
restrict,command="sudo -n /usr/local/lib/anker/anker-host --read-only" ssh-ed25519 PUBLIC_KEY ANKER
```

4. Den SSH-Hostfingerprint über eine unabhängige, vertrauenswürdige Verbindung prüfen. Erst danach den Hostschlüssel in `/etc/anker/known_hosts` hinterlegen. Ein ungeprüftes `ssh-keyscan` ist keine Identitätsprüfung.
5. In Anker Hostname, Adresse, Gruppe, SSH-Schlüssel und `known_hosts` eintragen. „Verbindung prüfen“, danach „Jetzt sichern“ ausführen. Auftrag, Pflichtlücken und gespeicherte Dateien prüfen.

Für automatische Einzeldateiübernahme zusätzlich auf dem Ziel `sudo ./scripts/install-host.sh --enable-restore` ausführen. Ein **anderes** Schlüsselpaar in `/var/lib/anker-restore-ssh/.ssh/authorized_keys` mit `restrict,command="sudo -n /usr/local/lib/anker/anker-host"` eintragen. In den erweiterten Hosteinstellungen Benutzer `anker-restore` und den eigenen Wiederherstellungsschlüssel hinterlegen. Der normale `anker`-Zugang kann ausschließlich Probe/Collect ausführen. Der Restorezugang kann privilegierte Konfiguration ändern und wird deshalb separat vergeben und geschützt. Ohne Restorezugang bleiben Planung und Export möglich; der Plan erklärt die fehlende Freigabe und sperrt die Ausführung.

Passwortlose Synchronisation verwendet hier **SSH-Schlüssel**, keine Zertifikate. SSH-Zertifikate sind bei einer vorhandenen SSH-CA optional möglich, aber nicht Teil dieses Installationswegs. TLS-Zertifikate sichern den Webzugriff.

Das Hostprofil `/etc/anker-host.json` enthält beispielsweise `{"paths":["/etc","/usr/local","/opt/my-config"]}`. Es muss root gehören und darf nicht von anderen beschreibbar sein. Zusätzliche Pflichtpfade in Anker müssen in diesem Profil tatsächlich enthalten sein. Fehlende oder unlesbare Dateien erzeugen eine unvollständige Sicherung. VM-Datenträger und große Anwendungsdaten nicht als Configprofil hinzufügen.

## Bedienung

- **Hosts:** Verbindung, Inventar, Sicherungszeit und zusätzliche Pflichtpfade.
- **Sicherungen:** Dateien ansehen, Stände vergleichen, Prüfsummen prüfen, schützen, archivieren und exportieren.
- **Wiederherstellung:** Einzeldateien, neue Hardware, Standalone-, Cluster- und Versionsszenarien planen. Ziel wird neu gelesen; Drift blockiert die Ausführung.
- **Aufträge:** Fortschritt, Abbruch und Fehler; unterbrochene Aufträge bleiben nach Neustart erkennbar.
- **Einstellungen:** Zeitplan, Aufbewahrung, E-Mail/Webhook, Benutzer und Aktivitätsprotokoll.

Im Terminal: Tab oder 1–5 wechseln die Bereiche, Pfeile/Enter öffnen Einträge, `b` startet eine Sicherung, `v` prüft einen Stand, `/` öffnet den Befehlseingang, `q` beendet. Mausauswahl wird unterstützt. Der Befehlseingang bietet dieselben CLI-Funktionen; `help` zeigt die genaue Syntax. Der lokale Unix-Socket ist nur für den Dienstbenutzer beziehungsweise root zugänglich und erlaubt Administration ohne Webpasswort.

## Sicherungsformat

```text
/srv/anker/
  catalog.db
  hosts/<host-id>/backups/<backup-id>/
    files/etc/...
    files/usr/local/...
    recovery/config.db
    inventory/host.json
    manifest.json
    checksums.sha256
    WIEDERHERSTELLUNG.md
    archive.tar.gz             # nur bei Archivierung
  plans/<plan-id>/
    prepared-files/...
    plan.json
    mapping.json
    manifest.json
    WIEDERHERSTELLUNG.md
```

Dateien werden unter Anker mit `0600`, Ordner mit `0700` gespeichert. Originalrechte, UID/GID, Zeitstempel, Links und erfasste Extended Attributes stehen im Manifest. Symbolische Links sind im lesbaren Baum bewusst inert als Linkzieltext gespeichert. Der vollständige TAR-Export enthält zusätzlich `original-files/` mit Originalmodi und tatsächlichen Links. Niemals ein komplettes Exportarchiv ungeprüft in `/` entpacken.

Archivierung komprimiert nur den Dateiordner. Manifest und Anleitung bleiben sichtbar; Anker prüft das Archiv vor Entfernen der lesbaren Dateien und öffnet es bei Bedarf wieder. Geschützte Stände, der letzte erfolgreiche Stand und von Plänen referenzierte Sicherungen werden nicht durch Aufbewahrung gelöscht. Tages-, Wochen- und Monatsregeln bilden zusammen die aufzubewahrenden Stände. Zeitplan und Aufbewahrung stehen in `docs/OPERATIONS.md`.

## Wiederherstellung und SFTP

Siehe [Recovery](docs/RECOVERY.md) und [Betrieb](docs/OPERATIONS.md). Der Planexport ist ohne laufenden Anker-Dienst nutzbar. Ein optionaler read-only SFTP-Zugang stellt die bestehenden Ordner bereit und erzeugt keine zweite Sicherungskopie. Zugang zu Originalen bedeutet Zugang zu den darin enthaltenen Secrets; nur ausdrücklich berechtigten Administratoren geben.

## Geprüfter Umfang

Diese Fassung bietet ausführbare lokale Abläufe und abgesicherte Dateiübernahme. Gesamtrecovery, Hardwaremigration und Cluster-/Versionswechsel bleiben auf echten Hosts **manuell geführte Pläne**. Automatische Gesamtausführung ist gesperrt, bis passende Proxmox-Versionen und Hardware-/Clusterfälle im Labor geprüft wurden. Reboot, Storage, Quorum, HA und PBS-Erreichbarkeit müssen separat bestätigt werden. Details: [Unterstützung](docs/SUPPORT.md).

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s host -p '*test*.py'
cd web && npm ci && npm run build && npx playwright install chromium && npm test
```

Die Browsertests starten eine isolierte Demo auf Port 8088 mit einem neuen Datenordner unter `/tmp`. Sie verändern weder Produktionshosts noch die normale Demo. Python-Helfertests verwenden ebenfalls ausschließlich temporäre lokale Verzeichnisse.
