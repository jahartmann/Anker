import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});

async function openBackups(page: Page) {
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
}

test("row actions keep files accessible and choose the strictly older same-host source from the menu", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const backups = [
    {
      id: "current-node-a",
      host_id: "a",
      host_name: "Node A",
      created_at: "2026-10-07T10:00:00Z",
    },
    {
      id: "equal-node-a",
      host_id: "a",
      host_name: "Node A",
      created_at: "2026-10-07T10:00:00Z",
    },
    {
      id: "current-node-b",
      host_id: "b",
      host_name: "Node B",
      created_at: "2026-10-07T09:59:00Z",
    },
    {
      id: "previous-node-a",
      host_id: "a",
      host_name: "Node A",
      created_at: "2026-10-07T09:58:00Z",
    },
  ].map((backup) => ({
    ...base.backups[0],
    pinned: false,
    archived: false,
    ...backup,
  }));
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, backups, references: {}, jobs: [] } }),
  );
  await page.route("**/api/backups/current-node-a/files", (route) =>
    route.fulfill({ json: [] }),
  );
  await page.route("**/api/backups/compare", (route) =>
    route.fulfill({ json: [] }),
  );
  await openBackups(page);
  const current = page.getByRole("row", { name: /Node A.*current-no/ });
  await expect(
    current.getByRole("link", { name: "Herunterladen", exact: true }),
  ).toBeVisible();
  await expect(
    current.getByRole("button", { name: "Mit vorherigem Stand vergleichen" }),
  ).toHaveCount(0);
  await expect(
    page.getByText("Kein älterer Stand dieses Hosts", { exact: true }),
  ).toHaveCount(0);
  await current.getByRole("button", { name: "Dateien", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page
    .getByRole("button", { name: "Dialog schließen", exact: true })
    .click();
  await current.getByRole("button", { name: /^Aktionen für/ }).click();
  await page
    .getByRole("button", {
      name: "Mit vorherigem Stand vergleichen",
      exact: true,
    })
    .click();
  const comparison = page.getByRole("region", { name: "Sicherungsvergleich" });
  await expect(comparison.getByLabel("Vergleich von")).toHaveValue(
    "previous-node-a",
  );
  await expect(comparison.getByLabel("Vergleich bis")).toHaveValue(
    "current-node-a",
  );
  await page.getByRole("button", { name: "Zurück zu Sicherungen" }).click();
  await page
    .getByRole("row", { name: /Node B.*current-no/ })
    .getByRole("button", { name: /^Aktionen für/ })
    .click();
  const unavailable = page.getByRole("button", {
    name: "Mit vorherigem Stand vergleichen",
    exact: true,
  });
  await expect(unavailable).toBeDisabled();
  await expect(unavailable).toHaveAttribute(
    "title",
    "Kein älterer Stand dieses Hosts",
  );
});

test("newest badges use the complete host history through filters and pagination", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const backups = Array.from({ length: 28 }, (_, index) => ({
    ...base.backups[0],
    id: `history-${String(index).padStart(3, "0")}`,
    host_id: "history-host",
    host_name: "History node",
    created_at: new Date(Date.UTC(2026, 9, 1, 0, index)).toISOString(),
    pinned: index === 0,
    archived: false,
  }));
  backups.push({
    ...backups[0],
    id: "other-latest",
    host_id: "other-host",
    host_name: "Other node",
    pinned: false,
  });
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, backups, references: {}, jobs: [] } }),
  );
  await openBackups(page);
  const table = page.getByRole("table", { name: "Sicherungen", exact: true });
  await expect(
    table.getByRole("row", { name: /history-02/ }).first(),
  ).toContainText("Neuester Stand");
  await page
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  await expect(
    table.getByRole("row", { name: /History node.*history-00/ }).first(),
  ).not.toContainText("Neuester Stand");
  await expect(table.getByRole("row", { name: /Other node/ })).toContainText(
    "Neuester Stand",
  );
  await page.getByLabel("Schutz und Archiv filtern").selectOption("pinned");
  await expect(table.locator("tbody tr")).toHaveCount(1);
  await expect(table).toContainText("history-00");
  await expect(table).not.toContainText("Neuester Stand");
  await page.getByLabel("Schutz und Archiv filtern").selectOption("");
  await page.getByLabel("Sicherungen durchsuchen").fill("other-latest");
  await expect(table.locator("tbody tr")).toHaveCount(1);
  await expect(table).toContainText("Neuester Stand");
});

test("reader menus expose comparison without download or editing permissions", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const me = await (await page.request.get("/api/me")).json();
  const backups = [
    {
      ...base.backups[0],
      id: "reader-new",
      host_id: "reader-host",
      host_name: "Reader node",
      created_at: "2026-10-07T10:00:00Z",
    },
    {
      ...base.backups[0],
      id: "reader-old",
      host_id: "reader-host",
      host_name: "Reader node",
      created_at: "2026-10-06T10:00:00Z",
    },
  ];
  await page.route("**/api/me", (route) =>
    route.fulfill({
      json: { ...me, user: { ...me.user, role: "reader", secrets: false } },
    }),
  );
  await page.route("**/api/status", (route) =>
    route.fulfill({ json: { ...base, backups, references: {}, jobs: [] } }),
  );
  await openBackups(page);
  const current = page.getByRole("row", { name: /Reader node.*reader-new/ });
  await expect(
    current.getByRole("link", { name: "Herunterladen", exact: true }),
  ).toHaveCount(0);
  await current.getByRole("button", { name: /^Vergleich für/ }).click();
  await expect(
    page.getByRole("button", {
      name: "Mit vorherigem Stand vergleichen",
      exact: true,
    }),
  ).toBeEnabled();
  await expect(
    page.getByRole("button", { name: "Als Ausgangsstand", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Als Vergleichsstand", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Wiederherstellen", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Prüfsummen prüfen", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Archivieren", exact: true }),
  ).toHaveCount(0);
});
