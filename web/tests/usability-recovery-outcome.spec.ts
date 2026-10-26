import { test, expect } from "@playwright/test";
import { writeFile, readFile } from "node:fs/promises";
import { demoCookies } from "./demo-auth";

async function restored(page: any, request: any) {
  await writeFile(
    `/tmp/anker-browser-${process.env.ANKER_BROWSER_RUN_ID}/demo-hosts/demo1/files/etc/sysctl.d/99-anker.conf`,
    "net.ipv4.ip_forward=0\n",
  );
  await page.context().addCookies(await demoCookies(request));
  await page.goto("/");
  const status = await (await page.request.get("/api/status")).json();
  const backup = status.backups
    .filter((b: any) => b.host_id === "demo1")
    .sort((a: any, b: any) => a.created_at.localeCompare(b.created_at))[0];
  const post = async (path: string, data: any) => {
    const r = await page.request.post(`/api/${path}`, {
      headers: { "X-Anker-Request": "1" },
      data,
    });
    expect(r.ok(), await r.text()).toBeTruthy();
    return r.json();
  };
  const body = {
    backup_id: backup.id,
    target_id: backup.host_id,
    scenario: "files",
    files: ["etc/sysctl.d/99-anker.conf"],
  };
  const inspection = await post("plans/inspect", body);
  const plan = await post("plans", {
    ...body,
    snapshot_id: inspection.snapshot_id,
    decisions: { "etc/sysctl.d/99-anker.conf": "backup" },
  });
  const job = await post(`plans/${plan.id}/apply`, { confirmation: plan.id });
  await expect
    .poll(async () => {
      const s = await (await page.request.get("/api/status")).json();
      return s.jobs.find((j: any) => j.id === job.id)?.state;
    })
    .toBe("successful");
  await page.reload();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan ansehen", exact: true })
    .first()
    .click();
  return {
    plan,
    post,
    region: page.getByRole("region", {
      name: "Ergebnis der Wiederherstellung",
      exact: true,
    }),
  };
}

test("automatic result cards confirm actual files and declarations; a reread shows drift with evidence", async ({
  page,
  request,
}, info) => {
  const { region } = await restored(page, request);
  await expect(region).toContainText("Die geprüften Dateien stimmen überein");
  await expect(
    region.getByRole("region", { name: "Geprüfte Einstellungen" }),
  ).toContainText("IPv4-Weiterleitung");
  await expect(region.locator(".outcome-setting-values strong")).toHaveText([
    "Aktiviert",
    "Aktiviert",
  ]);
  await expect(region).toContainText("Lokale Demodateien");
  const root = `/tmp/anker-browser-${process.env.ANKER_BROWSER_RUN_ID}/demo-hosts/demo1/files/etc/sysctl.d`;
  await writeFile(
    `${root}/99-anker.conf`,
    "# Abweichung nach der Übernahme\nnet.ipv4.ip_forward=0\n",
  );
  const writes: string[] = [];
  page.on("request", (r: any) => {
    if (r.method() === "POST") writes.push(new URL(r.url()).pathname);
  });
  await region
    .getByRole("button", {
      name: "Dateien und Einstellungen erneut prüfen",
      exact: true,
    })
    .click();
  await expect(region).toContainText("Hier weicht das Ziel noch ab");
  await expect(region.locator(".outcome-setting-values strong")).toHaveText([
    "Aktiviert",
    "Deaktiviert",
  ]);
  const file = region.locator(".outcome-file-card");
  await expect(file).toContainText("Dateiinhalt weicht vom Sollstand ab");
  await file.getByText("Nachweis zur Datei ansehen", { exact: true }).click();
  await expect(file.locator(".diff-remove")).toContainText([
    "net.ipv4.ip_forward=1",
  ]);
  await expect(file.locator(".diff-add")).toContainText([
    "net.ipv4.ip_forward=0",
  ]);
  expect(writes).toHaveLength(1);
  expect(writes[0]).toMatch(/\/verify-files$/);
  await region.getByText("1 frühere Prüfversuche", { exact: true }).click();
  await expect(region).toContainText("Bestätigt");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await region.screenshot({
    path: info.outputPath("outcome-drift-mobile.png"),
  });
});

test("failed or interrupted checks replace success with an explicit missing proof and allow retry", async ({
  page,
  request,
}) => {
  // The preceding test deliberately left the running fixture changed. Restore it via the normal plan again.
  const { region, plan } = await restored(page, request);
  await page.route(`**/api/plans/${plan.id}/verify-files`, async (route) => {
    const response = await route.fetch();
    const p = await response.json();
    const latest = p.outcome_checks.at(-1);
    latest.state = "unknown";
    latest.reason = "Aktuelle Zielabfrage fehlgeschlagen";
    latest.files = [];
    latest.settings = [];
    await route.fulfill({ json: p });
  });
  await region
    .getByRole("button", {
      name: "Dateien und Einstellungen erneut prüfen",
      exact: true,
    })
    .click();
  await expect(region).toContainText("Aktuelle Zielabfrage fehlgeschlagen");
  await expect(
    region.getByRole("region", { name: "Geprüfte Dateien" }),
  ).toHaveCount(0);
  await expect(
    region.getByRole("heading", {
      name: "Die geprüften Dateien stimmen überein.",
    }),
  ).toHaveCount(0);
  await page.unroute(`**/api/plans/${plan.id}/verify-files`);
  await region
    .getByRole("button", {
      name: "Dateien und Einstellungen erneut prüfen",
      exact: true,
    })
    .click();
  await expect(region).toContainText("Die geprüften Dateien stimmen überein");
});

test("protected changed contents never appear in result evidence", async ({
  page,
  request,
}) => {
  const root = `/tmp/anker-browser-${process.env.ANKER_BROWSER_RUN_ID}/demo-hosts/demo1/files/etc/sysctl.d`;
  await writeFile(`${root}/99-anker.conf`, "net.ipv4.ip_forward=0\n");
  const { region } = await restored(page, request);
  const secret = "outcome-browser-private-789";
  await writeFile(
    `${root}/99-anker.conf`,
    `password=${secret}\nnet.ipv4.ip_forward=0\n`,
  );
  await region
    .getByRole("button", {
      name: "Dateien und Einstellungen erneut prüfen",
      exact: true,
    })
    .click();
  await expect(region).toContainText("Geschützter Inhalt");
  await region.getByText("Nachweis zur Datei ansehen", { exact: true }).click();
  await expect(region).not.toContainText(secret);
  await expect(region.locator(".file-comparison")).toHaveCount(0);
  expect(await readFile(`${root}/99-anker.conf`, "utf8")).toContain(secret);
});

test("network failures never leave an old green result looking current", async ({
  page,
  request,
}) => {
  const { region, plan } = await restored(page, request);
  await page.route(`**/api/plans/${plan.id}/verify-files`, (route) =>
    route.abort("connectionfailed"),
  );
  await region
    .getByRole("button", {
      name: "Dateien und Einstellungen erneut prüfen",
      exact: true,
    })
    .click();
  await expect(region).toContainText("Erneute Zielprüfung nicht abgeschlossen");
  await expect(
    region.getByRole("region", { name: "Geprüfte Dateien" }),
  ).toHaveCount(0);
  await expect(
    region.getByRole("heading", {
      name: "Die geprüften Dateien stimmen überein.",
    }),
  ).toHaveCount(0);
});

test("readers see recorded results but cannot trigger a new host read", async ({
  page,
  request,
}) => {
  await restored(page, request);
  await page.route("**/api/me", async (route) => {
    const response = await route.fetch();
    const data = await response.json();
    data.user.role = "reader";
    await route.fulfill({ json: data });
  });
  await page.reload();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Wiederherstellung", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan ansehen", exact: true })
    .first()
    .click();
  const region = page.getByRole("region", {
    name: "Ergebnis der Wiederherstellung",
    exact: true,
  });
  await expect(region).toContainText("Die geprüften Dateien stimmen überein");
  await expect(
    region.getByRole("button", {
      name: "Dateien und Einstellungen erneut prüfen",
      exact: true,
    }),
  ).toHaveCount(0);
});

test("many results remain paged and a mismatched plan response cannot replace current evidence", async ({
  page,
  request,
}) => {
  const { region, plan } = await restored(page, request);
  let mismatch = false;
  await page.route(`**/api/plans/${plan.id}/verify-files`, async (route) => {
    const response = await route.fetch();
    const data = await response.json();
    const result = data.outcome_checks.at(-1);
    const file = result.files[0];
    result.files = Array.from({ length: 9 }, (_, i) => ({
      ...file,
      path: `etc/sysctl.d/check-${i}.conf`,
    }));
    if (mismatch) data.id = "different-plan";
    await route.fulfill({ json: data });
  });
  await region
    .getByRole("button", {
      name: "Dateien und Einstellungen erneut prüfen",
      exact: true,
    })
    .click();
  await expect(region.locator(".outcome-file-card")).toHaveCount(4);
  await region
    .getByRole("button", { name: "Nächste Seite", exact: true })
    .click();
  await expect(region.locator(".outcome-file-card")).toHaveCount(4);
  mismatch = true;
  await region
    .getByRole("button", {
      name: "Dateien und Einstellungen erneut prüfen",
      exact: true,
    })
    .click();
  await expect(region).toContainText(
    "Prüfantwort gehört zu einem anderen Plan",
  );
  await expect(region.locator(".outcome-file-card")).toHaveCount(0);
});
