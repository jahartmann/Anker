import { useEffect, useRef, useState } from "react";
import { api } from "../api";
import type { Host, HostIdentity } from "../api";
import { Dialog, Field } from "../components/shared";
import type { Notify } from "../components/shared";

type Config = {
  ranges: string[];
  web_port: number;
  pbs_port?: number;
  ssh_port: number;
  parallel?: number;
  use_cluster_hints?: boolean;
};
type Found = {
  address: string;
  name: string;
  version: string;
  ssh_reachable: boolean;
  kind?: "pve" | "pbs";
  web_port?: number;
  source?: string;
  services?: Found[];
};
type Scan = {
  status: string;
  total: number;
  checked: number;
  results: Found[];
  config: Config;
  error?: string;
  phase?: string;
  tcp_checked?: number;
  candidates?: number;
  resumable?: boolean;
  warnings?: string[];
};
type Snapshot = { config: Config; suggested_ranges?: string[]; scan: Scan };
type Access = {
  status: "ok" | "rejected" | "error" | "enrolled";
  password?: number;
  message: string;
};

export default function HostDiscovery({
  hosts,
  onClose,
  onConfigure,
  onSaved,
  notify,
}: {
  hosts: Host[];
  onClose: () => void;
  onConfigure: (address: string, port: number) => void;
  onSaved: () => void;
  notify: Notify;
}) {
  const rangesInput = useRef<HTMLTextAreaElement>(null);
  const [ranges, setRanges] = useState("");
  const [ports, setPorts] = useState({
    web_port: 8006,
    pbs_port: 8007,
    ssh_port: 22,
  });
  const [parallel, setParallel] = useState(256);
  const [clusterHints, setClusterHints] = useState(true);
  const [suggested, setSuggested] = useState<string[]>([]);
  const [scan, setScan] = useState<Scan | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState("");
  const pending = useRef(false);
  const stop = useRef(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [identities, setIdentities] = useState<Record<string, HostIdentity>>(
    {},
  );
  const [confirmed, setConfirmed] = useState<string[]>([]);
  const [username, setUsername] = useState("root");
  const [passwords, setPasswords] = useState(["", ""]);
  const [access, setAccess] = useState<Record<string, Access>>({});
  const running = scan?.status === "running";
  const existing = (found: Found) =>
    hosts.some(
      (h) =>
        h.address.toLowerCase() === found.address.toLowerCase() ||
        (found.name &&
          h.address.toLowerCase().replace(/\.$/, "") ===
            found.name.toLowerCase()),
    );
  const results = Array.from(
    (scan?.results || [])
      .reduce((grouped, found) => {
        const old = grouped.get(found.address);
        if (old) {
          old.services!.push(found);
          old.ssh_reachable ||= found.ssh_reachable;
        } else grouped.set(found.address, { ...found, services: [found] });
        return grouped;
      }, new Map<string, Found>())
      .values(),
  );
  const eligible = selected.filter(
    (address) =>
      confirmed.includes(address) &&
      identities[address] &&
      !identities[address].changed &&
      !results.some((r) => r.address === address && existing(r)) &&
      access[address]?.status !== "enrolled",
  );
  const config = (): Config => ({
    ...ports,
    parallel,
    use_cluster_hints: clusterHints,
    ranges: ranges
      .split(/[\n;,]+/)
      .map((s) => s.trim())
      .filter(Boolean),
  });

  useEffect(() => {
    const controller = new AbortController();
    api<Snapshot>("hosts/discovery", "GET", undefined, controller.signal)
      .then((data) => {
        if (controller.signal.aborted) return;
        setParallel(data.config.parallel || 256);
        setClusterHints(data.config.use_cluster_hints !== false);
        setRanges(data.config.ranges.join("\n"));
        setPorts({
          web_port: data.config.web_port,
          pbs_port: data.config.pbs_port || 8007,
          ssh_port: data.config.ssh_port,
        });
        setSuggested(data.suggested_ranges || []);
        setScan(data.scan);
        setLoaded(true);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(e.message);
      });
    return () => {
      controller.abort();
      stop.current = true;
    };
  }, []);
  useEffect(() => {
    if (!loaded) return;
    if (!rangesInput.current?.disabled) rangesInput.current?.focus();
    else
      rangesInput.current
        ?.closest('[role="dialog"]')
        ?.querySelector<HTMLButtonElement>("button:not(:disabled)")
        ?.focus();
  }, [loaded]);
  useEffect(() => {
    if (!running) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const data = await api<Snapshot>(
          "hosts/discovery",
          "GET",
          undefined,
          controller.signal,
        );
        if (!controller.signal.aborted) setScan(data.scan);
      } catch (e) {
        if (!controller.signal.aborted) setError((e as Error).message);
      }
      if (!controller.signal.aborted) timer = setTimeout(poll, 1500);
    };
    timer = setTimeout(poll, 700);
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [running]);

  async function action(label: string, work: () => Promise<void>) {
    if (pending.current) return;
    pending.current = true;
    stop.current = false;
    setBusy(label);
    setError("");
    setMessage("");
    try {
      await work();
    } catch (e) {
      setError("Aktion fehlgeschlagen: " + (e as Error).message);
    } finally {
      pending.current = false;
      setBusy("");
    }
  }
  async function save(start: boolean) {
    await action(
      start ? "Suche wird gestartet" : "Netzbereiche werden gespeichert",
      async () => {
        const data = await api<Snapshot>(
          "hosts/discovery",
          start ? "POST" : "PUT",
          config(),
        );
        setRanges(data.config.ranges.join("\n"));
        setPorts({
          web_port: data.config.web_port,
          pbs_port: data.config.pbs_port || 8007,
          ssh_port: data.config.ssh_port,
        });
        setScan(data.scan);
        if (start) {
          setSelected([]);
          setIdentities({});
          setConfirmed([]);
          setAccess({});
        }
        setMessage(
          start
            ? "Serversuche gestartet. Du kannst dieses Fenster schließen und später zurückkehren."
            : "Netzbereiche gespeichert.",
        );
      },
    );
  }
  async function inspect() {
    await action("SSH-Identitäten werden gelesen", async () => {
      for (const address of selected) {
        if (stop.current) break;
        try {
          const identity = await api<HostIdentity>(
            "hosts/connection/inspect",
            "POST",
            { address, port: scan!.config.ssh_port },
          );
          setIdentities((old) => ({ ...old, [address]: identity }));
          setConfirmed((old) => old.filter((a) => a !== address));
          setAccess((old) => {
            const next = { ...old };
            delete next[address];
            return next;
          });
        } catch {
          setAccess((old) => ({
            ...old,
            [address]: {
              status: "error",
              message: "SSH-Identität nicht lesbar. Verbindung prüfen.",
            },
          }));
        }
      }
    });
  }
  async function checkAccess() {
    await action("Zugangsdaten werden geprüft", async () => {
      // Each round only includes hosts whose previous authentication was rejected.
      let remaining = eligible.filter((a) => access[a]?.status !== "ok");
      for (let slot = 0; slot < passwords.length; slot++) {
        if (!passwords[slot] || stop.current) continue;
        const rejected: string[] = [];
        for (const address of remaining) {
          if (stop.current) break;
          try {
            const result = await api<{ authenticated: boolean }>(
              "hosts/connection/check",
              "POST",
              {
                address,
                port: scan!.config.ssh_port,
                username: username.trim(),
                password: passwords[slot],
                fingerprint: identities[address].fingerprint,
                confirmed: true,
              },
            );
            if (!result.authenticated) rejected.push(address);
            setAccess((old) => ({
              ...old,
              [address]: result.authenticated
                ? {
                    status: "ok",
                    password: slot,
                    message: `Anmeldung mit Passwort ${slot + 1} erfolgreich`,
                  }
                : { status: "rejected", message: "Anmeldung abgelehnt" },
            }));
          } catch {
            setAccess((old) => ({
              ...old,
              [address]: {
                status: "error",
                message:
                  "Prüfung fehlgeschlagen. Verbindung und Identität prüfen.",
              },
            }));
          }
        }
        remaining = rejected;
      }
    });
  }
  async function enroll() {
    await action("Hosts werden eingerichtet", async () => {
      for (const address of eligible) {
        if (stop.current) break;
        const checked = access[address];
        if (checked?.status !== "ok" || checked.password === undefined)
          continue;
        try {
          await api("hosts/connection/enroll", "POST", {
            host: { address, ssh_port: scan!.config.ssh_port, enabled: true },
            username: username.trim(),
            password: passwords[checked.password],
            fingerprint: identities[address].fingerprint,
            confirmed: true,
          });
          setAccess((old) => ({
            ...old,
            [address]: { status: "enrolled", message: "Eingerichtet" },
          }));
          onSaved();
        } catch {
          setAccess((old) => ({
            ...old,
            [address]: {
              ...checked,
              message:
                "Einrichtung fehlgeschlagen. Zugang und Rechte prüfen; erneuter Versuch möglich.",
            },
          }));
        }
      }
      notify("Einrichtungsdurchlauf beendet. Ergebnisse je Host prüfen.");
    });
  }
  const resetAccess = () =>
    setAccess((old) =>
      Object.fromEntries(
        Object.entries(old).filter(([, value]) => value.status === "enrolled"),
      ),
    );

  return (
    <Dialog
      topic="network"
      title="Proxmox-Server suchen"
      onClose={() => {
        if (!pending.current) {
          setPasswords(["", ""]);
          onClose();
        }
      }}
      wide
      busy={!!busy}
    >
      <p className="dialog-intro">
        Finde erreichbare Proxmox-Server vom Anker-Server aus. Für VPN und
        weitere Standorte die jeweiligen Netze ergänzen. Die Port- und
        HTTPS-Prüfung benötigt kein Passwort. Optionale Konfigurationshinweise
        werden ausschließlich über bereits eingerichtete SSH-Zugänge gelesen;
        nur Adressen innerhalb der ausgewählten Netze werden geprüft.
      </p>
      <Field
        label="Netzbereiche für LAN und VPN"
        hint="Ein Netz pro Zeile; auch Semikolon und Komma sind möglich. Bis zu 128 Netze und 65.536 Adressen je Suche."
      >
        <textarea
          ref={rangesInput}
          rows={7}
          value={ranges}
          disabled={!loaded || !!busy || running}
          onChange={(e) => setRanges(e.target.value)}
          placeholder={
            "10.20.0.0/24\n10.21.0.0 255.255.255.0\n10.22.0.0/255.255.255.0"
          }
        />
      </Field>
      {suggested.length > 0 && (
        <div className="discovery-suggestions">
          <span className="muted">Netze der Anker-Schnittstellen:</span>
          {suggested.map((net) => (
            <button
              type="button"
              className="secondary"
              disabled={!loaded || !!busy || running}
              key={net}
              onClick={() =>
                setRanges((old) =>
                  [...new Set([...old.split("\n").filter(Boolean), net])].join(
                    "\n",
                  ),
                )
              }
            >
              + {net}
            </button>
          ))}
        </div>
      )}
      <details className="detail-section">
        <summary>Suchports anpassen</summary>
        <div className="form-grid">
          <Field label="Proxmox-Webport">
            <input
              type="number"
              min={1}
              max={65535}
              value={ports.web_port}
              disabled={!!busy || running}
              onChange={(e) =>
                setPorts((old) => ({ ...old, web_port: +e.target.value }))
              }
            />
          </Field>
          <Field label="SSH-Port für gefundene Hosts">
            <input
              type="number"
              min={1}
              max={65535}
              value={ports.ssh_port}
              disabled={!!busy || running}
              onChange={(e) =>
                setPorts((old) => ({ ...old, ssh_port: +e.target.value }))
              }
            />
          </Field>
        </div>
        <div className="form-grid">
          <Field label="PBS-Webport">
            <input
              type="number"
              min={1}
              max={65535}
              value={ports.pbs_port}
              disabled={!!busy || running}
              onChange={(e) =>
                setPorts((old) => ({ ...old, pbs_port: +e.target.value }))
              }
            />
          </Field>
          <Field
            label="Gleichzeitige Adressprüfungen"
            hint="16 bis 256; zusätzliche Begrenzung auf 256 neue Adressen pro Sekunde."
          >
            <input
              type="number"
              min={16}
              max={256}
              value={parallel}
              disabled={!!busy || running}
              onChange={(e) => setParallel(+e.target.value)}
            />
          </Field>
        </div>
        <label className="check">
          <input
            type="checkbox"
            checked={clusterHints}
            disabled={!!busy || running}
            onChange={(e) => setClusterHints(e.target.checked)}
          />{" "}
          Hinweise aus bekannten PVE-Clustern und PBS-Konfigurationen zusätzlich
          verwenden
        </label>
      </details>
      <p className="field-hint">
        Die Schnellrunde verteilt bevorzugte Host-Adressen über alle
        Standortnetze. Danach folgen die übrigen Adressen und eine langsamere
        Nachprüfung. Ein Treffer beendet die Suche im jeweiligen Netz nicht.
      </p>
      <div className="discovery-actions">
        <button
          disabled={!loaded || !!busy || running || !ranges.trim()}
          onClick={() => save(true)}
        >
          Server suchen
        </button>
        <button
          className="secondary"
          disabled={!loaded || !!busy || running || !ranges.trim()}
          onClick={() => save(false)}
        >
          Netzbereiche speichern
        </button>
        {scan?.resumable && !running && (
          <button
            className="secondary"
            disabled={!!busy}
            onClick={() =>
              action("Suche wird fortgesetzt", async () => {
                const data = await api<{ scan: Scan }>(
                  "hosts/discovery",
                  "PATCH",
                  {},
                );
                setScan(data.scan);
                setRanges(data.scan.config.ranges.join("\n"));
                setPorts({
                  web_port: data.scan.config.web_port,
                  pbs_port: data.scan.config.pbs_port || 8007,
                  ssh_port: data.scan.config.ssh_port,
                });
                setParallel(data.scan.config.parallel || 256);
                setClusterHints(data.scan.config.use_cluster_hints !== false);
                setSelected([]);
                setIdentities({});
                setConfirmed([]);
                setAccess({});
                setMessage(
                  "Gespeicherte Suche fortgesetzt. Bereits abgeschlossene Prüfungen bleiben erhalten.",
                );
              })
            }
          >
            Suche fortsetzen
          </button>
        )}
        {running && (
          <button
            className="secondary"
            disabled={!!busy}
            onClick={() =>
              action("Suche wird abgebrochen", async () => {
                await api("hosts/discovery", "DELETE");
              })
            }
          >
            Suche abbrechen
          </button>
        )}
      </div>
      {scan && scan.status !== "idle" && (
        <div className="discovery-progress" role="status">
          <span>
            {scan.status === "running"
              ? "Suche läuft"
              : scan.status === "cancelled" || scan.status === "paused"
                ? "Suche pausiert – fortsetzbare Teilergebnisse"
                : scan.status === "failed"
                  ? "Suchfortschritt nicht verfügbar"
                  : "Suche abgeschlossen"}
            : {scan.checked} / {scan.total} Adressen geprüft · {results.length}{" "}
            Server gefunden
          </span>
          <progress
            aria-label="Suchfortschritt"
            value={scan.checked}
            max={scan.total || 1}
          />
          {scan.phase && (
            <p>
              {(
                {
                  hints: "Bekannte Konfigurationen lesen",
                  fast: "Schnellrunde: TCP-Ports prüfen",
                  slow: "Langsamere Nachprüfung für unklare Verbindungen",
                  confirm: "PVE-/PBS-Erkennung abschließen",
                  paused: "Gespeichert – Fortsetzen möglich",
                  completed: "Vollständig geprüft",
                } as Record<string, string>
              )[scan.phase] || scan.phase}{" "}
              · {scan.tcp_checked ?? scan.checked} Adressen in der
              TCP-Schnellrunde geprüft
            </p>
          )}
          {scan.resumable && !running && (
            <p>
              Fortsetzen verwendet die ursprünglich gespeicherten Netzbereiche
              und Ports. Mit „Server suchen“ beginnt eine neue Suche.
            </p>
          )}
          <p className="muted">
            Ergebnisse gelten für die Suchports {scan.config.web_port} (PVE),{" "}
            {scan.config.pbs_port || 8007} (PBS) und {scan.config.ssh_port}{" "}
            (SSH). Nicht erreichbare oder durch Firewalls gesperrte Server
            können fehlen. Die Erkennung bestätigt noch keine Hostidentität.
          </p>
        </div>
      )}
      {scan?.warnings?.map((warning, index) => (
        <p className="notice" key={index}>
          {warning}
        </p>
      ))}
      {scan?.error && (
        <p className="notice warning" role="alert">
          {scan.error}
        </p>
      )}
      {error && (
        <p className="notice warning" role="alert">
          {error}
        </p>
      )}
      {message && <p role="status">{message}</p>}
      {!running && scan?.status !== "idle" && scan && !results.length && (
        <p>
          Keine Proxmox-Server gefunden. Netzbereiche, VPN-Routen, Webport und
          Firewall vom Anker-Server aus prüfen.
        </p>
      )}
      {results.length > 0 && (
        <>
          <h3>Gefundene Server</h3>
          <label className="check">
            <input
              type="checkbox"
              disabled={!!busy || running}
              checked={
                results.some((r) => !existing(r) && r.ssh_reachable) &&
                results
                  .filter((r) => !existing(r) && r.ssh_reachable)
                  .every((r) => selected.includes(r.address))
              }
              onChange={(e) =>
                setSelected(
                  e.target.checked
                    ? results
                        .filter((r) => !existing(r) && r.ssh_reachable)
                        .map((r) => r.address)
                    : [],
                )
              }
            />
            Alle neuen Hosts mit erreichbarem SSH auswählen
          </label>
          <div className="table-scroll">
            <table className="discovery-results">
              <thead>
                <tr>
                  <th>Auswahl / Server</th>
                  <th>Erkennung</th>
                  <th>Einrichtung</th>
                </tr>
              </thead>
              <tbody>
                {results.map((found) => {
                  const known = existing(found);
                  const identity = identities[found.address];
                  const chosen = selected.includes(found.address);
                  return (
                    <tr key={found.address}>
                      <td>
                        <label className="check">
                          <input
                            type="checkbox"
                            aria-label={`${found.address} auswählen`}
                            disabled={
                              !!busy ||
                              running ||
                              known ||
                              !found.ssh_reachable ||
                              access[found.address]?.status === "enrolled"
                            }
                            checked={chosen}
                            onChange={(e) =>
                              setSelected((old) =>
                                e.target.checked
                                  ? [...old, found.address]
                                  : old.filter((a) => a !== found.address),
                              )
                            }
                          />
                          <span className="mono">{found.address}</span>
                        </label>
                        {found.name && <div>{found.name}</div>}
                        {identity && (
                          <div className="discovery-identity">
                            <span className="mono">{identity.fingerprint}</span>
                            {identity.changed ? (
                              <p className="warning">
                                Hostschlüssel geändert – gesperrt
                              </p>
                            ) : (
                              <label className="check">
                                <input
                                  type="checkbox"
                                  aria-label={`Fingerprint für ${found.address} bestätigt`}
                                  disabled={!!busy}
                                  checked={confirmed.includes(found.address)}
                                  onChange={(e) =>
                                    setConfirmed((old) =>
                                      e.target.checked
                                        ? [...old, found.address]
                                        : old.filter(
                                            (a) => a !== found.address,
                                          ),
                                    )
                                  }
                                />
                                Fingerprint unabhängig geprüft und bestätigt
                              </label>
                            )}
                          </div>
                        )}
                      </td>
                      <td>
                        {(found.services || [found]).map((service, index) => (
                          <div key={index}>
                            {service.kind === "pbs"
                              ? "Proxmox Backup Server"
                              : "Proxmox VE"}
                            {service.version ? ` ${service.version}` : ""}
                            {service.web_port
                              ? ` · Port ${service.web_port}`
                              : ""}
                          </div>
                        ))}
                        {found.source && found.source !== "network" && (
                          <div className="muted">
                            Hinweis:{" "}
                            {found.source === "cluster"
                              ? "bekannter Cluster"
                              : found.source === "pbs-config"
                                ? "PBS-Konfiguration"
                                : "bekannter Host"}
                          </div>
                        )}
                        <div className="muted">
                          SSH{" "}
                          {found.ssh_reachable
                            ? "erreichbar"
                            : "nicht erreichbar"}
                        </div>
                        {known && <div>Bereits eingerichtet</div>}
                      </td>
                      <td>
                        {access[found.address] && (
                          <p role="status">{access[found.address].message}</p>
                        )}
                        {!known &&
                          access[found.address]?.status !== "enrolled" && (
                            <button
                              className="secondary"
                              disabled={
                                !!busy || running || !found.ssh_reachable
                              }
                              onClick={() =>
                                onConfigure(
                                  found.address,
                                  scan!.config.ssh_port,
                                )
                              }
                            >
                              Einzeln einrichten
                            </button>
                          )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <details className="detail-section">
            <summary>
              Optional: gemeinsame Zugangsdaten prüfen und Hosts einrichten
            </summary>
            <p>
              Wähle die Hosts und lies ihre SSH-Identitäten. Vergleiche die
              Fingerprints über einen vertrauenswürdigen Zugang, etwa die
              Proxmox-Konsole. Erst nach der Bestätigung werden Passwörter
              gesendet.
            </p>
            <button
              className="secondary"
              disabled={!!busy || running || !selected.length}
              onClick={inspect}
            >
              SSH-Identitäten lesen
            </button>
            <p className="muted">
              Die Zugangsprüfung führt keine Befehle aus. Passwort 1 wird zuerst
              geprüft, Passwort 2 nur bei abgelehnter Anmeldung. Die Passwörter
              bleiben nur bis zum Schließen im Arbeitsspeicher dieses Fensters.
            </p>
            <div className="form-grid">
              <Field label="Gemeinsamer SSH-Benutzer">
                <input
                  autoComplete="off"
                  value={username}
                  disabled={!!busy}
                  onChange={(e) => {
                    setUsername(e.target.value);
                    resetAccess();
                  }}
                />
              </Field>
              {passwords.map((value, slot) => (
                <Field
                  key={slot}
                  label={`Passwort ${slot + 1}${slot ? " (optional)" : ""}`}
                >
                  <input
                    type="password"
                    autoComplete="new-password"
                    value={value}
                    disabled={!!busy}
                    onChange={(e) => {
                      setPasswords((old) =>
                        old.map((p, i) => (i === slot ? e.target.value : p)),
                      );
                      resetAccess();
                    }}
                  />
                </Field>
              ))}
            </div>
            <div className="discovery-actions">
              <button
                className="secondary"
                disabled={
                  !!busy ||
                  running ||
                  !eligible.length ||
                  !username.trim() ||
                  !passwords[0]
                }
                onClick={checkAccess}
              >
                Zugänge prüfen
              </button>
              <button
                disabled={
                  !!busy ||
                  running ||
                  !eligible.some((a) => access[a]?.status === "ok")
                }
                onClick={enroll}
              >
                Geprüfte Hosts einrichten
              </button>
            </div>
          </details>
        </>
      )}
      {busy && <p role="status">{busy} …</p>}
      {!!busy && (
        <button
          className="secondary"
          onClick={() => {
            stop.current = true;
            setMessage(
              "Der laufende Schritt wird beendet. Weitere Hosts werden nicht begonnen.",
            );
          }}
        >
          Nach aktuellem Schritt stoppen
        </button>
      )}
      <footer className="dialog-footer">
        <button className="secondary" disabled={!!busy} onClick={onClose}>
          Schließen
        </button>
      </footer>
    </Dialog>
  );
}
