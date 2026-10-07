import { test, expect } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { readFile } from "node:fs/promises";
let demoCookies: Awaited<
  ReturnType<APIRequestContext["storageState"]>
>["cookies"];
test.beforeAll(async ({ request }) => {
  const response = await request.post("/api/login", {
    headers: { "X-Anker-Request": "1" },
    data: { name: "demo", password: "anker-demo-2026" },
  });
  expect(response.ok()).toBeTruthy();
  demoCookies = (await request.storageState()).cookies;
});
test.beforeEach(async ({ page }) => {
  await page.context().addCookies(demoCookies);
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
});
test("storage settings keep demo isolated", async ({ page }) => {
  await page
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Speicher", exact: true }).click();
  await expect(
    page.getByText(/Speicherverwaltung ist in der Demo ausgeschaltet/),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Erweitern", exact: true }),
  ).toHaveCount(0);
});

const storageVolume = {
  id: "anker-volume",
  mount: "/srv/anker",
  paths: ["/srv/anker"],
  source: "/dev/vdb1",
  fs_type: "ext4",
  total: 214748364800,
  used: 150323855360,
  available: 60129542144,
  reserved: 4294967296,
  used_percent: 71.4,
  inodes: 1000000,
  inodes_used: 650000,
  is_data: true,
  is_system: false,
  read_only: false,
  history: [
    {
      at: "2026-10-01T12:00:00Z",
      total: 214748364800,
      used: 144955146240,
      available: 65498251264,
    },
    {
      at: "2026-10-06T12:00:00Z",
      total: 214748364800,
      used: 150323855360,
      available: 60129542144,
    },
  ],
  forecast: {
    status: "growing",
    message: "Schätzung bei gleichbleibendem Nettozuwachs.",
    growth_per_day: 1073741824,
    days_to_full: 56,
    full_at: "2026-12-01T12:00:00Z",
    based_on_days: 5,
  },
};

test("storage shows consumption and requires a fresh confirmed growth plan", async ({
  page,
}) => {
  let changed = false,
    growthAttempts = 0;
  await page.route("**/api/storage", (route) =>
    route.fulfill({
      json: {
        environment: "vm:kvm",
        collected_at: "2026-10-06T12:00:00Z",
        data_path: "/srv/anker",
        demo: false,
        volumes: [storageVolume],
        devices: [],
        warnings: [],
      },
    }),
  );
  await page.route("**/api/storage/state", (route) =>
    route.fulfill({
      json: {
        status: changed ? "successful" : "idle",
        mount: "/srv/anker",
        message: changed ? "Dateisystem erweitert und neue Größe geprüft." : "",
        after_bytes: changed ? 429496729600 : 0,
      },
    }),
  );
  await page.route("**/api/storage/plan", (route) =>
    route.fulfill({
      json: {
        id: "verified-plan",
        volume_id: "anker-volume",
        mount: "/srv/anker",
        source: "/dev/vdb1",
        fs_type: "ext4",
        environment: "vm:kvm",
        device_bytes: 429496729600,
        filesystem_bytes: 214748364800,
        can_grow: true,
        message: "Zusätzlicher Platz ist zugewiesen.",
        steps: [
          {
            title: "Dateisystem erweitern",
            command: "sudo resize2fs '/dev/vdb1'",
            explanation: "Bereits zugewiesenen Platz übernehmen.",
          },
        ],
      },
    }),
  );
  await page.route("**/api/storage/grow", (route) => {
    expect(route.request().postDataJSON()).toEqual({
      volume_id: "anker-volume",
      plan_id: "verified-plan",
      confirmation: "/srv/anker",
    });
    growthAttempts++;
    if (growthAttempts === 1)
      return route.fulfill({
        status: 409,
        json: { error: "Plan hat sich verändert; erneut prüfen" },
      });
    changed = true;
    return route.fulfill({
      status: 202,
      json: { status: "running", mount: "/srv/anker" },
    });
  });
  await page
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Speicher", exact: true }).click();
  await expect(
    page.getByRole("progressbar", { name: "Belegung /srv/anker" }),
  ).toHaveAttribute("aria-valuenow", "71");
  await expect(page.getByText(/56 Tage/)).toBeVisible();
  await expect(
    page.getByRole("img", { name: /Belegungsverlauf/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Erweitern", exact: true }).click();
  const dialog = page.getByRole("dialog");
  const submit = dialog.getByRole("button", {
    name: "Platz übernehmen",
    exact: true,
  });
  await expect(submit).toBeDisabled();
  await dialog.getByLabel("Mountpoint bestätigen").fill("/dev/vdb1");
  await expect(submit).toBeDisabled();
  await dialog.getByLabel("Mountpoint bestätigen").fill("/srv/anker");
  await submit.click();
  await expect(dialog.getByRole("alert")).toContainText(
    "Plan hat sich verändert",
  );
  await expect(dialog).toBeVisible();
  await expect(submit).toBeDisabled();
  await dialog.getByRole("button", { name: "Plan erneut prüfen" }).click();
  await dialog.getByLabel("Mountpoint bestätigen").fill("/srv/anker");
  await submit.click();
  await expect(
    page.getByText("Dateisystem erweitert und neue Größe geprüft."),
  ).toBeVisible({ timeout: 8000 });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
});

test("LXC storage assistant creates only an explicit host resize command", async ({
  page,
}) => {
  await page.route("**/api/storage", (route) =>
    route.fulfill({
      json: {
        environment: "container:lxc",
        collected_at: "2026-10-06T12:00:00Z",
        data_path: "/srv/anker",
        volumes: [storageVolume],
        devices: [],
        warnings: [],
      },
    }),
  );
  await page.route("**/api/storage/state", (route) =>
    route.fulfill({ json: { status: "idle" } }),
  );
  await page.route("**/api/storage/plan", (route) =>
    route.fulfill({
      json: {
        volume_id: "anker-volume",
        mount: "/srv/anker",
        environment: "container:lxc",
        can_grow: false,
        message: "Auf dem Proxmox-Host erweitern.",
        steps: [],
      },
    }),
  );
  await page
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Speicher", exact: true }).click();
  await page.getByRole("button", { name: "Erweitern", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Container-ID").fill("123");
  await dialog.getByLabel("Mountpoint bei Proxmox").selectOption("mp0");
  await dialog.getByLabel("Zusätzlicher Platz in GiB").fill("50");
  await expect(
    dialog.getByText("pct resize 123 mp0 +50G", { exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "Platz übernehmen", exact: true }),
  ).toHaveCount(0);
});

test("storage retries unavailable reports and keeps the last values on refresh failure", async ({
  page,
}) => {
  let available = false;
  await page.route("**/api/storage", (route) =>
    available
      ? route.fulfill({
          json: {
            environment: "vm:kvm",
            collected_at: "2026-10-06T12:00:00Z",
            volumes: [storageVolume],
            devices: [],
            warnings: [],
          },
        })
      : route.fulfill({
          status: 503,
          json: { error: "Mount nicht erreichbar" },
        }),
  );
  await page.route("**/api/storage/state", (route) =>
    route.fulfill({
      status: 503,
      json: { error: "Speicherwerkzeuge nicht erreichbar" },
    }),
  );
  await page.route("**/api/storage/plan", (route) =>
    route.fulfill({
      status: 503,
      json: { error: "Speicherwerkzeuge nicht erreichbar" },
    }),
  );
  await page
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Speicher", exact: true }).click();
  await expect(
    page.getByText("Mount nicht erreichbar", { exact: true }),
  ).toBeVisible();
  available = true;
  await page.getByRole("button", { name: "Erneut laden", exact: true }).click();
  await expect(
    page.getByRole("progressbar", { name: "Belegung /srv/anker" }),
  ).toHaveAttribute("aria-valuenow", "71");
  await expect(
    page.getByText("Speicherwerkzeuge nicht erreichbar", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Erweitern", exact: true }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Speicherwerkzeuge nicht erreichbar",
  );
  await expect(
    page.getByRole("button", { name: "Platz übernehmen" }),
  ).toHaveCount(0);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Schließen", exact: true })
    .click();
  available = false;
  await page.getByRole("button", { name: "Belegung aktualisieren" }).click();
  await expect(
    page.getByText(/angezeigten Werte stammen vom letzten erfolgreichen Abruf/),
  ).toBeVisible();
  await expect(
    page.getByRole("progressbar", { name: "Belegung /srv/anker" }),
  ).toHaveAttribute("aria-valuenow", "71");
});

test("storage polling failure keeps an accepted operation running and blocks duplicates", async ({
  page,
}) => {
  let requests = 0;
  await page.route("**/api/storage", (route) =>
    route.fulfill({
      json: {
        environment: "vm:kvm",
        collected_at: "2026-10-06T12:00:00Z",
        volumes: [storageVolume],
        devices: [],
        warnings: [],
      },
    }),
  );
  await page.route("**/api/storage/state", (route) =>
    ++requests === 1
      ? route.fulfill({ json: { status: "running", mount: "/srv/anker" } })
      : route.fulfill({
          status: 503,
          json: { error: "Verbindung unterbrochen" },
        }),
  );
  await page
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Speicher", exact: true }).click();
  await expect(
    page.getByText(
      /Status nicht erreichbar; die Erweiterung kann weiterlaufen/,
    ),
  ).toBeVisible({ timeout: 8000 });
  await expect(
    page.getByRole("button", { name: "Erweitern", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Neues Laufwerk einbinden", exact: true }),
  ).toBeDisabled();
});

test("new backup drive offers a downloadable manual migration with mount checks", async ({
  page,
}) => {
  await page.route("**/api/storage", (route) =>
    route.fulfill({
      json: {
        environment: "vm:kvm",
        collected_at: "2026-10-06T12:00:00Z",
        volumes: [storageVolume],
        devices: [
          {
            name: "/dev/vdc1",
            type: "part",
            size: 429496729600,
            fstype: "ext4",
            uuid: "fixture-123",
            mountpoints: [null],
          },
        ],
        warnings: [],
      },
    }),
  );
  await page.route("**/api/storage/state", (route) =>
    route.fulfill({ json: { status: "idle" } }),
  );
  await page
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Speicher", exact: true }).click();
  await page
    .getByRole("button", { name: "Neues Laufwerk einbinden", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Noch nicht eingebundenes Gerät")).toHaveValue(
    "/dev/vdc1",
  );
  const downloaded = page.waitForEvent("download");
  await dialog
    .getByRole("button", { name: "Anleitung herunterladen", exact: true })
    .click();
  const path = await (await downloaded).path();
  const guide = await readFile(path!, "utf8");
  expect(guide).toContain("rsync -aHAX --numeric-ids");
  expect(guide).toContain("findmnt -n -o UUID --mountpoint /srv/anker");
  expect(guide).toContain("sudo umount /srv/anker");
  expect(guide).toContain("lost+found");
  expect(guide).not.toContain("mkfs");
});

test("host search and backup files work", async ({ page }) => {
  await page.getByPlaceholder("Hosts durchsuchen").fill("berlin");
  await expect(page.getByRole("row")).toHaveCount(4);
  await page.getByPlaceholder("Hosts durchsuchen").fill("");
  await page
    .getByRole("button", { name: "Öffnen", exact: true })
    .first()
    .click();
  await page
    .getByRole("main")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: /^etc\/network\/interfaces/ }).click();
  await expect(
    page.getByText("bridge-ports eno1", { exact: false }),
  ).toBeVisible();
});

test("selected file carries into a fresh restore request", async ({ page }) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: /^etc\/sysctl.d\/99-anker.conf/ })
    .click();
  await page
    .getByRole("button", { name: "Datei wiederherstellen", exact: true })
    .click();
  await expect(
    page.getByRole("checkbox", {
      name: "etc/sysctl.d/99-anker.conf",
      exact: true,
    }),
  ).toBeChecked();
  await expect(
    page.getByRole("checkbox", { name: "etc/hostname", exact: true }),
  ).not.toBeChecked();
});

test("file list failures can be retried without closing the dialog", async ({
  page,
}) => {
  let fail = true;
  await page.route("**/api/backups/*/files", (route) =>
    fail
      ? route.fulfill({
          status: 503,
          json: { error: "Ablage vorübergehend nicht erreichbar" },
        })
      : route.continue(),
  );
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Ablage vorübergehend nicht erreichbar",
  );
  fail = false;
  await page.getByRole("button", { name: "Erneut laden", exact: true }).click();
  await expect(
    page.getByRole("button", { name: /^etc\/hostname/ }),
  ).toBeVisible();
  await page.getByPlaceholder("Pfad suchen").fill("no-match");
  await expect(
    page.getByText("Keine passenden Dateien", { exact: true }),
  ).toBeVisible();
});

test("preview failure retains selected file and supports retry", async ({
  page,
}) => {
  let fail = true;
  await page.route("**/api/backups/*/file?*", (route) =>
    fail
      ? route.fulfill({
          status: 503,
          json: { error: "Datei momentan nicht lesbar" },
        })
      : route.continue(),
  );
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: /^etc\/hostname/ }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Datei momentan nicht lesbar",
  );
  await expect(
    page.getByRole("link", { name: "Datei herunterladen", exact: true }),
  ).toBeVisible();
  fail = false;
  await page.getByRole("button", { name: "Erneut lesen", exact: true }).click();
  await expect(page.locator(".file-preview pre")).toBeVisible();
});

test("comparison cannot retain the same source and destination", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const { backups } = await (await page.request.get("/api/status")).json();
  await page.getByLabel("Vergleich von").selectOption(backups[0].id);
  await page.getByLabel("Vergleich bis").selectOption(backups[1].id);
  await page.getByLabel("Vergleich von").selectOption(backups[1].id);
  await expect(
    page.getByRole("button", { name: "Vergleichen", exact: true }),
  ).toBeDisabled();
});

test("last backup actions stay visible and close with Escape", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const trigger = page
    .locator(".action-menu > summary, .action-menu > button")
    .last();
  await trigger.click();
  const archive = page.getByRole("button", {
    name: "Archivieren",
    exact: true,
  });
  await expect(archive).toBeInViewport();
  const visible = await archive.evaluate((el) => {
    const rect = el.getBoundingClientRect();
    return el.contains(
      document.elementFromPoint(
        rect.x + rect.width / 2,
        rect.y + rect.height / 2,
      ),
    );
  });
  expect(visible).toBe(true);
  await page.keyboard.press("Escape");
  await expect(archive).not.toBeVisible();
  await expect(trigger).toBeFocused();
});

test("mobile backup actions require no sideways scrolling", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Navigation öffnen" }).click();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const files = page
    .getByRole("button", { name: "Dateien", exact: true })
    .first();
  await expect(files).toBeInViewport();
  expect(
    await page
      .locator(".table-scroll")
      .first()
      .evaluate((el) => el.scrollWidth <= el.clientWidth),
  ).toBe(true);
});

test("settings load failure offers retry and changes have a visible saved state", async ({
  page,
}) => {
  let fail = true;
  await page.route("**/api/settings", (route) =>
    fail
      ? route.fulfill({
          status: 503,
          json: { error: "Einstellungen momentan nicht verfügbar" },
        })
      : route.continue(),
  );
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Einstellungen momentan nicht verfügbar",
  );
  fail = false;
  await page.getByRole("button", { name: "Erneut laden", exact: true }).click();
  await page.getByLabel("Tagesstände").fill("37");
  await expect(
    page.getByText("Ungespeicherte Änderungen", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Einstellungen speichern" }).click();
  await expect(
    page.getByText("Alle Änderungen gespeichert", { exact: true }),
  ).toBeVisible();
});

test("an overview with paused hosts does not claim they are secured", async ({
  page,
}) => {
  await page.route("**/api/status", async (route) => {
    const response = await route.fetch();
    const status = await response.json();
    status.hosts.forEach(
      (host: { enabled: boolean }) => (host.enabled = false),
    );
    await route.fulfill({ response, json: status });
  });
  await page.reload();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "Automatische Sicherung pausiert",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", {
      name: "Alle Hosts sind aktuell gesichert",
      exact: true,
    }),
  ).not.toBeVisible();
});

test("settings drafts stay intact when navigating away is cancelled", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByLabel("Tagesstände").fill("39");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Änderungen noch nicht gespeichert" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Weiter bearbeiten", exact: true })
    .click();
  await expect(page.getByLabel("Tagesstände")).toHaveValue("39");
});

test("a running plan request cannot be dismissed or submitted twice", async ({
  page,
}) => {
  let calls = 0;
  let finish!: () => void;
  const gate = new Promise<void>((resolve) => (finish = resolve));
  await page.route("**/api/plans", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    calls++;
    await gate;
    await route.fulfill({
      status: 503,
      json: { error: "Ziel für Prüfung nicht erreichbar" },
    });
  });
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  const status = await (await page.request.get("/api/status")).json();
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(status.backups[0].id);
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(status.hosts[0].id);
  await page
    .getByRole("checkbox", { name: "etc/sysctl.d/99-anker.conf", exact: true })
    .check();
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Dialog schließen", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeVisible();
  expect(calls).toBe(1);
  finish();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Ziel für Prüfung nicht erreichbar",
  );
});

test("logout network failure stays visible without losing the session", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.route("**/api/logout", (route) => route.abort("connectionfailed"));
  await page.getByRole("button", { name: "Abmelden", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Verbindung");
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
test("host form validates and creates real record", async ({ page }) => {
  await page.getByRole("button", { name: "Host hinzufügen" }).click();
  await page.getByLabel("Hostname", { exact: true }).fill("pve-browser-test");
  await page.getByLabel("Adresse", { exact: true }).fill("192.0.2.90");
  await page.getByRole("button", { name: "Host speichern" }).click();
  await expect(
    page.getByText("pve-browser-test", { exact: true }),
  ).toBeVisible();
  const hosts = await (await page.request.get("/api/hosts")).json();
  expect(
    hosts.find((h: { name: string }) => h.name === "pve-browser-test"),
  ).toMatchObject({
    key_path: "/etc/anker/keys/backup",
    known_hosts_path: "/etc/anker/known_hosts",
  });
});
test("settings save and mobile navigation", async ({ page }) => {
  await page
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Einstellungen", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Tagesstände").fill("35");
  await page.getByRole("button", { name: "Einstellungen speichern" }).click();
  await expect(page.getByText("Einstellungen gespeichert")).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Navigation öffnen" }).click();
  await page.getByRole("button", { name: "Hosts", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  await expect(page.locator("body")).toHaveJSProperty("scrollWidth", 390);
});

test("secrets stay masked until explicitly revealed and export downloads", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: /^etc\/pve\/priv\/storage\/demo.pw/ })
    .click();
  await expect(
    page.getByRole("heading", { name: "Geschützter Inhalt" }),
  ).toBeVisible();
  await expect(
    page.getByText("demo-secret-not-a-production-password", { exact: false }),
  ).not.toBeVisible();
  await page.getByRole("button", { name: "Inhalt anzeigen" }).click();
  await expect(
    page.getByText("demo-secret-not-a-production-password", { exact: false }),
  ).toBeVisible();
  const downloading = page.waitForEvent("download");
  await page.getByRole("link", { name: "Stand herunterladen" }).click();
  const download = await downloading;
  expect(await download.failure()).toBeNull();
  expect(download.suggestedFilename()).toMatch(/^anker-.*\.tar$/);
  const fileDownload = page.waitForEvent("download");
  await page
    .getByRole("link", { name: "Datei herunterladen", exact: true })
    .click();
  const file = await fileDownload;
  expect(await file.failure()).toBeNull();
  expect((await readFile((await file.path())!)).toString()).toContain(
    "demo-secret-not-a-production-password",
  );
});
test("file restore plans and executes only after exact confirmation", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  const status = await (await page.request.get("/api/status")).json();
  const b = status.backups.find(
    (b: { host_name: string }) => b.host_name === "pve-berlin-01",
  );
  await page.getByLabel("Sicherung", { exact: true }).selectOption(b.id);
  await page.getByLabel("Zielhost", { exact: true }).selectOption(b.host_id);
  await page.getByLabel("etc/sysctl.d/99-anker.conf", { exact: true }).check();
  await page.getByRole("button", { name: "Plan prüfen" }).click();
  await expect(
    page.getByRole("dialog", { name: "Wiederherstellungsplan", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "In Demo anwenden" }),
  ).toBeDisabled();
  const planDownload = page.waitForEvent("download");
  await page
    .getByRole("link", { name: "Plan herunterladen", exact: true })
    .click();
  const planFile = await planDownload;
  expect(await planFile.failure()).toBeNull();
  expect(planFile.suggestedFilename()).toMatch(/^anker-plan-.*\.tar$/);
  expect((await readFile((await planFile.path())!)).toString()).toContain(
    "prepared-files/etc/sysctl.d/99-anker.conf",
  );
  const updated = await (await page.request.get("/api/status")).json();
  const plan = updated.plans.find(
    (p: { backup_id: string }) => p.backup_id === b.id,
  );
  await page.getByLabel("Plan-ID zur Ausführung eingeben").fill(plan.id);
  await page.getByRole("button", { name: "In Demo anwenden" }).click();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Aufträge", exact: true })
    .click();
  await expect(page.getByText("Abgeschlossen", { exact: true })).toBeVisible();
});

test("backup queue and changed-file comparison use the service", async ({
  page,
}) => {
  await page
    .getByRole("button", { name: "pve-berlin-01", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Jetzt sichern", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("Sicherung gestartet");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Aufträge", exact: true })
    .click();
  await expect(
    page.getByText("Abgeschlossen", { exact: true }).first(),
  ).toBeVisible();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const state = await (await page.request.get("/api/status")).json();
  const backups = state.backups
    .filter((b: { host_name: string }) => b.host_name === "pve-berlin-01")
    .sort((a: { created_at: string }, b: { created_at: string }) =>
      a.created_at.localeCompare(b.created_at),
    );
  await page.getByLabel("Vergleich von").selectOption(backups[0].id);
  await page.getByLabel("Vergleich bis").selectOption(backups[1].id);
  await page.getByRole("button", { name: "Vergleichen", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText(
    "etc/sysctl.d/99-anker.conf",
  );
});

test("removing a test host requires its name and keeps backups", async ({
  page,
}) => {
  const before = await (await page.request.get("/api/status")).json();
  await page
    .getByRole("button", { name: "pve-browser-test", exact: true })
    .click();
  await page
    .getByRole("main")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Host entfernen", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Host entfernen",
    exact: true,
  });
  await expect(
    dialog.getByRole("button", { name: "Host entfernen", exact: true }),
  ).toBeDisabled();
  await dialog.getByLabel("Hostnamen bestätigen").fill("pve-browser-test");
  await dialog
    .getByRole("button", { name: "Host entfernen", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "pve-browser-test", exact: true }),
  ).toHaveCount(0);
  const after = await (await page.request.get("/api/status")).json();
  expect(after.backups.length).toBe(before.backups.length);
});

test("backup list downloads a complete recovery archive directly", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  const downloading = page.waitForEvent("download");
  await page
    .getByRole("link", { name: "Herunterladen", exact: true })
    .first()
    .click();
  const file = await downloading;
  expect(await file.failure()).toBeNull();
  const data = await readFile((await file.path())!);
  expect(data.toString()).toContain("original-files/etc/network/interfaces");
  expect(data.toString()).toContain("WIEDERHERSTELLUNG.md");
  await expect(
    page.getByRole("heading", { name: "Sicherungen", exact: true }),
  ).toBeVisible();
});
test("failed downloads keep the user in the interface and show the reason", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.route("**/download?check=1", (route) =>
    route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({ error: "Keine Exportberechtigung" }),
    }),
  );
  await page
    .getByRole("link", { name: "Herunterladen", exact: true })
    .first()
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "Keine Exportberechtigung",
  );
  await expect(
    page.getByRole("heading", { name: "Sicherungen", exact: true }),
  ).toBeVisible();
  expect(new URL(page.url()).pathname).toBe("/");
});
test("late file responses never overwrite the newly selected file", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  let release!: () => void, seen!: () => void;
  const gate = new Promise<void>((r) => (release = r)),
    started = new Promise<void>((r) => (seen = r));
  await page.route("**/file?path=etc%2Fnetwork%2Finterfaces", async (route) => {
    const response = await route.fetch();
    seen();
    await gate;
    await route.fulfill({ response });
  });
  const late = page.waitForResponse((r) =>
    r.url().includes("file?path=etc%2Fnetwork%2Finterfaces"),
  );
  await page.getByRole("button", { name: /^etc\/network\/interfaces/ }).click();
  await started;
  await page
    .getByRole("button", { name: /^etc\/sysctl.d\/99-anker.conf/ })
    .click();
  await expect(page.locator(".file-preview strong")).toHaveText(
    "etc/sysctl.d/99-anker.conf",
  );
  release();
  await late;
  await expect(page.locator(".file-preview strong")).toHaveText(
    "etc/sysctl.d/99-anker.conf",
  );
});
test("connection failures show persistent stale-data notice", async ({
  page,
}) => {
  await page.route("**/api/status", (route) =>
    route.abort("connectionrefused"),
  );
  await expect(page.getByRole("alert")).toContainText(
    "Anker ist nicht erreichbar",
    { timeout: 10000 },
  );
  await expect(page.getByRole("alert")).toContainText("Angezeigte Daten");
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
});
test("an expired session returns to login immediately during a download", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  await page.route("**/download?check=1", (route) =>
    route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ error: "Anmeldung erforderlich" }),
    }),
  );
  await page
    .getByRole("link", { name: "Stand herunterladen", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Anmelden", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
test("three months of backups are shown in manageable pages", async ({
  page,
}) => {
  const state = await (await page.request.get("/api/status")).json();
  state.backups = Array.from({ length: 3600 }, (_, i) => ({
    ...state.backups[0],
    id: "test-" + i,
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(state),
    }),
  );
  await page.reload();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(page.getByRole("row")).toHaveCount(51);
  await page
    .getByRole("button", { name: /Weitere Sicherungen anzeigen/ })
    .click();
  await expect(page.getByRole("row")).toHaveCount(101);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator("body")).toHaveJSProperty("scrollWidth", 390);
});

test("failed latest backups do not leave a reassuring green host status", async ({
  page,
}) => {
  const state = await (await page.request.get("/api/status")).json();
  const h = state.hosts.find(
    (h: { name: string }) => h.name === "pve-berlin-01",
  );
  h.enabled = true;
  state.jobs.push({
    id: "failed-latest",
    host_id: h.id,
    kind: "backup",
    state: "failed",
    created_at: new Date(Date.now() + 1000).toISOString(),
    finished_at: new Date(Date.now() + 2000).toISOString(),
    error: "SSH unreachable",
    attempts: 4,
  });
  await page.route("**/api/status", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(state),
    }),
  );
  await page.reload();
  await expect(
    page.getByRole("row").filter({
      has: page.getByRole("button", { name: "pve-berlin-01", exact: true }),
    }),
  ).toContainText("Letzter Versuch fehlgeschlagen");
});

test("maintenance and notification failures remain visible to administrators", async ({
  page,
}) => {
  const state = await (await page.request.get("/api/status")).json();
  state.maintenance_health = {
    at: new Date().toISOString(),
    error: "Prüfsumme falsch",
  };
  state.notification_health = {
    at: new Date().toISOString(),
    error: "Webhook HTTP 500",
  };
  await page.route("**/api/status", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(state),
    }),
  );
  await page.reload();
  await expect(
    page.getByRole("alert").filter({ hasText: "Wartung nicht abgeschlossen" }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("alert")
      .filter({ hasText: "Benachrichtigung konnte nicht zugestellt werden" }),
  ).toBeVisible();
});

test("resizing with an open action menu closes it without browser errors", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: /^Aktionen für/ })
    .first()
    .click();
  await expect(
    page.getByRole("button", { name: "Archivieren", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 1000, height: 800 });
  await expect(page.locator(".action-popover")).not.toBeVisible();
  expect(errors).toEqual([]);
});

test("access administration shows session policy and protects the signed-in account", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Zugriff", exact: true }).click();
  await expect(page.getByLabel("Sitzungsdauer in Tagen")).toHaveValue("30");
  await page.getByRole("button", { name: "Aktionen für demo" }).click();
  await page.getByRole("button", { name: "Zugang bearbeiten" }).click();
  await expect(page.getByRole("dialog")).toContainText("demo");
  await expect(page.getByLabel("Zugang gesperrt")).toBeDisabled();
});
test("update panel describes setup when the updater is unavailable", async ({
  page,
}) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "System", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Updates", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Updates sind in der Demo ausgeschaltet.", { exact: false }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Nach Updates suchen" }),
  ).toBeDisabled();
});

test("web certificate settings keep demo isolated", async ({ page }) => {
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "System", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Webzertifikat", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(
      "Zertifikatsverwaltung ist in der lokalen Demo ausgeschaltet.",
      { exact: true },
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Jetzt erneuern", exact: true }),
  ).toHaveCount(0);
});

test("web certificate renewal confirms browser trust and keeps policy after failure", async ({
  page,
}) => {
  let automatic = true,
    days = 30,
    renewed = false;
  const state = () => ({
    enabled: true,
    managed: true,
    automatic,
    renew_before_days: days,
    expires_at: renewed ? "2027-10-06T12:00:00Z" : "2026-10-16T12:00:00Z",
    valid_from: "2026-10-06T12:00:00Z",
    days_remaining: renewed ? 365 : 10,
    fingerprint: renewed ? "BB22" : "AA11",
    last_error: renewed ? "" : "Speicherplatz reicht nicht aus",
    names: ["anker.internal"],
    last_renewed_at: renewed ? "2026-10-06T12:00:00Z" : "",
    message:
      "Selbstsigniertes Zertifikat. Nach Erneuerung kann eine neue Browserfreigabe nötig sein.",
  });
  await page.route("**/api/tls", (route) => {
    if (route.request().method() === "POST") {
      const input = route.request().postDataJSON();
      automatic = input.automatic;
      days = input.renew_before_days;
    }
    return route.fulfill({ json: state() });
  });
  await page.route("**/api/tls/renew", (route) =>
    renewed
      ? route.fulfill({ json: state() })
      : route.fulfill({
          status: 409,
          json: { error: "Einrichtung läuft; erneut versuchen" },
        }),
  );
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "System", exact: true }).click();
  await expect(page.getByText("AA11", { exact: true })).toBeVisible();
  await expect(page.getByRole("alert")).toContainText(
    "Speicherplatz reicht nicht aus",
  );
  await page.getByLabel("Automatisch erneuern").uncheck();
  await page.getByLabel("Vorlauf in Tagen").fill("14");
  await page
    .getByRole("button", { name: "Erneuerung speichern", exact: true })
    .click();
  await expect(page.getByLabel("Automatisch erneuern")).not.toBeChecked();
  await expect(page.getByLabel("Vorlauf in Tagen")).toHaveValue("14");
  await page
    .getByRole("button", { name: "Jetzt erneuern", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText("Browserfreigabe");
  await page
    .getByRole("button", { name: "Zertifikat jetzt erneuern", exact: true })
    .click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Einrichtung läuft",
  );
  renewed = true;
  await page
    .getByRole("button", { name: "Zertifikat jetzt erneuern", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByText("BB22", { exact: true })).toBeVisible();
});
test("update installation requires a reviewed version and recovers after a restart", async ({
  page,
}) => {
  let installing = false;
  const state = () => ({
    configured: true,
    repository: "example/anker",
    current: installing ? "0.3.0" : "0.2.0",
    status: installing ? "successful" : "idle",
    available: installing
      ? undefined
      : {
          version: "v0.3.0",
          url: "https://github.com/example/anker/releases/tag/v0.3.0",
          artifact: { size: 123456 },
        },
    message: installing ? "Update installiert und Start geprüft" : "",
  });
  await page.route("**/api/updates", (route) =>
    route.fulfill({ json: state() }),
  );
  await page.route("**/api/updates/check", (route) =>
    route.fulfill({ json: state() }),
  );
  await page.route("**/api/updates/install", (route) => {
    expect(route.request().postDataJSON()).toEqual({ version: "v0.3.0" });
    installing = true;
    return route.fulfill({
      json: { ...state(), status: "installing", target: "v0.3.0" },
    });
  });
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "System", exact: true }).click();
  await page
    .getByRole("button", { name: "Update installieren", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText("v0.3.0");
  await page
    .getByRole("button", { name: "Jetzt installieren", exact: true })
    .click();
  await expect(
    page.getByText("Update installiert und Start geprüft", { exact: true }),
  ).toBeVisible({ timeout: 10000 });
});

test("job details link to their backup and removing history preserves it", async ({
  page,
}) => {
  const status = await (await page.request.get("/api/status")).json();
  const created = await page.request.post(
    `/api/hosts/${status.hosts[0].id}/backup`,
    { headers: { "X-Anker-Request": "1" }, data: {} },
  );
  expect(created.ok()).toBeTruthy();
  const job = await created.json();
  await expect
    .poll(
      async () =>
        (await (await page.request.get(`/api/jobs/${job.id}`)).json()).state,
    )
    .toBe("successful");
  const done = await (await page.request.get(`/api/jobs/${job.id}`)).json();
  await page.getByRole("button", { name: "Aufträge", exact: true }).click();
  await page.getByRole("button", { name: `Auftrag ${job.id} ansehen` }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText(job.id, { exact: true })).toBeVisible();
  await expect(dialog.getByText("Manuell", { exact: true })).toBeVisible();
  await dialog
    .getByRole("button", { name: "Sicherung öffnen", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText("Dateien");
  await page.getByRole("button", { name: "Dialog schließen" }).click();
  await page.getByRole("button", { name: `Auftrag ${job.id} ansehen` }).click();
  await dialog
    .getByRole("button", { name: "Eintrag entfernen", exact: true })
    .click();
  await expect(dialog).toContainText(
    "Sicherungen und Tagesmarker bleiben erhalten",
  );
  await dialog.getByRole("button", { name: "Entfernen bestätigen" }).click();
  await expect(dialog).toHaveCount(0);
  expect((await page.request.get(`/api/jobs/${job.id}`)).status()).toBe(404);
  expect(
    (await page.request.get(`/api/backups/${done.result_id}/files`)).ok(),
  ).toBeTruthy();
});

test("job cancellation stays disabled until the worker finishes and failed jobs can be retried", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  let job = {
    id: "ui-job-lifecycle",
    host_id: base.hosts[0].id,
    kind: "backup",
    state: "running",
    created_at: "2026-10-06T10:00:00Z",
    started_at: "2026-10-06T10:00:01Z",
    attempts: 1,
    trigger: "manual",
  };
  let cancelCalls = 0,
    retryCalls = 0;
  let releaseCancel!: () => void;
  const gate = new Promise<void>((resolve) => {
    releaseCancel = resolve;
  });
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, jobs: [job] } }),
  );
  await page.route("**/api/jobs/ui-job-lifecycle", (route) =>
    route.fulfill({ json: job }),
  );
  await page.route("**/api/jobs/ui-job-lifecycle/cancel", async (route) => {
    cancelCalls++;
    await gate;
    await route.fulfill({ json: { ok: true } });
  });
  await page.route("**/api/jobs/ui-job-lifecycle/retry", (route) => {
    retryCalls++;
    job = { ...job, id: "ui-retry-job", state: "queued", attempts: 0 };
    return route.fulfill({ json: job });
  });
  await page.reload();
  await page.getByRole("button", { name: "Aufträge", exact: true }).click();
  await page
    .getByRole("button", { name: "Auftrag ui-job-lifecycle ansehen" })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("button", { name: "Eintrag entfernen", exact: true }),
  ).toHaveCount(0);
  await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
  await expect(
    dialog.getByRole("button", { name: "Wird angefordert …" }),
  ).toBeDisabled();
  expect(cancelCalls).toBe(1);
  releaseCancel();
  await expect(
    dialog.getByRole("button", { name: "Abbruch angefordert", exact: true }),
  ).toBeDisabled();
  job = { ...job, state: "failed" };
  await expect(
    dialog.getByRole("button", { name: "Erneut starten", exact: true }),
  ).toBeVisible();
  await dialog
    .getByRole("button", { name: "Erneut starten", exact: true })
    .click();
  await expect(dialog.getByText("ui-retry-job", { exact: true })).toBeVisible();
  await expect(dialog.getByText("Geplant", { exact: true })).toBeVisible();
  expect(retryCalls).toBe(1);
});

test("late status responses cannot overwrite newer job states", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const job = {
    id: "out-of-order-job",
    host_id: base.hosts[0].id,
    kind: "backup",
    state: "running",
    attempts: 1,
    created_at: "2026-10-06T10:00:00Z",
  };
  let requests = 0,
    release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/status", async (route) => {
    if (++requests === 1) {
      await gate;
      await route.fulfill({ json: { ...base, jobs: [job] } });
    } else
      await route.fulfill({
        json: { ...base, jobs: [{ ...job, state: "cancelled" }] },
      });
  });
  await page.reload();
  await page.getByRole("button", { name: "Aufträge", exact: true }).click();
  await expect(page.getByText("Abgebrochen", { exact: true })).toBeVisible({
    timeout: 10000,
  });
  const stale = page.waitForResponse(
    async (response) =>
      response.url().endsWith("/api/status") &&
      (await response.json()).jobs[0]?.state === "running",
  );
  release();
  await stale;
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
  await expect(page.getByText("Abgebrochen", { exact: true })).toBeVisible();
  await expect(page.getByText("Läuft", { exact: true })).toHaveCount(0);
});

test("scheduler failures are visible to the administrator", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: {
        ...base,
        scheduler_health: {
          at: "2026-10-06T10:00:00Z",
          error: "Host pve-test: Tagesmarker nicht lesbar",
        },
      },
    }),
  );
  await page.reload();
  await expect(page.getByRole("alert")).toContainText(
    "Tagesmarker nicht lesbar",
  );
  await page.getByRole("button", { name: "Aufträge", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Zeitplan konnte nicht vollständig ausgeführt werden",
  );
});

test("fresh job details take precedence over an older status list", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const job = {
    id: "fresh-job-detail",
    host_id: base.hosts[0].id,
    kind: "backup",
    state: "running",
    attempts: 1,
    created_at: "2026-10-06T10:00:00Z",
  };
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, jobs: [job] } }),
  );
  await page.route("**/api/jobs/fresh-job-detail", (route) =>
    route.fulfill({
      json: {
        ...job,
        state: "failed",
        finished_at: "2026-10-06T10:01:00Z",
        error: "SSH-Verbindung zum Host fehlgeschlagen",
      },
    }),
  );
  await page.reload();
  await page.getByRole("button", { name: "Aufträge", exact: true }).click();
  await page
    .getByRole("button", { name: "Auftrag fresh-job-detail ansehen" })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByText("Fehlgeschlagen", { exact: true }),
  ).toBeVisible();
  await expect(dialog).toContainText("SSH-Verbindung zum Host fehlgeschlagen");
  await expect(
    dialog.getByRole("button", { name: "Erneut starten", exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "Abbrechen", exact: true }),
  ).toHaveCount(0);
});

test("late job detail responses cannot replace a different selected job", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const a = {
    id: "delayed-job-a",
    host_id: base.hosts[0].id,
    kind: "backup",
    state: "successful",
    attempts: 1,
    created_at: "2026-10-06T10:00:00Z",
  };
  const b = { ...a, id: "current-job-b", state: "running" };
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, jobs: [a, b] } }),
  );
  await page.route("**/api/jobs/delayed-job-a", async (route) => {
    await gate;
    await route.fulfill({ json: a });
  });
  await page.route("**/api/jobs/current-job-b", (route) =>
    route.fulfill({ json: b }),
  );
  await page.reload();
  await page.getByRole("button", { name: "Aufträge", exact: true }).click();
  await page
    .getByRole("button", { name: "Auftrag delayed-job-a ansehen" })
    .click();
  await expect(page.getByRole("dialog")).toContainText("Auftrag wird geladen");
  await page.getByRole("button", { name: "Dialog schließen" }).click();
  await page
    .getByRole("button", { name: "Auftrag current-job-b ansehen" })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText(b.id, { exact: true })).toBeVisible();
  const late = page.waitForResponse((response) =>
    response.url().endsWith("/api/jobs/delayed-job-a"),
  );
  release();
  await late;
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
  await expect(dialog.getByText(b.id, { exact: true })).toBeVisible();
  await expect(dialog.getByText(a.id, { exact: true })).toHaveCount(0);
  await expect(
    dialog.getByRole("button", { name: "Eintrag entfernen", exact: true }),
  ).toHaveCount(0);
});

test("password settings and eight-character account changes use the saved policy", async ({
  page,
}) => {
  const headers = { "X-Anker-Request": "1" };
  const original = await (await page.request.get("/api/settings")).json();
  const name = "password-policy-browser";
  try {
    await page
      .getByRole("navigation")
      .getByRole("button", { name: "Einstellungen", exact: true })
      .click();
    await page.getByRole("button", { name: "Zugriff", exact: true }).click();
    await expect(
      page.getByLabel("Passwort-Mindestlänge", { exact: true }),
    ).toHaveValue("8");
    await page
      .getByRole("button", { name: "Benutzer hinzufügen", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Benutzername", { exact: true }).fill(name);
    await dialog.getByLabel("Passwort", { exact: true }).fill("12345678");
    await dialog
      .getByLabel("Passwort wiederholen", { exact: true })
      .fill("12345678");
    await dialog
      .getByRole("button", { name: "Benutzer anlegen", exact: true })
      .click();
    await expect(dialog).toHaveCount(0);
    expect(
      (
        await page.request.post("/api/login", {
          headers,
          data: { name, password: "12345678" },
        })
      ).ok(),
    ).toBeTruthy();
    // API login changed the browser cookie; restore the administrator session.
    await page.request.post("/api/login", {
      headers,
      data: { name: "demo", password: "anker-demo-2026" },
    });
    await page.getByLabel("Passwort-Mindestlänge", { exact: true }).fill("12");
    await page
      .getByRole("button", { name: "Einstellungen speichern", exact: true })
      .click();
    await expect(
      page.getByText("Alle Änderungen gespeichert", { exact: true }),
    ).toBeVisible();
    expect(
      await (await page.request.get("/api/settings")).json(),
    ).toMatchObject({ password_min_length: 12 });
    await page.getByRole("button", { name: "Aktionen für " + name }).click();
    await page
      .getByRole("button", { name: "Passwort ändern", exact: true })
      .click();
    await expect(dialog).toContainText("Mindestens 12 Zeichen");
    await dialog.getByLabel("Neues Passwort", { exact: true }).fill("87654321");
    await dialog
      .getByLabel("Passwort wiederholen", { exact: true })
      .fill("87654321");
    await expect(
      dialog.getByRole("button", { name: "Änderungen speichern", exact: true }),
    ).toBeDisabled();
    await dialog
      .getByRole("button", { name: "Abbrechen", exact: true })
      .click();
    await page.getByLabel("Passwort-Mindestlänge", { exact: true }).fill("8");
    await page
      .getByRole("button", { name: "Einstellungen speichern", exact: true })
      .click();
    await expect(
      page.getByText("Alle Änderungen gespeichert", { exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Aktionen für " + name }).click();
    await page
      .getByRole("button", { name: "Passwort ändern", exact: true })
      .click();
    await dialog.getByLabel("Neues Passwort", { exact: true }).fill("87654321");
    await dialog
      .getByLabel("Passwort wiederholen", { exact: true })
      .fill("87654321");
    await dialog
      .getByRole("button", { name: "Änderungen speichern", exact: true })
      .click();
    await expect(dialog).toHaveCount(0);
    expect(
      (
        await page.request.post("/api/login", {
          headers,
          data: { name, password: "87654321" },
        })
      ).ok(),
    ).toBeTruthy();
  } finally {
    await page.request.post("/api/login", {
      headers,
      data: { name: "demo", password: "anker-demo-2026" },
    });
    await page.request.delete("/api/users/" + name, { headers });
    await page.request.put("/api/settings", { headers, data: original });
  }
});

test("official update source can be configured after fingerprint confirmation", async ({
  page,
}) => {
  let configured = false;
  const key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
  const fingerprint =
    "SHA256:66687aadf862bd776c8fc18b8e9f8e20089714856ee233b3902a591d0d5f2925";
  const configuration = () => ({
    repository: "jahartmann/Anker",
    public_key: key,
    fingerprint,
    configured,
    official: true,
    has_token: false,
    official_repository: "jahartmann/Anker",
    official_public_key: key,
    official_fingerprint: fingerprint,
  });
  await page.route("**/api/updates", (route) =>
    route.fulfill({
      json: {
        configured,
        repository: configured ? "jahartmann/Anker" : "",
        current: "v0.2.0",
        status: configured ? "idle" : "unconfigured",
      },
    }),
  );
  await page.route("**/api/updates/configuration", (route) =>
    route.fulfill({ json: configuration() }),
  );
  await page.route("**/api/updates/configure", (route) => {
    expect(route.request().postDataJSON()).toEqual({
      repository: "jahartmann/Anker",
      public_key: key,
      confirmed: true,
    });
    configured = true;
    return route.fulfill({ json: configuration() });
  });
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "System", exact: true }).click();
  await page
    .getByRole("button", { name: "Updatequelle einrichten", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText(fingerprint);
  await expect(
    dialog.getByRole("button", { name: "Quelle speichern", exact: true }),
  ).toBeDisabled();
  await dialog
    .getByLabel("Ich vertraue dieser Quelle und diesem Signierschlüssel")
    .check();
  await dialog
    .getByRole("button", { name: "Quelle speichern", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Nach Updates suchen" }),
  ).toBeEnabled();
  await expect(page.getByText("ist verfügbar", { exact: false })).toHaveCount(
    0,
  );
});

test("custom update source needs a valid key and renewed trust confirmation", async ({
  page,
}) => {
  const key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
  const configuration = {
    repository: "jahartmann/Anker",
    public_key: key,
    fingerprint: "SHA256:official",
    configured: false,
    official: true,
    has_token: false,
    official_repository: "jahartmann/Anker",
    official_public_key: key,
    official_fingerprint: "SHA256:official",
  };
  await page.route("**/api/updates", (route) =>
    route.fulfill({
      json: { configured: false, current: "v0.2.0", status: "unconfigured" },
    }),
  );
  await page.route("**/api/updates/configuration", (route) =>
    route.fulfill({ json: configuration }),
  );
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "System", exact: true }).click();
  await page
    .getByRole("button", { name: "Updatequelle einrichten", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog
    .getByRole("combobox", { name: "Updatequelle", exact: true })
    .selectOption("custom");
  await dialog.getByLabel("GitHub-Repository").fill("example/custom");
  await dialog.getByLabel("Öffentlicher Signierschlüssel").fill("invalid");
  await expect(
    dialog.getByRole("button", { name: "Quelle speichern", exact: true }),
  ).toBeDisabled();
  await dialog.getByLabel("Öffentlicher Signierschlüssel").fill(key);
  await expect(dialog).toContainText("SHA256:66687aad");
  await dialog
    .getByLabel("Ich vertraue dieser Quelle und diesem Signierschlüssel")
    .check();
  await expect(
    dialog.getByRole("button", { name: "Quelle speichern", exact: true }),
  ).toBeEnabled();
  await dialog.getByLabel("GitHub-Repository").fill("another/custom");
  await expect(
    dialog.getByLabel("Ich vertraue dieser Quelle und diesem Signierschlüssel"),
  ).not.toBeChecked();
  await expect(
    dialog.getByRole("button", { name: "Quelle speichern", exact: true }),
  ).toBeDisabled();
});
