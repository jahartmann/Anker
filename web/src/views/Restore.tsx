import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { api, date, labels } from "../api";
import type { Entry, Inventory, Plan, Status, User } from "../api";
import {
  Dialog,
  Download,
  Empty,
  Field,
  Heading,
  State,
} from "../components/shared";
import type { Notify } from "../components/shared";
export default function Restore({
  status,
  initialBackup,
  user,
  notify,
  refresh,
}: {
  status: Status;
  initialBackup: string;
  user: User;
  notify: Notify;
  refresh: () => void;
}) {
  const [wizard, setWizard] = useState(!!initialBackup);
  const [backup, setBackup] = useState(initialBackup);
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
  const [confirmation, setConfirmation] = useState("");
  useEffect(() => {
    if (initialBackup) {
      setBackup(initialBackup);
      setWizard(true);
    }
  }, [initialBackup]);
  useEffect(() => {
    setFiles([]);
    if (!backup) {
      setEntries([]);
      return;
    }
    let active = true;
    setSource(null);
    api<Inventory>("backups/" + backup + "/inventory")
      .then((v) => {
        if (active) setSource(v);
      })
      .catch((e) => notify(e.message, true));
    api<Entry[]>("backups/" + backup + "/files")
      .then((v) => {
        if (active) setEntries(v);
      })
      .catch((e) => notify(e.message, true));
    return () => {
      active = false;
    };
  }, [backup, notify]);
  async function create(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
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
      setWizard(false);
      setConfirmation("");
      refresh();
      notify("Wiederherstellungsplan erstellt");
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  }
  async function apply() {
    if (!plan) return;
    setBusy(true);
    try {
      await api("plans/" + plan.id + "/apply", "POST", { confirmation });
      setPlan(null);
      refresh();
      notify("Wiederherstellung gestartet. Ergebnis im Auftrag prüfen.");
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  }
  const dest = status.hosts.find((h) => h.id === target);
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
              }}
            >
              Plan erstellen
            </button>
          )
        }
      />
      {status.plans.length ? (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Quelle → Ziel</th>
                <th>Szenario</th>
                <th>Status</th>
                <th className="right">Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {[...status.plans]
                .sort((a, b) => b.created_at.localeCompare(a.created_at))
                .map((p) => (
                  <tr key={p.id}>
                    <td>
                      <strong>
                        {p.source.hostname} → {p.target.hostname}
                      </strong>
                      <div className="secondary-line">{date(p.created_at)}</div>
                    </td>
                    <td>{labels[p.scenario]}</td>
                    <td>
                      <State value={p.state} />
                    </td>
                    <td className="right">
                      <button
                        className="text-button"
                        onClick={() => {
                          setPlan(p);
                          setConfirmation("");
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
          onClose={() => setWizard(false)}
        >
          <form onSubmit={create}>
            <p className="dialog-intro">
              Der Plan liest das Zielinventar und verändert noch keine
              Konfiguration.
            </p>
            <div className="form-grid">
              <Field label="Sicherung">
                <select
                  required
                  value={backup}
                  onChange={(e) => setBackup(e.target.value)}
                >
                  <option value="">Stand auswählen</option>
                  {status.backups.map((b) => (
                    <option key={b.id} value={b.id}>
                      {b.host_name} · {date(b.created_at)}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Zielhost">
                <select
                  required
                  value={target}
                  onChange={(e) => {
                    setTarget(e.target.value);
                    setInterfaces({});
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
            </div>
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
            {scenario === "files" && (
              <fieldset className="file-selection">
                <legend>Dateien auswählen</legend>
                {entries
                  .filter((f) => f.type !== "directory")
                  .map((f) => (
                    <label className="check" key={f.path}>
                      <input
                        type="checkbox"
                        checked={files.includes(f.path)}
                        onChange={(e) =>
                          setFiles((old) =>
                            e.target.checked
                              ? [...old, f.path]
                              : old.filter((v) => v !== f.path),
                          )
                        }
                      />
                      <span className="mono">{f.path}</span>
                      {f.secret && <small className="muted">Geschützt</small>}
                    </label>
                  ))}
              </fieldset>
            )}
            {source?.interfaces.length && dest ? (
              <section className="mapping">
                <h3>Netzwerkports zuordnen</h3>
                <p className="muted">
                  Für neue Hardware werden Ports ausdrücklich zugeordnet. Das
                  Ziel wird bei der Planerstellung erneut geprüft.
                </p>
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
                        {dest.inventory?.interfaces.map((t) => (
                          <option key={t.name} value={t.name}>
                            {t.name} · {t.mac}
                          </option>
                        ))}
                      </select>
                    </Field>
                  ))}
              </section>
            ) : null}
            <label className="check">
              <input
                type="checkbox"
                checked={consoleOK}
                onChange={(e) => setConsoleOK(e.target.checked)}
              />
              Konsolenzugang zum Ziel ist verfügbar
            </label>
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
            <footer className="dialog-footer">
              <button
                type="button"
                className="secondary"
                onClick={() => setWizard(false)}
              >
                Abbrechen
              </button>
              <button
                disabled={
                  busy ||
                  !backup ||
                  !target ||
                  (scenario === "files" && !files.length)
                }
              >
                {busy ? "Ziel wird geprüft …" : "Plan prüfen"}
              </button>
            </footer>
          </form>
        </Dialog>
      )}
      {plan && (
        <Dialog
          title="Wiederherstellungsplan"
          wide
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
          <div className="plan-files">
            {plan.steps.map((s) => (
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
          </div>
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
          <footer className="dialog-footer">
            {user.secrets && (
              <Download path={"plans/" + plan.id + "/download"}>
                Plan und Dateien exportieren
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
