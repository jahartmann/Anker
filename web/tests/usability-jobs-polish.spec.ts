import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";
import type { Job, Status } from "../src/api";
import { demoCookies } from "./demo-auth";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await demoCookies(request);
});

async function fixture(page: Page, states = ["successful"]) {
  await page.context().addCookies(cookies);
  const base = (await (await page.request.get("/api/status")).json()) as Status;
  const jobs: Job[] = states.map((state, index) => ({
    id: "polish-job-" + index,
    host_id: base.hosts[0].id,
    kind: "backup",
    state,
    result_id: state === "successful" ? base.backups[0].id : undefined,
    created_at: "2026-10-15T12:00:00Z",
    started_at: state === "queued" ? undefined : "2026-10-15T12:01:00Z",
    finished_at: state === "successful" ? "2026-10-15T12:02:00Z" : undefined,
    attempts: state === "queued" ? 0 : 1,
    trigger: "manual",
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, jobs } }),
  );
  await page.route("**/api/jobs/*", (route) => {
    const id = route.request().url().split("/").pop();
    return route.fulfill({ json: jobs.find((job) => job.id === id) });
  });
  await page.route("**/api/backups/*/files", (route) =>
    route.fulfill({ json: [] }),
  );
  await page.goto("/");
  return base;
}

async function openJobs(page: Page) {
  await page
    .getByRole("navigation")
    .getByRole("button", {
      name: "Aufträge",
      exact: true,
    })
    .click();
}

test("a successful backup can be opened directly from its history result", async ({
  page,
}) => {
  await fixture(page);
  await openJobs(page);
  const table = page.getByRole("table", { name: "Auftragsverlauf" });
  await table
    .getByRole("button", { name: "Sicherung öffnen", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByRole("dialog")).not.toContainText("Auftragsdetails");
});

test("successful job details use the same status as the history", async ({
  page,
}) => {
  await fixture(page);
  await openJobs(page);
  await page
    .getByRole("button", { name: "Auftrag polish-job-0 ansehen" })
    .click();
  await expect(
    page.getByRole("dialog").getByText("Erfolgreich", { exact: true }),
  ).toBeVisible();
});

test("status shortcuts include queued and running work and retain accurate filtered totals", async ({
  page,
}) => {
  await fixture(page, ["successful", "queued", "running", "failed"]);
  await openJobs(page);
  await expect(
    page.getByText("4 von 4 Aufträgen", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Aktiv 2", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "Auftragsstatus" }),
  ).toHaveValue("active");
  await expect(
    page.getByText("2 von 4 Aufträgen", { exact: true }),
  ).toBeVisible();
  const active = page.getByRole("region", {
    name: "Aktive Aufträge",
    exact: true,
  });
  await expect(active).toContainText("1 laufen");
  await expect(active).toContainText("1 warten");
  await page
    .getByRole("combobox", { name: "Auftragsstatus" })
    .selectOption("successful");
  await expect(
    page.getByRole("button", { name: "Erfolgreich 1", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByText("1 von 4 Aufträgen", { exact: true }),
  ).toBeVisible();
});

test("empty search retains the source total and reset restores the result", async ({
  page,
}) => {
  await fixture(page);
  await openJobs(page);
  await expect(
    page.getByText("1 von 1 Auftrag", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("searchbox", { name: "Aufträge suchen" })
    .fill("missing-job");
  await expect(
    page.getByText("0 von 1 Auftrag", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Keine passenden Aufträge" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Filter zurücksetzen" }).click();
  await expect(
    page.getByText("1 von 1 Auftrag", { exact: true }),
  ).toBeVisible();
});

test("overview recent activity is compact and keeps job details reachable", async ({
  page,
}) => {
  await fixture(page);
  await page
    .getByRole("navigation")
    .getByRole("button", {
      name: "Übersicht",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("table", { name: "Auftragsverlauf" }),
  ).toHaveCount(0);
  await expect(page.locator(".overview-history")).not.toHaveAttribute(
    "open",
    "",
  );
  await page.locator(".overview-history > summary").click();
  const recent = page.getByRole("list", { name: "Letzte Aufträge" });
  await expect(recent.getByRole("listitem")).toHaveCount(1);
  await expect(recent).toContainText("Erfolgreich");
  await recent
    .getByRole("button", { name: "Auftrag polish-job-0 ansehen" })
    .click();
  await expect(page.getByRole("dialog")).toContainText("polish-job-0");
});

test("failed restore keeps its error visible beside the plan action on narrow screens", async ({
  page,
}) => {
  const base = await fixture(page, ["failed"]);
  const host = { ...base.hosts[0], name: "long-host-".repeat(18) };
  const job = {
    id: "failed-restore",
    host_id: host.id,
    kind: "restore",
    state: "failed",
    result_id: "restore-plan",
    error: "Zielverbindung unterbrochen",
    created_at: "2026-10-15T12:00:00Z",
    attempts: 1,
  };
  const plan = {
    id: "restore-plan",
    backup_id: base.backups[0].id,
    target_id: host.id,
    created_at: "2026-10-15T12:00:00Z",
    scenario: "files",
    source: host.inventory,
    target: host.inventory,
    steps: [],
    manual: [],
    mapping: { interfaces: {}, storage: {} },
    state: "ready",
    blockers: [],
    tasks: [],
  };
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: { ...base, hosts: [host], jobs: [job], plans: [plan] },
    }),
  );
  await page.reload();
  await openJobs(page);
  const history = page.getByRole("table", { name: "Auftragsverlauf" });
  await expect(history).toContainText("Zielverbindung unterbrochen");
  await expect(
    history.getByRole("button", { name: "Wiederherstellungsplan prüfen" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 375, height: 812 });
  await page.getByRole("button", { name: "Navigation öffnen" }).click();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  await page.locator(".overview-history > summary").click();
  const recent = page.getByRole("list", { name: "Letzte Aufträge" });
  await expect(recent).toContainText("Zielverbindung unterbrochen");
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    375,
  );
});
