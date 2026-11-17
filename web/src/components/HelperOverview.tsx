import { useEffect, useState } from "react";
import { api, date } from "../api";
import type { Host, HostIdentity } from "../api";
import { displayHostName, HostName } from "../hostNames";
import { Dialog, Field, Pagination } from "./shared";
import { ShieldCheck } from "lucide-react";
import { useListPageSize } from "../listPageSize";

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
export default function HelperOverview({
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
