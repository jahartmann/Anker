import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import { changeHighlights } from "../src/changeHighlights";
import type { FileDiff } from "../src/api";
const row = (path: string, extra: Partial<FileDiff> = {}): FileDiff => ({
  path,
  change: "changed",
  secret: false,
  content_changed: true,
  metadata_changed: false,
  mtime_changed: false,
  mtime_only: false,
  ...extra,
});
const diffs = [
  row("etc/pve/storage.cfg"),
  row("etc/pve/nodes/pve-berlin-01/qemu-server/100.conf", { change: "added" }),
  row("etc/sysctl.d/99-anker.conf"),
  row("etc/network/interfaces", {
    content_changed: false,
    metadata_changed: true,
  }),
  row("etc/fstab", { change: "removed" }),
  row("etc/ssh/private-key", { secret: true }),
];
test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});
async function setup(page: any) {
  const base = await (await page.request.get("/api/status")).json();
  const host = { ...base.hosts[0], name: "pve-berlin-01", enabled: true };
  const backups = Array.from({ length: 12 }, (_, n) => ({
    ...base.backups[0],
    id: `detail-${n}`,
    host_id: host.id,
    host_name: host.name,
    status: "successful",
    verification_error: "",
    created_at: new Date(Date.now() - (n + 1) * 3600000).toISOString(),
  }));
  await page.route("**/api/status", (r: any) =>
    r.fulfill({ json: { ...base, hosts: [host], backups, jobs: [] } }),
  );
  await page.route("**/api/backups/compare", (r: any) =>
    r.fulfill({ json: diffs }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  return { base, host, backups };
}
test("highlights explain captured subjects, keep metadata distinct and never expose secret filenames", () => {
  const highlights = changeHighlights([
    ...diffs,
    row("etc/sysctl.d/10-other.conf"),
    row("etc/resolv.conf", { content_changed: false, mtime_only: true }),
  ]);
  expect(highlights).toContainEqual({
    text: "Storage-Anbindungen geändert",
    count: 1,
    kind: "changed",
  });
  expect(highlights).toContainEqual({
    text: "VM 100: Konfiguration hinzugefügt",
    count: 1,
    kind: "added",
  });
  expect(highlights).toContainEqual({
    text: "Kernel- und Netzwerkparameter geändert",
    count: 2,
    kind: "changed",
  });
  expect(highlights).toContainEqual({
    text: "Netzwerkdefinition: Metadaten geändert",
    count: 1,
    kind: "metadata",
  });
  expect(JSON.stringify(highlights)).not.toContain("private-key");
  expect(JSON.stringify(highlights)).not.toContain("DNS-Konfiguration");
});
test("timeline cards explain at most three changes without reading file contents", async ({
  page,
}) => {
  await setup(page);
  let contents = 0;
  await page.route("**/api/backups/compare/file", (r: any) => {
    contents++;
    return r.fulfill({ json: {} });
  });
  await page.locator(".host-tabs").getByRole("button", { name: "Änderungen", exact: true }).click();
  await page.locator(".change-timeline > summary").click();
  const card = page.locator(".timeline-event").first();
  await expect(card.getByRole("listitem")).toHaveCount(3);
  await expect(card).toContainText("Storage-Anbindungen geändert");
  await expect(card).toContainText("VM 100: Konfiguration hinzugefügt");
  await expect(card).toContainText("Kernel- und Netzwerkparameter geändert");
  await expect(card).toContainText("3 weitere Dateien im Detailvergleich");
  expect(contents).toBe(0);
});
test("dedicated detail page is wider and returning restores timeline page and focus", async ({
  page,
}, info) => {
  await setup(page);
  await page.locator(".host-tabs").getByRole("button", { name: "Änderungen", exact: true }).click();
  await page.locator(".change-timeline > summary").click();
  const timeline = page.locator(".change-timeline");
  await timeline
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  const entry = timeline.getByRole("button", {
    name: "Änderungen zum Stand detail-5 öffnen",
    exact: true,
  });
  await entry.click();
  const detail = page.getByRole("region", {
    name: "Änderungsdetails der Node",
  });
  await expect(detail).toBeVisible();
  await expect(
    detail.getByRole("heading", { name: "pve-berlin-01", exact: true }),
  ).toBeFocused();
  await expect(
    detail.getByRole("button", {
      name: "Änderung ansehen: etc/network/interfaces",
      exact: true,
    }),
  ).toContainText("Dateieigenschaften wurden aktualisiert.");
  await expect(detail.getByLabel("Vergleich von", { exact: true })).toHaveValue(
    "detail-6",
  );
  await expect(detail.getByLabel("Vergleich bis", { exact: true })).toHaveValue(
    "detail-5",
  );
  await expect(
    detail.getByLabel("Vergleich von", { exact: true }),
  ).not.toBeVisible();
  const width = await detail.evaluate(
    (e: any) => e.getBoundingClientRect().width,
  );
  expect(width).toBeGreaterThan(1050);
  await page.screenshot({
    path: info.outputPath("node-change-detail.png"),
    fullPage: true,
  });
  await detail
    .getByRole("button", { name: "Zurück zum Hostverlauf", exact: true })
    .click();
  await expect(detail).toHaveCount(0);
  await expect(timeline).toHaveAttribute("open", "");
  await expect(entry).toBeFocused();
  await expect(timeline).toContainText("5–8");
});
test("detail comparison keeps file reading explicit and remains usable on a phone", async ({
  page,
}, info) => {
  await setup(page);
  let files = 0;
  await page.route("**/api/backups/compare/file", (r: any) => {
    files++;
    return r.fulfill({
      json: {
        path: "etc/pve/storage.cfg",
        state: "different",
        content_changed: true,
        masked: false,
        metadata_changes: [],
        hunks: [
          {
            before_start: 1,
            after_start: 1,
            lines: [
              { kind: "remove", text: "old\n", before_line: 1 },
              { kind: "add", text: "new\n", after_line: 1 },
            ],
          },
        ],
        before_label: "Vorher",
        after_label: "Nachher",
      },
    });
  });
  await page
    .getByRole("button", { name: "Änderungen ansehen", exact: true })
    .click();
  const detail = page.getByRole("region", {
    name: "Änderungsdetails der Node",
  });
  await expect(
    detail.getByRole("region", { name: "Was sich geändert hat" }),
  ).toBeVisible();
  expect(files).toBe(0);
  const restoreTools = detail.locator(".comparison-restore-tools");
  await expect(restoreTools).not.toHaveAttribute("open", "");
  await expect(
    restoreTools.getByRole("button", {
      name: "Ausgangsstand zur Wiederherstellung wählen",
      exact: true,
    }),
  ).not.toBeVisible();
  await restoreTools.locator("summary").click();
  await expect(
    restoreTools.getByRole("button", {
      name: "Ausgangsstand zur Wiederherstellung wählen",
      exact: true,
    }),
  ).toBeVisible();
  await restoreTools.locator("summary").click();
  await expect(
    detail.locator(".comparison-list .comparison-filters"),
  ).toBeVisible();
  await expect(
    detail.locator(".comparison-preview .comparison-pane-heading"),
  ).toContainText("Dateidetails");

  await detail
    .getByRole("button", {
      name: "Änderung ansehen: etc/pve/storage.cfg",
      exact: true,
    })
    .click();
  await expect(detail.locator(".comparison-preview")).toBeFocused();
  await expect(
    detail.getByRole("region", { name: "Was im Inhalt geändert wurde" }),
  ).toBeVisible();
  await expect(detail.locator(".full-line-diff")).not.toHaveAttribute(
    "open",
    "",
  );
  await detail.locator(".full-line-diff > summary").click();
  await expect(
    detail.getByRole("region", { name: "Zeilenvergleich" }),
  ).toBeVisible();
  expect(files).toBe(1);
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await detail.screenshot({
    path: info.outputPath("node-change-detail-mobile.png"),
  });
  await detail
    .getByText("Andere Stände dieser Node vergleichen", { exact: true })
    .click();
  await expect(
    detail.getByLabel("Vergleich von", { exact: true }),
  ).toBeVisible();
});

test("collection comparisons use a separate page and preserve the expanded collection on return", async ({
  page,
}) => {
  const { host } = await setup(page);
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const collection = page.getByRole("region", {
    name: "Sicherungen von " + host.name,
  });
  await collection
    .getByRole("button", { name: "Verlauf für " + host.name, exact: true })
    .click();
  await collection
    .locator(".backup-table tbody tr")
    .first()
    .getByRole("button", { name: /^Aktionen für/ })
    .click();
  await page
    .getByRole("button", {
      name: "Mit vorherigem Stand vergleichen",
      exact: true,
    })
    .click();
  const detail = page.locator(".comparison-detail-page");
  await expect(detail).toBeVisible();
  await expect(collection).toBeHidden();
  await expect(detail.getByLabel("Vergleich von", { exact: true })).toHaveValue(
    "detail-1",
  );
  await expect(detail.getByLabel("Vergleich bis", { exact: true })).toHaveValue(
    "detail-0",
  );
  await detail
    .getByRole("button", { name: "Zurück zu Sicherungen", exact: true })
    .click();
  await expect(collection).toBeVisible();
  await expect(
    collection.getByRole("button", {
      name: "Verlauf für " + host.name,
      exact: true,
    }),
  ).toHaveAttribute("aria-expanded", "true");
});
