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
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[0].id);
  await page.getByLabel("etc/network/interfaces", { exact: true }).check();
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
  await expect(page.getByLabel("eno1 → Zielport", { exact: true })).toHaveValue(
    "enp7s0",
  );
  await expect(page.locator(".recovery-mapping select")).toHaveCount(1);
  await expect(
    page.getByLabel("Alter Host ist ausgeschaltet oder isoliert"),
  ).toHaveCount(0);
  expect(creates).toBe(0);
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
  await expect(
    page.getByLabel("etc/network/interfaces", { exact: true }),
  ).toBeChecked();
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Ziel wird geprüft …", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("dialog", { name: "Wiederherstellung vorbereiten" }),
  ).toBeVisible();
  release();
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
  await page.getByLabel("Konsolenzugang zum Ziel ist verfügbar").check();
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
    page.getByRole("button", { name: "Plan prüfen", exact: true }),
  ).toBeDisabled();
  await page
    .getByLabel("eno2 → Zielport", { exact: true })
    .selectOption("enp8s0");
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Prüfung unterbrochen");
  expect(payload.mapping.storage).toEqual({ "local-zfs": "manual" });
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
  await expect(page.getByRole("dialog")).toContainText("Dateien übernommen");
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
        ports: [],
        target_ports: [],
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
  await page.getByLabel("Szenario", { exact: true }).selectOption("migration");
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByText("Hostidentität festlegen (optional, manuell)", { exact: true })
    .click();
  await page
    .getByLabel("Gewünschter Hostname (manuell)", { exact: true })
    .fill("pve-ersatz");
  await page
    .getByLabel("Gewünschte Adresse (manuell)", { exact: true })
    .fill("10.50.0.77");
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Planprüfung pausiert");
  expect(payload.mapping.hostname).toBe("pve-ersatz");
  expect(payload.mapping.address).toBe("10.50.0.77");
  expect(inspections).toBe(1);
  await page.getByLabel("Szenario", { exact: true }).selectOption("files");
  await expect(
    page.getByLabel("Gewünschter Hostname (manuell)", { exact: true }),
  ).toHaveCount(0);
});
