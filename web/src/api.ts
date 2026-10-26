import { assessHost } from "./hostHealth";
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
  physical?: boolean;
  kind?: string;
}
export interface Inventory {
  query_states?: Record<
    string,
    { state: "ok" | "failed" | "not_applicable" | "unknown" }
  >;
  captured_at?: string;
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
  evidence_revision?: string;
  manifest_sha?: string;
  verification_attempt?: { at: string; state: string };
  copy_verification?: {
    version: number;
    backup_id: string;
    manifest_sha: string;
    state: string;
    attempt_at: string;
    verified_at?: string;
    provenance: string;
    failure_domain: "unknown";
  };
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
export interface BackupReference {
  host_id: string;
  backup_id?: string;
  manifest_sha?: string;
  name?: string;
  reason?: string;
  updated_at?: string;
  updated_by?: string;
  state: "missing" | "valid" | "unresolved";
  drill?: {
    tested_at: string;
    outcome: "passed" | "failed";
    evidence: string;
    actor: string;
    recorded_at: string;
    backup_id: string;
    manifest_sha: string;
  };
}
export interface ReferenceChangeSummary {
  reference: BackupReference;
  to_id: string;
  changed: number;
  added: number;
  removed: number;
  content: number;
  metadata: number;
  mtime_only: number;
}
export interface BackupReadiness {
  version: number;
  backup_id: string;
  items: {
    id: string;
    state: "ok" | "missing" | "unknown" | "failed" | "stale";
    title: string;
    detail: string;
    at?: string;
    next_action?: string;
    provenance?: string;
  }[];
  coverage: {
    path: string;
    state: "captured" | "missing";
    reason: string;
    authority: "collector" | "required";
  }[];
  dependencies: {
    path: string;
    source?: string;
    state: "captured" | "missing" | "unknown";
    reason?: string;
  }[];
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
export interface FileDiff {
  path: string;
  change: "added" | "removed" | "changed";
  secret: boolean;
  content_changed: boolean;
  metadata_changed: boolean;
  mtime_changed: boolean;
  mtime_only: boolean;
}
export interface MetadataChange {
  field: string;
  before: unknown;
  after: unknown;
}
export interface DiffLine {
  kind: "context" | "add" | "remove";
  text: string;
  before_line?: number;
  after_line?: number;
}
export interface DiffHunk {
  before_start: number;
  after_start: number;
  lines: DiffLine[];
}
export interface FileComparison {
  path: string;
  before_label: string;
  after_label: string;
  state:
    | "identical"
    | "different"
    | "missing_before"
    | "missing_after"
    | "unavailable"
    | "binary"
    | "too_large"
    | "masked";
  content_changed: boolean;
  metadata_changes: MetadataChange[];
  hunks: DiffHunk[];
  error?: string;
  masked: boolean;
}
export interface Job {
  revision?: number;
  max_attempts?: number;
  retry_at?: string;
  message?: string;
  last_error?: string;
  retry_of?: string;
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
export interface RecoveryEffect {
  path: string;
  field: string;
  before: string;
  after: string;
  reason: string;
}
export interface RecoveryTask {
  id: string;
  kind: string;
  title: string;
  instruction: string;
  requires: string[];
  state: string;
  evidence?: string;
  confirmed_by?: string;
  confirmed_at?: string;
}
export interface Step {
  decision?: FileDecision;
  destination?: string;
  effects?: RecoveryEffect[];
  comparison_state?: string;
  path: string;
  action: string;
  reason: string;
  diff?: string;
  secret?: boolean;
}
export interface RecoveryVerificationAttempt {
  id: string;
  plan_id: string;
  backup_id: string;
  target_id: string;
  started_at: string;
  finished_at: string;
  actor: string;
  origin: string;
  state: string;
  reason: string;
  identity_state: string;
  reboot_state: string;
  services_state: string;
  volumes_state: string;
  source_manifest_sha: string;
  snapshot_sha: string;
  connection_sha: string;
  baseline_identity: string;
  baseline_boot_id: string;
  baseline_captured_at: string;
  result?: {
    protocol: number;
    observed_at: string;
    identity: { state: string; value: string };
    boot: { state: string; value: string };
    services: Record<string, { state: string; value: string }>;
    storage_volumes: Record<string, { state: string; volumes?: string[] }>;
  };
}
export interface Plan {
  outcome_checks?: import("./components/recovery/RecoveryOutcomeCards").RecoveryOutcome[];
  remove_files?: string[];
  removals_confirmed?: boolean;
  verification_attempts?: RecoveryVerificationAttempt[];
  schema_version?: number;
  target_snapshot_id?: string;
  decisions?: Record<string, FileDecision>;
  preserved?: { path: string; reason: string }[];
  tasks?: RecoveryTask[];
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
    network?: Record<
      string,
      { address_cidr?: string; gateway?: string; dns_servers?: string[] }
    >;
    hostname?: string;
    address?: string;
  };
  result?: {
    state?: string;
    operation_id?: string;
    error?: string;
    applied: string[];
    checks: string[];
    rollback_path: string;
    reboot_verified: boolean;
  };
}
export interface RecoveryStorage {
  paths?: string[];
  backing?: Record<string, string>;
  content?: string[];
  nodes?: string[];
  verified?: boolean;
  id: string;
  kind: string;
  path: string;
  reason?: string;
  suggested?: string;
}
export type FileDecision = "backup" | "prepared" | "keep" | "manual" | "remove";
export interface RecoveryNetworkMapping {
  address_cidr?: string;
  gateway?: string;
  dns_servers?: string[];
}
export interface RecoveryPreview {
  snapshot_id: string;
  manifest_sha: string;
  comparison: FileComparison;
  original_action: { action: string; reason: string };
  prepared_action: { action: string; reason: string };
}
export interface RecoveryInspection {
  manifest_sha?: string;
  network_stanzas?: {
    key: string;
    path: string;
    name: string;
    family: string;
  }[];
  snapshot_id?: string;
  comparison_complete?: boolean;
  preserved?: { path: string; reason: string }[];
  files?: {
    path: string;
    state: string;
    sha256?: string;
    metadata?: Entry;
    error?: string;
  }[];
  source: Inventory;
  target: Inventory;
  ports: (Port & { reason: string; suggested?: string; paths?: string[] })[];
  target_ports: Port[];
  storage: RecoveryStorage[];
  target_storage: RecoveryStorage[];
  blockers: string[];
  warnings: string[];
  manual: string[];
  requires_console: boolean;
  requires_source_offline: boolean;
  automatic: boolean;
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
  references?: Record<string, BackupReference>;
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
export class ApiError extends Error {
  status?: number;
  constructor(message: string, status?: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch("/api/" + path, {
      method,
      signal,
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-Anker-Request": "1" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (e) {
    if (signal?.aborted) throw e;
    throw new ApiError(
      "Anker ist nicht erreichbar. Verbindung und Dienst prüfen.",
    );
  }
  if (response.status === 401 && path !== "login")
    window.dispatchEvent(new Event("anker:session-expired"));
  let value;
  try {
    value = await response.json();
  } catch {
    throw new ApiError(
      "Ungültige Serverantwort. Verbindung und Dienst prüfen.",
      response.status,
    );
  }
  if (!response.ok)
    throw new ApiError(
      value?.error || "Anfrage fehlgeschlagen",
      response.status,
    );
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
const calendarFormats = new Map<
  string,
  {
    parts: Intl.DateTimeFormat;
    month: Intl.DateTimeFormat;
    day: Intl.DateTimeFormat;
  }
>();
export function calendarDate(value: string) {
  if (!Number.isFinite(Date.parse(value)))
    return {
      monthKey: "unknown",
      dayKey: "unknown",
      monthLabel: "Zeitstempel ungeklärt",
      dayLabel: "Datum nicht lesbar",
    };
  let formats = calendarFormats.get(displayTimezone);
  if (!formats) {
    formats = {
      parts: new Intl.DateTimeFormat("en-CA", {
        timeZone: displayTimezone,
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      }),
      month: new Intl.DateTimeFormat("de-DE", {
        timeZone: displayTimezone,
        month: "long",
        year: "numeric",
      }),
      day: new Intl.DateTimeFormat("de-DE", {
        timeZone: displayTimezone,
        weekday: "short",
        day: "numeric",
        month: "long",
      }),
    };
    calendarFormats.set(displayTimezone, formats);
  }
  const timestamp = new Date(value);
  const parts = formats.parts.formatToParts(timestamp);
  const part = (type: string) => parts.find((p) => p.type === type)!.value;
  return {
    monthKey: `${part("year")}-${part("month")}`,
    dayKey: `${part("year")}-${part("month")}-${part("day")}`,
    monthLabel: formats.month.format(timestamp),
    dayLabel: formats.day.format(timestamp),
  };
}
export const date = (v?: string) =>
  v
    ? !Number.isFinite(Date.parse(v))
      ? "Ungültiger Zeitstempel"
      : new Intl.DateTimeFormat("de-DE", {
          dateStyle: "short",
          timeStyle: "short",
          timeZone: displayTimezone,
        }).format(new Date(v))
    : "—";
export const preciseDate = (v?: string) =>
  v
    ? !Number.isFinite(Date.parse(v))
      ? "Ungültiger Zeitstempel"
      : new Intl.DateTimeFormat("de-DE", {
          dateStyle: "short",
          timeStyle: "medium",
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
  interrupted: "Unterbrochen · Hostzustand prüfen",
  applied: "Dateien übernommen",
  rolled_back: "Dateien zurückgesetzt",
  rolling_back: "Wird zurückgesetzt",
  rollback_conflict: "Rücksetzungskonflikt",
  unknown: "Hostzustand ungeklärt",
  not_found: "Hostjournal nicht gefunden",
  writing: "Host schreibt Dateien",
  ready: "Bereit zur Prüfung",
  blocked: "Entscheidung erforderlich",
  manual: "Manuell geführt",
  applying: "Wird angewendet",
  checks_pending: "Prüfung ausstehend",
  demo_applied: "Demo angewendet",
  files: "Einzelne Dateien",
  host: "Host wiederaufbauen",
  standalone: "Host wiederaufbauen",
  migration: "Neue Hardware",
  version: "Neue Proxmox-Version",
  "cluster-node": "Cluster-Node ersetzen",
  "cluster-disaster": "Cluster wiederaufbauen",
  topology: "Topologie ändern",
  backup: "Sicherung",
  probe: "Hostprüfung",
  restore: "Wiederherstellung",
  rollback: "Rücksetzung",
};
export const hostState = assessHost;

export interface RecoveryRecipe {
  id: string;
  version: number;
  origin_plan_id: string;
  label: string;
  reviewed_by: string;
  reviewed_at: string;
}
export interface RecoveryRecipePreview {
  recipe: RecoveryRecipe;
  snapshot_id: string;
  applicable: boolean;
  suggestions: NonNullable<Plan["mapping"]>;
  mismatches: string[];
}

export interface ClusterBackupSet {
  version: number;
  id: string;
  cluster_id: string;
  purpose: "manualSourcePlanning";
  atomic: false;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  start_skew_ms: number;
  state: string;
  notes: string[];
  members: {
    host_id: string;
    host_name: string;
    job_id?: string;
    backup_id?: string;
    manifest_sha?: string;
    state: string;
    started_at?: string;
    completed_at?: string;
    captured_members?: string[];
    membership_known: boolean;
    shared_known: boolean;
  }[];
}
