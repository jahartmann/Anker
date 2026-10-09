import { test, expect } from "@playwright/test";
import type { Page, APIRequestContext } from "@playwright/test";
import { demoCookies } from "./demo-auth";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await demoCookies(request);
});
test.beforeEach(async ({ page }) => {
  await page.context().addCookies(cookies);
});
const row = (path: string, flags = {}) => ({
  path,
  change: "changed",
  secret: false,
  content_changed: true,
  metadata_changed: false,
  mtime_changed: false,
  mtime_only: false,
  ...flags,
});
const comparison = (path: string, extra = {}) => ({
  path,
  before_label: "Sicherung older",
  after_label: "Sicherung newer",
  state: "different",
  content_changed: true,
  masked: false,
  metadata_changes: [],
  hunks: [
    {
      before_start: 1,
      after_start: 1,
      lines: [
        { kind: "context", text: "context\n", before_line: 1, after_line: 1 },
        { kind: "remove", text: "old\n", before_line: 2 },
        { kind: "add", text: "new\n", after_line: 2 },
      ],
    },
  ],
  ...extra,
});
async function setup(page: Page, rows: ReturnType<typeof row>[]) {
  const base = await (await page.request.get("/api/status")).json();
  const backups = [
    {
      ...base.backups[0],
      id: "older",
      host_id: "one",
      host_name: "Quelle",
      created_at: "2026-10-01T10:00:00Z",
    },
    {
      ...base.backups[0],
      id: "newer",
      host_id: "two",
      host_name: "Ziel",
      created_at: "2026-10-02T10:00:00Z",
    },
  ];
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, backups, jobs: [] } }),
  );
  await page.route("**/api/backups/compare", (r) => r.fulfill({ json: rows }));
  await page.goto("/");
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Stände vergleichen" }).click();
  await page.getByLabel("Vergleich von").selectOption("older");
  await page.getByLabel("Vergleich bis").selectOption("newer");
  await page.getByRole("button", { name: "Vergleichen", exact: true }).click();
  return page.getByRole("region", { name: "Sicherungsvergleich" });
}
test("comparison names both selected stands and direction", async ({
  page,
}) => {
  const requests: { from: string; to: string }[] = [];
  await page.route("**/api/backups/compare/file", (r) => {
    const body = r.request().postDataJSON();
    requests.push(body);
    return r.fulfill({
      json: comparison(body.path, {
        before_label: "Sicherung " + body.from,
        after_label: "Sicherung " + body.to,
        hunks: [
          {
            before_start: 1,
            after_start: 1,
            lines: [
              { kind: "remove", text: body.from + "\n", before_line: 1 },
              { kind: "add", text: body.to + "\n", after_line: 1 },
            ],
          },
        ],
      }),
    });
  });
  const view = await setup(page, [row("etc/hostname")]);
  await expect(
    view.getByText("Hostübergreifender Vergleich", { exact: true }),
  ).toBeVisible();
  await expect(view.locator(".comparison-direction")).toContainText("Quelle");
  await expect(view.locator(".comparison-direction")).toContainText("older");
  await expect(view.locator(".comparison-direction")).toContainText("Ziel");
  await view.getByRole("button", { name: /^etc\/hostname/ }).click();
  await expect(view.locator(".diff-remove")).toContainText("older");
  await view.getByRole("button", { name: "Richtung umkehren" }).click();
  await view.getByRole("button", { name: /^etc\/hostname/ }).click();
  await expect(view.locator(".diff-remove")).toContainText("newer");
  await expect(view.locator(".diff-add")).toContainText("older");
  expect(requests.map((r) => [r.from, r.to])).toEqual([
    ["older", "newer"],
    ["newer", "older"],
  ]);
});

test("comparison principal count excludes mtime-only noise", async ({
  page,
}) => {
  const view = await setup(page, [
    row("etc/content", { content_changed: true }),
    row("etc/time", {
      content_changed: false,
      mtime_changed: true,
      mtime_only: true,
    }),
  ]);
  await expect(view).toContainText("1 wesentliche Änderung");
  await expect(view).toContainText("1 nur Änderungszeit");
});
test("large summaries are filtered and paged with lazy content", async ({
  page,
}) => {
  const requests: string[] = [];
  await page.route("**/api/backups/compare/file", (r) => {
    const path = r.request().postDataJSON().path;
    requests.push(path);
    return r.fulfill({ json: comparison(path) });
  });
  const view = await setup(
    page,
    Array.from({ length: 1600 }, (_, i) =>
      row(
        `etc/config-${String(i).padStart(4, "0")}`,
        i === 1599
          ? { content_changed: false, metadata_changed: true }
          : i === 1598
            ? { content_changed: false, mtime_changed: true, mtime_only: true }
            : {},
      ),
    ),
  );
  await expect(view.locator(".comparison-items > button")).toHaveCount(25);
  expect(requests).toEqual([]);
  await view.getByRole("button", { name: "Nächste Seite" }).click();
  await expect(
    view.locator(".comparison-items > button").first(),
  ).toContainText("0025");
  await view.getByLabel("Änderungen pro Seite").selectOption("50");
  await expect(view.locator(".comparison-items > button")).toHaveCount(50);
  await view.getByLabel("Änderungsart").selectOption("metadata");
  await expect(view.locator(".comparison-items > button")).toHaveCount(1);
  await expect(view.locator(".comparison-items")).toContainText("1599");
  await view.getByLabel("Änderungsart").selectOption("mtime");
  await expect(view.locator(".comparison-items")).toContainText("1598");
  expect(requests).toEqual([]);
  await view.getByLabel("Änderungsart").selectOption("");
  await view.getByLabel("Änderungen durchsuchen").fill("1599");
  await view.getByRole("button", { name: /^etc\/config-1599/ }).click();
  await expect(view.locator(".comparison-preview")).toContainText("context");
  expect(requests).toEqual(["etc/config-1599"]);
});
test("late content response cannot replace current file", async ({ page }) => {
  let release!: () => void;
  const gate = new Promise<void>((r) => {
    release = r;
  });
  await page.route("**/api/backups/compare/file", async (r) => {
    const path = r.request().postDataJSON().path;
    if (path === "etc/slow") await gate;
    await r.fulfill({ json: comparison(path) });
  });
  const view = await setup(page, [row("etc/slow"), row("etc/current")]);
  await view.getByRole("button", { name: /^etc\/slow/ }).click();
  await view.getByRole("button", { name: /^etc\/current/ }).click();
  await expect(view.locator(".comparison-preview")).toContainText(
    "etc/current",
  );
  const response = page.waitForResponse(
    (r) =>
      r.url().endsWith("/compare/file") &&
      r.request().postDataJSON().path === "etc/slow",
  );
  release();
  await (await response).finished();
  await page.evaluate(() => new Promise(requestAnimationFrame));
  await expect(view.locator(".comparison-preview")).not.toContainText(
    "etc/slow",
  );
});
test("metadata binary errors and secrets remain distinct", async ({ page }) => {
  const states: Record<string, object> = {
    "etc/meta": {
      content_changed: false,
      hunks: [],
      metadata_changes: [{ field: "mode", before: 420, after: 384 }],
    },
    "etc/binary": { state: "binary", hunks: [] },
    "etc/secret": { state: "masked", masked: true, hunks: [] },
    "etc/large": { state: "too_large", hunks: [] },
    "etc/error": {
      state: "unavailable",
      error: "Prüfsumme stimmt nicht",
      hunks: [],
    },
  };
  const bodies: object[] = [];
  await page.route("**/api/backups/compare/file", (r) => {
    const body = r.request().postDataJSON();
    bodies.push(body);
    return r.fulfill({
      json: comparison(body.path, body.reveal ? {} : states[body.path]),
    });
  });
  const view = await setup(
    page,
    Object.keys(states).map((p) => row(p)),
  );
  await view.getByRole("button", { name: /^etc\/meta/ }).click();
  await expect(view.locator(".comparison-preview")).toContainText(
    "Nur Metadaten geändert",
  );
  await expect(view.locator(".comparison-preview")).toContainText("0644");
  await view.getByRole("button", { name: /^etc\/binary/ }).click();
  await expect(view.locator(".comparison-preview")).toContainText("Binärdatei");
  await view.getByRole("button", { name: /^etc\/secret/ }).click();
  await expect(view.locator(".comparison-preview")).toContainText(
    "Geschützter Inhalt",
  );
  await view
    .getByRole("button", { name: "Geschützten Inhalt anzeigen" })
    .click();
  await expect(view.locator(".diff-add")).toContainText("new");
  expect(bodies.at(-1)).toMatchObject({ path: "etc/secret", reveal: true });
  await view.getByRole("button", { name: /^etc\/large/ }).click();
  await expect(view.locator(".comparison-preview")).toContainText(
    "Vollständige Dateien herunterladen",
  );
  await view.getByRole("button", { name: /^etc\/error/ }).click();
  await expect(view.locator(".comparison-preview")).toContainText(
    "Prüfsumme stimmt nicht",
  );
  await expect(view.locator(".comparison-preview pre")).toHaveCount(0);
});

test("empty comparison and failed summary are visible and retryable", async ({
  page,
}) => {
  const view = await setup(page, []);
  await expect(
    view.getByText("Keine Unterschiede", { exact: true }),
  ).toBeVisible();
  await page.route("**/api/backups/compare", (r) =>
    r.fulfill({ status: 500, json: { error: "Manifest nicht lesbar" } }),
  );
  await view.getByRole("button", { name: "Vergleichen", exact: true }).click();
  await expect(view.getByRole("alert")).toContainText("Manifest nicht lesbar");
  await page.route("**/api/backups/compare", (r) =>
    r.fulfill({ json: [row("etc/retried")] }),
  );
  await view.getByRole("button", { name: "Erneut laden", exact: true }).click();
  await expect(view.locator(".comparison-items")).toContainText("etc/retried");
});

test("table selection and keyboard reach responsive diff without clipping", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => {
    if (m.type() === "error" || m.type() === "warning") errors.push(m.text());
  });
  await page.route("**/api/backups/compare/file", (r) =>
    r.fulfill({
      json: comparison(r.request().postDataJSON().path, {
        before_label: "Sicherung " + r.request().postDataJSON().from,
        after_label: "Sicherung " + r.request().postDataJSON().to,
      }),
    }),
  );
  const path =
    "etc/pve/nodes/a-very-long-hostname/qemu-server/100-custom-configuration.conf";
  const view = await setup(page, [row(path)]);
  await page.getByRole("button", { name: "Zurück zu Sicherungen" }).click();
  await page.getByRole("button", { name: /^Aktionen für Ziel/ }).click();
  await page
    .getByRole("button", { name: "Als Ausgangsstand", exact: true })
    .click();
  await view.getByLabel("Vergleich bis").selectOption("older");
  await expect(view.getByLabel("Vergleich von")).toHaveValue("newer");
  await view.getByRole("button", { name: "Vergleichen", exact: true }).click();
  const file = view.getByRole("button", { name: new RegExp("^" + path) });
  await file.focus();
  await page.keyboard.press("Enter");
  await expect(file).toBeFocused();
  await expect(view.locator(".diff-context")).toContainText("context");
  await expect(view.locator(".diff-remove .diff-symbol")).toHaveText("−");
  await expect(view.locator(".diff-add .diff-symbol")).toHaveText("+");
  for (const [width, height] of [
    [1440, 1000],
    [1280, 720],
    [390, 844],
  ]) {
    await page.setViewportSize({ width, height });
    await view.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: `/tmp/anker-task7-${width}.png`,
      fullPage: true,
    });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    if (width === 390) {
      const search = await view
        .getByLabel("Änderungen durchsuchen")
        .boundingBox();
      expect(search!.width).toBeGreaterThan(250);
    }
    const list = await view.locator(".comparison-list").boundingBox();
    const preview = await view.locator(".comparison-preview").boundingBox();
    if (width === 390) expect(preview!.y).toBeGreaterThan(list!.y);
  }
  await expect(page).toHaveTitle(/Anker/);
  expect(errors).toEqual([]);
});

test("reader cannot reveal protected content or start a restore", async ({
  page,
}) => {
  const me = await (await page.request.get("/api/me")).json();
  await page.route("**/api/me", (r) =>
    r.fulfill({
      json: { ...me, user: { ...me.user, role: "reader", secrets: false } },
    }),
  );
  await page.route("**/api/backups/compare/file", (r) =>
    r.fulfill({
      json: comparison("etc/protected", {
        state: "masked",
        masked: true,
        hunks: [],
        metadata_changes: [],
      }),
    }),
  );
  const view = await setup(page, [row("etc/protected", { secret: true })]);
  await view.getByRole("button", { name: /^etc\/protected/ }).click();
  await expect(
    view.getByText("Geschützter Inhalt", { exact: true }),
  ).toBeVisible();
  await expect(
    view.getByRole("button", { name: "Geschützten Inhalt anzeigen" }),
  ).toHaveCount(0);
  await expect(
    view.getByRole("button", { name: /Wiederherstellung wählen/ }),
  ).toHaveCount(0);
  await expect(view.getByRole("link", { name: /^Original/ })).toHaveCount(0);
});

test("changing the stand invalidates late summary responses", async ({
  page,
}) => {
  const view = await setup(page, [row("etc/initial")]);
  let release!: () => void;
  const gate = new Promise<void>((r) => {
    release = r;
  });
  await page.route("**/api/backups/compare", async (r) => {
    await gate;
    await r.fulfill({ json: [row("etc/stale")] });
  });
  await view.getByRole("button", { name: "Vergleichen", exact: true }).click();
  await view.getByLabel("Vergleich von").selectOption("");
  const response = page.waitForResponse((r) =>
    r.url().endsWith("/backups/compare"),
  );
  release();
  await (await response).finished();
  await page.evaluate(() => new Promise(requestAnimationFrame));
  await expect(view.locator(".comparison-items")).toHaveCount(0);
  await expect(
    view.getByRole("button", { name: "Vergleichen", exact: true }),
  ).toBeDisabled();
});

test("compact filters distinguish content added removed and protected files", async ({
  page,
}) => {
  const view = await setup(page, [
    row("etc/content"),
    row("etc/added", { change: "added" }),
    row("etc/removed", { change: "removed" }),
    row("etc/secret", { secret: true, content_changed: false }),
  ]);
  for (const [filter, paths] of [
    ["content", ["etc/content", "etc/added", "etc/removed"]],
    ["added", ["etc/added"]],
    ["removed", ["etc/removed"]],
    ["secret", ["etc/secret"]],
  ] as const) {
    await view.getByLabel("Änderungsart").selectOption(filter);
    await expect(view.locator(".comparison-items > button")).toHaveCount(
      paths.length,
    );
    for (const path of paths)
      await expect(view.locator(".comparison-items")).toContainText(path);
  }
});

test("missing comparison sides disable only their download and retain exact line numbers", async ({
  page,
}) => {
  await page.route("**/api/backups/compare/file", (r) => {
    const path = r.request().postDataJSON().path;
    return r.fulfill({
      json: comparison(path, {
        state: path === "etc/added" ? "missing_before" : "missing_after",
        hunks: [
          {
            before_start: 7,
            after_start: 11,
            lines: [
              {
                kind: "context",
                text: "context\n",
                before_line: 7,
                after_line: 11,
              },
              { kind: "remove", text: "old\n", before_line: 8 },
              { kind: "add", text: "new", after_line: 12 },
            ],
          },
        ],
      }),
    });
  });
  const view = await setup(page, [
    row("etc/added", { change: "added" }),
    row("etc/removed", { change: "removed" }),
  ]);
  await view.getByRole("button", { name: /^etc\/added/ }).click();
  await expect(
    view.getByRole("link", { name: "Original vorher", exact: true }),
  ).toHaveCount(0);
  await expect(
    view.getByRole("link", { name: "Original nachher", exact: true }),
  ).toBeVisible();
  await expect(view.locator(".diff-context .diff-number")).toHaveText([
    "7",
    "11",
  ]);
  await expect(view.locator(".diff-remove .diff-number")).toHaveText(["8", ""]);
  await expect(view.locator(".diff-add .diff-number")).toHaveText(["", "12"]);
  await expect(view.locator(".diff-add .diff-no-newline")).toHaveText(
    "Kein Zeilenumbruch am Dateiende",
  );
  await expect(view.locator(".diff-remove .diff-no-newline")).toHaveCount(0);
  await view.getByRole("button", { name: /^etc\/removed/ }).click();
  await expect(
    view.getByRole("link", { name: "Original nachher", exact: true }),
  ).toHaveCount(0);
  await expect(
    view.getByRole("link", { name: "Original vorher", exact: true }),
  ).toBeVisible();
});

for (const state of ["binary", "too_large"])
  test(`missing ${state} comparison sides retain original download presence including reveal`, async ({
    page,
  }) => {
    await page.route("**/api/backups/compare/file", (r) =>
      r.fulfill({
        json: comparison(r.request().postDataJSON().path, {
          state: r.request().postDataJSON().reveal ? state : "masked",
          masked: !r.request().postDataJSON().reveal,
        }),
      }),
    );
    const view = await setup(page, [
      row("etc/added", { change: "added", secret: true }),
      row("etc/removed", { change: "removed", secret: true }),
    ]);
    for (const [path, absent, present] of [
      ["added", "Original vorher", "Original nachher"],
      ["removed", "Original nachher", "Original vorher"],
    ]) {
      await view
        .getByRole("button", { name: new RegExp(`^etc/${path}`) })
        .click();
      await expect(
        view.getByRole("link", { name: absent, exact: true }),
      ).toHaveCount(0);
      await view
        .getByRole("button", {
          name: "Geschützten Inhalt anzeigen",
          exact: true,
        })
        .click();
      await expect(
        view.getByRole("link", { name: absent, exact: true }),
      ).toHaveCount(0);
      await expect(
        view.getByRole("link", { name: present, exact: true }),
      ).toBeVisible();
    }
  });
