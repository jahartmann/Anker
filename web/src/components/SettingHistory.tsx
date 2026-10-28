import SettingExplanation from "./SettingExplanation";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Fingerprint, ArrowRight, Clock3 } from "lucide-react";
import { api, preciseDate } from "../api";
import type { Backup } from "../api";
import { LoadError } from "./shared";
import {
  observationLabels,
  settingChoices,
  settingRuns,
  settingValue,
} from "../settingHistory";
import type {
  SettingHistoryResult,
  SettingObservation,
} from "../settingHistory";

export default function SettingHistory({
  hostID,
  backups,
  request,
}: {
  hostID: string;
  backups: Backup[];
  request?: { id: string; sequence: number };
}) {
  const [open, setOpen] = useState(false);
  const [setting, setSetting] = useState("ipv4-forwarding");
  const [reload, setReload] = useState(0);
  const [visible, setVisible] = useState(4);
  const [result, setResult] = useState<SettingHistoryResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const generation = useRef(0);
  const panel = useRef<HTMLDetailsElement>(null);
  const selector = useRef<HTMLSelectElement>(null);
  const sourceKey = JSON.stringify(
    backups.map((b) => [
      b.id,
      b.manifest_sha,
      b.created_at,
      b.status,
      b.verification_error,
    ]),
  );
  useEffect(() => {
    if (!request || !settingChoices.some((s) => s.id === request.id)) return;
    generation.current++;
    setResult(null);
    setSetting(request.id);
    setOpen(true);
    setReload((n) => n + 1);
  }, [request]);
  useLayoutEffect(() => {
    if (!open || !request) return;
    selector.current?.focus({ preventScroll: true });
    panel.current?.scrollIntoView({
      block: "start",
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "auto"
        : "smooth",
    });
  }, [open, request]);
  useEffect(() => {
    const current = ++generation.current;
    setResult(null);
    setError("");
    setVisible(4);
    setBusy(false);
    if (!open) return;
    setBusy(true);
    api<SettingHistoryResult>(
      `hosts/${encodeURIComponent(hostID)}/setting-history?setting=${encodeURIComponent(setting)}`,
    )
      .then((r) => {
        if (current === generation.current) setResult(r);
      })
      .catch((e) => {
        if (current === generation.current) setError((e as Error).message);
      })
      .finally(() => {
        if (current === generation.current) setBusy(false);
      });
    return () => {
      generation.current++;
    };
  }, [hostID, setting, sourceKey, open, reload]);
  async function more() {
    if (!result?.next || busy) return;
    const current = ++generation.current;
    setBusy(true);
    setError("");
    try {
      const r = await api<SettingHistoryResult>(
        `hosts/${encodeURIComponent(hostID)}/setting-history?setting=${encodeURIComponent(setting)}&before=${encodeURIComponent(result.next)}`,
      );
      if (current !== generation.current) return;
      const ids = new Set(result.observations.map((o) => o.backup_id));
      setResult({
        ...r,
        observations: [
          ...result.observations,
          ...r.observations.filter((o) => !ids.has(o.backup_id)),
        ],
      });
      setVisible((n) => n + 4);
    } catch (e) {
      if (current === generation.current) setError((e as Error).message);
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }
  const runs = settingRuns(result?.observations || []);
  const latest = runs[0];
  const observationTime = (o: SettingObservation) => preciseDate(o.created_at);
  return (
    <details
      ref={panel}
      className="setting-trace"
      data-surface-topic="catalog"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>
        <Fingerprint size={17} aria-hidden="true" />
        <span>Änderungsspur</span>
        <span className="setting-trace-summary">
          Eine Einstellung durch die Zeit verfolgen
        </span>
      </summary>
      {open && (
        <div className="setting-trace-content">
          <div className="setting-trace-heading">
            <div>
              <span className="eyebrow">Gesicherte Einstellungen</span>
              <h3>Seit wann ist das so?</h3>
            </div>
            <span className="setting-trace-badge">
              <Clock3 size={13} aria-hidden="true" /> Gesicherte Konfiguration
            </span>
          </div>
          <label className="setting-trace-picker">
            Einstellung verfolgen
            <select
              ref={selector}
              aria-label="Einstellung verfolgen"
              value={setting}
              onChange={(e) => {
                generation.current++;
                setResult(null);
                setError("");
                setSetting(e.target.value);
              }}
            >
              {settingChoices.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>
          <p className="setting-trace-note">
            Anker liest die Werte aus den gesicherten Dateien. Ob sie am Host
            wirksam sind, welche Standardwerte gelten oder welche Datei Vorrang
            hat, wird hier nicht geprüft.
          </p>
          {busy && !result && <p role="status">Werte werden geladen …</p>}
          {error && (
            <LoadError
              message={error}
              retry={() => setReload((n) => n + 1)}
              label="Spur erneut laden"
            />
          )}
          {result && (
            <>
              {result.undated > 0 && (
                <p className="notice warning">
                  {result.undated} Stände mit unplausiblem Datum sind zeitlich
                  nicht eingeordnet.
                </p>
              )}
              {latest ? (
                <div
                  className="setting-trace-current"
                  data-state={latest.state}
                  aria-live="polite"
                >
                  <span className="eyebrow">Neuester gesicherter Stand</span>
                  <strong>
                    {latest.state === "value"
                      ? settingValue(setting, latest.value)
                      : observationLabels[latest.state] || "Wert nicht bekannt"}
                  </strong>
                  <code>
                    {result.setting.key}
                    {latest.state === "value" ? ` = ${latest.value}` : ""}
                  </code>
                  {latest.state === "value" && (
                    <span>
                      Belegt ab {observationTime(latest.observations.at(-1)!)} ·{" "}
                      {latest.observations.length}{" "}
                      {latest.observations.length === 1
                        ? "Sicherung"
                        : "Sicherungen"}{" "}
                      mit diesem Wert
                    </span>
                  )}
                </div>
              ) : (
                <p>
                  Noch keine datierten Sicherungen für diese Node vorhanden.
                </p>
              )}
              <div
                className="setting-trace-events"
                aria-label="Verlauf dieser Einstellung"
              >
                {runs.slice(0, visible).map((run, index) => {
                  const oldest = run.observations.at(-1)!;
                  const newest = run.observations[0];
                  const previous = runs[index + 1];
                  const previousObs = previous?.observations[0];
                  const ordered =
                    previousObs &&
                    Date.parse(previousObs.created_at) <
                      Date.parse(oldest.created_at) &&
                    !oldest.order_uncertain &&
                    !previousObs.order_uncertain;
                  const changed =
                    run.state === "value" &&
                    previous?.state === "value" &&
                    ordered;
                  return (
                    <article
                      className={
                        "setting-trace-event" +
                        (run.state !== "value" ? " trace-gap" : "")
                      }
                      key={newest.backup_id}
                    >
                      <span className="setting-trace-dot" aria-hidden="true" />
                      <time dateTime={oldest.created_at}>
                        {observationTime(oldest)}
                      </time>
                      {run.state === "value" ? (
                        <>
                          <h4>
                            {changed
                              ? "Neuer Wert erfasst"
                              : previous && !ordered
                                ? "Zeitliche Zuordnung nicht eindeutig"
                                : previous
                                  ? "Wert nach einer Nachweislücke"
                                  : "Ältester geladener Wert"}
                          </h4>
                          <div className="setting-trace-transition">
                            {changed && (
                              <>
                                <span>
                                  {settingValue(setting, previous.value)}
                                </span>
                                <ArrowRight size={15} aria-hidden="true" />
                              </>
                            )}
                            <strong>{settingValue(setting, run.value)}</strong>
                          </div>
                          {changed && (
                            <p>
                              Änderung zwischen {observationTime(previousObs!)}{" "}
                              und diesem Stand erstmals belegt.
                            </p>
                          )}
                          {previous && !changed && (
                            <p>
                              {!ordered
                                ? "Zwischen diesen Werten lässt sich kein sicherer Änderungszeitpunkt zuordnen."
                                : "Der vorherige Wert ist nicht verlässlich bekannt."}
                            </p>
                          )}
                        </>
                      ) : (
                        <>
                          <h4>
                            {observationLabels[run.state] ||
                              "Wert nicht bekannt"}
                          </h4>
                          <p>
                            Diese Lücke wird nicht als unveränderter Wert
                            gewertet.
                          </p>
                        </>
                      )}
                      {changed && previousObs && (
                        <SettingExplanation
                          id={setting}
                          before={previousObs}
                          after={oldest}
                        />
                      )}
                      <span className="setting-trace-run-count">
                        {run.observations.length}{" "}
                        {run.observations.length === 1
                          ? "Sicherung"
                          : "Sicherungen"}
                        {run.observations.length > 1
                          ? ` · bis ${observationTime(newest)}`
                          : ""}
                      </span>
                      <details className="setting-trace-evidence">
                        <summary>Nachweise ansehen</summary>
                        {Array.from(
                          new Set(run.observations.flatMap((o) => o.sources)),
                        ).map((p) => (
                          <code key={p}>{p}</code>
                        ))}
                        <p>
                          Erster Nachweis in dieser Gruppe:{" "}
                          <code>{oldest.backup_id}</code>
                        </p>
                        <p>
                          Neuester Nachweis: <code>{newest.backup_id}</code>
                        </p>
                      </details>
                    </article>
                  );
                })}
              </div>
              {(runs.length > visible || result.next) && (
                <button
                  className="secondary"
                  disabled={busy}
                  onClick={() =>
                    runs.length > visible
                      ? setVisible((n) => n + 4)
                      : void more()
                  }
                >
                  {busy
                    ? "Weitere Stände werden geprüft …"
                    : runs.length > visible
                      ? "Weitere Zeitpunkte anzeigen"
                      : "Ältere Sicherungen verfolgen"}
                </button>
              )}
              {!!result.observations.length && (
                <p className="setting-trace-note">
                  {result.observations.length} Stände geprüft
                  {result.next
                    ? " · weitere Sicherungen vorhanden"
                    : " · Ende des vorhandenen datierten Verlaufs"}
                  . Der tatsächliche Änderungszeitpunkt zwischen Sicherungen ist
                  nicht bekannt.
                </p>
              )}
            </>
          )}
        </div>
      )}
    </details>
  );
}
