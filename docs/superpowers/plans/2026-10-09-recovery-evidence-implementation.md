# Recovery Evidence and File Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Implement the accepted audit recommendations and intuitive backup/restore folder navigation while preserving Anker's narrow recovery mission.

**Architecture:** Extend existing Go service and SQLite evidence records with backward-compatible optional fields and focused read-only operations. Reuse React file navigation and comparison components, keep permissions/integrity gates server-side, and preserve Python fixed-command SSH separation.

**Tech Stack:** Go 1.27, SQLite, Python host helper, React 19/TypeScript, Playwright.

**Spec:** `docs/superpowers/specs/2026-10-09-recovery-evidence-design.md`

## Global Constraints

- Scope is host-configuration backup, comparison and plan-bound recovery. No general SSH terminal, package/VM/resource/disk/HA/Ceph management.
- Preserve original manifest paths, original metadata, source integrity, secret permissions, strict SSH trust, drift gates, journals and separate mutation confirmations.
- Legacy backups/plans remain readable/exportable; absent evidence means unknown/missing, never proven.
- No production access, actual recovery execution, push, merge or release. User has no isolated PVE system: prepare a runnable acceptance package, leave real acceptance open.
- German product copy; shared muted green design; small text contrast at least 4.5:1 on white; desktop 1440x1000 and mobile 390x844.
- Preserve stale-response guards and invalidate decisions/evidence on source/target change. Human testimony and machine results are distinct.
- Use installed dependencies. Go: `GOCACHE=/tmp/anker-go-cache GOPATH=/tmp/anker-go-path /private/tmp/anker-toolchain/go/bin/go`; Node: `/opt/homebrew/bin`; browsers: `PLAYWRIGHT_BROWSERS_PATH=/tmp/anker-playwright`.
- No child subagents from implementers/reviewers. One implementation agent at a time. Commit only task files; no push.

## Review Focus

- Missing explicit directory entries, empty subfolders and very deep/long names must remain navigable without changing original paths (Task 1).
- Late async responses and switching host/source/target must not leak secret previews or transplant a previous decision (Tasks 1, 3, 4).
- A successful backup with old helper/missing account metadata or failed latest integrity attempt must not appear proven recoverable (Task 2).
- Encrypted imports, copied trees and plan packages with traversal/symlinks/corruption/foreign paths must fail without overwriting live state (Tasks 5, 6, 7).
- Restart/identity/service/volume evidence after interrupted verification and divergent cluster capture must stay unknown/failed and preserve history (Tasks 8, 11).

## Verification commands

Focused tests first; full relevant suite once per task. Controller runs final aggregate checks once, repeating only after material changes. Browser agents rebuild `bin/anker` from current embedded assets before focused browser cases. Reports include commands and red/green evidence, concerns, commit hashes and changed files. No visual claim without actual current render.

### Task 1: Shared folder navigation and intuitive file browsing

**Files:** Create `web/src/components/files/manifest.ts` and `FileNavigation.tsx`; modify `web/src/views/Backups.tsx`, `web/src/components/recovery/RecoverySource.tsx`, `web/src/styles.css`; test `web/tests/usability-backups.spec.ts` and a focused new `usability-file-navigation.spec.ts` if needed.

**Interfaces:** Pure `indexManifest(entries: Entry[]): Entry[]`, `parentPath(path: string): string`, and `visibleEntries(entries: Entry[], directory: string, query: string): Entry[]`. Shared navigation exposes directory/query/breadcrumb state without owning fetches, content, permissions or selected recovery paths.

- [ ] Write failing cases: search finds empty and synthetic nested folders; folder result navigates; file result has explicit containing-folder action; breadcrumbs/up/root preserve location; recovery selection survives folder/search/page changes; source change clears selection; protected preview remains masked.
- [ ] Run focused Playwright cases and record intended RED.
- [ ] Implement folder-first children, global folder+file search with full paths, explicit root/current-folder/up affordances and shared restore navigation. Retain 25/50 backup pagination and 25 recovery pagination. No recursive implicit folder selection. Keep fetch cancellation generations.
- [ ] Run build and focused cases; self-review; commit and report.

### Task 2: Structured coverage and proactive recovery readiness

**Files:** Create `internal/anker/readiness.go`, tests; modify `model.go`, `backup.go`, `dependencies.go`, `http.go`, `host/anker_host.py`, host tests; add types in `web/src/api.ts` and CLI adapter `internal/client/client.go`.

**Interfaces:** `Service.BackupReadiness(id string) (BackupReadiness, error)`; GET `/api/backups/{id}/readiness`; CLI `backup readiness ID`. Result contains individual evidence items `{id,state,title,detail,at,next_action}` and coverage roots/dependency paths/reasons, never an overall fully-recoverable boolean. Optional collection profile/dependency fields remain backward compatible.

- [ ] RED tests for old helper, missing mandatory queries/users/groups, unresolved extra path/link target, failed verification attempt and tampered manifest; old manifest has unknown effective profile.
- [ ] Persist actual authorized collector roots and bounded structured dependencies; do not change root-owned capture authority. Derive readiness from current validated manifest, inventory/capabilities, Backup.VerifiedAt plus no VerificationError, capture status and evidence age. Missing proof states remain separate.
- [ ] API/CLI tests enforce readable projection and secrets; run focused Go/Python; commit and report.

### Task 3: Direct comparison workspace, readiness UI and plan page

**Files:** `web/src/views/Backups.tsx`, `Hosts.tsx`, `Restore.tsx`, `Jobs.tsx`, `web/src/App.tsx`, `components/BackupComparison.tsx`, `components/recovery/RecoveryPlan.tsx`, `styles.css`, relevant browser tests.

**Interfaces:** Consume Task 2 readiness API. Existing ComparisonSelection remains compatible; explicit pair may additionally prefill nearest strictly older same-host backup. RecoveryPlan keeps props/state, changes outer Dialog to guarded page section.

- [ ] RED cases: direct same-host previous comparison; unavailable previous state explained; both comparison sources can be chosen for restore; source selection identity precision; readiness missing/stale state; plan page back/busy/mapping transitions; mobile keyboard focus.
- [ ] Move comparison to focused workspace with return to list, readable precise labels/full-ID details and compact summaries. Add direct row action. Surface coverage and last integrity attempt accurately in backup/host details and overview without new broad metrics. Plan page exposes next open task and file/task/evidence sections with retained confirmations. Jobs restore links remain intact. Fix shared small-text contrast.
- [ ] Run focused browser/build; migrate only plan-dialog-specific tests; commit/report.

### Task 4: Source-target-prepared comparison before decisions

**Files:** `internal/anker/recovery_snapshot.go`, `file_compare.go`, `recovery_http.go`, tests; `web/src/components/recovery/RecoveryDecisions.tsx`, `useRecoveryDraft.ts`, `web/src/api.ts`, tests.

**Interfaces:** A read-only preview endpoint for a validated draft snapshot/backup/mapping/path/basis using the same preparation and comparison logic as saved plans; exact endpoint and DTO documented in task report. Must not create a plan or mutate target. Comparisons always bind current snapshot ID and backup manifest hash.

- [ ] RED: actual original-target differences visible before choice; prepared mapping effect matches saved plan; identical source/target explained early; secret masking/rights, target switch, stale delayed response, unknown metadata and tampered snapshot all fail safely.
- [ ] Add lazy per-file comparison at decision with three explicit bases and expected action. Reuse FileComparison and server gates; no eager full content loading. Maintain typed preparation and unchanged-file normalization.
- [ ] Focused Go/browser tests and build; commit/report.

### Task 5: Rotating integrity checks and verified independent-copy receipts

**Files:** `internal/anker/scheduler.go`, `notify.go`, new `copy_verification.go`, tests; CLI/HTTP adapters; readiness/UI types and details.

**Interfaces:** Keep newest-complete daily verification. Additional maximum one older retained backup per host/run, protected first, never checked/oldest checked next. `Service.VerifyIndependentCopy(id, copyRoot string)` returns stored receipt bound to backup ID and current manifest hash after checking actual bytes in separate tree; CLI-only local path selection, sanitized web read projection.

- [ ] RED cases older pinned corruption detected, newest not double checked, bounded run, space failure deferred not corrupt; real copied tree mismatch/missing/tampered manifest/symlink/same source tree rejected; valid receipt becomes stale on different manifest.
- [ ] Implement bounded rotation and deduplicated integrity incident/recovery notification using existing maintenance locks. Verify copied material independently, no self-reported timestamps or general replicator. Surface date/provenance accurately.
- [ ] Focused Go/CLI tests; build; commit/report.

### Task 6: Encrypted cold disaster export, inspect and isolated import

**Files:** New `internal/anker/disaster.go`, tests; `cmd/anker/disaster.go`, main dispatch/help/tests; `docs/OPERATIONS.md`.

**Interfaces:** Local offline CLI `disaster export`, `disaster inspect`, `disaster import`, dispatched before Unix client requirement. Versioned authenticated encryption using existing x/crypto KDF, random salt/nonce; key/passphrase via restrictive file or terminal, never command-line secret. Inspect only counts/mode/version/prerequisites. Use InstanceLock for cold export/import.

- [ ] RED roundtrip restores raw credential catalog, settings, mode, app-local keys/trust and private plan evidence; encrypted tamper/wrong password fails; running instance, symlink/traversal, nonempty/live destination, unsafe external credential references and missing data rejected/explained without overwrites; sessions invalidated; old queued/running jobs not resumed.
- [ ] Cold consistency including SQLite WAL through read-only snapshot/checkpoint-safe strategy. Explicit package inventory/checksums; no arbitrary inclusion of paths referenced in catalog. Preserve permitted app-local files and classify required external TLS/key/config references with concrete migration instructions. Import stages to fresh root, rewrites owned root paths only, enforces permissions and catalog integrity, checks present backup data, reports absent sources, publishes atomically only after full preflight.
- [ ] Focused Go/CLI tests and document exact stopped-service workflow/limits. Self-review; commit/report.

### Task 7: Offline scenario dossier, package verifier and PVE lab kit

**Files:** `internal/anker/plan.go`, `recovery.go`, plan export tests; new package verifier/CLI; `docs/RECOVERY.md`, `docs/SUPPORT.md`, new `docs/recovery-lab/` scripts/templates.

**Interfaces:** Existing plan archive paths stay; top-level versioned index/checksums covers plan, prepared, target-before and original inventory. Local read-only verification command validates whole package safely with bounded entries/bytes. Optional structured recovery dependency references hold locations/roles/owner/date, no new secret capture.

- [ ] RED export contains human Evidence readable with origin/date and complete top-level checksums; corrupt/missing/traversal package fails; valid old exports report limited verification rather than forged proof.
- [ ] Generate practical per-scenario order/hold points and dependency dossier. Add executable non-destructive lab preflight/evidence capture plus manual test runner/checklist for standalone/replacement/isolated cluster/offline drill, interruption/low space/drift/reboot, binding build/helper/PVE/backup/plan IDs. Destructive laboratory steps require explicit operator action; no auto SSH writes/restore/reboot.
- [ ] Focused export/CLI/script tests; clearly leave real lab results unrun; commit/report.

### Task 8: Explicit machine-observed recovery verification

**Files:** New `internal/anker/recovery_verification.go`, tests; model/SSH/helper/recovery routes/CLI; RecoveryChecks/Plan UI and browser tests.

**Interfaces:** Optional `RecoveryVerifier` alongside Collector. Fixed helper `verify-recovery` operation and capability `recovery_verify_protocol:1`, allowed through --read-only. `Service.VerifyRecovery(ctx, planID)` retains bounded append-only attempts. POST plan verification route with recovery role, no arbitrary commands. Never changes operation success or human tasks.

- [ ] RED cases changed SSH trust/connection identity, missing or unchanged boot baseline, failed SSH/service/volume, helper unsupported, response corruption/interruption and old plan; repeated attempts preserve history and provenance; humans cannot create machine pass.
- [ ] Revalidate source/plan/private snapshot integrity and current connection before/after read. Compare stable hashed machine identity with matching captured evidence, not broad fingerprint that includes boot changes. Services from fixed allowlist and relevant storage/volumes only; command/query state/time explicit. Reboot requires valid captured boot baseline and different current boot ID of same identity. Current missing identity means unknown. No actual reboot or activation.
- [ ] Focused Go/Python/browser tests; CLI/Web show separate evidence history; commit/report.

### Task 9: Named protected reference and deterministic change summaries

**Files:** New backup reference service/tests; model/routes/CLI; BackupTable/Comparison/host detail and tests.

**Interfaces:** Optional name/reason/manual drill record on protected stand; one explicit reference per host, validation by host+manifest. Change summary uses existing CompareBackups semantics ignoring mtime-only in principal count.

- [ ] RED name/protection not equal drill-pass; reference cannot cross host; missing/deleted/corrupt ref shown unresolved; metadata-only changes counted, mtime noise separate; role enforcement.
- [ ] UI/CLI edit reference label and human drill evidence, directly compare against reference. Keep human success attributed and dated; no automatic rollback or policy enforcement.
- [ ] Focused Go/browser tests, build, commit/report.

### Task 10: Reviewed mapping recipes with fresh invalidation

**Files:** New recipe service/tests/routes/CLI; RecoveryMapping/useRecoveryDraft/UI/tests.

**Interfaces:** Versioned recipe records originating plan, reviewed mappings and source/target identity/topology/version constraints. Loading produces suggestions plus mismatch list, never confirms source-offline/console/file actions.

- [ ] RED stale NIC/MAC/PCI on different identity, backing/volume/version/topology mismatch, missing target proof, same name false match and permission/source invalidation.
- [ ] Save reviewed plan intent with labels/review date; preview applicability against fresh snapshot; only applicable reviewed choices prefill, mismatches require decisions. Existing preparation gates remain final authority.
- [ ] Focused Go/browser tests, commit/report.

### Task 11: Coordinated cluster recovery backup sets

**Files:** New `cluster_sets.go` and tests; model/jobs/routes/CLI; focused list/detail UI and tests.

**Interfaces:** Versioned set with expected membership, per-host backup IDs, capture start/end/skew and logical shared-pmxcfs hashes. Explicit capture request uses current backup job admission/exclusion; completion evidence uses real results, never merely job success.

- [ ] RED missing node, membership drift, different shared content, interrupted capture, stale preexisting backup mistakenly included and secret projection.
- [ ] Coordinate collection within available parallel limit, record each actual result, distinguish complete/agreed versus incomplete/divergent without atomic claim. Expose selection/export of set references for manual disaster source planning. No cluster management commands.
- [ ] Focused Go/browser tests; document source-set use; commit/report.

### Task 12: Aggregate verification, UI evidence and support documentation

**Files:** Current changed surfaces/tests as required by verification; `docs/IMPLEMENTATION_STATUS.md`, `docs/SUPPORT.md`, README/CLI help consistency.

- [ ] Verify every spec requirement against actual tasks and report evidence, identify unresolved external laboratory limits explicitly.
- [ ] Run full Go race/vet, Python helper/script suites, full browser tests, web/embed builds and Linux amd64/arm64 builds on final code. Debug actual failures with focused regression before rerun.
- [ ] Controller performs current in-app Desktop/Mobile render/keyboard checks for folder search/navigation, direct comparison, readiness, draft differences and plan page; screenshots outside repo.
- [ ] Update support matrix distinguishing implemented/local proof/real PVE open. Commit/report, then controller dispatches fresh whole-branch review and one reviewed fix wave if needed.
