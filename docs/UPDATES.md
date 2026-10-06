# Installation und Updates

Der Updateweg gilt für den zentralen Anker-Server auf Linux mit systemd. Die Proxmox-Helfer werden damit nicht automatisch ausgetauscht. Das aktuelle Hostprotokoll bleibt Version 1; inkompatible Releases werden abgelehnt.

## Repository vorbereiten

Ein öffentliches GitHub-Repository ohne zusätzliche Schreibberechtigte reicht aus. Besucher können den Code herunterladen oder einen Fork erstellen. Sie können nicht in das Original pushen. MIT erlaubt ausdrücklich Änderungen und Weitergabe eigener Kopien; die Schreibrechte am Original bleiben davon unberührt.

Vor dem ersten Release:

1. Das öffentliche Repository `jahartmann/Anker` anlegen beziehungsweise den vorhandenen Projektstand dort übernehmen. Noch keine produktiven Backups, Schlüssel oder Configdateien einchecken.
2. `main` gegen Force-Push und Löschen schützen. Für Änderungen bestandene „Checks“ verlangen. Schreibrechte nur gezielt vergeben. Tags `v*` über eine Ruleset-Regel auf Maintainer beschränken.
3. In „Settings → Security“ private vulnerability reporting und die verfügbaren Secret-Scanning-Funktionen aktivieren.
4. Die Actions-Umgebung `release` anlegen, auf Release-Tags beschränken und – falls verfügbar – eine Maintainer-Freigabe verlangen. Fork-Pull-Requests erhalten den Signierschlüssel nicht.
5. Ein Signierschlüsselpaar außerhalb des Repositorys erzeugen:

```sh
go run ./cmd/anker-release -mode keygen -dir /sicherer/pfad/anker-release-keys
```

`private.key` enthält den privaten Ed25519-Schlüssel als Base64. Als Secret `ANKER_SIGNING_KEY` in der Umgebung `release` hinterlegen. Den privaten Schlüssel offline sichern. `public.key` und `public.pem` dürfen veröffentlicht werden; ihren Fingerprint zusätzlich über einen vertrauenswürdigen Kanal bekanntgeben.

```sh
openssl pkey -pubin -in /sicherer/pfad/anker-release-keys/public.pem -outform DER | openssl dgst -sha256
```

Der Server braucht nur `public.key`. Ein öffentlicher Release braucht keinen GitHub-Zugangsschlüssel.

## Erstinstallation aus einem Release

Der geführte Weg benötigt einen veröffentlichten stabilen Release. Aus einem bekannten Projektstand auf dem Linux-Server:

```sh
sudo python3 scripts/install-release.py --public-key /pfad/zur/geprüften/public.pem
```

Der Installer erkennt amd64/arm64, lädt über die GitHub-API, prüft Signatur, Version, Architektur, Größe und SHA-256 und entpackt erst danach. Links, Gerätedateien und ausbrechende Archivpfade werden abgelehnt. Anschließend fragt Anker die Ersteinrichtung ab, erzeugt die beiden SSH-Schlüssel und aktiviert die Dienste. Der bereits vertraute öffentliche Schlüssel wird für Updates übernommen. Repository und Schlüssel werden im normalen Assistenten nicht erneut abgefragt; Änderungen über `sudo anker setup --updates`. Pakete enthalten ausschließlich den öffentlichen Schlüssel, keine private Signieridentität.

Für ein privates Repository:

```sh
sudo python3 scripts/install-release.py --public-key /pfad/zur/geprüften/public.pem --repo OWNER/Anker --private
```

Der GitHub-Token wird verdeckt abgefragt. Ein Fine-grained Token mit Zugriff nur auf dieses Repository und **Contents: read-only** reicht für Release-Downloads. Er wird root-eigen mit `0600` gespeichert und bei Weiterleitungen zu Asset-Storage nicht übertragen. Alternativ `--token-file /etc/mein-geschützter-token`. Bei Ablauf oder Widerruf des Tokens scheitert die Updateprüfung; Anker selbst und vorhandene Sicherungen bleiben lokal nutzbar. Privates GitHub ist jederzeit vom Server erreichbar, solange Token, Repositoryberechtigung und Netzwerk verfügbar sind. SSH-Deploy-Keys erlauben einen Quellcode-Clone, ersetzen aber nicht den HTTP-Token für die Release-API.

Die Einrichtung kann mit `sudo anker setup` wiederholt werden. Bestehende Benutzer und private SSH-Schlüssel werden erhalten; unvollständige öffentliche Schlüssel und Dateirechte werden repariert. Die Einrichtung sperrt den Updater während Änderungen. Bei der ersten Einrichtung wird HTTPS für LAN/VPN vorgeschlagen; der Assistent erzeugt auf Wunsch ein Zertifikat für den angegebenen DNS-Namen oder die IP. Ein selbst erzeugtes Zertifikat braucht eine unabhängige Fingerprintprüfung im Browser. Alternativ vorhandene CA-Dateien importieren oder den lokalen SSH-Tunnel wählen. Gültige bestehende TLS-Dateien bleiben bei Wiederholung erhalten. Details zu Vertrauen und Erneuerung stehen in der README.

Unterbrechung während der Installation: Solange `/etc/anker/install-pending` existiert, dasselbe geprüfte Installationsskript erneut ausführen. Der Hauptprogrammwechsel erfolgt erst nach synchronisierten Grundlagen. Ist die Installation abgeschlossen, `anker setup` verwenden; der Installer ersetzt keine bestehende Installation. Eine reine Quellcode-Installation unterstützt `scripts/install-server.sh --no-setup` für automatisierte Abläufe.

Manueller Paketweg:

Zum Prozessor passendes Paket, `release.json` und `release.json.sig` herunterladen. `amd64` gilt für übliche Intel-/AMD-Server, `arm64` für 64-Bit-ARM.

Den öffentlichen Schlüssel vor der ersten Installation unabhängig prüfen. Ein Schlüssel aus demselben ungeprüften Download beweist keine Identität. Auch das Prüfskript aus einem bekannten Projektstand verwenden. OpenSSL 3 und Python 3 reichen für die Paketprüfung; kein Go oder Node auf dem Server nötig.

```sh
/path/zum/geprüften/projekt/scripts/verify-release.sh /path/zum/geprüften/public.pem anker-linux-amd64.tar.gz
mkdir anker-install
# Erst nach bestandener Prüfung entpacken.
tar -xzf anker-linux-amd64.tar.gz -C anker-install
cd anker-install
sudo ./scripts/install-server.sh ./anker
```

Das Installationsskript öffnet die Einrichtung automatisch. Der Installer überschreibt eine bestehende Anker-Installation nicht. Eine ältere Entwicklungsinstallation zuerst im Wartungsfenster stoppen und nach Sicherung des Katalogs auf diesen Installationsstand bringen.

## Updater auf dem Server einrichten

`/etc/anker/update.json` für `jahartmann/Anker` mit dem Inhalt von `public.key` erstellen:

```json
{
  "repository": "jahartmann/Anker",
  "public_key": "BASE64_PUBLIC_KEY"
}
```

```sh
sudo chown root:anker /etc/anker/update.json
sudo chmod 0640 /etc/anker/update.json
sudo systemctl enable --now anker-updater.service
sudo anker update check
```

`BASE64_PUBLIC_KEY` durch den Inhalt von `public.key` ersetzen. Der Updater akzeptiert keine Konfigurationsdatei, die andere Benutzer beschreiben können. Bei einem privaten Repository kann zusätzlich `token_file` auf eine root-eigene Datei mit einem passenden GitHub-Lese-Token verweisen. Den Token nicht in die Weboberfläche oder die Befehlszeile kopieren.

Der Webdienst läuft als `anker`. Der Updater läuft als `root`, nimmt ausschließlich `status`, `check` und `install` über `/run/anker-updater/socket` entgegen und liest Zielpfade, Repository und Signierschlüssel aus seiner lokalen Konfiguration. Es gibt keinen allgemeinen Shell- oder Download-Endpunkt. Mitglieder der Gruppe `anker` haben damit administrative Updateberechtigung; keine gewöhnlichen Benutzer dieser Gruppe hinzufügen.

## Release erstellen

`CHANGELOG.md` aktualisieren, den geprüften Stand auf `main` übernehmen und einen Tag setzen:

```sh
git tag v0.2.0
git push origin v0.2.0
```

GitHub Actions führt die Tests aus, baut die Weboberfläche ins Binary ein und erstellt Linux-Pakete für beide Architekturen. Der Release enthält Binärdateien, Installer, systemd-Units, Hosthelfer, Dokumentation und Lizenzhinweise. `release.json` enthält Größe und SHA-256 der Binärdateien und Pakete; `release.json.sig` signiert das komplette Manifest.

Der Workflow erstellt einen **Entwurf**. Paketinhalt und Release-Hinweise prüfen, zuerst auf einem Testserver installieren, dann den Entwurf veröffentlichen. Anker prüft nur veröffentlichte stabile Releases, keine Entwürfe oder Pre-Releases. Veröffentlichte Assets nicht nachträglich ersetzen; für Korrekturen einen neuen Versions-Tag verwenden.

## Update installieren

Web: „Einstellungen → System → Updates → Nach Updates suchen“. Version und Release-Hinweise prüfen, danach „Update installieren“ bestätigen.

Shell:

```sh
sudo anker update check
sudo anker update install
sudo anker update status
```

`install` stößt die Installation an. `status` zeigt den Fortschritt beziehungsweise das Ergebnis. Die Oberfläche prüft den Zustand während eines Updates automatisch erneut. Eine Anmeldung bleibt bis zum ursprünglichen Ablaufdatum erhalten.

Ablauf:

1. Signatur, Release-Tag, Format und Plattform prüfen.
2. Download begrenzen, Dateigröße und SHA-256 prüfen. Freien Platz für Staging, Katalogkopie und Rückfall prüfen, dann die Binärdatei auf dem Ziellaufwerk vorbereiten.
3. Neue Aufträge sperren. Bei aktiven Aufträgen oder laufender Aufbewahrungsprüfung abbrechen; keine Wiederherstellung unterbrechen.
4. Dienst stoppen. Vorherige Binärdatei und Katalog einschließlich eines gegebenenfalls verbliebenen SQLite-WAL im root-eigenen `/var/lib/anker-updater` sichern.
5. Binärdatei atomar austauschen, Dienst starten und über den lokalen Socket die tatsächlich laufende Version prüfen. Während der Prüfung bleiben automatische Wartung und Schreibaktionen gesperrt.
6. Bei Erfolg die Sperre aufheben, die separate Updater-Binärdatei atomar erneuern und den Updater neu starten. Bei Fehlern vorherige Binärdatei und Katalog zurückspielen und deren Start ebenfalls prüfen.

Der Updater startet aus der separaten, root-eigenen Datei `/usr/local/libexec/anker-updater`. Diese wird erst nach bestandener Startprüfung erneuert. Ein defektes neues Hauptprogramm kann deshalb den Rückfall nach einem Neustart nicht verhindern.

Die persistente Sperrdatei `/etc/anker/update-maintenance` und ein root-eigenes Journal verhindern neue Aufträge während der Wiederanlaufprüfung nach einem abgebrochenen Update. Der Updater behandelt ein offenes Journal beim nächsten Start, bevor er weitere Installationen annimmt.

Die Wartungssperre wird erst entfernt und das Verzeichnis synchronisiert, wenn der laufende Dienst die Freigabe bestätigt hat. Ist dessen Socket beim Booten noch nicht bereit, bleibt der Marker für den nächsten Versuch erhalten.

## Fehler und Rückfall

```sh
sudo anker update status
sudo journalctl -u anker -u anker-updater -n 100 --no-pager
sudo systemctl restart anker-updater.service
```

Ein Neustart des Updaters stößt bei vorhandenem Journal die Wiederherstellung der vorherigen Version an. Scheitert auch deren Start, bleibt das Journal erhalten; weitere Installationen sind gesperrt. Dann zuerst Dienstkonfiguration, Speicher und Katalog prüfen. Das Journal nicht unbesehen löschen.

Das Verfahren sichert Programm und Katalog, keine komplette Maschine. SSH-Schlüssel, TLS-Dateien, Servicekonfiguration und Backupordner bleiben beim Update unverändert und gehören weiterhin in das betriebliche Sicherungskonzept. Releases mit inkompatiblen Datenformaten brauchen eine gesonderte Migration; der normale Updater führt sie nicht aus.

Lokale Tests prüfen unter anderem beschädigte Downloads, volle Datenträger, defekte Kandidaten, Katalog/WAL-Rückfall und unterbrochene Prozesse. Der GitHub-Workflow ergänzt eine isolierte Linux-VM mit echten systemd-Diensten: interaktive Erst- und Wiederholeinrichtung, Programmwechsel, defektes neues Binary, Journal-Wiederherstellung vor und nach dem Katalogsnapshot, Sitzungen und Dienstneustarts. Das sind Dienst- und Prozessneustarts, kein tatsächlicher Maschinen-Reboot oder Stromausfall. Ein kontrollierter Reboot-/Stromausfalltest auf der Zielumgebung bleibt erforderlich; eine hundertprozentige Garantie lässt sich daraus nicht ableiten.
