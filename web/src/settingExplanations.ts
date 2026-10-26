import type { SettingObservation } from "./settingHistory";

export interface SettingExplanationText {
  meaning: string;
  impact: string;
  check: string;
  documentation: { title: string; url: string };
}
// Directional explanations require two explicit, ordered, readable declarations.
// A missing/default/protected value is never converted into an enabled/disabled
// state. These rules describe documented semantics, not observed live effects.
export function explainSettingChange(
  id: string,
  before: SettingObservation,
  after: SettingObservation,
): SettingExplanationText | null {
  const a = before.value,
    b = after.value;
  if (
    before.state !== "value" ||
    after.state !== "value" ||
    a === undefined ||
    b === undefined ||
    a === b ||
    !before.backup_id ||
    !after.backup_id ||
    before.backup_id === after.backup_id ||
    before.order_uncertain ||
    after.order_uncertain
  )
    return null;
  const start = Date.parse(before.created_at),
    end = Date.parse(after.created_at);
  if (
    !Number.isFinite(start) ||
    !Number.isFinite(end) ||
    start >= end ||
    end > Date.now() + 300000
  )
    return null;
  const validSource = (p: string) =>
    id === "hostname"
      ? p === "etc/hostname"
      : id === "dns-servers"
        ? p === "etc/resolv.conf"
        : /^etc\/(sysctl\.conf|sysctl\.d\/[^/]+\.conf)$/.test(p);
  if (
    !before.sources.length ||
    !after.sources.length ||
    !before.sources.every(validSource) ||
    !after.sources.every(validSource)
  )
    return null;
  const ipDocumentation = {
    title: "Linux-Kernel: IP-Parameter",
    url: "https://docs.kernel.org/networking/ip-sysctl.html",
  };
  if (id === "ipv4-forwarding" || id === "ipv6-forwarding") {
    if (![a, b].every((v) => v === "0" || v === "1")) return null;
    const protocol = id === "ipv4-forwarding" ? "IPv4" : "IPv6";
    const enabled = b === "1";
    return {
      meaning: `Die gespeicherte Einstellung ${enabled ? "aktiviert" : "deaktiviert"} die Weiterleitung von ${protocol}-Paketen zwischen Netzwerkschnittstellen.`,
      impact:
        id === "ipv4-forwarding"
          ? enabled
            ? "Wenn dieser Wert wirksam wird, kann die Node als Router arbeiten. Welche Verbindungen möglich sind, hängt auch von Routen und Firewall ab."
            : "Wenn dieser Wert wirksam wird, können Verbindungen ausfallen, die auf IPv4-Routing über diese Node angewiesen sind. Ein Ausfall ist damit nicht nachgewiesen."
          : "Eine Übernahme kann IPv6-Routing und die Verarbeitung von Router-Ankündigungen beeinflussen. Das Zusammenspiel mit accept_ra ist entscheidend.",
      check:
        id === "ipv4-forwarding"
          ? "Livewert, Routen und Firewall prüfen. Beim Umschalten können weitere IPv4-Parameter auf Standardwerte zurückgesetzt werden."
          : "Livewert, Routen, Firewall und accept_ra gemeinsam prüfen.",
      documentation: ipDocumentation,
    };
  }
  if (id === "swappiness") {
    if (
      ![a, b].every(
        (v) => /^\d+$/.test(v) && Number(v) >= 0 && Number(v) <= 200,
      )
    )
      return null;
    if (Number(a) === Number(b)) return null;
    return {
      meaning:
        Number(b) > Number(a)
          ? "Der höhere gespeicherte Wert bewertet Swap-I/O im Verhältnis zum Freigeben von Dateicache als günstiger."
          : "Der niedrigere gespeicherte Wert bewertet Swap-I/O im Verhältnis zum Freigeben von Dateicache als teurer.",
      impact:
        "Bei Speicherdruck kann sich das Verhältnis zwischen Auslagern und Freigeben von Dateicache ändern. Der Wert ist keine Prozentangabe und garantiert keine bessere Leistung.",
      check:
        "Swap-I/O und Antwortzeiten unter vergleichbarer Last beobachten. Auch ein Wert von 0 schaltet Swap nicht grundsätzlich ab.",
      documentation: {
        title: "Linux-Kernel: Swappiness",
        url: "https://docs.kernel.org/admin-guide/sysctl/vm.html#swappiness",
      },
    };
  }
  if (id === "hostname") {
    if (![a, b].every((v) => /^[A-Za-z0-9][A-Za-z0-9.-]{0,63}$/.test(v)))
      return null;
    return {
      meaning:
        "In der gesicherten Datei ist ein anderer lokaler Name für diese Node hinterlegt.",
      impact:
        "Bei Übernahme können Dienste, Protokolle oder Überwachung einen anderen Namen anzeigen. Namensabhängige Zuordnungen sollten geprüft werden; eine Umbenennung im Proxmox-Cluster ist dadurch nicht bestätigt.",
      check:
        "Aktiven Hostnamen und abhängige Namenszuordnungen prüfen. Andere Konfigurationsquellen können den gespeicherten Namen übersteuern.",
      documentation: {
        title: "systemd: hostname",
        url: "https://www.freedesktop.org/software/systemd/man/latest/hostname.html",
      },
    };
  }
  if (id === "dns-servers") {
    if (
      ![a, b].every(
        (v) =>
          v.length <= 512 && /^[0-9a-fA-F:.]+(?: · [0-9a-fA-F:.]+)*$/.test(v),
      )
    )
      return null;
    return {
      meaning:
        "Die gespeicherten DNS-Server oder ihre Reihenfolge haben sich geändert.",
      impact:
        "Programme, die diese Resolver-Konfiguration nutzen, können Namen anders oder langsamer auflösen. Ob der neue Resolver erreichbar ist und welche Antworten er liefert, ist nicht belegt.",
      check:
        "Aktiv verwendeten Resolver und die Auflösung relevanter Namen prüfen. Andere Resolver-Dienste oder Anwendungseinstellungen können abweichen.",
      documentation: {
        title: "Linux man-pages: resolv.conf",
        url: "https://man7.org/linux/man-pages/man5/resolv.conf.5.html",
      },
    };
  }
  return null;
}
