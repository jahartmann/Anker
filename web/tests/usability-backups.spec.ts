import { demoCookies as loginDemo } from "./demo-auth";
import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";
import type { Entry } from "../src/api";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await loginDemo(request);
});
test.beforeEach(async ({ page }) => {
  await page.context().addCookies(cookies);
});

const entry = (path: string, type = "file", secret = false): Entry => ({
  path,
  type,
  secret,
  mode: 420,
  uid: 0,
  gid: 0,
  size: type === "directory" ? 0 : 120,
});
async function openFiles(page: Page, files: Entry[]) {
  await page.route("**/api/backups/*/files", (route) =>
    route.fulfill({ json: files }),
  );
  await page.route("**/api/backups/*/file?*", (route) => {
    const path = new URL(route.request().url()).searchParams.get("path")!;
    const selected = files.find((file) => file.path === path)!;
    return route.fulfill({
      json: {
        entry: selected,
        content: selected.link || `Content of ${path}`,
        masked: selected.secret && !route.request().url().includes("reveal=1"),
      },
    });
  });
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
}

test("large file collections use folders, bounded pages and recursive path search", async ({
  page,
}) => {
  const files = [
    entry("etc", "directory"),
    entry("etc/empty", "directory"),
    entry("etc/conf", "directory"),
    entry("etc/current", "symlink"),
    ...Array.from({ length: 1600 }, (_, i) =>
      entry(`etc/conf/config-${String(i).padStart(4, "0")}.conf`),
    ),
  ];
  files[3].link = "conf";
  await openFiles(page, files);
  const dialog = page.getByRole("dialog");
  await expect(dialog.locator(".file-items > button")).toHaveCount(1);
  await dialog
    .getByRole("button", { name: "Ordner etc öffnen", exact: true })
    .click();
  await expect(dialog.locator(".file-items > button")).toHaveCount(3);
  await dialog
    .getByRole("button", { name: "Ordner etc/empty öffnen", exact: true })
    .click();
  await expect(
    dialog.getByText("Dieser Ordner ist leer", { exact: true }),
  ).toBeVisible();
  await dialog
    .getByRole("navigation", { name: "Dateipfad" })
    .getByRole("button", { name: "etc", exact: true })
    .click();
  await dialog.getByRole("button", { name: /^etc\/current/ }).click();
  await expect(dialog.locator(".file-preview")).toContainText("conf");
  await expect(
    dialog.getByRole("navigation", { name: "Dateipfad" }),
  ).toContainText("etc");
  await dialog
    .getByRole("button", { name: "Ordner etc/conf öffnen", exact: true })
    .click();
  await expect(dialog.locator(".file-items > button")).toHaveCount(25);
  await dialog
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  await expect(
    dialog.getByRole("button", { name: /^etc\/conf\/config-0025.conf/ }),
  ).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: /^etc\/conf\/config-0000.conf/ }),
  ).toHaveCount(0);
  await dialog.getByLabel("Einträge pro Seite").selectOption("50");
  await expect(dialog.locator(".file-items > button")).toHaveCount(50);
  await dialog
    .getByRole("navigation", { name: "Dateipfad" })
    .getByRole("button", { name: "Alle Dateien", exact: true })
    .click();
  await dialog.getByPlaceholder("Pfad suchen").fill("config-1599");
  await expect(dialog.locator(".file-items > button")).toHaveCount(1);
  await dialog
    .getByRole("button", { name: /^etc\/conf\/config-1599.conf/ })
    .click();
  await expect(dialog.locator(".file-preview pre")).toContainText(
    "Content of etc/conf/config-1599.conf",
  );
  await dialog.getByPlaceholder("Pfad suchen").fill("missing-entry");
  await expect(
    dialog.getByText("Keine passenden Dateien", { exact: true }),
  ).toBeVisible();
});

test("backup overview replaces pages and resets pagination after search and protection filters", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const template = base.backups[0];
  const backups = Array.from({ length: 130 }, (_, i) => ({
    ...template,
    id: `paged-${i}`,
    host_id: `host-${i}`,
    host_name: `Server ${String(i).padStart(3, "0")}`,
    created_at: new Date(Date.UTC(2026, 9, 1, 0, i)).toISOString(),
    pinned: i === 0,
    archived: i === 1,
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, backups, jobs: [] } }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(25);
  await expect(page.locator(".backup-table tbody tr").first()).toContainText(
    "Server 129",
  );
  await page
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(25);
  await expect(page.locator(".backup-table tbody tr").first()).toContainText(
    "Server 104",
  );
  await page.getByLabel("Sicherungen durchsuchen").fill("Server 000");
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(1);
  await expect(page.locator(".backup-table")).toContainText("Server 000");
  await page.getByLabel("Sicherungen durchsuchen").fill("");
  await page.getByLabel("Schutz und Archiv filtern").selectOption("pinned");
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(1);
  await expect(page.locator(".backup-table")).toContainText("Server 000");
  await page.getByLabel("Schutz und Archiv filtern").selectOption("archived");
  await expect(page.locator(".backup-table")).toContainText("Server 001");
  await page.getByLabel("Sicherungen durchsuchen").fill("no-server");
  await expect(
    page.getByText("Keine passenden Sicherungen", { exact: true }),
  ).toBeVisible();
});

test("inventory tables stay compact while searches and disclosed source details remain available", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const host = {
    ...base.hosts[0],
    inventory: {
      ...base.hosts[0].inventory,
      hostname: "inventory-node",
      pve_version: "9.0",
      debian: "13",
      kernel: "6.14-test",
      boot_mode: "UEFI",
      cluster_id: "test-cluster",
      quorate: false,
      interfaces: Array.from({ length: 80 }, (_, i) => ({
        name: `port-${String(i).padStart(2, "0")}`,
        mac: `02:00:00:00:00:${String(i).padStart(2, "0")}`,
        pci: `pci-${i}`,
      })),
      disks: Array.from({ length: 70 }, (_, i) => ({
        name: `disk-${String(i).padStart(2, "0")}`,
        id: `stable-${i}`,
        size: 1024,
        uuid: `uuid-${i}`,
        mount: i === 69 ? "/archive" : "",
      })),
      details: { storage: { pool: "actual-pool", state: "ONLINE" } },
    },
  };
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, hosts: [host] } }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: "Öffnen", exact: true }).click();
  await page.getByRole("button", { name: "Inventar", exact: true }).click();
  await expect(page.getByText("6.14-test", { exact: true })).toBeVisible();
  await expect(page.getByText("UEFI", { exact: true })).toBeVisible();
  await expect(page.getByText("Quorum fehlt", { exact: true })).toBeVisible();
  await expect(page.locator(".inventory-network tbody tr")).toHaveCount(25);
  await expect(page.locator(".inventory-disks tbody tr")).toHaveCount(25);
  await page.getByLabel("Netzwerk durchsuchen").fill("port-79");
  await expect(page.locator(".inventory-network tbody tr")).toHaveCount(1);
  await expect(page.locator(".inventory-network")).toContainText("port-79");
  await page.getByLabel("Datenträger durchsuchen").fill("/archive");
  await expect(page.locator(".inventory-disks tbody tr")).toHaveCount(1);
  await expect(page.locator(".inventory-disks")).toContainText("disk-69");
  await page
    .getByRole("button", { name: "Details zu disk-69", exact: true })
    .click();
  await expect(page.getByText("uuid-69", { exact: true })).toBeVisible();
  await expect(
    page.getByText("actual-pool", { exact: false }),
  ).not.toBeVisible();
  await page.getByText("Erfasste Details", { exact: true }).click();
  await page.getByText("Storage", { exact: true }).click();
  await expect(page.locator(".inventory-raw")).toContainText("actual-pool");
});
