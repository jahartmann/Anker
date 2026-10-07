# Robuste Wiederherstellung und relevantes Mapping

> Umsetzung: getrennte Arbeitsbereiche für Hosthelfer, Mapping-Dienst und Oberfläche; gemeinsame Integration und Prüfung vor Veröffentlichung. Testfälle werden vor den jeweiligen Korrekturen ausgeführt.

**Ziel:** Nur benötigte Zuordnungen zeigen, eindeutig vorbereiten und Einzeldateiübernahmen mit dauerhaften Recovery-Nachweisen absichern. Die bestehende Notion-Gestaltung bleibt erhalten.

**Grundlage:** `docs/RECOVERY_REVIEW.md` und die ausdrückliche Nutzeranweisung vom 7. Oktober 2026, die Fehler sowie Mapping und UI zu überarbeiten.

**Grenzen:** Keine Formatierung, Partitionierung, ungetestete Cluster-/Gesamtausführung oder automatische Netzwerkaktivierung. Netzwerkdateien werden vorbereitet und als manuelle Konsolenschritte exportiert, bis ein unabhängig geprüfter Rückweg vorhanden ist. Das beseitigt die ungesicherte automatische Übernahme; ein Konsolenhaken darf diese Grenze nicht umgehen. Bestehende Originale bleiben unverändert.

## 1. Hostausführung

- [x] Fehlerursachen, Konkurrenzänderungen, Metadatenabweichungen, doppelte Aufrufe, Teilübernahme und kontrollierten Rollback zuerst reproduzieren.
- [x] Lokale Exklusivsperre, erforderliche Metadaten-/Benutzerprüfung, staging und planbezogenes synchronisiertes Journal einführen.
- [x] Jeden Dateizustand unmittelbar vor Übernahme prüfen; Ergebnis nachlesen. Bei gewöhnlichem Ausführungsfehler bereits geschriebene Dateien nur bei passenden Nachherbedingungen rücksetzen. Externe Änderungen erhalten.
- [x] Statusabfrage und explizit bestätigten Rollback anhand Recovery-ID anbieten. Unklare/persistierte Ausführungen nicht erneut anwenden.
- [x] Inventarkommandos mit nachvollziehbaren Ergebnissen, physischen Ports, Datenträgertypen und Restore-Fähigkeit erfassen.

## 2. Szenariobezogenes Mapping

- [x] Irrelevante virtuelle Ports, ungenutzte Platten, doppelte Portzuordnungen, unbekannte Referenzen und alle sieben Szenarien zuerst testen.
- [x] `POST /api/plans/inspect` liest Sicherung und aktuelles Ziel ohne Plan oder Mutationen. Liefert nur verwendete physische Quellports, geeignete physische Zielports, referenzierte Storage-Konfiguration und szenariobezogene Hinweise.
- [x] `CreatePlan` verwendet dieselbe Analyse, verwirft keine Entscheidung still und prüft Zuordnungen erneut. Unbekannte Pfade/Inventare fail-closed. Identität/Storage bleibt eine sichtbar manuelle Entscheidung, kein dekoratives automatisches Mapping.
- [x] Netzwerkvorbereitung, VLAN-Abhängigkeiten und eindeutige Portrollen verbessern; vorbereitete manuelle Dateien ebenfalls mit Hash/Diff exportieren.

## 3. Geführte Oberfläche

- [x] Browserregressionen für kleine relevante Listen, Auswahlwechsel, verspätete Antworten, Prüfungsfehler, Wiederholung und Ergebniszustände schreiben.
- [x] Bestehenden Assistenten in Quelle/Auswahl, Zielprüfung/Zuordnung und Plan unterteilen; frische Prüfung ohne Umweg über blockierten Plan.
- [x] Nur nötige Ports und referenzierte Speicher zeigen. Schlüssige Vorschläge kenntlich machen; Unklarheit als konkrete Entscheidung. Große Listen begrenzen/suchen, lange Nachweise aufklappbar.
- [x] Keine verlorenen Auswahlen oder Bestätigungen nach Wechsel; laufende Prüfung nicht doppelt starten oder wegklicken.
- [x] Übernahme, Rücksetzung, ungeklärten Hostzustand und noch ausstehende Betriebsprüfung getrennt zeigen.

## 4. Integration und Abschluss

- [x] SSH-/Plan-/Status-/Rollback-Kette einschließlich alter Hosthelfer und Dienstneustart verbinden; alte Helfer bis zur erneuten Anbindung sicher sperren.
- [x] Terminal/CLI informieren über dieselben Schutzgrenzen und relevante Zuordnungen.
- [x] Vollständige Go-/Python-/Browserprüfungen, Desktop-/Mobilprüfung und frische Codeprüfung durchführen.
- [x] Anleitung und tatsächlichen Unterstützungsstand aktualisieren; geprüften signierten Release veröffentlichen, öffentliche Downloads prüfen.

Prüfschwerpunkte: Fremdänderung trotz gleicher Bytes/anderer Rechte; Antwortverlust nach Dateiaustausch; fehlende erforderliche Inventarabfrage; Portalias/VLAN/Bond; Storage-Abhängigkeiten; neu angeschlossener alter Helfer; falsche SSH-Identität; geringe Kapazität während Staging; keine Behauptung einer global atomaren Hosttransaktion.

Lokale Abschlussprüfung: vollständige Go-Suite mit Race Detector, Vet, 53 Python-Helferprüfungen und 14 Installerprüfungen bestanden. Eingebettete Weboberfläche: 83 Browserfälle bestanden; nach zusätzlicher Fokus-/Kopfzeilenkorrektur 20 einschlägige Fälle erneut bestanden. Laptop 1280×720 und Mobil 390×844 visuell kontrolliert. Unabhängige Codeprüfung und Nachprüfung korrigierten Protokoll-, Zugangs-, Antwort-, Navigations- und Mappingfälle. GitHub Checks 37630894379 und Release 37630929952 am unveränderten Code 16b3a30b8596a0dfe63cf9fc837ef879a1ea87b1 bestanden, einschließlich Linux-/systemd-Installation und Update-Rückfall. Signierte amd64-/arm64-Pakete unabhängig geprüft und 0.2.8 veröffentlicht.

Öffentlicher Updateweg: tatsächlicher Anker-Updater hat 0.2.8 und beide Binärdateien ohne Token mit offizieller Signaturprüfung abgerufen.
