# Access and updates implementation plan

**Goal:** Public GitHub distribution under MIT, signed in-place updates, mandatory web login with persistent 30-day sessions and complete account administration.

**Architecture:** The Anker service stays unprivileged. A separate root-owned updater listens on a group-restricted Unix socket and accepts only status, check and install requests. Its repository, signing key and installation paths come from a root-owned configuration. The updater verifies an Ed25519 manifest, stages the platform binary, drains the service, saves the previous binary and catalog, then atomically installs and checks the running version. Failure or interrupted installation restores the previous binary and catalog. No unattended installation by default.

**Task 1 — Access:** Add transactional account changes and hashed persistent session records; 30 days by default, configurable 1–365 days. Revoke sessions on password/permission changes, disable/delete and explicit logout. Protect the final active admin. Test restart persistence, expiry, revocation, concurrent account changes, cookies and HTTP authorization. Extend the existing access panel with session policy and account controls.

**Task 2 — Updates:** Add build version information, signed release metadata and a bounded GitHub client. Implement the restricted updater service, staged installation, exclusive locking, catalog snapshot and recovery journal. Test tampering, platform mismatch, download failures, busy jobs, successful installation, startup failure and interrupted installation. Add administrative web/CLI controls and systemd installation.

**Task 3 — Distribution:** Build and sign Linux amd64/arm64 releases in GitHub Actions. Include binary, installer, service units and host helper, checksums and release notes. Document first-install trust, signing-key setup, upgrade/rollback operation and supported limits in a plain administrator-facing README. Add MIT license, contribution/security guidance and repository settings recommendations. Do not publish to an unspecified repository.

**Verification:** Go race tests and vet, Python helper tests, TypeScript/Vite build, browser access/update flows, Linux cross builds, shell syntax and a rendered UI check.

**Review focus:** Root/user privilege boundary; restart-safe revocation; no final-admin lockout; rollback preserves compatible catalog state; installer never executes unsigned download; updates cannot interrupt an active restore.
