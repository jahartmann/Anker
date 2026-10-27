import type { ReactNode } from "react";
import { Archive, CircleAlert, RotateCcw } from "lucide-react";
import type { Backup, Status } from "../api";
import { date } from "../api";
import { HostName } from "../hostNames";
import { recoverySource, usableRecoverySource } from "../hostHealth";
import ChangeSummary from "./ChangeSummary";
import { State } from "./shared";

function problem(backup: Backup) {
  return !usableRecoverySource(backup);
}
export function focusBackups(status: Status, hostID: string) {
  const groups = new Map<string, Backup[]>();
  for (const backup of status.backups) {
    const history = groups.get(backup.host_id) || [];
    history.push(backup);
    groups.set(backup.host_id, history);
  }
  for (const history of groups.values())
    history.sort((a, b) => b.created_at.localeCompare(a.created_at));
  if (hostID) return groups.get(hostID) || [];
  return (
    [...groups.values()].sort(
      (a, b) =>
        Number(
          problem(b[0]) ||
            status.references?.[b[0].host_id]?.state === "unresolved",
        ) -
          Number(
            problem(a[0]) ||
              status.references?.[a[0].host_id]?.state === "unresolved",
          ) ||
        b[0].created_at.localeCompare(a[0].created_at) ||
        a[0].host_id.localeCompare(b[0].host_id),
    )[0] || []
  );
}
export default function BackupFocus({
  status,
  hostID,
  hostPicker,
  canRestore,
  onOpen,
  onRestore,
  onCompare,
}: {
  status: Status;
  hostID: string;
  hostPicker?: ReactNode;
  canRestore: boolean;
  onOpen: (id: string) => void;
  onRestore: (id: string) => void;
  onCompare: (from: string, to: string) => void;
}) {
  const history = focusBackups(status, hostID);
  const latest = history[0];
  if (!latest) return null;
  const isPBS = latest.host_type === "pbs";
  const source = recoverySource(history);
  const reference = status.references?.[latest.host_id];
  const needsReview = problem(latest) || reference?.state === "unresolved";
  const reason =
    latest.verification_error || latest.status === "damaged"
      ? "Bei der Prüfung wurden Fehler gefunden. Sieh dir die Dateien und Prüfergebnisse an."
      : latest.status === "partial"
        ? "In dieser Sicherung fehlen benötigte Dateien. Prüfe die Hinweise."
        : !usableRecoverySource(latest)
          ? "Status oder Datum sind unklar. Prüfe den Stand, bevor du ihn verwendest."
          : "Hier findest du die Dateien und Prüfergebnisse der letzten Sicherung.";
  return (
    <section className="backup-focus" aria-label="Sicherung im Fokus">
      <header className="backup-focus-heading">
        <div>
          <span className="eyebrow">
            {needsReview ? "Diesen Stand zuerst prüfen" : "Neueste Sicherung"}
          </span>
          <h2>
            <HostName name={latest.host_name} />
          </h2>
        </div>
        <div className="backup-focus-picker">
          {hostPicker}
          <span className="backup-focus-context">
            {hostID ? "Alle Stände dieses Hosts" : "Aus allen Hosts ausgewählt"}
          </span>
        </div>
      </header>
      <div className="backup-focus-grid">
        <section
          className={
            "host-workspace backup-focus-card " +
            (needsReview ? "needs-review" : "")
          }
          aria-label="Letzter Sicherungsstand"
        >
          <span className="eyebrow">
            {needsReview ? (
              <CircleAlert size={15} aria-hidden="true" />
            ) : (
              <Archive size={15} aria-hidden="true" />
            )}{" "}
            Letzter Stand
          </span>
          <h3>{date(latest.created_at)}</h3>
          <State
            value={
              latest.verification_error
                ? "damaged"
                : latest.status === "successful" &&
                    !usableRecoverySource(latest)
                  ? "partial"
                  : latest.status
            }
            label={
              latest.status === "successful" &&
              !latest.verification_error &&
              !usableRecoverySource(latest)
                ? "Datum prüfen"
                : undefined
            }
          />
          <p>{reason}</p>
          {reference?.state === "unresolved" && (
            <p className="backup-focus-source-note">
              Die Referenz lässt sich nicht öffnen. Prüfe sie im Verlauf.
            </p>
          )}
          {(latest.pinned ||
            latest.archived ||
            (reference?.state === "valid" &&
              reference.backup_id === latest.id)) && (
            <p className="backup-focus-tags">
              {[
                latest.pinned ? "Geschützt" : "",
                latest.archived ? "Archiviert" : "",
                reference?.state === "valid" &&
                reference.backup_id === latest.id
                  ? "Referenzstand"
                  : "",
              ]
                .filter(Boolean)
                .join(" · ")}
            </p>
          )}
          <button className="secondary" onClick={() => onOpen(latest.id)}>
            Dateien dieses Stands öffnen
          </button>
        </section>
        <ChangeSummary
          key={latest.host_id}
          backups={history}
          compact
          openChanges={onCompare}
        />
        <section
          className="host-workspace backup-focus-card"
          aria-label="Stand zur Wiederherstellung"
        >
          <span className="eyebrow">
            <RotateCcw size={15} aria-hidden="true" /> Wiederherstellung
          </span>
          <h3>
            {source ? date(source.created_at) : "Keine passende Sicherung"}
          </h3>
          <p>
            {isPBS
              ? "Der Assistent stellt ausgewählte PBS-Konfigurationen bei gestoppten PBS-Diensten wieder her. Datastore-Inhalte bleiben erhalten."
              : source
                ? "Dieser Stand ist vollständig und hat keine bekannten Prüfsummenfehler. Der Assistent prüft Quelle und Ziel vor dem Wiederherstellen erneut."
                : "Es gibt noch keinen vollständigen Stand ohne bekannte Prüfsummenfehler."}
          </p>
          {source && source.id !== latest.id && (
            <p className="backup-focus-source-note">
              Der letzte Stand ist nicht geeignet. Deshalb ist hier ein älterer
              ausgewählt.
            </p>
          )}
          {!canRestore && (
            <p>Mit deinen Leserechten kannst du die Dateien ansehen.</p>
          )}
          {canRestore ? (
            <button
              className="secondary"
              disabled={!source}
              onClick={() => source && onRestore(source.id)}
            >
              Mit diesem Stand planen
            </button>
          ) : (
            source && (
              <button className="secondary" onClick={() => onOpen(source.id)}>
                Dateien der Quelle ansehen
              </button>
            )
          )}
        </section>
      </div>
    </section>
  );
}
