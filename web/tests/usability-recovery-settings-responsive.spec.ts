import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

for (const [width, height] of [
  [1280, 720],
  [375, 812],
]) {
  test(`settings sections and recovery stay readable at ${width}px`, async ({
    page,
    request,
  }, testInfo) => {
    await page.context().addCookies(await demoCookies(request));
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.setViewportSize({ width, height });
    await page.goto("/");
    async function navigate(name: string) {
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
    await navigate("Einstellungen");
    await expect(page.getByLabel("Startzeit", { exact: true })).toBeVisible();
    for (const tab of [
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
        .getByRole("button", { name: tab, exact: true })
        .click();
      await expect(page.locator(".settings-context")).toBeVisible();
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
      ).toBe(width);
      await page.screenshot({
        path: testInfo.outputPath(`${tab}-${width}.png`),
        fullPage: true,
      });
    }
    await navigate("Wiederherstellung");
    await page
      .getByRole("button", { name: "Plan erstellen", exact: true })
      .click();
    await expect(
      page.getByRole("region", { name: "Vorbereitungskontext" }),
    ).toContainText("Nach Zielprüfung ermitteln");
    await expect(
      page
        .getByRole("navigation", { name: "Vorbereitungsschritte" })
        .getByRole("listitem"),
    ).toHaveCount(5);
    await expect(
      page.getByRole("button", { name: "Weiter zum Ziel", exact: true }),
    ).toBeDisabled();
    await expect(
      page.getByRole("button", { name: "Weiter zum Ziel", exact: true }),
    ).toBeInViewport();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
    await page.screenshot({
      path: testInfo.outputPath(`Wiederherstellung-${width}.png`),
      fullPage: true,
    });
    expect(errors).toEqual([]);
  });
}
