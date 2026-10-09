import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
const sample = (id: string, state = "incomplete") => ({
  version: 1,
  id,
  cluster_id: "berlin",
  purpose: "manualSourcePlanning",
  atomic: false,
  created_at: "2026-10-09T08:00:00Z",
  started_at: "2026-10-09T08:00:01Z",
  completed_at: "2026-10-09T08:00:05Z",
  start_skew_ms: 2000,
  state,
  members: [
    {
      host_id: "h1",
      host_name: "pve-eins",
      backup_id: "actual-backup-1",
      job_id: "job1",
      state: "captured",
      started_at: "2026-10-09T08:00:01Z",
      completed_at: "2026-10-09T08:00:04Z",
      membership_known: true,
      shared_known: true,
    },
    {
      host_id: "h2",
      host_name: "pve-zwei",
      state: "interrupted",
      membership_known: false,
      shared_known: false,
    },
  ],
  notes: ["Zeitversetzte Erfassung; kein atomarer Clustersnapshot."],
});
for (const width of [1440, 390])
  test(`Sicherungssets zeigen Grenzen und exportieren Referenzen bei ${width}px`, async ({
    page,
    request,
  }) => {
    await page.context().addCookies(await demoCookies(request));
    await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 });
    await page.route("**/api/cluster-sets", (r) =>
      r.fulfill({ json: [sample("set1")] }),
    );
    await page.route("**/api/cluster-sets/set1", (r) =>
      r.fulfill({ json: sample("set1") }),
    );
    await page.goto("/");
    if (width === 390)
      await page.getByRole("button", { name: "Navigation öffnen" }).click();
    await page
      .getByRole("navigation")
      .getByRole("button", { name: "Sicherungen", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Cluster-Sicherungssets", exact: true })
      .click();
    await expect(page.getByText("Nur manuelle Quellplanung")).toBeVisible();
    await page.getByRole("button", { name: /set1/ }).click();
    await expect(
      page.getByText("actual-backup-1", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("Unterbrochen", { exact: true })).toBeVisible();
    await expect(
      page.getByText("Unvollständig", { exact: true }).last(),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Quellreferenzen exportieren" }),
    ).toHaveAttribute("href", "/api/cluster-sets/set1/export");
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBeTruthy();
    await page.screenshot({
      path: `/tmp/anker-task11-${width}.png`,
      fullPage: true,
    });
  });
test("Sicherungssetwechsel verwirft verspätete Details", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  await page.route("**/api/cluster-sets", (r) =>
    r.fulfill({ json: [sample("set1"), sample("set2", "divergent")] }),
  );
  let release: () => void = () => {};
  let reached: () => void = () => {};
  const reachedPromise = new Promise<void>((r) => (reached = r));
  const gate = new Promise<void>((r) => (release = r));
  await page.route("**/api/cluster-sets/set1", async (r) => {
    reached();
    await gate;
    await r.fulfill({ json: sample("set1") });
  });
  await page.route("**/api/cluster-sets/set2", (r) =>
    r.fulfill({ json: sample("set2", "divergent") }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Cluster-Sicherungssets", exact: true })
    .click();
  await page.getByRole("button", { name: /set1/ }).click();
  await reachedPromise;
  await page.getByRole("button", { name: /set2/ }).click();
  await expect(
    page.getByRole("link", { name: "Quellreferenzen exportieren" }),
  ).toHaveAttribute("href", "/api/cluster-sets/set2/export");
  release();
  await expect(
    page.getByRole("heading", { name: "Sicherungsset set2" }),
  ).toBeVisible();
});

test("Echte Demo-Erfassung enthält neue Referenzen und bleibt bei unbekannter Mitgliedschaft unvollständig", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Cluster-Sicherungssets", exact: true })
    .click();
  await page.getByLabel("Cluster für die Erfassung").selectOption("berlin");
  const captured = page.waitForResponse(
    (r) =>
      r.url().endsWith("/api/cluster-sets") && r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Neues Sicherungsset erfassen" }).click();
  const result = await (await captured).json();
  expect(result.members.length).toBeGreaterThan(1);
  await expect(
    page.getByRole("heading", { name: `Sicherungsset ${result.id}` }),
  ).toBeVisible();
  await expect(
    page.getByText("Unvollständig", { exact: true }).last(),
  ).toBeVisible({ timeout: 15000 });
  await expect(
    page.getByText("Nachweis fehlt", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Quellreferenzen exportieren" }),
  ).toHaveAttribute("href", `/api/cluster-sets/${result.id}/export`);
});
