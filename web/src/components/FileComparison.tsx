import type { FileComparison } from "../api";
import { date } from "../api";
import { Empty, LoadError } from "./shared";

const fields: Record<string, string> = {
  type: "Typ",
  mode: "Rechte",
  uid: "Benutzer-ID",
  gid: "Gruppen-ID",
  mtime: "Änderungszeit",
  link: "Verknüpfungsziel",
  xattrs: "Erweiterte Attribute",
};
function metadataValue(field: string, value: unknown): string {
  if (value === null || value === undefined) return "Nicht vorhanden";
  if (field === "mode" && typeof value === "number")
    return "0" + value.toString(8);
  if (field === "mtime" && typeof value === "number")
    return date(new Date(value * 1000).toISOString());
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}
const states: Record<FileComparison["state"], string> = {
  identical: "Inhalt und Metadaten stimmen überein",
  different: "Inhalt geändert",
  missing_before: "Hinzugefügt · im Ausgangsstand nicht vorhanden",
  missing_after: "Entfernt · im Vergleichsstand nicht vorhanden",
  unavailable: "Vergleich nicht verfügbar",
  binary: "Binärdatei · keine Textvorschau",
  too_large: "Datei zu groß für die Textvorschau",
  masked: "Geschützter Inhalt",
};
export default function FileComparisonView({
  comparison,
  loading,
  error,
  retry,
}: {
  comparison: FileComparison | null;
  loading: boolean;
  error: string;
  retry: () => void;
}) {
  if (loading)
    return (
      <p className="muted" role="status">
        Dateivergleich wird geladen …
      </p>
    );
  if (error)
    return <LoadError message={error} retry={retry} label="Erneut lesen" />;
  if (!comparison)
    return (
      <Empty title="Datei auswählen">
        Die Übersicht lädt keine Dateiinhalte. Eine Datei öffnen, um ihren
        Vergleich zu lesen.
      </Empty>
    );
  const c = comparison;
  const restricted = c.masked || c.state === "masked";
  const unavailable = c.state === "unavailable";
  const status = restricted
    ? "Geschützter Inhalt"
    : c.state === "different" && !c.content_changed
      ? c.metadata_changes.every((m) => m.field === "mtime")
        ? "Nur Änderungszeit geändert · kein Schreibbedarf"
        : "Nur Metadaten geändert"
      : states[c.state];
  return (
    <div className="file-comparison">
      <div className="preview-heading">
        <strong className="mono">{c.path}</strong>
      </div>
      <p className="comparison-labels">
        {c.before_label} → {c.after_label}
      </p>
      <p className="comparison-state">{status}</p>
      {restricted ? (
        <p className="muted">
          Die Datei kann Zugangsdaten enthalten. Inhalt und Metadaten sind
          geschützt.
        </p>
      ) : unavailable ? (
        <LoadError
          message={
            c.error || "Dateien konnten nicht verlässlich gelesen werden."
          }
          retry={retry}
          label="Erneut lesen"
        />
      ) : (
        <>
          {c.state === "too_large" && (
            <p className="muted">
              Vollständige Dateien herunterladen und lokal prüfen.
            </p>
          )}
          {c.error && <p className="muted">{c.error}</p>}
          {!!c.metadata_changes.length && (
            <div className="table-scroll">
              <table className="comparison-metadata">
                <caption>Metadaten</caption>
                <thead>
                  <tr>
                    <th>Merkmal</th>
                    <th>Vorher</th>
                    <th>Nachher</th>
                  </tr>
                </thead>
                <tbody>
                  {c.metadata_changes.map((m) => (
                    <tr key={m.field}>
                      <th scope="row">{fields[m.field] || m.field}</th>
                      <td className="mono">
                        {metadataValue(m.field, m.before)}
                      </td>
                      <td className="mono">
                        {metadataValue(m.field, m.after)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {c.state !== "binary" &&
            c.state !== "too_large" &&
            !!c.hunks.length && (
              <div
                className="diff-hunks"
                role="region"
                aria-label="Zeilenvergleich"
              >
                <p className="diff-legend">
                  − Entfernt · + Hinzugefügt · unveränderte Zeilen als Kontext
                </p>
                {c.hunks.map((h, index) => (
                  <div className="diff-hunk" key={index}>
                    <div className="diff-hunk-heading">
                      Vorher ab Zeile {h.before_start || "—"} · Nachher ab Zeile{" "}
                      {h.after_start || "—"}
                    </div>
                    {h.lines.map((line, i) => (
                      <div
                        className={"diff-line diff-" + line.kind}
                        key={i}
                        aria-label={
                          line.kind === "add"
                            ? "Hinzugefügt"
                            : line.kind === "remove"
                              ? "Entfernt"
                              : "Kontext"
                        }
                      >
                        <span className="diff-number" title="Zeile vorher">
                          {line.before_line ?? ""}
                        </span>
                        <span className="diff-number" title="Zeile nachher">
                          {line.after_line ?? ""}
                        </span>
                        <span className="diff-symbol" aria-hidden="true">
                          {line.kind === "add"
                            ? "+"
                            : line.kind === "remove"
                              ? "−"
                              : " "}
                        </span>
                        <span className="diff-text">
                          {line.text}
                          {!line.text.endsWith("\n") && (
                            <small className="diff-no-newline">
                              Kein Zeilenumbruch am Dateiende
                            </small>
                          )}
                        </span>
                      </div>
                    ))}
                  </div>
                ))}
              </div>
            )}
        </>
      )}
    </div>
  );
}
