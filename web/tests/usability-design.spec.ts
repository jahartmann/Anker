import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import type { APIRequestContext } from "@playwright/test";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await demoCookies(request);
});

test("view actions remain reachable at narrow viewport", async ({ page }) => {
  await page.context().addCookies(cookies);
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator(".problem-card summary").first().click();
  const action = page.getByRole("button", { name: /^Host .* prüfen$/ }).first();
  await expect(action).toBeVisible();
  const hostName = (await action.getAttribute("aria-label"))!.slice(5, -7);
  const box = await action.boundingBox();
  expect(box!.x + box!.width).toBeLessThanOrEqual(390);
  await action.focus();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("heading", { name: hostName, exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Verbindung prüfen", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Inventar", exact: true }).click();
  await expect(
    page.getByRole("heading", {
      name: /Hostinventar|Noch kein Inventar|Inventarabfrage fehlgeschlagen/,
    }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});

test("inventory distinguishes failed query from absence", async ({ page }) => {
  await page.context().addCookies(cookies);
  const base = await (await page.request.get("/api/status")).json();
  const host = {
    ...base.hosts[0],
    inventory: {
      ...base.hosts[0].inventory,
      interfaces: [],
      disks: [],
      query_states: {
        interfaces: { state: "failed" },
        disks: { state: "ok" },
        zfs: { state: "not_applicable" },
      },
    },
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [host] } }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await page.getByRole("button", { name: "Inventar", exact: true }).click();
  await expect(
    page
      .getByRole("alert")
      .filter({ hasText: "Netzwerkabfrage fehlgeschlagen" }),
  ).toContainText("Keine vollständigen Daten verfügbar");
  await expect(
    page.getByRole("heading", { name: "Keine Schnittstellen erfasst" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Keine Datenträger erfasst" }),
  ).toBeVisible();
  await page.getByText("Erfasste Details", { exact: true }).click();
  await expect(
    page.getByText("ZFS-Pools · Nicht anwendbar", { exact: true }),
  ).toBeVisible();
});

test("browser identity is available before login and follows the current view", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page).toHaveTitle("Anmelden · Anker");
  const icon = await page.locator('link[rel="icon"]').getAttribute("href");
  expect(icon).toBeTruthy();
  const response = await page.request.get(icon!);
  expect(response.ok()).toBeTruthy();
  expect(response.headers()["content-type"]).toContain("image/svg+xml");
  expect(await response.text()).toContain("<svg");
  await page.context().addCookies(cookies);
  await page.reload();
  await expect(page).toHaveTitle("Hosts · Anker");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Alle Stände", exact: true }).click();
  await expect(page).toHaveTitle("Sicherungen · Anker");
});

test("overview distinguishes active, paused and unsaved hosts from live jobs", async ({
  page,
}) => {
  await page.context().addCookies(cookies);
  const base = await (await page.request.get("/api/status")).json();
  const hosts = base.hosts
    .slice(0, 4)
    .map((h: any, i: number) => ({ ...h, enabled: i !== 1 }));
  const successful = {
    ...base.backups[0],
    host_id: hosts[0].id,
    created_at: new Date().toISOString(),
    status: "successful",
  };
  const paused = { ...successful, id: "paused-backup", host_id: hosts[1].id };
  const jobs = ["running", "queued", "successful"].map((state, i) => ({
    id: "overview-job-" + i,
    host_id: hosts[i].id,
    kind: "probe",
    state,
    created_at: new Date().toISOString(),
    attempts: 1,
    error: "",
  }));
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: { ...base, hosts, backups: [successful, paused], jobs },
    }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  const summary = page.getByRole("region", { name: "Sicherungsstatus" });
  await expect(
    summary
      .getByText("Aktive Hosts", { exact: true })
      .locator("..")
      .locator("dd"),
  ).toHaveText("3");
  await expect(
    summary
      .getByText("Aktuell gesichert", { exact: true })
      .locator("..")
      .locator("dd"),
  ).toHaveText("1");
  await expect(
    summary
      .getByText("Handlungsbedarf", { exact: true })
      .locator("..")
      .locator("dd"),
  ).toHaveText("2");
  await expect(
    summary
      .getByText("Aktive Aufträge", { exact: true })
      .locator("..")
      .locator("dd"),
  ).toHaveText("2");
  await expect(summary).toContainText("1 Host pausiert");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(summary).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});

test("backup file actions remain fully visible on a short laptop viewport", async ({
  page,
}) => {
  await page.context().addCookies(cookies);
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Alle Stände", exact: true }).click();
  await page
    .getByRole("row")
    .filter({ hasText: "PVE · Hamburg 02" })
    .getByRole("button", { name: "Dateien", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "Dateien · pve-hamburg-02" });
  await expect(
    dialog.getByRole("button", { name: "Wiederherstellung planen" }),
  ).toBeEnabled();
  const frame = await dialog.boundingBox();
  const action = await dialog
    .getByRole("link", { name: "Stand herunterladen", exact: true })
    .boundingBox();
  expect(frame).toBeTruthy();
  expect(action).toBeTruthy();
  expect(action!.y + action!.height).toBeLessThanOrEqual(
    frame!.y + frame!.height - 1,
  );
  await dialog
    .getByRole("textbox", { name: "Dateien durchsuchen", exact: true })
    .fill("sysctl");
  await dialog
    .getByRole("button", { name: /^etc\/sysctl.d\/99-anker.conf/ })
    .click();
  await expect(
    dialog.getByRole("link", { name: "Datei herunterladen", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  const mobileFrame = await dialog.boundingBox();
  const mobileAction = await dialog
    .getByRole("button", { name: "Datei wiederherstellen" })
    .boundingBox();
  expect(mobileAction!.y + mobileAction!.height).toBeLessThanOrEqual(
    mobileFrame!.y + mobileFrame!.height - 1,
  );
});

test("overview host handoff is consumed after dismiss and navigation", async ({
  page,
}) => {
  await page.context().addCookies(cookies);
  await page.goto("/");
  const nav = page.getByRole("navigation", { name: "Hauptnavigation" });
  await nav.getByRole("button", { name: "Übersicht", exact: true }).click();
  await page.locator(".problem-card summary").first().click();
  await page
    .getByRole("button", { name: /^Host .* prüfen$/ })
    .first()
    .click();
  await page.locator(".back-button").click();
  await nav.getByRole("button", { name: "Aufträge", exact: true }).click();
  await nav.getByRole("button", { name: "Hosts", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".back-button")).toHaveCount(0);
});

test("actual public inventory query projection reaches host UI", async ({
  page,
}) => {
  await page.context().addCookies(cookies);
  const base = await (await page.request.get("/api/status")).json();
  const host = base.hosts.find((h: any) => h.inventory);
  expect(host.inventory.query_states?.interfaces).toEqual({ state: "ok" });
  expect(host.inventory.details).toBeUndefined();
  const hosts = await (await page.request.get("/api/hosts")).json();
  expect(
    hosts.find((h: any) => h.id === host.id).inventory.query_states,
  ).toEqual(host.inventory.query_states);
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await page.getByRole("button", { name: "Inventar", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Hostinventar", exact: true }),
  ).toBeVisible();
  await expect(page.getByText(/Abfragestatus nicht übermittelt/)).toHaveCount(
    0,
  );
});

test("action menu ignores a stale scroll event but closes after its anchor moves", async ({
  page,
}) => {
  await page.context().addCookies(cookies);
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Zugriff", exact: true }).click();
  const trigger = page.getByRole("button", {
    name: "Aktionen für demo",
    exact: true,
  });
  await trigger.click();
  const first = page.getByRole("button", {
    name: "Zugang bearbeiten",
    exact: true,
  });
  await expect(first).toBeFocused();
  await page.evaluate(() => document.dispatchEvent(new Event("scroll")));
  await expect(first).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await trigger.click();
  await expect(first).toBeVisible();
  // Narrowing the viewport closes the menu; an actual document scroll does too.
  await page.setViewportSize({ width: 390, height: 600 });
  await expect(first).toHaveCount(0);
  await trigger.scrollIntoViewIfNeeded();
  await trigger.click();
  await expect(first).toBeVisible();
  const before = await trigger.boundingBox();
  await page.evaluate(() => scrollTo(0, 0));
  await expect
    .poll(async () => (await trigger.boundingBox())!.y)
    .not.toBe(before!.y);
  await expect(first).toHaveCount(0);
});

test("unknown and omitted inventory evidence stay distinct from failed and successful empty queries", async ({
  page,
}) => {
  await page.context().addCookies(cookies);
  const base = await (await page.request.get("/api/status")).json();
  const host = {
    ...base.hosts[0],
    inventory: {
      ...base.hosts[0].inventory,
      interfaces: [],
      disks: [],
      query_states: {
        interfaces: { state: "unknown" },
        packages: { state: "unknown" },
        zfs: { state: "not_applicable" },
        storage: { state: "failed" },
      },
    },
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [host] } }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await page.getByRole("button", { name: "Inventar", exact: true }).click();
  await expect(
    page.getByText(/Netzwerk · Abfragestatus unbekannt/),
  ).toContainText("keine Abwesenheit");
  await expect(
    page.getByText(/Datenträger · Abfragestatus unbekannt/),
  ).toContainText("keine Abwesenheit");
  await expect(
    page.getByRole("heading", { name: "Keine Schnittstellen erfasst" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Keine Datenträger erfasst" }),
  ).toHaveCount(0);
  await page.getByText("Erfasste Details", { exact: true }).click();
  await expect(
    page.getByText(/Pakete · Abfragestatus unbekannt/),
  ).toBeVisible();
  await expect(
    page.getByText("Storage · Abfrage fehlgeschlagen", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("ZFS-Pools · Nicht anwendbar", { exact: true }),
  ).toBeVisible();
  for (const width of [1440, 1280, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await page.screenshot({
      path: `/tmp/anker-final-fix-evidence/inventory-${width}.png`,
      fullPage: true,
    });
  }
});

test("inventory topics preserve unfamiliar details and keep missing evidence with its subject", async ({
  page,
}) => {
  await page.context().addCookies(cookies);
  const base = await (await page.request.get("/api/status")).json();
  const host = {
    ...base.hosts[0],
    inventory: {
      ...base.hosts[0].inventory,
      details: {
        routes: [{ destination: "default", gateway: "192.0.2.1" }],
        packages: ["pve-manager"],
        guests: [{ name: "test-vm" }],
        future_inventory_field: { evidence: "retained" },
        file_hashes: { private: "hidden-internal-data" },
      },
      query_states: {
        ...base.hosts[0].inventory.query_states,
        routes: { state: "ok" },
        packages: { state: "ok" },
        guests: { state: "ok" },
        zfs: { state: "failed", error: "Poolabfrage fehlgeschlagen" },
        lvm: { state: "not_applicable" },
      },
    },
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [host] } }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await page.getByRole("button", { name: "Inventar", exact: true }).click();
  await page.getByText("Erfasste Details", { exact: true }).click();
  const network = page.getByRole("region", { name: "Netzwerk", exact: true });
  await network.getByText("Netzwerkrouten", { exact: true }).click();
  await expect(network).toContainText("192.0.2.1");
  await expect(
    page.getByRole("region", { name: "System und Software", exact: true }),
  ).toContainText("Pakete");
  await expect(
    page.getByRole("region", { name: "Gäste und Cluster", exact: true }),
  ).toContainText("Virtuelle Maschinen");
  const storage = page.getByRole("region", { name: "Speicher", exact: true });
  await expect(storage.getByRole("alert")).toContainText(
    "Poolabfrage fehlgeschlagen",
  );
  await expect(storage).toContainText("LVM · Nicht anwendbar");
  const other = page.getByRole("region", {
    name: "Weitere Angaben",
    exact: true,
  });
  await other.getByText("future_inventory_field", { exact: true }).click();
  await expect(other).toContainText("retained");
  await expect(page.locator(".inventory-source")).not.toContainText(
    "hidden-internal-data",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});
