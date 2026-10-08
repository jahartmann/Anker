import { useEffect, useRef, useState } from "react";
import { api, ApiError, date, labels } from "../api";
import type { Job, Plan, Status, User } from "../api";
import {
  Dialog,
  Download,
  Empty,
  Field,
  Heading,
  Pagination,
  State,
} from "../components/shared";
import type { Notify } from "../components/shared";
import RecoveryEvidence from "../components/recovery/RecoveryEvidence";
import RecoveryWizard from "../components/recovery/RecoveryWizard";
import { useRecoveryDraft } from "../components/recovery/useRecoveryDraft";

function PlanFiles({ steps }: { steps: Plan["steps"] }) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const matches = steps.filter((s) =>
    s.path.toLowerCase().includes(query.trim().toLowerCase()),
  );
  const current = Math.min(page, Math.max(1, Math.ceil(matches.length / 25)));
  return (
    <section className="plan-files">
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
      {matches.slice((current - 1) * 25, current * 25).map((s) => (
        <details className="file-diff" key={s.path}>
          <summary>
            <span className="mono">{s.path}</span>
            <span className="muted">
              {s.action === "apply" ? "Vorbereitet" : "Manuell"}
            </span>
          </summary>
          <p>
            {s.reason ||
              "Originalinhalt mit geprüften Vorbedingungen übernehmen."}
          </p>
          {s.diff && <pre>{s.diff}</pre>}
        </details>
      ))}
      {!matches.length && <p className="muted">Keine passenden Plandateien.</p>}
      <Pagination
        page={current}
        pageSize={25}
        total={matches.length}
        onPageChange={setPage}
        label="Plandateien"
      />
    </section>
  );
}
export default function Restore({
  status,
  initialBackup,
  initialFile,
  user,
  notify,
  refresh,
  onConnectTarget,
  onBusyChange,
}: {
  status: Status;
  initialBackup: string;
  initialFile?: string;
  user: User;
  notify: Notify;
  refresh: () => void;
  onConnectTarget: () => void;
  onBusyChange: (busy: boolean) => void;
}) {
  const [wizard, setWizard] = useState(!!initialBackup);
  const draft = useRecoveryDraft(initialBackup, initialFile);
  const selectedTarget = status.hosts.find((h) => h.id === draft.target);
  const connection = JSON.stringify(selectedTarget ? [
    selectedTarget.address, selectedTarget.ssh_port, selectedTarget.ssh_user,
    selectedTarget.known_hosts_path, selectedTarget.key_path,
    selectedTarget.restore_ssh_user, selectedTarget.restore_key_path,
  ] : []);
  const previousConnection = useRef({ target: draft.target, connection });
  useEffect(() => {
    const previous = previousConnection.current;
    if (previous.target === draft.target && previous.connection !== connection) {
      draft.changeTarget(draft.target);
      if (draft.step > 2) draft.setStep(2);
    }
    previousConnection.current = { target: draft.target, connection };
  }, [draft.target, connection]);
  const [pendingJob, setPendingJob] = useState("");
  const [uncertainPlans, setUncertainPlans] = useState<Record<string, boolean>>(
    {},
  );
  const [rollbackConfirmation, setRollbackConfirmation] = useState("");
  const [rollbackOpen, setRollbackOpen] = useState(false);
  const activeRequest = useRef(false);
  const mounted = useRef(true);
  const [busy, setBusy] = useState(false);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [canRevise, setCanRevise] = useState(false);
  const uncertain = !!plan && !!uncertainPlans[plan.id];
  const targetBusy =
    !!plan &&
    status.jobs.some(
      (j) =>
        j.host_id === plan.target_id && ["queued", "running"].includes(j.state),
    );
  const [planQuery, setPlanQuery] = useState("");
  const [planState, setPlanState] = useState("");
  const [planPage, setPlanPage] = useState(1);
  const [confirmation, setConfirmation] = useState("");
  const [formError, setFormError] = useState("");
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    onBusyChange(busy);
    if (!busy) return;
    const protect = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener("beforeunload", protect);
    return () => {
      window.removeEventListener("beforeunload", protect);
      onBusyChange(false);
    };
  }, [busy, onBusyChange]);
  useEffect(() => {
    if (!plan) return;
    const latest = status.plans.find((p) => p.id === plan.id);
    const job = pendingJob && status.jobs.find((j) => j.id === pendingJob);
    const finished = job && !["queued", "running"].includes(job.state);
    if (!finished && (!latest || latest.state === plan.state)) return;
    let active = true;
    api<Plan>("plans/" + plan.id)
      .then((p) => {
        if (!active) return;
        setPlan(p);
        if (finished) {
          setPendingJob("");
          setConfirmation("");
        }
      })
      .catch((e) => {
        if (active) setFormError((e as Error).message);
      });
    return () => {
      active = false;
    };
  }, [status.plans, status.jobs, pendingJob, plan?.id, plan?.state]);
  async function execute(action: "apply" | "rollback" | "reconcile") {
    if (
      !plan ||
      activeRequest.current ||
      pendingJob ||
      (action !== "reconcile" && (uncertain || targetBusy))
    )
      return;
    activeRequest.current = true;
    setBusy(true);
    setFormError("");
    const id = plan.id;
    let jobAccepted = false;
    try {
      if (action === "reconcile") {
        const p = await api<Plan>("plans/" + id + "/reconcile", "POST");
        if (!mounted.current) return;
        setPlan(p);
        if (
          [
            "applied",
            "checks_pending",
            "rolled_back",
            "failed",
            "rollback_conflict",
          ].includes(p.state)
        )
          setUncertainPlans((old) => ({ ...old, [id]: false }));
      } else {
        const job = await api<Job>("plans/" + id + "/" + action, "POST", {
          confirmation:
            action === "apply" ? confirmation : rollbackConfirmation,
        });
        jobAccepted = true;
        if (!mounted.current) return;
        setPendingJob(job.id);
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
        if (action !== "reconcile" && !rejected)
          setUncertainPlans((old) => ({ ...old, [id]: true }));
        if (rejected) refresh();
      }
    } finally {
      activeRequest.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  const plans = status.plans
    .filter(
      (p) =>
        (p.source.hostname + " " + p.target.hostname + " " + p.id)
          .toLowerCase()
          .includes(planQuery.trim().toLowerCase()) &&
        (!planState || p.state === planState),
    )
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
  const currentPlanPage = Math.min(
    planPage,
    Math.max(1, Math.ceil(plans.length / 25)),
  );
  return (
    <>
      {!wizard && (
        <>
          <Heading
            title="Wiederherstellung"
            description="Originale bewahren. Änderungen am Ziel zuerst prüfen."
            action={
              user.role !== "reader" && (
                <button
                  onClick={() => {
                    if (activeRequest.current) return;
                    setWizard(true);
                    setPlan(null);
                    draft.invalidate();
                    draft.setStep(1);
                    setFormError("");
                  }}
                >
                  Plan erstellen
                </button>
              )
            }
          />
          {status.plans.length ? (
            <div>
              <div className="restore-plan-filters">
                <input
                  aria-label="Wiederherstellungspläne durchsuchen"
                  placeholder="Quelle, Ziel oder Plan suchen"
                  value={planQuery}
                  onChange={(e) => {
                    setPlanQuery(e.target.value);
                    setPlanPage(1);
                  }}
                />
                <select
                  aria-label="Planstatus filtern"
                  value={planState}
                  onChange={(e) => {
                    setPlanState(e.target.value);
                    setPlanPage(1);
                  }}
                >
                  <option value="">Alle Status</option>
                  {Array.from(
                    new Set(
                      [...status.plans.map((p) => p.state), planState].filter(
                        Boolean,
                      ),
                    ),
                  )
                    .sort()
                    .map((state) => (
                      <option key={state} value={state}>
                        {labels[state] || state}
                      </option>
                    ))}
                </select>
              </div>
              <div className="table-scroll">
                <table className="restore-plans">
                  <thead>
                    <tr>
                      <th>Quelle → Ziel</th>
                      <th>Szenario</th>
                      <th>Status</th>
                      <th className="right">Aktionen</th>
                    </tr>
                  </thead>
                  <tbody>
                    {plans
                      .slice((currentPlanPage - 1) * 25, currentPlanPage * 25)
                      .map((p) => (
                        <tr key={p.id}>
                          <td>
                            <strong>
                              {p.source.hostname} → {p.target.hostname}
                            </strong>
                            <div className="secondary-line">
                              {date(p.created_at)}
                            </div>
                          </td>
                          <td>{labels[p.scenario]}</td>
                          <td>
                            <State value={p.state} />
                          </td>
                          <td className="right">
                            <button
                              className="text-button"
                              disabled={busy}
                              onClick={async () => {
                                if (activeRequest.current) return;
                                activeRequest.current = true;
                                setBusy(true);
                                try {
                                  const opened = await api<Plan>(
                                    "plans/" + p.id,
                                  );
                                  if (!mounted.current) return;
                                  setPlan(opened);
                                  setCanRevise(false);
                                  setConfirmation("");
                                  setRollbackConfirmation("");
                                  setRollbackOpen(false);
                                  setPendingJob("");
                                  setFormError("");
                                } catch (e) {
                                  if (mounted.current)
                                    notify((e as Error).message, true);
                                } finally {
                                  activeRequest.current = false;
                                  if (mounted.current) setBusy(false);
                                }
                              }}
                            >
                              Plan ansehen
                            </button>
                          </td>
                        </tr>
                      ))}
                  </tbody>
                </table>
              </div>
              {!plans.length && (
                <Empty title="Keine passenden Pläne">
                  Suche oder Statusfilter ändern.
                </Empty>
              )}
              <Pagination
                page={currentPlanPage}
                pageSize={25}
                total={plans.length}
                onPageChange={setPlanPage}
                label="Pläne"
              />
            </div>
          ) : (
            <Empty title="Noch keine Wiederherstellungspläne">
              Eine Sicherung und einen Zielhost auswählen. Anker zeigt danach
              die möglichen Schritte und offenen Entscheidungen.
            </Empty>
          )}
        </>
      )}
      {wizard && (
        <RecoveryWizard
          status={status}
          user={user}
          draft={draft}
          onClose={() => {
            draft.invalidate();
            if (draft.step > 2) draft.setStep(2);
            setWizard(false);
          }}
          onConnectTarget={onConnectTarget}
          notify={notify}
          onBusyChange={onBusyChange}
          onPlan={(p) => {
            setPlan(p);
            setCanRevise(true);
            setWizard(false);
            setConfirmation("");
            setRollbackConfirmation("");
            setRollbackOpen(false);
            setPendingJob("");
            refresh();
          }}
        />
      )}
      {plan && (
        <Dialog
          title="Wiederherstellungsplan"
          wide
          busy={busy}
          onClose={() => {
            if (!activeRequest.current) setPlan(null);
          }}
        >
          <div className="plan-meta">
            <strong>
              {plan.source.hostname} → {plan.target.hostname}
            </strong>
            <State value={plan.state} />
          </div>
          <p className="dialog-intro">
            {labels[plan.scenario]} · Quelle {plan.source.pve_version} · Ziel{" "}
            {plan.target.pve_version}
          </p>
          {plan.blockers?.map((s, i) => (
            <p className="notice warning" key={i}>
              {s}
            </p>
          ))}
          <h3 className="recovery-section-title">3 · Änderungen prüfen</h3>
          <p className="field-hint">
            {plan.steps.filter((s) => s.action === "apply").length} vorbereitete reguläre Dateien · {plan.steps.filter((s) => s.action !== "apply").length}{" "}
            manuelle Dateien
          </p>
          {plan.manual?.length > 0 && (
            <RecoveryEvidence
              key={plan.id + "manual"}
              title="Manuelle Schritte"
              items={plan.manual}
              ordered
            />
          )}
          <p className="field-hint">
            Die Dateiansicht zeigt Anpassungen an der Sicherung. Sie ist kein
            vollständiger Vergleich mit den aktuellen Dateiinhalten des
            Zielhosts.
          </p>
          <PlanFiles key={plan.id} steps={plan.steps} />
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
                {plan.state === "rolled_back"
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
              {!!plan.result.checks?.length && (
                <RecoveryEvidence
                  key={plan.id + "checks"}
                  title="Hostnachweise"
                  items={plan.result.checks}
                />
              )}
            </div>
          )}
          {(uncertain ||
            ["interrupted", "failed", "unknown", "rollback_conflict"].includes(
              plan.state,
            )) && (
            <p className="notice warning">
              Den aktuellen Hostzustand über das Journal abgleichen. Eine
              unklare Übernahme wird nicht automatisch wiederholt.
            </p>
          )}
          {plan.state === "ready" && user.role !== "reader" && !pendingJob && (
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
                Nur unveränderte Dateien aus dieser Übernahme werden
                kontrolliert zurückgesetzt. Zwischenzeitliche Fremdänderungen
                können die Rücksetzung blockieren.
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
                  uncertain ||
                  targetBusy ||
                  rollbackConfirmation !== plan.id
                }
                onClick={() => execute("rollback")}
              >
                Rücksetzung bestätigen
              </button>
            </section>
          )}
          {formError && (
            <p className="notice warning" role="alert">
              {formError}
            </p>
          )}
          <footer className="dialog-footer">
            {canRevise &&
              !uncertain &&
              !pendingJob &&
              ["ready", "blocked", "manual"].includes(plan.state) && (
                <button
                  className="secondary"
                  disabled={busy}
                  onClick={() => {
                    draft.invalidate();
                    draft.setStep(2);
                    setPlan(null);
                    setWizard(true);
                    setFormError("");
                  }}
                >
                  Zuordnungen anpassen
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
                [
                  "interrupted",
                  "failed",
                  "unknown",
                  "rollback_conflict",
                ].includes(plan.state)) && (
                <button
                  className="secondary"
                  disabled={busy || !!pendingJob}
                  onClick={() => execute("reconcile")}
                >
                  Hostzustand abgleichen
                </button>
              )}
            {!status.demo &&
              user.role !== "reader" &&
              !rollbackOpen &&
              !!plan.result?.rollback_path &&
              [
                "applied",
                "checks_pending",
                "interrupted",
                "failed",
                "rollback_conflict",
              ].includes(plan.state) && (
                <button
                  className="secondary"
                  disabled={busy || !!pendingJob || uncertain || targetBusy}
                  onClick={() => {
                    setRollbackOpen(true);
                    setRollbackConfirmation("");
                    setConfirmation("");
                  }}
                >
                  Dateien zurücksetzen
                </button>
              )}
            {plan.state === "ready" && user.role !== "reader" && (
              <button
                disabled={
                  confirmation !== plan.id ||
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
        </Dialog>
      )}
    </>
  );
}
