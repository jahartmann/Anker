import { useListPageSize } from "../../listPageSize";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api";
import type { FileDecision, RecoveryPreview } from "../../api";
import FileComparison from "../FileComparison";
import RecoveryRecipes from "./RecoveryRecipes";
import RecoveryNetwork from "./RecoveryNetwork";
import RecoveryMapping from "../RecoveryMapping";
import { Field, Pagination } from "../shared";
import type { RecoveryDraft } from "./useRecoveryDraft";
const captureStates: Record<string, string> = {
  missing: "Am Ziel nicht vorhanden; die Datei wird neu angelegt.",
  unreadable: "Am Ziel nicht lesbar.",
  unsupported: "Am Ziel kein gewöhnliches File.",
  too_large: "Am Ziel zu groß für einen Vergleich.",
  changed: "Am Ziel während der Prüfung verändert.",
  limit: "Prüfgrenze erreicht; nicht verglichen.",
};
/** Says something only when the target file is not simply readable. */
function captureHint(state?: string) {
  return state === "read"
    ? undefined
    : captureStates[state || ""] || "Zielstand unbekannt.";
}

export default function RecoveryDecisions({
  draft: d,
  canReveal,
}: {
  draft: RecoveryDraft;
  canReveal: boolean;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [listSize, setListSize] = useListPageSize("recovery-decisions", 25);
  const rows = d.selectedPaths.filter((p) =>
    p.toLowerCase().includes(query.toLowerCase()),
  );
  const current = Math.min(
    page,
    Math.max(1, Math.ceil(rows.length / listSize)),
  );
  if (!d.inspection) return null;
  const open = d.openFiles + d.openMappings;
  const undecided = d.selectedPaths.filter(
    (p) => !d.decisions[p] && !d.removeFiles.includes(p),
  );
  // On other hardware the network and name files need the prepared version,
  // which carries the port and hostname mapping; on the same host the
  // original is exactly right.
  const sameHost =
    d.inspection.source.hostname === d.inspection.target.hostname;
  const adapted = (path: string) =>
    !sameHost &&
    (path.startsWith("etc/network/") ||
      ["etc/hosts", "etc/hostname", "etc/mailname"].includes(path));
  const adaptedOpen = undecided.filter(adapted).length;
  function decideOpen(decision: FileDecision) {
    d.setDecisions((old) => ({
      ...old,
      ...Object.fromEntries(
        undecided.map((p) => [
          p,
          decision === "backup" && adapted(p) ? "prepared" : decision,
        ]),
      ),
    }));
  }
  return (
    <section aria-label="Entscheidungen" data-surface-topic="recovery">
      <h2>3 · Entscheidungen</h2>
      <p>
        {open === 1 ? "1 offene Entscheidung" : `${open} offene Entscheidungen`}{" "}
        ·{" "}
        {d.selectedPaths.length === 1
          ? "1 Datei"
          : `${d.selectedPaths.length} Dateien`}
      </p>
      <RecoveryRecipes draft={d} />
      {!!(
        d.mappingInspection?.ports?.length ||
        d.mappingInspection?.storage?.length
      ) && (
        <RecoveryMapping
          inspection={d.mappingInspection!}
          interfaces={d.interfaces}
          storage={d.storage}
          onInterfaces={d.setInterfaces}
          onStorage={d.setStorage}
        />
      )}
      {d.duplicatePorts && (
        <p className="notice warning" role="alert">
          Mehrere Quellports sind demselben Zielport zugeordnet. Für jeden Port
          ein eigenes Ziel wählen.
        </p>
      )}
      {d.selectedPaths.some((p) =>
        ["etc/hostname", "etc/hosts", "etc/mailname"].includes(p),
      ) && (
        <Field
          label="Gewünschter Hostname"
          hint="Optional; wirkt auf vorbereitete Dateien."
        >
          <input
            value={d.hostname}
            onChange={(e) => d.setHostname(e.target.value)}
          />
        </Field>
      )}
      {<RecoveryNetwork draft={d} />}
      {!!d.mappingInspection?.ports?.length && (
        <Field
          label="Gewünschte Adresse (manuell)"
          hint="Adresswünsche benötigen einen manuellen Netzwerkschritt."
        >
          <input
            value={d.address}
            onChange={(e) => d.setAddress(e.target.value)}
          />
        </Field>
      )}
      {d.needsInspection && (
        <p className="notice">
          Speicherzuordnungen wurden geändert. „Zuordnungen prüfen“ erfasst den
          konkreten Zielbestand für diese Auswahl.
        </p>
      )}
      {d.inspection.source.host_type === "pbs" && (
        <p className="notice">
          Für Datastore- und Jobdateien die vorbereitete Fassung wählen: Sie
          behält die Anbindungen des Ziels und deaktiviert Zeitpläne.
        </p>
      )}
      <h3>Dateien entscheiden</h3>
      <p className="field-hint">
        Identische Dateien werden nicht erneut geschrieben.
      </p>
      {undecided.length > 0 && (
        <div className="recovery-decide-all">
          <button type="button" onClick={() => decideOpen("backup")}>
            {undecided.length === 1
              ? "Offene Datei aus Sicherung übernehmen"
              : `Alle ${undecided.length} offenen Dateien aus Sicherung übernehmen`}
          </button>
          <button
            type="button"
            className="secondary"
            onClick={() => decideOpen("manual")}
          >
            Offene Dateien manuell bearbeiten
          </button>
          {adaptedOpen > 0 && (
            <p className="field-hint">
              Anderer Zielhost: Netzwerk- und Namensdateien werden dabei in der
              vorbereiteten Fassung mit den Zuordnungen oben übernommen.
            </p>
          )}
        </div>
      )}
      <Field label="Dateientscheidungen durchsuchen">
        <input
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(1);
          }}
        />
      </Field>
      <div className="recovery-decision-files">
        {rows
          .slice((current - 1) * listSize, current * listSize)
          .map((path) => (
            <div key={path}>
              <Field
                label={"Entscheidung für " + path}
                hint={captureHint(
                  d.inspection?.files?.find((f) => f.path === path)?.state,
                )}
              >
                <select
                  value={d.decisions[path] || ""}
                  onChange={(e) =>
                    d.setDecisions((old) => ({
                      ...old,
                      [path]: e.target.value as FileDecision,
                    }))
                  }
                >
                  <option value="">Entscheidung auswählen</option>
                  {d.removeFiles.includes(path) ? (
                    <>
                      <option value="remove">
                        Datei ausdrücklich entfernen
                      </option>
                      <option value="keep">
                        Ziel behalten · nicht löschen
                      </option>
                    </>
                  ) : (
                    <>
                      <option value="backup">
                        Original aus Sicherung übernehmen
                      </option>
                      <option value="prepared">
                        Vorbereitete Fassung verwenden
                      </option>
                      <option value="keep">Ziel behalten</option>
                      <option value="manual">Manuell bearbeiten</option>
                    </>
                  )}
                </select>
              </Field>
              <DraftFileComparison
                key={d.previewKey + path}
                draft={d}
                path={path}
                canReveal={canReveal}
              />
            </div>
          ))}
      </div>
      <Pagination
        page={current}
        pageSize={listSize}
        onPageSizeChange={setListSize}
        total={rows.length}
        onPageChange={setPage}
        label="Dateientscheidungen"
      />
      {!!d.openFiles && (
        <p className="notice">Für jede Datei eine Entscheidung auswählen.</p>
      )}
      {!!d.openMappings && (
        <p className="notice">
          Fehlende Port- oder Speicherzuordnungen auswählen; ungeklärte Speicher
          manuell entscheiden.
        </p>
      )}
    </section>
  );
}

function DraftFileComparison({
  draft: d,
  path,
  canReveal,
}: {
  draft: RecoveryDraft;
  path: string;
  canReveal: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [basis, setBasis] = useState("source-target");
  const [result, setResult] = useState<RecoveryPreview | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const sequence = useRef(0);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
      sequence.current++;
    };
  }, []);
  async function load(nextBasis = basis, reveal = false) {
    const id = ++sequence.current;
    setOpen(true);
    setBasis(nextBasis);
    setResult(null);
    setError("");
    setLoading(true);
    try {
      const value = await api<RecoveryPreview>("plans/preview", "POST", {
        ...d.requestBody(true),
        manifest_sha: d.inspection?.manifest_sha,
        path,
        basis: nextBasis,
        reveal,
      });
      if (!active.current || id !== sequence.current) return;
      if (
        value.snapshot_id !== d.snapshotID ||
        value.manifest_sha !== d.inspection?.manifest_sha ||
        value.comparison.path !== path
      )
        throw new Error(
          "Vergleich gehört zu einem anderen Stand. Ziel erneut prüfen.",
        );
      setResult(value);
    } catch (e) {
      if (active.current && id === sequence.current)
        setError((e as Error).message);
    } finally {
      if (active.current && id === sequence.current) setLoading(false);
    }
  }
  const available = !!d.snapshotID && !!d.inspection?.manifest_sha;
  const actionLabel: Record<string, string> = {
    keep: "Ziel behalten · kein Schreibbedarf",
    apply: "Übernahme nach Planprüfung",
    remove: "Datei entfernen · vorher für Rücksetzung sichern",
    manual: "Manuell bearbeiten",
    unknown: "Noch nicht beurteilbar",
  };
  return (
    <div className="draft-file-comparison">
      <button
        type="button"
        className="text-button"
        disabled={!available}
        aria-expanded={open}
        onClick={() => {
          if (open) {
            sequence.current++;
            setOpen(false);
            setResult(null);
          } else void load();
        }}
      >
        Vergleich für {path}
      </button>
      {!available && (
        <p className="field-hint">
          Für den Dateivergleich das Ziel mit der aktuellen Auswahl und
          Zuordnung erneut prüfen.
        </p>
      )}
      {open && (
        <section aria-label={"Dateivergleich für " + path}>
          {!d.removeFiles.includes(path) && (
            <Field label="Vergleichsbasis">
              <select value={basis} onChange={(e) => void load(e.target.value)}>
                <option value="source-target">Original ↔ Zielaufnahme</option>
                <option value="source">Original ↔ Vorbereitet</option>
                <option value="target">Zielaufnahme ↔ Vorbereitet</option>
              </select>
            </Field>
          )}
          <p className="field-hint">
            {d.removeFiles.includes(path)
              ? "Aktuell gelesener Zielinhalt → nach Entfernung. Das Original wird vor dem Löschen für die Rücksetzung gesichert."
              : "Vorbereitete Fassung mit den aktuellen Zuordnungen. Zielstand aus der Prüfung; vor der Ausführung erneut prüfen."}
          </p>
          <FileComparison
            comparison={result?.comparison || null}
            loading={loading}
            error={error}
            retry={() => void load()}
          />
          {result && (
            <div className="notice">
              <strong>Voraussichtliche Aktion</strong>
              <p>
                {d.removeFiles.includes(path) ? "Entfernung: " : "Original: "}
                {actionLabel[result.original_action.action] ||
                  "Noch nicht beurteilbar"}
                . {result.original_action.reason}
              </p>
              {!d.removeFiles.includes(path) && (
                <p>
                  Vorbereitet:{" "}
                  {actionLabel[result.prepared_action.action] ||
                    "Noch nicht beurteilbar"}
                  . {result.prepared_action.reason}
                </p>
              )}
            </div>
          )}
          {canReveal && result?.comparison.masked && (
            <button
              type="button"
              className="secondary"
              onClick={() => void load(basis, true)}
            >
              Geschützten Vergleich anzeigen
            </button>
          )}
        </section>
      )}
    </div>
  );
}
