import BackupRemediation from "../components/BackupRemediation";
import { useListPageSize, PageSizeSelect } from "../listPageSize";
import { ServerRack as Server } from "../components/InfrastructureDetails";
import BackupFocus from "../components/BackupFocus";
import { HostName } from "../hostNames";
import { useState, useMemo } from "react";
import { date, plural } from "../api";
import type { Backup, Status, User } from "../api";
import {
  Empty,
  Heading,
  Pagination,
  PageEmblem,
  State,
} from "../components/shared";
import {
  History,
  Layers,
  GitCompareArrows,
  ChevronRight,
  Folder,
  Search,
} from "lucide-react";
import type { Notify } from "../components/shared";
import ActiveJobs from "../components/ActiveJobs";
import BackupComparison from "../components/BackupComparison";
import type { ComparisonSelection } from "../components/BackupComparison";
import BackupReferencePanel from "../components/BackupReference";
import ClusterSets from "../components/ClusterSets";
import { BackupHistory, BackupTable } from "../components/BackupHistory";
import type { HistoryProps } from "../components/BackupHistory";
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
        <span>
          Letzter Stand je Host · {plural(collections.length, "Host", "Hosts")}
        </span>
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
            {plural(status.backups.length, "Stand", "Stände")} ·{" "}
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
