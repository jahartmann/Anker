import { useEffect, useRef, useState } from "react";
import { api, preciseDate } from "../api";
import type { ClusterBackupSet, Host } from "../api";
import { Heading } from "./shared";
const states: Record<string, string> = {
  complete: "Vollständig · gemeinsame Inhalte gleich",
  incomplete: "Unvollständig",
  divergent: "Abweichend",
  capturing: "Erfassung läuft",
  queued: "Wartend",
  running: "Erfassung läuft",
  captured: "Erfasst",
  partial: "Pflichtlücken",
  not_admitted: "Nicht zugelassen",
  interrupted: "Unterbrochen",
  cancelled: "Abgebrochen",
  failed: "Fehlgeschlagen",
  unavailable: "Quelle nicht verfügbar",
  finalization_unknown: "Auftragsabschluss unbekannt",
  successful: "Kein Erfassungsnachweis",
};
export default function ClusterSets({
  hosts,
  canEdit,
  onBack,
}: {
  hosts: Host[];
  canEdit: boolean;
  onBack: () => void;
}) {
  const [sets, setSets] = useState<ClusterBackupSet[]>([]);
  const [selected, setSelected] = useState("");
  const [detail, setDetail] = useState<ClusterBackupSet>();
  const [cluster, setCluster] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [reload, setReload] = useState(0);
  const generation = useRef(0);
  const acting = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      generation.current++;
    };
  }, []);
  useEffect(() => {
    let active = true;
    setLoading(true);
    api<ClusterBackupSet[]>("cluster-sets")
      .then((v) => {
        if (active) setSets(v);
      })
      .catch((e) => {
        if (active) setError(e.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [reload]);
  useEffect(() => {
    const seq = ++generation.current;
    if (!selected) return;
    api<ClusterBackupSet>("cluster-sets/" + selected)
      .then((v) => {
        if (seq === generation.current) setDetail(v);
      })
      .catch((e) => {
        if (seq === generation.current) setError(e.message);
      });
    return () => {
      generation.current++;
    };
  }, [selected, reload]);
  useEffect(() => {
    if (
      !sets.some((v) => v.state === "capturing") &&
      detail?.state !== "capturing"
    )
      return;
    const timer = setTimeout(() => setReload((n) => n + 1), 2000);
    return () => clearTimeout(timer);
  }, [sets, detail]);
  function choose(id: string) {
    if (id === selected) return;
    generation.current++;
    setSelected(id);
    setDetail(undefined);
    setError("");
  }
  async function capture() {
    if (acting.current || !cluster) return;
    acting.current = true;
    setBusy(true);
    setError("");
    try {
      const v = await api<ClusterBackupSet>("cluster-sets", "POST", {
        cluster_id: cluster,
      });
      if (mounted.current) {
        choose(v.id);
        setReload((n) => n + 1);
      }
    } catch (e) {
      if (mounted.current) setError((e as Error).message);
    } finally {
      acting.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <div className="cluster-sets">
      <button className="back-button" onClick={onBack}>
        ← Zurück zu Sicherungen
      </button>
      <Heading
        title="Cluster-Sicherungssets"
        description="Koordinierte Sicherungen als Quellen für einen manuell geplanten Cluster-Wiederaufbau."
      />
      <div className="cluster-set-notice">
        <strong>Nur manuelle Quellplanung</strong>
        <p>
          Die Hosts werden innerhalb des eingestellten Auftragslimits
          zeitversetzt erfasst. Kein atomarer Clustersnapshot. Die Bewertung
          gilt für das Erfassungsfenster. Aktuelle Integrität, Quorum, Isolation
          und Wiederherstellbarkeit separat prüfen.
        </p>
      </div>
      {canEdit && (
        <section
          className="cluster-set-capture"
          aria-label="Sicherungsset erfassen"
        >
          <label>
            Cluster für die Erfassung
            <select
              value={cluster}
              disabled={busy}
              onChange={(e) => setCluster(e.target.value)}
            >
              <option value="">Cluster auswählen</option>
              {[...new Set(hosts.map((h) => h.cluster_id).filter(Boolean))]
                .sort()
                .map((c) => (
                  <option key={c}>{c}</option>
                ))}
            </select>
          </label>
          {cluster && (
            <p>
              Erwartete Hosts:{" "}
              {hosts
                .filter((h) => h.cluster_id === cluster)
                .map((h) => h.name)
                .join(", ")}
              . Bereits belegte Hosts bleiben als fehlend im Satz sichtbar.
            </p>
          )}
          <button disabled={busy || !cluster} onClick={capture}>
            {busy
              ? "Erfassung wird angefordert …"
              : "Neues Sicherungsset erfassen"}
          </button>
        </section>
      )}
      {error && (
        <p role="alert" className="form-error">
          {error}
        </p>
      )}
      <button
        className="secondary"
        onClick={() => {
          setError("");
          setReload((n) => n + 1);
        }}
        disabled={loading}
      >
        Liste aktualisieren
      </button>
      {!loading && sets.length === 0 && (
        <p>Noch keine Cluster-Sicherungssets erfasst.</p>
      )}
      <div className="cluster-set-layout">
        <section aria-label="Sicherungssets" className="cluster-set-list">
          {sets.map((v) => (
            <button
              className="secondary"
              key={v.id}
              aria-pressed={selected === v.id}
              onClick={() => choose(v.id)}
            >
              <strong>{v.cluster_id}</strong>
              <span>{states[v.state] || "Unbekannt"}</span>
              <small>{v.id}</small>
            </button>
          ))}
        </section>
        <section aria-live="polite" className="cluster-set-detail">
          {selected && !detail && <p>Sicherungsset wird geladen …</p>}
          {detail && (
            <>
              <h2>Sicherungsset {detail.id}</h2>
              <p
                className={
                  detail.state === "complete" ? "state ok" : "state warning"
                }
              >
                {states[detail.state] || "Unbekannt"}
              </p>
              <p>
                Erfassungsfenster:{" "}
                {detail.started_at
                  ? preciseDate(detail.started_at)
                  : "Noch unbekannt"}{" "}
                bis{" "}
                {detail.completed_at
                  ? preciseDate(detail.completed_at)
                  : "Offen"}
                . Startversatz:{" "}
                {detail.started_at ? `${detail.start_skew_ms} ms` : "unbekannt"}
                .
              </p>
              {detail.notes.map((note, i) => (
                <p key={i}>{note}</p>
              ))}
              <ul className="cluster-set-members">
                {detail.members.map((m) => (
                  <li key={m.host_id}>
                    <h3>{m.host_name}</h3>
                    <p>{states[m.state] || "Unbekannt"}</p>
                    <dl>
                      <dt>Tatsächliche Sicherung</dt>
                      <dd>{m.backup_id || "Kein Ergebnis dieser Erfassung"}</dd>
                      <dt>Auftrag</dt>
                      <dd>{m.job_id || "Nicht zugelassen"}</dd>
                      <dt>Erfassung</dt>
                      <dd>
                        {m.started_at ? preciseDate(m.started_at) : "Unbekannt"}{" "}
                        –{" "}
                        {m.completed_at ? preciseDate(m.completed_at) : "Offen"}
                      </dd>
                      <dt>Erfasste Mitgliedschaft</dt>
                      <dd>
                        {m.membership_known
                          ? m.captured_members?.join(", ") || "Nachweis erfasst"
                          : "Unbekannt"}
                      </dd>
                      <dt>Gemeinsames Material</dt>
                      <dd>
                        {m.shared_known
                          ? "Logische Prüfsumme erfasst"
                          : "Nachweis fehlt"}
                      </dd>
                    </dl>
                  </li>
                ))}
              </ul>
              <a
                className="button secondary"
                href={`/api/cluster-sets/${detail.id}/export`}
                download
              >
                Quellreferenzen exportieren
              </a>
              <p>
                Export enthält Referenzen und Grenzen, keine Sicherungsdateien
                und keine Ausführungsfreigabe. Einzelne Quellen und ihr Material
                separat prüfen und exportieren.
              </p>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
