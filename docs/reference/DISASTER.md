# Anker-Server wiederherstellen

[Doku-Übersicht](../README.md) · [Zur Anleitung](../OPERATIONS.md)

[Export](#export-und-prüfung-bei-gestopptem-dienst) · [Import](#isolierter-import-und-bewusste-aktivierung) · [Format](#format-1-umfang-und-grenzen)

## Kaltes, verschlüsseltes Disaster-Paket

`disaster` arbeitet lokal ohne laufenden Unix-Socket. Es sichert die Anker-Instanz
(Katalog, lokale Sicherungen und private Recovery-Nachweise), nicht den gesamten
Server und keine Proxmox-VMs. Der Import startet keinen Dienst, Updater oder Restore.
Eine echte Wiederherstellungsabnahme auf isoliertem PVE bleibt separat erforderlich.

### Export und Prüfung bei gestopptem Dienst

Dienst **und Updater** stoppen; auch Einrichtung, Installer und sonstige Schreiber
müssen beendet sein. Beispiel für eine installierte Linux-Instanz:

```sh
sudo systemctl stop anker.service anker-updater.service
sudo anker --data /srv/anker disaster export \
  --config-root /etc/anker --output /sicherer-datentraeger/anker.disaster \
  --passphrase-file /geschuetzt/disaster-passphrase
sudo anker disaster inspect --input /sicherer-datentraeger/anker.disaster \
  --passphrase-file /geschuetzt/disaster-passphrase
```

Die Passphrase-Datei ist eine reguläre Datei ohne Symlink, höchstens 4097 Bytes,
mit ausschließlich Eigentümerrechten (beispielsweise `0600`). Eine abschließende
LF-Zeile wird entfernt; die Passphrase enthält 1–4096 Bytes. Eine lange zufällige
Passphrase getrennt vom Paket verwahren. Ohne `--passphrase-file` fragt Anker sie
verdeckt am Terminal ab; Pipes und Passphrasen als CLI-Argument sind nicht erlaubt.
Der Export überschreibt keine vorhandene Paketdatei. `inspect` authentifiziert
und prüft Inventar sowie enthaltene Sicherungs-/Planbytes in einem privaten temporären
Verzeichnis. Es zeigt Version, Modus, Zähler, Quellen-IDs/Host-IDs, bekannte Manifestdigests,
Abdeckungszustände und allgemeine Migrationsvoraussetzungen; keine Zugangsdaten,
Originalpfade oder privaten Dateiinhalte. Export, Inspect und Import prüfen enthaltenes Material.

### Installation und Sicherungsdaten getrennt bewahren

Neue Exporte verwenden Nutzlastversion 2. Für jede katalogisierte Sicherung enthält
`sources` genau einen Eintrag mit `backup_id`, `host_id`, `manifest_sha` und `state`:

- `included`: Das Paket enthält die tatsächlichen Manifest-/Datei-/Artefaktbytes
  beziehungsweise das geprüfte Archiv. Export, Inspect und Import prüfen diese Bytes.
- `missing`: Das gesamte lokale Sicherungsverzeichnis fehlt. Der Katalog und vorhandene
  private Plan-, Zielaufnahme-, Rezept- und Prüfhistorie bleiben enthalten. Ein bekanntes
  Manifestdigest ist nur eine erwartete Bindung; ein leerer Digest bedeutet unbekannt.
- `separate`: Bewusst ausgelassene Sicherungsdaten. Beim ersten Split-Export werden
  vorhandene lokale Quellen einzeln vollständig geprüft, aber ihre Bytes nicht eingepackt.
  Nach Import/Reexport bleibt diese offene Einstufung bei weiter fehlenden Daten erhalten.
  Sie bestätigt weder eine verfügbare externe Kopie noch eine aktuelle Integritätsprüfung.

Für viele aufbewahrte Sicherungen mit getrenntem Datenträger:

```sh
sudo anker --data /srv/anker disaster export --backup-data separate \
  --config-root /etc/anker --output /sicherer-datentraeger/anker-installation.disaster \
  --passphrase-file /geschuetzt/disaster-passphrase
```

Der Standard `--backup-data include` nimmt alle vorhandenen Quellen auf; ganz fehlende
Quellen bleiben ausdrücklich offen. Ein vorhandenes leeres Verzeichnis, fehlendes
Manifest, beschädigte Dateien, Links oder unsichere private Nachweise führen in beiden
Modi zum Abbruch. Solche Fehler werden nie in `missing` umgedeutet. Der Split-Modus
prüft jeweils eine Quelle innerhalb der unten genannten Grenzen; die gesamte Menge
separater Sicherungen muss nicht in das 256-MiB-Paket passen. Er kopiert keine Daten auf
einen anderen Datenträger und folgt keinen externen Katalogpfaden.

Nach dem isolierten Import stehen dieselben Quellenbindungen im privaten
`disaster-migration.json` und im internen Migrationskatalog. Für `missing`/`separate`
bleiben tatsächliche Bytevollständigkeit, Verfügbarkeit und physische Unabhängigkeit
unbekannt. Alte Prüfzeiten sind historische Angaben, keine Importprüfung ausgelassener
Bytes. Fehlende Daten separat beschaffen, vor Ort gegen Backup-ID, Host-ID und den
bekannten Manifestdigest prüfen und als vollständiges reguläres Verzeichnis unter
`hosts/HOST/backups/BACKUP` bereitstellen; keine bestehenden Daten überschreiben und
keine Symlinks einsetzen. Erst nach Prüfung aller Migrationsvoraussetzungen aktivieren,
Quellen mit `anker backup verify BACKUP` prüfen und neue Recovery-Pläne erstellen.
Alte Pläne und Zielaufnahmen bleiben auch nach Nachlieferung dauerhaft widerrufen.

### Isolierter Import und bewusste Aktivierung

Das Elternverzeichnis muss als echtes Verzeichnis bereits existieren. In diesem Verzeichnis `pwd -P` verwenden und den neuen Zielnamen an diesen kanonischen Pfad anhängen. Unter macOS ist `/private/tmp` der reale Elternpfad, `/tmp` ein abgewiesener Symlink. Die Beschränkung bleibt bestehen. Das Ziel darf **noch nicht
existieren**, auch nicht als leeres Verzeichnis oder Symlink. Es darf nicht
innerhalb einer vorhandenen Anker-Instanz liegen:

```sh
sudo anker disaster import --input /sicherer-datentraeger/anker.disaster \
  --destination /srv/anker-isoliert --passphrase-file /geschuetzt/disaster-passphrase
```

Anker prüft zuerst Authentifizierung, Inventar, Originalmanifeste, Sicherungsdaten,
private Pläne und Zielnachweise in einem privaten temporären Verzeichnis. Erst
nach erfolgreicher Prüfung wird eine vollständig geschriebene Ablage atomar und
ohne Überschreiben veröffentlicht. Verzeichnisse erhalten `0700`, Dateien `0600`.
Import unter dem künftigen Dienstkonto ausführen oder anschließend Eigentümer der
neuen Ablage bewusst an dieses Konto übertragen. UID/GID und Originalmodi in
Sicherungsmanifesten werden dabei unverändert erhalten.

Die Datei `.disaster-isolated` verhindert Dienststart, Demo-Start und Initialisierung.
Alle Hosts sind deaktiviert; Sitzungen sind gelöscht; wartende/laufende Aufträge
sind `interrupted`. Bestehende Plan- und Zielaufnahme-Dateien bleiben unverändert
lesbare Belege. Separate Katalogsperren verhindern die Wiederverwendung alter
Pläne für Apply, Reconcile und Rollback sowie alter Zielaufnahmen für neue Pläne,
auch nach Aktivierung. Neue Zielprüfung und neuer Plan sind erforderlich.

Vor Aktivierung auf dem **isolierten** Rechner:

1. `disaster-migration.json` lokal unter Eigentümerrechten lesen. Dort stehen
   ursprüngliche Daten-/Konfigurationswurzeln und konkrete externe Referenzen.
   Fehlende Quellhosts werden offline weder kontaktiert noch als verfügbar bestätigt.
2. App-eigene Schlüssel, `known_hosts`, TLS-Dateien und Konfiguration prüfen.
   Material aus `--config-root` liegt unter `disaster-config/` in Quarantäne.
   Die Hostfelder für app-eigene Key-/Trust-Pfade sind auf die neue Ablage umgestellt.
   Externe absolute Pfade werden niemals verfolgt oder automatisch umgeschrieben:
   externe Schlüssel/TLS-Zertifikate separat sicher übertragen, Rechte setzen und
   SSH-Fingerprints über einen unabhängigen Kanal kontrollieren. Ein Schlüssel
   ersetzt keine erneute Vertrauensprüfung am neuen Einsatzort.
3. `service.env` **nicht ungeprüft sourcen**. Für den späteren Dienst eine neue
   Konfiguration mit dem neuen `ANKER_DATA`, einem isolierten Listen-/Socketpfad
   und geprüften `ANKER_TLS_CERT`/`ANKER_TLS_KEY` erstellen. Quarantänisierte
   `service.env`, `update.json` (einschließlich `token_file`) und TLS-Konfiguration
   behalten ihre ursprünglichen Bytes und Pfade; erforderliche Pfade bewusst in
   einer neuen aktiven Konfiguration anpassen. Den Updater separat neu einrichten;
   keine alten Pending-/Installationsdateien übernehmen. Benachrichtigungsziele
   und Zeitpläne vor Netzwerkfreigabe prüfen; Einstellungen wurden erhalten.
4. Erst nach dieser lokalen Prüfung die Aktivierung ausdrücklich vornehmen,
   beispielsweise `sudo rm -- /srv/anker-isoliert/.disaster-isolated`. Dieser
   einzelne Schritt hebt ausschließlich die Startsperre auf. Er startet nichts.
   Für die erste Prüfung Anker manuell mit `--data /srv/anker-isoliert` und eigener
   Loopback-Adresse starten; den erhaltenen Modus beachten (`serve` für production,
   `demo` nur für eine ausdrücklich getrennte Demo). Hosts erst nach Prüfung ihrer
   Adressen/Zugänge einzeln aktivieren. Dienste erst anschließend bewusst einrichten.

### Weitere Disaster-Exporte nach einem Import

Ein normaler Folgeexport mit `--data NEUER_ROOT` nimmt die erlaubten Dateien unter
`NEUER_ROOT/disaster-config/` automatisch wieder auf. Ein erneutes `--config-root`
ist dafür nicht erforderlich. Beispiel bei weiterhin gestoppter Instanz:

```sh
sudo anker --data /srv/anker-isoliert disaster export \
  --output /sicherer-datentraeger/anker-zweite-generation.disaster \
  --passphrase-file /geschuetzt/disaster-passphrase
```

Wird `--config-root` ausdrücklich auf genau dieselbe Quarantäne gesetzt, erkennt
Anker deren geöffnete kanonische Verzeichnisidentität und erfasst sie nur einmal.
Andere überlappende Daten-/Konfigurationswurzeln werden abgelehnt. Auch eine andere,
separate Konfigurationswurzel wird bei bereits vorhandenen Quarantänedateien
abgelehnt: unterschiedliche Schlüssel und Konfigurationen werden nicht still
zusammengeführt oder überschrieben.

Wenn nach Aktivierung beispielsweise neue aktive Dateien unter `/etc/anker`
entstanden sind, historische Quarantäne unverändert aufbewahren. Die aktiven
Schlüssel/Trust-Dateien nach Prüfung unter neuen eindeutigen Namen in
`NEUER_ROOT/keys/active/`, TLS-Dateien in `NEUER_ROOT/tls/active/` und aktive
Anker-Konfiguration in `NEUER_ROOT/config/` kopieren (Verzeichnisse `0700`, Dateien
`0600`, passender Eigentümer). Aktive Host-Key-/Trust-Pfade und die neu geprüfte
Dienst-/TLS-/Token-Konfiguration bewusst auf diese Kopien umstellen; anschließend
bei gestoppten Schreibern ohne `--config-root` exportieren. So enthält der normale
Datenroot-Export sowohl die unveränderten historischen Quarantänedateien als auch
das aktuelle app-eigene Material. Historische `service.env`-/JSON-Dateien behalten
alte Pfade; deren Referenzen können weiterhin als manuell zu prüfende historische
Migrationshinweise erscheinen. Sie werden nicht automatisch aktiv verwendet.

Für jede explizit geöffnete Daten-/Konfigurationswurzel merkt sich das Paket die
angegebene und die dazu geprüfte kanonische Schreibweise, beispielsweise
`/var/...` und `/private/var/...`. Ein app-eigener Key-/Trust-Pfad unter einer dieser
beiden Schreibweisen wird beim Import auf die neue lokale Kopie umgestellt.
Andere, ausschließlich im Katalog genannte Symlink-Aliase werden nicht aufgelöst
und bleiben externe Migrationsreferenzen. Alle Quarantänebytes, deaktivierten
Hosts und dauerhaften Sperren alter Pläne/Zielaufnahmen bleiben auch nach
weiteren Export-/Importgenerationen erhalten.

Der Import ist kein Nachweis einer unabhängigen Zweitkopie oder erfolgreicher
Hostwiederherstellung. Vorhandene Verifikationsdaten und Copy-Receipts behalten
Datum, Herkunft und ursprüngliche Quellenbindung; Export/Import erzeugt keine
neuen Backup-Verifikationsbelege und bestätigt keine Fehlerdomänentrennung.

### Format 1, Umfang und Grenzen

Das binäre Format beginnt mit `ANKER-DISASTER`, NUL und Versionsbyte `0x01`,
anschließend 16 zufälligen Salt-Bytes und 12 zufälligen Nonce-Bytes. scrypt mit
festem `N=32768`, `r=8`, `p=1` erzeugt einen 32-Byte-Schlüssel. AES-256-GCM schützt
JSON-Nutzlast und gesamten Header als Associated Data; der Tag ist 16 Bytes lang.
KDF-Parameter können vom Paket nicht erhöht werden. Ein falsches Passwort,
veränderte Daten oder eine unbekannte Version werden abgelehnt.

Die verschlüsselte JSON-Nutzlast enthält `version`, `created_at`, `source_root`,
optional `config_root`, `source_canonical_root`, `config_canonical_root`,
`mode`, `records`, `files`, `references` sowie ab Nutzlastversion 2 `sources`.
Der verschlüsselte äußere Header bleibt Version 1; neue Leser unterstützen vollständige
alte Nutzlasten der Version 1. Alte Binaries können Version 2 nicht lesen. Die optionalen kanonischen Wurzeln
stammen ausschließlich aus identitätsgeprüften, explizit geöffneten Verzeichnissen;
ältere Pakete ohne diese Zusatzfelder bleiben lesbar. Jeder Katalogeintrag
enthält `bucket`, `id`, seine **unveränderten Rohbytes** als Base64-`value` und
`sha256`. Dadurch bleiben getrennte Credentials, JSON-ausgeblendete/private Felder
und unbekannte Zusatzfelder erhalten. Jeder reguläre Dateieintrag enthält einen
normalisierten relativen `path`, Base64-`data` und `sha256`. Doppelte Pfade/Datensätze,
Traversal, absolute Paketpfade, Datei-/Verzeichniskollisionen, Sonderdateien,
Symlinks und mehrfach verknüpfte Quelldateien werden abgewiesen. Private Plan- und
Zielnachweise müssen schon an der Quelle Dateien `0600`/Verzeichnisse `0700` haben.

Unter dem Datenroot werden `.anker-mode`, `hosts/` (einschließlich vollständiger
Sicherungsdaten oder Archive), `plans/`, `recovery-inspections/`, `recovery-recipes/`, `clusters/`,
`keys/`, `ssh/`, `tls/`, `known_hosts`, `service.env` und erlaubte Anker-Konfiguration
unter `config/` und `disaster-config/` aufgenommen. `--config-root` ist eine explizite zusätzliche Quelle;
zugelassen sind `keys/`, `tls/`, `known_hosts`, `service.env`, `tls-renewal.json`,
`update.json`, `setup-complete`, `release-public.key`, `release.pub`,
`release-key.pub` und reguläre `github-token-*`-Dateien. Die Tooling-Binaries,
Systemd-Units, Logs, Exporte, Staging-Verzeichnisse, Demo-Quellhosts, Sockets und
Updater-/Setup-/Installations-Pendingzustände gehören nicht zum Paket. Externe
Backup-Symlinks werden nicht verfolgt. Ganz fehlende Sicherungsverzeichnisse werden
im Quelleninventar als offen erfasst. Referenzierte, fehlende lokale Schlüssel
oder teilweise vorhandene/beschädigte Sicherungs-/Plan-/Zieldaten führen zum Abbruch; externe
Credential-/Konfigurationspfade werden stattdessen als Migrationspflicht aufgeführt.

Die vorhandene `InstanceLock` schließt laufende Anker-Instanzen aus. Der Katalog
wird samt vorhandenem SQLite-WAL descriptorgebunden in eine private Kopie gelesen,
dort read-only mit `integrity_check` geöffnet und als Rohdatensätze aufgenommen.
Die Quelle erhält kein Schema-Upgrade und keinen erzwungenen Checkpoint. Alle
anderen Schreiber müssen ebenfalls gestoppt sein; die Sperre ersetzt keine
Dateisystem-Snapshotgarantie gegen fremde Prozesse. Dateioperationen verwenden
geöffnete Verzeichnisidentitäten, `O_NOFOLLOW`, Inventarprüfungen und abschließende
Identitätsprüfungen. Der finale Import verwendet descriptorgebundenes Staging
und Linux `RENAME_NOREPLACE` beziehungsweise macOS `RENAME_EXCL`.

Grenzen: 64 MiB je Quelldatei und je DB-/WAL-Datei, 64 MiB rohe Katalogwerte,
256 MiB Dateinutzdaten, höchstens 50.000 Quelldateien/-verzeichniseinträge und
50.000 Katalogdatensätze, Verzeichnistiefe bis 101 Komponenten; maximal 384 MiB
verschlüsseltes Paket einschließlich Base64-/JSON-Aufwand. Sicherungsarchive
werden zusätzlich auf insgesamt 256 MiB entpackte Nutzdaten und 50.000 Tar-Einträge
begrenzt. Das Format wird im Speicher verarbeitet; genügend RAM und privaten
Temporärspeicher sowie Platz für Prüfung und Zielkopie einplanen. Im Split-Modus gelten
die Datei-/Daten-/Archivgrenzen je einzeln geprüfter Quelle und separat für das
Installationspaket; Quelleninventar und Katalog bleiben auf jeweils 50.000 Einträge begrenzt.
Einzelne größere Quellen benötigen einen gesonderten, offline geprüften Migrationsweg. Temporäre Klartexte
werden mit privaten Rechten erzeugt und regulär entfernt, nicht kryptografisch
vom Datenträger gelöscht; für sensible Systeme verschlüsselten Temporärspeicher
verwenden. Alte temporäre Stagingreste nach einem Prozess-/Stromausfall nur bei
weiterhin gestopptem Dienst prüfen und entfernen.

### Historischen Recovery-Vorgang am ursprünglichen Ziel klären

`anker restore reconcile PLAN` und die separat bestätigte Rücksetzung prüfen die
private gespeicherte v2-Zielaufnahme und ihre SSH-Verbindungsbindung bei jedem
Journalzugriff. Adresse, Port, Sicherungsrolle, Schlüsselbezug und SSH-Vertrauen müssen
zur Aufnahme passen. Änderungen während der Abfrage werden verworfen; unmittelbar
vor einer Rücksetzung wird erneut geprüft. Eine während der Rücksetzung geänderte
Bindung lässt das Ergebnis unterbrochen, ohne automatischen Wiederholungsversuch.
Eine verfügbare/gesunde Quellsicherung ist dafür nicht erforderlich: Rücksetzung
betrifft das vorhandene Journal am ursprünglichen Ziel. Bei Legacy-Plänen oder
fehlender/defekter Zielbindung an der ursprünglichen, unabhängig identifizierten
Zielkonsole Journal und Drift manuell prüfen. Keine neue Adresse in einen historischen
Plan einsetzen und keine blinde zweite Übernahme starten. Importierte Pläne bleiben gesperrt.
