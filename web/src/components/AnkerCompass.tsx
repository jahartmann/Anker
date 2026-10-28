import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Archive,
  Compass,
  GitBranch,
  RotateCcw,
  Search,
  ShieldCheck,
} from "lucide-react";
import type { Host, Status } from "../api";
import { hostState } from "../api";
import { displayHostName, HostName } from "../hostNames";
import { recoverySource } from "../hostHealth";
import { Dialog } from "./shared";

export type CompassIntent = "understand" | "secure" | "changes" | "restore";
export type CompassDestination =
  | {
      kind: "host";
      hostID: string;
      tab: "Übersicht" | "Sicherungen" | "Änderungen" | "Einstellungen";
    }
  | { kind: "restore"; hostID: string }
  | { kind: "job"; id: string };
export type HostVisit = Extract<CompassDestination, { kind: "host" }> & {
  sequence: number;
};
const intents = [
  {
    id: "understand",
    title: "Zustand verstehen",
    description: "Was braucht gerade Aufmerksamkeit?",
    icon: Compass,
    tone: "sage",
  },
  {
    id: "secure",
    title: "Sicherungen im Blick",
    description: "Sicherungen und Prüfergebnisse ansehen.",
    icon: Archive,
    tone: "blue",
  },
  {
    id: "changes",
    title: "Änderungen verfolgen",
    description: "Was hat sich wann geändert?",
    icon: GitBranch,
    tone: "sand",
  },
  {
    id: "restore",
    title: "Gezielt zurücksetzen",
    description: "Älteren Stand auswählen und prüfen.",
    icon: RotateCcw,
    tone: "rose",
  },
] as const;

export function CompassEntrypoints({
  onChoose,
}: {
  onChoose: (intent: CompassIntent) => void;
}) {
  return (
    <div className="compass-intents">
      {intents.map(({ id, title, description, icon: Icon, tone }) => (
        <button
          key={id}
          className={`compass-intent compass-${tone}`}
          onClick={() => onChoose(id)}
        >
          <span className="compass-emblem">
            <Icon size={19} strokeWidth={1.6} aria-hidden="true" />
          </span>
          <span>
            <strong>{title}</strong>
            <small>{description}</small>
          </span>
          <ArrowRight size={15} aria-hidden="true" />
        </button>
      ))}
    </div>
  );
}

export default function AnkerCompass({
  status,
  initialIntent,
  initialHost = "",
  canRestore,
  stale,
  onClose,
  onGo,
}: {
  status: Status;
  initialIntent: CompassIntent | null;
  initialHost?: string;
  canRestore: boolean;
  stale: boolean;
  onClose: () => void;
  onGo: (destination: CompassDestination) => void;
}) {
  const [intent, setIntent] = useState<CompassIntent | null>(initialIntent);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(initialHost);
  const [page, setPage] = useState(0);
  const stage = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (selected && intent)
      stage.current?.querySelector<HTMLElement>(".compass-next h3")?.focus();
  }, [selected, intent]);
  const hosts = useMemo(() => {
    const normalize = (value: string) =>
      value
        .toLocaleLowerCase("de")
        .replace(/[·_\-]+/g, " ")
        .replace(/\s+/g, " ")
        .trim();
    const needle = normalize(query);
    return status.hosts
      .filter((h) =>
        normalize(
          [h.name, displayHostName(h.name), h.address, h.group].join(" "),
        ).includes(needle),
      )
      .sort((a, b) => {
        const priority = (h: Host) =>
          h.enabled && hostState(h, status).tone !== "success"
            ? 0
            : h.enabled
              ? 1
              : 2;
        return priority(a) - priority(b) || a.name.localeCompare(b.name, "de");
      });
  }, [status, query]);
  const lastPage = Math.max(0, Math.ceil(hosts.length / 4) - 1);
  const currentPage = Math.min(page, lastPage);
  const host = status.hosts.find((h) => h.id === selected);
  const state = host && hostState(host, status);
  const backups = status.backups.filter((b) => b.host_id === selected);
  const source = recoverySource(backups);
  const active = status.jobs.find(
    (j) => j.host_id === selected && ["queued", "running"].includes(j.state),
  );
  const chosen = intents.find((i) => i.id === intent);
  let destination: CompassDestination | undefined;
  let action = "Host öffnen";
  let explanation = state?.reason || "";
  if (host) {
    destination = { kind: "host", hostID: host.id, tab: "Übersicht" };
    if (intent === "secure") {
      destination = { kind: "host", hostID: host.id, tab: "Sicherungen" };
      action = "Sicherungsverlauf öffnen";
      explanation = backups.length
        ? `${backups.length} Sicherungen. Prüfe die Dateien, bevor du sie verwendest.`
        : "Noch kein Stand erfasst. Im Host kannst du die Verbindung prüfen und eine erste Sicherung starten.";
    } else if (intent === "changes") {
      destination = { kind: "host", hostID: host.id, tab: "Änderungen" };
      action = "Konfigurationsreise öffnen";
      explanation =
        backups.length > 1
          ? "Sieh, wann sich Dateien und Einstellungen geändert haben. Die passenden Sicherungen kannst du direkt vergleichen."
          : "Für einen Vergleich brauchst du zwei erfasste Stände. Die Hostansicht zeigt dir die vorhandenen Sicherungen.";
    } else if (intent === "restore") {
      destination =
        source && canRestore
          ? { kind: "restore", hostID: host.id }
          : { kind: "host", hostID: host.id, tab: "Sicherungen" };
      action =
        source && canRestore
          ? "Im Assistenten vorbereiten"
          : "Sicherungen ansehen";
      explanation = !canRestore
        ? "Deine Leserechte erlauben das Ansehen der Stände. Eine Wiederherstellung benötigt zusätzliche Rechte."
        : source
          ? "Eine vollständige Sicherung ohne bekannte Prüfsummenfehler ist vorhanden. Der Assistent prüft Quelle und Ziel aktuell, bevor du den Plan freigibst."
          : "Es gibt noch keine vollständige Sicherung ohne bekannte Prüfsummenfehler. Sieh dir zuerst die vorhandenen Stände an.";
    } else if (active || state?.jobID) {
      destination = { kind: "job", id: active?.id || state!.jobID! };
      action = active
        ? "Laufenden Auftrag ansehen"
        : "Auftrag und Ursache ansehen";
      if (active)
        explanation =
          "Für diesen Host ist bereits ein Auftrag aktiv. Prüfe den Fortschritt, bevor du weitere Schritte planst.";
    } else if (state?.action === "files") {
      destination = { kind: "host", hostID: host.id, tab: "Sicherungen" };
      action = "Sicherungen und Nachweise ansehen";
    } else if (state?.action === "settings") {
      destination = { kind: "host", hostID: host.id, tab: "Einstellungen" };
      action = "Host-Einstellungen ansehen";
    }
  }
  return (
    <Dialog topic="server" title="Anker Kompass" onClose={onClose} wide>
      <div className="compass-body" ref={stage}>
        {stale && (
          <p className="notice warning" role="status">
            Verbindung unterbrochen. Die angezeigten Daten können veraltet sein.
          </p>
        )}
        {!intent ? (
          <>
            <p className="compass-intro">
              Was möchtest du{host ? ` für ${displayHostName(host.name)}` : ""}{" "}
              als Nächstes tun?
            </p>
            <CompassEntrypoints onChoose={setIntent} />
          </>
        ) : (
          <>
            <button
              className="compass-back"
              onClick={() => {
                if (selected) setSelected("");
                else {
                  setIntent(null);
                  setQuery("");
                  setPage(0);
                }
              }}
            >
              <ArrowLeft size={14} aria-hidden="true" />
              {selected ? "Anderen Host wählen" : "Andere Aufgabe wählen"}
            </button>
            <div className="compass-step">
              <span>{selected ? "02" : "01"}</span>
              <div>
                <h3>{chosen?.title}</h3>
                <p>
                  {selected
                    ? "Dein nächster Schritt"
                    : "Für welchen Host? Aufmerksamkeit steht zuerst."}
                </p>
              </div>
            </div>
            {!host ? (
              <>
                <label className="compass-search">
                  <Search size={17} aria-hidden="true" />
                  <input
                    autoFocus
                    aria-label="Host im Kompass suchen"
                    placeholder="Name, Adresse oder Gruppe …"
                    value={query}
                    onChange={(e) => {
                      setQuery(e.target.value);
                      setPage(0);
                    }}
                  />
                </label>
                <div className="compass-hosts">
                  {hosts
                    .slice(currentPage * 4, currentPage * 4 + 4)
                    .map((h) => {
                      const health = hostState(h, status);
                      return (
                        <button
                          className="compass-host"
                          key={h.id}
                          onClick={() => setSelected(h.id)}
                        >
                          <span>
                            <strong>
                              <HostName name={h.name} />
                            </strong>
                            <small>{h.group || h.address}</small>
                          </span>
                          <span className={"state " + health.tone}>
                            {health.text}
                          </span>
                          <ArrowRight size={15} aria-hidden="true" />
                        </button>
                      );
                    })}
                </div>
                {!hosts.length && (
                  <p className="compass-empty">
                    {status.hosts.length
                      ? "Kein passender Host. Versuche einen anderen Namen oder eine Adresse."
                      : "Noch keine Hosts verbunden. Lege im Bereich Hosts deinen ersten Host an."}
                  </p>
                )}
                {hosts.length > 4 && (
                  <div className="compass-pagination">
                    <span>
                      {currentPage * 4 + 1}–
                      {Math.min(currentPage * 4 + 4, hosts.length)} von{" "}
                      {hosts.length} Hosts
                    </span>
                    <button
                      className="secondary"
                      disabled={currentPage === 0}
                      onClick={() => setPage(currentPage - 1)}
                    >
                      Zurück
                    </button>
                    <button
                      className="secondary"
                      disabled={currentPage === lastPage}
                      onClick={() => setPage(currentPage + 1)}
                    >
                      Weitere Hosts
                    </button>
                  </div>
                )}
              </>
            ) : (
              <section className="compass-next" aria-label="Nächster Schritt">
                <div className="compass-next-heading">
                  <span className="compass-emblem">
                    <ShieldCheck
                      size={22}
                      strokeWidth={1.5}
                      aria-hidden="true"
                    />
                  </span>
                  <div>
                    <h3 tabIndex={-1}>
                      <HostName name={host.name} />
                    </h3>
                    <small>{host.address}</small>
                  </div>
                  <span className={"state " + state!.tone}>{state!.text}</span>
                </div>
                <p>{explanation}</p>
                <button onClick={() => destination && onGo(destination)}>
                  {action}
                  <ArrowRight size={15} aria-hidden="true" />
                </button>
                <small className="compass-assurance">
                  Du siehst zuerst die Details. Hier wird noch nichts gestartet.
                </small>
              </section>
            )}
          </>
        )}
      </div>
    </Dialog>
  );
}
