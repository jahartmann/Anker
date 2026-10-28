import type { Host } from "./api";
import { displayHostName } from "./hostNames";

export type HostGrouping = "automatic" | "site" | "type" | "network" | "none";
export const groupingOptions: { value: HostGrouping; label: string }[] = [
  { value: "automatic", label: "Automatisch gruppieren" },
  { value: "site", label: "Nach Standort" },
  { value: "type", label: "Nach Servertyp" },
  { value: "network", label: "Nach IP-Netz (/24)" },
  { value: "none", label: "Ohne Gruppierung" },
];
export function readHostGrouping(): HostGrouping {
  try {
    const value = localStorage.getItem("anker.hostGrouping");
    return (
      groupingOptions.find((option) => option.value === value)?.value ||
      "automatic"
    );
  } catch {
    return "automatic";
  }
}
type GroupingHost = Pick<Host, "id" | "name" | "address" | "group"> & {
  inventory?: Pick<
    NonNullable<Host["inventory"]>,
    "host_type" | "pbs_version" | "pve_version"
  >;
};
export function serverType(host: GroupingHost): "pve" | "pbs" | "unknown" {
  if (host.inventory?.host_type) return host.inventory.host_type;
  // A version is evidence; a name is only a hint and must not classify a server.
  if (host.inventory?.pbs_version) return "pbs";
  if (host.inventory?.pve_version) return "pve";
  return "unknown";
}
export function serverTypeLabel(host: GroupingHost) {
  return { pve: "PVE", pbs: "PBS", unknown: "Typ offen" }[serverType(host)];
}
export function serverPalette(hosts: GroupingHost[]) {
  const types = new Set(hosts.map(serverType));
  return types.size === 1 ? [...types][0] : "site";
}
function network(host: GroupingHost) {
  const parts = host.address.split(".");
  if (
    parts.length !== 4 ||
    parts.some((part) => !/^\d{1,3}$/.test(part) || Number(part) > 255)
  )
    return "";
  return parts.slice(0, 3).map(Number).join(".") + ".0/24";
}
function nameSite(host: GroupingHost) {
  const name = host.name.toLocaleLowerCase("de").split(".")[0];
  const words = name
    .split(/[-_\s]+/)
    .filter(
      (word) =>
        word &&
        !/^(pve|pbs|proxmox|backup|backupserver|server|host|node|nas|vm|lxc)\d*$/.test(
          word,
        ) &&
        !/^\d+$/.test(word),
    );
  return words.join("-");
}
export interface HostBucket<T extends GroupingHost = Host> {
  key: string;
  label: string;
  reason: string;
  inferred: boolean;
  hosts: T[];
}
// Calculate against the whole inventory so search and paging cannot change a site.
export function groupHosts<T extends GroupingHost>(
  hosts: T[],
  mode: HostGrouping,
): HostBucket<T>[] {
  const names = new Map<string, number>();
  const nets = new Map<string, number>();
  for (const host of hosts) {
    const name = nameSite(host),
      net = network(host);
    if (name) names.set(name, (names.get(name) || 0) + 1);
    if (net) nets.set(net, (nets.get(net) || 0) + 1);
  }
  const anchors = (field: (host: GroupingHost) => string) => {
    const result = new Map<string, Map<string, string>>();
    for (const host of hosts) {
      const value = field(host),
        group = host.group.trim();
      if (!value || !group) continue;
      if (!result.has(value)) result.set(value, new Map());
      result.get(value)!.set(group.toLocaleLowerCase("de"), group);
    }
    return result;
  };
  const namedGroups = anchors(nameSite),
    networkGroups = anchors(network);
  const uniqueGroup = (groups: Map<string, string> | undefined) =>
    groups?.size === 1 ? [...groups.values()][0] : "";
  const buckets = new Map<string, HostBucket<T>>();
  for (const host of hosts) {
    let key = "all",
      label = "Alle Hosts",
      reason = "Gemeinsame Hostliste",
      inferred = false;
    const name = nameSite(host),
      net = network(host);
    if (mode === "type") {
      key = serverType(host);
      label = serverTypeLabel(host);
      reason =
        key === "unknown"
          ? "Noch kein Servertyp durch Inventar bestätigt"
          : "Servertyp aus dem Inventar";
    } else if (mode === "network") {
      key = net ? `net:${net}` : "no-network";
      label = net || "Ohne IPv4-Netz";
      reason = "Adressbereich /24 · kein Nachweis eines gemeinsamen Standorts";
    } else if (mode !== "none") {
      if (host.group.trim()) {
        key = "group:" + host.group.trim().toLocaleLowerCase("de");
        label = displayHostName(host.group.trim());
        reason = "Standort aus der konfigurierten Hostgruppe";
      } else if (
        uniqueGroup(namedGroups.get(name)) ||
        (mode === "automatic" && uniqueGroup(networkGroups.get(net)))
      ) {
        const matchingGroup =
          uniqueGroup(namedGroups.get(name)) ||
          uniqueGroup(networkGroups.get(net));
        key = "group:" + matchingGroup.toLocaleLowerCase("de");
        label = displayHostName(matchingGroup);
        reason = "Hostgruppe mit Standortvorschlägen aus Namen oder /24-Netz";
        inferred = true;
      } else if (name && (names.get(name) || 0) > 1) {
        key = "name:" + name;
        label = displayHostName(name);
        reason = "Standortvorschlag aus gemeinsamen Namensteilen";
        inferred = true;
      } else if (mode === "automatic" && net && (nets.get(net) || 0) > 1) {
        key = "net:" + net;
        label = net;
        reason = "Standortvorschlag aus gemeinsamem /24-Netz";
        inferred = true;
      } else {
        key = "host:" + host.id;
        label = displayHostName(name || host.name);
        reason = "Standort offen · im Host über die Gruppe festlegen";
      }
    }
    if (!buckets.has(key))
      buckets.set(key, { key, label, reason, inferred, hosts: [] });
    const bucket = buckets.get(key)!;
    if (inferred) {
      bucket.inferred = true;
      bucket.reason = reason;
    }
    bucket.hosts.push(host);
  }
  return [...buckets.values()].sort((a, b) =>
    a.label.localeCompare(b.label, "de", { numeric: true }),
  );
}
