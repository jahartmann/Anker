import { useEffect, useRef, useState } from "react";
import { api, date, labels } from "../api";
import type { Job, Status } from "../api";
import {
  Dialog,
  Empty,
  Field,
  LoadError,
  Pagination,
  State,
} from "../components/shared";
import type { Notify } from "../components/shared";
import ActiveJobs, { jobDuration } from "../components/ActiveJobs";

const active = (j: Job) => ["queued", "running"].includes(j.state);
const finished = (j: Job) =>
  ["successful", "failed", "cancelled", "interrupted"].includes(j.state);
const trigger = (j: Job) =>
  j.trigger === "scheduled"
    ? "Zeitplan"
    : j.trigger === "manual"
      ? "Manuell"
      : "Nicht erfasst";

export default function Jobs({
  status,
  notify,
  refresh,
  canEdit,
  canManage,
  openBackup,
  limit,
  initialJob,
  onJobClose,
}: {
  status: Status;
  notify: Notify;
  refresh: () => void;
  canEdit: boolean;
  canManage: boolean;
  openBackup: (id: string) => void;
  limit?: number;
  initialJob?: string;
  onJobClose?: () => void;
}) {
  const [page, setPage] = useState(1);
  const [query, setQuery] = useState("");
  const [host, setHost] = useState("");
  const [kind, setKind] = useState("");
  const [state, setState] = useState("");
  const [detail, setDetail] = useState<Job | null>(null);
  const [detailID, setDetailID] = useState("");
  const detailSequence = useRef(0);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [cancelling, setCancelling] = useState<string[]>([]);
  const hostNames = new Map(status.hosts.map((h) => [h.id, h.name]));
  for (const job of status.jobs)
    if (!hostNames.has(job.host_id)) hostNames.set(job.host_id, job.host_id);
  const search = query.trim().toLocaleLowerCase("de");
  const filtered = status.jobs.filter(
    (j) =>
      (!host || j.host_id === host) &&
      (!kind || j.kind === kind) &&
      (!state || j.state === state) &&
      (!search ||
        [
          j.id,
          hostNames.get(j.host_id),
          j.host_id,
          labels[j.kind] || j.kind,
          labels[j.state] || j.state,
          j.state === "successful" ? "Erfolgreich" : "",
          j.state === "queued" ? "In Warteschlange" : "",
          trigger(j),
          j.error,
        ]
          .filter(Boolean)
          .join(" ")
          .toLocaleLowerCase("de")
          .includes(search)),
  );
  const history = [
    ...(limit ? status.jobs : filtered.filter((j) => !active(j))),
  ].sort(
    (a, b) =>
      b.created_at.localeCompare(a.created_at) || b.id.localeCompare(a.id),
  );
  const pageSize = limit || 15;
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(history.length / pageSize)),
  );
  const jobs = history.slice(
    limit ? 0 : (currentPage - 1) * pageSize,
    limit ? pageSize : currentPage * pageSize,
  );
  const hasFilters = Boolean(query || host || kind || state);
  useEffect(() => setPage(currentPage), [currentPage]);
  useEffect(() => {
    if (initialJob) void load(initialJob);
  }, [initialJob]);
  function resetFilters() {
    setQuery("");
    setHost("");
    setKind("");
    setState("");
    setPage(1);
  }
  const currentDetail = detail?.id === detailID ? detail : null;
  const polled = status.jobs.find((j) => j.id === detailID);
  // Each job advances once from queued to running to its final state. Keep
  // later progress from either endpoint when a response or poll is delayed.
  const progress = (j: Job) =>
    finished(j) ? 2 : j.state === "running" ? 1 : 0;
  const selected =
    currentDetail && polled
      ? progress(currentDetail) > progress(polled) ||
        (progress(currentDetail) === progress(polled) &&
          currentDetail.attempts >= polled.attempts)
        ? currentDetail
        : polled
      : currentDetail || polled;
  function closeDetail() {
    ++detailSequence.current;
    setDetailID("");
    setDetail(null);
    onJobClose?.();
  }
  async function load(id: string) {
    const sequence = ++detailSequence.current;
    setDetailID(id);
    setDetail(null);
    setError("");
    setConfirmRemove(false);
    try {
      const value = await api<Job>("jobs/" + id);
      if (sequence !== detailSequence.current) return;
      if (value.id !== id)
        throw new Error("Auftragsantwort passt nicht zur Auswahl.");
      setDetail(value);
    } catch (e) {
      if (sequence === detailSequence.current) setError((e as Error).message);
    }
  }
  async function perform(j: Job, action: "cancel" | "retry" | "remove") {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await api<Job>(
        "jobs/" + j.id + (action === "remove" ? "" : "/" + action),
        action === "remove" ? "DELETE" : "POST",
        action === "remove" ? undefined : {},
      );
      if (action === "cancel") {
        setCancelling((v) => [...v, j.id]);
        notify("Abbruch angefordert");
      } else if (action === "remove") {
        closeDetail();
        notify("Auftragseintrag entfernt");
      } else {
        ++detailSequence.current;
        setDetailID(result.id);
        setDetail(result);
        setConfirmRemove(false);
        notify("Neuer Auftrag gestartet");
      }
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      {!limit && status.jobs.length > 0 && (
        <div className="collection-toolbar jobs-toolbar">
          <Field label="Aufträge suchen">
            <input
              type="search"
              value={query}
              placeholder="Host, Auftrag oder Fehler …"
              onChange={(e) => {
                setQuery(e.target.value);
                setPage(1);
              }}
            />
          </Field>
          <Field label="Host">
            <select
              value={host}
              onChange={(e) => {
                setHost(e.target.value);
                setPage(1);
              }}
            >
              <option value="">Alle Hosts</option>
              {[...hostNames]
                .sort((a, b) => a[1].localeCompare(b[1], "de"))
                .map(([id, name]) => (
                  <option key={id} value={id}>
                    {name}
                  </option>
                ))}
            </select>
          </Field>
          <Field label="Auftragstyp">
            <select
              value={kind}
              onChange={(e) => {
                setKind(e.target.value);
                setPage(1);
              }}
            >
              <option value="">Alle Typen</option>
              {[
                ...new Set([
                  "backup",
                  "probe",
                  "restore",
                  ...status.jobs.map((j) => j.kind),
                ]),
              ].map((value) => (
                <option key={value} value={value}>
                  {labels[value] || value}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Auftragsstatus">
            <select
              value={state}
              onChange={(e) => {
                setState(e.target.value);
                setPage(1);
              }}
            >
              <option value="">Alle Status</option>
              {[
                ["running", "Laufende Aufträge"],
                ["queued", "Aufträge in Warteschlange"],
                ["successful", "Erfolgreiche Aufträge"],
                ["failed", "Fehlgeschlagene Aufträge"],
                ["cancelled", "Abgebrochene Aufträge"],
                ["interrupted", "Unterbrochene Aufträge"],
              ].map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </Field>
        </div>
      )}
      {!limit && hasFilters && (
        <button className="text-button collection-reset" onClick={resetFilters}>
          Filter zurücksetzen
        </button>
      )}
      {!limit && (
        <ActiveJobs
          key={[query, host, kind, state].join("\0")}
          status={{ ...status, jobs: filtered }}
          openJob={load}
        />
      )}
      {!limit && history.length > 0 && (
        <div className="section-heading jobs-history-heading">
          <h3>Auftragsverlauf</h3>
          <span className="collection-count">
            {history.length} abgeschlossen
          </span>
        </div>
      )}
      {jobs.length ? (
        <div>
          <div className="table-scroll">
            <table className="jobs-table" aria-label="Auftragsverlauf">
              <thead>
                <tr>
                  <th>Auftrag</th>
                  <th>Status</th>
                  <th>Erstellt</th>
                  <th>Laufzeit</th>
                  <th>Ergebnis</th>
                  <th>
                    <span className="sr-only">Details</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((j) => (
                  <tr key={j.id}>
                    <td>
                      <strong>{labels[j.kind] || j.kind}</strong>
                      <div className="secondary-line">
                        {status.hosts.find((h) => h.id === j.host_id)?.name ||
                          j.host_id}
                        {j.trigger === "scheduled" ? " · Zeitplan" : ""}
                      </div>
                    </td>
                    <td>
                      <State
                        value={j.state}
                        label={
                          j.state === "successful" ? "Erfolgreich" : undefined
                        }
                      />
                    </td>
                    <td className="date">{date(j.created_at)}</td>
                    <td className="date">{jobDuration(j) || "—"}</td>
                    <td>
                      {j.error ? (
                        <span className="warning job-error" title={j.error}>
                          {j.error}
                        </span>
                      ) : j.state === "successful" ? (
                        "Abgeschlossen"
                      ) : j.state === "running" ? (
                        "Versuch " + j.attempts
                      ) : (
                        "—"
                      )}
                    </td>
                    <td>
                      <button
                        className="text-button"
                        aria-label={`Auftrag ${j.id} ansehen`}
                        onClick={() => load(j.id)}
                      >
                        Details
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {!limit && (
            <Pagination
              page={currentPage}
              pageSize={pageSize}
              total={history.length}
              onPageChange={setPage}
              label="Aufträge"
              previousLabel="Vorherige Auftragsseite"
              nextLabel="Nächste Auftragsseite"
            />
          )}
        </div>
      ) : !status.jobs.length ? (
        <Empty title="Noch keine Aufträge">
          Gestartete Vorgänge erscheinen hier mit ihrem Ergebnis.
        </Empty>
      ) : !limit && !filtered.length ? (
        <Empty title="Keine passenden Aufträge">
          Suche oder Filter anpassen, um weitere Aufträge zu sehen.
        </Empty>
      ) : !limit ? (
        <p className="collection-count">
          Keine abgeschlossenen Aufträge für diese Auswahl.
        </p>
      ) : null}
      {detailID && (
        <Dialog title="Auftragsdetails" onClose={closeDetail} busy={busy}>
          {!currentDetail ? (
            error ? (
              <LoadError message={error} retry={() => load(detailID)} />
            ) : (
              <p role="status">Auftrag wird geladen …</p>
            )
          ) : (
            selected && (
              <>
                <p className="dialog-intro">
                  {labels[selected.kind] || selected.kind} ·{" "}
                  {status.hosts.find((h) => h.id === selected.host_id)?.name ||
                    selected.host_id}
                </p>
                <dl className="update-facts">
                  <div>
                    <dt>Auftrag</dt>
                    <dd className="mono">{selected.id}</dd>
                  </div>
                  <div>
                    <dt>Status</dt>
                    <dd>
                      <State value={selected.state} />
                    </dd>
                  </div>
                  <div>
                    <dt>Auslöser</dt>
                    <dd>
                      {trigger(selected)}
                      {selected.scheduled_day
                        ? " · " + selected.scheduled_day
                        : ""}
                    </dd>
                  </div>
                  <div>
                    <dt>Erstellt</dt>
                    <dd>{date(selected.created_at)}</dd>
                  </div>
                  <div>
                    <dt>Gestartet</dt>
                    <dd>
                      {selected.started_at
                        ? date(selected.started_at)
                        : "Nicht erfasst"}
                    </dd>
                  </div>
                  <div>
                    <dt>Beendet</dt>
                    <dd>
                      {selected.finished_at ? date(selected.finished_at) : "—"}
                    </dd>
                  </div>
                  <div>
                    <dt>Versuche</dt>
                    <dd>{selected.attempts}</dd>
                  </div>
                  <div>
                    <dt>Laufzeit</dt>
                    <dd>{jobDuration(selected) || "Nicht erfasst"}</dd>
                  </div>
                </dl>
                {selected.error && (
                  <p className="notice warning">{selected.error}</p>
                )}
                {selected.kind === "restore" && finished(selected) && (
                  <p className="hint">
                    Für eine erneute Wiederherstellung einen neuen Plan prüfen
                    und bestätigen.
                  </p>
                )}
                {selected.result_id &&
                  selected.kind === "backup" &&
                  (status.backups.some((b) => b.id === selected.result_id) ? (
                    <button
                      className="secondary"
                      disabled={busy}
                      onClick={() => {
                        closeDetail();
                        openBackup(selected.result_id!);
                      }}
                    >
                      Sicherung öffnen
                    </button>
                  ) : (
                    <p className="hint">
                      Die zugehörige Sicherung ist nicht mehr im Katalog.
                    </p>
                  ))}
                {error && (
                  <p className="form-error" role="alert">
                    {error}
                  </p>
                )}
                {confirmRemove ? (
                  <>
                    <p className="notice">
                      Nur dieser Auftragseintrag wird entfernt. Sicherungen und
                      Tagesmarker bleiben erhalten.
                    </p>
                    <footer className="dialog-footer">
                      <button
                        className="secondary"
                        disabled={busy}
                        onClick={() => {
                          setConfirmRemove(false);
                          setError("");
                        }}
                      >
                        Zurück
                      </button>
                      <button
                        disabled={busy}
                        onClick={() => perform(selected, "remove")}
                      >
                        {busy ? "Entfernt …" : "Entfernen bestätigen"}
                      </button>
                    </footer>
                  </>
                ) : (
                  <footer className="dialog-footer">
                    {canManage && finished(selected) && (
                      <button
                        className="text-button"
                        disabled={busy}
                        onClick={() => setConfirmRemove(true)}
                      >
                        Eintrag entfernen
                      </button>
                    )}
                    {canEdit && active(selected) && (
                      <button
                        className="secondary"
                        disabled={busy || cancelling.includes(selected.id)}
                        onClick={() => perform(selected, "cancel")}
                      >
                        {cancelling.includes(selected.id)
                          ? "Abbruch angefordert"
                          : busy
                            ? "Wird angefordert …"
                            : "Abbrechen"}
                      </button>
                    )}
                    {canEdit &&
                      ["backup", "probe"].includes(selected.kind) &&
                      ["failed", "cancelled", "interrupted"].includes(
                        selected.state,
                      ) && (
                        <button
                          disabled={busy}
                          onClick={() => perform(selected, "retry")}
                        >
                          {busy ? "Startet …" : "Erneut starten"}
                        </button>
                      )}
                  </footer>
                )}
              </>
            )
          )}
        </Dialog>
      )}
    </>
  );
}
