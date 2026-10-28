import { useEffect, useState } from "react";
import { api } from "../../api";
import type { RecoveryRecipe, Plan } from "../../api";
import { Field } from "../shared";
import type { RecoveryDraft } from "./useRecoveryDraft";

export default function RecoveryRecipes({
  draft: d,
}: {
  draft: RecoveryDraft;
}) {
  const [recipes, setRecipes] = useState<RecoveryRecipe[]>([]);
  const [selected, setSelected] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    api<RecoveryRecipe[]>("recovery-recipes")
      .then((rows) => {
        if (live) setRecipes(rows);
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    return () => {
      live = false;
    };
  }, []);
  const result = d.recipePreview;
  if (!recipes.length && !error) return null;
  return (
    <section
      className="recovery-recipes"
      data-surface-topic="network"
      aria-label="Geprüfte Zuordnungen"
    >
      <h3>Zuordnungsvorlage</h3>
      <p className="field-hint">
        Geprüfte Zuordnungen mit dem aktuellen Ziel vergleichen. Dateien,
        Konsole und Quelltrennung anschließend neu entscheiden.
      </p>
      <Field label="Zuordnungsvorlage">
        <select
          value={selected}
          onChange={(e) => {
            setSelected(e.target.value);
            d.clearRecipe();
          }}
        >
          <option value="">Vorlage auswählen</option>
          {recipes.map((r) => (
            <option key={r.id} value={r.id}>
              {r.label}
            </option>
          ))}
        </select>
      </Field>
      <button
        className="secondary"
        disabled={!selected || d.recipeBusy || !d.snapshotID}
        onClick={() => d.previewRecipe(selected)}
      >
        Vorlage frisch prüfen
      </button>
      {d.recipeBusy && <p role="status">Zuordnungen werden geprüft …</p>}
      {d.needsInspection && (
        <p className="notice">
          Ziel für die geänderten Speicherzuordnungen erneut prüfen.
        </p>
      )}
      {result && (
        <div role="status">
          <p>
            Geprüft von {result.recipe.reviewed_by} ·{" "}
            {new Date(result.recipe.reviewed_at).toLocaleString("de-DE")} · Plan{" "}
            <code>{result.recipe.origin_plan_id}</code>
          </p>
          {result.applicable ? (
            <>
              <p>Die geprüften Zuordnungen passen zum aufgenommenen Ziel.</p>
              <button
                className="secondary"
                disabled={d.recipeBusy}
                onClick={() => d.previewRecipe(selected, true)}
              >
                Geprüfte Zuordnungen einsetzen
              </button>
            </>
          ) : (
            <>
              <p className="notice warning">
                Vorlage passt nicht. Zuordnungen neu entscheiden.
              </p>
              <ul>
                {result.mismatches.map((m) => (
                  <li key={m}>{m}</li>
                ))}
              </ul>
            </>
          )}
        </div>
      )}
      {error && (
        <p className="notice warning" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

export function SaveRecoveryRecipe({
  planID,
  mapping,
  disabled,
}: {
  planID: string;
  mapping: Plan["mapping"];
  disabled: boolean;
}) {
  const [label, setLabel] = useState("");
  const [reviewed, setReviewed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  useEffect(() => {
    setLabel("");
    setReviewed(false);
    setMessage("");
  }, [planID]);
  return (
    <section
      className="recovery-recipes"
      data-surface-topic="network"
      aria-label="Zuordnungsvorlage speichern"
    >
      <h3>Zuordnungen wiederverwenden</h3>
      <p className="field-hint">
        Speichert die Zuordnungen dieses Plans mit deinem Namen und Prüfdatum.
        Beim Laden ist eine neue Zielprüfung erforderlich.
      </p>
      <ul>
        {Object.entries(mapping?.interfaces || {}).map(([a, b]) => (
          <li key={"port" + a}>
            Netzwerkport: {a} → {b}
          </li>
        ))}
        {Object.entries(mapping?.storage || {}).map(([a, b]) => (
          <li key={"storage" + a}>
            Speicher: {a} → {b}
          </li>
        ))}
        {mapping?.hostname && <li>Hostname: {mapping.hostname}</li>}
        {mapping?.address && <li>Adresse: {mapping.address}</li>}
        {Object.entries(mapping?.network || {}).map(([key, n]) => (
          <li key={"network" + key}>
            {key}: {n.address_cidr || "Adresse erhalten"} ·{" "}
            {n.gateway || "Gateway erhalten"} · DNS:{" "}
            {n.dns_servers?.join(", ") || "erhalten"}
          </li>
        ))}
      </ul>
      <Field label="Bezeichnung der Vorlage">
        <input
          maxLength={120}
          value={label}
          disabled={disabled || busy}
          onChange={(e) => setLabel(e.target.value)}
        />
      </Field>
      <label className="check">
        <input
          type="checkbox"
          checked={reviewed}
          disabled={disabled || busy}
          onChange={(e) => setReviewed(e.target.checked)}
        />
        Ich habe die Zuordnungen dieses Plans geprüft.
      </label>
      <button
        className="secondary"
        disabled={disabled || busy || !reviewed || !label.trim()}
        onClick={async () => {
          setBusy(true);
          setMessage("");
          try {
            await api("recovery-recipes", "POST", {
              plan_id: planID,
              label: label.trim(),
              reviewed: true,
            });
            setMessage("Zuordnungsvorlage gespeichert.");
            setReviewed(false);
          } catch (e) {
            setMessage((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        Als geprüfte Vorlage speichern
      </button>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
