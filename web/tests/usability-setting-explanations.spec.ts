import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import { explainSettingChange } from "../src/settingExplanations";
import type { SettingObservation } from "../src/settingHistory";
const observation = (
  id: string,
  value: string,
  path = "etc/sysctl.d/99-anker.conf",
): SettingObservation => ({
  backup_id: id,
  created_at:
    id === "explain-old" ? "2026-01-01T10:00:00Z" : "2026-01-02T10:00:00Z",
  state: "value",
  value,
  sources: [path],
});

test("documented rules distinguish direction, relative swappiness and conditional impacts", () => {
  const off = explainSettingChange(
    "ipv4-forwarding",
    observation("explain-old", "1"),
    observation("explain-new", "0"),
  )!;
  expect(off.meaning).toContain("deaktiviert");
  expect(off.impact).toContain("nicht nachgewiesen");
  expect(off.check).toContain("Standardwerte");
  expect(
    explainSettingChange(
      "ipv4-forwarding",
      observation("explain-old", "0"),
      observation("explain-new", "1"),
    )!.meaning,
  ).toContain("aktiviert");
  expect(
    explainSettingChange(
      "ipv6-forwarding",
      observation("explain-old", "0"),
      observation("explain-new", "1"),
    )!.impact,
  ).toContain("accept_ra");
  const swap = explainSettingChange(
    "swappiness",
    observation("explain-old", "60"),
    observation("explain-new", "10"),
  )!;
  expect(swap.meaning).toContain("teurer");
  expect(swap.impact).toContain("keine Prozentangabe");
  expect(
    explainSettingChange(
      "swappiness",
      observation("explain-old", "10"),
      observation("explain-new", "60"),
    )!.meaning,
  ).toContain("günstiger");
  expect(
    explainSettingChange(
      "hostname",
      observation("explain-old", "one", "etc/hostname"),
      observation("explain-new", "two", "etc/hostname"),
    )!.impact,
  ).toContain("Proxmox-Cluster");
  expect(
    explainSettingChange(
      "dns-servers",
      observation("explain-old", "192.0.2.1", "etc/resolv.conf"),
      observation("explain-new", "192.0.2.2", "etc/resolv.conf"),
    )!.meaning,
  ).toContain("Reihenfolge");
});

test("unproven, protected, unchanged or unordered observations have no impact explanation", () => {
  const old = observation("explain-old", "1"),
    after = observation("explain-new", "0");
  for (const state of [
    "protected",
    "ambiguous",
    "unavailable",
    "unreliable",
    "not_recorded",
  ])
    expect(
      explainSettingChange("ipv4-forwarding", old, { ...after, state }),
    ).toBeNull();
  for (const input of [
    { ...after, value: "1" },
    { ...after, created_at: old.created_at },
    { ...after, order_uncertain: true },
    { ...after, sources: [] },
    { ...after, sources: ["etc/shadow"] },
    { ...after, backup_id: old.backup_id },
    { ...after, value: "oops" },
  ])
    expect(explainSettingChange("ipv4-forwarding", old, input)).toBeNull();
  expect(
    explainSettingChange(
      "swappiness",
      observation("explain-old", "60"),
      observation("explain-new", "201"),
    ),
  ).toBeNull();
  expect(explainSettingChange("unknown", old, after)).toBeNull();
});

async function setup(page: any, request: any, protectedFile = false) {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const host = { ...base.hosts[0], name: "pve-berlin-01" };
  const backups = [
    {
      ...base.backups[0],
      id: "explain-old",
      host_id: host.id,
      host_name: host.name,
      status: "successful",
      verification_error: "",
      created_at: "2026-01-01T10:00:00Z",
    },
    {
      ...base.backups[0],
      id: "explain-new",
      host_id: host.id,
      host_name: host.name,
      status: "successful",
      verification_error: "",
      created_at: "2026-01-02T10:00:00Z",
    },
  ];
  await page.route("**/api/status", (r) =>
    r.fulfill({ json: { ...base, hosts: [host], backups, jobs: [] } }),
  );
  await page.route("**/api/backups/compare", (r) =>
    r.fulfill({
      json: [
        {
          path: "etc/sysctl.d/99-anker.conf",
          change: "changed",
          content_changed: true,
          metadata_changed: false,
          mtime_only: false,
          secret: false,
        },
      ],
    }),
  );
  await page.route("**/api/backups/*/setting-snapshot", (r) => {
    const id = new URL(r.request().url()).pathname.split("/")[3];
    return r.fulfill({
      json: {
        backup_id: id,
        settings: [
          {
            setting: {
              id: "ipv4-forwarding",
              name: "IPv4-Weiterleitung",
              key: "net.ipv4.ip_forward",
              kind: "sysctl",
            },
            observation: observation(id, id === "explain-old" ? "1" : "0"),
          },
        ],
      },
    });
  });
  await page.route("**/api/hosts/*/setting-history?*", (r) =>
    r.fulfill({
      json: {
        setting: {
          id: "ipv4-forwarding",
          name: "IPv4-Weiterleitung",
          key: "net.ipv4.ip_forward",
          kind: "sysctl",
        },
        observations: [
          observation("explain-new", "0"),
          observation("explain-old", "1"),
        ],
        undated: 0,
      },
    }),
  );
  const reads: string[] = [];
  await page.route("**/api/backups/*/file?*", (r) => {
    const id = new URL(r.request().url()).pathname.split("/")[3];
    reads.push(id);
    return r.fulfill({
      json: {
        entry: { path: "etc/sysctl.d/99-anker.conf", secret: protectedFile },
        masked: protectedFile,
        content: protectedFile
          ? "must-not-leak"
          : `net.ipv4.ip_forward=${id === "explain-old" ? "1" : "0"}\n`,
      },
    });
  });
  await page.goto("/");
  await page.getByRole("button", { name: host.name, exact: true }).click();
  return {
    reads,
    journey: page.getByRole("region", { name: "Konfigurationsreise" }),
  };
}

test("journey explains only the observed change, then reads the exact original on demand", async ({
  page,
  request,
}, info) => {
  const { reads, journey } = await setup(page, request);
  await page
    .getByRole("button", { name: "Konfigurationsreise öffnen" })
    .click();
  const explanation = journey.getByRole("region", {
    name: "Bedeutung der Änderung",
  });
  await expect(explanation).toContainText("deaktiviert");
  await expect(explanation).toContainText("Mögliche Auswirkung");
  expect(reads).toHaveLength(0);
  await explanation.locator("summary").click();
  await expect(explanation).toContainText("explain-old");
  await expect(explanation).toContainText("explain-new");
  await expect(
    explanation.getByRole("link", { name: /Fachliche Grundlage/ }),
  ).toHaveAttribute(
    "href",
    "https://docs.kernel.org/networking/ip-sysctl.html",
  );
  expect(reads).toHaveLength(0);
  const before = explanation.getByRole("button", {
    name: "Quelldatei vorher ansehen: etc/sysctl.d/99-anker.conf",
  });
  await before.click();
  const dialog = page.getByRole("dialog", { name: "Gespeicherter Nachweis" });
  await expect(dialog.locator("pre")).toHaveText("net.ipv4.ip_forward=1");
  expect(reads).toEqual(["explain-old"]);
  await expect(dialog.getByLabel("Passende Deklarationen")).toContainText(
    "Zeile 1",
  );
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(before).toBeFocused();
  await explanation
    .getByRole("button", {
      name: "Quelldatei nachher ansehen: etc/sysctl.d/99-anker.conf",
    })
    .click();
  await expect(dialog.locator("pre")).toHaveText("net.ipv4.ip_forward=0");
  await dialog.getByRole("button", { name: "Dialog schließen" }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await explanation.screenshot({
    path: info.outputPath("setting-explanation-mobile.png"),
  });
  await journey.getByRole("slider").press("Home");
  await expect(explanation).toHaveCount(0);
});

test("setting trail uses the same explanation and masked evidence never exposes the body", async ({
  page,
  request,
}) => {
  const { reads } = await setup(page, request, true);
  await page.locator(".host-tabs").getByRole("button", { name: "Änderungen", exact: true }).click();
  const trace = page.locator(".setting-trace");
  await trace.locator(":scope > summary").click();
  const explanation = trace.getByRole("region", {
    name: "Bedeutung der Änderung",
  });
  await expect(explanation).toContainText("deaktiviert");
  await explanation.locator("summary").click();
  await explanation
    .getByRole("button", {
      name: "Quelldatei nachher ansehen: etc/sysctl.d/99-anker.conf",
    })
    .click();
  const dialog = page.getByRole("dialog", { name: "Gespeicherter Nachweis" });
  await expect(dialog).toContainText("Geschützter Inhalt");
  await expect(dialog).not.toContainText("must-not-leak");
  await expect(dialog.locator("pre")).toHaveCount(0);
  expect(reads).toEqual(["explain-new"]);
});

test("failed original reads retry the same source without losing its evidence context", async ({
  page,
  request,
}) => {
  const { journey } = await setup(page, request);
  let calls = 0;
  await page.route("**/api/backups/explain-new/file?*", (r) =>
    ++calls === 1
      ? r.fulfill({ status: 500, json: { error: "Quelldatei nicht lesbar" } })
      : r.fulfill({
          json: {
            entry: { secret: false },
            masked: false,
            content: "net.ipv4.ip_forward=0\n",
          },
        }),
  );
  await page
    .getByRole("button", { name: "Konfigurationsreise öffnen" })
    .click();
  const explanation = journey.getByRole("region", {
    name: "Bedeutung der Änderung",
  });
  await explanation.locator("summary").click();
  await explanation
    .getByRole("button", {
      name: "Quelldatei nachher ansehen: etc/sysctl.d/99-anker.conf",
    })
    .click();
  const dialog = page.getByRole("dialog", { name: "Gespeicherter Nachweis" });
  await expect(dialog).toContainText("Quelldatei nicht lesbar");
  await expect(dialog).toContainText("explain-new");
  await dialog.getByRole("button", { name: "Quelldatei erneut lesen" }).click();
  await expect(dialog.locator("pre")).toHaveText("net.ipv4.ip_forward=0");
  expect(calls).toBe(2);
});
