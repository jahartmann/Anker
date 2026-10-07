import { useEffect, useState } from "react";
import { api } from "./api";
import type { Notify } from "./components/shared";

const key = "anker:update-pending";
const event = "anker:update-pending-changed";
type Pending = { target: string; started: number };
let fallback: Pending | null = null;

function readPending(): Pending | null {
  try {
    const raw = sessionStorage.getItem(key);
    const value = raw ? JSON.parse(raw) : fallback;
    if (!value) return null;
    return typeof value.target === "string" &&
      /^v?\d+\.\d+\.\d+$/.test(value.target) &&
      Number.isFinite(value.started) &&
      Date.now() - value.started < 30 * 60 * 1000
      ? value
      : null;
  } catch {
    return fallback && Date.now() - fallback.started < 30 * 60 * 1000
      ? fallback
      : null;
  }
}
export function beginUpdateReload(target: string) {
  const previous = readPending();
  if (previous?.target === target) return;
  fallback =
    previous?.target === target ? previous : { target, started: Date.now() };
  try {
    sessionStorage.setItem(key, JSON.stringify(fallback));
  } catch {
    /* In-memory recovery still works. */
  }
  window.dispatchEvent(new Event(event));
}
function clearPending() {
  fallback = null;
  try {
    sessionStorage.removeItem(key);
  } catch {
    /* Storage may be disabled. */
  }
  window.dispatchEvent(new Event(event));
}
export const cancelUpdateReload = clearPending;

// Keep observing across view changes and temporary service restarts.
export function useUpdateReload(enabled: boolean, notify: Notify) {
  const [pending, setPending] = useState<Pending | null>(readPending);
  useEffect(() => {
    const changed = () => setPending(readPending());
    window.addEventListener(event, changed);
    return () => window.removeEventListener(event, changed);
  }, []);
  useEffect(() => {
    if (!enabled || !pending) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function check() {
      if (Date.now() - pending!.started > 30 * 60 * 1000) {
        clearPending();
        notify(
          "Updateergebnis noch nicht bestätigt. Unter System → Updates erneut prüfen.",
          true,
        );
        return;
      }
      try {
        const state = await api<{
          status: string;
          current: string;
          target?: string;
        }>("updates");
        if (!active) return;
        const sameVersion =
          state.current?.replace(/^v/, "") ===
          pending!.target.replace(/^v/, "");
        if (state.status === "successful" && sameVersion) {
          clearPending();
          window.location.reload();
          return;
        }
        if (
          ["failed", "rolled_back"].includes(state.status) &&
          (!state.target || state.target === pending!.target)
        ) {
          clearPending();
          notify(
            "Update nicht abgeschlossen. Die vorherige Version bleibt aktiv; Details unter System → Updates.",
            true,
          );
          return;
        }
      } catch {
        /* The service is expected to be unavailable during restart. */
      }
      if (active) timer = setTimeout(check, 2000);
    }
    // Give the accepted install request time to reach its persisted state.
    timer = setTimeout(check, 1000);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [enabled, pending, notify]);
  return enabled ? pending : null;
}
