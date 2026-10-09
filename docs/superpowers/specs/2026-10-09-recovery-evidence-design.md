# Anker: sichere Recovery-Nachweise und intuitive Dateiarbeit

## Auftrag

Der Nutzer hat den Produktaudit vom 9. Oktober freigegeben und zusätzlich einen intuitiven Dateibrowser mit gut auffindbaren Unterordnern verlangt. Kern bleiben Hostkonfigurationssicherung, Vergleich und zielbezogene Wiederherstellung. Keine allgemeine Serververwaltung. Die bestehende ruhige Gestaltung bleibt.

## Benutzerabläufe

1. Sicherung öffnen → Ordner und Unterordner sehen → über anklickbaren Pfad zurückgehen → Ordner oder Dateien global suchen → Treffer im Ordner zeigen → Datei ansehen/herunterladen oder ausdrücklich zur Wiederherstellung wählen. Auch leere und synthetische Ordner sind auffindbar. Wiederherstellungsdateien verwenden dieselbe Ordnerlogik; Auswahl bleibt über Suche, Seiten und Ordner erhalten. Ordnerauswahl darf keine unbemerkte rekursive Schreibfreigabe erzeugen.
2. Stand → direkt mit vorherigem Stand desselben Hosts vergleichen oder zwei Quellen ausdrücklich wählen → Originale unterscheiden → gewünschte Quelle auswählen. Zeitstempel und kurze IDs machen gleiche Minuten unterscheidbar. Volle IDs bleiben Details. Vergleichs- und Restoreauswahl bleiben explizit.
3. Sicherungsabdeckung prüfen → erfasste/fehlende/unklare Abhängigkeiten und zugelassene Wurzeln erkennen → konkrete nächste Schritte erhalten. Erfolgreiche Erfassung, Integrität, Abfrage-/Kontometadaten, Helferfähigkeit und Offline-/Drillnachweis sind getrennte Fakten mit Herkunft und Datum; kein pauschaler Wiederherstellbarkeitswert.
4. Recovery vorbereiten → aktuelle Zielaufnahme → Quelle/Ziel/vorbereitete Fassung schon bei Dateientscheidung vergleichen → erwartete Schreibwirkung sehen → Plan speichern → eigene Planseite mit nächster Aufgabe, Dateiaktionen und Nachweisen. Beibehalten: Hostidentität, Geheimnisrechte, Driftprüfung, Journal, Konfliktrücksetzung und separate Übernahmebestätigung.
5. Explizit „Wiederherstellung nachprüfen“ → feste lesende SSH-/Helferabfragen für diesen Plan → Ergebnis und Fehlversuche dauerhaft nachvollziehen. Keine automatischen Neustarts oder Aktivierungen. SSH-Vertrauen und aktuelle Verbindungsbindung prüfen. Boot-ID nur mit gültiger Vorher-Abfrage vergleichen; geänderte Boot-ID beweist nicht Gast-/Gesamtrecovery. Menschliche Notizen ersetzen keine maschinellen Ergebnisse.
6. Anker verloren → geschütztes versioniertes Installationspaket lokal prüfen → in neues leeres Ziel importieren → alte Sitzungen ungültig, Jobs nicht wieder ausführen, Schlüsselpfade korrekt übernehmen, Sicherungen prüfen. Kalter Export mit Instanzsperre; keine laufende Installation überschreiben. Geheimnisse bleiben verschlüsselt und erscheinen nicht in Inspect-Ausgabe, Argumenten oder Webdownloads. Sicherungsdaten können getrennt vorliegen; fehlende Quellen bleiben ausdrücklich offen.
7. Notfallpaket ohne Anker benutzen → Paket prüfen → Szenario, Artefakte, Abhängigkeiten, Haltepunkte und alle lesbaren Nachweise verstehen. Für echte PVE-Abnahme gibt es ein ausführbares manuell kontrolliertes Laborpaket mit Ergebnisformularen und Fehlerfällen. Der Nutzer hat bestätigt: derzeit kein isoliertes Testsystem. Keine reale Abnahme behaupten oder Produktionssysteme kontaktieren.

## Weitere fokussierte Ausbauten

- Neuester vollständiger Stand bleibt täglich geprüft. Zusätzlich höchstens ein älterer Stand je Host und Wartungslauf, geschützt zuerst, dann am längsten ungeprüft; bereits aktueller Stand wird nicht doppelt geprüft. Beschädigung versus Platzmangel/verschobene Prüfung getrennt behandeln.
- Unabhängige Kopie anhand tatsächlicher Manifest-/Dateibytes prüfen, Receipt an Backup-ID und Manifesthash binden. Empfangene/alte/manuelle Behauptungen sind kein Beweis unabhängiger aktueller Bytes. Kein allgemeiner Replikationsdienst.
- Benannter geschützter Referenzstand mit explizitem manuellen Drillnachweis; Änderungsübersicht gegenüber Referenz ohne mtime-Rauschen. Schutz allein bedeutet nicht getestet.
- Reviewte Mapping-Rezepte füllen nur voraus; jede neue Identität, Topologie, Version, NIC-/Storage-/Volumeabweichung erfordert neue Prüfung und Entscheidung. Keine Wiederverwendung alter Freigaben.
- Cluster-Sicherungssets bündeln Quellen mit erwarteten Nodes, realen Zeitfenstern, gemeinsamen logischen Konfigurationshashes und unvollständigen/divergenten Zuständen. Keine Quorum-/HA-/Ceph-/Join-Aktionen und kein Anspruch globaler Atomizität.

## Technik und Sicherheit

Go-Dienst und bestehender SQLite-Katalog bleiben Autorität. Neue Evidenz ist versioniert, konservativ bei Altformaten und serverseitig gegen Integrität/Identität/Geheimnisrechte geprüft. Go/Python-Protokolle akzeptieren keine beliebigen Kommandos oder vom Browser frei bestimmten Prüfpfade. Secretinhalte werden wie bisher maskiert. UI-Datenauswahl verliert veraltete Antworten und bestätigte Wirkung bei Ziel-/Quellenwechsel. Neue Darstellung baut auf vorhandenen Komponenten auf; Tabellenüberschriften erhalten mindestens 4,5:1 Textkontrast auf Weiß.

## Abnahme

Jedes Arbeitspaket erhält verhaltensbezogene Regressionen und unabhängigen Review. Abschließend Go Race/Vet, Python-Helfer, Webbuild, Browserregressionen, eingebettete/Linux-Builds und sichtbare Desktop-/Mobilprüfung. Echte Hardware, Stromverlust und vollständige PVE-Recovery bleiben bis zum vorhandenen Labor offen.
