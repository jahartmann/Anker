import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";
import { demoCookies } from "./demo-auth";
let cookies: Awaited<ReturnType<APIRequestContext["storageState"]>>["cookies"];
test.beforeAll(async ({ request }) => {
  cookies = await demoCookies(request);
});
const config = { ranges: ["10.20.0.0/24"], web_port: 8006, ssh_port: 2222 };
const results = [
  {
    address: "10.20.0.11",
    name: "pve-one.example",
    version: "9.0",
    ssh_reachable: true,
  },
  {
    address: "10.20.0.12",
    name: "pve-two.example",
    version: "8.4",
    ssh_reachable: true,
  },
  {
    address: "10.20.0.13",
    name: "pve-three.example",
    version: "9.0",
    ssh_reachable: true,
  },
  { address: "10.20.0.14", name: "", version: "9.0", ssh_reachable: false },
];
async function setup(page: Page, completed = false, paused = false) {
  await page.context().addCookies(cookies);
  const saved = { ...config };
  let status = paused ? "paused" : completed ? "completed" : "idle";
  const scannedConfig = { ...config, pbs_port: 9007 };
  const writes: any[] = [];
  const checks: any[] = [];
  const enrolls: any[] = [];
  let inspections = 0;
  await page.route("**/api/hosts/discovery", async (route) => {
    const method = route.request().method();
    if (method === "DELETE") {
      status = "cancelled";
      return route.fulfill({ json: {} });
    }
    if (method !== "GET") {
      Object.assign(saved, route.request().postDataJSON());
      writes.push({ method, ...saved });
      if (method === "POST") status = "completed";
      if (method === "PATCH") status = "completed";
    }
    await route.fulfill({
      json: {
        config: saved,
        suggested_ranges: ["10.99.0.0/24"],
        scan: {
          status,
          total: 254,
          checked: status === "idle" ? 0 : status === "paused" ? 100 : 254,
          tcp_checked: status === "paused" ? 200 : 254,
          phase: status === "paused" ? "paused" : "completed",
          resumable: status === "paused",
          config: paused ? scannedConfig : saved,
          results:
            status === "idle"
              ? []
              : paused
                ? [...results, { ...results[0], kind: "pbs", web_port: 9007 }]
                : results,
        },
      },
    });
  });
  await page.route("**/api/hosts/connection/inspect", (route) => {
    inspections++;
    return route.fulfill({
      json: {
        ...route.request().postDataJSON(),
        fingerprint: "SHA256:test-fingerprint",
        key_type: "ssh-ed25519",
        known: false,
        changed: false,
      },
    });
  });
  await page.route("**/api/hosts/connection/check", (route) => {
    const data = route.request().postDataJSON();
    checks.push(data);
    if (data.address.endsWith("13"))
      return route.fulfill({
        status: 400,
        json: { error: "Verbindung fehlgeschlagen" },
      });
    return route.fulfill({
      json: {
        authenticated: data.address.endsWith("11")
          ? data.password === "first-secret"
          : data.password === "second-secret",
      },
    });
  });
  await page.route("**/api/hosts/connection/enroll", (route) => {
    enrolls.push(route.request().postDataJSON());
    return route.fulfill({ json: { id: "added" } });
  });
  await page.goto("/");
  await page
    .getByRole("button", { name: "Server suchen", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Netzbereiche für LAN und VPN")).toHaveValue(
    "10.20.0.0/24",
  );
  return { dialog, writes, checks, enrolls, inspected: () => inspections };
}

test("35 VPN networks accept masks, persist and find hosts without a password", async ({
  page,
}) => {
  const f = await setup(page);
  const ranges = Array.from({ length: 35 }, (_, i) =>
    `10.${i}.0.0 ${i % 2 ? "255.255.255.0" : "/24"}`.replace(" /", "/"),
  );
  await f.dialog
    .getByLabel("Netzbereiche für LAN und VPN")
    .fill(ranges.join("\n"));
  await f.dialog
    .getByRole("button", { name: "Netzbereiche speichern", exact: true })
    .click();
  await expect(
    f.dialog.getByText("Netzbereiche gespeichert.", { exact: true }),
  ).toBeVisible();
  expect(f.writes[0].ranges).toEqual(ranges);
  await f.dialog
    .getByRole("button", { name: "Server suchen", exact: true })
    .click();
  await expect(f.dialog.getByText(/4 Server gefunden/)).toBeVisible();
  expect(f.writes[1]).toMatchObject({ method: "POST", ranges, ssh_port: 2222 });
  expect(f.checks).toEqual([]);
  expect(f.enrolls).toEqual([]);
  expect(f.inspected()).toBe(0);
  await expect(f.dialog.getByLabel("10.20.0.14 auswählen")).toBeDisabled();
  await f.dialog
    .getByRole("button", { name: "Einzeln einrichten", exact: true })
    .first()
    .click();
  await expect(
    page.getByRole("dialog").getByLabel("Adresse", { exact: true }),
  ).toHaveValue("10.20.0.11");
  await page
    .getByRole("button", { name: "Weitere Angaben", exact: true })
    .click();
  await expect(page.getByLabel("SSH-Port", { exact: true })).toHaveValue(
    "2222",
  );
});

test("resume uses saved scope and shows PBS alongside PVE without duplicate enrollment rows", async ({
  page,
}) => {
  const f = await setup(page, true, true);
  await expect(f.dialog.getByText(/Suche pausiert/)).toBeVisible();
  await expect(f.dialog.getByText(/4 Server gefunden/)).toBeVisible();
  await expect(f.dialog.getByLabel("10.20.0.11 auswählen")).toHaveCount(1);
  await expect(f.dialog.getByText(/PBS.*9007/).first()).toBeVisible();
  await f.dialog
    .getByLabel("Netzbereiche für LAN und VPN")
    .fill("10.99.0.0/24");
  await f.dialog
    .getByRole("button", { name: "Suche fortsetzen", exact: true })
    .click();
  await expect(f.dialog.getByText(/Suche abgeschlossen/)).toBeVisible();
  await expect(f.dialog.getByLabel("Netzbereiche für LAN und VPN")).toHaveValue(
    "10.20.0.0/24",
  );
  expect(f.writes).toHaveLength(1);
  expect(f.writes[0].method).toBe("PATCH");
  expect(f.checks).toEqual([]);
  expect(f.enrolls).toEqual([]);
});

test("optional password rounds require identity approval and skip successful hosts and network errors", async ({
  page,
}) => {
  const f = await setup(page, true);
  await f.dialog
    .getByLabel("Alle neuen Hosts mit erreichbarem SSH auswählen")
    .check();
  await f.dialog
    .getByText(
      "Optional: gemeinsame Zugangsdaten prüfen und Hosts einrichten",
      { exact: true },
    )
    .click();
  await f.dialog.getByLabel("Passwort 1", { exact: true }).fill("first-secret");
  await f.dialog
    .getByLabel("Passwort 2 (optional)", { exact: true })
    .fill("second-secret");
  await expect(
    f.dialog.getByRole("button", { name: "Zugänge prüfen", exact: true }),
  ).toBeDisabled();
  await f.dialog
    .getByRole("button", { name: "SSH-Identitäten lesen", exact: true })
    .click();
  await expect(
    f.dialog.getByLabel("Fingerprint für 10.20.0.13 bestätigt"),
  ).toBeVisible();
  expect(f.inspected()).toBe(3);
  expect(f.checks).toEqual([]);
  for (const address of ["10.20.0.11", "10.20.0.12", "10.20.0.13"])
    await f.dialog.getByLabel(`Fingerprint für ${address} bestätigt`).check();
  await f.dialog
    .getByRole("button", { name: "Zugänge prüfen", exact: true })
    .click();
  await expect(
    f.dialog.getByText("Anmeldung mit Passwort 2 erfolgreich"),
  ).toBeVisible();
  expect(f.checks.map((c) => [c.address, c.password])).toEqual([
    ["10.20.0.11", "first-secret"],
    ["10.20.0.12", "first-secret"],
    ["10.20.0.13", "first-secret"],
    ["10.20.0.12", "second-secret"],
  ]);
  expect(f.enrolls).toEqual([]);
  await f.dialog
    .getByRole("button", { name: "Geprüfte Hosts einrichten", exact: true })
    .click();
  await expect(f.dialog.getByText("Eingerichtet", { exact: true })).toHaveCount(
    2,
  );
  expect(f.enrolls.map((e) => [e.host.address, e.password])).toEqual([
    ["10.20.0.11", "first-secret"],
    ["10.20.0.12", "second-secret"],
  ]);
  expect(
    await page.evaluate(() => JSON.stringify([localStorage, sessionStorage])),
  ).not.toContain("secret");
  await f.dialog
    .getByRole("button", { name: "Schließen", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Server suchen", exact: true })
    .click();
  await page
    .getByText(
      "Optional: gemeinsame Zugangsdaten prüfen und Hosts einrichten",
      { exact: true },
    )
    .click();
  await expect(page.getByLabel("Passwort 1", { exact: true })).toHaveValue("");
});

test("discovery dialog remains usable on narrow screens and with keyboard", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const f = await setup(page, true);
  await expect(
    f.dialog.getByRole("heading", { name: "Proxmox-Server suchen" }),
  ).toBeVisible();
  await expect(
    f.dialog.getByLabel("Netzbereiche für LAN und VPN"),
  ).toBeFocused();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "/tmp/anker-discovery-mobile.png",
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
