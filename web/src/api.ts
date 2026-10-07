export interface User {
  disabled: boolean;
  id: string;
  name: string;
  role: string;
  secrets: boolean;
}
export interface Port {
  name: string;
  mac: string;
  pci?: string;
}
export interface Inventory {
  hostname: string;
  pve_version: string;
  debian: string;
  kernel: string;
  boot_mode: string;
  cluster_id: string;
  quorate: boolean;
  interfaces: Port[];
  disks: {
    name: string;
    id: string;
    size: number;
    uuid?: string;
    mount?: string;
  }[];
  details?: Record<string, unknown>;
}
export interface HostIdentity {
  address: string;
  port: number;
  fingerprint: string;
  key_type: string;
  known: boolean;
  changed: boolean;
}
export interface Host {
  restore_key_path?: string;
  restore_ssh_user?: string;
  id: string;
  name: string;
  address: string;
  group: string;
  cluster_id: string;
  ssh_user: string;
  ssh_port: number;
  key_path: string;
  known_hosts_path: string;
  enabled: boolean;
  schedule: string;
  extra_paths: string[];
  inventory?: Inventory;
  last_probe?: string;
  probe_error?: string;
}
export interface Backup {
  verified_at?: string;
  verification_error?: string;
  id: string;
  host_id: string;
  host_name: string;
  created_at: string;
  status: string;
  size: number;
  files: number;
  pinned: boolean;
  archived: boolean;
  warnings: string[];
}
export interface Entry {
  path: string;
  type: string;
  mode: number;
  uid: number;
  gid: number;
  size: number;
  secret: boolean;
  link?: string;
}
export interface Job {
  started_at?: string;
  trigger?: string;
  scheduled_day?: string;
  id: string;
  host_id: string;
  kind: string;
  state: string;
  created_at: string;
  finished_at?: string;
  result_id?: string;
  error?: string;
  attempts: number;
}
export interface Step {
  path: string;
  action: string;
  reason: string;
  diff?: string;
  secret?: boolean;
}
export interface Plan {
  id: string;
  backup_id: string;
  target_id: string;
  scenario: string;
  created_at: string;
  state: string;
  source: Inventory;
  target: Inventory;
  steps: Step[];
  blockers: string[];
  manual: string[];
  mapping?: {
    interfaces: Record<string, string>;
    storage?: Record<string, string>;
    hostname?: string;
    address?: string;
  };
  result?: {
    applied: string[];
    checks: string[];
    rollback_path: string;
    reboot_verified: boolean;
  };
}
export interface Settings {
  password_min_length: number;
  session_days: number;
  timezone: string;
  schedule: string;
  parallel: number;
  retries: number;
  daily: number;
  weekly: number;
  monthly: number;
  archive_days: number;
  stale_hours: number;
  webhook: string;
  smtp_server: string;
  smtp_user: string;
  smtp_password?: string;
  mail_from: string;
  mail_to: string;
}
export interface Status {
  scheduler_health?: { at: string; error: string };
  notification_health?: { at: string; error: string };
  maintenance_health?: { at: string; error: string };
  hosts: Host[];
  backups: Backup[];
  jobs: Job[];
  plans: Plan[];
  demo: boolean;
  timezone: string;
  stale_hours: number;
}
export const emptyStatus: Status = {
  hosts: [],
  backups: [],
  jobs: [],
  plans: [],
  demo: false,
  timezone: "Europe/Berlin",
  stale_hours: 26,
};
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch("/api/" + path, {
      method,
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-Anker-Request": "1" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new Error(
      "Anker ist nicht erreichbar. Verbindung und Dienst prüfen.",
    );
  }
  if (response.status === 401 && path !== "login")
    window.dispatchEvent(new Event("anker:session-expired"));
  let value;
  try {
    value = await response.json();
  } catch {
    throw new Error("Ungültige Serverantwort. Verbindung und Dienst prüfen.");
  }
  if (!response.ok) throw new Error(value.error || "Anfrage fehlgeschlagen");
  return value as T;
}
let displayTimezone = "Europe/Berlin";
export function configureTimezone(v: string) {
  try {
    new Intl.DateTimeFormat("de-DE", { timeZone: v });
    displayTimezone = v;
  } catch {
    displayTimezone = "Europe/Berlin";
  }
}
export const date = (v?: string) =>
  v
    ? new Intl.DateTimeFormat("de-DE", {
        dateStyle: "short",
        timeStyle: "short",
        timeZone: displayTimezone,
      }).format(new Date(v))
    : "—";
export function bytes(v: number) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return (i ? v.toFixed(1) : v) + " " + units[i];
}
export const labels: Record<string, string> = {
  damaged: "Beschädigt",
  successful: "Gesichert",
  partial: "Unvollständig",
  failed: "Fehlgeschlagen",
  queued: "Geplant",
  running: "Läuft",
  cancelled: "Abgebrochen",
  interrupted: "Unterbrochen",
  ready: "Bereit zur Prüfung",
  blocked: "Entscheidung erforderlich",
  manual: "Manuell geführt",
  applying: "Wird angewendet",
  checks_pending: "Prüfung ausstehend",
  demo_applied: "Demo angewendet",
  files: "Einzelne Dateien",
  standalone: "Host wiederaufbauen",
  migration: "Neue Hardware",
  version: "Neue Proxmox-Version",
  "cluster-node": "Cluster-Node ersetzen",
  "cluster-disaster": "Cluster wiederaufbauen",
  topology: "Topologie ändern",
  backup: "Sicherung",
  probe: "Hostprüfung",
  restore: "Wiederherstellung",
};
export function hostState(h: Host, s: Status) {
  const hostBackups = s.backups
    .filter((b) => b.host_id === h.id)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
  const b = hostBackups.find(
    (b) => b.host_id === h.id && b.status === "successful",
  );
  if (!h.enabled) return { text: "Pausiert", tone: "muted", backup: b };
  if (hostBackups[0]?.status === "damaged")
    return {
      text: "Beschädigter Stand",
      tone: "danger",
      backup: hostBackups[0],
    };
  if (hostBackups[0]?.status === "partial")
    return {
      text: "Letzte Sicherung unvollständig",
      tone: "warning",
      backup: hostBackups[0],
    };
  const latestJob = s.jobs
    .filter(
      (j) =>
        j.host_id === h.id &&
        j.kind === "backup" &&
        ["failed", "interrupted", "cancelled"].includes(j.state),
    )
    .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
  if (
    latestJob &&
    (!b || latestJob.created_at > b.created_at) &&
    latestJob.result_id !== b?.id
  )
    return {
      text: "Letzter Versuch fehlgeschlagen",
      tone: latestJob.state === "failed" ? "danger" : "warning",
      backup: b,
    };
  if (!b) return { text: "Ohne Sicherung", tone: "muted", backup: undefined };
  if (Date.now() - new Date(b.created_at).getTime() > s.stale_hours * 3600000)
    return { text: "Überfällig", tone: "warning", backup: b };
  return { text: "Gesichert", tone: "success", backup: b };
}
