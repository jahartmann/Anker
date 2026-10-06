import { test, expect } from "@playwright/test";

test("production starts empty, requires its own login and has no demo account", async ({
  page,
  request,
}) => {
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Anmelden", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Benutzername")).toHaveValue("");
  await expect(page.getByLabel("Passwort")).toHaveValue("");
  expect((await request.get("/api/status")).status()).toBe(401);
  const demo = await request.post("/api/login", {
    headers: { "X-Anker-Request": "1" },
    data: { name: "demo", password: "anker-demo-2026" },
  });
  expect(demo.status()).toBe(401);

  await page.getByLabel("Benutzername").fill("admin");
  await page.getByLabel("Passwort").fill("production-browser-test-password");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Noch keine Hosts", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Lokale Demo", { exact: true })).toHaveCount(0);
  const status = await (await page.request.get("/api/status")).json();
  expect(status.demo).toBe(false);
  for (const bucket of ["hosts", "backups", "jobs", "plans"])
    expect(status[bucket]).toEqual([]);
  const users = await (await page.request.get("/api/users")).json();
  expect(users).toHaveLength(1);
  expect(users[0]).toMatchObject({ name: "admin", role: "admin" });

  for (const [name, empty] of [
    ["Sicherungen", "Noch keine Sicherungen"],
    ["Wiederherstellung", "Noch keine Wiederherstellungspläne"],
    ["Aufträge", "Noch keine Aufträge"],
  ]) {
    await page
      .getByRole("navigation")
      .getByRole("button", { name, exact: true })
      .click();
    await expect(page.getByText(empty, { exact: true })).toBeVisible();
  }
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Noch keine Hosts", { exact: true }),
  ).toBeVisible();
});
