import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";

test.beforeEach(async ({ page, request }) => {
  await page.context().addCookies(await demoCookies(request));
});

async function backupsPage(page: Page) {
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await page.getByRole("button", { name: "Alle Stände", exact: true }).click();
}

test("direct previous comparison uses the strictly older same-host stand and can restore either source", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const backups = [
    {
      ...base.backups[0],
      id: "newest123456",
      host_id: "h1",
      host_name: "Node A",
      created_at: "2026-10-07T10:00:00Z",
    },
    {
      ...base.backups[0],
      id: "otherhost",
      host_id: "h2",
      host_name: "Node B",
      created_at: "2026-10-07T09:59:00Z",
    },
    {
      ...base.backups[0],
      id: "equal-time",
      host_id: "h1",
      host_name: "Node A",
      created_at: "2026-10-07T10:00:00Z",
    },
    {
      ...base.backups[0],
      id: "older1234567",
      host_id: "h1",
      host_name: "Node A",
      created_at: "2026-10-07T09:58:00Z",
    },
  ];
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, backups, jobs: [] } }),
  );
  const pairs: { from: string; to: string }[] = [];
  await page.route("**/api/backups/compare", (r) => {
    pairs.push(r.request().postDataJSON());
    return r.fulfill({ json: [] });
  });
  await backupsPage(page);
  await page
    .getByRole("row", { name: /Node A.*newest1234/ })
    .getByRole("button", { name: /^Aktionen für/ })
    .click();
  await page
    .getByRole("button", { name: "Mit vorherigem Stand vergleichen" })
    .click();
  const view = page.getByRole("region", { name: "Sicherungsvergleich" });
  await expect(view).toBeVisible();
  await expect(view.getByLabel("Vergleich von")).toHaveValue("older1234567");
  await expect(view.getByLabel("Vergleich bis")).toHaveValue("newest123456");
  await view.getByText("Vollständige ID", { exact: true }).first().click();
  await expect(view.getByText("older1234567", { exact: true })).toBeVisible();
  expect(pairs.length).toBeGreaterThan(0);
  // The overview and explicit detail view may each read this pair.
  expect(
    pairs.every(
      (pair) => pair.from === "older1234567" && pair.to === "newest123456",
    ),
  ).toBe(true);
  await view
    .getByText("Wiederherstellung aus diesen Ständen vorbereiten", {
      exact: true,
    })
    .click();
  await expect(
    view.getByRole("button", {
      name: "Ausgangsstand zur Wiederherstellung wählen",
    }),
  ).toBeVisible();
  await expect(
    view.getByRole("button", {
      name: "Vergleichsstand zur Wiederherstellung wählen",
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Zurück zu Sicherungen" }).click();
  await expect(page.getByRole("table", { name: "Sicherungen" })).toBeVisible();
  await page
    .getByRole("row", { name: /Node A.*older12345/ })
    .getByRole("button", { name: /^Aktionen für/ })
    .click();
  await expect(
    page.getByRole("button", {
      name: "Mit vorherigem Stand vergleichen",
      exact: true,
    }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", {
      name: "Mit vorherigem Stand vergleichen",
      exact: true,
    }),
  ).toHaveAttribute("title", "Kein älterer Stand dieses Hosts");
  await page.keyboard.press("Escape");
  await page
    .getByRole("row", { name: /Node A.*newest1234/ })
    .getByRole("button", { name: /^Aktionen für/ })
    .click();
  await page
    .getByRole("button", { name: "Mit vorherigem Stand vergleichen" })
    .click();
  await view
    .getByText("Wiederherstellung aus diesen Ständen vorbereiten", {
      exact: true,
    })
    .click();
  await view
    .getByRole("button", { name: "Ausgangsstand zur Wiederherstellung wählen" })
    .click();
  await expect(page.getByLabel("Sicherung", { exact: true })).toHaveValue(
    "older1234567",
  );
  await backupsPage(page);
  await page
    .getByRole("row", { name: /Node A.*newest1234/ })
    .getByRole("button", { name: /^Aktionen für/ })
    .click();
  await page
    .getByRole("button", { name: "Mit vorherigem Stand vergleichen" })
    .click();
  await view
    .getByText("Wiederherstellung aus diesen Ständen vorbereiten", {
      exact: true,
    })
    .click();
  await view
    .getByRole("button", {
      name: "Vergleichsstand zur Wiederherstellung wählen",
    })
    .click();
  await expect(page.getByLabel("Sicherung", { exact: true })).toHaveValue(
    "newest123456",
  );
});

test("focused source readiness keeps missing proof and failed integrity attempt distinct", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const backup = {
    ...base.backups[0],
    id: "source-evidence",
    host_id: "h1",
    host_name: "Node A",
    verification_error: "failed",
    verified_at: "2026-10-08T10:01:00Z",
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, backups: [backup], jobs: [] } }),
  );
  await page.route("**/api/backups/source-evidence/readiness", (r) =>
    r.fulfill({
      json: {
        version: 1,
        backup_id: backup.id,
        items: [
          {
            id: "integrity",
            state: "failed",
            title: "Integrität",
            detail: "Letzter Prüfversuch fehlgeschlagen",
            at: "2026-10-08T10:01:00Z",
            next_action: "Sicherung erneut prüfen",
          },
          {
            id: "offline_drill",
            state: "unknown",
            title: "Offlineprobe",
            detail: "Kein Nachweis",
            next_action: "Probe durchführen",
          },
        ],
        coverage: [
          {
            path: "etc/network",
            state: "missing",
            reason: "Nicht erfasst",
            authority: "required",
          },
        ],
        dependencies: [
          {
            path: "etc/pve/nodes",
            source: "etc/pve",
            state: "unknown",
            reason: "Ungeklärt",
          },
        ],
      },
    }),
  );
  await page.route("**/api/backups/source-evidence/files", (r) =>
    r.fulfill({ json: [] }),
  );
  await backupsPage(page);
  await page.getByRole("button", { name: "Dateien" }).click();
  await page.getByText("Sicherung und Prüfergebnisse ansehen").click();
  const readiness = page.getByRole("region", { name: "Prüfergebnisse" });
  await expect(readiness).toContainText("Letzter Prüfversuch fehlgeschlagen");
  await expect(readiness).toContainText("Kein Nachweis");
  await expect(readiness).toContainText("etc/network");
  await expect(readiness).toContainText("etc/pve/nodes");
  await expect(readiness).toContainText("Sicherung erneut prüfen");
});

test("same source refresh hides old integrity evidence before a changed attempt resolves", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  let failed = false;
  const backup = {
    ...base.backups[0],
    id: "changing-source",
    host_id: "h1",
    host_name: "Node A",
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({
      json: {
        ...base,
        backups: [
          {
            ...backup,
            status: failed ? "damaged" : "successful",
            verified_at: failed
              ? "2026-10-08T12:00:00Z"
              : "2026-10-08T10:00:00Z",
            verification_error: failed ? "failed" : "",
          },
        ],
        jobs: [],
      },
    }),
  );
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/backups/changing-source/readiness", async (r) => {
    if (failed) await gate;
    await r.fulfill({
      json: {
        version: 1,
        backup_id: backup.id,
        items: [
          {
            id: "integrity",
            state: failed ? "failed" : "ok",
            title: "Integrität",
            detail: failed
              ? "Letzter Versuch fehlgeschlagen"
              : "Letzter Versuch erfolgreich",
          },
        ],
        coverage: [],
        dependencies: [],
      },
    });
  });
  await backupsPage(page);
  await page.getByRole("button", { name: "Dateien" }).click();
  await page.getByText("Sicherung und Prüfergebnisse ansehen").click();
  const readiness = page.getByRole("region", { name: "Prüfergebnisse" });
  await expect(readiness).toContainText("Letzter Versuch erfolgreich");
  failed = true;
  await expect(readiness).not.toContainText("Letzter Versuch erfolgreich", {
    timeout: 10000,
  });
  release();
  await expect(readiness).toContainText("Letzter Versuch fehlgeschlagen");
});

test("overview and host detail load only their latest source, not historical rows", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const host = base.hosts[0];
  const backups = Array.from({ length: 120 }, (_, i) => ({
    ...base.backups[0],
    id: `history-${i}`,
    host_id: host.id,
    host_name: host.name,
    created_at: new Date(Date.UTC(2026, 9, 8, 0, i)).toISOString(),
  }));
  const requested: string[] = [];
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, backups, jobs: [] } }),
  );
  await page.route("**/api/backups/*/readiness", (r) => {
    const id = r.request().url().split("/").at(-2)!;
    requested.push(id);
    return r.fulfill({
      json: {
        version: 1,
        backup_id: id,
        items: [],
        coverage: [],
        dependencies: [],
      },
    });
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Übersicht", exact: true })
    .click();
  await page
    .getByText("Prüfergebnisse der letzten Sicherung", { exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Prüfergebnisse" }),
  ).toContainText("history-119");
  expect(requested).toEqual(["history-119"]);
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  await page.getByRole("button", { name: host.name, exact: true }).click();
  await page
    .getByText("Details zum Host und zur letzten Sicherung", { exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Prüfergebnisse" }),
  ).toContainText("history-119");
  expect(requested).toEqual(["history-119", "history-119"]);
});

test("saved plan opens as a page with a guarded back path and next task", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const plan = {
    id: "page-plan",
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    created_at: "2026-10-08T10:00:00Z",
    scenario: "files",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    steps: [],
    manual: [],
    mapping: { interfaces: {}, storage: {} },
    schema_version: 2,
    state: "ready",
    blockers: [],
    tasks: [
      {
        id: "inspect",
        kind: "manual-check",
        title: "Netzwerk prüfen",
        instruction: "An Konsole prüfen",
        requires: [],
        state: "pending",
      },
    ],
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, plans: [plan] } }),
  );
  await page.route("**/api/plans/page-plan", (r) => r.fulfill({ json: plan }));
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen" }).click();
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toBeVisible();
  await expect(page.getByText("Nächste Aufgabe")).toBeVisible();
  await expect(page.locator(".plan-next-task")).toContainText(
    "Netzwerk prüfen",
  );
  await page.getByRole("button", { name: "Zurück zu Plänen" }).click();
  await expect(
    page.getByRole("button", { name: "Plan ansehen" }),
  ).toBeVisible();
});

test("plan confirmation stays guarded through page back and main navigation", async ({
  page,
}) => {
  const base = await (await page.request.get("/api/status")).json();
  const plan = {
    id: "guard-plan",
    backup_id: base.backups[0].id,
    target_id: base.hosts[0].id,
    created_at: "2026-10-08T10:00:00Z",
    scenario: "files",
    source: base.hosts[0].inventory,
    target: base.hosts[0].inventory,
    steps: [],
    manual: [],
    mapping: { interfaces: {}, storage: {} },
    schema_version: 2,
    state: "ready",
    blockers: [],
    tasks: [],
  };
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, plans: [plan] } }),
  );
  await page.route("**/api/plans/guard-plan", (r) => r.fulfill({ json: plan }));
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page.getByRole("button", { name: "Plan ansehen" }).click();
  await page.getByLabel("Plan-ID zur Ausführung eingeben").fill("guard-plan");
  await page.getByRole("button", { name: "Zurück zu Plänen" }).click();
  await expect(
    page.getByText("Eingaben auf dieser Planseite sind noch offen."),
  ).toBeVisible();
  await page.getByRole("button", { name: "Weiter bearbeiten" }).click();
  await expect(page.getByLabel("Plan-ID zur Ausführung eingeben")).toHaveValue(
    "guard-plan",
  );
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Änderungen noch nicht gespeichert" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Weiter bearbeiten" }).click();
  await expect(
    page.getByRole("region", { name: "Wiederherstellungsplan" }),
  ).toBeVisible();
});

for (const changedEvidence of ["receipt", "manifest"]) {
  test(`same source refresh invalidates changed copy ${changedEvidence} evidence`, async ({
    page,
  }) => {
    const base = await (await page.request.get("/api/status")).json();
    let failed = false;
    const backup = {
      ...base.backups[0],
      id: "changing-source",
      host_id: "h1",
      host_name: "Node A",
    };
    await page.route("**/api/status", (r) =>
      r.fulfill({
        json: {
          ...base,
          backups: [
            {
              ...backup,
              status: "successful",
              manifest_sha:
                failed && changedEvidence === "manifest"
                  ? "new-manifest"
                  : "old-manifest",
              copy_verification: {
                attempt_at:
                  failed && changedEvidence === "receipt"
                    ? "new-attempt"
                    : "old-attempt",
                state:
                  failed && changedEvidence === "receipt" ? "failed" : "ok",
              },
              verified_at: "2026-10-08T10:00:00Z",
              verification_error: "",
            },
          ],
          jobs: [],
        },
      }),
    );
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route("**/api/backups/changing-source/readiness", async (r) => {
      if (failed) await gate;
      await r.fulfill({
        json: {
          version: 1,
          backup_id: backup.id,
          items: [
            {
              id: "independent_copy",
              state: failed ? "failed" : "ok",
              title: "Geprüfte Kopie",
              detail: failed
                ? "Kopienachweis veraltet"
                : "Kopie anhand tatsächlicher Bytes geprüft",
            },
          ],
          coverage: [],
          dependencies: [],
        },
      });
    });
    await backupsPage(page);
    await page.getByRole("button", { name: "Dateien" }).click();
    await page.getByText("Sicherung und Prüfergebnisse ansehen").click();
    const readiness = page.getByRole("region", { name: "Prüfergebnisse" });
    await expect(readiness).toContainText(
      "Kopie anhand tatsächlicher Bytes geprüft",
    );
    failed = true;
    await expect(readiness).not.toContainText(
      "Kopie anhand tatsächlicher Bytes geprüft",
      {
        timeout: 10000,
      },
    );
    release();
    await expect(readiness).toContainText("Kopienachweis veraltet");
  });
}

test("local CLI byte receipt appears with date and distinct unknown physical independence", async ({
  page,
}) => {
  const { cp, mkdtemp, rm } = await import("node:fs/promises");
  const { execFile } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const { fileURLToPath } = await import("node:url");
  const run = promisify(execFile);
  const base = await (await page.request.get("/api/status")).json();
  const backup = base.backups[0];
  expect(backup.archived).toBe(false);
  const dataRoot = `/tmp/anker-browser-${process.env.ANKER_BROWSER_RUN_ID}`;
  const copyRoot = await mkdtemp("/tmp/anker-copy-browser-");
  try {
    await cp(
      `${dataRoot}/hosts/${backup.host_id}/backups/${backup.id}`,
      copyRoot,
      { recursive: true },
    );
    const { stdout } = await run(
      fileURLToPath(new URL("../../bin/anker", import.meta.url)),
      ["--data", dataRoot, "backup", "verify-copy", backup.id, copyRoot],
    );
    const receipt = JSON.parse(stdout);
    expect(receipt.state).toBe("ok");
    expect(receipt.failure_domain).toBe("unknown");
    expect(stdout).not.toContain(copyRoot);
    await backupsPage(page);
    await page
      .getByRole("button", { name: "Dateien", exact: true })
      .first()
      .click();
    await page.getByText("Sicherung und Prüfergebnisse ansehen").click();
    const readiness = page.getByRole("region", { name: "Prüfergebnisse" });
    await expect(readiness).toContainText("tatsächlicher Bytes geprüft");
    await expect(readiness).toContainText("Lokaler Byte-Prüfer");
    await expect(readiness).toContainText(
      "Ein separates Verzeichnis belegt keinen getrennten Datenträger",
    );
    await expect(readiness).not.toContainText(copyRoot);
    for (const [label, width, height] of [
      ["desktop", 1440, 1000],
      ["mobile", 390, 844],
    ] as const) {
      await page.setViewportSize({ width, height });
      await readiness.scrollIntoViewIfNeeded();
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(width);
      await page.screenshot({
        path: `/tmp/anker-task5-${label}.png`,
        fullPage: true,
      });
    }
  } finally {
    await rm(copyRoot, { recursive: true, force: true });
  }
});
