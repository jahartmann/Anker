import { useEffect, useMemo, useState } from "react";
import { date, labels } from "../../api";
import type { Entry, Status } from "../../api";
import { Field, Pagination, LoadError } from "../shared";
import type { RecoveryDraft } from "./useRecoveryDraft";
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

export default function RecoverySource({
  status,
  draft: d,
}: {
  status: Status;
  draft: RecoveryDraft;
}) {
  const [sourceHost, setSourceHost] = useState("");
  const sources = new Map(status.backups.map((b) => [b.host_id, b.host_name]));
  return (
    <section aria-label="Quelle und Situation">
      <h2>1 · Quelle und Situation</h2>
      <div className="form-grid">
        <Field label="Quellhost">
          <select
            value={sourceHost}
            onChange={(e) => {
              setSourceHost(e.target.value);
              d.changeBackup("");
            }}
          >
            <option value="">Alle Quellhosts</option>
            {Array.from(sources).map(([id, name]) => (
              <option key={id} value={id}>
                {name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Sicherung">
          <select
            value={d.backup}
            onChange={(e) => d.changeBackup(e.target.value)}
          >
            <option value="">Stand auswählen</option>
            {status.backups
              .filter((b) => !sourceHost || b.host_id === sourceHost)
              .sort((a, b) => b.created_at.localeCompare(a.created_at))
              .map((b) => (
                <option key={b.id} value={b.id}>
                  {b.host_name} · {date(b.created_at)}
                </option>
              ))}
          </select>
        </Field>
      </div>
      <Field label="Situation">
        <select
          value={d.scenario}
          onChange={(e) => d.changeScenario(e.target.value)}
        >
          {["files", "host", "cluster-node", "cluster-disaster"].map((s) => (
            <option key={s} value={s}>
              {labels[s]}
            </option>
          ))}
        </select>
      </Field>
      {d.loading && <p role="status">Sicherung wird gelesen …</p>}
      {d.loadError && (
        <LoadError message={d.loadError} retry={d.reloadEntries} />
      )}
      {d.scenario === "files" && d.backup && !d.loading && !d.loadError && (
        <RestoreFiles
          entries={d.entries}
          files={d.files}
          onChange={d.changeFiles}
        />
      )}
      {d.scenario !== "files" && (
        <p className="notice">
          Der Plan führt den Wiederaufbau manuell. Gastdatenträger kommen aus
          vorhandenen Storages oder PBS. Zielinstallation und bestehender
          Cluster behalten ihre geschützten Konfigurationen.
        </p>
      )}
    </section>
  );
}
