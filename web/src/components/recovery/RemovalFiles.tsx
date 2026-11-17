import { plural } from "../../api";
export default function RemovalFiles({
  paths,
  onDeselect,
  label = "Wird gelöscht",
}: {
  paths: string[];
  onDeselect?: (path: string) => void;
  /** Card caption when the list is read-only. */
  label?: string;
}) {
  const card = (path: string) => (
    <article className="removal-card" key={path}>
      <span className="eyebrow">
        {onDeselect ? "Dateientfernung prüfen" : label}
      </span>
      <strong>{path}</strong>
      {onDeselect && (
        <button
          type="button"
          className="text-button"
          onClick={() => onDeselect(path)}
        >
          Entfernung abwählen: {path}
        </button>
      )}
    </article>
  );
  return (
    <>
      {paths.slice(0, 3).map(card)}
      {paths.length > 3 && (
        <details className="removal-more">
          <summary>
            {plural(paths.length - 3, "weitere Datei", "weitere Dateien")}{" "}
            anzeigen
          </summary>
          {paths.slice(3).map(card)}
        </details>
      )}
    </>
  );
}
