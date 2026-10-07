import { useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent } from "react";
import { api, ApiError, date, labels } from "../api";
import type {
  Entry,
  Job,
  Plan,
  RecoveryInspection,
  Status,
  User,
} from "../api";
import {
  Dialog,
  Download,
  Empty,
  Field,
  Heading,
  LoadError,
  Pagination,
  State,
} from "../components/shared";
import type { Notify } from "../components/shared";
import RecoveryMapping from "../components/RecoveryMapping";

function RestoreFiles({
  entries,
  files,
  onChange,
}: {
  entries: Entry[];
  files: string[];
  onChange: (files: string[]) => void;
}) {
  const [query, setQuery] = useState("");
  const [selectedOnly, setSelectedOnly] = useState(false);
  const [page, setPage] = useState(1);
  const selected = useMemo(() => new Set(files), [files]);
  const matches = useMemo(
    () =>
      entries
        .filter(
          (f) =>
            f.type !== "directory" &&
            f.path.toLowerCase().includes(query.trim().toLowerCase()) &&
            (!selectedOnly || selected.has(f.path)),
        )
        .sort((a, b) =>
          a.path.localeCompare(b.path, undefined, { numeric: true }),
        ),
    [entries, query, selectedOnly, selected],
  );
  useEffect(() => {
    setPage(1);
  }, [query, selectedOnly, entries]);
  const current = Math.min(page, Math.max(1, Math.ceil(matches.length / 25)));
  return (
    <fieldset className="file-selection restore-file-selection">
      <legend>Dateien auswählen · {files.length} ausgewählt</legend>
      <div className="restore-selection-toolbar">
        <input
          aria-label="Wiederherstellungsdateien durchsuchen"
          placeholder="Dateipfad suchen"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <button
          type="button"
          className="secondary"
          aria-pressed={selectedOnly}
          onClick={() => setSelectedOnly((v) => !v)}
        >
          Nur ausgewählte Dateien
        </button>
      </div>
      <div className="restore-file-items">
        {matches.slice((current - 1) * 25, current * 25).map((f) => (
          <label className="check" key={f.path}>
            <input
              type="checkbox"
              checked={selected.has(f.path)}
              onChange={(e) =>
                onChange(
                  e.target.checked
                    ? [...files, f.path]
                    : files.filter((p) => p !== f.path),
                )
              }
            />
            <span className="mono">{f.path}</span>
            {f.secret && <small className="muted">Geschützt</small>}
          </label>
        ))}
        {!matches.length && (
          <p className="muted">
            {selectedOnly && !files.length
              ? "Noch keine Dateien ausgewählt."
              : "Keine passenden Dateien."}
          </p>
        )}
      </div>
      <Pagination
        page={current}
        pageSize={25}
        total={matches.length}
        onPageChange={setPage}
        label="Dateien"
      />
      <p className="field-hint">
        Die Auswahl bleibt beim Suchen und Blättern erhalten.
      </p>
    </fieldset>
  );
}

function RecoveryEvidence({
  title,
  items,
  ordered = false,
}: {
  title: string;
  items: string[];
  ordered?: boolean;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const matches = useMemo(
    () =>
      items.filter((item) =>
        item.toLowerCase().includes(query.trim().toLowerCase()),
      ),
    [items, query],
  );
  const current = Math.min(page, Math.max(1, Math.ceil(matches.length / 25)));
  const rows = matches
    .slice((current - 1) * 25, current * 25)
    .map((item, i) => <li key={(current - 1) * 25 + i}>{item}</li>);
  return (
    <details
      className={
        "recovery-disclosure recovery-evidence" +
        (ordered ? " manual-steps" : "")
      }
    >
      <summary>
        {title} · {items.length}
      </summary>
      <section aria-label={title}>
        {items.length > 25 && (
          <Field label={title + " durchsuchen"}>
            <input
              value={query}
              placeholder="Text suchen"
              onChange={(e) => {
                setQuery(e.target.value);
                setPage(1);
              }}
            />
          </Field>
        )}
        {ordered ? (
          <ol start={(current - 1) * 25 + 1}>{rows}</ol>
        ) : (
          <ul>{rows}</ul>
        )}
        {!matches.length && <p className="muted">Keine passenden Einträge.</p>}
        {items.length > 25 && (
          <Pagination
            page={current}
            pageSize={25}
            total={matches.length}
            onPageChange={setPage}
            label={title}
          />
        )}
      </section>
    </details>
  );
}

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
}: {
  status: Status;
  initialBackup: string;
  initialFile?: string;
  user: User;
  notify: Notify;
  refresh: () => void;
}) {
  const [wizard, setWizard] = useState(!!initialBackup);
  const [backup, setBackup] = useState(initialBackup);
  const [sourceHost, setSourceHost] = useState(
    status.backups.find((b) => b.id === initialBackup)?.host_id || "",
  );
  const [target, setTarget] = useState("");
  const [scenario, setScenario] = useState("files");
  const [hostname, setHostname] = useState("");
  const [address, setAddress] = useState("");
  const [entries, setEntries] = useState<Entry[]>([]);
  const [files, setFiles] = useState<string[]>([]);
  const [interfaces, setInterfaces] = useState<Record<string, string>>({});
  const [storage, setStorage] = useState<Record<string, string>>({});
  const [inspection, setInspection] = useState<RecoveryInspection | null>(null);
  const [inspectionKey, setInspectionKey] = useState("");
  const [pendingJob, setPendingJob] = useState("");
  const [uncertainPlans, setUncertainPlans] = useState<Record<string, boolean>>(
    {},
  );
  const [rollbackConfirmation, setRollbackConfirmation] = useState("");
  const [rollbackOpen, setRollbackOpen] = useState(false);
  const activeRequest = useRef(false);
  const mounted = useRef(true);
  const [consoleOK, setConsoleOK] = useState(false);
  const [offline, setOffline] = useState(false);
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
  const [loadError, setLoadError] = useState("");
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);
  const [reload, setReload] = useState(0);
  const contextKey = JSON.stringify([
    backup,
    target,
    scenario,
    [...files].sort(),
  ]);
  const currentContext = useRef(contextKey);
  currentContext.current = contextKey;
  const inspected = !!inspection && inspectionKey === contextKey;
  const missingDecisions =
    inspected &&
    (inspection.ports.some((p) => !interfaces[p.name]) ||
      inspection.storage.some((s) => !storage[s.id]) ||
      (inspection.requires_console && !consoleOK) ||
      (inspection.requires_source_offline && !offline));
  const portValues =
    inspection?.ports.map((p) => interfaces[p.name]).filter(Boolean) || [];
  const duplicatePorts = new Set(portValues).size !== portValues.length;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    setInspection(null);
    setInspectionKey("");
    setInterfaces({});
    setStorage({});
    setConsoleOK(false);
    setOffline(false);
    setFormError("");
  }, [contextKey]);
  useEffect(() => {
    if (!busy) return;
    const protect = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, [busy]);
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
  useEffect(() => {
    if (initialBackup) {
      setBackup(initialBackup);
      setWizard(true);
    }
  }, [initialBackup]);
  useEffect(() => {
    setFiles([]);
    setEntries([]);
    setInterfaces({});
    setConsoleOK(false);
    setOffline(false);
    setLoadError("");
    setFormError("");
    if (!backup) {
      setLoading(false);
      return;
    }
    let active = true;
    setLoading(true);
    api<Entry[]>("backups/" + backup + "/files")
      .then((items) => {
        if (!active) return;
        setEntries(items);
        if (
          backup === initialBackup &&
          initialFile &&
          items.some((f) => f.path === initialFile && f.type !== "directory")
        )
          setFiles([initialFile]);
      })
      .catch((e) => {
        if (active) setLoadError(e.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [backup, initialBackup, initialFile, reload]);
  function requestBody() {
    return {
      backup_id: backup,
      target_id: target,
      scenario,
      files: scenario === "files" ? files : [],
      mapping: {
        interfaces,
        storage,
        ...(scenario !== "files"
          ? { hostname: hostname.trim(), address: address.trim() }
          : {}),
      },
      console_confirmed: consoleOK,
      source_offline: offline,
    };
  }
  async function create(e: FormEvent) {
    e.preventDefault();
    if (
      activeRequest.current ||
      loading ||
      loadError ||
      !backup ||
      !target ||
      (scenario === "files" && !files.length)
    )
      return;
    if (inspected && (missingDecisions || duplicatePorts)) return;
    activeRequest.current = true;
    const key = contextKey;
    setBusy(true);
    setFormError("");
    try {
      if (!inspected) {
        const result = await api<RecoveryInspection>(
          "plans/inspect",
          "POST",
          requestBody(),
        );
        if (!mounted.current || currentContext.current !== key) return;
        setInspection(result);
        setInspectionKey(key);
        setInterfaces(
          Object.fromEntries(
            (result.ports || [])
              .filter((p) => p.suggested)
              .map((p) => [p.name, p.suggested!]),
          ),
        );
        // Storage decisions remain explicit even when an inventory suggests a match.
        setStorage({});
      } else {
        const p = await api<Plan>("plans", "POST", requestBody());
        if (!mounted.current || currentContext.current !== key) return;
        setPlan(p);
        setCanRevise(true);
        setWizard(false);
        setConfirmation("");
        setRollbackConfirmation("");
        setRollbackOpen(false);
        setPendingJob("");
        refresh();
        notify("Wiederherstellungsplan erstellt");
      }
    } catch (e) {
      if (mounted.current && currentContext.current === key)
        setFormError((e as Error).message);
    } finally {
      activeRequest.current = false;
      if (mounted.current) setBusy(false);
    }
  }
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
  const sources = new Map(status.backups.map((b) => [b.host_id, b.host_name]));
  const stands = status.backups
    .filter((b) => !sourceHost || b.host_id === sourceHost)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
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
                setInspection(null);
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
                              const opened = await api<Plan>("plans/" + p.id);
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
          Eine Sicherung und einen Zielhost auswählen. Anker zeigt danach die
          möglichen Schritte und offenen Entscheidungen.
        </Empty>
      )}
      {wizard && (
        <Dialog
          title="Wiederherstellung vorbereiten"
          wide
          busy={busy}
          onClose={() => {
            if (!activeRequest.current) setWizard(false);
          }}
        >
          <form onSubmit={create}>
            <fieldset className="form-fields" disabled={busy}>
              <p className="dialog-intro">
                Quelle auswählen, Ziel frisch prüfen, dann Änderungen ansehen.
                Die Vorbereitung verändert keine Konfiguration.
              </p>
              <h3 className="recovery-section-title">1 · Quelle und Ziel</h3>
              <div className="form-grid">
                <Field
                  label="Quellhost"
                  hint="Bei vielen Ständen zuerst nach dem Quellhost filtern."
                >
                  <select
                    value={sourceHost}
                    onChange={(e) => {
                      setSourceHost(e.target.value);
                      setBackup("");
                    }}
                  >
                    <option value="">Alle Quellhosts</option>
                    {Array.from(sources)
                      .sort((a, b) => a[1].localeCompare(b[1]))
                      .map(([id, name]) => (
                        <option key={id} value={id}>
                          {name}
                        </option>
                      ))}
                  </select>
                </Field>
                <Field label="Sicherung">
                  <select
                    required
                    value={backup}
                    onChange={(e) => setBackup(e.target.value)}
                  >
                    <option value="">Stand auswählen</option>
                    {stands.map((b) => (
                      <option key={b.id} value={b.id}>
                        {b.host_name} · {date(b.created_at)}
                      </option>
                    ))}
                  </select>
                </Field>
              </div>
              <div className="form-grid">
                <Field label="Zielhost">
                  <select
                    required
                    value={target}
                    onChange={(e) => {
                      setTarget(e.target.value);
                      setInterfaces({});
                      setConsoleOK(false);
                      setOffline(false);
                    }}
                  >
                    <option value="">Ziel auswählen</option>
                    {status.hosts.map((h) => (
                      <option key={h.id} value={h.id}>
                        {h.name}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="Szenario">
                  <select
                    value={scenario}
                    onChange={(e) => setScenario(e.target.value)}
                  >
                    {[
                      "files",
                      "standalone",
                      "migration",
                      "version",
                      "cluster-node",
                      "cluster-disaster",
                      "topology",
                    ].map((s) => (
                      <option key={s} value={s}>
                        {labels[s]}
                      </option>
                    ))}
                  </select>
                </Field>
              </div>
              {loading && (
                <p className="notice" role="status">
                  Sicherung wird gelesen …
                </p>
              )}
              {loadError && (
                <LoadError
                  message={loadError}
                  retry={() => setReload((v) => v + 1)}
                />
              )}
              {scenario === "files" && backup && !loading && !loadError && (
                <RestoreFiles
                  entries={entries}
                  files={files}
                  onChange={setFiles}
                />
              )}
              {scenario !== "files" && (
                <p className="notice">
                  Dieses Szenario wird manuell geführt. Der Plan bereitet einen
                  Export und konkrete Prüfschritte vor; er baut keinen
                  vollständigen Host oder Cluster automatisch wieder auf.
                </p>
              )}
              {scenario !== "files" && (
                <details className="recovery-disclosure">
                  <summary>Hostidentität festlegen (optional, manuell)</summary>
                  <p className="field-hint">
                    Diese Wünsche werden im Plan dokumentiert. Hostname und
                    Adresse werden nicht automatisch in Konfigurationsdateien
                    geändert. Leer lassen, wenn die Identität später vor Ort
                    entschieden wird.
                  </p>
                  <div className="form-grid">
                    <Field label="Gewünschter Hostname (manuell)">
                      <input
                        value={hostname}
                        onChange={(e) => setHostname(e.target.value)}
                        placeholder="z. B. pve-ersatz"
                      />
                    </Field>
                    <Field label="Gewünschte Adresse (manuell)">
                      <input
                        value={address}
                        onChange={(e) => setAddress(e.target.value)}
                        placeholder="IP-Adresse oder DNS-Name"
                      />
                    </Field>
                  </div>
                </details>
              )}
              <h3 className="recovery-section-title">
                2 · Zielprüfung und Entscheidungen
              </h3>
              {!inspected && (
                <p className="field-hint">
                  „Ziel prüfen“ liest das aktuelle Zielinventar. Danach
                  erscheinen nur die für diese Auswahl benötigten
                  Entscheidungen.
                </p>
              )}
              {inspected && inspection && (
                <>
                  <div className="recovery-inspection-meta">
                    <strong>
                      {inspection.source.hostname} →{" "}
                      {inspection.target.hostname}
                    </strong>
                    <span className="state success">Ziel geprüft</span>
                  </div>
                  <p className="field-hint">
                    Quelle {inspection.source.pve_version} · Ziel{" "}
                    {inspection.target.pve_version} ·{" "}
                    {inspection.automatic
                      ? "Dateiübernahme nach Planprüfung möglich"
                      : "Manuelle Vorbereitung erforderlich"}
                  </p>
                  {inspection.blockers?.map((s, i) => (
                    <p className="notice warning" key={i}>
                      {s}
                    </p>
                  ))}
                  {inspection.warnings?.length > 0 && (
                    <RecoveryEvidence
                      key={inspectionKey + "warnings"}
                      title="Hinweise zur Zielprüfung"
                      items={inspection.warnings}
                    />
                  )}
                  <RecoveryMapping
                    inspection={inspection}
                    interfaces={interfaces}
                    storage={storage}
                    onInterfaces={setInterfaces}
                    onStorage={setStorage}
                  />
                  {duplicatePorts && (
                    <p className="notice warning" role="alert">
                      Mehrere Quellports sind demselben Zielport zugeordnet. Für
                      jeden Port ein eigenes Ziel wählen.
                    </p>
                  )}
                  {inspection.requires_console && (
                    <label className="check">
                      <input
                        type="checkbox"
                        checked={consoleOK}
                        onChange={(e) => setConsoleOK(e.target.checked)}
                      />
                      Konsolenzugang zum Ziel ist verfügbar
                    </label>
                  )}
                  {inspection.requires_source_offline && (
                    <label className="check">
                      <input
                        type="checkbox"
                        checked={offline}
                        onChange={(e) => setOffline(e.target.checked)}
                      />
                      Alter Host ist ausgeschaltet oder isoliert
                    </label>
                  )}
                  {inspection.manual?.length > 0 && (
                    <RecoveryEvidence
                      key={inspectionKey + "manual"}
                      title="Manuelle Schritte"
                      items={inspection.manual}
                      ordered
                    />
                  )}
                  <p className="field-hint">
                    „Plan prüfen“ prüft das Ziel und diese Entscheidungen
                    erneut. Änderungen an Quelle, Ziel, Szenario oder
                    Dateiauswahl erfordern eine neue Zielprüfung.
                  </p>
                </>
              )}
              {formError && (
                <p className="notice warning" role="alert">
                  {formError}
                </p>
              )}
              <footer className="dialog-footer">
                <button
                  type="button"
                  className="secondary"
                  disabled={busy}
                  onClick={() => setWizard(false)}
                >
                  Abbrechen
                </button>
                <button
                  disabled={
                    busy ||
                    loading ||
                    !!loadError ||
                    !backup ||
                    !target ||
                    (scenario === "files" && !files.length) ||
                    !!missingDecisions ||
                    duplicatePorts
                  }
                >
                  {busy
                    ? inspected
                      ? "Plan wird geprüft …"
                      : "Ziel wird geprüft …"
                    : inspected
                      ? "Plan prüfen"
                      : "Ziel prüfen"}
                </button>
              </footer>
            </fieldset>
          </form>
        </Dialog>
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
                    setInspection(null);
                    setInspectionKey("");
                    setConsoleOK(false);
                    setOffline(false);
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
