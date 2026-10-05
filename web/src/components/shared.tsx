import { useEffect, useRef, useId, isValidElement, cloneElement } from "react";
import type { ReactNode } from "react";
import { X } from "lucide-react";
import { labels } from "../api";
export type Notify = (message: string, error?: boolean) => void;
export function State({ value, label }: { value: string; label?: string }) {
  const tone = ["successful", "demo_applied"].includes(value)
    ? "success"
    : ["failed", "interrupted", "blocked"].includes(value)
      ? "warning"
      : "muted";
  return (
    <span className={"state " + tone}>{label || labels[value] || value}</span>
  );
}
export function Heading({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <header className="page-heading">
      <div>
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {action && <div className="heading-action">{action}</div>}
    </header>
  );
}
export function Empty({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty">
      <h3>{title}</h3>
      <p>{children}</p>
    </div>
  );
}
export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  const id = useId();
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {isValidElement<{ id?: string; "aria-describedby"?: string }>(children)
        ? cloneElement(children, {
            id,
            "aria-describedby": hint ? id + "-hint" : undefined,
          })
        : children}
      {hint && <small id={id + "-hint"}>{hint}</small>}
    </div>
  );
}
export function Dialog({
  title,
  children,
  onClose,
  wide = false,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
}) {
  const root = useRef<HTMLDivElement>(null);
  const titleID = useId();
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    const old = document.activeElement as HTMLElement;
    const node = root.current;
    (
      node?.querySelector<HTMLElement>("input,select,textarea") ||
      node?.querySelector<HTMLElement>("button")
    )?.focus();
    function keys(e: KeyboardEvent) {
      if (e.key === "Escape") {
        close.current();
        return;
      }
      if (e.key === "Tab") {
        const elements = Array.from(
          node?.querySelectorAll<HTMLElement>(
            "input:not([disabled]),select:not([disabled]),button:not([disabled]),textarea:not([disabled]),a[href]",
          ) || [],
        ).filter((x) => x.offsetParent !== null);
        const first = elements[0],
          last = elements.at(-1);
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last?.focus();
        }
        if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first?.focus();
        }
      }
    }
    document.addEventListener("keydown", keys);
    const oldOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", keys);
      document.body.style.overflow = oldOverflow;
      old?.focus();
    };
  }, []);
  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        className={"dialog " + (wide ? "dialog-wide" : "")}
        ref={root}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
      >
        <div className="dialog-title">
          <h2 id={titleID}>{title}</h2>
          <button
            className="icon-button"
            onClick={onClose}
            aria-label="Dialog schließen"
          >
            <X size={17} />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}
export function Download({
  path,
  children,
}: {
  path: string;
  children: ReactNode;
}) {
  return (
    <a className="button secondary" href={"/api/" + path}>
      {children}
    </a>
  );
}
