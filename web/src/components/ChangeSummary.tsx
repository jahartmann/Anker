import { useEffect, useState } from "react";
import { GitCompareArrows } from "lucide-react";
import { api, preciseDate } from "../api";
import type { Backup, FileDiff } from "../api";
import { recoverySource, usableRecoverySource } from "../hostHealth";

export function changeArea(path: string) {
  path = "/" + path.replace(/^\/+/, "");
  if (
    /^\/etc\/(network(?:\/|$)|pve\/sdn(?:\/|$)|hosts$|hostname$|resolv\.conf$)/.test(
      path,
    )
  )
    return "Netzwerk";
  if (
    /^\/etc\/(pve\/storage\.cfg$|proxmox-backup\/datastore\.cfg$|fstab$|crypttab$|lvm(?:\/|$)|zfs(?:\/|$))/.test(
      path,
    )
  )
    return "Storage";
  if (
    /^\/etc\/(systemd(?:\/|$)|ssh(?:\/|$)|pve\/datacenter\.cfg$|default(?:\/|$)|modules(?:$|\.)|sysctl(?:$|\.))/.test(
      path,
    )
  )
    return "System";
  return "Weitere Konfiguration";
}
export default function ChangeSummary({
  backups,
  openChanges,
  compact = false,
  onHistory,
}: {
  backups: Backup[];
  compact?: boolean;
  onHistory?: () => void;
  openChanges: (from: string, to: string) => void;
}) {
  const eligible = backups.filter((b) => usableRecoverySource(b));
  const after = recoverySource(eligible);
  const before =
    after &&
    recoverySource(
      eligible.filter(
        (b) =>
          b.id !== after.id &&
          Date.parse(b.created_at) < Date.parse(after.created_at),
      ),
    );
  const [result, setResult] = useState<{
    key: string;
    rows: FileDiff[] | null;
    error: string;
  } | null>(null);
  const [retry, setRetry] = useState(0);
  // Same-ID manifest replacement must invalidate a previous response as well.
  const sourceKey = JSON.stringify([
    before?.id,
    before?.manifest_sha,
    after?.id,
    after?.manifest_sha,
    retry,
  ]);
  const rows = result?.key === sourceKey ? result.rows : null;
  const error = result?.key === sourceKey ? result.error : "";
  useEffect(() => {
    let active = true;
    setResult(null);
    if (before && after)
      api<FileDiff[]>("backups/compare", "POST", {
        from: before.id,
        to: after.id,
      })
        .then((result) => {
          if (active) setResult({ key: sourceKey, rows: result, error: "" });
        })
        .catch((e) => {
          if (active)
            setResult({
              key: sourceKey,
              rows: null,
              error: (e as Error).message,
            });
        });
    return () => {
      active = false;
    };
  }, [sourceKey, retry]);
  const significant = rows?.filter((r) => !r.mtime_only) || [];
  const added = significant.filter((r) => r.change === "added").length;
  const removed = significant.filter((r) => r.change === "removed").length;
  const changed = significant.filter((r) => r.change === "changed").length;
  return (
    <section
      className={
        "host-workspace change-summary" +
        (compact ? " change-summary-compact" : "")
      }
      aria-label="Änderungszusammenfassung"
      data-topic="changes"
    >
      <span className="eyebrow">
        <GitCompareArrows size={14} aria-hidden="true" /> Seit dem vorherigen
        Stand
      </span>
      <h3>
        {!before || !after
          ? "Noch kein Vergleich möglich"
          : error
            ? "Vergleich nicht verfügbar"
            : rows === null
              ? "Änderungen werden geprüft …"
              : significant.length
                ? `${significant.length} ${significant.length === 1 ? "Datei mit Änderungen" : "Dateien mit Änderungen"}`
                : "Keine wesentlichen Änderungen"}
      </h3>
      {!before || !after ? (
        <p>
          Für den Vergleich brauchst du zwei vollständige Sicherungen mit
          gültigem Datum und ohne bekannte Prüfsummenfehler.
        </p>
      ) : (
        <>
          {!compact && (
            <p>
              Die zwei letzten passenden Sicherungen im Vergleich: Dateiinhalte
              und Metadaten.
            </p>
          )}
          {error ? (
            <>
              <p role="alert">{error}</p>
              <button
                className="secondary"
                onClick={() => setRetry((r) => r + 1)}
              >
                Vergleich erneut laden
              </button>
            </>
          ) : (
            rows !== null && (
              <>
                {rows.length > 0 && (
                  <p className="change-totals">
                    {[
                      added ? `${added} hinzugefügt` : "",
                      removed ? `${removed} entfernt` : "",
                      changed ? `${changed} geändert` : "",
                      rows.some((r) => r.mtime_only)
                        ? `${rows.filter((r) => r.mtime_only).length} nur Zeitstempel`
                        : "",
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                )}
                <div className="change-areas">
                  {["Netzwerk", "Storage", "System", "Weitere Konfiguration"]
                    .filter(
                      (area) =>
                        !compact ||
                        significant.some((r) => changeArea(r.path) === area),
                    )
                    .map((area) => {
                      const count = significant.filter(
                        (r) => changeArea(r.path) === area,
                      ).length;
                      return (
                        <span
                          key={area}
                          className={count ? "area-changed" : "area-quiet"}
                        >
                          <strong>{area}</strong>
                          {count
                            ? `${count} ${count === 1 ? "Änderung" : "Änderungen"}`
                            : "Keine Änderung erfasst"}
                        </span>
                      );
                    })}
                </div>
              </>
            )
          )}
          <details className="change-sources">
            <summary>Verglichene Stände</summary>
            <p>
              Vorher: {preciseDate(before.created_at)} ·{" "}
              <code>{before.id}</code>
            </p>
            <p>
              Nachher: {preciseDate(after.created_at)} · <code>{after.id}</code>
            </p>
            <p>
              Der Vergleich zeigt nur die gesicherten Dateien. Spätere
              Änderungen am Host sind hier nicht enthalten.
            </p>
          </details>
          <button
            className="text-button"
            onClick={() => openChanges(before.id, after.id)}
          >
            Vergleich öffnen
          </button>
        </>
      )}
      {onHistory && (
        <button className="text-button" onClick={onHistory}>
          Konfigurationsreise öffnen
        </button>
      )}
    </section>
  );
}
