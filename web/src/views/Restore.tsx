import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import { api, date, labels } from "../api";
import type { Entry, Inventory, Plan, Status, User } from "../api";
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
  const [source, setSource] = useState<Inventory | null>(null);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [files, setFiles] = useState<string[]>([]);
  const [interfaces, setInterfaces] = useState<Record<string, string>>({});
  const [consoleOK, setConsoleOK] = useState(false);
  const [offline, setOffline] = useState(false);
  const [busy, setBusy] = useState(false);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [canRevise, setCanRevise] = useState(false);
  const [targetInventory, setTargetInventory] = useState<Inventory | null>(
    null,
  );
  const [planQuery, setPlanQuery] = useState("");
  const [planState, setPlanState] = useState("");
  const [planPage, setPlanPage] = useState(1);
  const [confirmation, setConfirmation] = useState("");
  const [loadError, setLoadError] = useState("");
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);
  const [reload, setReload] = useState(0);
  useEffect(() => {
    if (initialBackup) {
      setBackup(initialBackup);
      setWizard(true);
    }
  }, [initialBackup]);
  useEffect(() => {
    setFiles([]);
    setSource(null);
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
    Promise.all([
      api<Inventory>("backups/" + backup + "/inventory"),
      api<Entry[]>("backups/" + backup + "/files"),
    ])
      .then(([inventory, items]) => {
        if (!active) return;
        setSource(inventory);
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
  async function create(e: FormEvent) {
    e.preventDefault();
    if (busy || loading || loadError) return;
    setBusy(true);
    setFormError("");
    try {
      const p = await api<Plan>("plans", "POST", {
        backup_id: backup,
        target_id: target,
        scenario,
        files: scenario === "files" ? files : [],
        mapping: { interfaces, storage: {} },
        console_confirmed: consoleOK,
        source_offline: offline,
      });
      setPlan(p);
      setCanRevise(true);
      setWizard(false);
      setConfirmation("");
      refresh();
      notify("Wiederherstellungsplan erstellt");
    } catch (e) {
      setFormError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function apply() {
    if (!plan) return;
    setBusy(true);
    setFormError("");
    try {
      await api("plans/" + plan.id + "/apply", "POST", { confirmation });
      setPlan(null);
      refresh();
      notify("Wiederherstellung gestartet. Ergebnis im Auftrag prüfen.");
    } catch (e) {
      setFormError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const dest = status.hosts.find((h) => h.id === target);
  const targetPorts =
    targetInventory?.interfaces || dest?.inventory?.interfaces || [];
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
                setWizard(true);
                setPlan(null);
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
                          onClick={async () => {
                            try {
                              setPlan(await api<Plan>("plans/" + p.id));
                              setCanRevise(false);
                              setConfirmation("");
                              setFormError("");
                            } catch (e) {
                              notify((e as Error).message, true);
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
          onClose={() => setWizard(false)}
        >
          <form onSubmit={create}>
            <fieldset className="form-fields" disabled={busy}>
              <p className="dialog-intro">
                Der Plan liest das Zielinventar und verändert noch keine
                Konfiguration.
              </p>
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
                      setTargetInventory(null);
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
              {source?.interfaces.length &&
              dest &&
              (scenario !== "files" ||
                files.some((f) => f.startsWith("etc/network/"))) ? (
                <section className="mapping">
                  <h3>Netzwerkports zuordnen</h3>
                  <p className="muted">
                    Für neue Hardware werden Ports ausdrücklich zugeordnet. Das
                    Ziel wird bei der Planerstellung erneut geprüft.
                  </p>
                  {!targetPorts.length && (
                    <p className="field-hint">
                      Noch kein Zielinventar vorhanden. „Plan prüfen“ liest die
                      Ports; danach kannst du die Zuordnungen im Plan anpassen.
                    </p>
                  )}
                  {source.interfaces
                    .filter((p) => p.name !== "lo")
                    .map((p) => (
                      <Field key={p.name} label={p.name + " → Zielport"}>
                        <select
                          value={interfaces[p.name] || ""}
                          onChange={(e) =>
                            setInterfaces((old) => ({
                              ...old,
                              [p.name]: e.target.value,
                            }))
                          }
                        >
                          <option value="">
                            Gleicher Name, sofern vorhanden
                          </option>
                          {targetPorts
                            .filter((t) => t.name !== "lo")
                            .map((t) => (
                              <option key={t.name} value={t.name}>
                                {t.name} · {t.mac}
                              </option>
                            ))}
                        </select>
                      </Field>
                    ))}
                </section>
              ) : null}
              {(scenario !== "files" ||
                files.some((f) => f.startsWith("etc/network/"))) && (
                <label className="check">
                  <input
                    type="checkbox"
                    checked={consoleOK}
                    onChange={(e) => setConsoleOK(e.target.checked)}
                  />
                  Konsolenzugang zum Ziel ist verfügbar
                </label>
              )}
              {scenario !== "files" && (
                <>
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={offline}
                      onChange={(e) => setOffline(e.target.checked)}
                    />
                    Alter Host ist ausgeschaltet oder isoliert
                  </label>
                  <p className="notice">
                    Gesamtszenarien werden auf realen Hosts manuell geführt, bis
                    Hardware und Clusterabläufe im Labor validiert sind.
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
                    (scenario === "files" && !files.length)
                  }
                >
                  {busy ? "Ziel wird geprüft …" : "Plan prüfen"}
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
          onClose={() => setPlan(null)}
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
          {plan.manual?.length > 0 && (
            <section className="manual-steps">
              <h3>Manuelle Schritte</h3>
              <ol>
                {plan.manual.map((s, i) => (
                  <li key={i}>{s}</li>
                ))}
              </ol>
            </section>
          )}
          <p className="field-hint">
            Die Dateiansicht zeigt Anpassungen an der Sicherung. Sie ist kein
            vollständiger Vergleich mit den aktuellen Dateiinhalten des
            Zielhosts.
          </p>
          <PlanFiles key={plan.id} steps={plan.steps} />
          {plan.result && (
            <div className="notice">
              <p>
                {plan.result.applied?.length || 0} Dateien übernommen. Rollback:{" "}
                <span className="mono">{plan.result.rollback_path}</span>
              </p>
              {plan.result.checks?.map((s, i) => (
                <p key={i}>{s}</p>
              ))}
            </div>
          )}
          {plan.state === "ready" && user.role !== "reader" && (
            <Field label="Plan-ID zur Ausführung eingeben" hint={plan.id}>
              <input
                autoComplete="off"
                value={confirmation}
                onChange={(e) => setConfirmation(e.target.value)}
                placeholder={plan.id}
              />
            </Field>
          )}
          {formError && (
            <p className="notice warning" role="alert">
              {formError}
            </p>
          )}
          <footer className="dialog-footer">
            {canRevise &&
              ["ready", "blocked", "manual"].includes(plan.state) && (
                <button
                  className="secondary"
                  disabled={busy}
                  onClick={() => {
                    setTargetInventory(plan.target);
                    setInterfaces(plan.mapping?.interfaces || interfaces);
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
            {plan.state === "ready" && user.role !== "reader" && (
              <button
                disabled={confirmation !== plan.id || busy}
                onClick={apply}
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
