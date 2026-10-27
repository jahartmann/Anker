import { useEffect, useRef, useState } from "react";
import { api } from "../../api";
import type { FileDecision, RecoveryPreview } from "../../api";
import FileComparison from "../FileComparison";
import RecoveryRecipes from "./RecoveryRecipes";
import RecoveryNetwork from "./RecoveryNetwork";
import RecoveryMapping from "../RecoveryMapping";
import { Field, Pagination } from "../shared";
import type { RecoveryDraft } from "./useRecoveryDraft";
export default function RecoveryDecisions({
  draft: d,
  canReveal,
}: {
  draft: RecoveryDraft;
  canReveal: boolean;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const rows = d.selectedPaths.filter((p) =>
    p.toLowerCase().includes(query.toLowerCase()),
  );
  const current = Math.min(page, Math.max(1, Math.ceil(rows.length / 25)));
  if (!d.inspection) return null;
  return (
    <section aria-label="Entscheidungen">
      <h2>3 · Entscheidungen</h2>
      <p>
        {d.openFiles + d.openMappings} offene Entscheidungen ·{" "}
        {d.selectedPaths.length} Dateien
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
          hint="Optional für vorbereitete Dateien; den Vergleich nach Änderungen erneut öffnen."
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
          Für Datastore- und Jobdateien die vorbereitete Fassung verwenden:
          vorhandene Ziel-Anbindungen bleiben erhalten und automatische Auslöser
          werden deaktiviert. Quelldateien mit aktiven Zeitplänen werden nicht
          automatisch übernommen.
        </p>
      )}
      <h3>Dateien entscheiden</h3>
      <p className="field-hint">
        Original übernehmen, vorbereitete Fassung prüfen, Ziel behalten oder
        manuell bearbeiten. Nicht freigegebene Systemdateien bleiben manuell.
        Identische Inhalte und Schreibmetadaten werden nicht erneut geschrieben.
      </p>
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
        {rows.slice((current - 1) * 25, current * 25).map((path) => (
          <div key={path}>
            <DraftFileComparison
              key={d.previewKey + path}
              draft={d}
              path={path}
              canReveal={canReveal}
            />
            <Field
              label={"Entscheidung für " + path}
              hint={
                "Zielaufnahme: " +
                ({
                  read: "gelesen",
                  missing: "nicht vorhanden",
                  unreadable: "nicht lesbar",
                  unsupported: "nicht unterstützt",
                  too_large: "zu groß",
                  changed: "während Prüfung verändert",
                  limit: "Prüfgrenze erreicht",
                }[
                  d.inspection?.files?.find((f) => f.path === path)?.state || ""
                ] || "unbekannt")
              }
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
                    <option value="remove">Datei ausdrücklich entfernen</option>
                    <option value="keep">Ziel behalten · nicht löschen</option>
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
          </div>
        ))}
      </div>
      <Pagination
        page={current}
        pageSize={25}
        total={rows.length}
        onPageChange={setPage}
        label="Dateientscheidungen"
      />
      {!!d.openFiles && (
        <button
          type="button"
          className="secondary"
          onClick={() =>
            d.setDecisions((old) => ({
              ...old,
              ...Object.fromEntries(
                d.selectedPaths
                  .filter((p) => !d.removeFiles.includes(p))
                  .map((p) => [p, old[p] || ("manual" as const)]),
              ),
            }))
          }
        >
          Offene Dateien manuell bearbeiten
        </button>
      )}
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
        className="secondary"
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
