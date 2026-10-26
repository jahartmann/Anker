import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { api, date } from "../api";
import type { Settings as Values } from "../api";
import { DetailSection, Field, Heading, LoadError } from "../components/shared";
import type { Notify } from "../components/shared";
import Access from "./Access";
import Updates from "./Updates";
import Certificates from "./Certificates";
import Storage from "./Storage";
import SystemDiagnosis from "../components/SystemDiagnosis";

const settingsTabs = [
  "Zeitplan",
  "Aufbewahrung",
  "Benachrichtigungen",
  "Zugriff",
  "Zertifikate",
  "Updates",
  "Betrieb",
] as const;

const settingsSections = {
  Zeitplan: ["schedule", "timezone", "parallel", "retries", "stale_hours"],
  Aufbewahrung: ["daily", "weekly", "monthly", "archive_days"],
  Benachrichtigungen: [
    "webhook",
    "smtp_server",
    "smtp_user",
    "smtp_password",
    "mail_from",
    "mail_to",
  ],
  Zugriff: ["session_days", "password_min_length"],
} satisfies Record<string, (keyof Values)[]>;

const settingsContext: Record<string, [string, string]> = {
  Zeitplan: [
    "Zeitplan",
    "Hosts starten zeitlich versetzt innerhalb einer Stunde.",
  ],
  Aufbewahrung: [
    "Aufbewahrung",
    "Je Host der neueste erfolgreiche Stand pro Zeitraum.",
  ],
  Benachrichtigungen: [
    "Benachrichtigungen",
    "Anker meldet Fehler und gibt Bescheid, wenn Sicherungen wieder funktionieren.",
  ],
  Zugriff: ["Zugriff", "Anmeldung, Sitzungsdauer und Benutzer verwalten."],
  Zertifikate: [
    "HTTPS und Erneuerung",
    "Webzertifikat prüfen und die automatische Erneuerung verwalten.",
  ],
  Updates: [
    "Software verwalten",
    "Installierte Version, Updatequelle und verfügbare Updates prüfen.",
  ],
  Betrieb: ["Betrieb", "Speicher prüfen und letzte Aktionen ansehen."],
};

const betriebAreas = [
  ["betrieb-speicher", "Speicher", "Volumes und Prognose"],
  ["betrieb-pruefung", "Prüfung", "Doctor und Index"],
  ["betrieb-protokoll", "Protokoll", "Letzte Aktionen"],
] as const;

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
  const [tab, setTab] = useState("Zeitplan");
  const [audit, setAudit] = useState<
    { at: string; action: string; object: string; user_id: string }[]
  >([]);
  const [doctor, setDoctor] = useState<unknown>(null);
  const [saved, setSaved] = useState("");
  const [loadError, setLoadError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [timezoneError, setTimezoneError] = useState("");
  const timezoneInput = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (timezoneError && tab === "Zeitplan") timezoneInput.current?.focus();
  }, [timezoneError, tab]);
  const [systemError, setSystemError] = useState("");
  const [inspecting, setInspecting] = useState(false);
  const [testError, setTestError] = useState("");
  const [testing, setTesting] = useState(false);
  const [reload, setReload] = useState(0);
  const [archiveAfter, setArchiveAfter] = useState(90);
  const dirty = !!value && JSON.stringify(value) !== saved;
  const [certificateDirty, setCertificateDirty] = useState(false);
  const savedValue = saved ? (JSON.parse(saved) as Values) : null;
  const changedSections = Object.entries(settingsSections)
    .filter(([, keys]) =>
      keys.some((key) => value && savedValue && value[key] !== savedValue[key]),
    )
    .map(([name]) => name);
  if (certificateDirty) changedSections.push("Zertifikate");
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
  const set = (key: keyof Values, v: unknown) => {
    if (key === "timezone") setTimezoneError("");
    setSaveError("");
    setTestError("");
    setValue((old) => (old ? { ...old, [key]: v } : old));
  };
  async function save(e: FormEvent) {
    e.preventDefault();
    if (busy || !value) return;
    try {
      if (value.timezone !== "Local")
        new Intl.DateTimeFormat("de-DE", { timeZone: value.timezone }).format();
    } catch {
      setTimezoneError(
        "Gültige Zeitzone eingeben, zum Beispiel Europe/Berlin.",
      );
      if (tab !== "Zeitplan") setTab("Zeitplan");
      timezoneInput.current?.focus();
      return;
    }
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
    setInspecting(true);
    setSystemError("");
    try {
      const [checked, events] = await Promise.all([
        api("doctor"),
        api<typeof audit>("audit"),
      ]);
      setDoctor(checked);
      setAudit(events);
    } catch (e) {
      setSystemError((e as Error).message);
    } finally {
      setInspecting(false);
    }
  }
  const saveControls = (
    <footer className="form-footer settings-savebar">
      {saveError && (
        <p className="form-error" role="alert">
          {saveError}
        </p>
      )}
      <div className="settings-save-details">
        <span className="save-state" role="status">
          {dirty ? "Ungespeicherte Änderungen" : "Alle Änderungen gespeichert"}
        </span>
        <small className="muted">
          Speichert alle geänderten Einstellungsbereiche.
        </small>
      </div>
      <button disabled={busy || updating || !dirty}>
        {busy ? "Speichert …" : "Einstellungen speichern"}
      </button>
    </footer>
  );
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
        description="Zeitplan, Speicher und Zugriff verwalten."
        action={
          dirty || certificateDirty ? (
            <span className="save-state" role="status">
              Ungespeicherte Änderungen
            </span>
          ) : undefined
        }
      />
      <div className="tabs settings-tabs" aria-label="Einstellungsbereiche">
        {settingsTabs.map((t) => (
          <button
            key={t}
            type="button"
            className={t === tab ? "selected" : ""}
            aria-pressed={t === tab}
            title={
              changedSections.includes(t)
                ? `${t}: ungespeicherte Änderungen`
                : undefined
            }
            onClick={() => {
              setTab(t);
              if (t === "Betrieb") inspect();
            }}
          >
            {t}
            {changedSections.includes(t) && (
              <span className="settings-tab-dirty" aria-hidden="true">
                •
              </span>
            )}
          </button>
        ))}
      </div>
      {changedSections.length > 0 && (
        <div
          className="settings-change-summary"
          role="status"
          aria-label="Ungespeicherte Bereiche"
        >
          <span>Geändert:</span>
          {changedSections.map((name) => (
            <span key={name}>{name}</span>
          ))}
          {certificateDirty && (
            <small>Zertifikate separat im Bereich speichern.</small>
          )}
        </div>
      )}
      <div className="settings-context">
        <h3>{settingsContext[tab][0]}</h3>
        <p className="muted">{settingsContext[tab][1]}</p>
      </div>
      {["Zeitplan", "Aufbewahrung", "Benachrichtigungen"].includes(tab) && (
        <form className="settings-form" onSubmit={save}>
          {saveControls}
          <fieldset className="form-fields" disabled={busy || updating}>
            {tab === "Zeitplan" ? (
              <section>
                <fieldset className="settings-group">
                  <legend>Start und Zeitzone</legend>
                  <div className="form-grid">
                    <Field
                      label="Startzeit"
                      hint="Eigene Hostzeiten haben Vorrang. Änderungen gelten für noch nicht eingeplante Läufe; ein bereits eingeplanter Tageslauf wird nicht erneut gestartet."
                    >
                      <input
                        type="time"
                        required
                        value={value.schedule}
                        onChange={(e) => set("schedule", e.target.value)}
                      />
                    </Field>
                    <Field label="Zeitzone" error={timezoneError}>
                      <input
                        ref={timezoneInput}
                        name="timezone"
                        required
                        value={value.timezone}
                        onChange={(e) => set("timezone", e.target.value)}
                      />
                    </Field>
                  </div>
                </fieldset>
                <fieldset className="settings-group">
                  <legend>Ausführung</legend>
                  <div className="form-grid">
                    <Field
                      label="Parallele Aufträge"
                      hint="Ein neues Parallelitätslimit gilt für wartende Aufträge. Laufende Aufträge werden dafür nicht abgebrochen."
                    >
                      <input
                        type="number"
                        min={1}
                        max={16}
                        value={value.parallel}
                        onChange={(e) => set("parallel", +e.target.value)}
                      />
                    </Field>
                    <Field
                      label="Wiederholungen"
                      hint="Nur vorübergehende Verbindungsfehler bei Sicherung und Hostprüfung. 0 deaktiviert Wiederholungen; 1–5 zusätzliche Versuche mit wachsender Wartezeit, im selben Auftrag. Rechte-, Identitäts- und Inhaltsfehler werden nicht wiederholt."
                    >
                      <input
                        type="number"
                        min={0}
                        max={5}
                        value={value.retries}
                        onChange={(e) => set("retries", +e.target.value)}
                      />
                    </Field>
                  </div>
                </fieldset>
                <fieldset className="settings-group">
                  <legend>Aktualität</legend>
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
                </fieldset>
              </section>
            ) : tab === "Aufbewahrung" ? (
              <section>
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
                  Gezählt werden Zeiträume, in denen eine Sicherung vorhanden
                  ist. Ein Stand kann mehrere Regeln erfüllen; die Zahlen werden
                  nicht addiert. Stände außerhalb dieser Regeln werden bei der
                  täglichen Wartung gelöscht.
                </p>
                <div className="retention-protection">
                  <span className="count-badge success">Bleiben erhalten</span>
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
                        set("archive_days", e.target.checked ? archiveAfter : 0)
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
            ) : (
              <section>
                <fieldset className="settings-group">
                  <legend>Webhook</legend>
                  <Field label="Webhook-URL">
                    <input
                      type="url"
                      value={value.webhook}
                      onChange={(e) => set("webhook", e.target.value)}
                    />
                  </Field>
                </fieldset>
                <fieldset className="settings-group">
                  <legend>E-Mail (SMTP)</legend>
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
                </fieldset>
                <button
                  type="button"
                  className="secondary"
                  disabled={dirty || busy || testing}
                  title={dirty ? "Zuerst die Änderungen speichern" : undefined}
                  onClick={async () => {
                    setTesting(true);
                    setTestError("");
                    try {
                      await api("notifications/test", "POST", {});
                      notify("Testnachricht gesendet");
                    } catch (e) {
                      setTestError((e as Error).message);
                    } finally {
                      setTesting(false);
                    }
                  }}
                >
                  {testing ? "Testet …" : "Gespeicherte Verbindung testen"}
                </button>
                {testError && (
                  <p className="form-error" role="alert">
                    {testError}
                  </p>
                )}
                {dirty && (
                  <p className="field-hint">
                    Zum Testen zuerst die Änderungen speichern.
                  </p>
                )}
              </section>
            )}
          </fieldset>
        </form>
      )}
      {tab === "Zugriff" && (
        <div inert={updating}>
          <form className="settings-form access-policy" onSubmit={save}>
            {saveControls}
            <section>
              <h3>Anmeldung</h3>
              <p className="muted">Eine Anmeldung ist immer erforderlich.</p>
              <Field
                label="Sitzungsdauer in Tagen"
                hint="1 bis 365 Tage. Standard: 30 Tage. Gilt für neue Sitzungen, auch über einen Neustart hinweg."
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
      <div hidden={tab !== "Zertifikate"}>
        <div inert={updating}>
          <Certificates notify={notify} onDirtyChange={setCertificateDirty} />
        </div>
      </div>
      {tab === "Updates" && (
        <div className="updates-page">
          <Updates canInstall={!dirty && !certificateDirty} />
        </div>
      )}
      {tab === "Betrieb" && (
        <>
          <nav className="jump-nav" aria-label="Bereiche im Betrieb">
            {betriebAreas.map(([id, label, hint]) => (
              <button
                key={id}
                type="button"
                className="secondary"
                aria-label={label}
                onClick={() =>
                  document.getElementById(id)?.scrollIntoView({
                    behavior: "smooth",
                    block: "start",
                  })
                }
              >
                <span aria-hidden="true">{label}</span>
                <small aria-hidden="true">{hint}</small>
              </button>
            ))}
          </nav>
          <section id="betrieb-speicher" className="betrieb-section">
            <Storage notify={notify} />
          </section>
          <section id="betrieb-pruefung" className="betrieb-section">
            <DetailSection title="Systemprüfung">
              {systemError && (
                <LoadError message={systemError} retry={inspect} />
              )}
              {inspecting && <p role="status">Systemdaten werden geladen …</p>}
              {doctor != null && !systemError && !inspecting && (
                <SystemDiagnosis value={doctor} />
              )}
              <button
                className="secondary"
                onClick={async () => {
                  try {
                    const r = await api<{ indexed: number }>(
                      "reindex",
                      "POST",
                      {},
                    );
                    notify(r.indexed + " Sicherungen neu eingelesen");
                  } catch (e) {
                    notify((e as Error).message, true);
                  }
                }}
              >
                Sicherungsindex neu einlesen
              </button>
            </DetailSection>
          </section>
          <section id="betrieb-protokoll" className="betrieb-section">
            <DetailSection title="Aktivitätsprotokoll">
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
            </DetailSection>
          </section>
        </>
      )}
    </>
  );
}
