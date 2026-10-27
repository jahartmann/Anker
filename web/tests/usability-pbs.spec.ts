import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test("PBS shows configuration inventory and opens configuration-only guided recovery", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const host = {
    ...base.hosts[0],
    name: "pbs-config-test",
    inventory: {
      ...base.hosts[0].inventory,
      hostname: "pbs-config-test",
      host_type: "pbs",
      pve_version: "",
      pbs_version: "4.0.1-1",
      cluster_id: "",
      pbs_datastores: [{ name: "main", path: "/mnt/datastore/main" }],
      pbs_jobs: [
        {
          kind: "Sync",
          name: "nightly",
          store: "main",
          schedule: "daily",
          disabled: false,
        },
      ],
      query_states: {
        interfaces: { state: "ok" },
        disks: { state: "ok" },
        pbs_datastores: { state: "ok" },
        pbs_sync_jobs: { state: "ok" },
        pbs_prune_jobs: { state: "ok" },
        pbs_verify_jobs: { state: "ok" },
      },
    },
  };
  const backups = base.backups
    .filter((b: { host_id: string }) => b.host_id === host.id)
    .map((b: object) => ({ ...b, host_type: "pbs" }));
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [host], backups, jobs: [] } }),
  );
  await page.route("**/api/backups/compare", (r) => r.fulfill({ json: [] }));
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Wiederherstellung planen", exact: true }),
  ).toBeEnabled();
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Inventar", exact: true })
    .click();
  await expect(
    page.getByText("Proxmox Backup Server", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("4.0.1-1", { exact: true })).toBeVisible();
  const config = page.getByRole("region", {
    name: "PBS-Konfiguration",
    exact: true,
  });
  await expect(config).toContainText("/mnt/datastore/main");
  await expect(config).toContainText("nightly");
  await expect(config).toContainText(
    "Datastore-Inhalte werden separat gesichert",
  );
  await expect(
    page.locator(".inventory-summary").getByText("Cluster", { exact: true }),
  ).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    config.getByRole("heading", { name: "PBS-Konfiguration", exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth + 1,
    ),
  ).toBeTruthy();
  await page.setViewportSize({ width: 1280, height: 900 });
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  const backupID = backups[0].id;
  const entries = [
    "etc/proxmox-backup/remote.cfg",
    "etc/proxmox-backup/datastore.cfg",
    "etc/hosts",
  ].map((path) => ({
    path,
    type: "file",
    mode: 384,
    uid: 0,
    gid: 0,
    size: 20,
    secret: true,
  }));
  await page.route(`**/api/backups/${backupID}/files`, (r) =>
    r.fulfill({ json: entries }),
  );
  await page
    .getByRole("button", { name: "Wiederherstellung planen", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "PBS-Wiederherstellung", exact: true }),
  ).toContainText("systemctl stop proxmox-backup-proxy proxmox-backup");
  await expect(
    page.getByLabel("Situation", { exact: true }).locator("option"),
  ).toHaveCount(1);
  await page
    .getByRole("button", {
      name: "Gesamte PBS-Konfiguration auswählen",
      exact: true,
    })
    .click();
  await expect(
    page.getByText("Dateien auswählen · 2 ausgewählt", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page.getByLabel("Zielhost", { exact: true }).selectOption(host.id);
  await expect(
    page.getByText(/PBS-Dienste müssen gestoppt sein/),
  ).toBeVisible();
  await page.route("**/api/plans/inspect", (r) =>
    r.fulfill({
      json: {
        source: host.inventory,
        target: host.inventory,
        ports: [],
        target_ports: [],
        storage: [],
        target_storage: [],
        blockers: [
          "Vor Zielprüfung und Übernahme auf dem PBS-Ziel proxmox-backup und proxmox-backup-proxy stoppen",
        ],
        manual: [],
        warnings: [],
        files: [],
        comparison_complete: false,
        automatic: false,
      },
    }),
  );
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  for (const path of entries
    .filter((e) => e.path.startsWith("etc/proxmox-backup/"))
    .map((e) => e.path)) {
    await expect(
      page.getByLabel("Entscheidung für " + path, { exact: true }),
    ).toBeVisible();
  }
  await expect(
    page.getByLabel("Entscheidung für etc/hosts", { exact: true }),
  ).toHaveCount(0);
});
