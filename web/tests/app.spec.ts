import { test, expect } from "@playwright/test";
test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Benutzername").fill("demo");
  await page.getByLabel("Passwort").fill("anker-demo-2026");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
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
  await page.getByRole("link", { name: "Stand exportieren" }).click();
  const download = await downloading;
  expect(await download.failure()).toBeNull();
  expect(download.suggestedFilename()).toMatch(/^anker-.*\.tar$/);
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
