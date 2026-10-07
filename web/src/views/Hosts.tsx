import { useRef, useState } from "react";
import type { FormEvent } from "react";
import { ArrowLeft, ChevronDown, Search, ChevronRight } from "lucide-react";
import { api, date, hostState } from "../api";
import type { Host, HostIdentity, Status } from "../api";
import {
  Dialog,
  Empty,
  Field,
  Heading,
  Pagination,
  State,
} from "../components/shared";
import type { Notify } from "../components/shared";
import { BackupTable } from "./Backups";
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
}) {
  const [query, setQuery] = useState("");
  const [group, setGroup] = useState("");
  const [stateFilter, setStateFilter] = useState("");
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState("");
  const [tab, setTab] = useState("Übersicht");
  const [form, setForm] = useState<Host | "new" | null>(null);
  const [reconnecting, setReconnecting] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removeName, setRemoveName] = useState("");
  const host = status.hosts.find((h) => h.id === selected);
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
          : stateFilter === "attention"
            ? h.enabled && hostState(h, status).tone !== "success"
            : h.enabled && hostState(h, status).tone === "success")),
  );
  const currentPage = Math.min(page, Math.max(1, Math.ceil(hosts.length / 25)));
  const saved = status.hosts.filter(
    (h) => hostState(h, status).tone === "success",
  ).length;
  async function action(path: string, message: string) {
    try {
      await api(path, "POST", {});
      notify(message);
      refresh();
    } catch (e) {
      notify((e as Error).message, true);
    }
  }
  return (
    <>
      {host ? (
        <>
          <button className="back-button" onClick={() => setSelected("")}>
            <ArrowLeft size={14} />
            Alle Hosts
          </button>
          <Heading
            title={host.name}
            description={host.address + (host.group ? " · " + host.group : "")}
            action={
              canOperate && (
                <>
                  {canEdit && (
                    <button
                      className="secondary"
                      onClick={() => {
                        setReconnecting(true);
                        setForm(host);
                      }}
                    >
                      Verbindung einrichten
                    </button>
                  )}
                  <button
                    disabled={status.jobs.some(
                      (j) =>
                        j.host_id === host.id &&
                        ["queued", "running"].includes(j.state),
                    )}
                    className="secondary"
                    onClick={() =>
                      action(
                        "hosts/" + host.id + "/probe",
                        "Hostprüfung gestartet",
                      )
                    }
                  >
                    Verbindung prüfen
                  </button>
                  <button
                    disabled={status.jobs.some(
                      (j) =>
                        j.host_id === host.id &&
                        ["queued", "running"].includes(j.state),
                    )}
                    onClick={() =>
                      action(
                        "hosts/" + host.id + "/backup",
                        "Sicherung gestartet",
                      )
                    }
                  >
                    Jetzt sichern
                  </button>
                </>
              )
            }
          />
          <div className="tabs">
            {["Übersicht", "Sicherungen", "Inventar", "Einstellungen"].map(
              (v) => (
                <button
                  key={v}
                  className={tab === v ? "selected" : ""}
                  onClick={() => setTab(v)}
                >
                  {v}
                </button>
              ),
            )}
          </div>
          <ActiveJobs
            status={{
              hosts: [host],
              jobs: status.jobs.filter((j) => j.host_id === host.id),
            }}
            openJob={openJob}
          />
          {tab === "Übersicht" && (
            <div className="detail-section">
              <dl className="properties">
                <dt>Status</dt>
                <dd className={hostState(host, status).tone}>
                  {hostState(host, status).text}
                </dd>
                <dt>Letzte Sicherung</dt>
                <dd>{date(hostState(host, status).backup?.created_at)}</dd>
                <dt>Proxmox</dt>
                <dd>{host.inventory?.pve_version || "Noch nicht geprüft"}</dd>
                <dt>Cluster</dt>
                <dd>
                  {host.inventory?.cluster_id ||
                    host.cluster_id ||
                    "Standalone"}
                </dd>
                <dt>Automatische Sicherung</dt>
                <dd>
                  {host.enabled
                    ? host.schedule || "Gemeinsamer Zeitplan"
                    : "Ausgeschaltet"}
                </dd>
                <dt>Letzte Hostprüfung</dt>
                <dd>{date(host.last_probe)}</dd>
              </dl>
              {host.probe_error && (
                <p className="notice warning">{host.probe_error}</p>
              )}
              <div className="section-rule" />
              <h3>Wiederherstellung vorbereiten</h3>
              <p className="muted">
                Eine Sicherung auswählen, das Ziel prüfen und die Unterschiede
                vor der Übernahme ansehen.
              </p>
              {canOperate && (
                <button
                  className="secondary"
                  disabled={!status.backups.some((b) => b.host_id === host.id)}
                  onClick={() =>
                    onRestore(
                      status.backups.find((b) => b.host_id === host.id)?.id ||
                        "",
                    )
                  }
                >
                  Wiederherstellung planen
                </button>
              )}
            </div>
          )}
          {tab === "Sicherungen" && (
            <BackupTable
              filterKey={host.id}
              backups={status.backups.filter((b) => b.host_id === host.id)}
              notify={notify}
              refresh={refresh}
              openBackup={openBackup}
              canEdit={canOperate}
              canDownload={canDownload}
              onRestore={onRestore}
            />
          )}{" "}
          {tab === "Inventar" &&
            (host.inventory ? (
              <Inventory
                key={host.id}
                inventory={host.inventory}
                collectedAt={host.last_probe}
              />
            ) : (
              <Empty title="Noch kein Inventar">
                Mit „Verbindung prüfen“ liest Anker den aktuellen Hostzustand.
              </Empty>
            ))}
          {tab === "Einstellungen" && (
            <div className="detail-section">
              <dl className="properties">
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
          <Heading
            title="Hosts"
            description="Konfigurationen sichern und wiederherstellen."
            action={
              canEdit && (
                <button
                  onClick={() => {
                    setReconnecting(false);
                    setForm("new");
                  }}
                >
                  Host hinzufügen
                </button>
              )
            }
          />
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
                setStateFilter(e.target.value);
                setPage(1);
              }}
            >
              <option value="">Alle Status</option>
              <option value="attention">Handlungsbedarf</option>
              <option value="saved">Aktuell gesichert</option>
              <option value="paused">Pausiert</option>
            </select>
          </div>
          {hosts.length ? (
            <div className="table-scroll">
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
                    .slice((currentPage - 1) * 25, currentPage * 25)
                    .map((h) => {
                      const state = hostState(h, status);
                      return (
                        <tr key={h.id}>
                          <td>
                            <button
                              className="name-link"
                              onClick={() => {
                                setSelected(h.id);
                                setTab("Übersicht");
                              }}
                            >
                              {h.name}
                            </button>
                            <div className="secondary-line mono">
                              {h.address}
                            </div>
                          </td>
                          <td>
                            <span
                              className={
                                h.cluster_id ? "group-tag cluster" : "group-tag"
                              }
                            >
                              {h.group || "Standalone"}
                            </span>
                          </td>
                          <td className="date">
                            {date(state.backup?.created_at)}
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
              pageSize={25}
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
    </>
  );
}
