import { HostName } from "../hostNames";
import { useState } from "react";
import { CircleAlert, Clock3, ShieldCheck } from "lucide-react";
import { hostState } from "../api";
import type { Host, Status } from "../api";
import { Empty, Pagination } from "./shared";

function ProblemGroup({
  title,
  hosts,
  status,
  openHost,
  critical,
}: {
  title: string;
  hosts: Host[];
  status: Status;
  openHost: (id: string) => void;
  critical: boolean;
}) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const filtered = hosts.filter((h) =>
    h.name.toLowerCase().includes(query.trim().toLowerCase()),
  );
  const current = Math.min(page, Math.max(1, Math.ceil(filtered.length / 4)));
  const Icon = critical ? CircleAlert : Clock3;
  return (
    <article className={"problem-card" + (critical ? " problem-critical" : "")}>
      <div className="problem-card-heading">
        <Icon size={18} aria-hidden="true" />
        <h4>{title}</h4>
        <span className="count-badge">{hosts.length}</span>
      </div>
      <p>
        {critical
          ? "Bekannte Fehler zuerst prüfen. Vorhandene ältere Stände bleiben separat erreichbar."
          : title === "Sicherung fehlt oder ist überfällig"
            ? "Ein neuer vollständiger Stand ist erforderlich."
            : "Status oder Zeitangaben benötigen eine Prüfung."}
      </p>
      <details>
        <summary>
          {hosts.length === 1
            ? "Betroffenen Host ansehen"
            : `${hosts.length} betroffene Hosts ansehen`}
        </summary>
        {hosts.length > 4 && (
          <input
            aria-label={`${title}: Hosts durchsuchen`}
            placeholder="Host suchen"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setPage(1);
            }}
          />
        )}
        <div className="problem-hosts">
          {filtered.slice((current - 1) * 4, current * 4).map((h) => {
            const state = hostState(h, status);
            return (
              <div className="problem-host" key={h.id}>
                <div>
                  <strong>
                    <HostName name={h.name} />
                  </strong>
                  <span className={"state " + state.tone}>{state.text}</span>
                  <p>{state.reason}</p>
                </div>
                <button
                  className="secondary"
                  aria-label={`Host ${h.name} prüfen`}
                  onClick={() => openHost(h.id)}
                >
                  Host prüfen
                </button>
              </div>
            );
          })}
          {!filtered.length && <p className="muted">Keine passenden Hosts.</p>}
        </div>
        {hosts.length > 4 && (
          <Pagination
            page={current}
            pageSize={4}
            total={filtered.length}
            onPageChange={setPage}
            label="Betroffene Hosts"
          />
        )}
      </details>
    </article>
  );
}
export default function ProblemOverview({
  status,
  openHost,
  allHosts,
}: {
  status: Status;
  openHost: (id: string) => void;
  allHosts: () => void;
}) {
  const groups: Host[][] = [[], [], []];
  for (const host of status.hosts) {
    if (!host.enabled) continue;
    const state = hostState(host, status);
    if (state.tone === "success") continue;
    groups[
      state.tone === "danger"
        ? 0
        : ["Ohne Sicherung", "Überfällig"].includes(state.text)
          ? 1
          : 2
    ].push(host);
  }
  const count = groups.reduce((sum, g) => sum + g.length, 0);
  const titles = [
    "Fehler bei der Sicherung",
    "Sicherung fehlt oder ist überfällig",
    "Prüfung erforderlich",
  ];
  return (
    <section aria-label="Jetzt wichtig" className="problem-overview">
      <div className="section-heading">
        <div className="section-heading-title">
          <h3>Jetzt wichtig</h3>
          <span className="count-badge">{count}</span>
        </div>
        <button className="text-button" onClick={allHosts}>
          Alle Hosts ansehen
        </button>
      </div>
      {count ? (
        <>
          <div className="problem-grid">
            {groups.map((hosts, i) =>
              hosts.length ? (
                <ProblemGroup
                  key={titles[i]}
                  title={titles[i]}
                  hosts={hosts}
                  status={status}
                  openHost={openHost}
                  critical={i === 0}
                />
              ) : null,
            )}
          </div>
        </>
      ) : (
        <Empty
          title={
            !status.hosts.length
              ? "Noch keine Hosts eingerichtet"
              : !status.hosts.some((h) => h.enabled)
                ? "Automatische Sicherung pausiert"
                : "Alle Hosts sind aktuell gesichert"
          }
        >
          <ShieldCheck size={18} aria-hidden="true" />{" "}
          {!status.hosts.length
            ? "Mit „Host hinzufügen“ richtest du die erste Sicherung ein."
            : "Verbindung und Wiederherstellungsnachweise werden separat bewertet."}
        </Empty>
      )}
    </section>
  );
}
