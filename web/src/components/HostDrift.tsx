import { useEffect, useState } from "react";
import { api, date, interval } from "../api";
import type { Backup, Host } from "../api";
import { Pagination } from "./shared";
import { ScanLine } from "lucide-react";
import { useListPageSize } from "../listPageSize";

type Run = { targets: { job_id?: string; message: string }[] };

export default function HostDrift({
  host,
  backups = [],
  openBackup,
  canOperate,
  refresh,
  openJob,
}: {
  host: Host;
  backups?: Backup[];
  openBackup?: (id: string) => void;
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
  const reference = backups.find((b) => b.id === report?.backup_id);
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
              ? interval(host.drift_minutes)
              : "ausgeschaltet"}{" "}
            · Referenz:{" "}
            {reference && openBackup ? (
              <button
                type="button"
                className="text-button"
                onClick={() => openBackup(reference.id)}
              >
                Sicherung vom {date(reference.created_at)}
              </button>
            ) : (
              report.backup_id || "fehlt"
            )}{" "}
            · Beobachtung: {date(report.at)}
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
