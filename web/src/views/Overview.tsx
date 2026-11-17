import IncidentCenter from "../components/IncidentCenter";
import { ServerRack as Server } from "../components/InfrastructureDetails";
import { Activity, ShieldCheck, CircleAlert, Clock3 } from "lucide-react";
import { hostState } from "../api";
import type { Status, User } from "../api";
import { Heading } from "../components/shared";
import ActiveJobs from "../components/ActiveJobs";
import Jobs from "./Jobs";
import BackupReadiness from "../components/BackupReadiness";
import type { HostStateFilter } from "./Hosts";
import type { Notify } from "../components/shared";

export default function Overview({
  status,
  user,
  notify,
  refresh,
  navigate,
  openJob,
  openPlan,
  openBackup,
  onHostFilter,
  onJobFilter,
  onSelectHost,
}: {
  status: Status;
  user: User;
  notify: Notify;
  refresh: () => void;
  navigate: (page: string) => void;
  openJob: (id: string) => void;
  openPlan: (id: string) => void;
  openBackup: (id: string) => void;
  onHostFilter: (filter: HostStateFilter) => void;
  onJobFilter: (filter: string) => void;
  onSelectHost: (id: string) => void;
}) {
  const overdue = status.hosts.filter(
    (h) => h.enabled && hostState(h, status).tone !== "success",
  );
  const enabledHosts = status.hosts.filter((h) => h.enabled);
  const pausedHosts = status.hosts.length - enabledHosts.length;
  const active = status.jobs.filter((j) =>
    ["queued", "running"].includes(j.state),
  );
  return (
    <>
      <Heading
        title="Übersicht"
        description="Was gesichert ist und wo Handlungsbedarf besteht."
      />
      {(!status.hosts.length || !enabledHosts.length) && (
        <div
          className={
            "overview-brief" + (overdue.length ? " needs-attention" : "")
          }
        >
          <span
            className={"object-icon " + (overdue.length ? "ochre" : "sage")}
          >
            {overdue.length ? (
              <CircleAlert size={21} strokeWidth={1.6} aria-hidden="true" />
            ) : (
              <ShieldCheck size={21} strokeWidth={1.6} aria-hidden="true" />
            )}
          </span>
          <div>
            <span className="eyebrow">Sicherungsüberblick</span>
            <h2>
              {!status.hosts.length
                ? "Noch kein Host verbunden"
                : !enabledHosts.length
                  ? "Die automatische Sicherung pausiert"
                  : overdue.length
                    ? `${overdue.length} ${overdue.length === 1 ? "Host braucht" : "Hosts brauchen"} Aufmerksamkeit`
                    : "Alle aktiven Hosts sind aktuell gesichert"}
            </h2>
            <p>
              {!status.hosts.length
                ? "Verbinde einen Proxmox-Host, um seine Konfiguration zu sichern."
                : overdue.length
                  ? "Offene Punkte findest du direkt unter dem Sicherungsstatus."
                  : "Sicherungen und Prüfergebnisse findest du beim jeweiligen Host."}
            </p>
          </div>
        </div>
      )}
      <section className="overview-summary" aria-label="Sicherungsstatus">
        <dl className="overview-metrics">
          {[
            {
              label: "Aktive Hosts",
              count: enabledHosts.length,
              tone: "neutral",
              filter: "active" as const,
            },
            {
              label: "Aktuell gesichert",
              count: enabledHosts.length - overdue.length,
              tone: "success",
              filter: "saved" as const,
            },
            {
              label: "Handlungsbedarf",
              count: overdue.length,
              tone: overdue.length ? "warning" : "neutral",
              filter: "attention" as const,
            },
            {
              label: "Aktive Aufträge",
              count: active.length,
              tone: active.length ? "active" : "neutral",
              filter: "jobs" as const,
            },
          ].map((metric) => (
            <div
              className={"overview-metric " + metric.tone}
              key={metric.label}
              data-surface-topic={
                metric.filter === "jobs"
                  ? "activity"
                  : metric.filter === "saved"
                    ? "archive"
                    : metric.filter === "attention"
                      ? "recovery"
                      : "server"
              }
            >
              <dt>
                {(() => {
                  const Icon =
                    metric.filter === "jobs"
                      ? Activity
                      : metric.filter === "attention"
                        ? CircleAlert
                        : metric.filter === "saved"
                          ? ShieldCheck
                          : Server;
                  return (
                    <Icon size={14} strokeWidth={1.7} aria-hidden="true" />
                  );
                })()}
                {metric.label}
              </dt>
              <dd>
                <button
                  className="overview-metric-link"
                  aria-label={`${metric.label}: ${metric.count} anzeigen`}
                  onClick={() => {
                    if (metric.filter === "jobs") {
                      onJobFilter("active");
                      navigate("Aufträge");
                    } else {
                      onHostFilter(metric.filter);
                      navigate("Hosts");
                    }
                  }}
                >
                  {metric.count}
                </button>
              </dd>
            </div>
          ))}
        </dl>
        <p className="overview-context">
          {status.demo
            ? "Automatischer Zeitplan in der Demo deaktiviert."
            : !status.hosts.length
              ? "Noch keine Hosts eingerichtet."
              : !enabledHosts.length
                ? "Automatische Sicherung für alle Hosts pausiert."
                : "Zeitplan aktiv."}{" "}
          Zeitzone {status.timezone}.
          {pausedHosts > 0 && enabledHosts.length > 0 && (
            <span className="context-tag">
              {pausedHosts} {pausedHosts === 1 ? "Host" : "Hosts"} pausiert
            </span>
          )}
        </p>
      </section>
      <IncidentCenter
        hosts={status.hosts}
        openJob={openJob}
        refresh={refresh}
        allHosts={() => navigate("Hosts")}
        revision={JSON.stringify([
          status.backups.map((b) => [b.id, b.status, b.verified_at]),
          status.jobs.map((j) => [j.id, j.state]),
          status.hosts.map((h) => [
            h.id,
            h.last_probe,
            h.maintenance_until,
            h.enabled,
            h.config_revision,
          ]),
        ])}
        canOperate={user.role !== "reader"}
        openHost={(id) => {
          onSelectHost(id);
          navigate("Hosts");
        }}
      />
      <ActiveJobs status={status} openJob={openJob} />
      <details className="detail-section overview-evidence">
        <summary>Prüfergebnisse der letzten Sicherung</summary>
        <p className="muted">
          Diese Nachweise gelten für einen einzelnen Sicherungsstand. Sie
          bewerten nicht automatisch alle Hosts.
        </p>
        <BackupReadiness
          backup={
            [...status.backups].sort((a, b) =>
              b.created_at.localeCompare(a.created_at),
            )[0]
          }
          compact
        />
        <button className="secondary" onClick={() => navigate("Sicherungen")}>
          Sicherungen prüfen und vergleichen
        </button>
      </details>
      <details className="detail-section overview-history">
        <summary>
          <Clock3 size={15} aria-hidden="true" /> Letzte abgeschlossene Aufträge
        </summary>
        <Jobs
          status={{
            ...status,
            jobs: status.jobs.filter(
              (j) => !["queued", "running"].includes(j.state),
            ),
          }}
          notify={notify}
          refresh={refresh}
          canEdit={user.role !== "reader"}
          canManage={user.role === "admin"}
          openBackup={openBackup}
          openPlan={openPlan}
          limit={5}
        />
      </details>
    </>
  );
}
