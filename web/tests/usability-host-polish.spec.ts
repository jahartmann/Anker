import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await demoCookies(request);
});

async function openHosts(page: Page) {
  await page.context().addCookies(cookies);
  const base = await (await page.request.get("/api/status")).json();
  const createdAt = new Date(Date.now() - 3 * 3600000).toISOString();
  const hosts = [
    {
      ...base.hosts[0],
      id: "polish-saved",
      name: "Polish Saved",
      enabled: true,
    },
    {
      ...base.hosts[0],
      id: "polish-attention",
      name: "Polish Attention",
      enabled: true,
    },
    {
      ...base.hosts[0],
      id: "polish-paused",
      name: "Polish Paused",
      enabled: false,
    },
  ];
  const backups = [
    {
      ...base.backups[0],
      id: "polish-backup",
      host_id: hosts[0].id,
      host_name: hosts[0].name,
      created_at: createdAt,
      status: "successful",
    },
  ];
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: { ...base, hosts, backups, jobs: [], stale_hours: 24 },
    }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
}

test("host quick filters count hosts and combine with search and status selection", async ({
  page,
}) => {
  await openHosts(page);
  const filters = page.getByRole("group", { name: "Host-Schnellfilter" });
  await expect(
    filters.getByRole("button", { name: "Alle 3", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await filters
    .getByRole("button", { name: "Handlungsbedarf 1", exact: true })
    .click();
  await expect(
    page.getByRole("combobox", { name: "Hoststatus filtern" }),
  ).toHaveValue("attention");
  await expect(
    page.getByRole("button", { name: "Polish Attention", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Polish Saved", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("textbox", { name: "Hosts durchsuchen" }).fill("Saved");
  await expect(
    page.getByRole("heading", { name: "Keine passenden Hosts" }),
  ).toBeVisible();
  await filters.getByRole("button", { name: "Alle 3", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Polish Saved", exact: true }),
  ).toBeVisible();
  await page.getByRole("textbox", { name: "Hosts durchsuchen" }).clear();
  await filters
    .getByRole("button", { name: "Pausiert 1", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Polish Paused", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Polish Attention", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("combobox", { name: "Hoststatus filtern" })
    .selectOption("active");
  await expect(
    page.getByRole("button", { name: "Polish Saved", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Polish Attention", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Polish Paused", exact: true }),
  ).toHaveCount(0);
});

test("relative backup age retains an accessible exact timestamp", async ({
  page,
}) => {
  await openHosts(page);
  const row = page.getByRole("row").filter({ hasText: "Polish Saved" });
  const age = row.locator("td.date time");
  await expect(age).toHaveText("vor 3 Stunden");
  await expect(age).toHaveAttribute("datetime", /T/);
  const exactDate = await age.getAttribute("title");
  expect(exactDate).toBeTruthy();
  await expect(age).toHaveAttribute(
    "aria-label",
    `vor 3 Stunden · ${exactDate}`,
  );
});

test("host source evidence starts collapsed and restore stays reachable", async ({
  page,
}) => {
  await openHosts(page);
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.getByRole("button", { name: "Polish Saved", exact: true }).click();
  const evidence = page
    .locator("details")
    .filter({ has: page.locator("summary", { hasText: "Quellnachweise" }) });
  await expect(evidence).not.toHaveAttribute("open", "");
  await expect(
    evidence.getByRole("region", { name: "Quellnachweise" }),
  ).not.toBeVisible();
  const restore = page.getByRole("button", {
    name: "Wiederherstellung planen",
    exact: true,
  });
  await expect(restore).toBeEnabled();
  await expect(restore).toBeInViewport();
  await evidence.locator("summary").click();
  await expect(
    evidence.getByRole("region", { name: "Quellnachweise" }),
  ).toBeVisible();
});
