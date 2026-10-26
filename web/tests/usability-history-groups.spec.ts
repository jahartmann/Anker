import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test("older stands use local calendar months and days, expand lazily and search across closed groups", async ({
  page,
  request,
}, testInfo) => {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const host = base.hosts[0];
  const sources = [
    ["newest", "2026-10-20T08:00:00Z", "successful"],
    ["october-boundary", "2026-09-30T22:30:00Z", "successful"],
    ["september-boundary", "2026-09-30T21:30:00Z", "partial"],
    ["september-old", "2026-09-15T08:00:00Z", "successful"],
    ["august-old", "2026-08-15T08:00:00Z", "successful"],
  ];
  const backups = sources.map(([id, created_at, status]) => ({
    ...base.backups[0],
    id,
    created_at,
    status,
    host_id: host.id,
    host_name: host.name,
    verification_error: "",
    warnings: [],
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: {
        ...base,
        timezone: "Europe/Berlin",
        hosts: [host],
        backups,
        jobs: [],
        references: {},
      },
    }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(1);
  await expect(page.locator(".history-month")).toHaveCount(3);
  await expect(page.locator(".history-day")).toHaveCount(0);
  const october = page
    .locator(".history-month")
    .filter({
      has: page.locator(".history-month-name", { hasText: "Oktober 2026" }),
    });
  await october.locator("summary").focus();
  await page.keyboard.press("Enter");
  await expect(october.locator(".history-day > summary")).toContainText([
    "1. Oktober",
  ]);
  await expect(october.locator(".backup-table")).toHaveCount(0);
  await october.locator(".history-day > summary").click();
  await expect(october.locator(".backup-table")).toContainText("october-bo");
  const september = page
    .locator(".history-month")
    .filter({
      has: page.locator(".history-month-name", { hasText: "September 2026" }),
    });
  await expect(september.locator("summary")).toContainText("1 mit Hinweisen");
  await september.locator("summary").click();
  await expect(october).not.toHaveAttribute("open", "");
  await expect(september.locator(".history-day > summary")).toContainText([
    "30. September",
    "15. September",
  ]);
  await page.getByLabel("Sicherungsverlauf durchsuchen").fill("august-old");
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(1);
  await expect(page.locator(".backup-table")).toContainText("august-old");
  await page.getByLabel("Sicherungsverlauf durchsuchen").clear();
  await page.screenshot({
    path: testInfo.outputPath("grouped-history.png"),
    fullPage: true,
    animations: "disabled",
  });
});
