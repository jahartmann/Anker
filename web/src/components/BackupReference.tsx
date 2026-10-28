import { useEffect, useRef, useState } from "react";
import { api, date } from "../api";
import type { Backup, BackupReference, ReferenceChangeSummary } from "../api";
import { Dialog } from "./shared";
import type { Notify } from "./shared";

export default function BackupReferencePanel({
  hostID,
  hostName,
  backups,
  reference,
  canEdit,
  notify,
  refresh,
  onCompare,
}: {
  hostID: string;
  hostName: string;
  backups: Backup[];
  reference?: BackupReference;
  canEdit: boolean;
  notify: Notify;
  refresh: () => void;
  onCompare?: (referenceID: string, targetID: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [source, setSource] = useState("");
  const [name, setName] = useState("");
  const [reason, setReason] = useState("");
  const [testedAt, setTestedAt] = useState("");
  const [outcome, setOutcome] = useState("passed");
  const [evidence, setEvidence] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [summary, setSummary] = useState<ReferenceChangeSummary | null>(null);
  const [targetID, setTargetID] = useState("");
  const comparisonGeneration = useRef(0);
  const referenceState = useRef(reference?.state);
  referenceState.current = reference?.state;
  useEffect(() => {
    comparisonGeneration.current++;
    setSummary(null);
    setBusy(false);
  }, [
    reference?.backup_id,
    reference?.manifest_sha,
    reference?.updated_at,
    reference?.state,
  ]);
  const current = reference?.state === "valid" ? reference : null;
  const targetOptions = [...backups]
    .filter((b) => b.id !== current?.backup_id)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
  const target =
    targetOptions.find((b) => b.id === targetID) || targetOptions[0];
  function edit() {
    setSource(
      reference?.backup_id && backups.some((b) => b.id === reference.backup_id)
        ? reference.backup_id
        : backups[0]?.id || "",
    );
    setName(reference?.name || "");
    setReason(reference?.reason || "");
    setTestedAt("");
    setOutcome("passed");
    setEvidence("");
    setError("");
    setEditing(true);
  }
  async function save() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const drill =
        testedAt || evidence
          ? { tested_at: testedAt, outcome, evidence }
          : undefined;
      await api(`hosts/${encodeURIComponent(hostID)}/reference`, "PUT", {
        backup_id: source,
        name,
        reason,
        ...(drill ? { drill } : {}),
      });
      setEditing(false);
      notify("Referenz gespeichert");
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function clearDrill() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api(
        `hosts/${encodeURIComponent(hostID)}/reference/drill`,
        "DELETE",
      );
      setEditing(false);
      notify("Probenbericht entfernt");
      refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function compare() {
    if (!current || !target || !onCompare || busy) return;
    const generation = ++comparisonGeneration.current;
    setBusy(true);
    setError("");
    setSummary(null);
    try {
      const result = await api<ReferenceChangeSummary>(
        `hosts/${encodeURIComponent(hostID)}/reference/compare?to=${encodeURIComponent(target.id)}`,
      );
      if (
        result.reference.backup_id !== current.backup_id ||
        result.to_id !== target.id
      )
        throw new Error("Vergleich passt nicht zur Referenz.");
      if (
        generation !== comparisonGeneration.current ||
        referenceState.current !== "valid"
      )
        return;
      setSummary(result);
      onCompare(current.backup_id!, target.id);
    } catch (e) {
      if (generation === comparisonGeneration.current)
        setError((e as Error).message);
    } finally {
      if (generation === comparisonGeneration.current) setBusy(false);
    }
  }
  return (
    <section
      className="backup-reference"
      aria-label={`Referenz für ${hostName}`}
    >
      <div className="backup-reference-head">
        <div>
          <strong>{hostName}</strong>
          <p>
            {reference?.state === "valid"
              ? `Referenz: ${reference.name}`
              : reference?.state === "unresolved"
                ? `Referenz: ${reference.name} · nicht auflösbar`
                : "Keine benannte Referenz"}
          </p>
          {reference?.reason && (
            <p className="field-hint">Grund: {reference.reason}</p>
          )}
          {reference?.drill && (
            <p className="field-hint">
              Menschlicher Probenbericht:{" "}
              {reference.drill.outcome === "passed"
                ? "erfolgreich"
                : "fehlgeschlagen"}{" "}
              · {date(reference.drill.tested_at)} · {reference.drill.actor}. Von
              Anker nicht durchgeführt.
            </p>
          )}
          {reference?.state === "unresolved" && (
            <p className="field-hint">
              Gespeicherter Stand fehlt, ist beschädigt oder sein Manifest passt
              nicht mehr. Referenz prüfen und neu wählen.
            </p>
          )}
        </div>
        <div className="row-actions">
          {current && targetOptions.length > 1 && onCompare && (
            <label>
              Vergleichsstand
              <select
                aria-label={`Vergleichsstand für ${hostName}`}
                value={target?.id || ""}
                onChange={(e) => {
                  comparisonGeneration.current++;
                  setTargetID(e.target.value);
                  setSummary(null);
                  setBusy(false);
                }}
              >
                {targetOptions.map((b) => (
                  <option key={b.id} value={b.id}>
                    {date(b.created_at)} · {b.id.slice(0, 10)}
                  </option>
                ))}
              </select>
            </label>
          )}
          {canEdit && backups.length > 0 && (
            <button className="text-button" onClick={edit}>
              Referenz bearbeiten
            </button>
          )}
          {current && target && onCompare && (
            <button className="text-button" onClick={compare} disabled={busy}>
              Mit Referenz vergleichen
            </button>
          )}
        </div>
      </div>
      {summary && (
        <p className="field-hint" role="status">
          Seit Referenz: {summary.changed} wesentliche Änderungen ·{" "}
          {summary.content} Inhalt · {summary.metadata} Metadaten ·{" "}
          {summary.mtime_only} nur Änderungszeit.
        </p>
      )}
      {error && (
        <p className="notice warning" role="alert">
          {error}
        </p>
      )}
      {editing && (
        <Dialog
          topic="archive"
          title={`Referenz · ${hostName}`}
          onClose={() => setEditing(false)}
          busy={busy}
        >
          <p className="dialog-intro">
            Ein geschützter benannter Stand pro Host. Ein Name oder Schutz
            belegt keine erfolgreiche Offline-Probe.
          </p>
          <div className="form-grid">
            <label>
              Referenzstand
              <select
                aria-label="Referenzstand"
                value={source}
                onChange={(e) => setSource(e.target.value)}
              >
                {backups.map((b) => (
                  <option key={b.id} value={b.id}>
                    {date(b.created_at)} · {b.id.slice(0, 10)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Referenzname
              <input
                aria-label="Referenzname"
                value={name}
                maxLength={120}
                onChange={(e) => setName(e.target.value)}
              />
            </label>
            <label>
              Grund
              <textarea
                aria-label="Grund"
                value={reason}
                maxLength={1000}
                onChange={(e) => setReason(e.target.value)}
              />
            </label>
          </div>
          <h4>Manueller Probenbericht</h4>
          <p className="field-hint">
            Nur eigene Beobachtung mit Datum und Beleg eintragen. Anker prüft
            den externen Ablauf nicht.
          </p>
          {reference?.drill && source === reference.backup_id && (
            <p className="field-hint">
              Bisher: {date(reference.drill.tested_at)} ·{" "}
              {reference.drill.actor} · {reference.drill.evidence}
            </p>
          )}
          <div className="form-grid">
            <label>
              Datum der Probe (RFC3339)
              <input
                aria-label="Datum der Probe"
                placeholder="2026-10-09T10:00:00Z"
                value={testedAt}
                onChange={(e) => setTestedAt(e.target.value)}
              />
            </label>
            <label>
              Ergebnis laut Bericht
              <select
                aria-label="Ergebnis laut Bericht"
                value={outcome}
                onChange={(e) => setOutcome(e.target.value)}
              >
                <option value="passed">Erfolgreich</option>
                <option value="failed">Fehlgeschlagen</option>
              </select>
            </label>
            <label>
              Beleg
              <textarea
                aria-label="Beleg"
                value={evidence}
                maxLength={2000}
                onChange={(e) => setEvidence(e.target.value)}
              />
            </label>
          </div>
          {error && (
            <p className="notice warning" role="alert">
              {error}
            </p>
          )}
          <footer className="dialog-footer">
            {reference?.drill && (
              <button
                className="secondary"
                disabled={busy}
                onClick={clearDrill}
              >
                Probenbericht entfernen
              </button>
            )}
            <button disabled={busy || !source || !name.trim()} onClick={save}>
              Referenz speichern
            </button>
          </footer>
        </Dialog>
      )}
    </section>
  );
}
