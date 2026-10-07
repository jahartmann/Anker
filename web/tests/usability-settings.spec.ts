import { demoCookies as loginDemo } from "./demo-auth";
import { test, expect } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await loginDemo(request);
});
test.beforeEach(async ({ page }) => {
  await page.context().addCookies(cookies);
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
});

test("completed update reloads assets once even after leaving settings and temporary downtime", async ({
  page,
}) => {
  let phase = "idle",
    documentLoads = 0,
    unavailable = 0;
  page.on("framenavigated", (frame) => {
    if (frame === page.mainFrame()) documentLoads++;
  });
  await page.route("**/api/updates", (route) => {
    if (phase === "downtime" && unavailable++ < 2)
      return route.fulfill({ status: 503, json: { error: "Dienst startet" } });
    return route.fulfill({
      json: {
        configured: true,
        repository: "jahartmann/Anker",
        current: phase === "complete" ? "0.3.0" : "0.2.2",
        status:
          phase === "complete"
            ? "successful"
            : phase === "idle"
              ? "idle"
              : "installing",
        target: phase === "idle" ? undefined : "v0.3.0",
        message:
          phase === "complete" ? "Update installiert und Start geprüft" : "",
        available:
          phase === "idle"
            ? {
                version: "v0.3.0",
                url: "https://github.com/jahartmann/Anker/releases/tag/v0.3.0",
                artifact: { size: 1234 },
              }
            : undefined,
      },
    });
  });
  await page.route("**/api/updates/install", (route) => {
    phase = "installing";
    return route.fulfill({
      status: 202,
      json: {
        configured: true,
        repository: "jahartmann/Anker",
        current: "0.2.2",
        target: "v0.3.0",
        status: "installing",
      },
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
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Jetzt installieren", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  phase = "downtime";
  await expect
    .poll(() => unavailable, { timeout: 10000 })
    .toBeGreaterThanOrEqual(2);
  expect(documentLoads).toBe(0);
  phase = "complete";
  await expect.poll(() => documentLoads, { timeout: 10000 }).toBe(1);
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  await page.waitForTimeout(2500);
  expect(documentLoads).toBe(1);
});

test("failed update preserves page and never triggers a reload loop", async ({
  page,
}) => {
  let phase = "idle",
    documentLoads = 0,
    completedReads = 0;
  page.on("framenavigated", (frame) => {
    if (frame === page.mainFrame()) documentLoads++;
  });
  await page.route("**/api/updates", (route) => {
    if (phase === "failed") completedReads++;
    return route.fulfill({
      json: {
        configured: true,
        repository: "jahartmann/Anker",
        current: "0.2.2",
        target: phase === "idle" ? undefined : "v0.3.0",
        status: phase,
        message:
          phase === "failed" ? "Vorherige Version wiederhergestellt" : "",
        available:
          phase === "idle"
            ? {
                version: "v0.3.0",
                url: "https://github.com/jahartmann/Anker/releases/tag/v0.3.0",
                artifact: { size: 1234 },
              }
            : undefined,
      },
    });
  });
  await page.route("**/api/updates/install", (route) => {
    phase = "installing";
    return route.fulfill({
      status: 202,
      json: {
        configured: true,
        repository: "jahartmann/Anker",
        current: "0.2.2",
        target: "v0.3.0",
        status: "installing",
      },
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
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Jetzt installieren", exact: true })
    .click();
  phase = "failed";
  await expect(
    page
      .getByText("Vorherige Version wiederhergestellt", { exact: true })
      .first(),
  ).toBeVisible({ timeout: 10000 });
  await expect.poll(() => completedReads).toBeGreaterThan(0);
  expect(documentLoads).toBe(0);
  await expect
    .poll(() =>
      page.evaluate(() => sessionStorage.getItem("anker:update-pending")),
    )
    .toBeNull();
});

test("retention explains per-host buckets and can disable automatic archiving without changing retention", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await expect(page.getByText(/Regeln gelten für jeden Host/)).toBeVisible();
  await expect(
    page.getByText(/Ein Stand kann mehrere Regeln erfüllen/),
  ).toBeVisible();
  await page.getByLabel("Tagesstände", { exact: true }).fill("7");
  await page.getByLabel("Wochenstände", { exact: true }).fill("4");
  await page.getByLabel("Monatsstände", { exact: true }).fill("3");
  await expect(
    page.getByRole("status", { name: "Aufbewahrungsübersicht" }),
  ).toContainText("7 Tage");
  await expect(
    page.getByRole("status", { name: "Aufbewahrungsübersicht" }),
  ).toContainText("4 Wochen");
  await expect(
    page.getByRole("status", { name: "Aufbewahrungsübersicht" }),
  ).toContainText("3 Monate");
  await page
    .getByLabel("Ältere Stände automatisch archivieren", { exact: true })
    .uncheck();
  await page
    .getByRole("button", { name: "Einstellungen speichern", exact: true })
    .click();
  await expect(
    page.getByText("Alle Änderungen gespeichert", { exact: true }).first(),
  ).toBeVisible();
  const saved = await (await page.request.get("/api/settings")).json();
  expect(saved).toMatchObject({
    daily: 7,
    weekly: 4,
    monthly: 3,
    archive_days: 0,
  });
  await page.reload();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await expect(
    page.getByLabel("Ältere Stände automatisch archivieren", { exact: true }),
  ).not.toBeChecked();
  await expect(page.getByLabel("Tagesstände", { exact: true })).toHaveValue(
    "7",
  );
});
