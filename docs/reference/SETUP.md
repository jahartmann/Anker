# Einrichtung im Detail

[Doku-Übersicht](../README.md) · [Zur Anleitung](../HANDBOOK.md)

[Installation](#installation) · [TLS](#webzugriff-und-tls) · [Container](#anker-im-container-und-speicher) · [Zugriff](#anmeldung-und-benutzer)

## Installation

Auf Debian 13 mit systemd, als root:

```sh
git clone https://github.com/jahartmann/Anker.git /opt/anker-source
cd /opt/anker-source
./scripts/install-server.sh
```

Der Installer installiert fehlende Systempakete, lädt bei Bedarf den passenden Go-Compiler, baut Anker und öffnet den Einrichtungsassistenten. Die Weboberfläche ist bereits enthalten; Node.js und make werden auf dem Server nicht gebraucht. Nach der Bestätigung laufen beide Dienste und starten beim Booten automatisch. Die Angaben für Administratorzugang und Webadresse bleiben im Assistenten erforderlich. Bei einem vorhandenen Anker werden Programme und Katalog vor dem Austausch gesichert; Benutzer, SSH-Schlüssel, Zertifikate und Einstellungen bleiben erhalten. Ein fehlgeschlagener Programmstart löst einen Rückfall aus. Lokale Änderungen an systemd-Units werden bei Aktualisierung nicht überschrieben.

Für spätere Aktualisierungen aus dem Quellcode im selben Ordner:

```sh
git pull --ff-only
./scripts/install-server.sh
```

Dieser Weg gilt für den normalen Clone auf `main`. Der Installer aktualisiert die Dateien aus deinem Checkout; er lädt keine Git-Änderungen im Hintergrund. Falls der Checkout früher auf einen einzelnen Commit gesetzt wurde, einmal `git switch main` ausführen. Für Installation ohne anschließenden interaktiven Assistenten `--no-setup` verwenden. Fehlende Werkzeuge installiert der Installer über APT; auf anderen Linux-Systemen müssen sie zuvor vorhanden sein. Unterstützt werden amd64 und arm64. Der erste Build benötigt Internetzugriff und vorübergehend zusätzlichen Platz für Compiler und Module.

Der Quellcodeweg arbeitet mit dem von dir gewählten Checkout. Signierte Versionen stehen unter [Releases](https://github.com/jahartmann/Anker/releases). Für spätere Updates reicht die Weboberfläche; Compiler und Git werden dafür auf dem Server nicht gebraucht.

Eine Erstinstallation aus einem signierten Release benötigt keinen Compiler:

```sh
sudo python3 scripts/install-release.py --public-key /pfad/zur/geprüften/public.pem
```

Das Skript erkennt die Architektur, lädt das Paket, prüft Signatur und SHA-256 und startet dieselbe Einrichtung. Es benötigt zusätzlich OpenSSL 3. Den öffentlichen Signierschlüssel vor der ersten Verwendung unabhängig prüfen. Details zu privaten Repositories und Release-Erstellung stehen in [UPDATES.md](../UPDATES.md).

## Erste Einrichtung

Der Assistent führt in drei Abschnitten durch Zugang, Verbindung und die abschließende Prüfung. Die Zusammenfassung zeigt die tatsächlichen Einstellungen vor dem Speichern; beim Start der Dienste läuft ein dezenter Statusanzeiger. Passwörter bleiben verdeckt. Für eine ruhige Ausgabe `ANKER_NO_ANIMATION=1 anker setup` verwenden; `NO_COLOR=1` deaktiviert Farben. Bei umgeleiteter Ausgabe oder `TERM=dumb` werden keine Animationen oder Farbcodes ausgegeben.

Der Assistent fragt nur nach den Angaben, die er nicht selbst bestimmen kann:

1. Administratorname und ein eigenes Passwort mit mindestens acht Zeichen und Wiederholung.
2. Webzugriff: direkt über LAN/VPN mit HTTPS oder über einen SSH-Tunnel.
3. Bei HTTPS: DNS-Name oder IP für den Browser und die Zertifikatswahl.
4. Die angezeigte Konfiguration bestätigen.

Anker legt Benutzer und Ordner an, setzt die Dateirechte, erzeugt getrennte SSH-Schlüssel für Sicherung und Wiederherstellung und startet die Dienste. Eine bereits bekannte Updatequelle wird übernommen. Bei einem geprüften Release wird dessen öffentlicher Signierschlüssel automatisch verwendet. Für ein öffentliches Repository ist kein GitHub-Token nötig.

Ein eigenes Dateisystem oder LXC-Mount unter `/srv/anker` ist möglich. Ein vorhandenes `lost+found` direkt darin bleibt unangetastet; Anker benötigt darauf keinen Zugriff.

Die normale Installation startet leer. Es gibt keine Beispielhosts, Backups, Aufträge oder Demobenutzer. Voreingestellt sind tägliche Sicherungen ab 02:00 Uhr in Europe/Berlin, vier parallele Aufträge sowie 30 Tages-, 12 Wochen- und 12 Monatsstände. Zusätzliche Benutzer, SMTP und Webhooks können später in der Oberfläche eingerichtet werden.

Einrichtung wiederholen oder nach einem Abbruch fortsetzen:

```sh
sudo anker setup
```

Bei erneuter Einrichtung fragt Anker nach „Fortsetzen“ oder „Neu konfigurieren“. Mit Enter werden die vorhandenen Verbindungseinstellungen übernommen, einschließlich Port und TLS-Zertifikat. „Neu konfigurieren“ öffnet die Fragen zur Verbindung erneut. Beide Optionen behalten Benutzer, Sicherungen und private SSH-Schlüssel; das Administratorpasswort wird nicht erneut abgefragt. Fehlen noch Verbindungseinstellungen oder ist das Zertifikat ungültig, führt Anker durch die fehlenden Angaben.

Vor dem Speichern kann der Assistent ohne Änderungen abgebrochen werden. Beim Speichern sichert Anker die bisherigen Konfigurationsdateien. Bei einem Fehler stellt er diesen Stand wieder her; nach einem Prozessabbruch oder Stromausfall erkennt der nächste Aufruf von `sudo anker setup` die unterbrochene Einrichtung und stellt ihn vor den Fragen wieder her. Bereits angelegte Benutzer und SSH-Schlüssel werden weiterverwendet. Neu erzeugte, verworfene TLS- und Token-Dateien werden entfernt. Nach einem Abbruch beim Speichern den Assistenten erneut aufrufen; dieser Wiederanlauf ersetzt keinen automatischen Rollback beim Booten. Öffentliche Updatequellen lassen sich in der Weboberfläche ändern. Einen privaten GitHub-Zugang ausdrücklich mit `sudo anker setup --updates` einrichten.

`--no-setup` am Installer installiert nur die Dateien. Für einen regulären ersten Start danach `sudo anker setup` ausführen. `anker init` ist der kleinere Weg zur reinen Benutzeranlage; er ersetzt keine vollständige Einrichtung und überschreibt keine vorhandenen Zugänge. Für Automatisierung akzeptiert `init` die Umgebungsvariable `ANKER_INITIAL_PASSWORD`, kein Passwortargument.

## Webzugriff und TLS

| Zugriff | Was einzurichten ist |
| --- | --- |
| LAN/VPN, mehrere Nutzer | Im Assistenten HTTPS wählen. DNS-Name oder feste IP angeben. Anker bindet beim ersten Einrichten an Port 8087. |
| Nur über SSH-Tunnel | Im Assistenten SSH-Tunnel wählen. Anker bleibt auf `127.0.0.1:8087`; kein zusätzliches TLS-Zertifikat nötig. |
| Optional: vorhandenes Zertifikat | HTTPS wählen und Zertifikat samt Kette und passendem privatem Schlüssel importieren. Der Assistent prüft Adresse, Gültigkeit und Schlüsselpaar. |

Ohne interne CA und ohne eigene öffentliche Domain im Assistenten „Erzeugen“ wählen. Anker erstellt ein Zertifikat für den angegebenen internen DNS-Namen oder die IP. Dafür sind weder Domainregistrierung noch ein öffentlicher ACME-Dienst erforderlich. Die Verbindung ist verschlüsselt; das Zertifikat ist zunächst **nicht vom Browser vertraut**. Den am Server angezeigten SHA256-Fingerprint mit dem Browser vergleichen, bevor eine Ausnahme bestätigt wird. Eine interne CA wird dafür nicht vorausgesetzt. Die Prüfung ist in jedem verwendeten Browser beziehungsweise auf jedem Arbeitsplatz erforderlich. Bereits vorhandene, vom Browser vertraute Zertifikate lassen sich optional importieren.

Das automatisch erzeugte Zertifikat gilt ein Jahr. Bei neuer Einrichtung ist die automatische Erneuerung eingeschaltet: Anker prüft beim Start und alle sechs Stunden und erneuert standardmäßig innerhalb der letzten 30 Tage. Der Webdienst übernimmt das neue Zertifikat ohne Neustart. Der private Schlüssel und die eingerichteten DNS-Namen/IPs bleiben erhalten; das vorherige öffentliche Zertifikat bleibt als Rückfallkopie vorhanden.

Unter **Einstellungen → Zertifikate** Ablaufdatum, Webadressen und SHA256-Fingerprint ansehen, das öffentliche Zertifikat herunterladen, Automatik ein-/ausschalten und den Vorlauf zwischen 7 und 90 Tagen einstellen. „Jetzt erneuern“ ist nach Bestätigung jederzeit möglich. Änderungen erfordern Administratorrechte; der letzte Automatikfehler bleibt bis zur erfolgreichen Prüfung/Erneuerung sichtbar. Die Einstellungen bleiben nach Dienst- und Containerneustarts erhalten.

Wichtig: Erneuerung verändert den Zertifikatsfingerprint. Bei selbstsignierten Zertifikaten kann deshalb auf jedem Arbeitsplatz eine neue Browserfreigabe erforderlich werden. Automatische Erneuerung auf dem Server ersetzt kein dauerhaftes Browservertrauen. Den neuen Fingerprint bei Bedarf direkt am Server mit `sudo anker tls status` ablesen und vergleichen. Eine eigene interne CA ist weiterhin keine Voraussetzung.

Für bereits vorhandene Zertifikate einmal `sudo anker setup` ausführen, „Neu konfigurieren“ und anschließend die TLS-Option „Erzeugen“ wählen. Ein noch ausreichend gültiges vorhandenes Zertifikat mit gleicher Adresse bleibt dabei erhalten; der Assistent registriert es für die Erneuerung und übernimmt ältere Dienstkonfigurationen. Zertifikate aus der Option „eigene Zertifikatsdateien“ bleiben extern verwaltet, auch wenn sie selbstsigniert sind. Sie werden nicht automatisch überschrieben. Die Zertifikatsverwaltung läuft auch ohne eingerichtete GitHub-Updatequelle.

Alternativ im Terminal:

```sh
sudo anker tls status
sudo anker tls auto on --days 30
sudo anker tls auto off
sudo anker tls renew
sudo anker tls certificate ./anker-server.crt
```

Die CLI-Erneuerung erfolgt sofort; vorher den Zugriff für eine eventuell notwendige Browserfreigabe sicherstellen. Automatik und Einstellungen brauchen den laufenden `anker-updater.service`, der die root-eigenen Zertifikatsdateien verwaltet. Fehler stehen zusätzlich in `journalctl -u anker-updater`.

HTTPS schützt Anmeldung, Sitzung und heruntergeladene Konfigurationen. Ein VPN ersetzt die Absicherung der Webverbindung auf den beteiligten Rechnern und Netzabschnitten nicht automatisch. Deshalb bleibt direkter Webzugriff im Assistenten bei HTTPS. Hintergrund: [OWASP TLS](https://cheatsheetseries.owasp.org/cheatsheets/Transport_Layer_Security_Cheat_Sheet.html).

Für den SSH-Tunnel auf dem Arbeitsplatz:

```sh
ssh -N -L 8087:127.0.0.1:8087 BENUTZER@ANKER-SERVER
```

Danach `http://127.0.0.1:8087` öffnen. Die Netzwerkverbindung wird bereits durch SSH verschlüsselt. Den Tunnel für die Nutzung offen lassen. Ein Reverse Proxy ist für diese Wege nicht erforderlich; Proxy-Terminierung ist in dieser Fassung kein getesteter Installationsweg.

## Anker im Container und Speicher

Für etwa 40 Hosts ist ein unprivilegierter Debian-LXC mit 2 vCPU, 4 GiB RAM, 512 MiB Swap und 16 GiB Systemlaufwerk ein sinnvoller Startpunkt. Die Backup-Ablage separat unter `/srv/anker` einbinden, beispielsweise zunächst mit 200 GiB. Das sind Planungswerte, keine gemessenen Mindestanforderungen: zusätzlicher Configumfang, Aufbewahrung und Archivierung bestimmen den tatsächlichen Bedarf. Vier parallele Aufträge sind voreingestellt. Für die Speicheranzeige braucht der Container weder privilegierten Betrieb noch durchgereichte Blockgeräte.

Unter **Einstellungen → Betrieb → Speicher** sehen Administratoren die Dateisysteme des Anker-Servers: Größe, Belegung, verfügbarer Platz, reservierter Platz, Inodes und eingebundene Pfade. Ein gemeinsames Dateisystem für System und Backups wird einmal angezeigt. Die Werte umfassen auch andere Dateien auf demselben Laufwerk. Andere Proxmox-VMs und Container werden hier nicht überwacht; der freie Platz im Proxmox-Speicherpool ist aus dem Container nicht bestimmbar.

Anker misst stündlich und speichert bis zu 90 Tage Verlauf im Katalog. Eine Schätzung bis zur Vollbelegung erscheint erst nach mindestens 24 Messungen über drei Tage mit ausreichend gleichmäßigem Wachstum. Sie verwendet die letzten 14 Tage und den tatsächlich verfügbaren Platz. Ohne belastbaren Trend oder bei veralteten Messungen erscheint kein scheinbar genaues Datum. Nach einer Vergrößerung beginnt die Messreihe mit der neuen Kapazität. Die Demo liest keine echten Laufwerke und enthält keine erfundenen Speichermessungen.

**Erweitern** prüft den Aufbau und zeigt die nächsten Schritte:

- Im LXC: CT-ID, `rootfs` oder `mp`-Eintrag und zusätzliche GiB angeben. Den erzeugten `pct resize`-Befehl auf dem zuständigen Proxmox-Host ausführen. Bindmounts über das Dateisystem auf dem Host erweitern.
- In einer VM oder auf einem physischen Server: zuerst Disk, Partition beziehungsweise LVM-Volume gezielt vergrößern. Hat das Blockgerät bereits zusätzlichen Platz, kann Anker ein eindeutig zugeordnetes ext4- oder XFS-Dateisystem nach Mountpoint-Bestätigung erweitern.
- Neues Laufwerk: der Assistent bietet eine kopierbare und herunterladbare Anleitung mit UUID-Prüfung, gestoppten Diensten, Datenübernahme und Mountkontrolle. Formatierung und der eigentliche Umzug erfolgen bewusst durch den Administrator.

Anker formatiert keine Laufwerke, verändert keine Partitionstabellen und verkleinert keine Dateisysteme. Eine unterbrochene Erweiterung wird nach Neustart als unterbrochen angezeigt und nicht automatisch wiederholt. Der tatsächliche Zustand lässt sich neu prüfen. Updates, Einrichtung und Erweiterungen sind gegeneinander gesperrt.

Die Anzeige verwendet `lsblk` aus util-linux. Für Dateisystemerweiterungen müssen `e2fsprogs` beziehungsweise `xfsprogs` und der root-Hilfsdienst `anker-updater.service` vorhanden sein. Der manuelle Umzug benötigt zusätzlich `rsync`, `findmnt`, `blkid` und `mountpoint`. Fehlende Werkzeuge erscheinen als Fehler; Anker installiert sie nicht während einer Speicheraktion nach.

Auch im Terminal verfügbar:

```sh
sudo anker storage status
sudo anker storage state
sudo anker storage plan VOLUME-ID
# PLAN-ID und Mountpoint aus dem geprüften Plan übernehmen:
sudo anker storage grow VOLUME-ID --plan PLAN-ID --confirm /srv/anker
```

Vor Umzug, bei Laufwerksausfall und bei SFTP-Bindmounts die [Betriebsanleitung](../OPERATIONS.md) beachten. Der [Prüfumfang](../SUPPORT.md) unterscheidet lokale Tests, Linux-Prüfungen und noch offene Proxmox-Abnahme.

## Anmeldung und Benutzer

Standardmäßig brauchen neue und geänderte Passwörter mindestens **acht Zeichen**. Unter **Einstellungen → Zugriff → Passwort-Mindestlänge** können Administratoren 8 bis 128 Zeichen einstellen. Eine höhere Vorgabe gilt bei der nächsten Benutzeranlage oder Passwortänderung; bestehende Passwörter und Sitzungen bleiben bis dahin nutzbar. Das Passwort selbst über **Benutzer → Aktionen → Passwort ändern** setzen, auch für den eigenen Administrator. Ein tatsächlicher Passwortwechsel beendet die bisherigen Sitzungen.

Die Weboberfläche verlangt eine Anmeldung. Standardmäßig gilt sie 30 Tage, einstellbar von 1 bis 365 Tagen für neue Sitzungen. Anmeldung bleibt über Dienstneustarts erhalten. Abmelden, Passwort- oder Rechteänderungen sowie Sperren und Löschen beenden die betroffenen Sitzungen.

Unter „Einstellungen → Zugriff“ weitere Benutzer anlegen:

- **Lesen:** Hosts, Aufträge und freigegebene Dateien ansehen und herunterladen.
- **Wiederherstellung:** Zusätzlich Sicherungen starten und Wiederherstellungen ausführen.
- **Administrator:** Zusätzlich Benutzer, Einstellungen und Updates verwalten.

Secrets und vollständige Exporte brauchen eine eigene Freigabe. Der eigene Administrator und der letzte aktive Administrator sind gegen Aussperren geschützt. Der lokale Unix-Socket erlaubt root und dem Dienstbenutzer Administration ohne Webpasswort; er darf nicht über das Netz freigegeben werden. Passwort zurücksetzen: [Betrieb](../OPERATIONS.md).
