# Abschlussprüfung

Ein frischer unabhängiger Reviewer prüfte die gesamte Implementierung von `05d9c75` bis `513ace0` read-only und reproduzierte Fehler in einer separaten temporären Kopie. Sein Urteil zu diesem Ausgangsstand war „noch nicht freigeben“. Danach wurden die folgenden Befunde korrigiert und lokal verifiziert.

| Befund | Korrektur / Regression |
|---|---|
| Einzeldateiauswahl erweitert sich unbemerkt | Feste Auswahlbedingung; Test mit früher Datei und späteren ungewählten Einträgen |
| Sicherungsschlüssel erlaubt root-Schreibzugriff | Read-only-Hostkanal; separat berechtigter Restoreaccount und eigener Schlüssel; Python-/SSH-Tests |
| Gängige Secrets unmaskiert | Konservative Pfadfreigabe, robustere Zuweisungserkennung, erneute Klassifikation beim Lesen und Diff |
| Transiente Fehler ohne Wiederholung | Result-ID nur für erfolgreiche Probe oder veröffentlichte Sicherung; Retry-Test |
| Beschädigte Reindex-Kandidaten bereits veröffentlicht | Kandidaten zuerst verifizieren; bisherige Prüfwerte und Schutzmarkierungen behalten; Korruptionstest |
| Archivierung löscht Dateien während Export/Planung | Gemeinsame Lesesperren und exklusive Mutation; pausierter Export im Regressionstest; fehlgeschlagene Streams abbrechen |
| Zweiter Dienst verändert aktive Aufträge | Datenverzeichnis exklusiv sperren, bevor Katalog oder Aufträge bearbeitet werden; Startup-Test |
| Metadatenfehler stillschweigend vollständig | Lesefehler als Pflichtlücke melden; Python-Test |
| Externe Config-/Hookreferenzen fehlen | Begrenzte Referenzprüfung mit ausdrücklichen Warnungen; Hook- und Schlüsseldateitest |
| Spätere Hosts im Terminal unsichtbar | Scrolloffset, stabile Auswahl und offsetbezogene Mausauswahl; 40-Host-Test |
| 8-MiB-Vorschau begrenzt Wiederherstellungspläne | Eigene geprüfte Recoverylesung bis Erfassungslimit; 9-MiB-Dateitest |
| Reader-Diff gesperrt; Metadatenänderungen unsichtbar | Diff als Leseoperation autorisieren; Eigentümer, Zeit, Typ und Attribute vergleichen |

Zusätzlich: volatile Lease-Laufzeiten aus Ziel-Fingerprints ausnehmen; Dateien und Verzeichnisse vor Veröffentlichung synchronisieren; neue Pläne erst nach vollständiger Datei-/Anleitungserstellung katalogisieren.

Nicht lokal belegt sind reale Proxmox-Version-/Decoderkompatibilität, physische Netzwerke, Boot, Cluster und die tatsächliche Linux-/SFTP-Installation. Diese Laborgrenzen bleiben ausdrücklich in `SUPPORT.md` stehen. Die Korrekturen wurden durch den implementierenden Agenten mit Regressionstests geprüft; das unabhängige Ausgangsurteil wird hier nicht als Freigabe eines späteren Standes ausgegeben.
