import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});
async function open(page: Page, intent = "Änderungen verfolgen") {
  await page.getByRole("button", { name: "Anker Kompass öffnen" }).click();
  const dialog = page.getByRole("dialog", { name: "Anker Kompass" });
  await dialog.getByRole("button", { name: new RegExp(intent) }).click();
  return dialog;
}

test("global compass is keyboard accessible, bounded, searchable and returns focus", async ({
  page,
}) => {
  await page.goto("/");
  const trigger = page.getByRole("button", { name: "Anker Kompass öffnen" });
  await trigger.focus();
  await page.keyboard.press("Control+k");
  let dialog = page.getByRole("dialog", { name: "Anker Kompass" });
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Control+k");
  await expect(page.getByRole("dialog")).toHaveCount(1);
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  dialog = await open(page);
  await expect(dialog.locator(".compass-host")).toHaveCount(4);
  await dialog.getByRole("button", { name: "Weitere Hosts" }).click();
  await expect(dialog.locator(".compass-host")).toHaveCount(2);
  await dialog.getByLabel("Host im Kompass suchen").fill("unfindable-host");
  await expect(dialog).toContainText("Kein passender Host");
  await dialog.getByLabel("Host im Kompass suchen").fill("PVE Berlin 02");
  await expect(dialog.locator(".compass-host")).toHaveCount(1);
  await dialog.locator(".compass-host").click();
  await dialog
    .getByRole("button", { name: "Konfigurationsreise öffnen", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Konfigurationsreise", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .locator(".host-tabs")
      .getByRole("button", { name: "Änderungen", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("same host and same page can be revisited in a different workspace", async ({
  page,
}) => {
  await page.goto("/");
  let dialog = await open(page, "Sicherungen im Blick");
  await dialog.getByLabel("Host im Kompass suchen").fill("Berlin 02");
  await dialog.locator(".compass-host").click();
  await dialog
    .getByRole("button", { name: "Sicherungsverlauf öffnen" })
    .click();
  await expect(
    page
      .locator(".host-tabs")
      .getByRole("button", { name: "Sicherungen", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  dialog = await open(page);
  await expect(
    dialog.getByRole("region", { name: "Nächster Schritt" }),
  ).toContainText("Berlin 02");
  await dialog
    .getByRole("button", { name: "Konfigurationsreise öffnen", exact: true })
    .click();
  await expect(
    page
      .locator(".host-tabs")
      .getByRole("button", { name: "Änderungen", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("a contextual compass can deliberately switch to another host", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "pve-berlin-02", exact: true })
    .click();
  const dialog = await open(page, "Sicherungen im Blick");
  await expect(
    dialog.getByRole("region", { name: "Nächster Schritt" }),
  ).toContainText("Berlin 02");
  await dialog.getByRole("button", { name: "Anderen Host wählen" }).click();
  await dialog.getByLabel("Host im Kompass suchen").fill("Berlin 01");
  await dialog.locator(".compass-host").click();
  await dialog
    .getByRole("button", { name: "Sicherungsverlauf öffnen" })
    .click();
  await expect(
    page.getByRole("heading", { name: "pve-berlin-01", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .locator(".host-tabs")
      .getByRole("button", { name: "Sicherungen", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("overview task cards prepare restoration without performing operations", async ({
  page,
}) => {
  await page.goto("/");
  const writes: string[] = [];
  page.on("request", (req) => {
    if (["POST", "PUT", "DELETE"].includes(req.method()))
      writes.push(req.url());
  });
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  await page
    .getByRole("region", { name: "Direkt zum nächsten Schritt" })
    .getByRole("button", { name: /Gezielt zurücksetzen/ })
    .click();
  const dialog = page.getByRole("dialog", { name: "Anker Kompass" });
  await dialog.getByLabel("Host im Kompass suchen").fill("Berlin 02");
  await dialog.locator(".compass-host").click();
  await expect(dialog).toContainText("Quelle und Ziel aktuell");
  await dialog
    .getByRole("button", { name: "Im Assistenten vorbereiten" })
    .click();
  await expect(page.getByLabel("Sicherung", { exact: true })).not.toHaveValue(
    "",
  );
  expect(writes).toEqual([]);
});

test("no source, read-only roles and no hosts have honest next steps", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, backups: [], jobs: [] } }),
  );
  await page.goto("/");
  let dialog = await open(page, "Gezielt zurücksetzen");
  await dialog.locator(".compass-host").first().click();
  await expect(dialog).toContainText(
    "Es gibt noch keine vollständige Sicherung",
  );
  await expect(
    dialog.getByRole("button", { name: "Im Assistenten vorbereiten" }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
  const me = await (await page.request.get("/api/me")).json();
  await page.route("**/api/me", (r) =>
    r.fulfill({ json: { ...me, user: { ...me.user, role: "reader" } } }),
  );
  await page.reload();
  dialog = await open(page, "Gezielt zurücksetzen");
  await dialog.locator(".compass-host").first().click();
  await expect(dialog).toContainText("Deine Leserechte");
  await page.keyboard.press("Escape");
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [], backups: [], jobs: [] } }),
  );
  await page.reload();
  dialog = await open(page);
  await expect(dialog).toContainText("Noch keine Hosts verbunden");
});

test("an open settings draft survives a compass destination", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  const input = page.getByLabel("Überfällig nach Stunden", { exact: true });
  await input.fill("39");
  const dialog = await open(page, "Sicherungen im Blick");
  await dialog.locator(".compass-host").first().click();
  await dialog
    .getByRole("button", { name: "Sicherungsverlauf öffnen" })
    .click();
  await expect(input).toHaveValue("39");
  await expect(page.getByRole("alert")).toContainText(
    "Dein Entwurf ist noch offen",
  );
});

test("active work opens the existing job instead of starting a duplicate", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const host = base.hosts[0];
  const job = {
    ...base.jobs[0],
    id: "compass-active",
    host_id: host.id,
    kind: "backup",
    state: "running",
    created_at: new Date().toISOString(),
    attempts: 1,
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [host], jobs: [job] } }),
  );
  await page.route("**/api/jobs/compass-active", (r) =>
    r.fulfill({ json: job }),
  );
  await page.goto("/");
  const dialog = await open(page, "Zustand verstehen");
  await dialog.locator(".compass-host").click();
  await expect(dialog).toContainText("bereits ein Auftrag aktiv");
  await dialog
    .getByRole("button", { name: "Laufenden Auftrag ansehen" })
    .click();
  await expect(
    page.getByRole("heading", { name: "Aufträge", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("dialog", { name: "Auftragsdetails" }),
  ).toContainText("compass-active");
});

test("mobile compass fits and retains the next step without a long list", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  const dialog = await open(page);
  await expect(dialog.locator(".compass-host")).toHaveCount(4);
  await dialog.locator(".compass-host").first().click();
  await expect(
    dialog.getByRole("button", {
      name: "Konfigurationsreise öffnen",
      exact: true,
    }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: testInfo.outputPath("compass-mobile.png") });
  expect(errors).toEqual([]);
});
