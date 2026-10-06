import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { api, date } from "../api";
import { Dialog, Download, Field, LoadError } from "../components/shared";
import type { Notify } from "../components/shared";

type CertificateStatus = {
  enabled: boolean;
  managed: boolean;
  automatic: boolean;
  renew_before_days: number;
  expires_at?: string;
  valid_from?: string;
  days_remaining: number;
  fingerprint?: string;
  names?: string[];
  last_renewed_at?: string;
  message?: string;
  last_error?: string;
};

export default function Certificates({
  notify,
  onDirtyChange,
}: {
  notify: Notify;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const [status, setStatus] = useState<CertificateStatus | null>(null);
  const [automatic, setAutomatic] = useState(false),
    [days, setDays] = useState(30);
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false),
    [reload, setReload] = useState(0);
  const dirty =
    !!status?.managed &&
    (automatic !== status.automatic || days !== status.renew_before_days);
  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);
  useEffect(() => {
    let active = true;
    api<CertificateStatus>("tls")
      .then((value) => {
        if (active) {
          setStatus(value);
          setAutomatic(value.automatic);
          setDays(value.renew_before_days);
          setError("");
        }
      })
      .catch((e) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
    };
  }, [reload]);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const value = await api<CertificateStatus>("tls", "POST", {
        automatic,
        renew_before_days: days,
      });
      setStatus(value);
      setAutomatic(value.automatic);
      setDays(value.renew_before_days);
      notify("Zertifikatseinstellungen gespeichert");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function renew() {
    setBusy(true);
    setError("");
    try {
      const value = await api<CertificateStatus>("tls/renew", "POST", {});
      setStatus(value);
      setConfirm(false);
      notify(
        "Zertifikat erneuert. Neuen Fingerprint für die Browserfreigabe prüfen.",
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="update-section certificate-section">
      <div className="section-heading">
        <div>
          <h3>Webzertifikat</h3>
          <p className="muted">
            HTTPS und Erneuerung für den Webzugriff verwalten.
          </p>
        </div>
        {status?.managed && (
          <button
            type="button"
            className="secondary"
            disabled={busy || dirty}
            title={dirty ? "Zuerst die Einstellungen speichern" : undefined}
            onClick={() => {
              setError("");
              setConfirm(true);
            }}
          >
            Jetzt erneuern
          </button>
        )}
      </div>
      {!status ? (
        error ? (
          <LoadError message={error} retry={() => setReload((n) => n + 1)} />
        ) : (
          <p className="muted" role="status">
            Zertifikat wird geprüft …
          </p>
        )
      ) : (
        <>
          {!status.enabled ? (
            <p className="muted">{status.message}</p>
          ) : (
            <>
              <dl className="update-facts">
                <div>
                  <dt>Gültig bis</dt>
                  <dd
                    className={
                      status.days_remaining <= 0 ? "warning" : undefined
                    }
                  >
                    {date(status.expires_at)} ·{" "}
                    {status.days_remaining <= 0
                      ? "Abgelaufen"
                      : `${status.days_remaining} Tage verbleibend`}
                  </dd>
                </div>
                <div>
                  <dt>Webadresse</dt>
                  <dd>{status.names?.join(", ") || "—"}</dd>
                </div>
                <div>
                  <dt>Zuletzt erneuert</dt>
                  <dd>{date(status.last_renewed_at)}</dd>
                </div>
              </dl>
              <div className="certificate-fingerprint">
                <span className="muted">Fingerprint · SHA256</span>
                <code>{status.fingerprint}</code>
              </div>
              <p className="muted">{status.message}</p>
              <Download path="tls/certificate" notify={notify}>
                Öffentliches Zertifikat herunterladen
              </Download>
              {status.managed && (
                <form onSubmit={save} className="certificate-policy">
                  <fieldset className="form-fields" disabled={busy}>
                    <label className="check">
                      <input
                        type="checkbox"
                        checked={automatic}
                        onChange={(e) => setAutomatic(e.target.checked)}
                      />
                      Automatisch erneuern
                    </label>
                    <Field
                      label="Vorlauf in Tagen"
                      hint="7 bis 90 Tage. Prüfung beim Start und alle sechs Stunden."
                    >
                      <input
                        type="number"
                        min={7}
                        max={90}
                        required
                        value={days}
                        onChange={(e) => setDays(+e.target.value)}
                      />
                    </Field>
                    <p className="field-hint">
                      Gültigkeit nach Erneuerung: ein Jahr. Der private
                      Schlüssel bleibt erhalten. Nach einem Zertifikatswechsel
                      kann eine neue Browserfreigabe nötig sein.
                    </p>
                    <footer className="form-footer">
                      <span className="save-state" role="status">
                        {dirty
                          ? "Ungespeicherte Änderungen"
                          : "Alle Änderungen gespeichert"}
                      </span>
                      <button disabled={busy || !dirty}>
                        {busy && !confirm
                          ? "Speichert …"
                          : "Erneuerung speichern"}
                      </button>
                    </footer>
                  </fieldset>
                </form>
              )}
            </>
          )}
          {status.last_error && (
            <p className="notice warning" role="alert">
              Automatische Erneuerung fehlgeschlagen: {status.last_error}
            </p>
          )}
          {error && !confirm && (
            <p className="notice warning" role="alert">
              {error}
            </p>
          )}
        </>
      )}
      {confirm && (
        <Dialog
          title="Zertifikat erneuern"
          busy={busy}
          onClose={() => {
            setConfirm(false);
            setError("");
          }}
        >
          <p className="dialog-intro">
            Das neue Zertifikat gilt ein Jahr und wird ohne Dienstneustart
            übernommen.
          </p>
          <p>
            Der Fingerprint ändert sich. Eine neue Browserfreigabe kann
            erforderlich werden; dazu den neuen Fingerprint auf dem Server mit
            dem Browser vergleichen.
          </p>
          <p className="muted">
            Alternativ am Server: <code>sudo anker tls status</code>.
            Sicherungen und bestehende Sitzungen bleiben erhalten.
          </p>
          {error && (
            <p className="notice warning" role="alert">
              {error}
            </p>
          )}
          <footer className="dialog-footer">
            <button
              type="button"
              className="secondary"
              disabled={busy}
              onClick={() => {
                setConfirm(false);
                setError("");
              }}
            >
              Abbrechen
            </button>
            <button type="button" disabled={busy} onClick={renew}>
              {busy ? "Erneuert …" : "Zertifikat jetzt erneuern"}
            </button>
          </footer>
        </Dialog>
      )}
    </section>
  );
}
