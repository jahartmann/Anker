# Wiederherstellung und Oberfläche neu aufbauen

Stand: 9. Oktober 2026. Vom Benutzer am 8. Oktober 2026 freigegeben; lokale Recovery-/UI-Implementierung integriert. Abschlussnachweise und unabhängige Nachprüfung werden in RECOVERY_REVIEW.md und IMPLEMENTATION_STATUS.md geführt. Reale Proxmox-Abnahme bleibt offen.

## Ziel und Umfang

Der Benutzer möchte praktisch bedienbare Wiederherstellungsszenarien, nachvollziehbare Zuordnungen, einen direkten SSH-Abgleich mit dem neuen Server und eine bessere Oberfläche in allen Ansichten, besonders beim Sicherungsvergleich. Fehler sollen durch überprüfte Voraussetzungen, eindeutige Entscheidungen und nachvollziehbare Rücksetzung vermieden werden.

Annahme: Anker bleibt ein Werkzeug für Proxmox-Hostkonfigurationen. Gastdatenträger kommen aus vorhandenen Storages oder PBS. Der bestehende feste SSH-Helfer, die getrennten Sicherungs-/Wiederherstellungsrollen, unveränderliche Sicherungen und das Hostjournal bleiben die Grundlage. Ein pauschales Kopieren von `/etc` ist kein unterstützter Ablauf.

Der Umbau ist architektonisch: SSH-Protokoll, Planmodell, Dateivorbereitung, Export, Web, CLI und Terminal müssen dieselben Entscheidungen ausdrücken. Die UI-Arbeit gehört zum selben Benutzerablauf; allgemeine Ansichten erhalten gemeinsame Layout- und Interaktionsverbesserungen.

Erfolg bedeutet: Der Benutzer sieht, was am Ziel vorhanden ist, welche Werte angepasst werden, was übernommen wird und was erhalten bleibt. Jede offene Voraussetzung nennt die nächste konkrete Handlung. Dateien, Dienstprüfung und Neustart haben getrennte Nachweise. Eine ausgefallene Verbindung kann keinen falschen Erfolg oder zweiten unkontrollierten Schreibversuch erzeugen.

## Befunde am bestehenden Stand

Untersucht: Commit `a649389`, darunter `recovery_mapping.go`, `plan.go`, `restore.go`, `ssh.go`, `host/anker_host.py`, Webansichten und vorhandene Tests. Keine echten Hosts kontaktiert.

1. `Step.Diff` vergleicht Sicherungsinhalt mit vorbereitetem Inhalt. Der tatsächliche Zielinhalt ist nicht enthalten. `Probe` liefert Zielprüfsummen und Metadaten, jedoch keine Dateiinhalte.
2. Portnamen werden gezielt ersetzt. Storage, Hostname und Adresse werden lediglich in manuelle Hinweise geschrieben. Ein gewählter Zielspeicher verändert keine Gastreferenz.
3. Gesamtszenarien wählen standardmäßig alle gesicherten Dateien. Das ist für einen Ersatznode im lebenden Cluster ungeeignet: gemeinsame und andere nodespezifische Konfiguration müssen ausdrücklich erhalten bzw. ausgeschlossen werden.
4. Zielstorage wird überwiegend aus `pvesm status` abgeleitet. Der Helfer liefert `storage_config` derzeit nicht als eigenen geprüften Inventarwert. Gleiche IDs belegen keine gleichen Pools, Pfade oder vorhandenen Volumes.
5. Ein Major-Versionsunterschied blockiert sämtliche automatische Übernahme. Die UI muss trotzdem einen brauchbaren manuellen Ablauf mit erhaltenem Zielsystem und einzelnen Kompatibilitätsentscheidungen liefern.
6. Der Sicherungsvergleich lädt sämtliche Diffs gleichzeitig. Suche, Änderungsfilter, Seiteneinteilung und eine eindeutige Anzeige der beiden Stände fehlen. Der gemeinsame Diff-Formatter nennt diese Seiten fälschlich „Original“ und „Vorbereitet“.
7. `lineDiff` markiert alle Zeilen als entfernt und hinzugefügt. Ein kleiner Unterschied in einer langen Datei ist damit schwer prüfbar.
8. Der Vergleich ignoriert Lesefehler beim Laden von Inhalten. Ein fehlgeschlagener Read muss sichtbar von einer tatsächlich fehlenden Datei unterschieden werden.
9. Die Wiederherstellung vereint Auswahl, Prüfung, Zuordnungen und lange Hinweislisten in einem scrollenden Dialog. Nach der Zielprüfung sind Fortsetzen und offene Entscheidungen außerhalb des ersten sichtbaren Bereichs.

## Empfohlener Aufbau und Alternativen

Empfohlen: bestehende Sicherungs- und Journalmechanismen behalten, aber davor einen neuen Szenario-, Abgleich- und Entscheidungsablauf bauen. Der Helfer erhält eine begrenzte lesende Dateioperation. Daraus entstehen ein versiegelter Zielstand und ein Plan mit expliziten Dateiaktionen.

Alternative 1: vorhandenen Dialog und Texte ergänzen. Das beseitigt weder den fehlenden Zielvergleich noch die unscharfen Szenarien und wird verworfen.

Alternative 2: sofort vollständige automatische Host-, Netzwerk- und Cluster-Recovery bauen. Dafür fehlen ein unabhängig geprüfter Netzwerkrückweg und reale Proxmox-Abnahmen. Die Architektur muss eine spätere Erweiterung zulassen; in dieser Umsetzung bleiben diese Aktionen geführt und manuell.

## Praktischer Benutzerablauf

### 1. Sicherung und Situation wählen

Eine eigene Arbeitsansicht ersetzt den langen Vorbereitungsdialog. Die Kopfzeile zeigt Schritt, Quelle, Ziel und offene Entscheidungen. Vorherige Schritte bleiben erreichbar, ohne bereits getroffene Entscheidungen zu verlieren, solange ihre Voraussetzungen unverändert sind.

Vier verständliche Situationen: „Einzelne Dateien“, „Host neu aufbauen“, „Node im gesunden Cluster ersetzen“, „Gesamten Cluster wiederherstellen“. Hardware-, Versions- und Topologieänderung sind zusätzliche Eigenschaften des Ziels, keine miteinander konkurrierenden Situationen. Bestehende Szenario-IDs bleiben für alte Pläne lesbar und werden auf diese Darstellung abgebildet.

Sicherungen zeigen Host, Zeitpunkt, Version, Integritätsstatus und Pflichtlücken. Vor einer belastbaren Vorschau wird die Integrität geprüft. Bei fehlenden Pflichtdaten bleibt ein Originalexport verfügbar, automatische Übernahme gesperrt.

### 2. Ziel verbinden und prüfen

Vorhandenen Host auswählen oder über die vorhandene Hostanbindung einen neuen Zielserver hinzufügen. Neue Anbindung führt zu SSH-Adresse, Port, unabhängig geprüftem Hostschlüssel und dem bestehenden festen Hosthelfer. Keine neue frei ausführbare Remote-Shell und keine Speicherung eines Rootpassworts in einem Plan.

Die Zielkarte zeigt tatsächlichen Hostnamen, SSH-Endpunkt, Hostschlüsselbezug, Proxmox-/Debian-Version, Bootmodus, Clusterzugehörigkeit und Zeit der Prüfung. Quellidentität und Verbindung zum Ziel sind getrennte Werte. Eine künftig gewünschte Management-IP ersetzt nicht die aktuell funktionierende SSH-Verbindung während der Vorbereitung.

Lesende Prüfung erfasst benötigte Ports, Adressen/Routen, Storagekonfiguration und -status, vorhandene Gäste, UID/GID-Namen und Inhalte/Metadaten der betroffenen Dateien. Zustand jeder Abfrage: geprüft, nicht anwendbar, fehlgeschlagen oder unbekannt. Fehlende Daten werden nicht als fehlende Ausstattung ausgegeben.

### 3. Unterschiede und Zuordnungen klären

Bereiche: Netzwerk, Speicher/Gäste, Hostidentität, Dateien. Jeder zeigt offene Entscheidungen und Quell-/Zielbelege. Ohne benötigte Referenzen entfällt der Bereich.

Pro Datei ist genau eine Aktion hinterlegt: „Sicherung übernehmen“, „Angepasste Datei verwenden“, „Ziel behalten“ oder „Manuell bearbeiten“. Geschützte Systemteile bekommen eine feste Vorgabe, die nicht durch einen allgemeinen Übernehmen-Schalter aufgehoben wird. Identische Dateien werden nicht erneut geschrieben.

Die Inhaltsansicht zeigt wahlweise **Sicherung → vorbereitet**, **aktuelles Ziel → vorbereitet** oder **Sicherung → aktuelles Ziel**. Metadatenänderungen stehen separat. Nicht lesbar, Binärinhalt, ausgeblendetes Geheimnis und nicht vorhandene Datei haben unterschiedliche Zustände.

### 4. Übernahme prüfen

Die Zusammenfassung zeigt Zielidentität, Sicherung, tatsächliche Dateianzahl, erhaltene Zielbestandteile, manuelle Aufgaben, Rückfallpfad und Prüfungen. Jede Zuordnung nennt, welche Datei bzw. Referenz sie verändert. Ein Wunsch ohne ausführbare Regel wird als manuelle Aufgabe ausgewiesen.

Erneute Zielprüfung prüft die relevanten Vorbedingungen. Drift verwirft die Freigabe und fordert einen neuen Vergleich. Freigegebene Einzeldateiaktionen nutzen weiterhin die exakte Plan-ID, getrennte Schreibberechtigung, synchronisiertes Staging und das Hostjournal. Netzwerk, Cluster und Storageverwaltung haben eigene manuelle Schritte.

### 5. Betrieb bestätigen

Ergebnis unterscheidet „Dateien übernommen“, „Dienstprüfung ausstehend“, „Erreichbarkeit geprüft“ und „Neustart geprüft“. Durch Benutzer bestätigte Schritte werden als solche gekennzeichnet und ersetzen keinen maschinellen Nachweis. Einen vollständigen Wiederaufbau darf erst die erforderliche Betriebsprüfung abschließen.

Bei SSH-Abbruch ist der nächste Schritt „Hostzustand abgleichen“. Eine erneut gesendete Übernahme ist gesperrt. Rücksetzung bleibt eine eigene Aktion und erhält fremde Änderungen. Nach manuellen Netzwerkänderungen zuerst den neuen Endpunkt und dessen geprüfte Hostidentität verbinden, dann vergleichen.

## Szenarioregeln

| Situation | Übernehmen / vorbereiten | Zielstand erhalten | Erforderlicher Abschluss |
|---|---|---|---|
| Einzeldateien | explizit gewählte gewöhnliche Dateien; geschützte Dateien nur manuell | ungewählte Dateien, Dienste und andere Schreiber koordinieren | Inhalt/Metadaten und betroffene Dienste prüfen |
| Frischer Standalone-Host | ausgewählte lokale Funktionen, geprüftes Netzwerk, gezielte Proxmox-/Gastdefinitionen | Bootloader, Root-FS, maschinenspezifische Identität, funktionierender SSH-Zugang | Storage/PBS, Dienste, Netz, Gäste und Neustart prüfen |
| Andere Hardware | Standalone-Ablauf plus Port-, PCI-, Mount- und Geräteentscheidungen | Zieltreiber, Boot, neue Geräteidentitäten | reale Verkabelung, Datenträgerbestand und Passthrough prüfen |
| Ersatznode im gesunden Cluster | ausschließlich relevante lokale Nodekonfiguration | aktuelles `/etc/pve`, Corosync, HA, gemeinsame Storageconfig, andere Nodes | alten Node entfernen, Ziel beitreten, Mitgliedschaft/Quorum/Storage prüfen |
| Gesamter Clusterverlust | isolierter gemeinsamer Ausgangsstand plus explizite lokale Nodepläne | keine unkontrollierte Mischung von nodebezogenen Sicherungen | Clusterzustand, Quorum, HA, Ceph, Gäste und isolierten Wiederanlauf prüfen |
| Versions-/Topologiewechsel | kompatible, einzeln geprüfte Funktionskonfiguration | Paketquellen und versionsabhängige Zielkonfiguration | unterstützten Versionspfad bzw. neue Clusterstruktur separat abnehmen |

Cluster-Gesamtverlust bleibt ein manueller Ablauf. Es wird keine koordinierte Multi-Node-Ausführung suggeriert, solange Clusterkatalog, gemeinsame Sicherungsstände und reale Abnahme fehlen.

## Verbindliche Zuordnungsregeln

| Bestandteil | Zuordnung und Nachweis | Wirkung |
|---|---|---|
| Physische NIC | Funktion aus Bridge/Bond/VLAN, MAC, PCI, Zielport und Verkabelungsbestätigung | nur bekannte Interface-Tokens ersetzen; Zielport eindeutig |
| Bridge/Bond/VLAN | Namen und Abhängigkeiten beibehalten; Kollision mit bestehender Zieltopologie prüfen | Parser behandelt VLAN-Eltern, Bond-Primary, MTU und referenzierte Ports; unbekannte Hooks bleiben manuell |
| Managementnetz | vorhandene Zieladresse, gewünschte Adresse/CIDR, Gateway, IPv4/IPv6 und zugehöriges Interface | nur ausdrücklich ausgewählte Stanzas vorbereiten; keine globale IP-Ersetzung und keine SSH-Aktivierung |
| Hostname | vorhandener Zielname, gewünschter Name, DNS und `/etc/hosts`; Szenarioregel | lokale Identitätsdateien gezielt vorbereiten; keine globale Pfad-/Textersetzung; Clusteridentität bleibt manuell |
| Storage-ID | Typ, Server/Export, Pfad, ZFS-Pool/LVM-VG, Nodes, Content und verfügbaren Bestand vergleichen | gleiche ID ist nur Kandidat; keine Formatierung, Anlage, Umbenennung oder Datenverschiebung |
| Gastvolumes | Storage-ID plus konkrete Volume-ID und Gast-ID | Parser ersetzt nur unterstützte Storage-Präfixe; ohne bestätigtes vorhandenes bzw. separat zurückgespieltes Volume manuelle Aufgabe |
| Mount / UUID | UUID, WWN/Serial, FSType, Mountpoint und systemrelevante Rolle | Quell-Root/Boot/Swap ausschließen; zusätzliche Mounts nur belegt vorbereiten; `/dev/sdX` nie als Identitätsbeweis |
| UID/GID | Benutzer-/Gruppenname und numerische Zielidentität | vorhandene abgesicherte Einzeldateiregel beibehalten; abweichende Konten manuell, keine automatische Accountanlage |
| Secrets | gesicherter Verwendungszweck und passende Zielverbindung | gesonderte Inhaltsberechtigung; benötigte PBS-/Storagecredentials gezielt; neue Zertifikate und SSH-Hostkeys grundsätzlich erhalten |
| PCI/Passthrough | Hersteller/Gerät, Funktion, IOMMU-Gruppe und Zielverfügbarkeit | nicht aus gleichem PCI-Slot auf gleiche Hardware schließen; manuelle Anpassung und Funktionsprüfung |
| Gastdefinitionen | ausschließlich relevanter Quellnode, Gast-ID, MAC, Bridge, Volumes, Hooks und Zielbestand | Kollisionen/fehlende Abhängigkeiten blockieren Übernahme; Config allein belegt keine Gastdaten |

Netzwerk-, Storage- und Identitätsvorbereitung bleibt exportierbar. Ausführbare Anpassungsregeln sind an unterstützte Syntax gebunden; unerkannte Fälle werden mit Dateipfad und nächster Handlung ausgewiesen.

## SSH-Abgleich und Datenfluss

1. `InspectRecovery` prüft Quelle und Szenario und inventarisiert das identifizierte Ziel über den bisherigen lesenden SSH-Zugang.
2. Eine neue feste Helferoperation liest nur explizit angeforderte, im Hostprofil erlaubte Dateien. Relative Pfade, Symlink-Eltern/-Ziele, besondere Dateien, Dateianzahl und Gesamtgröße werden begrenzt. Es gibt keine vom Benutzer gelieferte Shellanweisung.
3. Jede Antwort enthält Status, Inhalt, Hash und Metadaten. Vorher-/Nachherprüfung erkennt Änderungen während des Lesens. Der Inhalt muss zum zuvor erfassten Zielhash passen; andernfalls Prüfung wiederholen und Freigabe verwerfen.
4. Der Zielstand erhält eine Referenz und einen Fingerprint; private Inhalte liegen in geschützten Dateien, nicht in Listenendpunkten, Logs oder ungeschütztem `plan.json`. API-Ausgabe redigiert sowohl Quell- als auch Zielgeheimnisse.
5. Szenarioregeln und bestätigte Zuordnungen erzeugen vorbereitete Dateien und explizite Aktionen. Vorschau wird aus diesem Stand erstellt und verweist auf dessen Zeitpunkt.
6. Vor dem Schreiben werden bekannte Zielidentität, relevante Inventardaten und alle betroffenen Datei-/Metadatenvorbedingungen erneut geprüft. Die bestehenden Helferprüfungen werden nicht durch den sichtbaren Diff ersetzt.
7. Der bisherige Journalablauf bleibt für freigegebene Einzeldateien zuständig. Neue Gesamtautomatik wird nicht über eine Demo-Ausnahme freigegeben.

Ältere Helfer ohne Inhaltsabgleich zeigen „Inhaltsvergleich nicht verfügbar – Hostverbindung aktualisieren“. Keine erfundene leere Zieldatei und keine Ausführung auf Grundlage eines unvollständigen Vergleichs. Alte Pläne bleiben lesbar und exportierbar; eine neue Freigabe erfordert einen neu erstellten Plan.

## Oberfläche in allen Ansichten

Gemeinsam: gleiche Seitenkopfstruktur, Ergebniszahl, Suche/Filter, sichtbare Hauptaktion, klare Lade-/Fehler-/Leerzustände, Tastaturfokus, verständliche Statusbezeichnungen und konsistente Seitensteuerung. Fehler gehören direkt an den betroffenen Bereich. Bei kleinen Fenstern umbrechende Aktionen und aufklappbare Details statt abgeschnittener Bedienelemente. Lange Aufgaben erhalten eine feste Schrittleiste und einen sichtbaren Fußbereich.

| Ansicht | Konkrete Verbesserung |
|---|---|
| Übersicht | offene Aufgaben nach Dringlichkeit mit direktem Weg zum betroffenen Host/Plan; Status mit prüfbarem Zeitpunkt |
| Hosts | Verbindung, letzte Prüfung und Sicherung klar trennen; neue Zielanbindung direkt aus Recovery erreichbar; kompakte Inventarkarten mit Details |
| Sicherungen | Vergleich aus zwei gewählten Tabellenständen starten; Quelle/Ziel inkl. Host/Zeit sichtbar; hostübergreifende Auswahl bewusst kennzeichnen; Vergleichsrichtung umkehrbar |
| Vergleich | Dateiliste mit Suche, Änderungs-/Inhalts-/Metadatenfilter, Zählern und Seiten; rechts einzelne Vorschau; Zeilennummern und Kontextdiff; mobile Ansicht untereinander |
| Dateibrowser | Ordner/Pfadnavigation und Auswahl erhalten; Inhalt, Rechte und Secrets getrennt sichtbar; Datei gezielt in Recovery übernehmen |
| Wiederherstellung | eigene fünfstufige Arbeitsansicht; offene Entscheidungen zählen; Bereichsweise Quelle/Ziel und Auswirkungen; erhaltene Bestandteile explizit sichtbar |
| Aufträge | Ergebnis und nächste Handlung hervorheben; unterbrochene Recovery führt zum Journalabgleich; technische Details aufklappbar |
| Einstellungen | überschaubare Abschnitte und sichtbarer Speicherstatus; Speichern erreichbar; Formularfehler lokal; Warnung bei ungespeicherten Änderungen erhalten |
| Speicher | Größe/Belegung/Trend und verfügbare Aktion getrennt; Container-/VM-Handlungen konkret erklären; unbekannte Messwerte kennzeichnen |
| Zugriff | Rolle und Geheimnisrechte verständlich vergleichen; Benutzer-/Key-Aktionen mit klarer Zielidentität |
| Updates / Zertifikate | aktueller Zustand und nächste Aktion zuerst; technische Details und Nachweise aufklappbar; laufende Operationen klar gesperrt |
| Terminal / CLI | dieselben Szenario- und Zuordnungsregeln, Zielvergleich und erhaltene Bestandteile; Webdarstellung darf keine abweichende Freigabe implementieren |

Vergleichsdaten werden zunächst als kompakte Zusammenfassung geladen. Einzeldateiinhalte folgen erst beim Öffnen. Lesefehler werden nicht als „entfernt“, „identisch“ oder „Metadaten geändert“ ausgegeben. Pure MTime-Änderungen werden als Zeitstempel gekennzeichnet und sind separat filterbar.

## Architekturgrenzen und Kompatibilität

Getrennte Einheiten: Szenarioauswahl/-regeln, Zielaufnahme, Referenzanalyse, Zuordnungsprüfung, Vorbereitung, Vergleich und Ausführung. Die Aufteilung verhindert zusätzliche Logik in den bereits großen Restore- und Backups-Komponenten.

Plan ergänzt explizite Aktionen, erhaltene Bestandteile, Zielstandreferenz, Vergleichszustände und geordnete Aufgaben mit Voraussetzungen. Bestehende `apply/manual`, Szenario-IDs und alte gespeicherte Pläne werden kompatibel gelesen. Parser und Entscheidungsgates liegen zentral im Go-Dienst; Python prüft Remote-Dateioperationen, Rechte und Journalzustände; React, CLI und TUI zeigen diese Ergebnisse.

Planexport enthält Originalmanifest, Zuordnungen mit Wirkungsnachweisen, vorbereitete Dateien, Zielnachweis und konkrete Schritte. Zielkopien mit Secrets sind genauso geschützt wie vorhandene Downloads. Unverändert bleiben der Originalexport und der manuelle Recoveryweg ohne Anker.

## Nachweise und Abnahme

Vor Produktivfreigabe jeder unterstützten Regel: zuerst Regression für das gewünschte Verhalten, dann Umsetzung und Prüfung. Relevante Fälle:

- Zielinhalt fehlt, ist identisch/geändert, binär, zu groß, nicht lesbar oder während Aufnahme/Übernahme verändert.
- Metadatenabweichung, ACL/xattrs, unbekannte Eigentümer, Symlink, Pfadausbruch und nicht erlaubtes Hostprofilziel.
- NICs mit gleichen Namen, anderen MACs/PCI, Bonds/VLANs, `bond-primary`, Include-Dateien, mehrfachen Zielen und fremden Hooks.
- Storage-ID gleich bei anderer VG/Pool/Export, Gast-ID-Kollisionen, fehlende Volumes, ISO/CDROM, Hooks und PBScredentials.
- Quellroot/Boot/Swap erhalten; fstab-Gerätenamen ohne stabile Identität nicht übernehmen.
- Gesunder Clusterersatz schließt alte gemeinsame Konfiguration und fremde Nodes aus; Disaster bleibt isoliert/manuell.
- IPv4/IPv6, CIDR, Gateway, DNS und Hostname ändern nur unterstützte betroffene Werte; aktive SSH-Verbindung bleibt erhalten.
- Vorab-Drift, Schreibfehler nach erster Datei, Abbruch vor/nach Hostantwort, Neustart, volles Dateisystem und Rücksetzungskonflikt.
- Alte Helfer/Pläne, Rollenrechte, geheime Quell- und Zielinhalte sowie lesbare Exporte ohne laufenden Dienst.
- Browser: vier Grundsituationen, Navigieren zurück, veraltete Antworten, doppelte Klicks, Filter/Seiten, große Dateilisten, Tastatur und 1440-/390-Pixel-Ansichten aller Views.

Reale Abnahme bleibt erforderlich für Proxmox-Standalone, Hardwarewechsel, Clusterersatz, isolierten Clusterverlust, Netzwerkverlust, Reboot und Datenträger-/PBSzustand. Lokale Tests können diese Nachweise nicht ersetzen. Bis dahin bleiben unbewiesene Schreib-/Aktivierungswege gesperrt.

Aktuelle Baseline dieser Untersuchung: vollständige Go-Suite mit Race Detector bestanden; 53 Python-Helfertests bestanden; Webbuild bestanden; 13 gezielte Browsertests für Sicherungen und Recoveryprüfung bestanden. Browserprüfung erfolgte mit vorhandenem Playwright/Chromium, da kein Browser-Skill installiert ist. Screenshotaufnahme der Hauptviews, Einstellungsbereiche, Vergleich und Recoveryprüfung in lokaler Demo; keine Browser-JavaScriptfehler während der Aufnahme. Vergleich und Recovery wurden bei 1440 × 1000 sowie 390 × 844 Pixel aufgenommen. Die Systemaufnahme zeigt noch einen Ladezustand und ist kein Nachweis einer abgeschlossenen Systemprüfung. Diese Baseline prüft bestehenden Code und belegt keine Umsetzung dieses Entwurfs.

## Offizielle Grundlagen

- [Proxmox Clusterverwaltung](https://github.com/proxmox/pve-docs/blob/master/pvecm.adoc): beim Clusterbeitritt wird `/etc/pve` überschrieben; bestehende Gäste und IDs erfordern einen gesonderten Umgang.
- [Proxmox Netzwerkkonfiguration](https://pve.proxmox.com/wiki/Network_Configuration): Bridges, Bonds, VLANs und Gerätebenennung haben konkrete Topologie- und Versionsabhängigkeiten.
- [Proxmox Konfigurationsdateien](https://github.com/proxmox/pve-docs/blob/master/configuration-files.adoc): viele Konfigurationen liegen im gemeinsamen Clusterdateisystem, lokale Dateien erfordern getrennte Behandlung.

## Entscheidung vor Umsetzung

Empfehlung zur Freigabe: den neuen Ablauf samt direktem SSH-Inhaltsabgleich, Szenarioregeln, expliziten Dateiaktionen, nachvollziehbaren Zuordnungen und gemeinsamen UI-Verbesserungen umsetzen. Vollständige automatische Netzwerk-, Storage- und Cluster-Recovery erst nach gesonderter realer Abnahme freigeben.
