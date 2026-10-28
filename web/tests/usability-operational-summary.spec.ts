import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});
async function fixture(page: any) {
  const base = await (await page.request.get("/api/status")).json();
  const host = { ...base.hosts[0], enabled: true };
  const backups = [0, 1].map((n) => ({
    ...base.backups[0],
    host_id: host.id,
    host_name: host.name,
    id: `summary-${n}`,
    manifest_sha: `sha-${n}`,
    verification_error: "",
    status: "successful",
    created_at: new Date(Date.now() - (2 - n) * 3600000).toISOString(),
  }));
  await page.route("**/api/status", (r: any) =>
    r.fulfill({ json: { ...base, hosts: [host], backups, jobs: [] } }),
  );
  return { base, host, backups };
}
const row = (path: string, change = "changed", extra = {}) => ({
  path,
  change,
  content_changed: true,
  metadata_changed: false,
  mtime_only: false,
  ...extra,
});

test("summary classifies relative paths, separates timestamps and opens the exact source pair", async ({
  page,
}) => {
  const { host } = await fixture(page);
  await page.route("**/api/backups/compare", (r) =>
    r.fulfill({
      json: [
        row("etc/network/interfaces"),
        row("/etc/pve/storage.cfg", "added"),
        row("etc/ssh/sshd_config", "removed"),
        row("etc/custom.conf", "changed", {
          content_changed: false,
          metadata_changed: true,
        }),
        row("etc/fstab", "changed", {
          content_changed: false,
          mtime_only: true,
        }),
      ],
    }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  const summary = page.getByRole("region", {
    name: "Änderungszusammenfassung",
  });
  await expect(summary).toContainText("4 Dateien mit Änderungen");
  await expect(summary).toContainText(
    "1 hinzugefügt · 1 entfernt · 2 geändert · 1 nur Zeitstempel",
  );
  for (const area of ["Netzwerk", "Storage", "System", "Weitere Konfiguration"])
    await expect(
      summary.locator(".change-areas > span").filter({ hasText: area }),
    ).toContainText("1 Änderung");
  await expect(summary.getByText(/Vorher:/)).not.toBeVisible();
  await summary.getByText("Verglichene Stände", { exact: true }).click();
  await expect(summary).toContainText("summary-0");
  await summary.getByRole("button", { name: "Vergleich öffnen" }).click();
  await expect(page.getByLabel("Vergleich von", { exact: true })).toHaveValue(
    "summary-0",
  );
  await expect(page.getByLabel("Vergleich bis", { exact: true })).toHaveValue(
    "summary-1",
  );
});

test("failed comparison retries and never reports unchanged before success", async ({
  page,
}) => {
  const { host } = await fixture(page);
  let fail = true;
  await page.route("**/api/backups/compare", (r) =>
    fail
      ? r.fulfill({ status: 500, json: { error: "Quelle nicht lesbar" } })
      : r.fulfill({ json: [] }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  const summary = page.getByRole("region", {
    name: "Änderungszusammenfassung",
  });
  await expect(summary).toContainText("Vergleich nicht verfügbar");
  await expect(summary).not.toContainText("Keine wesentlichen Änderungen");
  fail = false;
  await summary.getByRole("button", { name: "Vergleich erneut laden" }).click();
  await expect(summary).toContainText("Keine wesentlichen Änderungen");
});

test("incomplete sources cannot produce a reassuring summary", async ({
  page,
}) => {
  const { base, host, backups } = await fixture(page);
  let calls = 0;
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        jobs: [],
        backups: [
          backups[0],
          { ...backups[1], verification_error: "hash mismatch" },
        ],
      },
    }),
  );
  await page.route("**/api/backups/compare", (r) => {
    calls++;
    return r.fulfill({ json: [] });
  });
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Änderungszusammenfassung" }),
  ).toContainText("Noch kein Vergleich möglich");
  expect(calls).toBe(0);
});

test("a large problem collection exposes three bounded searchable groups without hiding errors", async ({
  page,
}, info) => {
  const { base, host } = await fixture(page);
  const hosts = Array.from({ length: 101 }, (_, i) => ({
    ...host,
    id: `problem-${i}`,
    name: `problem-${String(i).padStart(3, "0")}`,
    enabled: i !== 100,
  }));
  const backups = hosts.slice(0, 2).map((h, i) => ({
    ...base.backups[0],
    host_id: h.id,
    id: `bad-${i}`,
    status: i ? "partial" : "damaged",
  }));
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts, backups, jobs: [] } }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  const region = page.getByRole("region", { name: "Jetzt wichtig" });
  await expect(region.locator(".problem-card")).toHaveCount(3);
  await expect(region.locator(".section-heading .count-badge")).toHaveText(
    "100",
  );
  await expect(
    region.getByRole("button", { name: /Host .* prüfen/ }),
  ).toHaveCount(6);
  await region.getByRole("textbox").fill("problem-099");
  await expect(
    region.getByRole("button", { name: /Host .* prüfen/ }),
  ).toHaveCount(3);
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.screenshot({
    path: info.outputPath("problem-cards-mobile.png"),
    fullPage: true,
  });
  await region.getByRole("button", { name: "Host problem-099 prüfen" }).click();
  await expect(
    page.getByRole("heading", { name: "problem-099", exact: true }),
  ).toBeVisible();
});

test("late comparison cannot replace another host summary", async ({
  page,
}) => {
  const { base, host, backups } = await fixture(page);
  const other = { ...host, id: "other", name: "other-host" };
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host, other],
        jobs: [],
        backups: [
          ...backups,
          ...backups.map((b) => ({
            ...b,
            id: `other-${b.id}`,
            host_id: other.id,
            host_name: other.name,
          })),
        ],
      },
    }),
  );
  let release!: () => void;
  const gate = new Promise<void>((r) => (release = r));
  await page.route("**/api/backups/compare", async (r) => {
    if (!r.request().postDataJSON().to.startsWith("other-")) await gate;
    await r.fulfill({
      json: r.request().postDataJSON().to.startsWith("other-")
        ? []
        : [row("etc/network/interfaces")],
    });
  });
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Änderungszusammenfassung" }),
  ).toContainText("Änderungen werden geprüft");
  await page.getByRole("button", { name: "Alle Hosts", exact: true }).click();
  await page.getByRole("button", { name: other.name, exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Änderungszusammenfassung" }),
  ).toContainText("Keine wesentlichen Änderungen");
  const late = page.waitForResponse(
    (r) =>
      r.url().endsWith("/api/backups/compare") &&
      !r.request().postDataJSON().to.startsWith("other-"),
  );
  release();
  await late;
  await expect(
    page.getByRole("region", { name: "Änderungszusammenfassung" }),
  ).not.toContainText("1 Datei mit Änderungen");
});

test("manifest refresh invalidates an older pending summary and remains readable on mobile", async ({
  page,
}, info) => {
  const { base, host, backups } = await fixture(page);
  let changed = false;
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        jobs: [],
        backups: backups.map((b) => ({
          ...b,
          manifest_sha: changed ? "replacement" : b.manifest_sha,
        })),
      },
    }),
  );
  let release!: () => void;
  const gate = new Promise<void>((r) => (release = r));
  let comparisons = 0;
  await page.route("**/api/backups/compare", async (r) => {
    const n = ++comparisons;
    if (n === 1) await gate;
    await r.fulfill({ json: n === 1 ? [row("etc/network/interfaces")] : [] });
  });
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await expect.poll(() => comparisons).toBe(1);
  changed = true;
  const summary = page.getByRole("region", {
    name: "Änderungszusammenfassung",
  });
  await expect(summary).toContainText("Keine wesentlichen Änderungen", {
    timeout: 10000,
  });
  const late = page.waitForResponse((r) =>
    r.url().endsWith("/api/backups/compare"),
  );
  release();
  await late;
  await expect(summary).not.toContainText("1 Datei mit Änderungen");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await summary.screenshot({
    path: info.outputPath("change-summary-mobile.png"),
  });
});
