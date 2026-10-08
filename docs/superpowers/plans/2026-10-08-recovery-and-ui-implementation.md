# Wiederherstellung und Oberfläche Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The user must review this plan and select the execution method before implementation.

**Goal:** Einen einfach bedienbaren Wiederherstellungsablauf mit echtem SSH-Zielvergleich, eindeutigen Zuordnungen und konsistenter Oberfläche in allen Ansichten bauen.

**Architecture:** Der Go-Dienst entscheidet zentral über Szenario, Dateiumfang, Zuordnungen und Übernahme. Der feste Python-Hosthelfer liefert begrenzte lesende Zielaufnahmen; das vorhandene Schreib-/Rollbackjournal bleibt zuständig für bereits unterstützte automatische Einzeldateien. Web, CLI und Terminal verwenden dieselben Modelle und Regeln; Vergleich und Wiederherstellung werden in kleinere Komponenten aufgeteilt.

**Tech Stack:** Go 1.27.1, Python 3, React 19, TypeScript, Vite, vorhandenes Playwright. Keine neue Laufzeitabhängigkeit erforderlich.

**Spec:** [Freigegebener Entwurf](../specs/2026-10-08-recovery-and-ui-design.md).

## Global Constraints

- „Anker bleibt ein Werkzeug für Proxmox-Hostkonfigurationen.“ Gastdatenträger kommen aus vorhandenen Storages oder PBS.
- „Unveränderliche Sicherungen und das Hostjournal bleiben die Grundlage.“ Keine Veränderung bestehender Sicherungen bei Vorschau oder Vorbereitung.
- „Keine neue frei ausführbare Remote-Shell und keine Speicherung eines Rootpassworts in einem Plan.“ Hostschlüsselprüfung und getrennte SSH-Rollen erhalten.
- „Gleiche IDs belegen keine gleichen Pools, Pfade oder vorhandenen Volumes.“ Storageentscheidungen müssen ihre Belege nennen.
- „Keine Formatierung, Anlage, Umbenennung oder Datenverschiebung.“ Dies gilt auch für eine vermeintlich eindeutige Zuordnung.
- „Identische Dateien werden nicht erneut geschrieben.“ Inhalt UND Metadaten berücksichtigen.
- „Netzwerk, Cluster und Storageverwaltung haben eigene manuelle Schritte.“ Keine Freigabe über Konsolencheckbox, Demo oder API-Umgehung.
- „Alte Pläne bleiben lesbar und exportierbar; eine neue Freigabe erfordert einen neu erstellten Plan.“ Altes Hostjournal und bereits laufende Rücksetzungen bleiben lesbar.
- „Private Inhalte liegen in geschützten Dateien, nicht in Listenendpunkten, Logs oder ungeschütztem `plan.json`.“ Rechte 0700 für Verzeichnisse, 0600 für lokale Inhalte.
- „Durch Benutzer bestätigte Schritte werden als solche gekennzeichnet und ersetzen keinen maschinellen Nachweis.“ Kein pauschaler Erfolgsstatus nach Dateiübernahme.
- „Reale Abnahme bleibt erforderlich für Proxmox-Standalone, Hardwarewechsel, Clusterersatz, isolierten Clusterverlust, Netzwerkverlust, Reboot und Datenträger-/PBSzustand.“ Ohne diese Nachweise keine neue Gesamtautomatik.

## Review Focus

Diese fünf Bedingungen erhalten ausdrückliche Regressionen in den genannten Aufgaben:

1. Dateiaustausch durch einen fremden Prozess während der lesenden Aufnahme: kein Symlink-Follow, keine inkonsistente Vorschau und keine Freigabe; Aufgabe 2/3.
2. Gleich benannter Storage mit anderer VG, anderem Pool oder anderem Export: kein automatischer Identitätsbeweis und keine erfundene Volumeverfügbarkeit; Aufgabe 4.
3. Alte Node-Sicherung im gesunden Cluster: gemeinsame Konfiguration und fremde Nodepfade bleiben aus der Übernahme ausgeschlossen; Aufgabe 1/5.
4. SSH-Endpunkt ändert sich zwischen Planung und Schreiben: alte Freigabe verfällt, auch bei gleichem Hostnamen und Inhalt; Aufgabe 3/5.
5. Verlorene Antwort, zurückliegende Browserantwort oder Seitenwechsel: kein zweiter unkontrollierter Schreibversuch und kein Überschreiben des aktuellen Entwurfs; Aufgabe 8/9.

## Arbeitsort und Baseline

Vor Umsetzung den vorhandenen Arbeitsort und Git-Zustand prüfen; keine Nutzeränderungen überschreiben. Die derzeitigen Änderungen sind ausschließlich freigegebener Entwurf und dieser Plan. Worktree-Präferenz vor einer neuen Worktreeanlage klären; vorhandene Isolation wiederverwenden. Keine Veröffentlichung, Serververbindung oder Releaseinstallation gehört zur lokalen Umsetzung.

Im vorigen Untersuchungsschritt bestanden Go mit Race Detector, 53 Python-Helfertests, Webbuild und 13 gezielte Browserfälle. Nach Beginn der Umsetzung nur dann eine neue Baseline aufnehmen, wenn der Checkout oder die Abhängigkeiten seitdem verändert wurden.

Lokale Werkzeuge dieser Sitzung:

```sh
GOCACHE=/tmp/anker-go-cache GOPATH=/tmp/anker-go-path /private/tmp/anker-toolchain/go/bin/go test -race ./...
/Users/Janik/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3 -m unittest discover -s host -p 'test_*.py'
PATH=/opt/homebrew/bin:$PATH npm run build
PLAYWRIGHT_BROWSERS_PATH=/tmp/anker-playwright PATH=/opt/homebrew/bin:$PATH npm test
```

Die beiden npm-Befehle laufen in `web/`. Chromium benötigt auf diesem Mac einen Start außerhalb der Sandbox; die vorhandenen Browserdateien verwenden, keine Paketänderung dafür vornehmen. Baseline-/Screenshotdaten bleiben außerhalb des Repositorys.

## Dateigrenzen und gemeinsame Schnittstellen

| Datei / Gruppe | Verantwortung |
|---|---|
| `internal/anker/recovery_scenarios.go` | gemeinsame Situationen, Legacy-IDs, relevante Dateiauswahl und feste Erhaltung |
| `internal/anker/recovery_read.go`, `host/anker_host.py` | lesender SSH-Vertrag und Hostprofil-/Pfadgrenzen |
| `internal/anker/recovery_snapshot.go` | geschützte Zielaufnahmen, Kontextbindung, Integrität und Lebensdauer |
| `internal/anker/recovery_prepare.go`, `recovery_mapping.go` | bekannte Syntax und gezielte vorbereitete Änderungen mit Belegen |
| `internal/anker/plan.go`, `restore.go`, `restore_status.go` | Aktionen, Freigabe, Export und bestehende Journalzustände |
| `internal/anker/file_compare.go` | begrenzter Kontextdiff, separate Metadaten und Vergleichszustände |
| `internal/anker/recovery_http.go`, `http.go` | redigierte Vorschau-/Einzeldateiendpunkte und Rollen |
| `web/src/components/FileComparison.tsx`, `BackupComparison.tsx` | gemeinsame Inhaltsansicht und Sicherungsvergleich |
| `web/src/components/recovery/` | fünf Schritte, Entscheidungen, Planprüfung und Betriebsnachweise |
| `web/src/views/Restore.tsx`, `Backups.tsx`, `api.ts` | Integration und Typen, keine zweite fachliche Freigabelogik |
| `web/src/components/shared.tsx`, `styles.css`, alle Views | gemeinsame Layout-/Status-/Interaktionsverbesserungen |
| `internal/client/`, `internal/tui/` | dieselben Entscheidungen über CLI und Terminal |

Gemeinsame Typentscheidungen:

- `RecoverySituation`: `files`, `host`, `cluster-node`, `cluster-disaster`; bestehende `Plan.Scenario`-IDs bleiben gespeichert und lesbar. Legacy `standalone/migration/version/topology` werden `host` zugeordnet und behalten ihre zusätzlichen Einschränkungen.
- `FileDecision`: `backup`, `prepared`, `keep`, `manual`. `PlanRequest.Decisions map[string]FileDecision` ist additiv. Bestehende `Step.Action`-Werte `apply/manual` bleiben; `keep` kommt hinzu. `Step.Decision`, `Destination`, `Effects []RecoveryEffect` und `ComparisonState` werden ergänzt.
- `RecoverySelection`: `Paths []string`, `Preserved []RecoveryPreservation`; `RecoveryPreservation` enthält `Path` und `Reason`. Aufgabe 1 definiert diese Typen.
- `RecoveryEffect`: `Path`, `Field`, `Before`, `After`, `Reason`; `PreparedRecoveryFile`: `Content []byte`, `Destination string`, `Effects []RecoveryEffect`, `ManualTasks []RecoveryTask`. Aufgabe 4 definiert diese Typen.
- `RecoveryReadFile`: `Path`, `State`, `SHA256`, `Metadata *Entry`, `Content []byte`, `Error`. Zustände: `read`, `missing`, `unreadable`, `unsupported`, `too_large`, `changed`, `limit`. Inhalt nur im internen Modell, niemals in Status-/Planlisten. Aufgabe 3 definiert das Go-Modell; Aufgabe 2 liefert denselben JSON-Vertrag.
- Optionales `RecoveryReader`-Interface: `ReadRecoveryFiles(context.Context, Host, []string, []string) (RecoveryReadResult, error)`; der letzte Parameter enthält validierte benötigte Storage-IDs. Das bestehende `Collector`-Interface wird nicht erweitert; fehlende Fähigkeit bleibt sichtbar.
- `RecoveryReadResult`: `Files []RecoveryReadFile`, `StorageVolumes map[string]RecoveryVolumeResult`. `RecoveryVolumeResult`: `State`, `Volumes []string`, `Error`; `ok` mit leerer Liste ist von `unknown/failed/limit` verschieden.
- `RecoverySnapshot`: ID, BackupID, TargetID, Situation, SelectionHash, CapturedAt, ExpiresAt, Target Inventory, ConnectionSHA, Files-Metadaten und Content-Prüfsummen. `RecoveryInspection.SnapshotID` und `PlanRequest.SnapshotID` binden die Vorschau. `Plan.TargetSnapshotID` verweist auf die in den Plan kopierte Aufnahme.
- `ConnectionSHA` wird aus Host-ID, Adresse, Port, SSH-Benutzer und dem geprüften Hostschlüsselbezug berechnet; Schlüsseldateipfade stehen nicht in der Benutzeransicht.
- `RecoveryTask`: ID, Kind, Title, Instruction, Requires []string, State, Evidence. Bestätigung speichert Benutzer und Zeitpunkt; sie setzt `ApplyResult.RebootVerified` nicht.
- `FileComparison`: Path, BeforeLabel, AfterLabel, State, ContentChanged, MetadataChanges, Hunks, Error, Masked. Zustände: `identical`, `different`, `missing_before`, `missing_after`, `unavailable`, `binary`, `too_large`, `masked`. Aufgabe 6 definiert `MetadataChanges []MetadataChange` mit Field/Before/After und `Hunks []DiffHunk` mit BeforeStart/AfterStart und Lines; eine `DiffLine` enthält Kind (`context/add/remove`), Text und die jeweils vorhandenen Zeilennummern. Aufgabe 7 ergänzt die entsprechenden TypeScript-Typen.

## Task 1: Szenario und Erhaltung zentral festlegen

**Files:** Create `internal/anker/recovery_scenarios.go`, `internal/anker/recovery_scenarios_test.go`; Modify `internal/anker/recovery_mapping.go`, `model.go`.

**Interfaces:** Produces `recoverySituation(scenario string) (RecoverySituation, error)` und `scenarioSelection(m Manifest, r PlanRequest) (RecoverySelection, error)`. `RecoverySelection` enthält `Paths []string` sowie `Preserved []RecoveryPreservation` mit Pfad und Grund. Andere Aufgaben beziehen ihre Dateimenge ausschließlich hieraus.

- [ ] **1. Regressionen schreiben:** `TestClusterNodePreservesSharedAndForeignNodeFiles`, `TestFreshHostPreservesBootAndAccessIdentity`, `TestExplicitFilesKeepProtectedRules`, `TestLegacyRecoveryScenariosRemainReadable`. Fixture mit `etc/pve/storage.cfg`, `etc/pve/nodes/old/host.fw`, `etc/pve/nodes/other/qemu-server/100.conf`, `etc/fstab`, `etc/ssh/ssh_host_ed25519_key` und einer gewöhnlichen Config.

  ```go
  selection, err := scenarioSelection(manifest, PlanRequest{Scenario: "cluster-node"})
  if err != nil { t.Fatal(err) }
  // Gemeinsame und fremde Nodepfade müssen in Preserved und außerhalb Paths stehen.
  ```

- [ ] **2. Rot nachweisen:** `go test ./internal/anker -run 'Test(ClusterNode|FreshHost|ExplicitFiles|LegacyRecovery)'`; erwartet fehlende neue Auswahlregeln bzw. falsche relevante Pfade.
- [ ] **3. Auswahl implementieren:** bei Gesamtszenarien account-/boot-/zugangsbezogene Identität erhalten. Im gesunden Cluster gemeinsame `/etc/pve`-Konfiguration und alle fremden Nodes ausschließen; lokale Quellnode-Inhalte ausschließlich als manuelle Referenz anbieten. Explizit gewählte geschützte Einzeldateien bleiben manuell, ihre Auswahl erweitert keine Schreibfreigabe.
- [ ] **4. Grün prüfen:** neue Tests und bestehende `recovery_mapping_test.go`/`plan_test.go`; Legacyausgaben bei Bedarf um ausdrückliche Erhaltungsgründe erweitern.
- [ ] **5. Änderung einzeln prüfen und sichern:** diff mit Tests und fachlichen Regeln gemeinsam reviewen; einen ausschließlich dazugehörigen lokalen Commit erstellen, sofern der Ausführungsworkflow Commits vorsieht.

## Task 2: Begrenzte lesende Hostoperation

**Files:** Modify `host/anker_host.py`; Create `host/test_recovery_read.py`; Modify `host/test_restore_hardening.py`.

**Interfaces:** Produces `read_recovery_files(root, paths, storage_ids, profile_paths)` und feste Operation `read-recovery`. Envelope bleibt Version 1; Fähigkeit `recovery_read_protocol: 1` kommt neben unverändertem `restore_protocol: 2`. Lesende SSH-Rolle darf `read-recovery`, jedoch weiterhin kein apply/status/rollback.

- [ ] **1. Regressionen schreiben:** `test_read_authority_reads_without_mutation`, `test_symlink_swap_never_exposes_external_content`, `test_missing_unreadable_and_empty_are_distinct`, `test_profile_boundary_and_request_limits`, `test_storage_listing_failure_is_not_empty`. Leere reguläre Datei liefert `read` und ihren echten SHA; fehlende Datei liefert `missing` ohne Content. Explizite Includes außerhalb des Hostprofils werden abgewiesen.
- [ ] **2. Rot nachweisen:** `python3 -m unittest discover -s host -p 'test_recovery_read.py'`; fehlt die feste Operation, sollen die Verhaltenstests scheitern.
- [ ] **3. Helfer implementieren:** maximal 4096 eindeutige Pfade, 1 MiB Inhalt je Datei, 8 MiB Inhalt insgesamt und 32 angeforderte Storage-IDs. Grenzen ergeben explizite Zustände, keine abgeschnittenen scheinbar vollständigen Inhalte. Mit Verzeichnisdeskriptoren und `O_NOFOLLOW` jeden Pfadbestandteil öffnen; fstat, Inhalt und Metadaten vor/nach Lesen prüfen. Root-eigenes Hostprofil identisch zur Sammlung auswerten. `pvesm list <ID>` nur mit geprüfter ID und ohne Shell; Ausgabe/Timeout begrenzen. Keine Volumes anlegen.
- [ ] **4. Grün prüfen:** komplette Host-Python-Suite, einschließlich unverändertem Schreibjournal. Tests mit erwarteten Capabilities um den neuen additiven Wert erweitern.
- [ ] **5. Änderung prüfen und sichern:** Pfadgrenzen, fehlgeschlagene Reads und Leseberechtigung zusammen reviewen; nur Hostoperation und dazugehörige Tests sichern.

## Task 3: SSH-Transport und geschützte Zielaufnahme

**Files:** Create `internal/anker/recovery_read.go`, `recovery_snapshot.go`, `recovery_read_test.go`, `recovery_snapshot_test.go`; Modify `ssh.go`, `demo.go`, `recovery_mapping.go`, `model.go`.

**Interfaces:** Consumes Aufgabe 1/2. Produces `(*Service).CaptureRecovery(ctx context.Context, r PlanRequest) (RecoveryInspection, error)`, `(*Service).LoadRecoverySnapshot(id string, r PlanRequest) (RecoverySnapshot, error)` und `(*Service).ReadSnapshotFile(snapshotID, path string) (RecoveryReadFile, error)`. `InspectRecovery` delegiert an CaptureRecovery; fehlender Reader liefert einen ausdrücklich unvollständigen Vergleich.

- [ ] **1. Regressionen schreiben:** `TestRecoveryReadUsesBackupSSHAuthority`, `TestRecoverySnapshotRejectsChangedContent`, `TestRecoverySnapshotBindsConnectionAndSelection`, `TestRecoverySnapshotTamperingAndExpiry`, `TestOldReaderNeverInventsTargetContent`. Anfragen an alte Snapshot-ID mit anderem Ziel oder anderer Dateiauswahl scheitern. Leser liefert anderen Hash als Probe: kein freigegebener Snapshot. Änderung des Endpunkts bei gleichem Hostnamen invalidiert die Aufnahme.
- [ ] **2. Rot nachweisen:** `go test ./internal/anker -run 'TestRecovery(Read|Snapshot)|TestOldReader'`.
- [ ] **3. Transport und Aufnahme implementieren:** exakte Antwortmenge, Zustände, Hashes und Metadata validieren; doppelte/unerwartete Pfade oder abschließende JSON-Fragmente ablehnen. Antwortlimit 16 MiB. Ein fehlender Path gilt nur bei nachweislich vollständigem Inventar als `missing`. Aufnahme unter `recovery-inspections/<ID>` privat atomar schreiben und synchronisieren; Lebensdauer eine Stunde. Inhaltshashes und Kontext bei jedem Read prüfen. Erfasste Storageconfig und Volumeabfragen haben eigene nachvollziehbare Ergebniszustände; rohe Config mit Credentials bleibt ausschließlich in der privaten Aufnahme. Inventar-/Inspektionslisten liefern daraus nur bereinigte typisierte Storagebelege. Demo implementiert dieselben Modelle aus lokalen Dateien ohne Netzwerkzugriff. Snapshotbereinigung nimmt ausschließlich abgelaufene Aufnahmen unter eigener Sperre und nie kopierte Planinhalte weg.
- [ ] **4. Grün prüfen:** neue Go-Tests und SSH-/Demo-/Recoverytests mit Race Detector. `Collector`-Testdoubles für neue Inhalte gezielt mit `RecoveryReader` ergänzen; unbekannte Fähigkeit nicht pauschal in Produktion umgehen.
- [ ] **5. Änderung prüfen und sichern:** Transport, private Persistenz und Kontexthash als ein Deliverable reviewen.

## Task 4: Belegte Zuordnungen und gezielte Vorbereitung

**Files:** Create `internal/anker/recovery_prepare.go`, `recovery_prepare_test.go`; Modify `recovery_mapping.go`, `recovery_mapping_test.go`, `model.go`.

**Interfaces:** Consumes Snapshot/Selection. Produces `prepareRecoveryFile(e Entry, source []byte, snapshot RecoverySnapshot, mapping Mapping, scenario string) (PreparedRecoveryFile, error)`; Ergebnis Content, Destination, Effects und ManualTasks. Ergänzt `Mapping.Network map[string]RecoveryNetworkMapping` mit `AddressCIDR`, `Gateway`, `DNSServers`; bestehende Hostname-/Address-Felder bleiben lesbar, freie Legacy-Adresse bleibt manuelle Entscheidung.

- [ ] **1. Regressionen schreiben:** `TestStorageIDRequiresMatchingBacking`, `TestGuestMappingPreservesVolumeAndOptions`, `TestGuestCollisionAndMissingVolumeRemainManual`, `TestNetworkPreparationChangesOnlySelectedStanza`, `TestHostnamePreparationAvoidsGlobalReplacement`, `TestRootBootAndSwapMountsPreserved`. `scsi0: old:vm-100-disk-0,size=8G` darf zu `new:vm-100-disk-0,size=8G` vorbereitet werden; fremde Textvorkommen von `old:` bleiben erhalten. Gleich benanntes `local` mit anderem `vgname` ist kein bestätigter Match.
- [ ] **2. Rot nachweisen:** `go test ./internal/anker -run 'Test(StorageID|GuestMapping|GuestCollision|NetworkPreparation|HostnamePreparation|RootBoot)'`.
- [ ] **3. Parser und Regeln implementieren:** Storagecfg Typ, Backend, Pfad/Pool/VG, Server/Export, Content/Nodes getrennt analysieren. Nur geprüfte TargetIDs oder `manual` akzeptieren. Gastreferenzen in bekannten QEMU-/LXC-Feldern analysieren; Gast-/Volume-/Bridge-/Hook-/PCI-Kollisionen als konkrete Aufgaben nennen. Netzwerk nur bekannte Deklarationen und ausgewählte Stanzas ändern; IPv4/IPv6 über `net/netip` validieren, Portkollisionen und unbekannte Includes/Hooks manuell lassen. Hostname ausschließlich in `etc/hostname` und passenden Hostnametokens der ausgewählten `etc/hosts`-Zeile vorbereiten. Fstab nie pauschal auf neue UUIDs setzen. Jeder tatsächliche Ersatz erhält Effect mit Datei, Feld, altem und neuem Wert.
- [ ] **4. Grün prüfen:** neue und bestehende Mappingtests; zusätzlich Bond-Primary, VLAN-Alias, IPv6, NFS/PBS, ISO/CDROM, `unusedN`, CT rootfs/mp, hookscript und unbekannte Syntax als Table Tests. Keine neue automatische protectedPath-Freigabe.
- [ ] **5. Änderung prüfen und sichern:** vorbereitete Inhalte UND unverändert erhaltene Zielbestandteile reviewen.

## Task 5: Entscheidungen in Plan, Ausführung und Export

**Files:** Modify `internal/anker/model.go`, `plan.go`, `restore.go`, `restore_status.go`; Create `recovery_decisions_test.go`; Modify `plan_export_test.go`, `restore_result_test.go`.

**Interfaces:** Consumes Aufgaben 1–4. `CreatePlan(context.Context, PlanRequest) (Plan, error)` bleibt öffentlich. Neuer Plan hat SchemaVersion 2, TargetSnapshotID, Decisions, Preserved und Tasks. `(*Service).ConfirmRecoveryTask(ctx context.Context, planID, taskID, userID, evidence string) (Plan, error)` protokolliert ausschließlich erlaubte manuelle Aufgabenbestätigung.

- [ ] **1. Regressionen schreiben:** `TestKeepAndIdenticalFilesNeverReachApply`, `TestProtectedDecisionCannotEscalate`, `TestPlanRejectsSnapshotConnectionDrift`, `TestClusterPlanPreservesSharedFilesInExport`, `TestLegacyPlanReadableButNewApplyNeedsComparison`, `TestManualChecksDoNotClaimRebootProof`. Ein `keep`-Step mit geänderter Zieldatei wird weder vorbereitet noch zum Helfer geschickt. Bei identischem Inhalt und abweichendem Modus ist die Datei nicht „identisch“.
- [ ] **2. Rot nachweisen:** `go test ./internal/anker -run 'Test(Keep|ProtectedDecision|PlanRejectsSnapshot|ClusterPlan|LegacyPlan|ManualChecks)'`.
- [ ] **3. Planpfad implementieren:** neuen Szenariowert `host` und Legacy-IDs über Aufgabe 1 validieren; gültigen Snapshot laden, frische relevante Ziel-/Verbindungsprüfung durchführen und Aufnahme privat nach `plans/<ID>/target-files` kopieren. Angepasste geschützte Inhalte bleiben manuell. Bereits vorhandene `apply/manual`-Verbraucher um `keep` ergänzen. Planmanifest/-integrität umfasst Snapshot, Entscheidungen und Effects. Exporte enthalten `target-before/`, Erhaltungsgründe, passende Dateipfade und geordnete Voraussetzungen; Originale ausgeschlossener Quellconfigs bleiben ausschließlich als solche im Originalexport gekennzeichnet. Downloads behalten die bestehende Secrets-Anforderung und prüfen Quell- UND Zielinhalte. Bei fehlendem Snapshot darf CreatePlan einen neuen erfassen, gibt dessen Vergleich zurück und ersetzt dadurch keine ausdrückliche Schreibfreigabe. Alte Pläne bleiben sichtbar/exportierbar und journalfähig, neue automatische apply-Freigabe braucht Schema 2. Nutzer-ID einer Aufgabenbestätigung aus authentifiziertem Kontext beziehen, nicht aus dem Requestbody; unbekannte Tasks oder ungeprüfte Voraussetzungen ablehnen.
- [ ] **4. Grün prüfen:** Plan-, Export-, Apply-, Status-/Rollback- und Neustarttests mit Race Detector. Fehlende/fehlerhafte Snapshotdateien vor Hostmutation erkennen. Aufgabenbestätigung kann Blocker nicht überstimmen und markiert „vom Benutzer bestätigt“.
- [ ] **5. Änderung prüfen und sichern:** insbesondere Anzahl tatsächlich geschriebener Dateien, Exportrechte und Legacy-Recovery reviewen.

## Task 6: Gemeinsamer Dateivergleich und kompakte Backup-API

**Files:** Create `internal/anker/file_compare.go`, `file_compare_test.go`, `recovery_http.go`, `file_compare_http_test.go`; Modify `archive.go`, `http.go`.

**Interfaces:** Produces `compareFiles(before, after []byte, beforeEntry, afterEntry *Entry, beforeLabel, afterLabel string) FileComparison`. API: `POST /api/backups/compare` mit from/to liefert Zusammenfassung; `POST /api/backups/compare/file` mit from/to/path liefert eine Datei. `GET /api/plans/<ID>/comparison?path=...&basis=target|source|source-target` liefert ein redigiertes FileComparison; manuelle Aufgabenkonfirmation per `POST /api/plans/<ID>/checks/<TASK-ID>`.

- [ ] **1. Regressionen schreiben:** `TestComparisonShowsContextAndSeparateMetadata`, `TestComparisonNeverTreatsReadFailureAsMissing`, `TestComparisonMasksEitherSideSecret`, `TestComparisonLimitsLongAndBinaryFiles`, `TestCompareSummaryDoesNotLoadFileContent`, `TestPlanComparisonRequiresRestoreRole`, `TestRecoveryListsNeverExposeStorageCredentials`. Eine einzelne Änderung zwischen 200 identischen Zeilen darf nicht 400 entfernte/hinzugefügte Zeilen produzieren. Modusänderung bei identischem Inhalt erscheint nur in MetadataChanges. Zugangsdaten einer Target-storage.cfg erscheinen weder in `/status`, Plan-/Inspektionslisten noch in Vergleichsmetadaten.
- [ ] **2. Rot nachweisen:** `go test ./internal/anker -run 'Test(Comparison|CompareSummary|PlanComparison)'`.
- [ ] **3. Vergleich implementieren:** deterministischer zeilenweiser Kontextdiff mit drei Kontextzeilen; Arbeit auf höchstens 1 MiB je Seite und 10.000 Zeilen begrenzen, bei ausgeschöpftem Rechenbudget Zustand `too_large` mit Downloadhinweis. Keine quadratische unbeschränkte LCS-Matrix. Für Binärinhalt keine Textdiffs. Reads explizit auswerten. Vorhandenes `backups/diff` kompatibel lassen, jedoch Readfehler korrekt melden und Seiten als tatsächliche Sicherungen benennen. Secretanzeige muss explizites reveal und vorhandene Rechte verlangen; Liste enthält niemals Content. Source-/Target-Inventardetails, Effects und Tasktexte zentral redigieren, auch für alte Pläne; rohe Storageconfig oder credentialhaltige Fehlermeldungen nicht in öffentlichen Metadaten ausgeben. Rollen auf allen neuen Routen zentral prüfen.
- [ ] **4. Grün prüfen:** Go-Vergleichs-/HTTP-/Auth-/Archivtests; Readfehler, neu hinzugefügte/leere/entfernte Dateien, Symlinkziel, xattrs und pure MTime separat testen.
- [ ] **5. Änderung prüfen und sichern:** API-Summaries auf Geheimnisse und unnötige Readkosten reviewen.

## Task 7: Sicherungsvergleich als bedienbare Arbeitsansicht

**Files:** Create `web/src/components/FileComparison.tsx`, `BackupComparison.tsx`; Modify `views/Backups.tsx`, `api.ts`, `styles.css`; Create `web/tests/usability-comparison.spec.ts`.

**Interfaces:** `FileComparisonView({ comparison: FileComparison | null, loading: boolean, error: string, retry: () => void })`. `BackupComparison({ backups, user, notify, onRestore })` verwendet die API aus Aufgabe 6; `Backups` erhält dafür den vorhandenen Userkontext aus App.

- [ ] **1. Browserregressionen schreiben:** `comparison names both selected stands and direction`, `large summaries are filtered and paged with lazy content`, `late content response cannot replace current file`, `metadata binary errors and secrets remain distinct`. 1600 Änderungen: höchstens 25 Listeneinträge gleichzeitig; Öffnen einer Datei löst genau deren Inhaltsrequest aus. Umkehren tauscht Standnamen und Diff-Richtung.
- [ ] **2. Rot nachweisen:** `npm run build`, Binärdatei mit eingebetteter Oberfläche bauen und `npm test -- usability-comparison.spec.ts --project=demo`; neue Bedienung und Richtung müssen zunächst fehlen.
- [ ] **3. Komponenten implementieren:** zwei bewusst gewählte Stände, sichtbare Host-/Zeitangaben, hostübergreifender Vergleich ausdrücklich markiert. Auswahl direkt aus Tabelle oder bisherigem Standpaar möglich. Such-/Änderungs-/Metadatenfilter, Zähler, 25/50 pro Seite und separate Einzeldateivorschau. Requestgeneration schützt vor verspäteten Antworten. Diff zeigt Zeilennummern, Plus/Minus und Klartextstatus; Farbe nur ergänzend. Mobil Liste und Inhalt untereinander, Aktionen und Dateipfad umbrechen.
- [ ] **4. Grün prüfen:** Vergleichs- und vorhandene Sicherungsbrowserfälle; Tastatur, leerer Vergleich, geschützter Inhalt, 1440 × 1000, 1280 × 720 und 390 × 844 prüfen und Screenshots außerhalb des Repositorys speichern.
- [ ] **5. Änderung prüfen und sichern:** Dateiliste, beide Standnamen, Diff-Kontext und mobile Auswahl visuell reviewen.

## Task 8: Fünf Schritte für Vorbereitung und Zielanbindung

**Files:** Create `web/src/components/recovery/RecoveryWizard.tsx`, `RecoverySource.tsx`, `RecoveryTarget.tsx`, `RecoveryDecisions.tsx`, `useRecoveryDraft.ts`; Modify `Restore.tsx`, `RecoveryMapping.tsx`, `App.tsx`, `api.ts`; Create `web/tests/usability-recovery-wizard.spec.ts`.

**Interfaces:** `useRecoveryDraft(initialBackup, initialFile)` liefert Requestbody, aktuelle Stufe, Auswahl/Entscheidungen, SnapshotID und generationgebundene Prüffunktionen. `RecoveryWizard({status,user,initialBackup,initialFile,onPlan,onClose,onConnectTarget,notify})` zeigt Stufen 1–4; Stufe 5 gehört zum gespeicherten Plan. `App` verwaltet die Rückkehr aus vorhandener Hosts-Anbindung und erhält den Recoveryentwurf bis zur Rückkehr.

- [ ] **1. Browserregressionen schreiben:** `four situations retain draft through back navigation`, `connect target returns to same recovery draft`, `context change invalidates only dependent decisions`, `late inspection cannot replace new target`, `blocked step shows next concrete action`, `ordinary files avoid irrelevant mappings`. Nichtadministratoren bekommen die vorhandene nötige Anbindungsaktion als Hinweis; keine erhöhte Hostberechtigung.
- [ ] **2. Rot nachweisen:** Browserfälle mit eingebettetem aktuellen Webbuild starten; erwartet fehlende Schrittleiste bzw. verlorener Entwurf bei Zielanbindung.
- [ ] **3. Arbeitsansicht implementieren:** langen Wizarddialog durch eigene Arbeitsfläche ersetzen. Quelle/Situation, SSH-Zielprüfung, Entscheidungen und abschließende Planprüfung jeweils separat. Sichtbare Schrittleiste, feste erreichbare Fußaktionen und Zähler offener Entscheidungen. Netzwerk/Storage/Identität/Dateien nur bei Relevanz. Änderungen an Quelle/Ziel/Dateimenge invalidieren den Snapshot; Entscheidungen ohne geänderte Voraussetzung bleiben erhalten. Keine Speicherung geheimer Inhalte in localStorage. Konsole/Quellisolierung erst im passenden Schritt bestätigen, nicht durch frühere Haken vortäuschen.
- [ ] **4. Grün prüfen:** neue und vorhandene Recoverytests; bestehende Tests auf die neue Navigation umstellen, ihre Zusicherungen zu Requestverlust, Auswahl, Grenzen und Rollen erhalten. Schließen/Seitenwechsel bei laufender Mutation weiterhin absichern.
- [ ] **5. Änderung prüfen und sichern:** alle vier Situationen mit echten Demoendpunkten durchlaufen; keine automatisierte Gesamt-Recoverywirkung behaupten.

## Task 9: Planvergleich, Ausführung und Betriebsnachweise

**Files:** Create `web/src/components/recovery/RecoveryPlan.tsx`, `RecoveryChecks.tsx`; Modify `Restore.tsx`, `api.ts`, `styles.css`; Extend `web/tests/usability-recovery-wizard.spec.ts`, `usability-recovery-inspection.spec.ts`.

**Interfaces:** Consumes Plan/Comparison/Tasks aus Aufgabe 5/6. `RecoveryPlan({plan,status,user,notify,refresh,onRevise,onClose})` übernimmt vorhandene apply/rollback/reconcile-Steuerung; `RecoveryChecks({plan,onConfirm})` zeigt Herkunft jedes Nachweises.

- [ ] **1. Browserregressionen schreiben:** `plan compares actual target against prepared content`, `keep files and preserved system parts are explicit`, `lost apply reply remains uncertain after reopen`, `checks never imply verified reboot`, `legacy plan requires new preparation`. Ungeklärte Ausführung bleibt auch nach Neuöffnen gesperrt; Serverzustand und Hostjournal haben Vorrang vor lokalem UIzustand.
- [ ] **2. Rot nachweisen:** bestehende API plus fehlende Vergleichs-/Nachweiskomponenten müssen die neuen Erwartungen verfehlen.
- [ ] **3. Planansicht implementieren:** Quellen-/Ziel-/Vorbereitungsvergleich, tatsächliche Dateiaktionen, Erhaltung und Effects anzeigen. Manuelle Aufgaben in ausführbarer Reihenfolge samt Voraussetzung und Bestätigungsherkunft. Bestehende separate Plan-ID-Bestätigungen beibehalten. Nach Abbruch reconcile als primäre nächste Aktion anbieten; erneutes apply bleibt gesperrt, bis Server und Journal einen eindeutigen Zustand liefern. Version-/Cluster-/Netzwerkgrenzen sichtbar neben der betroffenen Aktion statt nur in globalen Warntexten.
- [ ] **4. Grün prüfen:** alte und neue Recoverybrowserfälle, API-Fehler 4xx/5xx, Antwortverlust, Jobwechsel, Dialog-Neuöffnung, Tastatur und mobile Nachweise.
- [ ] **5. Änderung prüfen und sichern:** Statuswörter gegen tatsächlichen Plan-/Journalzustand prüfen; keine unbelegte Erfolgsmeldung.

## Task 10: Gemeinsame UI-Verbesserungen in allen Views

**Files:** Modify `web/src/components/shared.tsx`, `ActiveJobs.tsx`, `Inventory.tsx`, `styles.css`, `App.tsx`, `views/Hosts.tsx`, `Jobs.tsx`, `Settings.tsx`, `Storage.tsx`, `Access.tsx`, `Updates.tsx`, `Certificates.tsx`; Extend `web/tests/usability-design.spec.ts`, `usability-settings.spec.ts`, `usability-jobs.spec.ts`.

**Interfaces:** Additive Props für Heading-Ergebniszahl, gemeinsame kompakte Filter-/Aktionsleisten und aufklappbare Detailabschnitte. Keine neue konkurrierende Statusberechnung; `hostState`, Serverwerte und existierende Zustandslabels verwenden.

- [ ] **1. Verhaltensregressionen schreiben:** `view actions remain reachable at narrow viewport`, `settings save reports local errors and dirty state`, `recovery job result links to reconciliation`, `inventory distinguishes failed query from absence`. Tests für reine Abstände/Farben vermeiden; sie werden visuell geprüft.
- [ ] **2. Rot nachweisen:** Browserfälle müssen die fehlende Navigation, lokale Fehlerzuordnung bzw. nicht erkennbare Abfragefehler zeigen.
- [ ] **3. Views verbessern:** Übersicht priorisiert offene Probleme mit direkter Handlung; Hosts trennen Verbindung/Inventar/letzte Sicherung; Jobs zeigen Ergebnis und nächsten Schritt; Settings erhalten erreichbare Speicheraktion und lokale Validierung; Storage trennt Werte/Trend/Handlung; Zugriff zeigt Rolle/Secretrechte eindeutig; Updates/Zertifikate zeigen Zustand und nächste Aktion zuerst. Gemeinsame Fokus-, Umbruch-, Lade-/Fehler-/Leer- und Seitenmuster aktualisieren. Bestehenden ruhigen Anker-Stil erhalten; Änderungen in gemeinsame CSS-Regeln integrieren, keine weiteren widersprüchlichen Override-Blöcke anhängen.
- [ ] **4. Grün prüfen:** entsprechende Browserfälle und alle Hauptansichten einschließlich Einstellungsbereiche bei Desktop, Laptop und 390 Pixel. Keine relevanten Consolefehler, abgeschnittenen Aktionen oder unerreichbaren Fokusziele. Demo-unverfügbare Systemfunktionen separat als ungetestet kennzeichnen.
- [ ] **5. Änderung prüfen und sichern:** jede View gezielt visuell reviewen; keine behauptete globale Barrierefreiheitszertifizierung.

## Task 11: CLI und Terminal auf dieselben Regeln bringen

**Files:** Modify `internal/client/client.go`, `recovery_test.go`, `internal/tui/recovery.go`, `forms.go`, `view.go`, `actions.go`; Create `internal/tui/recovery_comparison_test.go`; Modify `deploy/anker.1`.

**Interfaces:** CLI ergänzt `restore compare PLAN --path PATH --basis target|source|source-target`, `restore check PLAN --task ID --evidence TEXT` und `restore plan --decision PATH=backup|prepared|keep|manual`. Bestehende Flags `--storage`, `--hostname`, `--address`, apply/status/rollback bleiben. TUI übernimmt SnapshotID, Decisions und Tasks aus den vorhandenen API-Aufrufen.

- [ ] **1. Regressionen schreiben:** `TestCLIRecoveryComparisonAndDecisionParsing`, `TestTUIRecoveryPreservesDraftAndShowsTargetDiff`, `TestTUIProtectedDecisionCannotEnableApply`, `TestCLIManualCheckNotRebootProof`. Mehrfach/leer/falsch formulierte Entscheidungen ablehnen; Vergleich verlangt keinen Schreibauftrag.
- [ ] **2. Rot nachweisen:** `go test ./internal/client ./internal/tui -run 'Test(CLIRecovery|TUIRecovery|TUIProtected|CLIManual)'`.
- [ ] **3. Bedienung implementieren:** vier Situationen mit lesbaren Legacynamen, passende Zuordnungen, erhaltene Zielteile, tatsächlichen Zielvergleich und geordnete Aufgaben zeigen. Ausführung und Rücksetzung behalten ihre Bestätigungen. Neue Flags in Hilfe/Manpage und vorhandenen Help-Tests verankern; keine unabhängige TUI-/CLI-Freigaberegel.
- [ ] **4. Grün prüfen:** gesamte Client-/TUI-Suite und relevante PTY-Prüfungen, einschließlich schmalem Terminal und Abbruch/Zurücknavigation.
- [ ] **5. Änderung prüfen und sichern:** identische Pläne über Web und CLI/TUI abrufen und Aktionen/Blocker vergleichen.

## Task 12: Integration, frische Abschlussprüfung und Dokumentation

**Files:** Modify `docs/RECOVERY.md`, `docs/RECOVERY_REVIEW.md`, `docs/IMPLEMENTATION_STATUS.md`, `docs/SUPPORT.md`, `README.md`, `CHANGELOG.md`; Add/extend betroffene Integrationsregressionen statt generischer Testattrappen.

**Interfaces:** Keine neue Produktfunktion. Bestätigt Aufgaben 1–11 an unverändertem finalem Code und beschreibt dessen tatsächlichen Umfang.

- [ ] **1. Durchgehende Fälle ergänzen:** `TestRecoveryTargetComparePrepareApplyReconcileRoundTrip`, `TestRecoveryHelperUnavailableKeepsExport`, `TestRecoveryExportWithoutRunningService`. Dienst + echter lokaler Python-Helfer mit temporären Dateien verwenden, soweit möglich; SSH-Aufruf kann über den vorhandenen festen Transportfixture geführt werden. Neues Lesen, Planintegrität, Schutzrechte und bestehendes Journal zusammen prüfen.
- [ ] **2. Neue Integrationslücken rot nachweisen:** Fehler muss fehlende Verknüpfung zwischen Aufnahme, Planung, Vergleich und Ausführung belegen; keine Assertions lediglich auf Testmockantworten.
- [ ] **3. Integrationsfehler beheben und Dokumentation aktualisieren:** fünf Schritte, alte Helfer aktualisieren, tatsächliche Mappings und manuelle Grenzen, Konten/Metadaten, Snapshotdrift, Journalabgleich und Export ohne Dienst. Unterstützungsstand trennt lokale Nachweise und reale Proxmox-Abnahme. Quell-/Zielsicherungen und Exporte unverändert lesbar.
- [ ] **4. Abschlussprüfung auf finalem Stand:** `go test -race ./...`, `go vet ./...`, beide Python-Suites für host und scripts, `npm run build`, finale Binärdatei bauen und gesamte `npm test` ausführen. Linux amd64/arm64 crossbuild. Desktop-/Mobil-/Tastaturprüfung aller betroffenen Views mit finaler eingebetteter Oberfläche. Bereits bestandene Prüfungen nur nach weiteren Änderungen oder neuen Befunden wiederholen.
- [ ] **5. Frische unabhängige Codeprüfung und Übergabe:** Schnittstellen, Rechte, Snapshot-/Planintegrität, Pfadgrenzen, Drift, Legacyzustände und Erfolgsmeldungen prüfen. Befunde mit Regression korrigieren; nach Änderungen betroffene Prüfungen erneut ausführen. Ohne echte Proxmoxumgebung bleibt neue Gesamtautomatik gesperrt. Lokale Änderungen und verbleibende Abnahmegrenzen konkret übergeben; kein Release oder Produktivschreiben ohne eigenen Auftrag.

## Abhängigkeiten und Abnahmekriterien

Reihenfolge: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12. Aufgabe 10 enthält unabhängige UI-Arbeit, bleibt aber hinter der Recoveryintegration, damit gemeinsame Komponenten nur einmal geformt werden. Bei delegierter Umsetzung teilen Aufgaben 3–6 ihre oben definierten Typen und müssen nacheinander integriert werden.

Fertig ist die lokale Umsetzung, wenn:

- alle vier Situationen eine nachvollziehbare relevante Auswahl und Erhaltung liefern;
- Zielvergleich echte gelesene Inhalte und Metadaten zeigt, unbekannte Zustände sperrt und Secrets schützt;
- jede Zuordnung ihre Wirkung nennt und keine Storage-/Netzwerk-/Clusterwirkung vortäuscht;
- gewöhnliche freigegebene Einzeldateien weiterhin kontrolliert über das vorhandene Journal laufen;
- alte Backups/Pläne lesbar sind und alte Helfer sichtbar aktualisiert werden können;
- Web, CLI und Terminal dieselbe fachliche Entscheidung zeigen;
- Vergleich und alle Views bei 1440, 1280 und 390 Pixel bedienbar geprüft sind;
- die vollständigen lokalen Prüfungen am finalen Code bestanden und neue Befunde bearbeitet sind;
- echte Proxmox-/Reboot-/Cluster-/Datenträgernachweise als separat ausstehend dokumentiert bleiben.

## Ausführungsentscheidung

Planstatus: vom Benutzer freigegeben; Umsetzung mit Subagenten am 8. Oktober 2026 begonnen.

Empfehlung: direkte Umsetzung hier im Chat (`superpowers:executing-plans`) Aufgabe für Aufgabe mit frischer unabhängiger Abschlussprüfung. Die Aufgaben hängen stark von denselben Snapshot-, Plan- und Vergleichsschnittstellen ab; eine durchgehende Implementierung vermeidet konkurrierende Modelländerungen. Alternativ kann der Benutzer delegierte Umsetzung mit einzelnen Implementierungs- und Reviewagenten pro Aufgabe wählen (`superpowers:subagent-driven-development`). Vor der Umsetzung muss der Benutzer den Plan prüfen und eine Methode wählen.
