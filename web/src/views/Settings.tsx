import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { api, date } from "../api";
import type { Settings as Values, User } from "../api";
import { Dialog, Field, Heading, LoadError } from "../components/shared";
import type { Notify } from "../components/shared";
export default function Settings({
  notify,
  onDirtyChange,
}: {
  notify: Notify;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const [value, setValue] = useState<Values | null>(null);
  const [busy, setBusy] = useState(false);
  const [tab, setTab] = useState("Sicherung");
  const [users, setUsers] = useState<User[]>([]);
  const [audit, setAudit] = useState<
    { at: string; action: string; object: string; user_id: string }[]
  >([]);
  const [doctor, setDoctor] = useState<unknown>(null);
  const [resetUser, setResetUser] = useState<User | null>(null);
  const [form, setForm] = useState(false);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("reader");
  const [secrets, setSecrets] = useState(false);
  const [saved, setSaved] = useState("");
  const [loadError, setLoadError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [reload, setReload] = useState(0);
  const dirty = !!value && JSON.stringify(value) !== saved;
  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);
  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);
  useEffect(() => {
    let active = true;
    setLoadError("");
    api<Values>("settings")
      .then((v) => {
        if (active) {
          setValue(v);
          setSaved(JSON.stringify(v));
        }
      })
      .catch((e) => {
        if (active) setLoadError(e.message);
      });
    api<User[]>("users")
      .then(setUsers)
      .catch((e) => notify(e.message, true));
    return () => {
      active = false;
    };
  }, [notify, reload]);
  const set = (key: keyof Values, v: unknown) =>
    setValue((old) => (old ? { ...old, [key]: v } : old));
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setSaveError("");
    try {
      const result = await api<Values>("settings", "PUT", value);
      setValue(result);
      setSaved(JSON.stringify(result));
      notify("Einstellungen gespeichert");
    } catch (e) {
      setSaveError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function addUser(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await api("users", "POST", { name, password, role, secrets });
      setUsers(await api("users"));
      setForm(false);
      setPassword("");
      notify("Benutzer angelegt");
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  }
  async function inspect() {
    try {
      setDoctor(await api("doctor"));
      setAudit(await api("audit"));
    } catch (e) {
      notify((e as Error).message, true);
    }
  }
  if (!value)
    return loadError ? (
      <LoadError message={loadError} retry={() => setReload((v) => v + 1)} />
    ) : (
      <p className="muted" role="status">
        Einstellungen werden geladen …
      </p>
    );
  return (
    <>
      <Heading
        title="Einstellungen"
        description="Zeitplan, Aufbewahrung und Zugriff zentral verwalten."
      />
      <div className="tabs">
        {["Sicherung", "Benachrichtigungen", "Zugriff", "System"].map((t) => (
          <button
            key={t}
            className={t === tab ? "selected" : ""}
            aria-pressed={t === tab}
            onClick={() => {
              setTab(t);
              if (t === "System") inspect();
            }}
          >
            {t}
          </button>
        ))}
      </div>
      {["Sicherung", "Benachrichtigungen"].includes(tab) && (
        <form className="settings-form" onSubmit={save}>
          <fieldset className="form-fields" disabled={busy}>
            {tab === "Sicherung" ? (
              <>
                <section>
                  <h3>Zeitplan</h3>
                  <p className="muted">
                    Hosts starten zeitlich versetzt innerhalb einer Stunde.
                    Eigene Hostzeiten haben Vorrang.
                  </p>
                  <div className="form-grid">
                    <Field label="Startzeit">
                      <input
                        type="time"
                        required
                        value={value.schedule}
                        onChange={(e) => set("schedule", e.target.value)}
                      />
                    </Field>
                    <Field label="Zeitzone">
                      <input
                        required
                        value={value.timezone}
                        onChange={(e) => set("timezone", e.target.value)}
                      />
                    </Field>
                    <Field label="Parallele Aufträge">
                      <input
                        type="number"
                        min={1}
                        max={16}
                        value={value.parallel}
                        onChange={(e) => set("parallel", +e.target.value)}
                      />
                    </Field>
                    <Field label="Wiederholungen">
                      <input
                        type="number"
                        min={0}
                        max={5}
                        value={value.retries}
                        onChange={(e) => set("retries", +e.target.value)}
                      />
                    </Field>
                  </div>
                </section>
                <section>
                  <h3>Aufbewahrung</h3>
                  <p className="muted">
                    Geschützte Stände, unvollständige Sicherungen und
                    referenzierte Pläne bleiben erhalten.
                  </p>
                  <div className="form-grid">
                    {(
                      [
                        ["daily", "Tagesstände"],
                        ["weekly", "Wochenstände"],
                        ["monthly", "Monatsstände"],
                        ["archive_days", "Archivierung nach Tagen"],
                        ["stale_hours", "Überfällig nach Stunden"],
                      ] as const
                    ).map(([k, l]) => (
                      <Field key={k} label={l}>
                        <input
                          type="number"
                          min={1}
                          value={value[k]}
                          onChange={(e) => set(k, +e.target.value)}
                        />
                      </Field>
                    ))}
                  </div>
                </section>
              </>
            ) : (
              <>
                <section>
                  <h3>Benachrichtigungen</h3>
                  <p className="muted">
                    Anker meldet fehlgeschlagene Sicherungen und die
                    anschließende Erholung.
                  </p>
                  <Field label="Webhook-URL">
                    <input
                      type="url"
                      value={value.webhook}
                      onChange={(e) => set("webhook", e.target.value)}
                    />
                  </Field>
                  <Field
                    label="SMTP-Server"
                    hint="Hostname und Port; verschlüsselte Verbindung erforderlich."
                  >
                    <input
                      value={value.smtp_server}
                      onChange={(e) => set("smtp_server", e.target.value)}
                      placeholder="mail.example.de:587"
                    />
                  </Field>
                  <div className="form-grid">
                    <Field label="SMTP-Benutzer">
                      <input
                        value={value.smtp_user}
                        onChange={(e) => set("smtp_user", e.target.value)}
                      />
                    </Field>
                    <Field
                      label="SMTP-Passwort"
                      hint="Leer lassen, um das vorhandene Passwort zu behalten."
                    >
                      <input
                        type="password"
                        autoComplete="new-password"
                        value={value.smtp_password || ""}
                        onChange={(e) => set("smtp_password", e.target.value)}
                      />
                    </Field>
                    <Field label="Absender">
                      <input
                        type="email"
                        value={value.mail_from}
                        onChange={(e) => set("mail_from", e.target.value)}
                      />
                    </Field>
                    <Field label="Empfänger">
                      <input
                        type="email"
                        value={value.mail_to}
                        onChange={(e) => set("mail_to", e.target.value)}
                      />
                    </Field>
                  </div>
                  <button
                    type="button"
                    className="secondary"
                    disabled={dirty || busy}
                    title={
                      dirty ? "Zuerst die Änderungen speichern" : undefined
                    }
                    onClick={async () => {
                      try {
                        await api("notifications/test", "POST", {});
                        notify("Testnachricht gesendet");
                      } catch (e) {
                        notify((e as Error).message, true);
                      }
                    }}
                  >
                    Gespeicherte Verbindung testen
                  </button>
                  {dirty && (
                    <p className="field-hint">
                      Zum Testen zuerst die Änderungen speichern.
                    </p>
                  )}
                </section>
              </>
            )}
            {saveError && (
              <p className="notice warning" role="alert">
                {saveError}
              </p>
            )}
            <footer className="form-footer">
              <span className="save-state" role="status">
                {dirty
                  ? "Ungespeicherte Änderungen"
                  : "Alle Änderungen gespeichert"}
              </span>
              <button disabled={busy || !dirty}>
                {busy ? "Speichert …" : "Einstellungen speichern"}
              </button>
            </footer>
          </fieldset>
        </form>
      )}
      {tab === "Zugriff" && (
        <section className="detail-section">
          <div className="section-heading">
            <div>
              <h3>Benutzer</h3>
              <p className="muted">
                Lesen, Wiederherstellen und Administration getrennt vergeben.
              </p>
            </div>
            <button className="secondary" onClick={() => setForm(true)}>
              Benutzer hinzufügen
            </button>
          </div>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Rolle</th>
                  <th>Geschützte Inhalte</th>
                  <th className="right">Aktionen</th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr key={u.id}>
                    <td>{u.name}</td>
                    <td>
                      {u.role === "admin"
                        ? "Administrator"
                        : u.role === "restore"
                          ? "Wiederherstellung"
                          : "Lesen"}
                    </td>
                    <td>{u.secrets ? "Freigegeben" : "Verdeckt"}</td>
                    <td className="right">
                      <button
                        className="text-button"
                        onClick={() => setResetUser(u)}
                      >
                        Passwort ändern
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="table-footnote">
            Ein neues Passwort beendet bestehende Sitzungen dieses Benutzers.
          </p>
        </section>
      )}
      {tab === "System" && (
        <section className="detail-section">
          <h3>Systemprüfung</h3>
          <pre className="system-output">
            {doctor ? JSON.stringify(doctor, null, 2) : "Wird geladen …"}
          </pre>
          <button
            className="secondary"
            onClick={async () => {
              try {
                const r = await api<{ indexed: number }>("reindex", "POST", {});
                notify(r.indexed + " Sicherungen neu eingelesen");
              } catch (e) {
                notify((e as Error).message, true);
              }
            }}
          >
            Sicherungsindex neu einlesen
          </button>
          <h3>Aktivitätsprotokoll</h3>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Zeitpunkt</th>
                  <th>Aktion</th>
                  <th>Ziel</th>
                </tr>
              </thead>
              <tbody>
                {audit.map((a, i) => (
                  <tr key={i}>
                    <td className="date">{date(a.at)}</td>
                    <td>{a.action}</td>
                    <td className="mono">{a.object}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
      {resetUser && (
        <ResetPassword
          user={resetUser}
          notify={notify}
          onClose={() => setResetUser(null)}
        />
      )}
      {form && (
        <Dialog
          title="Benutzer hinzufügen"
          busy={busy}
          onClose={() => {
            setForm(false);
            setPassword("");
          }}
        >
          <form onSubmit={addUser}>
            <Field label="Benutzername">
              <input
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </Field>
            <Field label="Passwort" hint="Mindestens 12 Zeichen.">
              <input
                type="password"
                required
                minLength={12}
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            <Field label="Rolle">
              <select value={role} onChange={(e) => setRole(e.target.value)}>
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
            <footer className="dialog-footer">
              <button
                type="button"
                className="secondary"
                disabled={busy}
                onClick={() => {
                  setForm(false);
                  setPassword("");
                }}
              >
                Abbrechen
              </button>
              <button disabled={busy}>Benutzer anlegen</button>
            </footer>
          </form>
        </Dialog>
      )}
    </>
  );
}

function ResetPassword({
  user,
  notify,
  onClose,
}: {
  user: User;
  notify: Notify;
  onClose: () => void;
}) {
  const [password, setPassword] = useState("");
  const [repeat, setRepeat] = useState("");
  const [busy, setBusy] = useState(false);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await api("users/" + encodeURIComponent(user.id) + "/password", "POST", {
        password,
      });
      notify("Passwort geändert; bestehende Sitzungen beendet");
      onClose();
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog title="Passwort ändern" onClose={onClose} busy={busy}>
      <p className="dialog-intro">
        Neues Passwort für {user.name}. Bestehende Sitzungen dieses Benutzers
        werden beendet.
      </p>
      <form onSubmit={save}>
        <Field label="Neues Passwort" hint="Mindestens 12 Zeichen.">
          <input
            type="password"
            autoComplete="new-password"
            required
            minLength={12}
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
        <footer className="dialog-footer">
          <button className="secondary" type="button" onClick={onClose}>
            Abbrechen
          </button>
          <button
            disabled={busy || password.length < 12 || password !== repeat}
          >
            Passwort speichern
          </button>
        </footer>
      </form>
    </Dialog>
  );
}
