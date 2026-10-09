import { useEffect, useRef, useState } from "react";
import { api, labels } from "../../api";
import type { Plan, Status, User } from "../../api";
import type { Notify } from "../shared";
import RecoverySource from "./RecoverySource";
import RecoveryTarget from "./RecoveryTarget";
import RecoveryEvidence from "./RecoveryEvidence";
import RecoveryDecisions from "./RecoveryDecisions";
import { useRecoveryDraft } from "./useRecoveryDraft";
import type { RecoveryDraft } from "./useRecoveryDraft";

export default function RecoveryWizard({
  status,
  user,
  draft,
  initialBackup,
  initialFile,
  onPlan,
  onClose,
  onConnectTarget,
  notify,
  onBusyChange,
}: {
  status: Status;
  user: User;
  draft?: RecoveryDraft;
  initialBackup?: string;
  initialFile?: string;
  onPlan: (plan: Plan) => void;
  onClose: () => void;
  onConnectTarget: () => void;
  notify: Notify;
  onBusyChange?: (busy: boolean) => void;
}) {
  const localDraft = useRecoveryDraft(
    draft ? "" : initialBackup,
    draft ? undefined : initialFile,
  );
  const d = draft || localDraft;
  const [busy, setBusy] = useState(false);
  const request = useRef(false);
  const mounted = useRef(true);
  const heading = useRef<HTMLElement>(null);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    heading.current?.focus();
  }, [d.step]);
  useEffect(() => {
    onBusyChange?.(busy);
    if (!busy) return;
    const protect = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", protect);
    return () => {
      window.removeEventListener("beforeunload", protect);
      onBusyChange?.(false);
    };
  }, [busy, onBusyChange]);
  const decisionsReady =
    !d.recipeBusy &&
    !!d.inspection &&
    !d.needsInspection &&
    !d.openFiles &&
    !d.openMappings &&
    !d.duplicatePorts;
  const confirmationsReady =
    (!d.inspection?.requires_console || d.consoleOK) &&
    (!d.inspection?.requires_source_offline || d.offline);
  async function create() {
    if (request.current || !decisionsReady || !confirmationsReady) return;
    request.current = true;
    setBusy(true);
    d.setError("");
    const generation = d.generation.current;
    try {
      const plan = await api<Plan>("plans", "POST", d.requestBody());
      if (mounted.current && generation === d.generation.current) {
        onPlan(plan);
        notify("Wiederherstellungsplan erstellt");
      }
    } catch (e) {
      if (mounted.current) d.setError((e as Error).message);
    } finally {
      request.current = false;
      if (mounted.current) setBusy(false);
      onBusyChange?.(false);
    }
  }
  return (
    <section
      className="recovery-workspace"
      aria-label="Wiederherstellung vorbereiten"
      ref={heading}
      tabIndex={-1}
    >
      <div className="section-heading">
        <h1>Wiederherstellung vorbereiten</h1>
        <button className="secondary" disabled={busy} onClick={onClose}>
          Vorbereitung schließen
        </button>
      </div>
      <p className="recovery-draft-context">
        Quelle:{" "}
        {status.backups.find((b) => b.id === d.backup)?.host_name ||
          "Noch auswählen"}{" "}
        · Ziel:{" "}
        {status.hosts.find((h) => h.id === d.target)?.name || "Noch auswählen"}{" "}
        · {d.openFiles + d.openMappings} offene Entscheidungen
      </p>
      <nav aria-label="Vorbereitungsschritte" className="recovery-steps">
        <ol>
          {[
            "Quelle und Situation",
            "Ziel prüfen",
            "Entscheidungen",
            "Plan prüfen",
            "Ausführen und nachprüfen",
          ].map((label, i) => (
            <li
              key={label}
              aria-current={d.step === i + 1 ? "step" : undefined}
            >
              <span>{i + 1}</span>
              {label}
              {i === 4 && <small>Im gespeicherten Plan</small>}
            </li>
          ))}
        </ol>
      </nav>
      <fieldset className="form-fields" disabled={busy}>
        {d.step === 1 && <RecoverySource status={status} draft={d} />}
        {d.step === 2 && (
          <RecoveryTarget
            status={status}
            user={user}
            draft={d}
            onConnectTarget={onConnectTarget}
          />
        )}
        {d.step === 3 && (
          <RecoveryDecisions draft={d} canReveal={user.secrets} />
        )}
        {d.step === 4 && (
          <section aria-label="Planprüfung">
            <h2>4 · Plan prüfen und speichern</h2>
            <p>
              {labels[d.scenario]} · {d.selectedPaths.length} Dateien · Ziel{" "}
              {d.inspection?.target.hostname}
            </p>
            <p className="field-hint">Sicherung {d.backup}</p>
            <ul className="recovery-choice-summary">
              {(
                [
                  ["backup", "Original übernehmen"],
                  ["prepared", "Vorbereitete Fassung"],
                  ["keep", "Ziel behalten"],
                  ["manual", "Manuell bearbeiten"],
                ] as const
              ).map(([decision, label]) => (
                <li key={decision}>
                  {label}:{" "}
                  {
                    d.selectedPaths.filter((p) => d.decisions[p] === decision)
                      .length
                  }
                </li>
              ))}
            </ul>
            <p>
              Der gespeicherte Plan zeigt den erfassten Zielstand, die
              vorbereiteten Dateien und konkrete Unterschiede. Erst dort wird
              eine mögliche Dateiübernahme gesondert bestätigt.
            </p>
            {d.inspection?.requires_console && (
              <label className="check">
                <input
                  type="checkbox"
                  checked={d.consoleOK}
                  onChange={(e) => d.setConsoleOK(e.target.checked)}
                />
                Konsolenzugang zum Ziel ist verfügbar
              </label>
            )}
            {d.inspection?.requires_source_offline && (
              <label className="check">
                <input
                  type="checkbox"
                  checked={d.offline}
                  onChange={(e) => d.setOffline(e.target.checked)}
                />
                Alter Host ist ausgeschaltet oder isoliert
              </label>
            )}
            {(d.inspection?.requires_console ||
              d.inspection?.requires_source_offline) && (
              <p className="field-hint">
                Diese Angaben werden als Benutzerbestätigung dokumentiert und
                ersetzen keinen maschinellen Nachweis. Netzwerk, Cluster und
                Storageverwaltung bleiben eigene manuelle Schritte.
              </p>
            )}
            {!confirmationsReady && (
              <p className="notice">
                Konsolenzugang und erforderliche Quellisolierung vor Ort prüfen
                und hier bestätigen.
              </p>
            )}
          </section>
        )}
        {d.step >= 2 && d.inspection && (
          <div className="recovery-inspection-notes">
            {d.inspection.blockers?.map((s, i) => (
              <p className="notice warning" key={i}>
                {s}
              </p>
            ))}
            {!!(
              d.inspection.warnings?.length || d.inspection.manual?.length
            ) && (
              <RecoveryEvidence
                title="Hinweise und manuelle Schritte"
                items={[
                  ...(d.inspection.warnings || []),
                  ...(d.inspection.manual || []),
                ]}
              />
            )}
            {!!d.inspection.preserved?.length && (
              <RecoveryEvidence
                title="Zieldateien bleiben erhalten"
                items={d.inspection.preserved.map(
                  (p) => `${p.path}: ${p.reason}`,
                )}
              />
            )}
          </div>
        )}
        {d.error && (
          <p className="notice warning" role="alert">
            {d.error} Auswahl bleibt erhalten. Anfrage erneut versuchen oder
            zurück zum Ziel wechseln.
          </p>
        )}
        <footer className="recovery-footer">
          <button
            type="button"
            className="secondary"
            disabled={busy || d.step === 1}
            onClick={() => {
              d.setError("");
              d.setStep(d.step - 1);
            }}
          >
            Zurück
          </button>
          <span>Schritt {d.step} von 5</span>
          {d.step === 1 && (
            <button
              type="button"
              disabled={!d.sourceReady}
              onClick={() => d.setStep(2)}
            >
              Weiter zum Ziel
            </button>
          )}
          {d.step === 2 && (
            <button
              type="button"
              disabled={!d.inspection || d.inspecting}
              onClick={() => d.setStep(3)}
            >
              Weiter zu Entscheidungen
            </button>
          )}
          {d.step === 3 && (
            <button
              type="button"
              disabled={
                d.inspecting ||
                d.duplicatePorts ||
                !!d.openMappings ||
                (!d.needsInspection && !decisionsReady)
              }
              onClick={() => (d.needsInspection ? d.inspect() : d.setStep(4))}
            >
              {d.inspecting
                ? "Zuordnungen werden geprüft …"
                : d.needsInspection
                  ? "Zuordnungen prüfen"
                  : "Weiter zur Planprüfung"}
            </button>
          )}
          {d.step === 4 && (
            <button
              type="button"
              disabled={busy || !decisionsReady || !confirmationsReady}
              onClick={create}
            >
              {busy ? "Plan wird geprüft …" : "Plan prüfen"}
            </button>
          )}
        </footer>
      </fieldset>
    </section>
  );
}
