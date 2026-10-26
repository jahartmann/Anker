import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
async function setup(page: any, request: any) {
  await page.context().addCookies(await demoCookies(request));
  await page.goto("/");
  await page
    .getByRole("button", { name: "pve-berlin-01", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Konfigurationsreise öffnen" })
    .click();
  const explanation = page
    .getByRole("region", { name: "Konfigurationsreise" })
    .getByRole("region", { name: "Bedeutung der Änderung" });
  await expect(explanation).toBeVisible();
  return explanation.getByRole("button", {
    name: "Wiederherstellungsvorschau",
    exact: true,
  });
}

test("real preview reverses the observed value and whole-file diff without creating an operation", async ({
  page,
  request,
}, info) => {
  const trigger = await setup(page, request);
  const writes: string[] = [];
  page.on("request", (r) => {
    if (r.method() !== "GET") writes.push(new URL(r.url()).pathname);
  });
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Wiederherstellungsvorschau",
    exact: true,
  });
  await expect(dialog.locator(".rollback-value-card").first()).toContainText(
    "IPv4-Weiterleitung",
  );
  const values = dialog
    .locator(".rollback-value-card")
    .first()
    .locator(".rollback-values strong");
  await expect(values).toHaveText(["Deaktiviert", "Aktiviert"]);
  await expect(dialog).toContainText("keine Übernahme und kein Auftrag");
  await expect(dialog.locator(".rollback-file-card")).toHaveCount(1);
  await expect(dialog.locator(".rollback-file-card")).toContainText(
    "etc/sysctl.d/99-anker.conf",
  );
  await dialog.locator(".rollback-file-diff > summary").click();
  await expect(dialog.locator(".diff-add")).toContainText(
    "net.ipv4.ip_forward=1",
  );
  await expect(dialog.locator(".diff-remove")).toContainText(
    "net.ipv4.ip_forward=0",
  );
  await expect(dialog).toContainText("ganze Dateien");
  expect(writes).toEqual([]);
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await dialog.screenshot({
    path: info.outputPath("restore-preview-mobile.png"),
  });
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("collateral values and removals are explicit while files stay paged", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  await page.route(
    "**/api/backups/*/change-restore-preview?*",
    async (route) => {
      const response = await route.fetch();
      const preview = await response.json();
      preview.additional = [
        {
          ...preview.selected,
          setting: {
            id: "swappiness",
            name: "Swap-Neigung",
            key: "vm.swappiness",
            kind: "sysctl",
          },
          before: { ...preview.selected.before, value: "10" },
          after: { ...preview.selected.after, value: "60" },
        },
      ];
      const file = preview.files[0];
      preview.files = Array.from({ length: 5 }, (_, i) => ({
        ...file,
        path: `etc/sysctl.d/${i}.conf`,
        action: i === 0 ? "remove" : i === 1 ? "create" : "replace",
        comparison: { ...file.comparison, path: `etc/sysctl.d/${i}.conf` },
      }));
      await route.fulfill({ json: preview });
    },
  );
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Wiederherstellungsvorschau",
    exact: true,
  });
  await expect(
    dialog.getByRole("region", { name: "Zusätzlich betroffene Werte" }),
  ).toContainText("Swap-Neigung");
  await expect(
    dialog.locator(".rollback-additional .rollback-values strong"),
  ).toHaveText(["10", "60"]);
  await expect(dialog).toContainText("Entfernung erfordern");
  await expect(dialog).toContainText("Datei wieder anlegen");
  await expect(dialog.locator(".rollback-file-card")).toHaveCount(4);
  await dialog
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  await expect(dialog.locator(".rollback-file-card")).toHaveCount(1);
});

test("failed and mismatching previews never display a rollback outcome", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  let calls = 0;
  await page.route(
    "**/api/backups/*/change-restore-preview?*",
    async (route) => {
      if (++calls === 1)
        return route.fulfill({
          status: 409,
          json: { error: "Werte haben sich geändert. Neu prüfen." },
        });
      const response = await route.fetch();
      const preview = await response.json();
      preview.source.id = "wrong-source";
      return route.fulfill({ json: preview });
    },
  );
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Wiederherstellungsvorschau",
    exact: true,
  });
  await expect(dialog).toContainText("Werte haben sich geändert");
  await expect(dialog.locator(".rollback-value-card")).toHaveCount(0);
  await dialog
    .getByRole("button", { name: "Wiederherstellungsvorschau erneut laden" })
    .click();
  await expect(dialog).toContainText("passt nicht mehr");
  await expect(dialog.locator(".rollback-file-card")).toHaveCount(0);
});

test("closing a loading preview discards its late response", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  let release!: () => void,
    started = false;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(
    "**/api/backups/*/change-restore-preview?*",
    async (route) => {
      started = true;
      const response = await route.fetch();
      await gate;
      await route.fulfill({ response }).catch(() => {});
    },
  );
  await trigger.click();
  await expect.poll(() => started).toBe(true);
  await page
    .getByRole("dialog", { name: "Wiederherstellungsvorschau", exact: true })
    .getByRole("button", { name: "Zur Änderung zurück" })
    .click();
  release();
  await expect(
    page.getByRole("dialog", {
      name: "Wiederherstellungsvorschau",
      exact: true,
    }),
  ).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("the setting trail offers the same read-only return preview", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  await page.goto("/");
  await page
    .getByRole("button", { name: "pve-berlin-01", exact: true })
    .click();
  await page
    .locator(".host-tabs")
    .getByRole("button", { name: "Änderungen", exact: true })
    .click();
  const trace = page.locator(".setting-trace");
  await trace.locator(":scope > summary").click();
  await trace
    .getByRole("button", { name: "Wiederherstellungsvorschau", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Wiederherstellungsvorschau",
    exact: true,
  });
  await expect(dialog.locator(".rollback-values strong").first()).toHaveText(
    "Deaktiviert",
  );
  await expect(dialog).toContainText("frisch gelesener Zielzustand");
});

test("handoff keeps source, node and selected files but requires a fresh inspection and explicit decisions", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  let preview: any;
  page.on("response", async (r) => {
    if (r.url().includes("/change-restore-preview?")) preview = await r.json();
  });
  const writes: { path: string; body: any }[] = [];
  page.on("request", (r) => {
    if (r.method() === "POST")
      writes.push({ path: new URL(r.url()).pathname, body: r.postDataJSON() });
  });
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Wiederherstellungsvorschau",
    exact: true,
  });
  const proceed = dialog.getByRole("button", {
    name: "Im Wiederherstellungsassistenten prüfen",
  });
  await expect(proceed).toBeEnabled();
  const checkbox = dialog.getByRole("checkbox", {
    name: "Für Assistenten auswählen: etc/sysctl.d/99-anker.conf",
  });
  await checkbox.uncheck();
  await expect(proceed).toBeDisabled();
  await checkbox.check();
  await proceed.click();
  await expect(dialog).toHaveCount(0);
  const wizard = page.getByRole("region", {
    name: "Wiederherstellung vorbereiten",
    exact: true,
  });
  await expect(wizard).toBeVisible();
  await expect(page.getByLabel("Sicherung", { exact: true })).toHaveValue(
    preview.source.id,
  );
  await expect(
    wizard.getByRole("region", { name: "Aus der Änderung übernommen" }),
  ).toContainText("Aktuelle Zielprüfung steht noch aus");
  await expect(
    page.getByLabel("etc/sysctl.d/99-anker.conf", { exact: true }),
  ).toBeChecked();
  expect(writes).toEqual([]);
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  const status = await (await page.request.get("/api/status")).json();
  const source = status.backups.find((b: any) => b.id === preview.source.id);
  await expect(page.getByLabel("Zielhost", { exact: true })).toHaveValue(
    source.host_id,
  );
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeEnabled();
  const inspect = writes.find((r) => r.path === "/api/plans/inspect")!;
  expect(inspect.body).toMatchObject({
    backup_id: preview.source.id,
    target_id: source.host_id,
    scenario: "files",
    files: ["etc/sysctl.d/99-anker.conf"],
    decisions: {},
  });
  expect(inspect.body.snapshot_id).toBeUndefined();
  expect(writes.filter((r) => r.path === "/api/plans")).toHaveLength(0);
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await expect(
    page.getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", {
      exact: true,
    }),
  ).toHaveValue("");
  await expect(
    page.getByRole("button", { name: "Weiter zur Planprüfung", exact: true }),
  ).toBeDisabled();
  // Changing the scope invalidates the freshly read target again.
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page.getByRole("button", { name: "Zurück", exact: true }).click();
  await page
    .getByRole("button", { name: "Nur ausgewählte Dateien", exact: true })
    .click();
  await page
    .getByLabel("Wiederherstellungsdateien durchsuchen")
    .fill("etc/sysctl.d/99-anker.conf");
  await page
    .getByLabel("etc/sysctl.d/99-anker.conf", { exact: true })
    .uncheck();
  await expect(
    page.getByRole("region", { name: "Aus der Änderung übernommen" }),
  ).toContainText("Auswahl gegenüber der Vorschau geändert");
  await page.getByLabel("etc/sysctl.d/99-anker.conf", { exact: true }).check();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeDisabled();
});

test("only selected files reach the assistant and a new handoff does not reuse an earlier inspection", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  await page.route(
    "**/api/backups/*/change-restore-preview?*",
    async (route) => {
      const response = await route.fetch();
      const data = await response.json();
      data.files.push({
        ...data.files[0],
        path: "etc/hostname",
        comparison: { ...data.files[0].comparison, path: "etc/hostname" },
      });
      await route.fulfill({ json: data });
    },
  );
  await trigger.click();
  await page
    .getByRole("checkbox", {
      name: "Für Assistenten auswählen: etc/hostname",
      exact: true,
    })
    .uncheck();
  await page
    .getByRole("button", { name: "Im Wiederherstellungsassistenten prüfen" })
    .click();
  await expect(
    page.locator(".restore-file-items input[type=checkbox]"),
  ).toHaveCount(1);
  await expect(
    page.getByLabel("etc/sysctl.d/99-anker.conf", { exact: true }),
  ).toBeChecked();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeEnabled();
  await page
    .getByRole("button", { name: "Vorbereitung schließen", exact: true })
    .click();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  await page
    .getByRole("button", { name: "pve-berlin-01", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Konfigurationsreise öffnen" })
    .click();
  await page
    .getByRole("button", { name: "Wiederherstellungsvorschau", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Im Wiederherstellungsassistenten prüfen" })
    .click();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeDisabled();
});

test("removals require explicit selection before entering the assistant", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  await page.route(
    "**/api/backups/*/change-restore-preview?*",
    async (route) => {
      const response = await route.fetch();
      const data = await response.json();
      data.files[0].action = "remove";
      await route.fulfill({ json: data });
    },
  );
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Wiederherstellungsvorschau",
    exact: true,
  });
  const choice = dialog.getByRole("checkbox");
  await expect(choice).not.toBeChecked();
  const proceed = dialog.getByRole("button", {
    name: "Im Wiederherstellungsassistenten prüfen",
  });
  await expect(proceed).toBeDisabled();
  await choice.check();
  await proceed.click();
  await expect(
    page.getByRole("region", { name: "Vorgemerkte Dateientfernungen" }),
  ).toContainText("etc/sysctl.d/99-anker.conf");
});

test("a source whose manifest changed cannot be handed off", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  await page.route(
    "**/api/backups/*/change-restore-preview?*",
    async (route) => {
      const response = await route.fetch();
      const data = await response.json();
      data.source.manifest_sha = "stale-manifest";
      await route.fulfill({ json: data });
    },
  );
  await trigger.click();
  await page
    .getByRole("button", { name: "Im Wiederherstellungsassistenten prüfen" })
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "Quelle oder Ziel ist nicht mehr verfügbar",
  );
  await expect(
    page.getByRole("dialog", {
      name: "Wiederherstellungsvorschau",
      exact: true,
    }),
  ).toBeVisible();
});

test("handoff reaches the existing guarded demo execution after current inspection", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  await trigger.click();
  await page
    .getByRole("button", { name: "Im Wiederherstellungsassistenten prüfen" })
    .click();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await page
    .getByRole("button", { name: "Weiter zu Entscheidungen", exact: true })
    .click();
  await page
    .getByLabel("Entscheidung für etc/sysctl.d/99-anker.conf", { exact: true })
    .selectOption("backup");
  await page
    .getByRole("button", { name: "Weiter zur Planprüfung", exact: true })
    .click();
  const response = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === "/api/plans" &&
      r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Plan prüfen", exact: true }).click();
  const plan = await (await response).json();
  await expect(
    page.getByRole("button", { name: "In Demo anwenden", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("Plan-ID zur Ausführung eingeben").fill("wrong-id");
  await expect(
    page.getByRole("button", { name: "In Demo anwenden", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("Plan-ID zur Ausführung eingeben").fill(plan.id);
  await page
    .getByRole("button", { name: "In Demo anwenden", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan", exact: true }),
  ).toContainText("Demo angewendet");
});

test("readers can inspect historical evidence but cannot hand off a restore", async ({
  page,
  request,
}) => {
  await page.route("**/api/me", async (route) => {
    const response = await route.fetch();
    const data = await response.json();
    data.user.role = "reader";
    await route.fulfill({ json: data });
  });
  const trigger = await setup(page, request);
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Wiederherstellungsvorschau",
    exact: true,
  });
  await expect(dialog.locator(".rollback-values strong").first()).toHaveText(
    "Deaktiviert",
  );
  await expect(
    dialog.getByRole("button", {
      name: "Im Wiederherstellungsassistenten prüfen",
    }),
  ).toHaveCount(0);
  await expect(dialog.getByRole("checkbox")).toHaveCount(0);
});

test("missing handed-off source files stop preparation instead of silently shrinking its scope", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  await trigger.click();
  await page.route("**/api/backups/*/files", (route) =>
    route.fulfill({ json: [] }),
  );
  await page
    .getByRole("button", { name: "Im Wiederherstellungsassistenten prüfen" })
    .click();
  await expect(
    page.getByRole("region", {
      name: "Wiederherstellung vorbereiten",
      exact: true,
    }),
  ).toContainText("Eine übergebene Datei fehlt");
  await expect(
    page.getByRole("button", { name: "Weiter zum Ziel", exact: true }),
  ).toBeDisabled();
});

test("failed current target inspection never enables planning", async ({
  page,
  request,
}) => {
  const trigger = await setup(page, request);
  await trigger.click();
  await page
    .getByRole("button", { name: "Im Wiederherstellungsassistenten prüfen" })
    .click();
  await page
    .getByRole("button", { name: "Weiter zum Ziel", exact: true })
    .click();
  await page.route("**/api/plans/inspect", (route) =>
    route.fulfill({ status: 409, json: { error: "Zielzustand nicht lesbar" } }),
  );
  await page.getByRole("button", { name: "Ziel prüfen", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Zielzustand nicht lesbar",
  );
  await expect(
    page.getByRole("button", { name: "Weiter zu Entscheidungen", exact: true }),
  ).toBeDisabled();
});
