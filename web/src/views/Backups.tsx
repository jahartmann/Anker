import BackupRemediation from "../components/BackupRemediation";
import { useListPageSize, PageSizeSelect } from "../listPageSize";
import { ServerRack as Server } from "../components/InfrastructureDetails";
import BackupFocus from "../components/BackupFocus";
import BackupIssues from "../components/BackupIssues";
import { HostName } from "../hostNames";
import { useEffect, useState, useRef, useMemo } from "react";
import { api, bytes, date, preciseDate, calendarDate } from "../api";
import type { Backup, BackupReference, Entry, Status, User } from "../api";
import {
  ActionMenu,
  Dialog,
  Download,
  Empty,
  Heading,
  LoadError,
  Pagination,
  PageEmblem,
  State,
} from "../components/shared";
import {
  Download as DownloadIcon,
  History,
  CalendarDays,
  Layers,
  GitCompareArrows,
  ChevronRight,
  File,
  Folder,
  Link,
  Search,
} from "lucide-react";
import FileNavigation, {
  useFileNavigation,
} from "../components/files/FileNavigation";
import {
  indexManifest,
  parentPath,
  visibleEntries,
} from "../components/files/manifest";
import type { Notify } from "../components/shared";
import ActiveJobs from "../components/ActiveJobs";
import BackupComparison from "../components/BackupComparison";
import type { ComparisonSelection } from "../components/BackupComparison";
import BackupReadiness from "../components/BackupReadiness";
import BackupReferencePanel from "../components/BackupReference";
import ClusterSets from "../components/ClusterSets";
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
                            `${b.warnings.length} Hinweise`}
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
type HistoryProps = Omit<Parameters<typeof BackupTable>[0], "historyMode">;

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

function BackupCollections({
  backups,
  status,
  ...props
}: HistoryProps & { status: Status }) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useListPageSize("latest-backups", 6);
  const [expanded, setExpanded] = useState("");
  const collections = useMemo(() => {
    const groups = new Map<string, Backup[]>();
    for (const backup of [...backups].sort((a, b) =>
      b.created_at.localeCompare(a.created_at),
    )) {
      const group = groups.get(backup.host_id) || [];
      group.push(backup);
      groups.set(backup.host_id, group);
    }
    return [...groups.entries()];
  }, [backups]);
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(collections.length / pageSize)),
  );
  if (!backups.length)
    return (
      <div>
        {status.hosts
          .filter((h) => status.references?.[h.id]?.state === "unresolved")
          .map((h) => (
            <BackupReferencePanel
              key={h.id}
              hostID={h.id}
              hostName={h.name}
              backups={[]}
              reference={status.references?.[h.id]}
              canEdit={props.canEdit}
              notify={props.notify}
              refresh={props.refresh}
            />
          ))}
        <Empty title="Noch keine Sicherungen">
          Eine Sicherung kann in der Hostansicht gestartet werden.
        </Empty>
      </div>
    );
  return (
    <div className="backup-collections">
      <div className="collection-display-options">
        <span>Letzter Stand je Host · {collections.length} Hosts</span>
        <PageSizeSelect
          value={pageSize}
          initial={6}
          label="Letzte Sicherungen pro Seite"
          onChange={(size) => {
            setPageSize(size);
            setPage(1);
          }}
        />
      </div>
      <div className="collection-label">
        <span>Host & letzter passender Stand</span>
        <span>Verlauf</span>
      </div>
      {collections
        .slice((currentPage - 1) * pageSize, currentPage * pageSize)
        .map(([hostID, history]) => {
          const latest = history[0];
          const host = status.hosts.find((h) => h.id === hostID);
          return (
            <section
              className={
                "backup-collection" + (expanded === hostID ? " is-open" : "")
              }
              key={hostID}
              aria-label={"Sicherungen von " + latest.host_name}
            >
              <div className="collection-row">
                <PageEmblem tone="blue">
                  <Server size={19} strokeWidth={1.6} aria-hidden="true" />
                </PageEmblem>
                <div className="collection-identity">
                  <h3 aria-label={latest.host_name}>
                    <HostName name={latest.host_name} />
                  </h3>
                  <p>
                    {date(latest.created_at)}
                    {!host && " · Host entfernt"}
                    {host?.group && " · " + host.group}
                  </p>
                  {status.references?.[hostID]?.state === "unresolved" && (
                    <p className="collection-warning">
                      Referenz nicht auflösbar · im Verlauf prüfen
                    </p>
                  )}
                  {latest.verification_error && (
                    <p className="collection-warning">
                      Integritätsprüfung fehlgeschlagen
                    </p>
                  )}
                  {latest.issues?.[0] && (
                    <p className="collection-warning">
                      {latest.issues[0].title}
                      {latest.issues[0].path
                        ? ` · ${latest.issues[0].path}`
                        : ""}
                    </p>
                  )}
                </div>
                <State
                  value={latest.verification_error ? "damaged" : latest.status}
                />
                <button
                  className="text-button collection-files"
                  onClick={() => props.openBackup(latest.id)}
                >
                  <Folder size={15} aria-hidden="true" /> Dateien
                </button>
                <button
                  className="history-toggle"
                  aria-expanded={expanded === hostID}
                  aria-controls={"history-" + hostID}
                  onClick={() => setExpanded(expanded === hostID ? "" : hostID)}
                  aria-label={"Verlauf für " + latest.host_name}
                >
                  <History size={15} aria-hidden="true" />
                  <span>
                    {history.length} {history.length === 1 ? "Stand" : "Stände"}
                  </span>
                  <ChevronRight size={14} aria-hidden="true" />
                </button>
              </div>
              {expanded === hostID && (
                <div className="collection-history" id={"history-" + hostID}>
                  <BackupHistory
                    {...props}
                    backups={history}
                    comparisonPool={status.backups}
                    filterKey={hostID}
                    references={status.references}
                    referenceHosts={
                      host ? [host] : [{ id: hostID, name: latest.host_name }]
                    }
                  />
                </div>
              )}
            </section>
          );
        })}
      <Pagination
        page={currentPage}
        pageSize={pageSize}
        total={collections.length}
        onPageChange={setPage}
        label="Hosts"
      />
    </div>
  );
}

export default function Backups({
  status,
  openJob,
  user,
  ...props
}: {
  status: Status;
  user: User;
  notify: Notify;
  refresh: () => void;
  openBackup: (id: string) => void;
  canEdit: boolean;
  canDownload?: boolean;
  onRestore: (id: string) => void;
  openJob?: (id: string) => void;
}) {
  const [showSets, setShowSets] = useState(false);
  const [view, setView] = useState<"hosts" | "all">("hosts");
  const [host, setHost] = useState("");
  const [query, setQuery] = useState("");
  const [state, setState] = useState("");
  const [protection, setProtection] = useState("");
  const [comparisonSelection, setComparisonSelection] =
    useState<ComparisonSelection>();
  const [comparing, setComparing] = useState(false);
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
  const showFocus =
    view === "hosts" &&
    !query &&
    !state &&
    !protection &&
    status.backups.some((b) => !host || b.host_id === host);
  const hostPicker = (
    <select
      aria-label="Sicherungen nach Host filtern"
      value={host}
      onChange={(e) => {
        setHost(e.target.value);
      }}
    >
      <option value="">Alle Hosts</option>
      {Array.from(knownHosts).map(([id, name]) => (
        <option key={id} value={id}>
          {name}
        </option>
      ))}
    </select>
  );
  if (showSets)
    return (
      <ClusterSets
        hosts={status.hosts}
        canEdit={props.canEdit}
        onBack={() => setShowSets(false)}
      />
    );
  return (
    <>
      {comparing && (
        <div className="comparison-workspace comparison-detail-page">
          <button className="back-button" onClick={() => setComparing(false)}>
            <span aria-hidden="true">←</span> Zurück zu Sicherungen
          </button>
          <Heading
            title="Sicherungsvergleich"
            description="Zwei Sicherungen auswählen und vergleichen."
          />
          <BackupComparison
            backups={[...status.backups].sort((a, b) =>
              b.created_at.localeCompare(a.created_at),
            )}
            user={user}
            notify={props.notify}
            onRestore={props.onRestore}
            selection={comparisonSelection}
            spacious
            collapseSources={false}
          />
        </div>
      )}
      <div hidden={comparing}>
        <Heading
          title="Sicherungen"
          description="Gesicherte Dateien, Inventar und Prüfergebnisse ansehen."
          action={
            <div className="cluster-set-actions">
              <button className="secondary" onClick={() => setShowSets(true)}>
                <Layers size={15} aria-hidden="true" /> Cluster-Sicherungssets
              </button>
              <button className="secondary" onClick={() => setComparing(true)}>
                <GitCompareArrows size={15} aria-hidden="true" /> Stände
                vergleichen
              </button>
            </div>
          }
        />
        <BackupRemediation
          status={status}
          actorID={user.id}
          canEdit={props.canEdit}
          refresh={props.refresh}
          openJob={openJob}
          openBackup={props.openBackup}
        />
        {showFocus && (
          <BackupFocus
            status={status}
            hostID={host}
            hostPicker={hostPicker}
            canRestore={user.role !== "reader"}
            onOpen={props.openBackup}
            onRestore={props.onRestore}
            onCompare={(from, to) => {
              setComparisonSelection((current) => ({
                side: "to",
                id: to,
                pairedId: from,
                sequence: (current?.sequence || 0) + 1,
              }));
              setComparing(true);
              window.scrollTo(0, 0);
            }}
          />
        )}
        <div className="collection-viewbar">
          <div
            className="view-switch"
            role="group"
            aria-label="Sicherungsansicht"
          >
            <button
              className={view === "hosts" ? "selected" : ""}
              aria-pressed={view === "hosts"}
              onClick={() => setView("hosts")}
            >
              <Server size={14} aria-hidden="true" /> Nach Host
            </button>
            <button
              className={view === "all" ? "selected" : ""}
              aria-pressed={view === "all"}
              onClick={() => setView("all")}
            >
              <History size={14} aria-hidden="true" /> Alle Stände
            </button>
          </div>
          <span className="muted collection-caption">
            {status.backups.length} Stände ·{" "}
            {new Set(status.backups.map((b) => b.host_id)).size}{" "}
            {new Set(status.backups.map((b) => b.host_id)).size === 1
              ? "Host"
              : "Hosts"}
          </span>
        </div>
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
          {!showFocus && hostPicker}
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
              (j) =>
                ["backup", "backup_verify"].includes(j.kind) &&
                (!host || j.host_id === host),
            ),
          }}
          openJob={openJob}
          title="Aktuelle Sicherungsaufträge"
        />
        {!backups.length && status.backups.length ? (
          <Empty title="Keine passenden Sicherungen">
            Suche oder Filter anpassen.
          </Empty>
        ) : view === "hosts" ? (
          <BackupCollections
            key={[host, query, state, protection].join("|")}
            backups={backups}
            status={status}
            {...props}
            onComparePrevious={(id, previous) => {
              setComparisonSelection((current) => ({
                side: "to",
                id,
                pairedId: previous,
                sequence: (current?.sequence || 0) + 1,
              }));
              setComparing(true);
              window.scrollTo(0, 0);
            }}
          />
        ) : (
          <BackupTable
            onComparePrevious={(id, previous) => {
              setComparisonSelection((current) => ({
                side: "to",
                id,
                pairedId: previous,
                sequence: (current?.sequence || 0) + 1,
              }));
              setComparing(true);
            }}
            onCompare={(side, id) => {
              setComparisonSelection((current) => ({
                side,
                id,
                sequence: (current?.sequence || 0) + 1,
              }));
              setComparing(true);
            }}
            filterKey={[host, query, state, protection].join("|")}
            comparisonPool={status.backups}
            references={status.references}
            referenceHosts={status.hosts.filter((h) =>
              host
                ? h.id === host
                : !!status.references?.[h.id] &&
                  status.references[h.id].state !== "missing",
            )}
            backups={backups}
            {...props}
          />
        )}
      </div>
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
