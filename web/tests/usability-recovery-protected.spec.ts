import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import type { APIRequestContext } from "@playwright/test";

let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await demoCookies(request);
});
test.beforeEach(async ({ page }) => {
  await page.context().addCookies(cookies);
});

test("protected system files are written back only after one explicit confirmation", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const backup = base.backups.find((b: any) => b.host_id === "demo2");
  let payload: any;
  await page.route("**/api/plans", (r) => {
    if (r.request().method() !== "POST") return r.continue();
    payload = r.request().postDataJSON();
    return r.continue();
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  await page.getByLabel("Sicherung", { exact: true }).selectOption(backup.id);
  const files = page.getByRole("group", { name: /Dateien auswählen/ });
  await files.getByRole("button", { name: "Ordner etc öffnen" }).click();
  await files.getByLabel("etc/hosts", { exact: true }).check();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  // The source host is preselected as target.
  await expect(page.getByLabel("Zielhost", { exact: true })).toHaveValue(
    "demo2",
  );
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await page
    .getByLabel("Entscheidung für etc/hosts", { exact: true })
    .selectOption("backup");
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  const confirm = page.getByRole("region", {
    name: "Geschützte Systemdateien bestätigen",
  });
  await expect(confirm).toContainText("etc/hosts");
  const save = page.getByRole("button", {
    name: "Plan speichern",
    exact: true,
  });
  await expect(save).toBeDisabled();
  await confirm
    .getByLabel("Diese Systemdateien aus der Sicherung zurückschreiben")
    .check();
  await save.click();
  expect(payload.protected_confirmed).toBe(true);
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan", exact: true }),
  ).toBeVisible();
});
