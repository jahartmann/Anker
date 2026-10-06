import { useEffect, useState } from "react";
import { api, date } from "../api";
import { Dialog, Field, LoadError } from "../components/shared";
import type { Notify } from "../components/shared";

type Point = { at: string; total: number; used: number; available: number };
type Volume = {
  id: string;
  mount: string;
  paths: string[];
  source: string;
  fs_type: string;
  total: number;
  used: number;
  available: number;
  reserved: number;
  used_percent: number;
  inodes: number;
  inodes_used: number;
  read_only: boolean;
  is_data: boolean;
  is_system: boolean;
  error?: string;
  history?: Point[];
  forecast: {
    status: string;
    message: string;
    growth_per_day: number;
    days_to_full?: number;
    full_at?: string;
    based_on_days: number;
  };
};
type Device = {
  name: string;
  type: string;
  size: number;
  fstype?: string;
  uuid?: string;
  mountpoints?: (string | null)[];
  children?: Device[];
};
type Report = {
  collected_at: string;
  data_path: string;
  environment: string;
  volumes: Volume[];
  devices: Device[];
  warnings: string[];
  demo?: boolean;
};
type Plan = {
  id: string;
  volume_id: string;
  mount: string;
  source: string;
  fs_type: string;
  environment: string;
  device_bytes: number;
  filesystem_bytes: number;
  can_grow: boolean;
  message: string;
  steps: { title: string; command?: string; explanation: string }[];
};
type Operation = {
  status: string;
  mount?: string;
  message?: string;
  started_at?: string;
  finished_at?: string;
  before_bytes?: number;
  after_bytes?: number;
};

const bytes = (n: number) => {
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n === 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  const index = Math.min(
    Math.floor(Math.log(n) / Math.log(1024)),
    units.length - 1,
  );
  return `${new Intl.NumberFormat("de-DE", { maximumFractionDigits: index > 0 ? 1 : 0 }).format(n / 1024 ** index)} ${units[index]}`;
};
const percent = (n: number) => Math.max(0, Math.min(100, Math.round(n)));
const shell = (s: string) => `'${s.replaceAll("'", "'\\''")}'`;

function Command({ value, notify }: { value: string; notify: Notify }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="storage-command">
      <code>{value}</code>
      <button
        className="text-button"
        type="button"
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value);
            setCopied(true);
          } catch {
            notify(
              "Kopieren nicht möglich. Den Befehl markieren oder die Anleitung herunterladen.",
              true,
            );
          }
        }}
      >
        {copied ? "Kopiert" : "Kopieren"}
      </button>
    </div>
  );
}
function downloadGuide(text: string) {
  const url = URL.createObjectURL(
    new Blob([text], { type: "text/plain;charset=utf-8" }),
  );
  const a = document.createElement("a");
  a.href = url;
  a.download = "anker-speicher-anleitung.txt";
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 30000);
}
function History({ volume }: { volume: Volume }) {
  const points = (volume.history || []).filter(
    (p) => p.total === volume.total && p.total > 0,
  );
  if (points.length < 2)
    return (
      <p className="field-hint">
        Der Verlauf erscheint nach weiteren Messungen.
      </p>
    );
  const start = new Date(points[0].at).getTime(),
    end = new Date(points.at(-1)!.at).getTime();
  if (end <= start) return null;
  const line = points
    .map(
      (p) =>
        `${8 + ((new Date(p.at).getTime() - start) / (end - start)) * 484},${84 - Math.max(0, Math.min(1, p.used / p.total)) * 76}`,
    )
    .join(" ");
  return (
    <div className="storage-history">
      <svg
        viewBox="0 0 500 94"
        role="img"
        aria-label={`Belegungsverlauf ${volume.mount} · ${date(points[0].at)} bis ${date(points.at(-1)!.at)}`}
      >
        <path className="storage-grid" d="M8 8H492 M8 46H492 M8 84H492" />
        <polyline points={line} fill="none" className="storage-line" />
      </svg>
      <div className="storage-history-caption">
        <span>{date(points[0].at)}</span>
        <span>Belegung · echte Tagesmessungen</span>
        <span>{date(points.at(-1)!.at)}</span>
      </div>
    </div>
  );
}
function VolumeRow({
  volume: v,
  running,
  onExpand,
}: {
  volume: Volume;
  running: boolean;
  onExpand: () => void;
}) {
  const usage = percent(v.used_percent),
    inodeUsage = v.inodes ? percent((100 * v.inodes_used) / v.inodes) : 0;
  const severity =
    usage >= 95 || inodeUsage >= 95 || v.available === 0
      ? "critical"
      : usage >= 85 || inodeUsage >= 85
        ? "attention"
        : "";
  return (
    <article className={`storage-volume ${severity}`}>
      <div className="section-heading">
        <div>
          <h3 className="mono">{v.mount}</h3>
          <p className="muted">
            {v.is_data && v.is_system
              ? "System und Anker-Ablage · ein gemeinsames Dateisystem"
              : v.is_data
                ? "Anker-Ablage"
                : v.is_system
                  ? "System"
                  : "Eingebundenes Laufwerk"}{" "}
            · {v.fs_type}
            {v.read_only ? " · schreibgeschützt" : ""}
          </p>
        </div>
        {(v.is_data || v.is_system) && (
          <button
            className="secondary"
            disabled={running || !!v.error}
            onClick={onExpand}
          >
            Erweitern
          </button>
        )}
      </div>
      {v.error ? (
        <p className="notice warning" role="alert">
          Belegung nicht lesbar: {v.error}
        </p>
      ) : (
        <>
          <div className="storage-usage">
            <strong>
              {bytes(v.used)} <span className="muted">belegt</span>
            </strong>
            <span>
              {bytes(v.available)} frei · {bytes(v.total)} gesamt
            </span>
          </div>
          <div
            className="storage-meter"
            role="progressbar"
            aria-label={`Belegung ${v.mount}`}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={usage}
          >
            <span style={{ width: `${usage}%` }} />
          </div>
          <div className="storage-forecast">
            {v.available === 0 ? (
              <strong>Kein verfügbarer Speicher mehr.</strong>
            ) : v.forecast.days_to_full !== undefined ? (
              <strong>
                Rechnerisch noch etwa {v.forecast.days_to_full} Tage · bis{" "}
                {date(v.forecast.full_at)}
              </strong>
            ) : (
              <strong>
                {v.forecast.status === "learning"
                  ? "Prognose lernt den Verlauf"
                  : v.forecast.status === "stable"
                    ? "Aktuell kein anhaltender Zuwachs"
                    : "Noch keine belastbare Terminprognose"}
              </strong>
            )}
            <p className="muted">
              {v.forecast.message}
              {v.forecast.growth_per_day > 0
                ? ` Nettozuwachs: ${bytes(v.forecast.growth_per_day)}/Tag.`
                : ""}
            </p>
          </div>
          <History volume={v} />
          {inodeUsage >= 85 && (
            <p className="notice warning" role="alert">
              {inodeUsage}% der Inodes sind belegt. Auch mit freien Gigabytes
              können neue Dateien scheitern.
            </p>
          )}
          <details className="storage-details">
            <summary>Technische Details</summary>
            <dl className="update-facts">
              <div>
                <dt>Gerät / Quelle</dt>
                <dd className="mono">{v.source}</dd>
              </div>
              <div>
                <dt>Zugeordnete Pfade</dt>
                <dd className="mono">{v.paths.join(", ")}</dd>
              </div>
              <div>
                <dt>Reservierter Speicher</dt>
                <dd>
                  {bytes(v.reserved)} · für normale Prozesse nicht verfügbar
                </dd>
              </div>
              <div>
                <dt>Inodes / Dateien</dt>
                <dd>
                  {v.inodes
                    ? `${new Intl.NumberFormat("de-DE").format(v.inodes_used)} von ${new Intl.NumberFormat("de-DE").format(v.inodes)} · ${inodeUsage}% belegt`
                    : "Vom Dateisystem nicht gemeldet"}
                </dd>
              </div>
              <div>
                <dt>Prognosebasis</dt>
                <dd>
                  {v.forecast.based_on_days
                    ? `${v.forecast.based_on_days} Tage · letzte 14 Tage`
                    : "Messreihe wird aufgebaut"}
                </dd>
              </div>
            </dl>
          </details>
        </>
      )}
    </article>
  );
}

function Expand({
  volume,
  notify,
  onClose,
  onStarted,
}: {
  volume: Volume;
  notify: Notify;
  onClose: () => void;
  onStarted: (state: Operation) => void;
}) {
  const [plan, setPlan] = useState<Plan | null>(null),
    [error, setError] = useState("");
  const [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [stale, setStale] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [ct, setCT] = useState(""),
    [mp, setMP] = useState("rootfs"),
    [extra, setExtra] = useState("50");
  async function inspect() {
    setLoading(true);
    setError("");
    setStale(false);
    setConfirmation("");
    try {
      setPlan(
        await api<Plan>("storage/plan", "POST", { volume_id: volume.id }),
      );
    } catch (e) {
      setError((e as Error).message);
      setStale(true);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    let active = true;
    api<Plan>("storage/plan", "POST", { volume_id: volume.id })
      .then((v) => {
        if (active) setPlan(v);
      })
      .catch((e) => {
        if (active) {
          setError(e.message);
          setStale(true);
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [volume.id]);
  const container = plan?.environment === "container:lxc";
  const validCT = /^\d+$/.test(ct) && +ct >= 100 && +ct <= 999999999;
  const validExtra = /^\d+$/.test(extra) && +extra > 0 && +extra <= 1048576;
  const hostCommand =
    validCT && validExtra ? `pct resize ${+ct} ${mp} +${+extra}G` : "";
  async function grow() {
    if (!plan || stale || confirmation !== plan.mount) return;
    setBusy(true);
    setError("");
    try {
      onStarted(
        await api<Operation>("storage/grow", "POST", {
          volume_id: volume.id,
          plan_id: plan.id,
          confirmation,
        }),
      );
    } catch (e) {
      setError((e as Error).message);
      setStale(true);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      title="Speicher erweitern"
      onClose={onClose}
      busy={loading || busy}
      wide
    >
      <p className="muted">
        Für <strong className="mono">{volume.mount}</strong> · {volume.fs_type}
      </p>
      {loading ? (
        <p role="status">Gerät und Dateisystem werden geprüft …</p>
      ) : (
        <>
          {error && (
            <p className="notice warning" role="alert">
              {error}
            </p>
          )}
          {plan && (
            <>
              <p>{plan.message}</p>
              {!!plan.device_bytes && (
                <dl className="update-facts">
                  <div>
                    <dt>Blockgerät</dt>
                    <dd>{bytes(plan.device_bytes)}</dd>
                  </div>
                  <div>
                    <dt>Dateisystem</dt>
                    <dd>{bytes(plan.filesystem_bytes)}</dd>
                  </div>
                </dl>
              )}
              {plan.steps?.map((s, i) => (
                <div className="storage-step" key={i}>
                  <h4>
                    {i + 1}. {s.title}
                  </h4>
                  <p className="muted">{s.explanation}</p>
                  {s.command && <Command value={s.command} notify={notify} />}
                </div>
              ))}
              {container && (
                <div className="storage-container-form">
                  <p className="muted">
                    Diese Werte im Proxmox-Interface nachsehen. Den Befehl auf
                    dem zugehörigen Proxmox-Host ausführen.
                  </p>
                  <div className="form-grid">
                    <Field label="Container-ID">
                      <input
                        type="number"
                        min={100}
                        max={999999999}
                        value={ct}
                        onChange={(e) => setCT(e.target.value)}
                      />
                    </Field>
                    <Field
                      label="Mountpoint bei Proxmox"
                      hint="rootfs oder der zur Ablage passende mp-Eintrag. Bindmounts separat auf dem Host erweitern."
                    >
                      <select
                        value={mp}
                        onChange={(e) => setMP(e.target.value)}
                      >
                        <option value="rootfs">rootfs</option>
                        {Array.from({ length: 256 }, (_, i) => (
                          <option value={`mp${i}`} key={i}>
                            mp{i}
                          </option>
                        ))}
                      </select>
                    </Field>
                    <Field label="Zusätzlicher Platz in GiB">
                      <input
                        type="number"
                        min={1}
                        max={1048576}
                        value={extra}
                        onChange={(e) => setExtra(e.target.value)}
                      />
                    </Field>
                  </div>
                  {hostCommand && (
                    <Command value={hostCommand} notify={notify} />
                  )}
                  <p className="field-hint">
                    Danach die Belegung aktualisieren. Freien Platz des
                    Proxmox-Speicherpools vorher auf dem Host prüfen.
                  </p>
                </div>
              )}
              {plan.can_grow && (
                <div className="storage-confirm">
                  <p>
                    Der vorhandene Inhalt bleibt erhalten. Anker lässt das
                    Dateisystem den bereits zugewiesenen zusätzlichen Platz
                    übernehmen. Vorher eine aktuelle Sicherung der Anker-Daten
                    sicherstellen.
                  </p>
                  <Field
                    label="Mountpoint bestätigen"
                    hint={`Zur Bestätigung ${plan.mount} exakt eingeben.`}
                  >
                    <input
                      value={confirmation}
                      disabled={busy || stale}
                      onChange={(e) => setConfirmation(e.target.value)}
                      autoComplete="off"
                      spellCheck={false}
                    />
                  </Field>
                </div>
              )}
            </>
          )}
        </>
      )}
      <footer className="dialog-footer">
        <button
          className="secondary"
          disabled={loading || busy}
          onClick={onClose}
        >
          Schließen
        </button>
        {!loading && (
          <button className="secondary" onClick={inspect} disabled={busy}>
            Plan erneut prüfen
          </button>
        )}
        {plan && !loading && (
          <button
            className="secondary"
            disabled={busy}
            onClick={() =>
              downloadGuide(
                [
                  `Anker · Speicher erweitern · ${plan.mount}`,
                  plan.message,
                  ...plan.steps.flatMap((s) => [
                    s.title,
                    s.explanation,
                    s.command || "",
                  ]),
                  hostCommand ? `Auf dem Proxmox-Host:\n${hostCommand}` : "",
                  "Anschließend Belegung in Anker aktualisieren.",
                ].join("\n\n"),
              )
            }
          >
            Anleitung herunterladen
          </button>
        )}
        {plan?.can_grow && (
          <button
            disabled={loading || busy || stale || confirmation !== plan.mount}
            onClick={grow}
          >
            {busy ? "Startet …" : "Platz übernehmen"}
          </button>
        )}
      </footer>
    </Dialog>
  );
}

function freeDevices(devices: Device[]): Device[] {
  return devices.flatMap((d) => {
    const children = freeDevices(d.children || []);
    const mounted =
      d.mountpoints?.some(Boolean) ||
      (d.children || []).some((c) => c.mountpoints?.some(Boolean));
    if (d.children?.length) return children;
    return !mounted &&
      ["disk", "part", "lvm"].includes(d.type) &&
      !d.name.startsWith("/dev/loop")
      ? [d]
      : [];
  });
}
function NewDrive({
  report,
  notify,
  onClose,
}: {
  report: Report;
  notify: Notify;
  onClose: () => void;
}) {
  const candidates = freeDevices(report.devices || []);
  const [name, setName] = useState(candidates[0]?.name || "");
  const drive = candidates.find((d) => d.name === name);
  const container = report.environment.startsWith("container:");
  const ready =
    !!drive?.uuid &&
    /^[a-zA-Z0-9-]+$/.test(drive.uuid) &&
    ["ext4", "xfs"].includes(drive.fstype || "");
  const verifyNewMount = drive?.uuid
    ? `test "$(findmnt -n -o UUID --mountpoint /mnt/anker-new)" = ${shell(drive.uuid)}`
    : "false";
  const verifyFinalMount = drive?.uuid
    ? `test "$(findmnt -n -o UUID --mountpoint /srv/anker)" = ${shell(drive.uuid)}`
    : "false";
  const steps =
    ready && drive
      ? [
          {
            title: "Vorhandenen Inhalt und Dateisystem prüfen",
            command: `sudo lsblk -f ${shell(drive.name)}\nsudo blkid ${shell(drive.name)}`,
            explanation:
              "Das Laufwerk muss für die Anker-Daten vorgesehen sein und genug freien Platz haben.",
          },
          {
            title: "Laufwerk vorübergehend einbinden",
            command: `test "$(sudo blkid -s UUID -o value ${shell(drive.name)})" = ${shell(drive.uuid!)} &&\nsudo mkdir -p /mnt/anker-new &&\nsudo mount -- ${shell(drive.name)} /mnt/anker-new &&\n${verifyNewMount}`,
            explanation:
              "Den Inhalt prüfen und die tatsächlich eingebundene Quelle mit dem gewählten Laufwerk vergleichen.",
          },
          {
            title: "Dienste stoppen und Daten übernehmen",
            command: `${verifyNewMount} &&\ntest -z "$(sudo find /mnt/anker-new -mindepth 1 -maxdepth 1 ! -name lost+found -print -quit)" &&\nsudo systemctl stop anker-updater anker &&\nsudo rsync -aHAX --numeric-ids /srv/anker/ /mnt/anker-new/`,
            explanation:
              "Das Ziel muss bis auf lost+found leer sein. Erst nach bestätigtem Mount und ohne laufendes Update ausführen. Beim Kopierfehler nicht zur Umschaltung weitergehen. rsync muss installiert sein. Vorhandene SFTP-Bindmounts vor dem Umzug lösen und danach neu einbinden.",
          },
          {
            title: "Dauerhaft einbinden",
            command: `sudoedit /etc/fstab`,
            explanation: `Im Editor den vorhandenen /srv/anker-Eintrag gezielt ersetzen oder ergänzen: UUID=${drive.uuid} /srv/anker ${drive.fstype} defaults 0 ${drive.fstype === "ext4" ? "2" : "0"}. UUID und Dateisystem vergleichen. Der bisherige Datenbestand bleibt am alten Ort als Rückfall erhalten.`,
          },
          {
            title: "Umschalten und prüfen",
            command: `sudo umount /mnt/anker-new &&\n(if mountpoint -q /srv/anker; then sudo umount /srv/anker; fi) &&\nsudo mount /srv/anker &&\n${verifyFinalMount} &&\nsudo systemctl start anker anker-updater`,
            explanation:
              "Die Dienste erst starten, wenn /srv/anker erfolgreich vom neuen Laufwerk eingebunden ist. Anmeldung und Sicherungen prüfen, danach erst eine Bereinigung des alten Bestands planen.",
          },
        ]
      : [];
  return (
    <Dialog title="Neues Laufwerk einbinden" onClose={onClose} wide>
      {container ? (
        <p>
          Zusätzlichen Speicher in der Containerverwaltung als Mountpoint an den
          Anker-Container hängen (bei LXC im Proxmox-Interface). Für einen Umzug
          der Ablage zuerst vorübergehend einbinden, Anker stoppen und den
          Datenbestand übernehmen. Den neuen Speicher anschließend unter{" "}
          <code>/srv/anker</code> oder einem Unterordner einbinden und die
          Anzeige aktualisieren.
        </p>
      ) : (
        <>
          {candidates.length ? (
            <Field label="Noch nicht eingebundenes Gerät">
              <select value={name} onChange={(e) => setName(e.target.value)}>
                {candidates.map((d) => (
                  <option key={d.name} value={d.name}>
                    {d.name} · {bytes(d.size)} ·{" "}
                    {d.fstype || "ohne erkanntes Dateisystem"}
                  </option>
                ))}
              </select>
            </Field>
          ) : (
            <p>
              Kein zusätzliches, ungemountetes Blockgerät erkannt. Laufwerk in
              Proxmox hinzufügen und die Belegung erneut aktualisieren.
            </p>
          )}
          {drive && !ready && (
            <p className="notice warning">
              Dieses Gerät hat noch kein unterstütztes, eindeutig erkanntes
              Dateisystem. Es muss zuerst bewusst eingerichtet werden; dabei
              vorhandene Daten und Partitionen prüfen. Danach Anker
              aktualisieren.
            </p>
          )}
          {!!steps.length && (
            <>
              <p className="muted">
                Anleitung für den Administrator auf der Anker-Maschine. Jeden
                Schritt einzeln prüfen. Anker führt diesen Umzug nicht
                automatisch aus.
              </p>
              {steps.map((s, i) => (
                <div className="storage-step" key={s.title}>
                  <h4>
                    {i + 1}. {s.title}
                  </h4>
                  <p className="muted">{s.explanation}</p>
                  <Command value={s.command} notify={notify} />
                </div>
              ))}
            </>
          )}
        </>
      )}
      <footer className="dialog-footer">
        <button className="secondary" onClick={onClose}>
          Schließen
        </button>
        {!!steps.length && (
          <button
            onClick={() =>
              downloadGuide(
                [
                  `Anker · Ablage umziehen · ${name}`,
                  ...steps.flatMap((s) => [s.title, s.explanation, s.command]),
                ].join("\n\n"),
              )
            }
          >
            Anleitung herunterladen
          </button>
        )}
      </footer>
    </Dialog>
  );
}

export default function Storage({ notify }: { notify: Notify }) {
  const [report, setReport] = useState<Report | null>(null),
    [operation, setOperation] = useState<Operation | null>(null);
  const [error, setError] = useState(""),
    [toolsError, setToolsError] = useState("");
  const [reload, setReload] = useState(0),
    [refreshing, setRefreshing] = useState(false);
  const [selected, setSelected] = useState<Volume | null>(null),
    [newDrive, setNewDrive] = useState(false);
  useEffect(() => {
    let active = true;
    setRefreshing(true);
    api<Report>("storage")
      .then(async (value) => {
        if (!active) return;
        setReport(value);
        setError("");
        if (!value.demo) {
          try {
            const state = await api<Operation>("storage/state");
            if (active) {
              setOperation(state);
              setToolsError("");
            }
          } catch (e) {
            if (active) setToolsError((e as Error).message);
          }
        }
      })
      .catch((e) => {
        if (active) setError(e.message);
      })
      .finally(() => {
        if (active) setRefreshing(false);
      });
    return () => {
      active = false;
    };
  }, [reload]);
  useEffect(() => {
    if (operation?.status !== "running") return;
    let active = true;
    const timer = setInterval(async () => {
      try {
        const next = await api<Operation>("storage/state");
        if (!active) return;
        setOperation(next);
        setToolsError("");
        if (next.status !== "running") {
          clearInterval(timer);
          setReload((n) => n + 1);
        }
      } catch (e) {
        if (active)
          setToolsError(
            `Status nicht erreichbar; die Erweiterung kann weiterlaufen. ${(e as Error).message}`,
          );
      }
    }, 2000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [operation?.status]);
  if (!report)
    return error ? (
      <LoadError message={error} retry={() => setReload((n) => n + 1)} />
    ) : (
      <p className="muted" role="status">
        Dateisysteme und Laufwerke werden gelesen …
      </p>
    );
  const running = operation?.status === "running";
  return (
    <section className="storage-section">
      <div className="section-heading">
        <div>
          <h3>Speicher auf dem Anker-Server</h3>
          <p className="muted">
            System, Backup-Ablage und eingebundene Laufwerke.
          </p>
        </div>
        {!report.demo && (
          <button
            className="secondary"
            disabled={refreshing}
            onClick={() => setReload((n) => n + 1)}
          >
            {refreshing ? "Aktualisiert …" : "Belegung aktualisieren"}
          </button>
        )}
      </div>
      {report.demo ? (
        <p className="notice">{report.warnings?.join(" ")}</p>
      ) : (
        <>
          <p className="storage-updated muted">
            {report.environment.startsWith("container:")
              ? "Container"
              : report.environment.startsWith("vm:")
                ? "Virtuelle Maschine"
                : report.environment === "physical"
                  ? "Physischer Server"
                  : "Umgebung nicht eindeutig erkannt"}{" "}
            · Stand {date(report.collected_at)}
          </p>
          {error && (
            <div className="notice warning" role="alert">
              Aktualisierung fehlgeschlagen. Die angezeigten Werte stammen vom
              letzten erfolgreichen Abruf.
              <LoadError
                message={error}
                retry={() => setReload((n) => n + 1)}
              />
            </div>
          )}
          {report.warnings?.map((warning, i) => (
            <p className="notice" key={i}>
              {warning}
            </p>
          ))}
          {operation && operation.status !== "idle" && (
            <p
              className={`notice ${["failed", "interrupted"].includes(operation.status) ? "warning" : ""}`}
              role={operation.status === "running" ? "status" : "alert"}
            >
              {operation.status === "running"
                ? `Erweiterung für ${operation.mount} läuft. Sie läuft auch beim Schließen dieser Ansicht weiter.`
                : operation.message}
              {operation.status === "successful" && operation.after_bytes
                ? ` Neue Dateisystemgröße: ${bytes(operation.after_bytes)}.`
                : ""}
            </p>
          )}
          {toolsError && (
            <p className="notice warning" role="alert">
              {toolsError}
            </p>
          )}
          {report.volumes.map((v) => (
            <VolumeRow
              key={v.id}
              volume={v}
              running={running}
              onExpand={() => setSelected(v)}
            />
          ))}
          <section className="storage-add">
            <div>
              <h3>Zusätzlichen Speicher hinzufügen</h3>
              <p className="muted">
                Ein neues Laufwerk einbinden oder die Ablage auf größeren
                Speicher umziehen.
              </p>
            </div>
            <button
              className="secondary"
              disabled={running}
              onClick={() => setNewDrive(true)}
            >
              Neues Laufwerk einbinden
            </button>
          </section>
          <p className="field-hint">
            Anker misst einmal pro Stunde und hält bis zu 90 Tage Verlauf.
            Gemeinsame Dateisysteme werden einmal angezeigt. Die Belegung
            umfasst auch andere Dateien auf demselben Laufwerk.
          </p>
        </>
      )}
      {selected && (
        <Expand
          volume={selected}
          notify={notify}
          onClose={() => setSelected(null)}
          onStarted={(state) => {
            setOperation(state);
            setSelected(null);
            notify("Speichererweiterung gestartet");
          }}
        />
      )}
      {newDrive && (
        <NewDrive
          report={report}
          notify={notify}
          onClose={() => setNewDrive(false)}
        />
      )}
    </section>
  );
}
