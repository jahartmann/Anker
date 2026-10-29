import { useEffect, useRef, useState } from "react";
import { api, date } from "../api";
import type { Host, HostIdentity } from "../api";
import { displayHostName, HostName } from "../hostNames";
import { Dialog, Field } from "./shared";
import { CheckSquare, ScanLine, Wrench, ShieldCheck } from "lucide-react";
import { Pagination } from "./shared";
import { useListPageSize } from "../listPageSize";

type Target = {
  host_id: string;
  revision: string;
  name?: string;
  state?: string;
  message?: string;
  job_id?: string;
};
type Run = { id: string; targets: Target[] };
const actions: Record<string, string> = {
  backup: "Sichern und verifizieren",
  probe: "Verbindung und Inventar prüfen",
  verify: "Letzte Sicherung verifizieren",
  drift: "Liveänderungen prüfen",
  maintenance: "Befristete Wartung",
  maintenance_end: "Wartung beenden",
  pause: "Automatik pausieren",
  resume: "Automatik aktivieren",
  schedule: "Zeitplan setzen",
  group: "Standort / Gruppe setzen",
  drift_schedule: "Automatische Liveprüfung",
};
export default function HostOperations({
  hosts,
  filtered,
  selection,
  onSelection,
  canManage,
  canOperate,
  refresh,
  openJob,
  actorID,
}: {
  hosts: Host[];
  filtered: Host[];
  selection: Record<string, string>;
  onSelection: (v: Record<string, string>) => void;
  canManage: boolean;
  canOperate: boolean;
  refresh: () => void;
  openJob?: (id: string) => void;
  actorID: string;
}) {
  const [action, setAction] = useState("backup");
  const [value, setValue] = useState("");
  const [minutes, setMinutes] = useState(0);
  const [hours, setHours] = useState(4);
  const [review, setReview] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [run, setRun] = useState<Run>();
  const [helper, setHelper] = useState(false);
  const targets = Object.entries(selection).map(([host_id, revision]) => ({
    host_id,
    revision,
  }));
  const [request, setRequest] = useState<{
    request_id: string;
    action: string;
    targets: Target[];
    value: string;
    minutes: number;
    until: string;
  }>();
  const activeRequest = useRef("");
  const key = `anker.host-operation.${actorID}`;
  useEffect(() => {
    setRun(undefined);
    setRequest(undefined);
    try {
      const saved = sessionStorage.getItem(key);
      if (saved) {
        const parsed = JSON.parse(saved);
        if (
          typeof parsed.request_id !== "string" ||
          !/^[a-zA-Z0-9_-]{1,100}$/.test(parsed.request_id) ||
          !Array.isArray(parsed.targets) ||
          typeof parsed.action !== "string"
        )
          throw new Error("invalid stored selection");
        activeRequest.current = parsed.request_id;
        setRequest(parsed);
        api<Run>(`operations/hosts/${parsed.request_id}`)
          .then((value) => {
            if (activeRequest.current === parsed.request_id) setRun(value);
          })
          .catch(() => {
            if (activeRequest.current === parsed.request_id)
              setError(
                "Letzte Aktion noch nicht bestätigt. Mit derselben Auswahl erneut abfragen oder ausführen.",
              );
          });
      }
    } catch {
      /* Storage optional. */
    }
    return () => {
      activeRequest.current = "";
    };
  }, [key]);
  function prepare() {
    if (error && request) {
      setReview(true);
      return;
    }
    const next = {
      request_id: crypto.randomUUID(),
      action,
      targets,
      value,
      minutes,
      until:
        action === "maintenance"
          ? new Date(Date.now() + hours * 3600000).toISOString()
          : "",
    };
    activeRequest.current = next.request_id;
    setRequest(next);
    setReview(true);
    setError("");
  }
  async function execute() {
    if (!request) return;
    setBusy(true);
    setError("");
    try {
      sessionStorage.setItem(key, JSON.stringify(request));
    } catch {
      /* Request still remains in memory. */
    }
    try {
      const result = await api<Run>("operations/hosts", "POST", request);
      setRun(result);
      setReview(false);
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="operations-card" aria-label="Sammelaktionen">
      <div className="operations-heading">
        <CheckSquare size={20} />
        <div>
          <h3>
            {canOperate ? "Gemeinsam verwalten" : "Hosthelfer im Überblick"}
          </h3>
          <p>
            Feste Auswahl über alle Seiten. Geänderte Hosts werden vor der
            Ausführung erkannt.
          </p>
        </div>
        {canOperate && (
          <span className="count-badge">{targets.length} ausgewählt</span>
        )}
      </div>
      <div className="operations-controls">
        {canOperate && (
          <>
            <button
              className="secondary"
              onClick={() =>
                onSelection({
                  ...selection,
                  ...Object.fromEntries(
                    filtered
                      .filter((h) => h.config_revision)
                      .map((h) => [h.id, h.config_revision!]),
                  ),
                })
              }
            >
              {filtered.length === 1
                ? "Diesen gefilterten Host auswählen"
                : `Alle ${filtered.length} gefilterten Hosts auswählen`}
            </button>
            {targets.length > 0 && (
              <>
                <button
                  className="text-button"
                  disabled={!targets.length}
                  onClick={() => onSelection({})}
                >
                  Auswahl leeren
                </button>
                <select
                  aria-label="Sammelaktion"
                  value={action}
                  onChange={(e) => {
                    setAction(e.target.value);
                    setValue("");
                  }}
                >
                  {Object.entries(actions)
                    .filter(
                      ([key]) =>
                        canManage ||
                        ["backup", "probe", "verify", "drift"].includes(key),
                    )
                    .map(([key, label]) => (
                      <option key={key} value={key}>
                        {label}
                      </option>
                    ))}
                </select>
                {action === "schedule" && (
                  <input
                    aria-label="Gemeinsamer Zeitplan"
                    type="time"
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                  />
                )}
                {action === "group" && (
                  <input
                    aria-label="Gemeinsame Gruppe"
                    placeholder="Standort / Gruppe"
                    maxLength={200}
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                  />
                )}
                {action === "maintenance" && (
                  <select
                    aria-label="Wartungsdauer"
                    value={hours}
                    onChange={(e) => setHours(+e.target.value)}
                  >
                    {[1, 4, 24, 72].map((h) => (
                      <option key={h} value={h}>
                        {h} Stunden
                      </option>
                    ))}
                  </select>
                )}
                {action === "drift_schedule" && (
                  <select
                    aria-label="Prüfintervall"
                    value={minutes}
                    onChange={(e) => setMinutes(+e.target.value)}
                  >
                    {[0, 15, 30, 60, 240, 1440].map((m) => (
                      <option key={m} value={m}>
                        {m ? `Alle ${m} Minuten` : "Ausgeschaltet"}
                      </option>
                    ))}
                  </select>
                )}
                <button disabled={!targets.length || busy} onClick={prepare}>
                  {error && request
                    ? "Unbestätigte Aktion prüfen"
                    : "Aktion prüfen"}
                </button>
              </>
            )}
          </>
        )}
        <button className="secondary" onClick={() => setHelper(true)}>
          <Wrench size={15} /> Helferübersicht
        </button>
      </div>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {error && request && (
        <button
          className="text-button"
          disabled={busy}
          onClick={() => {
            setError("");
            setRequest(undefined);
            try {
              sessionStorage.removeItem(key);
            } catch {
              /* Optional */
            }
          }}
        >
          Quittung schließen und neue Auswahl vorbereiten
        </button>
      )}
      {request && !review && error && canOperate && (
        <button
          className="secondary"
          disabled={busy}
          onClick={() => setReview(true)}
        >
          Letzte Aktion erneut prüfen
        </button>
      )}
      {run && (
        <details className="operation-receipt" open>
          <summary>
            Ergebnis:{" "}
            {
              run.targets.filter((t) =>
                ["queued", "applied"].includes(t.state || ""),
              ).length
            }{" "}
            von {run.targets.length} übernommen
          </summary>
          <div className="operation-results">
            {run.targets.map((t) => (
              <div key={t.host_id}>
                <strong>
                  <HostName
                    name={
                      hosts.find((h) => h.id === t.host_id)?.name ||
                      t.name ||
                      t.host_id
                    }
                  />
                </strong>
                <span
                  className={`state ${t.state === "skipped" ? "warning" : "muted"}`}
                >
                  {t.state === "queued"
                    ? "Auftrag gestartet"
                    : t.state === "applied"
                      ? "Übernommen"
                      : t.state === "pending"
                        ? "Noch offen"
                        : "Übersprungen"}
                </span>
                <span>{t.message}</span>
                {t.job_id && openJob && (
                  <button
                    className="text-button"
                    onClick={() => openJob(t.job_id!)}
                  >
                    Auftrag ansehen
                  </button>
                )}
              </div>
            ))}
          </div>
        </details>
      )}
      {review && request && (
        <Dialog
          title="Sammelaktion prüfen"
          onClose={() => {
            if (!busy) setReview(false);
          }}
        >
          <p>
            <strong>{actions[request.action]}</strong> für{" "}
            {request.targets.length} ausdrücklich ausgewählte Hosts.
          </p>
          {["schedule", "group"].includes(request.action) && (
            <p>
              Neuer Wert: <strong>{request.value || "Leeren"}</strong>
            </p>
          )}
          {request.action === "maintenance" && (
            <p>
              Bis {date(request.until)}. Die bisherigen Zeitpläne bleiben
              gespeichert und gelten nach Ablauf wieder.
            </p>
          )}
          {request.action === "drift_schedule" && (
            <p>
              {request.minutes
                ? `Lesende Liveprüfung alle ${request.minutes} Minuten.`
                : "Automatische Liveprüfung ausschalten."}{" "}
              Konfigurationsinhalte werden dabei nicht als Sicherung
              gespeichert.
            </p>
          )}
          <ul className="selection-review">
            {request.targets.map((t) => (
              <li key={t.host_id}>
                <HostName
                  name={
                    hosts.find((h) => h.id === t.host_id)?.name || t.host_id
                  }
                />
              </li>
            ))}
          </ul>
          <p className="muted">
            Belegte oder inzwischen bearbeitete Hosts werden mit Begründung
            übersprungen. Manuelle Prüfungen sind auch während einer Wartung
            möglich.
          </p>
          {error && <p role="alert">{error}</p>}
          <button disabled={busy} onClick={execute}>
            {busy ? "Wird übermittelt …" : "Für diese Auswahl ausführen"}
          </button>
        </Dialog>
      )}
      {helper && (
        <HelperOverview
          hosts={hosts}
          canManage={canManage}
          canOperate={canOperate}
          openJob={openJob}
          onClose={() => setHelper(false)}
          refresh={refresh}
        />
      )}
    </section>
  );
}

type Helper = {
  host_id: string;
  name: string;
  state: string;
  installed_version?: string;
  installed_hash?: string;
  desired_version: string;
  desired_hash: string;
  last_probe?: string;
  capabilities?: Record<string, unknown>;
};
function HelperOverview({
  hosts,
  canManage,
  canOperate,
  openJob,
  onClose,
  refresh,
}: {
  hosts: Host[];
  canManage: boolean;
  canOperate: boolean;
  openJob?: (id: string) => void;
  onClose: () => void;
  refresh: () => void;
}) {
  const [loading, setLoading] = useState(true);
  const [list, setList] = useState<Helper[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [target, setTarget] = useState<Host>();
  const [identity, setIdentity] = useState<HostIdentity>();
  const [username, setUsername] = useState("root");
  const [password, setPassword] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [done, setDone] = useState("");
  const [page, setPage] = useState(1);
  const [size, setSize] = useListPageSize("helpers", 10);
  const observationRevision = JSON.stringify(
    hosts.map((h) => [h.id, h.last_probe]),
  );
  useEffect(() => {
    let valid = true;
    setLoading(true);
    api<Helper[]>("operations/helpers")
      .then((v) => {
        if (valid) {
          setList(v);
          setLoading(false);
          setError("");
        }
      })
      .catch((e) => {
        if (valid) {
          setError(e.message);
          setLoading(false);
        }
      });
    return () => {
      valid = false;
    };
  }, [observationRevision]);
  async function probe(id: string) {
    setBusy(true);
    setError("");
    try {
      const job = await api<{ id: string }>(`hosts/${id}/probe`, "POST");
      setDone(
        "Hostprüfung gestartet. Version und Fähigkeiten werden nach Abschluss neu eingelesen.",
      );
      refresh();
      openJob?.(job.id);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function inspect(h: Host) {
    setBusy(true);
    setError("");
    setDone("");
    setTarget(h);
    setIdentity(undefined);
    setConfirmed(false);
    setPassword("");
    try {
      setIdentity(
        await api<HostIdentity>("hosts/connection/inspect", "POST", {
          address: h.address,
          port: h.ssh_port,
        }),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function update() {
    if (!target || !identity) return;
    setBusy(true);
    setError("");
    try {
      await api("operations/helpers/refresh", "POST", {
        host_id: target.id,
        revision: target.config_revision,
        username,
        password,
        fingerprint: identity.fingerprint,
        confirmed,
      });
      setDone(
        `${displayHostName(target.name)}: Helferdatei aktualisiert. Jetzt Host prüfen, um Version und Fähigkeiten neu einzulesen.`,
      );
      setPassword("");
      setTarget(undefined);
      refresh();
    } catch (e) {
      setError((e as Error).message);
      setPassword("");
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      title="Hosthelfer · Versionen und Fähigkeiten"
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <p>
        Installierter Stand aus der letzten Hostprüfung. Ältere Helfer ohne
        Selbstkennung bleiben „Unbekannt“.
      </p>
      {loading && <p role="status">Helferstände werden geladen …</p>}
      {!loading && !error && list.length === 0 && (
        <p>Noch keine Hosts eingerichtet.</p>
      )}
      <div className="helper-summary" aria-busy={loading}>
        <span>
          <ShieldCheck size={16} />{" "}
          {list.filter((v) => v.state === "current").length} passend
        </span>
        <span>
          {list.filter((v) => v.state === "different").length} abweichend
        </span>
        <span>
          {list.filter((v) => v.state === "unknown").length} unbekannt
        </span>
      </div>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {done && <p role="status">{done}</p>}
      <div className="operation-results">
        {list.slice((page - 1) * size, page * size).map((v) => (
          <div key={v.host_id}>
            <strong>
              <HostName name={v.name} />
            </strong>
            <span
              className={`state ${v.state === "current" ? "success" : "muted"}`}
            >
              {v.state === "current"
                ? "Passend"
                : v.state === "different"
                  ? "Abweichend"
                  : "Unbekannt"}
            </span>
            <small>
              Installiert: {v.installed_version || "unbekannt"} · Ziel:{" "}
              {v.desired_version} · {date(v.last_probe)}
            </small>
            <details>
              <summary>Technische Identität und Fähigkeiten</summary>
              <p className="hash-line">
                Installiert: {v.installed_hash || "Kein Hash gemeldet"}
                <br />
                Ziel: {v.desired_hash}
              </p>
              <p>
                {v.capabilities && Object.keys(v.capabilities).length > 0
                  ? Object.entries(v.capabilities)
                      .map(([k, value]) => `${k}: ${String(value)}`)
                      .join(" · ")
                  : "Für Fähigkeiten Host mit aktuellem Helfer prüfen."}
              </p>
            </details>
            {canOperate && (
              <button
                className="secondary"
                disabled={busy}
                onClick={() => probe(v.host_id)}
              >
                Version und Fähigkeiten prüfen
              </button>
            )}
            {canManage && (
              <button
                className="secondary"
                disabled={busy}
                onClick={() => {
                  const h = hosts.find((h) => h.id === v.host_id);
                  if (h) void inspect(h);
                }}
              >
                Helfer aktualisieren
              </button>
            )}
          </div>
        ))}
      </div>
      <Pagination
        page={page}
        pageSize={size}
        total={list.length}
        onPageChange={setPage}
        onPageSizeChange={setSize}
        label="Hosthelfer"
      />
      {target && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void update();
          }}
          className="helper-update"
        >
          <h3>
            <HostName name={target.name} /> · Helferdatei ersetzen
          </h3>
          <p>
            Hostprofil, Zeitpläne und Rollenschlüssel bleiben bestehen.
            Administratorzugang wird nur für diese Übertragung verwendet.
          </p>
          {identity && (
            <>
              <p className="mono hash-line">{identity.fingerprint}</p>
              {!identity.known || identity.changed ? (
                <p role="alert">
                  Keine unveränderte, bereits vertraute Hostidentität.
                  Aktualisierung gesperrt.
                </p>
              ) : (
                <>
                  <Field label="Administrator">
                    <input
                      autoComplete="username"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                    />
                  </Field>
                  <Field label="Passwort">
                    <input
                      type="password"
                      autoComplete="current-password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                    />
                  </Field>
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={confirmed}
                      onChange={(e) => setConfirmed(e.target.checked)}
                    />
                    Diese Hostidentität bestätigen und nur die Helferdatei
                    ersetzen
                  </label>
                  <button disabled={busy || !password || !confirmed}>
                    {busy ? "Wird aktualisiert …" : "Aktualisierung ausführen"}
                  </button>
                </>
              )}
            </>
          )}
        </form>
      )}
    </Dialog>
  );
}

export function HostDrift({
  host,
  canOperate,
  refresh,
  openJob,
}: {
  host: Host;
  canOperate: boolean;
  refresh: () => void;
  openJob?: (id: string) => void;
}) {
  type Report = {
    state: string;
    detail: string;
    at?: string;
    backup_id?: string;
    unknown: number;
    files: { path: string; state: string }[];
  };
  const [report, setReport] = useState<Report>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [page, setPage] = useState(1);
  const [size, setSize] = useListPageSize("live-drift", 10);
  useEffect(() => {
    let valid = true;
    setReport(undefined);
    setError("");
    setPage(1);
    api<Report>(`operations/drift/${host.id}`)
      .then((value) => {
        if (valid) setReport(value);
      })
      .catch((e) => {
        if (valid) setError(e.message);
      });
    return () => {
      valid = false;
    };
  }, [host.id, host.last_probe]);
  async function check() {
    setBusy(true);
    try {
      const r = await api<Run>("operations/hosts", "POST", {
        request_id: crypto.randomUUID(),
        action: "drift",
        targets: [{ host_id: host.id, revision: host.config_revision }],
      });
      const target = r.targets[0];
      if (target.job_id) openJob?.(target.job_id);
      else setError(target.message || "Prüfung nicht gestartet");
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const titles: Record<string, string> = {
    unknown: "Noch kein belastbarer Vergleich",
    stale: "Liveprüfung veraltet",
    unchanged: "Keine Inhaltsänderung beobachtet",
    changed: "Ungesicherte Änderungen erkannt",
    partial: "Vergleich unvollständig",
  };
  return (
    <section className="operations-card drift-card">
      <div className="operations-heading">
        <ScanLine size={20} />
        <div>
          <h3>Livezustand gegenüber Sicherung</h3>
          <p>
            Lesender Vergleich im Konfigurationsumfang. Datastore-Inhalte
            bleiben ausgeschlossen.
          </p>
        </div>
        {canOperate && (
          <button className="secondary" disabled={busy} onClick={check}>
            Jetzt live prüfen
          </button>
        )}
      </div>
      {error && <p role="alert">{error}</p>}
      {!report && !error && <p role="status">Livevergleich wird geladen …</p>}
      {report && (
        <>
          <strong
            className={`state ${report.state === "changed" || report.state === "partial" ? "warning" : "muted"}`}
          >
            {titles[report.state] || report.state}
          </strong>
          <p>{report.detail}</p>
          <p className="muted">
            Automatik:{" "}
            {host.drift_minutes
              ? `alle ${host.drift_minutes} Minuten`
              : "ausgeschaltet"}{" "}
            · Referenz: {report.backup_id || "fehlt"} · Beobachtung:{" "}
            {date(report.at)}
          </p>
          {report.files.length > 0 && (
            <>
              <div className="drift-files">
                {report.files.slice((page - 1) * size, page * size).map((f) => (
                  <div key={f.path}>
                    <span className="state muted">
                      {
                        {
                          added: "Neu",
                          removed: "Entfernt",
                          changed: "Geändert",
                        }[f.state]
                      }
                    </span>
                    <code>{f.path}</code>
                  </div>
                ))}
              </div>
              <Pagination
                page={page}
                pageSize={size}
                total={report.files.length}
                onPageChange={setPage}
                onPageSizeChange={setSize}
                label="Liveänderungen"
              />
            </>
          )}
        </>
      )}
    </section>
  );
}
