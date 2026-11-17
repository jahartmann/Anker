import HostOperations, { HostDrift } from "../components/HostOperations";
import BackupRemediation from "../components/BackupRemediation";
import { useListPageSize } from "../listPageSize";
import {
  groupHosts,
  groupingOptions,
  readHostGrouping,
  serverType,
  serverTypeLabel,
  serverPalette,
  type HostGrouping,
} from "../hostGrouping";
import HostDiscovery from "./HostDiscovery";
import { HostEndpoint, TopicSymbol } from "../components/InfrastructureDetails";
import type { HostVisit } from "../components/AnkerCompass";
import ConfigurationJourney from "../components/ConfigurationJourney";
import SettingHistory from "../components/SettingHistory";
import HostChangeDetail from "../components/HostChangeDetail";
import { displayHostName, HostName } from "../hostNames";
import ChangeTimeline from "../components/ChangeTimeline";
import ChangeSummary from "../components/ChangeSummary";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  Search,
  ChevronRight,
  ShieldCheck,
  Network,
  GitCompareArrows,
  Plus,
  FolderOpen,
  RotateCcw,
  CircleAlert,
} from "lucide-react";
import { recoverySource } from "../hostHealth";
import { api, date, hostState } from "../api";
import type { Host, Status } from "../api";
import {
  Dialog,
  Empty,
  Field,
  Heading,
  Pagination,
  State,
} from "../components/shared";
import type { Notify } from "../components/shared";
import { BackupHistory } from "../components/BackupHistory";
import BackupReadiness from "../components/BackupReadiness";
import BackupIssues from "../components/BackupIssues";
import Inventory from "../components/Inventory";
import ActiveJobs from "../components/ActiveJobs";
import { HostForm } from "../components/HostForm";
export type HostStateFilter = "" | "active" | "saved" | "attention" | "paused";

function BackupAge({ createdAt }: { createdAt?: string }) {
  if (!createdAt) return <>—</>;
  if (!Number.isFinite(Date.parse(createdAt)))
    return <span>Datum ungeklärt</span>;
  if (Date.parse(createdAt) > Date.now() + 5 * 60 * 1000)
    return (
      <time dateTime={createdAt} title={date(createdAt)}>
        In der Zukunft
      </time>
    );
  const seconds = Math.max(
    0,
    (Date.now() - new Date(createdAt).getTime()) / 1000,
  );
  const formatter = new Intl.RelativeTimeFormat("de", { numeric: "always" });
  const relative =
    seconds < 60
      ? "gerade eben"
      : seconds < 3600
        ? formatter.format(-Math.floor(seconds / 60), "minute")
        : seconds < 86400
          ? formatter.format(-Math.floor(seconds / 3600), "hour")
          : formatter.format(-Math.floor(seconds / 86400), "day");
  const exact = date(createdAt);
  return (
    <time
      dateTime={createdAt}
      title={exact}
      aria-label={`${relative} · ${exact}`}
    >
      {relative}
    </time>
  );
}

export default function Hosts({
  actorID = "",
  status,
  notify,
  refresh,
  canEdit,
  canDownload = false,
  canOperate = canEdit,
  openBackup,
  openJob,
  onRestore,
  onHostContextChange,
  visit,
  onVisitConsumed,
  initialHost = "",
  onInitialHostConsumed,
  initialStateFilter = "",
  onInitialStateFilterConsumed,
}: {
  actorID?: string;
  status: Status;
  notify: Notify;
  refresh: () => void;
  canEdit: boolean;
  canDownload?: boolean;
  canOperate?: boolean;
  openBackup: (id: string) => void;
  openJob?: (id: string) => void;
  onRestore: (id: string) => void;
  onHostContextChange?: (id: string) => void;
  visit?: HostVisit | null;
  onVisitConsumed?: () => void;
  initialHost?: string;
  onInitialHostConsumed?: () => void;
  initialStateFilter?: HostStateFilter;
  onInitialStateFilterConsumed?: () => void;
}) {
  const [bulkSelection, setBulkSelection] = useState<Record<string, string>>(
    {},
  );
  const [discoveryOpen, setDiscoveryOpen] = useState(false);
  const [initialEndpoint, setInitialEndpoint] = useState<{
    address: string;
    port: number;
  }>();
  const [query, setQuery] = useState("");
  const [group, setGroup] = useState("");
  const [grouping, setGrouping] = useState<HostGrouping>(readHostGrouping);
  function changeGrouping(value: HostGrouping) {
    setGrouping(value);
    setPage(1);
    try {
      localStorage.setItem("anker.hostGrouping", value);
    } catch {
      /* Storage may be unavailable. */
    }
  }
  const [stateFilter, setStateFilter] =
    useState<HostStateFilter>(initialStateFilter);
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState(initialHost);
  useEffect(() => {
    onHostContextChange?.(selected);
  }, [selected, onHostContextChange]);
  useEffect(() => () => onHostContextChange?.(""), [onHostContextChange]);
  useEffect(() => {
    if (initialHost) {
      setSelected(initialHost);
      setTab("Übersicht");
      onInitialHostConsumed?.();
    }
  }, [initialHost, onInitialHostConsumed]);
  const [tab, setTab] = useState("Übersicht");
  const [journeyTrace, setJourneyTrace] = useState<{
    hostID: string;
    id: string;
    sequence: number;
  }>();
  useEffect(() => {
    if (!initialStateFilter) return;
    setStateFilter(initialStateFilter);
    setQuery("");
    setGroup("");
    setPage(1);
    setSelected("");
    setTab("Übersicht");
    onInitialStateFilterConsumed?.();
  }, [initialStateFilter, onInitialStateFilterConsumed]);
  const [form, setForm] = useState<Host | "new" | null>(null);
  const [reconnecting, setReconnecting] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removeName, setRemoveName] = useState("");
  const host = status.hosts.find((h) => h.id === selected);
  const selectedState = host ? hostState(host, status) : undefined;
  const hostBackups = useMemo(
    () =>
      status.backups
        .filter((b) => b.host_id === selected)
        .sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [status.backups, selected],
  );
  const latest = hostBackups[0];
  const restoreSource = recoverySource(hostBackups);
  const latestSuccessful = restoreSource;
  const previous =
    latest &&
    hostBackups.find(
      (b) => Date.parse(b.created_at) < Date.parse(latest.created_at),
    );
  const [detailOpen, setDetailOpen] = useState(false);
  useEffect(() => {
    if (!visit) return;
    setSelected(visit.hostID);
    setTab(visit.tab);
    setDetailOpen(false);
    setJourneyTrace(undefined);
    setQuery("");
    setGroup("");
    setStateFilter("");
    onVisitConsumed?.();
  }, [visit, onVisitConsumed]);
  const returnFocus = useRef<HTMLElement | null>(null);
  const returnScroll = useRef(0);
  function openChangeDetail(from: string, to: string) {
    if (!host) return;
    returnFocus.current = document.activeElement as HTMLElement;
    returnScroll.current = window.scrollY;
    setSummaryPair({ hostID: host.id, from, to });
    setDetailOpen(true);
  }
  function closeChangeDetail() {
    setDetailOpen(false);
    requestAnimationFrame(() => {
      returnFocus.current?.focus({ preventScroll: true });
      window.scrollTo(0, returnScroll.current);
    });
  }
  const [summaryPair, setSummaryPair] = useState<{
    hostID: string;
    from: string;
    to: string;
  } | null>(null);
  useEffect(() => {
    setSummaryPair(null);
    setDetailOpen(false);
  }, [selected]);
  const groups = Array.from(
    new Set(status.hosts.map((h) => h.group).filter(Boolean)),
  ).sort();
  const hosts = status.hosts.filter(
    (h) =>
      (h.name + " " + h.address).toLowerCase().includes(query.toLowerCase()) &&
      (!group || h.group === group) &&
      (!stateFilter ||
        (stateFilter === "paused"
          ? !h.enabled
          : stateFilter === "active"
            ? h.enabled
            : stateFilter === "attention"
              ? h.enabled && hostState(h, status).tone !== "success"
              : h.enabled && hostState(h, status).tone === "success")),
  );
  const inventoryBuckets = groupHosts(status.hosts, grouping);
  const filteredIDs = new Set(hosts.map((host) => host.id));
  const orderedHosts = inventoryBuckets.flatMap((bucket) =>
    bucket.hosts.filter((host) => filteredIDs.has(host.id)),
  );
  const [hostPageSize, setHostPageSize] = useListPageSize("hosts", 8);
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(hosts.length / hostPageSize)),
  );
  const visibleIDs = new Set(
    orderedHosts
      .slice((currentPage - 1) * hostPageSize, currentPage * hostPageSize)
      .map((host) => host.id),
  );
  const visibleBuckets = inventoryBuckets
    .map((bucket) => ({
      ...bucket,
      palette: serverPalette(bucket.hosts),
      matching: bucket.hosts.filter((host) => filteredIDs.has(host.id)),
      hosts: bucket.hosts.filter((host) => visibleIDs.has(host.id)),
    }))
    .filter((bucket) => bucket.hosts.length > 0);
  const saved = status.hosts.filter(
    (h) => hostState(h, status).tone === "success",
  ).length;
  const paused = status.hosts.filter((h) => !h.enabled).length;
  const attention = status.hosts.length - paused - saved;
  const actionInFlight = useRef(false);
  const [startingAction, setStartingAction] = useState(false);
  const activeHostJob = status.jobs.find(
    (j) => j.host_id === selected && ["queued", "running"].includes(j.state),
  );
  const diagnosticFocus =
    !activeHostJob &&
    latest?.status === "partial" &&
    !!(latest.issues?.length || latest.warnings?.length);
  // The focus card only appears when something needs doing; a healthy host
  // is fully described by the status strip.
  const showFocus =
    !diagnosticFocus &&
    !!host &&
    (!!activeHostJob ||
      !!host.probe_error ||
      (!!selectedState && selectedState.tone !== "success"));
  async function action(path: string, message: string) {
    if (actionInFlight.current) return;
    actionInFlight.current = true;
    setStartingAction(true);
    try {
      await api(path, "POST", {});
      notify(message);
      await refresh();
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      actionInFlight.current = false;
      setStartingAction(false);
    }
  }
  return (
    <>
      {detailOpen && host && summaryPair?.hostID === host.id && (
        <HostChangeDetail
          host={host}
          backups={hostBackups}
          from={summaryPair.from}
          to={summaryPair.to}
          canOperate={canOperate}
          canDownload={canDownload}
          notify={notify}
          onRestore={onRestore}
          onBack={closeChangeDetail}
        />
      )}
      <div hidden={detailOpen && !!host && summaryPair?.hostID === host.id}>
        {host ? (
          <>
            <button className="back-button" onClick={() => setSelected("")}>
              <ArrowLeft size={14} />
              Alle Hosts
            </button>
            <Heading
              title={displayHostName(host.name)}
              accessibleTitle={host.name}
              description={
                host.name +
                " · " +
                host.address +
                (host.group ? " · " + host.group : "")
              }
              action={
                canOperate && (
                  <>
                    <button
                      disabled={
                        startingAction ||
                        status.jobs.some(
                          (j) =>
                            j.host_id === host.id &&
                            ["queued", "running"].includes(j.state),
                        )
                      }
                      className="secondary"
                      onClick={() =>
                        action(
                          "hosts/" + host.id + "/probe",
                          "Hostprüfung gestartet",
                        )
                      }
                    >
                      <Network size={15} aria-hidden="true" /> Verbindung prüfen
                    </button>
                    <button
                      disabled={
                        startingAction ||
                        status.jobs.some(
                          (j) =>
                            j.host_id === host.id &&
                            ["queued", "running"].includes(j.state),
                        )
                      }
                      onClick={() =>
                        action(
                          "hosts/" + host.id + "/backup",
                          "Sicherung gestartet",
                        )
                      }
                    >
                      <ShieldCheck size={15} aria-hidden="true" /> Jetzt sichern
                    </button>
                  </>
                )
              }
            />
            <HostEndpoint host={host} />
            <div className="tabs host-tabs">
              {[
                "Übersicht",
                "Sicherungen",
                "Änderungen",
                "Inventar",
                "Einstellungen",
              ].map((v) => (
                <button
                  key={v}
                  className={tab === v ? "selected" : ""}
                  aria-pressed={tab === v}
                  onClick={() => setTab(v)}
                >
                  <TopicSymbol
                    topic={
                      v === "Übersicht"
                        ? "server"
                        : v === "Sicherungen"
                          ? "archive"
                          : v === "Änderungen"
                            ? "activity"
                            : v === "Inventar"
                              ? "system"
                              : "settings"
                    }
                  />
                  {v}
                </button>
              ))}
            </div>
            {tab !== "Übersicht" && (
              <ActiveJobs
                status={{
                  hosts: [host],
                  jobs: status.jobs.filter((j) => j.host_id === host.id),
                }}
                openJob={openJob}
              />
            )}
            {tab === "Übersicht" && (
              <div
                className={`detail-section host-overview-detail${diagnosticFocus ? " has-backup-diagnosis" : ""}${showFocus ? "" : " without-focus"}`}
              >
                {showFocus && (
                  <section
                    className={
                      "host-next-step host-focus-card " +
                      (activeHostJob
                        ? "is-running"
                        : selectedState?.tone === "success" && host.probe_error
                          ? "warning"
                          : selectedState?.tone || "")
                    }
                    aria-label="Für diesen Host"
                  >
                    <span
                      className={
                        "object-icon " +
                        (activeHostJob
                          ? "blue"
                          : selectedState?.tone === "success" &&
                              !host.probe_error
                            ? "sage"
                            : "ochre")
                      }
                    >
                      {selectedState?.tone === "success" &&
                      !host.probe_error &&
                      !activeHostJob ? (
                        <ShieldCheck
                          size={19}
                          strokeWidth={1.6}
                          aria-hidden="true"
                        />
                      ) : (
                        <CircleAlert
                          size={19}
                          strokeWidth={1.6}
                          aria-hidden="true"
                        />
                      )}
                    </span>
                    <div>
                      <span className="eyebrow">
                        {activeHostJob ? "Gerade aktiv" : "Auf einen Blick"}
                      </span>
                      <h2>
                        {activeHostJob
                          ? activeHostJob.retry_at
                            ? "Der Auftrag wartet auf einen neuen Versuch"
                            : activeHostJob.state === "queued"
                              ? "Ein Auftrag wartet"
                              : ["backup", "backup_verify"].includes(
                                    activeHostJob.kind,
                                  )
                                ? "Die Sicherung läuft"
                                : activeHostJob.kind === "probe"
                                  ? "Die Verbindung wird geprüft"
                                  : "Ein Auftrag läuft"
                          : selectedState?.tone === "success" &&
                              host.probe_error
                            ? "Die letzte Verbindungsprüfung ist fehlgeschlagen"
                            : selectedState?.tone === "success"
                              ? "Die letzte Sicherung ist aktuell"
                              : selectedState?.text}
                      </h2>
                      <p>
                        {activeHostJob
                          ? "Für diesen Host ist bereits ein Auftrag offen. Den Fortschritt und mögliche Fehler findest du im Auftrag."
                          : selectedState?.tone === "success" &&
                              host.probe_error
                            ? "Die Sicherung ist aktuell. Prüfe die Verbindung, bevor du die nächste Sicherung startest."
                            : selectedState?.reason}
                      </p>
                      {host.probe_error && (
                        <details className="host-connection-error">
                          <summary>Verbindungsfehler ansehen</summary>
                          <p>{host.probe_error}</p>
                        </details>
                      )}
                    </div>
                    {selectedState && (
                      <button
                        className="host-focus-action"
                        disabled={
                          startingAction || (!!activeHostJob && !openJob)
                        }
                        onClick={() => {
                          if (activeHostJob && openJob) {
                            openJob(activeHostJob.id);
                            return;
                          }
                          if (
                            selectedState.tone === "success" &&
                            host.probe_error &&
                            canOperate
                          ) {
                            void action(
                              "hosts/" + host.id + "/probe",
                              "Hostprüfung gestartet",
                            );
                            return;
                          }
                          if (selectedState.action === "files") {
                            setTab("Sicherungen");
                            return;
                          }
                          if (
                            selectedState.action === "job" &&
                            selectedState.jobID &&
                            openJob
                          ) {
                            openJob(selectedState.jobID);
                            return;
                          }
                          if (selectedState.action === "settings") {
                            setTab("Einstellungen");
                            return;
                          }
                          if (!canOperate) {
                            setTab("Sicherungen");
                            return;
                          }
                          if (
                            selectedState.action === "probe" ||
                            selectedState.action === "backup"
                          )
                            void action(
                              "hosts/" + host.id + "/" + selectedState.action,
                              selectedState.action === "probe"
                                ? "Hostprüfung gestartet"
                                : "Sicherung gestartet",
                            );
                          else setTab("Sicherungen");
                        }}
                      >
                        {activeHostJob
                          ? "Laufenden Auftrag ansehen"
                          : selectedState.tone === "success" &&
                              host.probe_error &&
                              canOperate
                            ? "Verbindung erneut prüfen"
                            : ["probe", "backup"].includes(
                                  selectedState.action,
                                ) && !canOperate
                              ? "Verlauf ansehen"
                              : selectedState.actionLabel}
                        <ChevronRight size={14} aria-hidden="true" />
                      </button>
                    )}
                  </section>
                )}
                <BackupIssues
                  backup={latest}
                  onSettings={
                    canEdit ? () => setTab("Einstellungen") : undefined
                  }
                  onRetry={
                    canOperate && !activeHostJob && !startingAction
                      ? () => {
                          void action(
                            "hosts/" + host.id + "/backup",
                            "Neue Sicherung gestartet",
                          );
                        }
                      : undefined
                  }
                />
                <div className="host-status-strip">
                  <div>
                    <span className="eyebrow">Sicherungsstatus</span>
                    <strong className={selectedState?.tone}>
                      {selectedState?.text}
                    </strong>
                    <small>
                      {latestSuccessful ? (
                        <>
                          Letzter erfolgreicher Stand{" "}
                          <BackupAge createdAt={latestSuccessful.created_at} />
                        </>
                      ) : (
                        "Noch kein erfolgreicher Stand vorhanden"
                      )}
                    </small>
                  </div>
                  <div>
                    <span className="eyebrow">Automatische Sicherung</span>
                    <strong>
                      {status.demo
                        ? "In der Demo aus"
                        : host.maintenance_until &&
                            Date.parse(host.maintenance_until) > Date.now()
                          ? "Wartung bis " + date(host.maintenance_until)
                          : host.enabled
                            ? host.schedule || "Gemeinsamer Zeitplan"
                            : "Pausiert"}
                    </strong>
                    <small>
                      {status.demo
                        ? "Sicherungen startest du hier von Hand"
                        : host.enabled
                          ? "Täglich · " + status.timezone
                          : "Vorhandene Stände bleiben erhalten"}
                    </small>
                  </div>
                  <div>
                    <span className="eyebrow">Verbindung</span>
                    <strong className={host.probe_error ? "warning" : ""}>
                      {host.probe_error
                        ? "Prüfung fehlgeschlagen"
                        : host.last_probe
                          ? "Zuletzt erreichbar"
                          : "Noch nicht geprüft"}
                    </strong>
                    <small>
                      {host.last_probe
                        ? date(host.last_probe)
                        : "Mit einer Hostprüfung beginnen"}
                    </small>
                  </div>
                </div>
                <div className="host-workspaces">
                  <section className="host-workspace" data-topic="capture">
                    <span className="eyebrow">
                      <FolderOpen size={14} aria-hidden="true" /> Letzter Stand
                    </span>
                    <h3>
                      {latest
                        ? date(latest.created_at)
                        : "Noch keine Sicherung"}
                    </h3>
                    <p>
                      {latest
                        ? latest.files +
                          " Einträge · " +
                          (latest.verified_at && !latest.verification_error
                            ? "Prüfsummen geprüft"
                            : "Prüfergebnis ansehen")
                        : "Nach der ersten Sicherung findest du hier deine Konfigurationsdateien."}
                    </p>
                    {latest && (
                      <div className="host-workspace-actions">
                        <button
                          className="secondary"
                          onClick={() => openBackup(latest.id)}
                        >
                          <FolderOpen size={15} aria-hidden="true" /> Dateien
                          öffnen
                        </button>
                        <button
                          className="text-button"
                          disabled={!previous}
                          onClick={() =>
                            previous &&
                            latest &&
                            openChangeDetail(previous.id, latest.id)
                          }
                        >
                          <GitCompareArrows size={15} aria-hidden="true" />{" "}
                          Änderungen ansehen
                        </button>
                      </div>
                    )}
                  </section>
                  <ChangeSummary
                    key={host.id}
                    compact
                    onHistory={() => {
                      setTab("Änderungen");
                      window.scrollTo(0, 0);
                    }}
                    backups={hostBackups}
                    openChanges={(from, to) => {
                      openChangeDetail(from, to);
                    }}
                  />
                  <section className="host-workspace" data-topic="recovery">
                    <span className="eyebrow">
                      <RotateCcw size={14} aria-hidden="true" />{" "}
                      Wiederherstellung
                    </span>
                    <h3>Älteren Stand wiederherstellen</h3>
                    <p>
                      {host.inventory?.host_type === "pbs" ? (
                        "Stellt die PBS-Konfiguration wieder her. Vorher die PBS-Dienste stoppen; Datastore-Inhalte bleiben unberührt."
                      ) : restoreSource ? (
                        <>
                          Vorgeschlagene Quelle:{" "}
                          {date(restoreSource.created_at)}. Ziel und Änderungen
                          werden vor der Übernahme geprüft.
                        </>
                      ) : (
                        "Kein vollständiger Stand ohne bekannten Integritätsfehler verfügbar. Prüfe zuerst den Sicherungsverlauf."
                      )}
                      {restoreSource &&
                        latest &&
                        restoreSource.id !== latest.id && (
                          <span className="restore-source-note">
                            Jüngere Stände haben offene Fehler oder unplausible
                            Zeitstempel und werden hier nicht vorgeschlagen.
                          </span>
                        )}
                    </p>
                    {canOperate && (
                      <button
                        className="secondary"
                        disabled={!restoreSource}
                        onClick={() =>
                          restoreSource && onRestore(restoreSource.id)
                        }
                      >
                        Wiederherstellung planen{" "}
                        <ChevronRight size={14} aria-hidden="true" />
                      </button>
                    )}
                  </section>
                </div>
                <HostDrift
                  key={`${host.id}-${latest?.id}-${latest?.status}-${latest?.verified_at}`}
                  host={host}
                  backups={hostBackups}
                  openBackup={openBackup}
                  canOperate={canOperate}
                  refresh={refresh}
                  openJob={openJob}
                />
                <details className="host-extra-details" key={host.id}>
                  <summary>Details zum Host und zur letzten Sicherung</summary>
                  <BackupReadiness backup={latest} />
                  <dl className="host-facts">
                    <div>
                      <dt>Proxmox</dt>
                      <dd>
                        {host.inventory?.host_type === "pbs"
                          ? `PBS ${host.inventory.pbs_version || "Unbekannt"}`
                          : host.inventory?.pve_version || "Noch nicht geprüft"}
                      </dd>
                    </div>
                    <div>
                      <dt>Cluster</dt>
                      <dd>
                        {host.inventory?.cluster_id ||
                          host.cluster_id ||
                          "Standalone"}
                      </dd>
                    </div>
                    <div>
                      <dt>Inventar</dt>
                      <dd>
                        {host.inventory ? "Vorhanden" : "Noch nicht erfasst"}
                      </dd>
                    </div>
                  </dl>
                </details>
              </div>
            )}
            {tab === "Sicherungen" && (
              <BackupHistory
                filterKey={host.id}
                onComparePrevious={(id, previousID) =>
                  openChangeDetail(previousID, id)
                }
                backups={hostBackups}
                references={status.references}
                referenceHosts={[host]}
                notify={notify}
                refresh={refresh}
                openBackup={openBackup}
                canEdit={canOperate}
                canDownload={canDownload}
                onRestore={onRestore}
              />
            )}{" "}
            {tab === "Änderungen" && (
              <ConfigurationJourney
                key={"journey-" + host.id}
                hostID={host.id}
                backups={hostBackups}
                onCompare={openChangeDetail}
                onOpenBackup={openBackup}
                onTrackSetting={(id) =>
                  setJourneyTrace((r) => ({
                    hostID: host.id,
                    id,
                    sequence: (r?.sequence || 0) + 1,
                  }))
                }
              />
            )}
            {tab === "Änderungen" && (
              <SettingHistory
                key={"changes-trace-" + host.id}
                request={
                  journeyTrace?.hostID === host.id ? journeyTrace : undefined
                }
                hostID={host.id}
                backups={hostBackups}
              />
            )}
            {tab === "Änderungen" &&
              (hostBackups.length > 1 ? (
                <ChangeTimeline
                  key={"changes-" + host.id}
                  backups={hostBackups}
                  onCompare={openChangeDetail}
                />
              ) : (
                <Empty title="Noch kein Vergleich möglich">
                  Für einen Vergleich werden zwei Sicherungsstände dieses Hosts
                  benötigt.
                </Empty>
              ))}
            {tab === "Inventar" &&
              (host.inventory ? (
                <Inventory
                  key={host.id}
                  inventory={host.inventory}
                  collectedAt={host.last_probe}
                  queryError={host.probe_error}
                />
              ) : (
                <Empty
                  title={
                    host.probe_error
                      ? "Inventarabfrage fehlgeschlagen"
                      : "Noch kein Inventar"
                  }
                >
                  {host.probe_error && (
                    <span className="warning" role="alert">
                      {host.probe_error}{" "}
                    </span>
                  )}
                  Mit „Verbindung prüfen“ liest Anker den aktuellen Hostzustand.
                </Empty>
              ))}
            {tab === "Einstellungen" && (
              <div className="detail-section">
                <div className="host-settings-grid">
                  <section
                    className="host-settings-card"
                    aria-label="Sicherungsregeln für diesen Host"
                    data-surface-topic="schedule"
                  >
                    <h3 className="topic-heading">
                      <TopicSymbol topic="schedule" />
                      Sicherungsregeln
                    </h3>
                    <dl className="properties">
                      <dt>Automatische Sicherung</dt>
                      <dd>
                        {host.enabled
                          ? host.schedule || "Gemeinsamer Zeitplan"
                          : "Pausiert"}
                      </dd>
                      <dt>Aktualitätsgrenze</dt>
                      <dd>
                        {Number.isFinite(status.stale_hours) &&
                        status.stale_hours > 0
                          ? status.stale_hours + " Stunden"
                          : "Nicht verfügbar"}
                        <div className="secondary-line">
                          Gemeinsame Vorgaben unter Einstellungen → Zeitplan.
                        </div>
                      </dd>
                      <dt>Pflichtpfade</dt>
                      <dd>
                        /etc
                        {host.extra_paths?.map((p) => (
                          <div key={p}>{p}</div>
                        ))}
                      </dd>
                    </dl>
                  </section>
                  <section
                    className="host-settings-card"
                    aria-label="SSH-Verbindung für diesen Host"
                    data-surface-topic="network"
                  >
                    <h3 className="topic-heading">
                      <TopicSymbol topic="network" />
                      SSH-Verbindung
                    </h3>
                    <dl className="properties">
                      <dt>SSH-Benutzer</dt>
                      <dd>{host.ssh_user}</dd>
                      <dt>SSH-Port</dt>
                      <dd>{host.ssh_port}</dd>
                    </dl>
                  </section>
                </div>
                {canEdit && (
                  <div className="inline-form">
                    <button
                      className="secondary"
                      onClick={() => {
                        setReconnecting(true);
                        setForm(host);
                      }}
                    >
                      <Network size={15} aria-hidden="true" /> Verbindung
                      einrichten
                    </button>
                    <button
                      className="secondary"
                      onClick={() => {
                        setReconnecting(false);
                        setForm(host);
                      }}
                    >
                      Host bearbeiten
                    </button>
                    <button
                      className="text-button"
                      onClick={() => {
                        setRemoving(true);
                        setRemoveName("");
                      }}
                    >
                      Host entfernen
                    </button>
                  </div>
                )}
              </div>
            )}
          </>
        ) : (
          <>
            <Heading
              title="Hosts"
              description="Konfigurationen sichern und wiederherstellen."
              action={
                canEdit && (
                  <>
                    <button
                      className="secondary"
                      onClick={() => setDiscoveryOpen(true)}
                    >
                      <Search size={15} aria-hidden="true" /> Server suchen
                    </button>
                    <button
                      onClick={() => {
                        setInitialEndpoint(undefined);
                        setReconnecting(false);
                        setForm("new");
                      }}
                    >
                      <Plus size={15} aria-hidden="true" /> Host hinzufügen
                    </button>
                  </>
                )
              }
            />

            <BackupRemediation
              status={status}
              actorID={actorID}
              canEdit={canOperate}
              refresh={refresh}
              openJob={openJob}
              openBackup={openBackup}
            />
            <div
              className="host-filter-chips"
              role="group"
              aria-label="Host-Schnellfilter"
            >
              {[
                { value: "", label: "Alle", count: status.hosts.length },
                {
                  value: "attention",
                  label: "Handlungsbedarf",
                  count: attention,
                },
                { value: "saved", label: "Gesichert", count: saved },
                { value: "paused", label: "Pausiert", count: paused },
                ...(stateFilter === "active"
                  ? [
                      {
                        value: "active",
                        label: "Aktiv",
                        count: status.hosts.filter((h) => h.enabled).length,
                      },
                    ]
                  : []),
              ].map((filter) => (
                <button
                  key={filter.value}
                  type="button"
                  className={`host-filter-chip${stateFilter === filter.value ? " selected" : ""}`}
                  aria-pressed={stateFilter === filter.value}
                  onClick={() => {
                    setStateFilter(filter.value as HostStateFilter);
                    setPage(1);
                  }}
                >
                  {filter.label} <span>{filter.count}</span>
                </button>
              ))}
            </div>
            <div className="toolbar host-toolbar">
              <div className="search-field">
                <Search size={15} aria-hidden="true" />
                <input
                  className="search"
                  aria-label="Hosts durchsuchen"
                  placeholder="Hosts durchsuchen"
                  value={query}
                  onChange={(e) => {
                    setQuery(e.target.value);
                    setPage(1);
                  }}
                />
              </div>
              <select
                aria-label="Gruppe filtern"
                value={group}
                onChange={(e) => {
                  setGroup(e.target.value);
                  setPage(1);
                }}
              >
                <option value="">Alle Gruppen</option>
                {groups.map((g) => (
                  <option key={g} value={g}>
                    {displayHostName(g)}
                  </option>
                ))}
              </select>
              <select
                aria-label="Ansicht gruppieren"
                title="Manuelle Hostgruppen haben Vorrang. Automatische Standorte sind Vorschläge aus Namen oder IPv4-Netzen. Die Ansicht wird in diesem Browser gespeichert."
                value={grouping}
                onChange={(event) =>
                  changeGrouping(event.target.value as HostGrouping)
                }
              >
                {groupingOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
            <HostOperations
              hosts={status.hosts}
              filtered={hosts}
              selection={bulkSelection}
              onSelection={setBulkSelection}
              canManage={canEdit}
              canOperate={canOperate}
              refresh={refresh}
              openJob={openJob}
              actorID={actorID}
              jobs={status.jobs}
            />
            {hosts.length ? (
              <div className="host-site-grid">
                {visibleBuckets.map((bucket) => (
                  <section
                    className="host-site"
                    key={bucket.key}
                    aria-label={`Hostbereich ${bucket.label}`}
                    data-inferred={bucket.inferred}
                    data-server-kind={bucket.palette}
                  >
                    {grouping !== "none" && (
                      <header className="host-site-header">
                        <span className="host-site-symbol">
                          <Network size={22} aria-hidden="true" />
                        </span>
                        <div>
                          <div className="host-site-eyebrow">
                            {grouping === "type"
                              ? "Servertyp"
                              : grouping === "network"
                                ? "IP-Netz"
                                : bucket.inferred
                                  ? "Standortvorschlag"
                                  : "Hostbereich"}
                          </div>
                          <h2>{bucket.label}</h2>
                          <p>{bucket.reason}</p>
                        </div>
                        <span className="host-site-count">
                          {bucket.hosts.length === bucket.matching.length
                            ? `${bucket.hosts.length} ${bucket.hosts.length === 1 ? "Host" : "Hosts"}`
                            : `${bucket.hosts.length} von ${bucket.matching.length} Hosts`}
                        </span>
                      </header>
                    )}
                    <div className="table-scroll data-surface">
                      <table className="host-table">
                        <thead>
                          <tr>
                            <th>Host</th>
                            <th>Gruppe</th>
                            <th>Letzte Sicherung</th>
                            <th>Status</th>
                            <th className="right">Aktionen</th>
                          </tr>
                        </thead>
                        <tbody>
                          {bucket.hosts.map((h) => {
                            const state = hostState(h, status);
                            return (
                              <tr key={h.id}>
                                <td>
                                  <div className="host-select-name">
                                    {canOperate && (
                                      <input
                                        type="checkbox"
                                        aria-label={`Host ${h.name} auswählen`}
                                        checked={!!bulkSelection[h.id]}
                                        disabled={!h.config_revision}
                                        onChange={(e) =>
                                          setBulkSelection((old) => {
                                            const next = { ...old };
                                            if (e.target.checked)
                                              next[h.id] = h.config_revision!;
                                            else delete next[h.id];
                                            return next;
                                          })
                                        }
                                      />
                                    )}
                                    <button
                                      className="name-link"
                                      aria-label={h.name}
                                      onClick={() => {
                                        setSelected(h.id);
                                        setTab("Übersicht");
                                      }}
                                    >
                                      <HostName name={h.name} />
                                    </button>
                                  </div>
                                  <div className="host-node-meta">
                                    <span
                                      className={`server-role ${serverType(h)}`}
                                    >
                                      {serverTypeLabel(h)}
                                    </span>
                                    <span className="mono">{h.address}</span>
                                  </div>
                                  <div className="secondary-line host-backup-mobile">
                                    Letzter Stand:{" "}
                                    <BackupAge
                                      createdAt={state.backup?.created_at}
                                    />
                                  </div>
                                </td>
                                <td>
                                  <span
                                    className={
                                      h.cluster_id
                                        ? "group-tag cluster"
                                        : "group-tag"
                                    }
                                  >
                                    {h.group
                                      ? displayHostName(h.group)
                                      : "Nicht festgelegt"}
                                  </span>
                                </td>
                                <td className="date">
                                  <BackupAge
                                    createdAt={state.backup?.created_at}
                                  />
                                </td>
                                <td>
                                  <span className={"state " + state.tone}>
                                    {state.text}
                                  </span>
                                  {status.jobs.some(
                                    (j) =>
                                      j.host_id === h.id &&
                                      ["backup", "backup_verify"].includes(
                                        j.kind,
                                      ) &&
                                      ["queued", "running"].includes(j.state),
                                  ) && (
                                    <div className="secondary-line">
                                      <State
                                        value={
                                          status.jobs.some(
                                            (j) =>
                                              j.host_id === h.id &&
                                              [
                                                "backup",
                                                "backup_verify",
                                              ].includes(j.kind) &&
                                              j.state === "running",
                                          )
                                            ? "running"
                                            : "queued"
                                        }
                                        label={
                                          status.jobs.some(
                                            (j) =>
                                              j.host_id === h.id &&
                                              [
                                                "backup",
                                                "backup_verify",
                                              ].includes(j.kind) &&
                                              j.state === "running",
                                          )
                                            ? "Sicherung läuft"
                                            : "Sicherung wartet"
                                        }
                                      />
                                    </div>
                                  )}
                                </td>
                                <td className="right">
                                  <button
                                    className="text-button"
                                    onClick={() => {
                                      setSelected(h.id);
                                      setTab("Übersicht");
                                    }}
                                  >
                                    Öffnen{" "}
                                    <ChevronRight
                                      size={14}
                                      aria-hidden="true"
                                    />
                                  </button>
                                </td>
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                  </section>
                ))}
              </div>
            ) : (
              <Empty
                title={
                  query || group || stateFilter
                    ? "Keine passenden Hosts"
                    : "Noch keine Hosts"
                }
              >
                {query || group || stateFilter
                  ? "Suche oder Filter ändern."
                  : "Mit „Host hinzufügen“ richtest du die erste Verbindung ein."}
              </Empty>
            )}
            {hosts.length > 0 && (
              <Pagination
                page={currentPage}
                pageSize={hostPageSize}
                initialPageSize={8}
                onPageSizeChange={setHostPageSize}
                total={hosts.length}
                onPageChange={setPage}
                label="Hosts"
              />
            )}
          </>
        )}
        {removing && host && (
          <Dialog
            topic="server"
            title="Host entfernen"
            onClose={() => setRemoving(false)}
          >
            <p className="dialog-intro">
              Zeitplan und Zugangseintrag für {host.name} entfernen. Vorhandene
              Sicherungen bleiben erhalten.
            </p>
            <Field label="Hostnamen bestätigen">
              <input
                value={removeName}
                onChange={(e) => setRemoveName(e.target.value)}
                placeholder={host.name}
              />
            </Field>
            <footer className="dialog-footer">
              <button className="secondary" onClick={() => setRemoving(false)}>
                Abbrechen
              </button>
              <button
                disabled={removeName !== host.name}
                onClick={async () => {
                  try {
                    await api("hosts/" + host.id, "DELETE");
                    setRemoving(false);
                    setSelected("");
                    notify("Host entfernt");
                    refresh();
                  } catch (e) {
                    notify((e as Error).message, true);
                  }
                }}
              >
                Host entfernen
              </button>
            </footer>
          </Dialog>
        )}
        {discoveryOpen && (
          <HostDiscovery
            grouping={grouping}
            onGroupingChange={changeGrouping}
            hosts={status.hosts}
            onClose={() => setDiscoveryOpen(false)}
            onSaved={refresh}
            notify={notify}
            onConfigure={(address, port) => {
              setDiscoveryOpen(false);
              setInitialEndpoint({ address, port });
              setReconnecting(false);
              setForm("new");
            }}
          />
        )}
        {form && (
          <HostForm
            initialEndpoint={form === "new" ? initialEndpoint : undefined}
            host={form === "new" ? undefined : form}
            automatic={form === "new" || reconnecting}
            onClose={() => setForm(null)}
            onSaved={refresh}
            notify={notify}
          />
        )}
      </div>
    </>
  );
}
