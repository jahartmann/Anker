import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";
test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});
async function start(page: Page, scenario = "files") {
  const base = await (await page.request.get("/api/status")).json();
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  await expect(
    page.getByRole("navigation", { name: "Vorbereitungsschritte" }),
  ).toBeVisible();
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[0].id);
  await page.getByLabel("Situation", { exact: true }).selectOption(scenario);
  if (scenario === "files")
    await page
      .getByLabel("etc/sysctl.d/99-anker.conf", { exact: true })
      .check();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[0].id);
  return base;
}
async function inspect(page: Page) {
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
}
test("four situations retain draft through back navigation", async ({
  page,
}) => {
  for (const scenario of [
    "files",
    "host",
    "cluster-node",
    "cluster-disaster",
  ]) {
    const base = await start(page, scenario);
    await inspect(page);
    await page
      .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", {
        exact: true,
      })
      .selectOption("keep");
    await page.getByRole("button", { name: "Zurück", exact: true }).click();
    await expect(page.getByLabel("Zielhost", { exact: true })).toHaveValue(
      base.hosts[0].id,
    );
    await page
      .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
      .click();
    await expect(
      page.getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", {
        exact: true,
      }),
    ).toHaveValue("keep");
  }
});
test("connect target returns to same recovery draft", async ({ page }) => {
  const base = await start(page);
  await page
    .getByRole("button", { name: "Neues Ziel anbinden", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Host hinzufügen", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Dialog schließen", exact: true })
    .click();
  await expect(page.getByLabel("Zielhost", { exact: true })).toHaveValue(
    base.hosts[0].id,
  );
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await expect(
    page.getByLabel("etc/sysctl.d/99-anker.conf", { exact: true }),
  ).toBeChecked();
});
test("context change invalidates only dependent decisions", async ({
  page,
}) => {
  const base = await start(page);
  await inspect(page);
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("backup");
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[1].id);
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeDisabled();
  await inspect(page);
  await expect(
    page.getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", {
      exact: true,
    }),
  ).toHaveValue("backup");
});
test("late inspection cannot replace new target", async ({ page }) => {
  const base = await start(page);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/plans/inspect", async (route) => {
    const response = await route.fetch();
    await gate;
    await route.fulfill({ response });
  });
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[1].id);
  const late = page.waitForResponse((r) =>
    r.url().endsWith("/api/plans/inspect"),
  );
  release();
  await (await late).finished();
  await expect(
    page.getByRole("button", { name: "Ziel prüfen", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeDisabled();
});
test("blocked step shows next concrete action", async ({ page }) => {
  await start(page);
  await inspect(page);
  await expect(
    page.getByText("Für jede Datei eine Entscheidung auswählen.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Weiter zur Planprüfung", exact: true }),
  ).toBeDisabled();
});
test("ordinary files avoid irrelevant mappings", async ({ page }) => {
  await start(page);
  await inspect(page);
  await expect(page.locator(".recovery-mapping")).toHaveCount(0);
  await expect(
    page.getByLabel("Gewünschter Hostname", { exact: true }),
  ).toHaveCount(0);
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("backup");
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  const request = page.waitForRequest(
    (r) => r.url().endsWith("/api/plans") && r.method() === "POST",
  );
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  const body = (await request).postDataJSON();
  expect(body.decisions).toEqual({ "etc/sysctl.d/99-anker.conf": "backup" });
  expect(body.snapshot_id).toBeTruthy();
  await expect(
    page.getByRole("dialog", { name: "Wiederherstellungsplan", exact: true }),
  ).toBeVisible();
});

test("all four situations save explicit decisions through real demo endpoints", async ({
  page,
}) => {
  for (const scenario of [
    "files",
    "host",
    "cluster-node",
    "cluster-disaster",
  ]) {
    await start(page, scenario);
    await inspect(page);
    for (const select of await page.locator(".recovery-mapping select").all()) {
      const options = await select
        .locator("option")
        .evaluateAll((nodes) =>
          nodes.map((n) => (n as HTMLOptionElement).value),
        );
      await select.selectOption(
        options.includes("manual") ? "manual" : options[1],
      );
    }
    await page
      .getByRole("button", {
        name: "Offene Dateien manuell bearbeiten",
        exact: true,
      })
      .click();
    const recheck = page.getByRole("button", {
      name: "Zuordnungen prüfen",
      exact: true,
    });
    if (await recheck.count()) await recheck.click();
    await page
      .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
      .click();
    for (const checkbox of await page
      .getByRole("region", { name: "Planprüfung", exact: true })
      .getByRole("checkbox")
      .all())
      await checkbox.check();
    const response = page.waitForResponse(
      (r) => r.url().endsWith("/api/plans") && r.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "Plan prüfen", exact: true })
      .click();
    const created = await response;
    expect(created.ok()).toBeTruthy();
    const plan = await created.json();
    expect(plan.scenario).toBe(scenario);
    expect(
      Object.values(created.request().postDataJSON().decisions).every(
        (v) => v === "manual",
      ),
    ).toBe(true);
    expect(
      Object.values(plan.decisions).every(
        (v) => v === "manual" || v === "keep",
      ),
    ).toBe(true);
    expect(
      plan.steps.every((s: { action: string }) => s.action !== "apply"),
    ).toBe(true);
    await expect(
      page.getByRole("dialog", { name: "Wiederherstellungsplan", exact: true }),
    ).toBeVisible();
  }
});

test("administrator connects a new demo target and returns without storing credentials", async ({
  page,
}) => {
  const base = await start(page);
  await page
    .getByRole("button", { name: "Neues Ziel anbinden", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Host hinzufügen",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Manuell einrichten", exact: true })
    .click();
  await dialog
    .getByLabel("Hostname", { exact: true })
    .fill("recovery-draft-target");
  await dialog.getByLabel("Adresse", { exact: true }).fill("192.0.2.240");
  await dialog
    .getByRole("button", { name: "Host speichern", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  const target = page.getByLabel("Zielhost", { exact: true });
  await expect(target).toHaveValue(base.hosts[0].id);
  await expect(
    target.getByRole("option", { name: "recovery-draft-target", exact: true }),
  ).toHaveCount(1);
  await target.selectOption({ label: "recovery-draft-target" });
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await expect(
    page.getByLabel("etc/sysctl.d/99-anker.conf", { exact: true }),
  ).toBeChecked();
  const local = await page.evaluate(() => JSON.stringify({ ...localStorage }));
  expect(local).not.toMatch(
    /recovery-draft-target|password|snapshot_id|decisions|192\.0\.2\.240/,
  );
});

test("restore role receives connection guidance without administrator controls", async ({
  page,
}) => {
  const me = await (await page.request.get("/api/me")).json();
  await page.route("**/api/me", (route) =>
    route.fulfill({
      json: { ...me, user: { ...me.user, role: "restore", secrets: false } },
    }),
  );
  await start(page);
  await expect(
    page.getByRole("button", { name: "Neues Ziel anbinden", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByText(/Ein Administrator muss ein neues Ziel/),
  ).toBeVisible();
  await inspect(page);
  await expect(
    page.getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", {
      exact: true,
    }),
  ).toBeVisible();
});

test("recovery workspace stays readable with keyboard at desktop laptop and mobile sizes", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => {
    if (["error", "warning"].includes(m.type())) errors.push(m.text());
  });
  await start(page);
  await inspect(page);
  await expect(page).toHaveTitle("Wiederherstellung · Anker");
  expect(new URL(page.url()).origin).toBe("http://127.0.0.1:8088");
  await expect(page.locator("vite-error-overlay")).toHaveCount(0);
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("keep");
  const next = page.getByRole("button", {
    name: "Weiter zur Planprüfung",
    exact: true,
  });
  await next.focus();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("heading", {
      name: "4 · Plan prüfen und speichern",
      exact: true,
    }),
  ).toBeVisible();
  for (const [name, width, height] of [
    ["desktop", 1440, 1000],
    ["laptop", 1024, 768],
    ["mobile", 390, 844],
  ] as const) {
    await page.setViewportSize({ width, height });
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    await page
      .getByRole("button", { name: "Plan prüfen", exact: true })
      .scrollIntoViewIfNeeded();
    const footer = await page.locator(".recovery-footer").boundingBox();
    expect(footer).not.toBeNull();
    expect(footer!.y + footer!.height).toBeLessThanOrEqual(height);
    await page.screenshot({
      path: `/tmp/anker-task8-${name}.png`,
      fullPage: false,
    });
  }
  expect(errors).toEqual([]);
});

test("fresh changed target evidence invalidates dependent file choice", async ({
  page,
}) => {
  await start(page);
  let inspections = 0;
  await page.route("**/api/plans/inspect", async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    if (++inspections > 1)
      body.files = body.files.map((f: { path: string }) =>
        f.path === "etc/sysctl.d/99-anker.conf"
          ? { ...f, sha256: "changed-evidence" }
          : f,
      );
    await route.fulfill({ response, json: body });
  });
  await inspect(page);
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("keep");
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await inspect(page);
  await expect(
    page.getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", {
      exact: true,
    }),
  ).toHaveValue("");
  await expect(
    page.getByRole("button", { name: "Weiter zur Planprüfung", exact: true }),
  ).toBeDisabled();
});

test("starting another plan returns to source selection with its draft", async ({ page }) => {
  await start(page);
  await inspect(page);
  await page.getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true }).selectOption("keep");
  await page.getByRole("button", { name: "Weiter zur Planprüfung", exact: true }).click();
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await page.getByRole("button", { name: "Dialog schließen", exact: true }).click();
  await page.getByRole("button", { name: "Plan erstellen", exact: true }).click();
  await expect(page.getByRole("heading", { name: "1 · Quelle und Situation", exact: true })).toBeVisible();
  await expect(page.getByLabel("etc/sysctl.d/99-anker.conf", { exact: true })).toBeChecked();
});
