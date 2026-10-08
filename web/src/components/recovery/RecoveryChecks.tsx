import { useMemo, useState } from "react";
import { date } from "../../api";
import type { Plan, RecoveryTask } from "../../api";
import { Field, Pagination } from "../shared";

// Stable topological order keeps prerequisites ahead of their dependants.
function orderedTasks(tasks: RecoveryTask[]) {
  const byID = new Map(tasks.map((t) => [t.id, t]));
  const seen = new Set<string>();
  const result: RecoveryTask[] = [];
  function visit(task: RecoveryTask) {
    if (seen.has(task.id)) return;
    seen.add(task.id);
    for (const id of task.requires || []) {
      const dependency = byID.get(id);
      if (dependency) visit(dependency);
    }
    result.push(task);
  }
  tasks.forEach(visit);
  return result;
}
const manualKinds = new Set(
  "manual-check backing bridge cluster file-type guest-collision guest-inventory guest-mac guest-node guest-syntax guest-volume hook hostname hostname-identity legacy-address mount network network-activation network-ambiguous network-field network-method network-netmask network-port-collision network-stanza network-syntax network-topology network-value pbs pci storage storage-content storage-nodes volume".split(
    " ",
  ),
);
export default function RecoveryChecks({
  plan,
  onConfirm,
  disabled = false,
}: {
  plan: Plan;
  onConfirm?: (task: RecoveryTask, evidence: string) => void;
  disabled?: boolean;
}) {
  const [evidence, setEvidence] = useState<Record<string, string>>({});
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const tasks = useMemo(() => orderedTasks(plan.tasks || []), [plan.tasks]);
  const byID = new Map(tasks.map((t) => [t.id, t]));
  const matches = tasks.filter((t) =>
    (t.title + " " + t.instruction)
      .toLowerCase()
      .includes(query.trim().toLowerCase()),
  );
  const current = Math.min(page, Math.max(1, Math.ceil(matches.length / 25)));
  return (
    <section aria-label="Betriebsprüfungen" className="recovery-checks">
      <h3>Manuelle Aufgaben und Betriebsprüfungen</h3>
      <p className="field-hint">
        Benutzerbestätigungen dokumentieren eigene Prüfungen. Sie ersetzen keine
        maschinellen Nachweise und geben Netzwerk, Cluster oder
        Storageverwaltung nicht automatisch frei.
      </p>
      <p className="notice">
        {plan.result?.reboot_verified
          ? "Maschineller Neustartnachweis laut Hostresultat vorhanden."
          : "Neustart maschinell nicht nachgewiesen."}
      </p>
      {tasks.length > 25 && (
        <Field label="Aufgaben durchsuchen">
          <input
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setPage(1);
            }}
          />
        </Field>
      )}
      <ol start={(current - 1) * 25 + 1}>
        {matches.slice((current - 1) * 25, current * 25).map((task) => {
          const dependencies = (task.requires || []).map((id) => byID.get(id));
          const prerequisites = dependencies.every(
            (t) => t?.state === "user_confirmed",
          );
          const operationalBlocked =
            task.kind === "manual-check" &&
            [
              "ready",
              "applying",
              "interrupted",
              "failed",
              "rolling_back",
              "rollback_conflict",
              "unknown",
            ].includes(plan.state);
          const allowed =
            !disabled &&
            plan.schema_version === 2 &&
            !plan.blockers?.length &&
            task.state === "pending" &&
            manualKinds.has(task.kind) &&
            prerequisites &&
            !operationalBlocked;
          const value = evidence[task.id] || "";
          return (
            <li key={task.id} className="recovery-task">
              <strong>{task.title}</strong>
              <p>{task.instruction}</p>
              <p className="field-hint">
                Voraussetzungen:{" "}
                {dependencies.length
                  ? (task.requires || [])
                      .map(
                        (id) =>
                          `${byID.get(id)?.title || id} (${byID.get(id)?.state === "user_confirmed" ? "vom Benutzer bestätigt" : "offen"})`,
                      )
                      .join(" · ")
                  : "Keine"}
              </p>
              {task.state === "user_confirmed" ? (
                <div className="notice">
                  <p>
                    Vom Benutzer bestätigt ·{" "}
                    {task.confirmed_by || "Benutzer unbekannt"} ·{" "}
                    {date(task.confirmed_at)}
                  </p>
                  <p>{task.evidence}</p>
                  <p className="field-hint">
                    Herkunft: Benutzerbestätigung, kein maschineller Nachweis.
                  </p>
                </div>
              ) : (
                <>
                  <p className="field-hint">
                    Offen · Herkunft: manuelle Aufgabe
                    {operationalBlocked
                      ? " · Dateiübernahme und Hostzustand zuerst klären."
                      : !prerequisites
                        ? " · Voraussetzungen zuerst bestätigen."
                        : "."}
                  </p>
                  {onConfirm && (
                    <>
                      <Field label={`Nachweis für ${task.title}`}>
                        <textarea
                          disabled={!allowed}
                          maxLength={4096}
                          value={value}
                          onChange={(e) =>
                            setEvidence((old) => ({
                              ...old,
                              [task.id]: e.target.value,
                            }))
                          }
                        />
                      </Field>
                      <button
                        className="secondary"
                        disabled={
                          !allowed ||
                          !value.trim() ||
                          new TextEncoder().encode(value).length > 4096
                        }
                        onClick={() => onConfirm(task, value)}
                      >
                        Schritt bestätigen: {task.title}
                      </button>
                    </>
                  )}
                </>
              )}
            </li>
          );
        })}
      </ol>
      {tasks.length > 25 && (
        <Pagination
          page={current}
          pageSize={25}
          total={matches.length}
          onPageChange={setPage}
          label="Aufgaben"
        />
      )}
    </section>
  );
}
