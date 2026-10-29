import { useListPageSize } from "../listPageSize";
import { traceableSettings } from "../settingHistory";
import { changeSubject } from "../changeHighlights";
import { displayHostName, HostName } from "../hostNames";
import { changeArea } from "./ChangeSummary";
import {
  Network,
  HardDrive,
  Settings2,
  Files,
  GitCompareArrows,
} from "lucide-react";
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
  spacious = false,
  collapseSources = spacious,
  onTrackSetting,
}: {
  backups: Backup[];
  user: User;
  notify: Notify;
  onRestore: (id: string) => void;
  selection?: ComparisonSelection;
  spacious?: boolean;
  collapseSources?: boolean;
  onTrackSetting?: (id: string) => void;
}) {
  const [scope, setScope] = useState("");
  const [area, setArea] = useState("");
  const nodes = Array.from(
    new Map(backups.map((b) => [b.host_id, b.host_name])).entries(),
  );
  const scopedBackups = scope
    ? backups.filter((b) => b.host_id === scope)
    : backups;
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [rows, setRows] = useState<FileDiff[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [summaryError, setSummaryError] = useState("");
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useListPageSize("comparison-files", 25);
  const [selected, setSelected] = useState("");
  const [comparison, setComparison] = useState<FileComparison | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const previewRef = useRef<HTMLDivElement>(null);
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
    setArea("");
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
    setScope("");
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
          (!area || changeArea(r.path) === area) &&
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
    [rows, query, filter, area],
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
  const sourceControls = (
    <>
      {" "}
      <div className="comparison-selectors">
        <label>
          Ausgangsstand
          <select
            aria-label="Vergleich von"
            value={from}
            onChange={(e) => select("from", e.target.value)}
          >
            <option value="">Stand wählen</option>
            {scopedBackups.map((b) => (
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
            {scopedBackups
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
    </>
  );
  return (
    <section
      className="detail-section backup-comparison"
      data-surface-topic="activity"
      aria-label="Sicherungsvergleich"
    >
      <div className="comparison-intro">
        <span className="eyebrow">
          <GitCompareArrows size={15} aria-hidden="true" /> Sicherungsvergleich
        </span>
        <h3>Stände vergleichen</h3>
        <p>
          {nodes.length === 1
            ? "Vergleiche, was sich auf diesem Host geändert hat."
            : "Wähle einen Host und zwei seiner Sicherungen."}
        </p>
      </div>
      {nodes.length > 1 && (
        <label className="comparison-node-scope">
          Node im Vergleich
          <select
            aria-label="Node im Vergleich"
            value={scope}
            onChange={(e) => {
              const id = e.target.value;
              setScope(id);
              invalidate();
              setFrom("");
              setTo("");
              const candidates = backups
                .filter(
                  (b) =>
                    b.host_id === id &&
                    b.status === "successful" &&
                    !b.verification_error &&
                    Number.isFinite(Date.parse(b.created_at)) &&
                    Date.parse(b.created_at) <= Date.now() + 300000,
                )
                .sort(
                  (a, b) => Date.parse(b.created_at) - Date.parse(a.created_at),
                );
              const after = candidates[0],
                before =
                  after &&
                  candidates.find(
                    (b) =>
                      Date.parse(b.created_at) < Date.parse(after.created_at),
                  );
              if (before && after) {
                setFrom(before.id);
                setTo(after.id);
                void compare(before.id, after.id);
              }
            }}
          >
            <option value="">Alle Nodes · freie Auswahl</option>
            {nodes.map(([id, name]) => (
              <option key={id} value={id}>
                {displayHostName(name)}
              </option>
            ))}
          </select>
        </label>
      )}
      <p className="muted comparison-reading-note">
        Richtung: Ausgangsstand → Vergleichsstand. Die Auswahl einer
        Wiederherstellungsquelle erfolgt getrennt.
      </p>
      {collapseSources ? (
        <details className="comparison-source-controls">
          <summary>Andere Stände dieser Node vergleichen</summary>
          {sourceControls}
        </details>
      ) : (
        sourceControls
      )}
      {valid && (
        <>
          <div className="comparison-direction">
            <div>
              <small>Ausgangsstand · Vorher</small>
              <strong>
                <HostName name={before.host_name} />
              </strong>
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
              <strong>
                <HostName name={after.host_name} />
              </strong>
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
          {user.role !== "reader" &&
            (spacious ? (
              <details className="comparison-restore-tools">
                <summary>
                  Wiederherstellung aus diesen Ständen vorbereiten
                </summary>{" "}
                <div className="comparison-actions comparison-restore-actions">
                  <button className="secondary" onClick={() => onRestore(from)}>
                    Ausgangsstand zur Wiederherstellung wählen
                  </button>
                  <button className="secondary" onClick={() => onRestore(to)}>
                    Vergleichsstand zur Wiederherstellung wählen
                  </button>
                </div>
              </details>
            ) : (
              <>
                {" "}
                <div className="comparison-actions comparison-restore-actions">
                  <button className="secondary" onClick={() => onRestore(from)}>
                    Ausgangsstand zur Wiederherstellung wählen
                  </button>
                  <button className="secondary" onClick={() => onRestore(to)}>
                    Vergleichsstand zur Wiederherstellung wählen
                  </button>
                </div>
              </>
            ))}
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
            {spacious && (
              <section
                className="detail-change-brief"
                aria-label="Was sich geändert hat"
              >
                <h3>Was sich geändert hat</h3>
                <p className="change-brief-intro">
                  {rows.filter((r) => !r.mtime_only).length}{" "}
                  {rows.filter((r) => !r.mtime_only).length === 1
                    ? "geänderte Datei"
                    : "geänderte Dateien"}
                  . Wähle eine Änderung, um die Werte vorher und nachher zu
                  sehen.
                </p>
                <div className="change-story-grid">
                  {rows
                    .filter((r) => !r.mtime_only)
                    .slice(0, 6)
                    .map((r) => (
                      <button
                        className={
                          "change-story-card" +
                          (selected === r.path ? " selected" : "")
                        }
                        key={r.path}
                        aria-label={`Änderung ansehen: ${r.path}`}
                        aria-pressed={selected === r.path}
                        onClick={() => {
                          void open(r.path);
                          previewRef.current?.focus();
                        }}
                      >
                        <span className="change-story-top">
                          <span>{changeArea(r.path)}</span>
                          <span>
                            {r.change === "added"
                              ? "Neu"
                              : r.change === "removed"
                                ? "Entfernt"
                                : r.content_changed
                                  ? "Inhalt"
                                  : "Metadaten"}
                          </span>
                        </span>
                        <strong>{changeSubject(r)}</strong>
                        <span className="change-story-description">
                          {r.secret
                            ? "Geschützte Konfiguration · Inhalte bleiben verborgen."
                            : r.change === "added"
                              ? "Diese Datei ist im Vergleichsstand hinzugekommen."
                              : r.change === "removed"
                                ? "Diese Datei ist im Vergleichsstand nicht mehr vorhanden."
                                : r.content_changed
                                  ? "Der Dateiinhalt unterscheidet sich zwischen diesen Ständen."
                                  : "Dateieigenschaften wurden aktualisiert."}
                        </span>
                        <code>{r.path}</code>
                        <span className="change-story-link">
                          Änderung ansehen <span aria-hidden="true">↗</span>
                        </span>
                      </button>
                    ))}
                </div>
                {rows.filter((r) => !r.mtime_only).length > 6 && (
                  <p className="muted">
                    Weitere Änderungen findest du in der Dateiauswahl unten.
                  </p>
                )}
                {!rows.filter((r) => !r.mtime_only).length && (
                  <p className="muted">
                    {rows.length
                      ? "Nur Zeitangaben aktualisiert; Inhalte bleiben unverändert."
                      : "Die Stände stimmen überein."}
                  </p>
                )}
                <details className="comparison-explanation">
                  <summary>Wie die Zusammenfassung entsteht</summary>
                  <p>
                    Beschreibungen beziehen sich auf erfasste Dateien und
                    Metadaten. Die genauen Werte stehen im Dateivergleich.
                  </p>
                </details>
              </section>
            )}
            <div
              className="comparison-area-cards"
              aria-label="Änderungsbereiche"
            >
              {[
                ["Netzwerk", Network],
                ["Storage", HardDrive],
                ["System", Settings2],
                ["Weitere Konfiguration", Files],
              ].map(([name, Icon]) => {
                const label = name as string;
                const AreaIcon = Icon as typeof Network;
                const count = rows.filter(
                  (r) => changeArea(r.path) === label && !r.mtime_only,
                ).length;
                return (
                  <button
                    key={label}
                    className={
                      "comparison-area-card" +
                      (count ? " has-changes" : "") +
                      (area === label ? " selected" : "")
                    }
                    aria-pressed={area === label}
                    onClick={() => {
                      setArea(area === label ? "" : label);
                      setPage(1);
                    }}
                  >
                    <AreaIcon size={18} aria-hidden="true" />
                    <strong>{label}</strong>
                    <span>
                      {count
                        ? `${count} ${count === 1 ? "Änderung" : "Änderungen"}`
                        : "Keine Änderung erfasst"}
                    </span>
                  </button>
                );
              })}
            </div>
            {area && (
              <button
                className="text-button"
                onClick={() => {
                  setArea("");
                  setPage(1);
                }}
              >
                Bereichsfilter aufheben
              </button>
            )}
            <p className="comparison-counts">
              {rows.filter((r) => !r.mtime_only).length} wesentliche Änderung
              {rows.filter((r) => !r.mtime_only).length === 1
                ? ""
                : "en"} · {counts.content} Inhalt · {counts.metadata} Metadaten
              · {counts.mtime} nur Änderungszeit
            </p>
            {!rows.length ? (
              <Empty title="Keine Unterschiede">
                Inhalt und Metadaten der gewählten Stände stimmen überein.
              </Empty>
            ) : (
              <>
                <div className="comparison-browser">
                  <div className="comparison-list">
                    <div className="comparison-pane-heading">
                      <strong>Dateien im Vergleich</strong>
                      <span>{filtered.length}</span>
                    </div>
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
                      <div
                        className="change-filter-chips"
                        role="group"
                        aria-label="Änderungsart"
                      >
                        {[
                          ["", "Alle"],
                          ["content", "Inhalt"],
                          ["metadata", "Eigenschaften"],
                          ["mtime", "Zeitangaben"],
                          ["added", "Neu"],
                          ["removed", "Entfernt"],
                          ["secret", "Geschützt"],
                        ].map(([value, label]) => (
                          <button
                            key={value}
                            className="change-filter-chip"
                            aria-pressed={filter === value}
                            onClick={() => {
                              setFilter(value);
                              setPage(1);
                            }}
                          >
                            {label}
                          </button>
                        ))}
                      </div>
                    </div>
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
                              <strong>
                                {r.path.split("/").filter(Boolean).at(-1)}
                              </strong>
                              <span className="file-entry-directory mono">
                                {r.path}
                              </span>
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
                    {filtered.length > 25 && (
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
                          <option value={100}>100 pro Seite</option>
                          <option value={200}>200 pro Seite</option>
                        </select>
                        <Pagination
                          page={currentPage}
                          pageSize={pageSize}
                          total={filtered.length}
                          onPageChange={setPage}
                          label="Änderungen"
                        />
                      </div>
                    )}
                  </div>
                  <div
                    ref={previewRef}
                    tabIndex={-1}
                    className="comparison-preview"
                    aria-busy={loading}
                  >
                    <div className="comparison-pane-heading">
                      <strong>Dateidetails</strong>
                      <span>Vorher → Nachher</span>
                    </div>
                    {selected && !comparison && (
                      <strong className="mono comparison-path">
                        {selected}
                      </strong>
                    )}
                    <FileComparisonView
                      collapseContext
                      comparison={comparison}
                      loading={loading}
                      error={error}
                      retry={() => open(selected)}
                    />
                    {comparison && onTrackSetting && (
                      <div className="comparison-trace-actions">
                        {traceableSettings(comparison).map((s) => (
                          <button
                            key={s.id}
                            className="secondary"
                            onClick={() => onTrackSetting(s.id)}
                          >
                            {s.name} verfolgen <span aria-hidden="true">↗</span>
                          </button>
                        ))}
                      </div>
                    )}
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
