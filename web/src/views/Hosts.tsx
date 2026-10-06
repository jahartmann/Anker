import { useState } from "react";
import type { FormEvent } from "react";
import { ArrowLeft, ChevronDown, Search, ChevronRight } from "lucide-react";
import { api, bytes, date, hostState } from "../api";
import type { Host, Status } from "../api";
import { Dialog, Empty, Field, Heading } from "../components/shared";
import type { Notify } from "../components/shared";
import { BackupTable } from "./Backups";
export function HostForm({
  host,
  onClose,
  onSaved,
  notify,
}: {
  host?: Host;
  onClose: () => void;
  onSaved: () => void;
  notify: Notify;
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
      title={host ? "Host bearbeiten" : "Host hinzufügen"}
      onClose={onClose}
    >
      <form onSubmit={submit}>
        <p className="dialog-intro">
          Anker verbindet sich über SSH. Der verifizierte Hostschlüssel und das
          Sicherungsprofil bestimmen den Zugriff.
        </p>
        <div className="form-grid">
          <Field label="Hostname">
            <input
              required
              autoComplete="off"
              value={value.name}
              onChange={(e) => set("name", e.target.value)}
              placeholder="Hostname des Proxmox-Hosts"
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
          <button type="button" className="secondary" onClick={onClose}>
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
export default function Hosts({
  status,
  notify,
  refresh,
  canEdit,
  canDownload = false,
  canOperate = canEdit,
  openBackup,
  onRestore,
}: {
  status: Status;
  notify: Notify;
  refresh: () => void;
  canEdit: boolean;
  canDownload?: boolean;
  canOperate?: boolean;
  openBackup: (id: string) => void;
  onRestore: (id: string) => void;
}) {
  const [query, setQuery] = useState("");
  const [group, setGroup] = useState("");
  const [selected, setSelected] = useState("");
  const [tab, setTab] = useState("Übersicht");
  const [form, setForm] = useState<Host | "new" | null>(null);
  const [removing, setRemoving] = useState(false);
  const [removeName, setRemoveName] = useState("");
  const host = status.hosts.find((h) => h.id === selected);
  const groups = Array.from(
    new Set(status.hosts.map((h) => h.group).filter(Boolean)),
  ).sort();
  const hosts = status.hosts.filter(
    (h) =>
      (h.name + " " + h.address).toLowerCase().includes(query.toLowerCase()) &&
      (!group || h.group === group),
  );
  const saved = status.hosts.filter(
    (h) => hostState(h, status).tone === "success",
  ).length;
  async function action(path: string, message: string) {
    try {
      await api(path, "POST", {});
      notify(message);
      refresh();
    } catch (e) {
      notify((e as Error).message, true);
    }
  }
  return (
    <>
      {host ? (
        <>
          <button className="back-button" onClick={() => setSelected("")}>
            <ArrowLeft size={14} />
            Alle Hosts
          </button>
          <Heading
            title={host.name}
            description={host.address + (host.group ? " · " + host.group : "")}
            action={
              canOperate && (
                <>
                  <button
                    className="secondary"
                    onClick={() =>
                      action(
                        "hosts/" + host.id + "/probe",
                        "Hostprüfung gestartet",
                      )
                    }
                  >
                    Verbindung prüfen
                  </button>
                  <button
                    onClick={() =>
                      action(
                        "hosts/" + host.id + "/backup",
                        "Sicherung gestartet",
                      )
                    }
                  >
                    Jetzt sichern
                  </button>
                </>
              )
            }
          />
          <div className="tabs">
            {["Übersicht", "Sicherungen", "Inventar", "Einstellungen"].map(
              (v) => (
                <button
                  key={v}
                  className={tab === v ? "selected" : ""}
                  onClick={() => setTab(v)}
                >
                  {v}
                </button>
              ),
            )}
          </div>
          {tab === "Übersicht" && (
            <div className="detail-section">
              <dl className="properties">
                <dt>Status</dt>
                <dd className={hostState(host, status).tone}>
                  {hostState(host, status).text}
                </dd>
                <dt>Letzte Sicherung</dt>
                <dd>{date(hostState(host, status).backup?.created_at)}</dd>
                <dt>Proxmox</dt>
                <dd>{host.inventory?.pve_version || "Noch nicht geprüft"}</dd>
                <dt>Cluster</dt>
                <dd>
                  {host.inventory?.cluster_id ||
                    host.cluster_id ||
                    "Standalone"}
                </dd>
                <dt>Automatische Sicherung</dt>
                <dd>
                  {host.enabled
                    ? host.schedule || "Gemeinsamer Zeitplan"
                    : "Ausgeschaltet"}
                </dd>
                <dt>Letzte Hostprüfung</dt>
                <dd>{date(host.last_probe)}</dd>
              </dl>
              {host.probe_error && (
                <p className="notice warning">{host.probe_error}</p>
              )}
              <div className="section-rule" />
              <h3>Wiederherstellung vorbereiten</h3>
              <p className="muted">
                Eine Sicherung auswählen, das Ziel prüfen und die Unterschiede
                vor der Übernahme ansehen.
              </p>
              {canOperate && (
                <button
                  className="secondary"
                  disabled={!status.backups.some((b) => b.host_id === host.id)}
                  onClick={() =>
                    onRestore(
                      status.backups.find((b) => b.host_id === host.id)?.id ||
                        "",
                    )
                  }
                >
                  Wiederherstellung planen
                </button>
              )}
            </div>
          )}
          {tab === "Sicherungen" && (
            <BackupTable
              backups={status.backups.filter((b) => b.host_id === host.id)}
              notify={notify}
              refresh={refresh}
              openBackup={openBackup}
              canEdit={canOperate}
              canDownload={canDownload}
              onRestore={onRestore}
            />
          )}{" "}
          {tab === "Inventar" &&
            (host.inventory ? (
              <div className="detail-section">
                <dl className="properties">
                  <dt>Proxmox</dt>
                  <dd>{host.inventory.pve_version}</dd>
                  <dt>Debian</dt>
                  <dd>{host.inventory.debian}</dd>
                  <dt>Kernel</dt>
                  <dd className="mono">{host.inventory.kernel}</dd>
                  <dt>Bootmodus</dt>
                  <dd>{host.inventory.boot_mode}</dd>
                </dl>
                <h3>Netzwerk</h3>
                <div className="table-scroll">
                  <table>
                    <thead>
                      <tr>
                        <th>Schnittstelle</th>
                        <th>MAC-Adresse</th>
                        <th>PCI-Pfad</th>
                      </tr>
                    </thead>
                    <tbody>
                      {host.inventory.interfaces.map((p) => (
                        <tr key={p.name}>
                          <td className="mono">{p.name}</td>
                          <td className="mono muted">{p.mac}</td>
                          <td className="mono muted">{p.pci || "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <h3>Datenträger</h3>
                <div className="table-scroll">
                  <table>
                    <thead>
                      <tr>
                        <th>Gerät</th>
                        <th>Kennung</th>
                        <th>Größe</th>
                        <th>Mount</th>
                      </tr>
                    </thead>
                    <tbody>
                      {host.inventory.disks.map((d, i) => (
                        <tr key={i}>
                          <td className="mono">{d.name}</td>
                          <td className="mono">{d.id}</td>
                          <td>{bytes(d.size)}</td>
                          <td>{d.mount || "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ) : (
              <Empty title="Noch kein Inventar">
                Mit „Verbindung prüfen“ liest Anker den aktuellen Hostzustand.
              </Empty>
            ))}
          {tab === "Einstellungen" && (
            <div className="detail-section">
              <dl className="properties">
                <dt>SSH-Benutzer</dt>
                <dd>{host.ssh_user}</dd>
                <dt>SSH-Port</dt>
                <dd>{host.ssh_port}</dd>
                <dt>Pflichtpfade</dt>
                <dd>
                  /etc
                  {host.extra_paths?.map((p) => (
                    <div key={p}>{p}</div>
                  ))}
                </dd>
              </dl>
              {canEdit && (
                <div className="inline-form">
                  <button className="secondary" onClick={() => setForm(host)}>
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
                <button onClick={() => setForm("new")}>Host hinzufügen</button>
              )
            }
          />
          <div className="toolbar">
            <div className="search-field">
              <Search size={15} aria-hidden="true" />
              <input
                className="search"
                aria-label="Hosts durchsuchen"
                placeholder="Hosts durchsuchen"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
            <select
              aria-label="Gruppe filtern"
              value={group}
              onChange={(e) => setGroup(e.target.value)}
            >
              <option value="">Alle Gruppen</option>
              {groups.map((g) => (
                <option key={g}>{g}</option>
              ))}
            </select>
          </div>
          {hosts.length ? (
            <div className="table-scroll">
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
                  {hosts.map((h) => {
                    const state = hostState(h, status);
                    return (
                      <tr key={h.id}>
                        <td>
                          <button
                            className="name-link"
                            onClick={() => {
                              setSelected(h.id);
                              setTab("Übersicht");
                            }}
                          >
                            {h.name}
                          </button>
                          <div className="secondary-line mono">{h.address}</div>
                        </td>
                        <td>{h.group || "Standalone"}</td>
                        <td className="date">
                          {date(state.backup?.created_at)}
                        </td>
                        <td>
                          <span className={"state " + state.tone}>
                            {state.text}
                          </span>
                        </td>
                        <td className="right">
                          <button
                            className="text-button"
                            onClick={() => {
                              setSelected(h.id);
                              setTab("Übersicht");
                            }}
                          >
                            Öffnen <ChevronRight size={14} aria-hidden="true" />
                          </button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty
              title={
                query || group ? "Keine passenden Hosts" : "Noch keine Hosts"
              }
            >
              {query || group
                ? "Suche oder Filter ändern."
                : "Mit „Host hinzufügen“ richtest du die erste Verbindung ein."}
            </Empty>
          )}
          <p className="table-footnote">
            {status.hosts.length} Hosts · {saved} gesichert ·{" "}
            {status.hosts.filter((h) => h.enabled).length - saved} mit
            Handlungsbedarf
            {status.hosts.some((h) => !h.enabled) &&
              " · " +
                status.hosts.filter((h) => !h.enabled).length +
                " pausiert"}
          </p>
        </>
      )}
      {removing && host && (
        <Dialog title="Host entfernen" onClose={() => setRemoving(false)}>
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
      {form && (
        <HostForm
          host={form === "new" ? undefined : form}
          onClose={() => setForm(null)}
          onSaved={refresh}
          notify={notify}
        />
      )}
    </>
  );
}
