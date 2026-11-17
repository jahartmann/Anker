import { useListPageSize } from "../listPageSize";
import BackupIssues from "./BackupIssues";
import { useEffect, useState, useRef, useMemo } from "react";
import { api, bytes, date } from "../api";
import type { Backup, Entry, User } from "../api";
import { Dialog, Download, Empty, LoadError, Pagination } from "./shared";
import { File, Folder, Link } from "lucide-react";
import FileNavigation, { useFileNavigation } from "./files/FileNavigation";
import { indexManifest, parentPath, visibleEntries } from "./files/manifest";
import type { Notify } from "./shared";
import BackupReadiness from "./BackupReadiness";

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
  const { query, setQuery, directory, navigate, up } = useFileNavigation(
    backup.id,
  );
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useListPageSize("backup-files", 25);
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
  const entries = useMemo(() => indexManifest(files), [files]);
  const search = query.trim().toLowerCase();
  const visibleFiles = visibleEntries(entries, directory, query);
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(visibleFiles.length / pageSize)),
  );
  function moveTo(path: string) {
    navigate(path);
    setPage(1);
  }
  const selectedEntry = files.find((f) => f.path === selected);
  return (
    <Dialog
      topic="archive"
      title={"Dateien · " + backup.host_name}
      wide
      onClose={onClose}
    >
      <p className="dialog-intro">
        {date(backup.created_at)} · {backup.files} Einträge · Dateien werden im
        Original heruntergeladen.
      </p>
      <BackupIssues backup={backup} />
      <FileNavigation
        directory={directory}
        query={query}
        navigate={moveTo}
        up={() => {
          up();
          setPage(1);
        }}
      />
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
                  <div className="file-result" key={f.path}>
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
                      onClick={() =>
                        f.type === "directory" ? moveTo(f.path) : open(f.path)
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
                    {search && f.type !== "directory" && (
                      <button
                        type="button"
                        className="text-button file-show-folder"
                        onClick={() => moveTo(parentPath(f.path))}
                        aria-label={"Im Ordner zeigen: " + f.path}
                      >
                        Im Ordner zeigen
                      </button>
                    )}
                  </div>
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
                <option value={100}>100 pro Seite</option>
                <option value={200}>200 pro Seite</option>
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
        <div
          className={
            "file-preview" + (!selected ? " file-preview-unselected" : "")
          }
        >
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
              {preview.entry.capture_origin === "inventory_symlink" && (
                <p className="field-hint">
                  Link anhand übereinstimmender Inventarangaben ergänzt; sein
                  Ziel wurde nicht gelesen.
                  {preview.entry.mtime_unknown &&
                    " Die ursprüngliche Änderungszeit ist nicht bekannt."}
                </p>
              )}
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
      <details className="backup-notes backup-readiness-disclosure">
        <summary>Sicherung und Prüfergebnisse ansehen</summary>
        <BackupReadiness backup={backup} />
      </details>
    </Dialog>
  );
}
