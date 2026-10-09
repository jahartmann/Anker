import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import type { Entry } from "../src/api";

const entry = (path: string, type = "file", secret = false): Entry => ({
  path,
  type,
  secret,
  mode: 420,
  uid: 0,
  gid: 0,
  size: 12,
});

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});

test("backup search finds real and synthetic folders and offers file location", async ({
  page,
}) => {
  const entries = [
    entry("etc/empty", "directory"),
    entry("etc/generated/deep/config.conf"),
    entry("etc/private/secret.conf", "file", true),
  ];
  await page.route("**/api/backups/*/files", (route) =>
    route.fulfill({ json: entries }),
  );
  await page.route("**/api/backups/*/file?*", (route) => {
    const path = new URL(route.request().url()).searchParams.get("path")!;
    return route.fulfill({
      json: {
        entry: entries.find((e) => e.path === path),
        content: "sensitive content",
        masked: true,
      },
    });
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen" })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  const dialog = page.getByRole("dialog");
  const search = dialog.getByLabel("Dateien durchsuchen");
  await search.fill("empty");
  await expect(
    dialog.getByRole("button", { name: "Ordner etc/empty öffnen" }),
  ).toBeVisible();
  await search.fill("generated");
  await dialog
    .getByRole("button", { name: "Ordner etc/generated öffnen", exact: true })
    .click();
  await expect(
    dialog.getByRole("navigation", { name: "Dateipfad" }),
  ).toContainText("generated");
  await dialog.getByRole("button", { name: "Übergeordneter Ordner" }).click();
  await expect(
    dialog.getByRole("navigation", { name: "Dateipfad" }),
  ).not.toContainText("generated");
  await search.fill("secret.conf");
  await dialog
    .getByRole("button", { name: "Im Ordner zeigen: etc/private/secret.conf" })
    .click();
  await expect(
    dialog.getByRole("navigation", { name: "Dateipfad" }),
  ).toContainText("private");
  await dialog
    .getByRole("button", { name: /^etc\/private\/secret.conf/ })
    .click();
  await expect(dialog.getByText("Geschützter Inhalt")).toBeVisible();
  await expect(dialog.getByText("sensitive content")).toHaveCount(0);
  await dialog
    .getByRole("button", { name: "Alle Dateien", exact: true })
    .click();
  await expect(
    dialog.getByRole("button", { name: "Ordner etc öffnen" }),
  ).toBeVisible();
});

test("backup search shows complete paths and keeps mobile results and actions usable", async ({
  page,
}) => {
  const longPath =
    "etc/pve/priv/storage/nested/long-location/demo-password-and-access-credentials.conf";
  const entries = [
    entry("etc/pve/priv", "directory"),
    entry("etc/pve/priv/storage", "directory"),
    entry(longPath, "file", true),
  ];
  await page.route("**/api/backups/*/files", (route) =>
    route.fulfill({ json: entries }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen" })
    .click();
  await page
    .getByRole("button", { name: "Dateien", exact: true })
    .first()
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Dateien durchsuchen").fill("priv");
  const path = dialog.locator(".file-entry-name small", { hasText: longPath });
  await expect(path).toBeVisible();
  async function completePathIsVisible() {
    return path.evaluate((node) => {
      const text = node as HTMLElement;
      const style = getComputedStyle(text);
      return (
        style.whiteSpace !== "nowrap" &&
        style.overflow !== "hidden" &&
        text.scrollWidth <= text.clientWidth + 1 &&
        text.getBoundingClientRect().height > 20
      );
    });
  }
  expect(await completePathIsVisible()).toBe(true);

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(path).toBeVisible();
  expect(await completePathIsVisible()).toBe(true);
  const layout = await dialog.evaluate((node) => {
    const list = node.querySelector(".file-items")!.getBoundingClientRect();
    const preview = node.querySelector(".file-preview")!;
    const action = [...node.querySelectorAll(".dialog-footer button")]
      .find((button) =>
        button.textContent?.includes("Wiederherstellung planen"),
      )!
      .getBoundingClientRect();
    return {
      listHeight: list.height,
      emptyDisplay: getComputedStyle(preview).display,
      actionBottom: action.bottom,
    };
  });
  expect(layout.listHeight).toBeGreaterThanOrEqual(220);
  expect(layout.emptyDisplay).toBe("none");
  expect(layout.actionBottom).toBeLessThanOrEqual(844);
});

test("recovery uses folders while selections survive navigation, search and pages until source changes", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const entries = [
    entry("etc/empty", "directory"),
    entry("etc/config.conf"),
    ...Array.from({ length: 27 }, (_, i) =>
      entry(`var/log/log-${String(i).padStart(2, "0")}.txt`),
    ),
  ];
  await page.route("**/api/backups/*/files", (route) =>
    route.fulfill({ json: entries }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung" })
    .click();
  await page.getByRole("button", { name: "Plan erstellen" }).click();
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[0].id);
  const files = page.getByRole("group", { name: /Dateien auswählen/ });
  await files.getByRole("button", { name: "Ordner etc öffnen" }).click();
  await files.getByLabel("etc/config.conf", { exact: true }).check();
  await files.getByRole("button", { name: "Übergeordneter Ordner" }).click();
  await files.getByRole("button", { name: "Ordner var öffnen" }).click();
  await files.getByRole("button", { name: "Ordner var/log öffnen" }).click();
  await expect(files.getByRole("checkbox")).toHaveCount(25);
  await files.getByRole("button", { name: "Nächste Seite" }).click();
  await files.getByLabel("var/log/log-25.txt", { exact: true }).check();
  await files.getByLabel("Wiederherstellungsdateien durchsuchen").fill("empty");
  await expect(
    files.getByRole("button", { name: "Ordner etc/empty öffnen" }),
  ).toBeVisible();
  await expect(files.getByRole("checkbox")).toHaveCount(0);
  await files.getByRole("button", { name: "Nur ausgewählte Dateien" }).click();
  await expect(files.getByRole("checkbox")).toHaveCount(0);
  await files
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("config.conf");
  await expect(files.getByRole("checkbox")).toHaveCount(1);
  await files.getByLabel("Wiederherstellungsdateien durchsuchen").fill("");
  await expect(files.getByRole("checkbox")).toHaveCount(2);
  await page
    .getByLabel("Sicherung", { exact: true })
    .selectOption(base.backups[1].id);
  await expect(
    page.getByRole("group", { name: /Dateien auswählen · 0 ausgewählt/ }),
  ).toBeVisible();
  await files.getByRole("button", { name: "Ordner etc öffnen" }).click();
  await expect(
    files.getByLabel("etc/config.conf", { exact: true }),
  ).not.toBeChecked();
});
