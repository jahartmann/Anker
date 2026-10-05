import { test, expect } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { readFile } from "node:fs/promises";
let demoCookies: Awaited<
  ReturnType<APIRequestContext["storageState"]>
>["cookies"];
test.beforeAll(async ({ request }) => {
  const response = await request.post("/api/login", {
    headers: { "X-Anker-Request": "1" },
    data: { name: "demo", password: "anker-demo-2026" },
  });
  expect(response.ok()).toBeTruthy();
  demoCookies = (await request.storageState()).cookies;
});
test.beforeEach(async ({ page }) => {
  await page.context().addCookies(demoCookies);
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
});
test("host search and backup files work", async ({ page }) => {
  await page.getByPlaceholder("Hosts durchsuchen").fill("berlin");
  await expect(page.getByRole("row")).toHaveCount(4);
  await page.getByPlaceholder("Hosts durchsuchen").fill("");
  await page
    .getByRole("button", { name: "Öffnen", exact: true })
    .first()
    .click();
  await page
    .getByRole("main")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: /^etc\/network\/interfaces/ }).click();
  await expect(
    page.getByText("bridge-ports eno1", { exact: false }),
  ).toBeVisible();
});
test("host form validates and creates real record", async ({ page }) => {
  await page.getByRole("button", { name: "Host hinzufügen" }).click();
  await page.getByLabel("Hostname", { exact: true }).fill("pve-browser-test");
  await page.getByLabel("Adresse", { exact: true }).fill("192.0.2.90");
  await page.getByRole("button", { name: "Host speichern" }).click();
  await expect(
    page.getByText("pve-browser-test", { exact: true }),
  ).toBeVisible();
});
test("settings save and mobile navigation", async ({ page }) => {
  await page
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Einstellungen", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Tagesstände").fill("35");
  await page.getByRole("button", { name: "Einstellungen speichern" }).click();
  await expect(page.getByText("Einstellungen gespeichert")).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Navigation öffnen" }).click();
  await page.getByRole("button", { name: "Hosts", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  await expect(page.locator("body")).toHaveJSProperty("scrollWidth", 390);
});

test("secrets stay masked until explicitly revealed and export downloads", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: /^etc\/pve\/priv\/storage\/demo.pw/ })
    .click();
  await expect(
    page.getByRole("heading", { name: "Geschützter Inhalt" }),
  ).toBeVisible();
  await expect(
    page.getByText("demo-secret-not-a-production-password", { exact: false }),
  ).not.toBeVisible();
  await page.getByRole("button", { name: "Inhalt anzeigen" }).click();
  await expect(
    page.getByText("demo-secret-not-a-production-password", { exact: false }),
  ).toBeVisible();
  const downloading = page.waitForEvent("download");
  await page.getByRole("link", { name: "Stand herunterladen" }).click();
  const download = await downloading;
  expect(await download.failure()).toBeNull();
  expect(download.suggestedFilename()).toMatch(/^anker-.*\.tar$/);
  const fileDownload = page.waitForEvent("download");
  await page
    .getByRole("link", { name: "Datei herunterladen", exact: true })
    .click();
  const file = await fileDownload;
  expect(await file.failure()).toBeNull();
  expect((await readFile((await file.path())!)).toString()).toContain(
    "demo-secret-not-a-production-password",
  );
});
test("file restore plans and executes only after exact confirmation", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  const status = await (await page.request.get("/api/status")).json();
  const b = status.backups.find(
    (b: { host_name: string }) => b.host_name === "pve-berlin-01",
  );
  await page.getByLabel("Sicherung", { exact: true }).selectOption(b.id);
  await page.getByLabel("Zielhost", { exact: true }).selectOption(b.host_id);
  await page.getByLabel("etc/sysctl.d/99-anker.conf", { exact: true }).check();
  await page.getByRole("button", { name: "Plan prüfen" }).click();
  await expect(
    page.getByRole("dialog", { name: "Wiederherstellungsplan", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "In Demo anwenden" }),
  ).toBeDisabled();
  const planDownload = page.waitForEvent("download");
  await page
    .getByRole("link", { name: "Plan herunterladen", exact: true })
    .click();
  const planFile = await planDownload;
  expect(await planFile.failure()).toBeNull();
  expect(planFile.suggestedFilename()).toMatch(/^anker-plan-.*\.tar$/);
  expect((await readFile((await planFile.path())!)).toString()).toContain(
    "prepared-files/etc/sysctl.d/99-anker.conf",
  );
  const updated = await (await page.request.get("/api/status")).json();
  const plan = updated.plans.find(
    (p: { backup_id: string }) => p.backup_id === b.id,
  );
  await page.getByLabel("Plan-ID zur Ausführung eingeben").fill(plan.id);
  await page.getByRole("button", { name: "In Demo anwenden" }).click();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Aufträge", exact: true })
    .click();
  await expect(page.getByText("Abgeschlossen", { exact: true })).toBeVisible();
});

test("backup queue and changed-file comparison use the service", async ({
  page,
}) => {
  await page
    .getByRole("button", { name: "pve-berlin-01", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Jetzt sichern", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("Sicherung gestartet");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Aufträge", exact: true })
    .click();
  await expect(
    page.getByText("Abgeschlossen", { exact: true }).first(),
  ).toBeVisible();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const state = await (await page.request.get("/api/status")).json();
  const backups = state.backups
    .filter((b: { host_name: string }) => b.host_name === "pve-berlin-01")
    .sort((a: { created_at: string }, b: { created_at: string }) =>
      a.created_at.localeCompare(b.created_at),
    );
  await page.getByLabel("Vergleich von").selectOption(backups[0].id);
  await page.getByLabel("Vergleich bis").selectOption(backups[1].id);
  await page.getByRole("button", { name: "Vergleichen", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText(
    "etc/sysctl.d/99-anker.conf",
  );
});

test("removing a test host requires its name and keeps backups", async ({
  page,
}) => {
  const before = await (await page.request.get("/api/status")).json();
  await page
    .getByRole("button", { name: "pve-browser-test", exact: true })
    .click();
  await page
    .getByRole("main")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Host entfernen", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Host entfernen",
    exact: true,
  });
  await expect(
    dialog.getByRole("button", { name: "Host entfernen", exact: true }),
  ).toBeDisabled();
  await dialog.getByLabel("Hostnamen bestätigen").fill("pve-browser-test");
  await dialog
    .getByRole("button", { name: "Host entfernen", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "pve-browser-test", exact: true }),
  ).toHaveCount(0);
  const after = await (await page.request.get("/api/status")).json();
  expect(after.backups.length).toBe(before.backups.length);
});

test("backup list downloads a complete recovery archive directly", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const downloading = page.waitForEvent("download");
  await page
    .getByRole("link", { name: "Herunterladen", exact: true })
    .first()
    .click();
  const file = await downloading;
  expect(await file.failure()).toBeNull();
  const data = await readFile((await file.path())!);
  expect(data.toString()).toContain("original-files/etc/network/interfaces");
  expect(data.toString()).toContain("WIEDERHERSTELLUNG.md");
  await expect(
    page.getByRole("heading", { name: "Sicherungen", exact: true }),
  ).toBeVisible();
});
test("failed downloads keep the user in the interface and show the reason", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.route("**/download?check=1", (route) =>
    route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({ error: "Keine Exportberechtigung" }),
    }),
  );
  await page
    .getByRole("link", { name: "Herunterladen", exact: true })
    .first()
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "Keine Exportberechtigung",
  );
  await expect(
    page.getByRole("heading", { name: "Sicherungen", exact: true }),
  ).toBeVisible();
  expect(new URL(page.url()).pathname).toBe("/");
});
test("late file responses never overwrite the newly selected file", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  let release!: () => void, seen!: () => void;
  const gate = new Promise<void>((r) => (release = r)),
    started = new Promise<void>((r) => (seen = r));
  await page.route("**/file?path=etc%2Fnetwork%2Finterfaces", async (route) => {
    const response = await route.fetch();
    seen();
    await gate;
    await route.fulfill({ response });
  });
  const late = page.waitForResponse((r) =>
    r.url().includes("file?path=etc%2Fnetwork%2Finterfaces"),
  );
  await page.getByRole("button", { name: /^etc\/network\/interfaces/ }).click();
  await started;
  await page
    .getByRole("button", { name: /^etc\/sysctl.d\/99-anker.conf/ })
    .click();
  await expect(page.locator(".file-preview strong")).toHaveText(
    "etc/sysctl.d/99-anker.conf",
  );
  release();
  await late;
  await expect(page.locator(".file-preview strong")).toHaveText(
    "etc/sysctl.d/99-anker.conf",
  );
});
test("connection failures show persistent stale-data notice", async ({
  page,
}) => {
  await page.route("**/api/status", (route) =>
    route.abort("connectionrefused"),
  );
  await expect(page.getByRole("alert")).toContainText(
    "Anker ist nicht erreichbar",
    { timeout: 10000 },
  );
  await expect(page.getByRole("alert")).toContainText("Angezeigte Daten");
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
});
test("an expired session returns to login immediately during a download", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page.route("**/download?check=1", (route) =>
    route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ error: "Anmeldung erforderlich" }),
    }),
  );
  await page
    .getByRole("link", { name: "Stand herunterladen", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Anmelden", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
test("three months of backups are shown in manageable pages", async ({
  page,
}) => {
  const state = await (await page.request.get("/api/status")).json();
  state.backups = Array.from({ length: 3600 }, (_, i) => ({
    ...state.backups[0],
    id: "test-" + i,
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(state),
    }),
  );
  await page.reload();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(page.getByRole("row")).toHaveCount(51);
  await page
    .getByRole("button", { name: /Weitere Sicherungen anzeigen/ })
    .click();
  await expect(page.getByRole("row")).toHaveCount(101);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator("body")).toHaveJSProperty("scrollWidth", 390);
});

test("failed latest backups do not leave a reassuring green host status", async ({
  page,
}) => {
  const state = await (await page.request.get("/api/status")).json();
  const h = state.hosts.find(
    (h: { name: string }) => h.name === "pve-berlin-01",
  );
  h.enabled = true;
  state.jobs.push({
    id: "failed-latest",
    host_id: h.id,
    kind: "backup",
    state: "failed",
    created_at: new Date(Date.now() + 1000).toISOString(),
    finished_at: new Date(Date.now() + 2000).toISOString(),
    error: "SSH unreachable",
    attempts: 4,
  });
  await page.route("**/api/status", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(state),
    }),
  );
  await page.reload();
  await expect(
    page.getByRole("row").filter({
      has: page.getByRole("button", { name: "pve-berlin-01", exact: true }),
    }),
  ).toContainText("Letzter Versuch fehlgeschlagen");
});

test("maintenance and notification failures remain visible to administrators", async ({
  page,
}) => {
  const state = await (await page.request.get("/api/status")).json();
  state.maintenance_health = {
    at: new Date().toISOString(),
    error: "Prüfsumme falsch",
  };
  state.notification_health = {
    at: new Date().toISOString(),
    error: "Webhook HTTP 500",
  };
  await page.route("**/api/status", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(state),
    }),
  );
  await page.reload();
  await expect(
    page.getByRole("alert").filter({ hasText: "Wartung nicht abgeschlossen" }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("alert")
      .filter({ hasText: "Benachrichtigung konnte nicht zugestellt werden" }),
  ).toBeVisible();
});
