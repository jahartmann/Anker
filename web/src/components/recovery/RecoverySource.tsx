import { useEffect, useMemo, useState } from "react";
import { date, labels } from "../../api";
import type { Entry, Status } from "../../api";
import { Field, Pagination, LoadError } from "../shared";
import type { RecoveryDraft } from "./useRecoveryDraft";
import FileNavigation, { useFileNavigation } from "../files/FileNavigation";
import { indexManifest, parentPath, visibleEntries } from "../files/manifest";
function RestoreFiles({
  entries,
  files,
  onChange,
  source,
}: {
  entries: Entry[];
  files: string[];
  onChange: (files: string[]) => void;
  source: string;
}) {
  const { query, setQuery, directory, navigate, up } =
    useFileNavigation(source);
  const [selectedOnly, setSelectedOnly] = useState(false);
  const [page, setPage] = useState(1);
  const selected = useMemo(() => new Set(files), [files]);
  const indexed = useMemo(() => indexManifest(entries), [entries]);
  const matches = useMemo(
    () =>
      selectedOnly
        ? indexed
            .filter(
              (entry) =>
                entry.type !== "directory" &&
                selected.has(entry.path) &&
                entry.path.toLowerCase().includes(query.trim().toLowerCase()),
            )
            .sort((a, b) =>
              a.path.localeCompare(b.path, undefined, { numeric: true }),
            )
        : visibleEntries(indexed, directory, query),
    [indexed, directory, query, selectedOnly, selected],
  );
  useEffect(() => {
    setPage(1);
  }, [query, selectedOnly, entries, directory]);
  const current = Math.min(page, Math.max(1, Math.ceil(matches.length / 25)));
  function moveTo(path: string) {
    navigate(path);
    setPage(1);
  }
  return (
    <fieldset className="file-selection restore-file-selection">
      <legend>Dateien auswählen · {files.length} ausgewählt</legend>
      <FileNavigation
        directory={directory}
        query={query}
        navigate={moveTo}
        up={() => {
          up();
          setPage(1);
        }}
      />
      <div className="restore-selection-toolbar">
        <input
          aria-label="Wiederherstellungsdateien durchsuchen"
          placeholder="Datei oder Ordner suchen"
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
        {matches.slice((current - 1) * 25, current * 25).map((f) =>
          f.type === "directory" ? (
            <button
              type="button"
              className="restore-folder"
              key={f.path}
              aria-label={"Ordner " + f.path + " öffnen"}
              onClick={() => moveTo(f.path)}
            >
              <span aria-hidden="true">▸</span> {f.path.split("/").at(-1)}
              {query.trim() && <small className="mono">{f.path}</small>}
            </button>
          ) : (
            <div className="restore-file-result" key={f.path}>
              <label className="check">
                <input
                  type="checkbox"
                  aria-label={f.path}
                  checked={selected.has(f.path)}
                  onChange={(e) =>
                    onChange(
                      e.target.checked
                        ? [...files, f.path]
                        : files.filter((p) => p !== f.path),
                    )
                  }
                />
                <span className="mono">
                  {query.trim() || selectedOnly
                    ? f.path
                    : f.path.split("/").at(-1)}
                </span>
                {f.secret && <small className="muted">Geschützt</small>}
              </label>
              {query.trim() && !selectedOnly && (
                <button
                  type="button"
                  className="text-button"
                  aria-label={"Im Ordner zeigen: " + f.path}
                  onClick={() => moveTo(parentPath(f.path))}
                >
                  Im Ordner zeigen
                </button>
              )}
            </div>
          ),
        )}
        {!matches.length && (
          <p className="muted">
            {selectedOnly && !files.length
              ? "Noch keine Dateien ausgewählt."
              : "Keine passenden Einträge."}
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
          source={d.backup}
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
