import RemovalFiles from "./RemovalFiles";
import type { ChangeRecoveryIntent } from "./ChangeRecoveryIntent";
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
  changeIntent,
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
  changeIntent?: ChangeRecoveryIntent;
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
  const removals = d.selectedPaths.filter((p) => d.decisions[p] === "remove");
  const confirmationsReady =
    (!removals.length || d.removalsConfirmed) &&
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
      data-surface-topic="recovery"
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
      <section
        className="recovery-draft-context"
        aria-label="Vorbereitungskontext"
      >
        <dl className="recovery-context">
          <div>
            <dt>Quelle</dt>
            <dd>
              {status.backups.find((backup) => backup.id === d.backup)
                ?.host_name || "Noch auswählen"}
            </dd>
          </div>
          <div>
            <dt>Ziel</dt>
            <dd>
              {status.hosts.find((host) => host.id === d.target)?.name ||
                "Noch auswählen"}
            </dd>
          </div>
          <div>
            <dt>Entscheidungen</dt>
            <dd>
              {!d.inspection
                ? "Nach Zielprüfung ermitteln"
                : d.needsInspection
                  ? "Erneute Zielprüfung erforderlich"
                  : `${d.openFiles + d.openMappings} offen`}
            </dd>
          </div>
          <div>
            <dt>Nächster Schritt</dt>
            <dd>
              {d.step === 1
                ? "Quelle und Dateien auswählen"
                : d.step === 2
                  ? d.needsInspection
                    ? "Ziel erneut prüfen"
                    : d.inspection
                      ? "Weiter zu Entscheidungen"
                      : "Ziel auswählen und prüfen"
                  : d.step === 3
                    ? d.needsInspection
                      ? "Zuordnungen erneut prüfen"
                      : decisionsReady
                        ? "Weiter zur Planprüfung"
                        : "Dateien und Zuordnungen entscheiden"
                    : confirmationsReady
                      ? "Plan prüfen und speichern"
                      : "Erforderliche Bestätigungen prüfen"}
            </dd>
          </div>
        </dl>
      </section>
      {changeIntent && (
        <section
          className="rollback-handoff-card"
          aria-label="Aus der Änderung übernommen"
        >
          <span className="eyebrow">Aus der Änderung übernommen</span>
          <h3>
            {changeIntent.settingName} · Zielwert {changeIntent.desiredValue}
          </h3>
          <p>
            Die Auswahl umfasst ganze Dateien. Der frühere Wert ist eine
            Absicht; erst der Vergleich mit dem aktuell gelesenen Ziel zeigt die
            tatsächlichen Änderungen.
          </p>
          <strong>
            {d.backup === changeIntent.backupID &&
            d.scenario === "files" &&
            d.target === changeIntent.targetID &&
            d.files.length === changeIntent.files.length &&
            d.removeFiles.length === (changeIntent.removeFiles?.length || 0) &&
            (changeIntent.removeFiles || []).every((p) =>
              d.removeFiles.includes(p),
            ) &&
            changeIntent.files.every((p) => d.files.includes(p))
              ? `${d.files.length + d.removeFiles.length} ${d.files.length + d.removeFiles.length === 1 ? "Datei" : "Dateien"} aus der Vorschau ausgewählt${d.removeFiles.length ? ` · ${d.removeFiles.length} ${d.removeFiles.length === 1 ? "Entfernung" : "Entfernungen"} vorgemerkt` : ""}`
              : "Auswahl gegenüber der Vorschau geändert · Umfang erneut prüfen"}
          </strong>
          <p>
            {!d.inspection
              ? "Aktuelle Zielprüfung steht noch aus. Keine Dateientscheidung wurde vorweggenommen."
              : d.needsInspection
                ? "Erneute Zielprüfung erforderlich."
                : "Aktuelle Zielaufnahme vorhanden. Die Unterschiede und jede Dateientscheidung vor der Übernahme prüfen."}
          </p>
        </section>
      )}
      <nav aria-label="Vorbereitungsschritte" className="recovery-steps">
        <p className="recovery-progress" role="status">
          Schritt {d.step} von 5
        </p>
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
              data-step-state={
                d.step === i + 1
                  ? "current"
                  : d.step > i + 1
                    ? "previous"
                    : "upcoming"
              }
            >
              <span>{i + 1}</span>
              <span className="recovery-step-label">
                {label}
                {i === 4 && <small>Im gespeicherten Plan</small>}
              </span>
            </li>
          ))}
        </ol>
        <p className="field-hint recovery-step-next">
          Schritt 5: Ausführen und nachprüfen – im gespeicherten Plan.
        </p>
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
                  ["remove", "Dateien entfernen"],
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
            {!!removals.length && (
              <section
                className="removal-cards"
                aria-label="Dateientfernungen bestätigen"
              >
                <h3>
                  {removals.length} Datei{removals.length === 1 ? "" : "en"}{" "}
                  würde{removals.length === 1 ? "" : "n"} gelöscht
                </h3>
                <p>
                  Vor dem Löschen werden Inhalt und Eigenschaften im
                  Rücksetzjournal gesichert. Eine spätere Rücksetzung stellt
                  fehlende Dateien wieder her und überschreibt keine
                  zwischenzeitlich neu angelegten Dateien.
                </p>
                <RemovalFiles paths={removals} />
                <label className="check">
                  <input
                    type="checkbox"
                    checked={d.removalsConfirmed}
                    onChange={(e) => d.setRemovalsConfirmed(e.target.checked)}
                  />
                  Ich bestätige die aufgeführten Dateientfernungen und habe den
                  aktuellen Zielvergleich geprüft.
                </label>
              </section>
            )}
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
                Anker speichert deine Bestätigung, kann diese Angaben aber nicht
                selbst prüfen. Netzwerk, Cluster und Storage musst du getrennt
                einrichten.
              </p>
            )}
            {!confirmationsReady &&
              (d.inspection?.requires_console ||
                d.inspection?.requires_source_offline) && (
                <p className="notice">
                  Konsolenzugang und erforderliche Quellisolierung vor Ort
                  prüfen und hier bestätigen.
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
