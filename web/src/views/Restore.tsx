import { useEffect, useRef, useState } from "react";
import { api, date, labels } from "../api";
import type { Plan, Status, User } from "../api";
import { Empty, Heading, Pagination, State } from "../components/shared";
import type { Notify } from "../components/shared";
import RecoveryPlan from "../components/recovery/RecoveryPlan";
import type { RecoveryOperation } from "../components/recovery/RecoveryPlan";
import RecoveryWizard from "../components/recovery/RecoveryWizard";
import { useRecoveryDraft } from "../components/recovery/useRecoveryDraft";

export default function Restore({
  status,
  initialBackup,
  initialFile,
  initialPlan,
  user,
  notify,
  refresh,
  onConnectTarget,
  onBusyChange,
  onDirtyChange,
}: {
  status: Status;
  initialBackup: string;
  initialFile?: string;
  initialPlan?: { id: string; sequence: number } | null;
  user: User;
  notify: Notify;
  refresh: () => void;
  onConnectTarget: () => void;
  onBusyChange: (busy: boolean) => void;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const [wizard, setWizard] = useState(!!initialBackup);
  const draft = useRecoveryDraft(initialBackup, initialFile);
  const selectedTarget = status.hosts.find((h) => h.id === draft.target);
  const connection = JSON.stringify(
    selectedTarget
      ? [
          selectedTarget.address,
          selectedTarget.ssh_port,
          selectedTarget.ssh_user,
          selectedTarget.known_hosts_path,
          selectedTarget.key_path,
          selectedTarget.restore_ssh_user,
          selectedTarget.restore_key_path,
        ]
      : [],
  );
  const previousConnection = useRef({ target: draft.target, connection });
  useEffect(() => {
    const previous = previousConnection.current;
    if (
      previous.target === draft.target &&
      previous.connection !== connection
    ) {
      draft.changeTarget(draft.target);
      if (draft.step > 2) draft.setStep(2);
    }
    previousConnection.current = { target: draft.target, connection };
  }, [draft.target, connection]);
  const [operations, setOperations] = useState<
    Record<string, RecoveryOperation>
  >({});
  const activeRequest = useRef(false);
  const mounted = useRef(true);
  const [busy, setBusy] = useState(false);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [canRevise, setCanRevise] = useState(false);
  const [planQuery, setPlanQuery] = useState("");
  const [planState, setPlanState] = useState("");
  const [planPage, setPlanPage] = useState(1);
  const [formError, setFormError] = useState("");
  useEffect(() => {
    if (!initialPlan || activeRequest.current) return;
    let current = true;
    activeRequest.current = true;
    setBusy(true);
    onBusyChange(true);
    setWizard(false);
    setPlan(null);
    setFormError("");
    api<Plan>("plans/" + initialPlan.id)
      .then((opened) => {
        if (!current) return;
        if (opened.id !== initialPlan.id)
          throw new Error("Planantwort passt nicht zur Auswahl.");
        setPlan(opened);
        setCanRevise(false);
      })
      .catch((e) => {
        if (current) setFormError(e.message);
      })
      .finally(() => {
        if (current) {
          activeRequest.current = false;
          setBusy(false);
          onBusyChange(false);
        }
      });
    return () => {
      current = false;
      activeRequest.current = false;
      onBusyChange(false);
    };
  }, [initialPlan, onBusyChange]);
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
      {!wizard && !plan && (
        <>
          {formError && (
            <p className="notice warning" role="alert">
              {formError} Den Plan in der Liste erneut öffnen.
            </p>
          )}
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
            refresh();
          }}
        />
      )}
      {plan && (
        <RecoveryPlan
          key={plan.id}
          plan={plan}
          status={status}
          user={user}
          notify={notify}
          refresh={refresh}
          setPlan={setPlan}
          onClose={() => setPlan(null)}
          onBusyChange={onBusyChange}
          onDirtyChange={onDirtyChange}
          operation={
            operations[plan.id] || { pendingJob: "", uncertain: false }
          }
          onOperationChange={(operation) =>
            setOperations((old) => ({ ...old, [plan.id]: operation }))
          }
          onRevise={(legacy) => {
            if (!canRevise || legacy) {
              draft.changeBackup(plan.backup_id);
              draft.changeTarget(plan.target_id);
              draft.changeScenario(
                ["files", "host", "cluster-node", "cluster-disaster"].includes(
                  plan.scenario,
                )
                  ? plan.scenario
                  : "host",
              );
              draft.changeFiles(plan.steps.map((s) => s.path));
            }
            draft.invalidate();
            draft.setStep(legacy || !canRevise ? 1 : 2);
            setPlan(null);
            setWizard(true);
            setFormError("");
          }}
        />
      )}
    </>
  );
}
