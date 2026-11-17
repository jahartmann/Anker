import { date } from "../../api";
import type { Status, User } from "../../api";
import { Field } from "../shared";
import { displayHostName } from "../../hostNames";
import type { RecoveryDraft } from "./useRecoveryDraft";
export default function RecoveryTarget({
  status,
  user,
  draft: d,
  onConnectTarget,
}: {
  status: Status;
  user: User;
  draft: RecoveryDraft;
  onConnectTarget: () => void;
}) {
  const host = status.hosts.find((h) => h.id === d.target);
  const isPBS =
    status.backups.find((b) => b.id === d.backup)?.host_type === "pbs";
  return (
    <section aria-label="SSH-Zielprüfung" data-surface-topic="network">
      <h2>2 · Ziel anbinden und prüfen</h2>
      <p>Die Prüfung liest nur; am Ziel wird nichts verändert.</p>
      <Field label="Zielhost">
        <select
          value={d.target}
          onChange={(e) => d.changeTarget(e.target.value)}
        >
          <option value="">Ziel auswählen</option>
          {status.hosts
            .filter((h) =>
              isPBS
                ? h.inventory?.host_type === "pbs"
                : h.inventory?.host_type !== "pbs",
            )
            .map((h) => (
              <option value={h.id} key={h.id}>
                {displayHostName(h.name)}
              </option>
            ))}
        </select>
      </Field>
      {user.role === "admin" ? (
        <button type="button" className="secondary" onClick={onConnectTarget}>
          Neues Ziel anbinden
        </button>
      ) : (
        <p className="notice">
          Ein Administrator muss ein neues Ziel unter „Hosts → Host hinzufügen“
          anbinden. Danach hier den Zielhost auswählen.
        </p>
      )}
      {isPBS && (
        <p className="notice">
          Die PBS-Dienste müssen gestoppt sein. Geprüft werden Konfiguration und
          eingebundene Datastores; Daten werden nicht angefasst.
        </p>
      )}
      <button
        type="button"
        disabled={d.inspecting || !d.target || !d.sourceReady}
        onClick={d.inspect}
      >
        {d.inspecting ? "Ziel wird geprüft …" : "Ziel prüfen"}
      </button>
      {d.inspection && (
        <div className="recovery-inspection-meta">
          <strong>
            {d.inspection.source.hostname} → {d.inspection.target.hostname}
          </strong>
          <span
            className={`state ${d.inspection.comparison_complete ? "success" : "warning"}`}
          >
            {d.inspection.comparison_complete
              ? "Ziel geprüft"
              : "Zielaufnahme mit Hinweisen"}
          </span>
        </div>
      )}
      {d.inspection && (
        <dl className="recovery-target-facts">
          <dt>SSH-Endpunkt</dt>
          <dd>
            {host?.address}:{host?.ssh_port}
          </dd>
          <dt>Proxmox / Debian</dt>
          <dd>
            {(isPBS
              ? d.inspection.target.pbs_version
              : d.inspection.target.pve_version) || "Unbekannt"}{" "}
            / {d.inspection.target.debian || "Unbekannt"}
          </dd>
          <dt>Bootmodus / Cluster</dt>
          <dd>
            {d.inspection.target.boot_mode || "Unbekannt"} /{" "}
            {d.inspection.target.cluster_id || "Kein Cluster gemeldet"}
          </dd>
          <dt>Aufnahme</dt>
          <dd>
            {date(d.inspection.target.captured_at)} ·{" "}
            {d.inspection.files?.length ?? 0} Datei-Abfragen
          </dd>
        </dl>
      )}
      {d.inspection && !d.inspection.comparison_complete && (
        <p className="notice warning">
          Nicht alle Zieldateien konnten gelesen werden; diese bleiben manuell.
          Für einen neuen Versuch „Ziel prüfen“ wählen.
        </p>
      )}
      {d.needsInspection && (
        <p className="notice warning" role="status">
          Zuordnungen wurden geändert. Ziel erneut prüfen.
        </p>
      )}
      {!d.target && (
        <p className="notice">
          Einen vorhandenen Zielhost auswählen oder ein neues Ziel anbinden.
        </p>
      )}
      {d.target && !d.inspection && (
        <p className="field-hint">
          Als Nächstes „Ziel prüfen“, um die benötigten Entscheidungen zu
          ermitteln.
        </p>
      )}
    </section>
  );
}
