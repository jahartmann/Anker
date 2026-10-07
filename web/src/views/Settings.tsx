import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { api, date } from "../api";
import type { Settings as Values } from "../api";
import { Field, Heading, LoadError } from "../components/shared";
import type { Notify } from "../components/shared";
import Access from "./Access";
import Updates from "./Updates";
import Certificates from "./Certificates";
import Storage from "./Storage";
export default function Settings({
  notify,
  onDirtyChange,
  updating = false,
}: {
  notify: Notify;
  onDirtyChange: (dirty: boolean) => void;
  updating?: boolean;
}) {
  const [value, setValue] = useState<Values | null>(null);
  const [busy, setBusy] = useState(false);
  const [tab, setTab] = useState("Sicherung");
  const [audit, setAudit] = useState<
    { at: string; action: string; object: string; user_id: string }[]
  >([]);
  const [doctor, setDoctor] = useState<unknown>(null);
  const [saved, setSaved] = useState("");
  const [loadError, setLoadError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [reload, setReload] = useState(0);
  const [archiveAfter, setArchiveAfter] = useState(90);
  const dirty = !!value && JSON.stringify(value) !== saved;
  const [certificateDirty, setCertificateDirty] = useState(false);
  useEffect(() => {
    onDirtyChange(dirty || certificateDirty);
  }, [dirty, certificateDirty, onDirtyChange]);
  useEffect(() => {
    if (!dirty && !certificateDirty) return;
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty, certificateDirty]);
  useEffect(() => {
    let active = true;
    setLoadError("");
    api<Values>("settings")
      .then((v) => {
        if (active) {
          setValue(v);
          setSaved(JSON.stringify(v));
          if (v.archive_days > 0) setArchiveAfter(v.archive_days);
        }
      })
      .catch((e) => {
        if (active) setLoadError(e.message);
      });
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
        {[
          "Sicherung",
          "Speicher",
          "Benachrichtigungen",
          "Zugriff",
          "System",
        ].map((t) => (
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
      {tab === "Speicher" && <Storage notify={notify} />}
      {["Sicherung", "Benachrichtigungen"].includes(tab) && (
        <form className="settings-form" onSubmit={save}>
          <fieldset className="form-fields" disabled={busy || updating}>
            {tab === "Sicherung" ? (
              <>
                <section>
                  <h3>Zeitplan</h3>
                  <p className="muted">
                    Hosts starten zeitlich versetzt innerhalb einer Stunde.
                    Eigene Hostzeiten haben Vorrang. Änderungen gelten für noch
                    nicht eingeplante Läufe; ein bereits eingeplanter Tageslauf
                    wird nicht erneut gestartet.
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
                  <p className="hint">
                    Ein neues Parallelitätslimit gilt für wartende Aufträge.
                    Laufende Aufträge werden dafür nicht abgebrochen.
                  </p>
                  <div className="freshness-setting">
                    <Field
                      label="Überfällig nach Stunden"
                      hint="Ohne aktuelle erfolgreiche Sicherung erscheint ein Hinweis am Host. Diese Grenze löscht keine Daten."
                    >
                      <input
                        type="number"
                        min={1}
                        required
                        value={value.stale_hours}
                        onChange={(e) => set("stale_hours", +e.target.value)}
                      />
                    </Field>
                  </div>
                </section>
                <section>
                  <h3>Aufbewahrung</h3>
                  <p className="muted">
                    Die Regeln gelten für jeden Host einzeln. Anker behält
                    jeweils den neuesten erfolgreichen Stand pro Tag, Woche und
                    Monat. Gezählt werden Zeiträume, in denen eine Sicherung
                    vorhanden ist.
                  </p>
                  <div className="retention-rules">
                    {(
                      [
                        [
                          "daily",
                          "Tagesstände",
                          "Ein Stand je Sicherungstag – für Änderungen der letzten Tage.",
                        ],
                        [
                          "weekly",
                          "Wochenstände",
                          "Ein Stand je Kalenderwoche – für den Blick weiter zurück.",
                        ],
                        [
                          "monthly",
                          "Monatsstände",
                          "Ein Stand je Kalendermonat – für die langfristige Historie.",
                        ],
                      ] as const
                    ).map(([k, l, hint]) => (
                      <Field key={k} label={l} hint={hint}>
                        <input
                          type="number"
                          min={1}
                          required
                          value={value[k]}
                          onChange={(e) => set(k, +e.target.value)}
                        />
                      </Field>
                    ))}
                  </div>
                  <div
                    className="retention-summary"
                    role="status"
                    aria-label="Aufbewahrungsübersicht"
                  >
                    <span className="count-badge">Je Host</span>
                    <span>{value.daily} Tage</span>
                    <span>{value.weekly} Wochen</span>
                    <span>{value.monthly} Monate</span>
                  </div>
                  <p className="field-hint">
                    Ein Stand kann mehrere Regeln erfüllen; die Zahlen werden
                    nicht addiert. Stände außerhalb dieser Regeln werden bei der
                    täglichen Wartung gelöscht.
                  </p>
                  <div className="retention-protection">
                    <span className="count-badge success">
                      Bleiben erhalten
                    </span>
                    <p>
                      Der letzte erfolgreiche Stand, geschützte Stände,
                      unvollständige oder beschädigte Sicherungen und Stände aus
                      Wiederherstellungsplänen.
                    </p>
                  </div>
                  <div className="archive-policy">
                    <label className="check">
                      <input
                        type="checkbox"
                        checked={value.archive_days > 0}
                        onChange={(e) =>
                          set(
                            "archive_days",
                            e.target.checked ? archiveAfter : 0,
                          )
                        }
                      />
                      Ältere Stände automatisch archivieren
                    </label>
                    <p className="field-hint">
                      Archivierung spart Platz: Die Dateien werden komprimiert,
                      der Stand bleibt im Web lesbar und herunterladbar. Sie
                      verlängert die Aufbewahrung nicht.
                    </p>
                    <Field
                      label="Archivierung nach Tagen"
                      hint="Geschützte und in Plänen verwendete Stände bleiben als lesbare Ordner erhalten."
                    >
                      <input
                        type="number"
                        min={1}
                        required={value.archive_days > 0}
                        disabled={value.archive_days === 0}
                        value={value.archive_days || archiveAfter}
                        onChange={(e) => {
                          setArchiveAfter(+e.target.value);
                          set("archive_days", +e.target.value);
                        }}
                      />
                    </Field>
                    {value.archive_days === 0 && (
                      <p className="field-hint">
                        Automatische Archivierung ist ausgeschaltet. Die
                        Aufbewahrungsregeln gelten weiterhin.
                      </p>
                    )}
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
        <div inert={updating}>
          <form className="settings-form access-policy" onSubmit={save}>
            <section>
              <h3>Anmeldung</h3>
              <p className="muted">
                Eine Anmeldung ist immer erforderlich. Die Dauer gilt für neue
                Sitzungen, auch über einen Neustart hinweg.
              </p>
              <Field
                label="Sitzungsdauer in Tagen"
                hint="1 bis 365 Tage. Standard: 30 Tage."
              >
                <input
                  type="number"
                  min={1}
                  max={365}
                  required
                  value={value.session_days}
                  disabled={busy || updating}
                  onChange={(e) => set("session_days", +e.target.value)}
                />
              </Field>
              <Field
                label="Passwort-Mindestlänge"
                hint="8 bis 128 Zeichen. Gilt für neu angelegte und geänderte Passwörter; bestehende bleiben gültig."
              >
                <input
                  type="number"
                  min={8}
                  max={128}
                  required
                  value={value.password_min_length ?? 8}
                  disabled={busy || updating}
                  onChange={(e) => set("password_min_length", +e.target.value)}
                />
              </Field>
              {saveError && (
                <p className="notice warning" role="alert">
                  {saveError}
                </p>
              )}
              <footer className="form-footer">
                <span className="save-state">
                  {dirty
                    ? "Ungespeicherte Änderungen"
                    : "Alle Änderungen gespeichert"}
                </span>
                <button disabled={busy || !dirty}>
                  {busy ? "Speichert …" : "Einstellungen speichern"}
                </button>
              </footer>
            </section>
          </form>
          <Access
            notify={notify}
            passwordMinimum={
              (JSON.parse(saved) as Values).password_min_length || 8
            }
          />
        </div>
      )}
      <section className="detail-section" hidden={tab !== "System"}>
        <div inert={updating}>
          <Certificates notify={notify} onDirtyChange={setCertificateDirty} />
        </div>
        <Updates canInstall={!dirty && !certificateDirty} />
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
    </>
  );
}
