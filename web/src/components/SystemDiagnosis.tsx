import { bytes } from "../api";

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function nonnegative(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : undefined;
}

export default function SystemDiagnosis({ value }: { value: unknown }) {
  const source = record(value);
  const disk = record(source.disk);
  const total = nonnegative(disk.total);
  const free = nonnegative(disk.free);
  const used = nonnegative(disk.used_percent);
  const validDisk =
    total !== undefined &&
    total > 0 &&
    free !== undefined &&
    free <= total &&
    used !== undefined &&
    used <= 100;
  const version =
    typeof source.version === "string" &&
    /^v?\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?$/.test(source.version)
      ? source.version
      : undefined;
  const protocol =
    typeof source.protocol === "number" &&
    Number.isSafeInteger(source.protocol) &&
    source.protocol > 0
      ? source.protocol
      : undefined;
  const checks = [
    {
      title: "Serverantwort",
      state: version ? "In Ordnung" : "Nicht geprüft",
      detail: version
        ? `Anker ${version} antwortet auf die Systemprüfung.`
        : "Keine gültige Versionsangabe erhalten. Systemprüfung erneut öffnen.",
    },
    {
      title:
        validDisk && used >= 90 ? "Speicherplatz prüfen" : "Ablagespeicher",
      state: !validDisk
        ? "Nicht geprüft"
        : used >= 90
          ? "Prüfung nötig"
          : "In Ordnung",
      detail: validDisk
        ? `${Math.round(used)} % belegt · ${bytes(free)} verfügbar. ${used >= 90 ? "Unter Betrieb → Speicher Belegung und Aufbewahrung prüfen." : "Die Messung betrifft das Dateisystem der Anker-Ablage."}`
        : "Keine vollständigen Speichermesswerte erhalten. Unter Betrieb → Speicher erneut prüfen.",
    },
    {
      title: "Katalogformat",
      state:
        protocol === undefined
          ? "Nicht geprüft"
          : protocol === 1
            ? "In Ordnung"
            : "Prüfung nötig",
      detail:
        protocol === 1
          ? "Das gemeldete Katalogformat 1 wird unterstützt."
          : "Katalogformat nicht bestätigt. Technische Details prüfen und vor Änderungen den Betreiber kontaktieren.",
    },
    {
      title: "Reale Wiederherstellung",
      state: "Nicht geprüft",
      detail:
        "Diese Systemabfrage prüft keinen Restore auf Proxmox. Wiederherstellungsplan und isolierte Laborprobe separat durchführen und dokumentieren.",
    },
  ];
  const report = {
    format: 1,
    created_at: new Date().toISOString(),
    version,
    demo: typeof source.demo === "boolean" ? source.demo : undefined,
    protocol,
    disk: validDisk ? { total, free, used_percent: used } : undefined,
    checks,
  };
  function downloadReport() {
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(report, null, 2)], {
        type: "application/json;charset=utf-8",
      }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = "anker-diagnose.json";
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30000);
  }
  return (
    <section
      className="system-diagnosis"
      data-surface-topic="system"
      aria-label="Diagnoseübersicht"
    >
      <p className="muted">
        Aktuelle Serverantwort und Ablagespeicher. Hostverbindungen,
        Sicherungsintegrität und echte Wiederherstellungen werden separat
        geprüft.
      </p>
      {source.demo === true && (
        <p className="notice">
          Demo aktiv: Produktive Updates und Speicheränderungen sind
          ausgeschaltet.
        </p>
      )}
      <div className="diagnosis-grid">
        {checks.map((check) => (
          <article className="diagnosis-check" key={check.title}>
            <h4>{check.title}</h4>
            <span
              className={
                "state " +
                (check.state === "In Ordnung"
                  ? "success"
                  : check.state === "Prüfung nötig"
                    ? "warning"
                    : "neutral")
              }
            >
              {check.state}
            </span>
            <p>{check.detail}</p>
          </article>
        ))}
      </div>
      <button type="button" className="secondary" onClick={downloadReport}>
        Diagnosebericht herunterladen
      </button>
      <p className="field-hint">
        Der Bericht enthält Version, Betriebsmodus, Speicherwerte und diese
        Prüfergebnisse. Zugangsdaten, Hostadressen, lokale Pfade und das
        Aktivitätsprotokoll werden nicht exportiert.
      </p>
      <details className="detail-section">
        <summary>Technische Details</summary>
        <pre className="system-output">{JSON.stringify(report, null, 2)}</pre>
      </details>
    </section>
  );
}
