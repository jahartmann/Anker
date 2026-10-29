import { useEffect, useState } from "react";
import type { Host } from "../api";
import { api, date } from "../api";
import { HostName } from "../hostNames";
import { Dialog, Pagination } from "./shared";
import { useListPageSize } from "../listPageSize";
import { CircleAlert, ShieldCheck } from "lucide-react";
type Incident = {
  id: string;
  code: string;
  title: string;
  detail: string;
  action: string;
  path?: string;
  state: string;
  first_seen: string;
  last_seen: string;
  resolved_at?: string;
  acknowledged_by?: string;
  reopen_count: number;
  hosts: { id: string; name: string; muted: boolean }[];
};
const labels: Record<string, string> = {
  new: "Neu",
  acknowledged: "Zur Kenntnis genommen",
  in_progress: "Hostauftrag aktiv",
  reopened: "Erneut aufgetreten",
  resolved: "Durch neuen Nachweis erledigt",
  unmonitored: "Nicht mehr überwacht",
};
export default function IncidentCenter({
  revision,
  canOperate,
  openHost,
  allHosts,
  hosts,
  openJob,
  refresh,
}: {
  revision: string;
  canOperate: boolean;
  openHost: (id: string) => void;
  allHosts: () => void;
  hosts: Host[];
  openJob: (id: string) => void;
  refresh: () => void;
}) {
  const [loading, setLoading] = useState(true);
  const [repair, setRepair] = useState<{
    request_id: string;
    action: string;
    targets: { host_id: string; revision: string }[];
  }>();
  const [results, setResults] =
    useState<{ host_id: string; job_id?: string; message?: string }[]>();
  const [list, setList] = useState<Incident[]>([]);
  const [error, setError] = useState("");
  const [history, setHistory] = useState(false);
  const [page, setPage] = useState(1);
  const [size, setSize] = useListPageSize("incidents", 10);
  const [busy, setBusy] = useState("");
  useEffect(() => {
    let valid = true;
    api<Incident[]>("operations/incidents")
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
  }, [revision]);
  async function ack(v: Incident) {
    setBusy(v.id);
    try {
      const next = await api<Incident>(`operations/incidents/${v.id}`, "POST", {
        acknowledged: !v.acknowledged_by,
      });
      setList((items) => items.map((i) => (i.id === next.id ? next : i)));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy("");
    }
  }
  async function runRepair() {
    if (!repair) return;
    setBusy("repair");
    try {
      const result = await api<{
        targets: { host_id: string; job_id?: string; message?: string }[];
      }>("operations/hosts", "POST", repair);
      setResults(result.targets);
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy("");
    }
  }
  const visible = list.filter(
    (v) => history || !["resolved", "unmonitored"].includes(v.state),
  );
  const current = Math.min(page, Math.max(1, Math.ceil(visible.length / size)));
  return (
    <section className="incident-center" aria-label="Jetzt wichtig">
      <div className="section-heading">
        <div>
          <h3>Jetzt wichtig · Ursachen und Verlauf</h3>
          <span className="count-badge">
            {new Set(visible.flatMap((v) => v.hosts.map((h) => h.id))).size}
          </span>
          <p className="muted">
            Mehrere Hosts mit derselben Ursache erscheinen gemeinsam. Eine
            Bestätigung behebt keinen Fehler.
          </p>
        </div>
        <button className="text-button" onClick={allHosts}>
          Alle Hosts ansehen
        </button>
        <label className="check">
          <input
            type="checkbox"
            checked={history}
            onChange={(e) => {
              setHistory(e.target.checked);
              setPage(1);
            }}
          />
          Erledigte Ursachen anzeigen
        </label>
      </div>
      {hosts.length > 0 && !hosts.some((h) => h.enabled) && (
        <div className="notice">
          <h4>Automatische Sicherung pausiert</h4>
          <p>
            Vorhandene Ursachen bleiben sichtbar. Aktivieren erfolgt
            ausdrücklich über die Hostverwaltung.
          </p>
        </div>
      )}
      {error && (
        <p role="alert">Problemverlauf konnte nicht geladen werden: {error}</p>
      )}
      {loading && <p role="status">Ursachen werden geladen …</p>}
      {!loading && !error && !visible.length && (
        <div className="incident-clear">
          <ShieldCheck size={18} />
          <span>
            <h4>
              {!hosts.length
                ? "Noch keine Hosts eingerichtet"
                : "Keine offenen Ursachen im Problemverlauf"}
            </h4>
            Konfigurationssicherungen und Wiederherstellungsnachweise werden
            getrennt bewertet.
          </span>
        </div>
      )}
      <div className="incident-grid">
        {visible.slice((current - 1) * size, current * size).map((v) => (
          <article key={v.id} className="incident-card" data-state={v.state}>
            <div className="operations-heading">
              <CircleAlert size={18} />
              <h4>{v.title}</h4>
              <span className="count-badge">
                {v.hosts.length} {v.hosts.length === 1 ? "Host" : "Hosts"}
              </span>
            </div>
            <span
              className={`state ${v.state === "resolved" ? "success" : v.state === "in_progress" ? "active" : "warning"}`}
            >
              {labels[v.state]}
            </span>
            <p>{v.detail}</p>
            {v.path && <code>{v.path}</code>}
            <p className="incident-action">{v.action}</p>
            <details>
              <summary>Betroffene Hosts und Nachweise</summary>
              <IncidentHosts incident={v} openHost={openHost} />
              <p className="muted">
                Erstmals: {date(v.first_seen)} · Letzte Änderung:{" "}
                {date(v.last_seen)}
                {v.resolved_at && ` · Erledigt: ${date(v.resolved_at)}`}
                {v.reopen_count > 0 &&
                  ` · ${v.reopen_count}× erneut aufgetreten`}
              </p>
            </details>
            {canOperate && !["resolved", "unmonitored"].includes(v.state) && (
              <>
                <button
                  className="secondary"
                  onClick={() => {
                    setResults(undefined);
                    setRepair({
                      request_id: crypto.randomUUID(),
                      action: "backup",
                      targets: v.hosts
                        .map((h) => hosts.find((host) => host.id === h.id))
                        .filter((h): h is Host => !!h && !!h.config_revision)
                        .map((h) => ({
                          host_id: h.id,
                          revision: h.config_revision!,
                        })),
                    });
                  }}
                >
                  Betroffene Hosts sichern und prüfen
                </button>
                <button
                  className="text-button"
                  disabled={busy === v.id}
                  onClick={() => ack(v)}
                >
                  {v.acknowledged_by
                    ? "Bestätigung zurücknehmen"
                    : "Zur Kenntnis nehmen"}
                </button>
              </>
            )}
          </article>
        ))}
      </div>
      {visible.length > size && (
        <Pagination
          page={current}
          pageSize={size}
          total={visible.length}
          onPageChange={setPage}
          onPageSizeChange={setSize}
          label="Ursachen"
        />
      )}
      {repair && (
        <Dialog
          title="Betroffene Hosts sichern und prüfen"
          onClose={() => {
            if (!busy) setRepair(undefined);
          }}
        >
          <p>
            Für {repair.targets.length} Hosts wird eine neue
            Konfigurationssicherung mit anschließender Integritätsprüfung
            gestartet. Bisherige Stände und Hostprofile bleiben erhalten.
          </p>
          <ul className="selection-review">
            {repair.targets.map((t) => (
              <li key={t.host_id}>
                <HostName
                  name={
                    hosts.find((h) => h.id === t.host_id)?.name || t.host_id
                  }
                />
              </li>
            ))}
          </ul>
          {error && <p role="alert">{error}</p>}
          {results ? (
            <div className="operation-results">
              {results.map((t) => (
                <div key={t.host_id}>
                  <HostName
                    name={
                      hosts.find((h) => h.id === t.host_id)?.name || t.host_id
                    }
                  />
                  {t.job_id ? (
                    <button
                      className="text-button"
                      onClick={() => openJob(t.job_id!)}
                    >
                      Auftrag ansehen
                    </button>
                  ) : (
                    <span>{t.message}</span>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <button
              disabled={!!busy || !repair.targets.length}
              onClick={runRepair}
            >
              {busy ? "Wird gestartet …" : "Neue Sicherungen starten"}
            </button>
          )}
        </Dialog>
      )}
    </section>
  );
}

function IncidentHosts({
  incident,
  openHost,
}: {
  incident: Incident;
  openHost: (id: string) => void;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [size, setSize] = useListPageSize("incident-hosts", 5);
  const hosts = incident.hosts.filter((h) =>
    h.name.toLowerCase().includes(query.toLowerCase()),
  );
  const current = Math.min(page, Math.max(1, Math.ceil(hosts.length / size)));
  return (
    <>
      {incident.hosts.length > 5 && (
        <input
          aria-label={`${incident.title}: Hosts durchsuchen`}
          placeholder="Betroffenen Host suchen"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(1);
          }}
        />
      )}
      <div className="incident-hosts">
        {hosts.slice((current - 1) * size, current * size).map((h) => (
          <button
            className="secondary"
            key={h.id}
            aria-label={`Host ${h.name} prüfen`}
            onClick={() => openHost(h.id)}
          >
            <HostName name={h.name} />
            {h.muted && <small>Automatik pausiert / Wartung</small>}
          </button>
        ))}
      </div>
      {incident.hosts.length > 5 && (
        <Pagination
          page={current}
          pageSize={size}
          total={hosts.length}
          onPageChange={setPage}
          onPageSizeChange={setSize}
          label="Betroffene Hosts"
        />
      )}
    </>
  );
}
