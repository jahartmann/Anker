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
  if (scenario === "files") {
    await page
      .getByLabel("Wiederherstellungsdateien durchsuchen")
      .fill("etc/sysctl.d/99-anker.conf");
    await page
      .getByLabel("etc/sysctl.d/99-anker.conf", { exact: true })
      .check();
  }
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
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("etc/sysctl.d/99-anker.conf");
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
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("etc/sysctl.d/99-anker.conf");
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

test("starting another plan returns to source selection with its draft", async ({
  page,
}) => {
  await start(page);
  await inspect(page);
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("keep");
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Dialog schließen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "1 · Quelle und Situation",
      exact: true,
    }),
  ).toBeVisible();
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("etc/sysctl.d/99-anker.conf");
  await expect(
    page.getByLabel("etc/sysctl.d/99-anker.conf", { exact: true }),
  ).toBeChecked();
});

test("saved plan comparison and checks remain readable with keyboard at desktop laptop and mobile sizes", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => {
    if (["error", "warning"].includes(m.type())) errors.push(m.text());
  });
  await start(page);
  await inspect(page);
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("keep");
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Wiederherstellungsplan",
    exact: true,
  });
  await expect(dialog).toBeVisible();
  await page
    .getByRole("button", { name: "Meldung schließen", exact: true })
    .click();
  const compare = dialog.getByRole("button", {
    name: "Vergleich öffnen: etc/sysctl.d/99-anker.conf",
    exact: true,
  });
  await compare.focus();
  await page.keyboard.press("Enter");
  await expect(
    dialog.getByText("Inhalt und Metadaten stimmen überein", { exact: true }),
  ).toBeVisible();
  await expect(page).toHaveTitle("Wiederherstellung · Anker");
  expect(new URL(page.url()).origin).toBe("http://127.0.0.1:8088");
  await expect(page.locator("vite-error-overlay")).toHaveCount(0);
  for (const [name, width, height] of [
    ["desktop", 1440, 1000],
    ["laptop", 1280, 900],
    ["mobile", 390, 844],
  ] as const) {
    await page.setViewportSize({ width, height });
    await dialog
      .getByLabel("Vergleichsbasis", { exact: true })
      .scrollIntoViewIfNeeded();
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    const box = await dialog.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    await page.screenshot({
      path: `/tmp/anker-task9-${name}-comparison.png`,
      fullPage: false,
    });
    await dialog
      .getByLabel("Nachweis für Dienste prüfen", { exact: true })
      .scrollIntoViewIfNeeded();
    await page.screenshot({
      path: `/tmp/anker-task9-${name}-checks.png`,
      fullPage: false,
    });
  }
  await dialog
    .getByRole("button", { name: "Dialog schließen", exact: true })
    .focus();
  await page.keyboard.press("Tab");
  await expect(dialog.locator("summary").first()).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(
    dialog.getByLabel("Plandateien durchsuchen", { exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("revising a freshly saved plan retains its controlled draft and requires fresh target inspection", async ({
  page,
}) => {
  const base = await start(page);
  await inspect(page);
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("keep");
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Zuordnungen anpassen", exact: true })
    .click();
  await expect(page.getByLabel("Zielhost", { exact: true })).toHaveValue(
    base.hosts[0].id,
  );
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeDisabled();
  await inspect(page);
  await expect(
    page.getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", {
      exact: true,
    }),
  ).toHaveValue("keep");
});

test("real demo task confirmations remain user attestations including reboot", async ({
  page,
}) => {
  await start(page);
  await inspect(page);
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("keep");
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  const created = page.waitForResponse(
    (r) => r.url().endsWith("/api/plans") && r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  const plan = await (await created).json();
  for (const title of [
    "Dienste prüfen",
    "Erreichbarkeit prüfen",
    "Neustart prüfen",
  ]) {
    const response = page.waitForResponse((r) =>
      r.url().includes(`/api/plans/${plan.id}/checks/`),
    );
    await page
      .getByLabel(`Nachweis für ${title}`, { exact: true })
      .fill(`${title}: an lokaler Demo geprüft`);
    await page
      .getByRole("button", {
        name: `Schritt bestätigen: ${title}`,
        exact: true,
      })
      .click();
    const confirmed = await response;
    expect(confirmed.ok()).toBe(true);
    const body = await confirmed.json();
    expect(body.result?.reboot_verified || false).toBe(false);
    expect(Object.keys(confirmed.request().postDataJSON())).toEqual([
      "evidence",
    ]);
  }
  const saved = await (await page.request.get(`/api/plans/${plan.id}`)).json();
  expect(
    saved.tasks.every((t: { state: string }) => t.state === "user_confirmed"),
  ).toBe(true);
  expect(saved.state).toBe("manual");
  await expect(page.getByRole("dialog")).toContainText(
    "Neustart maschinell nicht nachgewiesen",
  );
  await expect(
    page.getByText(/Herkunft: Benutzerbestätigung, kein maschineller Nachweis/),
  ).toHaveCount(3);
});

test("cleared decision becomes manual through bulk action", async ({
  page,
}) => {
  await start(page);
  await inspect(page);
  const choice = page.getByLabel(
    "Entscheidung für etc/sysctl.d/99-anker.conf",
    { exact: true },
  );
  await choice.selectOption("keep");
  await choice.selectOption("");
  await page
    .getByRole("button", {
      name: "Offene Dateien manuell bearbeiten",
      exact: true,
    })
    .click();
  await expect(choice).toHaveValue("manual");
  await expect(
    page.getByRole("button", { name: "Weiter zur Planprüfung", exact: true }),
  ).toBeEnabled();
});

test("kept network and storage prune only their mappings and recapture storage binding", async ({
  page,
}) => {
  await start(page);
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  for (const path of ["etc/network/interfaces", "etc/pve/storage.cfg"]) {
    await page.getByLabel("Wiederherstellungsdateien durchsuchen").fill(path);
    await page.getByLabel(path, { exact: true }).check();
  }
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await inspect(page);
  for (const select of await page.locator(".recovery-mapping select").all()) {
    const options = await select
      .locator("option")
      .evaluateAll((nodes) => nodes.map((n) => (n as HTMLOptionElement).value));
    await select.selectOption(
      options.includes("manual") ? "manual" : options[1],
    );
  }
  await page
    .getByRole("button", { name: "Zuordnungen prüfen", exact: true })
    .click();
  for (const path of ["etc/network/interfaces", "etc/pve/storage.cfg"])
    await page
      .getByLabel(`Entscheidung für ${path}`, { exact: true })
      .selectOption("keep");
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("backup");
  await expect(page.locator(".recovery-mapping select")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Weiter zur Planprüfung", exact: true }),
  ).toHaveCount(0);
  const inspection = page.waitForRequest(
    (r) => r.url().endsWith("/api/plans/inspect") && r.method() === "POST",
  );
  await page
    .getByRole("button", { name: "Zuordnungen prüfen", exact: true })
    .click();
  expect((await inspection).postDataJSON().mapping.storage).toEqual({});
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  for (const box of await page
    .getByRole("region", { name: "Planprüfung", exact: true })
    .getByRole("checkbox")
    .all())
    await box.check();
  const saved = page.waitForResponse(
    (r) => r.url().endsWith("/api/plans") && r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  const response = await saved;
  expect(response.ok()).toBe(true);
  const body = response.request().postDataJSON();
  expect(body.mapping.interfaces).toEqual({});
  expect(body.mapping.storage).toEqual({});
  expect((await response.json()).blockers).toEqual([]);
});

test("typed source network stanza prepares effects and remains manual with keyboard and narrow layouts", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await start(page);
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("etc/network/interfaces");
  await page.getByLabel("etc/network/interfaces", { exact: true }).check();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await inspect(page);
  for (const select of await page.locator(".recovery-mapping select").all())
    await select.selectOption({ index: 1 });
  await page
    .getByLabel("Entscheidung für etc/network/interfaces", { exact: true })
    .selectOption("prepared");
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("keep");
  await page.getByLabel("Stanza vmbr0/inet anpassen", { exact: true }).focus();
  await page.keyboard.press("Space");
  await page
    .getByLabel("Adresse/CIDR für vmbr0/inet", { exact: true })
    .fill("198.51.100.9/24");
  await page
    .getByLabel("Gateway für vmbr0/inet", { exact: true })
    .fill("198.51.100.1");
  await page
    .getByLabel("DNS-Server für vmbr0/inet", { exact: true })
    .fill("192.0.2.53,2001:db8::53");
  for (const width of [1440, 1280, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await page
      .getByLabel("Adresse/CIDR für vmbr0/inet", { exact: true })
      .scrollIntoViewIfNeeded();
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `/tmp/anker-final-fix-evidence/network-${width}.png`,
    });
  }
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  await page.getByLabel("Konsolenzugang zum Ziel ist verfügbar").check();
  const saved = page.waitForResponse(
    (r) => r.url().endsWith("/api/plans") && r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  const response = await saved;
  expect(response.ok()).toBe(true);
  expect(response.request().postDataJSON().mapping.network).toEqual({
    "vmbr0/inet": {
      address_cidr: "198.51.100.9/24",
      gateway: "198.51.100.1",
      dns_servers: ["192.0.2.53", "2001:db8::53"],
    },
  });
  const plan = await response.json();
  const step = plan.steps.find((s: any) => s.path === "etc/network/interfaces");
  expect(step.action).toBe("manual");
  expect(step.effects.length).toBeGreaterThan(0);
  const comparison = await (
    await page.request.get(
      `/api/plans/${plan.id}/comparison?path=etc/network/interfaces&basis=source`,
    )
  ).json();
  const changes = comparison.hunks
    .flatMap((h: any) => h.lines)
    .filter((l: any) => l.kind !== "context")
    .map((l: any) => l.text)
    .join("");
  expect(changes).toContain("address 198.51.100.9/24");
  expect(changes).not.toContain("iface lo inet loopback");
  expect(errors).toEqual([]);
});
