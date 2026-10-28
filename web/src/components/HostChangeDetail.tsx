import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, GitCompareArrows } from "lucide-react";
import type { Backup, Host } from "../api";
import { displayHostName } from "../hostNames";
import type { Notify } from "./shared";
import { Empty, PageEmblem } from "./shared";
import BackupComparison from "./BackupComparison";
import SettingHistory from "./SettingHistory";

export default function HostChangeDetail({
  host,
  backups,
  from,
  to,
  canOperate,
  canDownload,
  notify,
  onRestore,
  onBack,
}: {
  host: Host;
  backups: Backup[];
  from: string;
  to: string;
  canOperate: boolean;
  canDownload: boolean;
  notify: Notify;
  onRestore: (id: string) => void;
  onBack: () => void;
}) {
  const [traceRequest, setTraceRequest] = useState<{
    id: string;
    sequence: number;
  }>();
  const title = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    title.current?.focus({ preventScroll: true });
    window.scrollTo(0, 0);
  }, []);
  const selection = useMemo(
    () => ({ side: "to" as const, id: to, pairedId: from, sequence: 1 }),
    [from, to],
  );
  const valid =
    from !== to &&
    [from, to].every((id) =>
      backups.some((b) => b.id === id && b.host_id === host.id),
    );
  return (
    <section
      className="node-change-detail"
      data-surface-topic="activity"
      aria-label="Änderungsdetails der Node"
    >
      <button className="back-button" onClick={onBack}>
        <ArrowLeft size={15} aria-hidden="true" /> Zurück zum Hostverlauf
      </button>
      <header className="node-change-detail-heading">
        <PageEmblem tone="blue">
          <GitCompareArrows size={22} aria-hidden="true" />
        </PageEmblem>
        <div>
          <span className="eyebrow">Konfiguration · Änderungsdetails</span>
          <h1 ref={title} tabIndex={-1} aria-label={host.name}>
            {displayHostName(host.name)}
          </h1>
          <p>
            {host.name} · {host.address}
          </p>
        </div>
      </header>
      <p className="muted">
        Wähle eine Datei, um ihren Inhalt und ihre Metadaten vorher und nachher
        zu sehen.
      </p>
      <SettingHistory
        key={host.id}
        hostID={host.id}
        backups={backups.filter((b) => b.host_id === host.id)}
        request={traceRequest}
      />
      {valid ? (
        <BackupComparison
          key={from + ":" + to}
          backups={backups.filter((b) => b.host_id === host.id)}
          user={{
            id: "",
            name: "",
            role: canOperate ? "restore" : "reader",
            secrets: canDownload,
            disabled: false,
          }}
          notify={notify}
          onRestore={onRestore}
          selection={selection}
          spacious
          onTrackSetting={(id) =>
            setTraceRequest((r) => ({ id, sequence: (r?.sequence || 0) + 1 }))
          }
        />
      ) : (
        <Empty title="Vergleichsstände nicht mehr verfügbar">
          Mindestens ein ausgewählter Stand ist nicht mehr vorhanden. Wähle im
          Hostverlauf ein anderes Ständepaar.
        </Empty>
      )}
    </section>
  );
}
