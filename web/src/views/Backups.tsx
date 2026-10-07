import { useEffect, useState, useRef, useMemo } from "react";
import { api, bytes, date } from "../api";
import type { Backup, Entry, Status, User } from "../api";
import {
  ActionMenu,
  Dialog,
  Download,
  Empty,
  Heading,
  LoadError,
  Pagination,
  State,
} from "../components/shared";
import { ChevronRight, File, Folder, Link, Search } from "lucide-react";
import type { Notify } from "../components/shared";
import ActiveJobs from "../components/ActiveJobs";
export function BackupTable({
  backups,
  notify,
  refresh,
  openBackup,
  canEdit,
  canDownload = false,
  onRestore,
  filterKey = "",
}: {
  backups: Backup[];
  notify: Notify;
  refresh: () => void;
  openBackup: (id: string) => void;
  canEdit: boolean;
  canDownload?: boolean;
  onRestore: (id: string) => void;
  filterKey?: string;
}) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  useEffect(() => setPage(1), [filterKey]);
  const sorted = [...backups].sort((a, b) =>
    b.created_at.localeCompare(a.created_at),
  );
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(sorted.length / pageSize)),
  );
  const [pending, setPending] = useState("");
  const acting = useRef(false);
  async function act(b: Backup, action: string) {
    if (acting.current) return;
    acting.current = true;
    setPending(b.id);
    try {
      await api(
        "backups/" + b.id + "/" + action,
        "POST",
        action === "pin" ? { pinned: !b.pinned } : {},
      );
      notify(
        action === "verify"
          ? "Prüfsummen stimmen"
          : action === "archive"
            ? "Sicherung archiviert"
            : b.pinned
              ? "Schutz aufgehoben"
              : "Sicherung geschützt",
      );
      refresh();
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      acting.current = false;
      setPending("");
    }
  }
  return backups.length ? (
    <div>
      <div className="table-scroll">
        <table className="backup-table">
          <thead>
            <tr>
              <th>Host / Zeitpunkt</th>
              <th>Status</th>
              <th>Umfang</th>
              <th className="right">Aktionen</th>
            </tr>
          </thead>
          <tbody>
            {sorted
              .slice((currentPage - 1) * pageSize, currentPage * pageSize)
              .map((b) => (
                <tr key={b.id}>
                  <td>
                    <strong>{b.host_name}</strong>
                    <div className="secondary-line">
                      {date(b.created_at)}
                      {b.pinned ? " · Geschützt" : ""}
                      {b.archived ? " · Archiv" : ""}
                    </div>
                  </td>
                  <td>
                    <State value={b.status} />
                    {b.verification_error && (
                      <div className="secondary-line warning">
                        Prüfung fehlgeschlagen
                      </div>
                    )}
                    {b.warnings?.length > 0 && (
                      <div className="secondary-line warning">
                        {b.warnings.length} Hinweise
                      </div>
                    )}
                  </td>
                  <td>
                    {b.files} Einträge
                    <div className="secondary-line">{bytes(b.size)}</div>
                  </td>
                  <td className="right">
                    <div className="row-actions">
                      {canDownload && (
                        <Download
                          path={"backups/" + b.id + "/download"}
                          compact
                          notify={notify}
                        >
                          Herunterladen
                        </Download>
                      )}
                      <button
                        className="text-button"
                        onClick={() => openBackup(b.id)}
                      >
                        Dateien
                      </button>
                      {canEdit && (
                        <ActionMenu
                          label={
                            "Aktionen für " +
                            b.host_name +
                            " · " +
                            date(b.created_at)
                          }
                          disabled={!!pending}
                          actions={[
                            {
                              label: "Wiederherstellen",
                              run: () => onRestore(b.id),
                            },
                            {
                              label: "Prüfsummen prüfen",
                              run: () => act(b, "verify"),
                            },
                            {
                              label: b.pinned
                                ? "Schutz aufheben"
                                : "Stand schützen",
                              run: () => act(b, "pin"),
                            },
                            ...(!b.archived
                              ? [
                                  {
                                    label: "Archivieren",
                                    run: () => act(b, "archive"),
                                  },
                                ]
                              : []),
                          ]}
                        />
                      )}
                    </div>
                  </td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
      <div className="list-footer">
        <select
          aria-label="Sicherungen pro Seite"
          value={pageSize}
          onChange={(e) => {
            setPageSize(Number(e.target.value));
            setPage(1);
          }}
        >
          <option value={25}>25 pro Seite</option>
          <option value={50}>50 pro Seite</option>
        </select>
        <Pagination
          page={currentPage}
          pageSize={pageSize}
          total={backups.length}
          onPageChange={setPage}
          label="Sicherungen"
        />
      </div>
    </div>
  ) : (
    <Empty title="Noch keine Sicherungen">
      Eine Sicherung kann in der Hostansicht gestartet werden.
    </Empty>
  );
}
export default function Backups({
  status,
  openJob,
  ...props
}: {
  status: Status;
  notify: Notify;
  refresh: () => void;
  openBackup: (id: string) => void;
  canEdit: boolean;
  canDownload?: boolean;
  onRestore: (id: string) => void;
  openJob?: (id: string) => void;
}) {
  const [host, setHost] = useState("");
  const [query, setQuery] = useState("");
  const [state, setState] = useState("");
  const [protection, setProtection] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [diff, setDiff] = useState<
    { path: string; change: string; secret: boolean; diff: string }[] | null
  >(null);
  const [busy, setBusy] = useState(false);
  async function compare() {
    setBusy(true);
    try {
      setDiff(await api("backups/diff", "POST", { from, to, path: "" }));
    } catch (e) {
      props.notify((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  }
  const hostBackups = status.backups
    .filter((b) => !host || b.host_id === host)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
  const backups = hostBackups.filter(
    (b) =>
      (b.host_name + " " + b.id)
        .toLowerCase()
        .includes(query.trim().toLowerCase()) &&
      (!state || b.status === state) &&
      (!protection ||
        (protection === "pinned"
          ? b.pinned
          : protection === "archived"
            ? b.archived
            : !b.archived)),
  );
  const knownHosts = new Map(status.hosts.map((h) => [h.id, h.name]));
  for (const b of status.backups)
    if (!knownHosts.has(b.host_id))
      knownHosts.set(b.host_id, b.host_name + " · entfernt");
  return (
    <>
      <Heading
        title="Sicherungen"
        description="Lesbare Konfigurationen. Jeder Stand mit Inventar und Prüfsummen."
      />
      <div className="toolbar backup-filters">
        <div className="search-field">
          <Search size={15} aria-hidden="true" />
          <input
            aria-label="Sicherungen durchsuchen"
            placeholder="Host oder Sicherung suchen"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <select
          aria-label="Sicherungen nach Host filtern"
          value={host}
          onChange={(e) => {
            setHost(e.target.value);
            setFrom("");
            setTo("");
          }}
        >
          <option value="">Alle Hosts</option>
          {Array.from(knownHosts).map(([id, name]) => (
            <option key={id} value={id}>
              {name}
            </option>
          ))}
        </select>
        <select
          aria-label="Status filtern"
          value={state}
          onChange={(e) => setState(e.target.value)}
        >
          <option value="">Alle Status</option>
          <option value="successful">Gesichert</option>
          <option value="partial">Unvollständig</option>
          <option value="damaged">Beschädigt</option>
          <option value="failed">Fehlgeschlagen</option>
        </select>
        <select
          aria-label="Schutz und Archiv filtern"
          value={protection}
          onChange={(e) => setProtection(e.target.value)}
        >
          <option value="">Alle Stände</option>
          <option value="pinned">Geschützt</option>
          <option value="active">Ohne Archiv</option>
          <option value="archived">Archiviert</option>
        </select>
      </div>
      <ActiveJobs
        status={{
          hosts: status.hosts,
          jobs: status.jobs.filter(
            (j) => j.kind === "backup" && (!host || j.host_id === host),
          ),
        }}
        openJob={openJob}
        title="Aktuelle Sicherungsaufträge"
      />
      {!backups.length && status.backups.length ? (
        <Empty title="Keine passenden Sicherungen">
          Suche oder Filter anpassen.
        </Empty>
      ) : (
        <BackupTable
          filterKey={[host, query, state, protection].join("|")}
          backups={backups}
          {...props}
        />
      )}
      <section className="detail-section">
        <h3>Stände vergleichen</h3>
        <p className="muted">
          Dateiänderungen ansehen, bevor ein älterer Stand übernommen wird.
        </p>
        <div className="inline-form">
          <select
            aria-label="Vergleich von"
            value={from}
            onChange={(e) => {
              setFrom(e.target.value);
              if (e.target.value === to) setTo("");
            }}
          >
            <option value="">Älterer Stand</option>
            {hostBackups.map((b) => (
              <option key={b.id} value={b.id}>
                {b.host_name} · {date(b.created_at)}
              </option>
            ))}
          </select>
          <span className="muted">→</span>
          <select
            aria-label="Vergleich bis"
            value={to}
            onChange={(e) => setTo(e.target.value)}
          >
            <option value="">Neuerer Stand</option>
            {hostBackups
              .filter((b) => b.id !== from)
              .map((b) => (
                <option key={b.id} value={b.id}>
                  {b.host_name} · {date(b.created_at)}
                </option>
              ))}
          </select>
          <button
            className="secondary"
            disabled={!from || !to || from === to || busy}
            onClick={compare}
          >
            {busy ? "Vergleicht …" : "Vergleichen"}
          </button>
        </div>
      </section>
      {diff && (
        <Dialog
          title="Änderungen zwischen den Ständen"
          wide
          onClose={() => setDiff(null)}
        >
          {diff.length ? (
            diff.map((d) => (
              <details className="file-diff" key={d.path}>
                <summary>
                  <span className="mono">{d.path}</span>
                  <span className="muted">
                    {{
                      added: "Hinzugefügt",
                      changed: "Geändert",
                      removed: "Entfernt",
                    }[d.change] || d.change}
                  </span>
                </summary>
                <pre>
                  {d.secret
                    ? "Geschützter Inhalt"
                    : d.diff || "Metadaten geändert"}
                </pre>
              </details>
            ))
          ) : (
            <Empty title="Keine Unterschiede">
              Die ausgewählten Dateien stimmen überein.
            </Empty>
          )}
        </Dialog>
      )}
    </>
  );
}
export function FileBrowser({
  backup,
  user,
  onClose,
  notify,
  onRestore,
}: {
  backup: Backup;
  user: User;
  onClose: () => void;
  notify: Notify;
  onRestore: (id: string, file?: string) => void;
}) {
  const [files, setFiles] = useState<Entry[]>([]);
  const [query, setQuery] = useState("");
  const [directory, setDirectory] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const [selected, setSelected] = useState("");
  const [preview, setPreview] = useState<{
    entry: Entry;
    content: string;
    masked: boolean;
  } | null>(null);
  const request = useRef(0);
  const [busy, setBusy] = useState(false);
  const [listing, setListing] = useState(true);
  const [listError, setListError] = useState("");
  const [previewError, setPreviewError] = useState("");
  const [reload, setReload] = useState(0);
  useEffect(() => {
    let active = true;
    request.current++;
    setSelected("");
    setPreview(null);
    setFiles([]);
    setQuery("");
    setDirectory("");
    setPage(1);
    setBusy(false);
    setListing(true);
    setListError("");
    setPreviewError("");
    api<Entry[]>("backups/" + backup.id + "/files")
      .then((v) => {
        if (active) setFiles(v);
      })
      .catch((e) => {
        if (active) setListError(e.message);
      })
      .finally(() => {
        if (active) setListing(false);
      });
    return () => {
      active = false;
      request.current++;
    };
  }, [backup.id, reload]);
  async function open(path: string, reveal = false) {
    const sequence = ++request.current;
    setSelected(path);
    setPreview(null);
    setPreviewError("");
    setBusy(true);
    try {
      const result = await api<{
        entry: Entry;
        content: string;
        masked: boolean;
      }>(
        "backups/" +
          backup.id +
          "/file?path=" +
          encodeURIComponent(path) +
          (reveal ? "&reveal=1" : ""),
      );
      if (sequence === request.current) setPreview(result);
    } catch (e) {
      if (sequence === request.current) setPreviewError((e as Error).message);
    } finally {
      if (sequence === request.current) setBusy(false);
    }
  }
  const entries = useMemo(() => {
    const indexed = new Map(files.map((f) => [f.path, f]));
    for (const file of files) {
      const parts = file.path.split("/");
      for (let i = 1; i < parts.length; i++) {
        const path = parts.slice(0, i).join("/");
        if (!indexed.has(path))
          indexed.set(path, {
            path,
            type: "directory",
            mode: 0,
            uid: 0,
            gid: 0,
            size: 0,
            secret: false,
          });
      }
    }
    return Array.from(indexed.values());
  }, [files]);
  const search = query.trim().toLowerCase();
  const visibleFiles = entries
    .filter((f) =>
      search
        ? f.type !== "directory" && f.path.toLowerCase().includes(search)
        : f.path.slice(0, Math.max(0, f.path.lastIndexOf("/"))) === directory,
    )
    .sort(
      (a, b) =>
        Number(b.type === "directory") - Number(a.type === "directory") ||
        a.path.localeCompare(b.path, undefined, { numeric: true }),
    );
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(visibleFiles.length / pageSize)),
  );
  function navigate(path: string) {
    setDirectory(path);
    setQuery("");
    setPage(1);
  }
  const breadcrumbs = directory ? directory.split("/") : [];
  const selectedEntry = files.find((f) => f.path === selected);
  return (
    <Dialog title={"Dateien · " + backup.host_name} wide onClose={onClose}>
      <p className="dialog-intro">
        {date(backup.created_at)} · {backup.files} Einträge · Dateien werden im
        Original heruntergeladen.
      </p>
      {!!backup.warnings?.length && (
        <details className="backup-notes">
          <summary>
            <span className="state warning">
              {backup.warnings.length}{" "}
              {backup.warnings.length === 1 ? "Hinweis" : "Hinweise"}
            </span>{" "}
            Hinweise zu diesem Stand ansehen
          </summary>
          <ul>
            {backup.warnings.map((w, i) => (
              <li key={i}>{w}</li>
            ))}
          </ul>
        </details>
      )}
      <nav className="breadcrumbs" aria-label="Dateipfad">
        <button
          className="text-button"
          onClick={() => navigate("")}
          aria-current={!directory && !search ? "page" : undefined}
        >
          Alle Dateien
        </button>
        {breadcrumbs.map((part, i) => (
          <span key={i}>
            <ChevronRight size={13} aria-hidden="true" />
            <button
              className="text-button"
              onClick={() => navigate(breadcrumbs.slice(0, i + 1).join("/"))}
              aria-current={
                i === breadcrumbs.length - 1 && !search ? "page" : undefined
              }
            >
              {part}
            </button>
          </span>
        ))}
        {search && <span className="muted">Suche in allen Ordnern</span>}
      </nav>
      <div className="file-browser">
        <div className="file-list">
          <input
            aria-label="Dateien durchsuchen"
            placeholder="Pfad suchen"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setPage(1);
            }}
          />
          <div className="file-items">
            {listing ? (
              <p className="muted file-list-message" role="status">
                Dateien werden geladen …
              </p>
            ) : listError ? (
              <LoadError
                message={listError}
                retry={() => setReload((v) => v + 1)}
              />
            ) : !visibleFiles.length ? (
              <p className="muted file-list-message">
                {query
                  ? "Keine passenden Dateien"
                  : directory
                    ? "Dieser Ordner ist leer"
                    : "Keine Dateien in diesem Stand"}
              </p>
            ) : (
              visibleFiles
                .slice((currentPage - 1) * pageSize, currentPage * pageSize)
                .map((f) => (
                  <button
                    className={
                      "file-entry" + (selected === f.path ? " selected" : "")
                    }
                    aria-label={
                      f.type === "directory"
                        ? "Ordner " + f.path + " öffnen"
                        : f.path +
                          " · " +
                          (f.type === "symlink"
                            ? "Verknüpfung"
                            : bytes(f.size)) +
                          (f.secret ? " · Geschützt" : "")
                    }
                    aria-pressed={
                      f.type === "directory" ? undefined : selected === f.path
                    }
                    key={f.path}
                    onClick={() =>
                      f.type === "directory" ? navigate(f.path) : open(f.path)
                    }
                  >
                    {f.type === "directory" ? (
                      <Folder size={17} aria-hidden="true" />
                    ) : f.type === "symlink" ? (
                      <Link size={17} aria-hidden="true" />
                    ) : (
                      <File size={17} aria-hidden="true" />
                    )}
                    <span className="file-entry-name">
                      <strong>{f.path.split("/").at(-1)}</strong>
                      {search && <small className="mono">{f.path}</small>}
                    </span>
                    <span className="file-entry-meta">
                      <span>
                        {f.type === "directory"
                          ? "Ordner"
                          : f.type === "symlink"
                            ? "Verknüpfung"
                            : bytes(f.size)}
                      </span>
                      {f.secret && (
                        <span className="state warning">Geschützt</span>
                      )}
                    </span>
                  </button>
                ))
            )}
          </div>
          {!listing && !listError && (
            <div className="file-list-footer">
              <select
                aria-label="Einträge pro Seite"
                value={pageSize}
                onChange={(e) => {
                  setPageSize(Number(e.target.value));
                  setPage(1);
                }}
              >
                <option value={25}>25 pro Seite</option>
                <option value={50}>50 pro Seite</option>
              </select>
              <Pagination
                page={currentPage}
                pageSize={pageSize}
                total={visibleFiles.length}
                onPageChange={setPage}
                label={search ? "Treffer" : "Einträge"}
              />
            </div>
          )}
        </div>
        <div className="file-preview">
          {busy ? (
            <p className="muted">Datei wird gelesen …</p>
          ) : previewError ? (
            <>
              <div className="preview-heading">
                <strong className="mono">{selected}</strong>
              </div>
              <LoadError
                message={previewError}
                label="Erneut lesen"
                retry={() => open(selected)}
              />
            </>
          ) : preview ? (
            <>
              <div className="preview-heading">
                <strong className="mono">{preview.entry.path}</strong>
                <span className="muted">
                  {preview.entry.mode.toString(8)} · {preview.entry.uid}:
                  {preview.entry.gid}
                </span>
              </div>
              {preview.masked ? (
                <div className="empty">
                  <h3>Geschützter Inhalt</h3>
                  <p>Die Datei kann Zugangsdaten enthalten.</p>
                  {user.secrets && (
                    <button
                      className="secondary"
                      onClick={() => open(selected, true)}
                    >
                      Inhalt anzeigen
                    </button>
                  )}
                </div>
              ) : (
                <pre>{preview.content}</pre>
              )}
            </>
          ) : (
            <Empty title="Datei auswählen">
              Ein Original ansehen oder den vollständigen Stand exportieren.
            </Empty>
          )}
        </div>
      </div>
      <footer className="dialog-footer">
        {selected &&
          selectedEntry &&
          (user.secrets || !selectedEntry.secret) && (
            <Download
              path={
                "backups/" +
                backup.id +
                "/file-download?path=" +
                encodeURIComponent(selected)
              }
              notify={notify}
            >
              Datei herunterladen
            </Download>
          )}
        {user.secrets && (
          <Download path={"backups/" + backup.id + "/download"} notify={notify}>
            Stand herunterladen
          </Download>
        )}
        {user.role !== "reader" && (
          <button
            onClick={() => {
              onClose();
              onRestore(backup.id, selected || undefined);
            }}
          >
            {selected ? "Datei wiederherstellen" : "Wiederherstellung planen"}
          </button>
        )}
      </footer>
    </Dialog>
  );
}
