import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { api, date } from "../api";
import type { User } from "../api";
import { ActionMenu, Dialog, Field, LoadError } from "../components/shared";
import type { Notify } from "../components/shared";

export default function Access({
  notify,
  passwordMinimum,
}: {
  notify: Notify;
  passwordMinimum: number;
}) {
  const [users, setUsers] = useState<User[]>([]),
    [me, setMe] = useState("");
  const [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [reload, setReload] = useState(0);
  const [dialog, setDialog] = useState<{ kind: string; user?: User } | null>(
    null,
  );
  const refresh = () => setReload((n) => n + 1);
  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    Promise.all([api<User[]>("users"), api<{ user: User }>("me")])
      .then(([users, me]) => {
        if (active) {
          setUsers(users);
          setMe(me.user.id);
        }
      })
      .catch((e) => {
        if (active) setError(e.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [reload]);
  return (
    <section className="detail-section">
      <div className="section-heading">
        <div>
          <h3>Benutzer</h3>
          <p className="muted">
            Jeder Zugang hat eigene Rechte. Änderungen beenden bestehende
            Sitzungen.
          </p>
        </div>
        <button
          className="secondary"
          onClick={() => setDialog({ kind: "create" })}
        >
          Benutzer hinzufügen
        </button>
      </div>
      {error ? (
        <LoadError message={error} retry={refresh} />
      ) : loading ? (
        <p role="status" className="muted">
          Benutzer werden geladen …
        </p>
      ) : (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Rolle</th>
                <th>Geschützte Inhalte</th>
                <th>Status</th>
                <th className="right">Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id}>
                  <td>
                    <strong>{u.name}</strong>
                    {u.id === me && (
                      <small className="muted user-you">Du</small>
                    )}
                  </td>
                  <td>
                    {u.role === "admin"
                      ? "Administrator"
                      : u.role === "restore"
                        ? "Wiederherstellung"
                        : "Lesen"}
                  </td>
                  <td>{u.secrets ? "Freigegeben" : "Verdeckt"}</td>
                  <td>
                    <span className={u.disabled ? "muted" : ""}>
                      {u.disabled ? "Gesperrt" : "Aktiv"}
                    </span>
                  </td>
                  <td className="right">
                    <ActionMenu
                      label={"Aktionen für " + u.name}
                      actions={[
                        {
                          label: "Zugang bearbeiten",
                          run: () => setDialog({ kind: "edit", user: u }),
                        },
                        {
                          label: "Passwort ändern",
                          run: () => setDialog({ kind: "password", user: u }),
                        },
                        {
                          label: "Sitzungen verwalten",
                          run: () => setDialog({ kind: "sessions", user: u }),
                        },
                        ...(u.id === me
                          ? []
                          : [
                              {
                                label: "Benutzer löschen",
                                run: () =>
                                  setDialog({ kind: "delete", user: u }),
                              },
                            ]),
                      ]}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="table-footnote">
        Mindestens ein aktiver Administrator bleibt erhalten. Der eigene
        Administratorzugang lässt sich nicht sperren oder löschen.
      </p>
      {dialog && (
        <UserDialog
          key={dialog.kind + dialog.user?.id}
          kind={dialog.kind}
          user={dialog.user}
          self={dialog.user?.id === me}
          passwordMinimum={passwordMinimum}
          notify={notify}
          done={() => {
            setDialog(null);
            refresh();
          }}
          close={() => setDialog(null)}
        />
      )}
    </section>
  );
}
function UserDialog({
  passwordMinimum,
  kind,
  user,
  self,
  notify,
  done,
  close,
}: {
  passwordMinimum: number;
  kind: string;
  user?: User;
  self: boolean;
  notify: Notify;
  done: () => void;
  close: () => void;
}) {
  const [name, setName] = useState(""),
    [password, setPassword] = useState(""),
    [repeat, setRepeat] = useState("");
  const [role, setRole] = useState(user?.role || "reader"),
    [secrets, setSecrets] = useState(user?.secrets || false),
    [disabled, setDisabled] = useState(user?.disabled || false);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [sessions, setSessions] = useState<
      { id: string; created_at: string; expires: string }[] | null
    >(null);
  const path = "users/" + encodeURIComponent(user?.id || "");
  const loadSessions = () => {
    setError("");
    api<typeof sessions>(path + "/sessions")
      .then(setSessions)
      .catch((e) => setError(e.message));
  };
  useEffect(() => {
    if (kind === "sessions") loadSessions();
  }, []);
  const title = (
    {
      create: "Benutzer hinzufügen",
      edit: "Zugang bearbeiten",
      password: "Passwort ändern",
      sessions: "Sitzungen verwalten",
      delete: "Benutzer löschen",
    } as Record<string, string>
  )[kind];
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      if (kind === "create")
        await api("users", "POST", { name, password, role, secrets });
      if (kind === "edit") await api(path, "PUT", { role, secrets, disabled });
      if (kind === "password")
        await api(path + "/password", "POST", { password });
      if (kind === "delete") await api(path, "DELETE");
      if (kind === "sessions") await api(path + "/sessions", "DELETE");
      notify(
        kind === "create"
          ? "Benutzer angelegt"
          : kind === "delete"
            ? "Benutzer gelöscht"
            : kind === "sessions"
              ? "Alle Sitzungen beendet"
              : "Zugang geändert; bestehende Sitzungen beendet",
      );
      done();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog title={title} busy={busy} onClose={close}>
      <p className="dialog-intro">
        {kind === "create"
          ? "Ein eigener Zugang für die Arbeit mit Anker."
          : kind === "delete"
            ? `Der Zugang von ${user?.name} und alle zugehörigen Sitzungen werden entfernt. Sicherungen bleiben erhalten.`
            : `Zugang von ${user?.name}${self ? " · dein Benutzer" : ""}.`}
      </p>
      <form onSubmit={save}>
        <fieldset className="form-fields" disabled={busy}>
          {kind === "create" && (
            <Field
              label="Benutzername"
              hint="Buchstaben, Zahlen, Bindestrich und Unterstrich."
            >
              <input
                required
                pattern="[A-Za-z0-9_-]+"
                maxLength={100}
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoComplete="off"
              />
            </Field>
          )}
          {(kind === "create" || kind === "password") && (
            <>
              <Field
                label={kind === "password" ? "Neues Passwort" : "Passwort"}
                hint={`Mindestens ${passwordMinimum} Zeichen.`}
              >
                <input
                  type="password"
                  autoComplete="new-password"
                  required
                  minLength={passwordMinimum}
                  maxLength={1024}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </Field>
              <Field label="Passwort wiederholen">
                <input
                  type="password"
                  autoComplete="new-password"
                  required
                  value={repeat}
                  onChange={(e) => setRepeat(e.target.value)}
                />
              </Field>
            </>
          )}
          {(kind === "create" || kind === "edit") && (
            <>
              <Field label="Rolle">
                <select
                  value={role}
                  disabled={self}
                  onChange={(e) => setRole(e.target.value)}
                >
                  <option value="reader">Lesen</option>
                  <option value="restore">Wiederherstellung</option>
                  <option value="admin">Administrator</option>
                </select>
              </Field>
              <label className="check">
                <input
                  type="checkbox"
                  checked={role === "admin" || secrets}
                  disabled={role === "admin"}
                  onChange={(e) => setSecrets(e.target.checked)}
                />
                Geschützte Inhalte und vollständige Exporte erlauben
              </label>
              {kind === "edit" && (
                <label className="check">
                  <input
                    type="checkbox"
                    checked={disabled}
                    disabled={self}
                    onChange={(e) => setDisabled(e.target.checked)}
                  />
                  Zugang gesperrt
                </label>
              )}
              <p className="field-hint">
                {role === "reader"
                  ? "Darf Hosts und freigegebene Dateien ansehen und herunterladen."
                  : role === "restore"
                    ? "Darf außerdem Sicherungen erstellen und Wiederherstellungen ausführen."
                    : "Darf außerdem Benutzer, Einstellungen und Updates verwalten."}
              </p>
            </>
          )}
          {kind === "sessions" && (
            <>
              <p className="muted">
                {self
                  ? "Wenn du alle Sitzungen beendest, musst auch du dich erneut anmelden."
                  : "Beendet die Anmeldung auf allen Geräten dieses Benutzers."}
              </p>
              {sessions ? (
                <>
                  {sessions.length ? (
                    <ul className="session-list">
                      {sessions.map((s, i) => (
                        <li key={s.id}>
                          <div>
                            <strong>Sitzung {i + 1}</strong>
                            <small>Angemeldet am {date(s.created_at)}</small>
                          </div>
                          <span>Läuft ab am {date(s.expires)}</span>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p>Keine aktiven Sitzungen.</p>
                  )}
                </>
              ) : (
                !error && <p role="status">Sitzungen werden geladen …</p>
              )}
            </>
          )}
          {error && (
            <p className="notice warning" role="alert">
              {error}
            </p>
          )}
          {kind === "sessions" && error && (
            <button type="button" className="secondary" onClick={loadSessions}>
              Erneut laden
            </button>
          )}
          <footer className="dialog-footer">
            <button
              type="button"
              className="secondary"
              onClick={close}
              disabled={busy}
            >
              Abbrechen
            </button>
            <button
              className={kind === "delete" ? "danger" : ""}
              disabled={
                busy ||
                ((kind === "password" || kind === "create") &&
                  (Array.from(password).length < passwordMinimum ||
                    password !== repeat)) ||
                (kind === "sessions" && (!sessions || sessions.length === 0))
              }
            >
              {busy
                ? "Wird gespeichert …"
                : kind === "create"
                  ? "Benutzer anlegen"
                  : kind === "delete"
                    ? "Benutzer löschen"
                    : kind === "sessions"
                      ? "Alle Sitzungen beenden"
                      : "Änderungen speichern"}
            </button>
          </footer>
        </fieldset>
      </form>
    </Dialog>
  );
}
