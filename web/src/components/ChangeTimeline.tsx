import ChangeHighlights from "./ChangeHighlights";
import { useEffect, useMemo, useState } from "react";
import { Clock3, GitCompareArrows, Check, CircleDot } from "lucide-react";
import { api, preciseDate, calendarDate } from "../api";
import type { Backup, FileDiff } from "../api";
import { usableRecoverySource } from "../hostHealth";
import { changeArea } from "./ChangeSummary";
import { Pagination } from "./shared";

function TimelineEvent({
  before,
  after,
  onCompare,
  latest = false,
}: {
  latest?: boolean;
  before?: Backup;
  after: Backup;
  onCompare: (from: string, to: string) => void;
}) {
  const [retry, setRetry] = useState(0);
  const key = JSON.stringify([
    before?.id,
    before?.manifest_sha,
    before?.status,
    before?.verification_error,
    after.id,
    after.manifest_sha,
    after.status,
    after.verification_error,
    retry,
  ]);
  const valid =
    before &&
    before.host_id === after.host_id &&
    usableRecoverySource(before) &&
    usableRecoverySource(after);
  const [result, setResult] = useState<{
    key: string;
    rows?: FileDiff[];
    error?: string;
  } | null>(null);
  useEffect(() => {
    let active = true;
    if (valid && before)
      api<FileDiff[]>("backups/compare", "POST", {
        from: before.id,
        to: after.id,
      })
        .then((rows) => {
          if (active) setResult({ key, rows });
        })
        .catch((e) => {
          if (active) setResult({ key, error: (e as Error).message });
        });
    return () => {
      active = false;
    };
  }, [key, !!valid]);
  const current = result?.key === key ? result : null;
  const significant = current?.rows?.filter((r) => !r.mtime_only);
  const areas = Array.from(
    new Set(significant?.map((r) => changeArea(r.path))),
  );
  const title = !before
    ? "Erster erfasster Stand"
    : !valid
      ? "Vergleich nicht belastbar"
      : current?.error
        ? "Vergleich nicht verfügbar"
        : !significant
          ? "Änderungen werden geprüft …"
          : significant.length
            ? `${significant.length} ${significant.length === 1 ? "Datei mit Änderungen" : "Dateien mit Änderungen"}`
            : "Keine wesentlichen Änderungen";
  return (
    <article
      className={
        "timeline-event" +
        (significant?.length ? " timeline-changed" : "") +
        (!before ? " timeline-baseline" : "") +
        (current?.error || (!valid && before) ? " timeline-warning" : "")
      }
    >
      <div className="timeline-date-rail">
        <time dateTime={after.created_at} title={preciseDate(after.created_at)}>
          <strong>{calendarDate(after.created_at).dayLabel}</strong>
          <span>
            {calendarDate(after.created_at).monthKey.slice(0, 4)} ·{" "}
            {preciseDate(after.created_at).split(", ").at(-1)}
          </span>
        </time>
        {latest && <span className="timeline-latest">Neuester Stand</span>}
      </div>
      <span className="timeline-marker" aria-hidden="true">
        {!before ? (
          <CircleDot size={14} />
        ) : significant?.length ? (
          <GitCompareArrows size={14} />
        ) : significant ? (
          <Check size={14} />
        ) : (
          <Clock3 size={14} />
        )}
      </span>
      <div className="timeline-event-body">
        <div className="timeline-card-heading">
          <h4>{title}</h4>
          {significant && before && valid && (
            <button
              className="text-button timeline-open"
              aria-label={`Änderungen zum Stand ${after.id} öffnen`}
              onClick={() => onCompare(before.id, after.id)}
            >
              Details <span aria-hidden="true">↗</span>
            </button>
          )}
        </div>
        {!before ? (
          <p>
            Ausgangspunkt im vorhandenen Verlauf; frühere Änderungen sind nicht
            bekannt.
          </p>
        ) : !valid ? (
          <p>
            Mindestens ein Stand ist unvollständig oder hat offene
            Integritätsfehler. Daraus wird keine Entwarnung abgeleitet.
          </p>
        ) : current?.error ? (
          <>
            <p role="alert">{current.error}</p>
            <button
              className="text-button"
              onClick={() => setRetry((n) => n + 1)}
            >
              Diesen Vergleich erneut laden
            </button>
          </>
        ) : (
          significant && (
            <>
              <div className="timeline-areas">
                {areas.map((area) => (
                  <span key={area}>{area}</span>
                ))}
              </div>
              <ChangeHighlights rows={current?.rows || []} />
              <p className="timeline-counts">
                {(
                  [
                    ["added", "hinzugefügt"],
                    ["removed", "entfernt"],
                    ["changed", "geändert"],
                  ] as const
                )
                  .map(([kind, label]) => {
                    const count = significant.filter(
                      (r) => r.change === kind,
                    ).length;
                    return count ? `${count} ${label}` : "";
                  })
                  .filter(Boolean)
                  .join(" · ")}
                {current?.rows?.some((r) => r.mtime_only)
                  ? `${significant.length ? " · " : ""}Zeitstempel aktualisiert`
                  : ""}
              </p>
            </>
          )
        )}
        <details className="timeline-source">
          <summary>Stände zu diesem Zeitpunkt</summary>
          <p>
            Nachher: <code>{after.id}</code>
          </p>
          {before && (
            <p>
              Vorher: {preciseDate(before.created_at)} ·{" "}
              <code>{before.id}</code>
            </p>
          )}
        </details>
      </div>
    </article>
  );
}
export default function ChangeTimeline({
  backups,
  onCompare,
  initiallyOpen = false,
}: {
  backups: Backup[];
  onCompare: (from: string, to: string) => void;
  initiallyOpen?: boolean;
}) {
  const [open, setOpen] = useState(initiallyOpen);
  const [page, setPage] = useState(1);
  const sorted = useMemo(
    () =>
      backups
        .filter(
          (b) =>
            Number.isFinite(Date.parse(b.created_at)) &&
            Date.parse(b.created_at) <= Date.now() + 300000,
        )
        .slice()
        .sort(
          (a, b) =>
            Date.parse(b.created_at) - Date.parse(a.created_at) ||
            a.id.localeCompare(b.id),
        ),
    [backups],
  );
  const current = Math.min(page, Math.max(1, Math.ceil(sorted.length / 4)));
  return (
    <details
      className="change-timeline"
      data-surface-topic="activity"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>
        <Clock3 size={16} aria-hidden="true" />
        <span>Änderungsverlauf dieser Node</span>
        <span className="count-badge">{sorted.length} Stände</span>
      </summary>
      {open && (
        <div className="timeline-content">
          <div className="timeline-intro">
            <p>Die Konfiguration deiner Node im Zeitverlauf.</p>
            <details>
              <summary>So liest du den Verlauf</summary>
              <p>
                Neueste Zeitpunkte zuerst. Jeder Vergleich bezieht sich auf den
                vorherigen erfassten Stand derselben Node. Der genaue Zeitpunkt
                einer Änderung zwischen zwei Sicherungen ist nicht bekannt.
              </p>
            </details>
          </div>
          {sorted.length !== backups.length && (
            <p className="notice warning">
              Stände mit unplausiblem Datum sind hier nicht eingeordnet und
              bleiben im Sicherungsverlauf erreichbar.
            </p>
          )}
          {sorted.slice((current - 1) * 4, current * 4).map((after, offset) => {
            const index = (current - 1) * 4 + offset;
            const before = sorted
              .slice(index + 1)
              .find(
                (b) =>
                  b.host_id === after.host_id &&
                  Date.parse(b.created_at) < Date.parse(after.created_at),
              );
            return (
              <TimelineEvent
                key={after.id}
                latest={index === 0}
                before={before}
                after={after}
                onCompare={(from, to) => {
                  onCompare(from, to);
                }}
              />
            );
          })}
          {!sorted.length && (
            <p>Noch keine datierten Sicherungsstände vorhanden.</p>
          )}
          <Pagination
            page={current}
            pageSize={4}
            total={sorted.length}
            onPageChange={setPage}
            label="Änderungszeitpunkte"
          />
        </div>
      )}
    </details>
  );
}
