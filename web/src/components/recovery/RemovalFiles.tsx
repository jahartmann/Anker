export default function RemovalFiles({
  paths,
  onDeselect,
}: {
  paths: string[];
  onDeselect?: (path: string) => void;
}) {
  const card = (path: string) => (
    <article className="removal-card" key={path}>
      <span className="eyebrow">
        {onDeselect ? "Dateientfernung prüfen" : "Wird gelöscht"}
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
          <summary>{paths.length - 3} weitere Dateien anzeigen</summary>
          {paths.slice(3).map(card)}
        </details>
      )}
    </>
  );
}
