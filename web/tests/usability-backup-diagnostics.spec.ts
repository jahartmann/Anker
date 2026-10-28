import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});

async function partialHost(page: Page) {
  const base = await (await page.request.get("/api/status")).json();
  const host = base.hosts[0];
  const backup = {
    ...base.backups[0],
    host_id: host.id,
    host_name: host.name,
    created_at: new Date().toISOString(),
    status: "partial",
    verification_error: "",
    warnings: ["[Verdeckter Inhalt]", "invalid path"],
    issues: [
      {
        code: "dependency_not_captured",
        title: "Verwendete Datei nicht in der Sicherung",
        path: "/opt/application/settings.conf",
        detail:
          "Eine Konfiguration verweist auf eine weitere Datei. Ob sie auf dem Host existiert, ist noch nicht geklärt.",
        action:
          "Verweis auf dem Host prüfen und benötigte Dateien im Hostprofil aufnehmen.",
      },
      {
        code: "invalid_path",
        title: "Pfad konnte nicht verarbeitet werden",
        path_protected: true,
        detail:
          "Die Pfadangabe entspricht nicht dem erlaubten Format. Das ist kein Nachweis, dass die Datei fehlt.",
        action: "Hostprofil prüfen und erneut sichern.",
      },
    ],
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        backups: [backup],
        jobs: [],
        references: {},
      },
    }),
  );
  await page.route("**/api/backups/compare", (r) => r.fulfill({ json: [] }));
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  return { host, backup, base };
}

test("host overview exposes causes and remedies before opening files; profile action stays explicit", async ({
  page,
}) => {
  await partialHost(page);
  const issues = page.getByRole("region", {
    name: "Ursachen der unvollständigen Sicherung",
  });
  await expect(issues).toBeVisible();
  await expect(
    issues.getByText("/opt/application/settings.conf", { exact: true }),
  ).toBeVisible();
  await expect(
    issues.getByRole("heading", {
      name: "Pfad konnte nicht verarbeitet werden",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    issues.getByText(/kein Nachweis, dass die Datei fehlt/),
  ).toBeVisible();
  await expect(issues.getByText(/bleiben geschützt/)).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const panelColors = await page
    .locator(".host-workspaces > [data-topic]")
    .evaluateAll((panels) =>
      panels.map((p) => getComputedStyle(p).borderTopColor),
    );
  expect(new Set(panelColors).size).toBe(3);
  const shadows = await page
    .locator(".host-workspaces > [data-topic]")
    .evaluateAll((panels) => panels.map((p) => getComputedStyle(p).boxShadow));
  for (const shadow of shadows) {
    const offset = shadow.match(/\s(\d+(?:\.\d+)?)px\s+(\d+(?:\.\d+)?)px/);
    expect(offset).not.toBeNull();
    expect(Number(offset![1])).toBeGreaterThanOrEqual(3);
    expect(Number(offset![2])).toBeGreaterThanOrEqual(3);
  }
  await page
    .locator(".host-workspaces")
    .screenshot({ path: "/tmp/anker-host-cards-preview.png" });
  await page.screenshot({
    path: "/tmp/anker-diagnostics-host-desktop.png",
    fullPage: true,
  });
  await issues.getByRole("button", { name: "Sicherungsprofil prüfen" }).click();
  await expect(
    page.getByRole("button", { name: "Einstellungen", exact: true }).last(),
  ).toHaveAttribute("aria-pressed", "true");
});

test("diagnostics and topic panels fit phones and retry only after an explicit click", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const { host } = await partialHost(page);
  let retries = 0;
  await page.route(`**/api/hosts/${host.id}/backup`, (r) => {
    retries++;
    return r.fulfill({ json: { id: "new-backup-job" } });
  });
  const issues = page.getByRole("region", {
    name: "Ursachen der unvollständigen Sicherung",
  });
  await expect(issues).toBeVisible();
  expect(retries).toBe(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "/tmp/anker-diagnostics-host-mobile.png",
    fullPage: true,
  });
  await issues.getByRole("button", { name: "Neue Sicherung starten" }).click();
  await expect.poll(() => retries).toBe(1);
});

test("failed backup jobs expose their cause and link directly to the retained partial capture", async ({
  page,
}) => {
  const { host, backup, base } = await partialHost(page);
  const job = {
    ...base.jobs[0],
    id: "diagnostic-job",
    host_id: host.id,
    kind: "backup",
    state: "failed",
    result_id: backup.id,
    error: "Sicherung unvollständig",
    created_at: new Date().toISOString(),
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        hosts: [host],
        backups: [backup],
        jobs: [job],
        references: {},
      },
    }),
  );
  await page.reload();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Aufträge", exact: true })
    .click();
  await expect(
    page.getByText("Verwendete Datei nicht in der Sicherung", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", {
      name: "Unvollständige Sicherung prüfen",
      exact: true,
    })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("region", {
      name: "Ursachen der unvollständigen Sicherung",
    }),
  ).toBeVisible();
  await expect(
    dialog.getByText("/opt/application/settings.conf", { exact: true }),
  ).toBeVisible();
});
