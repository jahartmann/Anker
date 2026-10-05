import { useEffect, useState, useRef } from "react";
import { api, bytes, date } from "../api";
import type { Backup, Entry, Status, User } from "../api";
import { Dialog, Download, Empty, Heading, State } from "../components/shared";
import type { Notify } from "../components/shared";
export function BackupTable({
  backups,
  notify,
  refresh,
  openBackup,
  canEdit,
  canDownload = false,
  onRestore,
}: {
  backups: Backup[];
  notify: Notify;
  refresh: () => void;
  openBackup: (id: string) => void;
  canEdit: boolean;
  canDownload?: boolean;
  onRestore: (id: string) => void;
}) {
  const [visible, setVisible] = useState(50);
  async function act(b: Backup, action: string) {
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
    }
  }
  return backups.length ? (
    <div>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Host / Zeitpunkt</th>
              <th>Status</th>
              <th>Umfang</th>
              <th className="right">Aktionen</th>
            </tr>
          </thead>
          <tbody>
            {backups.slice(0, visible).map((b) => (
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
                  {b.files} Dateien
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
                      <details className="action-menu">
                        <summary aria-label={"Aktionen für " + b.id}>
                          •••
                        </summary>
                        <div>
                          <button onClick={() => onRestore(b.id)}>
                            Wiederherstellen
                          </button>
                          <button onClick={() => act(b, "verify")}>
                            Prüfsummen prüfen
                          </button>
                          <button onClick={() => act(b, "pin")}>
                            {b.pinned ? "Schutz aufheben" : "Stand schützen"}
                          </button>
                          <button onClick={() => act(b, "archive")}>
                            Archivieren
                          </button>
                        </div>
                      </details>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {backups.length > visible && (
        <button
          className="text-button"
          onClick={() => setVisible((v) => v + 50)}
        >
          Weitere Sicherungen anzeigen ({backups.length - visible})
        </button>
      )}
    </div>
  ) : (
    <Empty title="Noch keine Sicherungen">
      Eine Sicherung kann in der Hostansicht gestartet werden.
    </Empty>
  );
}
export default function Backups({
  status,
  ...props
}: {
  status: Status;
  notify: Notify;
  refresh: () => void;
  openBackup: (id: string) => void;
  canEdit: boolean;
  canDownload?: boolean;
  onRestore: (id: string) => void;
}) {
  const [host, setHost] = useState("");
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
  const backups = status.backups.filter((b) => !host || b.host_id === host);
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
      <div className="toolbar">
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
      </div>
      <BackupTable backups={backups} {...props} />
      <section className="detail-section">
        <h3>Stände vergleichen</h3>
        <p className="muted">
          Dateiänderungen ansehen, bevor ein älterer Stand übernommen wird.
        </p>
        <div className="inline-form">
          <select
            aria-label="Vergleich von"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          >
            <option value="">Älterer Stand</option>
            {backups.map((b) => (
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
            {backups
              .filter((b) => b.id !== from)
              .map((b) => (
                <option key={b.id} value={b.id}>
                  {b.host_name} · {date(b.created_at)}
                </option>
              ))}
          </select>
          <button
            className="secondary"
            disabled={!from || !to || busy}
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
                  <span className="muted">{d.change}</span>
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
  onRestore: (id: string) => void;
}) {
  const [files, setFiles] = useState<Entry[]>([]);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState("");
  const [preview, setPreview] = useState<{
    entry: Entry;
    content: string;
    masked: boolean;
  } | null>(null);
  const request = useRef(0);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let active = true;
    request.current++;
    setSelected("");
    setPreview(null);
    api<Entry[]>("backups/" + backup.id + "/files")
      .then((v) => {
        if (active) setFiles(v);
      })
      .catch((e) => notify(e.message, true));
    return () => {
      active = false;
      request.current++;
    };
  }, [backup.id, notify]);
  async function open(path: string, reveal = false) {
    const sequence = ++request.current;
    setSelected(path);
    setPreview(null);
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
      if (sequence === request.current) notify((e as Error).message, true);
    } finally {
      if (sequence === request.current) setBusy(false);
    }
  }
  return (
    <Dialog title={"Dateien · " + backup.host_name} wide onClose={onClose}>
      <p className="dialog-intro">
        {date(backup.created_at)} · {backup.files} Dateien · Originalrechte
        bleiben im Manifest erhalten.
      </p>
      {backup.warnings?.map((w, i) => (
        <p className="notice warning" key={i}>
          {w}
        </p>
      ))}
      <div className="file-browser">
        <div className="file-list">
          <input
            aria-label="Dateien durchsuchen"
            placeholder="Pfad suchen"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <div className="file-items">
            {files
              .filter((f) => f.type !== "directory" && f.path.includes(query))
              .map((f) => (
                <button
                  className={selected === f.path ? "selected" : ""}
                  key={f.path}
                  onClick={() => open(f.path)}
                >
                  {f.path}
                  <small>
                    {f.secret
                      ? "Geschützt"
                      : f.type === "symlink"
                        ? "Verknüpfung"
                        : bytes(f.size)}
                  </small>
                </button>
              ))}
          </div>
        </div>
        <div className="file-preview">
          {busy ? (
            <p className="muted">Datei wird gelesen …</p>
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
          (user.secrets || !files.find((f) => f.path === selected)?.secret) && (
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
              onRestore(backup.id);
            }}
          >
            Wiederherstellung planen
          </button>
        )}
      </footer>
    </Dialog>
  );
}
