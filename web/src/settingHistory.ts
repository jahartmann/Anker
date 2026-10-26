import type { FileComparison } from "./api";
export const settingChoices = [
  {
    id: "ipv4-forwarding",
    name: "IPv4-Weiterleitung",
    key: "net.ipv4.ip_forward",
  },
  {
    id: "ipv6-forwarding",
    name: "IPv6-Weiterleitung",
    key: "net.ipv6.conf.all.forwarding",
  },
  { id: "swappiness", name: "Swap-Neigung", key: "vm.swappiness" },
  { id: "hostname", name: "Node-Name", key: "hostname" },
  { id: "dns-servers", name: "DNS-Server", key: "nameserver" },
];
export interface SettingObservation {
  backup_id: string;
  created_at: string;
  state: string;
  value?: string;
  sources: string[];
  order_uncertain?: boolean;
}
export interface SettingHistoryResult {
  setting: { id: string; name: string; key: string; kind: string };
  observations: SettingObservation[];
  next?: string;
  undated: number;
}
export interface SettingRun {
  observations: SettingObservation[];
  state: string;
  value?: string;
}
export function settingRuns(observations: SettingObservation[]): SettingRun[] {
  const runs: SettingRun[] = [];
  for (const o of observations) {
    const last = runs.at(-1);
    if (last && last.state === o.state && last.value === o.value)
      last.observations.push(o);
    else runs.push({ state: o.state, value: o.value, observations: [o] });
  }
  return runs;
}
export function settingValue(id: string, value?: string) {
  if (value === undefined) return "Nicht bekannt";
  if (id === "ipv4-forwarding" || id === "ipv6-forwarding")
    return value === "1" ? "Aktiviert" : value === "0" ? "Deaktiviert" : value;
  return value;
}
export const observationLabels: Record<string, string> = {
  not_recorded: "Keine Deklaration erfasst",
  protected: "Geschützter Inhalt",
  unreliable: "Stand nicht belastbar",
  unavailable: "Stand nicht lesbar",
  ambiguous: "Widersprüchliche Deklarationen",
  unsupported: "Wert nicht eindeutig lesbar",
  limited: "Lesebudget erreicht",
};
export function traceableSettings(c: FileComparison) {
  if (c.masked || c.state === "masked" || c.state === "unavailable") return [];
  if (c.path === "etc/hostname")
    return settingChoices.filter((s) => s.id === "hostname");
  if (c.path === "etc/resolv.conf")
    return settingChoices.filter((s) => s.id === "dns-servers");
  if (!/^etc\/(sysctl\.conf|sysctl\.d\/[^/]+\.conf)$/.test(c.path)) return [];
  const keys = c.hunks
    .flatMap((h) => h.lines)
    .map((line) =>
      line.text
        .trim()
        .replace(/^-/, "")
        .split("=")[0]
        .trim()
        .replaceAll("/", "."),
    );
  return settingChoices.filter(
    (s) =>
      s.id !== "hostname" && s.id !== "dns-servers" && keys.includes(s.key),
  );
}

export interface SettingSnapshotResult {
  backup_id: string;
  settings: {
    setting: SettingHistoryResult["setting"];
    observation: SettingObservation;
  }[];
}
