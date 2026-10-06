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

test("selected file carries into a fresh restore request", async ({ page }) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: /^etc\/sysctl.d\/99-anker.conf/ })
    .click();
  await page
    .getByRole("button", { name: "Datei wiederherstellen", exact: true })
    .click();
  await expect(
    page.getByRole("checkbox", {
      name: "etc/sysctl.d/99-anker.conf",
      exact: true,
    }),
  ).toBeChecked();
  await expect(
    page.getByRole("checkbox", { name: "etc/hostname", exact: true }),
  ).not.toBeChecked();
});

test("file list failures can be retried without closing the dialog", async ({
  page,
}) => {
  let fail = true;
  await page.route("**/api/backups/*/files", (route) =>
    fail
      ? route.fulfill({
          status: 503,
          json: { error: "Ablage vorübergehend nicht erreichbar" },
        })
      : route.continue(),
  );
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Ablage vorübergehend nicht erreichbar",
  );
  fail = false;
  await page.getByRole("button", { name: "Erneut laden", exact: true }).click();
  await expect(
    page.getByRole("button", { name: /^etc\/hostname/ }),
  ).toBeVisible();
  await page.getByPlaceholder("Pfad suchen").fill("no-match");
  await expect(
    page.getByText("Keine passenden Dateien", { exact: true }),
  ).toBeVisible();
});

test("preview failure retains selected file and supports retry", async ({
  page,
}) => {
  let fail = true;
  await page.route("**/api/backups/*/file?*", (route) =>
    fail
      ? route.fulfill({
          status: 503,
          json: { error: "Datei momentan nicht lesbar" },
        })
      : route.continue(),
  );
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: /^etc\/hostname/ }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Datei momentan nicht lesbar",
  );
  await expect(
    page.getByRole("link", { name: "Datei herunterladen", exact: true }),
  ).toBeVisible();
  fail = false;
  await page.getByRole("button", { name: "Erneut lesen", exact: true }).click();
  await expect(page.locator(".file-preview pre")).toBeVisible();
});

test("comparison cannot retain the same source and destination", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const { backups } = await (await page.request.get("/api/status")).json();
  await page.getByLabel("Vergleich von").selectOption(backups[0].id);
  await page.getByLabel("Vergleich bis").selectOption(backups[1].id);
  await page.getByLabel("Vergleich von").selectOption(backups[1].id);
  await expect(
    page.getByRole("button", { name: "Vergleichen", exact: true }),
  ).toBeDisabled();
});

test("last backup actions stay visible and close with Escape", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const trigger = page
    .locator(".action-menu > summary, .action-menu > button")
    .last();
  await trigger.click();
  const archive = page.getByRole("button", {
    name: "Archivieren",
    exact: true,
  });
  await expect(archive).toBeInViewport();
  const visible = await archive.evaluate((el) => {
    const rect = el.getBoundingClientRect();
    return el.contains(
      document.elementFromPoint(
        rect.x + rect.width / 2,
        rect.y + rect.height / 2,
      ),
    );
  });
  expect(visible).toBe(true);
  await page.keyboard.press("Escape");
  await expect(archive).not.toBeVisible();
  await expect(trigger).toBeFocused();
});

test("mobile backup actions require no sideways scrolling", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Navigation öffnen" }).click();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const files = page
    .getByRole("button", { name: "Dateien", exact: true })
    .first();
  await expect(files).toBeInViewport();
  expect(
    await page
      .locator(".table-scroll")
      .first()
      .evaluate((el) => el.scrollWidth <= el.clientWidth),
  ).toBe(true);
});

test("settings load failure offers retry and changes have a visible saved state", async ({
  page,
}) => {
  let fail = true;
  await page.route("**/api/settings", (route) =>
    fail
      ? route.fulfill({
          status: 503,
          json: { error: "Einstellungen momentan nicht verfügbar" },
        })
      : route.continue(),
  );
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Einstellungen momentan nicht verfügbar",
  );
  fail = false;
  await page.getByRole("button", { name: "Erneut laden", exact: true }).click();
  await page.getByLabel("Tagesstände").fill("37");
  await expect(
    page.getByText("Ungespeicherte Änderungen", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Einstellungen speichern" }).click();
  await expect(
    page.getByText("Alle Änderungen gespeichert", { exact: true }),
  ).toBeVisible();
});

test("an overview with paused hosts does not claim they are secured", async ({
  page,
}) => {
  await page.route("**/api/status", async (route) => {
    const response = await route.fetch();
    const status = await response.json();
    status.hosts.forEach(
      (host: { enabled: boolean }) => (host.enabled = false),
    );
    await route.fulfill({ response, json: status });
  });
  await page.reload();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "Automatische Sicherung pausiert",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", {
      name: "Alle Hosts sind aktuell gesichert",
      exact: true,
    }),
  ).not.toBeVisible();
});

test("settings drafts stay intact when navigating away is cancelled", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByLabel("Tagesstände").fill("39");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Änderungen noch nicht gespeichert" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Weiter bearbeiten", exact: true })
    .click();
  await expect(page.getByLabel("Tagesstände")).toHaveValue("39");
});

test("a running plan request cannot be dismissed or submitted twice", async ({
  page,
}) => {
  let calls = 0;
  let finish!: () => void;
  const gate = new Promise<void>((resolve) => (finish = resolve));
  await page.route("**/api/plans", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    calls++;
    await gate;
    await route.fulfill({
      status: 503,
      json: { error: "Ziel für Prüfung nicht erreichbar" },
    });
  });
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  const status = await (await page.request.get("/api/status")).json();
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(status.backups[0].id);
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(status.hosts[0].id);
  await page
    .getByRole("checkbox", { name: "etc/sysctl.d/99-anker.conf", exact: true })
    .check();
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Dialog schließen", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeVisible();
  expect(calls).toBe(1);
  finish();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Ziel für Prüfung nicht erreichbar",
  );
});

test("logout network failure stays visible without losing the session", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.route("**/api/logout", (route) => route.abort("connectionfailed"));
  await page.getByRole("button", { name: "Abmelden", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Verbindung");
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
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

test("resizing with an open action menu closes it without browser errors", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: /^Aktionen für/ })
    .first()
    .click();
  await expect(
    page.getByRole("button", { name: "Archivieren", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 1000, height: 800 });
  await expect(page.locator(".action-popover")).not.toBeVisible();
  expect(errors).toEqual([]);
});

test("access administration shows session policy and protects the signed-in account", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Zugriff", exact: true }).click();
  await expect(page.getByLabel("Sitzungsdauer in Tagen")).toHaveValue("30");
  await page.getByRole("button", { name: "Aktionen für demo" }).click();
  await page.getByRole("button", { name: "Zugang bearbeiten" }).click();
  await expect(page.getByRole("dialog")).toContainText("demo");
  await expect(page.getByLabel("Zugang gesperrt")).toBeDisabled();
});
test("update panel describes setup when the updater is unavailable", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "System", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Updates", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Updates sind in der Demo ausgeschaltet.", { exact: false }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Nach Updates suchen" }),
  ).toBeDisabled();
});
test("update installation requires a reviewed version and recovers after a restart", async ({
  page,
}) => {
  let installing = false;
  const state = () => ({
    configured: true,
    repository: "example/anker",
    current: installing ? "0.3.0" : "0.2.0",
    status: installing ? "successful" : "idle",
    available: installing
      ? undefined
      : {
          version: "v0.3.0",
          url: "https://github.com/example/anker/releases/tag/v0.3.0",
          artifact: { size: 123456 },
        },
    message: installing ? "Update installiert und Start geprüft" : "",
  });
  await page.route("**/api/updates", (route) =>
    route.fulfill({ json: state() }),
  );
  await page.route("**/api/updates/check", (route) =>
    route.fulfill({ json: state() }),
  );
  await page.route("**/api/updates/install", (route) => {
    expect(route.request().postDataJSON()).toEqual({ version: "v0.3.0" });
    installing = true;
    return route.fulfill({
      json: { ...state(), status: "installing", target: "v0.3.0" },
    });
  });
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "System", exact: true }).click();
  await page
    .getByRole("button", { name: "Update installieren", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText("v0.3.0");
  await page
    .getByRole("button", { name: "Jetzt installieren", exact: true })
    .click();
  await expect(
    page.getByText("Update installiert und Start geprüft", { exact: true }),
  ).toBeVisible({ timeout: 10000 });
});
