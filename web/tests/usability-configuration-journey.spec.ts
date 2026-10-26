import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import type { Backup } from "../src/api";

const setting = (id: string, key: string, value?: string, state = "value") => ({
  setting: {
    id,
    key,
    name: id === "hostname" ? "Node-Name" : "IPv4-Weiterleitung",
    kind: id === "hostname" ? "hostname" : "sysctl",
  },
  observation: {
    backup_id: "",
    created_at: "",
    state,
    value,
    sources: ["etc/sysctl.d/99-anker.conf"],
  },
});
const snapshot = (id: string) => ({
  backup_id: id,
  settings: [
    setting(
      "ipv4-forwarding",
      "net.ipv4.ip_forward",
      id === "journey-2" ? "0" : "1",
    ),
    setting("hostname", "hostname", "pve-berlin-01"),
    setting("swappiness", "vm.swappiness", undefined, "not_recorded"),
  ],
});
const rows = [
  {
    path: "etc/sysctl.d/99-anker.conf",
    change: "changed",
    content_changed: true,
    metadata_changed: false,
    mtime_only: false,
    mtime_changed: false,
    secret: false,
  },
];
async function setup(
  page: any,
  request: any,
  edit?: (backups: Backup[]) => void,
) {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const host = { ...base.hosts[0], name: "pve-berlin-01" };
  const backups = Array.from({ length: 4 }, (_, i) => ({
    ...base.backups[0],
    id: `journey-${i}`,
    host_id: host.id,
    host_name: host.name,
    status: "successful",
    verification_error: "",
    created_at: new Date(Date.now() - (5 - i) * 3600000).toISOString(),
  })) as Backup[];
  edit?.(backups);
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [host], backups, jobs: [] } }),
  );
  const comparisons: any[] = [];
  const reads: string[] = [];
  await page.route("**/api/backups/compare", (r) => {
    comparisons.push(r.request().postDataJSON());
    return r.fulfill({ json: rows });
  });
  await page.route("**/api/backups/*/setting-snapshot", (r) => {
    const id = new URL(r.request().url()).pathname.split("/")[3];
    reads.push(id);
    return r.fulfill({ json: snapshot(id) });
  });
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Änderungszusammenfassung" }),
  ).toContainText("1 Datei mit Änderungen");
  return {
    backups,
    comparisons,
    reads,
    journey: page.getByRole("region", { name: "Konfigurationsreise" }),
  };
}
async function enter(page: any) {
  await page
    .getByRole("button", { name: "Konfigurationsreise öffnen" })
    .click();
}

test("journey starts lazily at the newest point and steps through exact same-node pairs", async ({
  page,
  request,
}) => {
  const { journey, reads, comparisons } = await setup(page, request);
  expect(reads).toHaveLength(0);
  await enter(page);
  await expect(journey.locator(".journey-selected-time code")).toHaveText(
    "journey-3",
  );
  await expect(journey.locator(".journey-value-card")).toHaveCount(2);
  await expect(journey.locator(".journey-value-changed")).toContainText(
    "IPv4-Weiterleitung",
  );
  expect(reads.sort()).toEqual(["journey-2", "journey-3"]);
  await journey
    .getByRole("button", { name: "Vorheriger Sicherungszeitpunkt" })
    .click();
  await expect(journey.locator(".journey-selected-time code")).toHaveText(
    "journey-2",
  );
  await expect(journey.locator(".journey-value-changed")).toContainText(
    "Deaktiviert",
  );
  expect(comparisons.at(-1)).toEqual({ from: "journey-1", to: "journey-2" });
  await journey.getByRole("slider").press("Home");
  await expect(journey.locator(".journey-selected-time code")).toHaveText(
    "journey-0",
  );
  await expect(journey.locator(".journey-value-card")).toHaveCount(2);
  await expect(journey).toContainText("Erster erfasster Stand");
  await expect(
    journey.getByRole("button", {
      name: "Diesen Zeitpunkt im Detail vergleichen",
    }),
  ).toHaveCount(0);
  await expect(
    journey.getByRole("button", { name: "Vorheriger Sicherungszeitpunkt" }),
  ).toBeDisabled();
});

test("area cards distinguish changed settings from file categories and remain compact on phones", async ({
  page,
  request,
}, info) => {
  const { journey } = await setup(page, request);
  await enter(page);
  await expect(journey.locator(".journey-value-card")).toHaveCount(2);
  const network = journey
    .getByRole("group", { name: "Bereich der Konfigurationsreise" })
    .getByRole("button", { name: /Netzwerk/ });
  await expect(network).toContainText("1 Wert geändert");
  await network.click();
  await expect(journey.locator(".journey-value-card")).toHaveCount(2);
  await expect(journey.locator(".journey-file-card")).toHaveCount(0);
  await journey.getByRole("button", { name: /Alle Bereiche/ }).click();
  await journey
    .getByRole("button", {
      name: /weitere Einstellungen ohne eindeutigen Wert anzeigen/,
    })
    .click();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await journey.screenshot({
    path: info.outputPath("configuration-journey-mobile.png"),
  });
});

test("opening a historical detail preserves the journey position and return focus", async ({
  page,
  request,
}) => {
  const { journey } = await setup(page, request);
  await enter(page);
  await journey
    .getByRole("button", { name: "Vorheriger Sicherungszeitpunkt" })
    .click();
  const link = journey.getByRole("button", {
    name: "Diesen Zeitpunkt im Detail vergleichen",
  });
  await link.click();
  const detail = page.getByRole("region", {
    name: "Änderungsdetails der Node",
  });
  await expect(detail.getByLabel("Vergleich von", { exact: true })).toHaveValue(
    "journey-1",
  );
  await expect(detail.getByLabel("Vergleich bis", { exact: true })).toHaveValue(
    "journey-2",
  );
  await detail.getByRole("button", { name: "Zurück zum Hostverlauf" }).click();
  await expect(journey.locator(".journey-selected-time code")).toHaveText(
    "journey-2",
  );
  await expect(link).toBeFocused();
});

test("incomplete predecessors never produce a trustworthy diff", async ({
  page,
  request,
}) => {
  const { journey, comparisons } = await setup(page, request, (b) => {
    b[2].status = "partial";
  });
  await enter(page);
  await expect(journey).toContainText("Dazwischen bleibt eine Nachweislücke");
  await expect(journey.locator(".journey-value-changed")).toHaveCount(0);
  const count = comparisons.length;
  await journey.getByRole("slider").press("Home");
  await expect(journey.locator(".journey-value-card")).toHaveCount(2);
  expect(comparisons.length).toBe(count);
});

test("slow responses from abandoned points cannot overwrite the selected point", async ({
  page,
  request,
}) => {
  const { journey } = await setup(page, request);
  let release!: () => void;
  let started = false;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/backups/journey-3/setting-snapshot", async (r) => {
    started = true;
    await gate;
    await r.fulfill({ json: snapshot("journey-3") }).catch(() => {});
  });
  await enter(page);
  await expect.poll(() => started).toBe(true);
  await journey.getByRole("slider").press("Home");
  await expect(journey.locator(".journey-selected-time code")).toHaveText(
    "journey-0",
  );
  await expect(journey.locator(".journey-value-card")).toHaveCount(2);
  release();
  await expect(journey.locator(".journey-selected-time code")).toHaveText(
    "journey-0",
  );
  await expect(journey.locator(".journey-value-changed")).toHaveCount(0);
});

test("failed projections retain available evidence and have a retry without false reassurance", async ({
  page,
  request,
}) => {
  const { journey } = await setup(page, request);
  let fail = true;
  await page.route("**/api/backups/compare", (r) =>
    fail
      ? r.fulfill({ status: 500, json: { error: "Nicht lesbar" } })
      : r.fulfill({ json: rows }),
  );
  await enter(page);
  await expect(journey).toContainText("nicht vollständig gelesen");
  await expect(journey.locator(".journey-value-card")).toHaveCount(2);
  await expect(
    journey.getByRole("button", {
      name: "Diesen Zeitpunkt im Detail vergleichen",
    }),
  ).toHaveCount(0);
  fail = false;
  await journey.getByRole("button", { name: "Zeitpunkt erneut lesen" }).click();
  await expect(
    journey.getByRole("button", {
      name: "Diesen Zeitpunkt im Detail vergleichen",
    }),
  ).toBeVisible();
});

test("setting trail links select the matching setting", async ({
  page,
  request,
}) => {
  const { journey } = await setup(page, request);
  await page.route("**/api/hosts/*/setting-history?*", (r) =>
    r.fulfill({
      json: {
        setting: {
          id: "ipv4-forwarding",
          name: "IPv4-Weiterleitung",
          key: "net.ipv4.ip_forward",
          kind: "sysctl",
        },
        observations: [
          {
            backup_id: "journey-3",
            created_at: new Date().toISOString(),
            value: "1",
            state: "value",
            sources: [],
          },
        ],
        undated: 0,
      },
    }),
  );
  await enter(page);
  await journey
    .locator(".journey-value-card")
    .filter({ hasText: "IPv4-Weiterleitung" })
    .getByRole("button", { name: "Einstellung verfolgen" })
    .click();
  const trace = page.locator(".setting-trace");
  await expect(trace.getByLabel("Einstellung verfolgen")).toHaveValue(
    "ipv4-forwarding",
  );
  await expect(trace.getByLabel("Einstellung verfolgen")).toBeFocused();
});

test("equal-time backups keep their values without an invented change direction", async ({
  page,
  request,
}) => {
  const { journey, comparisons } = await setup(page, request, (b) => {
    b[2].created_at = b[3].created_at;
  });
  await enter(page);
  await expect(journey).toContainText("zeitliche Reihenfolge");
  await expect(journey.locator(".journey-value-card")).toHaveCount(2);
  await expect(journey.locator(".journey-value-changed")).toHaveCount(0);
  await expect(
    journey.getByRole("button", {
      name: "Diesen Zeitpunkt im Detail vergleichen",
    }),
  ).toHaveCount(0);
});

test("masked projection values are never rendered, including in technical labels", async ({
  page,
  request,
}) => {
  const { journey } = await setup(page, request);
  await page.route("**/api/backups/journey-3/setting-snapshot", (r) => {
    const value = snapshot("journey-3");
    value.settings[0].observation.state = "protected";
    value.settings[0].observation.value = "never-show-this";
    return r.fulfill({ json: value });
  });
  await enter(page);
  await expect(journey).toContainText("Geschützter Inhalt");
  await expect(journey).not.toContainText("never-show-this");
  await expect(
    journey
      .locator(".journey-value-card")
      .filter({ hasText: "IPv4-Weiterleitung" }),
  ).toContainText("Nachweislücke");
});
