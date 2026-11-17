import { useRef, useState } from "react";
import type { FormEvent } from "react";
import { ChevronDown } from "lucide-react";
import { api } from "../api";
import type { Host, HostIdentity } from "../api";
import { Dialog, Field } from "./shared";
import type { Notify } from "./shared";

export function HostForm({
  host,
  automatic = !host,
  initialEndpoint,
  onClose,
  onSaved,
  notify,
}: {
  host?: Host;
  automatic?: boolean;
  initialEndpoint?: { address: string; port: number };
  onClose: () => void;
  onSaved: () => void;
  notify: Notify;
}) {
  const [method, setMethod] = useState(automatic ? "automatic" : "manual");
  const props = { host, onClose, onSaved, notify };
  return method === "automatic" ? (
    <HostConnection
      {...props}
      initialEndpoint={initialEndpoint}
      onManual={() => setMethod("manual")}
    />
  ) : (
    <ManualHostForm {...props} onAutomatic={() => setMethod("automatic")} />
  );
}

function HostConnection({
  host,
  initialEndpoint,
  onClose,
  onSaved,
  notify,
  onManual,
}: {
  host?: Host;
  initialEndpoint?: { address: string; port: number };
  onClose: () => void;
  onSaved: () => void;
  notify: Notify;
  onManual: () => void;
}) {
  const [value, setValue] = useState<Host>(() => ({
    id: "",
    name: "",
    address: initialEndpoint?.address || "",
    group: "",
    cluster_id: "",
    enabled: true,
    schedule: "",
    extra_paths: [],
    ...host,
    ssh_user: "anker",
    restore_ssh_user: "anker-restore",
    ssh_port: host?.ssh_port || initialEndpoint?.port || 22,
    key_path: "/etc/anker/keys/backup",
    restore_key_path: "/etc/anker/keys/restore",
    known_hosts_path: "/etc/anker/known_hosts",
  }));
  const [username, setUsername] = useState("root");
  const [password, setPassword] = useState("");
  const [identity, setIdentity] = useState<HostIdentity | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [advanced, setAdvanced] = useState(false);
  const [busy, setBusy] = useState<"inspect" | "enroll" | null>(null);
  const [error, setError] = useState("");
  const pending = useRef(false);
  const invalidate = () => {
    setIdentity(null);
    setConfirmed(false);
  };
  const set = (key: keyof Host, v: unknown) => {
    invalidate();
    setValue((old) => ({ ...old, [key]: v }));
  };
  const close = () => {
    if (pending.current) return;
    setPassword("");
    onClose();
  };
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (pending.current || (identity && (!confirmed || identity.changed)))
      return;
    pending.current = true;
    setBusy(identity ? "enroll" : "inspect");
    setError("");
    try {
      if (!identity) {
        const result = await api<HostIdentity>(
          "hosts/connection/inspect",
          "POST",
          {
            address: value.address.trim(),
            port: value.ssh_port,
          },
        );
        setIdentity(result);
        setConfirmed(false);
      } else {
        await api<Host>("hosts/connection/enroll", "POST", {
          host: {
            id: value.id,
            name: value.name,
            address: identity.address,
            group: value.group,
            cluster_id: value.cluster_id,
            ssh_user: value.ssh_user,
            ssh_port: identity.port,
            key_path: value.key_path,
            restore_ssh_user: value.restore_ssh_user,
            restore_key_path: value.restore_key_path,
            known_hosts_path: value.known_hosts_path,
            enabled: value.enabled,
            schedule: value.schedule,
            extra_paths: value.extra_paths,
          },
          username: username.trim(),
          password,
          fingerprint: identity.fingerprint,
          confirmed: true,
        });
        setPassword("");
        notify("Host angebunden");
        onSaved();
        onClose();
      }
    } catch (e) {
      const message = (e as Error).message;
      setError(password ? message.split(password).join("[entfernt]") : message);
      if (identity) {
        setPassword("");
        invalidate();
      }
    } finally {
      pending.current = false;
      setBusy(null);
    }
  }
  return (
    <Dialog
      topic="server"
      title={host ? "Verbindung einrichten" : "Host hinzufügen"}
      onClose={close}
      busy={!!busy}
    >
      <form onSubmit={submit}>
        <p className="dialog-intro">
          Anker richtet die SSH-Schlüssel und Hosthelfer automatisch ein. Das
          Passwort wird einmalig zur Einrichtung verwendet und nicht
          gespeichert.
        </p>
        {identity ? (
          <section className="host-identity" aria-label="SSH-Identität">
            <h3>Hostidentität bestätigen</h3>
            <dl>
              <dt>Adresse</dt>
              <dd className="mono">
                {identity.address}:{identity.port}
              </dd>
              <dt>Schlüsseltyp</dt>
              <dd className="mono">{identity.key_type}</dd>
              <dt>Fingerprint</dt>
              <dd className="mono">{identity.fingerprint}</dd>
            </dl>
            {identity.changed ? (
              <p className="notice warning" role="alert">
                Der Hostschlüssel hat sich geändert, die Anbindung ist deshalb
                gesperrt. Identität am Host prüfen und den neuen Schlüssel über
                die Hostschlüsselprüfung übernehmen.
              </p>
            ) : (
              <>
                <p className="muted">
                  {identity.known
                    ? "Bekannter Hostschlüssel stimmt überein."
                    : "Noch nicht bekannt. Die Identität wurde noch nicht verifiziert."}
                </p>
                <p className="notice">
                  Vergleiche den Fingerprint an der Proxmox-Konsole. Das
                  Passwort wird erst nach deiner Bestätigung gesendet.
                </p>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={confirmed}
                    disabled={!!busy}
                    onChange={(e) => setConfirmed(e.target.checked)}
                  />
                  Fingerprint geprüft und bestätigt
                </label>
              </>
            )}
            <button
              type="button"
              className="text-button"
              disabled={!!busy}
              onClick={invalidate}
            >
              Angaben ändern
            </button>
          </section>
        ) : (
          <>
            <Field label="Adresse">
              <input
                required
                autoComplete="off"
                disabled={!!busy}
                value={value.address}
                onChange={(e) => set("address", e.target.value)}
                placeholder="IP-Adresse oder DNS-Name"
              />
            </Field>
            <div className="form-grid">
              <Field
                label="SSH-Benutzer"
                hint="root oder ein Benutzer mit sudo-Rechten."
              >
                <input
                  required
                  autoComplete="off"
                  disabled={!!busy}
                  value={username}
                  onChange={(e) => {
                    invalidate();
                    setUsername(e.target.value);
                  }}
                />
              </Field>
              <Field label="SSH-Passwort">
                <input
                  type="password"
                  required
                  autoComplete="new-password"
                  disabled={!!busy}
                  value={password}
                  onChange={(e) => {
                    invalidate();
                    setPassword(e.target.value);
                  }}
                />
              </Field>
            </div>
            <button
              type="button"
              className="disclosure"
              aria-expanded={advanced}
              disabled={!!busy}
              onClick={() => setAdvanced(!advanced)}
            >
              <ChevronDown size={14} className={advanced ? "rotated" : ""} />
              Weitere Angaben
            </button>
            {advanced && (
              <div className="advanced">
                <div className="form-grid">
                  <Field
                    label="Hostname"
                    hint="Optional; wird vom Host übernommen."
                  >
                    <input
                      disabled={!!busy}
                      value={value.name}
                      onChange={(e) => set("name", e.target.value)}
                    />
                  </Field>
                  <Field label="SSH-Port">
                    <input
                      type="number"
                      required
                      min={1}
                      max={65535}
                      disabled={!!busy}
                      value={value.ssh_port}
                      onChange={(e) => set("ssh_port", +e.target.value)}
                    />
                  </Field>
                  <Field label="Gruppe">
                    <input
                      disabled={!!busy}
                      value={value.group}
                      onChange={(e) => set("group", e.target.value)}
                    />
                  </Field>
                  <Field
                    label="Cluster"
                    hint="Für Standalone-Hosts leer lassen."
                  >
                    <input
                      disabled={!!busy}
                      value={value.cluster_id}
                      onChange={(e) => set("cluster_id", e.target.value)}
                    />
                  </Field>
                </div>
                <Field
                  label="Startzeit"
                  hint="Leer übernimmt den gemeinsamen Zeitplan."
                >
                  <input
                    type="time"
                    disabled={!!busy}
                    value={value.schedule}
                    onChange={(e) => set("schedule", e.target.value)}
                  />
                </Field>
                <label className="check">
                  <input
                    type="checkbox"
                    disabled={!!busy}
                    checked={value.enabled}
                    onChange={(e) => set("enabled", e.target.checked)}
                  />
                  Automatisch sichern
                </label>
              </div>
            )}
            <button
              type="button"
              className="text-button host-method"
              disabled={!!busy}
              onClick={() => {
                setPassword("");
                onManual();
              }}
            >
              Manuell einrichten
            </button>
          </>
        )}
        {error && (
          <p className="notice warning" role="alert">
            {error}
          </p>
        )}
        {busy && (
          <p className="muted" role="status">
            {busy === "inspect"
              ? "SSH-Identität wird ohne Passwort geprüft …"
              : "Schlüssel und Hosthelfer werden eingerichtet und beide Zugänge geprüft. Das kann einen Moment dauern …"}
          </p>
        )}
        <footer className="dialog-footer">
          <button
            type="button"
            className="secondary"
            disabled={!!busy}
            onClick={close}
          >
            Abbrechen
          </button>
          <button
            disabled={
              !!busy || (!!identity && (identity.changed || !confirmed))
            }
          >
            {busy
              ? busy === "inspect"
                ? "Prüft …"
                : "Bindet an …"
              : identity
                ? "Host anbinden"
                : "Identität prüfen"}
          </button>
        </footer>
      </form>
    </Dialog>
  );
}

function ManualHostForm({
  host,
  onClose,
  onSaved,
  notify,
  onAutomatic,
}: {
  host?: Host;
  onClose: () => void;
  onSaved: () => void;
  notify: Notify;
  onAutomatic: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [advanced, setAdvanced] = useState(!!host);
  const [value, setValue] = useState<Host>(
    host || {
      id: "",
      name: "",
      address: "",
      group: "",
      cluster_id: "",
      ssh_user: "anker",
      ssh_port: 22,
      key_path: "/etc/anker/keys/backup",
      known_hosts_path: "/etc/anker/known_hosts",
      enabled: true,
      schedule: "",
      extra_paths: [],
    },
  );
  const set = (key: keyof Host, v: unknown) =>
    setValue((old) => ({ ...old, [key]: v }));
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await api("hosts", "POST", value);
      notify("Host gespeichert");
      onSaved();
      onClose();
    } catch (e) {
      notify((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      topic="server"
      title={host ? "Host bearbeiten" : "Host hinzufügen"}
      onClose={onClose}
      busy={busy}
    >
      <form onSubmit={submit}>
        <p className="dialog-intro">
          Anker verbindet sich über SSH. Der verifizierte Hostschlüssel und das
          Sicherungsprofil bestimmen den Zugriff.
        </p>
        <button
          type="button"
          className="text-button host-method"
          disabled={busy}
          onClick={onAutomatic}
        >
          Automatisch einrichten
        </button>
        <div className="form-grid">
          <Field label="Hostname">
            <input
              required
              autoComplete="off"
              value={value.name}
              onChange={(e) => set("name", e.target.value)}
              placeholder="Hostname des PVE- oder PBS-Hosts"
            />
          </Field>
          <Field label="Adresse">
            <input
              required
              value={value.address}
              onChange={(e) => set("address", e.target.value)}
              placeholder="IP-Adresse oder DNS-Name"
            />
          </Field>
          <Field label="Gruppe">
            <input
              value={value.group}
              onChange={(e) => set("group", e.target.value)}
              placeholder="Standort oder Gruppe"
            />
          </Field>
          <Field label="Cluster" hint="Für Standalone-Hosts leer lassen.">
            <input
              value={value.cluster_id}
              onChange={(e) => set("cluster_id", e.target.value)}
            />
          </Field>
        </div>
        <button
          className="disclosure"
          type="button"
          onClick={() => setAdvanced(!advanced)}
        >
          <ChevronDown size={14} className={advanced ? "rotated" : ""} />
          SSH und Sicherungsprofil
        </button>
        {advanced && (
          <div className="advanced">
            <div className="form-grid">
              <Field label="SSH-Benutzer">
                <input
                  value={value.ssh_user}
                  onChange={(e) => set("ssh_user", e.target.value)}
                />
              </Field>
              <Field label="SSH-Port">
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={value.ssh_port}
                  onChange={(e) => set("ssh_port", +e.target.value)}
                />
              </Field>
            </div>
            <Field
              label="Privater SSH-Schlüssel"
              hint="Geschützter Dateipfad auf dem Anker-Server."
            >
              <input
                value={value.key_path}
                onChange={(e) => set("key_path", e.target.value)}
                placeholder="/etc/anker/keys/backup"
              />
            </Field>
            <Field
              label="Wiederherstellungsschlüssel"
              hint="Optionaler, separat berechtigter SSH-Schlüssel. Der Sicherungsschlüssel kann nur lesen."
            >
              <input
                value={value.restore_key_path || ""}
                onChange={(e) => set("restore_key_path", e.target.value)}
                placeholder="/etc/anker/keys/restore"
              />
            </Field>
            <Field label="Wiederherstellungsbenutzer">
              <input
                value={value.restore_ssh_user || "anker-restore"}
                onChange={(e) => set("restore_ssh_user", e.target.value)}
              />
            </Field>
            <Field label="Verifizierte Hostschlüssel">
              <input
                value={value.known_hosts_path}
                onChange={(e) => set("known_hosts_path", e.target.value)}
                placeholder="/etc/anker/known_hosts"
              />
            </Field>
            <Field
              label="Zusätzliche Pflichtpfade"
              hint="Eine Zeile je Pfad. Diese Pfade müssen im Profil auf dem Host zugelassen sein."
            >
              <textarea
                rows={3}
                value={(value.extra_paths || []).join("\n")}
                onChange={(e) =>
                  set("extra_paths", e.target.value.split("\n").filter(Boolean))
                }
              />
            </Field>
            <Field
              label="Startzeit"
              hint="Leer übernimmt den gemeinsamen Zeitplan."
            >
              <input
                type="time"
                value={value.schedule}
                onChange={(e) => set("schedule", e.target.value)}
              />
            </Field>
            <label className="check">
              <input
                type="checkbox"
                checked={value.enabled}
                onChange={(e) => set("enabled", e.target.checked)}
              />
              Automatisch sichern
            </label>
          </div>
        )}
        <footer className="dialog-footer">
          <button
            type="button"
            className="secondary"
            disabled={busy}
            onClick={onClose}
          >
            Abbrechen
          </button>
          <button disabled={busy}>
            {busy ? "Speichert …" : "Host speichern"}
          </button>
        </footer>
      </form>
    </Dialog>
  );
}
