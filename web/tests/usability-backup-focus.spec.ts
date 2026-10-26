import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});
async function setup(
  page: Page,
  options: {
    damaged?: boolean;
    reader?: boolean;
    missing?: boolean;
    many?: boolean;
  } = {},
) {
  const base = await (await page.request.get("/api/status")).json();
  const host = base.hosts[0],
    other = base.hosts[1];
  const backups = [
    {
      ...base.backups[0],
      id: "focus-new",
      host_id: host.id,
      host_name: host.name,
      created_at: new Date(Date.now() - 3600000).toISOString(),
      status: options.damaged ? "damaged" : "successful",
      verification_error: options.damaged ? "hash mismatch" : "",
    },
    {
      ...base.backups[0],
      id: "focus-old",
      host_id: host.id,
      host_name: host.name,
      created_at: new Date(Date.now() - 7200000).toISOString(),
      status: options.missing ? "partial" : "successful",
      verification_error: "",
    },
    {
      ...base.backups[0],
      id: "focus-other",
      host_id: other.id,
      host_name: other.name,
      created_at: new Date().toISOString(),
      status: "successful",
      verification_error: "",
    },
  ];
  const hosts = options.many
    ? Array.from({ length: 20 }, (_, i) => ({
        ...host,
        id: `collection-${i}`,
        name: `node-${i}`,
      }))
    : [host, other];
  const all = options.many
    ? hosts.map((h) => ({
        ...backups[0],
        id: `backup-${h.id}`,
        host_id: h.id,
        host_name: h.name,
      }))
    : backups;
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: { ...base, hosts, backups: all, jobs: [], references: {} },
    }),
  );
  const pairs: { from: string; to: string }[] = [];
  await page.route("**/api/backups/compare", (r) => {
    pairs.push(r.request().postDataJSON());
    return r.fulfill({
      json: [
        {
          path: "etc/sysctl.d/99-anker.conf",
          change: "changed",
          content_changed: true,
          metadata_changed: false,
          mtime_only: false,
        },
      ],
    });
  });
  if (options.reader) {
    const me = await (await page.request.get("/api/me")).json();
    await page.route("**/api/me", (r) =>
      r.fulfill({ json: { ...me, user: { ...me.user, role: "reader" } } }),
    );
  }
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  if (
    await page
      .getByRole("button", { name: "Navigation öffnen", exact: true })
      .isVisible()
  )
    await page
      .getByRole("button", { name: "Navigation öffnen", exact: true })
      .click();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  return {
    host,
    other,
    pairs,
    focus: page.getByRole("region", { name: "Sicherung im Fokus" }),
  };
}

test("the focus prioritizes a damaged latest capture and offers an older complete source", async ({
  page,
}) => {
  const { focus, host } = await setup(page, { damaged: true });
  await expect(
    focus.getByRole("heading", { name: host.name, exact: true }),
  ).toBeVisible();
  await expect(
    focus.getByRole("region", { name: "Letzter Sicherungsstand" }),
  ).toContainText("Beschädigt");
  await expect(
    focus.getByRole("region", { name: "Stand zur Wiederherstellung" }),
  ).toContainText("ein älterer ausgewählt");
  await focus.getByRole("button", { name: "Mit diesem Stand planen" }).click();
  await expect(page.getByLabel("Sicherung", { exact: true })).toHaveValue(
    "focus-old",
  );
});

test("one selected host shows its real change summary and the exact comparison pair", async ({
  page,
}) => {
  const { host, focus, pairs } = await setup(page);
  await page
    .getByLabel("Sicherungen nach Host filtern", { exact: true })
    .selectOption(host.id);
  await expect(
    focus.getByRole("region", { name: "Änderungszusammenfassung" }),
  ).toContainText("1 Datei mit Änderungen");
  await focus
    .getByRole("button", { name: "Vergleich öffnen", exact: true })
    .click();
  await expect(page.getByLabel("Vergleich von", { exact: true })).toHaveValue(
    "focus-old",
  );
  await expect(page.getByLabel("Vergleich bis", { exact: true })).toHaveValue(
    "focus-new",
  );
  expect(
    pairs.every((p) => p.from === "focus-old" && p.to === "focus-new"),
  ).toBe(true);
  await page
    .getByRole("button", { name: "Zurück zu Sicherungen", exact: true })
    .click();
  await expect(page.getByLabel("Sicherungen nach Host filtern")).toHaveValue(
    host.id,
  );
});

test("three focus cards and same-row host cards have equal heights and matching action baselines", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const { focus, host } = await setup(page);
  await page.getByLabel("Sicherungen nach Host filtern").selectOption(host.id);
  await expect(
    focus.getByRole("region", { name: "Änderungszusammenfassung" }),
  ).toContainText("1 Datei mit Änderungen");
  const cards = await focus
    .locator(".backup-focus-grid > section")
    .evaluateAll((es) =>
      es.map((e) => {
        const r = e.getBoundingClientRect();
        const b = e
          .querySelector(":scope > button:last-child")!
          .getBoundingClientRect();
        return {
          top: r.top,
          width: r.width,
          height: r.height,
          bottom: b.bottom,
        };
      }),
    );
  expect(cards).toHaveLength(3);
  for (const key of ["top", "width", "height", "bottom"] as const)
    expect(
      Math.max(...cards.map((c) => c[key])) -
        Math.min(...cards.map((c) => c[key])),
    ).toBeLessThan(2);
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  await page.getByRole("button", { name: host.name, exact: true }).click();
  const heights = await page
    .locator(".host-workspaces > section")
    .evaluateAll((es) =>
      es.slice(0, 2).map((e) => e.getBoundingClientRect().height),
    );
  expect(Math.abs(heights[0] - heights[1])).toBeLessThan(2);
});

test("bad sources and read-only access cannot offer a restore action", async ({
  page,
}) => {
  let view = await setup(page, { damaged: true, missing: true });
  await expect(
    view.focus.getByRole("button", { name: "Mit diesem Stand planen" }),
  ).toBeDisabled();
  view = await setup(page, { reader: true, damaged: true });
  await expect(
    view.focus.getByRole("button", { name: "Mit diesem Stand planen" }),
  ).toHaveCount(0);
  await expect(
    view.focus.getByRole("button", { name: "Dateien der Quelle ansehen" }),
  ).toBeVisible();
});

test("filters and all-stands mode avoid an unrelated focus panel and collections stay bounded", async ({
  page,
}) => {
  const { focus } = await setup(page, { many: true });
  await expect(page.locator(".backup-collection")).toHaveCount(6);
  await page
    .getByLabel("Sicherungen durchsuchen", { exact: true })
    .fill("not-present");
  await expect(focus).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Keine passenden Sicherungen" }),
  ).toBeVisible();
  await page.getByLabel("Sicherungen durchsuchen", { exact: true }).clear();
  await page.getByRole("button", { name: "Alle Stände", exact: true }).click();
  await expect(focus).toHaveCount(0);
  await expect(page.locator(".backup-table tbody tr")).toHaveCount(20);
});

test("focus cards fit phones without horizontal overflow or runtime errors", async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const { focus, host } = await setup(page);
  await page.getByLabel("Sicherungen nach Host filtern").selectOption(host.id);
  await expect(
    focus.getByRole("button", { name: "Vergleich öffnen", exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(errors).toEqual([]);
  await page.screenshot({
    path: info.outputPath("backup-focus-phone.png"),
    fullPage: true,
  });
});

test("tablet cards stay in three matching columns rather than stretching one topic across a row", async ({
  page,
}) => {
  await page.setViewportSize({ width: 662, height: 744 });
  const { focus, host } = await setup(page);
  await page.getByLabel("Sicherungen nach Host filtern").selectOption(host.id);
  await expect(
    focus.getByRole("button", { name: "Vergleich öffnen", exact: true }),
  ).toBeVisible();
  const rects = await focus
    .locator(".backup-focus-grid > section")
    .evaluateAll((es) =>
      es.map((e) => {
        const r = e.getBoundingClientRect();
        return { width: r.width, height: r.height, top: r.top };
      }),
    );
  for (const key of ["width", "height", "top"] as const)
    expect(
      Math.max(...rects.map((r) => r[key])) -
        Math.min(...rects.map((r) => r[key])),
    ).toBeLessThan(2);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
