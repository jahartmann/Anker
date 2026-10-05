import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";
import {
  LayoutGrid,
  Server,
  FolderArchive,
  RotateCcw,
  ListTodo,
  Settings as SettingsIcon,
  Menu,
  ChevronRight,
  LogOut,
  X,
} from "lucide-react";
import { api, date, emptyStatus, hostState, labels } from "./api";
import type { Status, User } from "./api";
import { Empty, Field, Heading, State } from "./components/shared";
import Hosts from "./views/Hosts";
import Backups, { FileBrowser } from "./views/Backups";
import Restore from "./views/Restore";
import Settings from "./views/Settings";
const pages = [
  ["Übersicht", LayoutGrid],
  ["Hosts", Server],
  ["Sicherungen", FolderArchive],
  ["Wiederherstellung", RotateCcw],
  ["Aufträge", ListTodo],
  ["Einstellungen", SettingsIcon],
] as const;
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
      <div className="login-form">
        <div className="wordmark">Anker</div>
        <h1>Anmelden</h1>
        <p>Deine Hostkonfigurationen an einem Ort.</p>
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
  const [page, setPage] = useState("Hosts");
  const [nav, setNav] = useState(false);
  const [file, setFile] = useState("");
  const [restoreBackup, setRestoreBackup] = useState("");
  const [toast, setToast] = useState<{
    message: string;
    error: boolean;
  } | null>(null);
  const notify = useCallback(
    (message: string, error = false) => setToast({ message, error }),
    [],
  );
  const refresh = useCallback(async () => {
    try {
      const s = await api<Status>("status");
      setStatus({
        ...s,
        hosts: s.hosts || [],
        backups: s.backups || [],
        plans: s.plans || [],
        jobs: s.jobs || [],
      });
    } catch (e) {
      if ((e as Error).message === "Anmeldung erforderlich") setUser(null);
      else notify((e as Error).message, true);
    }
  }, [notify]);
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
  function navigate(p: string) {
    setPage(p);
    setNav(false);
    setRestoreBackup("");
  }
  function restore(id: string) {
    setRestoreBackup(id);
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
    openBackup: setFile,
    onRestore: restore,
  };
  const overdue = status.hosts.filter(
    (h) => hostState(h, status).tone !== "success",
  );
  const active = status.jobs.filter((j) =>
    ["queued", "running"].includes(j.state),
  );
  return (
    <div className="app-shell">
      {nav && (
        <button
          className="nav-scrim"
          aria-label="Navigation schließen"
          onClick={() => setNav(false)}
        />
      )}
      <aside className={nav ? "sidebar open" : "sidebar"}>
        <div className="brand">Anker</div>
        <nav aria-label="Hauptnavigation">
          {pages
            .filter(([p]) => p !== "Einstellungen" || user.role === "admin")
            .map(([p]) => (
              <button
                key={p}
                className={page === p ? "active" : ""}
                onClick={() => navigate(p)}
              >
                {p}
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
              onClick={async () => {
                await api("logout", "POST", {});
                setUser(null);
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
            onClick={() => setNav(true)}
          >
            <Menu size={19} />
          </button>
          <span>Anker</span>
          <ChevronRight size={12} />
          <span>{page}</span>
        </div>
        <main key={page}>
          <div className="content">
            {page === "Hosts" && (
              <Hosts {...common} canEdit={user.role === "admin"} />
            )}{" "}
            {page === "Sicherungen" && <Backups {...common} />}{" "}
            {page === "Wiederherstellung" && (
              <Restore
                status={status}
                initialBackup={restoreBackup}
                user={user}
                notify={notify}
                refresh={refresh}
              />
            )}{" "}
            {page === "Einstellungen" && user.role === "admin" && (
              <Settings notify={notify} />
            )}{" "}
            {page === "Übersicht" && (
              <>
                <Heading
                  title="Übersicht"
                  description="Was gesichert ist und wo Handlungsbedarf besteht."
                />
                <section className="overview-summary">
                  <p>
                    <strong>
                      {status.hosts.length - overdue.length} von{" "}
                      {status.hosts.length} Hosts
                    </strong>{" "}
                    haben eine aktuelle Sicherung.
                  </p>
                  <p className="muted">
                    {active.length
                      ? active.length + " Aufträge laufen oder warten."
                      : "Keine aktiven Aufträge."}{" "}
                    Zeitzone {status.timezone}.
                  </p>
                </section>
                <section>
                  <div className="section-heading">
                    <h3>Handlungsbedarf</h3>
                    <button
                      className="text-button"
                      onClick={() => navigate("Hosts")}
                    >
                      Alle Hosts ansehen
                    </button>
                  </div>
                  {overdue.length ? (
                    <div className="table-scroll">
                      <table>
                        <thead>
                          <tr>
                            <th>Host</th>
                            <th>Status</th>
                            <th>Letzter Stand</th>
                          </tr>
                        </thead>
                        <tbody>
                          {overdue.map((h) => (
                            <tr key={h.id}>
                              <td>{h.name}</td>
                              <td className="warning">
                                {hostState(h, status).text}
                              </td>
                              <td>
                                {date(hostState(h, status).backup?.created_at)}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <Empty title="Alle Hosts sind aktuell gesichert">
                      Die Aufbewahrung läuft nach dem hinterlegten Zeitplan.
                    </Empty>
                  )}
                </section>
                <section className="detail-section">
                  <h3>Letzte Aufträge</h3>
                  <Jobs
                    status={status}
                    notify={notify}
                    refresh={refresh}
                    canEdit={user.role !== "reader"}
                    limit={5}
                  />
                </section>
              </>
            )}{" "}
            {page === "Aufträge" && (
              <>
                <Heading
                  title="Aufträge"
                  description="Sicherungen, Hostprüfungen und Wiederherstellungen nachvollziehen."
                />
                <Jobs {...common} />
              </>
            )}
          </div>
        </main>
      </div>
      {backup && (
        <FileBrowser
          backup={backup}
          user={user}
          onClose={() => setFile("")}
          notify={notify}
          onRestore={restore}
        />
      )}{" "}
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
  );
}
function Jobs({
  status,
  notify,
  refresh,
  canEdit,
  limit,
}: {
  status: Status;
  notify: (s: string, b?: boolean) => void;
  refresh: () => void;
  canEdit: boolean;
  limit?: number;
}) {
  const jobs = [...status.jobs]
    .sort((a, b) => b.created_at.localeCompare(a.created_at))
    .slice(0, limit);
  return jobs.length ? (
    <div className="table-scroll">
      <table>
        <thead>
          <tr>
            <th>Auftrag</th>
            <th>Status</th>
            <th>Start</th>
            <th>Ergebnis</th>
          </tr>
        </thead>
        <tbody>
          {jobs.map((j) => (
            <tr key={j.id}>
              <td>
                <strong>{labels[j.kind] || j.kind}</strong>
                <div className="secondary-line">
                  {status.hosts.find((h) => h.id === j.host_id)?.name ||
                    j.host_id}
                </div>
              </td>
              <td>
                <State value={j.state} />
              </td>
              <td className="date">{date(j.created_at)}</td>
              <td>
                {j.error ? (
                  <span className="warning">{j.error}</span>
                ) : j.state === "successful" ? (
                  "Abgeschlossen"
                ) : j.state === "running" ? (
                  "Versuch " + j.attempts
                ) : (
                  "—"
                )}
                {canEdit && ["queued", "running"].includes(j.state) && (
                  <button
                    className="text-button"
                    onClick={async () => {
                      try {
                        await api("jobs/" + j.id + "/cancel", "POST", {});
                        notify("Abbruch angefordert");
                        refresh();
                      } catch (e) {
                        notify((e as Error).message, true);
                      }
                    }}
                  >
                    Abbrechen
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  ) : (
    <Empty title="Noch keine Aufträge">
      Gestartete Vorgänge erscheinen hier mit ihrem Ergebnis.
    </Empty>
  );
}
