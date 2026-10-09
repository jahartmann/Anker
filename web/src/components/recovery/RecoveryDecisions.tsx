import { useState } from "react";
import type { FileDecision } from "../../api";
import RecoveryNetwork from "./RecoveryNetwork";
import RecoveryMapping from "../RecoveryMapping";
import { Field, Pagination } from "../shared";
import type { RecoveryDraft } from "./useRecoveryDraft";
export default function RecoveryDecisions({
  draft: d,
}: {
  draft: RecoveryDraft;
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
          hint="Optional für vorbereitete Dateien; Änderungen werden erst im gespeicherten Plan geprüft."
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
      <h3>Dateien entscheiden</h3>
      <p className="field-hint">
        Original übernehmen, vorbereitete Fassung prüfen, Ziel behalten oder
        manuell bearbeiten. Geschützte Dateien bleiben manuell. Identische
        Inhalte und Schreibmetadaten werden nicht erneut geschrieben.
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
          <Field
            key={path}
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
              <option value="backup">Original aus Sicherung übernehmen</option>
              <option value="prepared">Vorbereitete Fassung verwenden</option>
              <option value="keep">Ziel behalten</option>
              <option value="manual">Manuell bearbeiten</option>
            </select>
          </Field>
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
                d.selectedPaths.map((p) => [p, old[p] || ("manual" as const)]),
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
