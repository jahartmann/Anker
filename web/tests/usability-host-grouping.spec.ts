import { test, expect } from "@playwright/test";
import { demoCookies } from "./demo-auth";
import { displayHostName } from "../src/hostNames";
import { groupHosts, type HostGrouping } from "../src/hostGrouping";
import type { Host } from "../src/api";

const host = (id: string, name: string, address: string, group = "") =>
  ({
    id,
    name,
    address,
    group,
  }) as Host;

test("names preserve acronyms, deliberate casing and original catalog values", () => {
  for (const [raw, label] of [
    ["pve-cp-01", "PVE · CP 01"],
    ["cpmusterort", "CP Musterort"],
    ["Cpmusterort", "CP Musterort"],
    ["gsbeispielstadt", "GS Beispielstadt"],
    ["gstestdorf-Backup", "GS Testdorf · Backup"],
    ["fzbeispiel-backup", "FZ Beispiel · Backup"],
    ["fzbeispielstadt", "FZ Beispielstadt"],
    ["zp01", "ZP01"],
    ["gymnasium", "Gymnasium"],
    ["rsync", "Rsync"],
    ["pbs-rs", "PBS · RS"],
    ["gs_fz_gym", "GS FZ Gym"],
    ["PVE-cp-RS", "PVE · CP RS"],
    ["pbs-cp01", "PBS · CP01"],
    ["pbs-cp.example.org", "PBS · CP.example.org"],
    ["Mein Server", "Mein Server"],
    ["constructor", "Constructor"],
    ["pve-berlin-01", "PVE · Berlin 01"],
  ])
    expect(displayHostName(raw)).toBe(label);
});

test("sites pair PVE/PBS names across subnets and use IP only as a labelled fallback", () => {
  const paired = [
    host("1", "pve-cp-01", "10.1.0.2"),
    host("2", "pbs-cp-02", "10.2.0.3"),
  ];
  expect(groupHosts(paired, "automatic")).toMatchObject([
    { label: "CP", inferred: true, hosts: paired },
  ]);
  const nearby = [
    host("3", "pve-rs", "10.3.0.2"),
    host("4", "archive", "10.3.0.9"),
  ];
  expect(groupHosts(nearby, "automatic")).toMatchObject([
    { label: "10.3.0.0/24", inferred: true },
  ]);
  expect(groupHosts(nearby, "site")).toHaveLength(2);
  expect(
    groupHosts(
      [host("5", "foo", "10.30.0.1"), host("6", "bar", "10.3.0.2")],
      "automatic",
    ),
  ).toHaveLength(2);
});

test("manual sites win, compatible new hosts join as suggestions, ambiguous sites stay separate", () => {
  const a = host("a", "pve-cp-01", "10.1.0.2", "CP");
  const b = host("b", "pbs-cp-02", "10.2.0.2");
  expect(groupHosts([a, b], "automatic")).toMatchObject([
    { label: "CP", inferred: true, hosts: [a, b] },
  ]);
  const c = host("c", "pve-cp-03", "10.1.0.3", "RS");
  const grouped = groupHosts([a, c, b], "automatic");
  expect(grouped).toHaveLength(3);
  expect(
    grouped.find((bucket) => bucket.hosts.some((h) => h.id === "a"))!.hosts,
  ).toEqual([a]);
  expect(
    grouped.find((bucket) => bucket.hosts.some((h) => h.id === "c"))!.hosts,
  ).toEqual([c]);
  const unknown = [
    host("x", "alpha", "vpn.example.org"),
    host("y", "beta", "vpn.example.net"),
  ];
  expect(groupHosts(unknown, "automatic")).toHaveLength(2);
});

test("host overview keeps roles, filtering and persisted grouping usable on desktop and mobile", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  const base = await (await page.request.get("/api/status")).json();
  const hosts = [
    {
      ...base.hosts[0],
      id: "pve-cp",
      name: "pve-cp-01",
      address: "10.1.0.2",
      group: "",
      inventory: { ...base.hosts[0].inventory, host_type: "pve" },
    },
    {
      ...base.hosts[0],
      id: "pbs-cp",
      name: "pbs-cp-02",
      address: "10.2.0.2",
      group: "",
      inventory: { ...base.hosts[0].inventory, host_type: "pbs" },
    },
    {
      ...base.hosts[0],
      id: "pve-rs",
      name: "pve-rs",
      address: "10.3.0.2",
      group: "RS",
      inventory: undefined,
    },
  ];
  await page.route("**/api/status", (route) =>
    route.fulfill({
      json: { ...base, hosts, jobs: [], backups: [], references: {} },
    }),
  );
  await page.goto("/");
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  const cp = page.getByRole("region", { name: "Hostbereich CP", exact: true });
  await expect(
    cp.getByRole("button", { name: "pve-cp-01", exact: true }),
  ).toHaveText("PVE · CP 01");
  await expect(
    cp.getByRole("button", { name: "pbs-cp-02", exact: true }),
  ).toHaveText("PBS · CP 02");
  await expect(
    cp.getByText("Standortvorschlag", { exact: true }),
  ).toBeVisible();
  const colors = await cp
    .locator(".host-site-services .server-role")
    .evaluateAll((els) =>
      els.map((el) => getComputedStyle(el).backgroundColor),
    );
  expect(new Set(colors).size).toBe(2);
  const shadow = await cp.evaluate((el) => getComputedStyle(el).boxShadow);
  expect(shadow).toContain("3px 3px");
  await page.getByLabel("Hosts durchsuchen").fill("pbs");
  await expect(
    cp.getByRole("button", { name: "pbs-cp-02", exact: true }),
  ).toBeVisible();
  await expect(
    cp.getByRole("button", { name: "pve-cp-01", exact: true }),
  ).toHaveCount(0);
  await page.getByLabel("Hosts durchsuchen").fill("");
  await page.screenshot({
    path: "/tmp/anker-host-sites-automatic.png",
    fullPage: true,
  });
  for (const mode of [
    "type",
    "network",
    "site",
    "none",
    "automatic",
  ] as HostGrouping[]) {
    await page
      .getByLabel("Ansicht gruppieren", { exact: true })
      .selectOption(mode);
    await expect(page.locator(".host-table tbody tr")).toHaveCount(3);
  }
  await page
    .getByLabel("Ansicht gruppieren", { exact: true })
    .selectOption("type");
  await page.reload();
  await page
    .getByRole("navigation", { name: "Hauptnavigation" })
    .getByRole("button", { name: "Hosts", exact: true })
    .click();
  await expect(
    page.getByLabel("Ansicht gruppieren", { exact: true }),
  ).toHaveValue("type");
  await expect(
    page.getByRole("region", { name: "Hostbereich PBS", exact: true }),
  ).toBeVisible();
  const headers = await page
    .locator(
      '.host-site[data-server-kind="pve"] .host-site-header, .host-site[data-server-kind="pbs"] .host-site-header',
    )
    .evaluateAll((els) =>
      els.map((el) => getComputedStyle(el).backgroundColor),
    );
  expect(new Set(headers).size).toBe(2);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 950 });
    await expect(
      page.getByRole("button", { name: "pbs-cp-02", exact: true }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `/tmp/anker-host-sites-${width}.png`,
      fullPage: true,
    });
  }
});

test("discovery groups related PVE/PBS names and keeps identity confirmation explicit", async ({
  page,
  request,
}) => {
  await page.context().addCookies(await demoCookies(request));
  const results = [
    {
      address: "10.51.0.2",
      name: "pve-cp-01.example.org",
      kind: "pve",
      version: "9",
      ssh_reachable: true,
    },
    {
      address: "10.52.0.3",
      name: "pbs-cp-02.example.org",
      kind: "pbs",
      version: "4",
      ssh_reachable: true,
    },
  ];
  await page.route("**/api/hosts/discovery", (route) =>
    route.fulfill({
      json: {
        config: {
          ranges: ["10.51.0.0/24", "10.52.0.0/24"],
          web_port: 8006,
          pbs_port: 8007,
          ssh_port: 22,
        },
        scan: {
          status: "completed",
          total: 508,
          checked: 508,
          results,
          config: { ranges: [], web_port: 8006, pbs_port: 8007, ssh_port: 22 },
        },
      },
    }),
  );
  await page.goto("/");
  await page
    .getByRole("button", { name: "Server suchen", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  const cp = dialog.getByRole("region", {
    name: "Suchbereich CP",
    exact: true,
  });
  await expect(cp).toBeVisible();
  await expect(
    cp.getByText("PVE · CP 01.example.org", { exact: true }),
  ).toBeVisible();
  await expect(
    cp.getByText("PBS · CP 02.example.org", { exact: true }),
  ).toBeVisible();
  await expect(cp.getByRole("checkbox")).toHaveCount(2);
  await expect(cp.locator(".discovery-results tbody tr")).toHaveCount(2);
  await expect(cp.getByRole("checkbox").first()).not.toBeChecked();
  await dialog.getByLabel("Suchergebnisse gruppieren").selectOption("type");
  await expect(
    dialog.getByRole("region", { name: "Suchbereich PBS", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 950 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await dialog
    .getByRole("region", { name: "Suchbereich PBS", exact: true })
    .scrollIntoViewIfNeeded();
  await page.screenshot({ path: "/tmp/anker-discovery-sites-390.png" });
  await dialog
    .getByRole("button", { name: "Dialog schließen", exact: true })
    .click();
  await expect(
    page.getByLabel("Ansicht gruppieren", { exact: true }),
  ).toHaveValue("type");
});
