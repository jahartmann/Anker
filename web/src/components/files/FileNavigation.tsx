import { useEffect, useState } from "react";
import { ChevronRight } from "lucide-react";
import { parentPath } from "./manifest";

export function useFileNavigation(source: string) {
  const [directory, setDirectory] = useState("");
  const [query, setQuery] = useState("");
  useEffect(() => {
    setDirectory("");
    setQuery("");
  }, [source]);
  function navigate(path: string) {
    setDirectory(path);
    setQuery("");
  }
  return {
    directory,
    query,
    setQuery,
    navigate,
    up: () => navigate(parentPath(directory)),
  };
}

export default function FileNavigation({
  directory,
  query,
  navigate,
  up,
  scopeLabel,
}: {
  directory: string;
  query: string;
  navigate: (path: string) => void;
  up: () => void;
  scopeLabel?: string;
}) {
  const parts = directory ? directory.split("/") : [];
  return (
    <div className="file-navigation">
      <nav className="breadcrumbs" aria-label="Dateipfad">
        <button
          type="button"
          className="text-button"
          onClick={() => navigate("")}
          aria-current={!directory && !query.trim() ? "page" : undefined}
        >
          Alle Dateien
        </button>
        {parts.map((part, index) => (
          <span key={index}>
            <ChevronRight size={13} aria-hidden="true" />
            <button
              type="button"
              className="text-button"
              onClick={() => navigate(parts.slice(0, index + 1).join("/"))}
              aria-current={
                index === parts.length - 1 && !query.trim() ? "page" : undefined
              }
            >
              {part}
            </button>
          </span>
        ))}
        {query.trim() && <span className="muted">Suche in allen Ordnern</span>}
      </nav>
      <div className="file-location">
        <span>
          {scopeLabel || (
            <>
              Aktueller Ordner:{" "}
              <strong className="mono">{directory || "/"}</strong>
            </>
          )}
        </span>
        <button
          type="button"
          className="text-button"
          onClick={up}
          disabled={!directory}
        >
          Übergeordneter Ordner
        </button>
      </div>
    </div>
  );
}
