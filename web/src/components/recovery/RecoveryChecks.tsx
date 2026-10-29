import { useListPageSize } from "../../listPageSize";
import { useEffect, useMemo, useState } from "react";
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
  onVerify,
  disabled = false,
  onDirtyChange,
}: {
  plan: Plan;
  onVerify?: () => Promise<void>;
  onConfirm?: (task: RecoveryTask, evidence: string) => Promise<boolean>;
  disabled?: boolean;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const [evidence, setEvidence] = useState<Record<string, string>>({});
  useEffect(() => {
    onDirtyChange?.(Object.values(evidence).some((value) => !!value.trim()));
  }, [evidence, onDirtyChange]);
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [listSize, setListSize] = useListPageSize("recovery-checks", 25);
  const tasks = useMemo(() => orderedTasks(plan.tasks || []), [plan.tasks]);
  const byID = new Map(tasks.map((t) => [t.id, t]));
  const matches = tasks.filter((t) =>
    (t.title + " " + t.instruction)
      .toLowerCase()
      .includes(query.trim().toLowerCase()),
  );
  const current = Math.min(
    page,
    Math.max(1, Math.ceil(matches.length / listSize)),
  );
  return (
    <section
      aria-label="Betriebsprüfungen"
      className="recovery-checks"
      data-surface-topic="security"
    >
      <h3>Manuelle Aufgaben und Betriebsprüfungen</h3>
      <p className="field-hint">
        Benutzerbestätigungen dokumentieren eigene Prüfungen. Sie ersetzen keine
        maschinellen Nachweise und geben Netzwerk, Cluster oder
        Storageverwaltung nicht automatisch frei.
      </p>
      <section
        aria-label="Maschinelle Prüfversuche"
        className="recovery-checks"
        data-surface-topic="security"
      >
        <h3>Maschinelle Prüfversuche</h3>
        <p className="field-hint">
          Frische lesende Abfrage am gebundenen Ziel. Dienste und aufgenommene
          Volumes werden geprüft; keine automatische Aktivierung, kein Neustart
          und keine Betriebsfreigabe.
        </p>
        {onVerify && (
          <button
            className="secondary"
            disabled={
              disabled ||
              plan.schema_version !== 2 ||
              !plan.target_snapshot_id ||
              (plan.verification_attempts?.length || 0) >= 100
            }
            onClick={() => void onVerify()}
          >
            Maschinell prüfen
          </button>
        )}
        {!plan.verification_attempts?.length && (
          <p className="notice">
            Keine frische maschinelle Prüfung vorhanden. Neustart maschinell
            nicht nachgewiesen.
          </p>
        )}
        {(plan.verification_attempts || []).map((a) => (
          <article className="recovery-task" key={a.id}>
            <strong>
              Versuch {a.id} · {verificationLabel(a.state)}
            </strong>
            <p>
              {date(a.started_at)} →{" "}
              {a.finished_at ? date(a.finished_at) : "Abschluss fehlt"} ·
              Ausgelöst von {a.actor || "unbekannt"}
            </p>
            <p>
              Identität: {verificationLabel(a.identity_state)} · Neustart:{" "}
              {verificationLabel(a.reboot_state)} · Dienste:{" "}
              {verificationLabel(a.services_state)} · Aufgenommene Volumes:{" "}
              {verificationLabel(a.volumes_state)}
            </p>
            <p>{a.reason}</p>
            <details className="recovery-disclosure">
              <summary>Herkunft und Bindung</summary>
              <p>
                Ursprung: Maschinenabfrage · Plan {a.plan_id} · Sicherung{" "}
                {a.backup_id} · Ziel {a.target_id}
              </p>
              <p>
                Quellmanifest SHA256: {a.source_manifest_sha || "unbekannt"}
              </p>
              <p>Zielaufnahme SHA256: {a.snapshot_sha || "unbekannt"}</p>
              <p>Verbindung SHA256: {a.connection_sha || "unbekannt"}</p>
              <p>
                Baseline: {date(a.baseline_captured_at)} · Maschinenhash:{" "}
                {a.baseline_identity || "unbekannt"} · Boot-ID:{" "}
                {a.baseline_boot_id || "unbekannt"}
              </p>
              {a.result && (
                <>
                  <p>
                    Abfrage: {date(a.result.observed_at)} · Protokoll{" "}
                    {a.result.protocol}
                  </p>
                  <p>
                    Aktueller Maschinenhash:{" "}
                    {a.result.identity.value || "unbekannt"} · Boot-ID:{" "}
                    {a.result.boot.value || "unbekannt"}
                  </p>
                  <ul>
                    {Object.entries(a.result.services).map(([name, q]) => (
                      <li key={name}>
                        {name}: {verificationLabel(q.state)}
                      </li>
                    ))}
                    {Object.entries(a.result.storage_volumes).map(
                      ([name, q]) => (
                        <li key={name}>
                          Storage {name}: {verificationLabel(q.state)} ·{" "}
                          {q.volumes?.length || 0} gelistete Volumes
                        </li>
                      ),
                    )}
                  </ul>
                </>
              )}
            </details>
          </article>
        ))}
      </section>
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
        {matches
          .slice((current - 1) * listSize, current * listSize)
          .map((task) => {
            const dependencies = (task.requires || []).map((id) =>
              byID.get(id),
            );
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
                            onChange={(e) => {
                              // Capture the input before React restores a controlled
                              // field during a concurrent plan/status update.
                              const nextValue = e.currentTarget.value;
                              setEvidence((old) => ({
                                ...old,
                                [task.id]: nextValue,
                              }));
                            }}
                          />
                        </Field>
                        <button
                          className="secondary"
                          disabled={
                            !allowed ||
                            !value.trim() ||
                            new TextEncoder().encode(value).length > 4096
                          }
                          onClick={() => {
                            void onConfirm(task, value).then((saved) => {
                              if (saved)
                                setEvidence((old) => ({
                                  ...old,
                                  [task.id]: "",
                                }));
                            });
                          }}
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
          pageSize={listSize}
          onPageSizeChange={setListSize}
          total={matches.length}
          onPageChange={setPage}
          label="Aufgaben"
        />
      )}
    </section>
  );
}

function verificationLabel(state: string) {
  return (
    (
      {
        passed: "nachgewiesen",
        failed: "fehlgeschlagen",
        unknown: "unbekannt",
        unsupported: "nicht unterstützt",
        interrupted: "unterbrochen",
        observed: "beobachtet",
        unchanged: "Boot-ID unverändert",
        ok: "erfolgreich",
        limit: "Limit erreicht",
      } as Record<string, string>
    )[state] || "unbekannt"
  );
}
