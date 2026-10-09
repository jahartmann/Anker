import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";
const recipe = {
  id: "reviewed",
  version: 1,
  label: "Geprüfter Zielhost",
  origin_plan_id: "original",
  reviewed_by: "admin",
  reviewed_at: "2026-10-09T10:00:00Z",
};
async function start(page: Page) {
  const base = await (await page.request.get("/api/status")).json();
  await page.route("**/api/recovery-recipes", (r) =>
    r.fulfill({ json: [recipe] }),
  );
  await page.goto("/");
  if ((page.viewportSize()?.width || 1440) < 600)
    await page.getByRole("button", { name: "Navigation öffnen" }).click();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan erstellen", exact: true })
    .click();
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[0].id);
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("etc/hostname");
  await page.getByLabel("etc/hostname", { exact: true }).check();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[0].id);
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  return base;
}
test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});
for (const width of [1440, 390])
  test(`reviewed mapping intent and mismatches at ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 });
    let applicable = true;
    await page.route("**/api/recovery-recipes/preview", async (r) => {
      const body = r.request().postDataJSON();
      expect(body.request.snapshot_id).toBeTruthy();
      await r.fulfill({
        json: {
          recipe,
          snapshot_id: body.request.snapshot_id,
          applicable,
          suggestions: applicable
            ? { hostname: "reviewed-target", interfaces: {}, storage: {} }
            : {},
          mismatches: applicable
            ? []
            : ["Zielidentität: geändert oder nicht belegt"],
        },
      });
    });
    await start(page);
    await page
      .getByLabel("Zuordnungsvorlage", { exact: true })
      .selectOption(recipe.id);
    await page
      .getByRole("button", { name: "Vorlage frisch prüfen", exact: true })
      .click();
    await expect(
      page.getByText("Geprüft von admin", { exact: false }),
    ).toBeVisible();
    await page
      .getByRole("button", {
        name: "Geprüfte Zuordnungen einsetzen",
        exact: true,
      })
      .click();
    await expect(
      page.getByLabel("Gewünschter Hostname", { exact: true }),
    ).toHaveValue("reviewed-target");
    await expect(
      page.getByLabel("Entscheidung für etc/hostname", { exact: true }),
    ).toHaveValue("");
    await page.screenshot({
      path: `/tmp/anker-task10-recipe-${width}.png`,
      fullPage: true,
    });
    applicable = false;
    await page
      .getByRole("button", { name: "Vorlage frisch prüfen", exact: true })
      .click();
    await expect(
      page.getByText("Zielidentität: geändert oder nicht belegt", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", {
        name: "Geprüfte Zuordnungen einsetzen",
        exact: true,
      }),
    ).toHaveCount(0);
    await expect(
      page.getByLabel("Gewünschter Hostname", { exact: true }),
    ).toHaveValue("");
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBeTruthy();
  });
test("late recipe response cannot fill another target", async ({ page }) => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  await page.route("**/api/recovery-recipes/preview", async (r) => {
    const body = r.request().postDataJSON();
    await gate;
    await r.fulfill({
      json: {
        recipe,
        snapshot_id: body.request.snapshot_id,
        applicable: true,
        suggestions: { hostname: "stale-target" },
        mismatches: [],
      },
    });
  });
  const base = await start(page);
  await page
    .getByLabel("Zuordnungsvorlage", { exact: true })
    .selectOption(recipe.id);
  await page
    .getByRole("button", { name: "Vorlage frisch prüfen", exact: true })
    .click();
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page
    .getByLabel("Zielhost", { exact: true })
    .selectOption(base.hosts[1].id);
  release();
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await expect(
    page.getByLabel("Gewünschter Hostname", { exact: true }),
  ).toHaveValue("");
  await expect(
    page.getByRole("button", {
      name: "Geprüfte Zuordnungen einsetzen",
      exact: true,
    }),
  ).toHaveCount(0);
});

test("saving reviewed intent requires explicit review and shows the saved mapping", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const plan = {
    id: "recipe-origin",
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    created_at: "2026-10-09T10:00:00Z",
    scenario: "files",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    steps: [],
    manual: [],
    mapping: {
      interfaces: { eno1: "ens3" },
      storage: {},
      hostname: "reviewed-host",
    },
    schema_version: 2,
    target_snapshot_id: "snapshot",
    state: "manual",
    blockers: [],
    tasks: [],
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, plans: [plan], jobs: [] } }),
  );
  await page.route("**/api/plans/recipe-origin", (r) =>
    r.fulfill({ json: plan }),
  );
  let saved = 0;
  await page.route("**/api/recovery-recipes", (r) => {
    expect(r.request().postDataJSON()).toEqual({
      plan_id: plan.id,
      label: "Geprüfter Host",
      reviewed: true,
    });
    saved++;
    return r.fulfill({ json: recipe });
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen", exact: true }).click();
  const save = page.getByRole("region", {
    name: "Zuordnungsvorlage speichern",
  });
  await expect(save).toContainText("eno1 → ens3");
  await save.getByLabel("Bezeichnung der Vorlage").fill("Geprüfter Host");
  await expect(
    save.getByRole("button", { name: "Als geprüfte Vorlage speichern" }),
  ).toBeDisabled();
  await save
    .getByLabel("Ich habe die Zuordnungen dieses Plans geprüft.")
    .check();
  await save
    .getByRole("button", { name: "Als geprüfte Vorlage speichern" })
    .click();
  await expect(save.getByText("Zuordnungsvorlage gespeichert.")).toBeVisible();
  expect(saved).toBe(1);
});

test("application rechecks permissions and does not reuse the displayed result", async ({
  page,
}) => {
  let calls = 0;
  await page.route("**/api/recovery-recipes/preview", (r) => {
    calls++;
    const body = r.request().postDataJSON();
    return calls === 1
      ? r.fulfill({
          json: {
            recipe,
            snapshot_id: body.request.snapshot_id,
            applicable: true,
            suggestions: {
              hostname: "must-not-fill",
              interfaces: {},
              storage: {},
            },
            mismatches: [],
          },
        })
      : r.fulfill({
          status: 403,
          json: { error: "Berechtigung wurde entzogen" },
        });
  });
  await start(page);
  await page
    .getByLabel("Zuordnungsvorlage", { exact: true })
    .selectOption(recipe.id);
  await page
    .getByRole("button", { name: "Vorlage frisch prüfen", exact: true })
    .click();
  await page
    .getByRole("button", {
      name: "Geprüfte Zuordnungen einsetzen",
      exact: true,
    })
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "Berechtigung wurde entzogen",
  );
  await expect(
    page.getByLabel("Gewünschter Hostname", { exact: true }),
  ).toHaveValue("");
  expect(calls).toBe(2);
});
