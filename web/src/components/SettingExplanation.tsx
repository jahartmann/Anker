import ChangeRestorePreview from "./ChangeRestorePreview";
import { useEffect, useState } from "react";
import { Lightbulb, FileCheck2, ExternalLink } from "lucide-react";
import { api, preciseDate } from "../api";
import type { Entry } from "../api";
import type { SettingObservation } from "../settingHistory";
import { settingChoices } from "../settingHistory";
import { explainSettingChange } from "../settingExplanations";
import { Dialog, LoadError } from "./shared";

function EvidenceFile({
  observation,
  path,
  settingKey,
  onClose,
}: {
  observation: SettingObservation;
  path: string;
  settingKey: string;
  onClose: () => void;
}) {
  const [retry, setRetry] = useState(0);
  const [result, setResult] = useState<{
    entry: Entry;
    content: string;
    masked: boolean;
  } | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    setResult(null);
    setError("");
    api<{ entry: Entry; content: string; masked: boolean }>(
      `backups/${encodeURIComponent(observation.backup_id)}/file?path=${encodeURIComponent(path)}`,
      "GET",
      undefined,
      controller.signal,
    )
      .then((r) => {
        if (!controller.signal.aborted) setResult(r);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError((e as Error).message);
      });
    return () => controller.abort();
  }, [observation.backup_id, path, retry]);
  const protectedContent = result?.masked || result?.entry.secret;
  const matchingLines =
    !result || protectedContent
      ? []
      : result.content
          .split("\n")
          .map((text, index) => ({ text, number: index + 1 }))
          .filter((line) => {
            const trimmed = line.text.trim();
            if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith(";"))
              return false;
            if (settingKey === "hostname") return true;
            if (settingKey === "nameserver")
              return trimmed.split(/\s+/)[0] === "nameserver";
            return (
              trimmed
                .replace(/^-/, "")
                .split("=")[0]
                .trim()
                .replaceAll("/", ".") === settingKey
            );
          })
          .slice(0, 8);
  return (
    <Dialog title="Gespeicherter Nachweis" onClose={onClose} wide>
      <div className="setting-proof-file">
        <strong>{path}</strong>
        <time dateTime={observation.created_at}>
          {preciseDate(observation.created_at)}
        </time>
        <code>{observation.backup_id}</code>
      </div>
      {error ? (
        <LoadError
          message={error}
          retry={() => setRetry((n) => n + 1)}
          label="Quelldatei erneut lesen"
        />
      ) : !result ? (
        <p role="status">Quelldatei wird gelesen …</p>
      ) : protectedContent ? (
        <p className="notice warning">
          Geschützter Inhalt · dieser Nachweis wird nicht offengelegt.
        </p>
      ) : (
        <>
          <div
            className="setting-proof-matches"
            aria-label="Passende Deklarationen"
          >
            <strong>Treffer für die Einstellung</strong>
            {matchingLines.length ? (
              matchingLines.map((line) => (
                <div key={line.number}>
                  <span>Zeile {line.number}</span>
                  <code>
                    {line.text.slice(0, 2048)}
                    {line.text.length > 2048 ? " …" : ""}
                  </code>
                </div>
              ))
            ) : (
              <p>
                Keine passende Deklaration gefunden. Die Quelldatei und den
                ausgewählten Nachweis prüfen.
              </p>
            )}
          </div>
          <details className="setting-proof-full">
            <summary>Textvorschau mit weiterem Kontext</summary>
            <pre className="setting-proof-content">
              {result.content.slice(0, 12000)}
            </pre>
            {result.content.length > 12000 && (
              <p className="muted">Vorschau auf 12.000 Zeichen begrenzt.</p>
            )}
          </details>
          <p className="setting-proof-scope">
            Das ist die Datei aus der Sicherung. Ob der laufende Host diese
            Werte verwendet, wurde nicht geprüft.
          </p>
        </>
      )}
    </Dialog>
  );
}
export default function SettingExplanation({
  id,
  before,
  after,
}: {
  id: string;
  before: SettingObservation;
  after: SettingObservation;
}) {
  const explanation = explainSettingChange(id, before, after);
  const [source, setSource] = useState<{
    observation: SettingObservation;
    path: string;
  } | null>(null);
  const [restorePreview, setRestorePreview] = useState(false);
  const evidenceKey = JSON.stringify([id, before, after]);
  // A changed time selection also closes any source view belonging to the old pair.
  useEffect(() => {
    setSource(null);
    setRestorePreview(false);
  }, [evidenceKey]);
  if (!explanation) return null;
  return (
    <section
      className="setting-explanation"
      aria-label="Bedeutung der Änderung"
    >
      <div className="setting-explanation-heading">
        <Lightbulb size={15} aria-hidden="true" />
        <strong>Was bedeutet das?</strong>
      </div>
      <p>{explanation.meaning}</p>
      <div className="setting-impact">
        <span>Mögliche Auswirkung</span>
        <p>{explanation.impact}</p>
      </div>
      <details className="setting-explanation-evidence">
        <summary>
          <FileCheck2 size={14} aria-hidden="true" /> Dateien und Erklärung
          ansehen
        </summary>
        <p className="setting-explanation-scope">
          Die Werte stammen aus diesen Sicherungen. Die Auswirkungen am
          laufenden Host wurden nicht geprüft.
        </p>
        <div className="setting-proof-sides">
          {(
            [
              ["Vorher", before],
              ["Nachher", after],
            ] as const
          ).map(([label, o]) => (
            <div key={label}>
              <span>
                {label} ·{" "}
                <time dateTime={o.created_at}>{preciseDate(o.created_at)}</time>
              </span>
              <code>{o.backup_id}</code>
              <strong>Gespeicherter Wert: {o.value}</strong>
              <div className="setting-proof-paths">
                {o.sources.map((p) => (
                  <button
                    className="text-button"
                    key={p}
                    aria-label={`Quelldatei ${label.toLowerCase()} ansehen: ${p}`}
                    onClick={() => setSource({ observation: o, path: p })}
                  >
                    <code>{p}</code>
                    <ExternalLink size={12} aria-hidden="true" />
                  </button>
                ))}
              </div>
            </div>
          ))}
        </div>
        <div className="setting-explanation-check">
          <strong>Am Host prüfen</strong>
          <p>{explanation.check}</p>
        </div>
        <a
          className="setting-documentation"
          href={explanation.documentation.url}
          target="_blank"
          rel="noreferrer"
        >
          Fachliche Grundlage: {explanation.documentation.title}{" "}
          <ExternalLink size={12} aria-hidden="true" />
        </a>
      </details>
      <button
        className="secondary setting-rollback-trigger"
        onClick={() => setRestorePreview(true)}
      >
        Wiederherstellungsvorschau
      </button>
      {restorePreview && (
        <ChangeRestorePreview
          key={evidenceKey}
          settingID={id}
          source={before}
          baseline={after}
          onClose={() => setRestorePreview(false)}
        />
      )}
      {source && (
        <EvidenceFile
          key={evidenceKey + source.path + source.observation.backup_id}
          {...source}
          settingKey={settingChoices.find((s) => s.id === id)!.key}
          onClose={() => setSource(null)}
        />
      )}
    </section>
  );
}
