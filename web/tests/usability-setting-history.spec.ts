import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import { settingRuns } from "../src/settingHistory";
import type { SettingObservation } from "../src/settingHistory";

const observation = (
  id: string,
  value?: string,
  state = "value",
): SettingObservation => ({
  backup_id: `trace-${id}`,
  created_at: `2026-10-${id.padStart(2, "0")}T10:00:00Z`,
  state,
  value,
  sources: state === "value" ? ["etc/sysctl.d/99-anker.conf"] : [],
});
const response = (
  observations: SettingObservation[],
  next?: string,
  id = "ipv4-forwarding",
) => ({
  setting: {
    id,
    name: id === "hostname" ? "Node-Name" : "IPv4-Weiterleitung",
    key: id === "hostname" ? "hostname" : "net.ipv4.ip_forward",
    kind: id === "hostname" ? "hostname" : "sysctl",
  },
  observations,
  next,
  undated: 0,
});
async function hostPage(page: any, request: any) {
  await page.context().addCookies(await demoCookies(request));
  await page.goto("/");
  const host = await page.getByRole("button", {
    name: "pve-berlin-01",
    exact: true,
  });
  await host.click();
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Änderungen", exact: true })
    .click();
  return page.locator(".setting-trace");
}

test("runs merge adjacent identical settings while retaining gaps and reversions", () => {
  const runs = settingRuns([
    observation("21", "0"),
    observation("20", "0"),
    observation("19", undefined, "unavailable"),
    observation("18", "0"),
    observation("17", "1"),
    observation("16", "0"),
  ]);
  expect(runs).toHaveLength(5);
  expect(runs[0].observations).toHaveLength(2);
  expect(runs[2].value).toBe("0");
  expect(runs[1].state).toBe("unavailable");
});

test("trace is lazy and groups repeated values into compact dated evidence", async ({
  page,
  request,
}, info) => {
  let calls = 0;
  await page.route("**/api/hosts/*/setting-history?*", (r) => {
    calls++;
    return r.fulfill({
      json: response([
        observation("21", "0"),
        observation("20", "0"),
        observation("19", "1"),
        observation("18", "1"),
      ]),
    });
  });
  const panel = await hostPage(page, request);
  expect(calls).toBe(0);
  await panel.locator(":scope > summary").click();
  await expect(panel.locator(".setting-trace-current")).toContainText(
    "Deaktiviert",
  );
  await expect(panel.locator(".setting-trace-current")).toContainText(
    "2 Sicherungen mit diesem Wert",
  );
  await expect(panel.locator(".setting-trace-event")).toHaveCount(2);
  const changed = panel.locator(".setting-trace-event").first();
  await expect(changed).toContainText("Neuer Wert erfasst");
  await expect(changed.locator(":scope > time")).toHaveAttribute(
    "datetime",
    "2026-10-20T10:00:00Z",
  );
  await expect(changed.locator(".setting-trace-transition")).toContainText(
    "Aktiviert",
  );
  await expect(changed.locator(".setting-trace-transition")).toContainText(
    "Deaktiviert",
  );
  await changed.locator(".setting-trace-evidence > summary").click();
  await expect(changed).toContainText("trace-20");
  await expect(changed).toContainText("trace-21");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await panel.screenshot({ path: info.outputPath("setting-trace-mobile.png") });
  await panel.locator(":scope > summary").click();
  await expect(panel.getByLabel("Einstellung verfolgen")).not.toBeVisible();
});

test("protected missing and ambiguous snapshots stay visible without invented changes", async ({
  page,
  request,
}) => {
  await page.route("**/api/hosts/*/setting-history?*", (r) =>
    r.fulfill({
      json: response([
        observation("21", undefined, "protected"),
        observation("20", "0"),
        observation("19", undefined, "ambiguous"),
        observation("18", "1"),
        observation("17", undefined, "not_recorded"),
      ]),
    }),
  );
  const panel = await hostPage(page, request);
  await panel.locator(":scope > summary").click();
  await expect(panel.locator(".setting-trace-current")).toContainText(
    "Geschützter Inhalt",
  );
  await expect(panel.locator(".setting-trace-event")).toHaveCount(4);
  await expect(panel.locator(".setting-trace-event")).not.toContainText([
    "Neuer Wert erfasst",
    "Neuer Wert erfasst",
    "Neuer Wert erfasst",
    "Neuer Wert erfasst",
  ]);
  await expect(panel).toContainText("Widersprüchliche Deklarationen");
  await panel
    .getByRole("button", { name: "Weitere Zeitpunkte anzeigen" })
    .click();
  await expect(panel.locator(".setting-trace-event")).toHaveCount(5);
  await expect(panel).toContainText("Keine Deklaration erfasst");
});

test("older pages merge the same value across the boundary and deduplicate backup IDs", async ({
  page,
  request,
}) => {
  const urls: string[] = [];
  await page.route("**/api/hosts/*/setting-history?*", (r) => {
    urls.push(r.request().url());
    return r.fulfill({
      json: r.request().url().includes("before=")
        ? response([
            observation("20", "0"),
            observation("19", "0"),
            observation("18", "1"),
          ])
        : response(
            [observation("21", "0"), observation("20", "0")],
            "trace-20",
          ),
    });
  });
  const panel = await hostPage(page, request);
  await panel.locator(":scope > summary").click();
  await expect(panel.locator(".setting-trace-event")).toHaveCount(1);
  await panel
    .getByRole("button", { name: "Ältere Sicherungen verfolgen" })
    .click();
  await expect(panel.locator(".setting-trace-current")).toContainText(
    "3 Sicherungen mit diesem Wert",
  );
  await expect(panel.locator(".setting-trace-event")).toHaveCount(2);
  await expect(
    panel.getByRole("button", { name: "Ältere Sicherungen verfolgen" }),
  ).toHaveCount(0);
  expect(urls).toHaveLength(2);
  expect(urls[1]).toContain("before=trace-20");
});

test("late setting responses cannot replace the selected setting", async ({
  page,
  request,
}) => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let started = false;
  await page.route("**/api/hosts/*/setting-history?*", async (r) => {
    if (r.request().url().includes("setting=ipv4-forwarding")) {
      started = true;
      await gate;
      return r.fulfill({ json: response([observation("21", "1")]) });
    }
    return r.fulfill({
      json: response([observation("21", "pve-new")], undefined, "hostname"),
    });
  });
  const panel = await hostPage(page, request);
  await panel.locator(":scope > summary").click();
  await expect.poll(() => started).toBe(true);
  await panel.getByLabel("Einstellung verfolgen").selectOption("hostname");
  await expect(panel.locator(".setting-trace-current")).toContainText(
    "pve-new",
  );
  release();
  await expect(panel.getByLabel("Einstellung verfolgen")).toHaveValue(
    "hostname",
  );
  await expect(panel.locator(".setting-trace-current")).not.toContainText(
    "Aktiviert",
  );
});

test("history failures are retryable and equal timestamps never claim a change interval", async ({
  page,
  request,
}) => {
  let calls = 0;
  await page.route("**/api/hosts/*/setting-history?*", (r) => {
    if (++calls === 1)
      return r.fulfill({
        status: 500,
        json: { error: "Verlauf nicht lesbar" },
      });
    return r.fulfill({
      json: response([
        { ...observation("21", "0"), order_uncertain: true },
        {
          ...observation("20", "1"),
          created_at: observation("21").created_at,
          order_uncertain: true,
        },
      ]),
    });
  });
  const panel = await hostPage(page, request);
  await panel.locator(":scope > summary").click();
  await expect(panel).toContainText("Verlauf nicht lesbar");
  await panel.getByRole("button", { name: "Spur erneut laden" }).click();
  await expect(panel).toContainText("Zeitliche Zuordnung nicht eindeutig");
  await expect(panel).not.toContainText("Neuer Wert erfasst");
});

test("file comparison opens the matching setting trail with keyboard focus", async ({
  page,
  request,
}) => {
  await page.route("**/api/hosts/*/setting-history?*", (r) =>
    r.fulfill({
      json: response([observation("21", "0"), observation("20", "1")]),
    }),
  );
  await hostPage(page, request);
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Änderungen ansehen", exact: true })
    .click();
  const detail = page.getByRole("region", {
    name: "Änderungsdetails der Node",
  });
  await detail
    .getByRole("button", {
      name: "Änderung ansehen: etc/sysctl.d/99-anker.conf",
      exact: true,
    })
    .click();
  await detail
    .getByRole("button", { name: "IPv4-Weiterleitung verfolgen" })
    .click();
  await expect(detail.getByLabel("Einstellung verfolgen")).toBeFocused();
  await expect(detail.locator(".setting-trace-current")).toContainText(
    "Deaktiviert",
  );
  await expect(detail.locator(".setting-trace")).toHaveAttribute("open", "");
});
