import type { Backup, Host, Status } from "./api";

const clockTolerance = 5 * 60 * 1000;
export function usableRecoverySource(backup: Backup, at = Date.now()) {
  const created = Date.parse(backup.created_at);
  return (
    backup.status === "successful" &&
    !backup.verification_error &&
    Number.isFinite(created) &&
    created <= at + clockTolerance
  );
}

// A complete capture without known damage is a source candidate, never proof
// that a machine can be rebuilt. The plan still checks the actual source/target.
export function recoverySource(backups: Backup[], at = Date.now()) {
  return backups
    .filter((b) => usableRecoverySource(b, at))
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))[0];
}

export interface HostAssessment {
  text: string;
  tone: string;
  backup?: Backup;
  reason: string;
  action: "files" | "backup" | "probe" | "job" | "settings";
  actionLabel: string;
  jobID?: string;
}

export function assessHost(
  host: Host,
  status: Status,
  at = Date.now(),
): HostAssessment {
  const backups = status.backups
    .filter((b) => b.host_id === host.id)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
  const latest = backups[0];
  const source = recoverySource(backups, at);
  if (host.maintenance_until && Date.parse(host.maintenance_until) > at)
    return {
      text: "Wartung",
      tone: "muted",
      backup: source,
      reason: `Automatische Sicherungen und Liveprüfungen pausieren bis ${new Date(host.maintenance_until).toLocaleString("de-DE")}. Vorhandene Probleme bleiben im Ursachenverlauf sichtbar.`,
      action: "settings",
      actionLabel: "Wartung ansehen",
    };
  if (!host.enabled)
    return {
      text: "Pausiert",
      tone: "muted",
      backup: source,
      reason:
        "Automatische Sicherungen sind ausgeschaltet. Deine bisherigen Stände bleiben erhalten.",
      action: "settings",
      actionLabel: "Zeitplan ansehen",
    };
  if (latest?.status === "damaged" || latest?.verification_error)
    return {
      text: "Beschädigter Stand",
      tone: "danger",
      backup: latest,
      reason:
        "Die letzte Sicherung hat einen Integritätsfehler. Prüfe die Dateien, bevor du sie zur Wiederherstellung nutzt.",
      action: "files",
      actionLabel: "Stand und Nachweise prüfen",
    };
  if (latest?.status === "partial")
    return {
      text: "Letzte Sicherung unvollständig",
      tone: "warning",
      backup: latest,
      reason: latest.issues?.[0]
        ? latest.issues[0].title + ": " + latest.issues[0].detail
        : "Die letzte Sicherung ist unvollständig. Ursache und nächste Schritte findest du direkt beim Host.",
      action: "files",
      actionLabel: "Fehlende Bestandteile prüfen",
    };
  if (
    latest &&
    (!Number.isFinite(Date.parse(latest.created_at)) ||
      Date.parse(latest.created_at) > at + clockTolerance)
  )
    return {
      text: "Zeitstempel prüfen",
      tone: "warning",
      backup: latest,
      reason:
        "Das Datum der letzten Sicherung ist ungültig oder liegt in der Zukunft. Deshalb ist unklar, wie aktuell sie ist.",
      action: "files",
      actionLabel: "Sicherungsstand prüfen",
    };
  const failed = status.jobs
    .filter(
      (j) =>
        j.host_id === host.id &&
        j.kind === "backup" &&
        ["failed", "interrupted", "cancelled"].includes(j.state),
    )
    .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
  if (
    failed &&
    (!source ||
      Date.parse(failed.created_at) > Date.parse(source.created_at)) &&
    (!source || failed.result_id !== source.id)
  )
    return {
      text: "Letzter Versuch fehlgeschlagen",
      tone: failed.state === "failed" ? "danger" : "warning",
      backup: source,
      reason:
        "Die letzte Sicherung ist fehlgeschlagen. Sieh dir den Fehler im Auftrag an, auch wenn ein älterer Stand vorhanden ist.",
      action: "job",
      actionLabel: "Fehler ansehen",
      jobID: failed.id,
    };
  if (latest && !["successful", "partial", "damaged"].includes(latest.status))
    return {
      text: "Sicherungsstand prüfen",
      tone: "warning",
      backup: latest,
      reason:
        "Die letzte Sicherung ist nicht vollständig abgeschlossen. Prüfe ihren Status.",
      action: "files",
      actionLabel: "Stand prüfen",
    };
  if (!source)
    return {
      text: "Ohne Sicherung",
      tone: "warning",
      reason:
        "Es gibt noch keine vollständige Sicherung ohne bekannte Prüfsummenfehler.",
      action: !host.last_probe || host.probe_error ? "probe" : "backup",
      actionLabel: !host.last_probe
        ? "Erste Hostprüfung starten"
        : host.probe_error
          ? "Hostprüfung wiederholen"
          : "Erste Sicherung starten",
    };
  if (!Number.isFinite(status.stale_hours) || status.stale_hours <= 0)
    return {
      text: "Aktualität ungeklärt",
      tone: "warning",
      backup: source,
      reason:
        "Es ist keine gültige Grenze für das Alter der Sicherungen hinterlegt. Deshalb ist ihre Aktualität unklar.",
      action: "settings",
      actionLabel: "Aktualitätsgrenze prüfen",
    };
  if (at - Date.parse(source.created_at) > status.stale_hours * 3600000)
    return {
      text: "Überfällig",
      tone: "warning",
      backup: source,
      reason: `Der letzte vollständige Stand ist älter als die eingestellte Grenze von ${status.stale_hours} Stunden.`,
      action: "backup",
      actionLabel: "Neuen Stand sichern",
    };
  return {
    text: "Gesichert",
    tone: "success",
    backup: source,
    reason:
      "Die letzte vollständige Sicherung ist noch aktuell. Ob ihre Dateien lesbar sind und wiederhergestellt werden können, wird gesondert geprüft.",
    action: "files",
    actionLabel: "Verlauf ansehen",
  };
}
