import type { SVGProps } from "react";
import { Network } from "lucide-react";
import type { Host } from "../api";

// Front-panel details are decorative, never a connection or health indicator.
export function ServerRack({
  size = 20,
  strokeWidth = 1.6,
  ...props
}: SVGProps<SVGSVGElement> & { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
      className={["server-rack-glyph", props.className]
        .filter(Boolean)
        .join(" ")}
    >
      <path d="M2 3v18M22 3v18" className="rack-rails" />
      <rect x="4.5" y="3.5" width="15" height="7" rx="1.5" />
      <rect x="4.5" y="13.5" width="15" height="7" rx="1.5" />
      <path d="M7.5 6v2M10 6v2M7.5 16v2M10 16v2" className="rack-vents" />
      <path d="M15.5 7h1M15.5 17h1" strokeWidth="2" />
    </svg>
  );
}

export function HostEndpoint({ host }: { host: Host }) {
  const address =
    host.address.includes(":") && !host.address.startsWith("[")
      ? `[${host.address}]`
      : host.address;
  const destination = `${host.ssh_user ? host.ssh_user + "@" : ""}${address}:${host.ssh_port || 22}`;
  return (
    <div className="host-endpoint" aria-label="SSH-Ziel für Sicherungen">
      <span className="host-endpoint-label">
        <Network size={13} aria-hidden="true" /> SSH · Sicherungen
      </span>
      <code title="Konfiguriertes SSH-Ziel">{destination}</code>
    </div>
  );
}
