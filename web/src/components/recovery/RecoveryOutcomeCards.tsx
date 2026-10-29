import { useListPageSize } from "../../listPageSize";
import { useEffect, useState } from "react";
import {
  CheckCircle2,
  CircleAlert,
  CircleHelp,
  RefreshCw,
  FileCheck2,
} from "lucide-react";
import { preciseDate } from "../../api";
import type { FileComparison, Plan } from "../../api";
import { observationLabels, settingValue } from "../../settingHistory";
import { Pagination } from "../shared";
import FileComparisonView from "../FileComparison";
export interface OutcomeFile {
  path: string;
  state: string;
  expected: string;
  observed: string;
  expected_sha?: string;
  observed_sha?: string;
  content_state: string;
  metadata_state: string;
  reason: string;
  comparison?: FileComparison;
}
export interface RecoveryOutcome {
  id: string;
  plan_id: string;
  started_at: string;
  finished_at?: string;
  direction: string;
  origin: string;
  state: string;
  reason: string;
  source_manifest_sha?: string;
  snapshot_sha: string;
  files: OutcomeFile[];
  settings: {
    setting: { id: string; name: string; key: string };
    state: string;
    expected_value?: string;
    observed_value?: string;
    observed_state: string;
    sources: string[];
    reason: string;
  }[];
}
const labels: Record<string, string> = {
  passed: "Bestätigt",
  different: "Abweichung",
  unknown: "Nicht nachgewiesen",
  running: "Abschluss fehlt",
};
function Mark({ state }: { state: string }) {
  const Icon =
    state === "passed"
      ? CheckCircle2
      : state === "different"
        ? CircleAlert
        : CircleHelp;
  return (
    <span className="outcome-mark" data-state={state}>
      <Icon size={16} aria-hidden="true" />
      {labels[state] || "Nicht nachgewiesen"}
    </span>
  );
}
function FileCard({ file }: { file: OutcomeFile }) {
  const [open, setOpen] = useState(false);
  return (
    <article className="outcome-file-card" data-state={file.state}>
      <div className="outcome-file-heading">
        <strong>{file.path}</strong>
        <Mark state={file.state} />
      </div>
      <p>{file.reason}</p>
      <div className="outcome-file-facts">
        <span>
          Inhalt: {labels[file.content_state] || "Nicht nachgewiesen"}
        </span>
        <span>
          Eigenschaften: {labels[file.metadata_state] || "Nicht nachgewiesen"}
        </span>
      </div>
      <details onToggle={(e) => setOpen(e.currentTarget.open)}>
        <summary>Nachweis zur Datei ansehen</summary>
        <dl>
          <dt>Soll laut Plan</dt>
          <dd>
            {file.expected === "missing" ? "Datei fehlt" : "Datei vorhanden"}
          </dd>
          <dt>Aktuelle Aufnahme</dt>
          <dd>
            {file.observed === "missing"
              ? "Datei fehlt"
              : file.observed === "read"
                ? "Datei gelesen"
                : "Nicht vollständig lesbar"}
          </dd>
          {file.expected_sha && (
            <>
              <dt>Soll-Prüfsumme</dt>
              <dd>
                <code>{file.expected_sha}</code>
              </dd>
            </>
          )}
          {file.observed_sha && (
            <>
              <dt>Ist-Prüfsumme</dt>
              <dd>
                <code>{file.observed_sha}</code>
              </dd>
            </>
          )}
        </dl>
        {open && file.comparison && (
          <FileComparisonView
            comparison={file.comparison}
            loading={false}
            error=""
            retry={() => {}}
            collapseContext
          />
        )}
        {file.state === "different" && !file.comparison && (
          <p>
            Kein Textvergleich verfügbar. Prüfsummen und Eigenschaften bleiben
            der Nachweis; geschützte Inhalte werden nicht offengelegt.
          </p>
        )}
      </details>
    </article>
  );
}
export default function RecoveryOutcomeCards({
  plan,
  checking = false,
  refreshError = "",
  disabled,
  onVerify,
}: {
  plan: Plan;
  checking?: boolean;
  refreshError?: string;
  disabled: boolean;
  onVerify?: () => Promise<void>;
}) {
  const [listSize, setListSize] = useListPageSize("recovery-outcomes", 4);
  const [page, setPage] = useState(1),
    [filter, setFilter] = useState("attention");
  const reports = plan.outcome_checks || [];
  const report = reports.at(-1);
  useEffect(() => {
    setPage(1);
    setFilter("attention");
  }, [report?.id]);
  const eligible = [
    "applied",
    "checks_pending",
    "demo_applied",
    "rolled_back",
  ].includes(plan.state);
  const current =
    !checking &&
    !refreshError &&
    !!report &&
    report.plan_id === plan.id &&
    report.direction ===
      (plan.state === "rolled_back" ? "rollback" : "apply") &&
    eligible;
  const files = report?.files || [],
    settings = report?.settings || [];
  const problems = files.filter((f) => f.state !== "passed");
  const rows = [
    ...(filter === "attention" && problems.length ? problems : files),
  ].sort(
    (a, b) =>
      Number(a.state === "passed") - Number(b.state === "passed") ||
      a.path.localeCompare(b.path),
  );
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(rows.length / listSize)),
  );
  if (!plan.result && !report) return null;
  return (
    <section
      className="recovery-outcome"
      data-surface-topic="security"
      aria-label="Ergebnis der Wiederherstellung"
    >
      <div className="outcome-heading">
        <div>
          <span className="eyebrow">
            <FileCheck2 size={14} aria-hidden="true" />
            Ergebnisprüfung
          </span>
          <h3>
            {current && report?.state === "passed"
              ? "Die geprüften Dateien stimmen überein."
              : current && report?.state === "different"
                ? "Hier weicht das Ziel noch ab."
                : "Was ist am Ziel angekommen?"}
          </h3>
        </div>
        {onVerify && eligible && (
          <button
            className="secondary"
            disabled={disabled}
            onClick={() => void onVerify()}
          >
            <RefreshCw size={14} aria-hidden="true" />
            Dateien und Einstellungen erneut prüfen
          </button>
        )}
      </div>
      <p className="outcome-scope">
        {plan.state === "rolled_back"
          ? "Vergleich mit dem ursprünglichen Zielstand vor der Übernahme."
          : "Vergleich der ausgeführten Dateiaktionen mit dem freigegebenen Plan."}{" "}
        Die Prüfung liest die Werte in den Dateien. Ob sie am Host bereits
        wirksam sind, Dienste laufen oder ein Neustart erfolgt ist, wird hier
        nicht geprüft.
      </p>
      {!report ? (
        <p className="notice">
          Noch keine aktuelle Datei- und Einstellungsprüfung vorhanden.
        </p>
      ) : (
        <div key={report.id}>
          <div className="outcome-status" role="status">
            <Mark state={current ? report.state : "unknown"} />
            <span>
              {checking
                ? "Aktuelle Dateien werden gelesen …"
                : refreshError ||
                  (current
                    ? report.reason
                    : "Früherer Prüfstand · für den aktuellen Planstatus ist eine neue Prüfung erforderlich")}
            </span>
          </div>
          <p className="outcome-date">
            {report.finished_at
              ? `Gelesen am ${preciseDate(report.finished_at)}`
              : "Abschluss des Prüfversuchs nicht nachgewiesen"}{" "}
            ·{" "}
            {report.origin === "demo-read"
              ? "Lokale Demodateien"
              : "Lesende Hostabfrage"}
          </p>
          {current && report.finished_at && files.length > 0 && (
            <div
              className="outcome-summary"
              aria-label="Prüfergebnis im Überblick"
            >
              <article>
                <strong>
                  {files.filter((f) => f.state === "passed").length}
                </strong>
                <span>Dateien bestätigt</span>
              </article>
              <article data-state={problems.length ? "different" : "passed"}>
                <strong>{problems.length}</strong>
                <span>Dateien mit Prüfbedarf</span>
              </article>
              <article>
                <strong>
                  {settings.filter((s) => s.state === "passed").length} /{" "}
                  {settings.length}
                </strong>
                <span>Einstellungen bestätigt</span>
              </article>
            </div>
          )}
          {current && report.finished_at && !!settings.length && (
            <section aria-label="Geprüfte Einstellungen">
              <h4>Einstellungen im geprüften Dateibestand</h4>
              <div className="outcome-settings">
                {settings.map((v) => (
                  <article
                    className="outcome-setting-card"
                    data-state={v.state}
                    key={v.setting.id}
                  >
                    <div className="outcome-file-heading">
                      <strong>{v.setting.name}</strong>
                      <Mark state={v.state} />
                    </div>
                    <div className="outcome-setting-values">
                      <div>
                        <small>Soll laut Plan</small>
                        <strong>
                          {settingValue(v.setting.id, v.expected_value)}
                        </strong>
                      </div>
                      <div>
                        <small>Aktuell deklariert</small>
                        <strong>
                          {v.observed_state === "value"
                            ? settingValue(v.setting.id, v.observed_value)
                            : observationLabels[v.observed_state] ||
                              "Nicht bekannt"}
                        </strong>
                      </div>
                    </div>
                    <p>{v.reason}</p>
                    <details>
                      <summary>Wert in der Datei ansehen</summary>
                      <code>{v.setting.key}</code>
                      {v.sources.map((p) => (
                        <code key={p}>{p}</code>
                      ))}
                    </details>
                  </article>
                ))}
              </div>
            </section>
          )}
          {current && report.finished_at && !!files.length && (
            <section aria-label="Geprüfte Dateien">
              <div className="outcome-files-heading">
                <h4>
                  {problems.length
                    ? "Dateien mit Prüfbedarf"
                    : "Geprüfte Dateien"}
                </h4>
                {!!problems.length && (
                  <button
                    className="text-button"
                    onClick={() => {
                      setFilter(filter === "attention" ? "all" : "attention");
                      setPage(1);
                    }}
                  >
                    {filter === "attention"
                      ? `Alle ${files.length} Dateien zeigen`
                      : "Nur Prüfbedarf zeigen"}
                  </button>
                )}
              </div>
              {rows
                .slice((currentPage - 1) * listSize, currentPage * listSize)
                .map((f) => (
                  <FileCard key={report.id + f.path} file={f} />
                ))}
              {rows.length > 4 && (
                <Pagination
                  page={currentPage}
                  pageSize={listSize}
                  onPageSizeChange={setListSize}
                  total={rows.length}
                  onPageChange={setPage}
                  label="Geprüfte Dateien"
                />
              )}
            </section>
          )}
          <details className="outcome-proof">
            <summary>Prüfzeitpunkt und Planbindung</summary>
            <code>Prüfung {report.id}</code>
            <code>Plan {report.plan_id}</code>
            <code>
              Quelle {report.source_manifest_sha || "Nicht bestätigt"}
            </code>
            <code>Zielaufnahme {report.snapshot_sha}</code>
            <p>
              Die Aussage gilt für diesen Lesezeitpunkt. Behaltene und manuelle
              Dateien sind kein bestätigtes Übernahmeziel.
            </p>
          </details>
        </div>
      )}
      {reports.length > 1 && (
        <details className="outcome-proof">
          <summary>{reports.length - 1} frühere Prüfversuche</summary>
          {reports
            .slice(0, -1)
            .reverse()
            .map((r) => (
              <p key={r.id}>
                {preciseDate(r.started_at)} ·{" "}
                {r.direction === "rollback" ? "Rücksetzung" : "Übernahme"} ·{" "}
                {labels[r.state] || "Nicht nachgewiesen"}
              </p>
            ))}
        </details>
      )}
    </section>
  );
}
