import { useState } from "react";
import type { RecoveryInspection } from "../api";
import { Field, Pagination } from "./shared";

export default function RecoveryMapping({
  inspection,
  interfaces,
  storage,
  onInterfaces,
  onStorage,
}: {
  inspection: RecoveryInspection;
  interfaces: Record<string, string>;
  storage: Record<string, string>;
  onInterfaces: (value: Record<string, string>) => void;
  onStorage: (value: Record<string, string>) => void;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [portQuery, setPortQuery] = useState("");
  const rows = [
    ...(inspection.ports || []).map((p) => ({
      key: p.name,
      kind: "port",
      reason: p.reason,
      suggested: p.suggested,
    })),
    ...(inspection.storage || []).map((s) => ({
      key: s.id,
      kind: "storage",
      reason: s.reason,
      suggested: s.suggested,
    })),
  ];
  const matches = rows.filter((r) =>
    (r.key + " " + r.reason).toLowerCase().includes(query.toLowerCase()),
  );
  const current = Math.min(page, Math.max(1, Math.ceil(matches.length / 8)));
  if (!rows.length)
    return (
      <p className="field-hint">
        Für diese Auswahl sind keine Port- oder Speicherzuordnungen
        erforderlich.
      </p>
    );
  return (
    <section className="recovery-mapping">
      <h3>Benötigte Zuordnungen</h3>
      <p className="field-hint">
        {inspection.ports?.length || 0} verwendete Ports ·{" "}
        {inspection.storage?.length || 0} Speicherreferenzen. Vorschläge vor dem
        Plan prüfen.
      </p>
      {!!inspection.storage?.length && (
        <p className="notice">
          Speicherzuordnungen werden als manuelle Entscheidungen dokumentiert.
          Anker formatiert oder verschiebt keine Datenträger.
        </p>
      )}
      {rows.length > 8 && (
        <Field label="Zuordnungen durchsuchen">
          <input
            value={query}
            placeholder="Port oder Speicher-ID suchen"
            onChange={(e) => {
              setQuery(e.target.value);
              setPage(1);
            }}
          />
        </Field>
      )}
      {(inspection.target_ports?.length || 0) > 20 && (
        <Field
          label="Zielports durchsuchen"
          hint="Gesuchte Ports und bereits gewählte Ports bleiben verfügbar."
        >
          <input
            value={portQuery}
            onChange={(e) => setPortQuery(e.target.value)}
            placeholder="Name, MAC oder PCI"
          />
        </Field>
      )}
      {matches.slice((current - 1) * 8, current * 8).map((row) => {
        const ports = row.kind === "port";
        const value = (ports ? interfaces : storage)[row.key] || "";
        const candidates = ports
          ? (inspection.target_ports || [])
              .filter(
                (p) =>
                  p.name === value ||
                  (p.name + " " + p.mac + " " + (p.pci || ""))
                    .toLowerCase()
                    .includes(portQuery.toLowerCase()),
              )
              .slice(0, 50)
              .map((p) => ({
                id: p.name,
                label: p.name + (p.mac ? " · " + p.mac : ""),
              }))
          : (inspection.target_storage || []).map((s) => ({
              id: s.id,
              label: s.id + (s.path ? " · " + s.path : ""),
            }));
        // A selected port remains in the bounded list even after a search changes.
        if (ports && value && !candidates.some((c) => c.id === value)) {
          const selected = inspection.target_ports.find(
            (p) => p.name === value,
          );
          if (selected)
            candidates.push({
              id: value,
              label: value + (selected.mac ? " · " + selected.mac : ""),
            });
        }
        return (
          <Field
            key={row.kind + row.key}
            label={
              row.key + (ports ? " → Zielport" : " → Zielspeicher (manuell)")
            }
            hint={[
              row.reason,
              row.suggested && value === row.suggested
                ? "Vorschlag aus Zielprüfung"
                : "",
            ]
              .filter(Boolean)
              .join(" · ")}
          >
            <select
              value={value}
              required
              onChange={(e) =>
                ports
                  ? onInterfaces({ ...interfaces, [row.key]: e.target.value })
                  : onStorage({ ...storage, [row.key]: e.target.value })
              }
            >
              <option value="">Zuordnung auswählen</option>
              {!ports && <option value="manual">Manuell entscheiden</option>}
              {candidates.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.label}
                </option>
              ))}
            </select>
          </Field>
        );
      })}
      {!matches.length && <p className="muted">Keine passenden Zuordnungen.</p>}
      {rows.length > 8 && (
        <Pagination
          page={current}
          pageSize={8}
          total={matches.length}
          onPageChange={setPage}
          label="Zuordnungen"
        />
      )}
      {(inspection.target_ports?.length || 0) > 50 && (
        <p className="field-hint">
          Bis zu 50 passende Zielports werden angezeigt. Suche eingrenzen, um
          weitere Ports auszuwählen.
        </p>
      )}
    </section>
  );
}
