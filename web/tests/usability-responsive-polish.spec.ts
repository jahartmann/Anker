import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test("all four views remain usable at phone width without console errors", async ({
  page,
  request,
}, testInfo) => {
  await page.context().addCookies(await demoCookies(request));
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const base = await (await page.request.get("/api/status")).json();
  const job = {
    id: "responsive-job",
    host_id: base.hosts[0].id,
    kind: "backup",
    state: "successful",
    result_id: base.backups[0].id,
    created_at: new Date().toISOString(),
    attempts: 1,
  };
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, jobs: [job] } }),
  );
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/");
  for (const view of ["Übersicht", "Hosts", "Sicherungen", "Aufträge"]) {
    await page.getByRole("button", { name: "Navigation öffnen" }).click();
    await page
      .getByRole("navigation", { name: "Hauptnavigation" })
      .getByRole("button", { name: view, exact: true })
      .click();
    await expect(page).toHaveTitle(`${view} · Anker`);
    await expect(
      page.getByRole("heading", { name: view, exact: true }),
    ).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(375);
    await page.screenshot({
      path: testInfo.outputPath(`${view}-375.png`),
      fullPage: true,
    });
  }
  expect(errors).toEqual([]);
});
