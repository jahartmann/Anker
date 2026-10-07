import { demoCookies as loginDemo } from "./demo-auth";
import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";
import type { Job, Status } from "../src/api";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await loginDemo(request);
});

async function fixture(page: Page, includeActive = false) {
  await page.context().addCookies(cookies);
  const base = (await (await page.request.get("/api/status")).json()) as Status;
  const hosts = Array.from({ length: 40 }, (_, index) => ({
    ...base.hosts[0],
    id: "host-" + (index + 1),
    name: "pve-" + String(index + 1).padStart(2, "0"),
  }));
  const jobs: Job[] = Array.from({ length: 120 }, (_, index) => ({
    id: "history-" + index,
    host_id: hosts[index % 40].id,
    kind: index % 3 === 0 ? "probe" : "backup",
    state: index % 4 === 0 ? "failed" : "successful",
    created_at: new Date(Date.UTC(2026, 9, 1, 0, index)).toISOString(),
    started_at: new Date(Date.UTC(2026, 9, 1, 0, index, 2)).toISOString(),
    finished_at: new Date(Date.UTC(2026, 9, 1, 0, index, 32)).toISOString(),
    attempts: 1,
    trigger: "scheduled",
  }));
  if (includeActive) {
    jobs.push(
      ...Array.from({ length: 10 }, (_, index) => ({
        id: index < 4 ? "active-running-" + index : "active-queued-" + index,
        host_id: hosts[index].id,
        kind: "backup",
        state: index < 4 ? "running" : "queued",
        created_at: "2026-10-07T12:00:00Z",
        started_at: index < 4 ? "2026-10-07T12:01:00Z" : undefined,
        attempts: index < 4 ? 2 : 0,
        trigger: "manual",
      })),
    );
  }
  const status = { ...base, hosts, jobs };
  await page.route("**/api/status", (route) => route.fulfill({ json: status }));
  await page.route("**/api/jobs/*", (route) => {
    const id = route.request().url().split("/").pop();
    const job = jobs.find((value) => value.id === id);
    return route.fulfill({ json: job });
  });
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Aufträge", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Aufträge", exact: true }),
  ).toBeVisible();
  return status;
}

test("job history stays bounded across pages and combined filters reset the page", async ({
  page,
}) => {
  await fixture(page);
  const rows = page
    .getByRole("table", { name: "Auftragsverlauf" })
    .locator("tbody tr");
  await expect(rows).toHaveCount(15);
  await expect(
    page.getByRole("button", { name: "Auftrag history-119 ansehen" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Nächste Auftragsseite" }).click();
  await expect(rows).toHaveCount(15);
  await expect(
    page.getByRole("button", { name: "Auftrag history-119 ansehen" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Auftrag history-104 ansehen" }),
  ).toBeVisible();
  await page
    .getByRole("combobox", { name: "Host", exact: true })
    .selectOption("host-1");
  await expect(rows).toHaveCount(3);
  await expect(
    page.getByRole("navigation", { name: "Aufträge · Seiten" }),
  ).toContainText("1–3 von 3");
  await page
    .getByRole("combobox", { name: "Auftragstyp" })
    .selectOption("backup");
  await page
    .getByRole("combobox", { name: "Auftragsstatus" })
    .selectOption("failed");
  await expect(rows).toHaveCount(2);
  await expect(
    page.getByRole("button", { name: "Auftrag history-80 ansehen" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Auftrag history-40 ansehen" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Auftrag history-0 ansehen" }),
  ).toHaveCount(0);
});

test("job search reports no matches and clearing filters restores the first page", async ({
  page,
}) => {
  await fixture(page);
  await page.getByRole("button", { name: "Nächste Auftragsseite" }).click();
  await page
    .getByRole("searchbox", { name: "Aufträge suchen" })
    .fill("history-119");
  await expect(
    page.getByRole("table", { name: "Auftragsverlauf" }).locator("tbody tr"),
  ).toHaveCount(1);
  await expect(
    page.getByRole("button", { name: "Auftrag history-119 ansehen" }),
  ).toBeVisible();
  await page
    .getByRole("searchbox", { name: "Aufträge suchen" })
    .fill("unauffindbarer-host");
  await expect(
    page.getByRole("heading", { name: "Keine passenden Aufträge" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Filter zurücksetzen" }).click();
  await expect(
    page.getByRole("searchbox", { name: "Aufträge suchen" }),
  ).toHaveValue("");
  await expect(
    page.getByRole("table", { name: "Auftragsverlauf" }).locator("tbody tr"),
  ).toHaveCount(15);
  await expect(
    page.getByRole("button", { name: "Auftrag history-119 ansehen" }),
  ).toBeVisible();
});

test("active jobs distinguish queued work from running attempts and retain open details on completion", async ({
  page,
}) => {
  await page.clock.setFixedTime(new Date("2026-10-07T12:10:00Z"));
  const status = await fixture(page, true);
  const active = page.getByRole("region", {
    name: "Aktive Aufträge",
    exact: true,
  });
  await expect(active).toContainText("4 laufen");
  await expect(active).toContainText("6 warten");
  await expect(active.getByRole("listitem")).toHaveCount(4);
  const running = active
    .getByRole("listitem")
    .filter({
      has: page.getByRole("button", {
        name: "Auftrag active-running-0 ansehen",
      }),
    });
  await expect(running).toContainText("Versuch 2");
  await expect(running).toContainText("Laufzeit 9 Min.");
  await expect(running).toContainText("Gestartet");
  await expect(active.getByRole("progressbar")).toHaveCount(0);
  await active.getByRole("button", { name: "Nächste aktive Aufträge" }).click();
  await expect(active.getByRole("listitem")).toHaveCount(4);
  await expect(active).toContainText("Wartet auf Ausführung");
  await expect(active).not.toContainText("Laufzeit");
  await active
    .getByRole("button", { name: "Vorherige aktive Aufträge" })
    .click();
  await active
    .getByRole("button", { name: "Auftrag active-running-0 ansehen" })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("active-running-0");
  const job = status.jobs.find((value) => value.id === "active-running-0")!;
  job.state = "successful";
  job.finished_at = "2026-10-07T12:10:00Z";
  await expect(active).toContainText("3 laufen", { timeout: 10000 });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("active-running-0");
  await expect(
    dialog.getByRole("button", { name: "Abbrechen", exact: true }),
  ).toHaveCount(0);
});
