# Operatorgeführte PVE-Abnahme

**Noch nicht auf realem PVE ausgeführt.** Lokale Fixture-Tests prüfen nur dieses Werkzeug. Es führt weder SSH noch Restore, Reboot, Cluster-, HA-, Ceph- oder Storageänderungen aus. Python 3 mit Standardbibliothek genügt. Auf dem späteren isolierten Laborhost aus einer privaten operatorverwalteten Ablage starten; keine Produktionssysteme verwenden.

## Vorbereiten

1. Privaten Arbeitsordner anlegen (`umask 077; mkdir LABOR`); `bindings.example.json` und `dependencies.example.json` dorthin kopieren. Keine tatsächlichen Zugangsdaten eintragen.
2. In Bindungen Labor-ID, erlaubte Szenarien (Zahlen 1–9), Person/UTC-Zeit, Quelle/Backup/Plan/Ziel/Fingerprints, PVE-Version, Build-ID, Paket-SHA256 und Boot-Baseline eintragen. Isolation mit Belegreferenz ausdrücklich bestätigen; physisch getrennte Netze, ausgeschlossene alte Nodes und Konsolenzugang durch Operator prüfen. Unbekannte Isolation stoppt die Erfassung.
3. Dossier mit Rolle, Ort/ID, Eigentümer, Prüfzeit und Status jeder externen Abhängigkeit ausfüllen. Gast-/PBS-Sicherungen, Schlüsselverwahrort, Disk-/Storage-Backing und Helferrechte müssen nachvollziehbar sein. Nur Referenzen, keine Secrets oder Config-Inhalte. Nicht benötigte Abhängigkeiten mit Begründung als `not_applicable` dokumentieren.
4. `preflight_reference` ist der eindeutige Berichtspfad. `baseline_boot_id`, `machine_sha256`, `source_manifest_sha`, `target_snapshot_sha` und `connection_sha` aus dem **privaten gespeicherten Plan und seiner Zielaufnahme** übernehmen; Inventar-Fingerprints sind separate Bindungen. Fehlende positive `target.details.command_results.machine_identity`-/`boot_id`-Vorherabfragen lassen sich nicht durch einen späteren Preflight ergänzen. In diesem Fall neue Zielaufnahme und neuen Plan erstellen, vor einem Reboottest.
5. Tatsächliches Anker-Binary und installierten Helfer separat hashen (`sha256sum` unter Linux, `shasum -a 256` unter macOS), als `build_sha256`/`helper_sha256` binden. `provenance_reference` dokumentiert, wie diese Bytes dem verwendeten Dienst und installierten Helfer zugeordnet wurden, einschließlich unabhängig geprüftem SSH-Vertrauen. Das Maschinenprotokoll enthält **keinen Digest des ausgeführten Helfers**; diese Zuordnung bleibt eine menschliche Vertrauensannahme. Paketdigest unabhängig bestimmen. `date` ist der UTC-Beginn dieses Laborlaufs im RFC3339-Format; auch jeder Schrittnachweis benötigt einen solchen Zeitpunkt.
6. Szenario 9 benötigt keine aktive Ziel-/Plan-/Bootbindung: Labor/Operator/Datum/Isolation, Build-ID/-Digest, Herkunftsreferenz, Abhängigkeitsdossier und `cold_package_sha256` binden. Die alten importierten Pläne bleiben widerrufen; ein Hosthelfer wird hier nicht ausgeführt und sein Digest ist für dieses Szenario nicht erforderlich.

```sh
# Ohne --collect-local-pve nur Bindungen/Dateidigests prüfen; keine Systemabfrage.
./docs/recovery-lab/lab-kit.py preflight \
  --bindings LABOR/bindings.json --output LABOR/preflight-before.json \
  --build /PFAD/anker --helper /PFAD/installierter-anker-host \
  --collect-local-pve
```

Das Werkzeug liest feste lokale Befehle: `pveversion`, `uname -r` und `systemctl is-active` für `pve-cluster`, `pvedaemon`, `pveproxy`. Es erfasst einen Hash von `/etc/machine-id`, die Boot-ID und freien Platz unter `/var/lib/anker-host`. Kein Configdump, keine Schlüssel, keine stderr-Rohdaten. Abfragen sind auf je 10 Sekunden/16 KiB begrenzt; Build/Helfer auf je 256 MiB, Bindungs-/Szenario-/Preflight-JSON auf 1 MiB, tatsächliche Plan-/Aufnahme-JSON auf je 32 MiB. Berichte sind 0600 und überschreiben keine vorhandenen Dateien oder Symlinks. Die Elternalage muss privat und vertrauenswürdig sein.

Fehlende Artefakte, Befehle, unbekanntes Backing, Servicezustand oder Helferprivilegien bleiben offen. `is-active` allein beweist keine Dienstfunktion. Ein Digest belegt nur die gelesenen Bytes, keine vertrauenswürdige Herkunft. PVE-Istversion, Maschinenhash und Boot-ID gegen die gebundenen Sollwerte **manuell vergleichen** und Abweichungen dokumentieren.

## Szenario durchführen

```sh
# Leeres Protokoll erzeugen; führt keinerlei Szenarioaktion aus.
./docs/recovery-lab/lab-kit.py scenario \
  --bindings LABOR/bindings.json --number 1 --output LABOR/scenario-1.json
# Alternativ dieselbe Checkliste interaktiv protokollieren, in einer NEUEN Datei:
./docs/recovery-lab/lab-kit.py scenario --interactive \
  --bindings LABOR/bindings.json --number 1 --output LABOR/scenario-1-run.json
./docs/recovery-lab/lab-kit.py review --input LABOR/scenario-1-run.json
```

An jedem **HALT** handelt nur der Operator, separat und ausdrücklich. `pass` dokumentiert eine menschliche Aussage mit Person, Zeit und SHA256 einer privaten tatsächlichen Belegdatei (`evidence_reference`, 64 kleine Hexzeichen). Die Belegdatei kann mehrere eindeutig beschriftete Schritt-/Unterlaufnachweise enthalten. `fail` ergibt `failed`; leer bleibt `unrun`. Restfehler eintragen. Checklisteninstruktionen nicht verändern. Bei Build-, Helfer-, Quell-, Ziel- oder Planwechsel neue Bindungen und neue Berichte erstellen.

| Nr. | Durchführung und erwarteter Nachweis |
|---|---|
| 1 | Standalone: gewöhnliche Einzeldatei auswählen, Ziel-vorher belegen, separat bestätigen/übernehmen, Journal prüfen, separat zurücksetzen und Vorherzustand/Metadaten vergleichen. |
| 2 | Replacement: andere NIC-Namen/Disks, neue Boot-/Rootinstallation, geprüfte Zuordnungen und Backing; externe Gast-/PBS-Daten separat. Unbekanntes bleibt HALT. |
| 3 | Zwei getrennte Läufe: gesunder isolierter Cluster-Ersatznode **und** vollständiger isolierter Clusterverlust. HALT vor Join/Quorum/Ceph/HA und Datenbankrekonstruktion; eigene versionsgerechte Anleitungen referenzieren. |
| 4 | Offline-Paket ohne Dienst prüfen; anschließend ausgewählte Dateien ohne laufendes Anker manuell übertragen. Kein pauschales Entpacken nach `/`, Symlinks bleiben zunächst Metadaten. Herkunft, Rechte, Vorherkopie und Funktion separat belegen. |
| 5 | Transferabbruch und verlorene Apply-Antwort getrennt auslösen; Journal abgleichen, keine blinde Wiederholung. Rücksetzung und Verweigerung bei fremder Drift separat nachweisen. |
| 6 | Kontrollierter Platzmangel auf entbehrlichem Testziel; Aufschub/Abbruch und unveränderte Quelldigests nachweisen. Keine Produktivplatte füllen. |
| 7 | Quell-, Ziel-/Verbindungs- und Configdrift in separaten Durchläufen; alte Entscheidung muss ungültig werden. Nur Laborkopie verändern; neuer Vergleich/Plan vor Fortsetzung. |
| 8 | Identität/Boot-Baseline vorab; ausdrücklich manueller Reboot; neue private Nachher-Erfassung und frische plangebundene Maschinenprüfung verlangen. |
| 9 | Anker Cold-Export/Inspect/isolierter Import; Credentials erhalten, Startup gesperrt, Hosts deaktiviert, Sessions/Arbeit und alte Recovery-Freigaben unwirksam. |

Für jeden Unterlauf neue Protokolldatei/Bindungen verwenden; Szenario 3 benötigt beide, Szenario 5 beide Unterbrechungsarten, Szenario 7 alle drei Driftarten. Quelle und unveränderte Originale bis zur Abnahme behalten.

## Offline-Paket prüfen

```sh
anker plan-package verify --input LABOR/PLAN.tar > LABOR/package-verification.json
```

Läuft ohne Anker-Dienst/Datenroot und extrahiert nichts. `coverage: complete` belegt vollständige **interne** Index-/Prüfsummenabdeckung; `limited` bedeutet Altexport mit geprüften gelisteten Quellprüfsummen. Beide melden `trusted_origin: false` und `recovery_verified: false`. Rückgabe 0 heißt „lesbar innerhalb ausgewiesener Abdeckung“, nicht Labor bestanden. Korruption/fehlende indexierte Dateien/unsafe Archivstruktur ergeben Fehler. Das Tool ist im Anker-Binary enthalten; der manuelle Transfer und die lesbare Anleitung brauchen keinen laufenden Anker.

## Anker-Katastrophe (Szenario 9)

Nur an der isolierten **Laborkopie**; vorher alle Schreiber einschließlich Dienst/Updater durch Operator stoppen. Passphrase ausschließlich aus privater 0600-Datei. Die Platzhalter müssen vor Ausführung auf Labordateien zeigen:

```sh
anker --data /LABOR/cold-data disaster export \
  --output /LABOR/disaster.bundle --passphrase-file /PRIVAT/passphrase
# Nur falls aktive Config nicht bereits app-eigen liegt: zusätzlich --config-root /LABOR/config
anker disaster inspect --input /LABOR/disaster.bundle --passphrase-file /PRIVAT/passphrase
anker disaster import --input /LABOR/disaster.bundle \
  --destination /LABOR/noch-nicht-vorhanden --passphrase-file /PRIVAT/passphrase
```

Das Import-Elternverzeichnis muss real und vorhanden sein. Den kanonischen Pfad mit `pwd -P` in diesem Verzeichnis ermitteln; unter macOS `/private/tmp` statt des Symlinks `/tmp` verwenden. Kein Zielordner vorab anlegen und keine Symlinkprüfung umgehen.

Im privaten Import prüfen: Credentials erhalten (nur Vergleichsergebnis protokollieren), `.disaster-isolated` vorhanden, Startup verweigert, Hosts disabled, Sessions verworfen, queued/running Arbeit interrupted, alte Pläne und Inspections dauerhaft widerrufen. Private historische Evidenz muss lesbar/exportierbar bleiben. **Marker nicht entfernen, alte Arbeit nicht fortsetzen.** Spätere Aktivierung erfordert eigene lokale Migrationsprüfung. Task6-Grenzen zu Quarantäne, Credentials, Größen und App-Pfaden gelten weiter, siehe [Betrieb](../OPERATIONS.md).

## Tatsächliche Maschinenresultate einlesen

Die Maschinenprüfung ist `anker restore verify PLAN`. Der Befehl benutzt den Dienst und dessen fest begrenzten lesenden SSH-Helfer; das Labkit selbst führt kein SSH aus. HTTP 200 oder CLI 0 bedeutet nur „Versuch gespeichert“. Die Ausgabe ist ein Plan mit `verification_attempts`; nur der **neueste** Versuch wird ausgewertet. Es gibt keinen Maschinen-Gesamtpass.

```sh
# Nur nach Laborfreigabe am laufenden Labor-Anker; einmal frisch nach der Operation:
umask 077
anker --data /LABOR/data restore verify PLAN > LABOR/verification-plan.json
# Bytegenau aus /LABOR/data/plans/PLAN/target-snapshot.json privat kopieren;
# alternativ denselben Eintrag aus einem vorher geprüften Planpaket gewinnen.
# Keine Public-API-Inventarprojektion als private Baseline verwenden.
./docs/recovery-lab/lab-kit.py review --input LABOR/scenario-8-run.json \
  --plan LABOR/verification-plan.json --snapshot LABOR/target-snapshot.json \
  --preflight LABOR/preflight-before.json \
  --artifact LABOR/operator-evidence.json --artifact LABOR/dependencies.json \
  --artifact LABOR/PLAN.tar \
  --output LABOR/validated-dossier.json
```

Die tatsächlichen JSON-Dateien müssen 0600 in privaten Elternverzeichnissen liegen. Kein Überschreiben, keine Symlinkdateien. Der Validator prüft exakte Plan-/Backup-/Ziel-/Quellmanifest-/Aufnahme-/Verbindungshashes, bytegenauen privaten Aufnahmedigest, private positive Baseline/Capability, identische stabile Maschine, Erfassungs-/Versuchszeiten (±5 Minuten Uhrtoleranz) und separat gebundene Build-/Helferdigests im Preflight. Doppelte JSON-Schlüssel und Case-Aliase werden abgewiesen. Die Maschinen-ID entspricht exakt SHA256 der getrimmten **32 ASCII-Zeichen in kleinem Hex**, ausschließlich Nullen sind ungültig. Breite Inventar-Fingerprints und Raw-Maschinen-ID sind kein Ersatz.

Szenarien 1/2/3/8 brauchen diese frische Maschinenbeobachtung. Szenario 8 verlangt neue Boot-ID bei gleicher Maschine und echter gespeicherter Vorher-Baseline; in 1/2/3 ist unveränderte Boot-ID zulässig. Die drei festen Dienste müssen aktuell `active` sein; das belegt nur den Aktivzustand. Die Volumenprüfung vergleicht aufgenommenen Bestand, keine Inhalte/Backing/Gastfunktion. Ohne aufgenommenen Storage ist der Volumezustand ausdrücklich `not_applicable_no_recorded_storage`, im tatsächlichen Versuch bleibt er `unknown`. Fehlende erwartete Fakten, neuer negativer Versuch oder abweichende Bindungen verhindern ein vollständiges Dossier. Historische Versuche bleiben erhalten.

Szenarien 4/5/6/7/9 verwenden ihre Offline-/Fehler-/Cold-Belege; eine neue Plan-SSH-Abfrage ist dafür nicht anwendbar. Für Szenario 9 beispielsweise:

```sh
./docs/recovery-lab/lab-kit.py review --input LABOR/scenario-9-run.json \
  --artifact LABOR/cold-evidence.json --artifact LABOR/disaster.bundle \
  --artifact LABOR/anker-private-copy \
  --output LABOR/cold-dossier.json
```

`cold-evidence.json` enthält vom Operator datierte Ergebnisse und nachvollziehbare Ausgaben/Referenzen für tatsächliches Export/Inspect/Import, unveränderte private Credentials (keine Inhalte), Startup-Verweigerung, disabled Hosts, entfernte Sessions, interrupted Arbeit, persistente Plan-/Inspection-Widerrufe sowie erhaltene Sicherungs-/private Belegbytes. Jeder Checklistenschritt referenziert den SHA256 der entsprechenden Datei. Der Paketdigest muss zusätzlich exakt `cold_package_sha256` entsprechen; die private 0600-Kopie des verwendeten Anker-Binarys exakt `build_sha256`. **Keine Markerentfernung, Aktivierung oder Prüfung über den widerrufenen alten Plan.** Externe Referenzen und separat liegende Quellen werden im Cold-Inspect/Migrationsdossier ausdrücklich geprüft. Ein `pass` eines erwarteten Fehlerfalls bedeutet, dass dessen Verweigerung/Isolation beobachtet wurde, nicht dass Restore gelang.

Szenarien 1–8 benötigen zusätzlich das tatsächliche private Planpaket als `--artifact`, exakt passend zu `plan_package_sha256`. Szenario 4 benötigt keinen ausgeführten Hosthelfer und darf dessen Referenzen offenlassen.

Alle Szenarien benötigen jeden datierten menschlichen Schrittnachweis, ausdrücklich dokumentierte Isolation/Freigabe/Funktion/Abhängigkeiten und dessen tatsächlich mit `--artifact` gelesene Datei (höchstens 32 Dateien, je maximal 256 MiB). Fehlende Referenzen, offene Schritte und Restfehler lassen das Dossier offen. Mehrere Unterläufe werden im Beleg eindeutig getrennt; bei Szenario 3 beide Clusterfälle, bei 5 beide Unterbrechungen, bei 7 alle drei Driftarten. Diese inhaltliche Bewertung bleibt beim Operator: das Werkzeug prüft Bindungen, Dateien und strukturierte Maschinenfakten, nicht die Wahrheit beliebiger menschlicher Texte oder physische Isolation. Für größere Belege einen privaten, datierten Prüfbericht mit unabhängig geprüftem Digest des separat aufbewahrten Originals verwenden; das Cold-Paket selbst ist in diesem Kit auf 256 MiB begrenzt.

Exit 0 / `complete` bedeutet **vollständiges Belegdossier für dieses Szenario**, keine allgemeine Recovery-Freigabe oder Hardwareattestierung. Exit 2 / `incomplete` steht für fehlende/unbekannte/ungültige Belege, Exit 1 / `failed` für einen menschlich protokollierten fehlgeschlagenen Szenarioschritt. Ein negativer Maschinenversuch wird nicht zu einem positiven umgewertet. Gespeicherte Dossiers sind datierte Ergebnisse; vor weiterer Nutzung mit den aktuellen Originalbelegen erneut prüfen. Fremdgeschriebene JSON-Dateien und Prüfsummen ersetzen weder vertrauenswürdige Herkunft noch SSH/root/Helfervertrauen.

**Aktueller Stand: alle neun realen Szenarien unrun, kein isoliertes PVE-System vorhanden.** Lokale positive und negative Testfixtures beweisen nur die Werkzeuglogik.

Werkzeugtests (lokal, kein PVE-Zugriff):

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s docs/recovery-lab -p 'test_*.py'
```
