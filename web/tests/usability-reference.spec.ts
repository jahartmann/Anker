import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test("named reference can be edited and compared directly with its host stand", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const host = { ...base.hosts[0], id: "reference-host", name: "Node A" };
  const backups = [
    {
      ...base.backups[0],
      id: "older",
      host_id: host.id,
      host_name: host.name,
      created_at: "2026-10-01T10:00:00Z",
    },
    {
      ...base.backups[0],
      id: "newer",
      host_id: host.id,
      host_name: host.name,
      created_at: "2026-10-02T10:00:00Z",
    },
  ];
  const reference = {
    host_id: host.id,
    backup_id: "older",
    manifest_sha: "abc",
    name: "Vor Umbau",
    reason: "Geprüft",
    state: "valid",
    updated_at: "2026-10-09T10:00:00Z",
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        backups,
        jobs: [],
        references: { [host.id]: reference },
      },
    }),
  );
  await page.route("**/api/hosts/reference-host/reference", (r) =>
    r.fulfill({ json: reference }),
  );
  await page.route(
    "**/api/hosts/reference-host/reference/compare?to=newer",
    (r) =>
      r.fulfill({
        json: { reference, to_id: "newer", changed: 1, mtime_only: 1 },
      }),
  );
  let saved: Record<string, unknown> | undefined;
  await page.route("**/api/hosts/reference-host/reference", async (r) => {
    if (r.request().method() === "PUT") saved = r.request().postDataJSON();
    await r.fulfill({ json: reference });
  });
  await page.route("**/api/backups/compare", (r) => r.fulfill({ json: [] }));
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(page.getByText("Referenz: Vor Umbau")).toBeVisible();
  await page.getByRole("button", { name: "Referenz bearbeiten" }).click();
  await page.getByLabel("Referenzname").fill("Vor Migration");
  await page.getByLabel("Datum der Probe").fill("2026-10-09T10:00:00Z");
  await page.getByLabel("Beleg", { exact: true }).fill("Laborprotokoll 17");
  await page.getByRole("button", { name: "Referenz speichern" }).click();
  expect(saved).toMatchObject({
    backup_id: "older",
    name: "Vor Migration",
    drill: {
      tested_at: "2026-10-09T10:00:00Z",
      outcome: "passed",
      evidence: "Laborprotokoll 17",
    },
  });
  await page.getByRole("button", { name: "Mit Referenz vergleichen" }).click();
  await expect(page.getByLabel("Vergleich von")).toHaveValue("older");
  await expect(page.getByLabel("Vergleich bis")).toHaveValue("newer");
});

test("unresolved reference is visible and cannot start direct comparison", async ({
  page,
  request,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const host = { ...base.hosts[0], id: "reference-host", name: "Node A" };
  const backup = {
    ...base.backups[0],
    id: "newer",
    host_id: host.id,
    host_name: host.name,
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        backups: [backup],
        jobs: [],
        references: {
          [host.id]: {
            host_id: host.id,
            backup_id: "deleted",
            name: "Verlorene Basis",
            state: "unresolved",
          },
        },
      },
    }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: "Navigation öffnen" }).click();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(
    page.getByText("Referenz: Verlorene Basis · nicht auflösbar"),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Mit Referenz vergleichen" }),
  ).toHaveCount(0);
});

test("changed human testimony refreshes only focused readiness and hides stale response", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const source = {
    ...base.backups[0],
    id: "testimony-source",
    evidence_revision: "first",
  };
  let changed = false;
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        backups: [
          { ...source, evidence_revision: changed ? "second" : "first" },
        ],
        jobs: [],
      },
    }),
  );
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let calls = 0;
  await page.route("**/api/backups/testimony-source/readiness", async (r) => {
    calls++;
    if (changed) await gate;
    await r.fulfill({
      json: {
        version: 1,
        backup_id: source.id,
        items: [
          {
            id: "offline_drill",
            state: changed ? "unknown" : "ok",
            title: "Offline-Probe",
            detail: changed
              ? "Bericht entfernt"
              : "Menschlicher Bericht vorhanden",
          },
        ],
        coverage: [],
        dependencies: [],
      },
    });
  });
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Dateien" }).first().click();
  await page.getByText("Quellnachweise und Abdeckung ansehen").click();
  const readiness = page.getByRole("region", { name: "Quellnachweise" });
  await expect(readiness).toContainText("Menschlicher Bericht vorhanden");
  changed = true;
  await expect(readiness).not.toContainText("Menschlicher Bericht vorhanden", {
    timeout: 10000,
  });
  release();
  await expect(readiness).toContainText("Bericht entfernt");
  expect(calls).toBe(2);
});

test("changing reference comparison target drops an older summary response", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const host = { ...base.hosts[0], id: "reference-host", name: "Node A" };
  const backups = ["old", "first", "second"].map((id, i) => ({
    ...base.backups[0],
    id,
    host_id: host.id,
    host_name: host.name,
    created_at: `2026-10-0${i + 1}T10:00:00Z`,
  }));
  const reference = {
    host_id: host.id,
    backup_id: "old",
    name: "Basis",
    state: "valid",
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        backups,
        jobs: [],
        references: { [host.id]: reference },
      },
    }),
  );
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(
    "**/api/hosts/reference-host/reference/compare?to=second",
    async (r) => {
      await gate;
      await r.fulfill({
        json: {
          reference,
          to_id: "second",
          changed: 1,
          content: 1,
          metadata: 0,
          mtime_only: 0,
        },
      });
    },
  );
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Mit Referenz vergleichen" }).click();
  await page.getByLabel("Vergleichsstand für Node A").selectOption("first");
  const response = page.waitForResponse((r) =>
    r.url().includes("/reference/compare?to=second"),
  );
  release();
  await response;
  await expect(
    page.getByRole("heading", { name: "Sicherungsvergleich" }),
  ).toHaveCount(0);
});
