import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
for (const width of [1440, 390])
  test(`machine attempts stay separate from human and operation state at ${width}`, async ({
    page,
    request,
  }) => {
    await page.context().addCookies(await demoCookies(request));
    await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 });
    const base = await (await page.request.get("/api/status")).json();
    const plan: any = {
      id: "verification-plan",
      backup_id: base.backups[0].id,
      target_id: base.hosts[0].id,
      created_at: "2026-10-09T10:00:00Z",
      scenario: "files",
      source: base.hosts[0].inventory,
      target: base.hosts[0].inventory,
      steps: [],
      manual: [],
      mapping: { interfaces: {}, storage: {} },
      schema_version: 2,
      target_snapshot_id: "snapshot",
      state: "checks_pending",
      blockers: [],
      tasks: [
        {
          id: "human",
          kind: "manual-check",
          title: "Konsole prüfen",
          instruction: "Vor Ort prüfen",
          requires: [],
          state: "pending",
        },
      ],
      verification_attempts: [],
    };
    await page.route("**/api/status", (r) =>
      r.fulfill({ json: { ...base, plans: [plan], jobs: [] } }),
    );
    await page.route("**/api/plans/verification-plan", (r) =>
      r.fulfill({ json: plan }),
    );
    let count = 0;
    await page.route("**/api/plans/verification-plan/verify", async (r) => {
      expect(r.request().method()).toBe("POST");
      expect(r.request().postDataJSON()).toEqual({});
      count++;
      plan.verification_attempts.push({
        id: `attempt-${count}`,
        state: count === 1 ? "failed" : "observed",
        started_at: "2026-10-09T10:00:00Z",
        finished_at: "2026-10-09T10:00:01Z",
        origin: "machine-read",
        actor: "admin",
        plan_id: plan.id,
        backup_id: plan.backup_id,
        target_id: plan.target_id,
        identity_state: count === 1 ? "unknown" : "passed",
        reboot_state: count === 1 ? "unknown" : "passed",
        services_state: count === 1 ? "unknown" : "passed",
        volumes_state: "unknown",
        reason:
          count === 1
            ? "SSH-Prüfung fehlgeschlagen"
            : "Nur beobachteter Zustand",
      });
      await r.fulfill({ json: plan });
    });
    await page.goto("/");
    if (width === 390)
      await page.getByRole("button", { name: "Navigation öffnen" }).click();
    await page
      .getByRole("navigation", { name: "Hauptnavigation" })
      .getByRole("button", { name: "Wiederherstellung", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Plan ansehen", exact: true })
      .click();
    const region = page.getByRole("region", {
      name: "Maschinelle Prüfversuche",
    });
    await expect(region).toContainText("Keine frische maschinelle Prüfung");
    await region
      .getByRole("button", { name: "Maschinell prüfen", exact: true })
      .click();
    await expect(region).toContainText("SSH-Prüfung fehlgeschlagen");
    await region
      .getByRole("button", { name: "Maschinell prüfen", exact: true })
      .click();
    await expect(region).toContainText("attempt-1");
    await expect(region).toContainText("attempt-2");
    await expect(region).toContainText("Neustart: nachgewiesen");
    await expect(page.getByLabel("Nachweis für Konsole prüfen")).toHaveValue(
      "",
    );
    expect(plan.state).toBe("checks_pending");
    expect(plan.tasks[0].state).toBe("pending");
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
  });
