import type { FileDiff } from "./api";

export function changeSubject(row: FileDiff): string {
  if (row.secret) return "Geschützte Konfiguration";
  const path = row.path.replace(/^\/+/, "");
  if (/^etc\/network\//.test(path)) return "Netzwerkdefinition";
  if (/^etc\/pve\/sdn\//.test(path)) return "Virtuelles Netzwerk (SDN)";
  if (path === "etc/pve/storage.cfg") return "Storage-Anbindungen";
  if (path === "etc/fstab") return "Dateisystem-Einhängepunkte";
  if (path === "etc/crypttab") return "Verschlüsselte Laufwerke";
  if (/^etc\/(sysctl\.|sysctl$)/.test(path))
    return "Kernel- und Netzwerkparameter";
  if (path === "etc/hostname") return "Node-Name";
  if (path === "etc/hosts") return "Lokale Namensauflösung";
  if (path === "etc/resolv.conf") return "DNS-Konfiguration";
  if (/^etc\/ssh\//.test(path)) return "SSH-Konfiguration";
  if (/^etc\/systemd\//.test(path)) return "Dienstkonfiguration";
  if (path === "etc/pve/datacenter.cfg") return "Proxmox-Grundeinstellungen";
  const guest = path.match(
    /^etc\/pve\/(?:nodes\/[^/]+\/)?(qemu-server|lxc)\/(\d+)\.conf$/,
  );
  if (guest)
    return `${guest[1] === "lxc" ? "Container" : "VM"} ${guest[2]}: Konfiguration`;
  return `Datei ${path.split("/").at(-1) || path}`;
}
export interface ChangeHighlight {
  text: string;
  count: number;
  kind: "added" | "removed" | "changed" | "metadata";
}
export function changeHighlights(rows: FileDiff[]): ChangeHighlight[] {
  const grouped = new Map<string, ChangeHighlight>();
  for (const row of rows) {
    if (row.mtime_only) continue;
    const metadataOnly =
      row.change === "changed" && !row.content_changed && row.metadata_changed;
    const kind = metadataOnly ? "metadata" : row.change;
    const verb =
      kind === "added"
        ? "hinzugefügt"
        : kind === "removed"
          ? "entfernt"
          : kind === "metadata"
            ? "Metadaten geändert"
            : "geändert";
    const text = `${changeSubject(row)}${metadataOnly ? ":" : ""} ${verb}`;
    const current = grouped.get(text);
    if (current) current.count++;
    else grouped.set(text, { text, count: 1, kind });
  }
  return Array.from(grouped.values());
}
