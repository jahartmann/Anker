import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});
async function manyHosts(page: any) {
  const base = await (await page.request.get("/api/status")).json();
  const hosts = Array.from({ length: 24 }, (_, i) => ({
    ...base.hosts[0],
    id: `ops-${i}`,
    name: `cp-site-${String(i).padStart(2, "0")}`,
    address: `10.42.0.${i + 1}`,
    group: i < 12 ? "CP" : "FZ",
    config_revision: "a".repeat(64),
    inventory: undefined,
  }));
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts, backups: [], jobs: [] } }),
  );
  return { base, hosts };
}
test("bulk selection survives pagination and filtering and reviews the exact targets", async ({
  page,
}, info) => {
  const { hosts } = await manyHosts(page);
  let posted: any;
  await page.route("**/api/operations/hosts", async (r) => {
    posted = r.request().postDataJSON();
    await r.fulfill({
      json: {
        id: "receipt",
        targets: posted.targets.map((t: any, i: number) => ({
          ...t,
          name: hosts.find((h: any) => h.id === t.host_id).name,
          state: i ? "skipped" : "applied",
          message: i
            ? "Hosteinstellungen inzwischen geändert"
            : "Einstellungen übernommen",
        })),
      },
    });
  });
  await page.goto("/");
  await page.getByLabel("Host cp-site-00 auswählen", { exact: true }).check();
  await page
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .first()
    .click();
  await page.getByLabel("Host cp-site-08 auswählen", { exact: true }).check();
  const controls = page.getByRole("region", { name: "Sammelaktionen" });
  await expect(controls).toContainText("2 ausgewählt");
  await page
    .getByLabel("Hosts durchsuchen", { exact: true })
    .fill("cp-site-23");
  await expect(controls).toContainText("2 ausgewählt");
  await controls
    .getByLabel("Sammelaktion", { exact: true })
    .selectOption("maintenance");
  await controls.getByLabel("Wartungsdauer").selectOption("4");
  await controls
    .getByRole("button", { name: "Aktion prüfen", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "Sammelaktion prüfen" });
  await expect(dialog).toContainText("2 ausdrücklich ausgewählte Hosts");
  await expect(dialog).toContainText("CP Site 00");
  await expect(dialog).not.toContainText("CP Site 23");
  await dialog
    .getByRole("button", { name: "Für diese Auswahl ausführen" })
    .click();
  expect(posted.action).toBe("maintenance");
  expect(posted.targets.map((t: any) => t.host_id)).toEqual(["ops-0", "ops-8"]);
  expect(Date.parse(posted.until) - Date.now()).toBeGreaterThan(3.9 * 3600000);
  await expect(controls).toContainText("1 von 2 übernommen");
  await expect(controls).toContainText("Hosteinstellungen inzwischen geändert");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page.getByRole("button", { name: "cp-site-23", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.screenshot({
    path: info.outputPath("bulk-maintenance-mobile.png"),
    fullPage: true,
  });
});
test("select filtered hosts includes every page without authorizing future hosts", async ({
  page,
}) => {
  await manyHosts(page);
  await page.goto("/");
  await page
    .getByRole("region", { name: "Sammelaktionen" })
    .getByRole("button", { name: "Alle 24 gefilterten Hosts auswählen" })
    .click();
  await expect(
    page.getByRole("region", { name: "Sammelaktionen" }),
  ).toContainText("24 ausgewählt");
  await page
    .getByRole("button", { name: "Aktion prüfen", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText(
    "24 ausdrücklich ausgewählte Hosts",
  );
});
test("live drift keeps unknown and partial evidence distinct and offers a read only check", async ({
  page,
}, info) => {
  const base = await (await page.request.get("/api/status")).json();
  const h = base.hosts[0];
  await page.route(`**/api/operations/drift/${h.id}`, (r) =>
    r.fulfill({
      json: {
        state: "partial",
        detail:
          "1 Änderung, 2 nicht vergleichbare Einträge; keine vollständige Entwarnung.",
        at: new Date().toISOString(),
        backup_id: "reference",
        unknown: 2,
        files: [{ path: "etc/network/interfaces", state: "changed" }],
      },
    }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: h.name, exact: true }).click();
  const card = page.locator(".drift-card");
  await expect(card).toContainText("Vergleich unvollständig");
  await expect(card).toContainText("Geändert");
  await expect(card).toContainText("etc/network/interfaces");
  await expect(card).toContainText("Automatik: ausgeschaltet");
  await expect(
    card.getByRole("button", { name: "Jetzt live prüfen" }),
  ).toBeVisible();
  await card.screenshot({ path: info.outputPath("live-drift.png") });
});
test("helper overview shows unknown versions and blocks changed identities before requesting credentials", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const h = base.hosts[0];
  await page.route("**/api/operations/helpers", (r) =>
    r.fulfill({
      json: [
        {
          host_id: h.id,
          name: h.name,
          state: "unknown",
          desired_version: "2.1",
          desired_hash: "a".repeat(64),
        },
      ],
    }),
  );
  await page.route("**/api/hosts/connection/inspect", (r) =>
    r.fulfill({
      json: {
        address: h.address,
        port: 22,
        fingerprint: "SHA256:changed",
        known: true,
        changed: true,
      },
    }),
  );
  await page.goto("/");
  await page
    .getByRole("button", { name: "Helferübersicht", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Unbekannt");
  await dialog
    .getByRole("button", { name: "Helfer aktualisieren", exact: true })
    .click();
  await expect(dialog).toContainText("Aktualisierung gesperrt");
  await expect(dialog.getByLabel("Passwort", { exact: true })).toHaveCount(0);
});
test("grouped incidents acknowledge without disappearing and paginate affected hosts", async ({
  page,
}, info) => {
  const { hosts } = await manyHosts(page);
  const incident = {
    id: "issue",
    code: "invalid_path",
    title: "Pfad konnte nicht verarbeitet werden",
    detail: "Die Pfadangabe benötigt eine Prüfung.",
    action: "Mit aktuellem Helfer neu sichern.",
    state: "new",
    first_seen: new Date().toISOString(),
    last_seen: new Date().toISOString(),
    reopen_count: 0,
    hosts: hosts.map((h: any) => ({ id: h.id, name: h.name, muted: false })),
  };
  await page.route("**/api/operations/incidents", (r) =>
    r.fulfill({ json: [incident] }),
  );
  await page.route("**/api/operations/incidents/issue", (r) =>
    r.fulfill({
      json: { ...incident, state: "acknowledged", acknowledged_by: "demo" },
    }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  const center = page.getByRole("region", { name: "Jetzt wichtig" });
  await expect(center.locator(".incident-card")).toHaveCount(1);
  await center
    .getByRole("button", { name: "Zur Kenntnis nehmen", exact: true })
    .click();
  await expect(center).toContainText("Zur Kenntnis genommen");
  await center
    .getByText("Betroffene Hosts und Nachweise", { exact: true })
    .click();
  await expect(
    center.getByRole("button", { name: /Host .* prüfen/ }),
  ).toHaveCount(5);
  await center
    .getByLabel("Pfad konnte nicht verarbeitet werden: Hosts durchsuchen")
    .fill("cp-site-23");
  await expect(
    center.getByRole("button", { name: /Host .* prüfen/ }),
  ).toHaveCount(1);
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await center.screenshot({
    path: info.outputPath("incident-center-mobile.png"),
  });
});

test("helper refresh uses confirmed identity and keeps its transient password out of browser storage", async ({
  page,
}, info) => {
  const base = await (await page.request.get("/api/status")).json();
  const h = base.hosts[0];
  let submitted: any;
  await page.route("**/api/operations/helpers", (r) =>
    r.fulfill({
      json: [
        {
          host_id: h.id,
          name: h.name,
          state: "unknown",
          desired_version: "2.1",
          desired_hash: "a".repeat(64),
        },
      ],
    }),
  );
  await page.route("**/api/hosts/connection/inspect", (r) =>
    r.fulfill({
      json: {
        address: h.address,
        port: h.ssh_port,
        fingerprint: "SHA256:already-pinned",
        known: true,
        changed: false,
      },
    }),
  );
  await page.route("**/api/operations/helpers/refresh", (r) => {
    submitted = r.request().postDataJSON();
    return r.fulfill({ json: { version: "2.1", sha256: "a".repeat(64) } });
  });
  await page.goto("/");
  await page
    .getByRole("button", { name: "Helferübersicht", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog
    .getByRole("button", { name: "Helfer aktualisieren", exact: true })
    .click();
  await dialog
    .getByLabel("Passwort", { exact: true })
    .fill("ephemeral-test-password");
  await dialog.getByRole("checkbox").check();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await dialog.screenshot({
    path: info.outputPath("helper-update-mobile.png"),
  });
  await dialog
    .getByRole("button", { name: "Aktualisierung ausführen", exact: true })
    .click();
  await expect(dialog).toContainText("Helferdatei aktualisiert");
  expect(submitted).toMatchObject({
    host_id: h.id,
    revision: h.config_revision,
    password: "ephemeral-test-password",
    fingerprint: "SHA256:already-pinned",
    confirmed: true,
  });
  await expect(dialog.getByLabel("Passwort", { exact: true })).toHaveCount(0);
  const stored = await page.evaluate(() =>
    JSON.stringify([
      Object.values(localStorage),
      Object.values(sessionStorage),
    ]),
  );
  expect(stored).not.toContain("ephemeral-test-password");
});
