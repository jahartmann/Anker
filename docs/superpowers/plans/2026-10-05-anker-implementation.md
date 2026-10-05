# Anker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Eine lokal ausführbare Anker-Anwendung mit echtem Sicherungsformat, SSH-Erfassung, Web, CLI, TUI, Export und szenariobezogener Wiederherstellung bauen.

**Architecture:** Ein Go-Dienst mit SQLite-Katalog und geschützter Dateisystemablage. Ein eingebettetes React-Frontend und CLI/TUI verwenden dieselbe HTTP-API; lokale Bedienung verwendet einen Unix-Socket. Ein begrenzter Python-Hosthelfer erfasst Linux- und Proxmox-Konfigurationen und führt geprüfte Dateischritte aus.

**Tech Stack:** Go, modernc SQLite, Bubble Tea, Python 3 Standardbibliothek auf Hosts, React, TypeScript, Vite, Lucide.

**Spec:** ../specs/2026-10-05-anker-design.md

## Global Constraints

- Originalstände bleiben unverändert; Migrationen erstellen separate Pläne.
- Pflichtlücken sind sichtbar und verhindern automatisches Restore.
- VM-Daten sind außerhalb des Sicherungsumfangs.
- Vollständige Dateien, Metadaten und Anleitung bleiben ohne Dienst exportierbar.
- Nicht getestete Cluster- und Versionskombinationen erhalten manuelle Schritte, keine unbewiesene automatische Freigabe.
- Keine echten Hoständerungen, Veröffentlichung oder Produktionsinstallation in dieser Entwicklungssitzung.

## Review Focus

- Pfad- und Symlinkausbruch: Kein Lesen oder Schreiben außerhalb der vorgesehenen Ablage.
- Unvollständige oder veränderte Sicherung: Kein Restore-Erfolg trotz defekter Daten.
- Zieländerung zwischen Plan und Apply: Ausführung verweigern.
- Geheimnisse und Autorisierung: Keine Secret-Ausgabe für normale Leser oder unberechtigte Änderungen.
- Gleichzeitige Aufträge und Neustart: Keine überlappenden Hostschritte oder falsche Erfolgsmeldung.

## Dateistruktur und Schnittstellen

`cmd/anker/main.go` enthält die Befehle und Servereinrichtung. `internal/anker` enthält getrennte Dateien für Modell, Katalog, Sicherung, Archiv, SSH, Planung, Ausführung, Authentifizierung, HTTP und Scheduler. `internal/tui` enthält die Terminaloberfläche. `web/src` enthält App-Shell und Ansichten. `host/anker_host.py` ist der privilegierte Helfer. `deploy` enthält systemd und SFTP-Einrichtung. `docs` enthält Betriebs- und Recovery-Anleitungen.

Gemeinsame Typen sind `Host`, `Inventory`, `Entry`, `Manifest`, `Backup`, `Job`, `Plan`, `Step`, `Settings` und `User`. Der Dienst `Service` besitzt `Store`, `Root`, `Collector` und eine Auftragssteuerung. `Collector` bietet `Probe(ctx, Host)`, `Collect(ctx, Host, destination)` und `Apply(ctx, Host, Plan, backupRoot)`. Die lokale Testimplementierung ist auf explizite Demo-Hosts beschränkt.

### Task 1: Sicherungsformat und Katalog

Files: `internal/anker/model.go`, `store.go`, `backup.go`, `backup_test.go`, `store_test.go`, `go.mod`.

Interfaces: `OpenStore(path)`, `NewService(root, store, collector)`, `CreateBackup(ctx, hostID)`, `VerifyBackup(id)`, `ListBackups(hostID)`.

- [ ] Tests schreiben: `TestBackupPreservesMetadataAndPublishes`, `TestBackupRejectsTraversal`, `TestBackupDetectsTampering`, `TestMissingRequiredFilesIsPartial`, `TestStoreSurvivesReopen`.
- [ ] Tests ausführen und fehlende Implementierung nachweisen.
- [ ] Typen, SQL-Katalog, atomare Publikation und Manifestprüfung implementieren.
- [ ] `go test ./internal/anker` ausführen; alle Fälle müssen bestehen.
- [ ] Getesteten Stand lokal committen und im Fortschrittsprotokoll dokumentieren.

### Task 2: Hosthelfer und SSH

Files: `host/anker_host.py`, `host/test_anker_host.py`, `internal/anker/ssh.go`, `ssh_test.go`.

Interfaces: `SSHCollector` implementiert `Collector`; Helfer akzeptiert versioniertes JSON mit `probe`, `collect`, `apply` und festen Parametern.

- [ ] Tests schreiben: SQLite-Snapshot und Dateibaum müssen übereinstimmen; Symlinks dürfen keine fremden Dateien öffnen; Fehler bei fehlenden Pflichtpfaden; Ziel-Drift verhindert Apply; SSH verwendet strikte Hostschlüsselprüfung.
- [ ] Tests ohne Helfer/SSH-Implementierung ausführen.
- [ ] Inventar, SQLite Online Backup, Datenbankdecoder, tar-Transfer, eingeschränkte Operationen und Hostprüfung implementieren.
- [ ] `python3 -m unittest discover -s host -v` und `go test ./internal/anker` bestehen lassen.
- [ ] Getesteten Stand committen und protokollieren.

### Task 3: Portabler Export und Archivierung

Files: `internal/anker/archive.go`, `archive_test.go`, `recovery.go`.

Interfaces: `ExportBackup(id, writer)`, `ArchiveBackup(id)`, `EnsureReadable(id)`, `DiffBackups(from, to, path)`.

- [ ] Tests schreiben: Export enthält echte Inhalte und Anleitung; tar erhält Modus und Symlinks; Archiv lässt sich verifizieren und entpacken; korrupte Archive verändern keine Originalsicherung; Diff zeigt hinzugefügt/geändert/gelöscht.
- [ ] Fehlende Implementierung durch Testlauf nachweisen.
- [ ] Streams, Archivprüfung, sichere Extraktion, Pinning und lesbare Recovery-Anleitung implementieren.
- [ ] `go test ./internal/anker` bestehen lassen.
- [ ] Getesteten Stand committen und protokollieren.

### Task 4: Restore und Migration

Files: `internal/anker/plan.go`, `restore.go`, `plan_test.go`, `restore_test.go`.

Interfaces: `CreatePlan(request)`, `ApplyPlan(ctx, id, confirmation)`, `ExportPlan(id, writer)`; `PlanRequest` benennt Sicherung, Ziel, Szenario, Dateiauswahl und Zuordnungen.

- [ ] Tests schreiben: Netzwerkport wird korrekt umgeschrieben; unbekannte Portzuordnung blockiert; Hardwaredateien werden nicht pauschal übernommen; gesunder Cluster bekommt keinen Datenbanktausch; Versionswechsel wird manuell geführt; Ziel-Drift und falsche Bestätigung verhindern Ausführung.
- [ ] Fehlende Regeln im Testlauf nachweisen.
- [ ] Planung, Ziel-Fingerprints, vorbereitete Dateien, Haltepunkte, Audit und sichere Dateiwiederherstellung implementieren.
- [ ] `go test ./internal/anker` und Helfertests bestehen lassen.
- [ ] Getesteten Stand committen und protokollieren.

### Task 5: Aufträge und Betrieb

Files: `internal/anker/jobs.go`, `scheduler.go`, `notify.go`, `jobs_test.go`, `scheduler_test.go`.

Interfaces: `QueueBackup(id)`, `QueueProbe(id)`, `QueueApply(id, confirmation)`, `Scheduler.Tick(time)`.

- [ ] Tests schreiben: Hostsperre, begrenzte Parallelität, unterbrochene Aufträge beim Neustart, einmaliger Tageslauf bei Sommerzeitwechsel, letzter erfolgreicher Stand bleibt erhalten.
- [ ] Fehlende Funktionen im Testlauf nachweisen.
- [ ] Persistente Aufträge, Abbruch, Wiederholungen, Zeitpläne, Aufbewahrung, Benachrichtigung und Speicherprüfung implementieren.
- [ ] `go test -race ./internal/anker` bestehen lassen.
- [ ] Getesteten Stand committen und protokollieren.

### Task 6: API und Zugänge

Files: `internal/anker/auth.go`, `http.go`, `auth_test.go`, `http_test.go`.

Interfaces: `Handler(service, auth, local)`; JSON-API für Hosts, Backups, Dateien, Diff, Pläne, Aufträge, Einstellungen und Benutzer.

- [ ] Tests schreiben: Login und Ablauf; Leser darf keine Mutation oder Secret-Ausgabe; Herkunftsprüfung verhindert CSRF; Dateipfade sind eingeschränkt; localhost ohne Anmeldung bleibt geschützt.
- [ ] Fehlende Autorisierung im Testlauf nachweisen.
- [ ] Passwort-Hashing, Sessions, Rollen, Ausgabelimits, Audit und API implementieren.
- [ ] `go test -race ./internal/anker` bestehen lassen.
- [ ] Getesteten Stand committen und protokollieren.

### Task 7: Befehle Terminal und Installation

Files: `cmd/anker/main.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `deploy/anker.service`, `deploy/install.sh`, `deploy/setup-sftp.sh`, `Makefile`.

- [ ] Tests schreiben: CLI führt Sicherung und Export auf lokaler Demo aus; TUI-Navigation reagiert auf Tastatur und Maus; Fehler liefern Exitcode ungleich null.
- [ ] Fehlende Befehle und Terminalmodell im Testlauf nachweisen.
- [ ] Init, Serve, Demo, Status, Hosts, Backup, Diff, Export, Restore, Doctor, Benutzer und TUI implementieren; Linux-Installationsdateien hinzufügen.
- [ ] `go test ./...` und CLI-End-to-End-Prüfung bestehen lassen.
- [ ] Getesteten Stand committen und protokollieren.

### Task 8: Weboberfläche

Files: `web/src/App.tsx`, `api.ts`, `components`, `views`, `styles.css`, `web/tests/app.spec.ts`, `internal/webassets`.

Interfaces: API aus Task 6; Build wird in die Go-Binärdatei eingebettet.

- [ ] Browsertests schreiben: Login, Hostliste und Suche, Host hinzufügen, Sicherung, Dateiansicht, Diff, Export, Restore-Plan, Einstellungen und mobile Navigation.
- [ ] Testlauf vor UI-Implementierung dokumentieren.
- [ ] Design aus `docs/design/anker-hosts-concept.png` mit gemeinsamen Komponenten und tatsächlichen API-Aufrufen umsetzen.
- [ ] Typecheck, Build und Browsertests bestehen lassen; Desktop und Mobilansicht visuell prüfen.
- [ ] Getesteten Stand committen und protokollieren.

### Task 9: Gesamtabnahme und Dokumentation

Files: `README.md`, `docs/OPERATIONS.md`, `docs/RECOVERY.md`, `docs/SUPPORT.md`, `docs/IMPLEMENTATION_STATUS.md`.

- [ ] Vollständige Go- und Python-Tests, Race Detector, Vet, Webbuild, Browserabläufe und Linux-Crossbuild ausführen.
- [ ] Tatsächlich geprüfte Unterstützung von nicht durchführbaren echten Hardware-/Cluster-Labortests unterscheiden.
- [ ] Betriebsanleitung, SSH-Einrichtung, SFTP, Recovery ohne Dienst und Unterstützungsstufen dokumentieren.
- [ ] Einen unabhängigen Gesamtprüfer gemäß Executing-Plans-Skill beauftragen und wichtige Befunde mit Regressionstests beheben.
- [ ] Abschließenden Stand lokal committen und ausführbare Demo zur Ansicht öffnen.
