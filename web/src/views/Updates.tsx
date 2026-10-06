import { useEffect, useState } from "react";
import { api, bytes, date } from "../api";
import { Dialog, LoadError } from "../components/shared";
type UpdateState = {
  configured: boolean;
  repository: string;
  current: string;
  status: string;
  message?: string;
  checked_at?: string;
  updated_at?: string;
  target?: string;
  available?: { version: string; url: string; artifact: { size: number } };
};
export default function Updates() {
  const [state, setState] = useState<UpdateState | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [confirm, setConfirm] = useState(false),
    [reload, setReload] = useState(0);
  const installing = state?.status === "installing";
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const load = async () => {
      try {
        const v = await api<UpdateState>("updates");
        if (active) {
          setState(v);
          setError("");
          if (v.status === "installing") timer = setTimeout(load, 2000);
        }
      } catch (e) {
        if (active) {
          setError((e as Error).message);
          if (installing) timer = setTimeout(load, 2000);
        }
      }
    };
    load();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [reload, installing]);
  async function check() {
    setBusy(true);
    setError("");
    try {
      setState(await api<UpdateState>("updates/check", "POST", {}));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function install() {
    if (!state?.available) return;
    setBusy(true);
    setError("");
    try {
      const next = await api<UpdateState>("updates/install", "POST", {
        version: state.available.version,
      });
      setState(next);
      setConfirm(false);
      setReload((n) => n + 1);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="update-section">
      <div className="section-heading">
        <div>
          <h3>Updates</h3>
          <p className="muted">
            Signierte Releases installieren. Einstellungen und Sicherungen
            bleiben erhalten.
          </p>
        </div>
        <button
          type="button"
          className="secondary"
          disabled={busy || installing || !state?.configured}
          onClick={check}
        >
          {busy && !confirm ? "Prüft …" : "Nach Updates suchen"}
        </button>
      </div>
      {!state ? (
        error ? (
          <LoadError message={error} retry={() => setReload((n) => n + 1)} />
        ) : (
          <p className="muted" role="status">
            Updatezustand wird geladen …
          </p>
        )
      ) : (
        <>
          <dl className="update-facts">
            <div>
              <dt>Installierte Version</dt>
              <dd className="mono">{state.current}</dd>
            </div>
            {state.configured && (
              <div>
                <dt>Quelle</dt>
                <dd>
                  <a
                    href={"https://github.com/" + state.repository}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {state.repository}
                  </a>
                </dd>
              </div>
            )}
            <div>
              <dt>Letzte Prüfung</dt>
              <dd>{date(state.checked_at)}</dd>
            </div>
          </dl>
          {!state.configured ? (
            <div className="update-note">
              <p>{state.message}</p>
              <p className="muted">
                Auf dem Linux-Server <code>sudo anker setup --updates</code>{" "}
                ausführen und den geprüften öffentlichen Signierschlüssel
                angeben. Die Schritte stehen in der Update-Anleitung.
              </p>
            </div>
          ) : (
            <>
              {installing ? (
                <div className="update-note" role="status">
                  <strong>{state.target} wird installiert …</strong>
                  <p>
                    Die Oberfläche kann beim Neustart kurz nicht erreichbar
                    sein. Anker prüft den Start und fällt bei einem Fehler auf
                    die vorherige Version zurück.
                  </p>
                </div>
              ) : state.available ? (
                <div className="update-release">
                  <div>
                    <strong>{state.available.version} ist verfügbar</strong>
                    <p className="muted">
                      {bytes(state.available.artifact.size)} · Signatur geprüft
                      ·{" "}
                      <a
                        href={state.available.url}
                        target="_blank"
                        rel="noreferrer"
                      >
                        Release ansehen
                      </a>
                    </p>
                  </div>
                  <button
                    onClick={() => {
                      setError("");
                      setConfirm(true);
                    }}
                  >
                    Update installieren
                  </button>
                </div>
              ) : state.checked_at && state.status !== "check_failed" ? (
                <p className="muted">Keine neuere stabile Version verfügbar.</p>
              ) : null}
              {state.message && !installing && (
                <p
                  className={
                    state.status === "failed" || state.status === "check_failed"
                      ? "notice warning"
                      : "update-message"
                  }
                  role={
                    state.status === "failed" || state.status === "check_failed"
                      ? "alert"
                      : "status"
                  }
                >
                  {state.message}
                </p>
              )}
            </>
          )}
          {error && !confirm && (
            <p className="notice warning" role="alert">
              {installing
                ? "Dienst wird neu gestartet. Die Verbindung wird erneut geprüft."
                : error}
            </p>
          )}
        </>
      )}
      {confirm && state?.available && (
        <Dialog
          title="Update installieren"
          busy={busy}
          onClose={() => setConfirm(false)}
        >
          <p className="dialog-intro">
            Anker {state.current} wird durch {state.available.version} ersetzt.
          </p>
          <p>
            Der Dienst startet dafür neu. Aktive Aufträge müssen vorher beendet
            sein. Bei einem fehlgeschlagenen Start werden die vorherige
            Programmversion und der Katalog wiederhergestellt.
          </p>
          <p className="muted">
            Die Anmeldung bleibt nach dem Update erhalten.
          </p>
          {error && (
            <p className="notice warning" role="alert">
              {error}
            </p>
          )}
          <footer className="dialog-footer">
            <button
              className="secondary"
              disabled={busy}
              onClick={() => setConfirm(false)}
            >
              Abbrechen
            </button>
            <button disabled={busy} onClick={install}>
              {busy ? "Wird angefordert …" : "Jetzt installieren"}
            </button>
          </footer>
        </Dialog>
      )}
    </section>
  );
}
