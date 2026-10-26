import { changeHighlights } from "../changeHighlights";
import type { FileDiff } from "../api";
export default function ChangeHighlights({
  rows,
  limit = 3,
}: {
  rows: FileDiff[];
  limit?: number;
}) {
  const highlights = changeHighlights(rows);
  const visible = highlights.slice(0, limit);
  const remaining = highlights
    .slice(limit)
    .reduce((sum, h) => sum + h.count, 0);
  return (
    <div className="change-highlights">
      {visible.length > 0 && (
        <ul aria-label="Kurzbeschreibung der Änderungen">
          {visible.map((h) => (
            <li key={h.text} data-change={h.kind}>
              <span className="highlight-mark" aria-hidden="true">
                {h.kind === "added" ? "+" : h.kind === "removed" ? "−" : "↗"}
              </span>
              <span>
                {h.text}
                {h.count > 1 && <small> · {h.count} Dateien</small>}
              </span>
            </li>
          ))}
        </ul>
      )}
      {remaining > 0 && (
        <p className="highlight-more">
          {remaining} weitere {remaining === 1 ? "Datei" : "Dateien"} im
          Detailvergleich.
        </p>
      )}
      {!highlights.length && rows.length > 0 && (
        <p>
          Nur Zeitangaben aktualisiert; Inhalt und übrige Metadaten sind
          unverändert.
        </p>
      )}
    </div>
  );
}
