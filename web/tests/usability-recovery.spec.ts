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
async function start(page: Page) {
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
}

test("console and source isolation approvals belong to the selected recovery target", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  await start(page);
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[0].id);
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[0].id);
  await page.getByLabel("Szenario", { exact: true }).selectOption("migration");
  await page.getByLabel("Konsolenzugang zum Ziel ist verfügbar").check();
  await page.getByLabel("Alter Host ist ausgeschaltet oder isoliert").check();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[1].id);
  await expect(
    page.getByLabel("Konsolenzugang zum Ziel ist verfügbar"),
  ).not.toBeChecked();
  await expect(
    page.getByLabel("Alter Host ist ausgeschaltet oder isoliert"),
  ).not.toBeChecked();
});

test("recovery selection pages and searches 1600 files without dropping prior selections", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const entries = Array.from({ length: 1600 }, (_, i) => ({
    path: `etc/sysctl.d/conf-${String(i).padStart(4, "0")}.conf`,
    type: "file",
    mode: 420,
    uid: 0,
    gid: 0,
    size: 16,
    secret: false,
  }));
  await page.route("**/api/backups/*/files", (r) =>
    r.fulfill({ json: entries }),
  );
  let payload: any;
  await page.route("**/api/plans", (r) => {
    if (r.request().method() !== "POST") return r.continue();
    payload = r.request().postDataJSON();
    return r.fulfill({
      status: 503,
      json: { error: "Testziel nicht erreichbar" },
    });
  });
  await start(page);
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[0].id);
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[0].id);
  const files = page.getByRole("group", { name: /Dateien auswählen/ });
  await expect(files.getByRole("checkbox")).toHaveCount(25);
  await files
    .getByLabel("etc/sysctl.d/conf-0000.conf", { exact: true })
    .check();
  await files
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  await expect(
    files.getByLabel("etc/sysctl.d/conf-0000.conf", { exact: true }),
  ).toHaveCount(0);
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("conf-1599");
  await files
    .getByLabel("etc/sysctl.d/conf-1599.conf", { exact: true })
    .check();
  await page.getByLabel("Wiederherstellungsdateien durchsuchen").fill("");
  await page
    .getByRole("button", { name: "Nur ausgewählte Dateien", exact: true })
    .click();
  await expect(files.getByRole("checkbox")).toHaveCount(2);
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Testziel nicht erreichbar",
  );
  expect(payload.files).toEqual([
    "etc/sysctl.d/conf-0000.conf",
    "etc/sysctl.d/conf-1599.conf",
  ]);
});

test("blocked recovery can use freshly inspected target ports without losing its source or files", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const target = { ...base.hosts[0], inventory: undefined };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [target] } }),
  );
  let last: any;
  const requests: any[] = [];
  await page.route("**/api/plans", (r) => {
    if (r.request().method() !== "POST") return r.continue();
    last = r.request().postDataJSON();
    requests.push(last);
    return r.fulfill({
      json: {
        id: "fresh-target-plan",
        created_at: "2026-10-07T10:00:00Z",
        ...last,
        state: "blocked",
        source: base.hosts[0].inventory,
        target: {
          ...base.hosts[0].inventory,
          interfaces: [{ name: "enp7s0", mac: "02:11:22:33:44:55" }],
        },
        steps: [
          {
            path: "etc/network/interfaces",
            action: "apply",
            diff: "eno1 → enp7s0",
          },
        ],
        blockers: ["Zielport fehlt: eno1"],
        manual: [],
      },
    });
  });
  await start(page);
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[0].id);
  await page.getByLabel("Zielhost", { exact: true }).selectOption(target.id);
  await page.getByLabel("etc/network/interfaces", { exact: true }).check();
  await page.getByLabel("Konsolenzugang zum Ziel ist verfügbar").check();
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "Wiederherstellungsplan", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Zuordnungen anpassen", exact: true })
    .click();
  await expect(page.getByLabel("Sicherung", { exact: true })).toHaveValue(
    base.backups[0].id,
  );
  await expect(
    page.getByLabel("etc/network/interfaces", { exact: true }),
  ).toBeChecked();
  await page
    .getByLabel("eno1 → Zielport", { exact: true })
    .selectOption("enp7s0");
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await expect.poll(() => requests.length).toBe(2);
  expect(requests[1].mapping.interfaces).toEqual({ eno1: "enp7s0" });
  expect(requests[1].files).toEqual(["etc/network/interfaces"]);
});

test("recovery plans and their steps stay bounded and searchable", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const template = {
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    scenario: "migration",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    blockers: [],
    manual: ["Storage am Ziel vorbereiten"],
    mapping: { interfaces: {}, storage: {} },
  };
  const plans = Array.from({ length: 70 }, (_, i) => ({
    ...template,
    id: `plan-${i}`,
    created_at: new Date(Date.UTC(2026, 9, 7, 0, i)).toISOString(),
    state: i % 2 ? "manual" : "blocked",
    source: { ...template.source, hostname: `source-${i}` },
    steps: Array.from({ length: 1600 }, (_, n) => ({
      path: `etc/conf/${String(n).padStart(4, "0")}.conf`,
      action: "manual",
      reason: "Manuell prüfen",
    })),
  }));
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, plans } }),
  );
  await page.route("**/api/plans/plan-*", (r) =>
    r.fulfill({
      json: plans.find((p) =>
        r
          .request()
          .url()
          .endsWith("/" + p.id),
      ),
    }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(25);
  await page
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  await expect(
    page.getByRole("table").locator("tbody tr").first(),
  ).toContainText("source-44");
  await page
    .getByLabel("Wiederherstellungspläne durchsuchen")
    .fill("source-69");
  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(1);
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  await expect(page.locator(".plan-files .file-diff")).toHaveCount(25);
  await page.getByLabel("Plandateien durchsuchen").fill("1599");
  await expect(page.locator(".plan-files .file-diff")).toHaveCount(1);
  await expect(page.locator(".plan-files")).toContainText("etc/conf/1599.conf");
});

test("large host lists page and filter by actionable state without losing hosts", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const hosts = Array.from({ length: 40 }, (_, i) => ({
    ...base.hosts[0],
    id: `fleet-${i}`,
    name: `node-${String(i).padStart(2, "0")}`,
    address: `10.50.0.${i + 1}`,
    group: i < 7 ? "Cluster" : "Standalone",
    enabled: i !== 39,
  }));
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts, backups: [], jobs: [] } }),
  );
  await page.goto("/");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(25);
  await page
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "node-39", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Hoststatus filtern").selectOption("paused");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(1);
  await expect(
    page.getByRole("button", { name: "node-39", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Hoststatus filtern").selectOption("attention");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(25);
  await page.getByLabel("Hosts durchsuchen").fill("node-38");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(1);
  await expect(
    page.getByRole("button", { name: "node-38", exact: true }),
  ).toBeVisible();
});

test("a selected plan-state filter remains visible when its last matching plan changes state", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  let state = "ready";
  const plan = {
    id: "changing-plan",
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    scenario: "files",
    created_at: "2026-10-07T10:00:00Z",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    steps: [],
    blockers: [],
    manual: [],
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, plans: [{ ...plan, state }] } }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByLabel("Planstatus filtern").selectOption("ready");
  state = "applying";
  await expect(
    page.getByText("Keine passenden Pläne", { exact: true }),
  ).toBeVisible({ timeout: 10000 });
  await expect(page.getByLabel("Planstatus filtern")).toHaveValue("ready");
  await page.getByLabel("Planstatus filtern").selectOption("");
  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(1);
  await expect(page.getByRole("table")).toContainText("Wird angewendet");
});
