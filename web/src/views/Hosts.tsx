import {
  ServerRack as Server,
  HostEndpoint,
} from "../components/InfrastructureDetails";
import type { HostVisit } from "../components/AnkerCompass";
import ConfigurationJourney from "../components/ConfigurationJourney";
import SettingHistory from "../components/SettingHistory";
import HostChangeDetail from "../components/HostChangeDetail";
import { displayHostName, HostName } from "../hostNames";
import ChangeTimeline from "../components/ChangeTimeline";
import ChangeSummary from "../components/ChangeSummary";
import { useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent } from "react";
import {
  ArrowLeft,
  ChevronDown,
  Search,
  ChevronRight,
  ShieldCheck,
  Network,
  History,
  GitCompareArrows,
  Plus,
  FolderOpen,
  RotateCcw,
  CircleAlert,
} from "lucide-react";
import { recoverySource } from "../hostHealth";
import { api, date, hostState } from "../api";
import type { Host, HostIdentity, Status } from "../api";
import {
  Dialog,
  Empty,
  Field,
  Heading,
  PageEmblem,
  Pagination,
  State,
} from "../components/shared";
import type { Notify } from "../components/shared";
import { BackupHistory } from "./Backups";
import BackupReadiness from "../components/BackupReadiness";
import Inventory from "../components/Inventory";
import ActiveJobs from "../components/ActiveJobs";
export function HostForm({
  host,
  automatic = !host,
  onClose,
  onSaved,
  notify,
}: {
  host?: Host;
  automatic?: boolean;
  onClose: () => void;
  onSaved: () => void;
  notify: Notify;
}) {
  const [method, setMethod] = useState(automatic ? "automatic" : "manual");
  const props = { host, onClose, onSaved, notify };
  return method === "automatic" ? (
    <HostConnection {...props} onManual={() => setMethod("manual")} />
  ) : (
    <ManualHostForm {...props} onAutomatic={() => setMethod("automatic")} />
  );
}

function HostConnection({
  host,
  onClose,
  onSaved,
  notify,
  onManual,
}: {
  host?: Host;
  onClose: () => void;
  onSaved: () => void;
  notify: Notify;
  onManual: () => void;
}) {
  const [value, setValue] = useState<Host>(() => ({
    id: "",
    name: "",
    address: "",
    group: "",
    cluster_id: "",
    enabled: true,
    schedule: "",
    extra_paths: [],
    ...host,
    ssh_user: "anker",
    restore_ssh_user: "anker-restore",
    ssh_port: host?.ssh_port || 22,
    key_path: "/etc/anker/keys/backup",
    restore_key_path: "/etc/anker/keys/restore",
    known_hosts_path: "/etc/anker/known_hosts",
  }));
  const [username, setUsername] = useState("root");
  const [password, setPassword] = useState("");
  const [identity, setIdentity] = useState<HostIdentity | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [advanced, setAdvanced] = useState(false);
  const [busy, setBusy] = useState<"inspect" | "enroll" | null>(null);
  const [error, setError] = useState("");
  const pending = useRef(false);
  const invalidate = () => {
    setIdentity(null);
    setConfirmed(false);
  };
  const set = (key: keyof Host, v: unknown) => {
    invalidate();
    setValue((old) => ({ ...old, [key]: v }));
  };
  const close = () => {
    if (pending.current) return;
    setPassword("");
    onClose();
  };
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (pending.current || (identity && (!confirmed || identity.changed)))
      return;
    pending.current = true;
    setBusy(identity ? "enroll" : "inspect");
    setError("");
    try {
      if (!identity) {
        const result = await api<HostIdentity>(
          "hosts/connection/inspect",
          "POST",
          {
            address: value.address.trim(),
            port: value.ssh_port,
          },
        );
        setIdentity(result);
        setConfirmed(false);
      } else {
        await api<Host>("hosts/connection/enroll", "POST", {
          host: {
            id: value.id,
            name: value.name,
            address: identity.address,
            group: value.group,
            cluster_id: value.cluster_id,
            ssh_user: value.ssh_user,
            ssh_port: identity.port,
            key_path: value.key_path,
            restore_ssh_user: value.restore_ssh_user,
            restore_key_path: value.restore_key_path,
            known_hosts_path: value.known_hosts_path,
            enabled: value.enabled,
            schedule: value.schedule,
            extra_paths: value.extra_paths,
          },
          username: username.trim(),
          password,
          fingerprint: identity.fingerprint,
          confirmed: true,
        });
        setPassword("");
        notify("Host angebunden");
        onSaved();
        onClose();
      }
    } catch (e) {
      const message = (e as Error).message;
      setError(password ? message.split(password).join("[entfernt]") : message);
      if (identity) {
        setPassword("");
        invalidate();
      }
    } finally {
      pending.current = false;
      setBusy(null);
    }
  }
  return (
    <Dialog
      title={host ? "Verbindung einrichten" : "Host hinzufügen"}
      onClose={close}
      busy={!!busy}
    >
      <form onSubmit={submit}>
        <p className="dialog-intro">
          Anker richtet die SSH-Schlüssel und Hosthelfer automatisch ein. Das
          Passwort wird einmalig zur Einrichtung verwendet und nicht
          gespeichert.
        </p>
        {identity ? (
          <section className="host-identity" aria-label="SSH-Identität">
            <h3>Hostidentität bestätigen</h3>
            <dl>
              <dt>Adresse</dt>
              <dd className="mono">
                {identity.address}:{identity.port}
              </dd>
              <dt>Schlüsseltyp</dt>
              <dd className="mono">{identity.key_type}</dd>
              <dt>Fingerprint</dt>
              <dd className="mono">{identity.fingerprint}</dd>
            </dl>
            {identity.changed ? (
              <p className="notice warning" role="alert">
                Der Hostschlüssel hat sich geändert. Die Anbindung ist gesperrt.
                Identität am Host prüfen und die Änderung über die vorhandene
                Hostschlüsselprüfung ausdrücklich übernehmen.
              </p>
            ) : (
              <>
                <p className="muted">
                  {identity.known
                    ? "Bekannter Hostschlüssel stimmt überein."
                    : "Noch nicht bekannt. Die Identität wurde noch nicht verifiziert."}
                </p>
                <p className="notice">
                  Vergleiche den Fingerprint über einen vertrauenswürdigen
                  Zugang, zum Beispiel die Proxmox-Konsole. Erst nach deiner
                  Bestätigung wird das Passwort an diesen Host gesendet.
                </p>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={confirmed}
                    disabled={!!busy}
                    onChange={(e) => setConfirmed(e.target.checked)}
                  />
                  Fingerprint geprüft und bestätigt
                </label>
              </>
            )}
            <button
              type="button"
              className="text-button"
              disabled={!!busy}
              onClick={invalidate}
            >
              Angaben ändern
            </button>
          </section>
        ) : (
          <>
            <Field label="Adresse">
              <input
                required
                autoComplete="off"
                disabled={!!busy}
                value={value.address}
                onChange={(e) => set("address", e.target.value)}
                placeholder="IP-Adresse oder DNS-Name"
              />
            </Field>
            <div className="form-grid">
              <Field
                label="SSH-Benutzer"
                hint="root oder ein Benutzer mit sudo-Rechten."
              >
                <input
                  required
                  autoComplete="off"
                  disabled={!!busy}
                  value={username}
                  onChange={(e) => {
                    invalidate();
                    setUsername(e.target.value);
                  }}
                />
              </Field>
              <Field label="SSH-Passwort">
                <input
                  type="password"
                  required
                  autoComplete="new-password"
                  disabled={!!busy}
                  value={password}
                  onChange={(e) => {
                    invalidate();
                    setPassword(e.target.value);
                  }}
                />
              </Field>
            </div>
            <button
              type="button"
              className="disclosure"
              aria-expanded={advanced}
              disabled={!!busy}
              onClick={() => setAdvanced(!advanced)}
            >
              <ChevronDown size={14} className={advanced ? "rotated" : ""} />
              Weitere Angaben
            </button>
            {advanced && (
              <div className="advanced">
                <div className="form-grid">
                  <Field
                    label="Hostname"
                    hint="Optional; wird vom Host übernommen."
                  >
                    <input
                      disabled={!!busy}
                      value={value.name}
                      onChange={(e) => set("name", e.target.value)}
                    />
                  </Field>
                  <Field label="SSH-Port">
                    <input
                      type="number"
                      required
                      min={1}
                      max={65535}
                      disabled={!!busy}
                      value={value.ssh_port}
                      onChange={(e) => set("ssh_port", +e.target.value)}
                    />
                  </Field>
                  <Field label="Gruppe">
                    <input
                      disabled={!!busy}
                      value={value.group}
                      onChange={(e) => set("group", e.target.value)}
                    />
                  </Field>
                  <Field
                    label="Cluster"
                    hint="Für Standalone-Hosts leer lassen."
                  >
                    <input
                      disabled={!!busy}
                      value={value.cluster_id}
                      onChange={(e) => set("cluster_id", e.target.value)}
                    />
                  </Field>
                </div>
                <Field
                  label="Startzeit"
                  hint="Leer übernimmt den gemeinsamen Zeitplan."
                >
                  <input
                    type="time"
                    disabled={!!busy}
                    value={value.schedule}
                    onChange={(e) => set("schedule", e.target.value)}
                  />
                </Field>
                <label className="check">
                  <input
                    type="checkbox"
                    disabled={!!busy}
                    checked={value.enabled}
                    onChange={(e) => set("enabled", e.target.checked)}
                  />
                  Automatisch sichern
                </label>
              </div>
            )}
            <button
              type="button"
              className="text-button host-method"
              disabled={!!busy}
              onClick={() => {
                setPassword("");
                onManual();
              }}
            >
              Manuell einrichten
            </button>
          </>
        )}
        {error && (
          <p className="notice warning" role="alert">
            {error}
          </p>
        )}
        {busy && (
          <p className="muted" role="status">
            {busy === "inspect"
              ? "SSH-Identität wird ohne Passwort geprüft …"
              : "Schlüssel und Hosthelfer werden eingerichtet und beide Zugänge geprüft. Das kann einen Moment dauern …"}
          </p>
        )}
        <footer className="dialog-footer">
          <button
            type="button"
            className="secondary"
            disabled={!!busy}
            onClick={close}
          >
            Abbrechen
          </button>
          <button
            disabled={
              !!busy || (!!identity && (identity.changed || !confirmed))
            }
          >
            {busy
              ? busy === "inspect"
                ? "Prüft …"
                : "Bindet an …"
              : identity
                ? "Host anbinden"
                : "Identität prüfen"}
          </button>
        </footer>
      </form>
    </Dialog>
  );
}

function ManualHostForm({
  host,
  onClose,
  onSaved,
  notify,
  onAutomatic,
}: {
  host?: Host;
  onClose: () => void;
  onSaved: () => void;
  notify: Notify;
  onAutomatic: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [advanced, setAdvanced] = useState(!!host);
  const [value, setValue] = useState<Host>(
    host || {
      id: "",
      name: "",
      address: "",
      group: "",
      cluster_id: "",
      ssh_user: "anker",
      ssh_port: 22,
      key_path: "/etc/anker/keys/backup",
      known_hosts_path: "/etc/anker/known_hosts",
      enabled: true,
      schedule: "",
      extra_paths: [],
    },
  );
  const set = (key: keyof Host, v: unknown) =>
    setValue((old) => ({ ...old, [key]: v }));
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await api("hosts", "POST", value);
      notify("Host gespeichert");
      onSaved();
      onClose();
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      title={host ? "Host bearbeiten" : "Host hinzufügen"}
      onClose={onClose}
      busy={busy}
    >
      <form onSubmit={submit}>
        <p className="dialog-intro">
          Anker verbindet sich über SSH. Der verifizierte Hostschlüssel und das
          Sicherungsprofil bestimmen den Zugriff.
        </p>
        <button
          type="button"
          className="text-button host-method"
          disabled={busy}
          onClick={onAutomatic}
        >
          Automatisch einrichten
        </button>
        <div className="form-grid">
          <Field label="Hostname">
            <input
              required
              autoComplete="off"
              value={value.name}
              onChange={(e) => set("name", e.target.value)}
              placeholder="Hostname des Proxmox-Hosts"
            />
          </Field>
          <Field label="Adresse">
            <input
              required
              value={value.address}
              onChange={(e) => set("address", e.target.value)}
              placeholder="IP-Adresse oder DNS-Name"
            />
          </Field>
          <Field label="Gruppe">
            <input
              value={value.group}
              onChange={(e) => set("group", e.target.value)}
              placeholder="Standort oder Gruppe"
            />
          </Field>
          <Field label="Cluster" hint="Für Standalone-Hosts leer lassen.">
            <input
              value={value.cluster_id}
              onChange={(e) => set("cluster_id", e.target.value)}
            />
          </Field>
        </div>
        <button
          className="disclosure"
          type="button"
          onClick={() => setAdvanced(!advanced)}
        >
          <ChevronDown size={14} className={advanced ? "rotated" : ""} />
          SSH und Sicherungsprofil
        </button>
        {advanced && (
          <div className="advanced">
            <div className="form-grid">
              <Field label="SSH-Benutzer">
                <input
                  value={value.ssh_user}
                  onChange={(e) => set("ssh_user", e.target.value)}
                />
              </Field>
              <Field label="SSH-Port">
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={value.ssh_port}
                  onChange={(e) => set("ssh_port", +e.target.value)}
                />
              </Field>
            </div>
            <Field
              label="Privater SSH-Schlüssel"
              hint="Geschützter Dateipfad auf dem Anker-Server."
            >
              <input
                value={value.key_path}
                onChange={(e) => set("key_path", e.target.value)}
                placeholder="/etc/anker/keys/backup"
              />
            </Field>
            <Field
              label="Wiederherstellungsschlüssel"
              hint="Optionaler, separat berechtigter SSH-Schlüssel. Der Sicherungsschlüssel kann nur lesen."
            >
              <input
                value={value.restore_key_path || ""}
                onChange={(e) => set("restore_key_path", e.target.value)}
                placeholder="/etc/anker/keys/restore"
              />
            </Field>
            <Field label="Wiederherstellungsbenutzer">
              <input
                value={value.restore_ssh_user || "anker-restore"}
                onChange={(e) => set("restore_ssh_user", e.target.value)}
              />
            </Field>
            <Field label="Verifizierte Hostschlüssel">
              <input
                value={value.known_hosts_path}
                onChange={(e) => set("known_hosts_path", e.target.value)}
                placeholder="/etc/anker/known_hosts"
              />
            </Field>
            <Field
              label="Zusätzliche Pflichtpfade"
              hint="Eine Zeile je Pfad. Diese Pfade müssen im Profil auf dem Host zugelassen sein."
            >
              <textarea
                rows={3}
                value={(value.extra_paths || []).join("\n")}
                onChange={(e) =>
                  set("extra_paths", e.target.value.split("\n").filter(Boolean))
                }
              />
            </Field>
            <Field
              label="Startzeit"
              hint="Leer übernimmt den gemeinsamen Zeitplan."
            >
              <input
                type="time"
                value={value.schedule}
                onChange={(e) => set("schedule", e.target.value)}
              />
            </Field>
            <label className="check">
              <input
                type="checkbox"
                checked={value.enabled}
                onChange={(e) => set("enabled", e.target.checked)}
              />
              Automatisch sichern
            </label>
          </div>
        )}
        <footer className="dialog-footer">
          <button
            type="button"
            className="secondary"
            disabled={busy}
            onClick={onClose}
          >
            Abbrechen
          </button>
          <button disabled={busy}>
            {busy ? "Speichert …" : "Host speichern"}
          </button>
        </footer>
      </form>
    </Dialog>
  );
}
export type HostStateFilter = "" | "active" | "saved" | "attention" | "paused";

function BackupAge({ createdAt }: { createdAt?: string }) {
  if (!createdAt) return <>—</>;
  if (!Number.isFinite(Date.parse(createdAt)))
    return <span>Datum ungeklärt</span>;
  if (Date.parse(createdAt) > Date.now() + 5 * 60 * 1000)
    return (
      <time dateTime={createdAt} title={date(createdAt)}>
        In der Zukunft
      </time>
    );
  const seconds = Math.max(
    0,
    (Date.now() - new Date(createdAt).getTime()) / 1000,
  );
  const formatter = new Intl.RelativeTimeFormat("de", { numeric: "always" });
  const relative =
    seconds < 60
      ? "gerade eben"
      : seconds < 3600
        ? formatter.format(-Math.floor(seconds / 60), "minute")
        : seconds < 86400
          ? formatter.format(-Math.floor(seconds / 3600), "hour")
          : formatter.format(-Math.floor(seconds / 86400), "day");
  const exact = date(createdAt);
  return (
    <time
      dateTime={createdAt}
      title={exact}
      aria-label={`${relative} · ${exact}`}
    >
      {relative}
    </time>
  );
}

export default function Hosts({
  status,
  notify,
  refresh,
  canEdit,
  canDownload = false,
  canOperate = canEdit,
  openBackup,
  openJob,
  onRestore,
  onHostContextChange,
  visit,
  onVisitConsumed,
  initialHost = "",
  onInitialHostConsumed,
  initialStateFilter = "",
  onInitialStateFilterConsumed,
}: {
  status: Status;
  notify: Notify;
  refresh: () => void;
  canEdit: boolean;
  canDownload?: boolean;
  canOperate?: boolean;
  openBackup: (id: string) => void;
  openJob?: (id: string) => void;
  onRestore: (id: string) => void;
  onHostContextChange?: (id: string) => void;
  visit?: HostVisit | null;
  onVisitConsumed?: () => void;
  initialHost?: string;
  onInitialHostConsumed?: () => void;
  initialStateFilter?: HostStateFilter;
  onInitialStateFilterConsumed?: () => void;
}) {
  const [query, setQuery] = useState("");
  const [group, setGroup] = useState("");
  const [stateFilter, setStateFilter] =
    useState<HostStateFilter>(initialStateFilter);
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState(initialHost);
  useEffect(() => {
    onHostContextChange?.(selected);
  }, [selected, onHostContextChange]);
  useEffect(() => () => onHostContextChange?.(""), [onHostContextChange]);
  useEffect(() => {
    if (initialHost) {
      setSelected(initialHost);
      setTab("Übersicht");
      onInitialHostConsumed?.();
    }
  }, [initialHost, onInitialHostConsumed]);
  const [tab, setTab] = useState("Übersicht");
  const [journeyTrace, setJourneyTrace] = useState<{
    hostID: string;
    id: string;
    sequence: number;
  }>();
  useEffect(() => {
    if (!initialStateFilter) return;
    setStateFilter(initialStateFilter);
    setQuery("");
    setGroup("");
    setPage(1);
    setSelected("");
    setTab("Übersicht");
    onInitialStateFilterConsumed?.();
  }, [initialStateFilter, onInitialStateFilterConsumed]);
  const [form, setForm] = useState<Host | "new" | null>(null);
  const [reconnecting, setReconnecting] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removeName, setRemoveName] = useState("");
  const host = status.hosts.find((h) => h.id === selected);
  const selectedState = host ? hostState(host, status) : undefined;
  const hostBackups = useMemo(
    () =>
      status.backups
        .filter((b) => b.host_id === selected)
        .sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [status.backups, selected],
  );
  const latest = hostBackups[0];
  const restoreSource = recoverySource(hostBackups);
  const latestSuccessful = restoreSource;
  const previous =
    latest &&
    hostBackups.find(
      (b) => Date.parse(b.created_at) < Date.parse(latest.created_at),
    );
  const [detailOpen, setDetailOpen] = useState(false);
  useEffect(() => {
    if (!visit) return;
    setSelected(visit.hostID);
    setTab(visit.tab);
    setDetailOpen(false);
    setJourneyTrace(undefined);
    setQuery("");
    setGroup("");
    setStateFilter("");
    onVisitConsumed?.();
  }, [visit, onVisitConsumed]);
  const returnFocus = useRef<HTMLElement | null>(null);
  const returnScroll = useRef(0);
  function openChangeDetail(from: string, to: string) {
    if (!host) return;
    returnFocus.current = document.activeElement as HTMLElement;
    returnScroll.current = window.scrollY;
    setSummaryPair({ hostID: host.id, from, to });
    setDetailOpen(true);
  }
  function closeChangeDetail() {
    setDetailOpen(false);
    requestAnimationFrame(() => {
      returnFocus.current?.focus({ preventScroll: true });
      window.scrollTo(0, returnScroll.current);
    });
  }
  const [summaryPair, setSummaryPair] = useState<{
    hostID: string;
    from: string;
    to: string;
  } | null>(null);
  useEffect(() => {
    setSummaryPair(null);
    setDetailOpen(false);
  }, [selected]);
  const groups = Array.from(
    new Set(status.hosts.map((h) => h.group).filter(Boolean)),
  ).sort();
  const hosts = status.hosts.filter(
    (h) =>
      (h.name + " " + h.address).toLowerCase().includes(query.toLowerCase()) &&
      (!group || h.group === group) &&
      (!stateFilter ||
        (stateFilter === "paused"
          ? !h.enabled
          : stateFilter === "active"
            ? h.enabled
            : stateFilter === "attention"
              ? h.enabled && hostState(h, status).tone !== "success"
              : h.enabled && hostState(h, status).tone === "success")),
  );
  const currentPage = Math.min(page, Math.max(1, Math.ceil(hosts.length / 8)));
  const saved = status.hosts.filter(
    (h) => hostState(h, status).tone === "success",
  ).length;
  const paused = status.hosts.filter((h) => !h.enabled).length;
  const attention = status.hosts.length - paused - saved;
  const actionInFlight = useRef(false);
  const [startingAction, setStartingAction] = useState(false);
  const activeHostJob = status.jobs.find(
    (j) => j.host_id === selected && ["queued", "running"].includes(j.state),
  );
  async function action(path: string, message: string) {
    if (actionInFlight.current) return;
    actionInFlight.current = true;
    setStartingAction(true);
    try {
      await api(path, "POST", {});
      notify(message);
      await refresh();
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      actionInFlight.current = false;
      setStartingAction(false);
    }
  }
  return (
    <>
      {detailOpen && host && summaryPair?.hostID === host.id && (
        <HostChangeDetail
          host={host}
          backups={hostBackups}
          from={summaryPair.from}
          to={summaryPair.to}
          canOperate={canOperate}
          canDownload={canDownload}
          notify={notify}
          onRestore={onRestore}
          onBack={closeChangeDetail}
        />
      )}
      <div hidden={detailOpen && !!host && summaryPair?.hostID === host.id}>
        {host ? (
          <>
            <button className="back-button" onClick={() => setSelected("")}>
              <ArrowLeft size={14} />
              Alle Hosts
            </button>
            <div className="page-kicker">
              <PageEmblem tone="blue">
                <Server size={20} strokeWidth={1.6} aria-hidden="true" />
              </PageEmblem>
              <span>
                {host.inventory?.cluster_id || host.cluster_id || "Standalone"}{" "}
                <span className="kicker-separator">/</span> Hostkonfiguration
              </span>
            </div>
            <Heading
              title={displayHostName(host.name)}
              accessibleTitle={host.name}
              description={
                host.name +
                " · " +
                host.address +
                (host.group ? " · " + host.group : "")
              }
              action={
                canOperate && (
                  <>
                    <button
                      disabled={
                        startingAction ||
                        status.jobs.some(
                          (j) =>
                            j.host_id === host.id &&
                            ["queued", "running"].includes(j.state),
                        )
                      }
                      className="secondary"
                      onClick={() =>
                        action(
                          "hosts/" + host.id + "/probe",
                          "Hostprüfung gestartet",
                        )
                      }
                    >
                      <Network size={15} aria-hidden="true" /> Verbindung prüfen
                    </button>
                    <button
                      disabled={
                        startingAction ||
                        status.jobs.some(
                          (j) =>
                            j.host_id === host.id &&
                            ["queued", "running"].includes(j.state),
                        )
                      }
                      onClick={() =>
                        action(
                          "hosts/" + host.id + "/backup",
                          "Sicherung gestartet",
                        )
                      }
                    >
                      <ShieldCheck size={15} aria-hidden="true" /> Jetzt sichern
                    </button>
                  </>
                )
              }
            />
            <HostEndpoint host={host} />
            <div className="tabs host-tabs">
              {[
                "Übersicht",
                "Sicherungen",
                "Änderungen",
                "Inventar",
                "Einstellungen",
              ].map((v) => (
                <button
                  key={v}
                  className={tab === v ? "selected" : ""}
                  aria-pressed={tab === v}
                  onClick={() => setTab(v)}
                >
                  {v}
                </button>
              ))}
            </div>
            {tab !== "Übersicht" && (
              <ActiveJobs
                status={{
                  hosts: [host],
                  jobs: status.jobs.filter((j) => j.host_id === host.id),
                }}
                openJob={openJob}
              />
            )}
            {tab === "Übersicht" && (
              <div className="detail-section host-overview-detail">
                <section
                  className={
                    "host-next-step host-focus-card " +
                    (activeHostJob
                      ? "is-running"
                      : selectedState?.tone === "success" && host.probe_error
                        ? "warning"
                        : selectedState?.tone || "")
                  }
                  aria-label="Für diesen Host"
                >
                  <span
                    className={
                      "object-icon " +
                      (activeHostJob
                        ? "blue"
                        : selectedState?.tone === "success" && !host.probe_error
                          ? "sage"
                          : "ochre")
                    }
                  >
                    {selectedState?.tone === "success" &&
                    !host.probe_error &&
                    !activeHostJob ? (
                      <ShieldCheck
                        size={19}
                        strokeWidth={1.6}
                        aria-hidden="true"
                      />
                    ) : (
                      <CircleAlert
                        size={19}
                        strokeWidth={1.6}
                        aria-hidden="true"
                      />
                    )}
                  </span>
                  <div>
                    <span className="eyebrow">
                      {activeHostJob ? "Gerade aktiv" : "Auf einen Blick"}
                    </span>
                    <h2>
                      {activeHostJob
                        ? activeHostJob.retry_at
                          ? "Der Auftrag wartet auf einen neuen Versuch"
                          : activeHostJob.state === "queued"
                            ? "Ein Auftrag wartet"
                            : activeHostJob.kind === "backup"
                              ? "Die Sicherung läuft"
                              : activeHostJob.kind === "probe"
                                ? "Die Verbindung wird geprüft"
                                : "Ein Auftrag läuft"
                        : selectedState?.tone === "success" && host.probe_error
                          ? "Die letzte Verbindungsprüfung ist fehlgeschlagen"
                          : selectedState?.tone === "success"
                            ? "Die letzte Sicherung ist aktuell"
                            : selectedState?.text}
                    </h2>
                    <p>
                      {activeHostJob
                        ? "Für diesen Host ist bereits ein Auftrag offen. Den Fortschritt und mögliche Fehler findest du im Auftrag."
                        : selectedState?.tone === "success" && host.probe_error
                          ? "Die Sicherung ist aktuell. Prüfe die Verbindung, bevor du die nächste Sicherung startest."
                          : selectedState?.reason}
                    </p>
                    {host.probe_error && (
                      <details className="host-connection-error">
                        <summary>Verbindungsfehler ansehen</summary>
                        <p>{host.probe_error}</p>
                      </details>
                    )}
                  </div>
                  {selectedState && (
                    <button
                      className="host-focus-action"
                      disabled={startingAction || (!!activeHostJob && !openJob)}
                      onClick={() => {
                        if (activeHostJob && openJob) {
                          openJob(activeHostJob.id);
                          return;
                        }
                        if (
                          selectedState.tone === "success" &&
                          host.probe_error &&
                          canOperate
                        ) {
                          void action(
                            "hosts/" + host.id + "/probe",
                            "Hostprüfung gestartet",
                          );
                          return;
                        }
                        if (selectedState.action === "files") {
                          setTab("Sicherungen");
                          return;
                        }
                        if (
                          selectedState.action === "job" &&
                          selectedState.jobID &&
                          openJob
                        ) {
                          openJob(selectedState.jobID);
                          return;
                        }
                        if (selectedState.action === "settings") {
                          setTab("Einstellungen");
                          return;
                        }
                        if (!canOperate) {
                          setTab("Sicherungen");
                          return;
                        }
                        if (
                          selectedState.action === "probe" ||
                          selectedState.action === "backup"
                        )
                          void action(
                            "hosts/" + host.id + "/" + selectedState.action,
                            selectedState.action === "probe"
                              ? "Hostprüfung gestartet"
                              : "Sicherung gestartet",
                          );
                        else setTab("Sicherungen");
                      }}
                    >
                      {activeHostJob
                        ? "Laufenden Auftrag ansehen"
                        : selectedState.tone === "success" &&
                            host.probe_error &&
                            canOperate
                          ? "Verbindung erneut prüfen"
                          : ["probe", "backup"].includes(
                                selectedState.action,
                              ) && !canOperate
                            ? "Verlauf ansehen"
                            : selectedState.actionLabel}
                      <ChevronRight size={14} aria-hidden="true" />
                    </button>
                  )}
                </section>
                <div className="host-status-strip">
                  <div>
                    <span className="eyebrow">Sicherungsstatus</span>
                    <strong className={selectedState?.tone}>
                      {selectedState?.text}
                    </strong>
                    <small>
                      {latestSuccessful ? (
                        <>
                          Letzter erfolgreicher Stand{" "}
                          <BackupAge createdAt={latestSuccessful.created_at} />
                        </>
                      ) : (
                        "Noch kein erfolgreicher Stand vorhanden"
                      )}
                    </small>
                  </div>
                  <div>
                    <span className="eyebrow">Automatische Sicherung</span>
                    <strong>
                      {status.demo
                        ? "In der Demo aus"
                        : host.enabled
                          ? host.schedule || "Gemeinsamer Zeitplan"
                          : "Pausiert"}
                    </strong>
                    <small>
                      {status.demo
                        ? "Sicherungen startest du hier von Hand"
                        : host.enabled
                          ? "Täglich · " + status.timezone
                          : "Vorhandene Stände bleiben erhalten"}
                    </small>
                  </div>
                  <div>
                    <span className="eyebrow">Verbindung</span>
                    <strong className={host.probe_error ? "warning" : ""}>
                      {host.probe_error
                        ? "Prüfung fehlgeschlagen"
                        : host.last_probe
                          ? "Zuletzt erreichbar"
                          : "Noch nicht geprüft"}
                    </strong>
                    <small>
                      {host.last_probe
                        ? date(host.last_probe)
                        : "Mit einer Hostprüfung beginnen"}
                    </small>
                  </div>
                </div>
                <div className="host-workspaces">
                  <section className="host-workspace">
                    <span className="eyebrow">
                      <FolderOpen size={14} aria-hidden="true" /> Letzter Stand
                    </span>
                    <h3>
                      {latest
                        ? date(latest.created_at)
                        : "Noch keine Sicherung"}
                    </h3>
                    <p>
                      {latest
                        ? latest.files +
                          " Einträge · " +
                          (latest.verified_at && !latest.verification_error
                            ? "Prüfsummen geprüft"
                            : "Prüfergebnis ansehen")
                        : "Nach der ersten Sicherung findest du hier deine Konfigurationsdateien."}
                    </p>
                    {latest && (
                      <div className="host-workspace-actions">
                        <button
                          className="secondary"
                          onClick={() => openBackup(latest.id)}
                        >
                          <FolderOpen size={15} aria-hidden="true" /> Dateien
                          öffnen
                        </button>
                        <button
                          className="text-button"
                          disabled={!previous}
                          onClick={() =>
                            previous &&
                            latest &&
                            openChangeDetail(previous.id, latest.id)
                          }
                        >
                          <GitCompareArrows size={15} aria-hidden="true" />{" "}
                          Änderungen ansehen
                        </button>
                      </div>
                    )}
                  </section>
                  <ChangeSummary
                    key={host.id}
                    compact
                    onHistory={() => {
                      setTab("Änderungen");
                      window.scrollTo(0, 0);
                    }}
                    backups={hostBackups}
                    openChanges={(from, to) => {
                      openChangeDetail(from, to);
                    }}
                  />
                  <section className="host-workspace">
                    <span className="eyebrow">
                      <RotateCcw size={14} aria-hidden="true" />{" "}
                      Wiederherstellung
                    </span>
                    <h3>Älteren Stand wiederherstellen</h3>
                    <p>
                      {restoreSource ? (
                        <>
                          Vorgeschlagene Quelle:{" "}
                          {date(restoreSource.created_at)}. Ziel und Änderungen
                          werden vor der Übernahme geprüft.
                        </>
                      ) : (
                        "Kein vollständiger Stand ohne bekannten Integritätsfehler verfügbar. Prüfe zuerst den Sicherungsverlauf."
                      )}
                      {restoreSource &&
                        latest &&
                        restoreSource.id !== latest.id && (
                          <span className="restore-source-note">
                            Jüngere Stände haben offene Fehler oder unplausible
                            Zeitstempel und werden hier nicht vorgeschlagen.
                          </span>
                        )}
                    </p>
                    {canOperate && (
                      <button
                        className="secondary"
                        disabled={!restoreSource}
                        onClick={() =>
                          restoreSource && onRestore(restoreSource.id)
                        }
                      >
                        Wiederherstellung planen{" "}
                        <ChevronRight size={14} aria-hidden="true" />
                      </button>
                    )}
                  </section>
                </div>
                <details className="host-extra-details" key={host.id}>
                  <summary>Details zum Host und zur letzten Sicherung</summary>
                  <BackupReadiness backup={latest} />
                  <dl className="host-facts">
                    <div>
                      <dt>Proxmox</dt>
                      <dd>
                        {host.inventory?.pve_version || "Noch nicht geprüft"}
                      </dd>
                    </div>
                    <div>
                      <dt>Cluster</dt>
                      <dd>
                        {host.inventory?.cluster_id ||
                          host.cluster_id ||
                          "Standalone"}
                      </dd>
                    </div>
                    <div>
                      <dt>Inventar</dt>
                      <dd>
                        {host.inventory ? "Vorhanden" : "Noch nicht erfasst"}
                      </dd>
                    </div>
                  </dl>
                </details>
              </div>
            )}
            {tab === "Sicherungen" && (
              <BackupHistory
                filterKey={host.id}
                onComparePrevious={(id, previousID) =>
                  openChangeDetail(previousID, id)
                }
                backups={hostBackups}
                references={status.references}
                referenceHosts={[host]}
                notify={notify}
                refresh={refresh}
                openBackup={openBackup}
                canEdit={canOperate}
                canDownload={canDownload}
                onRestore={onRestore}
              />
            )}{" "}
            {tab === "Änderungen" && (
              <ConfigurationJourney
                key={"journey-" + host.id}
                hostID={host.id}
                backups={hostBackups}
                onCompare={openChangeDetail}
                onOpenBackup={openBackup}
                onTrackSetting={(id) =>
                  setJourneyTrace((r) => ({
                    hostID: host.id,
                    id,
                    sequence: (r?.sequence || 0) + 1,
                  }))
                }
              />
            )}
            {tab === "Änderungen" && (
              <SettingHistory
                key={"changes-trace-" + host.id}
                request={
                  journeyTrace?.hostID === host.id ? journeyTrace : undefined
                }
                hostID={host.id}
                backups={hostBackups}
              />
            )}
            {tab === "Änderungen" &&
              (hostBackups.length > 1 ? (
                <div className="host-changes">
                  <div className="history-heading">
                    <div>
                      <span className="eyebrow">
                        <GitCompareArrows size={14} aria-hidden="true" />{" "}
                        Konfigurationsänderungen
                      </span>
                      <h3>Was hat sich verändert?</h3>
                      <p className="muted">
                        Dateiinhalte und Metadaten zwischen zwei Ständen dieses
                        Hosts.
                      </p>
                    </div>
                  </div>
                  <ChangeTimeline
                    key={"changes-" + host.id}
                    backups={hostBackups}
                    onCompare={openChangeDetail}
                  />
                  <button
                    className="secondary"
                    disabled={!previous || !latest}
                    onClick={() =>
                      previous &&
                      latest &&
                      openChangeDetail(previous.id, latest.id)
                    }
                  >
                    <GitCompareArrows size={16} aria-hidden="true" /> Letzte
                    zwei Stände im Detail vergleichen
                  </button>
                </div>
              ) : (
                <Empty title="Noch kein Vergleich möglich">
                  Für einen Vergleich werden zwei Sicherungsstände dieses Hosts
                  benötigt.
                </Empty>
              ))}
            {tab === "Inventar" &&
              (host.inventory ? (
                <Inventory
                  key={host.id}
                  inventory={host.inventory}
                  collectedAt={host.last_probe}
                  queryError={host.probe_error}
                />
              ) : (
                <Empty
                  title={
                    host.probe_error
                      ? "Inventarabfrage fehlgeschlagen"
                      : "Noch kein Inventar"
                  }
                >
                  {host.probe_error && (
                    <span className="warning" role="alert">
                      {host.probe_error}{" "}
                    </span>
                  )}
                  Mit „Verbindung prüfen“ liest Anker den aktuellen Hostzustand.
                </Empty>
              ))}
            {tab === "Einstellungen" && (
              <div className="detail-section">
                <dl className="properties">
                  <dt>Automatische Sicherung</dt>
                  <dd>
                    {host.enabled
                      ? host.schedule || "Gemeinsamer Zeitplan"
                      : "Pausiert"}
                  </dd>
                  <dt>Aktualitätsgrenze</dt>
                  <dd>
                    {Number.isFinite(status.stale_hours) &&
                    status.stale_hours > 0
                      ? status.stale_hours + " Stunden"
                      : "Nicht verfügbar"}
                    <div className="secondary-line">
                      Gemeinsame Vorgaben unter Einstellungen → Zeitplan.
                    </div>
                  </dd>
                  <dt>SSH-Benutzer</dt>
                  <dd>{host.ssh_user}</dd>
                  <dt>SSH-Port</dt>
                  <dd>{host.ssh_port}</dd>
                  <dt>Pflichtpfade</dt>
                  <dd>
                    /etc
                    {host.extra_paths?.map((p) => (
                      <div key={p}>{p}</div>
                    ))}
                  </dd>
                </dl>
                {canEdit && (
                  <div className="inline-form">
                    <button
                      className="secondary"
                      onClick={() => {
                        setReconnecting(true);
                        setForm(host);
                      }}
                    >
                      <Network size={15} aria-hidden="true" /> Verbindung
                      einrichten
                    </button>
                    <button
                      className="secondary"
                      onClick={() => {
                        setReconnecting(false);
                        setForm(host);
                      }}
                    >
                      Host bearbeiten
                    </button>
                    <button
                      className="text-button"
                      onClick={() => {
                        setRemoving(true);
                        setRemoveName("");
                      }}
                    >
                      Host entfernen
                    </button>
                  </div>
                )}
              </div>
            )}
          </>
        ) : (
          <>
            <div className="page-kicker">
              <PageEmblem tone="blue">
                <Server size={20} strokeWidth={1.6} aria-hidden="true" />
              </PageEmblem>
              <span>
                Infrastruktur <span className="kicker-separator">/</span> Hosts
                verwalten
              </span>
            </div>
            <Heading
              title="Hosts"
              count={hosts.length}
              description="Konfigurationen sichern und wiederherstellen."
              action={
                canEdit && (
                  <button
                    onClick={() => {
                      setReconnecting(false);
                      setForm("new");
                    }}
                  >
                    <Plus size={15} aria-hidden="true" /> Host hinzufügen
                  </button>
                )
              }
            />
            {status.hosts.length > 0 && (
              <section
                className="host-list-focus"
                aria-label="Hosts im Überblick"
              >
                <span className="compass-emblem">
                  {attention ? (
                    <CircleAlert size={20} aria-hidden="true" />
                  ) : (
                    <ShieldCheck size={20} aria-hidden="true" />
                  )}
                </span>
                <div>
                  <h2>
                    {attention
                      ? `${attention} ${attention === 1 ? "Host braucht" : "Hosts brauchen"} Aufmerksamkeit`
                      : paused === status.hosts.length
                        ? "Automatische Sicherungen sind pausiert"
                        : "Die Sicherungen sind aktuell"}
                  </h2>
                  <p>
                    {attention
                      ? "Sieh dir zuerst fehlende, ältere oder fehlerhafte Sicherungen an."
                      : paused === status.hosts.length
                        ? "Vorhandene Sicherungen kannst du weiterhin öffnen."
                        : "Dateien, Änderungen und Prüfungen findest du beim jeweiligen Host."}
                  </p>
                </div>
                <button
                  className="secondary"
                  onClick={() => {
                    setStateFilter(
                      attention
                        ? "attention"
                        : paused === status.hosts.length
                          ? "paused"
                          : "saved",
                    );
                    setQuery("");
                    setGroup("");
                    setPage(1);
                  }}
                >
                  {attention
                    ? "Betroffene Hosts anzeigen"
                    : paused === status.hosts.length
                      ? "Pausierte Hosts anzeigen"
                      : "Gesicherte Hosts anzeigen"}
                </button>
              </section>
            )}
            <div
              className="host-filter-chips"
              role="group"
              aria-label="Host-Schnellfilter"
            >
              {[
                { value: "", label: "Alle", count: status.hosts.length },
                {
                  value: "attention",
                  label: "Handlungsbedarf",
                  count: attention,
                },
                { value: "paused", label: "Pausiert", count: paused },
              ].map((filter) => (
                <button
                  key={filter.value}
                  type="button"
                  className={`host-filter-chip${stateFilter === filter.value ? " selected" : ""}`}
                  aria-pressed={stateFilter === filter.value}
                  onClick={() => {
                    setStateFilter(filter.value as HostStateFilter);
                    setPage(1);
                  }}
                >
                  {filter.label} <span>{filter.count}</span>
                </button>
              ))}
            </div>
            <div className="toolbar host-toolbar">
              <div className="search-field">
                <Search size={15} aria-hidden="true" />
                <input
                  className="search"
                  aria-label="Hosts durchsuchen"
                  placeholder="Hosts durchsuchen"
                  value={query}
                  onChange={(e) => {
                    setQuery(e.target.value);
                    setPage(1);
                  }}
                />
              </div>
              <select
                aria-label="Gruppe filtern"
                value={group}
                onChange={(e) => {
                  setGroup(e.target.value);
                  setPage(1);
                }}
              >
                <option value="">Alle Gruppen</option>
                {groups.map((g) => (
                  <option key={g}>{g}</option>
                ))}
              </select>
              <select
                aria-label="Hoststatus filtern"
                value={stateFilter}
                onChange={(e) => {
                  setStateFilter(e.target.value as HostStateFilter);
                  setPage(1);
                }}
              >
                <option value="">Alle Status</option>
                <option value="active">Aktive Hosts</option>
                <option value="attention">Handlungsbedarf</option>
                <option value="saved">Aktuell gesichert</option>
                <option value="paused">Pausiert</option>
              </select>
            </div>
            {hosts.length ? (
              <div className="table-scroll data-surface">
                <table className="host-table">
                  <thead>
                    <tr>
                      <th>Host</th>
                      <th>Gruppe</th>
                      <th>Letzte Sicherung</th>
                      <th>Status</th>
                      <th className="right">Aktionen</th>
                    </tr>
                  </thead>
                  <tbody>
                    {hosts
                      .slice((currentPage - 1) * 8, currentPage * 8)
                      .map((h) => {
                        const state = hostState(h, status);
                        return (
                          <tr key={h.id}>
                            <td>
                              <button
                                className="name-link"
                                aria-label={h.name}
                                onClick={() => {
                                  setSelected(h.id);
                                  setTab("Übersicht");
                                }}
                              >
                                <HostName name={h.name} />
                              </button>
                              <div className="secondary-line mono">
                                {h.address}
                              </div>
                              <div className="secondary-line host-backup-mobile">
                                Letzter Stand:{" "}
                                <BackupAge
                                  createdAt={state.backup?.created_at}
                                />
                              </div>
                            </td>
                            <td>
                              <span
                                className={
                                  h.cluster_id
                                    ? "group-tag cluster"
                                    : "group-tag"
                                }
                              >
                                {h.group || "Standalone"}
                              </span>
                            </td>
                            <td className="date">
                              <BackupAge createdAt={state.backup?.created_at} />
                            </td>
                            <td>
                              <span className={"state " + state.tone}>
                                {state.text}
                              </span>
                              {status.jobs.some(
                                (j) =>
                                  j.host_id === h.id &&
                                  j.kind === "backup" &&
                                  ["queued", "running"].includes(j.state),
                              ) && (
                                <div className="secondary-line">
                                  <State
                                    value={
                                      status.jobs.some(
                                        (j) =>
                                          j.host_id === h.id &&
                                          j.kind === "backup" &&
                                          j.state === "running",
                                      )
                                        ? "running"
                                        : "queued"
                                    }
                                    label={
                                      status.jobs.some(
                                        (j) =>
                                          j.host_id === h.id &&
                                          j.kind === "backup" &&
                                          j.state === "running",
                                      )
                                        ? "Sicherung läuft"
                                        : "Sicherung wartet"
                                    }
                                  />
                                </div>
                              )}
                            </td>
                            <td className="right">
                              <button
                                className="text-button"
                                onClick={() => {
                                  setSelected(h.id);
                                  setTab("Übersicht");
                                }}
                              >
                                Öffnen{" "}
                                <ChevronRight size={14} aria-hidden="true" />
                              </button>
                            </td>
                          </tr>
                        );
                      })}
                  </tbody>
                </table>
              </div>
            ) : (
              <Empty
                title={
                  query || group || stateFilter
                    ? "Keine passenden Hosts"
                    : "Noch keine Hosts"
                }
              >
                {query || group || stateFilter
                  ? "Suche oder Filter ändern."
                  : "Mit „Host hinzufügen“ richtest du die erste Verbindung ein."}
              </Empty>
            )}
            {hosts.length > 0 && (
              <Pagination
                page={currentPage}
                pageSize={8}
                total={hosts.length}
                onPageChange={setPage}
                label="Hosts"
              />
            )}
            <p className="table-footnote">
              {status.hosts.length} Hosts · {saved} gesichert ·{" "}
              {status.hosts.filter((h) => h.enabled).length - saved} mit
              Handlungsbedarf
              {status.hosts.some((h) => !h.enabled) &&
                " · " +
                  status.hosts.filter((h) => !h.enabled).length +
                  " pausiert"}
            </p>
          </>
        )}
        {removing && host && (
          <Dialog title="Host entfernen" onClose={() => setRemoving(false)}>
            <p className="dialog-intro">
              Zeitplan und Zugangseintrag für {host.name} entfernen. Vorhandene
              Sicherungen bleiben erhalten.
            </p>
            <Field label="Hostnamen bestätigen">
              <input
                value={removeName}
                onChange={(e) => setRemoveName(e.target.value)}
                placeholder={host.name}
              />
            </Field>
            <footer className="dialog-footer">
              <button className="secondary" onClick={() => setRemoving(false)}>
                Abbrechen
              </button>
              <button
                disabled={removeName !== host.name}
                onClick={async () => {
                  try {
                    await api("hosts/" + host.id, "DELETE");
                    setRemoving(false);
                    setSelected("");
                    notify("Host entfernt");
                    refresh();
                  } catch (e) {
                    notify((e as Error).message, true);
                  }
                }}
              >
                Host entfernen
              </button>
            </footer>
          </Dialog>
        )}
        {form && (
          <HostForm
            host={form === "new" ? undefined : form}
            automatic={form === "new" || reconnecting}
            onClose={() => setForm(null)}
            onSaved={refresh}
            notify={notify}
          />
        )}
      </div>
    </>
  );
}
