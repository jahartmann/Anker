# Änderungen

## Unveröffentlicht

- Bestätigungen im Wiederherstellungsplan bleiben bei identischen Hintergrundmeldungen bedienbar. Plan- und Statusantworten werden mit gleicher Inventarprojektion und sortierten Schlüsseln verglichen; abweichende Zustände erfordern weiterhin eine erneute Abfrage. Nachweise übernehmen den Eingabetext direkt im Ereignis. Der Browsertest prüft den gespeicherten Zustand jeder Aufgabe und nutzt für Wiederholungen getrennte Testhostnamen.

- Interner Testkandidat `0.2.0-internal.20261026`: vollständige lokale Go-/Browser-/Python-/Terminalprüfung, Linux-Crossbuilds und dokumentierte Laborgrenzen. Go auf 1.27.3 und Vite fest auf 8.3.5 aktualisiert. Browser-Integration nutzt einen eigenen Demo-Datenbestand; historische Vergleiche prüfen weiterhin das exakte Quellenpaar.

- Dezente Infrastruktur-Details im bestehenden hellen Design: eigene Rack-Symbole, technische Akzente an Cards und Statusanzeigen sowie einheitliche Zahlendarstellung. Hostseiten zeigen das konfigurierte SSH-Ziel für Sicherungen, auch bei IPv6-Adressen. Navigation, Card-Raster und Bedienabläufe bleiben erhalten.

- Sicherungsseite mit einem kompakten Host-Fokus: letzter Stand, Zusammenfassung der Änderungen und geeignete Wiederherstellungsquelle. Auffällige neueste Stände und unklare Referenzen stehen bei der Auswahl aus allen Hosts zuerst. Vergleiche übernehmen genau die angezeigten Stände; die Wiederherstellung geht weiterhin durch den Assistenten. Sechs Hostgruppen pro Seite, ältere Stände bleiben nach Monat und Tag aufklappbar.
- Zusammengehörige Cards in Sicherungs-, Host- und Änderungsansichten nutzen gleich hohe Rasterzeilen und einheitliche Abstände. Aktionen stehen am unteren Rand. Der zusätzliche Außenabstand an der Host-Änderungskarte entfällt. Breite, Höhe und Aktionsausrichtung sind durch Browserprüfungen abgedeckt.

- Hostseiten zeigen zuerst den aktuellen Handlungsbedarf und eine passende Aktion. Laufende Aufträge haben Vorrang; eine fehlgeschlagene Verbindung bleibt auch bei aktueller Sicherung sichtbar. Die Übersicht bündelt Dateien, Änderungen und Wiederherstellung in drei Cards. Änderungsspur und Zeitstrahl liegen im Bereich Änderungen, technische Details sind aufklappbar. Die Hostliste zeigt acht Einträge pro Seite und einen direkten Filter für offene Punkte.
- Kürzere, einfachere Texte in Übersicht, Kompass, Hoststatus, Sicherungsvergleich, Verlauf, Einstellungen und Wiederherstellung. Prüfergebnisse und gespeicherte Werte sind klar von Prüfungen am laufenden Host getrennt.

- Anker Kompass als gemeinsamer Aufgabeneinstieg in allen Ansichten (⌘/Strg + K): Zustand verstehen, Sicherungen ansehen, Konfigurationsreise und Wiederherstellung vorbereiten. Berücksichtigt den geöffneten Host, aktive Aufträge, Leserechte und geeignete Quellen; schützt offene Entwürfe und löst keine Aufträge aus. Hostsuche nach Anzeigename, Adresse und Gruppe mit vier Treffern je Seite. Die Übersicht bietet dieselben Aufgaben als ruhige Cards mit dezenten Farbschatten.

- Automatische lesende Ergebnisprüfung nach Dateiübernahme und Rücksetzung: Soll-/Ist-Prüfsummen, Dateieigenschaften und vorgesehene Entfernungen; unterstützte Einstellungsdeklarationen samt Konflikterkennung. Kompakte Ergebnis-Cards, priorisierte Abweichungen, aufklappbare Nachweise und erneute Zielabfrage. Fehlende oder ungültige Nachweise bleiben offen; Prüfversuche werden getrennt vom unveränderlichen Plan gespeichert.

- Geprüfte Dateientfernungen im Wiederherstellungsassistenten: zunächst abgewählt, aktuelle Zielaufnahme, eigene Dateientscheidung und Löschbestätigungen. Dauerhaftes Rücksetzjournal vor dem Löschen, Wiederherstellung von Inhalt und Rechten sowie Konfliktschutz bei neu angelegten Dateien. Neue Hostfähigkeit `remove_files`; ältere Helfer geben keine Entfernung frei. Kompakte Cards mit aufklappbarem weiterem Umfang und Demorücksetzung.

- Wiederherstellungsvorschau mit dem Assistenten verbunden: Dateiauswahl in kompakten Cards, vorausgewählte Quelle und ursprüngliche Node, anschließend zwingend neue Zielprüfung und ausdrückliche Dateientscheidungen. Keine automatische Plananlage oder Ausführung; fehlende Quellen blockieren eine unvollständige Übergabe.

- Lesende Wiederherstellungsvorschau direkt aus belegten Einstellungsänderungen: gewünschter früherer Wert, vollständige Dateiunterschiede in Rückkehrrichtung, zusätzliche Werte im Dateiumfang sowie sichtbare Anlage-/Entfernungs- und Metadatenänderungen. Vier Dateien je Seite; veraltete oder unvollständig lesbare Nachweise werden abgewiesen. Kein Livezugriff, keine Plananlage und keine Übernahme.

- Verständliche Einordnung geänderter Einstellungen in Konfigurationsreise und Änderungsspur: Bedeutung, mögliche Auswirkungen und passende Liveprüfung. Nachweise verbinden Vorher-/Nachher-Werte mit datierten Sicherungen, gezielt lesbaren Originaldateien und fachlicher Dokumentation. Geschützte, mehrdeutige und ungeordnete Werte erhalten keine Wirkungsbehauptung.

- Konfigurationsreise pro Node mit Zeitregler, Schrittsteuerung, Bereichs-Cards und damaligen Einstellungswerten aus genau den ausgewählten Sicherungen. Direkter Einstieg in historische Detailvergleiche und Änderungsspuren. Nachweislücken, unklare Zeitfolgen und geschützte Werte bleiben erkennbar; überholte Leseanfragen werden abgebrochen.

- Neue Änderungsspur pro Node: einzelne Einstellungen über mehrere Sicherungen verfolgen, identische Werte bündeln, datierte Wertwechsel und Nachweise anzeigen. Einstieg auf der Hostseite und direkt aus unterstützten Dateivergleichen; vier Gruppen pro Ansicht und begrenztes Nachladen älterer Stände. Prüfsummenfehler, geschützte Inhalte, widersprüchliche Deklarationen und unklare Zeitfolgen bleiben sichtbar.

- Änderungsdetails mit bis zu sechs direkt auswählbaren Erklärungskarten und einer Gegenüberstellung geänderter Textstellen. Vollständiger Zeilenkontext und Quellen-IDs bleiben aufklappbar; sichtbare Filterchips ersetzen das Dropdown. Die Seitenauswahl entfällt bei kurzen Ergebnissen.

- Änderungsverlauf mit eigener Datumsspalte, dezenter Zeitlinie, Kennzeichnung des neuesten Stands und kompakteren Cards. Nullzähler entfallen; Erklärungen und Quellen bleiben aufklappbar.
- Sicherungsvergleich mit kompakteren Bereichsfiltern, klar getrennten Dateilisten und Dateidetails sowie Suche direkt in der Dateiliste. Dateinamen sind hervorgehoben, vollständige Pfade bleiben sichtbar. Wiederherstellungsaktionen stehen in Detailansichten in einem eigenen aufklappbaren Bereich.

- Änderungszeitpunkte zeigen bis zu drei konkrete Kurzbeschreibungen der betroffenen Konfigurationen (z. B. Storage-Anbindungen, VM-Konfiguration, Systemparameter). Gleichartige Hinweise sind gebündelt; Metadaten und reine Zeitangaben bleiben getrennt. Geschützte Dateinamen werden in den Kurzbeschreibungen nicht genannt.
- Eigenständige breite Änderungsdetailseite pro Node mit Kurzüberblick, Dateiliste und Zeilenvergleich. Quellenwechsel sind aufklappbar; Einstieg aus Zeitstrahl, Zusammenfassung und Host-Sicherungsverlauf. Der Rückweg erhält die vorherige Verlaufsseite, aufgeklappte Cards und den Tastaturfokus.

- Sicherungs- und Prüfaufträge wiederholen nur erkennbare vorübergehende Verbindungsfehler, mit begrenztem exponentiellem Abstand im selben Auftrag. Versuchslimit, nächster Versuch und letzte Störung sind sichtbar; bekannte Identitäts-, Berechtigungs- und Inhaltsfehler werden nicht automatisch wiederholt. Wiederherstellungen bleiben ohne automatische Wiederholung.
- Wiederholte Wiederholungsanfragen zu einem abgeschlossenen Auftrag liefern denselben Folgeauftrag. Bei bereits veröffentlichtem Ergebnis erfolgt kein weiterer automatischer Sicherungsversuch. Neustart und geordnetes Dienstende hinterlassen klare Meldungen ohne automatisches Wiederanlaufen unbestätigter Arbeit.
- Hostnamen erhalten eine lesbare Anzeige (z. B. „PVE · Berlin 01“); technische Namen und Verbindungsziele bleiben erhalten. Node-Vergleiche bieten Bereichs-Cards, Vorher-/Nachher-Cards und gezielte Bereichsfilter.
- Hostbezogener Änderungszeitstrahl mit vier Zeitpunkten pro Seite, bei Bedarf geladenen Metadaten und direktem Detailvergleich des ausgewählten historischen Ständepaars. Unvollständige Stände und Ladefehler ergeben keine Entwarnung.

- Startseite bündelt Handlungsbedarf in höchstens drei ruhigen Cards; betroffene Hosts erscheinen aufklappbar, durchsuchbar und mit maximal vier Einträgen pro Seite.
- Hostübersicht fasst Änderungen der zwei neuesten geeigneten vollständigen Stände nach Netzwerk, Storage, System und weiterer Konfiguration zusammen. Hinzugefügte, entfernte und geänderte Dateien sowie reine Zeitstempeländerungen werden getrennt gezählt; Detailvergleich übernimmt genau dieses Quellenpaar. Ladefehler ergeben keine Entwarnung.

- Funktionsausbau für den täglichen Sicherungsbetrieb: Hostprobleme mit Begründung und passender nächster Aktion; lokale Sperre gegen doppelte Startanfragen während der Serverantwort.
- Wiederherstellung schlägt den neuesten vollständigen Stand ohne bekannten Integritätsfehler und mit plausiblem Zeitstempel vor. Ausgeschlossene jüngere Stände werden erklärt; ohne geeigneten Stand bleibt der Direkteinstieg gesperrt. Die Prüfung des tatsächlichen Inhalts und Ziels im Plan bleibt erforderlich.
- Integritätsfehler und unplausible Sicherungszeiten ergeben keinen grünen Hoststatus mehr. Ungültige Datumswerte bleiben in Oberfläche und Verlauf darstellbar.
- Überfälligkeitsmeldungen ignorieren bekannte Integritätsfehler und unplausible Quellzeiten bei der Aktualitätsbewertung. Nicht lesbare Vorfallzustände werden gemeldet statt stillschweigend als neuer Vorfall behandelt.

- Allgemeine Button-Hoverregel so begrenzt, dass individuelle Cards und Bedienelemente ihre eigenen Hintergründe behalten. Lesbare Kennzahlen beim Überfahren, stärkere Kontraste in Navigation, Reitern, Zurück-Links und Dateilisten sowie weiße Beschriftung auf roten Löschbuttons. Browserregression prüft Hover und Tastaturfokus über Hauptansichten, Detailreiter, Menüs und Dialoge.

- Einheitliche Seitensymbole mit dunkler Kontur auf Weiß und farbig versetzter Ebene; aktive Navigation ohne grüne Füllung. Tabellen, aktive Aufträge, Dialoge und Leerzustände übernehmen die gemeinsamen Card-Konturen und Schatten.

- Ältere Stände werden erst beim Aufklappen ihres Monats und Tages gerendert; die Gruppierung folgt der eingestellten Zeitzone. Hinweise bleiben schon an geschlossenen Gruppen sichtbar. Hoststatus, Arbeitsbereiche und gemeinsame Oberflächen mit ruhigen Cards und farbig versetzten Schatten; weniger dekorative Symbole in Hostdetails.

- Hostarbeitsbereich mit getrenntem Sicherungs-, Zeitplan- und Verbindungsstatus, direktem Dateizugriff und eigenem Änderungsvergleich. Wiederherstellungen starten mit dem zeitlich neuesten Stand, auch bei unsortierter Statusantwort.
- Sicherungen standardmäßig nach Host gebündelt; gezielt aufklappbarer, durchsuchbarer Verlauf mit Monats- und Tagesgruppen sowie direkt sichtbarem neuestem Stand. Die vollständige standbezogene Archivansicht bleibt über „Alle Stände“ erreichbar.
- Übersicht mit kurzer Zustandseinordnung, priorisierten beschädigten/fehlgeschlagenen Hosts und einklappbarem Auftragsverlauf. Einheitliche Navigation und Seitenakzente in Salbei und Ocker mit dezenten Funktionssymbolen.

## 0.2.14

- Wiederherstellung mit kompakter Fünf-Schritte-Übersicht, beschriftetem Quellen-/Zielkontext und nächster Aktion. Noch nicht ermittelte Entscheidungen erscheinen nicht als null offene Punkte.
- Unvollständige Zielaufnahmen zeigen Hinweise statt einer uneingeschränkten grünen Prüfmarkierung. Gespeicherte Pläne bündeln Quelle, Ziel, Status, Dateiaktionen und nächste Aufgabe; Warnungen und gesonderte Bestätigungen bleiben erhalten.
- Planliste mit Trefferzahl und Filterrücksetzung; der Einstieg erklärt die Trennung zwischen Vorbereitung und tatsächlicher Übernahme.
- Einstellungen mit klaren Formulargruppen für Start, Ausführung, Aktualität sowie Webhook und SMTP. Kompaktere erreichbare Speicherleiste und sichtbare Hinweise auf geänderte Bereiche auch nach einem Reiterwechsel; Updates bleiben ein eigener Reiter.
- Sicherheitsabfragen, Schutz ungespeicherter Entwürfe und Update-/Zertifikatsfreigaben bleiben bestehen. Reale Proxmox-, Hardware-, Cluster-, Reboot- und Stromausfallabnahmen bleiben offen.

## 0.2.13

- Übersicht mit anklickbaren Kennzahlen und passenden Filtern, kompakterem Handlungsbedarf und kurzer Auftragsaktivität statt einer zweiten Verlaufstabelle.
- Hosts mit gezählten Schnellfiltern, relativen Sicherungszeiten samt exaktem Datum und einklappbaren Quellnachweisen im Detail.
- Sicherungen mit hervorgehobenem Dateizugriff, zugänglichem Downloadsymbol, gebündelten Vergleichsaktionen und Markierung des neuesten Stands pro Host über Filter und Seiten hinweg.
- Aufträge mit einheitlicher Erfolgsanzeige, direkten Ergebnislinks, korrekten Trefferzahlen und Status-Schnellfiltern. Restorefehler bleiben neben Planlinks sichtbar; der kompakte Verlauf bleibt auch schmal bedienbar.
- Größere Aktionsflächen und Menü-Tastaturbedienung mit deaktivierten Einträgen. Reale Proxmox-, Hardware-, Cluster-, Reboot- und Stromausfallabnahmen bleiben offen.
- Terminal-Integrationstest prüft den erhaltenen Wiederherstellungsentwurf nach tatsächlicher Neuzeichnung statt unveränderte Bildschirmzeilen erneut im Ausgabestrom zu erwarten. Kandidat v0.2.12 wurde nicht veröffentlicht.

## 0.2.11

- Eigener, direkt sichtbarer Updates-Reiter mit getrennten Bereichen für Versionsstatus, Aktionen und Updatequelle. Einstellungsreiter bleiben auf schmalen Ansichten sichtbar; ungespeicherte Änderungen und die Installationssperre bleiben erhalten.
- Übersicht zeigt betroffene Hosts und aktive Aufträge vor einklappbaren Quellnachweisen. Der Nachweisblock kennzeichnet seinen einzelnen Sicherungsstand und führt zu Sicherungen und Prüfungen.
- Hostliste zeigt den letzten Sicherungszeitpunkt auch auf schmalen Ansichten. Wiederherstellung erklärt den Gesamtumfang mit Schrittzähler und Vorschau auf alle fünf Schritte.
- Systemprüfung mit verständlichen Ergebnissen und konkreten nächsten Maßnahmen. Herunterladbarer Diagnosebericht mit ausschließlich freigegebenen Versions-, Betriebsmodus- und Speicherwerten; keine lokalen Pfade, Zugangsdaten, Hostadressen oder Aktivitätsprotokolle. Technische Details bleiben einsehbar.
- Reale Proxmox-, Hardware-, Cluster-, Reboot- und Stromausfallabnahmen bleiben offen; die Systemdiagnose behauptet keine geprüfte Wiederherstellbarkeit.

## 0.2.10

- Vier Recovery-Situationen in fünf Schritten mit frischem Zielinhalt, privaten Snapshots, expliziten Dateientscheidungen und Inhalts-/Metadatenvergleich in Web, CLI und Terminal.
- Nachvollziehbare Vorbereitung unterstützter Netzwerk-, Hostnamen- und Gast-Storagefelder; unbekannte Abhängigkeiten bleiben manuell. Keine neue automatische Gesamtrecovery.
- Verlorene Anfrage bei noch bereitem Schema-2-Plan sicher mit dem Hostjournal abgleichen: ein fehlendes Journal gibt dieselbe Plan-ID niemals erneut zur Übernahme frei.
- Sichere öffentliche Inventar-Abfragestände ohne Rohdiagnostik, verbesserte Listen/Formulare und korrigierte Sammelentscheidungen, Tabfokus und Hostnavigation. Alte Sicherungen/Pläne und Offlineexporte bleiben lesbar.
- Quellenvergleich, Ordnernavigation, lesende Maschinenprüfung, Integritäts- und Kopienprüfung sowie geschützte Sicherungsreferenzen und reviewte Zuordnungsrezepte.
- Verschlüsselte Cold-Pakete mit isoliertem Import, geprüfte Offline-Pakete und koordinierte Cluster-Quellsets für die manuelle Wiederherstellungsplanung. Historische Journale und separate Archivquellen bleiben an ihre ursprünglichen Nachweise gebunden.
- Einstellungen in Zeitplan, Aufbewahrung, Benachrichtigungen, Zugriff, Zertifikate und Betrieb gegliedert. Updates und Updatehinweise sind unter Betrieb erreichbar.
- Lokale Integration mit echtem Python-Helfer und ausführbares Recovery-Labkit ergänzt; reale Proxmox-/Hardware-/Cluster-/Rebootabnahme bleibt offen.

## 0.2.8

- Wiederherstellung mit eigener frischer Zielprüfung, nur benötigten physischen Ports und referenzierten Storage-IDs. Bridges, Bonds, VLANs und Interface-Aliase werden berücksichtigt; doppelte Portziele und unbekannte Referenzen blockieren.
- Dateiinhalte, Rechte und Eigentümeridentitäten vor der Übernahme prüfen. Synchronisiertes Hostjournal, lokale Helfersperre und kontrollierte Rücksetzung erhalten Fremdänderungen und ursprüngliche Fehler.
- Hostzustand nach Unterbrechung im Web oder Terminal abgleichen; Rücksetzung separat bestätigen. Ungültige oder verlorene Antworten erlauben keine blinde Wiederholung. Große Listen und Nachweise bleiben begrenzt.
- Netzwerk und Zugangskonfiguration bleiben manuell; automatische Netzwerkaktivierung und vollständige Host-/Cluster-Recovery werden weiterhin nicht angeboten. Speicher- und Identitätsentscheidungen stehen im Plan statt als ungenutzte Zuordnung.
- Vor automatischen Übernahmen bestehende Hosts einmal neu anbinden und neu sichern: Restore-Protokoll 2 und Eigentümeridentitäten sind erforderlich. Alte Sicherungen bleiben exportierbar.

## 0.2.7

- Dateidialog passt Liste und Vorschau an die verfügbare Bildschirmhöhe an. Download und Wiederherstellungsaktionen bleiben auch auf kleineren Laptop- und Mobilansichten direkt sichtbar.

## 0.2.6

- Ankersymbol im Browser-Tab und einheitliche kleine Wortmarke; der Tabtitel folgt der geöffneten Ansicht. Das Symbol bleibt auf hellen und dunklen Browserflächen erkennbar.
- Übersicht mit getrennten Kennzahlen für aktive Hosts, aktuelle Sicherungen, Handlungsbedarf und aktive Aufträge. Pausierte Hosts und der ausdrücklich deaktivierte Demozeitplan bleiben sichtbar.
- Dezente Akzente für Navigation, Register, Fokus und Dateiauswahl; kompakte Gruppenkennzeichnung und einheitliche Statuskapseln. Echte Fehler unterscheiden sich von Hinweisen.
- Klarere Abschnittsabstände, besser lesbare sekundäre Texte und kurze Dialogübergänge, die bei reduzierter Bewegung deaktiviert bleiben. Die bestehende ruhige Tabellenbasis bleibt erhalten.

## 0.2.5

- Wiederherstellung mit Quellhostfilter, durchsuchbarer Dateiauswahl und festen Seiten für Dateien, Pläne und Planschritte. Auswahlen bleiben beim Suchen und Blättern erhalten; neu geprüfte Zielports sind über „Zuordnungen anpassen“ direkt erreichbar.
- Quell-/Zielwechsel setzt Konsolen-/Isolationsbestätigungen und Netzwerkzuordnungen zurück. Planstatusfilter bleiben auch bei laufenden Statusänderungen sichtbar. Hostliste mit Seitenwechsel, Handlungsbedarfsfilter und übersichtlichen mobilen Filtern.
- Terminal bietet sämtliche Wiederherstellungsszenarien und direkte Dateipfadeingabe mit `Ctrl+E`. Vollständige Szenarien werden ausdrücklich als manuell geführt erklärt.
- Planexport enthält jetzt Originaldateien mit gesicherten Rechten, Eigentümern, Linkzielen und erweiterten Metadaten, auch aus archivierten Sicherungen. Vorbereitete Dateien, Integritäts- und Zugriffsprüfungen bleiben erhalten.
- README und Implementierungsübersicht unterscheiden umgesetzte Betriebsfunktionen von offenen Recovery-Regeln und realen Laborabnahmen.

## 0.2.4

- Die Weboberfläche lädt nach einem bestätigten Update automatisch neu, auch nach einem Seitenwechsel oder vorübergehendem Verbindungsabbruch. Fehlgeschlagene Updates erzeugen keine Reload-Schleife.
- Laufende und wartende Aufträge sind getrennt vom Verlauf sichtbar, mit Host, tatsächlicher Laufzeit und Versuch. Aufträge und Sicherungen haben Filter und feste Seiten statt wachsender Listen.
- Sicherungsdateien über Ordner, Breadcrumbs und eine Suche über alle Pfade durchsuchen; 25 oder 50 Einträge pro Seite. Vorschau, geschützte Inhalte, Download und Wiederherstellung bleiben verfügbar.
- Inventar mit kompakten Systemangaben, durchsuchbaren Netzwerk-/Datenträgertabellen und aufklappbaren Details. Zurückhaltende Statusfarben und einheitliche Listensteuerung.
- Aufbewahrung erklärt die Tages-, Wochen- und Monatsregeln je Host. Archivierung und Überfälligkeitsgrenze sind getrennt; automatische Archivierung lässt sich ausschalten.
- Neue Sicherungen erkennen die Proxmox-RNG-Quelle `/dev/urandom` und nachweislich fehlende optionale Vim-Includes korrekt. Die zentrale Prüfung berücksichtigt vorhandene Hosthelfer; echte fehlende Dateien bleiben Hinweise. Vorhandene Sicherungsmanifeste bleiben unverändert.

## 0.2.2

- Hosts direkt im Web oder Terminal mit Adresse, SSH-Benutzer und einmaligem Passwort anbinden. Nach Fingerprintbestätigung installiert Anker den Helfer und getrennte Sicherungs-/Wiederherstellungszugänge; gespeichert wird erst nach erfolgreichen Schlüsselprüfungen.
- Bestehende Profile und fremde autorisierte Schlüssel bei erneuter Einrichtung erhalten. Passwort bleibt flüchtig; bekannte Schlüsselwechsel, konkurrierende Hostaufträge und Updates werden abgefangen. Manuelle Einrichtung bleibt verfügbar.
- Terminal: `a` bindet automatisch an, `m` legt manuell an und `v` richtet den ausgewählten Host erneut ein. README und Handbuch erklären beide Wege.

## 0.2.0

- Updatequelle direkt im Web einrichten: mitgelieferte Anker-Quelle, Fingerprint und bestätigter Wechsel; bestehende Signatur-, Start- und Rückfallprüfung bleiben erhalten.
- Geführte Terminalverwaltung mit Formularen und bestätigten Aktionen für Hosts, Sicherungen, Wiederherstellung, Aufträge, Zeitplan, Benutzer und Systemfunktionen, einschließlich SSH-Fingerprintprüfung und Speichererweiterung. `anker` öffnet sie im Terminal; `anker -help` und die installierte Manpage erklären den Betrieb.

- TLS-Ordnerrechte unabhängig von der Installer-umask setzen und bei erneuter Einrichtung reparieren. Der Webdienst kann seine Zertifikate lesen; private Schlüssel bleiben vor anderen Benutzern geschützt.

- Installation direkt nach dem Clone: fehlende Werkzeuge automatisch installieren und Anker bauen. Dasselbe Skript erkennt bestehende Installationen und aktualisiert Hauptprogramm und Systemhelfer mit Sicherung, Startprüfung und Rückfall.

- Wiederholte Terminaleinrichtung mit „Fortsetzen“ oder „Neu konfigurieren“. Vorhandene Ports, Zertifikate, Benutzer und SSH-Schlüssel bleiben erhalten; unterbrochene Konfigurationsänderungen werden beim nächsten Einrichtungsaufruf aus einem geschützten Journal zurückgenommen.

- Terminaleinrichtung mit klaren Abschnitten, kompakter Konfigurationsübersicht und animierter Dienstprüfung; ruhige Ausgabe für einfache Terminals und Logs.

- Ersteinrichtung auf einem direkt eingebundenen Dateisystem funktioniert auch mit root-eigenem `lost+found`; dessen Rechte und Inhalte bleiben unverändert.

- Passwortvorgabe standardmäßig acht Zeichen, administrativ auf 8 bis 128 Zeichen einstellbar. Einheitliche Prüfung in Ersteinrichtung, Web und CLI; bestehende Zugänge bleiben bei Erhöhung nutzbar.

- Aufträge mit Detailansicht, tatsächlichem Start, manuellem/automatischem Auslöser und direktem Sicherungszugriff. Abbruch, erneuter Start von Sicherung/Hostprüfung und administratives Entfernen abgeschlossener Einträge über Web/CLI.
- Tagesmarker und Einplanung atomar speichern, defekte Katalogwerte melden, geänderte Parallelität für wartende Aufträge übernehmen und Hostlöschung gegen gleichzeitig gestartete Aufträge schützen. Schedulerfehler bleiben sichtbar; verspätete Statusantworten überschreiben keine neueren Zustände.

- Speicherbereich für Anker und seine Backup-Laufwerke: Belegung, verfügbarer Platz, Inodes, echter Verlauf und bedingte Kapazitätsprognose. Bestätigte Dateisystemerweiterung für bereits zugewiesenen ext4-/XFS-Platz; LXC-Anleitung und herunterladbare Hinweise zum Einbinden neuer Laufwerke. Keine Formatierung oder automatische Partition-/LVM-Änderung.

- Webzertifikate automatisch und ohne Dienstneustart erneuern; Ablauf, Fingerprint, öffentlicher Download und administrativer Vorlauf in Einstellungen/CLI. Importierte Zertifikate bleiben unangetastet; selbstsignierte Zertifikate können eine neue Browserfreigabe benötigen.

- Kürzere Ersteinrichtung: eigene Zugangsdaten vor dem Dienstwechsel, bestehende Administratoren erkannt, bekannte Updatequellen automatisch übernommen.
- HTTPS-Zertifikat für private DNS-Namen oder IPs direkt im Assistenten erzeugen oder eigene Zertifikate importieren. Gültigkeit und Adresse werden geprüft; eine gültige bestehende TLS-Identität bleibt erhalten.
- Hostinstallation übernimmt eingeschränkte öffentliche SSH-Schlüssel in einem Aufruf. `anker host trust` prüft unabhängig angegebene Fingerprints und pflegt `known_hosts`; Hostformular und CLI verwenden die erzeugten SSH-Pfade automatisch.

- Produktionsinstallation startet ohne Demodaten. Demo nur durch ausdrücklichen Befehl, mit dauerhaft getrenntem Datenordner, lokalem Webzugriff und eigenem Socket. Release-Pakete enthalten keine Testfixtures oder Designbeispiele.

- Anmeldung bleibt über Dienstneustarts und Updates erhalten. Sitzungen laufen nach 30 Tagen ab; die Dauer ist einstellbar.
- Benutzer lassen sich sperren, entsperren, bearbeiten und löschen. Passwort- und Rechteänderungen beenden bestehende Sitzungen. Der letzte aktive Administrator bleibt geschützt.
- Signierte GitHub-Releases für Linux amd64 und arm64. Updates sind über die Weboberfläche und `anker update` möglich.
- Der Updater prüft Signatur, Plattform, Dateigröße und SHA-256. Vor dem Austausch sichert er Programm und Katalog. Ein fehlgeschlagener Start löst einen Rückfall aus.
- Aktive Sicherungen, Wiederherstellungen und Aufbewahrungsprüfungen sperren die Updateinstallation.
- Geführte Servereinrichtung mit verdecktem Passwort, TLS-Prüfung, getrennten SSH-Schlüsseln und wiederholbarer Ersteinrichtung. Verifizierter Release-Installer, auch für private GitHub-Repositories.
- Die Wartungsfreigabe nach einem Update wartet auf den laufenden Dienst; ein noch nicht erreichbarer Socket beim Booten verliert den Wiederholungsmarker nicht.
- Linux/systemd-Prüfung für Erstinstallation, Dienstwechsel, fehlerhafte Programme und unterbrochene Updatejournale im GitHub-Workflow.

## 0.1.0

Erster Entwicklungsstand mit zentraler Sicherung von Proxmox-Hostkonfigurationen, lesbarer Ablage, Weboberfläche, Shelloberfläche, Datei-Downloads und geführter Wiederherstellungsplanung.
