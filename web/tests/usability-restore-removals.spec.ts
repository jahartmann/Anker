import { test, expect } from "@playwright/test";
import { rename, readFile, writeFile, stat } from "node:fs/promises";
import { demoCookies } from "./demo-auth";

test("real moved declaration is selected, inspected, explicitly deleted and restored from its journal", async ({
  page,
  request,
}, info) => {
  await page.context().addCookies(await demoCookies(request));
  const root = `/tmp/anker-browser-${process.env.ANKER_BROWSER_RUN_ID}/demo-hosts/demo2/files/etc/sysctl.d`;
  const original = "etc/sysctl.d/99-anker.conf",
    later = "etc/sysctl.d/98-later.conf";
  const old = await readFile(`${root}/99-anker.conf`);
  await rename(`${root}/99-anker.conf`, `${root}/98-later.conf`);
  await writeFile(
    `${root}/98-later.conf`,
    "# Lokale Testkonfiguration\nnet.ipv4.ip_forward=0\n",
    { mode: 0o600 },
  );
  await page.goto("/");
  const backup = await page.request.post("/api/hosts/demo2/backup", {
    headers: { "X-Anker-Request": "1" },
  });
  expect(backup.ok()).toBeTruthy();
  const job = await backup.json();
  await expect
    .poll(async () => {
      const s = await (await page.request.get("/api/status")).json();
      return s.jobs.find((j: any) => j.id === job.id)?.state;
    })
    .toBe("successful");
  await page.reload();
  await page
    .getByRole("button", { name: "pve-berlin-02", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Konfigurationsreise öffnen" })
    .click();
  await page
    .getByRole("button", { name: "Wiederherstellungsvorschau", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Wiederherstellungsvorschau",
    exact: true,
  });
  const deletion = dialog.getByRole("checkbox", {
    name: `Für Assistenten auswählen: ${later}`,
    exact: true,
  });
  await expect(deletion).not.toBeChecked();
  await deletion.check();
  await dialog
    .getByRole("button", { name: "Im Wiederherstellungsassistenten prüfen" })
    .click();
  await expect(
    page.getByRole("region", { name: "Vorgemerkte Dateientfernungen" }),
  ).toContainText(later);
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await page
    .getByRole("button", { name: `Vergleich für ${later}`, exact: true })
    .click();
  const diff = page.getByRole("region", {
    name: `Dateivergleich für ${later}`,
    exact: true,
  });
  await expect(diff).toContainText("Nach Entfernung · Datei fehlt");
  await expect(diff.locator(".diff-remove")).toContainText([
    "net.ipv4.ip_forward=0",
  ]);
  await page
    .getByLabel(`Entscheidung für ${original}`, { exact: true })
    .selectOption("backup");
  await page
    .getByLabel(`Entscheidung für ${later}`, { exact: true })
    .selectOption("remove");
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Plan prüfen", exact: true }),
  ).toBeDisabled();
  await page
    .getByRole("checkbox", {
      name: "Ich bestätige die aufgeführten Dateientfernungen und habe den aktuellen Zielvergleich geprüft.",
    })
    .check();
  const planResponse = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === "/api/plans" &&
      r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  const plan = await (await planResponse).json();
  expect(plan.steps.find((s: any) => s.path === later).action).toBe("remove");
  await expect(
    page.getByRole("region", { name: "Löschumfang vor Ausführung" }),
  ).toContainText(later);
  await page.getByLabel("Plan-ID zur Ausführung eingeben").fill(plan.id);
  await expect(
    page.getByRole("button", { name: "In Demo anwenden", exact: true }),
  ).toBeDisabled();
  await page
    .getByRole("checkbox", {
      name: "Diese Dateien nach vorheriger Sicherung ausdrücklich löschen",
    })
    .check();
  await page
    .getByRole("region", { name: "Löschumfang vor Ausführung" })
    .screenshot({ path: info.outputPath("removal-confirmation.png") });
  await page
    .getByRole("button", { name: "In Demo anwenden", exact: true })
    .click();
  const planView = page.getByRole("region", {
    name: "Wiederherstellungsplan",
    exact: true,
  });
  await expect(planView).toContainText("Demo angewendet");
  await expect
    .poll(async () => {
      try {
        await stat(`${root}/98-later.conf`);
        return true;
      } catch {
        return false;
      }
    })
    .toBe(false);
  expect(await readFile(`${root}/99-anker.conf`)).toEqual(old);
  await page
    .getByRole("button", { name: "Dateien zurücksetzen", exact: true })
    .click();
  await page.getByLabel("Plan-ID zur Rücksetzung eingeben").fill(plan.id);
  await page
    .getByRole("button", { name: "Rücksetzung bestätigen", exact: true })
    .click();
  await expect(planView).toContainText("Dateien zurückgesetzt");
  expect(await readFile(`${root}/98-later.conf`, "utf8")).toContain(
    "net.ipv4.ip_forward=0",
  );
  await expect
    .poll(async () => {
      try {
        await stat(`${root}/99-anker.conf`);
        return true;
      } catch {
        return false;
      }
    })
    .toBe(false);
});
