import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

// An unordered API response must still select the newest source, with every
// older stand reachable without putting the complete history on the start page.
test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const host = { ...base.hosts[0], name: "pve-berlin-01", enabled: true };
  const backups = Array.from({ length: 35 }, (_, index) => ({
    ...base.backups[0],
    id: `workspace-${String(index).padStart(3, "0")}`,
    host_id: host.id,
    host_name: host.name,
    created_at: new Date(Date.now() - (35 - index) * 3600000).toISOString(),
    status: "successful",
    pinned: false,
    archived: false,
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: { ...base, hosts: [host], backups, references: {}, jobs: [] },
    }),
  );
  await page.route("**/api/backups/compare", (route) =>
    route.fulfill({ json: [] }),
  );
  await page.goto("/");
});

test("host workspace chooses newest restore source and compares only that host", async ({
  page,
}, testInfo) => {
  await page
    .getByRole("button", { name: "pve-berlin-01", exact: true })
    .click();
  await expect(page.locator(".host-status-strip")).toContainText("Gesichert");
  await expect(
    page.getByRole("button", { name: "Wiederherstellung planen", exact: true }),
  ).toBeEnabled();
  await page.screenshot({
    path: testInfo.outputPath("host-desktop.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Änderungen ansehen", exact: true })
    .click();
  await expect(page.getByLabel("Vergleich von", { exact: true })).toHaveValue(
    "workspace-033",
  );
  await expect(page.getByLabel("Vergleich bis", { exact: true })).toHaveValue(
    "workspace-034",
  );
  await page
    .getByRole("button", { name: "Zurück zum Hostverlauf", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Übersicht", exact: true })
    .last()
    .click();
  await page
    .getByRole("button", { name: "Wiederherstellung planen", exact: true })
    .click();
  await expect(page.getByLabel("Sicherung", { exact: true })).toHaveValue(
    "workspace-034",
  );
});

test("collections show one host and history stays bounded, searchable and keyboard accessible", async ({
  page,
}, testInfo) => {
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(page.locator(".backup-collection")).toHaveCount(1);
  await expect(page.locator(".backup-table")).toHaveCount(0);
  const toggle = page.getByRole("button", {
    name: "Verlauf für pve-berlin-01",
  });
  await toggle.focus();
  await page.keyboard.press("Enter");
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(1);
  await expect(page.locator(".history-month[open]")).toHaveCount(0);
  await page.locator(".history-month > summary").first().click();
  await expect(page.locator(".history-day[open]")).toHaveCount(0);
  await page.locator(".history-day > summary").first().click();
  await expect(
    page.locator(".history-day[open] .backup-table tbody tr").first(),
  ).toBeVisible();
  await page.getByLabel("Sicherungsverlauf durchsuchen").fill("workspace-000");
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(1);
  await expect(page.locator(".backup-table")).toContainText("workspace-");
  await page.getByLabel("Sicherungsverlauf durchsuchen").clear();
  await page.screenshot({
    path: testInfo.outputPath("history-desktop.png"),
    fullPage: true,
  });
  await toggle.click();
  await page.getByRole("button", { name: "Alle Stände", exact: true }).click();
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(25);
});

test("host workspace and expanded history fit phone screens without runtime errors", async ({
  page,
}, testInfo) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("button", { name: "pve-berlin-01", exact: true })
    .click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.screenshot({
    path: testInfo.outputPath("host-mobile.png"),
    fullPage: true,
  });
  await page
    .locator(".tabs")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.screenshot({
    path: testInfo.outputPath("history-mobile.png"),
    fullPage: true,
  });
  expect(errors).toEqual([]);
});

test("host history compares with the previous stand without leaving its workspace", async ({
  page,
}) => {
  await page
    .getByRole("button", { name: "pve-berlin-01", exact: true })
    .click();
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .locator(".backup-table tbody tr")
    .first()
    .getByRole("button", { name: /^Aktionen für/ })
    .click();
  await page
    .getByRole("button", {
      name: "Mit vorherigem Stand vergleichen",
      exact: true,
    })
    .click();
  const comparison = page.getByRole("region", { name: "Sicherungsvergleich" });
  await expect(
    comparison.getByLabel("Vergleich von", { exact: true }),
  ).toHaveValue("workspace-033");
  await expect(
    comparison.getByLabel("Vergleich bis", { exact: true }),
  ).toHaveValue("workspace-034");
  await expect(
    page.getByRole("heading", { name: "pve-berlin-01", exact: true }),
  ).toBeVisible();
});

test("empty collections retain unresolved references and explain the next step", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const host = base.hosts[0];
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: {
        ...base,
        hosts: [host],
        backups: [],
        jobs: [],
        references: {
          [host.id]: {
            host_id: host.id,
            state: "unresolved",
            name: "Vor Umbau",
            backup_id: "missing",
          },
        },
      },
    }),
  );
  await page.reload();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Noch keine Sicherungen", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Referenz: Vor Umbau · nicht auflösbar", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Mit Referenz vergleichen" }),
  ).toHaveCount(0);
});
