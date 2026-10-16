import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test("plan collection reports matching totals and offers a filter reset", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const plans = ["ready", "interrupted"].map((state, index) => ({
    id: `polish-plan-${index}`,
    state,
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    scenario: "files",
    created_at: new Date().toISOString(),
    source: { ...base.hosts[0].inventory, hostname: `source-${index}` },
    target: base.hosts[0].inventory,
    steps: [],
    blockers: [],
    manual: [],
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, plans } }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await expect(page.getByText("2 von 2 Plänen", { exact: true })).toBeVisible();
  await page.getByLabel("Planstatus filtern").selectOption("interrupted");
  await expect(page.getByText("1 von 2 Plänen", { exact: true })).toBeVisible();
  await page
    .getByLabel("Wiederherstellungspläne durchsuchen")
    .fill("not-present");
  await expect(page.getByText("0 von 2 Plänen", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "Planfilter zurücksetzen", exact: true })
    .click();
  await expect(
    page
      .getByRole("table", { name: "Wiederherstellungspläne", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
});
