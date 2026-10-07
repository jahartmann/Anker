# Anker

Anker sichert die **Konfiguration von Proxmox-Hosts** zentral über SSH. Die Sicherungen bleiben als lesbare Ordner verfügbar. Weboberfläche, Terminaloberfläche und Befehle verwenden denselben Dienst.

VM- und Containerdaten gehören weiterhin in den Proxmox Backup Server. Anker sichert Hostkonfigurationen, keine kompletten Festplattenimages. Für den zentralen Dienst einen eigenen Linux-Server, eine VM oder einen LXC-Container mit systemd verwenden.

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

Das Skript erkennt die Architektur, lädt das Paket, prüft Signatur und SHA-256 und startet dieselbe Einrichtung. Es benötigt zusätzlich OpenSSL 3. Den öffentlichen Signierschlüssel vor der ersten Verwendung unabhängig prüfen. Details zu privaten Repositories und Release-Erstellung stehen in [UPDATES.md](docs/UPDATES.md).

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

Unter **Einstellungen → System → Webzertifikat** Ablaufdatum, Webadressen und SHA256-Fingerprint ansehen, das öffentliche Zertifikat herunterladen, Automatik ein-/ausschalten und den Vorlauf zwischen 7 und 90 Tagen einstellen. „Jetzt erneuern“ ist nach Bestätigung jederzeit möglich. Änderungen erfordern Administratorrechte; der letzte Automatikfehler bleibt bis zur erfolgreichen Prüfung/Erneuerung sichtbar. Die Einstellungen bleiben nach Dienst- und Containerneustarts erhalten.

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

Unter **Einstellungen → Speicher** sehen Administratoren die Dateisysteme des Anker-Servers: Größe, Belegung, verfügbarer Platz, reservierter Platz, Inodes und eingebundene Pfade. Ein gemeinsames Dateisystem für System und Backups wird einmal angezeigt. Die Werte umfassen auch andere Dateien auf demselben Laufwerk. Andere Proxmox-VMs und Container werden hier nicht überwacht; der freie Platz im Proxmox-Speicherpool ist aus dem Container nicht bestimmbar.

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

Vor Umzug, bei Laufwerksausfall und bei SFTP-Bindmounts die [Betriebsanleitung](docs/OPERATIONS.md) beachten. Der [Prüfumfang](docs/SUPPORT.md) unterscheidet lokale Tests, Linux-Prüfungen und noch offene Proxmox-Abnahme.

## Proxmox-Hosts anbinden

In der Weboberfläche **Hosts → Host hinzufügen** öffnen. Adresse, SSH-Benutzer und dessen Passwort eingeben; `root` ist vorbelegt. Anker zeigt zuerst den Ed25519-Fingerprint des SSH-Servers. Auf der Proxmox-Konsole mit folgendem Befehl vergleichen und im Formular bestätigen:

```sh
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256
```

Danach installiert Anker den Hosthelfer, richtet die Benutzer `anker` und `anker-restore` ein und hinterlegt die beiden öffentlichen Schlüssel. Die privaten Schlüssel bleiben auf dem Anker-Server. Erst wenn beide Zugänge ohne Passwort funktionieren und das Proxmox-Inventar lesbar ist, wird der Host gespeichert. Der Hostname wird automatisch übernommen; eigener Anzeigename, Gruppe, Cluster und SSH-Port sind optional. Anschließend **Jetzt sichern** wählen. Der tägliche Zeitplan ist bei neuen Hosts bereits eingeschaltet.

Das Passwort wird einmal zur SSH-Anmeldung und gegebenenfalls für `sudo` verwendet. Es wird weder im Katalog noch in Aufträgen oder Protokollen gespeichert. Ein anderer SSH-Benutzer als root braucht sudo-Rechte für die Installation; Anker unterstützt denselben Passwortzugang oder passwortloses sudo. Passwort- und übliche PAM-Passwortanmeldung sind unterstützt; interaktive MFA benötigt die manuelle Einrichtung. Falls sudo auf dem Host fehlt, versucht Anker es über dessen vorhandene APT-Quellen zu installieren.

Für vorhandene Hosts **Verbindung einrichten** verwenden. Bestehende Hostprofile und fremde SSH-Schlüssel bleiben erhalten. Nach einem Fehler Zugang prüfen und erneut versuchen; der Installer kann wiederholt werden. Ein bereits bekannter, geänderter Hostschlüssel wird blockiert. Erst nach unabhängiger Prüfung der Änderung den Eintrag ausdrücklich mit `anker host trust` aktualisieren. Den neuen Schlüssel niemals nur aufgrund einer Fehlermeldung übernehmen.

Im Terminal `sudo anker` öffnen: unter Hosts startet `a` die automatische Anbindung, `v` richtet den ausgewählten Host erneut ein. Die Fingerprintbestätigung erfolgt vor dem Übermitteln des Passworts. `m` öffnet die manuelle Anlage.

### Manuell anbinden

Für bestehende Schlüsselverwaltung oder Hosts ohne SSH-Passwortanmeldung bleibt **Manuell einrichten** verfügbar. Nur die öffentlichen Dateien `/etc/anker/keys/backup.pub` und `/etc/anker/keys/restore.pub` zusammen mit dem Projekt- oder Release-Paket auf den Zielhost übertragen und dort ausführen:

```sh
sudo ./scripts/install-host.sh --backup-key /pfad/backup.pub --restore-key /pfad/restore.pub
```

Den unabhängig geprüften SSH-Fingerprint und Host anschließend auf dem Anker-Server hinterlegen:

```sh
sudo anker host trust HOSTADRESSE --fingerprint SHA256:FINGERPRINT
sudo anker host add --name HOSTNAME --address HOSTADRESSE --restore-key /etc/anker/keys/restore
sudo anker host probe HOST-ID
```

Bei einem anderen SSH-Port beiden Einrichtungsbefehlen `--port PORT` hinzufügen. Standardzugänge verwenden `anker`, `anker-restore` und `/etc/anker/known_hosts`. Abweichende Schlüsselpfade können im manuellen Formular oder mit `--key`, `--known-hosts`, `--restore-key` und `--restore-user` angegeben werden. `host probe` startet einen Auftrag; dessen Ergebnis in **Aufträge** ansehen. Für Planung, Downloads und manuelle Wiederherstellung reicht der Sicherungszugang; automatische Übernahme verwendet immer den getrennten Restorezugang.

Passwortlose Synchronisation verwendet **SSH-Schlüssel**. SSH-Zertifikate oder eine zusätzliche SSH-CA sind dafür nicht erforderlich. TLS-Zertifikate sichern den Webzugriff.

Das Hostprofil `/etc/anker-host.json` sichert standardmäßig `/etc` und `/usr/local`. Zusätzliche Konfiguration beispielsweise unter `/opt` ausdrücklich im Profil und als Pflichtpfad in Anker ergänzen. Das Profil muss root gehören und darf nicht von anderen beschreibbar sein. VM-Datenträger und große Anwendungsdaten nicht in das Configprofil aufnehmen.

### Zeitplan und Aufträge

Unter **Einstellungen → Sicherung** stehen tägliche Startzeit, Zeitzone, Parallelität und Wiederholungen. Ein Host übernimmt diese Zeit oder bekommt im Hostformular eine eigene Startzeit. „Automatisch sichern“ pausiert nur den Zeitplan; „Jetzt sichern“ bleibt möglich. Anker prüft jede Minute selbst, ob ein Lauf fällig ist. Keine Crontab anlegen. Hosts starten mit einem festen Versatz von weniger als einer Stunde, spätestens um 23:59 Uhr. Die Demo führt keine automatischen Sicherungen aus.

**Aufträge → Details** zeigt Auslöser, Erstellzeit, tatsächlichen Start, Versuche und Fehler. Von dort die fertige Sicherung öffnen, einen aktiven Auftrag abbrechen oder eine fehlgeschlagene Sicherung/Hostprüfung erneut starten. Administratoren können abgeschlossene Einträge entfernen; Sicherungen und Tagesmarker bleiben erhalten. Wiederherstellungen erneut über einen frisch geprüften Plan bestätigen. Bereits eingeplante Tagesläufe werden nach Zeitplanänderung oder Neustart nicht nochmals gestartet. Ausgefallene frühere Tage werden nicht nachträglich rekonstruiert.

```sh
sudo anker jobs
sudo anker job show AUFTRAG
sudo anker job cancel AUFTRAG
sudo anker job retry AUFTRAG
sudo anker job remove AUFTRAG
```

## Sicherungen und Inventar im Web

Unter **Sicherungen** nach Host, Status, Schutz oder Archiv filtern; die Suche findet Hostnamen und Sicherungs-IDs. Die Liste zeigt 25 oder 50 Stände pro Seite. Laufende Sicherungen stehen separat mit Host, Start und Laufzeit. **Aufträge** trennt aktive Arbeit vom Verlauf und lässt sich zusätzlich nach Auftragstyp durchsuchen.

**Dateien** öffnet die Ordner des gewählten Stands. Über die Pfadleiste zurückgehen oder mit **Pfad suchen** über alle Unterordner suchen. Die Treffer haben ebenfalls feste Seiten. Eine Datei auswählen, um sie anzusehen, herunterzuladen oder zur Wiederherstellung zu übernehmen. Verknüpfungen zeigen ihr Ziel; geschützte Inhalte benötigen weiterhin eine ausdrückliche Freigabe. Hinweise zur Erfassung sind aufklappbar.

Im **Inventar** stehen System und Cluster zuerst. Netzwerk und Datenträger haben getrennte Suchen und Seiten; Kennungen und weitere erfasste Daten lassen sich bei Bedarf öffnen.

Unter **Einstellungen → Sicherung → Aufbewahrung** gelten die Tages-, Wochen- und Monatsregeln einzeln je Host: jeweils der neueste erfolgreiche Stand eines Zeitraums bleibt. Ein Stand kann mehrere Regeln erfüllen; die Zahlen werden nicht addiert. Der letzte erfolgreiche Stand, geschützte/unvollständige/beschädigte Stände und Quellen von Wiederherstellungsplänen bleiben erhalten. Automatische Archivierung lässt sich ausschalten; komprimierte Stände unterliegen weiterhin der Aufbewahrung. **Überfällig nach Stunden** steuert nur den Hosthinweis und löscht nichts.

## Einrichtung im Überblick

| Schritt | Vereinfachung oder Grund für die manuelle Angabe |
| --- | --- |
| Server installieren | Ein Installer wählt die Architektur und öffnet den Assistenten. Benutzer, Ordner, Rechte und Dienste werden eingerichtet. |
| Zugang anlegen | Ein eigener Administrator; kein Standardpasswort. Vorhandene Zugänge werden bei Wiederholung erkannt. |
| Webzugriff | HTTPS mit erzeugtem Zertifikat oder vorhandener CA; alternativ SSH-Tunnel ohne TLS-Einrichtung. Adresse und erstes Vertrauen kann Anker nicht sicher erraten. |
| Updates | Der Anker-Schlüssel wird mitgeliefert. Quelle bei Bedarf direkt im Web bestätigen; öffentliche Downloads brauchen keinen Token. |
| Hostzugang | Adresse und einmaligen SSH-Zugang eingeben; Anker installiert Helfer und beide eingeschränkten Schlüsselzugänge und prüft sie. |
| Hostidentität | Fingerprint im Formular unabhängig vergleichen und bestätigen; bekannte Schlüsselwechsel werden blockiert. |
| Betrieb | Gemeinsamer Zeitplan, Aufbewahrung und 30-Tage-Anmeldung sind vorbelegt. Benachrichtigungen und Restorezugänge nur bei Bedarf einrichten. |

## Anmeldung und Benutzer

Standardmäßig brauchen neue und geänderte Passwörter mindestens **acht Zeichen**. Unter **Einstellungen → Zugriff → Passwort-Mindestlänge** können Administratoren 8 bis 128 Zeichen einstellen. Eine höhere Vorgabe gilt bei der nächsten Benutzeranlage oder Passwortänderung; bestehende Passwörter und Sitzungen bleiben bis dahin nutzbar. Das Passwort selbst über **Benutzer → Aktionen → Passwort ändern** setzen, auch für den eigenen Administrator. Ein tatsächlicher Passwortwechsel beendet die bisherigen Sitzungen.

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

Über SSH als root einfach `anker` öffnen; `anker tui` bleibt ebenfalls verfügbar. Tab oder 1–5 wechseln die Bereiche, Pfeile und Enter öffnen Einträge. Die unten angezeigten Aktionen lassen sich per Taste oder Maus wählen. Hostanlage, Zeitplan, Benutzer und Exporte haben Eingabeformulare. Sicherung, Änderungen und Wiederherstellung werden vor dem Ausführen bestätigt; bei einem Restore muss zusätzlich die vollständige Plan-ID eingegeben werden. Esc geht zurück, `q` beendet die Oberfläche. Bei umgeleiteter Ausgabe zeigt `anker` die Hilfe statt einer Terminaloberfläche.

Das kurze Handbuch ist auch ohne laufenden Dienst erreichbar:

```sh
anker -help
man anker
```

Der Installer richtet die Manpage automatisch ein. In den Formularen wechselt Tab das Feld; Enter geht weiter, Ctrl+S prüft die Angaben. Passwörter werden verdeckt eingegeben. Lokale Exporte bleiben auf dem Rechner, auf dem die Terminaloberfläche läuft; bei SSH also auf dem Anker-Server.

Unter Hosts öffnet `h` die SSH-Schlüsselprüfung. Den Fingerprint vorher auf der Proxmox-Konsole prüfen; Anker speichert nur einen passenden Schlüssel in `/etc/anker/known_hosts`. Unter System → Speicher führt `g` durch Laufwerksauswahl und Erweiterungsplan. Eine unterstützte Erweiterung startet erst nach Eingabe des vollständigen Mountpoints; in LXC und bei manuellen Plänen bleiben die Hinweise zur Erweiterung auf dem Host sichtbar.

Updates unter **Einstellungen → System → Updates** einrichten, prüfen und installieren. Fehlt die Quelle, „Updatequelle einrichten“ wählen, die mitgelieferte Anker-Quelle bestätigen und anschließend nach Updates suchen. Dafür ist kein Terminalbefehl nötig. Eine eigene Quelle kann mit ihrem unabhängig geprüften öffentlichen Ed25519-Schlüssel angegeben werden. Nur Administratoren dürfen die Quelle ändern; der Dialog zeigt den Fingerprint vor dem Speichern. Der Schlüssel wird nicht aus einem ungeprüften Release nachgeladen.

Alternativ `sudo anker update check`, danach `sudo anker update install`. Signatur und SHA-256 werden vor der Installation geprüft. Laufende Sicherungen und Wiederherstellungen blockieren das Update. Bei fehlgeschlagenem Start stellt der Updater die vorherige Version und den Katalog wieder her. [Einrichtung und Rückfall](docs/UPDATES.md).

Nach einem im Web gestarteten Update lädt die Seite automatisch neu, sobald die neue Version die Startprüfung bestanden hat. Das funktioniert auch bei einem Seitenwechsel und kurzer Nichterreichbarkeit des Dienstes. Bei einem fehlgeschlagenen Update bleibt die Seite bestehen und zeigt den Rückfall. Ungespeicherte Einstellungen vor dem Update speichern. Beim ersten Update von einer älteren Oberfläche auf 0.2.4 die Seite einmal selbst neu laden; die Automatik gilt ab der neuen Oberfläche.

## Wiederherstellung und Grenzen

Gesamtrecovery, Hardwaremigration und Cluster-/Versionswechsel sind derzeit **manuell geführte Pläne**. Automatische Gesamtausführung bleibt gesperrt, bis die Proxmox-, Hardware- und Clusterfälle im Labor geprüft sind. Reboot, Storage, Quorum, HA und PBS-Erreichbarkeit separat bestätigen.

Im Web führt der Assistent über **Quelle und Dateien → Ziel prüfen → Zuordnungen → Plan prüfen**. Er zeigt nur verwendete physische Ports und referenzierte Speicher. Bridges, Bonds, VLANs und Aliase behalten ihre Abhängigkeiten; unbeteiligte Laufwerke werden nicht zugeordnet. Speicher, gewünschter Hostname und Adresse bleiben bewusst dokumentierte manuelle Entscheidungen. Dateien, Planschritte und Nachweise sind durchsuchbar und in Seiten aufgeteilt. Die Dateiansicht zeigt Anpassungen an der Sicherung, keinen vollständigen Inhaltsvergleich mit dem Zielhost.

Eine automatische Dateiübernahme prüft Inhalte, Rechte und Benutzeridentitäten und führt ein dauerhaftes Hostjournal. Bei einem Schreibfehler werden eigene Änderungen kontrolliert zurückgesetzt; fremde Änderungen bleiben erhalten. Nach einem Verbindungsabbruch **Hostzustand abgleichen**, statt erneut anzuwenden. **Dateien zurücksetzen** verlangt eine separate Plan-ID-Bestätigung. Netzwerk- und Zugangskonfiguration bleiben manuell; Netzwerk wird nicht automatisch aktiviert.

Nach dem Update auf 0.2.8 vorhandene Hosts einmal über **Verbindung neu einrichten** aktualisieren und neu sichern. Automatische Übernahmen verlangen den aktuellen Hosthelfer und erfasste Eigentümeridentitäten. Bestehende Sicherungen bleiben lesbar und exportierbar. [Ablauf und Fehlerbehandlung](docs/RECOVERY.md).

Im Terminal sind alle Wiederherstellungsszenarien auswählbar. Im Dateifeld wechselt `Ctrl+E` zwischen der Liste und direkter Eingabe relativer Pfade; mehrere Pfade mit Komma trennen. Vollständige Szenarien bleiben auf echten Hosts manuell geführt. Mit `r` im geöffneten Plan den Hostzustand abgleichen, mit `b` die kontrollierte Rücksetzung separat bestätigen. Das Planpaket enthält vorbereitete Dateien und unter `original/original-files/` die Originale mit ihren gesicherten Metadaten.

CLI: `anker restore inspect --backup ID --target HOST --files etc/test.conf`, `anker restore status PLAN` und `anker restore rollback PLAN --confirm PLAN`. `restore plan` unterstützt außerdem `--storage ID=manual`, `--hostname NAME` und `--address IP` für dokumentierte manuelle Entscheidungen.

Der [Abgleich mit dem geplanten Umfang](docs/IMPLEMENTATION_STATUS.md) nennt die noch offenen Wiederherstellungsregeln, Clusterfunktionen und Unterschiede zwischen Web und Terminal.

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
