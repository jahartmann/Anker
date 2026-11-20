# Hosts und Sicherungen im Detail

[Doku-Übersicht](../README.md) · [Zur Anleitung](../HANDBOOK.md)

[PBS](#proxmox-backup-server) · [Anbindung](#proxmox-hosts-anbinden) · [Aufträge](#zeitplan-und-aufträge) · [Änderungen](#änderungen-prüfen)

## Proxmox Backup Server

PBS wird beim Anbinden und bei der Hostprüfung automatisch erkannt. Einrichtung über **Hosts → Host hinzufügen**, SSH-Fingerprint prüfen und anschließend **Jetzt sichern** wählen. Die Hostdetails zeigen PBS-Version, Datastores und konfigurierte Sync-, Prune- und Verify-Jobs. Bestehende PVE-Sicherungen bleiben kompatibel.

Gesichert werden `/etc/proxmox-backup/` sowie die Systemkonfiguration im Hostprofil. PBS erlaubt für dieses Konfigurationsbackup ausschließlich `/etc`, `/usr/local` und deren Unterverzeichnisse; `/etc/proxmox-backup` wird immer ergänzt. Konfigurierte Datastore-Verzeichnisse und erkannte Chunk-Speicher werden bei Dateierfassung und Inventarisierung ausgeschlossen. Ein nicht lesbares oder ungültiges Datastore-Verzeichnisprofil bricht die Erfassung ab. Datastore-Inhalte und clientseitige Verschlüsselungsschlüssel müssen separat gesichert werden.

PBS-Sicherungen unterstützen Prüfsummenprüfung, Sicherungsvergleich, geschützte Dateivergleiche und Download. Dateien unter `/etc/proxmox-backup/` gelten als vertraulich; Anzeigen und Downloads verwenden die vorhandenen Berechtigungen. Konfigurationsdateien werden live gelesen, ohne global atomaren Snapshot. Fehlende Dateien, Lesefehler und fehlgeschlagene PBS-Inventarabfragen erscheinen als Hinweise beziehungsweise unvollständige Sicherung.

### PBS-Konfiguration geführt wiederherstellen

Unter **Wiederherstellung planen** eine PBS-Sicherung wählen. **Gesamte PBS-Konfiguration auswählen** übernimmt die Dateiauswahl; anschließend jede Datei vergleichen und ausdrücklich eine Fassung wählen. Geführte Übernahme unterstützt freigegebene gewöhnliche Dateien unmittelbar unter `/etc/proxmox-backup/`, unter anderem ACLs, Benutzer, Remotes, Zertifikate, Schlüssel und Jobdefinitionen. Unbekannte Dateien, Unterverzeichnisse, Links und ACLs/xattrs bleiben manuell. Betriebssystemdateien und Dateientfernungen sind bei PBS ausgeschlossen. PVE und PBS dürfen nicht gemischt werden; Quelle und Ziel benötigen dieselbe unterstützte PBS-Hauptversion (3 oder 4).

Das PBS-Ziel zuerst mit aktuellem Hosthelfer anbinden. Vor **Ziel prüfen**, Übernahme und Rücksetzung beide PBS-Dienste am Ziel stoppen:

```sh
sudo systemctl stop proxmox-backup-proxy proxmox-backup
```

Anker prüft den Stillstand, Dateiinhalte und Metadaten sowie vorhandene Datastore-Verzeichnisse, `.chunks`-Verzeichnisidentitäten und Mount-IDs. Fehlende Datastores zuerst außerhalb dieses Assistenten ohne Initialisierung vorhandener Daten einbinden. Nicht eindeutig nachgewiesene, relative oder nicht eingehängte Datastore-Pfade sowie nicht eindeutig zuordenbare Bind-/Subvolume-Mounts im Konfigurations-/Journalpfad bleiben manuell.

Für `datastore.cfg`, `sync.cfg`, `prune.cfg` und `verification.cfg` die **vorbereitete Fassung** prüfen. `datastore.cfg` behält die vorhandenen Ziel-Datastores samt Pfaden, Backends, Gerätebindung und Wartungsmodus; lediglich unterstützte Einstellungen werden aus der Quelle übernommen. GC-/Prune-Zeitpläne und automatische Datastore-Aktionen werden entfernt. Zurückgespielte Sync-/Verify-Jobs erhalten keine Zeitplan-/Mount-Auslöser; Prune-Jobs werden deaktiviert. Zusätzliche Ziel-Datastores werden nicht entfernt. Es gibt keine Datastore-Anlage, Formatierung, Datensynchronisation, Gastwiederherstellung oder Datenlöschung.

Der gespeicherte Plan benötigt die vollständige Plan-ID zur Ausführung. Vorher-Dateien und Metadaten werden im Hostjournal gesichert; anschließend wird das Ergebnis lesend geprüft. Die separate Rücksetzung benötigt erneut die Plan-ID, gestoppte Dienste und unveränderte Datastore-Identitäten; fremde Dateiänderungen bleiben erhalten. Mountwechsel oder nicht mehr eindeutige Identität blockieren den automatischen Rückweg.

Anker startet weder Dienste noch Jobs. Vor einem späteren manuellen Dienststart **alle vorhandenen** Job- und Aufbewahrungsregeln am Ziel prüfen, einschließlich nicht ausgewählter Dateien. Zurückgespielte Zeitpläne erst nach einer eigenen Abnahme gezielt wieder aktivieren. Die automatisierten lokalen Prüfungen ersetzen noch keine Abnahme auf einem echten PBS.

## Proxmox-Hosts anbinden

Die Hostübersicht fasst Hosts in Standortkarten zusammen. **Ansicht gruppieren** bietet Automatik, Standort, Servertyp, IPv4-Netz (`/24`) und eine gemeinsame Liste. Die Auswahl gilt auch für die Suchergebnisse und wird in diesem Browser gespeichert. Manuelle **Gruppen** in den Hosteinstellungen haben Vorrang. Gemeinsame Namensteile nach Abzug von Serverrollen und separaten Knotennummern liefern Standortvorschläge, z. B. `pve-cp-01` und `pbs-cp-02` → **CP**. Automatik nutzt außerdem gemeinsame `/24`-Netze. Eindeutige passende Hostgruppen können als Vorschlag auch neue Hosts einordnen; widersprüchliche manuelle Gruppen werden nicht zusammengeführt. IP-Nähe beweist keinen Standort. Die Anzeige kennzeichnet abgeleitete Zuordnungen und ihre Grundlage; gespeichert werden sie erst durch eine bewusst gesetzte Hostgruppe. DNS-Ziele und IPv6-Adressen werden nicht anhand von Textähnlichkeit als IPv4-Netz behandelt.

Serverrollen erscheinen getrennt als PVE (Orange), PBS (Grün) oder unbekannt. Bei eingerichteten Hosts bestätigt das Inventar den Typ; in der Netzsuche beschreibt er nur die erkannte Webantwort. Die Namensanzeige berücksichtigt **CP, RS, GS, FZ, Gym, PVE und PBS**, auch mit Ziffern. Katalognamen, SSH-Ziele und IDs bleiben unverändert; die Suche verwendet weiterhin die Originalnamen.

Unter **Hosts → Server suchen** findet Anker Proxmox-VE-Server ohne Passwort. Die Suche läuft vom Anker-Server aus und nutzt dessen LAN-/VPN-Erreichbarkeit. Netze der lokalen Schnittstellen werden zum Ergänzen vorgeschlagen. Für mehrere Standorte die Suchbereiche gesammelt einfügen, zum Beispiel:

```text
10.20.0.0/24
10.21.0.0 255.255.255.0
10.22.0.0/255.255.255.0
```

Ein Netz pro Zeile; Komma und Semikolon trennen ebenfalls Einträge. **Netzbereiche speichern** behält die Liste nach einem Neustart. Bis zu 128 Netzbereiche und 65.536 unterschiedliche Zieladressen pro Suchlauf sind möglich; 35 Standortnetze mit jeweils `/24` passen in einen Lauf. Überlappende Bereiche werden zusammengefasst. Größere Bereiche auf mehrere Läufe verteilen. PVE-Webport (Standard 8006), PBS-Webport (Standard 8007), SSH-Port (Standard 22) und 16–256 parallele Adressprüfungen sind anpassbar.

Die Suche prüft zuerst kurz die beiden TCP-Webports. Nur offene Ports erhalten die aufwendigere HTTPS-Erkennung. Unklare Verbindungen werden in einer zweiten Runde mit längerem Zeitlimit erneut geprüft; ein fehlendes Ping-Signal schließt keinen Host aus. Bekannte Adressen und konfigurierte Peers kommen zuerst. Bevorzugte Adressendungen werden über alle Standortnetze verteilt und anhand bestätigter PVE-/PBS-Treffer höher gewichtet. Diese Reihenfolge beschleunigt frühe Treffer; trotzdem werden alle ausgewählten Adressen geprüft, auch nach dem ersten Fund je Netz.

Fortschritt und Treffer werden etwa alle zwei Sekunden und beim Anhalten im Katalog gespeichert. Nach Abbruch, erreichtem Zeitfenster oder Anker-Neustart **Suche fortsetzen** wählen. Abgeschlossene Prüfungen bleiben erhalten; offene oder beim Abbruch laufende Prüfungen werden fortgeführt. Bei einem abrupten Prozessabbruch kann die Arbeit seit dem letzten Speicherpunkt wiederholt werden. Fortsetzen verwendet die ursprünglichen Netzbereiche und Ports. **Server suchen** beginnt einen neuen Lauf. Vor einem Update hält Anker die Suche an und speichert den Stand.

Optional liest Anker über den bestehenden Sicherungszugang die IP-Adressen bekannter PVE-Cluster (`/cluster/status`, `corosync.conf`) und PBS-Gegenstellen (`remote.cfg`). Standalone-PVE-Hosts lösen keine Clusterabfrage aus. Hinweise außerhalb der ausgewählten Netze werden verworfen; konfigurierte DNS-Namen ohne IP-Hinweis bleiben für die normale Netzsuche offen. Ältere Hosthelfer über **Verbindung einrichten** aktualisieren. Auch ohne Hinweise funktioniert die vollständige Suche. Hinweise ersetzen keine SSH-Fingerprintbestätigung; Datastore-Inhalte werden weder gelesen noch verändert.

**Server suchen** zeigt Fortschritt und laufend gefundene Server mit Adresse, ermittelbarem DNS-Namen, verfügbarer Version und SSH-Erreichbarkeit. Die Suche lässt sich abbrechen; Teilergebnisse bleiben bis zum nächsten Suchlauf oder Dienstneustart verfügbar. Das Fenster kann während der Suche geschlossen und erneut geöffnet werden. Nicht erreichbare, anders adressierte oder durch Firewalls gesperrte Dienste können fehlen. Die Erkennung anhand der Webantwort ist noch keine Identitätsprüfung. Gefundene Hosts lassen sich über **Einzeln einrichten** mit vorausgefüllter Adresse anbinden.

Optional mehrere neue Hosts auswählen und **gemeinsame Zugangsdaten prüfen und Hosts einrichten** öffnen: SSH-Identitäten lesen, Fingerprints unabhängig vergleichen und einzeln bestätigen. Erst dann **Zugänge prüfen** starten. Passwort 1 wird zuerst verwendet, das optionale Passwort 2 nur nach abgelehnter Anmeldung; erfolgreiche Anmeldungen und Verbindungsfehler werden im zweiten Durchlauf übersprungen. Die Prüfung führt keine Befehle auf den Hosts aus. **Geprüfte Hosts einrichten** startet anschließend ausdrücklich die normale Installation für erfolgreich geprüfte, ausgewählte Hosts. Fehler werden je Host angezeigt. Passwörter bleiben nur im Arbeitsspeicher des geöffneten Fensters und werden nicht gespeichert.


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

### Gemeinsam verwalten, Wartung und Liveänderungen

In **Hosts → Gemeinsam verwalten** Hosts über mehrere Seiten auswählen oder alle aktuell gefilterten Hosts übernehmen. Ein Standortfilter begrenzt die gemeinsame Auswahl. Vor dem Start zeigt Anker die ausdrückliche Zielauswahl; später gefundene Hosts werden nicht nachträglich hinzugefügt. Sichern mit anschließender Verifikation, Hostprüfung, Verifikation des letzten Standes und lesender Livevergleich stehen der Rolle Wiederherstellung zur Verfügung. Zeitplan, Standortgruppe, Pause/Aktivierung, Wartung und Liveprüfintervall benötigen Administratorrechte. Veraltete Auswahlstände und bereits belegte Hosts werden mit Begründung übersprungen. Die Sammelquittung nennt die einzelnen Ergebnisse und verlinkt gestartete Aufträge; wiederholte Übertragung derselben Kennung erzeugt keine zusätzlichen Aufträge.

**Befristete Wartung** pausiert die automatische Sicherung, automatische Liveprüfungen und Überfälligkeitsmeldungen bis zum Endzeitpunkt. Der bisherige Zeitplan und der Aktivierungszustand bleiben gespeichert. Nach Ablauf gelten sie automatisch wieder, auch nach einem Dienstneustart. Bereits laufende Aufträge dürfen enden; noch wartende automatische Aufträge werden vor dem Start erneut geprüft. Manuelle Sicherungen und Prüfungen bleiben möglich. Vergangene Tagesläufe werden nicht nachgeholt. Pausieren oder Wartung erledigt keine vorhandenen Probleme.

**Übersicht → Jetzt wichtig** bündelt Ursachen über betroffene Hosts hinweg. „Zur Kenntnis genommen“ bleibt offen; „Hostauftrag aktiv“ bezeichnet laufende Arbeit, keine erfolgreiche Reparatur. Neuere passende Erfassungs-, Verbindungs- oder Integritätsnachweise können eine Ursache erledigen. Ein erneutes Auftreten öffnet sie wieder. Gelöschte Hosts gelten nicht als nachweislich repariert. Historische Sicherungen und ihre Warnungen bleiben unverändert.

**Hosts → Helferübersicht** zeigt gemeldete Helferversion, den exakten installierten Dateihash, den erwarteten Stand und bekannte Fähigkeiten. Ältere Helfer ohne Selbstkennung bleiben unbekannt. Der ausdrücklich gestartete Administratorpfad ersetzt ausschließlich die feste Helferdatei atomar. Er benötigt einen bereits vertrauten unveränderten SSH-Schlüssel im verwalteten Hostschlüsselspeicher und vorübergehende Administratorzugangsdaten. Profile, Benutzer, Rollenschlüssel, sudo-Regeln und Hosteinstellungen werden nicht neu eingerichtet. Eine laufende Hostübernahme sperrt den Austausch über dasselbe Hostjournal-Lock. Danach eine Hostprüfung zum Aktualisieren des Inventars starten. Das Passwort wird weder im Katalog noch im Browser-Speicher oder Auditverlauf abgelegt. Für Hosts mit eigenem Hostschlüsselspeicher bleibt die Aktualisierung administrativ manuell.

**Livezustand gegenüber Sicherung** im Hostdetail vergleicht die zuletzt lesend erhobenen Inhaltshashes mit dem letzten vollständigen Konfigurationsstand. Die Automatik ist standardmäßig ausgeschaltet und kann gemeinsam auf 15 bis 1440 Minuten gestellt werden. Geänderte, neue und entfernte Einträge werden getrennt angezeigt. Veraltete, unvollständige oder vor der Referenz erhobene Inventare liefern keine Entwarnung. Alte Sicherungen ohne Profilnachweis und außerhalb des Liveinventars liegende Zusatzpfade bleiben begrenzt vergleichbar. Der Vergleich betrifft Inhalte und Linkziele; er ersetzt weder eine Sicherung noch eine Wiederherstellungsabnahme. PBS-Datastore-Inhalte und `.chunks` bleiben ausgeschlossen.

### Zeitplan und Aufträge

**Sicherungsfehler verstehen:** Unvollständige Stände zeigen ihre Ursachen direkt in Hostübersicht und Sicherungsfokus. Jede Diagnose nennt den sicher darstellbaren Pfad, die Bedeutung und eine passende Abhilfe. „Ungültiger Pfad“ bezeichnet einen Formatfehler und belegt nicht, dass eine Datei fehlt. „Verwendete Datei nicht in der Sicherung“ bezeichnet einen Konfigurationsverweis außerhalb der erfassten Dateien; die Existenz auf dem Host muss gesondert geprüft werden. Geschützte Inhalte bleiben verborgen, die fachliche Erklärung und Abhilfe bleiben sichtbar. Vorhandene historische Sicherungen werden dafür nicht verändert.

**Sicherungsprofil prüfen** öffnet die Host-Einstellungen. Veraltete Pflichtpfade dort ausdrücklich korrigieren; benötigte zusätzliche Konfigurationspfade müssen auch im root-eigenen Hostprofil `/etc/anker-host.json` erlaubt sein. Danach **Neue Sicherung starten** wählen. Fehlende Dateien werden nicht durch leere Dateien ersetzt und Anforderungen nicht automatisch entfernt. Für die detaillierteren Formatmeldungen vorhandene Hosthelfer über **Hosts → Helferübersicht** aktualisieren. Fehlgeschlagene Aufträge mit gespeichertem Teilstand bieten **Unvollständige Sicherung prüfen** direkt an. PBS-Datastore-Inhalte bleiben außerhalb der Konfigurationssicherung.

Unter **Einstellungen → Zeitplan** stehen tägliche Startzeit, Zeitzone, Parallelität und Wiederholungen. Ein Host übernimmt diese Zeit oder bekommt im Hostformular eine eigene Startzeit. „Automatisch sichern“ pausiert nur den Zeitplan; „Jetzt sichern“ bleibt möglich. Anker prüft jede Minute selbst, ob ein Lauf fällig ist. Keine Crontab anlegen. Hosts starten mit einem festen Versatz von weniger als einer Stunde, spätestens um 23:59 Uhr. Die Demo führt keine automatischen Sicherungen aus.

**Aufträge → Ablauf ansehen** zeigt Auslöser, Erstellzeit, tatsächlichen Start, Versuche und Fehler. Von dort die fertige Sicherung öffnen, einen aktiven Auftrag abbrechen oder eine fehlgeschlagene Sicherung/Hostprüfung erneut starten. Administratoren können abgeschlossene Einträge entfernen; Sicherungen und Tagesmarker bleiben erhalten. Wiederherstellungen erneut über einen frisch geprüften Plan bestätigen. Bereits eingeplante Tagesläufe werden nach Zeitplanänderung oder Neustart nicht nochmals gestartet. Ausgefallene frühere Tage werden nicht nachträglich rekonstruiert.

```sh
sudo anker jobs
sudo anker job show AUFTRAG
sudo anker job cancel AUFTRAG
sudo anker job retry AUFTRAG
sudo anker job remove AUFTRAG
```

Sicherungs- und Hostprüfaufträge wiederholen ausschließlich erkennbare vorübergehende Verbindungsfehler (etwa Verbindungsabbruch oder Timeout). Die Einstellung „Wiederholungen“ erlaubt 0–5 zusätzliche Versuche mit 1, 2, 4, 8 und 16 Sekunden Abstand innerhalb derselben Auftrags-ID. Währenddessen bleibt der Host für weitere Aufträge gesperrt. Identitäts-, Rechte-, Inhalts- und lokale Speicherfehler werden nicht automatisch wiederholt; Wiederherstellungen ebenfalls nicht. Ein bereits veröffentlichtes Ergebnis beendet die Wiederholungsschleife. Wiederholte Anfragen zur manuellen Wiederholung desselben abgeschlossenen Auftrags liefern denselben Folgeauftrag. Nach Dienstabbruch wird unbestätigte Arbeit als unterbrochen markiert; sie wird nicht automatisch erneut ausgeführt. Ein geordnetes Dienstende bleibt mit eigener Meldung im Auftragsverlauf sichtbar.

## Sicherungen und Inventar im Web

Die Hostseite bündelt Zustand, Sicherungsverlauf, Änderungen, Inventar und Einstellungen. Unter Sicherungen erscheint standardmäßig ein Eintrag je Host; der Verlauf zeigt den neuesten Stand direkt und bündelt ältere Stände in zunächst geschlossenen Monats- und Tagesgruppen. Die Suche nach Datum oder Kennung findet auch eingeklappte Stände. „Alle Stände“ öffnet das vollständige Archiv mit den bisherigen Host-, Status- und Schutzfiltern.

Unter **Sicherungen** nach Host, Status, Schutz oder Archiv filtern; die Suche findet Hostnamen und Sicherungs-IDs. Die Liste zeigt 25 oder 50 Stände pro Seite. Laufende Sicherungen stehen separat mit Host, Start und Laufzeit. **Aufträge** trennt aktive Arbeit vom Verlauf und lässt sich zusätzlich nach Auftragstyp durchsuchen.

**Dateien** öffnet die Ordner des gewählten Stands. Über die Pfadleiste zurückgehen oder mit **Pfad suchen** über alle Unterordner suchen. Die Treffer haben ebenfalls feste Seiten. Eine Datei auswählen, um sie anzusehen, herunterzuladen oder zur Wiederherstellung zu übernehmen. Verknüpfungen zeigen ihr Ziel; geschützte Inhalte benötigen weiterhin eine ausdrückliche Freigabe. Hinweise zur Erfassung sind aufklappbar.

Im **Inventar** stehen System und Cluster zuerst. Netzwerk und Datenträger haben getrennte Suchen und Seiten; Kennungen und weitere erfasste Daten lassen sich bei Bedarf öffnen.

Unter **Einstellungen → Aufbewahrung** gelten die Tages-, Wochen- und Monatsregeln einzeln je Host: jeweils der neueste erfolgreiche Stand eines Zeitraums bleibt. Ein Stand kann mehrere Regeln erfüllen; die Zahlen werden nicht addiert. Der letzte erfolgreiche Stand, geschützte/unvollständige/beschädigte Stände und Quellen von Wiederherstellungsplänen bleiben erhalten. Automatische Archivierung lässt sich ausschalten; komprimierte Stände unterliegen weiterhin der Aufbewahrung. **Überfällig nach Stunden** im Zeitplan steuert nur den Hosthinweis und löscht nichts.

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

Updates unter **Einstellungen → Updates** einrichten, prüfen und installieren. Fehlt die Quelle, „Updatequelle einrichten“ wählen, die mitgelieferte Anker-Quelle bestätigen und anschließend nach Updates suchen. Dafür ist kein Terminalbefehl nötig. Eine eigene Quelle kann mit ihrem unabhängig geprüften öffentlichen Ed25519-Schlüssel angegeben werden. Nur Administratoren dürfen die Quelle ändern; der Dialog zeigt den Fingerprint vor dem Speichern. Der Schlüssel wird nicht aus einem ungeprüften Release nachgeladen.

Alternativ `sudo anker update check`, danach `sudo anker update install`. Signatur und SHA-256 werden vor der Installation geprüft. Laufende Sicherungen und Wiederherstellungen blockieren das Update. Bei fehlgeschlagenem Start stellt der Updater die vorherige Version und den Katalog wieder her. [Einrichtung und Rückfall](../UPDATES.md).

Nach einem im Web gestarteten Update lädt die Seite automatisch neu, sobald die neue Version die Startprüfung bestanden hat. Das funktioniert auch bei einem Seitenwechsel und kurzer Nichterreichbarkeit des Dienstes. Bei einem fehlgeschlagenen Update bleibt die Seite bestehen und zeigt den Rückfall. Ungespeicherte Einstellungen vor dem Update speichern. Beim ersten Update von einer älteren Oberfläche auf 0.2.4 die Seite einmal selbst neu laden; die Automatik gilt ab der neuen Oberfläche.

## Änderungen prüfen

Die Ansicht **Host → Änderungen** navigiert per Zeitregler oder Schrittbuttons durch die datierten Sicherungen desselben Hosts. Sie zeigt die damals gespeicherten unterstützten Einstellungen, ihre Werte gegenüber dem direkt vorherigen Stand und höchstens drei betroffene Konfigurationen je Bereich. Der erste Stand dient als Ausgangspunkt. Unvollständige Stände, unklare Zeitfolgen und nicht lesbare Werte werden nicht als unverändert gewertet. Wertprojektionen und Vergleiche werden nur für den gewählten Zeitpunkt geladen; schnelle Wechsel brechen überholte Leseanfragen ab. Detailvergleich und Änderungsspur sind direkt erreichbar.

Hostnamen erscheinen lesbarer, ohne Katalognamen oder SSH-Ziele zu ändern. Der Änderungsverlauf einer Node ist ein aufklappbarer Zeitstrahl mit vier Sicherungszeitpunkten pro Seite. Pro Zeitpunkt erscheinen maximal drei konkrete Dateithemen, etwa Storage-Anbindungen, Netzwerkdefinition oder VM-Konfiguration; gleichartige Hinweise werden gebündelt. Die Beschreibungen leiten sich aus Dateipfaden und Änderungsmetadaten ab, nicht aus einer Interpretation der Konfigurationswerte. Die eigene breite Detailseite zeigt den vollständigen Vergleich und erhält beim Zurückgehen die Position und Auswahl im Hostverlauf. Seine Vergleiche bleiben innerhalb derselben Node und laden keine Dateiinhalte. Der Detailvergleich bietet einen Nodefilter sowie Bereichs-Cards für Netzwerk, Storage, System und weitere Konfiguration. Ein Sicherungszeitpunkt belegt den erfassten Zustand, nicht den genauen Zeitpunkt einer Änderung am laufenden Host.

Die Startseite bündelt betroffene Hosts in maximal drei aufklappbaren Problem-Cards. Große Gruppen lassen sich durchsuchen und zeigen vier Hosts pro Seite. Am Host fasst eine Änderungs-Card die zwei neuesten geeigneten vollständigen Stände nach Netzwerk, Storage, System und weiterer Konfiguration zusammen. Reine Zeitstempeländerungen werden getrennt gezählt. Der Detailvergleich übernimmt genau dieses Ständepaar; ein Ladefehler ergibt keine Entwarnung. Die Zusammenfassung bewertet gespeicherte Dateien, nicht ungesicherte Änderungen am laufenden Host.

Bei Handlungsbedarf nennt die Hostseite die Ursache und führt zur passenden Prüfung, zum fehlgeschlagenen Auftrag oder zum erneuten Sichern. Für den direkten Wiederherstellungseinstieg wird der neueste vollständige Stand ohne bekannten Integritätsfehler und mit plausiblem Zeitstempel vorgeschlagen. Das ist eine Quellenauswahl, kein Nachweis einer erfolgreichen Gesamtrecovery; Integrität, tatsächliches Ziel und Änderungen werden weiterhin im Plan geprüft. Ungültige und mehr als fünf Minuten in der Zukunft liegende Quellzeiten werden nicht als aktuelle Sicherung bewertet.

### Änderungsspur

Die **Änderungsspur** verfolgt einzelne Einstellungen einer Node über ihre Sicherungen: IPv4-/IPv6-Weiterleitung, Swap-Neigung, Node-Name und DNS-Server. Gleiche aufeinanderfolgende Werte werden gebündelt; Wertwechsel nennen den ersten Nachweis und das Intervall zum vorherigen Wert. Geschützte, unvollständige, widersprüchliche oder unlesbare Stände bleiben als Nachweislücken sichtbar. Gleiche Zeitstempel werden nicht künstlich geordnet. Pro Abruf werden höchstens 20 Stände gelesen; ältere Sicherungen lassen sich nachladen. Die Ansicht startet mit vier Gruppen. Unterstützte Einstellungen lassen sich direkt aus dem Dateivergleich verfolgen. Die Spur zeigt explizite Deklarationen in gesicherten Dateien, keine wirksamen Livewerte, Standardwerte oder angenommene Dateiprioritäten. Geschützte Inhalte bleiben auch in dieser Ansicht verborgen.

Geänderte unterstützte Einstellungen erhalten in der Änderungsansicht und Änderungsspur eine fachliche Einordnung: Bedeutung der gespeicherten Deklaration, bedingte mögliche Auswirkung und eine passende Prüfung am Host. Die festen Regeln verweisen auf Kernel-, systemd- oder Linux-man-pages-Dokumentation. Sie erscheinen nur für unterschiedliche, explizite Werte mit geordneten Zeitpunkten und passenden Quelldateien. Der aufklappbare Nachweis zeigt beide Sicherungs-IDs, Zeitpunkte und Werte; die jeweiligen Originaldateien werden erst beim Anklicken gelesen. Geschützte Inhalte bleiben verborgen. Daraus wird weder ein bestätigter Ausfall noch eine erfolgreiche Übernahme auf dem laufenden Host abgeleitet.

### Vorschau und Übergabe an den Assistenten

Aus einer belegten Einstellungsänderung öffnet die **Wiederherstellungsvorschau** einen lesenden Vergleich mit dem früheren Quellstand. Sie zeigt den gewünschten Wert, betroffene ganze Dateien, Anlage-/Entfernungsbedarf, Dateieigenschaften und zusätzliche unterstützte Einstellungsunterschiede, deren Deklarationen vollständig im Dateiumfang liegen. Weitere Textänderungen bleiben im umgekehrten Dateivergleich sichtbar; pro Seite erscheinen höchstens vier Dateien. Veraltete Werte, nicht lesbare/geschützte Dateien, Nodewechsel oder unklare Zeitfolgen ergeben keine Teilfreigabe. Die Vorschau erzeugt keinen Plan und keinen Auftrag und prüft keinen Livehost. Ausgewählte Dateien lassen sich direkt in eine neue Vorbereitung im **Wiederherstellungsassistenten** übergeben. Früherer Quellstand und ursprüngliche Node werden vorausgewählt; historische Vergleiche, Dateientscheidungen und Zielaufnahmen werden nicht als Freigabe übernommen. Die aktuelle Zielprüfung, Entscheidung je Datei, Planprüfung und gesonderte Ausführungsbestätigung bleiben erforderlich. Änderungen an Quelle, Ziel oder Dateiumfang verwerfen die Zielaufnahme. Fehlende Quelldateien stoppen die Vorbereitung. Notwendige **Dateientfernungen** sind zunächst abgewählt und lassen sich einzeln vormerken. Sie erscheinen als eigene Cards und erfordern nach der aktuellen Zielaufnahme eine Entscheidung pro Datei, eine ausdrückliche Bestätigung des Löschumfangs und die gesonderte Ausführungsbestätigung im Plan. Nur im Quellstand fehlende, vollständig lesbare gewöhnliche Konfigurationsdateien ohne geschützte Inhalte oder ACLs/xattrs werden automatisch entfernt; Verzeichnisse, Programme und geschützte Systemdateien bleiben ausgeschlossen. Der Hosthelfer muss `remove_files` unterstützen. Vor dem Löschen sichert er Originalinhalt und Eigenschaften dauerhaft im Rücksetzjournal. „Dateien zurücksetzen“ stellt diese Dateien mit ihren ursprünglichen Rechten wieder her, solange keine fremde Datei den Pfad belegt. Konflikte bleiben sichtbar und fremde Dateien erhalten. Auch in der lokalen Demo lassen sich Entfernen und Rücksetzen ausprobieren. Lesende Benutzer erhalten keine Übergabeaktion.

### Ergebnisprüfung nach Übernahme oder Rücksetzung

Nach erfolgreicher Dateiübernahme oder kontrollierter Rücksetzung liest die **Ergebnisprüfung** den Zielbestand erneut. Sie vergleicht freigegebene Dateiaktionen mit dem Plan beziehungsweise dem ursprünglichen Zielstand: Inhalt, Typ, Rechte, Eigentümer, Gruppe und unterstützte Eigenschaften sowie die erwartete Abwesenheit entfernter Dateien. Die Cards zeigen bestätigte Dateien, Abweichungen und nicht nachgewiesene Ergebnisse getrennt; Textunterschiede und Prüfsummen sind aufklappbar. Unterstützte Einstellungen werden aus den geprüften Dateien gelesen, einschließlich bekannter weiterer Deklarationsdateien aus dem Zielinventar. Widersprüchliche Werte bleiben sichtbar. Das bestätigt gespeicherte Deklarationen, keine wirksamen Kernelwerte, Dienstaktivierung oder einen Neustart. Eine fehlgeschlagene Abfrage oder ungültige Plan-/Zielbindung gibt keine Teilfreigabe. Die Prüfung ist rein lesend und startet keine neue Übernahme. Bis zu 128 Dateipfade, 20 Prüfversuche und begrenzte Textnachweise halten den Umfang überschaubar.

Dateien sind über Ordner, Breadcrumbs und globale Pfadsuche erreichbar; Sicherungen lassen sich direkt mit dem älteren Stand desselben Hosts vergleichen. Abdeckung, Integrität, unabhängige Kopie, menschlicher Drill und frische maschinelle Beobachtungen bleiben getrennte datierte Nachweise. Für den Verlust der Anker-Installation gibt es einen verschlüsselten lokalen Cold-Export mit isoliertem Import. Das [operatorgeführte Abnahmekit](https://github.com/jahartmann/Anker/tree/main/docs/recovery-lab) ist ausführbar; sämtliche realen PVE-Szenarien bleiben mangels isoliertem Testsystem offen.

## Optionale lokale Demo

Nur der ausdrückliche Befehl `demo` erzeugt Beispieldaten:

```sh
make build
./bin/anker --data ./var/demo demo
```

Auf `http://127.0.0.1:8087` mit `demo` / `anker-demo-2026` anmelden. Mit `Ctrl+C` beenden. Die Demo kontaktiert keine echten Hosts und bleibt auf Loopback beschränkt. Ohne `--data` verwendet sie `./var/demo`, unabhängig von `ANKER_DATA`.

Beim ersten Start muss der Demoordner leer sein. `.anker-mode` hält die Betriebsart fest. Demo und Produktion dürfen sich nicht überlappen; `/srv/anker` und seine Eltern- und Unterverzeichnisse sind für Demo gesperrt. Alte unmarkierte Demoordner bleiben erhalten; stattdessen einen neuen Demoordner wählen. Den Marker nicht zum Wechseln der Betriebsart entfernen oder ändern.

## Entwicklung und Lizenz

Die Oberfläche behält helle Notion-Karten mit versetzten Schatten als Grundlage. Server-/Admin-Details entstehen durch technische Symbole, griffige Buttons und deutlich erkennbare Themenakzente. Sicherung, Änderungen und Wiederherstellung sollen sich über Beschriftung, Symbol und Gestaltung unterscheiden. Fehlerursachen und passende Abhilfen stehen direkt beim betroffenen Stand; Statusfarben ergänzen diese Angaben.

Die gemeinsame Basis in `web/src/paper.css` gilt für alle sechs Arbeitsbereiche, die fünf Hostregister und die sieben Einstellungsbereiche. Sie umfasst auch Anmeldung, Netzsuche, Benutzerverwaltung, Zertifikate, Updates, Speicherassistenten, Cluster-Sicherungssets, Datei- und Sicherungsvergleiche, Einstellungsverläufe, Wiederherstellungsschritte und Ergebnisnachweise. Dialoge erhalten ihr Thema ausdrücklich über die gemeinsame Dialogkomponente, da sie außerhalb des normalen Seitenbaums gerendert werden. Browserprüfungen gehen Haupt- und Unteransichten bei Desktop- und Mobilbreite durch.

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s host -p 'test_*.py'
python3 -m unittest discover -s scripts -p 'test_*.py'
cd web && npm ci && npm run build && npx playwright install chromium && npm test
```

Browserprüfungen verwenden getrennte temporäre Ordner für Demo und leere Produktion. Linux-/systemd-Prüfungen testen den echten Installer, Terminaleinrichtung, HTTPS-Anmeldung, Updates und Wiederanlauf in einer isolierten CI-VM. Tests und Designentwürfe werden nicht in Release-Pakete aufgenommen.

[MIT-Lizenz](../../LICENSE), [Beiträge](../../CONTRIBUTING.md), [Sicherheitsmeldungen](../../SECURITY.md). Schreibrechte am öffentlichen Originalrepository erhalten nur freigegebene Maintainer. Eigene Forks und Änderungen sind unter MIT erlaubt.
