import { useListPageSize } from "../../listPageSize";
import RemovalFiles from "./RemovalFiles";
import { useEffect, useMemo, useState } from "react";
import { preciseDate, labels } from "../../api";
import type { Entry, Status } from "../../api";
import { Field, Pagination, LoadError } from "../shared";
import type { RecoveryDraft } from "./useRecoveryDraft";
import FileNavigation, { useFileNavigation } from "../files/FileNavigation";
import {
  filesByDirectory,
  groupSelection,
  indexManifest,
  parentPath,
  toggleGroup,
  visibleEntries,
} from "../files/manifest";
import type { GroupSelection } from "../files/manifest";
import { displayHostName } from "../../hostNames";

function GroupCheckbox({
  label,
  state,
  disabled,
  onToggle,
}: {
  label: string;
  state: GroupSelection;
  disabled?: boolean;
  onToggle: () => void;
}) {
  return (
    <input
      type="checkbox"
      aria-label={label}
      checked={state === "all"}
      disabled={disabled}
      ref={(input) => {
        if (input) input.indeterminate = state === "some";
      }}
      onChange={onToggle}
    />
  );
}
function RestoreFiles({
  entries,
  files,
  onChange,
  source,
  selectedFirst = false,
}: {
  entries: Entry[];
  files: string[];
  onChange: (files: string[]) => void;
  source: string;
  selectedFirst?: boolean;
}) {
  const { query, setQuery, directory, navigate, up } =
    useFileNavigation(source);
  const [selectedOnly, setSelectedOnly] = useState(selectedFirst);
  const [page, setPage] = useState(1);
  const [listSize, setListSize] = useListPageSize("recovery-source", 25);
  const selected = useMemo(() => new Set(files), [files]);
  const indexed = useMemo(() => indexManifest(entries), [entries]);
  const below = useMemo(() => filesByDirectory(entries), [entries]);
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
  const current = Math.min(
    page,
    Math.max(1, Math.ceil(matches.length / listSize)),
  );
  function moveTo(path: string) {
    navigate(path);
    setPage(1);
  }
  const searching = !!query.trim();
  // Everything the "select all" box covers: search hits, or the open folder with its subfolders.
  const scope = searching
    ? matches.flatMap((entry) =>
        entry.type === "directory" ? [] : [entry.path],
      )
    : below.get(directory) || [];
  const scopeLabel = searching
    ? `Alle ${scope.length} gefundenen Dateien auswählen`
    : directory
      ? `Alles in ${directory} auswählen`
      : "Gesamte Sicherung auswählen";
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
        scopeLabel={
          selectedOnly ? "Ausgewählte Dateien aus allen Ordnern" : undefined
        }
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
      {!selectedOnly && scope.length > 0 && (
        <label className="check restore-select-all">
          <GroupCheckbox
            label={scopeLabel}
            state={groupSelection(selected, scope)}
            onToggle={() => onChange(toggleGroup(files, scope))}
          />
          <span>{scopeLabel}</span>
          <small className="muted">
            {scope.filter((path) => selected.has(path)).length} von{" "}
            {scope.length}
          </small>
        </label>
      )}
      <div className="restore-file-items">
        {matches.slice((current - 1) * listSize, current * listSize).map((f) =>
          f.type === "directory" ? (
            <FolderRow
              key={f.path}
              path={f.path}
              contents={below.get(f.path) || []}
              selected={selected}
              showPath={searching}
              open={() => moveTo(f.path)}
              toggle={(group) => onChange(toggleGroup(files, group))}
            />
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
        pageSize={listSize}
        onPageSizeChange={setListSize}
        total={matches.length}
        onPageChange={setPage}
        label="Dateien"
      />
    </fieldset>
  );
}

function FolderRow({
  path,
  contents,
  selected,
  showPath,
  open,
  toggle,
}: {
  path: string;
  contents: string[];
  selected: Set<string>;
  showPath: boolean;
  open: () => void;
  toggle: (group: string[]) => void;
}) {
  const chosen = contents.filter((file) => selected.has(file)).length;
  return (
    <div className="restore-folder-row">
      <GroupCheckbox
        label={"Ordner " + path + " auswählen"}
        state={groupSelection(selected, contents)}
        disabled={!contents.length}
        onToggle={() => toggle(contents)}
      />
      <button
        type="button"
        className="restore-folder"
        aria-label={"Ordner " + path + " öffnen"}
        onClick={open}
      >
        <span aria-hidden="true">▸</span> {path.split("/").at(-1)}
        {showPath && <small className="mono">{path}</small>}
      </button>
      <small className="muted">
        {contents.length
          ? chosen
            ? `${chosen} von ${contents.length}`
            : `${contents.length} ${contents.length === 1 ? "Datei" : "Dateien"}`
          : "leer"}
      </small>
    </div>
  );
}

const situationHint: Record<string, string> = {
  files: "Ausgewählte Dateien und Ordner aus der Sicherung zurückschreiben.",
  host: "Einen neu installierten oder neuen Host mit der gesicherten Konfiguration ausstatten. Bootloader, SSH-Zugang, Benutzer und Maschinenidentität des Ziels bleiben erhalten.",
  "cluster-node":
    "Einen ausgefallenen Node im laufenden Cluster ersetzen. Nur die Konfiguration dieses Nodes wird übernommen; gemeinsame Clusterdateien bleiben unverändert.",
  "cluster-disaster":
    "Den Cluster auf einem isolierten Node neu aufbauen, wenn kein Node mehr läuft. Alte Nodes müssen ausgeschaltet oder vom Netz getrennt sein.",
};

export default function RecoverySource({
  status,
  draft: d,
}: {
  status: Status;
  draft: RecoveryDraft;
}) {
  const [sourceHost, setSourceHost] = useState("");
  const sources = new Map(status.backups.map((b) => [b.host_id, b.host_name]));
  const selectedSource = status.backups.find((b) => b.id === d.backup);
  const isPBS = selectedSource?.host_type === "pbs";
  useEffect(() => {
    if (isPBS && d.scenario !== "files") d.changeScenario("files");
  }, [isPBS, d.scenario]);
  // Restoring onto the host the backup came from is the usual case, also when
  // the assistant opens with a backup chosen elsewhere.
  const origin = selectedSource?.host_id;
  useEffect(() => {
    if (!d.target && origin && status.hosts.some((h) => h.id === origin))
      d.changeTarget(origin);
  }, [origin]);
  function shortId(id: string) {
    let length = Math.min(10, id.length);
    while (
      length < id.length &&
      status.backups.some(
        (backup) =>
          backup.id !== id && backup.id.startsWith(id.slice(0, length)),
      )
    )
      length++;
    return id.slice(0, length);
  }
  function sourceLabel(source: (typeof status.backups)[number]) {
    return `${displayHostName(source.host_name)} · ${preciseDate(source.created_at)} · ${shortId(source.id)}`;
  }
  return (
    <section aria-label="Quelle und Situation" data-surface-topic="archive">
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
                {displayHostName(name)}
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
                  {sourceLabel(b)}
                </option>
              ))}
          </select>
        </Field>
      </div>
      <Field
        label="Situation"
        hint={isPBS ? undefined : situationHint[d.scenario]}
      >
        <select
          value={d.scenario}
          onChange={(e) => d.changeScenario(e.target.value)}
        >
          {(isPBS
            ? ["files"]
            : ["files", "host", "cluster-node", "cluster-disaster"]
          ).map((s) => (
            <option key={s} value={s}>
              {isPBS ? "PBS-Konfiguration wiederherstellen" : labels[s]}
            </option>
          ))}
        </select>
      </Field>
      {isPBS && (
        <section className="notice" aria-label="PBS-Wiederherstellung">
          <h3>PBS-Konfiguration wiederherstellen</h3>
          <p>
            Zurückgeschrieben werden Dateien aus /etc/proxmox-backup.
            Voraussetzung: gleiche PBS-Hauptversion und bereits eingebundene
            Datastores.
          </p>
          <p>
            Vor der Zielprüfung am Ziel ausführen:{" "}
            <code>systemctl stop proxmox-backup-proxy proxmox-backup</code>.
            Anker prüft den Stillstand über SSH und startet keine Dienste oder
            Jobs.
          </p>
          <p>
            Zurückgespielte Jobs bleiben deaktiviert. Vor dem Aktivieren alle
            Jobs und Aufbewahrungsregeln prüfen.
          </p>
          <button
            type="button"
            className="secondary"
            disabled={d.loading}
            onClick={() =>
              d.changeFiles(
                d.entries
                  .filter(
                    (e) =>
                      e.type !== "directory" &&
                      e.path.startsWith("etc/proxmox-backup/"),
                  )
                  .map((e) => e.path),
              )
            }
          >
            Gesamte PBS-Konfiguration auswählen
          </button>
        </section>
      )}
      {d.loading && <p role="status">Sicherung wird gelesen …</p>}
      {d.loadError && (
        <LoadError message={d.loadError} retry={d.reloadEntries} />
      )}
      {d.scenario === "files" && d.backup && !d.loading && !d.loadError && (
        <RestoreFiles
          entries={
            isPBS
              ? d.entries.filter(
                  (e) =>
                    e.path === "etc" ||
                    e.path === "etc/proxmox-backup" ||
                    e.path.startsWith("etc/proxmox-backup/"),
                )
              : d.entries
          }
          files={d.files}
          onChange={d.changeFiles}
          source={d.backup}
          selectedFirst={d.fromChange}
        />
      )}
      {d.removeFiles.length > 0 && (
        <section
          className="removal-cards"
          aria-label="Vorgemerkte Dateientfernungen"
        >
          <h3>Zum Entfernen vorgemerkt</h3>
          <p>
            Diese Dateien gab es im früheren Stand nicht. Nach der Zielprüfung
            entscheidest du für jede einzeln, ob sie gelöscht wird.
          </p>
          <RemovalFiles
            paths={d.removeFiles}
            onDeselect={(path) =>
              d.changeRemovals(d.removeFiles.filter((p) => p !== path))
            }
          />
        </section>
      )}
    </section>
  );
}
