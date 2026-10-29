import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
  await page.goto("/");
});

async function navigate(page: Page, name: string) {
  const menu = page.getByRole("button", {
    name: "Navigation öffnen",
    exact: true,
  });
  if (await menu.isVisible()) await menu.click();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name, exact: true })
    .click();
}
async function fits(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
}

test("all six workspaces keep the paper hierarchy on desktop and phones", async ({
  page,
}) => {
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    for (const name of [
      "Übersicht",
      "Hosts",
      "Sicherungen",
      "Wiederherstellung",
      "Aufträge",
      "Einstellungen",
    ]) {
      await navigate(page, name);
      await expect(page.locator("main .content")).toHaveAttribute(
        "data-workspace",
        name,
      );
      await expect(
        page.getByRole("heading", { name, exact: true }).first(),
      ).toBeVisible();
      await fits(page);
      const headingPaint = await page
        .locator(".page-heading")
        .first()
        .evaluate((element) => {
          const style = getComputedStyle(element);
          return {
            background: style.backgroundColor,
            shadow: style.boxShadow,
            edge: parseFloat(style.borderTopWidth),
          };
        });
      // Large headings stay paper-white; topic colour lives in the edge and small details.
      expect(
        headingPaint.background === "rgba(0, 0, 0, 0)" ||
          headingPaint.background === "rgb(255, 255, 255)",
      ).toBe(true);
      expect(headingPaint.shadow).not.toBe("none");
      expect(headingPaint.edge).toBeGreaterThanOrEqual(3);
      if (name === "Sicherungen") {
        const shadows = await page
          .locator(".backup-focus-grid > section, .backup-collection")
          .evaluateAll((cards) =>
            cards.map((card) => getComputedStyle(card).boxShadow),
          );
        expect(shadows.length).toBeGreaterThan(0);
        for (const shadow of shadows) {
          const offset = shadow.match(
            /\s(\d+(?:\.\d+)?)px\s+(\d+(?:\.\d+)?)px/,
          );
          expect(offset).not.toBeNull();
          expect(Number(offset![1])).toBeGreaterThanOrEqual(3);
          expect(Number(offset![2])).toBeGreaterThanOrEqual(3);
        }
      }
      await page.screenshot({
        path: `/tmp/anker-design-${width}-${name}.png`,
        fullPage: true,
        animations: "disabled",
      });
    }
  }
});

test("all host tabs and seven settings areas use the same readable surfaces", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await navigate(page, "Hosts");
    await page
      .getByRole("button", { name: base.hosts[0].name, exact: true })
      .click();
    for (const name of [
      "Übersicht",
      "Sicherungen",
      "Änderungen",
      "Inventar",
      "Einstellungen",
    ]) {
      const tab = page
        .locator(".host-tabs")
        .getByRole("button", { name, exact: true });
      await tab.click();
      await expect(tab).toHaveAttribute("aria-pressed", "true");
      await fits(page);
      await page.screenshot({
        path: `/tmp/anker-design-host-${width}-${name}.png`,
        fullPage: true,
        animations: "disabled",
      });
    }
    await navigate(page, "Einstellungen");
    for (const name of [
      "Zeitplan",
      "Aufbewahrung",
      "Benachrichtigungen",
      "Zugriff",
      "Zertifikate",
      "Updates",
      "Betrieb",
    ]) {
      await page
        .locator(".settings-tabs")
        .getByRole("button", { name, exact: true })
        .click();
      await expect(page.locator(".settings-context")).toBeVisible();
      if (["Zeitplan", "Aufbewahrung", "Benachrichtigungen"].includes(name)) {
        const contextTopic = await page
          .locator(".settings-context")
          .getAttribute("data-surface-topic");
        await expect(
          page.locator(".settings-form[data-surface-topic]").first(),
        ).toHaveAttribute("data-surface-topic", contextTopic!);
      }
      if (["Zugriff", "Zertifikate", "Updates", "Betrieb"].includes(name))
        await expect(
          page.locator(".paper-panel:visible").first(),
        ).toBeVisible();
      if (name === "Betrieb") {
        const summaryColors = await page
          .locator(".betrieb-section .detail-disclosure > summary")
          .evaluateAll((elements) =>
            elements.map(
              (element) => getComputedStyle(element).backgroundColor,
            ),
          );
        expect(new Set(summaryColors).size).toBeGreaterThanOrEqual(2);
      }
      await fits(page);
      await page.screenshot({
        path: `/tmp/anker-design-settings-${width}-${name}.png`,
        fullPage: true,
        animations: "disabled",
      });
    }
  }
});

test("network and host dialogs carry the topic into their portal without losing keyboard access", async ({
  page,
}) => {
  await navigate(page, "Hosts");
  await page
    .getByRole("button", { name: "Server suchen", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toHaveAttribute("data-surface-topic", "network");
  await expect(dialog.locator(".dialog-title .topic-symbol")).toBeVisible();
  const shadow = await dialog.evaluate((el) => getComputedStyle(el).boxShadow);
  expect(shadow).not.toBe("none");
  await page.screenshot({
    path: "/tmp/anker-design-dialog-network.png",
    fullPage: true,
    animations: "disabled",
  });
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await page
    .getByRole("button", { name: "Host hinzufügen", exact: true })
    .click();
  await expect(dialog).toHaveAttribute("data-surface-topic", "server");
  await fits(page);
});
