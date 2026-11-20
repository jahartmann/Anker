# Anker-Handbuch

[Doku-Übersicht](README.md) · [Betrieb](OPERATIONS.md) · [Wiederherstellung](RECOVERY.md)

## Installation

**Ziel:** Anker einrichten, einen Host anbinden und den ersten Sicherungsstand prüfen.

**Voraussetzungen:** Eigenes Linux-System, VM oder LXC mit systemd, `amd64` oder `arm64`, Python 3 und OpenSSL 3. SSH vom Anker-Server zum Proxmox-Host muss erreichbar sein. Einen vorgesehenen Daten-Mount unter `/srv/anker` vor der Einrichtung bereitstellen.

### 1. Release installieren

Aus einem vertrauenswürdigen Anker-Checkout auf dem Zielserver starten. Den öffentlichen Signierschlüssel vorher aus einer unabhängigen Quelle prüfen.

```sh
sudo python3 scripts/install-release.py --public-key /pfad/zur/geprüften/public.pem
```

Der Installer prüft Paket und Signatur und startet den Einrichtungsassistenten. [Signierte Downloads](https://github.com/jahartmann/Anker/releases) · [Quellcode, privates Repository und manuelle Paketprüfung](UPDATES.md#weitere-installationswege)

<a name="erste-einrichtung"></a>

### 2. Zugang einrichten

Administrator anlegen, Zugriffsweg auswählen und die Zusammenfassung bestätigen.

<a name="webzugriff-und-tls"></a>

| Zugriff | Einstellung im Assistenten |
| --- | --- |
| Direkt über LAN oder VPN | **HTTPS**, dann DNS-Name oder feste IP. Bei einem selbst erzeugten Zertifikat den Fingerprint am Server ablesen und mit dem Browser vergleichen. |
| Nur über SSH | **SSH-Tunnel**. Anker lauscht lokal auf `127.0.0.1:8087`. |

<details>
<summary>SSH-Tunnel und abgebrochene Einrichtung</summary>

Auf dem Arbeitsplatz den Tunnel offen lassen und dann `http://127.0.0.1:8087` öffnen.

```sh
ssh -N -L 8087:127.0.0.1:8087 BENUTZER@ANKER-SERVER
```

Eine unterbrochene Einrichtung auf dem Anker-Server mit `sudo anker setup` fortsetzen. [Zertifikate und SSH-Tunnel](reference/SETUP.md#webzugriff-und-tls)

</details>

<a name="proxmox-hosts-anbinden"></a>

### 3. Host anbinden

Unter **Hosts → Host hinzufügen** Hostadresse und einmaligen SSH-Zugang eingeben. Den angezeigten Fingerprint vor der Anmeldung an der Proxmox-Konsole vergleichen.

```sh
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256
```

Anker richtet getrennte SSH-Zugänge für Sicherung und Wiederherstellung ein. [Manuelle Anbindung und Hostschlüsselwechsel](reference/HOSTS.md#proxmox-hosts-anbinden)

### 4. Ersten Stand prüfen

**Jetzt sichern** starten. Den Auftrag unter **Aufträge → Ablauf ansehen** öffnen. Im fertigen Stand die benötigten Pfade kontrollieren und die Integrität prüfen. [Prüfbefehle und Fehlerbilder](OPERATIONS.md#erstprüfung-und-regelbetrieb)

## Im Alltag

| Aufgabe | Einstieg |
| --- | --- |
| Zeitpläne, Aufbewahrung, Fehler und Speicher | [Betrieb](OPERATIONS.md) |
| Zwei Stände eines Hosts vergleichen | **Hosts → Änderungen** · [Dateivergleich und Änderungsspur](reference/HOSTS.md#änderungen-prüfen) |
| Dateien wiederherstellen und Ergebnis prüfen | [Wiederherstellung](RECOVERY.md) |
| Version wechseln und Rückfall prüfen | [Installation und Updates](UPDATES.md) |

<a name="zeitplan-und-aufträge"></a>

Standardwerte für neue Installationen stehen unter [Zeitplan und Aufträge](OPERATIONS.md#zeitplan-und-aufträge).

## PBS-Konfiguration geführt wiederherstellen

PBS-Konfigurationsdateien sind Teil der Host-Sicherung. Datastore-Inhalte und Gastdaten werden separat gesichert. Für die geführte Wiederherstellung müssen Quelle und Ziel dieselbe unterstützte PBS-Hauptversion haben. Vor Zielprüfung, Übernahme und Rücksetzung beide PBS-Dienste am Ziel anhalten. Anschließend Jobregeln und Datastores prüfen, bevor Dienste oder Jobs wieder starten. [Vollständiger PBS-Ablauf](reference/HOSTS.md#pbs-konfiguration-geführt-wiederherstellen)

## Optionale lokale Demo

Im Checkout auf einem Entwicklungsrechner mit Go und `make`:

```sh
make build
./bin/anker --data ./var/demo demo
```

Unter `http://127.0.0.1:8087` mit `demo` und `anker-demo-2026` anmelden. Für den ersten Start einen leeren Demoordner verwenden. [Details zur Demo](reference/HOSTS.md#optionale-lokale-demo)

## Details

[Einrichtung, TLS und Benutzer](reference/SETUP.md) · [Hosts, Sicherungen und Bedienung](reference/HOSTS.md) · [Entwicklung](reference/HOSTS.md#entwicklung-und-lizenz)
