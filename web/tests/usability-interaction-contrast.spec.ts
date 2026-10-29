import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

// Evaluate the painted foreground against the composited ancestor backgrounds,
// rather than testing a particular CSS declaration or a named colour.
async function contrastIssues(target: ReturnType<Page["locator"]>) {
  return target.evaluate((root) => {
    const rgba = (value: string) => {
      const values = value.match(/[\d.]+/g)!.map(Number);
      // CSS color-mix serializes as color(srgb ...) with normalized channels.
      const scale = value.startsWith("color(srgb ") ? 255 : 1;
      return [
        values[0] * scale,
        values[1] * scale,
        values[2] * scale,
        values[3] ?? 1,
      ];
    };
    const blend = (front: number[], back: number[]) =>
      front
        .slice(0, 3)
        .map((v, i) => v * front[3] + back[i] * (1 - front[3]))
        .concat(1);
    const background = (element: Element) => {
      const chain: Element[] = [];
      for (
        let current: Element | null = element;
        current;
        current = current.parentElement
      )
        chain.unshift(current);
      return chain.reduce(
        (color, node) =>
          blend(rgba(getComputedStyle(node).backgroundColor), color),
        [255, 255, 255, 1],
      );
    };
    const luminance = (color: number[]) =>
      color
        .slice(0, 3)
        .map((v) => v / 255)
        .map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4))
        .reduce((sum, v, i) => sum + v * [0.2126, 0.7152, 0.0722][i], 0);
    const ratio = (front: number[], back: number[]) => {
      const a = luminance(blend(front, back));
      const b = luminance(back);
      return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
    };
    const issues: {
      text: string;
      ratio: number;
      minimum: number;
      foreground: string;
      background: number[];
    }[] = [];
    let checkedText = false;
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
      const node = walker.currentNode;
      const element = node.parentElement!;
      if (
        !node.textContent?.trim() ||
        !element.getClientRects().length ||
        element.closest('.sr-only, [aria-hidden="true"]')
      )
        continue;
      const style = getComputedStyle(element);
      if (style.visibility === "hidden" || style.display === "none") continue;
      checkedText = true;
      const size = parseFloat(style.fontSize);
      const minimum =
        size >= 24 || (size >= 18.66 && parseInt(style.fontWeight) >= 700)
          ? 3
          : 4.5;
      const back = background(element);
      const value = ratio(rgba(style.color), back);
      if (value < minimum)
        issues.push({
          text: node.textContent.trim(),
          ratio: Math.round(value * 100) / 100,
          minimum,
          foreground: style.color,
          background: back,
        });
    }
    if (!checkedText) {
      const icon = root.querySelector("svg");
      if (icon) {
        const stroke = getComputedStyle(icon).stroke;
        if (stroke !== "none") {
          const back = background(icon);
          const value = ratio(rgba(stroke), back);
          if (value < 3)
            issues.push({
              text: root.getAttribute("aria-label") || "Icon",
              ratio: value,
              minimum: 3,
              foreground: stroke,
              background: back,
            });
        }
      }
    }
    return issues;
  });
}

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
  await page.goto("/");
  await page.addStyleTag({
    content:
      "*, *::before, *::after { transition: none !important; animation: none !important; }",
  });
});

test("all overview numbers retain readable contrast on hover and keyboard focus", async ({
  page,
}, testInfo) => {
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  const metrics = page.locator(".overview-metric-link");
  for (let index = 0; index < (await metrics.count()); index++) {
    const metric = metrics.nth(index);
    await metric.hover();
    expect(
      await contrastIssues(metric),
      `Hover: ${await metric.getAttribute("aria-label")}`,
    ).toEqual([]);
    await page.mouse.move(0, 0);
    await page.keyboard.press("Tab");
    await metric.focus();
    expect(
      await contrastIssues(metric),
      `Fokus: ${await metric.getAttribute("aria-label")}`,
    ).toEqual([]);
  }
  await metrics.first().hover();
  await page.screenshot({
    path: testInfo.outputPath("overview-hover.png"),
    fullPage: true,
  });
});

async function auditControls(
  page: Page,
  view: string,
  scope = page.locator("body"),
) {
  const failures: {
    view: string;
    state: string;
    control: string;
    issues: Awaited<ReturnType<typeof contrastIssues>>;
  }[] = [];
  const controls = scope.locator(
    "button:visible:not(:disabled), a.button:visible, a.text-button:visible, summary:visible",
  );
  const count = await controls.count();
  for (let index = 0; index < count; index++) {
    const control = controls.nth(index);
    const label =
      (await control.getAttribute("aria-label")) ||
      (await control.textContent())?.trim() ||
      "Icon";
    await control.hover();
    const hovered = await contrastIssues(control);
    if (hovered.length)
      failures.push({ view, state: "hover", control: label, issues: hovered });
    await page.mouse.move(0, 0);
    await page.keyboard.press("Tab");
    await control.focus();
    const focused = await contrastIssues(control);
    if (focused.length)
      failures.push({ view, state: "focus", control: label, issues: focused });
  }
  return { count, failures };
}

test("visible controls across the main views keep contrast through hover and focus", async ({
  page,
}, testInfo) => {
  test.setTimeout(120000);
  const results = [];
  for (const view of [
    "Übersicht",
    "Hosts",
    "Sicherungen",
    "Wiederherstellung",
    "Aufträge",
    "Einstellungen",
  ]) {
    await page
      .getByRole("navigation", { name: "Hauptnavigation" })
      .getByRole("button", { name: view, exact: true })
      .click();
    await expect(
      page.getByRole("heading", { name: view, exact: true }),
    ).toBeVisible();
    results.push(await auditControls(page, view));
  }
  await testInfo.attach("contrast-audit-main-views", {
    body: JSON.stringify(results, null, 2),
    contentType: "application/json",
  });
  expect(
    results
      .flatMap((result) => result.failures)
      .map((f) => ({
        view: f.view,
        state: f.state,
        control: f.control,
        issues: f.issues,
      })),
  ).toEqual([]);
});

test("host tabs, archive actions, file browser and settings tabs keep interaction contrast", async ({
  page,
}, testInfo) => {
  test.setTimeout(120000);
  const results = [];
  await page
    .getByRole("button", { name: "pve-berlin-01", exact: true })
    .click();
  for (const view of [
    "Übersicht",
    "Sicherungen",
    "Änderungen",
    "Inventar",
    "Einstellungen",
  ]) {
    await page
      .locator(".host-tabs")
      .getByRole("button", { name: view, exact: true })
      .click();
    if (
      view === "Sicherungen" &&
      (await page.locator(".history-month > summary").count())
    ) {
      await page.locator(".history-month > summary").first().click();
      await page.locator(".history-day > summary").first().click();
    }
    results.push(await auditControls(page, "Host / " + view));
  }
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Alle Stände", exact: true }).click();
  await page
    .getByRole("button", { name: /^Aktionen für/ })
    .first()
    .click();
  results.push(
    await auditControls(
      page,
      "Archiv / Aktionen",
      page.locator(".action-popover"),
    ),
  );
  await page.keyboard.press("Escape");
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  results.push(
    await auditControls(page, "Dateibrowser", page.getByRole("dialog")),
  );
  await page
    .getByRole("button", { name: "Dialog schließen", exact: true })
    .click();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  const tabs = await page.locator(".settings-tabs button").allTextContents();
  for (const view of tabs) {
    await page
      .locator(".settings-tabs")
      .getByRole("button", { name: view.trim(), exact: true })
      .click();
    results.push(await auditControls(page, "Einstellungen / " + view));
  }
  await testInfo.attach("contrast-audit-detail-views", {
    body: JSON.stringify(results, null, 2),
    contentType: "application/json",
  });
  expect(
    results
      .flatMap((result) => result.failures)
      .map((f) => ({
        view: f.view,
        state: f.state,
        control: f.control,
        issues: f.issues,
      })),
  ).toEqual([]);
});

test("setup, recovery preparation and destructive confirmation controls remain readable", async ({
  page,
}, testInfo) => {
  test.setTimeout(60000);
  const results = [];
  await page
    .getByRole("button", { name: "Host hinzufügen", exact: true })
    .click();
  results.push(
    await auditControls(page, "Host hinzufügen", page.getByRole("dialog")),
  );
  await page
    .getByRole("button", { name: "Dialog schließen", exact: true })
    .click();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  results.push(
    await auditControls(
      page,
      "Wiederherstellung vorbereiten",
      page.locator(".recovery-workspace"),
    ),
  );
  await page
    .getByRole("button", { name: "Vorbereitung schließen", exact: true })
    .click();
  // The account exists only in this intercepted read response. The confirmation
  // is inspected without submitting it or changing any account.
  await page.route("**/api/users", (route) =>
    route.fulfill({
      json: [
        {
          id: "contrast-fixture",
          name: "Kontrast-Testzugang",
          role: "reader",
          secrets: false,
          disabled: false,
        },
      ],
    }),
  );
  let mutations = 0;
  await page.route("**/api/users/contrast-fixture", (route) => {
    mutations++;
    return route.abort();
  });
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page
    .locator(".settings-tabs")
    .getByRole("button", { name: "Zugriff", exact: true })
    .click();
  await page
    .getByRole("button", {
      name: "Aktionen für Kontrast-Testzugang",
      exact: true,
    })
    .click();
  await page
    .getByRole("button", { name: "Benutzer löschen", exact: true })
    .click();
  results.push(
    await auditControls(page, "Löschbestätigung", page.getByRole("dialog")),
  );
  await page.getByRole("button", { name: "Abbrechen", exact: true }).click();
  expect(mutations).toBe(0);
  await testInfo.attach("contrast-audit-dialogs", {
    body: JSON.stringify(results, null, 2),
    contentType: "application/json",
  });
  expect(results.flatMap((result) => result.failures)).toEqual([]);
});

test("compass task cards, host choices and contextual actions retain contrast", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Anker Kompass öffnen" }).click();
  const dialog = page.getByRole("dialog", { name: "Anker Kompass" });
  const tasks = await auditControls(page, "Kompass Aufgaben", dialog);
  await dialog.getByRole("button", { name: /Änderungen verfolgen/ }).click();
  const hosts = await auditControls(page, "Kompass Hosts", dialog);
  await dialog.locator(".compass-host").first().click();
  const next = await auditControls(page, "Kompass nächster Schritt", dialog);
  expect([...tasks.failures, ...hosts.failures, ...next.failures]).toEqual([]);
});
