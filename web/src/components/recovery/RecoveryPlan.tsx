import { useListPageSize } from "../../listPageSize";
import RecoveryOutcomeCards from "./RecoveryOutcomeCards";
import RemovalFiles from "./RemovalFiles";
import { useEffect, useRef, useState } from "react";
import { api, ApiError, labels } from "../../api";
import type {
  FileComparison,
  Job,
  Plan,
  RecoveryTask,
  Status,
  User,
} from "../../api";
import { Download, Field, Pagination, State } from "../shared";
import type { Notify } from "../shared";
import FileComparisonView from "../FileComparison";
import { SaveRecoveryRecipe } from "./RecoveryRecipes";
import RecoveryEvidence from "./RecoveryEvidence";
import RecoveryChecks from "./RecoveryChecks";

export interface RecoveryOperation {
  pendingJob: string;
  uncertain: boolean;
  reconcileRefused?: boolean;
  interruptedJournal?: string;
}
// Status omits inventory details and is encoded through a map, whose key order
// differs from individual plan responses. Compare the same public projection.
function planStatusSignature(plan?: Plan) {
  if (!plan) return "";
  return JSON.stringify(
    {
      ...plan,
      source: { ...plan.source, details: undefined, query_states: undefined },
      target: { ...plan.target, details: undefined, query_states: undefined },
    },
    (_key, value) =>
      value && typeof value === "object" && !Array.isArray(value)
        ? Object.fromEntries(
            Object.keys(value)
              .sort()
              .map((key) => [key, value[key]]),
          )
        : value,
  );
}
export default function RecoveryPlan({
  plan,
  status,
  user,
  notify,
  refresh,
  onRevise,
  onClose,
  setPlan,
  onBusyChange,
  operation,
  onOperationChange,
  onDirtyChange,
}: {
  plan: Plan;
  status: Status;
  user: User;
  notify: Notify;
  refresh: () => void;
  onRevise?: (legacy: boolean) => void;
  onClose: () => void;
  setPlan: (plan: Plan) => void;
  onBusyChange: (busy: boolean) => void;
  operation: RecoveryOperation;
  onOperationChange: (operation: RecoveryOperation) => void;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const { pendingJob } = operation;
  const latestSignature = planStatusSignature(
    status.plans.find((p) => p.id === plan.id),
  );
  const [acknowledgedStatus, setAcknowledgedStatus] = useState(latestSignature);
  const trackedJob = pendingJob && status.jobs.find((j) => j.id === pendingJob);
  const finishedJob =
    !!trackedJob && !["queued", "running"].includes(trackedJob.state);
  // A mutation response already contains the saved plan. An identical status
  // poll does not require a second read or briefly disable the next task.
  const refreshNeeded =
    (acknowledgedStatus !== latestSignature &&
      planStatusSignature(plan) !== latestSignature) ||
    finishedJob;
  const [refreshError, setRefreshError] = useState("");
  const journalState = plan.result?.state;
  const journalConflict =
    !!journalState &&
    journalState !== plan.state &&
    !(plan.state === "checks_pending" && journalState === "applied") &&
    !(
      status.demo &&
      plan.state === "demo_applied" &&
      journalState === "applied"
    );
  const uncertain =
    refreshNeeded ||
    operation.uncertain ||
    journalConflict ||
    ["interrupted", "unknown"].includes(plan.state) ||
    ["interrupted", "unknown", "writing", "not_found"].includes(
      journalState || "",
    );
  const rollbackUncertain =
    uncertain &&
    !(
      !refreshNeeded &&
      !operation.uncertain &&
      !journalConflict &&
      plan.state === "interrupted" &&
      journalState === "interrupted" &&
      plan.result?.operation_id === plan.id &&
      !!plan.result?.rollback_path &&
      operation.interruptedJournal === JSON.stringify(plan.result)
    );
  const reconcileRefused =
    !!operation.reconcileRefused && plan.state === "ready";
  const legacy = plan.schema_version !== 2;
  const targetBusy = status.jobs.some(
    (j) =>
      j.host_id === plan.target_id && ["queued", "running"].includes(j.state),
  );
  const [rollbackConfirmation, setRollbackConfirmation] = useState("");
  const [rollbackOpen, setRollbackOpen] = useState(false);
  const [deletionsConfirmed, setDeletionsConfirmed] = useState(false);
  const hasDeletions = plan.steps.some((s) => s.action === "remove");
  const [confirmation, setConfirmation] = useState("");
  useEffect(() => {
    setDeletionsConfirmed(false);
  }, [latestSignature]);
  const [outcomeError, setOutcomeError] = useState("");
  const [outcomeChecking, setOutcomeChecking] = useState(false);
  const [formError, setFormError] = useState("");
  const [busy, setBusy] = useState(false);
  const [checksDirty, setChecksDirty] = useState(false);
  const [leavePrompt, setLeavePrompt] = useState(false);
  const dirty = !!confirmation || !!rollbackConfirmation || checksDirty;
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);
  const activeRequest = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    onBusyChange(busy);
    return () => onBusyChange(false);
  }, [busy, onBusyChange]);
  useEffect(() => {
    if (!busy && !dirty) return;
    const protect = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [busy, dirty]);
  function back() {
    if (activeRequest.current || busy) return;
    if (dirty) {
      setLeavePrompt(true);
      return;
    }
    onClose();
  }
  const refreshGeneration = useRef(0);
  const refreshKey =
    latestSignature + (finishedJob ? `|finished:${pendingJob}` : "");
  const currentRefreshKey = useRef(refreshKey);
  currentRefreshKey.current = refreshKey;
  const refreshInFlight = useRef<{ key: string; generation: number } | null>(
    null,
  );
  useEffect(() => {
    if (activeRequest.current || !refreshNeeded) return;
    if (
      refreshInFlight.current?.key === refreshKey &&
      refreshInFlight.current.generation === refreshGeneration.current
    )
      return;
    const generation = ++refreshGeneration.current;
    refreshInFlight.current = { key: refreshKey, generation };
    api<Plan>("plans/" + plan.id)
      .then((p) => {
        if (
          !mounted.current ||
          generation !== refreshGeneration.current ||
          currentRefreshKey.current !== refreshKey
        )
          return;
        setPlan(p);
        setAcknowledgedStatus(latestSignature);
        setRefreshError("");
        if (finishedJob) {
          onOperationChange({ ...operation, pendingJob: "" });
          setDeletionsConfirmed(false);
          setConfirmation("");
        }
      })
      .catch((e) => {
        if (
          mounted.current &&
          generation === refreshGeneration.current &&
          currentRefreshKey.current === refreshKey
        )
          setRefreshError(e.message);
      })
      .finally(() => {
        if (refreshInFlight.current?.generation === generation)
          refreshInFlight.current = null;
      });
    // Equivalent status polls retry a failed read but retain a current in-flight read.
    // Only a semantic signature/job change, mutation, or unmount supersedes its result.
  }, [latestSignature, status.jobs, pendingJob, busy]);
  async function execute(action: "apply" | "rollback" | "reconcile") {
    if (
      !plan ||
      activeRequest.current ||
      refreshNeeded ||
      (action === "apply" &&
        (legacy ||
          !!plan.blockers?.length ||
          confirmation !== plan.id ||
          (hasDeletions && !deletionsConfirmed) ||
          plan.state !== "ready")) ||
      (action === "rollback" && rollbackConfirmation !== plan.id) ||
      pendingJob ||
      (action !== "reconcile" &&
        ((action === "rollback" ? rollbackUncertain : uncertain) || targetBusy))
    )
      return;
    refreshGeneration.current++;
    activeRequest.current = true;
    onBusyChange(true);
    setBusy(true);
    setFormError("");
    const id = plan.id;
    let jobAccepted = false;
    let acceptedJob = pendingJob;
    try {
      if (action === "reconcile") {
        const p = await api<Plan>("plans/" + id + "/reconcile", "POST");
        if (!mounted.current) return;
        setPlan(p);
        onOperationChange({ pendingJob: "", uncertain: true });
        if (
          p.state === "interrupted" &&
          p.result?.state === "interrupted" &&
          p.result.operation_id === id &&
          p.result.rollback_path
        )
          onOperationChange({
            pendingJob: "",
            uncertain: false,
            interruptedJournal: JSON.stringify(p.result),
          });
        if (
          [
            "applied",
            "demo_applied",
            "checks_pending",
            "rolled_back",
            "failed",
            "rollback_conflict",
          ].includes(p.state)
        )
          onOperationChange({ pendingJob: "", uncertain: false });
      } else {
        const job = await api<Job>("plans/" + id + "/" + action, "POST", {
          confirmation:
            action === "apply" ? confirmation : rollbackConfirmation,
        });
        jobAccepted = true;
        acceptedJob = job.id;
        if (!mounted.current) return;
        onOperationChange({ pendingJob: job.id, uncertain: false });
        setConfirmation("");
        setRollbackConfirmation("");
        setRollbackOpen(false);
        setPlan(await api<Plan>("plans/" + id));
        notify(
          action === "apply"
            ? "Dateiübernahme beauftragt. Hostnachweis abwarten."
            : "Kontrollierte Rücksetzung beauftragt.",
        );
      }
      refresh();
    } catch (e) {
      if (mounted.current) {
        setFormError((e as Error).message);
        const rejected =
          !jobAccepted &&
          e instanceof ApiError &&
          e.status !== undefined &&
          e.status >= 400 &&
          e.status < 500;
        if (action === "reconcile")
          onOperationChange({
            ...operation,
            uncertain: true,
            interruptedJournal: undefined,
          });
        if (action !== "reconcile" && !rejected)
          onOperationChange({ pendingJob: acceptedJob, uncertain: true });
        if (
          action === "reconcile" &&
          plan.state === "ready" &&
          uncertain &&
          (e as Error).message ===
            "Für diesen Plan ist keine ungeklärte Hostausführung gespeichert"
        )
          onOperationChange({
            ...operation,
            uncertain: true,
            reconcileRefused: true,
          });
        if (rejected) refresh();
      }
    } finally {
      activeRequest.current = false;
      if (mounted.current) {
        setBusy(false);
        onBusyChange(false);
      }
    }
  }
  const currentPlanID = useRef(plan.id);
  currentPlanID.current = plan.id;
  async function verifyRecovery(
    endpoint: "verify" | "verify-files" = "verify",
  ) {
    if (activeRequest.current || uncertain || pendingJob || targetBusy) return;
    const id = plan.id;
    refreshGeneration.current++;
    activeRequest.current = true;
    onBusyChange(true);
    setBusy(true);
    setFormError("");
    if (endpoint === "verify-files") {
      setOutcomeError("");
      setOutcomeChecking(true);
    }
    try {
      const updated = await api<Plan>(`plans/${id}/${endpoint}`, "POST", {});
      if (updated.id !== id)
        throw new Error("Prüfantwort gehört zu einem anderen Plan.");
      if (mounted.current && currentPlanID.current === id) {
        setPlan(updated);
        refresh();
      }
    } catch (e) {
      if (mounted.current && currentPlanID.current === id) {
        if (endpoint === "verify-files")
          setOutcomeError(
            "Erneute Zielprüfung nicht abgeschlossen: " + (e as Error).message,
          );
        setFormError((e as Error).message);
        refresh();
      }
    } finally {
      activeRequest.current = false;
      if (mounted.current && currentPlanID.current === id) {
        setOutcomeChecking(false);
        setBusy(false);
        onBusyChange(false);
      }
    }
  }
  async function confirmTask(task: RecoveryTask, evidence: string) {
    if (activeRequest.current || uncertain || pendingJob || targetBusy)
      return false;
    refreshGeneration.current++;
    activeRequest.current = true;
    onBusyChange(true);
    setBusy(true);
    setFormError("");
    try {
      const updated = await api<Plan>(
        `plans/${plan.id}/checks/${encodeURIComponent(task.id)}`,
        "POST",
        { evidence },
      );
      if (mounted.current) {
        setPlan(updated);
        refresh();
      }
      return true;
    } catch (e) {
      if (mounted.current) {
        setFormError((e as Error).message);
        refresh();
      }
      return false;
    } finally {
      activeRequest.current = false;
      if (mounted.current) {
        setBusy(false);
        onBusyChange(false);
      }
    }
  }
  return (
    <section
      className="recovery-workspace recovery-plan-page"
      data-surface-topic="recovery"
      aria-label="Wiederherstellungsplan"
      aria-busy={busy}
    >
      <button className="back-button" onClick={back} disabled={busy}>
        <span aria-hidden="true">←</span> Zurück zu Plänen
      </button>
      <header className="page-heading">
        <div>
          <h1>Wiederherstellungsplan</h1>
          <p>
            Gespeicherter Plan · <code>{plan.id}</code> ·{" "}
            {labels[plan.scenario]}
          </p>
        </div>
      </header>
      {leavePrompt && (
        <div className="notice warning" role="alert">
          <p>Eingaben auf dieser Planseite sind noch offen.</p>
          <div className="comparison-actions">
            <button className="secondary" onClick={() => setLeavePrompt(false)}>
              Weiter bearbeiten
            </button>
            <button
              disabled={busy}
              onClick={() => {
                if (activeRequest.current || busy) return;
                onDirtyChange?.(false);
                onClose();
              }}
            >
              Eingaben verwerfen
            </button>
          </div>
        </div>
      )}
      {plan.source.host_type === "pbs" && (
        <p className="notice" role="status">
          PBS-Konfigurationsplan: Datastore-Inhalte und Ziel-Anbindungen bleiben
          erhalten. PBS-Dienste bleiben während Übernahme und Rücksetzung
          gestoppt. Zurückgespielte Jobs sind in der aufbereiteten Fassung
          deaktiviert; alle bestehenden Jobs vor einem späteren Dienststart
          separat prüfen.
        </p>
      )}
      <section className="recovery-plan-summary" aria-label="Planübersicht">
        <dl className="recovery-context">
          <div>
            <dt>Quelle</dt>
            <dd>
              {plan.source.hostname}
              <small>
                Proxmox{" "}
                {plan.source.pbs_version ||
                  plan.source.pve_version ||
                  "Unbekannt"}
              </small>
            </dd>
          </div>
          <div>
            <dt>Ziel</dt>
            <dd>
              {plan.target.hostname}
              <small>
                Proxmox{" "}
                {plan.target.pbs_version ||
                  plan.target.pve_version ||
                  "Unbekannt"}
              </small>
            </dd>
          </div>
          <div>
            <dt>Planstatus</dt>
            <dd>
              <State value={rollbackUncertain ? "unknown" : plan.state} />
            </dd>
          </div>
          <div>
            <dt>Dateiaktionen</dt>
            <dd>
              {plan.steps.filter((step) => step.action === "apply").length}{" "}
              Dateien übernehmen ·{" "}
              {plan.steps.filter((step) => step.action === "remove").length}{" "}
              Entfernungen ·{" "}
              {plan.steps.filter((step) => step.action === "keep").length} Ziel
              behalten ·{" "}
              {plan.steps.filter((step) => step.action === "manual").length}{" "}
              manuelle Dateien
            </dd>
          </div>
        </dl>
        <div className="plan-next-task">
          <strong>Nächste Aufgabe</strong>
          <p>
            {plan.tasks?.find((task) => task.state !== "user_confirmed")
              ?.title ||
              (plan.state === "ready"
                ? "Dateiübernahme prüfen und gesondert bestätigen"
                : "Planstatus und Hostnachweise prüfen")}
          </p>
        </div>
      </section>
      {journalState && (
        <p className="field-hint">
          Hostjournal: {labels[journalState] || journalState} · Serverplan:{" "}
          {labels[plan.state] || plan.state}
        </p>
      )}
      <nav className="plan-section-nav" aria-label="Planabschnitte">
        <a href="#plan-evidence">Nachweise</a>
        <a href="#plan-files">Dateien</a>
        <a href="#plan-tasks">Aufgaben</a>
      </nav>
      {plan.blockers?.map((s, i) => (
        <p className="notice warning" key={i}>
          {s}
        </p>
      ))}
      <h3 className="recovery-section-title">Dateiaktionen und Vergleiche</h3>
      {legacy && (
        <p className="notice warning">
          Alter Plan: Für eine neue Freigabe ist eine neue Vorbereitung
          erforderlich. Lesen, Export und bestehende Rücksetzung bleiben
          verfügbar.
        </p>
      )}
      <p className="field-hint">
        Vergleiche verwenden die gespeicherte Zielaufnahme. Vor der Übernahme
        prüft Anker erneut auf Änderungen am Ziel.
      </p>
      <RecoveryOutcomeCards
        plan={plan}
        checking={outcomeChecking}
        refreshError={outcomeError}
        disabled={busy || !!pendingJob || uncertain || targetBusy}
        onVerify={
          user.role !== "reader"
            ? () => verifyRecovery("verify-files")
            : undefined
        }
      />
      <section id="plan-evidence" aria-label="Plannachweise">
        <h3>Nachweise und Hinweise</h3>
        {!!plan.preserved?.length && (
          <RecoveryEvidence
            title="Erhaltene Systemteile"
            items={plan.preserved.map((p) => `${p.path} · ${p.reason}`)}
          />
        )}
        {!!plan.manual?.length && (
          <RecoveryEvidence
            title="Manuelle Schritte"
            items={plan.manual}
            ordered
          />
        )}
        {targetBusy && (
          <p className="notice" role="status">
            Am Ziel läuft bereits ein Auftrag. Nach dessen Abschluss sind
            Übernahme und Rücksetzung wieder verfügbar.
          </p>
        )}
        {pendingJob && (
          <p className="notice" role="status">
            Auftrag läuft. Datei- und Hostnachweis werden nach Abschluss
            geladen.
          </p>
        )}
        {plan.result && (
          <div className="notice recovery-result">
            <p>
              {plan.result.applied?.length || 0} Dateien im Übernahmenachweis.{" "}
              {plan.state === "rolled_back" && !uncertain
                ? "Rücksetzung laut Hostjournal abgeschlossen."
                : "Betriebsprüfung bleibt ausstehend."}
            </p>
            {plan.result.operation_id && (
              <p>
                Recovery-ID:{" "}
                <span className="mono">{plan.result.operation_id}</span>
              </p>
            )}
            {plan.result.rollback_path && (
              <p>
                Rollback:{" "}
                <span className="mono">{plan.result.rollback_path}</span>
              </p>
            )}
            {plan.result.error && <p role="alert">{plan.result.error}</p>}
            <p>
              Ein vollständiger Host-Restore und ein geprüfter Neustart sind
              damit nicht bestätigt.
            </p>
            <p className="field-hint">
              {status.demo
                ? "Herkunft: lokales Demoresultat; kein realer Hostnachweis."
                : "Herkunft der Hostnachweise: maschinelles Hostresultat / Journal, getrennt von Benutzerbestätigungen."}
            </p>
            {!!plan.result.checks?.length && (
              <RecoveryEvidence
                key={plan.id + "checks"}
                title="Hostnachweise"
                items={plan.result.checks}
              />
            )}
          </div>
        )}
      </section>
      <div id="plan-files">
        <PlanFiles key={plan.id} plan={plan} user={user} />
      </div>
      <div id="plan-tasks">
        <RecoveryChecks
          plan={plan}
          onVerify={user.role !== "reader" ? verifyRecovery : undefined}
          onConfirm={user.role !== "reader" ? confirmTask : undefined}
          disabled={busy || !!pendingJob || uncertain || targetBusy}
          onDirtyChange={setChecksDirty}
        />
      </div>
      {(uncertain ||
        ["interrupted", "failed", "unknown", "rollback_conflict"].includes(
          plan.state,
        )) && (
        <p className="notice warning">
          {uncertain && !rollbackUncertain
            ? "Hostjournal abgeglichen: Ausführung unterbrochen. Kontrollierte Rücksetzung separat bestätigen; eine erneute Übernahme bleibt gesperrt."
            : "Den aktuellen Hostzustand über das Journal abgleichen. Eine unklare Übernahme wird nicht automatisch wiederholt."}
        </p>
      )}
      {reconcileRefused && (
        <p className="notice warning" role="status">
          Über die Zielkonsole das Hostjournal zur Recovery-ID {plan.id} und
          laufende Anker-Aufträge prüfen. Der Server speichert keine ungeklärte
          Ausführung. Dieser Plan bleibt für eine zweite Übernahme gesperrt,
          solange Server und Journal keinen eindeutigen Zustand liefern. Ein
          erneuter Abgleich wird verfügbar, sobald sich der Serverzustand
          ändert.
        </p>
      )}
      {hasDeletions && plan.state === "ready" && (
        <section
          className="removal-cards"
          aria-label="Löschumfang vor Ausführung"
        >
          <h3>Diese Dateien werden gelöscht</h3>
          <p>
            Vorherige Inhalte und Eigenschaften werden für die kontrollierte
            Rücksetzung gesichert. Nachträglich neu angelegte Dateien bleiben
            bei einer Rücksetzung erhalten.
          </p>
          <RemovalFiles
            paths={plan.steps
              .filter((s) => s.action === "remove")
              .map((s) => s.path)}
          />
          {user.role !== "reader" && (
            <label className="check">
              <input
                type="checkbox"
                checked={deletionsConfirmed}
                onChange={(e) => setDeletionsConfirmed(e.target.checked)}
              />
              Diese Dateien nach vorheriger Sicherung ausdrücklich löschen
            </label>
          )}
        </section>
      )}
      {!legacy &&
        plan.state === "ready" &&
        user.role !== "reader" &&
        !pendingJob && (
          <Field label="Plan-ID zur Ausführung eingeben" hint={plan.id}>
            <input
              autoComplete="off"
              disabled={busy}
              value={confirmation}
              onChange={(e) => setConfirmation(e.target.value)}
              placeholder={plan.id}
            />
          </Field>
        )}
      {rollbackOpen && (
        <section className="recovery-rollback">
          <p className="notice warning">
            Nur unveränderte Dateien aus dieser Übernahme werden kontrolliert
            zurückgesetzt. Zwischenzeitliche Fremdänderungen können die
            Rücksetzung blockieren.
          </p>
          <Field label="Plan-ID zur Rücksetzung eingeben" hint={plan.id}>
            <input
              autoComplete="off"
              disabled={busy}
              value={rollbackConfirmation}
              onChange={(e) => setRollbackConfirmation(e.target.value)}
              placeholder={plan.id}
            />
          </Field>
          <button
            disabled={
              busy ||
              !!pendingJob ||
              rollbackUncertain ||
              targetBusy ||
              rollbackConfirmation !== plan.id
            }
            onClick={() => execute("rollback")}
          >
            Rücksetzung bestätigen
          </button>
        </section>
      )}
      {refreshNeeded && (
        <p className="notice warning" role="status">
          Aktueller Serverplan wird nachgeladen. Übernahme, Rücksetzung und
          Aufgabenbestätigung bleiben bis zum verlässlichen Nachweis gesperrt.
        </p>
      )}
      {refreshError && (
        <p className="notice warning" role="alert">
          {refreshError}
        </p>
      )}
      {formError && (
        <p className="notice warning" role="alert">
          {formError}
        </p>
      )}
      {!legacy && user.role !== "reader" && (
        <SaveRecoveryRecipe
          key={plan.id}
          planID={plan.id}
          mapping={plan.mapping}
          disabled={busy || uncertain || !!pendingJob || targetBusy}
        />
      )}
      <footer className="dialog-footer">
        {onRevise &&
          user.role !== "reader" &&
          !uncertain &&
          !pendingJob &&
          ["ready", "blocked", "manual"].includes(plan.state) && (
            <button
              className="secondary"
              disabled={busy}
              onClick={() => onRevise(legacy)}
            >
              {legacy ? "Neu vorbereiten" : "Zuordnungen anpassen"}
            </button>
          )}
        {user.secrets && (
          <Download path={"plans/" + plan.id + "/download"} notify={notify}>
            Plan herunterladen
          </Download>
        )}
        {!status.demo &&
          user.role !== "reader" &&
          (uncertain ||
            ["interrupted", "failed", "unknown", "rollback_conflict"].includes(
              plan.state,
            )) && (
            <button
              disabled={
                busy || !!pendingJob || reconcileRefused || refreshNeeded
              }
              onClick={() => execute("reconcile")}
            >
              Hostzustand abgleichen
            </button>
          )}
        {(!status.demo || !!plan.remove_files?.length) &&
          user.role !== "reader" &&
          !rollbackOpen &&
          !!plan.result?.rollback_path &&
          [
            "applied",
            "demo_applied",
            "checks_pending",
            "interrupted",
            "failed",
            "rollback_conflict",
          ].includes(plan.state) && (
            <button
              className="secondary"
              disabled={busy || !!pendingJob || rollbackUncertain || targetBusy}
              onClick={() => {
                setRollbackOpen(true);
                setRollbackConfirmation("");
                setConfirmation("");
              }}
            >
              Dateien zurücksetzen
            </button>
          )}
        {!legacy && plan.state === "ready" && user.role !== "reader" && (
          <button
            disabled={
              confirmation !== plan.id ||
              (hasDeletions && !deletionsConfirmed) ||
              !!plan.blockers?.length ||
              busy ||
              !!pendingJob ||
              uncertain ||
              targetBusy
            }
            onClick={() => execute("apply")}
          >
            {busy
              ? "Startet …"
              : status.demo
                ? "In Demo anwenden"
                : "Dateien übernehmen"}
          </button>
        )}
      </footer>
    </section>
  );
}

function PlanFiles({ plan, user }: { plan: Plan; user: User }) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [listSize, setListSize] = useListPageSize("recovery-plan-files", 25);
  const [path, setPath] = useState("");
  const [basis, setBasis] = useState("target");
  const [comparison, setComparison] = useState<FileComparison | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [reveal, setReveal] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true;
    setComparison(null);
    setError("");
    if (!path) {
      setLoading(false);
      return;
    }
    setLoading(true);
    api<FileComparison>(
      `plans/${encodeURIComponent(plan.id)}/comparison?path=${encodeURIComponent(path)}&basis=${basis}${reveal ? "&reveal=1" : ""}`,
    )
      .then((c) => {
        if (active) setComparison(c);
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
  }, [plan.id, path, basis, reveal, retry]);
  const matches = plan.steps.filter((s) =>
    s.path.toLowerCase().includes(query.trim().toLowerCase()),
  );
  const current = Math.min(
    page,
    Math.max(1, Math.ceil(matches.length / listSize)),
  );
  function select(next: string) {
    if (next === path && !reveal) return;
    setComparison(null);
    setReveal(false);
    setPath(next);
  }
  return (
    <section className="plan-files" aria-label="Plandateien">
      <Field label="Plandateien durchsuchen">
        <input
          placeholder="Dateipfad suchen"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(1);
          }}
        />
      </Field>
      {matches
        .slice((current - 1) * listSize, current * listSize)
        .map((step) => (
          <article className="recovery-plan-file" key={step.path}>
            <div className="recovery-file-heading">
              <strong className="mono">{step.path}</strong>
              <span>
                {step.action === "keep"
                  ? "Ziel behalten · kein Schreiben"
                  : step.action === "remove"
                    ? "Datei löschen · rücksetzbar"
                    : step.action === "apply"
                      ? "Datei übernehmen"
                      : "Manuell · keine automatische Übernahme"}
              </span>
            </div>
            <p className="field-hint">
              Entscheidung:{" "}
              {step.decision === "backup"
                ? "Originalsicherung"
                : step.decision === "remove"
                  ? "Ausdrücklich entfernen"
                  : step.decision === "prepared"
                    ? "Vorbereitete Datei"
                    : step.decision === "keep"
                      ? "Ziel behalten"
                      : "Manuelle Referenz"}{" "}
              · Zielpfad: {step.destination || step.path}
            </p>
            {step.reason && <p>{step.reason}</p>}
            {!!step.effects?.length && (
              <RecoveryEvidence
                title={`Anpassungen an ${step.path}`}
                items={step.effects.map(
                  (e) =>
                    `${e.path} · ${e.field}: ${e.before || "(leer)"} → ${e.after || "(leer)"} · ${e.reason}`,
                )}
              />
            )}
            {/^etc\/network\//.test(step.path) && (
              <p className="notice warning">
                Netzwerkaktivierung und Erreichbarkeit separat an der Konsole
                prüfen. Anker aktiviert keine Netzwerkkonfiguration automatisch.
              </p>
            )}
            {/^etc\/pve\//.test(step.path) && (
              <p className="notice warning">
                Clusterzuordnung, Quorum und Gastdatenträger separat prüfen.
                Gleiche Speicher-IDs belegen keine verfügbaren Volumes; kein
                Anlegen, Formatieren oder Verschieben.
              </p>
            )}
            {step.path === "etc/fstab" && (
              <p className="notice warning">
                Root, Boot und Swap des Ziels erhalten; weitere Mounts manuell
                prüfen.
              </p>
            )}
            {plan.source.pve_version !== plan.target.pve_version && (
              <p className="notice warning">
                Quelle {plan.source.pve_version} → Ziel{" "}
                {plan.target.pve_version}: Versionsverträglichkeit für diese
                Datei manuell prüfen.
              </p>
            )}
            {user.role !== "reader" && (
              <button
                className="text-button"
                aria-pressed={path === step.path}
                onClick={() => select(step.path)}
              >
                Vergleich öffnen: {step.path}
              </button>
            )}
            {path === step.path && (
              <div className="recovery-plan-comparison">
                <Field label="Vergleichsbasis">
                  <select
                    value={basis}
                    onChange={(e) => {
                      setBasis(e.target.value);
                      setComparison(null);
                      setReveal(false);
                    }}
                  >
                    <option value="target">
                      Aufgenommenes Ziel → vorbereitet / behalten
                    </option>
                    <option value="source">
                      Originalsicherung → vorbereitet / behalten
                    </option>
                    <option value="source-target">
                      Originalsicherung → aufgenommenes Ziel
                    </option>
                  </select>
                </Field>
                <FileComparisonView
                  comparison={comparison}
                  loading={loading}
                  error={error}
                  retry={() => {
                    setReveal(false);
                    setRetry((n) => n + 1);
                  }}
                />
                {user.secrets && comparison?.masked && (
                  <button
                    className="secondary"
                    onClick={() => {
                      setComparison(null);
                      setReveal(true);
                    }}
                  >
                    Geschützten Inhalt anzeigen
                  </button>
                )}
                {reveal && (
                  <button
                    className="secondary"
                    onClick={() => {
                      setComparison(null);
                      setReveal(false);
                    }}
                  >
                    Inhalt wieder schützen
                  </button>
                )}
              </div>
            )}
          </article>
        ))}
      {!matches.length && <p className="muted">Keine passenden Plandateien.</p>}
      <Pagination
        page={current}
        pageSize={listSize}
        onPageSizeChange={setListSize}
        total={matches.length}
        onPageChange={setPage}
        label="Plandateien"
      />
    </section>
  );
}
