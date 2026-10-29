import {
  Check,
  CircleAlert,
  Clock3,
  LoaderCircle,
  Pause,
  Terminal,
} from "lucide-react";
import { useEffect, useState } from "react";
import type { Job } from "../api";
import { date } from "../api";

export default function JobProgress({
  job,
  expanded = false,
}: {
  job: Job;
  expanded?: boolean;
}) {
  const [now, setNow] = useState(Date.now);
  const running = job.state === "running";
  useEffect(() => {
    if (!running) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [running]);
  const stages = (job.stages || []).filter(
    (stage) => stage.attempt === job.attempts,
  );
  const current = stages.at(-1);
  const measured =
    running &&
    !job.retry_at &&
    current?.state === "running" &&
    (current.total || 0) > 0;
  const title = job.retry_at
    ? "Verbindung erneut versuchen"
    : job.state === "queued"
      ? "Wartet auf einen freien Platz"
      : running
        ? current?.title || "Hostoperation läuft"
        : job.state === "successful"
          ? "Auftrag abgeschlossen"
          : job.state === "cancelled"
            ? "Ausführung abgebrochen"
            : job.state === "interrupted"
              ? "Ausführung unterbrochen"
              : "Auftrag benötigt Aufmerksamkeit";
  const description = job.retry_at
    ? job.message
    : job.state === "queued"
      ? "Startet automatisch, sobald ein Ausführungsplatz frei wird."
      : running
        ? current?.detail ||
          job.message ||
          "Für diesen Vorgang liegt noch kein genauerer Arbeitsschritt vor."
        : job.message ||
          "Das Ergebnis und die zugehörigen Nachweise stehen unten bereit.";
  const age = current?.started_at
    ? Math.max(0, Math.floor((now - Date.parse(current.started_at)) / 1000))
    : undefined;
  const Icon = job.retry_at
    ? Pause
    : job.state === "queued"
      ? Clock3
      : running
        ? LoaderCircle
        : job.state === "successful"
          ? Check
          : CircleAlert;
  return (
    <div
      className={"job-execution" + (expanded ? " is-expanded" : "")}
      data-job-state={job.state}
    >
      <section className="job-current" aria-label="Aktueller Arbeitsschritt">
        <div className="job-current-caption">
          <Terminal size={14} aria-hidden="true" />
          <span>
            {job.retry_at
              ? "Automatische Wiederholung"
              : running
                ? "Jetzt in Arbeit"
                : job.state === "queued"
                  ? "Warteschlange"
                  : "Ausführungsergebnis"}
          </span>
          {running && !job.retry_at && (
            <span className="job-live">
              <i />
              Live
            </span>
          )}
        </div>
        <div className="job-current-title">
          <Icon
            size={21}
            className={running && !job.retry_at ? "job-spinning" : ""}
            aria-hidden="true"
          />
          <strong>{title}</strong>
        </div>
        <p>{description}</p>
        {running && !job.retry_at ? (
          <>
            <div
              className={
                "job-progress-track" +
                (measured ? " is-measured" : " is-indeterminate")
              }
              role="progressbar"
              aria-label={current?.title || "Hostoperation"}
              aria-valuemin={0}
              aria-valuemax={measured ? current!.total : undefined}
              aria-valuenow={measured ? current!.completed || 0 : undefined}
              aria-valuetext={
                measured
                  ? `${current!.completed || 0} von ${current!.total} Einträgen geprüft`
                  : "In Arbeit; genauer Fortschritt noch nicht messbar"
              }
            >
              <span
                style={
                  measured
                    ? {
                        width: `${Math.min(100, ((current!.completed || 0) / current!.total!) * 100)}%`,
                      }
                    : undefined
                }
              />
            </div>
            <div className="job-progress-caption">
              <span>
                {measured
                  ? `${current!.completed || 0} / ${current!.total} Einträge geprüft`
                  : "Arbeitsschritt läuft · keine Prozentangabe verfügbar"}
              </span>
              {age !== undefined && Number.isFinite(age) && (
                <span>
                  Seit{" "}
                  {age < 60 ? `${age} Sek.` : `${Math.floor(age / 60)} Min.`}
                </span>
              )}
            </div>
          </>
        ) : job.retry_at ? (
          <p className="job-retry-time">
            Nächster Versuch: {date(job.retry_at)}
          </p>
        ) : null}
        {!running && current && job.state !== "successful" && (
          <p className="job-stopped-at">
            Letzter Arbeitsschritt: {current.title}
          </p>
        )}
      </section>
      {expanded && (
        <section className="job-stage-panel" aria-label="Beobachteter Ablauf">
          <h3>
            Arbeitsschritte <span>Versuch {job.attempts || "—"}</span>
          </h3>
          {stages.length ? (
            <ol className="job-stage-list">
              {stages.map((stage, index) => (
                <li
                  key={`${stage.attempt}-${index}`}
                  data-stage-state={stage.state}
                >
                  <span className="job-stage-marker" aria-hidden="true">
                    {stage.state === "successful" ? (
                      <Check size={13} />
                    ) : stage.state === "running" ? (
                      <span />
                    ) : (
                      <CircleAlert size={13} />
                    )}
                  </span>
                  <div>
                    <strong>{stage.title}</strong>
                    <small>
                      {stage.state === "successful"
                        ? "Erledigt"
                        : stage.state === "running"
                          ? "In Arbeit"
                          : stage.state === "cancelled"
                            ? "Abgebrochen"
                            : stage.state === "interrupted"
                              ? "Unterbrochen"
                              : "Nicht abgeschlossen"}
                      {stage.total
                        ? ` · ${stage.completed || 0}/${stage.total}`
                        : ""}
                    </small>
                  </div>
                </li>
              ))}
            </ol>
          ) : (
            <p className="field-hint">
              {job.state === "queued"
                ? "Die Arbeitsschritte erscheinen hier, sobald der Auftrag startet."
                : "Für diesen Auftrag wurden keine einzelnen Arbeitsschritte aufgezeichnet."}
            </p>
          )}
          {(job.stages || []).some(
            (stage) => stage.attempt !== job.attempts,
          ) && (
            <details className="job-previous-attempts">
              <summary>Frühere Versuche</summary>
              <ul>
                {job
                  .stages!.filter((stage) => stage.attempt !== job.attempts)
                  .map((stage, index) => (
                    <li key={index}>
                      Versuch {stage.attempt} · {stage.title} ·{" "}
                      {stage.state === "successful"
                        ? "Erledigt"
                        : "Nicht abgeschlossen"}
                    </li>
                  ))}
              </ul>
            </details>
          )}
        </section>
      )}
    </div>
  );
}
