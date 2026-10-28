import SettingExplanation from "./SettingExplanation";
import { useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Compass,
  Network,
  HardDrive,
  Settings2,
  Files,
  Fingerprint,
} from "lucide-react";
import { api, preciseDate } from "../api";
import type { Backup, FileDiff } from "../api";
import { usableRecoverySource } from "../hostHealth";
import { observationLabels, settingValue } from "../settingHistory";
import type { SettingSnapshotResult } from "../settingHistory";
import { changeArea } from "./ChangeSummary";
import { changeSubject } from "../changeHighlights";
import { Empty, LoadError } from "./shared";

const settingArea = (id: string) =>
  id === "swappiness" ? "System" : "Netzwerk";
const areas = [
  { name: "Netzwerk", Icon: Network },
  { name: "Storage", Icon: HardDrive },
  { name: "System", Icon: Settings2 },
  { name: "Weitere Konfiguration", Icon: Files },
];
interface JourneyResult {
  key: string;
  after: SettingSnapshotResult | null;
  before: SettingSnapshotResult | null;
  rows: FileDiff[] | null;
  errors: string[];
}
export default function ConfigurationJourney({
  hostID,
  backups,
  onCompare,
  onTrackSetting,
  onOpenBackup,
}: {
  hostID: string;
  backups: Backup[];
  onCompare: (from: string, to: string) => void;
  onTrackSetting: (id: string) => void;
  onOpenBackup: (id: string) => void;
}) {
  const [chosen, setChosen] = useState<string | null>(null);
  const [area, setArea] = useState("");
  const [retry, setRetry] = useState(0);
  const [result, setResult] = useState<JourneyResult | null>(null);
  const [showAllSettings, setShowAllSettings] = useState(false);
  const sorted = useMemo(
    () =>
      backups
        .filter(
          (b) =>
            b.host_id === hostID &&
            Number.isFinite(Date.parse(b.created_at)) &&
            Date.parse(b.created_at) <= Date.now() + 300000,
        )
        .slice()
        .sort(
          (a, b) =>
            Date.parse(a.created_at) - Date.parse(b.created_at) ||
            a.id.localeCompare(b.id),
        ),
    [hostID, backups],
  );
  const index = chosen
    ? Math.max(
        0,
        sorted.findIndex((b) => b.id === chosen),
      )
    : sorted.length - 1;
  const after = sorted[index],
    before = sorted[index - 1];
  const ambiguous =
    !!after &&
    ((sorted[index + 1] &&
      Date.parse(sorted[index + 1].created_at) ===
        Date.parse(after.created_at)) ||
      (before &&
        Date.parse(before.created_at) === Date.parse(after.created_at)) ||
      (sorted[index - 2] &&
        before &&
        Date.parse(sorted[index - 2].created_at) ===
          Date.parse(before.created_at)));
  const valid =
    !!before &&
    !!after &&
    !ambiguous &&
    usableRecoverySource(before) &&
    usableRecoverySource(after);
  const key = JSON.stringify([
    hostID,
    before?.id,
    before?.manifest_sha,
    before?.status,
    before?.verification_error,
    after?.id,
    after?.manifest_sha,
    after?.status,
    after?.verification_error,
    valid,
    retry,
  ]);
  const data = result?.key === key ? result : null;
  useEffect(() => {
    if (!after) return;
    const controller = new AbortController();
    const timer = window.setTimeout(async () => {
      const outcomes = await Promise.allSettled([
        api<SettingSnapshotResult>(
          `backups/${encodeURIComponent(after.id)}/setting-snapshot`,
          "GET",
          undefined,
          controller.signal,
        ),
        valid
          ? api<SettingSnapshotResult>(
              `backups/${encodeURIComponent(before!.id)}/setting-snapshot`,
              "GET",
              undefined,
              controller.signal,
            )
          : Promise.resolve(null),
        valid
          ? api<FileDiff[]>(
              "backups/compare",
              "POST",
              { from: before!.id, to: after.id },
              controller.signal,
            )
          : Promise.resolve(null),
      ]);
      if (controller.signal.aborted) return;
      const value = <T,>(n: number) =>
        outcomes[n].status === "fulfilled" ? (outcomes[n].value as T) : null;
      setResult({
        key,
        after: value<SettingSnapshotResult>(0),
        before: value<SettingSnapshotResult>(1),
        rows: value<FileDiff[]>(2),
        errors: outcomes.flatMap((o) =>
          o.status === "rejected" ? [(o.reason as Error).message] : [],
        ),
      });
    }, 180);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [key]);
  const significant = data?.rows?.filter((r) => !r.mtime_only);
  const visibleFiles =
    significant?.filter((r) => !area || changeArea(r.path) === area) || [];
  const settings =
    data?.after?.settings.filter(
      (s) => !area || settingArea(s.setting.id) === area,
    ) || [];
  const recorded = settings.filter(
    (s) =>
      s.observation.state === "value" ||
      data?.before?.settings.find((b) => b.setting.id === s.setting.id)
        ?.observation.state === "value",
  );
  const visibleSettings = showAllSettings ? settings : recorded;
  const removedSelection = !!chosen && !sorted.some((b) => b.id === chosen);
  function move(i: number) {
    const b = sorted[i];
    if (b) setChosen(b.id);
  }
  const detailButton = valid && data?.rows !== null && data?.rows !== undefined;
  return (
    <section
      className="configuration-journey"
      data-surface-topic="activity"
      aria-label="Konfigurationsreise"
    >
      <header className="journey-heading">
        <div>
          <span className="eyebrow">
            <Compass size={15} aria-hidden="true" /> Konfigurationsreise
          </span>
          <h3>Konfiguration im Verlauf</h3>
          <p>
            Wähle eine Sicherung. Du siehst die damaligen Werte und die
            Änderungen seit dem Stand davor.
          </p>
        </div>
        <span className="journey-source-note">Gesicherte Konfiguration</span>
      </header>
      {sorted.length === 0 ? (
        <Empty title="Noch keine datierten Stände">
          Für diese Ansicht brauchst du eine Sicherung mit gültigem Datum.
        </Empty>
      ) : (
        <>
          {backups.filter((b) => b.host_id === hostID).length !==
            sorted.length && (
            <p className="notice warning">
              Stände mit unplausiblem Datum bleiben im Sicherungsverlauf
              erreichbar.
            </p>
          )}
          {removedSelection && (
            <p className="notice warning">
              Der gewählte Stand ist nicht mehr vorhanden. Die Reise zeigt den
              ältesten verfügbaren Zeitpunkt.
            </p>
          )}
          <div className="journey-time-card">
            <div className="journey-selected-time">
              <div>
                <span className="eyebrow">
                  {index === sorted.length - 1
                    ? "Neuester Stand"
                    : "Historischer Stand"}
                </span>
                <time dateTime={after.created_at}>
                  {preciseDate(after.created_at)}
                </time>
                <code>{after.id.slice(0, 10)}</code>
              </div>
              <span className="journey-position">
                {index + 1} / {sorted.length}
              </span>
            </div>
            <label
              className="journey-slider-label"
              htmlFor={`journey-time-${hostID}`}
            >
              Durch Sicherungszeitpunkte gehen
            </label>
            <input
              id={`journey-time-${hostID}`}
              className="journey-slider"
              type="range"
              min={0}
              max={Math.max(0, sorted.length - 1)}
              step={1}
              value={index}
              disabled={sorted.length < 2}
              aria-valuetext={`${index + 1} von ${sorted.length} · ${preciseDate(after.created_at)}`}
              onChange={(e) => move(Number(e.target.value))}
            />
            <div className="journey-time-navigation">
              <button
                className="secondary"
                aria-label="Vorheriger Sicherungszeitpunkt"
                disabled={index === 0}
                onClick={() => move(index - 1)}
              >
                <ArrowLeft size={15} aria-hidden="true" /> Früher
              </button>
              <span>
                {index === 0
                  ? "Ausgangspunkt"
                  : `${valid ? "Verglichen mit" : "Vorheriger Stand"} ${preciseDate(before.created_at)}`}
              </span>
              <button
                className="secondary"
                aria-label="Nächster Sicherungszeitpunkt"
                disabled={index === sorted.length - 1}
                onClick={() => move(index + 1)}
              >
                Später <ArrowRight size={15} aria-hidden="true" />
              </button>
            </div>
          </div>
          {!usableRecoverySource(after) ? (
            <p className="notice warning">
              Dieser Stand ist unvollständig oder hat offene Integritätsfehler.
              Die gespeicherten Werte sind nicht belastbar.
            </p>
          ) : !before ? (
            <p className="journey-context">
              Erster erfasster Stand · {after.files} Einträge. Hier beginnt der
              vorhandene Verlauf; frühere Änderungen sind nicht bekannt.
            </p>
          ) : !valid ? (
            <p className="notice warning">
              {ambiguous
                ? "Die zeitliche Reihenfolge dieser Stände ist nicht eindeutig. Es wird kein Wertwechsel daraus abgeleitet."
                : "Mindestens ein Vergleichsstand ist unvollständig oder hat offene Integritätsfehler. Dazwischen bleibt eine Nachweislücke."}
            </p>
          ) : (
            <p className="journey-context" role="status">
              {!data
                ? "Änderungen zu diesem Zeitpunkt werden gelesen …"
                : significant
                  ? `${significant.length} ${significant.length === 1 ? "Datei mit Änderungen" : "Dateien mit Änderungen"} seit dem vorherigen Stand${data.rows?.some((r) => r.mtime_only) ? " · zusätzlich Zeitangaben aktualisiert" : ""}`
                  : "Änderungen konnten nicht vollständig gelesen werden."}
            </p>
          )}
          {data?.errors.length ? (
            <LoadError
              message="Ein Teil des Verlaufs konnte nicht geladen werden. Die verfügbaren Daten bleiben sichtbar."
              retry={() => setRetry((n) => n + 1)}
              label="Zeitpunkt erneut lesen"
            />
          ) : null}
          <div
            className="journey-areas"
            role="group"
            aria-label="Bereich der Konfigurationsreise"
          >
            <button
              className="journey-area"
              aria-pressed={!area}
              onClick={() => setArea("")}
            >
              <Compass size={16} aria-hidden="true" />
              <strong>Alle Bereiche</strong>
              <span>
                {significant
                  ? `${significant.length} ${significant.length === 1 ? "Dateiänderung" : "Dateiänderungen"}`
                  : "Überblick"}
              </span>
            </button>
            {areas.map(({ name, Icon }) => {
              const count = significant?.filter(
                (r) => changeArea(r.path) === name,
              ).length;
              const valueCount = valid
                ? (data?.after?.settings || []).filter((s) => {
                    const older = data?.before?.settings.find(
                      (b) => b.setting.id === s.setting.id,
                    )?.observation;
                    return (
                      settingArea(s.setting.id) === name &&
                      s.observation.state === "value" &&
                      older?.state === "value" &&
                      older.value !== s.observation.value
                    );
                  }).length
                : 0;
              return (
                <button
                  className="journey-area"
                  key={name}
                  aria-pressed={area === name}
                  onClick={() => setArea(name)}
                >
                  <Icon size={16} aria-hidden="true" />
                  <strong>{name}</strong>
                  <span>
                    {count === undefined
                      ? "Ohne Dateivergleich"
                      : `${count} ${count === 1 ? "Datei" : "Dateien"}`}
                    {valueCount > 0
                      ? ` · ${valueCount} ${valueCount === 1 ? "Wert geändert" : "Werte geändert"}`
                      : ""}
                  </span>
                </button>
              );
            })}
          </div>
          <div className="journey-values" aria-live="polite" aria-busy={!data}>
            {visibleSettings.map((s) => {
              const older = data?.before?.settings.find(
                (b) => b.setting.id === s.setting.id,
              )?.observation;
              const now = s.observation;
              const known = now.state === "value";
              const changed =
                valid &&
                known &&
                older?.state === "value" &&
                older.value !== now.value;
              return (
                <article
                  className={
                    "journey-value-card" +
                    (changed ? " journey-value-changed" : "") +
                    (!known ? " journey-value-gap" : "")
                  }
                  key={s.setting.id}
                >
                  <div className="journey-value-heading">
                    <h4>{s.setting.name}</h4>
                    <span>
                      {changed
                        ? "Wert geändert"
                        : !known
                          ? "Nachweislücke"
                          : valid && older?.state === "value"
                            ? "Unverändert"
                            : before
                              ? "Wert in diesem Stand"
                              : "Erstmals erfasst"}
                    </span>
                  </div>
                  <div className="journey-value-transition">
                    {valid && older && (
                      <>
                        <div>
                          <small>Vorher</small>
                          <strong>
                            {older.state === "value"
                              ? settingValue(s.setting.id, older.value)
                              : observationLabels[older.state] ||
                                "Nicht bekannt"}
                          </strong>
                        </div>
                        <ArrowRight size={16} aria-hidden="true" />
                      </>
                    )}
                    <div>
                      <small>{valid ? "Nachher" : "In diesem Stand"}</small>
                      <strong>
                        {known
                          ? settingValue(s.setting.id, now.value)
                          : observationLabels[now.state] || "Nicht bekannt"}
                      </strong>
                    </div>
                  </div>
                  <code>
                    {s.setting.key}
                    {known ? ` = ${now.value}` : ""}
                  </code>
                  {changed && older && (
                    <SettingExplanation
                      id={s.setting.id}
                      before={older}
                      after={now}
                    />
                  )}
                  <button
                    className="text-button"
                    onClick={() => onTrackSetting(s.setting.id)}
                  >
                    <Fingerprint size={13} aria-hidden="true" /> Einstellung
                    verfolgen
                  </button>
                </article>
              );
            })}
          </div>
          {!data && (
            <p className="muted" role="status">
              Gespeicherte Einstellungen werden geladen …
            </p>
          )}
          {data?.after && !visibleSettings.length && (
            <p className="journey-context">
              Für diesen Bereich sind keine unterstützten Einstellungswerte
              eindeutig erfasst. Dateiänderungen stehen darunter.
            </p>
          )}
          {settings.length > recorded.length && (
            <button
              className="text-button journey-missing-toggle"
              aria-expanded={showAllSettings}
              onClick={() => setShowAllSettings((v) => !v)}
            >
              {showAllSettings
                ? "Unklare Werte einklappen"
                : `${settings.length - recorded.length} weitere Einstellungen ohne eindeutigen Wert anzeigen`}
            </button>
          )}
          {!!visibleFiles.length && (
            <div className="journey-file-brief">
              <h4>Betroffene Konfigurationen</h4>
              <div className="journey-file-cards">
                {visibleFiles.slice(0, 3).map((r) => (
                  <div className="journey-file-card" key={r.path}>
                    <strong>{changeSubject(r)}</strong>
                    <span>
                      {r.change === "added"
                        ? "Hinzugefügt"
                        : r.change === "removed"
                          ? "Entfernt"
                          : r.content_changed
                            ? "Inhalt geändert"
                            : "Metadaten geändert"}
                    </span>
                    {!r.secret && <code>{r.path}</code>}
                  </div>
                ))}
              </div>
              {visibleFiles.length > 3 && (
                <p>
                  {visibleFiles.length - 3} weitere Dateien im Detailvergleich.
                </p>
              )}
            </div>
          )}
          <div className="journey-actions">
            {detailButton && (
              <button
                className="secondary"
                onClick={() => onCompare(before!.id, after.id)}
              >
                Diesen Zeitpunkt im Detail vergleichen{" "}
                <ArrowRight size={14} aria-hidden="true" />
              </button>
            )}
            <button
              className="text-button"
              onClick={() => onOpenBackup(after.id)}
            >
              Sicherung dieses Zeitpunkts öffnen
            </button>
          </div>
          <p className="journey-footnote">
            Jeder Schritt steht für eine Sicherung. Angezeigt werden die Werte
            in den Dateien. Welche Werte am laufenden Host gelten, wird hier
            nicht geprüft; Standardwerte werden nicht ergänzt.
          </p>
        </>
      )}
    </section>
  );
}
