# Wiederherstellung

## Fünf Schritte

1. **Quelle und Situation:** Sicherung und `files` (Einzeldateien), `host` (Host/Hardware), `cluster-node` (Ersatznode) oder `cluster-disaster` (isolierter Clusterverlust) wählen. Die Auswahl nennt erhaltene Zielbestandteile; Root, Boot, Swap und fremde/shared Clusterteile werden nicht pauschal ersetzt.
2. **Ziel vergleichen:** Ziel anbinden und **Ziel prüfen**. Der feste lesende Helfer erfasst ausgewählte Dateien und nötige Abhängigkeiten mit Inhalt und Metadaten in einer privaten, zeitlich begrenzten Aufnahme. Fehlend, nicht lesbar, verändert, zu groß und unbekannt sind verschiedene Zustände. Ohne vollständigen Nachweis keine automatische Übernahme. Die Prüfung legt noch keinen Plan an.
3. **Entscheidungen:** Für jede Datei Original aus Sicherung, vorbereitete Fassung, Ziel behalten oder manuelle Bearbeitung wählen. Offene Entscheidungen bleiben offen; die Sammelaktion setzt ausschließlich offene Dateien auf manuell. Benötigte Ports und Storage-IDs zuordnen. Vorschläge prüfen; gleicher Name oder gleiche ID belegt keine Identität.
4. **Plan prüfen:** Quelle → vorbereitet, Ziel → vorbereitet und Quelle → Ziel vergleichen. Inhalte, Metadaten, Änderungszeit, fehlende Seiten und geschützte Inhalte bleiben getrennt. Jede tatsächliche Feldänderung nennt Vorher/Nachher und Grund. Der Plan kopiert die geprüfte Zielaufnahme privat und bleibt nach deren Ablauf exportierbar. Vor Übernahme prüft Anker Integrität, Zielidentität, aktive Dateien und Abhängigkeiten erneut; Drift verlangt erneute Vorbereitung. Änderungen an behaltenen/unbeteiligten Dateien erzeugen keinen Schreibbedarf.
5. **Übernehmen und prüfen:** Nur freigegebene gewöhnliche Einzeldateien nach exakter Plan-ID übernehmen. Identischer Inhalt und wirksame Metadaten ergeben kein erneutes Schreiben; reine MTime-Abweichung ist Vergleichsinformation. Journalabgleich, Rücksetzung, Dienst-/Erreichbarkeits-/Neustartprüfungen sind eigene Schritte. Benutzerbestätigungen tragen Benutzer und Zeitpunkt und ersetzen keinen maschinellen Nachweis.

Web, CLI und Terminal verwenden dieselben Entscheidungen, Snapshots, Plan- und Journalregeln. Der Webvergleich bietet Suche und Seiteneinteilung. Im Terminal öffnet `c` auf einem gespeicherten Plan den einzelnen Dateivergleich mit wählbarer Vergleichsbasis; `Esc` führt zurück. `Ctrl+R` erneuert im Vorbereitungsformular die Zielprüfung, `Ctrl+E` wechselt Dateiliste und direkte Pfadeingabe. Einen paginierten Sicherungsvergleich bietet das Terminal derzeit nicht. Lokale Socketverwaltung verwendet die administrativen Rechte des Socketzugangs.

## Zuordnungen und Grenzen

Portzuordnungen bearbeiten unterstützte Netzwerktokens einschließlich Bridge-/Bond-/VLAN-Abhängigkeiten. Statische Adresse/Präfix, Gateway und DNS werden nur in eindeutig unterstützten Quellstanzas vorbereitet; fehlende/mehrdeutige Felder, Hooks, dynamische Methoden oder unklare Zieltopologie erzeugen manuelle Aufgaben. Es gibt keinen allgemeinen Merge mit Zielstanzas. Hostname-/Hosts-Felder werden nur in unterstützten Formen vorbereitet. Immer den vollständigen Zielvergleich prüfen.

Web und Terminal bieten die aus der Quelle erfasste Stanza mit IP-Familie (`inet`/`inet6`) zur ausdrücklichen Auswahl sowie getrennte Felder für Adresse/CIDR, Gateway und DNS. Leere Werte bleiben erhalten. Der Dienst prüft Syntax, Familie und tatsächliche Feldwirkung; fehlende Felder bleiben eine manuelle Aufgabe. Im Terminal wird pro Plan eine Stanza gewählt. Für mehrere Stanzas in einem Plan das Web mit mehreren Auswahlen verwenden oder in der CLI `--network` für unterschiedliche Stanzas wiederholen:

```sh
anker restore plan --backup ID --target HOST --scenario files \
  --files etc/network/interfaces --decision etc/network/interfaces=prepared \
  --ports eno1=ens3 --console \
  --network 'vmbr0/inet=192.0.2.10/24|192.0.2.1|192.0.2.53,2001:db8::53'
```

`--address` bleibt ein freier manueller Adresswunsch. Nur Referenzen nicht behaltener Dateien benötigen Zuordnungen; gemeinsam mit einer manuell zu bearbeitenden Datei verwendete Referenzen bleiben nötig. Ändert sich die relevante Storageauswahl durch „Ziel behalten“, muss eine neue Zielaufnahme diese reduzierte Auswahl binden.

Storagezuordnung bearbeitet Gastreferenzen nur bei nachgewiesen kompatiblem Backing, Inhaltstyp, Nodeanwendbarkeit und exakt vorhandenem passenden Volume. Unbekannte oder fremde Gastzuordnung, MAC-/PCI-/Bridge-/Hookabhängigkeiten bleiben ausdrücklich offen. Gleiche Storage-IDs sind kein Nachweis. Anker formatiert, erzeugt, benennt oder verschiebt keine Pools, Datenträger oder Volumes. PBS und Gastdaten werden separat wiederhergestellt. Nodepfade werden nicht geraten.

Konten und Eigentümer werden anhand erfasster UID/GID-Namen geprüft; unbekannte oder widersprüchliche Konten bleiben manuell. Accounts, Identität, Boot, Cluster, Paketquellen, Netzwerk, Zugangs- und Anker-Helferkonfiguration, Links und Extended Attributes bleiben manuell. `fstab` ist eine Referenz: Root, Boot und Swap des Ziels erhalten und andere Mounts einzeln prüfen. **Netzwerk wird nicht automatisch geschrieben oder aktiviert.** Konsolenbestätigung und manuelle Aufgaben heben diese Grenze nicht auf.

## Alte Helfer, Sicherungen und Pläne

Zielinhalte benötigen `recovery_read_protocol: 1`, automatische Einzeldateien zusätzlich Restore-Protokoll 2 und Journalfähigkeit. Das SSH-Nachrichtenformat bleibt Version 1. Nach zentralem Update vorhandene Hosts über **Hosts → Verbindung neu einrichten** aktualisieren und einen neuen Backupstand mit Konten-/Metadatennachweisen erstellen. Fehlende Fähigkeiten werden sichtbar gemeldet; ein Lese-/Transportfehler belegt keine leere Datei.

Alte Sicherungen bleiben lesbar/exportierbar. Alte Pläne und vorhandene Hostjournale bleiben lesbar, aber eine neue Übernahme benötigt einen neu vorbereiteten Schema-2-Plan. Fehlende Nachweise werden nicht ergänzt oder geraten. Listen liefern ausschließlich erlaubte Inventar-Abfragestände (`ok`, `failed`, `not_applicable`, `unknown`), keine Rohdetails oder Befehlsfehler. Vertrauliche Inhalte liegen in Dateien mit 0600 und Verzeichnissen mit 0700; Inhaltsfreigabe und Downloads verlangen die entsprechenden Rechte.

## Unterbrechung und Rücksetzung

Der Host sichert Originale und schreibt ein synchronisiertes Journal unter `/var/lib/anker-host/rollback/<Plan-ID>/journal.json`. Eine lokale Exklusivsperre verhindert parallele Helfermutationen. Jede Datei wird unmittelbar vor Austausch und danach nach Inhalt und Metadaten geprüft. Betroffene Dienste und andere Schreiber für die Übernahme anhalten: Ankers Sperre kann fremde Programme nicht sperren, und mehrere Dateien bilden keine globale Transaktion.

Bei einem gewöhnlichen Schreibfehler setzt der Helfer eigene bereits geänderte Dateien kontrolliert zurück. Zwischenzeitliche Fremdänderungen bleiben erhalten und erscheinen als Rücksetzungskonflikt. Bei Verbindungsabbruch oder Prozessende keine zweite Übernahme starten. **Hostzustand abgleichen** liest nur das Journal. Auch wenn eine verlorene Anfrage noch einen bereiten Schema-2-Plan hinterlässt, kann der Abgleich mit passenden Zielverbindungsnachweisen und ohne anstehende/laufende Zielaufträge das authentifizierte Journal lesen. Ein bestätigt fehlendes Journal verbraucht diesen Plan als `interrupted`: **keine erneute Übernahme mit derselben Plan-ID**. Zuerst Zielkonsole und laufende Aufträge prüfen, dann Vergleich und Plan neu vorbereiten. Dies ist kein globaler Nachweis, dass nie geschrieben wurde. Lese-/Transportfehler und geänderte Verbindungen liefern keine solche Entscheidung. Ein später gefundenes passendes Journal kann erneut abgeglichen werden. Ein vorhandenes beschädigtes Journal blockiert weitere Schreiboperationen.

Ein erfolgreich abgeglichenes, passendes Journal darf weiterhin `interrupted` melden. Mit vorhandenem Rücksetzbeleg und ohne aktive Zielaufträge wird dann die getrennt bestätigte Rücksetzung verfügbar, auch nach Schließen und Wiederöffnen des Plandialogs. Eine erneute Übernahme bleibt gesperrt. Fehlendes, ungültiges oder nicht lesbares Journal erlaubt diesen Weg nicht.

**Dateien zurücksetzen** verlangt die Plan-ID erneut. Die Rücksetzung prüft für jede Datei den eigenen Nachherzustand oder den bereits wiederhergestellten Vorherzustand. Fremde Inhalte oder Rechte werden nicht überschrieben. Originalkopien und Protokoll erst nach bestätigter Betriebsprüfung entfernen. Alte Rollbackordner ohne Journal müssen vor einer neuen automatischen Übernahme manuell geprüft werden; nicht ungeprüft löschen.

„Dateien übernommen“ bedeutet: Dateischritte bestätigt. Syntax, Dienstfunktion, Netzwerk, Storage, PBS und Neustart bleiben zu prüfen. Eine erfolgreiche Rücksetzung bestätigt ebenfalls keinen vollständigen Hostbetrieb.

## Hostausfall oder neue Hardware

1. Alten Host ausschalten oder isolieren, Konsolenzugang sicherstellen.
2. Passende Proxmox-Zielversion frisch installieren. Bootloader, Rootdateisystem und Hardwaretreiber der Zielinstallation behalten.
3. Ziel über den festen SSH-Helfer inventarisieren. Sicherung und geeignetes Szenario auswählen.
4. Ports nach MAC/PCI und tatsächlicher Verkabelung zuordnen. Bridges, Bonds, VLANs, MTU, Management-IP und Routing prüfen. Anker ersetzt zugeordnete Interface-Tokens im vorbereiteten Netzwerkteil, entscheidet jedoch keine physische Verkabelung.
5. Storage-IDs, Disk-UUIDs, ZFS-Pools, LVM, Ceph und Mounts gezielt prüfen. Originale stehen im Export zur Verfügung; Anker legt keine Pools oder Gastdatenträger an. Storage-/Diskumbauten werden manuell dokumentiert.
6. Hardwareabhängige Configs und Identität anhand der Planhinweise übernehmen. Netzwerk erst mit Konsole und einem Rückweg aktivieren.
7. PBS-Anbindung herstellen, vorhandene VM-/Containerbackups separat zurückspielen, Funktionen und Neustart prüfen.

Gesamtszenarien bleiben auf realen Hosts manuell geführt. Auch eine erfolgreiche Dateiübernahme ist keine bestätigte vollständige Wiederherstellung.

## Cluster und Versionswechsel

Ein Node in einem gesunden Cluster wird nach dem passenden offiziellen Verfahren ersetzt und tritt dem bestehenden Cluster bei. Niemals eine alte `config.db` über den laufenden gesunden Cluster kopieren. Bei Gesamtausfall zuerst ein isoliertes Recovery-Netz und einen geeigneten gemeinsamen Stand auswählen. Quorum, Corosync, HA, Ceph, Zertifikate und Nodeidentitäten erfordern einen geprüften Ablauf; Anker erzwingt kein Quorum und startet keinen Cluster automatisch.

Versionsmigration verlangt einen unterstützten Installations-/Upgradepfad. Ein älteres `/etc` darf nicht komplett über eine neue Major-Version gelegt werden. Anker sperrt automatische Übernahme über Major-Grenzen und unbekannte Versionen. Auch gleiche Major-Versionen benötigen eine reale Laborabnahme.

## Ohne laufenden Anker-Dienst

Lesbare Sicherungsordner oder zuvor erzeugte TAR-/Planpakete sind ohne Dienst auswertbar. Der lokale Export aus dem Katalog benötigt keinen erreichbaren Zielhost; originale Sicherungen und private Planbelege werden dabei geprüft. Ohne Anker-Binärdatei vorhandene Sicherungsordner per SFTP kopieren oder bereits erzeugte TARs verwenden. In einem isolierten Arbeitsordner Prüfsummen prüfen:

```sh
cd /pfad/zur/sicherung
sha256sum -c checksums.sha256
```

`WIEDERHERSTELLUNG.md`, `inventory/host.json` und `manifest.json` lesen. Der rohe Baum `files/` enthält Links als Text; `manifest.json` hält Typ und Linkziel. Vollständige TAR-Exporte enthalten `original-files/` mit echten Links und Originalmodi; zuerst vollständig im isolierten Ordner prüfen. Extended Attributes stehen base64-kodiert im Manifest und zusätzlich im PAX-TAR-Export. Für vertrauliche Dateien müssen Eigentümer und Rechte vor Aktivierung stimmen.

`recovery/config.db` ist ein über die SQLite-Backup-API erzeugter Datenbankstand. `files/etc/pve/` wurde daraus dekodiert und dient dem Lesen und gezielten Extrahieren. Eine Rückspielung der Datenbank ist ein eigener manuell geprüfter Recoveryablauf. VM-/Containerdefinitionen im Baum sind Konfiguration, keine Sicherung ihrer Datenträger.

## Lokaler Nachweis und reale Abnahme

Die Integration prüft den tatsächlichen Dienst, den festen SSH-Transportvertrag und den echten Python-Helfer gegen temporäre Dateien: Sammlung, Vergleich, private Plankopie, Integrität, verlorene Antwort, Journalabgleich und Rücksetzung. Das lokale macOS-Fixture ersetzt Proxmox-Inventarkommandos und Linux-Privilegprüfung; fehlende Python-xattr-APIs werden für ausdrücklich xattr-freie Dateien modelliert. Dies belegt keine reale SSH-/Proxmoxinstallation oder Linux-xattr-Wiederherstellung.

Standalone, Hardwarewechsel, gesunder Clusterersatz, isolierter Clusterverlust, Netzwerkverlust/Rückweg, Reboot, Datenträger/Volumes, PBS/Gäste und Recovery ohne Anker sind im realen Labor weiter separat abzunehmen. Automatische Gesamtrecovery bleibt gesperrt. Prüfstand und Grenzen: [SUPPORT.md](SUPPORT.md).

## Vollständiges Offline-Planpaket

`anker restore export PLAN ./plan.tar` erhält die bisherigen Pfade (`prepared-files/`, `target-before/`, `original/` einschließlich Originalmetadaten). Zusätzlich enthält der Export `package-index.json` Version 1 mit allen logischen Archiveinträgen und `package-checksums.sha256` mit jeder regulären Datei einschließlich Index. Der Index enthält Typ, Größe, numerische Eigentümer, Modus, Änderungszeit, Symlinkziel, xattrs und Inhaltsdigest. Im Indexfeld `xattrs_base64` ist jeder Attributwert als gepaddetes Standard-Base64 (RFC 4648) codiert; auch NUL-Bytes und ungültige UTF-8-Bytes bleiben verlustfrei vergleichbar. Explizite leere `SCHILY.xattr.*`-PAX-Werte werden ebenfalls als leere Base64-Werte indexiert, auch wenn Gos TAR-Reader sie nicht in `Header.Xattrs` übernimmt. TAR-Attribute behalten ihre Rohbytes, das Originalmanifest seine bestehende Base64-Darstellung. Die Prüfsummenliste kann sich nicht selbst prüfen; sie ist kein Herkunftsbeleg. Das lesbare Dossier entsteht bei jedem Export aus dem aktuellen privaten Plan und Originalmanifest; menschlicher Evidence-Freitext bleibt mit Akteur, Datum und Ursprung erhalten.

`anker plan-package verify --input ./plan.tar` prüft lokal ohne Dienst und ohne Entpacken. Vollständige interne Abdeckung heißt `complete`; Altexporte mit nur gelisteten Quellprüfsummen heißen `limited`. Fehlende Nachweise bleiben unbekannt. Beide sind **kein Nachweis vertrauenswürdiger Herkunft oder erfolgreicher Wiederherstellung**; dafür unabhängigen Paketdigest/Herkunft und Betriebsprüfungen verwenden. Auch ein vollständig geprüftes rein manuelles Paket kann keine Zielaufnahme besitzen.

Grenzen: 50.000 logische Einträge, 256 MiB Dateiinhalte einschließlich Index/Listen, 64 MiB pro Datei, 512 MiB Archivtransport; Pfade maximal 4096 Bytes/101 Komponenten. Verifier weist Traversal, doppelte/unsichere Pfade, Hardlinks/Spezialdateien und Datei-/Link-Unterpfadkollisionen ab. Originalsymlinks werden ausschließlich als Metadaten geprüft, niemals verfolgt. Originalmetadaten werden im Archiv bewahrt; private Inhalte nicht ungeschützt entpacken oder an Support weitergeben. Für neue Exporte gelten dieselben Inhaltsgrenzen.

Das [operatorgeführte PVE-Abnahmekit](recovery-lab/README.md) liefert Preflight, Abhängigkeitsdossier und neun konkrete Szenarien. Reale Laborergebnisse fehlen. Die frische Maschinenprüfung aus Task8 ist verfügbar; die Integration ihres tatsächlichen CLI-/Ergebnisvertrags in das Abnahmekit steht in Task12 noch aus. Bis dahin meldet das Kit unvollständig.

## Frische maschinelle Prüfversuche

`anker restore verify PLAN` ruft `POST /api/plans/PLAN/verify` mit `{}` über die
Wiederherstellungsrolle auf. Die Antwort ist der Plan mit `verification_attempts`;
CLI-Exit 0 bedeutet, dass der Versuch gespeichert wurde, auch bei einem negativen
Befund. Auswerten: letzter Versuch und dessen `state`, `identity_state`,
`reboot_state`, `services_state`, `volumes_state`. Es gibt keine pauschale
Wiederherstellungsfreigabe. Die Webansicht zeigt sämtliche Versuche getrennt von
menschlichen Aufgaben. Weder diese Abfrage noch ein menschlicher Kommentar ändert
Übernahmestatus, Hostjournal oder Aufgabenbestätigungen.

Der optionale Hosthelfer meldet `recovery_verify_protocol: 1`. Seine feste Operation
`verify-recovery` verwendet ausschließlich den Leseschlüssel mit strenger
Known-Hosts-Prüfung und `--read-only`. Server und Helfer erlauben nur
`pve-cluster`, `pvedaemon`, `pveproxy` (`systemctl is-active --quiet`) und die maximal
32 Storage-IDs aus der privaten planbezogenen Zielaufnahme (`pvesm list ID`).
Geliefert werden Abfragezeit, explizite Zustände, Maschinenhash, Boot-ID und
Volumenlisten. Kein Shelltext, Pfad oder Dienst kann über den HTTP-/CLI-Aufruf
vorgegeben werden. Eine erfolgreiche Dienstabfrage belegt nur den aktiven
Systemd-Zustand; vorhandene Volumen-IDs belegen keine Inhalte, Mounts, Backing-
Identität oder Gastfunktion. Der Volumennachweis vergleicht den aufgenommenen
Zielbestand mit dem aktuellen Bestand; fehlende oder unbekannte Bestände bleiben
negativ beziehungsweise unbekannt. Unbekannte nicht aufgenommene Gastdatenträger
bleiben Teil der manuellen Aufgaben.

Stabile Identität: SHA256 der getrimmten ASCII-Bytes aus `/etc/machine-id`, genau
32 kleine Hexzeichen und nicht ausschließlich Nullen. Der Helfer erfasst dies
bereits beim Probe als `details.machine_identity_sha256` mit positivem
`details.command_results.machine_identity.state == "ok"`. Bootbaseline ist eine
syntaktisch gültige kleine UUID aus `details.boot_id`, deren echte Abfrage
`details.command_results.boot_id.state == "ok"` belegt. Beide müssen aus der
privaten, bei Planerstellung kopierten Zielaufnahme stammen. Öffentliche
`query_states`, breite Inventarfingerprints, frühere Apply-Resultate und später
nachgereichte Probe-Daten ersetzen diese Baseline nicht. Ein Neustart ist nur bei
gleichem Maschinenhash und unterschiedlicher aktueller Boot-ID nachgewiesen.
Klonen einer machine-id kann diese logische Identität duplizieren; sie ist kein
Hardwareattest. Kein echter Reboot wird ausgeführt.

Vor und nach dem Lesen prüft Anker Originalquelle, private Planbestandteile,
Zielaufnahme und aktuelle Verbindung. Die Verbindungsbindung enthält Ziel-ID,
Adresse, Port, Lesebenutzer, Known-Hosts-Pfad/-Inhaltshash und jetzt auch
Leseschlüsselpfad/-Inhaltshash. Alte Pläne bleiben lesbar und exportierbar;
eine alte Bindung oder fehlende Baseline liefert keinen frischen Nachweis und
benötigt eine neue Aufnahme/einen neuen Plan. Der Hostzeitpunkt muss innerhalb
der Serverabfrage mit fünf Minuten Uhrtoleranz liegen.

Versuche werden vor dem Hostzugriff dauerhaft als unvollständig hinterlegt und
abschließend ergänzt. Bei Prozessabbruch bleibt `interrupted` erhalten. Maximal
100 Versuche pro Plan, keine Entfernung älterer Versuche. Der eigene Katalogbucket
`recovery-verifications` wird von `Plan()` als `verification_attempts` projiziert;
der private `plan.json` und sein Ausführungszustand werden dadurch nicht umgeschrieben.
Antworten sind auf 256 KiB, pro Storage auf 10.000 Volumen-IDs mit jeweils maximal
256 Zeichen begrenzt. Dienstabfragen haben 10 Sekunden, Storageabfragen gemeinsam
20 Sekunden Zeitbudget. Falsche/fehlende/zusätzliche Felder, doppelte JSON-Felder,
unpassende Ergebnis-IDs und angehängte Antworten werden verworfen.

Jeder neue Offline-Export enthält `machine-verification.json` und eine frisch
gebildete separate Maschinenhistorie in `WIEDERHERSTELLUNG.md`, jeweils vom
Paketindex abgedeckt. Das JSON ist eine Liste historischer Versuche (`null` bei
keiner vorhandenen Historie); der private historische `plan.json` bleibt erhalten.
Disaster-Export/-Import erhält den Bucket als Rohkatalogdaten und lässt importierte
Pläne weiterhin für Hostzugriffe gesperrt. Hashes sind lokale Bindungen, keine
Signaturen. Protokollversion ist kein Beleg für einen bestimmten Helfer-Build;
Helfer-/Anker-Builddigests und reale Laborbelege müssen im Abnahmekit separat
zugeordnet werden. Reale PVE-Abnahme bleibt offen.

### Geprüfte Zuordnungsvorlagen

Im gespeicherten Plan lassen sich die angezeigten Port-, Speicher-, Hostname- und
Netzwerkzuordnungen nach ausdrücklicher Prüfung als benannte Vorlage speichern.
Der Dienst übernimmt die Absicht aus dem privaten Originalplan und ergänzt
Benutzer-ID, Prüfdatum und Vorlagenversion. Dateientscheidungen, Konsolenzugang,
Quelltrennung, Aufgabenbestätigungen und maschinelle Prüfversuche gehören nicht zur Vorlage.

Beim nächsten Entwurf zuerst das Ziel neu aufnehmen. Unter „Entscheidungen“ eine
Vorlage auswählen und frisch prüfen; erst passende Zuordnungen einsetzen.
Das Einsetzen prüft die privaten Nachweise erneut und leert Dateientscheidungen und
Bestätigungen. Abweichungen halten die gesamte Vorlage zurück. Originalplanaufnahme,
abgelaufene Aufnahme, geänderte Quelle, SSH-Verbindung einschließlich Leseschlüssel
oder Hostschlüsseldatei, Identität, Name/MAC/PCI, Version, Topologie, Backing oder
Volumebestand erlauben keine ungeprüfte Vorbelegung. Vorbereitung und Ausführung
behalten alle bisherigen Prüfungen und getrennten Bestätigungen.

Die Vorlage bindet die konkrete Sicherung einschließlich Manifest sowie die Auswahl
und Situation. Fehlende Nachweise im Originalplan bleiben historisch ungeklärt, auch
wenn eine spätere Aufnahme mehr Informationen liefert. Neue geprüfte Absicht dann
in einem neuen Plan festhalten und daraus eine neue Vorlage speichern.

Für automatische Vorbelegung von Speicherzuordnungen reicht eine gleiche Storage-ID
oder Konfigurationszeile nicht: aktuell ist nur `dir` mit vollständig gelesener
Storage-Konfiguration, erfolgreicher privater Datenträgerabfrage, genau passendem
beobachteten Mount, Dateisystem-UUID/Datenträger-ID und erfolgreicher Volumeliste
belegbar. Ein übergeordneter Mount reicht nicht. ZFS-/LVM-Namen, NFS/PBS-Endpunkte und
andere Backends liefern mit dem aktuellen Collector keine stabile tatsächliche
Backing-Identität; diese Vorlagen bleiben ungeklärt. `manual` bleibt eine manuelle
Zuordnungsabsicht. Anker legt keine Volumes an und aktiviert kein Netzwerk.

CLI (Dienst mit Wiederherstellungsberechtigung erforderlich):

```sh
anker restore recipe-list
anker restore recipe-save PLAN --label "Geprüfter Zielhost" --reviewed
anker restore recipe-preview --recipe VORLAGE --backup SICHERUNG --target ZIEL --files etc/hostname
```

`recipe-preview` nimmt das Ziel neu auf und liefert passende Vorschläge oder konkrete
Abweichungen als JSON. Es erstellt keinen Plan und führt keine Wiederherstellung aus.
Bei Speicherreferenzen muss die frische Aufnahme den benötigten Volumebestand enthalten;
fehlende Belege werden ausdrücklich aufgeführt. Vorlagen- und Snapshot-Dateien bleiben
privat; öffentliche Listen enthalten nur Bezeichnung, Ursprung und Review-Metadaten.

## Koordinierte Cluster-Sicherungssets

Unter **Sicherungen → Cluster-Sicherungssets** oder mit `anker cluster-set capture CLUSTER`
werden die aktuell für diese Clusterkennung eingerichteten Hosts als erwartete
Mitglieder festgehalten. Jeder Host erhält einen eigenen normalen Backupauftrag.
Hostausschluss, Wartungssperren und das konfigurierte Parallelitätslimit gelten
unverändert; belegte oder fehlgeschlagene Hosts werden nicht durch alte Sicherungen
ersetzt. Ein Wiederholungsauftrag erzeugt eine unabhängige Sicherung. Für einen neuen
koordinierten Versuch ein neues Sicherungsset erfassen.

`anker cluster-set list`, `show ID` und `export ID` geben die historischen
Quellreferenzen aus. Der JSON-Export enthält ausschließlich Referenzen, Status und
Zeitgrenzen (`purpose: manualSourcePlanning`, `atomic: false`), keine privaten
Dateien und keine Wiederherstellungsfreigabe. Benötigte Sicherungen zusätzlich
separat exportieren, prüfen und gegen Aufbewahrungsrotation schützen.

Ein vollständiger, übereinstimmender Satz benötigt tatsächliche veröffentlichte
Ergebnisse jedes erwarteten Hosts, erfolgreiche vollständige Sicherungen,
übereinstimmende erfasste Host-/Clusteridentität und die erwarteten Mitgliedsnamen
in der gesicherten `etc/pve/corosync.conf`. Nicht eindeutig auswertbare Syntax oder
fehlende Mitgliedsnamen bleiben unbekannt. Die logische gemeinsame pmxcfs-Prüfsumme
bindet Pfad, Dateityp und Inhalt der aus dem Datenbanksnapshot exportierten
`etc/pve`-Dateien außerhalb von `nodes/`; lokale Node-/Gastdateien werden damit
nicht auf Gleichheit geprüft. SQLite-Speicherlayout und Dateizeitstempel gehen
nicht in diese Inhaltsprüfung ein. Private Dateien sind in der internen Prüfsumme
enthalten; weder diese Prüfsumme noch Dateiinhalte werden öffentlich projiziert.

Beginn, Ende und maximaler Startversatz der tatsächlichen Erfassungen stehen im
Detail. „Vollständig“ beschreibt ausschließlich dieses zeitversetzte
Erfassungsfenster, keinen atomaren Clusterstand, keine aktuelle Integrität und
keinen gesunden/quoraten Cluster. Abweichungen und Pflichtlücken bleiben sichtbar.
Unterbrochene Aufträge liefern ohne erfasste Ergebnisse keinen Quellnachweis.
Gelöschte oder nicht mehr lesbare Quellmanifeste machen den Satz unvollständig;
historische IDs bleiben erhalten. Originaldateien vor Verwendung separat prüfen.

Sicherungssets sind versionierte Katalogeinträge und werden mit dem verschlüsselten
Disaster-Paket samt vorhandenem Sicherungsmaterial übernommen. Es entstehen keine
zusätzlichen privaten Sicherungssetdateien. Importisolation und die Ungültigkeit alter
Ausführungsfreigaben bleiben erhalten. Die Auswahl eines Satzes startet keine
Cluster-, Gast-, Storage- oder Wiederherstellungsaktion. Reale PVE-Abnahme bleibt
in einer isolierten Laborumgebung erforderlich.
