import { Field } from "../shared";
import type { RecoveryDraft } from "./useRecoveryDraft";

export default function RecoveryNetwork({
  draft: d,
}: {
  draft: RecoveryDraft;
}) {
  const stanzas = d.mappingInspection?.network_stanzas || [];
  if (!stanzas.length) return null;
  return (
    <section aria-label="Netzwerk vorbereiten">
      <h3>Managementnetz vorbereiten</h3>
      <p>
        Quellstanza und IP-Familie ausdrücklich auswählen. Leere Werte bleiben
        erhalten. Schreiben und Aktivierung über die Konsole bleiben manuell.
      </p>
      {stanzas.map((stanza) => {
        const value = d.network[stanza.key];
        const update = (field: string, next: string | string[]) =>
          d.setNetwork({
            ...d.network,
            [stanza.key]: { ...value, [field]: next },
          });
        return (
          <fieldset key={stanza.key}>
            <legend>
              {stanza.path} · {stanza.key}
            </legend>
            <label className="check">
              <input
                type="checkbox"
                checked={!!value}
                onChange={(e) => {
                  const next = { ...d.network };
                  if (e.target.checked) next[stanza.key] = {};
                  else delete next[stanza.key];
                  d.setNetwork(next);
                }}
              />
              Stanza {stanza.key} anpassen
            </label>
            {value && (
              <>
                <Field
                  label={`Adresse/CIDR für ${stanza.key}`}
                  hint={
                    stanza.family === "inet"
                      ? "IPv4 mit Präfix, z. B. 192.0.2.10/24"
                      : "IPv6 mit Präfix, z. B. 2001:db8::10/64"
                  }
                >
                  <input
                    value={value.address_cidr || ""}
                    onChange={(e) =>
                      update("address_cidr", e.target.value.trim())
                    }
                  />
                </Field>
                <Field label={`Gateway für ${stanza.key}`}>
                  <input
                    value={value.gateway || ""}
                    onChange={(e) => update("gateway", e.target.value.trim())}
                  />
                </Field>
                <Field
                  label={`DNS-Server für ${stanza.key}`}
                  hint="IP-Adressen mit Komma trennen; der Dienst prüft alle Werte."
                >
                  <input
                    value={(value.dns_servers || []).join(",")}
                    onChange={(e) =>
                      update(
                        "dns_servers",
                        e.target.value.split(",").map((v) => v.trim()),
                      )
                    }
                  />
                </Field>
              </>
            )}
          </fieldset>
        );
      })}
    </section>
  );
}
