import { useEffect, useMemo, useRef, useState } from "react";
import { api, preciseDate } from "../api";
import type { Backup, FileComparison, FileDiff, User } from "../api";
import { Download, Empty, LoadError, Pagination } from "./shared";
import type { Notify } from "./shared";
import FileComparisonView from "./FileComparison";

export interface ComparisonSelection {
  side: "from" | "to";
  id: string;
  sequence: number;
  pairedId?: string;
}
const changeLabel = {
  added: "Hinzugefügt",
  removed: "Entfernt",
  changed: "Geändert",
};
function stand(b: Backup) {
  return `${b.host_name} · ${preciseDate(b.created_at)} · ${b.id.slice(0, 10)}`;
}
export default function BackupComparison({
  backups,
  user,
  notify,
  onRestore,
  selection,
}: {
  backups: Backup[];
  user: User;
  notify: Notify;
  onRestore: (id: string) => void;
  selection?: ComparisonSelection;
}) {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [rows, setRows] = useState<FileDiff[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [summaryError, setSummaryError] = useState("");
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const [selected, setSelected] = useState("");
  const [comparison, setComparison] = useState<FileComparison | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const summaryGeneration = useRef(0);
  const contentGeneration = useRef(0);
  function clearFile() {
    contentGeneration.current++;
    setSelected("");
    setComparison(null);
    setError("");
    setLoading(false);
  }
  function invalidate() {
    summaryGeneration.current++;
    setRows(null);
    setSummaryError("");
    setBusy(false);
    setPage(1);
    clearFile();
  }
  function select(side: "from" | "to", id: string) {
    invalidate();
    if (side === "from") {
      setFrom(id);
      if (id === to) setTo("");
    } else {
      setTo(id);
      if (id === from) setFrom("");
    }
  }
  useEffect(() => {
    if (!selection) return;
    if (selection.pairedId && selection.side === "to") {
      invalidate();
      setFrom(selection.pairedId);
      setTo(selection.id);
      void compare(selection.pairedId, selection.id);
    } else select(selection.side, selection.id);
  }, [selection]);
  useEffect(
    () => () => {
      summaryGeneration.current++;
      contentGeneration.current++;
    },
    [],
  );
  const before = backups.find((b) => b.id === from);
  const after = backups.find((b) => b.id === to);
  const valid = !!before && !!after && from !== to;
  async function compare(first = from, second = to) {
    const generation = ++summaryGeneration.current;
    clearFile();
    setRows(null);
    setSummaryError("");
    setBusy(true);
    setPage(1);
    try {
      const result = await api<FileDiff[]>("backups/compare", "POST", {
        from: first,
        to: second,
      });
      if (generation === summaryGeneration.current) setRows(result);
    } catch (e) {
      if (generation === summaryGeneration.current)
        setSummaryError((e as Error).message);
    } finally {
      if (generation === summaryGeneration.current) setBusy(false);
    }
  }
  async function open(path: string, reveal = false) {
    const generation = ++contentGeneration.current;
    setSelected(path);
    setComparison(null);
    setError("");
    setLoading(true);
    try {
      const result = await api<FileComparison>("backups/compare/file", "POST", {
        from,
        to,
        path,
        ...(reveal ? { reveal: true } : {}),
      });
      if (generation === contentGeneration.current) setComparison(result);
    } catch (e) {
      if (generation === contentGeneration.current)
        setError((e as Error).message);
    } finally {
      if (generation === contentGeneration.current) setLoading(false);
    }
  }
  const filtered = useMemo(
    () =>
      (rows || []).filter(
        (r) =>
          r.path.toLowerCase().includes(query.trim().toLowerCase()) &&
          (!filter ||
            (filter === "content"
              ? r.content_changed
              : filter === "metadata"
                ? r.metadata_changed
                : filter === "mtime"
                  ? r.mtime_only
                  : filter === "secret"
                    ? r.secret
                    : r.change === filter)),
      ),
    [rows, query, filter],
  );
  const counts = useMemo(
    () =>
      (rows || []).reduce(
        (c, r) => ({
          content: c.content + Number(r.content_changed),
          metadata: c.metadata + Number(r.metadata_changed),
          mtime: c.mtime + Number(r.mtime_only),
        }),
        { content: 0, metadata: 0, mtime: 0 },
      ),
    [rows],
  );
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(filtered.length / pageSize)),
  );
  const selectedRow = rows?.find((r) => r.path === selected);
  const canDownload =
    !!comparison &&
    !comparison.masked &&
    comparison.state !== "masked" &&
    comparison.state !== "unavailable" &&
    (user.secrets || !selectedRow?.secret);
  return (
    <section
      className="detail-section backup-comparison"
      aria-label="Sicherungsvergleich"
    >
      <h3>Stände vergleichen</h3>
      <p className="muted">
        Richtung: Ausgangsstand → Vergleichsstand. Die Auswahl einer
        Wiederherstellungsquelle erfolgt getrennt.
      </p>
      <div className="comparison-selectors">
        <label>
          Ausgangsstand
          <select
            aria-label="Vergleich von"
            value={from}
            onChange={(e) => select("from", e.target.value)}
          >
            <option value="">Stand wählen</option>
            {backups.map((b) => (
              <option key={b.id} value={b.id}>
                {stand(b)}
              </option>
            ))}
          </select>
        </label>
        <span className="comparison-arrow" aria-hidden="true">
          →
        </span>
        <label>
          Vergleichsstand
          <select
            aria-label="Vergleich bis"
            value={to}
            onChange={(e) => select("to", e.target.value)}
          >
            <option value="">Stand wählen</option>
            {backups
              .filter((b) => b.id !== from)
              .map((b) => (
                <option key={b.id} value={b.id}>
                  {stand(b)}
                </option>
              ))}
          </select>
        </label>
      </div>
      <div className="comparison-actions">
        <button
          className="secondary"
          disabled={!valid || busy}
          onClick={() => compare()}
        >
          {busy ? "Vergleicht …" : "Vergleichen"}
        </button>
        <button
          className="text-button"
          disabled={!valid || busy}
          onClick={() => {
            setFrom(to);
            setTo(from);
            void compare(to, from);
          }}
        >
          Richtung umkehren
        </button>
      </div>
      {valid && (
        <>
          <div className="comparison-direction">
            <div>
              <small>Ausgangsstand · Vorher</small>
              <strong>{before.host_name}</strong>
              <span>
                {preciseDate(before.created_at)} ·{" "}
                <code>{before.id.slice(0, 10)}</code>
              </span>
              <details>
                <summary>Vollständige ID</summary>
                <code>{before.id}</code>
              </details>
            </div>
            <span aria-hidden="true">→</span>
            <div>
              <small>Vergleichsstand · Nachher</small>
              <strong>{after.host_name}</strong>
              <span>
                {preciseDate(after.created_at)} ·{" "}
                <code>{after.id.slice(0, 10)}</code>
              </span>
              <details>
                <summary>Vollständige ID</summary>
                <code>{after.id}</code>
              </details>
            </div>
          </div>
          {before.host_id !== after.host_id && (
            <p className="warning">Hostübergreifender Vergleich</p>
          )}
          {user.role !== "reader" && (
            <div className="comparison-actions comparison-restore-actions">
              <button className="secondary" onClick={() => onRestore(from)}>
                Ausgangsstand zur Wiederherstellung wählen
              </button>
              <button className="secondary" onClick={() => onRestore(to)}>
                Vergleichsstand zur Wiederherstellung wählen
              </button>
            </div>
          )}
        </>
      )}
      {busy ? (
        <p className="muted" role="status">
          Änderungsübersicht wird geladen …
        </p>
      ) : summaryError ? (
        <LoadError message={summaryError} retry={() => compare()} />
      ) : (
        rows !== null && (
          <>
            <p className="comparison-counts">
              {rows.length} Änderungen · {counts.content} Inhalt ·{" "}
              {counts.metadata} Metadaten · {counts.mtime} nur Änderungszeit
            </p>
            {!rows.length ? (
              <Empty title="Keine Unterschiede">
                Inhalt und Metadaten der gewählten Stände stimmen überein.
              </Empty>
            ) : (
              <>
                <div className="toolbar comparison-filters">
                  <input
                    aria-label="Änderungen durchsuchen"
                    placeholder="Dateipfad suchen"
                    value={query}
                    onChange={(e) => {
                      setQuery(e.target.value);
                      setPage(1);
                    }}
                  />
                  <select
                    aria-label="Änderungsart"
                    value={filter}
                    onChange={(e) => {
                      setFilter(e.target.value);
                      setPage(1);
                    }}
                  >
                    <option value="">Alle Änderungen</option>
                    <option value="content">Inhalt geändert</option>
                    <option value="metadata">Metadaten geändert</option>
                    <option value="mtime">Nur Änderungszeit</option>
                    <option value="added">Hinzugefügt</option>
                    <option value="removed">Entfernt</option>
                    <option value="secret">Geschützte Dateien</option>
                  </select>
                </div>
                <div className="comparison-browser">
                  <div className="comparison-list">
                    <div className="comparison-items">
                      {filtered
                        .slice(
                          (currentPage - 1) * pageSize,
                          currentPage * pageSize,
                        )
                        .map((r) => (
                          <button
                            className={
                              "file-entry" +
                              (selected === r.path ? " selected" : "")
                            }
                            key={r.path}
                            aria-pressed={selected === r.path}
                            aria-label={`${r.path} · ${changeLabel[r.change]}`}
                            onClick={() => open(r.path)}
                          >
                            <span className="file-entry-name">
                              <strong className="mono">{r.path}</strong>
                              <small>
                                {changeLabel[r.change]}
                                {r.content_changed ? " · Inhalt" : ""}
                                {r.metadata_changed ? " · Metadaten" : ""}
                                {r.mtime_only ? " · Nur Änderungszeit" : ""}
                                {r.secret ? " · Geschützt" : ""}
                              </small>
                            </span>
                          </button>
                        ))}
                      {!filtered.length && (
                        <p className="muted file-list-message">
                          Keine passenden Änderungen
                        </p>
                      )}
                    </div>
                    <div className="file-list-footer">
                      <select
                        aria-label="Änderungen pro Seite"
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
                        total={filtered.length}
                        onPageChange={setPage}
                        label="Änderungen"
                      />
                    </div>
                  </div>
                  <div className="comparison-preview" aria-busy={loading}>
                    {selected && !comparison && (
                      <strong className="mono comparison-path">
                        {selected}
                      </strong>
                    )}
                    <FileComparisonView
                      comparison={comparison}
                      loading={loading}
                      error={error}
                      retry={() => open(selected)}
                    />
                    {comparison &&
                      (comparison.masked || comparison.state === "masked") &&
                      user.secrets && (
                        <button
                          className="secondary"
                          onClick={() => open(selected, true)}
                        >
                          Geschützten Inhalt anzeigen
                        </button>
                      )}
                    {comparison && selected && (
                      <div className="comparison-actions">
                        {canDownload && selectedRow?.change !== "added" && (
                          <Download
                            path={`backups/${from}/file-download?path=${encodeURIComponent(selected)}`}
                            notify={notify}
                          >
                            Original vorher
                          </Download>
                        )}
                        {canDownload && selectedRow?.change !== "removed" && (
                          <Download
                            path={`backups/${to}/file-download?path=${encodeURIComponent(selected)}`}
                            notify={notify}
                          >
                            Original nachher
                          </Download>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              </>
            )}
          </>
        )
      )}
    </section>
  );
}
