import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});
async function setup(
  page: Page,
  options: {
    missing?: boolean;
    paused?: boolean;
    connectionError?: boolean;
    active?: boolean;
    failed?: boolean;
    reader?: boolean;
  } = {},
) {
  const base = await (await page.request.get("/api/status")).json();
  const host = {
    ...base.hosts[0],
    name: "pve-berlin-01",
    enabled: !options.paused,
    last_probe: options.missing ? "" : new Date().toISOString(),
    probe_error: options.connectionError ? "SSH: connection refused" : "",
  };
  const backups = options.missing
    ? []
    : [1, 2].map((n, i) => ({
        ...base.backups[i],
        id: `focus-${n}`,
        host_id: host.id,
        host_name: host.name,
        created_at: new Date(Date.now() - n * 3600000).toISOString(),
        status: "successful",
        verification_error: "",
      }));
  const job = {
    ...base.jobs[0],
    id: "focus-job",
    host_id: host.id,
    kind: "backup",
    state: options.active ? "running" : "failed",
    attempts: 1,
    created_at: new Date().toISOString(),
    error: options.failed ? "SSH: timeout" : "",
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        backups,
        jobs: options.active || options.failed ? [job] : [],
      },
    }),
  );
  await page.route("**/api/backups/compare", (r) => r.fulfill({ json: [] }));
  await page.route("**/api/jobs/focus-job", (r) => r.fulfill({ json: job }));
  if (options.reader) {
    const me = await (await page.request.get("/api/me")).json();
    await page.route("**/api/me", (r) =>
      r.fulfill({ json: { ...me, user: { ...me.user, role: "reader" } } }),
    );
  }
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  return { host, focus: page.getByRole("region", { name: "Für diesen Host" }) };
}

test("overview starts with one answer and three bounded cards; the full history lives in Changes", async ({
  page,
}) => {
  const { focus } = await setup(page);
  await expect(focus.getByRole("heading")).toHaveText(
    "Die letzte Sicherung ist aktuell",
  );
  await expect(focus.getByRole("button")).toHaveCount(1);
  await expect(page.locator(".host-workspaces > .host-workspace")).toHaveCount(
    3,
  );
  await expect(page.locator(".change-timeline, .setting-trace")).toHaveCount(0);
  await expect(page.locator(".host-extra-details")).not.toHaveAttribute(
    "open",
    "",
  );
  await expect(page.locator(".host-status-strip")).toContainText(
    "In der Demo aus",
  );
  await page
    .getByRole("button", { name: "Konfigurationsreise öffnen", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Konfigurationsreise", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".change-timeline")).toHaveCount(1);
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  await expect(focus).toBeVisible();
  await expect(page.locator(".change-timeline, .setting-trace")).toHaveCount(0);
});

test("a missing first backup offers a probe and locks duplicate clicks until acknowledgement", async ({
  page,
}) => {
  const { host, focus } = await setup(page, { missing: true });
  let calls = 0;
  let release!: () => void;
  const gate = new Promise<void>((r) => {
    release = r;
  });
  await page.route(`**/api/hosts/${host.id}/probe`, async (r) => {
    calls++;
    await gate;
    await r.fulfill({ json: {} });
  });
  const start = focus.getByRole("button", {
    name: "Erste Hostprüfung starten",
  });
  await start.click();
  await expect(start).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Jetzt sichern", exact: true }),
  ).toBeDisabled();
  expect(calls).toBe(1);
  release();
  await expect(start).toBeEnabled();
});

test("an active job takes priority and opens its existing details without a new operation", async ({
  page,
}) => {
  const { focus } = await setup(page, { active: true });
  await expect(focus.getByRole("heading")).toHaveText("Die Sicherung läuft");
  await expect(page.locator(".active-job-list")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Jetzt sichern", exact: true }),
  ).toBeDisabled();
  const writes: string[] = [];
  page.on("request", (r) => {
    if (r.method() === "POST") writes.push(r.url());
  });
  await focus
    .getByRole("button", { name: "Laufenden Auftrag ansehen" })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Auftragsdetails" }),
  ).toContainText("focus-job");
  expect(writes).toEqual([]);
});

test("a failed last job and a paused host lead to the relevant detail", async ({
  page,
}) => {
  let view = await setup(page, { failed: true });
  await expect(view.focus.getByRole("heading")).toHaveText(
    "Letzter Versuch fehlgeschlagen",
  );
  await view.focus.getByRole("button", { name: "Fehler ansehen" }).click();
  await expect(
    page.getByRole("dialog", { name: "Auftragsdetails" }),
  ).toContainText("SSH: timeout");
  view = await setup(page, { paused: true });
  await expect(view.focus).toContainText("Pausiert");
  await view.focus.getByRole("button", { name: "Zeitplan ansehen" }).click();
  await expect(
    page
      .locator(".host-tabs")
      .getByRole("button", { name: "Einstellungen", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("a recent backup does not hide a connection error", async ({ page }) => {
  const { host, focus } = await setup(page, { connectionError: true });
  await expect(focus.getByRole("heading")).toContainText(
    "Verbindungsprüfung ist fehlgeschlagen",
  );
  await expect(focus.getByText("SSH: connection refused")).not.toBeVisible();
  await focus.getByText("Verbindungsfehler ansehen").click();
  await expect(focus.getByText("SSH: connection refused")).toBeVisible();
  let calls = 0;
  await page.route(`**/api/hosts/${host.id}/probe`, (r) => {
    calls++;
    return r.fulfill({ json: {} });
  });
  await focus.getByRole("button", { name: "Verbindung erneut prüfen" }).click();
  await expect.poll(() => calls).toBe(1);
});

test("readers get a useful next step without operation controls", async ({
  page,
}) => {
  const { focus } = await setup(page, { missing: true, reader: true });
  await expect(
    page.getByRole("button", { name: "Jetzt sichern", exact: true }),
  ).toHaveCount(0);
  await expect(
    focus.getByRole("button", { name: "Erste Hostprüfung starten" }),
  ).toHaveCount(0);
  await focus.getByRole("button", { name: "Verlauf ansehen" }).click();
  await expect(
    page
      .locator(".host-tabs")
      .getByRole("button", { name: "Sicherungen", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("the next step is visible on a phone without overflow", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const { focus } = await setup(page, { missing: true });
  await expect(focus.getByRole("button")).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(errors).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("host-next-step-mobile.png"),
    fullPage: true,
  });
});

test("the host collection stays at eight rows and its summary clears unrelated filters", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const hosts = Array.from({ length: 25 }, (_, i) => ({
    ...base.hosts[0],
    id: `focus-host-${i}`,
    name: `node-${String(i).padStart(2, "0")}`,
    enabled: true,
    group: i === 24 ? "Andere Gruppe" : "Berlin",
  }));
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts, backups: [], jobs: [] } }),
  );
  await page.goto("/");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(8);
  await page.getByLabel("Hosts durchsuchen", { exact: true }).fill("node-24");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(1);
  await page
    .getByRole("region", { name: "Hosts im Überblick" })
    .getByRole("button", { name: "Betroffene Hosts anzeigen" })
    .click();
  await expect(
    page.getByLabel("Hosts durchsuchen", { exact: true }),
  ).toHaveValue("");
  await expect(page.getByLabel("Hoststatus filtern")).toHaveValue("attention");
  await expect(page.locator(".host-table tbody tr")).toHaveCount(8);
});

test("the configured SSH endpoint stays readable for IPv6 and long account names", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const host = {
    ...base.hosts[0],
    address: "2001:db8:ffff:1234::42",
    ssh_user: "backup-config-reader-with-a-long-account-name",
    ssh_port: 2222,
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [host], jobs: [] } }),
  );
  await page.route("**/api/backups/compare", (r) => r.fulfill({ json: [] }));
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  const endpoint = page.getByLabel("SSH-Ziel für Sicherungen", { exact: true });
  await expect(endpoint).toContainText(
    "backup-config-reader-with-a-long-account-name@[2001:db8:ffff:1234::42]:2222",
  );
  for (const width of [1440, 662, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(endpoint).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    expect(
      await endpoint.evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
  }
});
