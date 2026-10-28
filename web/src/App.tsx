import { ServerRack as Server } from "./components/InfrastructureDetails";
import AnkerCompass, { CompassEntrypoints } from "./components/AnkerCompass";
import type {
  CompassIntent,
  CompassDestination,
  HostVisit,
} from "./components/AnkerCompass";
import { recoverySource } from "./hostHealth";
import { ChangeRecoveryContext } from "./components/recovery/ChangeRecoveryIntent";
import type { ChangeRecoveryIntent } from "./components/recovery/ChangeRecoveryIntent";
import ProblemOverview from "./components/ProblemOverview";
import { useCallback, useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import {
  Compass,
  Menu,
  ChevronRight,
  LogOut,
  X,
  LayoutDashboard,
  Archive,
  RotateCcw,
  Activity,
  Settings2,
  ShieldCheck,
  CircleAlert,
  Clock3,
} from "lucide-react";
import {
  api,
  date,
  configureTimezone,
  emptyStatus,
  hostState,
  labels,
} from "./api";
import type { Status, User } from "./api";
import {
  BrandMark,
  PageEmblem,
  Dialog,
  Empty,
  Field,
  Heading,
  State,
} from "./components/shared";
import ActiveJobs from "./components/ActiveJobs";
import Jobs from "./views/Jobs";
import Hosts, { HostForm } from "./views/Hosts";
import Backups, { FileBrowser } from "./views/Backups";
import BackupReadiness from "./components/BackupReadiness";
import Restore from "./views/Restore";
import type { RecoveryOperation } from "./components/recovery/RecoveryPlan";
import Settings from "./views/Settings";
import { useUpdateReload } from "./useUpdateReload";
const pages = [
  "Übersicht",
  "Hosts",
  "Sicherungen",
  "Wiederherstellung",
  "Aufträge",
  "Einstellungen",
] as const;
const pageIcons = {
  Übersicht: LayoutDashboard,
  Hosts: Server,
  Sicherungen: Archive,
  Wiederherstellung: RotateCcw,
  Aufträge: Activity,
  Einstellungen: Settings2,
};
function Login({ onLogin }: { onLogin: (u: User, demo: boolean) => void }) {
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function login(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const user = await api<User>("login", "POST", { name, password });
      const me = await api<{ demo: boolean }>("me");
      onLogin(user, me.demo);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="login-page">
      <div className="login-form" data-surface-topic="server">
        <div className="wordmark">
          <BrandMark />
          Anker
        </div>
        <h1>Anmelden</h1>
        <p>Konfigurationen sichern und wiederherstellen.</p>
        <form onSubmit={login}>
          <Field label="Benutzername">
            <input
              required
              autoComplete="username"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field label="Passwort">
            <input
              type="password"
              required
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          <button disabled={busy}>{busy ? "Anmeldung …" : "Anmelden"}</button>
        </form>
      </div>
    </div>
  );
}
export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [status, setStatus] = useState<Status>(emptyStatus);
  const [connectionError, setConnectionError] = useState("");
  const [updatedAt, setUpdatedAt] = useState("");
  const [page, setPage] = useState("Hosts");
  const [nav, setNav] = useState(false);
  const [compass, setCompass] = useState<{
    intent: CompassIntent | null;
  } | null>(null);
  const [compassHost, setCompassHost] = useState("");
  const [hostVisit, setHostVisit] = useState<HostVisit | null>(null);
  useEffect(() => {
    if (!user) {
      setCompass(null);
      setHostVisit(null);
      return;
    }
    function shortcut(event: KeyboardEvent) {
      if (
        (event.metaKey || event.ctrlKey) &&
        event.key.toLowerCase() === "k" &&
        !event.repeat
      ) {
        // Do not stack over an existing review, file preview or host editor.
        event.preventDefault();
        if (document.querySelector('[role="dialog"]')) return;
        setCompass({ intent: null });
      }
    }
    document.addEventListener("keydown", shortcut);
    return () => document.removeEventListener("keydown", shortcut);
  }, [user]);
  const [recoveryBusy, setRecoveryBusy] = useState(false);
  const [recoveryDirty, setRecoveryDirty] = useState(false);
  const [connectTarget, setConnectTarget] = useState(false);
  const [settingsDirty, setSettingsDirty] = useState(false);
  const [pendingPage, setPendingPage] = useState("");
  const [file, setFile] = useState("");
  const [selectedJob, setSelectedJob] = useState("");
  const [selectedHost, setSelectedHost] = useState("");
  const [hostFilter, setHostFilter] = useState<
    "" | "active" | "saved" | "attention" | "paused"
  >("");
  const [jobFilter, setJobFilter] = useState("");
  const [planRequest, setPlanRequest] = useState<{
    id: string;
    sequence: number;
  } | null>(null);
  const [restoreRequest, setRestoreRequest] = useState<{
    id: string;
    file?: string;
    change?: ChangeRecoveryIntent;
    sequence: number;
  } | null>(null);
  const [recoveryOperations, setRecoveryOperations] = useState<{
    userId: string;
    plans: Record<string, RecoveryOperation>;
  }>({ userId: "", plans: {} });
  const [toast, setToast] = useState<{
    message: string;
    error: boolean;
  } | null>(null);
  const notify = useCallback(
    (message: string, error = false) => setToast({ message, error }),
    [],
  );
  const updating = useUpdateReload(user?.role === "admin", notify);
  const refreshSequence = useRef(0);
  const acceptedSequence = useRef(0);
  const refresh = useCallback(async () => {
    const sequence = ++refreshSequence.current;
    try {
      const s = await api<Status>("status");
      if (sequence < acceptedSequence.current) return;
      acceptedSequence.current = sequence;
      configureTimezone(s.timezone);
      setConnectionError("");
      setUpdatedAt(new Date().toISOString());
      setStatus({
        ...s,
        hosts: s.hosts || [],
        backups: s.backups || [],
        plans: s.plans || [],
        jobs: s.jobs || [],
      });
    } catch (e) {
      if (sequence < acceptedSequence.current) return;
      acceptedSequence.current = sequence;
      if ((e as Error).message === "Anmeldung erforderlich") {
        setUser(null);
        setRecoveryOperations({ userId: "", plans: {} });
      } else setConnectionError((e as Error).message);
    }
  }, [notify]);
  useEffect(() => {
    const expired = () => {
      acceptedSequence.current = ++refreshSequence.current;
      setUser(null);
      setStatus(emptyStatus);
      setFile("");
      setRestoreRequest(null);
      setPlanRequest(null);
      setRecoveryOperations({ userId: "", plans: {} });
      setSelectedHost("");
      setConnectTarget(false);
      setRecoveryBusy(false);
      setPage("Hosts");
      setSettingsDirty(false);
      setPendingPage("");
    };
    window.addEventListener("anker:session-expired", expired);
    return () => window.removeEventListener("anker:session-expired", expired);
  }, []);
  useEffect(() => {
    api<{ user: User; demo: boolean }>("me")
      .then((me) => {
        setUser(me.user);
        setStatus((s) => ({ ...s, demo: me.demo }));
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);
  useEffect(() => {
    if (!user) return;
    refresh();
    const timer = setInterval(refresh, 4000);
    return () => clearInterval(timer);
  }, [user, refresh]);
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 6000);
    return () => clearTimeout(t);
  }, [toast]);
  useEffect(() => {
    document.title = loading ? "Anker" : `${user ? page : "Anmelden"} · Anker`;
  }, [loading, !!user, page]);
  function navigate(p: string) {
    if (recoveryBusy) {
      notify("Plananfrage läuft. Bitte Antwort abwarten.", true);
      return;
    }
    if (p === page) {
      setNav(false);
      return;
    }
    if (
      (page === "Einstellungen" && settingsDirty) ||
      (page === "Wiederherstellung" && recoveryDirty)
    ) {
      setPendingPage(p);
      setNav(false);
      return;
    }
    setPage(p);
    setNav(false);
  }
  function goFromCompass(destination: CompassDestination) {
    if (recoveryBusy) {
      notify("Plananfrage läuft. Bitte Antwort abwarten.", true);
      return;
    }
    if (
      (page === "Einstellungen" && settingsDirty) ||
      (page === "Wiederherstellung" && recoveryDirty)
    ) {
      notify(
        "Dein Entwurf ist noch offen. Speichere oder verwerfe ihn in der aktuellen Ansicht, bevor du mit dem Kompass wechselst.",
        true,
      );
      setCompass(null);
      return;
    }
    if (destination.kind === "restore") {
      const source = recoverySource(
        status.backups.filter((b) => b.host_id === destination.hostID),
      );
      if (!source || user?.role === "reader") {
        notify(
          "Keine passende Quelle oder fehlende Berechtigung. Bitte Auswahl erneut prüfen.",
          true,
        );
        return;
      }
      restore(source.id);
    } else if (destination.kind === "job") {
      setSelectedJob(destination.id);
      setJobFilter("");
      setPage("Aufträge");
    } else {
      if (!status.hosts.some((h) => h.id === destination.hostID)) {
        notify("Der Host ist nicht mehr vorhanden.", true);
        return;
      }
      setSelectedHost("");
      setHostFilter("");
      setHostVisit((old) => ({
        ...destination,
        sequence: (old?.sequence || 0) + 1,
      }));
      setPage("Hosts");
    }
    setNav(false);
    setCompass(null);
    window.scrollTo(0, 0);
  }
  function restore(id: string, file?: string) {
    if (recoveryBusy) return;
    setPlanRequest(null);
    setRestoreRequest((old) => ({
      id,
      file,
      sequence: (old?.sequence || 0) + 1,
    }));
    setPage("Wiederherstellung");
  }
  if (loading) return <div className="loading">Anker wird geladen …</div>;
  if (!user)
    return (
      <Login
        onLogin={(u, demo) => {
          setUser(u);
          setStatus((s) => ({ ...s, demo }));
        }}
      />
    );
  const backup = status.backups.find((b) => b.id === file);
  const common = {
    status,
    notify,
    refresh,
    canEdit: user.role !== "reader",
    canManage: user.role === "admin",
    canDownload: user.secrets,
    openBackup: setFile,
    openPlan: (id: string) => {
      if (recoveryBusy) return;
      setPlanRequest((old) => ({ id, sequence: (old?.sequence || 0) + 1 }));
      navigate("Wiederherstellung");
    },
    openJob: (id: string) => {
      setSelectedJob(id);
      navigate("Aufträge");
    },
    onRestore: restore,
  };
  const overdue = status.hosts.filter(
    (h) => h.enabled && hostState(h, status).tone !== "success",
  );
  const enabledHosts = status.hosts.filter((h) => h.enabled);
  const pausedHosts = status.hosts.length - enabledHosts.length;
  const active = status.jobs.filter((j) =>
    ["queued", "running"].includes(j.state),
  );
  return (
    <ChangeRecoveryContext.Provider
      value={
        user.role === "reader"
          ? null
          : (intent) => {
              if (recoveryBusy || recoveryDirty) {
                notify(
                  "Die offene Wiederherstellung zuerst abschließen oder schließen.",
                  true,
                );
                return false;
              }
              const source = status.backups.find(
                (b) => b.id === intent.backupID,
              );
              const baseline = status.backups.find(
                (b) => b.id === intent.baselineID,
              );
              if (
                !source ||
                !baseline ||
                source.host_id !== baseline.host_id ||
                source.manifest_sha !== intent.manifestSHA ||
                source.status !== "successful" ||
                !status.hosts.some((h) => h.id === source.host_id)
              ) {
                notify(
                  "Quelle oder Ziel ist nicht mehr verfügbar. Änderung neu laden.",
                  true,
                );
                return false;
              }
              setPlanRequest(null);
              setRestoreRequest((old) => ({
                id: source.id,
                change: {
                  ...intent,
                  files: [...intent.files],
                  targetID: source.host_id,
                },
                sequence: (old?.sequence || 0) + 1,
              }));
              setPage("Wiederherstellung");
              return true;
            }
      }
    >
      <div className="app-shell">
        {nav && (
          <button
            className="nav-scrim"
            aria-label="Navigation schließen"
            onClick={() => setNav(false)}
          />
        )}
        <aside
          id="anker-navigation"
          className={nav ? "sidebar open" : "sidebar"}
        >
          <div className="brand">
            <BrandMark />
            <span className="brand-wordmark">
              <strong>Anker</strong>
              <small>Serververwaltung</small>
            </span>
          </div>
          <nav aria-label="Hauptnavigation">
            {pages
              .filter((p) => p !== "Einstellungen" || user.role === "admin")
              .map((p) => (
                <button
                  key={p}
                  className={page === p ? "active" : ""}
                  aria-current={page === p ? "page" : undefined}
                  onClick={() => navigate(p)}
                >
                  {(() => {
                    const Icon = pageIcons[p];
                    return (
                      <Icon size={16} strokeWidth={1.7} aria-hidden="true" />
                    );
                  })()}
                  <span>{p}</span>
                </button>
              ))}
          </nav>
          <div className="sidebar-bottom">
            {status.demo && <span className="demo-label">Lokale Demo</span>}
            <div className="user-row">
              <span>
                {user.name}
                <small>
                  {user.role === "admin"
                    ? "Administrator"
                    : user.role === "restore"
                      ? "Wiederherstellung"
                      : "Lesen"}
                </small>
              </span>
              <button
                className="icon-button"
                aria-label="Abmelden"
                disabled={recoveryBusy}
                onClick={async () => {
                  try {
                    await api("logout", "POST", {});
                    acceptedSequence.current = ++refreshSequence.current;
                    setUser(null);
                    setFile("");
                    setRestoreRequest(null);
                    setPlanRequest(null);
                    setRecoveryOperations({ userId: "", plans: {} });
                    setSelectedHost("");
                    setConnectTarget(false);
                    setRecoveryBusy(false);
                    setStatus(emptyStatus);
                    setPage("Hosts");
                    setSettingsDirty(false);
                    setPendingPage("");
                  } catch (e) {
                    notify((e as Error).message, true);
                  }
                }}
              >
                <LogOut size={15} />
              </button>
            </div>
          </div>
        </aside>
        <div className="workspace">
          <div className="topbar">
            <button
              className="icon-button mobile-menu"
              aria-label="Navigation öffnen"
              aria-expanded={nav}
              aria-controls="anker-navigation"
              onClick={() => setNav(true)}
            >
              <Menu size={19} />
            </button>
            <span>Anker</span>
            <ChevronRight size={12} />
            <span>{page}</span>
            <button
              className="compass-trigger"
              onClick={() => setCompass({ intent: null })}
              aria-label="Anker Kompass öffnen"
              aria-keyshortcuts="Control+k Meta+k"
            >
              <Compass size={16} aria-hidden="true" />
              <span>Kompass</span>
              <kbd>⌘ / Ctrl K</kbd>
            </button>
          </div>
          <main>
            <div className="content" data-workspace={page}>
              {updating && (
                <p className="update-progress" role="status">
                  <State value="running" label="Update" />
                  <span>
                    {updating.target} wird installiert. Die Seite lädt nach
                    erfolgreicher Startprüfung automatisch neu.
                  </span>
                </p>
              )}
              {status.scheduler_health?.error && user.role === "admin" && (
                <p className="notice warning" role="alert">
                  Zeitplan konnte nicht vollständig ausgeführt werden:{" "}
                  {status.scheduler_health.error}. Katalog und Dienstlog prüfen.
                </p>
              )}
              {status.maintenance_health?.error && user.role === "admin" && (
                <p className="notice warning" role="alert">
                  Wartung nicht abgeschlossen: {status.maintenance_health.error}
                  . Sicherungsablage und Katalog prüfen.
                </p>
              )}
              {status.notification_health?.error && user.role === "admin" && (
                <p className="notice warning" role="alert">
                  Benachrichtigung konnte nicht zugestellt werden:{" "}
                  {status.notification_health.error}. Einstellungen prüfen.
                </p>
              )}
              {connectionError && (
                <p className="notice warning" role="alert">
                  {connectionError} Angezeigte Daten: {date(updatedAt)}.
                </p>
              )}
              {page !== "Hosts" && page !== "Übersicht" && (
                <div className="page-kicker">
                  <PageEmblem
                    tone={
                      page === "Wiederherstellung"
                        ? "sand"
                        : page === "Einstellungen" || page === "Aufträge"
                          ? "blue"
                          : "sage"
                    }
                  >
                    {(() => {
                      const Icon =
                        pageIcons[page as keyof typeof pageIcons] || Server;
                      return (
                        <Icon size={20} strokeWidth={1.6} aria-hidden="true" />
                      );
                    })()}
                  </PageEmblem>
                  <span>
                    {page === "Einstellungen" || page === "Aufträge"
                      ? "Verwaltung"
                      : "Konfigurationssicherung"}{" "}
                    <span className="kicker-separator">/</span> {page}
                  </span>
                </div>
              )}
              {page === "Hosts" && (
                <Hosts
                  {...common}
                  initialHost={selectedHost}
                  visit={hostVisit}
                  onHostContextChange={setCompassHost}
                  onVisitConsumed={() => setHostVisit(null)}
                  initialStateFilter={hostFilter}
                  onInitialStateFilterConsumed={() => setHostFilter("")}
                  onInitialHostConsumed={() => setSelectedHost("")}
                  canEdit={user.role === "admin"}
                  canOperate={user.role !== "reader"}
                />
              )}{" "}
              {page === "Sicherungen" && <Backups {...common} user={user} />}{" "}
              <div hidden={page !== "Wiederherstellung"}>
                <Restore
                  key={restoreRequest?.sequence || "restore"}
                  status={status}
                  initialBackup={restoreRequest?.id || ""}
                  initialFile={restoreRequest?.file}
                  initialChange={restoreRequest?.change}
                  initialPlan={planRequest}
                  user={user}
                  notify={notify}
                  refresh={refresh}
                  onBusyChange={setRecoveryBusy}
                  onDirtyChange={setRecoveryDirty}
                  operations={
                    recoveryOperations.userId === user.id
                      ? recoveryOperations.plans
                      : {}
                  }
                  onOperationChange={(planId, operation) =>
                    setRecoveryOperations((old) => ({
                      userId: user.id,
                      plans: {
                        ...(old.userId === user.id ? old.plans : {}),
                        [planId]: operation,
                      },
                    }))
                  }
                  onConnectTarget={() => {
                    setConnectTarget(true);
                    setPage("Hosts");
                  }}
                />
              </div>{" "}
              {page === "Einstellungen" && user.role === "admin" && (
                <Settings
                  notify={notify}
                  onDirtyChange={setSettingsDirty}
                  updating={!!updating}
                />
              )}{" "}
              {page === "Übersicht" && (
                <>
                  <div className="page-kicker">
                    <PageEmblem tone="blue">
                      <LayoutDashboard
                        size={20}
                        strokeWidth={1.6}
                        aria-hidden="true"
                      />
                    </PageEmblem>
                    <span>
                      Infrastruktur <span className="kicker-separator">/</span>{" "}
                      Alles im Blick
                    </span>
                  </div>
                  <Heading
                    title="Übersicht"
                    description="Was gesichert ist und wo Handlungsbedarf besteht."
                  />
                  <section
                    className="compass-overview"
                    aria-label="Direkt zum nächsten Schritt"
                  >
                    <div className="compass-overview-heading">
                      <span className="eyebrow">Dein nächster Schritt</span>
                      <p>Was möchtest du tun?</p>
                    </div>
                    <CompassEntrypoints
                      onChoose={(intent) => setCompass({ intent })}
                    />
                  </section>
                  <div
                    className={
                      "overview-brief" +
                      (overdue.length ? " needs-attention" : "")
                    }
                  >
                    <span
                      className={
                        "object-icon " + (overdue.length ? "ochre" : "sage")
                      }
                    >
                      {overdue.length ? (
                        <CircleAlert
                          size={21}
                          strokeWidth={1.6}
                          aria-hidden="true"
                        />
                      ) : (
                        <ShieldCheck
                          size={21}
                          strokeWidth={1.6}
                          aria-hidden="true"
                        />
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
                  <section
                    className="overview-summary"
                    aria-label="Sicherungsstatus"
                  >
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
                                <Icon
                                  size={14}
                                  strokeWidth={1.7}
                                  aria-hidden="true"
                                />
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
                                  setJobFilter("active");
                                  navigate("Aufträge");
                                } else {
                                  setHostFilter(metric.filter);
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
                          {pausedHosts} {pausedHosts === 1 ? "Host" : "Hosts"}{" "}
                          pausiert
                        </span>
                      )}
                    </p>
                  </section>
                  <ProblemOverview
                    status={status}
                    openHost={(id) => {
                      setSelectedHost(id);
                      navigate("Hosts");
                    }}
                    allHosts={() => navigate("Hosts")}
                  />
                  <ActiveJobs status={status} openJob={common.openJob} />
                  <details className="detail-section overview-evidence">
                    <summary>Prüfergebnisse der letzten Sicherung</summary>
                    <p className="muted">
                      Diese Nachweise gelten für einen einzelnen
                      Sicherungsstand. Sie bewerten nicht automatisch alle
                      Hosts.
                    </p>
                    <BackupReadiness
                      backup={
                        [...status.backups].sort((a, b) =>
                          b.created_at.localeCompare(a.created_at),
                        )[0]
                      }
                      compact
                    />
                    <button
                      className="secondary"
                      onClick={() => navigate("Sicherungen")}
                    >
                      Sicherungen prüfen und vergleichen
                    </button>
                  </details>
                  <details className="detail-section overview-history">
                    <summary>
                      <Clock3 size={15} aria-hidden="true" /> Letzte
                      abgeschlossene Aufträge
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
                      openBackup={setFile}
                      openPlan={common.openPlan}
                      limit={5}
                    />
                  </details>
                </>
              )}{" "}
              {page === "Aufträge" && (
                <>
                  <Heading
                    title="Aufträge"
                    description="Sicherungen, Hostprüfungen und Wiederherstellungen nachvollziehen."
                  />
                  <p className="hint">
                    {status.demo
                      ? "In der Demo läuft der automatische Zeitplan nicht."
                      : "Der tägliche Zeitplan wird vom Anker-Dienst jede Minute geprüft."}{" "}
                    Zeitzone: {status.timezone}.
                    {status.scheduler_health?.at
                      ? " Zuletzt geprüft: " +
                        date(status.scheduler_health.at) +
                        "."
                      : ""}
                  </p>
                  <Jobs
                    {...common}
                    initialJob={selectedJob}
                    initialStateFilter={jobFilter}
                    onInitialStateFilterConsumed={() => setJobFilter("")}
                    onOpenHosts={() => navigate("Hosts")}
                    onJobClose={() => setSelectedJob("")}
                  />
                </>
              )}
            </div>
          </main>
        </div>
        {compass && (
          <AnkerCompass
            status={status}
            initialIntent={compass.intent}
            initialHost={page === "Hosts" ? compassHost : ""}
            canRestore={user.role !== "reader"}
            stale={!!connectionError}
            onClose={() => setCompass(null)}
            onGo={goFromCompass}
          />
        )}
        {connectTarget && user.role === "admin" && (
          <HostForm
            notify={notify}
            onSaved={refresh}
            onClose={() => {
              setConnectTarget(false);
              setPage("Wiederherstellung");
            }}
          />
        )}
        {backup && (
          <FileBrowser
            backup={backup}
            user={user}
            onClose={() => setFile("")}
            notify={notify}
            onRestore={restore}
          />
        )}{" "}
        {pendingPage && (
          <Dialog
            title="Änderungen noch nicht gespeichert"
            onClose={() => setPendingPage("")}
          >
            <p className="dialog-intro">
              {page === "Wiederherstellung"
                ? "Eingaben auf der Planseite sind noch offen."
                : "Deine Änderungen an den Einstellungen sind noch offen."}
            </p>
            <footer className="dialog-footer">
              <button
                className="secondary"
                onClick={() => {
                  setPage(pendingPage);
                  setPendingPage("");
                  setSettingsDirty(false);
                  setRecoveryDirty(false);
                  if (page === "Wiederherstellung") {
                    setPlanRequest(null);
                    setRestoreRequest((old) => ({
                      id: "",
                      sequence: (old?.sequence || 0) + 1,
                    }));
                  }
                }}
              >
                Änderungen verwerfen
              </button>
              <button onClick={() => setPendingPage("")}>
                Weiter bearbeiten
              </button>
            </footer>
          </Dialog>
        )}
        {toast && (
          <div
            className={"toast " + (toast.error ? "toast-error" : "")}
            role={toast.error ? "alert" : "status"}
          >
            {toast.message}
            <button
              className="icon-button"
              aria-label="Meldung schließen"
              onClick={() => setToast(null)}
            >
              <X size={14} />
            </button>
          </div>
        )}
      </div>
    </ChangeRecoveryContext.Provider>
  );
}
