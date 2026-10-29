import { useListPageSize } from "../listPageSize";
import { useState } from "react";
import { CircleAlert, FileWarning, Wrench } from "lucide-react";
import type { Backup, BackupIssue } from "../api";
import { date } from "../api";
import { Pagination } from "./shared";

export default function BackupIssues({
  backup,
  issues: suppliedIssues,
  onSettings,
  onRetry,
}: {
  backup?: Backup;
  issues?: BackupIssue[];
  onSettings?: () => void;
  onRetry?: () => void;
}) {
  const [page, setPage] = useState(1);
  const [listSize, setListSize] = useListPageSize("backup-issues", 3);
  if (
    !backup?.warnings?.length &&
    !backup?.issues?.length &&
    !suppliedIssues?.length
  )
    return null;
  const issues = backup?.issues?.length
    ? backup.issues
    : suppliedIssues?.length
      ? suppliedIssues
      : (backup?.warnings || []).map((warning) => ({
          code: "legacy_warning",
          title: "Sicherungsumfang prüfen",
          detail:
            warning === "[Verdeckter Inhalt]"
              ? "Dieser ältere Hinweis enthält geschützte Angaben. Das allein bedeutet nicht, dass eine Datei fehlt."
              : warning,
          action:
            "Hostprofil und benötigte Pfade prüfen. Danach mit aktuellem Hosthelfer erneut sichern.",
          path: undefined,
          path_protected: warning === "[Verdeckter Inhalt]",
        }));
  const current = Math.min(
    page,
    Math.max(1, Math.ceil(issues.length / listSize)),
  );
  return (
    <section
      className="backup-issues"
      aria-label="Ursachen der unvollständigen Sicherung"
    >
      <header className="backup-issues-heading">
        <span className="diagnostic-symbol">
          <CircleAlert size={23} aria-hidden="true" />
        </span>
        <div>
          <span className="eyebrow">Sicherung benötigt Aufmerksamkeit</span>
          <h3>
            {issues.length}{" "}
            {issues.length === 1 ? "konkreter Hinweis" : "konkrete Hinweise"}
          </h3>
          <p>
            {backup
              ? `Betrifft den Stand vom ${date(backup.created_at)}.`
              : "Ursachen und nächste Schritte für diesen Auftrag."}
          </p>
        </div>
      </header>
      <div className="backup-issues-list">
        {issues
          .slice((current - 1) * listSize, current * listSize)
          .map((issue, index) => (
            <article
              key={`${backup?.id || "job"}-${current}-${index}`}
              className="backup-issue"
              data-issue={issue.code}
            >
              <FileWarning size={19} aria-hidden="true" />
              <div>
                <h4>{issue.title}</h4>
                {issue.path && <code className="issue-path">{issue.path}</code>}
                <p>{issue.detail}</p>
                {issue.path_protected && (
                  <p className="issue-privacy">
                    {issue.code === "invalid_path"
                      ? "Dieser Hinweis enthält keinen sicher darstellbaren Pfad. Ohne detaillierte neue Aufnahme ist die betroffene Datei nicht eindeutig bestimmbar."
                      : "Pfad oder technische Angaben bleiben geschützt; die Ursache und Abhilfe stehen hier separat."}
                  </p>
                )}
                <div className="issue-remedy">
                  <Wrench size={15} aria-hidden="true" />
                  <p>{issue.action}</p>
                </div>
              </div>
            </article>
          ))}
      </div>
      {issues.length > 3 && (
        <Pagination
          page={current}
          pageSize={listSize}
          onPageSizeChange={setListSize}
          total={issues.length}
          onPageChange={setPage}
          label="Sicherungshinweise"
        />
      )}
      {(onSettings || onRetry) && (
        <footer className="issue-actions">
          {onSettings && (
            <button className="secondary" onClick={onSettings}>
              Sicherungsprofil prüfen
            </button>
          )}
          {onRetry && <button onClick={onRetry}>Neue Sicherung starten</button>}
        </footer>
      )}
      <p className="issue-boundary">
        Nach einer neuen vollständigen Sicherung verschwindet der Hinweis aus
        der Hostübersicht. Bei diesem älteren Stand bleibt er als Nachweis
        erhalten.
      </p>
    </section>
  );
}
