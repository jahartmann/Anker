import { useEffect, useRef, useState } from "react";
import {
  CircleAlert,
  CircleCheck,
  Clock3,
  RefreshCw,
  WandSparkles,
} from "lucide-react";
import { api } from "../api";
import type { Status } from "../api";
import { HostName } from "../hostNames";
import { useListPageSize } from "../listPageSize";
import { Pagination } from "./shared";

type Source = { host_id: string; backup_id: string };
type Target = Source & {
  host_name: string;
  reasons: string[];
  state: string;
  message?: string;
  job_id?: string;
};
type Plan = { targets: Target[]; other_hosts: number };
type Request = {
  request_id: string;
  sources: Source[];
  include_paused: boolean;
};
type Run = { id: string; request: Request; targets: Target[] };
const reasonLabels: Record<string, string> = {
  systemd_path: "Gültiger systemd-Pfad wurde abgelehnt",
  lynx_optional: "Optionale Lynx-Erweiterung prüfen",
  powerman_command: "Powerman-Gerätebefehl wurde als Dateiverweis erkannt",
};

export default function BackupRemediation({
  status,
  actorID,
  canEdit,
  refresh,
  openJob,
  openBackup,
}: {
  status: Status;
  actorID: string;
  canEdit: boolean;
  refresh: () => void;
  openJob?: (id: string) => void;
  openBackup: (id: string) => void;
}) {
  const [plan, setPlan] = useState<Plan | null>(null),
    [run, setRun] = useState<Run | null>(null);
  const [error, setError] = useState(""),
    [loadError, setLoadError] = useState("");
  const [busy, setBusy] = useState(false),
    [includePaused, setIncludePaused] = useState(false);
  const [retry, setRetry] = useState(0),
    [expanded, setExpanded] = useState(false),
    [page, setPage] = useState(1);
  const [size, setSize] = useListPageSize("backup-remediation", 10);
  const request = useRef<Request | null>(null),
    sending = useRef(false);
  const storageKey = "anker.backup-remediation." + actorID;
  const evidenceKey = JSON.stringify([
    status.backups.map((b) => [
      b.id,
      b.status,
      b.verification_error,
      b.warnings?.length,
    ]),
    status.hosts.map((h) => [h.id, h.enabled]),
    status.jobs
      .filter((j) => ["queued", "running"].includes(j.state))
      .map((j) => [j.id, j.state]),
  ]);
  useEffect(() => {
    let closed = false;
    api<Plan>("backup-remediation")
      .then((value) => {
        if (!closed) {
          setPlan(value);
          setLoadError("");
        }
      })
      .catch((e) => {
        if (!closed) setLoadError((e as Error).message);
      });
    return () => {
      closed = true;
    };
  }, [evidenceKey, retry]);
  useEffect(() => {
    let closed = false;
    request.current = null;
    setRun(null);
    setError("");
    if (actorID)
      try {
        const stored = JSON.parse(
          sessionStorage.getItem(storageKey) || "null",
        ) as Request | null;
        if (
          stored &&
          typeof stored.request_id === "string" &&
          Array.isArray(stored.sources) &&
          stored.sources.length > 0 &&
          stored.sources.length <= 1000 &&
          stored.sources.every(
            (source) =>
              typeof source.host_id === "string" &&
              typeof source.backup_id === "string",
          )
        ) {
          request.current = stored;
          setIncludePaused(Boolean(stored.include_paused));
          api<Run>(
            "backup-remediation/" + encodeURIComponent(stored.request_id),
          )
            .then((value) => {
              if (!closed) {
                request.current = value.request;
                setIncludePaused(value.request.include_paused);
                setRun(value);
                setExpanded(true);
              }
            })
            .catch(() => {
              if (!closed)
                setError(
                  "Die frühere Startantwort fehlt. Den Sammelauftrag abgleichen, bevor erneut gestartet wird.",
                );
            });
        }
      } catch {
        /* An unavailable browser store must not block a new action. */
      }
    return () => {
      closed = true;
    };
  }, [storageKey, actorID]);
  const actionTargets = (plan?.targets || []).slice(0, 1000);
  const eligible = actionTargets
    .filter(
      (t) => t.state === "ready" || (includePaused && t.state === "paused"),
    )
    .slice(0, 1000);
  const targets = run?.targets || plan?.targets || [];
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(targets.length / size)),
  );
  const queued = (run?.targets || []).filter(
    (t) => t.state === "queued" && t.job_id,
  );
  const finished = queued.filter((t) =>
    status.jobs.some(
      (j) =>
        j.id === t.job_id &&
        ["successful", "failed", "cancelled", "interrupted"].includes(j.state),
    ),
  ).length;
  function outcome(target: Target) {
    const job = status.jobs.find((j) => j.id === target.job_id);
    const backup = job?.result_id
      ? status.backups.find((b) => b.id === job.result_id)
      : undefined;
    if (target.state === "queued") {
      if (!job)
        return {
          label: "Auftragsnachweis noch nicht verfügbar",
          tone: "pending",
        };
      if (job.state === "queued")
        return { label: "Eingereiht", tone: "pending" };
      if (job.state === "running")
        return {
          label: job.retry_at
            ? "Wartet auf erneute Verbindung"
            : job.stages?.at(-1)?.title || "Sicherung läuft",
          tone: "pending",
        };
      if (
        job.state === "successful" &&
        backup?.status === "successful" &&
        !backup.verification_error &&
        backup.verification_attempt?.state === "ok"
      )
        return { label: "Vollständig gesichert & geprüft", tone: "ok" };
      if (backup?.status === "partial")
        return { label: "Weitere Hinweise – Ergebnis prüfen", tone: "warning" };
      if (backup?.verification_error || backup?.status === "damaged")
        return { label: "Integritätsprüfung fehlgeschlagen", tone: "warning" };
      return {
        label:
          job.state === "successful"
            ? "Prüfnachweis noch nicht verfügbar"
            : "Nicht abgeschlossen – Auftrag prüfen",
        tone: job.state === "successful" ? "pending" : "warning",
      };
    }
    return {
      label:
        target.state === "ready"
          ? "Bereit für neue Erfassung"
          : target.state === "paused"
            ? "Pausiert"
            : target.state === "busy"
              ? "Auftrag bereits aktiv"
              : target.state === "connection_missing"
                ? "Gespeicherter Zugang unvollständig"
                : target.state === "error"
                  ? "Start nicht möglich"
                  : target.state === "pending"
                    ? "Start noch abzugleichen"
                    : "Übersprungen",
      tone: target.state === "error" ? "warning" : "pending",
    };
  }
  const good = queued.filter((t) => outcome(t).tone === "ok").length;
  const unresolved = queued.filter((t) => outcome(t).tone === "warning").length;
  async function start() {
    if (sending.current || !canEdit) return;
    if (!request.current) {
      request.current = {
        request_id: Array.from(
          crypto.getRandomValues(new Uint8Array(16)),
          (byte) => byte.toString(16).padStart(2, "0"),
        ).join(""),
        sources: actionTargets.map((t) => ({
          host_id: t.host_id,
          backup_id: t.backup_id,
        })),
        include_paused: includePaused,
      };
      if (!request.current.sources.length) {
        request.current = null;
        return;
      }
      if (actorID)
        try {
          sessionStorage.setItem(storageKey, JSON.stringify(request.current));
        } catch {
          /* Retry remains safe within the open view. */
        }
    }
    sending.current = true;
    setBusy(true);
    setError("");
    try {
      const value = await api<Run>(
        "backup-remediation",
        "POST",
        request.current,
      );
      setRun(value);
      setExpanded(true);
      setPage(1);
      refresh();
      setRetry((v) => v + 1);
    } catch (e) {
      setError(
        (e as Error).message +
          " Bereits eingereihte Aufträge werden beim Abgleichen wiedergefunden.",
      );
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }
  function dismiss() {
    setRun(null);
    request.current = null;
    setError("");
    setExpanded(false);
    if (actorID)
      try {
        sessionStorage.removeItem(storageKey);
      } catch {
        /* Optional browser state. */
      }
    setRetry((v) => v + 1);
  }
  if (!targets.length && !request.current && !loadError) return null;
  const needsReconcile = Boolean(
    request.current && (!run || run.targets.some((t) => t.state === "pending")),
  );
  return (
    <section
      className="backup-remediation"
      aria-label="Sammelkorrektur bekannter Sicherungsprobleme"
      aria-busy={busy}
    >
      <div className="remediation-heading">
        <span className="remediation-symbol">
          <WandSparkles size={21} aria-hidden="true" />
        </span>
        <div>
          <span className="eyebrow">
            Vorhandene Hostzugänge weiterverwenden
          </span>
          <h3>
            {run
              ? "Sammelkorrektur im Blick"
              : "Bekannte Sicherungsprobleme gesammelt beheben"}
          </h3>
        </div>
        <span className="remediation-count">{targets.length} Hosts</span>
      </div>
      {!run && (
        <p>
          Für die bekannten systemd-, Lynx- und Powerman-Hinweise:
          Konfigurationen neu erfassen und anschließend automatisch auf
          Integrität prüfen. Das gilt für alle hier erkannten betroffenen Hosts,
          unabhängig von den Listenfiltern. Hostprofile und Datastore-Inhalte
          bleiben erhalten.
        </p>
      )}
      {loadError && (
        <p className="form-error">
          Sammelprüfung nicht verfügbar: {loadError}{" "}
          <button
            className="text-button"
            onClick={() => setRetry((v) => v + 1)}
          >
            Erneut laden
          </button>
        </p>
      )}
      {!run && plan && (
        <div className="remediation-availability">
          <strong>{eligible.length} bereit</strong>
          {plan.targets.some((t) => t.state === "paused") && (
            <span>
              {plan.targets.filter((t) => t.state === "paused").length} pausiert
              {includePaused ? " · wird einbezogen" : ""}
            </span>
          )}
          {plan.targets.some((t) => t.state === "busy") && (
            <span>
              {plan.targets.filter((t) => t.state === "busy").length} bereits
              aktiv
            </span>
          )}
          {plan.targets.some((t) => t.state === "connection_missing") && (
            <span>
              {
                plan.targets.filter((t) => t.state === "connection_missing")
                  .length
              }{" "}
              mit unvollständigem Zugang
            </span>
          )}
        </div>
      )}
      {!run && plan?.targets.some((t) => t.state === "paused") && (
        <label className="remediation-paused">
          <input
            type="checkbox"
            checked={includePaused}
            disabled={!canEdit || busy || needsReconcile}
            onChange={(e) => setIncludePaused(e.target.checked)}
          />
          Auch Hosts mit pausiertem Zeitplan einmalig einbeziehen
        </label>
      )}
      {run && (
        <div
          className="remediation-summary"
          data-outcome={unresolved ? "warning" : "normal"}
          role="status"
        >
          {unresolved > 0 ? (
            <CircleAlert size={17} aria-hidden="true" />
          ) : queued.length > 0 && good === queued.length ? (
            <CircleCheck size={17} aria-hidden="true" />
          ) : (
            <Clock3 size={17} aria-hidden="true" />
          )}
          <strong>{good} vollständig gesichert & geprüft</strong>
          <span>
            {finished} von {queued.length} Aufträgen beendet
          </span>
          {unresolved > 0 && <span>{unresolved} mit offenen Hinweisen</span>}
        </div>
      )}
      {run && queued.length > 0 && (
        <progress
          className="remediation-progress"
          data-outcome={unresolved ? "warning" : "normal"}
          aria-label="Beendete Aufträge der Sammelkorrektur"
          value={finished}
          max={queued.length}
        />
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {!run && (plan?.targets.length || 0) > 1000 && (
        <p className="field-hint">
          Ein Sammellauf umfasst die ersten 1000 erkannten Hosts. Weitere Hosts
          bleiben für einen späteren Lauf sichtbar.
        </p>
      )}
      <div className="remediation-actions">
        {canEdit && (!run || needsReconcile) && (
          <button
            disabled={busy || (!needsReconcile && !eligible.length)}
            onClick={start}
          >
            <RefreshCw size={15} aria-hidden="true" />
            {busy
              ? "Wird eingereiht …"
              : needsReconcile
                ? "Start abgleichen"
                : `${eligible.length} Hosts neu sichern & prüfen`}
          </button>
        )}
        <button
          className="text-button"
          aria-expanded={expanded}
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? "Hostliste einklappen" : "Betroffene Hosts ansehen"}
        </button>
        {run && !needsReconcile && (
          <button className="text-button" disabled={busy} onClick={dismiss}>
            {finished === queued.length
              ? "Ergebnis schließen"
              : "Ansicht schließen"}
          </button>
        )}
      </div>
      {!canEdit && (
        <p className="field-hint">
          Die Sammelaktion benötigt Sicherungsberechtigung.
        </p>
      )}
      {expanded && (
        <div className="remediation-targets">
          <ul>
            {targets
              .slice((currentPage - 1) * size, currentPage * size)
              .map((target) => {
                const state = outcome(target);
                const job = status.jobs.find((j) => j.id === target.job_id);
                return (
                  <li key={target.host_id}>
                    <div>
                      <strong>
                        <HostName
                          name={
                            target.host_name ||
                            status.hosts.find((h) => h.id === target.host_id)
                              ?.name ||
                            target.host_id
                          }
                        />
                      </strong>
                      <small>
                        {target.reasons
                          .map((code) => reasonLabels[code] || code)
                          .join(" · ")}
                      </small>
                      {target.message && target.state !== "queued" && (
                        <small>{target.message}</small>
                      )}
                    </div>
                    <span className={"remediation-outcome " + state.tone}>
                      {state.label}
                    </span>
                    <div>
                      {target.job_id && openJob && (
                        <button
                          className="text-button"
                          onClick={() => openJob(target.job_id!)}
                        >
                          Auftrag ansehen
                        </button>
                      )}
                      {job?.result_id &&
                        status.backups.some((b) => b.id === job.result_id) && (
                          <button
                            className="text-button"
                            onClick={() => openBackup(job.result_id!)}
                          >
                            Sicherung ansehen
                          </button>
                        )}
                    </div>
                  </li>
                );
              })}
          </ul>
          {targets.length > 10 && (
            <Pagination
              page={currentPage}
              pageSize={size}
              total={targets.length}
              onPageChange={setPage}
              onPageSizeChange={setSize}
              label="Betroffene Hosts"
            />
          )}
        </div>
      )}
      <p className="remediation-boundary">
        Eine Korrektur gilt erst nach vollständiger neuer Sicherung und
        erfolgreicher Integritätsprüfung als bestätigt. Andere Fehler bleiben
        sichtbar; alte Sicherungen werden nicht umgeschrieben.
        {plan?.other_hosts
          ? ` ${plan.other_hosts} weitere Hosts haben andere Hinweise und sind nicht Teil dieser Aktion.`
          : ""}
      </p>
    </section>
  );
}
