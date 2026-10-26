import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import type { APIRequestContext, Page } from "@playwright/test";
let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await demoCookies(request);
});
test.beforeEach(async ({ page }) => {
  await page.context().addCookies(cookies);
});
async function closePlan(page: Page) {
  await page
    .getByRole("button", { name: "Zurück zu Plänen", exact: true })
    .click();
  if (
    await page
      .getByText("Eingaben auf dieser Planseite sind noch offen.")
      .isVisible()
  )
    await page.getByRole("button", { name: "Eingaben verwerfen" }).click();
}
async function setup(page: Page) {
  const base = await (await page.request.get("/api/status")).json();
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[0].id);
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("etc/network/interfaces");
  await page.getByLabel("etc/network/interfaces", { exact: true }).check();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[0].id);
  return base;
}
function inspected(base: any) {
  return {
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    ports: [
      {
        name: "eno1",
        mac: "00:11:22:33:44:55",
        pci: "0000:01:00.0",
        reason: "Bridge vmbr0",
        suggested: "enp7s0",
      },
    ],
    target_ports: [{ name: "enp7s0", mac: "02:11:22:33:44:55" }],
    storage: [],
    target_storage: [],
    blockers: [],
    warnings: [],
    manual: ["Netzwerk über Konsole aktivieren"],
    requires_console: true,
    requires_source_offline: false,
    automatic: false,
  };
}
test("fresh inspection shows only necessary mappings before creating a plan", async ({
  page,
}) => {
  let base: any;
  let creates = 0;
  await page.route("**/api/plans/inspect", (r) =>
    r.fulfill({ json: inspected(base) }),
  );
  await page.route("**/api/plans", (r) => {
    if (r.request().method() !== "POST") return r.continue();
    creates++;
    return r.fulfill({ status: 503, json: { error: "Nicht erstellen" } });
  });
  base = await setup(page);
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await expect(page.getByLabel("eno1 → Zielport", { exact: true })).toHaveValue(
    "enp7s0",
  );
  await expect(page.locator(".recovery-mapping select")).toHaveCount(1);
  await expect(
    page.getByLabel("Alter Host ist ausgeschaltet oder isoliert"),
  ).toHaveCount(0);
  expect(creates).toBe(0);
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("etc/network/interfaces");
  await page.getByLabel("etc/network/interfaces", { exact: true }).uncheck();
  await expect(page.getByLabel("eno1 → Zielport", { exact: true })).toHaveCount(
    0,
  );
  await expect(
    page.getByRole("button", { name: "Plan prüfen", exact: true }),
  ).toHaveCount(0);
});
test("failed inspection retries the same file draft and prevents duplicate or closed requests", async ({
  page,
}) => {
  let base: any;
  let attempts = 0;
  let release!: () => void;
  const hold = new Promise<void>((resolve) => (release = resolve));
  const payloads: any[] = [];
  await page.route("**/api/plans/inspect", async (r) => {
    payloads.push(r.request().postDataJSON());
    attempts++;
    if (attempts === 1)
      return r.fulfill({
        status: 503,
        json: { error: "Inventar nicht erreichbar" },
      });
    await hold;
    return r.fulfill({ json: inspected(base) });
  });
  base = await setup(page);
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Inventar nicht erreichbar",
  );
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("etc/network/interfaces");
  await expect(
    page.getByLabel("etc/network/interfaces", { exact: true }),
  ).toBeChecked();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Ziel wird geprüft …", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("region", { name: "Wiederherstellung vorbereiten" }),
  ).toBeVisible();
  release();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await expect(
    page.getByLabel("eno1 → Zielport", { exact: true }),
  ).toBeVisible();
  expect(payloads[0].files).toEqual(["etc/network/interfaces"]);
  expect(payloads[1].files).toEqual(["etc/network/interfaces"]);
  expect(attempts).toBe(2);
});

test("duplicate physical port choices block preparation and storage stays an explicit manual decision", async ({
  page,
}) => {
  let base: any;
  let payload: any;
  await page.route("**/api/plans/inspect", (r) =>
    r.fulfill({
      json: {
        ...inspected(base),
        ports: [
          { name: "eno1", reason: "vmbr0", suggested: "enp7s0" },
          { name: "eno2", reason: "bond0", suggested: "enp8s0" },
        ],
        target_ports: [
          { name: "enp7s0", mac: "02:11:22:33:44:55" },
          { name: "enp8s0", mac: "02:11:22:33:44:56" },
        ],
        storage: [
          {
            id: "local-zfs",
            kind: "zfspool",
            path: "rpool/data",
            reason: "storage.cfg",
          },
        ],
        target_storage: [],
      },
    }),
  );
  await page.route("**/api/plans", (r) => {
    if (r.request().method() !== "POST") return r.continue();
    payload = r.request().postDataJSON();
    return r.fulfill({ status: 503, json: { error: "Prüfung unterbrochen" } });
  });
  base = await setup(page);
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await expect(
    page.getByLabel("local-zfs → Zielspeicher (manuell)", { exact: true }),
  ).toHaveValue("");
  await page
    .getByLabel("local-zfs → Zielspeicher (manuell)", { exact: true })
    .selectOption("manual");
  await page
    .getByLabel("eno2 → Zielport", { exact: true })
    .selectOption("enp7s0");
  await expect(page.getByRole("alert")).toContainText("Mehrere Quellports");
  await expect(
    page.getByRole("button", { name: "Zuordnungen prüfen", exact: true }),
  ).toBeDisabled();
  await page
    .getByLabel("eno2 → Zielport", { exact: true })
    .selectOption("enp8s0");
  await page
    .getByLabel("Entscheidung für etc/network/interfaces", { exact: true })
    .selectOption("prepared");
  await page
    .getByRole("button", { name: "Zuordnungen prüfen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  await page.getByLabel("Konsolenzugang zum Ziel ist verfügbar").check();
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Prüfung unterbrochen");
  expect(payload.mapping.storage).toEqual({ "local-zfs": "manual" });
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await expect(
    page.getByLabel("local-zfs → Zielspeicher (manuell)", { exact: true }),
  ).toHaveValue("manual");
});

test("uncertain host state is reconciled without reapply and rollback requires a separate confirmation", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  let plan = {
    id: "journal-plan",
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    scenario: "files",
    created_at: "2026-10-07T10:00:00Z",
    state: "interrupted",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    steps: [{ path: "etc/sysctl.conf", action: "apply", reason: "" }],
    blockers: [],
    manual: [],
    mapping: { interfaces: {}, storage: {} },
    result: {
      state: "interrupted",
      operation_id: "journal-plan",
      applied: ["etc/sysctl.conf"],
      rollback_path: "/var/lib/anker/rollback/journal-plan",
      checks: [],
      reboot_verified: false,
    },
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, demo: false, plans: [plan] } }),
  );
  await page.route("**/api/plans/journal-plan", (r) =>
    r.fulfill({ json: plan }),
  );
  await page.route("**/api/plans/journal-plan/reconcile", (r) => {
    plan = {
      ...plan,
      state: "applied",
      result: { ...plan.result, state: "applied" },
    };
    return r.fulfill({ json: plan });
  });
  let confirmation = "";
  await page.route("**/api/plans/journal-plan/rollback", (r) => {
    confirmation = r.request().postDataJSON().confirmation;
    return r.fulfill({
      json: { id: "rollback-job", state: "queued", kind: "rollback" },
    });
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Hostzustand abgleichen", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Dateien übernommen");
  await page
    .getByRole("button", { name: "Dateien zurücksetzen", exact: true })
    .click();
  const field = page.getByLabel("Plan-ID zur Rücksetzung eingeben", {
    exact: true,
  });
  await expect(field).toHaveValue("");
  await field.fill("wrong-plan");
  await expect(
    page.getByRole("button", { name: "Rücksetzung bestätigen", exact: true }),
  ).toBeDisabled();
  await field.fill("journal-plan");
  await page
    .getByRole("button", { name: "Rücksetzung bestätigen", exact: true })
    .click();
  await expect(
    page.getByRole("status").filter({ hasText: "Auftrag läuft" }),
  ).toBeVisible();
  expect(confirmation).toBe("journal-plan");
  await expect(
    page.getByRole("button", { name: "Dateien zurücksetzen", exact: true }),
  ).toBeDisabled();
});

test("lost apply response disables a second apply until the host journal is reconciled", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const plan = {
    id: "lost-response-plan",
    schema_version: 2,
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    scenario: "files",
    created_at: "2026-10-07T10:00:00Z",
    state: "ready",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    steps: [{ path: "etc/sysctl.conf", action: "apply", reason: "" }],
    blockers: [],
    manual: [],
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, demo: false, plans: [plan] } }),
  );
  await page.route("**/api/plans/lost-response-plan", (r) =>
    r.fulfill({ json: plan }),
  );
  await page.route("**/api/plans/lost-response-plan/apply", (r) =>
    r.abort("connectionfailed"),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("lost-response-plan");
  await page
    .getByRole("button", { name: "Dateien übernehmen", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("nicht erreichbar");
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Hostzustand abgleichen", exact: true }),
  ).toBeVisible();
});

test("many physical target candidates are searchable and preserve an off-page choice", async ({
  page,
}) => {
  let base: any;
  await page.route("**/api/plans/inspect", (r) =>
    r.fulfill({
      json: {
        ...inspected(base),
        ports: [{ name: "eno1", reason: "vmbr0", suggested: "" }],
        target_ports: Array.from({ length: 80 }, (_, i) => ({
          name: `port-${String(i).padStart(2, "0")}`,
          mac: `02:11:22:33:44:${String(i).padStart(2, "0")}`,
        })),
      },
    }),
  );
  base = await setup(page);
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  const port = page.getByLabel("eno1 → Zielport", { exact: true });
  await expect(port.getByRole("option")).toHaveCount(51);
  await page
    .getByLabel("Zielports durchsuchen", { exact: true })
    .fill("port-79");
  await port.selectOption("port-79");
  await page
    .getByLabel("Zielports durchsuchen", { exact: true })
    .fill("port-00");
  await expect(port).toHaveValue("port-79");
  await expect(port.getByRole("option", { name: /^port-79/ })).toHaveCount(1);
});

test("a late source file response cannot replace the newly selected backup", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  let release!: () => void;
  const hold = new Promise<void>((resolve) => (release = resolve));
  const first = base.backups[0].id;
  const second = base.backups[1].id;
  let requested = false;
  await page.route("**/api/backups/*/files", async (r) => {
    if (r.request().url().includes(first)) {
      requested = true;
      await hold;
      return r.fulfill({
        json: [{ path: "etc/late-source.conf", type: "file" }],
      });
    }
    return r.fulfill({
      json: [{ path: "etc/current-source.conf", type: "file" }],
    });
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  await page.getByLabel("Sicherung", { exact: true }).selectOption(first);
  await expect.poll(() => requested).toBe(true);
  await page.getByLabel("Sicherung", { exact: true }).selectOption(second);
  await page.getByRole("button", { name: "Ordner etc öffnen" }).click();
  await expect(
    page.getByLabel("etc/current-source.conf", { exact: true }),
  ).toBeVisible();
  const oldResponse = page.waitForResponse(
    (r) => r.url().includes(first) && r.url().endsWith("/files"),
  );
  release();
  await (await oldResponse).finished();
  await page.getByLabel("etc/current-source.conf", { exact: true }).check();
  await expect(
    page.getByLabel("etc/current-source.conf", { exact: true }),
  ).toBeChecked();
  await expect(
    page.getByLabel("etc/late-source.conf", { exact: true }),
  ).toHaveCount(0);
  await expect(page.getByLabel("Sicherung", { exact: true })).toHaveValue(
    second,
  );
});

test("a rejected busy-host apply can be retried after the backup finishes", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const plan = {
    id: "busy-plan",
    schema_version: 2,
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    scenario: "files",
    created_at: "2026-10-07T10:00:00Z",
    state: "ready",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    steps: [{ path: "etc/sysctl.conf", action: "apply", reason: "" }],
    blockers: [],
    manual: [],
  };
  let hostBusy = false;
  let attempts = 0;
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        demo: false,
        plans: [plan],
        jobs: hostBusy
          ? [
              {
                id: "backup-job",
                host_id: plan.target_id,
                kind: "backup",
                state: "running",
                created_at: plan.created_at,
                attempts: 1,
              },
            ]
          : [],
      },
    }),
  );
  await page.route("**/api/plans/busy-plan", (r) => r.fulfill({ json: plan }));
  await page.route("**/api/plans/busy-plan/apply", (r) => {
    attempts++;
    if (attempts === 1) {
      hostBusy = true;
      return r.fulfill({
        status: 409,
        json: { error: "Auf diesem Host läuft bereits ein Auftrag" },
      });
    }
    return r.fulfill({
      json: {
        id: "apply-job",
        state: "queued",
        kind: "restore",
        host_id: plan.target_id,
      },
    });
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill(plan.id);
  const apply = page.getByRole("button", {
    name: "Dateien übernehmen",
    exact: true,
  });
  await apply.click();
  await expect(page.getByRole("alert")).toContainText(
    "läuft bereits ein Auftrag",
  );
  await expect(
    page.getByRole("button", { name: "Hostzustand abgleichen", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByText(
      "Am Ziel läuft bereits ein Auftrag. Nach dessen Abschluss sind Übernahme und Rücksetzung wieder verfügbar.",
      { exact: true },
    ),
  ).toBeVisible({ timeout: 10000 });
  await expect(apply).toBeDisabled();
  hostBusy = false;
  await expect(apply).toBeEnabled({ timeout: 10000 });
  await apply.click();
  await expect(
    page.getByRole("status").filter({ hasText: "Auftrag läuft" }),
  ).toBeVisible();
  expect(attempts).toBe(2);
});

test("large manual instructions and host evidence are paged and searchable", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const plan = {
    id: "large-evidence-plan",
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    scenario: "migration",
    created_at: "2026-10-07T10:00:00Z",
    state: "applied",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    steps: [],
    blockers: [],
    manual: Array.from(
      { length: 1600 },
      (_, i) => `Speicherprüfung ${String(i).padStart(4, "0")}`,
    ),
    result: {
      applied: [],
      rollback_path: "",
      checks: Array.from(
        { length: 1600 },
        (_, i) => `Hostnachweis ${String(i).padStart(4, "0")}`,
      ),
      reboot_verified: false,
    },
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, plans: [plan] } }),
  );
  await page.route("**/api/plans/large-evidence-plan", (r) =>
    r.fulfill({ json: plan }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await page.getByText("Manuelle Schritte · 1600", { exact: true }).click();
  const manual = page.getByRole("region", {
    name: "Manuelle Schritte",
    exact: true,
  });
  await expect(manual.getByRole("listitem")).toHaveCount(25);
  await manual
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  await expect(manual.getByRole("listitem").first()).toHaveText(
    "Speicherprüfung 0025",
  );
  await page
    .getByLabel("Manuelle Schritte durchsuchen", { exact: true })
    .fill("1599");
  await expect(manual.getByRole("listitem")).toHaveCount(1);
  await expect(manual).toContainText("Speicherprüfung 1599");
  await page.getByText("Hostnachweise · 1600", { exact: true }).click();
  const checks = page.getByRole("region", {
    name: "Hostnachweise",
    exact: true,
  });
  await expect(checks.getByRole("listitem")).toHaveCount(25);
  await page
    .getByLabel("Hostnachweise durchsuchen", { exact: true })
    .fill("1599");
  await expect(checks.getByRole("listitem")).toHaveCount(1);
});

test("whole recovery records desired identity as a manual decision without another inventory query", async ({
  page,
}) => {
  let base: any;
  let inspections = 0;
  let payload: any;
  await page.route("**/api/plans/inspect", (r) => {
    inspections++;
    return r.fulfill({
      json: {
        ...inspected(base),
        requires_console: false,
        requires_source_offline: false,
      },
    });
  });
  await page.route("**/api/plans", (r) => {
    if (r.request().method() !== "POST") return r.continue();
    payload = r.request().postDataJSON();
    return r.fulfill({ status: 503, json: { error: "Planprüfung pausiert" } });
  });
  base = await setup(page);
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page.getByLabel("Situation", { exact: true }).selectOption("host");
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await page
    .getByLabel("Gewünschter Hostname", { exact: true })
    .fill("pve-ersatz");
  await page
    .getByLabel("Gewünschte Adresse (manuell)", { exact: true })
    .fill("10.50.0.77");
  await page
    .getByRole("button", {
      name: "Offene Dateien manuell bearbeiten",
      exact: true,
    })
    .click();
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Planprüfung pausiert");
  expect(payload.mapping.hostname).toBe("pve-ersatz");
  expect(payload.mapping.address).toBe("10.50.0.77");
  expect(inspections).toBe(1);
  for (let i = 0; i < 3; i++)
    await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page.getByLabel("Situation", { exact: true }).selectOption("files");
  await expect(
    page.getByLabel("Gewünschter Hostname", { exact: true }),
  ).toHaveCount(0);
});

async function savedPlan(page: Page, extra: Record<string, unknown> = {}) {
  const base = await (await page.request.get("/api/status")).json();
  const plan = {
    id: "review-plan",
    schema_version: 2,
    target_snapshot_id: "captured-target",
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    scenario: "files",
    created_at: "2026-10-07T10:00:00Z",
    state: "ready",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    blockers: [],
    manual: [],
    steps: [
      {
        path: "etc/sysctl.conf",
        action: "apply",
        decision: "prepared",
        destination: "etc/sysctl.conf",
        reason: "",
        effects: [
          {
            path: "etc/sysctl.conf",
            field: "hostname",
            before: "old",
            after: "new",
            reason: "Identität anpassen",
          },
        ],
      },
      {
        path: "etc/hosts",
        action: "keep",
        decision: "keep",
        reason: "Zielentscheidung",
      },
    ],
    preserved: [
      { path: "etc/fstab", reason: "Root und Boot am Ziel erhalten" },
    ],
    ...extra,
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, demo: false, jobs: [], plans: [plan] } }),
  );
  await page.route("**/api/plans/review-plan", (r) =>
    r.fulfill({ json: plan }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  return plan;
}

test("plan compares actual target against prepared content", async ({
  page,
}) => {
  const bases: string[] = [];
  await page.route("**/api/plans/review-plan/comparison?**", (r) => {
    const basis = new URL(r.request().url()).searchParams.get("basis")!;
    bases.push(basis);
    return r.fulfill({
      json: {
        path: "etc/sysctl.conf",
        before_label:
          basis === "target" ? "Aufgenommenes Ziel" : "Originalsicherung",
        after_label:
          basis === "source-target"
            ? "Aufgenommenes Ziel"
            : "Vorbereitete Datei",
        state: "different",
        content_changed: true,
        metadata_changes: [],
        masked: false,
        hunks: [
          {
            before_start: 1,
            after_start: 1,
            lines: [
              { kind: "remove", text: "target-only-value\n", before_line: 1 },
              { kind: "add", text: "prepared-value\n", after_line: 1 },
            ],
          },
        ],
      },
    });
  });
  await savedPlan(page);
  expect(bases).toEqual([]);
  await page
    .getByRole("button", {
      name: "Vergleich öffnen: etc/sysctl.conf",
      exact: true,
    })
    .click();
  await page.locator(".comparison-source-reference > summary").click();
  await expect(
    page.getByText("Aufgenommenes Ziel → Vorbereitete Datei", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("region", { name: "Zeilenvergleich" }),
  ).toContainText("target-only-value");
  await page
    .getByLabel("Vergleichsbasis", { exact: true })
    .selectOption("source");
  await expect.poll(() => bases).toEqual(["target", "source"]);
  await page
    .getByLabel("Vergleichsbasis", { exact: true })
    .selectOption("source-target");
  await expect.poll(() => bases).toEqual(["target", "source", "source-target"]);
});

test("keep files and preserved system parts are explicit", async ({ page }) => {
  await savedPlan(page);
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Ziel behalten · kein Schreiben");
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Erhaltene Systemteile");
  await page.getByText("Erhaltene Systemteile · 1", { exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Erhaltene Systemteile" }),
  ).toContainText("etc/fstab · Root und Boot am Ziel erhalten");
  await page
    .getByText("Anpassungen an etc/sysctl.conf · 1", { exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("hostname: old → new");
});

test("checks never imply verified reboot", async ({ page }) => {
  let confirmed = false;
  const plan = await savedPlan(page, {
    state: "applied",
    tasks: [
      {
        id: "check-services",
        kind: "manual-check",
        title: "Dienste prüfen",
        instruction: "Dienste am Ziel prüfen",
        requires: [],
        state: "pending",
      },
      {
        id: "check-reboot",
        kind: "manual-check",
        title: "Neustart prüfen",
        instruction: "Neustart am Ziel prüfen",
        requires: ["check-services"],
        state: "pending",
      },
    ],
    result: {
      applied: ["etc/sysctl.conf"],
      checks: ["Datei-Hash am Host geprüft"],
      rollback_path: "",
      reboot_verified: false,
    },
  });
  await page.route("**/api/plans/review-plan/checks/check-services", (r) => {
    confirmed = true;
    expect(r.request().postDataJSON()).toEqual({
      evidence: "Dienste an Konsole geprüft",
    });
    return r.fulfill({
      json: {
        ...plan,
        tasks: (plan.tasks as any[]).map((t) =>
          t.id === "check-services"
            ? {
                ...t,
                state: "user_confirmed",
                evidence: "Dienste an Konsole geprüft",
                confirmed_by: "authenticated-user",
                confirmed_at: "2026-10-08T10:00:00Z",
              }
            : t,
        ),
      },
    });
  });
  await expect(
    page.getByRole("button", {
      name: "Schritt bestätigen: Neustart prüfen",
      exact: true,
    }),
  ).toBeDisabled();
  await page
    .getByLabel("Nachweis für Dienste prüfen", { exact: true })
    .fill("Dienste an Konsole geprüft");
  await page
    .getByRole("button", {
      name: "Schritt bestätigen: Dienste prüfen",
      exact: true,
    })
    .click();
  expect(confirmed).toBe(true);
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Vom Benutzer bestätigt · authenticated-user");
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Neustart maschinell nicht nachgewiesen");
});

test("legacy plan requires new preparation", async ({ page }) => {
  await savedPlan(page, { schema_version: 1, target_snapshot_id: undefined });
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText(
    "Für eine neue Freigabe ist eine neue Vorbereitung erforderlich",
  );
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Neu vorbereiten", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "1 · Quelle und Situation",
      exact: true,
    }),
  ).toBeVisible();
});

test("lost apply reply remains uncertain after reopen", async ({ page }) => {
  await page.route("**/api/plans/review-plan/apply", (r) =>
    r.abort("connectionfailed"),
  );
  await savedPlan(page);
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("review-plan");
  await page
    .getByRole("button", { name: "Dateien übernehmen", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("nicht erreichbar");
  await closePlan(page);
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Hostzustand abgleichen", exact: true }),
  ).toBeVisible();
});

test("lost apply reply survives main-navigation input discard and plan reopen", async ({
  page,
}) => {
  let attempts = 0;
  await page.route("**/api/plans/review-plan/apply", (r) => {
    attempts++;
    return r.abort("connectionfailed");
  });
  await savedPlan(page);
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("review-plan");
  await page
    .getByRole("button", { name: "Dateien übernehmen", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("nicht erreichbar");
  const navigation = page.getByRole("navigation", { name: "Hauptnavigation" });
  await navigation.getByRole("button", { name: "Hosts", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "Änderungen noch nicht gespeichert" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Änderungen verwerfen" }).click();
  await expect(page).toHaveTitle("Hosts · Anker");
  await navigation
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Hostzustand abgleichen", exact: true }),
  ).toBeVisible();
  expect(attempts).toBe(1);
});

test("an open discard prompt cannot close a plan during a pending mutation", async ({
  page,
}) => {
  let release!: () => void;
  let requestStarted!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  const started = new Promise<void>((resolve) => (requestStarted = resolve));
  await page.route("**/api/plans/review-plan/apply", async (r) => {
    requestStarted();
    await gate;
    return r.fulfill({ status: 503, json: { error: "Antwort ungeklärt" } });
  });
  await savedPlan(page);
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("review-plan");
  await page
    .getByRole("button", { name: "Zurück zu Plänen", exact: true })
    .click();
  const discard = page.getByRole("button", {
    name: "Eingaben verwerfen",
    exact: true,
  });
  await expect(discard).toBeVisible();
  await page
    .getByRole("button", { name: "Dateien übernehmen", exact: true })
    .click();
  await started;
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toHaveAttribute("aria-busy", "true");
  await expect(discard).toBeDisabled();
  await discard.dispatchEvent("click");
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toBeVisible();
  release();
  await expect(
    page.getByRole("alert").filter({ hasText: "Antwort ungeklärt" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Hostzustand abgleichen", exact: true }),
  ).toBeVisible();
});

test("journal uncertainty overrides a ready server plan after reopen", async ({
  page,
}) => {
  await savedPlan(page, {
    result: {
      state: "writing",
      operation_id: "review-plan",
      applied: [],
      checks: [],
      rollback_path: "",
      reboot_verified: false,
    },
  });
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Hostzustand ungeklärt");
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("review-plan");
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Hostzustand abgleichen", exact: true }),
  ).toBeVisible();
});

test("failed apply response remains uncertain while a rejected 4xx permits retry", async ({
  page,
}) => {
  let attempts = 0;
  await page.route("**/api/plans/review-plan/apply", (r) => {
    attempts++;
    return r.fulfill({
      status: attempts === 1 ? 403 : 503,
      json: { error: attempts === 1 ? "Freigabe fehlt" : "Antwort verloren" },
    });
  });
  await savedPlan(page);
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("wrong-plan");
  const apply = page.getByRole("button", {
    name: "Dateien übernehmen",
    exact: true,
  });
  await expect(apply).toBeDisabled();
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("review-plan");
  await apply.click();
  await expect(page.getByRole("alert")).toContainText("Freigabe fehlt");
  await expect(apply).toBeEnabled();
  await apply.click();
  await expect(page.getByRole("alert")).toContainText("Antwort verloren");
  await expect(apply).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Hostzustand abgleichen", exact: true }),
  ).toBeVisible();
});

test("comparison secrets require explicit reveal and stale replies cannot replace another path", async ({
  page,
}) => {
  let release!: () => void;
  const gate = new Promise<void>((r) => (release = r));
  const requests: string[] = [];
  await page.route("**/api/plans/review-plan/comparison?**", async (r) => {
    const url = new URL(r.request().url());
    const path = url.searchParams.get("path")!;
    requests.push(url.search);
    if (
      path === "etc/sysctl.conf" &&
      url.searchParams.get("basis") === "source"
    )
      await gate;
    const reveal = url.searchParams.has("reveal");
    return r.fulfill({
      json: {
        path,
        before_label: "Aufgenommenes Ziel",
        after_label: "Vorbereitete Datei",
        state: reveal ? "different" : "masked",
        content_changed: reveal,
        metadata_changes: [],
        hunks: reveal
          ? [
              {
                before_start: 1,
                after_start: 1,
                lines: [
                  { kind: "add", text: "explicit-secret\n", after_line: 1 },
                ],
              },
            ]
          : [],
        masked: !reveal,
      },
    });
  });
  await savedPlan(page);
  await page
    .getByRole("button", {
      name: "Vergleich öffnen: etc/sysctl.conf",
      exact: true,
    })
    .click();
  await expect(
    page.getByText("Geschützter Inhalt", { exact: true }),
  ).toBeVisible();
  expect(requests[0]).not.toContain("reveal");
  await page
    .getByRole("button", { name: "Geschützten Inhalt anzeigen", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Zeilenvergleich" }),
  ).toContainText("explicit-secret");
  await page
    .getByLabel("Vergleichsbasis", { exact: true })
    .selectOption("source");
  await expect
    .poll(() => requests.some((s) => s.includes("basis=source")))
    .toBe(true);
  await page
    .getByRole("button", { name: "Vergleich öffnen: etc/hosts", exact: true })
    .click();
  await expect(page.locator(".preview-heading code")).toHaveText("etc/hosts");
  release();
  await expect(page.locator(".preview-heading code")).toHaveText("etc/hosts");
  await expect(page.getByText("explicit-secret", { exact: true })).toHaveCount(
    0,
  );
});

test("restore role sees masked comparisons without reveal or export controls", async ({
  page,
}) => {
  const me = await (await page.request.get("/api/me")).json();
  await page.route("**/api/me", (r) =>
    r.fulfill({
      json: { ...me, user: { ...me.user, role: "restore", secrets: false } },
    }),
  );
  await page.route("**/api/plans/review-plan/comparison?**", (r) =>
    r.fulfill({
      json: {
        path: "etc/sysctl.conf",
        before_label: "Aufgenommenes Ziel",
        after_label: "Vorbereitete Datei",
        state: "masked",
        content_changed: false,
        metadata_changes: [],
        hunks: [],
        masked: true,
      },
    }),
  );
  await savedPlan(page);
  await page
    .getByRole("button", {
      name: "Vergleich öffnen: etc/sysctl.conf",
      exact: true,
    })
    .click();
  await expect(
    page.getByText("Geschützter Inhalt", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", {
      name: "Geschützten Inhalt anzeigen",
      exact: true,
    }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Plan herunterladen", exact: true }),
  ).toHaveCount(0);
});

test("a completed accepted job refreshes server evidence across reopen without repeating apply", async ({
  page,
}) => {
  let current: any;
  let applyCount = 0;
  let phase = "ready";
  await page.route("**/api/plans/review-plan/apply", (r) => {
    applyCount++;
    phase = "running";
    return r.fulfill({
      json: {
        id: "accepted-job",
        kind: "restore",
        host_id: current.target_id,
        state: "queued",
      },
    });
  });
  current = await savedPlan(page);
  const base = await (await page.request.get("/api/status")).json();
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        demo: false,
        plans: [
          phase === "done"
            ? {
                ...current,
                state: "checks_pending",
                result: {
                  state: "applied",
                  applied: ["etc/sysctl.conf"],
                  checks: ["Server-Hostnachweis"],
                  rollback_path: "",
                  reboot_verified: false,
                },
              }
            : current,
        ],
        jobs:
          phase === "ready"
            ? []
            : [
                {
                  id: "accepted-job",
                  kind: "restore",
                  host_id: current.target_id,
                  state: phase === "done" ? "successful" : "running",
                },
              ],
      },
    }),
  );
  await page.route("**/api/plans/review-plan", (r) =>
    r.fulfill({
      json:
        phase === "done"
          ? {
              ...current,
              state: "checks_pending",
              result: {
                state: "applied",
                applied: ["etc/sysctl.conf"],
                checks: ["Server-Hostnachweis"],
                rollback_path: "",
                reboot_verified: false,
              },
            }
          : current,
    }),
  );
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("review-plan");
  await page
    .getByRole("button", { name: "Dateien übernehmen", exact: true })
    .click();
  await expect(
    page.getByRole("status").filter({ hasText: "Auftrag läuft" }),
  ).toBeVisible();
  await closePlan(page);
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toBeDisabled();
  phase = "done";
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Prüfung ausstehend", {
    timeout: 10000,
  });
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("status").filter({ hasText: "Auftrag läuft" }),
  ).toHaveCount(0);
  expect(applyCount).toBe(1);
});

test("check API failures retain an open task and its evidence without claiming confirmation", async ({
  page,
}) => {
  let attempts = 0;
  await page.route("**/api/plans/review-plan/checks/check-services", (r) => {
    attempts++;
    return r.fulfill({
      status: attempts === 1 ? 409 : 503,
      json: {
        error:
          attempts === 1 ? "Voraussetzung offen" : "Prüfung nicht gespeichert",
      },
    });
  });
  await savedPlan(page, {
    state: "applied",
    tasks: [
      {
        id: "check-services",
        kind: "manual-check",
        title: "Dienste prüfen",
        instruction: "Dienste am Ziel prüfen",
        requires: [],
        state: "pending",
      },
    ],
  });
  const evidence = page.getByLabel("Nachweis für Dienste prüfen", {
    exact: true,
  });
  await evidence.fill("Lokale Dienste geprüft");
  const confirm = page.getByRole("button", {
    name: "Schritt bestätigen: Dienste prüfen",
    exact: true,
  });
  await confirm.click();
  await expect(page.getByRole("alert")).toContainText("Voraussetzung offen");
  await expect(evidence).toHaveValue("Lokale Dienste geprüft");
  await confirm.click();
  await expect(page.getByRole("alert")).toContainText(
    "Prüfung nicht gespeichert",
  );
  await expect(page.getByText(/Vom Benutzer bestätigt ·/)).toHaveCount(0);
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Neustart maschinell nicht nachgewiesen");
});

test("an active plan mutation protects navigation logout and dialog close", async ({
  page,
}) => {
  let release!: () => void;
  const gate = new Promise<void>((r) => (release = r));
  await page.route("**/api/plans/review-plan/apply", async (r) => {
    await gate;
    return r.fulfill({ status: 503, json: { error: "Antwort ungeklärt" } });
  });
  await savedPlan(page);
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("review-plan");
  await page
    .getByRole("button", { name: "Dateien übernehmen", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toHaveAttribute("aria-busy", "true");
  const hosts = page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Hosts", exact: true });
  await expect(
    page.getByRole("button", { name: "Abmelden", exact: true }),
  ).toBeDisabled();
  await hosts.dispatchEvent("click");
  await expect(
    page.getByRole("alert").filter({ hasText: "Plananfrage läuft" }),
  ).toBeVisible();
  await expect(page).toHaveTitle("Wiederherstellung · Anker");
  await expect(
    page.getByRole("button", { name: "Abmelden", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Zurück zu Plänen", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toBeVisible();
  release();
  await expect(
    page.getByRole("alert").filter({ hasText: "Antwort ungeklärt" }),
  ).toBeVisible();
  await expect(hosts).toBeEnabled();
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toBeDisabled();
});

test("a lost reply with ready server and refused reconcile gives console guidance without repeating an unproven apply", async ({
  page,
}) => {
  let reconciles = 0;
  await page.route("**/api/plans/review-plan/apply", (r) =>
    r.fulfill({ status: 503, json: { error: "Antwort verloren" } }),
  );
  await page.route("**/api/plans/review-plan/reconcile", (r) => {
    reconciles++;
    return r.fulfill({
      status: 409,
      json: {
        error:
          "Für diesen Plan ist keine ungeklärte Hostausführung gespeichert",
      },
    });
  });
  await savedPlan(page);
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill("review-plan");
  await page
    .getByRole("button", { name: "Dateien übernehmen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Hostzustand abgleichen", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText(
    "Über die Zielkonsole das Hostjournal zur Recovery-ID review-plan und laufende Anker-Aufträge prüfen",
  );
  await expect(
    page.getByRole("button", { name: "Hostzustand abgleichen", exact: true }),
  ).toBeDisabled();
  const navigation = page.getByRole("navigation", { name: "Hauptnavigation" });
  await navigation.getByRole("button", { name: "Hosts", exact: true }).click();
  await page.getByRole("button", { name: "Änderungen verwerfen" }).click();
  await navigation
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Hostzustand abgleichen", exact: true }),
  ).toBeDisabled();
  expect(reconciles).toBe(1);
});

test("failed authoritative detail refresh keeps apply blocked and retries identical status without a tracked job", async ({
  page,
}) => {
  const plan = await savedPlan(page);
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill(plan.id);
  const base = await (await page.request.get("/api/status")).json();
  const current = {
    ...plan,
    state: "checks_pending",
    result: {
      state: "applied",
      applied: ["etc/sysctl.conf"],
      checks: [],
      rollback_path: "",
      reboot_verified: false,
    },
  };
  let detailReads = 0;
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, demo: false, jobs: [], plans: [current] } }),
  );
  await page.route("**/api/plans/review-plan", (r) => {
    detailReads++;
    return detailReads === 1
      ? r.fulfill({
          status: 503,
          json: { error: "Aktueller Plan vorübergehend nicht lesbar" },
        })
      : r.fulfill({ json: current });
  });
  await expect(page.getByRole("alert")).toContainText(
    "Aktueller Plan vorübergehend nicht lesbar",
    { timeout: 10000 },
  );
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toBeDisabled();
  await expect.poll(() => detailReads, { timeout: 10000 }).toBe(2);
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Prüfung ausstehend");
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toHaveCount(0);
});

test("a current authoritative detail reply delayed across equivalent jobs polls is retained without a tracked job", async ({
  page,
}) => {
  const plan = await savedPlan(page);
  await page
    .getByLabel("Plan-ID zur Ausführung eingeben", { exact: true })
    .fill(plan.id);
  const base = await (await page.request.get("/api/status")).json();
  const current = {
    ...plan,
    state: "checks_pending",
    result: {
      state: "applied",
      applied: ["etc/sysctl.conf"],
      checks: [],
      rollback_path: "",
      reboot_verified: false,
    },
  };
  let polls = 0;
  let detailReads = 0;
  let release!: () => void;
  const gate = new Promise<void>((r) => (release = r));
  await page.route("**/api/status", (r) => {
    polls++;
    return r.fulfill({
      json: { ...base, demo: false, jobs: [], plans: [current] },
    });
  });
  await page.route("**/api/plans/review-plan", async (r) => {
    detailReads++;
    await gate;
    return r.fulfill({ json: current });
  });
  await expect.poll(() => detailReads, { timeout: 10000 }).toBe(1);
  await expect.poll(() => polls, { timeout: 10000 }).toBeGreaterThanOrEqual(2);
  release();
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Prüfung ausstehend");
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toHaveCount(0);
  expect(detailReads).toBe(1);
});

test("reopening the selected comparison preserves its loaded evidence", async ({
  page,
}) => {
  let reads = 0;
  await page.route("**/api/plans/review-plan/comparison?**", (r) => {
    reads++;
    return r.fulfill({
      json: {
        path: "etc/sysctl.conf",
        before_label: "Aufgenommenes Ziel",
        after_label: "Vorbereitete Datei",
        state: "identical",
        content_changed: false,
        metadata_changes: [],
        hunks: [],
        masked: false,
      },
    });
  });
  await savedPlan(page);
  const open = page.getByRole("button", {
    name: "Vergleich öffnen: etc/sysctl.conf",
    exact: true,
  });
  await open.click();
  await expect(
    page.getByText("Inhalt und Metadaten stimmen überein", { exact: true }),
  ).toBeVisible();
  await open.click();
  await expect(
    page.getByText("Inhalt und Metadaten stimmen überein", { exact: true }),
  ).toBeVisible();
  expect(reads).toBe(1);
});

test("authenticated interrupted journal allows separately confirmed rollback after reopen", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  let plan = {
    id: "journal-plan",
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    scenario: "files",
    created_at: "2026-10-07T10:00:00Z",
    state: "interrupted",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    steps: [{ path: "etc/sysctl.conf", action: "apply", reason: "" }],
    blockers: [],
    manual: [],
    mapping: { interfaces: {}, storage: {} },
    result: {
      state: "interrupted",
      operation_id: "journal-plan",
      applied: ["etc/sysctl.conf"],
      rollback_path: "/var/lib/anker/rollback/journal-plan",
      checks: [],
      reboot_verified: false,
    },
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, demo: false, plans: [plan] } }),
  );
  await page.route("**/api/plans/journal-plan", (r) =>
    r.fulfill({ json: plan }),
  );
  await page.route("**/api/plans/journal-plan/reconcile", (r) => {
    plan = {
      ...plan,
      state: "interrupted",
      result: { ...plan.result, state: "interrupted" },
    };
    return r.fulfill({ json: plan });
  });
  let confirmation = "";
  await page.route("**/api/plans/journal-plan/rollback", (r) => {
    confirmation = r.request().postDataJSON().confirmation;
    return r.fulfill({
      json: { id: "rollback-job", state: "queued", kind: "rollback" },
    });
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Hostzustand abgleichen", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Dateien zurücksetzen", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toContainText("Hostjournal abgeglichen: Ausführung unterbrochen");
  for (const width of [1440, 1280, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await page
      .getByRole("button", { name: "Dateien zurücksetzen", exact: true })
      .scrollIntoViewIfNeeded();
    await page.screenshot({
      path: `/tmp/anker-final-fix-evidence/interrupted-${width}.png`,
    });
  }
  await closePlan(page);
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Dateien zurücksetzen", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Dateien zurücksetzen", exact: true })
    .click();
  const field = page.getByLabel("Plan-ID zur Rücksetzung eingeben", {
    exact: true,
  });
  await expect(field).toHaveValue("");
  await field.fill("wrong-plan");
  await expect(
    page.getByRole("button", { name: "Rücksetzung bestätigen", exact: true }),
  ).toBeDisabled();
  await field.fill("journal-plan");
  await page
    .getByRole("button", { name: "Rücksetzung bestätigen", exact: true })
    .click();
  await expect(
    page.getByRole("status").filter({ hasText: "Auftrag läuft" }),
  ).toBeVisible();
  expect(confirmation).toBe("journal-plan");
  await expect(
    page.getByRole("button", { name: "Dateien zurücksetzen", exact: true }),
  ).toBeDisabled();
});

for (const failure of [
  "not_found",
  "writing",
  "rolling_back",
  "read-error",
  "invalid",
  "active-job",
])
  test(`interrupted rollback stays blocked after ${failure} reconciliation`, async ({
    page,
  }) => {
    const base = await (await page.request.get("/api/status")).json();
    const id = "uncertain-final";
    let plan = {
      id,
      schema_version: 2,
      backup_id: base.backups[0].id,
      target_id: base.hosts[0].id,
      scenario: "files",
      created_at: "2026-10-09T10:00:00Z",
      state: "interrupted",
      source: base.hosts[0].inventory,
      target: base.hosts[0].inventory,
      steps: [],
      blockers: [],
      manual: [],
      mapping: { interfaces: {}, storage: {} },
      result: {
        operation_id: id,
        state: "interrupted",
        applied: [],
        rollback_path: "/var/lib/anker-host/rollback/" + id,
      },
    };
    let fail = false;
    await page.route("**/api/status", (r) =>
      r.fulfill({
        json: {
          ...base,
          demo: false,
          plans: [plan],
          jobs:
            failure === "active-job"
              ? [
                  {
                    id: "active-final",
                    host_id: plan.target_id,
                    state: "running",
                    kind: "backup",
                  },
                ]
              : [],
        },
      }),
    );
    await page.route(`**/api/plans/${id}`, (r) => r.fulfill({ json: plan }));
    await page.route(`**/api/plans/${id}/reconcile`, (r) => {
      if (fail && (failure === "read-error" || failure === "invalid"))
        return r.fulfill({
          status: 500,
          json: {
            error:
              failure === "invalid"
                ? "Hostprotokoll gehört nicht zu diesem Wiederherstellungsplan"
                : "Hostprotokoll nicht lesbar",
          },
        });
      if (fail && failure !== "active-job")
        plan = { ...plan, result: { ...plan.result, state: failure } };
      return r.fulfill({ json: plan });
    });
    await page.goto("/");
    await page
      .getByRole("navigation", { name: "Hauptnavigation" })
      .getByRole("button", { name: "Wiederherstellung", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Plan ansehen", exact: true })
      .click();
    const rollback = page.getByRole("button", {
      name: "Dateien zurücksetzen",
      exact: true,
    });
    await expect(rollback).toBeDisabled();
    await page
      .getByRole("button", { name: "Hostzustand abgleichen", exact: true })
      .click();
    if (failure !== "active-job") await expect(rollback).toBeEnabled();
    fail = true;
    await page
      .getByRole("button", { name: "Hostzustand abgleichen", exact: true })
      .click();
    await expect(rollback).toBeDisabled();
    await closePlan(page);
    await page
      .getByRole("button", { name: "Plan ansehen", exact: true })
      .click();
    await expect(rollback).toBeDisabled();
    await expect(
      page.getByRole("button", { name: "Dateien übernehmen", exact: true }),
    ).toHaveCount(0);
  });
