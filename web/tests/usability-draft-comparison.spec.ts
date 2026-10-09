import { test, expect, type Page } from "@playwright/test";
// Dedicated fresh demo server: other suites restore files and create backups.
test.beforeEach(async ({ page, request }) => {
  const login = await request.post("/api/login", {
    headers: { "X-Anker-Request": "1" },
    data: { name: "demo", password: "anker-demo-2026" },
  });
  expect(login.ok()).toBeTruthy();
  await page.context().addCookies((await request.storageState()).cookies);
});
async function decisions(page: Page, paths = ["etc/sysctl.d/99-anker.conf"]) {
  const base = await (await page.request.get("/api/status")).json();
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[0].id);
  for (const path of paths) {
    await page.getByLabel("Wiederherstellungsdateien durchsuchen").fill(path);
    await page.getByLabel(path, { exact: true }).check();
  }
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[0].id);
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  return base;
}
test("lazy draft comparison exposes original target and prepared evidence before any choice", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const paths = ["etc/hostname", "etc/sysctl.d/99-anker.conf"];
  let requests = 0;
  page.on("request", (r) => {
    if (r.url().endsWith("/api/plans/preview")) requests++;
  });
  await decisions(page, paths);
  expect(requests).toBe(0);
  await page.getByLabel("Gewünschter Hostname").fill("preview-host");
  await page
    .getByRole("button", { name: "Vergleich für etc/hostname", exact: true })
    .click();
  const region = page.getByRole("region", {
    name: "Dateivergleich für etc/hostname",
  });
  await expect(region.locator(".file-comparison")).toBeVisible();
  await expect(
    page.getByLabel("Entscheidung für etc/hostname", { exact: true }),
  ).toHaveValue("");
  await region.getByLabel("Vergleichsbasis").selectOption("source");
  await expect(
    region.getByRole("region", { name: "Zeilenvergleich" }),
  ).toContainText("preview-host");
  await expect(region).toContainText("Voraussichtliche Aktion");
  await region.getByLabel("Vergleichsbasis").selectOption("target");
  await expect(region.locator(".comparison-labels")).toContainText(
    "Ziel vor Übernahme → Vorbereitet",
  );
  await page
    .getByRole("button", {
      name: "Vergleich für etc/sysctl.d/99-anker.conf",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("region", {
      name: "Dateivergleich für etc/sysctl.d/99-anker.conf",
    }),
  ).toContainText("net.ipv4.ip_forward=0");
  const unchanged = page.getByRole("region", {
    name: "Dateivergleich für etc/sysctl.d/99-anker.conf",
  });
  // Prepared ordinary file preserves the original; both identical and target Diff
  // are asserted against the real service, without response mocks.
  await unchanged.getByLabel("Vergleichsbasis").selectOption("source");
  await expect(unchanged).toContainText("Inhalt und Metadaten stimmen überein");
  await unchanged.getByLabel("Vergleichsbasis").selectOption("target");
  await expect(unchanged).toContainText("net.ipv4.ip_forward=0");
  for (const [name, width, height] of [
    ["desktop", 1440, 1000],
    ["mobile", 390, 844],
  ] as const) {
    await page.setViewportSize({ width, height });
    await expect(page.locator("body")).toHaveJSProperty("scrollWidth", width);
    await region.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: `/tmp/anker-task4-${name}.png`,
      fullPage: true,
    });
  }
  expect(errors).toEqual([]);
});

test("mapping edit clears evidence and a delayed response cannot repopulate it", async ({
  page,
}) => {
  await decisions(page, ["etc/hostname"]);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let entered!: () => void;
  const waiting = new Promise<void>((resolve) => {
    entered = resolve;
  });
  await page.route(
    "**/api/plans/preview",
    async (route) => {
      const response = await route.fetch();
      entered();
      await gate;
      await route.fulfill({ response });
    },
    { times: 1 },
  );
  await page.getByLabel("Gewünschter Hostname").fill("old-preview-host");
  await page
    .getByRole("button", { name: "Vergleich für etc/hostname", exact: true })
    .click();
  await waiting;
  await page.getByLabel("Gewünschter Hostname").fill("new-preview-host");
  release();
  await expect(
    page.getByRole("region", { name: "Dateivergleich für etc/hostname" }),
  ).not.toBeVisible();
  await page
    .getByRole("button", { name: "Vergleich für etc/hostname", exact: true })
    .click();
  const region = page.getByRole("region", {
    name: "Dateivergleich für etc/hostname",
  });
  await region.getByLabel("Vergleichsbasis").selectOption("source");
  await expect(region).toContainText("new-preview-host");
  await expect(region).not.toContainText("old-preview-host");
  await page
    .getByLabel("Entscheidung für etc/hostname", { exact: true })
    .selectOption("prepared");
  await page.getByLabel("Gewünschter Hostname").fill("another-host");
  await expect(
    page.getByLabel("Entscheidung für etc/hostname", { exact: true }),
  ).toHaveValue("");
});

test("target and source changes discard comparison and file choices", async ({
  page,
}) => {
  const base = await decisions(page);
  const path = "etc/sysctl.d/99-anker.conf";
  await page
    .getByRole("button", { name: "Vergleich für " + path, exact: true })
    .click();
  await expect(
    page
      .getByRole("region", { name: "Dateivergleich für " + path })
      .locator(".file-comparison"),
  ).toBeVisible();
  await page
    .getByLabel("Entscheidung für " + path, { exact: true })
    .selectOption("backup");
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[1].id);
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await expect(
    page.getByLabel("Entscheidung für " + path, { exact: true }),
  ).toHaveValue("");
  await expect(
    page.getByRole("region", { name: "Dateivergleich für " + path }),
  ).not.toBeVisible();
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[1].id);
  await page.getByLabel("Wiederherstellungsdateien durchsuchen").fill(path);
  await expect(page.getByLabel(path, { exact: true })).not.toBeChecked();
});
