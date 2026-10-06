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

test("production schedule settings and paused host CRUD persist through the interface", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByLabel("Benutzername").fill("admin");
  await page.getByLabel("Passwort").fill("production-browser-test-password");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByLabel("Startzeit", { exact: true }).fill("23:59");
  await page.getByLabel("Zeitzone", { exact: true }).fill("UTC");
  await page.getByLabel("Parallele Aufträge", { exact: true }).fill("2");
  await page.getByRole("button", { name: "Einstellungen speichern" }).click();
  await expect(
    page.getByText("Einstellungen gespeichert", { exact: true }),
  ).toBeVisible();
  expect(await (await page.request.get("/api/settings")).json()).toMatchObject({
    schedule: "23:59",
    timezone: "UTC",
    parallel: 2,
  });
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Host hinzufügen", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Hostname", { exact: true }).fill("pve-schedule-ui");
  await dialog.getByLabel("Adresse", { exact: true }).fill("192.0.2.190");
  await dialog
    .getByRole("button", { name: "SSH und Sicherungsprofil" })
    .click();
  await dialog.getByLabel("Automatisch sichern", { exact: true }).uncheck();
  await dialog
    .getByRole("button", { name: "Host speichern", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  const hosts = await (await page.request.get("/api/hosts")).json();
  expect(hosts).toHaveLength(1);
  expect(hosts[0]).toMatchObject({
    name: "pve-schedule-ui",
    enabled: false,
    schedule: "",
  });
  await page
    .getByRole("button", { name: "pve-schedule-ui", exact: true })
    .click();
  await page
    .locator(".tabs")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Host bearbeiten", exact: true })
    .click();
  await dialog.getByLabel("Startzeit", { exact: true }).fill("22:30");
  await dialog
    .getByRole("button", { name: "Host speichern", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  expect(
    await (await page.request.get(`/api/hosts/${hosts[0].id}`)).json(),
  ).toMatchObject({ enabled: false, schedule: "22:30" });
  await page
    .getByRole("button", { name: "Host entfernen", exact: true })
    .click();
  await dialog.getByLabel("Hostnamen bestätigen").fill("pve-schedule-ui");
  await dialog
    .getByRole("button", { name: "Host entfernen", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  expect(await (await page.request.get("/api/hosts")).json()).toEqual([]);
  expect((await (await page.request.get("/api/status")).json()).jobs).toEqual(
    [],
  );
});

test("an administrator can change their own password to eight characters and log in again", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByLabel("Benutzername", { exact: true }).fill("admin");
  await page
    .getByLabel("Passwort", { exact: true })
    .fill("production-browser-test-password");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await page
    .getByRole("navigation")
    .getByRole("button", { name: "Einstellungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Zugriff", exact: true }).click();
  await page.getByRole("button", { name: "Aktionen für admin" }).click();
  await page
    .getByRole("button", { name: "Passwort ändern", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Neues Passwort", { exact: true }).fill("prod2026");
  await dialog
    .getByLabel("Passwort wiederholen", { exact: true })
    .fill("prod2026");
  await dialog
    .getByRole("button", { name: "Änderungen speichern", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Anmelden", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Benutzername", { exact: true }).fill("admin");
  await page.getByLabel("Passwort", { exact: true }).fill("prod2026");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
});
