import { useEffect, useState } from "react";
import { api, date } from "../api";
import type { Backup, BackupReadiness } from "../api";
import { LoadError } from "./shared";

const stateLabels: Record<string, string> = {
  ok: "Vorhanden",
  missing: "Fehlt",
  unknown: "Unbekannt",
  failed: "Fehlgeschlagen",
  stale: "Veraltet",
  captured: "Erfasst",
};

export default function BackupReadiness({
  backup,
  compact = false,
}: {
  backup?: Backup;
  compact?: boolean;
}) {
  const [readiness, setReadiness] = useState<{
    key: string;
    value: BackupReadiness;
  } | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [retry, setRetry] = useState(0);
  const sourceKey = JSON.stringify([
    backup?.id,
    backup?.status,
    backup?.verified_at,
    backup?.verification_error,
    backup?.manifest_sha,
    backup?.verification_attempt,
    backup?.copy_verification,
    backup?.evidence_revision,
  ]);
  const currentReadiness =
    readiness?.key === sourceKey ? readiness.value : null;
  useEffect(() => {
    let active = true;
    setReadiness(null);
    setError("");
    if (!backup) {
      setLoading(false);
      return;
    }
    setLoading(true);
    api<BackupReadiness>(`backups/${encodeURIComponent(backup.id)}/readiness`)
      .then((result) => {
        if (active && result.backup_id === backup.id && result.version === 1)
          setReadiness({ key: sourceKey, value: result });
        else if (active)
          setError("Die Prüfergebnisse gehören nicht zu dieser Sicherung.");
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
  }, [sourceKey, retry]);
  if (!backup)
    return (
      <section className="readiness" aria-label="Prüfergebnisse">
        <h3>Prüfergebnisse</h3>
        <p className="muted">Noch keine Sicherung für diese Ansicht.</p>
      </section>
    );
  return (
    <section className="readiness detail-section" aria-label="Prüfergebnisse">
      <h3>Prüfergebnisse</h3>
      <p className="field-hint">
        Quelle: {backup.host_name} · {date(backup.created_at)} ·{" "}
        <code>{backup.id}</code>. Die Ergebnisse gelten nur für diese Sicherung.
      </p>
      {loading && <p role="status">Prüfergebnisse werden geladen …</p>}
      {error && (
        <LoadError message={error} retry={() => setRetry((n) => n + 1)} />
      )}
      {currentReadiness && (
        <>
          <div className="readiness-items">
            {currentReadiness.items.map((item) => (
              <article key={item.id} className="readiness-item">
                <div>
                  <strong>{item.title}</strong>
                  <span className={`readiness-state ${item.state}`}>
                    {stateLabels[item.state] || item.state}
                  </span>
                </div>
                <p>{item.detail}</p>
                <p className="field-hint">
                  Herkunft: {item.provenance || "gespeicherter Stand"} ·{" "}
                  {item.at
                    ? `Stand/Versuch: ${date(item.at)}`
                    : "Kein datierter Nachweis"}
                </p>
                {item.next_action && (
                  <p className="readiness-action">
                    Nächster Schritt: {item.next_action}
                  </p>
                )}
              </article>
            ))}
          </div>
          {compact && (
            <div className="readiness-lists">
              <section aria-label="Sicherungsabdeckung">
                <h4>Abdeckung</h4>
                {currentReadiness.coverage.length ? (
                  <>
                    <p>
                      {
                        currentReadiness.coverage.filter(
                          (c) => c.state === "captured",
                        ).length
                      }{" "}
                      erfasste ·{" "}
                      {
                        currentReadiness.coverage.filter(
                          (c) => c.state === "missing",
                        ).length
                      }{" "}
                      fehlende Pfade
                    </p>
                    <ul>
                      {currentReadiness.coverage
                        .filter((c) => c.state === "missing")
                        .slice(0, 3)
                        .map((c, i) => (
                          <li key={`${c.path}-${i}`}>
                            <code>{c.path}</code> · {c.reason}
                          </li>
                        ))}
                    </ul>
                  </>
                ) : (
                  <p>Abdeckung unbekannt: keine gespeicherten Angaben.</p>
                )}
              </section>
              <section aria-label="Abhängigkeiten">
                <h4>Abhängigkeiten</h4>
                {currentReadiness.dependencies.length ? (
                  <>
                    <p>
                      {
                        currentReadiness.dependencies.filter(
                          (d) => d.state === "missing",
                        ).length
                      }{" "}
                      fehlende ·{" "}
                      {
                        currentReadiness.dependencies.filter(
                          (d) => d.state === "unknown",
                        ).length
                      }{" "}
                      ungeklärte Verweise
                    </p>
                    <ul>
                      {currentReadiness.dependencies
                        .filter((d) => d.state !== "captured")
                        .slice(0, 3)
                        .map((d, i) => (
                          <li key={`${d.path}-${i}`}>
                            <code>{d.path}</code> · {stateLabels[d.state]}
                          </li>
                        ))}
                    </ul>
                  </>
                ) : (
                  <p>Abhängigkeiten unbekannt: keine gespeicherten Angaben.</p>
                )}
              </section>
            </div>
          )}
          {!compact && (
            <div className="readiness-lists">
              <section aria-label="Sicherungsabdeckung">
                <h4>Abdeckung und Wurzeln</h4>
                {currentReadiness.coverage.length ? (
                  <ul>
                    {currentReadiness.coverage.map((c, i) => (
                      <li key={`${c.path}-${i}`}>
                        <code>{c.path}</code> · {stateLabels[c.state]} ·{" "}
                        {c.reason} · Herkunft:{" "}
                        {c.authority === "collector"
                          ? "Erfassungsprofil"
                          : "Anforderung"}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="muted">
                    Keine gespeicherten Abdeckungsangaben. Abdeckung unbekannt.
                  </p>
                )}
              </section>
              <section aria-label="Abhängigkeiten">
                <h4>Abhängigkeiten</h4>
                {currentReadiness.dependencies.length ? (
                  <ul>
                    {currentReadiness.dependencies.map((d, i) => (
                      <li key={`${d.path}-${i}`}>
                        <code>{d.path}</code> · {stateLabels[d.state]}
                        {d.source ? ` · Verweis aus ${d.source}` : ""}
                        {d.reason ? ` · ${d.reason}` : ""}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="muted">
                    Keine gespeicherten Abhängigkeitsangaben. Abhängigkeiten
                    unbekannt.
                  </p>
                )}
              </section>
            </div>
          )}
        </>
      )}
    </section>
  );
}
