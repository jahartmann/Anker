import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
  await page.goto("/");
});

test("overview prioritizes affected hosts and expands source evidence on demand", async ({
  page,
}) => {
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  const attention = page.getByRole("region", {
    name: "Jetzt wichtig",
  });
  await expect(attention).toBeVisible();
  const evidence = page.getByText("Prüfergebnisse der letzten Sicherung", {
    exact: true,
  });
  await expect(evidence).toBeVisible();
  await expect(
    page.getByRole("region", { name: "Prüfergebnisse" }),
  ).not.toBeVisible();
  const attentionBox = await attention.boundingBox();
  const evidenceBox = await evidence.boundingBox();
  expect(attentionBox!.y).toBeLessThan(evidenceBox!.y);
  await evidence.click();
  await expect(
    page.getByRole("region", { name: "Prüfergebnisse" }),
  ).toBeVisible();
});

test("narrow host list retains the latest backup date", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const row = page.getByRole("row").filter({
    has: page.getByRole("button", { name: "pve-berlin-01", exact: true }),
  });
  await expect(row.getByText(/^Letzter Stand:/)).toBeVisible();
  await expect(row.getByText(/^Letzter Stand:/)).not.toContainText("—");
});

test("recovery explains its total scope before selecting a source", async ({
  page,
}) => {
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page
      .getByRole("navigation", { name: "Vorbereitungsschritte" })
      .getByRole("status"),
  ).toBeVisible();
  await expect(
    page
      .getByRole("navigation", { name: "Vorbereitungsschritte" })
      .getByRole("status"),
  ).toHaveText("Schritt 1 von 5");
  await expect(
    page
      .getByRole("navigation", { name: "Vorbereitungsschritte" })
      .getByRole("listitem"),
  ).toHaveCount(5);
  await expect(
    page.getByText(
      "Schritt 5: Ausführen und nachprüfen – im gespeicherten Plan.",
      { exact: true },
    ),
  ).toBeVisible();
});

test("system diagnosis summarizes health and exports only approved support fields", async ({
  page,
}) => {
  await page.route("**/api/doctor", (route) =>
    route.fulfill({
      json: {
        version: "v0.2.10",
        demo: true,
        protocol: 1,
        data_root: "/private/secret-customer-path",
        disk: { total: 1000, free: 50, used_percent: 95 },
        host_helper: "secret-host-helper",
        token: "never-export-token",
      },
    }),
  );
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Betrieb", exact: true }).click();
  await page.getByText("Systemprüfung", { exact: true }).click();
  await expect(
    page.getByText("Speicherplatz prüfen", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("paragraph").filter({ hasText: /95 % belegt/ }),
  ).toBeVisible();
  const downloadEvent = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Diagnosebericht herunterladen", exact: true })
    .click();
  const download = await downloadEvent;
  const stream = await download.createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
  const text = Buffer.concat(chunks).toString("utf8");
  expect(JSON.parse(text)).toMatchObject({
    format: 1,
    version: "v0.2.10",
    demo: true,
    disk: { used_percent: 95 },
  });
  expect(text).not.toContain("secret-customer-path");
  expect(text).not.toContain("secret-host-helper");
  expect(text).not.toContain("never-export-token");
});
