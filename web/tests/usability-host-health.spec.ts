import {
  test,
  expect,
  type Page,
  type APIRequestContext,
} from "@playwright/test";
import { assessHost, recoverySource } from "../src/hostHealth";
import type { Backup, Host, Status } from "../src/api";
import { demoCookies } from "./demo-auth";

const at = Date.parse("2026-10-21T10:00:00Z");
const host: Host = {
  id: "health-host",
  name: "Health node",
  address: "192.0.2.50",
  group: "",
  cluster_id: "",
  ssh_user: "anker",
  ssh_port: 22,
  key_path: "",
  known_hosts_path: "",
  enabled: true,
  schedule: "",
  extra_paths: [],
};
const source = (id: string, overrides: Partial<Backup> = {}): Backup => ({
  id,
  host_id: host.id,
  host_name: host.name,
  created_at: "2026-10-21T09:00:00Z",
  status: "successful",
  files: 1,
  size: 12,
  pinned: false,
  archived: false,
  warnings: [],
  ...overrides,
});
const status = (backups: Backup[]): Status => ({
  hosts: [host],
  backups,
  jobs: [],
  plans: [],
  demo: false,
  timezone: "Europe/Berlin",
  stale_hours: 26,
});

test("known integrity failures, impossible clocks and incomplete sources never become the suggested recovery source", () => {
  const safe = source("older-safe", { created_at: "2026-10-21T08:00:00Z" });
  for (const overrides of [
    { verification_error: "mismatch" },
    { status: "damaged" },
    { status: "partial" },
    { status: "failed" },
    { created_at: "not-a-date" },
    { created_at: "2026-10-22T09:00:00Z" },
  ]) {
    const newest = source("newer-unsafe", overrides);
    const backups = [safe, newest];
    expect(recoverySource(backups, at)?.id).toBe(safe.id);
    expect(assessHost(host, status(backups), at).tone).not.toBe("success");
    expect(backups[0]).toBe(safe);
    expect(recoverySource([newest], at)).toBeUndefined();
  }
});

test("a failed first attempt links to its job instead of looking like an untouched host", () => {
  const failed = {
    id: "failed-first",
    host_id: host.id,
    kind: "backup",
    state: "failed",
    created_at: "2026-10-21T09:30:00Z",
    attempts: 1,
  };
  expect(assessHost(host, { ...status([]), jobs: [failed] }, at)).toMatchObject(
    { tone: "danger", action: "job", jobID: failed.id },
  );
});

test("stale and failed attempts carry a concrete next action while freshness cannot be invented", () => {
  const old = source("old", { created_at: "2026-10-18T08:00:00Z" });
  expect(assessHost(host, status([old]), at)).toMatchObject({
    text: "Überfällig",
    action: "backup",
  });
  expect(assessHost(host, status([]), at)).toMatchObject({ action: "probe" });
  expect(
    assessHost({ ...host, last_probe: "2026-10-21T09:00:00Z" }, status([]), at),
  ).toMatchObject({ action: "backup" });
  expect(
    assessHost(host, { ...status([source("fresh")]), stale_hours: 0 }, at).tone,
  ).not.toBe("success");
  expect(
    assessHost({ ...host, enabled: false }, status([old]), at),
  ).toMatchObject({ text: "Pausiert", action: "settings" });
});

async function openHealthHost(
  page: Page,
  request: APIRequestContext,
  backups: Backup[],
) {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: { ...base, hosts: [host], backups, jobs: [], references: {} },
    }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
}

test("host warns about a recent integrity error and proposes the older complete source for a plan", async ({
  page,
  request,
}) => {
  const created_at = new Date(Date.now() - 3600000).toISOString();
  const safe = source("safe-source", { created_at });
  const broken = source("broken-source", {
    created_at: new Date().toISOString(),
    verification_error: "checksum mismatch",
  });
  await openHealthHost(page, request, [broken, safe]);
  await expect(page.locator(".host-status-strip")).toContainText(
    "Beschädigter Stand",
  );
  await expect(page.locator(".host-next-step")).toContainText(
    "Integritätsfehler",
  );
  await expect(page.locator(".restore-source-note")).toBeVisible();
  await page
    .getByRole("button", { name: "Wiederherstellung planen", exact: true })
    .click();
  await expect(page.getByLabel("Sicherung", { exact: true })).toHaveValue(
    safe.id,
  );
});

test("no usable source disables the suggested restore and stays visible in the attention filter", async ({
  page,
  request,
}) => {
  await openHealthHost(page, request, [
    source("partial-only", {
      created_at: new Date().toISOString(),
      status: "partial",
    }),
  ]);
  await expect(
    page.getByRole("button", { name: "Wiederherstellung planen", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByText(
      "Kein vollständiger Stand ohne bekannten Integritätsfehler verfügbar.",
      { exact: false },
    ),
  ).toBeVisible();
});

test("starting a host operation prevents a second request before the server acknowledges it", async ({
  page,
  request,
}) => {
  await openHealthHost(page, request, []);
  let calls = 0;
  let finish!: () => void;
  const gate = new Promise<void>((resolve) => {
    finish = resolve;
  });
  await page.route("**/api/hosts/health-host/probe", async (route) => {
    calls++;
    await gate;
    await route.fulfill({ json: { id: "probe-job" } });
  });
  await page
    .getByRole("button", { name: "Verbindung prüfen", exact: true })
    .first()
    .click();
  await expect(
    page.getByRole("button", { name: "Jetzt sichern", exact: true }),
  ).toBeDisabled();
  await expect(
    page
      .getByRole("button", { name: "Verbindung prüfen", exact: true })
      .first(),
  ).toBeDisabled();
  expect(calls).toBe(1);
  finish();
  await expect(
    page.getByRole("button", { name: "Jetzt sichern", exact: true }),
  ).toBeEnabled();
});

test("unreadable dates are actionable and do not break host or history rendering", async ({
  page,
  request,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await openHealthHost(page, request, [
    source("bad-date", { created_at: "not-a-date" }),
  ]);
  await expect(page.locator(".host-status-strip")).toContainText(
    "Zeitstempel prüfen",
  );
  await expect(
    page.getByRole("button", { name: "Wiederherstellung planen", exact: true }),
  ).toBeDisabled();
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(page.locator(".backup-history")).toContainText(
    "Ungültiger Zeitstempel",
  );
  expect(errors).toEqual([]);
});
