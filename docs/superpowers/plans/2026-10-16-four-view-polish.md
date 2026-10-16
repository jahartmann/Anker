# Four-view polish implementation plan

> **For agentic workers:** Use test-driven development for the approved design audit; implement independent view files in parallel and integrate centrally.

**Goal:** Make Übersicht, Hosts, Sicherungen and Aufträge clearer without changing backup or recovery semantics, then publish a signed stable update.

**Architecture:** Preserve existing React views, navigation guards, permissions and API contracts. Share compact spacing and accessible action styles; route overview metrics into existing filtered collections.

**Tech Stack:** React, TypeScript, Playwright, Go, GitHub Actions and Ed25519 release manifests.

**Spec:** User-approved four-view design audit in the current conversation.

## Constraints and review focus
- Remain on main; no new branches, dependency changes or production data operations.
- Preserve strict older/same-host backup comparisons, permissions and recovery navigation guards.
- Test empty and filtered collections, repeated metric navigation, narrow layouts and keyboard actions.
- Never invent progress or imply that source evidence proves real-world recovery.
- No isolated Proxmox environment exists; disclose remaining real restoration tests.

## Tasks
1. Add dedicated browser regressions for each view and overview metric navigation; run against the old embedded bundle and confirm expected failures.
2. Hosts: counted quick filters, relative backup age with exact dates, collapsed source evidence. Optional consumed initialStateFilter integrates overview navigation.
3. Sicherungen: primary Files action, accessible compact download, grouped comparisons and accurate latest-per-host labels.
4. Aufträge: consistent success wording, direct result links, matching/total counts, quick status filters and compact overview history.
5. Übersicht/shared styles: metric navigation through existing guards, concise attention rows, clearer actionable missing-backup status, unified accessible spacing/action sizes.
6. Rebuild embedded assets and native binary; run focused tests, full browser suite, Go race/vet, Python suites, responsive visual inspection and independent code review. Repair regressions only.
7. Write v0.2.12 release notes, commit/push main, tag only after local checks pass; wait for CI and release signing. Audit draft assets and signatures, publish stable, then verify public updater metadata and downloads.
