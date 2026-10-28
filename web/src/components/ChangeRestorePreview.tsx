import { ChangeRecoveryContext } from "./recovery/ChangeRecoveryIntent";
import { useContext, useEffect, useState } from "react";
import { ArrowRight, Files, RotateCcw, ShieldCheck } from "lucide-react";
import { api, preciseDate } from "../api";
import type { FileComparison } from "../api";
import { settingValue } from "../settingHistory";
import type {
  SettingObservation,
  SettingHistoryResult,
} from "../settingHistory";
import { Dialog, LoadError, Pagination } from "./shared";
import FileComparisonView from "./FileComparison";
interface ValueChange {
  setting: SettingHistoryResult["setting"];
  before: SettingObservation;
  after: SettingObservation;
}
export interface ChangeRestorePreviewResult {
  basis: "saved-backup";
  source: { id: string; created_at: string; manifest_sha: string };
  baseline: { id: string; created_at: string; manifest_sha: string };
  selected: ValueChange;
  additional: ValueChange[];
  files: { path: string; action: string; comparison: FileComparison }[];
}
const actions: Record<string, string> = {
  replace: "Datei ersetzen",
  create: "Datei wieder anlegen",
  remove: "Datei entfernen",
  keep: "Datei behalten",
  metadata: "Eigenschaften ändern",
};
function ValueCard({
  value,
  additional = false,
}: {
  value: ValueChange;
  additional?: boolean;
}) {
  return (
    <article
      className={
        "rollback-value-card" + (additional ? " rollback-additional" : "")
      }
    >
      <span className="eyebrow">
        {additional ? "Zusätzlich im Dateiumfang" : "Gewünschte Rückkehr"}
      </span>
      <h4>{value.setting.name}</h4>
      <div className="rollback-values">
        <div>
          <small>Im gewählten gesicherten Stand</small>
          <strong>{settingValue(value.setting.id, value.before.value)}</strong>
        </div>
        <ArrowRight size={17} aria-hidden="true" />
        <div>
          <small>Aus dem früheren Quellstand</small>
          <strong>{settingValue(value.setting.id, value.after.value)}</strong>
        </div>
      </div>
      <code>
        {value.setting.key}: {value.before.value} → {value.after.value}
      </code>
    </article>
  );
}
function FileCard({
  file,
  selected,
  onSelect,
}: {
  selected: boolean;
  onSelect?: (checked: boolean) => void;
  file: ChangeRestorePreviewResult["files"][number];
}) {
  const [open, setOpen] = useState(false);
  const additions = file.comparison.hunks
    .flatMap((h) => h.lines)
    .filter((l) => l.kind === "add").length;
  const removals = file.comparison.hunks
    .flatMap((h) => h.lines)
    .filter((l) => l.kind === "remove").length;
  return (
    <article className="rollback-file-card">
      {onSelect && (
        <label className="check rollback-file-select">
          <input
            type="checkbox"
            aria-label={`Für Assistenten auswählen: ${file.path}`}
            checked={selected}
            onChange={(e) => onSelect(e.target.checked)}
          />
          {file.action === "remove"
            ? "Entfernung im Assistenten prüfen"
            : "Im Assistenten vorbereiten"}
        </label>
      )}
      <div className="rollback-file-heading">
        <Files size={17} aria-hidden="true" />
        <strong>{file.path}</strong>
        <span data-action={file.action}>
          {actions[file.action] || "Manuell prüfen"}
        </span>
      </div>
      {file.action === "remove" && (
        <p className="rollback-file-warning">
          Im früheren Quellstand fehlt diese Datei. Die Rückkehr würde eine
          Entfernung erfordern. Sie ist zunächst abgewählt und benötigt im
          Assistenten eine eigene Entscheidung und Bestätigung. Das Original
          wird vor dem Löschen zur Rücksetzung gesichert.
        </p>
      )}
      {(additions > 0 || removals > 0) && (
        <p className="rollback-file-counts">
          {removals} {removals === 1 ? "Zeile entfernt" : "Zeilen entfernt"} ·{" "}
          {additions}{" "}
          {additions === 1 ? "Zeile hinzugefügt" : "Zeilen hinzugefügt"}
        </p>
      )}
      {!!file.comparison.metadata_changes.filter((m) => m.field !== "mtime")
        .length && (
        <p className="rollback-file-counts">
          Auch Dateieigenschaften unterscheiden sich. Details vor einer
          Übernahme prüfen.
        </p>
      )}
      <details
        open={open}
        onToggle={(e) => setOpen(e.currentTarget.open)}
        className="rollback-file-diff"
      >
        <summary>Alle Änderungen dieser Datei ansehen</summary>
        {open && (
          <FileComparisonView
            comparison={file.comparison}
            loading={false}
            error=""
            retry={() => {}}
            collapseContext
          />
        )}
      </details>
    </article>
  );
}
export default function ChangeRestorePreview({
  settingID,
  source,
  baseline,
  onClose,
}: {
  settingID: string;
  source: SettingObservation;
  baseline: SettingObservation;
  onClose: () => void;
}) {
  const prepare = useContext(ChangeRecoveryContext);
  const [selectedFiles, setSelectedFiles] = useState<string[]>([]);
  const [retry, setRetry] = useState(0);
  const [data, setData] = useState<ChangeRestorePreviewResult | null>(null);
  const [error, setError] = useState("");
  const [page, setPage] = useState(1);
  useEffect(() => {
    const controller = new AbortController();
    setData(null);
    setSelectedFiles([]);
    setError("");
    setPage(1);
    const query = new URLSearchParams({
      baseline: baseline.backup_id,
      setting: settingID,
      expected_source: source.value || "",
      expected_baseline: baseline.value || "",
    });
    api<ChangeRestorePreviewResult>(
      `backups/${encodeURIComponent(source.backup_id)}/change-restore-preview?${query}`,
      "GET",
      undefined,
      controller.signal,
    )
      .then((r) => {
        if (controller.signal.aborted) return;
        if (
          r.basis !== "saved-backup" ||
          r.source.id !== source.backup_id ||
          r.baseline.id !== baseline.backup_id ||
          r.selected.setting.id !== settingID ||
          r.selected.before.state !== "value" ||
          r.selected.after.state !== "value" ||
          r.selected.before.backup_id !== baseline.backup_id ||
          r.selected.after.backup_id !== source.backup_id ||
          r.files.length === 0 ||
          r.files.some(
            (f) =>
              !Object.hasOwn(actions, f.action) ||
              f.comparison.path !== f.path ||
              f.comparison.masked ||
              ["masked", "unavailable", "binary", "too_large"].includes(
                f.comparison.state,
              ),
          ) ||
          r.additional.some(
            (v) => v.before.state !== "value" || v.after.state !== "value",
          ) ||
          r.selected.before.value !== baseline.value ||
          r.selected.after.value !== source.value
        )
          throw new Error(
            "Vorschau passt nicht mehr zur gewählten Änderung. Änderung erneut laden.",
          );
        setData(r);
        setSelectedFiles(
          r.files
            .filter((f) => f.action !== "keep" && f.action !== "remove")
            .map((f) => f.path),
        );
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError((e as Error).message);
      });
    return () => controller.abort();
  }, [
    settingID,
    source.backup_id,
    source.value,
    baseline.backup_id,
    baseline.value,
    retry,
  ]);
  const affected = data?.files.filter((f) => f.action !== "keep").length || 0;
  return (
    <Dialog
      topic="recovery"
      title="Wiederherstellungsvorschau"
      onClose={onClose}
      wide
    >
      <div className="rollback-preview" data-surface-topic="recovery">
        <div className="rollback-intro">
          <span className="eyebrow">
            <RotateCcw size={14} aria-hidden="true" /> Rückkehr zum früheren
            Stand
          </span>
          <p>
            Du siehst, was sich bei einer Rückkehr gegenüber dem gewählten
            Sicherungsstand ändern würde.
          </p>
        </div>
        <p className="rollback-readonly">
          <ShieldCheck size={15} aria-hidden="true" /> Nur Vorschau · keine
          Übernahme und kein Auftrag
        </p>
        {error ? (
          <LoadError
            message={error}
            retry={() => setRetry((n) => n + 1)}
            label="Wiederherstellungsvorschau erneut laden"
          />
        ) : !data ? (
          <p role="status">Dateiumfang und Werte werden geprüft …</p>
        ) : (
          <>
            <ValueCard value={data.selected} />
            {!!data.additional.length && (
              <section
                className="rollback-additional-section"
                aria-label="Zusätzlich betroffene Werte"
              >
                <h3>Diese Werte wären ebenfalls betroffen</h3>
                <p>
                  Eine ganze Datei enthält oft mehr als die gewählte
                  Einstellung.
                </p>
                <div className="rollback-extra-values">
                  {data.additional.map((v) => (
                    <ValueCard key={v.setting.id} value={v} additional />
                  ))}
                </div>
              </section>
            )}
            <section className="rollback-files" aria-label="Betroffene Dateien">
              <div className="rollback-section-heading">
                <h3>
                  {affected}{" "}
                  {affected === 1
                    ? "Datei würde geändert"
                    : "Dateien würden geändert"}
                </h3>
                <span>
                  {data.files.length}{" "}
                  {data.files.length === 1
                    ? "Datei geprüft"
                    : "Dateien geprüft"}
                </span>
              </div>
              <p className="rollback-scope">
                Der Umfang umfasst ganze Dateien: abweichende Textstellen,
                Dateirechte und weitere Eigenschaften. Die einzelnen
                Unterschiede stehen in der Dateivorschau.
              </p>
              {data.files.slice((page - 1) * 4, page * 4).map((file) => (
                <FileCard
                  key={data.source.id + file.path}
                  file={file}
                  selected={selectedFiles.includes(file.path)}
                  onSelect={
                    prepare && file.action !== "keep"
                      ? (checked) =>
                          setSelectedFiles((old) =>
                            checked
                              ? [...old, file.path]
                              : old.filter((p) => p !== file.path),
                          )
                      : undefined
                  }
                />
              ))}
              {data.files.length > 4 && (
                <Pagination
                  page={page}
                  pageSize={4}
                  total={data.files.length}
                  onPageChange={setPage}
                  label="Vorschaudateien"
                />
              )}
            </section>
            <details className="rollback-source-proof">
              <summary>Quell- und Vergleichsstand prüfen</summary>
              <div>
                {(
                  [
                    ["Verglichen mit", data.baseline],
                    ["Früherer Quellstand", data.source],
                  ] as const
                ).map(([label, point]) => (
                  <div key={label}>
                    <strong>{label}</strong>
                    <time dateTime={point.created_at}>
                      {preciseDate(point.created_at)}
                    </time>
                    <code>{point.id}</code>
                    <small>Manifest: {point.manifest_sha}</small>
                  </div>
                ))}
              </div>
            </details>
            <div className="rollback-next-step">
              <strong>Vor einer tatsächlichen Übernahme</strong>
              <p>
                Der laufende Host kann inzwischen abweichen. Im
                Wiederherstellungsassistenten müssen Quelle, Dateiauswahl und
                ein frisch gelesener Zielzustand erneut geprüft werden. Diese
                Vorschau gibt keine Übernahme frei.
              </p>
            </div>
            {prepare && (
              <section
                className="rollback-handoff-card"
                aria-label="Im Assistenten weiterführen"
              >
                <span className="eyebrow">Nächster Schritt</span>
                <h3>Im Assistenten weiter</h3>

                <>
                  <p>
                    {selectedFiles.length} Datei
                    {selectedFiles.length === 1 ? "" : "en"} ausgewählt.
                    Sicherung und Host sind im Assistenten schon ausgewählt.
                    Prüfe dort die Dateiauswahl und den aktuellen Zustand des
                    Ziels.
                  </p>
                  {selectedFiles.length !== affected && (
                    <p className="rollback-file-warning">
                      Teilauswahl: Der gewünschte Einstellungswert ist damit
                      nicht vollständig abgedeckt. Auswirkungen jeder gewählten
                      Datei prüfen.
                    </p>
                  )}
                  <button
                    disabled={!selectedFiles.length}
                    onClick={() => {
                      if (
                        prepare({
                          backupID: data.source.id,
                          manifestSHA: data.source.manifest_sha,
                          baselineID: data.baseline.id,
                          files: selectedFiles.filter(
                            (path) =>
                              data.files.find((f) => f.path === path)
                                ?.action !== "remove",
                          ),
                          removeFiles: selectedFiles.filter(
                            (path) =>
                              data.files.find((f) => f.path === path)
                                ?.action === "remove",
                          ),
                          settingName: data.selected.setting.name,
                          desiredValue: settingValue(
                            data.selected.setting.id,
                            data.selected.after.value,
                          ),
                        })
                      )
                        onClose();
                    }}
                  >
                    Im Wiederherstellungsassistenten prüfen{" "}
                    <ArrowRight size={15} aria-hidden="true" />
                  </button>
                  <small>
                    Öffnet eine neue Vorbereitung. Keine Zielprüfung, kein Plan
                    und keine Übernahme werden automatisch gestartet.
                  </small>
                </>
              </section>
            )}
          </>
        )}
        <div className="rollback-close">
          <button className="secondary" onClick={onClose}>
            Zur Änderung zurück
          </button>
        </div>
      </div>
    </Dialog>
  );
}
