import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await demoCookies(request);
});

async function openSettings(page: Page) {
  await page.context().addCookies(cookies);
  const settings = await (await page.request.get("/api/settings")).json();
  await page.route("**/api/settings", (route) =>
    route.fulfill({
      json:
        route.request().method() === "PUT"
          ? route.request().postDataJSON()
          : {
              ...settings,
              schedule: "02:00",
              daily: 7,
              session_days: 30,
              webhook: "",
            },
    }),
  );
  await page.goto("/");
  const openNavigation = page.getByRole("button", {
    name: "Navigation öffnen",
    exact: true,
  });
  if (await openNavigation.isVisible()) await openNavigation.click();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
}

test("settings groups keep timing, execution, freshness and notification channels distinct", async ({
  page,
}) => {
  await openSettings(page);
  await expect(
    page
      .getByRole("group", { name: "Start und Zeitzone", exact: true })
      .getByLabel("Startzeit", { exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("group", { name: "Start und Zeitzone", exact: true })
      .getByLabel("Zeitzone", { exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("group", { name: "Ausführung", exact: true })
      .getByLabel("Parallele Aufträge", { exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("group", { name: "Aktualität", exact: true })
      .getByLabel("Überfällig nach Stunden", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Benachrichtigungen", exact: true })
    .click();
  await expect(
    page
      .getByRole("group", { name: "Webhook", exact: true })
      .getByLabel("Webhook-URL", { exact: true }),
  ).toBeVisible();
  const email = page.getByRole("group", { name: "E-Mail (SMTP)", exact: true });
  await expect(email.getByLabel("SMTP-Server", { exact: true })).toBeVisible();
  await expect(
    email.getByLabel("SMTP-Passwort", { exact: true }),
  ).toHaveAttribute("type", "password");
});

test("changed sections remain identifiable across tabs, clear on revert, and save together", async ({
  page,
}) => {
  await openSettings(page);
  const changes = page.getByRole("status", {
    name: "Ungespeicherte Bereiche",
    exact: true,
  });
  await page.getByLabel("Startzeit", { exact: true }).fill("03:14");
  await page.getByRole("button", { name: "Aufbewahrung", exact: true }).click();
  await expect(changes).toContainText("Zeitplan");
  await page.getByLabel("Tagesstände", { exact: true }).fill("8");
  await page
    .getByRole("button", { name: "Benachrichtigungen", exact: true })
    .click();
  await page
    .getByLabel("Webhook-URL", { exact: true })
    .fill("https://example.test/notify");
  await page.getByRole("button", { name: "Zugriff", exact: true }).click();
  await page.getByLabel("Sitzungsdauer in Tagen", { exact: true }).fill("31");
  await expect(changes).toContainText("Zeitplan");
  await expect(changes).toContainText("Aufbewahrung");
  await expect(changes).toContainText("Benachrichtigungen");
  await expect(changes).toContainText("Zugriff");
  await page.getByRole("button", { name: "Aufbewahrung", exact: true }).click();
  await page.getByLabel("Tagesstände", { exact: true }).fill("7");
  await expect(changes).not.toContainText("Aufbewahrung");
  await page.getByRole("button", { name: "Updates", exact: true }).click();
  await expect(changes).toContainText("Zeitplan");
  await expect(changes).toContainText("Zugriff");
  await page.getByRole("button", { name: "Zugriff", exact: true }).click();
  await expect(
    page.getByText("Speichert alle geänderten Einstellungsbereiche.", {
      exact: true,
    }),
  ).toBeVisible();
  const savedRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith("/api/settings") && request.method() === "PUT",
  );
  await page
    .getByRole("button", { name: "Einstellungen speichern", exact: true })
    .click();
  expect((await savedRequest).postDataJSON()).toMatchObject({
    schedule: "03:14",
    daily: 7,
    session_days: 31,
    webhook: "https://example.test/notify",
  });
  await expect(changes).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Einstellungen speichern", exact: true }),
  ).toBeDisabled();
});

test("mobile notification editing keeps one compact save action reachable while scrolling", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openSettings(page);
  await page
    .getByRole("button", { name: "Benachrichtigungen", exact: true })
    .click();
  await page
    .getByLabel("Empfänger", { exact: true })
    .fill("settings@example.test");
  const save = page.getByRole("button", {
    name: "Einstellungen speichern",
    exact: true,
  });
  await expect(save).toHaveCount(1);
  await expect(save).toBeEnabled();
  await expect(save).toBeInViewport();
  const bar = page.locator(".settings-savebar");
  expect((await bar.boundingBox())!.height).toBeLessThan(150);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});
