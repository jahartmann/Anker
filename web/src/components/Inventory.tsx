import { Fragment, useState } from "react";
import { ChevronDown, Search } from "lucide-react";
import { bytes, date } from "../api";
import type { Inventory as HostInventory } from "../api";
import { Empty, Pagination } from "./shared";

export default function Inventory({
  inventory,
  collectedAt,
  queryError,
}: {
  inventory: HostInventory & { captured_at?: string };
  collectedAt?: string;
  queryError?: string;
}) {
  const isPBS = inventory.host_type === "pbs";
  const [networkQuery, setNetworkQuery] = useState("");
  const [diskQuery, setDiskQuery] = useState("");
  const [networkPage, setNetworkPage] = useState(1);
  const [diskPage, setDiskPage] = useState(1);
  const [networkSize, setNetworkSize] = useState(25);
  const [diskSize, setDiskSize] = useState(25);
  const [expanded, setExpanded] = useState<string[]>([]);
  const interfaces = (inventory.interfaces || []).filter((p) =>
    [p.name, p.mac, p.pci]
      .join(" ")
      .toLowerCase()
      .includes(networkQuery.trim().toLowerCase()),
  );
  const disks = (inventory.disks || []).filter((d) =>
    [d.name, d.id, d.uuid, d.mount]
      .join(" ")
      .toLowerCase()
      .includes(diskQuery.trim().toLowerCase()),
  );
  const currentNetworkPage = Math.min(
    networkPage,
    Math.max(1, Math.ceil(interfaces.length / networkSize)),
  );
  const currentDiskPage = Math.min(
    diskPage,
    Math.max(1, Math.ceil(disks.length / diskSize)),
  );
  const results = (inventory.query_states || {}) as Record<
    string,
    { state?: string; error?: string }
  >;
  const details = Object.entries(inventory.details || {}).filter(
    ([name]) =>
      ![
        "command_results",
        "capabilities",
        "file_hashes",
        "file_metadata",
      ].includes(name),
  );
  const queryLabel = (state?: string) =>
    state === "not_applicable"
      ? "Nicht anwendbar"
      : state === "failed"
        ? "Abfrage fehlgeschlagen"
        : "Abfragestatus unbekannt – keine vollständigen Nachweise";
  const queryNotice = (name: string, title: string) =>
    results[name]?.state === "ok" ? null : (
      <p
        className={
          results[name]?.state === "failed" ? "notice warning" : "notice"
        }
        role={results[name]?.state === "failed" ? "alert" : "status"}
      >
        {results[name]?.state === "failed"
          ? `${title}abfrage fehlgeschlagen: Keine vollständigen Daten verfügbar.`
          : `${title} · ${queryLabel(results[name]?.state)}.`}{" "}
        {results[name]?.state !== "not_applicable" &&
          "Die Anzeige belegt keine Abwesenheit."}
      </p>
    );
  const missingDetails = Object.entries(results).filter(
    ([name, result]) =>
      !details.some(([key]) => key === name) &&
      !["interfaces", "disks"].includes(name) &&
      result.state !== "ok",
  );
  const detailLabels: Record<string, string> = {
    pbs_version: "PBS-Version",
    pbs_datastores: "PBS-Datastores",
    pbs_sync_jobs: "PBS-Sync-Jobs",
    pbs_prune_jobs: "PBS-Prune-Jobs",
    pbs_verify_jobs: "PBS-Verify-Jobs",
    addresses: "IP-Adressen",
    routes: "Netzwerkrouten",
    packages: "Pakete",
    manual_packages: "Manuell installierte Pakete",
    storage: "Storage",
    zfs: "ZFS-Pools",
    lvm: "LVM",
    guests: "Virtuelle Maschinen",
    containers: "Container",
    cluster: "Cluster",
    ceph: "Ceph",
    pci: "PCI-Geräte",
    boot: "Boot-Konfiguration",
    boot_id: "Boot-ID",
    file_hashes: "Dateiprüfsummen",
    lsblk: "Blockgeräte",
  };
  const detailCount = (value: unknown) =>
    Array.isArray(value)
      ? value.length
      : value && typeof value === "object"
        ? Object.keys(value).length
        : typeof value === "string"
          ? value.split("\n").filter(Boolean).length
          : undefined;
  return (
    <div className="inventory">
      <h3>Hostinventar</h3>
      {!Object.keys(results).length && (
        <p className="notice">
          Abfragestatus nicht übermittelt. Leere Listen belegen keine
          Abwesenheit; bei fehlenden Daten die Hostprüfung und ihr Ergebnis
          prüfen.
        </p>
      )}
      {queryError && (
        <p className="notice warning" role="alert">
          Letzte Hostprüfung fehlgeschlagen: {queryError}. Angezeigt wird das
          zuletzt erfasste Inventar.
        </p>
      )}
      <div className="inventory-heading">
        <span className="muted">{inventory.hostname || "Hostinventar"}</span>
        <span className="muted">
          {inventory.captured_at
            ? "Erfasst " + date(inventory.captured_at)
            : collectedAt
              ? "Letzte Hostprüfung " + date(collectedAt)
              : "Erfassungszeit unbekannt"}
        </span>
      </div>
      <dl className="inventory-summary">
        <div>
          <dt>{isPBS ? "Proxmox Backup Server" : "Proxmox VE"}</dt>
          <dd>
            {(isPBS ? inventory.pbs_version : inventory.pve_version) ||
              "Nicht erfasst"}
          </dd>
        </div>
        <div>
          <dt>Debian</dt>
          <dd>{inventory.debian || "Nicht erfasst"}</dd>
        </div>
        <div>
          <dt>Kernel</dt>
          <dd className="mono">{inventory.kernel || "Nicht erfasst"}</dd>
        </div>
        <div>
          <dt>Bootmodus</dt>
          <dd>{inventory.boot_mode || "Nicht erfasst"}</dd>
        </div>
        {!isPBS && (
          <div>
            <dt>Cluster</dt>
            <dd>{inventory.cluster_id || "Standalone"}</dd>
          </div>
        )}
        {inventory.cluster_id && (
          <div>
            <dt>Quorum</dt>
            <dd>
              <span
                className={
                  "state " + (inventory.quorate ? "success" : "warning")
                }
              >
                {inventory.quorate ? "Quorum vorhanden" : "Quorum fehlt"}
              </span>
            </dd>
          </div>
        )}
      </dl>
      {isPBS && (
        <section className="inventory-section" aria-label="PBS-Konfiguration">
          <h3>PBS-Konfiguration</h3>
          <p className="notice">
            Gesichert wird die Konfiguration. Datastore-Inhalte werden separat
            gesichert. Die geführte Wiederherstellung übernimmt ausschließlich
            PBS-Konfigurationen; Datastore-Inhalte bleiben erhalten.
          </p>
          <h4>Datastores</h4>
          {queryNotice("pbs_datastores", "Datastores")}
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Pfad</th>
                </tr>
              </thead>
              <tbody>
                {(inventory.pbs_datastores || []).map((store) => (
                  <tr key={store.name}>
                    <td>{store.name}</td>
                    <td className="mono">{store.path}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {results.pbs_datastores?.state === "ok" &&
            !inventory.pbs_datastores?.length && (
              <p>Keine Datastores konfiguriert.</p>
            )}
          <h4>Konfigurierte Jobs</h4>
          {queryNotice("pbs_sync_jobs", "Sync-Jobs")}
          {queryNotice("pbs_prune_jobs", "Prune-Jobs")}
          {queryNotice("pbs_verify_jobs", "Verify-Jobs")}
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Typ</th>
                  <th>Name</th>
                  <th>Datastore</th>
                  <th>Zeitplan</th>
                </tr>
              </thead>
              <tbody>
                {(inventory.pbs_jobs || []).map((job) => (
                  <tr key={job.kind + ":" + job.name}>
                    <td>{job.kind}</td>
                    <td>{job.name}</td>
                    <td>{job.store}</td>
                    <td>
                      {job.disabled ? "Deaktiviert" : job.schedule || "Manuell"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
      <section
        className="inventory-section"
        aria-labelledby="inventory-network-title"
      >
        <div className="inventory-toolbar">
          <h3 id="inventory-network-title">
            Netzwerk{" "}
            <span className="count-badge">
              {inventory.interfaces?.length || 0}
            </span>
          </h3>
          <div className="search-field">
            <Search size={14} aria-hidden="true" />
            <input
              aria-label="Netzwerk durchsuchen"
              placeholder="Name, MAC oder PCI suchen"
              value={networkQuery}
              onChange={(e) => {
                setNetworkQuery(e.target.value);
                setNetworkPage(1);
              }}
            />
          </div>
        </div>
        {queryNotice("interfaces", "Netzwerk")}
        {interfaces.length ? (
          <div className="table-scroll">
            <table className="inventory-network">
              <thead>
                <tr>
                  <th>Schnittstelle</th>
                  <th>MAC-Adresse</th>
                  <th>PCI-Pfad</th>
                </tr>
              </thead>
              <tbody>
                {interfaces
                  .slice(
                    (currentNetworkPage - 1) * networkSize,
                    currentNetworkPage * networkSize,
                  )
                  .map((p) => (
                    <tr key={p.name}>
                      <td className="mono">{p.name}</td>
                      <td className="mono muted">{p.mac || "—"}</td>
                      <td className="mono muted">{p.pci || "—"}</td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        ) : results.interfaces?.state !== "ok" ? null : (
          <Empty
            title={
              networkQuery
                ? "Keine passenden Schnittstellen"
                : "Keine Schnittstellen erfasst"
            }
          >
            Suche oder Hostprüfung prüfen.
          </Empty>
        )}
        <div className="list-footer">
          <select
            aria-label="Netzwerkeinträge pro Seite"
            value={networkSize}
            onChange={(e) => {
              setNetworkSize(Number(e.target.value));
              setNetworkPage(1);
            }}
          >
            <option value={25}>25 pro Seite</option>
            <option value={50}>50 pro Seite</option>
          </select>
          <Pagination
            page={currentNetworkPage}
            pageSize={networkSize}
            total={interfaces.length}
            onPageChange={setNetworkPage}
            label="Schnittstellen"
            previousLabel="Vorherige Netzwerkseite"
            nextLabel="Nächste Netzwerkseite"
          />
        </div>
      </section>
      <section
        className="inventory-section"
        aria-labelledby="inventory-disks-title"
      >
        <div className="inventory-toolbar">
          <h3 id="inventory-disks-title">
            Datenträger{" "}
            <span className="count-badge">{inventory.disks?.length || 0}</span>
          </h3>
          <div className="search-field">
            <Search size={14} aria-hidden="true" />
            <input
              aria-label="Datenträger durchsuchen"
              placeholder="Gerät, Kennung oder Mount suchen"
              value={diskQuery}
              onChange={(e) => {
                setDiskQuery(e.target.value);
                setDiskPage(1);
              }}
            />
          </div>
        </div>
        {queryNotice("disks", "Datenträger")}
        {disks.length ? (
          <div className="table-scroll">
            <table className="inventory-disks">
              <thead>
                <tr>
                  <th>Gerät</th>
                  <th>Größe</th>
                  <th>Mount</th>
                  <th className="right">Details</th>
                </tr>
              </thead>
              <tbody>
                {disks
                  .slice(
                    (currentDiskPage - 1) * diskSize,
                    currentDiskPage * diskSize,
                  )
                  .map((d) => {
                    const key = [d.name, d.id, d.uuid].join(":");
                    const open = expanded.includes(key);
                    return (
                      <Fragment key={key}>
                        <tr>
                          <td className="mono">{d.name}</td>
                          <td>{bytes(d.size)}</td>
                          <td className="mono muted">{d.mount || "—"}</td>
                          <td className="right">
                            <button
                              className="text-button"
                              aria-label={"Details zu " + d.name}
                              aria-expanded={open}
                              onClick={() =>
                                setExpanded((old) =>
                                  open
                                    ? old.filter((v) => v !== key)
                                    : [...old, key],
                                )
                              }
                            >
                              <ChevronDown
                                size={14}
                                className={open ? "rotated" : ""}
                                aria-hidden="true"
                              />
                            </button>
                          </td>
                        </tr>
                        {open && (
                          <tr className="inventory-detail">
                            <td colSpan={4}>
                              <dl className="properties">
                                <dt>Kennung</dt>
                                <dd className="mono">
                                  {d.id || "Nicht erfasst"}
                                </dd>
                                <dt>UUID</dt>
                                <dd className="mono">
                                  {d.uuid || "Nicht erfasst"}
                                </dd>
                              </dl>
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    );
                  })}
              </tbody>
            </table>
          </div>
        ) : results.disks?.state !== "ok" ? null : (
          <Empty
            title={
              diskQuery
                ? "Keine passenden Datenträger"
                : "Keine Datenträger erfasst"
            }
          >
            Suche oder Hostprüfung prüfen.
          </Empty>
        )}
        <div className="list-footer">
          <select
            aria-label="Datenträgereinträge pro Seite"
            value={diskSize}
            onChange={(e) => {
              setDiskSize(Number(e.target.value));
              setDiskPage(1);
            }}
          >
            <option value={25}>25 pro Seite</option>
            <option value={50}>50 pro Seite</option>
          </select>
          <Pagination
            page={currentDiskPage}
            pageSize={diskSize}
            total={disks.length}
            onPageChange={setDiskPage}
            label="Datenträger"
            previousLabel="Vorherige Datenträgerseite"
            nextLabel="Nächste Datenträgerseite"
          />
        </div>
      </section>
      {(details.length > 0 || missingDetails.length > 0) && (
        <details className="inventory-source">
          <summary>
            <span>Erfasste Details</span>{" "}
            <span className="count-badge">{details.length}</span>
          </summary>
          <p className="muted">Zusätzliche Daten aus der Hostprüfung.</p>
          {missingDetails.map(([name, result]) => (
            <p
              key={name}
              className={result.state === "failed" ? "notice warning" : "muted"}
              role={result.state === "failed" ? "alert" : undefined}
            >
              {detailLabels[name] || name} · {queryLabel(result.state)}
              {result.error ? ": " + result.error : ""}
            </p>
          ))}
          {details.map(([name, value]) => (
            <details key={name} className="inventory-source-item">
              <summary>
                <span>{detailLabels[name] || name}</span>
                {results[name]?.state && results[name].state !== "ok" && (
                  <span className="warning">
                    {queryLabel(results[name].state)}
                  </span>
                )}
                {detailCount(value) !== undefined && (
                  <span className="count-badge">
                    {detailCount(value)} Einträge
                  </span>
                )}
              </summary>
              {queryNotice(name, detailLabels[name] || name)}
              <pre className="inventory-raw">
                {typeof value === "string"
                  ? value
                  : JSON.stringify(value, null, 2)}
              </pre>
            </details>
          ))}
        </details>
      )}
    </div>
  );
}
