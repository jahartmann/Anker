import {
  useEffect,
  useRef,
  useId,
  useState,
  useLayoutEffect,
  isValidElement,
  cloneElement,
} from "react";
import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import { Anchor, X, MoreHorizontal } from "lucide-react";
import { api, labels } from "../api";
export type Notify = (message: string, error?: boolean) => void;
export function BrandMark() {
  return (
    <span className="brand-mark" aria-hidden="true">
      <Anchor size={20} strokeWidth={2.2} />
    </span>
  );
}
export function State({ value, label }: { value: string; label?: string }) {
  const tone = ["successful", "demo_applied"].includes(value)
    ? "success"
    : ["failed", "blocked", "damaged"].includes(value)
      ? "danger"
      : ["interrupted", "partial"].includes(value)
        ? "warning"
        : ["running", "applying"].includes(value)
          ? "active"
          : value === "queued"
            ? "queued"
            : "muted";
  return (
    <span className={"state " + tone}>{label || labels[value] || value}</span>
  );
}
export function Pagination({
  page,
  pageSize,
  total,
  onPageChange,
  label = "Einträge",
  previousLabel = "Vorherige Seite",
  nextLabel = "Nächste Seite",
}: {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  label?: string;
  previousLabel?: string;
  nextLabel?: string;
}) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const current = Math.max(1, Math.min(page, pages));
  return (
    <nav className="pagination" aria-label={label + " · Seiten"}>
      <span className="pagination-range">
        {total ? (current - 1) * pageSize + 1 : 0}–
        {Math.min(current * pageSize, total)} von {total} {label}
      </span>
      {pages > 1 && (
        <div className="pagination-controls">
          <button
            type="button"
            className="secondary"
            aria-label={previousLabel}
            disabled={current === 1}
            onClick={() => onPageChange(current - 1)}
          >
            Zurück
          </button>
          <span className="pagination-page">
            {current} / {pages}
          </span>
          <button
            type="button"
            className="secondary"
            aria-label={nextLabel}
            disabled={current === pages}
            onClick={() => onPageChange(current + 1)}
          >
            Weiter
          </button>
        </div>
      )}
    </nav>
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
export function LoadError({
  message,
  retry,
  label = "Erneut laden",
}: {
  message: string;
  retry: () => void;
  label?: string;
}) {
  return (
    <div className="load-error">
      <p className="warning" role="alert">
        {message}
      </p>
      <button type="button" className="secondary" onClick={retry}>
        {label}
      </button>
    </div>
  );
}

export function ActionMenu({
  label,
  disabled,
  actions,
}: {
  label: string;
  disabled?: boolean;
  actions: { label: string; run: () => void }[];
}) {
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState<{
    left: number;
    top: number;
  } | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const id = useId();
  useLayoutEffect(() => {
    if (!open || !trigger.current || !panel.current) return;
    const anchor = trigger.current.getBoundingClientRect();
    const bounds = panel.current.getBoundingClientRect();
    setPosition({
      left: Math.max(
        12,
        Math.min(
          anchor.right - bounds.width,
          window.innerWidth - bounds.width - 12,
        ),
      ),
      top:
        anchor.bottom + bounds.height + 12 <= window.innerHeight
          ? anchor.bottom + 6
          : Math.max(12, anchor.top - bounds.height - 6),
    });
  }, [open]);
  useLayoutEffect(() => {
    if (open && position)
      panel.current?.querySelector<HTMLButtonElement>("button")?.focus();
  }, [open, position]);
  useEffect(() => {
    if (!open) return;
    function outside(e: PointerEvent) {
      if (
        !panel.current?.contains(e.target as Node) &&
        !trigger.current?.contains(e.target as Node)
      )
        setOpen(false);
    }
    function moved(e: Event) {
      if (!(e.target instanceof Node) || !panel.current?.contains(e.target))
        setOpen(false);
    }
    document.addEventListener("pointerdown", outside);
    window.addEventListener("resize", moved);
    window.addEventListener("scroll", moved, true);
    return () => {
      document.removeEventListener("pointerdown", outside);
      window.removeEventListener("resize", moved);
      window.removeEventListener("scroll", moved, true);
    };
  }, [open]);
  return (
    <div className="action-menu">
      <button
        className="icon-button"
        aria-label={label}
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        disabled={disabled}
        ref={trigger}
        onClick={() => {
          setPosition(null);
          setOpen(!open);
        }}
      >
        <MoreHorizontal size={18} aria-hidden="true" />
      </button>
      {open &&
        createPortal(
          <div
            ref={panel}
            id={id}
            className="action-popover"
            aria-label={label}
            style={{
              left: position?.left ?? 0,
              top: position?.top ?? 0,
              visibility: position ? "visible" : "hidden",
            }}
            onBlur={(e) => {
              if (
                !(e.relatedTarget instanceof Node) ||
                !e.currentTarget.contains(e.relatedTarget)
              )
                setOpen(false);
            }}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                e.preventDefault();
                setOpen(false);
                trigger.current?.focus();
              }
              if (["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) {
                e.preventDefault();
                const buttons = Array.from(
                  e.currentTarget.querySelectorAll("button"),
                );
                const index = buttons.indexOf(
                  document.activeElement as HTMLButtonElement,
                );
                buttons[
                  e.key === "Home"
                    ? 0
                    : e.key === "End"
                      ? buttons.length - 1
                      : (index +
                          (e.key === "ArrowDown" ? 1 : -1) +
                          buttons.length) %
                        buttons.length
                ]?.focus();
              }
            }}
          >
            {actions.map((action) => (
              <button
                type="button"
                key={action.label}
                onClick={() => {
                  setOpen(false);
                  trigger.current?.focus();
                  action.run();
                }}
              >
                {action.label}
              </button>
            ))}
          </div>,
          document.body,
        )}
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
  busy = false,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
  busy?: boolean;
}) {
  const root = useRef<HTMLDivElement>(null);
  const titleID = useId();
  const close = useRef(onClose);
  close.current = () => {
    if (!busy) onClose();
  };
  useEffect(() => {
    const old = document.activeElement as HTMLElement;
    const node = root.current;
    (
      node?.querySelector<HTMLElement>("input,select,textarea") ||
      node?.querySelector<HTMLElement>("button")
    )?.focus();
    function keys(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        close.current();
        return;
      }
      if (e.key === "Tab") {
        const elements = Array.from(
          node?.querySelectorAll<HTMLElement>(
            "input:not([disabled]),select:not([disabled]),button:not([disabled]),textarea:not([disabled]),a[href],summary",
          ) || [],
        ).filter((x) => x.offsetParent !== null);
        const first = elements[0],
          last = elements.at(-1);
        if (!node?.contains(document.activeElement)) {
          e.preventDefault();
          first?.focus();
        } else if (e.shiftKey && document.activeElement === first) {
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
    <div className="dialog-backdrop">
      <div
        className={"dialog " + (wide ? "dialog-wide" : "")}
        ref={root}
        role="dialog"
        aria-modal="true"
        aria-busy={busy}
        aria-labelledby={titleID}
      >
        <div className="dialog-title">
          <h2 id={titleID}>{title}</h2>
          <button
            className="icon-button"
            onClick={onClose}
            disabled={busy}
            type="button"
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
  notify,
  compact = false,
}: {
  path: string;
  children: ReactNode;
  notify?: Notify;
  compact?: boolean;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef(false);
  return (
    <>
      <a
        className={compact ? "text-button" : "button secondary"}
        href={"/api/" + path}
        download
        aria-disabled={busy}
        onClick={async (e) => {
          e.preventDefault();
          if (active.current) return;
          active.current = true;
          setBusy(true);
          setError("");
          try {
            await api(path + (path.includes("?") ? "&" : "?") + "check=1");
            const link = document.createElement("a");
            link.href = "/api/" + path;
            link.download = "";
            document.body.appendChild(link);
            link.click();
            link.remove();
          } catch (e) {
            const message = (e as Error).message;
            if (notify) notify(message, true);
            else setError(message);
          } finally {
            active.current = false;
            setBusy(false);
          }
        }}
      >
        {busy ? "Prüft …" : children}
      </a>
      {error && (
        <span className="warning" role="alert">
          {error}
        </span>
      )}
    </>
  );
}
