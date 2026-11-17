import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});

async function openDraft(page: Page) {
  const base = await (await page.request.get("/api/status")).json();
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  return base;
}

async function chooseTarget(page: Page, scenario = "files") {
  const base = await openDraft(page);
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[0].id);
  await page.getByLabel("Situation", { exact: true }).selectOption(scenario);
  if (scenario === "files") {
    await page
      .getByLabel("Wiederherstellungsdateien durchsuchen")
      .fill("etc/sysctl.d/99-anker.conf");
    await page
      .getByLabel("etc/sysctl.d/99-anker.conf", { exact: true })
      .check();
  }
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[0].id);
}

test("draft identifies the current step once", async ({ page }) => {
  await openDraft(page);
  const progress = page.getByRole("navigation", {
    name: "Vorbereitungsschritte",
  });
  await expect(progress.locator("li")).toHaveCount(5);
  await expect(progress.locator('[aria-current="step"]')).toContainText(
    "Quelle und Situation",
  );
  await expect(progress.locator("li").last()).toContainText(
    "Im gespeicherten Plan",
  );
  await expect(page.getByText("Schritt 1 von 5", { exact: true })).toHaveCount(
    1,
  );
});

for (const complete of [false, true]) {
  test(`target capture ${complete ? "complete with manual notes" : "incomplete"} has an honest status`, async ({
    page,
  }) => {
    await chooseTarget(page);
    await page.route("**/api/plans/inspect", async (route) => {
      const response = await route.fetch();
      const inspection = await response.json();
      await route.fulfill({
        json: {
          ...inspection,
          comparison_complete: complete,
          manual: [
            ...(inspection.manual || []),
            "Manuellen Betriebsschritt separat prüfen",
          ],
        },
      });
    });
    await page
      .getByRole("button", { name: "Ziel prüfen", exact: true })
      .click();
    const target = page.getByRole("region", {
      name: "SSH-Zielprüfung",
      exact: true,
    });
    await expect(
      target.getByText(
        complete ? "Ziel geprüft" : "Zielaufnahme mit Hinweisen",
        { exact: true },
      ),
    ).toBeVisible();
    if (!complete) {
      await expect(
        target.getByText("Ziel geprüft", { exact: true }),
      ).toHaveCount(0);
      await expect(target).toContainText(
        "Nicht alle Zieldateien konnten gelesen werden",
      );
    }
    await expect(
      page.getByRole("button", {
        name: "Weiter zu Entscheidungen",
        exact: true,
      }),
    ).toBeEnabled();
  });
}

test("saved plan summary labels source target state and file actions without losing the next task", async ({
  page,
}) => {
  await chooseTarget(page);
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("keep");
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan speichern", exact: true })
    .click();
  const summary = page.getByRole("region", {
    name: "Planübersicht",
    exact: true,
  });
  for (const label of [
    "Quelle",
    "Ziel",
    "Planstatus",
    "Dateiaktionen",
    "Nächste Aufgabe",
  ]) {
    await expect(summary.getByText(label, { exact: true })).toBeVisible();
  }
  await expect(summary).toContainText("1 bleibt unverändert");
  await expect(summary).not.toContainText("geschrieben");
  await expect(summary).toContainText("Dienste prüfen");
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan", exact: true }),
  ).toContainText(
    "Vor der Übernahme prüft Anker erneut auf Änderungen am Ziel",
  );
});

test("changed storage binding explains the required fresh target capture", async ({
  page,
}) => {
  await chooseTarget(page, "host");
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  const storage = page
    .locator(".recovery-mapping select")
    .filter({ has: page.locator('option[value="manual"]') })
    .first();
  await storage.selectOption("manual");
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await expect(
    page.getByText("Zuordnungen wurden geändert. Ziel erneut prüfen.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeEnabled();
});
