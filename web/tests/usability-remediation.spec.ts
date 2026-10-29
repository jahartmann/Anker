import { test, expect, type Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import type { Status } from "../src/api";

async function fixture(
  page: Page,
  { lostReply = false, reader = false, hold = false } = {},
) {
  await page.context().addCookies(await demoCookies(page.request));
  const base = (await (await page.request.get("/api/status")).json()) as Status;
  const me = await (await page.request.get("/api/me")).json();
  const hosts = Array.from({ length: 4 }, (_, i) => ({
    ...base.hosts[0],
    id: `repair-host-${i}`,
    name: i === 0 ? "cpmusterort" : `pbs-test-${i}`,
    enabled: i !== 2,
  }));
  const backups = hosts.map((h, i) => ({
    ...base.backups[0],
    id: `old-repair-${i}`,
    host_id: h.id,
    host_name: h.name,
    status: "partial",
    warnings: ["Historical known warning"],
    issues: [
      {
        code: "legacy_systemd_path",
        title: "Älterer Helfer hat einen gültigen systemd-Pfad abgelehnt",
        detail: "Bekannter Pfadfehler",
        action: "Neu sichern",
      },
    ],
  }));
  const targets = hosts.map((h, i) => ({
    host_id: h.id,
    host_name: h.name,
    backup_id: backups[i].id,
    reasons: i === 1 ? ["lynx_optional"] : ["systemd_path"],
    state: i === 2 ? "paused" : i === 3 ? "busy" : "ready",
    job_id: i === 3 ? "existing-job" : undefined,
  }));
  const state = { ...base, hosts, backups, jobs: [] } as Status;
  const requests: any[] = [];
  let run: any = null,
    receipts = 0;
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/status", (r) => r.fulfill({ json: state }));
  if (reader)
    await page.route("**/api/me", (r) =>
      r.fulfill({
        json: { ...me, user: { ...me.user, role: "reader", secrets: false } },
      }),
    );
  await page.route("**/api/backup-remediation", async (r) => {
    if (r.request().method() === "GET")
      return r.fulfill({ json: { targets, other_hosts: 1 } });
    const body = r.request().postDataJSON();
    requests.push(body);
    if (!run) {
      run = {
        id: "batch-test",
        request: body,
        targets: targets.map((t) => ({
          ...t,
          state:
            t.state === "ready" || (t.state === "paused" && body.include_paused)
              ? "queued"
              : t.state,
          job_id:
            t.state === "ready" || (t.state === "paused" && body.include_paused)
              ? "job-" + t.host_id
              : t.job_id,
        })),
      };
      state.jobs = run.targets
        .filter((t: any) => t.state === "queued")
        .map((t: any) => ({
          id: t.job_id,
          host_id: t.host_id,
          kind: "backup",
          state: "running",
          created_at: new Date().toISOString(),
          started_at: new Date().toISOString(),
          attempts: 1,
        }));
    }
    if (hold) await gate;
    if (lostReply && requests.length === 1) return r.abort("failed");
    return r.fulfill({ json: run });
  });
  await page.route("**/api/backup-remediation/*", (r) => {
    receipts++;
    return run
      ? r.fulfill({ json: run })
      : r.fulfill({ status: 404, json: { error: "Not started" } });
  });
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  const card = page.getByRole("region", {
    name: "Sammelkorrektur bekannter Sicherungsprobleme",
  });
  await expect(card).toBeVisible();
  return {
    state,
    requests,
    getRun: () => run,
    release,
    card,
    receiptCount: () => receipts,
  };
}
test("one click processes the whole detected group and reports actual verified results", async ({
  page,
}) => {
  const { card, state, requests, release, getRun } = await fixture(page, {
    hold: true,
  });
  await card.screenshot({
    path: "/tmp/anker-remediation-action.png",
    animations: "disabled",
  });
  await card
    .getByRole("button", { name: "2 Hosts neu sichern & prüfen" })
    .click();
  await expect(
    card.getByRole("button", { name: "Wird eingereiht …" }),
  ).toBeDisabled();
  await card
    .getByRole("button", { name: "Wird eingereiht …" })
    .dispatchEvent("click");
  expect(requests).toHaveLength(1);
  expect(requests[0].sources).toHaveLength(4);
  expect(requests[0].include_paused).toBe(false);
  release();
  await expect(card).toContainText("0 vollständig gesichert & geprüft");
  await expect(card).toContainText("Pausiert");
  await expect(card).toContainText("Auftrag bereits aktiv");
  const [a, b] = state.jobs;
  a.state = "successful";
  a.result_id = "new-verified";
  b.state = "failed";
  b.result_id = "new-partial";
  state.backups.push({
    ...state.backups[0],
    id: a.result_id,
    created_at: new Date(Date.now() + 1000).toISOString(),
    status: "successful",
    warnings: [],
    issues: [],
    verified_at: undefined,
    verification_attempt: undefined,
  });
  state.backups.push({
    ...state.backups[1],
    id: b.result_id,
    created_at: new Date(Date.now() + 1000).toISOString(),
    status: "partial",
    verification_attempt: { state: "ok", at: new Date().toISOString() },
  });
  await expect(card).toContainText("Weitere Hinweise – Ergebnis prüfen", {
    timeout: 10000,
  });
  await expect(card).toContainText("0 vollständig gesichert & geprüft");
  const fresh = state.backups.find((x) => x.id === a.result_id)!;
  fresh.verified_at = new Date().toISOString();
  fresh.verification_attempt = { state: "ok", at: fresh.verified_at };
  await expect(card).toContainText("1 vollständig gesichert & geprüft", {
    timeout: 10000,
  });
  await expect(
    card.getByRole("progressbar", {
      name: "Beendete Aufträge der Sammelkorrektur",
    }),
  ).toHaveAttribute("value", "2");
  expect(getRun().request.request_id).toBe(requests[0].request_id);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
    await card.screenshot({
      path: `/tmp/anker-backup-remediation-${width}.png`,
      animations: "disabled",
    });
  }
});
test("lost replies reconcile the same request and reopening restores the receipt", async ({
  page,
}) => {
  const { card, requests, receiptCount } = await fixture(page, {
    lostReply: true,
  });
  await card
    .getByRole("button", { name: "2 Hosts neu sichern & prüfen" })
    .click();
  await expect(card.getByRole("alert")).toContainText("Abgleichen");
  await card.getByRole("button", { name: "Start abgleichen" }).click();
  await expect(card).toContainText("Sammelkorrektur im Blick");
  expect(requests).toHaveLength(2);
  expect(requests[1]).toEqual(requests[0]);
  await page.reload();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Sicherungen", exact: true })
    .click();
  await expect(
    page.getByRole("region", {
      name: "Sammelkorrektur bekannter Sicherungsprobleme",
    }),
  ).toContainText("Sammelkorrektur im Blick");
  expect(receiptCount()).toBeGreaterThan(0);
  expect(requests).toHaveLength(2);
});
test("paused schedules require one explicit group option", async ({ page }) => {
  const { card, requests } = await fixture(page);
  await card
    .getByRole("checkbox", {
      name: "Auch Hosts mit pausiertem Zeitplan einmalig einbeziehen",
    })
    .check();
  await card
    .getByRole("button", { name: "3 Hosts neu sichern & prüfen" })
    .click();
  await expect(card).toContainText("Sammelkorrektur im Blick");
  expect(requests[0].include_paused).toBe(true);
  expect(requests[0].sources).toHaveLength(4);
});
test("reader can inspect detected problems but cannot dispatch a batch", async ({
  page,
}) => {
  const { card, requests } = await fixture(page, { reader: true });
  await expect(
    card.getByRole("button", { name: /Hosts neu sichern/ }),
  ).toHaveCount(0);
  await expect(card).toContainText("benötigt Sicherungsberechtigung");
  await card.getByRole("button", { name: "Betroffene Hosts ansehen" }).click();
  await expect(card).toContainText("CP Musterort");
  expect(requests).toHaveLength(0);
});
