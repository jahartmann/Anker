import { test, expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import { displayHostName } from "../src/hostNames";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});
async function setup(page: Page) {
  const base = await (await page.request.get("/api/status")).json();
  const host = { ...base.hosts[0], name: "pve-berlin-01", enabled: true };
  const other = { ...host, id: "other-node", name: "pve-hamburg-02" };
  const backups = Array.from({ length: 11 }, (_, i) => ({
    ...base.backups[0],
    id: `node-stand-${i}`,
    host_id: host.id,
    host_name: host.name,
    status: "successful",
    verification_error: "",
    created_at: new Date(Date.now() - (i + 1) * 3600000).toISOString(),
  }));
  const others = backups.slice(0, 2).map((b) => ({
    ...b,
    id: `other-${b.id}`,
    host_id: other.id,
    host_name: other.name,
  }));
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host, other],
        backups: [...others, ...backups].reverse(),
        jobs: [],
      },
    }),
  );
  return { base, host, other, backups };
}
const diff = (path = "etc/network/interfaces") => ({
  path,
  change: "changed",
  content_changed: true,
  metadata_changed: false,
  mtime_only: false,
  secret: false,
});

test("host names have a friendly presentation while the technical identity stays exact", async ({
  page,
}) => {
  expect(displayHostName("pve-berlin-01")).toBe("PVE · Berlin 01");
  expect(displayHostName("pbs-backup_02")).toBe("PBS · Backup 02");
  expect(displayHostName("Mein Server")).toBe("Mein Server");
  expect(displayHostName("proxmox1")).toBe("Proxmox1");
  const { host } = await setup(page);
  await page.route("**/api/backups/compare", (r) => r.fulfill({ json: [] }));
  await page.goto("/");
  const button = page.getByRole("button", { name: host.name, exact: true });
  await expect(button).toHaveText("PVE · Berlin 01");
  await button.click();
  await expect(
    page.getByRole("heading", { name: host.name, exact: true }),
  ).toHaveText("PVE · Berlin 01");
  await expect(
    page.locator(".page-heading p").filter({ hasText: host.name }),
  ).toContainText(host.name);
});

test("timeline is lazy, bounded, same-node and opens the exact historical pair", async ({
  page,
}, info) => {
  const { host } = await setup(page);
  const requested: { from: string; to: string }[] = [];
  await page.route("**/api/backups/compare", (r) => {
    requested.push(r.request().postDataJSON());
    return r.fulfill({ json: [diff()] });
  });
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await expect.poll(() => requested.length).toBe(1);
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Änderungen", exact: true })
    .click();
  const timeline = page.locator(".change-timeline");
  await expect(timeline).not.toHaveAttribute("open", "");
  await expect(timeline.locator(".timeline-event")).toHaveCount(0);
  await timeline.locator(":scope > summary").click();
  await expect(timeline.locator(".timeline-event")).toHaveCount(4);
  await expect(
    timeline.getByRole("button", { name: /Änderungen zum Stand/ }),
  ).toHaveCount(4);
  expect(
    requested.every(
      (r) => r.from.startsWith("node-stand-") && r.to.startsWith("node-stand-"),
    ),
  ).toBeTruthy();
  await timeline
    .getByRole("button", { name: "Änderungen zum Stand node-stand-2 öffnen" })
    .click();
  await expect(page.getByLabel("Vergleich von", { exact: true })).toHaveValue(
    "node-stand-3",
  );
  await expect(page.getByLabel("Vergleich bis", { exact: true })).toHaveValue(
    "node-stand-2",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("button", { name: "Zurück zum Hostverlauf", exact: true })
    .click();
  await expect(page.locator(".timeline-event")).toHaveCount(4);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page
    .locator(".change-timeline")
    .screenshot({ path: info.outputPath("node-timeline-mobile.png") });
});

test("timeline failures are retryable and incomplete captures never claim no changes", async ({
  page,
}) => {
  const { base, host, backups } = await setup(page);
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        jobs: [],
        backups: [{ ...backups[0], status: "partial" }, ...backups.slice(1)],
      },
    }),
  );
  let fail = true;
  await page.route("**/api/backups/compare", (r) =>
    fail
      ? r.fulfill({ status: 500, json: { error: "Manifest nicht lesbar" } })
      : r.fulfill({ json: [] }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Änderungen", exact: true })
    .click();
  await page.locator(".change-timeline > summary").click();
  const events = page.locator(".timeline-event");
  await expect(events.first()).toContainText("Vergleich nicht belastbar");
  await expect(events.nth(1)).toContainText("Vergleich nicht verfügbar");
  await expect(events.nth(1)).not.toContainText(
    "Keine wesentlichen Änderungen",
  );
  fail = false;
  await events
    .nth(1)
    .getByRole("button", { name: "Diesen Vergleich erneut laden" })
    .click();
  await expect(events.nth(1)).toContainText("Keine wesentlichen Änderungen");
});

test("node selector compares only its latest two captures and area cards filter without file requests", async ({
  page,
}, info) => {
  const { host } = await setup(page);
  const requested: any[] = [];
  let files = 0;
  await page.route("**/api/backups/compare", (r) => {
    requested.push(r.request().postDataJSON());
    return r.fulfill({ json: [diff(), diff("etc/pve/storage.cfg")] });
  });
  await page.route("**/api/backups/compare/file", (r) => {
    files++;
    return r.fulfill({ json: {} });
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Alle Stände", exact: true }).click();
  await page
    .getByRole("button", { name: "Stände vergleichen", exact: true })
    .click();
  await page
    .getByLabel("Node im Vergleich", { exact: true })
    .selectOption(host.id);
  await expect(page.getByLabel("Vergleich von", { exact: true })).toHaveValue(
    "node-stand-1",
  );
  await expect(page.getByLabel("Vergleich bis", { exact: true })).toHaveValue(
    "node-stand-0",
  );
  // The focus card and the detail view both read the same host-bound pair.
  expect(requested.length).toBeGreaterThan(0);
  for (const pair of requested)
    expect(pair).toEqual({ from: "node-stand-1", to: "node-stand-0" });
  await page
    .getByRole("button", { name: "Netzwerk 1 Änderung", exact: true })
    .click();
  await expect(page.locator(".comparison-items .file-entry")).toHaveCount(1);
  await expect(page.locator(".comparison-items")).toContainText(
    "etc/network/interfaces",
  );
  expect(files).toBe(0);
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page
    .locator(".comparison-area-cards")
    .screenshot({ path: info.outputPath("comparison-areas-mobile.png") });
});

test("a newer backoff revision reaches job details during the same attempt", async ({
  page,
}) => {
  const { base, host } = await setup(page);
  let waiting = false;
  const old = {
    id: "retry-ui",
    host_id: host.id,
    kind: "backup",
    state: "running",
    created_at: new Date().toISOString(),
    started_at: new Date().toISOString(),
    attempts: 1,
    max_attempts: 3,
    revision: 3,
    message: "Versuch läuft.",
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        backups: [],
        jobs: [
          waiting
            ? {
                ...old,
                revision: 4,
                message: "Verbindung vorübergehend gestört. Gleicher Auftrag.",
                retry_at: new Date(Date.now() + 10000).toISOString(),
              }
            : old,
        ],
      },
    }),
  );
  await page.route("**/api/jobs/retry-ui", (r) => r.fulfill({ json: old }));
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Aufträge", exact: true })
    .click();
  await page.getByRole("button", { name: "Auftrag retry-ui ansehen" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Versuch läuft.");
  waiting = true;
  await expect(dialog).toContainText("Gleicher Auftrag.", { timeout: 10000 });
  await expect(dialog).toContainText("Nächster Versuch");
});
