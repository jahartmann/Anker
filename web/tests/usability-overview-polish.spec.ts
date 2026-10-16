import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test("overview metrics open meaningful filters repeatedly", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const hosts = base.hosts
    .slice(0, 3)
    .map((host: any, index: number) => ({ ...host, enabled: index !== 1 }));
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, hosts, backups: [], jobs: [] } }),
  );
  await page.goto("/");
  const overview = () =>
    page
      .getByRole("navigation", { name: "Hauptnavigation" })
      .getByRole("button", { name: "Übersicht", exact: true })
      .click();
  await overview();
  await page.getByRole("button", { name: /^Aktive Hosts/ }).click();
  await expect(
    page.getByLabel("Hoststatus filtern", { exact: true }),
  ).toHaveValue("active");
  await overview();
  await page.getByRole("button", { name: /^Handlungsbedarf/ }).click();
  await expect(
    page.getByLabel("Hoststatus filtern", { exact: true }),
  ).toHaveValue("attention");
  await overview();
  await page.getByRole("button", { name: /^Aktuell gesichert/ }).click();
  await expect(
    page.getByLabel("Hoststatus filtern", { exact: true }),
  ).toHaveValue("saved");
  await overview();
  await page.getByRole("button", { name: /^Aktive Hosts/ }).click();
  await expect(
    page.getByLabel("Hoststatus filtern", { exact: true }),
  ).toHaveValue("active");
  await overview();
  await page.getByRole("button", { name: /^Aktive Aufträge/ }).click();
  await expect(
    page.getByRole("heading", { name: "Aufträge", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Aktiv 0", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});
