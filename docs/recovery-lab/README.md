# Operatorgeführte PVE-Abnahme

**Noch nicht auf realem PVE ausgeführt.** Lokale Fixture-Tests prüfen nur dieses Werkzeug. Es führt weder SSH noch Restore, Reboot, Cluster-, HA-, Ceph- oder Storageänderungen aus. Python 3 mit Standardbibliothek genügt. Auf dem späteren isolierten Laborhost aus einer privaten operatorverwalteten Ablage starten; keine Produktionssysteme verwenden.

## Vorbereiten

1. Privaten Arbeitsordner anlegen (`umask 077; mkdir LABOR`); `bindings.example.json` und `dependencies.example.json` dorthin kopieren. Keine tatsächlichen Zugangsdaten eintragen.
2. In Bindungen Labor-ID, erlaubte Szenarien (Zahlen 1–9), Person/UTC-Zeit, Quelle/Backup/Plan/Ziel/Fingerprints, PVE-Version, Build-ID, Paket-SHA256 und Boot-Baseline eintragen. Isolation mit Belegreferenz ausdrücklich bestätigen; physisch getrennte Netze, ausgeschlossene alte Nodes und Konsolenzugang durch Operator prüfen. Unbekannte Isolation stoppt die Erfassung.
3. Dossier mit Rolle, Ort/ID, Eigentümer, Prüfzeit und Status jeder externen Abhängigkeit ausfüllen. Gast-/PBS-Sicherungen, Schlüsselverwahrort, Disk-/Storage-Backing und Helferrechte müssen nachvollziehbar sein. Nur Referenzen, keine Secrets oder Config-Inhalte. Nicht benötigte Abhängigkeiten mit Begründung als `not_applicable` dokumentieren.
4. `preflight_reference` ist der geplante eindeutige Berichtspfad. `baseline_boot_id` vor Reboot lokal aus `/proc/sys/kernel/random/boot_id` notieren. Das ersetzt keine spätere plangebundene Baseline. Build und installierten Helfer als tatsächliche lokale Dateien für den Digest wählen; Paketdigest separat mit `sha256sum PLAN.tar` bestimmen.

```sh
# Ohne --collect-local-pve nur Bindungen/Dateidigests prüfen; keine Systemabfrage.
./docs/recovery-lab/lab-kit.py preflight \
  --bindings LABOR/bindings.json --output LABOR/preflight-before.json \
  --build /PFAD/anker --helper /PFAD/installierter-anker-host \
  --collect-local-pve
```

Das Werkzeug liest feste lokale Befehle: `pveversion`, `uname -r` und `systemctl is-active` für `pve-cluster`, `pvedaemon`, `pveproxy`. Es erfasst einen Hash von `/etc/machine-id`, die Boot-ID und freien Platz unter `/var/lib/anker-host`. Kein Configdump, keine Schlüssel, keine stderr-Rohdaten. Abfragen sind auf je 10 Sekunden/16 KiB begrenzt; Build/Helfer auf je 256 MiB, Eingabe-JSON auf 1 MiB. Berichte sind 0600 und überschreiben keine vorhandenen Dateien oder Symlinks. Die Elternalage muss privat und vertrauenswürdig sein.

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

An jedem **HALT** handelt nur der Operator, separat und ausdrücklich. `pass` dokumentiert eine menschliche Aussage mit Person, Zeit und privater Belegreferenz/Digest. `fail` ergibt `failed`; leer bleibt `unrun`. Restfehler eintragen. Checklisteninstruktionen nicht verändern. Bei Build-, Helfer-, Quell-, Ziel- oder Planwechsel neue Bindungen und neue Berichte erstellen.

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

Im privaten Import prüfen: Credentials erhalten (nur Vergleichsergebnis protokollieren), `.disaster-isolated` vorhanden, Startup verweigert, Hosts disabled, Sessions verworfen, queued/running Arbeit interrupted, alte Pläne und Inspections dauerhaft widerrufen. Private historische Evidenz muss lesbar/exportierbar bleiben. **Marker nicht entfernen, alte Arbeit nicht fortsetzen.** Spätere Aktivierung erfordert eigene lokale Migrationsprüfung. Task6-Grenzen zu Quarantäne, Credentials, Größen und App-Pfaden gelten weiter, siehe [Betrieb](../OPERATIONS.md).

## Ergebnis und noch offene Integration

`lab-kit.py` gibt 2 für unvollständig/noch nicht ausgeführt und 1 für protokollierte Fehler zurück. Aktuell gibt es bewusst keinen Gesamtstatus `passed`. Alle Operatorangaben bleiben menschliche Evidenz. Fixture-Erfolg ist niemals Laborerfolg.

**Task8 ist noch nicht implementiert:** Es fehlt der frische plangebundene Maschinenprüfvertrag, insbesondere gleiche Maschinenidentität, gebundene Vorher-Boot-ID, neue Nachher-Boot-ID und aktuelle Konfigurations-/Dienstnachweise. Die genaue Maschinenhashdefinition und Boot-Baseline müssen mit Task8 abgeglichen werden; der breite Inventar-Fingerprint ist keine stabile Maschinenidentität. Keine erfundene CLI und kein simulierter Pass. Task12 muss den tatsächlichen Task8-CLI-/Protokollvertrag einsetzen, Ergebnisse unverwechselbar an alle Bindungen koppeln und danach echte isolierte Abnahmen nachholen. Bis dahin bleiben alle neun Szenarien und Reboot-Abnahme offen.

Werkzeugtests (lokal, kein PVE-Zugriff):

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s docs/recovery-lab -p 'test_*.py'
```
