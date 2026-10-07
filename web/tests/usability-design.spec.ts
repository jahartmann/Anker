import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import type { APIRequestContext } from "@playwright/test";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await demoCookies(request);
});

test("browser identity is available before login and follows the current view", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page).toHaveTitle("Anmelden · Anker");
  const icon = await page.locator('link[rel="icon"]').getAttribute("href");
  expect(icon).toBeTruthy();
  const response = await page.request.get(icon!);
  expect(response.ok()).toBeTruthy();
  expect(response.headers()["content-type"]).toContain("image/svg+xml");
  expect(await response.text()).toContain("<svg");
  await page.context().addCookies(cookies);
  await page.reload();
  await expect(page).toHaveTitle("Hosts · Anker");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(page).toHaveTitle("Sicherungen · Anker");
});

test("overview distinguishes active, paused and unsaved hosts from live jobs", async ({
  page,
}) => {
  await page.context().addCookies(cookies);
  const base = await (await page.request.get("/api/status")).json();
  const hosts = base.hosts
    .slice(0, 4)
    .map((h: any, i: number) => ({ ...h, enabled: i !== 1 }));
  const successful = {
    ...base.backups[0],
    host_id: hosts[0].id,
    created_at: new Date().toISOString(),
    status: "successful",
  };
  const paused = { ...successful, id: "paused-backup", host_id: hosts[1].id };
  const jobs = ["running", "queued", "successful"].map((state, i) => ({
    id: "overview-job-" + i,
    host_id: hosts[i].id,
    kind: "probe",
    state,
    created_at: new Date().toISOString(),
    attempts: 1,
    error: "",
  }));
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: { ...base, hosts, backups: [successful, paused], jobs },
    }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  const summary = page.getByRole("region", { name: "Sicherungsstatus" });
  await expect(
    summary
      .getByText("Aktive Hosts", { exact: true })
      .locator("..")
      .locator("dd"),
  ).toHaveText("3");
  await expect(
    summary
      .getByText("Aktuell gesichert", { exact: true })
      .locator("..")
      .locator("dd"),
  ).toHaveText("1");
  await expect(
    summary
      .getByText("Handlungsbedarf", { exact: true })
      .locator("..")
      .locator("dd"),
  ).toHaveText("2");
  await expect(
    summary
      .getByText("Aktive Aufträge", { exact: true })
      .locator("..")
      .locator("dd"),
  ).toHaveText("2");
  await expect(summary).toContainText("1 Host pausiert");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(summary).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});
