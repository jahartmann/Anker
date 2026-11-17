import { useListPageSize, PageSizeSelect } from "../listPageSize";
import { HostName } from "../hostNames";
import { useEffect, useState, useRef, useMemo } from "react";
import { api, bytes, date, preciseDate, calendarDate, plural } from "../api";
import type { Backup, BackupReference } from "../api";
import { ActionMenu, Download, Empty, Pagination, State } from "./shared";
import {
  Download as DownloadIcon,
  CalendarDays,
  ChevronRight,
  Search,
} from "lucide-react";
import type { Notify } from "./shared";
import BackupComparison from "./BackupComparison";
import type { ComparisonSelection } from "./BackupComparison";
import BackupReferencePanel from "./BackupReference";

export function BackupTable({
  backups,
  notify,
  refresh,
  openBackup,
  canEdit,
  canDownload = false,
  onRestore,
  filterKey = "",
  onCompare,
  onComparePrevious,
  comparisonPool,
  references,
  referenceHosts,
  historyMode = false,
}: {
  historyMode?: boolean;
  backups: Backup[];
  notify: Notify;
  refresh: () => void;
  openBackup: (id: string) => void;
  canEdit: boolean;
  canDownload?: boolean;
  onRestore: (id: string) => void;
  filterKey?: string;
  onCompare?: (side: "from" | "to", id: string) => void;
  onComparePrevious?: (id: string, previous: string) => void;
  comparisonPool?: Backup[];
  references?: Record<string, BackupReference>;
  referenceHosts?: { id: string; name: string }[];
}) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useListPageSize(
    historyMode ? "backup-history" : "backups",
    historyMode ? 10 : 25,
  );
  useEffect(() => setPage(1), [filterKey]);
  const sorted = [...backups].sort((a, b) =>
    b.created_at.localeCompare(a.created_at),
  );
  const newestByHost = useMemo(() => {
    const newest = new Map<string, number>();
    for (const backup of comparisonPool || backups) {
      const createdAt = Date.parse(backup.created_at);
      if (
        Number.isFinite(createdAt) &&
        createdAt > (newest.get(backup.host_id) ?? -Infinity)
      )
        newest.set(backup.host_id, createdAt);
    }
    return newest;
  }, [comparisonPool, backups]);
  const previousByID = useMemo(() => {
    const byHost = new Map<string, Backup[]>();
    for (const backup of comparisonPool || backups) {
      if (!Number.isFinite(Date.parse(backup.created_at))) continue;
      const group = byHost.get(backup.host_id) || [];
      group.push(backup);
      byHost.set(backup.host_id, group);
    }
    const result = new Map<string, Backup>();
    for (const group of byHost.values()) {
      group.sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));
      let index = 0;
      while (index < group.length) {
        let next = index + 1;
        while (
          next < group.length &&
          Date.parse(group[next].created_at) ===
            Date.parse(group[index].created_at)
        )
          next++;
        if (next < group.length)
          for (let same = index; same < next; same++)
            result.set(group[same].id, group[next]);
        index = next;
      }
    }
    return result;
  }, [comparisonPool, backups]);
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(sorted.length / pageSize)),
  );
  const [pending, setPending] = useState("");
  const [localComparison, setLocalComparison] = useState<ComparisonSelection>();
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
      {(referenceHosts || []).map((h) => {
        const panel = (
          <BackupReferencePanel
            key={h.id}
            hostID={h.id}
            hostName={h.name}
            backups={(comparisonPool || backups).filter(
              (b) => b.host_id === h.id,
            )}
            reference={references?.[h.id]}
            canEdit={canEdit}
            notify={notify}
            refresh={refresh}
            onCompare={(referenceID, targetID) => {
              if (onComparePrevious) onComparePrevious(targetID, referenceID);
              else
                setLocalComparison((current) => ({
                  side: "to",
                  id: targetID,
                  pairedId: referenceID,
                  sequence: (current?.sequence || 0) + 1,
                }));
            }}
          />
        );
        return historyMode &&
          (!references?.[h.id] || references[h.id].state === "missing") ? (
          <details className="reference-disclosure" key={h.id}>
            <summary>Referenzstand festlegen</summary>
            {panel}
          </details>
        ) : (
          panel
        );
      })}
      <div className="table-scroll">
        <table
          className={
            "backup-table" + (historyMode ? " backup-history-table" : "")
          }
          aria-label="Sicherungen"
        >
          <thead>
            <tr>
              <th>
                {historyMode ? "Sicherungszeitpunkt" : "Host / Zeitpunkt"}
              </th>
              <th>Status</th>
              <th>Umfang</th>
              <th className="right">Aktionen</th>
            </tr>
          </thead>
          <tbody>
            {sorted
              .slice((currentPage - 1) * pageSize, currentPage * pageSize)
              .map((b) => {
                const older = previousByID.get(b.id);
                const comparisonActions = [
                  ...(onComparePrevious || historyMode
                    ? [
                        {
                          label: "Mit vorherigem Stand vergleichen",
                          disabled: !older,
                          title: older
                            ? undefined
                            : "Kein älterer Stand dieses Hosts",
                          run: () => {
                            if (older) {
                              if (onComparePrevious)
                                onComparePrevious(b.id, older.id);
                              else
                                setLocalComparison((current) => ({
                                  side: "to",
                                  id: b.id,
                                  pairedId: older.id,
                                  sequence: (current?.sequence || 0) + 1,
                                }));
                            }
                          },
                        },
                      ]
                    : []),
                  ...(onCompare
                    ? [
                        {
                          label: "Als Ausgangsstand",
                          run: () => onCompare("from", b.id),
                        },
                        {
                          label: "Als Vergleichsstand",
                          run: () => onCompare("to", b.id),
                        },
                      ]
                    : []),
                ];
                return (
                  <tr key={b.id}>
                    <td>
                      <strong>
                        {historyMode ? (
                          preciseDate(b.created_at)
                        ) : (
                          <HostName name={b.host_name} />
                        )}
                      </strong>
                      {newestByHost.get(b.host_id) ===
                        Date.parse(b.created_at) && (
                        <span className="backup-latest-badge">
                          Neuester Stand
                        </span>
                      )}
                      <div className="secondary-line">
                        {!historyMode && <>{preciseDate(b.created_at)} · </>}
                        <code title={b.id}>{b.id.slice(0, 10)}</code>
                        {b.pinned ? " · Geschützt" : ""}
                        {references?.[b.host_id]?.backup_id === b.id
                          ? " · Benannte Referenz (geschützt)"
                          : ""}
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
                          {b.issues?.[0]?.title ||
                            plural(b.warnings.length, "Hinweis", "Hinweise")}
                        </div>
                      )}
                    </td>
                    <td>
                      {b.files} Einträge
                      <div className="secondary-line">{bytes(b.size)}</div>
                    </td>
                    <td className="right">
                      <div className="row-actions backup-actions">
                        <button
                          className="backup-files-button"
                          onClick={() => openBackup(b.id)}
                        >
                          Dateien
                        </button>
                        {canDownload && (
                          <span className="backup-download-action">
                            <Download
                              path={"backups/" + b.id + "/download"}
                              compact
                              notify={notify}
                            >
                              <DownloadIcon size={17} aria-hidden="true" />
                              <span className="sr-only">Herunterladen</span>
                            </Download>
                          </span>
                        )}
                        {(onCompare || onComparePrevious || historyMode) &&
                          !canEdit && (
                            <ActionMenu
                              label={
                                "Vergleich für " +
                                b.host_name +
                                " · " +
                                date(b.created_at)
                              }
                              actions={comparisonActions}
                            />
                          )}
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
                              ...comparisonActions,
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
                );
              })}
          </tbody>
        </table>
      </div>
      {(!historyMode || sorted.length > 10) && (
        <div className="list-footer">
          <PageSizeSelect
            value={pageSize}
            initial={historyMode ? 10 : 25}
            label="Sicherungen pro Seite"
            onChange={(size) => {
              setPageSize(size);
              setPage(1);
            }}
          />
          <Pagination
            page={currentPage}
            pageSize={pageSize}
            total={backups.length}
            onPageChange={setPage}
            label="Sicherungen"
          />
        </div>
      )}
      {localComparison && (
        <BackupComparison
          backups={comparisonPool || backups}
          user={{
            id: "",
            name: "",
            role: canEdit ? "restore" : "reader",
            secrets: canDownload,
            disabled: false,
          }}
          notify={notify}
          onRestore={onRestore}
          selection={localComparison}
        />
      )}
    </div>
  ) : (
    <div>
      {(referenceHosts || [])
        .filter((h) => references?.[h.id]?.state === "unresolved")
        .map((h) => (
          <BackupReferencePanel
            key={h.id}
            hostID={h.id}
            hostName={h.name}
            backups={[]}
            reference={references?.[h.id]}
            canEdit={canEdit}
            notify={notify}
            refresh={refresh}
          />
        ))}
      <Empty title="Noch keine Sicherungen">
        Eine Sicherung kann in der Hostansicht gestartet werden.
      </Empty>
    </div>
  );
}
export type HistoryProps = Omit<
  Parameters<typeof BackupTable>[0],
  "historyMode"
>;

function HistoryDays({ backups, ...props }: HistoryProps) {
  const [openDay, setOpenDay] = useState("");
  const days = new Map<string, { label: string; backups: Backup[] }>();
  for (const backup of backups) {
    const calendar = calendarDate(backup.created_at);
    const day = days.get(calendar.dayKey) || {
      label: calendar.dayLabel,
      backups: [],
    };
    day.backups.push(backup);
    days.set(calendar.dayKey, day);
  }
  return (
    <div className="history-days">
      {[...days].map(([key, day]) => (
        <details className="history-day" key={key} open={openDay === key}>
          <summary
            onClick={(e) => {
              e.preventDefault();
              setOpenDay(openDay === key ? "" : key);
            }}
          >
            <ChevronRight size={14} aria-hidden="true" />
            <span>{day.label}</span>
            <span className="history-group-count">
              {day.backups.length}{" "}
              {day.backups.length === 1 ? "Stand" : "Stände"}
            </span>
            <HistoryWarnings backups={day.backups} />
          </summary>
          {openDay === key && (
            <BackupTable
              {...props}
              backups={day.backups}
              referenceHosts={[]}
              historyMode
            />
          )}
        </details>
      ))}
    </div>
  );
}

function HistoryWarnings({ backups }: { backups: Backup[] }) {
  const count = backups.filter(
    (b) =>
      b.status !== "successful" || b.verification_error || b.warnings?.length,
  ).length;
  return count ? (
    <span className="history-group-warning">{count} mit Hinweisen</span>
  ) : null;
}

export function BackupHistory(props: HistoryProps) {
  const [query, setQuery] = useState("");
  const [openMonth, setOpenMonth] = useState("");
  const [page, setPage] = useState(1);
  const [monthPageSize, setMonthPageSize] = useListPageSize(
    "backup-months",
    12,
  );
  const sorted = [...props.backups].sort((a, b) =>
    b.created_at.localeCompare(a.created_at),
  );
  const filtered = sorted.filter((b) =>
    (date(b.created_at) + " " + b.id)
      .toLowerCase()
      .includes(query.trim().toLowerCase()),
  );
  const months = new Map<string, { label: string; backups: Backup[] }>();
  for (const backup of sorted.slice(1)) {
    const calendar = calendarDate(backup.created_at);
    const month = months.get(calendar.monthKey) || {
      label: calendar.monthLabel,
      backups: [],
    };
    month.backups.push(backup);
    months.set(calendar.monthKey, month);
  }
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(months.size / monthPageSize)),
  );
  const pool = props.comparisonPool || props.backups;
  return (
    <section className="backup-history" aria-label="Sicherungsverlauf">
      <div className="history-heading">
        <div>
          <span className="eyebrow">Sicherungsverlauf</span>
          <h3>
            {props.backups.length}{" "}
            {props.backups.length === 1
              ? "gespeicherter Stand"
              : "gespeicherte Stände"}
          </h3>
        </div>
        <div className="search-field">
          <Search size={15} aria-hidden="true" />
          <input
            aria-label="Sicherungsverlauf durchsuchen"
            placeholder="Datum oder Kennung suchen"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
      </div>
      {!sorted.length ? (
        <BackupTable {...props} historyMode />
      ) : query.trim() ? (
        filtered.length ? (
          <BackupTable
            {...props}
            backups={filtered}
            comparisonPool={pool}
            filterKey={(props.filterKey || "") + query}
            historyMode
          />
        ) : (
          <Empty title="Keine passenden Stände">
            Datum oder Kennung ändern.
          </Empty>
        )
      ) : (
        <>
          <div className="history-latest">
            <span className="eyebrow">Neuester Stand · direkt verfügbar</span>
            <BackupTable
              {...props}
              backups={sorted.slice(0, 1)}
              comparisonPool={pool}
              historyMode
            />
          </div>
          {!!months.size && (
            <div className="history-archive">
              <div className="history-archive-heading">
                <h3>Ältere Stände</h3>
                <span>Nach Monat und Tag gebündelt</span>
              </div>
              {[...months]
                .slice(
                  (currentPage - 1) * monthPageSize,
                  currentPage * monthPageSize,
                )
                .map(([key, month]) => (
                  <details
                    className="history-month"
                    key={key}
                    open={openMonth === key}
                  >
                    <summary
                      onClick={(e) => {
                        e.preventDefault();
                        setOpenMonth(openMonth === key ? "" : key);
                      }}
                    >
                      <span className="calendar-tile">
                        <CalendarDays
                          size={18}
                          strokeWidth={1.5}
                          aria-hidden="true"
                        />
                      </span>
                      <span className="history-month-name">{month.label}</span>
                      <span className="history-group-count">
                        {month.backups.length}{" "}
                        {month.backups.length === 1 ? "Stand" : "Stände"}
                      </span>
                      <HistoryWarnings backups={month.backups} />
                      <ChevronRight size={15} aria-hidden="true" />
                    </summary>
                    {openMonth === key && (
                      <HistoryDays
                        {...props}
                        backups={month.backups}
                        comparisonPool={pool}
                      />
                    )}
                  </details>
                ))}
              {months.size > 0 && (
                <Pagination
                  page={currentPage}
                  pageSize={monthPageSize}
                  onPageSizeChange={setMonthPageSize}
                  total={months.size}
                  onPageChange={setPage}
                  label="Monate"
                />
              )}
            </div>
          )}
        </>
      )}
    </section>
  );
}
