import { useEffect, useId, useState } from "react";
import { api, bytes, date } from "../api";
import { Dialog, LoadError } from "../components/shared";
import { beginUpdateReload, cancelUpdateReload } from "../useUpdateReload";
type UpdateState = {
  busy?: boolean;
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
type UpdateConfiguration = {
  repository: string;
  public_key: string;
  fingerprint: string;
  configured: boolean;
  official: boolean;
  has_token: boolean;
  official_repository: string;
  official_public_key: string;
  official_fingerprint: string;
  demo?: boolean;
};

function UpdateSourceDialog({
  configuration,
  onSaved,
  onClose,
}: {
  configuration: UpdateConfiguration;
  onSaved: (value: UpdateConfiguration) => void;
  onClose: () => void;
}) {
  const sourceID = useId();
  const [preset, setPreset] = useState(
      configuration.official ? "official" : "custom",
    ),
    [repository, setRepository] = useState(
      configuration.official ? "" : configuration.repository,
    ),
    [publicKey, setPublicKey] = useState(
      configuration.official ? "" : configuration.public_key,
    ),
    [fingerprint, setFingerprint] = useState(""),
    [trusted, setTrusted] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const selectedRepository =
    preset === "official"
      ? configuration.official_repository
      : repository.trim();
  const selectedKey =
    preset === "official"
      ? configuration.official_public_key
      : publicKey.trim();
  const selectedFingerprint =
    preset === "official" ? configuration.official_fingerprint : fingerprint;
  const validRepository =
    /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(selectedRepository) &&
    !selectedRepository.includes("..") &&
    selectedRepository.length <= 200;
  useEffect(() => {
    let active = true;
    setFingerprint("");
    async function compute() {
      try {
        const decoded = atob(publicKey.trim());
        if (decoded.length !== 32 || btoa(decoded) !== publicKey.trim()) return;
        const raw = Uint8Array.from(decoded, (char) => char.charCodeAt(0));
        const hash = await crypto.subtle.digest("SHA-256", raw);
        if (active)
          setFingerprint(
            "SHA256:" +
              Array.from(new Uint8Array(hash), (byte) =>
                byte.toString(16).padStart(2, "0"),
              ).join(""),
          );
      } catch {
        /* Invalid keys cannot be trusted or submitted. */
      }
    }
    compute();
    return () => {
      active = false;
    };
  }, [publicKey]);
  async function save() {
    if (!trusted || !validRepository || !selectedFingerprint) return;
    setBusy(true);
    setError("");
    try {
      onSaved(
        await api<UpdateConfiguration>("updates/configure", "POST", {
          repository: selectedRepository,
          public_key: selectedKey,
          confirmed: true,
        }),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog title="Updatequelle einrichten" busy={busy} onClose={onClose}>
      <p className="dialog-intro">
        Anker installiert nur Releases mit einer gültigen Signatur dieser
        Quelle.
      </p>
      <div className="update-source-form">
        <div className="update-source-selection">
          <label htmlFor={sourceID}>Updatequelle</label>
          <select
            id={sourceID}
            value={preset}
            disabled={busy}
            onChange={(e) => {
              setPreset(e.target.value);
              setTrusted(false);
              setError("");
            }}
          >
            <option value="official">Offizielle Anker-Releases</option>
            <option value="custom">Eigenes GitHub-Repository</option>
          </select>
        </div>
        {preset === "custom" && (
          <>
            <label>
              GitHub-Repository
              <input
                value={repository}
                disabled={busy}
                placeholder="owner/repo"
                autoComplete="off"
                onChange={(e) => {
                  setRepository(e.target.value);
                  setTrusted(false);
                }}
              />
            </label>
            <label>
              Öffentlicher Signierschlüssel
              <textarea
                className="mono"
                value={publicKey}
                disabled={busy}
                rows={3}
                placeholder="Ed25519, 32 Bytes als Base64"
                onChange={(e) => {
                  setPublicKey(e.target.value);
                  setTrusted(false);
                  setFingerprint("");
                }}
              />
            </label>
            <p className="muted">
              Repository und öffentlichen Schlüssel aus einer vertrauenswürdigen
              Quelle übernehmen und den Fingerprint vergleichen.
            </p>
          </>
        )}
        <dl className="update-source-facts">
          <div>
            <dt>Repository</dt>
            <dd>{selectedRepository || "Noch nicht angegeben"}</dd>
          </div>
          <div>
            <dt>Signierschlüssel-Fingerprint</dt>
            <dd className="mono">
              {selectedFingerprint || "Gültigen öffentlichen Schlüssel angeben"}
            </dd>
          </div>
        </dl>
        {configuration.has_token && (
          <p className="muted">
            Bei einem Repositorywechsel wird der gespeicherte Repositoryzugang
            entfernt.
          </p>
        )}
        <label className="check">
          <input
            type="checkbox"
            checked={trusted}
            disabled={busy || !validRepository || !selectedFingerprint}
            onChange={(e) => setTrusted(e.target.checked)}
          />
          Ich vertraue dieser Quelle und diesem Signierschlüssel
        </label>
      </div>
      {error && (
        <p className="notice warning" role="alert">
          {error}
        </p>
      )}
      <footer className="dialog-footer">
        <button className="secondary" disabled={busy} onClick={onClose}>
          Abbrechen
        </button>
        <button
          disabled={
            busy || !trusted || !validRepository || !selectedFingerprint
          }
          onClick={save}
        >
          {busy ? "Speichert …" : "Quelle speichern"}
        </button>
      </footer>
    </Dialog>
  );
}

export default function Updates({
  canInstall = true,
}: {
  canInstall?: boolean;
}) {
  const [state, setState] = useState<UpdateState | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [confirm, setConfirm] = useState(false),
    [reload, setReload] = useState(0);
  const [configuration, setConfiguration] =
      useState<UpdateConfiguration | null>(null),
    [configurationError, setConfigurationError] = useState(""),
    [configure, setConfigure] = useState(false);
  const installing = state?.status === "installing";
  useEffect(() => {
    let active = true;
    api<UpdateConfiguration>("updates/configuration")
      .then((value) => {
        if (active) {
          setConfiguration(value);
          setConfigurationError("");
        }
      })
      .catch((e) => {
        if (active) setConfigurationError((e as Error).message);
      });
    return () => {
      active = false;
    };
  }, [reload]);
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const load = async () => {
      try {
        const v = await api<UpdateState>("updates");
        if (active) {
          setState(v);
          setError("");
          if (v.status === "installing") {
            if (v.target) beginUpdateReload(v.target);
            timer = setTimeout(load, 2000);
          }
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
    beginUpdateReload(state.available.version);
    try {
      const next = await api<UpdateState>("updates/install", "POST", {
        version: state.available.version,
      });
      setState(next);
      beginUpdateReload(next.target || state.available.version);
      setConfirm(false);
      setReload((n) => n + 1);
    } catch (e) {
      setError((e as Error).message);
      const message = (e as Error).message;
      if (
        !message.startsWith("Anker ist nicht erreichbar") &&
        !message.startsWith("Ungültige Serverantwort")
      )
        cancelUpdateReload();
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
        <div className="update-actions">
          <button
            type="button"
            className="secondary"
            disabled={
              busy ||
              installing ||
              state?.busy ||
              !state ||
              !configuration ||
              configuration.demo
            }
            onClick={() => setConfigure(true)}
          >
            {state?.configured
              ? "Updatequelle ändern"
              : "Updatequelle einrichten"}
          </button>
          <button
            type="button"
            className="secondary"
            disabled={busy || installing || state?.busy || !state?.configured}
            onClick={check}
          >
            {busy && !confirm ? "Prüft …" : "Nach Updates suchen"}
          </button>
        </div>
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
                {configuration?.demo
                  ? "Die Updatequelle lässt sich auf dem eigenen Server direkt hier einrichten."
                  : "Offizielle Anker-Releases auswählen, Signierschlüssel bestätigen und nach Updates suchen. Die Einrichtung benötigt keinen Dienstneustart."}
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
                    die vorherige Version zurück. Nach erfolgreichem Start wird
                    diese Seite automatisch neu geladen.
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
                    disabled={!canInstall}
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
              {!canInstall && !installing && (
                <p className="field-hint">
                  Änderungen in den Einstellungen vor dem Update speichern.
                </p>
              )}
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
      {configurationError && (
        <LoadError
          message={configurationError}
          retry={() => setReload((n) => n + 1)}
        />
      )}
      {configure && configuration && (
        <UpdateSourceDialog
          configuration={configuration}
          onClose={() => setConfigure(false)}
          onSaved={(value) => {
            setConfiguration(value);
            setConfigure(false);
            setError("");
            setState((previous) =>
              previous
                ? {
                    ...previous,
                    configured: true,
                    repository: value.repository,
                    available: undefined,
                    checked_at: undefined,
                    status: "idle",
                    message:
                      "Updatequelle eingerichtet. Jetzt nach signierten Releases suchen.",
                  }
                : previous,
            );
            setReload((n) => n + 1);
          }}
        />
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
