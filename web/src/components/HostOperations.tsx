import HelperOverview from "./HelperOverview";
export { default as HostDrift } from "./HostDrift";
import { useEffect, useRef, useState } from "react";
import { api, date, plural, interval } from "../api";
import type { Host, Job } from "../api";
import { HostName } from "../hostNames";
import { Dialog, State } from "./shared";
import { CheckSquare, Wrench } from "lucide-react";

type Target = {
  host_id: string;
  revision: string;
  name?: string;
  state?: string;
  message?: string;
  job_id?: string;
};
type Run = { id: string; targets: Target[] };
const actions: Record<string, string> = {
  backup: "Sichern und verifizieren",
  probe: "Verbindung und Inventar prüfen",
  verify: "Letzte Sicherung verifizieren",
  drift: "Liveänderungen prüfen",
  maintenance: "Befristete Wartung",
  maintenance_end: "Wartung beenden",
  pause: "Automatik pausieren",
  resume: "Automatik aktivieren",
  schedule: "Zeitplan setzen",
  group: "Standort / Gruppe setzen",
  drift_schedule: "Automatische Liveprüfung",
};
const actionHelp: Record<string, string> = {
  backup:
    "Erstellt eine neue Konfigurationssicherung und prüft anschließend deren Integrität.",
  probe:
    "Prüft die Verbindung und aktualisiert das Hostinventar. Dabei entsteht keine neue Sicherung.",
  verify:
    "Prüft die gespeicherten Dateien der jeweils letzten Sicherung. Erfasst keine neuen Konfigurationen vom Host.",
  drift:
    "Liest den aktuellen Konfigurationszustand und vergleicht Inhalte mit der letzten vollständigen Sicherung.",
  maintenance:
    "Pausiert automatische Sicherungen, Liveprüfungen und Überfälligkeitsmeldungen bis zum Endzeitpunkt. Von Hand geht weiterhin alles.",
  maintenance_end:
    "Beendet die befristete Wartung. Der bisherige Zeitplan gilt wieder; eine dauerhaft pausierte Automatik bleibt pausiert.",
  pause:
    "Pausiert die Automatik dauerhaft, bis du sie ausdrücklich wieder aktivierst. Manuelle Aktionen bleiben möglich.",
  resume:
    "Aktiviert die reguläre Automatik. Eine noch laufende befristete Wartung gilt weiter bis zu ihrem Ende.",
  schedule:
    "Setzt die tägliche Startzeit. Ein leeres Feld übernimmt den gemeinsamen Zeitplan aus den Einstellungen.",
  group:
    "Setzt die manuelle Standortgruppe. Ein leerer Wert entfernt diese Zuordnung; technische Hostnamen und Zugänge bleiben bestehen.",
  drift_schedule:
    "Aktiviert oder deaktiviert regelmäßige lesende Liveprüfungen. Diese erstellen keine Sicherung und berücksichtigen Wartung und pausierte Hosts.",
};
const readActions = ["backup", "probe", "verify", "drift"];
export default function HostOperations({
  hosts,
  filtered,
  selection,
  onSelection,
  canManage,
  canOperate,
  refresh,
  openJob,
  actorID,
  jobs = [],
}: {
  hosts: Host[];
  filtered: Host[];
  selection: Record<string, string>;
  onSelection: (v: Record<string, string>) => void;
  canManage: boolean;
  canOperate: boolean;
  refresh: () => void;
  openJob?: (id: string) => void;
  actorID: string;
  jobs?: Job[];
}) {
  const [action, setAction] = useState("backup");
  const [value, setValue] = useState("");
  const [minutes, setMinutes] = useState(0);
  const [hours, setHours] = useState(4);
  const [review, setReview] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [run, setRun] = useState<Run>();
  const [helper, setHelper] = useState(false);
  const targets = Object.entries(selection).map(([host_id, revision]) => ({
    host_id,
    revision,
  }));
  const [request, setRequest] = useState<{
    request_id: string;
    action: string;
    targets: Target[];
    value: string;
    minutes: number;
    until: string;
  }>();
  const activeRequest = useRef("");
  const filteredIDs = new Set(filtered.map((h) => h.id));
  const hiddenSelected = targets.filter(
    (t) => !filteredIDs.has(t.host_id),
  ).length;
  const key = `anker.host-operation.${actorID}`;
  useEffect(() => {
    setRun(undefined);
    setRequest(undefined);
    try {
      const saved = sessionStorage.getItem(key);
      if (saved) {
        const parsed = JSON.parse(saved);
        if (
          typeof parsed.request_id !== "string" ||
          !/^[a-zA-Z0-9_-]{1,100}$/.test(parsed.request_id) ||
          !Array.isArray(parsed.targets) ||
          typeof parsed.action !== "string"
        )
          throw new Error("invalid stored selection");
        activeRequest.current = parsed.request_id;
        setRequest(parsed);
        api<Run>(`operations/hosts/${parsed.request_id}`)
          .then((value) => {
            if (activeRequest.current === parsed.request_id) setRun(value);
          })
          .catch(() => {
            if (activeRequest.current === parsed.request_id)
              setError(
                "Letzte Aktion noch nicht bestätigt. Mit derselben Auswahl erneut abfragen oder ausführen.",
              );
          });
      }
    } catch {
      /* Storage optional. */
    }
    return () => {
      activeRequest.current = "";
    };
  }, [key]);
  function prepare() {
    if (error && request) {
      setReview(true);
      return;
    }
    const next = {
      request_id: crypto.randomUUID(),
      action,
      targets,
      value,
      minutes,
      until:
        action === "maintenance"
          ? new Date(Date.now() + hours * 3600000).toISOString()
          : "",
    };
    activeRequest.current = next.request_id;
    setRequest(next);
    setReview(true);
    setError("");
  }
  async function execute() {
    if (!request) return;
    setBusy(true);
    setError("");
    try {
      sessionStorage.setItem(key, JSON.stringify(request));
    } catch {
      /* Request still remains in memory. */
    }
    try {
      const result = await api<Run>("operations/hosts", "POST", request);
      setRun(result);
      setReview(false);
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section
      className={`operations-card${targets.length ? "" : " operations-idle"}`}
      aria-label="Sammelaktionen"
    >
      {targets.length > 0 && (
        <div className="operations-heading">
          <CheckSquare size={20} />
          <div>
            <h3>Gemeinsam verwalten</h3>
            <p>
              Die Auswahl gilt über alle Seiten. Geänderte Hosts werden vor der
              Ausführung erkannt.
            </p>
          </div>
          <span className="count-badge">{targets.length} ausgewählt</span>
        </div>
      )}
      <div className="operations-controls">
        {canOperate && (
          <>
            <button
              className="secondary"
              onClick={() =>
                onSelection({
                  ...selection,
                  ...Object.fromEntries(
                    filtered
                      .filter((h) => h.config_revision)
                      .map((h) => [h.id, h.config_revision!]),
                  ),
                })
              }
            >
              {filtered.length === 1
                ? "Diesen gefilterten Host auswählen"
                : `Alle ${filtered.length} gefilterten Hosts auswählen`}
            </button>
            {targets.length > 0 && (
              <>
                <button
                  className="text-button"
                  disabled={!targets.length}
                  onClick={() => onSelection({})}
                >
                  Auswahl leeren
                </button>
                <select
                  aria-label="Sammelaktion"
                  value={action}
                  onChange={(e) => {
                    setAction(e.target.value);
                    setValue("");
                  }}
                >
                  {Object.entries(actions)
                    .filter(
                      ([key]) =>
                        canManage ||
                        ["backup", "probe", "verify", "drift"].includes(key),
                    )
                    .map(([key, label]) => (
                      <option key={key} value={key}>
                        {label}
                      </option>
                    ))}
                </select>
                {action === "schedule" && (
                  <input
                    aria-label="Gemeinsamer Zeitplan"
                    type="time"
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                  />
                )}
                {action === "group" && (
                  <input
                    aria-label="Gemeinsame Gruppe"
                    placeholder="Standort / Gruppe"
                    maxLength={200}
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                  />
                )}
                {action === "maintenance" && (
                  <select
                    aria-label="Wartungsdauer"
                    value={hours}
                    onChange={(e) => setHours(+e.target.value)}
                  >
                    {[1, 4, 24, 72].map((h) => (
                      <option key={h} value={h}>
                        {plural(h, "Stunde", "Stunden")}
                      </option>
                    ))}
                  </select>
                )}
                {action === "drift_schedule" && (
                  <select
                    aria-label="Prüfintervall"
                    value={minutes}
                    onChange={(e) => setMinutes(+e.target.value)}
                  >
                    {[0, 15, 30, 60, 240, 1440].map((m) => (
                      <option key={m} value={m}>
                        {m
                          ? interval(m).replace(/^./, (c) => c.toUpperCase())
                          : "Ausgeschaltet"}
                      </option>
                    ))}
                  </select>
                )}
                <button disabled={!targets.length || busy} onClick={prepare}>
                  {error && request
                    ? "Unbestätigte Aktion prüfen"
                    : "Aktion prüfen"}
                </button>
              </>
            )}
          </>
        )}
        <button className="secondary" onClick={() => setHelper(true)}>
          <Wrench size={15} /> Helferübersicht
        </button>
      </div>
      {targets.length > 0 && (
        <div className="operation-guidance" data-action={action}>
          <p>{actionHelp[action]}</p>
          {hiddenSelected > 0 && (
            <p className="selection-hidden" role="status">
              {hiddenSelected} ausgewählte{" "}
              {hiddenSelected === 1 ? "Host liegt" : "Hosts liegen"} außerhalb
              der aktuellen Filter. Die Aktion gilt für die gesamte feste
              Auswahl.
            </p>
          )}
          <details>
            <summary>{targets.length} ausgewählte Hosts ansehen</summary>
            <ul className="selection-review">
              {targets.map((t) => {
                const h = hosts.find((h) => h.id === t.host_id);
                return (
                  <li key={t.host_id}>
                    <strong>
                      <HostName name={h?.name || t.host_id} />
                    </strong>
                    <small>
                      {h?.address || "Host nicht mehr vorhanden"}
                      {h?.config_revision !== t.revision &&
                        " · Einstellungen geändert"}
                    </small>
                  </li>
                );
              })}
            </ul>
          </details>
        </div>
      )}
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {error && request && (
        <button
          className="text-button"
          disabled={busy}
          onClick={() => {
            setError("");
            setRequest(undefined);
            try {
              sessionStorage.removeItem(key);
            } catch {
              /* Optional */
            }
          }}
        >
          Quittung schließen und neue Auswahl vorbereiten
        </button>
      )}
      {request && !review && error && canOperate && (
        <button
          className="secondary"
          disabled={busy}
          onClick={() => setReview(true)}
        >
          Letzte Aktion erneut prüfen
        </button>
      )}
      {run && (
        <details className="operation-receipt" open key={run.id}>
          <summary>
            Ergebnis:{" "}
            {
              run.targets.filter((t) =>
                ["queued", "applied"].includes(t.state || ""),
              ).length
            }{" "}
            von {run.targets.length} übernommen
          </summary>
          <div className="operation-results">
            {run.targets.map((t) => (
              <div key={t.host_id}>
                <strong>
                  <HostName
                    name={
                      hosts.find((h) => h.id === t.host_id)?.name ||
                      t.name ||
                      t.host_id
                    }
                  />
                </strong>
                {t.state === "queued" ? (
                  <State
                    value={
                      jobs.find((j) => j.id === t.job_id)?.state || "queued"
                    }
                    label={
                      jobs.some((j) => j.id === t.job_id)
                        ? undefined
                        : "Auftrag angenommen"
                    }
                  />
                ) : (
                  <span
                    className={`state ${t.state === "skipped" ? "warning" : "muted"}`}
                  >
                    {t.state === "queued"
                      ? "Auftrag gestartet"
                      : t.state === "applied"
                        ? "Übernommen"
                        : t.state === "pending"
                          ? "Noch offen"
                          : "Übersprungen"}
                  </span>
                )}
                <span>{t.message}</span>
                {t.job_id && openJob && (
                  <button
                    className="text-button"
                    onClick={() => openJob(t.job_id!)}
                  >
                    Auftrag ansehen
                  </button>
                )}
              </div>
            ))}
          </div>
        </details>
      )}
      {review && request && (
        <Dialog
          title="Sammelaktion prüfen"
          busy={busy}
          onClose={() => {
            if (!busy) setReview(false);
          }}
        >
          <p>
            <strong>{actions[request.action]}</strong> für{" "}
            {request.targets.length} ausdrücklich{" "}
            {request.targets.length === 1
              ? "ausgewählten Host"
              : "ausgewählte Hosts"}
            .
          </p>
          <p className="operation-help">{actionHelp[request.action]}</p>
          {["schedule", "group"].includes(request.action) && (
            <p>
              Neuer Wert:{" "}
              <strong>
                {request.value ||
                  (request.action === "schedule"
                    ? "Gemeinsamen Zeitplan übernehmen"
                    : "Manuelle Gruppe entfernen")}
              </strong>
            </p>
          )}
          {request.action === "maintenance" && (
            <p>
              Bis {date(request.until)}. Die bisherigen Zeitpläne bleiben
              gespeichert und gelten nach Ablauf wieder.
            </p>
          )}
          {request.action === "drift_schedule" && (
            <p>
              {request.minutes
                ? `Lesende Liveprüfung alle ${request.minutes} Minuten.`
                : "Automatische Liveprüfung ausschalten."}{" "}
              Konfigurationsinhalte werden dabei nicht als Sicherung
              gespeichert.
            </p>
          )}
          <ul className="selection-review">
            {request.targets.map((t) => (
              <li key={t.host_id}>
                <HostName
                  name={
                    hosts.find((h) => h.id === t.host_id)?.name || t.host_id
                  }
                />
                <small>
                  {hosts.find((h) => h.id === t.host_id)?.address ||
                    "Host nicht mehr vorhanden"}
                </small>
              </li>
            ))}
          </ul>
          <p className="muted">
            {readActions.includes(request.action)
              ? "Belegte oder inzwischen bearbeitete Hosts werden mit Begründung übersprungen."
              : "Inzwischen bearbeitete Hosts werden mit Begründung übersprungen. Laufende Aufträge werden durch diese Einstellung nicht abgebrochen."}
          </p>
          {error && <p role="alert">{error}</p>}
          <button disabled={busy} onClick={execute}>
            {busy ? "Wird übermittelt …" : "Für diese Auswahl ausführen"}
          </button>
        </Dialog>
      )}
      {helper && (
        <HelperOverview
          hosts={hosts}
          canManage={canManage}
          canOperate={canOperate}
          openJob={openJob}
          onClose={() => setHelper(false)}
          refresh={refresh}
        />
      )}
    </section>
  );
}
