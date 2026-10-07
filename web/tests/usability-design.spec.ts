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

test("backup file actions remain fully visible on a short laptop viewport", async ({
  page,
}) => {
  await page.context().addCookies(cookies);
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("row")
    .filter({ hasText: "pve-hamburg-02" })
    .getByRole("button", { name: "Dateien", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "Dateien · pve-hamburg-02" });
  await expect(
    dialog.getByRole("button", { name: "Wiederherstellung planen" }),
  ).toBeEnabled();
  const frame = await dialog.boundingBox();
  const action = await dialog
    .getByRole("link", { name: "Stand herunterladen", exact: true })
    .boundingBox();
  expect(frame).toBeTruthy();
  expect(action).toBeTruthy();
  expect(action!.y + action!.height).toBeLessThanOrEqual(
    frame!.y + frame!.height - 1,
  );
  await dialog
    .getByRole("textbox", { name: "Dateien durchsuchen", exact: true })
    .fill("sysctl");
  await dialog
    .getByRole("button", { name: /^etc\/sysctl.d\/99-anker.conf/ })
    .click();
  await expect(
    dialog.getByRole("link", { name: "Datei herunterladen", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  const mobileFrame = await dialog.boundingBox();
  const mobileAction = await dialog
    .getByRole("button", { name: "Datei wiederherstellen" })
    .boundingBox();
  expect(mobileAction!.y + mobileAction!.height).toBeLessThanOrEqual(
    mobileFrame!.y + mobileFrame!.height - 1,
  );
});
