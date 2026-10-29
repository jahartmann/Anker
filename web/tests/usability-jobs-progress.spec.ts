import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import type { Job, Status } from "../src/api";

async function setup(page: Page) {
  await page.context().addCookies(await demoCookies(page.request));
  const base = (await (await page.request.get("/api/status")).json()) as Status;
  const job: Job = {
    id: "phase-job",
    host_id: base.hosts[0].id,
    kind: "backup",
    state: "running",
    attempts: 1,
    revision: 12,
    created_at: new Date().toISOString(),
    started_at: new Date().toISOString(),
    stages: [
      {
        id: "collect",
        title: "Konfiguration erfassen",
        detail: "Hostdaten erfassen",
        state: "successful",
        attempt: 1,
        started_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        finished_at: new Date().toISOString(),
      },
      {
        id: "validate",
        title: "Dateien prüfen",
        detail: "Pfade, Metadaten und Inhaltsprüfsummen prüfen.",
        state: "running",
        attempt: 1,
        started_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        completed: 128,
        total: 512,
      },
    ],
  };
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, jobs: [job] } }),
  );
  await page.route("**/api/jobs/phase-job", (route) =>
    route.fulfill({ json: job }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Aufträge", exact: true })
    .click();
  return { job, base };
}
test("live phases show measured counts and follow revisions without inventing overall percentages", async ({
  page,
}) => {
  const { job } = await setup(page);
  const active = page.getByRole("region", {
    name: "Aktive Aufträge",
    exact: true,
  });
  await expect(
    active.getByRole("progressbar", { name: "Dateien prüfen" }),
  ).toHaveAttribute("aria-valuenow", "128");
  await expect(active).toContainText("128 / 512 Einträge geprüft");
  await active
    .getByRole("button", { name: "Auftrag phase-job ansehen" })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("region", { name: "Beobachteter Ablauf" }),
  ).toContainText("Konfiguration erfassen");
  await expect(dialog.getByText("Kennung und Zeitverlauf")).toBeVisible();
  await expect(
    dialog.getByRole("region", { name: "Zeitverlauf" }),
  ).not.toBeVisible();
  job.revision++;
  job.stages![1] = {
    ...job.stages![1],
    state: "successful",
    completed: 512,
    finished_at: new Date().toISOString(),
  };
  job.stages!.push({
    id: "dependencies",
    title: "Abdeckung prüfen",
    detail: "Pflichtpfade und Verweise abgleichen.",
    state: "running",
    attempt: 1,
    started_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  });
  await expect(
    dialog.getByRole("progressbar", { name: "Abdeckung prüfen" }),
  ).toBeVisible({ timeout: 10000 });
  await expect(dialog.getByRole("progressbar")).not.toHaveAttribute(
    "aria-valuenow",
  );
  await expect(
    dialog.getByRole("region", { name: "Beobachteter Ablauf" }),
  ).toContainText("512/512");
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
    const box = await dialog.boundingBox();
    expect(box!.width).toBeLessThanOrEqual(width);
    await page.screenshot({
      path: `/tmp/anker-job-execution-${width}.png`,
      animations: "disabled",
    });
  }
});
test("retry waiting pauses the bar and retains the failed attempt", async ({
  page,
}) => {
  const { job } = await setup(page);
  job.retry_at = new Date(Date.now() + 60000).toISOString();
  job.message = "Verbindung gestört. Derselbe Auftrag wird erneut versucht.";
  job.revision++;
  job.stages![1].state = "failed";
  const active = page.getByRole("region", {
    name: "Aktive Aufträge",
    exact: true,
  });
  await expect(active).toContainText("Nächster Versuch", { timeout: 10000 });
  await expect(active.getByRole("progressbar")).toHaveCount(0);
  await active
    .getByRole("button", { name: "Auftrag phase-job ansehen" })
    .click();
  await expect(page.getByRole("dialog")).toContainText("Nicht abgeschlossen");
  await expect(
    page
      .getByRole("dialog")
      .getByRole("button", { name: "Auftrag abbrechen", exact: true }),
  ).toBeEnabled();
});
test("hosts and latest backups expose larger persistent page sizes without losing filtered entries", async ({
  page,
}) => {
  const { base } = await setup(page);
  const hosts = Array.from({ length: 36 }, (_, i) => ({
    ...base.hosts[0],
    id: `large-host-${i}`,
    name: `pve-site-${i}`,
    address: `192.0.2.${i + 1}`,
  }));
  const backups = hosts.map((host, i) => ({
    ...base.backups[0],
    id: `large-backup-${i}`,
    host_id: host.id,
    host_name: host.name,
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: { ...base, hosts, backups, jobs: [], references: {} },
    }),
  );
  await page.reload();
  const nav = page.getByRole("navigation", { name: "Hauptnavigation" });
  await nav.getByRole("button", { name: "Hosts", exact: true }).click();
  await page
    .getByRole("combobox", { name: "Hosts pro Seite", exact: true })
    .selectOption("50");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(36);
  await page.getByLabel("Hosts durchsuchen").fill("pve-site-3");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(7);
  await page.reload();
  await nav.getByRole("button", { name: "Hosts", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "Hosts pro Seite", exact: true }),
  ).toHaveValue("50");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(36);
  await nav.getByRole("button", { name: "Sicherungen", exact: true }).click();
  await page
    .getByRole("combobox", { name: "Letzte Sicherungen pro Seite" })
    .selectOption("50");
  await expect(page.locator(".backup-collection")).toHaveCount(36);
  await page.reload();
  await nav.getByRole("button", { name: "Sicherungen", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "Letzte Sicherungen pro Seite" }),
  ).toHaveValue("50");
  await expect(page.locator(".backup-collection")).toHaveCount(36);
});

test("month navigation and slice sizes agree so no older backups disappear", async ({
  page,
}) => {
  const { base } = await setup(page);
  const host = base.hosts[0];
  const backups = Array.from({ length: 17 }, (_, index) => ({
    ...base.backups[0],
    id: `month-${index}`,
    host_id: host.id,
    host_name: host.name,
    created_at: new Date(Date.UTC(2024, index, 12)).toISOString(),
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: { ...base, hosts: [host], backups, jobs: [], references: {} },
    }),
  );
  await page.reload();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.locator(".backup-collection .history-toggle").click();
  const months = page.locator(".history-month > summary");
  await expect(months).toHaveCount(12);
  const first = await months.allTextContents();
  const navigation = page.getByRole("navigation", {
    name: "Monate · Seiten",
    exact: true,
  });
  await navigation.getByRole("button", { name: "Nächste Seite" }).click();
  await expect(months).toHaveCount(4);
  expect(new Set([...first, ...(await months.allTextContents())]).size).toBe(
    16,
  );
  await navigation
    .getByRole("combobox", { name: "Monate pro Seite" })
    .selectOption("25");
  await expect(months).toHaveCount(16);
});

test("execution desk keeps live work first and history readable at desktop and phone widths", async ({
  page,
}) => {
  const { base, job } = await setup(page);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const jobs = [
    job,
    {
      ...job,
      id: "queued-job",
      state: "queued",
      started_at: undefined,
      stages: undefined,
      attempts: 0,
    },
    {
      ...job,
      id: "done-job",
      state: "successful",
      finished_at: new Date().toISOString(),
      result_id: base.backups[0].id,
      stages: undefined,
    },
    {
      ...job,
      id: "failed-job",
      state: "failed",
      error: "Hostverbindung prüfen",
      stages: undefined,
    },
  ];
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, jobs } }),
  );
  await page.reload();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Aufträge", exact: true })
    .click();
  await expect(
    page.getByRole("table", { name: "Auftragsverlauf" }).locator("thead th"),
  ).toHaveCount(4);
  const live = page.getByRole("region", {
    name: "Aktive Aufträge",
    exact: true,
  });
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
    const liveBox = await live.boundingBox();
    const filterBox = await page.locator(".jobs-toolbar").boundingBox();
    expect(liveBox!.y).toBeLessThan(filterBox!.y);
    await page.screenshot({
      path: `/tmp/anker-job-desk-${width}.png`,
      fullPage: true,
      animations: "disabled",
    });
  }
  await live.getByRole("button", { name: "Auftrag phase-job ansehen" }).click();
  let cancellations = 0;
  await page.route("**/api/jobs/phase-job/cancel", (route) => {
    cancellations++;
    return route.fulfill({ json: { ok: true } });
  });
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Schließen", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(cancellations).toBe(0);
});
