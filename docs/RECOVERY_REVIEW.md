# Prüfung der Wiederherstellung

Prüfstand: 7. Oktober 2026, Programmstand `1456734d670334f249d763e1d10938d74a5c0bc1` (0.2.7). Die folgenden Befunde dokumentieren den Ausgangsstand. Umsetzung und Nachprüfung stehen am Ende; kein Produktionshost wurde kontaktiert.

## Urteil

Die Architektur ist sinnvoll: Originale und Metadaten erhalten, einen zielbezogenen Plan erzeugen, vor der Übernahme Integrität und Zielzustand erneut prüfen und einen nachvollziehbaren Export ohne Anker ermöglichen. Der Stand ist jedoch noch kein fertig abgenommener vollständiger Host-Restore. Auch die automatische Einzeldateiübernahme benötigt zusätzliche Absicherungen.

| Fall | Aktueller Stand | Bewertung |
|---|---|---|
| Ausgewählte gewöhnliche Config auf einem bekannten Host | Inhaltsprüfsummen, Bestätigung, getrennte SSH-Berechtigung, Rollbackkopie, atomarer Austausch je Datei | Tragfähige Grundlage; Konkurrenzänderungen, Metadaten und Unterbrechungsjournal nachbessern |
| Netzwerkdatei | Portnamen werden angepasst; Konsole erforderlich; Aktivierung manuell | Kein nachgewiesener automatischer Rückweg bei Erreichbarkeitsverlust |
| Vollständiger Standalone-Host / neue Hardware | Frische Proxmox-Installation, vorbereitete Dateien, manuelle Schritte und Originalexport | Kein automatischer vollständiger Wiederaufbau |
| Ersatznode im gesunden Cluster | Manueller Ausbau/Beitritt; alte Datenbank nicht in den lebenden Cluster übernehmen | Konzept richtig, koordinierter Ablauf und reale Abnahme fehlen |
| Vollständiger Clusterverlust | Isolierter Wiederaufbau und manueller gemeinsamer Stand | Keine koordinierte Cluster-Recovery; Quorum, HA und Storage separat abnehmen |
| SFTP-/TAR-Recovery ohne Anker | Lesbare Dateien, Metadaten, Prüfsummen und Originaldateibaum im Export | Nutzbarer Rückweg; keine ungeprüfte Extraktion nach `/` |

## Was bereits richtig abgesichert ist

- Sicherung, Planmanifest, Mapping und vorbereitete Inhalte werden vor Verwendung geprüft.
- Eine aktuelle Zielprüfung verhindert die Nutzung eines Plans nach vielen Zieländerungen; zusätzlich prüft der Hosthelfer die ursprünglichen Dateiinhalte.
- Sicherung und Wiederherstellung haben getrennte SSH-Zugänge. Die lesende Sicherungsberechtigung erlaubt keine Übernahme.
- Rollbackdateien und Metadaten werden vor der ersten Änderung synchronisiert. Der Austausch einer einzelnen Datei erfolgt über eine temporäre Datei und Rename.
- Unbekannte Versionen, Pflichtlücken, bestimmte Systemdateien, Symlinks und Extended Attributes werden gesperrt oder als manuell behandelt.
- Wiederherstellungen werden nach Fehler oder Neustart nicht ungeprüft automatisch wiederholt. Nach dem Schreiben bleibt die Betriebsprüfung ausstehend.
- Produktion lässt die automatische Gesamtausführung der anderen Szenarien gesperrt.

## Konkrete Befunde

### 1. Konkurrenzänderung nach der Vorprüfung wird überschrieben

`host/anker_host.py:225–259` prüft alle Dateiinhalte zuerst, legt anschließend Rollbackkopien an und schreibt danach sämtliche Dateien. Zwischen Prüfung und Austausch erfolgt keine erneute Prüfung der jeweiligen Datei. Eine zwischenzeitliche Änderung durch Administrator, Dienst oder einen zweiten lokalen Helfer kann verloren gehen. Die zentrale Hostsperre koordiniert nur Ankers eigenen Prozess.

Reproduktion: nach dem Synchronisieren der Rollbackkopie die Zielconfig von `old-a` auf `concurrent-admin-change` ändern. Der Helfer akzeptiert die Übernahme, schreibt `new-a` und enthält in der Rollbackkopie weiterhin nur `old-a`.

Erforderlich: lokale Exklusivsperre für Helfermutationen, definierter Änderungsstopp für betroffene Dienste, überprüfte Vorbedingungen je Datei unmittelbar vor dem Austausch und Erkennung von Abweichungen bei Nachprüfungen. Eine Helfersperre allein hält fremde Schreiber nicht auf; diese Grenze muss sichtbar bleiben.

### 2. Fehler beim zweiten Austausch hinterlässt einen gemischten Stand

Ein Rename ist nur für eine einzelne Datei atomar. Die Gruppe ausgewählter Dateien ist keine Transaktion. Reproduktion: die zweite von zwei Dateien wirft beim Austausch einen simulierten Datenträgerfehler. Ergebnis: erste Datei neu, zweite Datei alt. Rollbackdaten und `failure.json` bleiben auf dem Host, aber es gibt keine automatisch kontrollierte Wiederaufnahme oder Rücksetzung.

Erforderlich: dauerhaftes, planbezogenes Hostjournal mit vorbereiteten Dateien, Vorher-/Nachherhashes, Metadaten, einzelnen Schreibphasen und eindeutigem Abschluss. Teilzustände dürfen nicht als Erfolg oder sicherer Neuversuch erscheinen. Ein Rollback muss prüfen, ob inzwischen andere Änderungen vorgenommen wurden; blindes Zurückkopieren kann weitere Daten überschreiben. Abhängige Configs benötigen gemeinsame Reihenfolge und geprüfte Aktivierung. Eine globale atomare Hosttransaktion wird nicht versprochen.

### 3. Ursprünglicher Schreibfehler wird verdeckt

`host/anker_host.py:263–265` schreibt im `except` die Fehlerdatei, führt das nackte `raise` aber erst außerhalb des Blocks aus. Reproduktion des zweiten Dateifehlers meldet `RuntimeError: No active exception to reraise` statt des Datenträgerfehlers. Die eigentliche Ursache steht nur in der lokalen Fehlerdatei.

Erforderlich: ursprüngliche Ausnahme im Handler weiterreichen, Journalfehler gesondert behandeln und den bereits veränderten Zustand samt Recovery-ID strukturiert melden. Auch das Fehler-/Abschlussjournal muss dauerhaft geschrieben werden.

### 4. UID/GID werden auf anderer Hardware ungeprüft übernommen

`internal/anker/ssh.go:112–155` sendet numerische Eigentümer aus dem Quellmanifest; `host/anker_host.py:256–257` setzt sie auf dem Ziel. Gleiche Proxmox-Hauptversion bedeutet nicht gleiche Benutzer- und Gruppenidentität. Der Inhaltsfingerprint erfasst auch keine Änderung von Dateieigentümer, Modus oder ACL bei unveränderten Bytes.

Erforderlich: Benutzer-/Gruppenidentität und vorhandene Zielmetadaten erfassen; benötigte Konten explizit zuordnen oder die automatische Übernahme sperren. Ziel-ACLs/xattrs dürfen beim Dateiaustausch nicht unbemerkt verschwinden. Bei einem bekannten unveränderten Host kann eine eng geprüfte direkte Zuordnung ausreichen.

### 5. Netzwerk ist vorbereitet, aber nicht mit Rückweg aktiviert

Die Konsolenbestätigung und Rollbackkopie sind sinnvoll. Es fehlen jedoch ein lokaler Wächter, Rücksetzung nach ausbleibender Bestätigung sowie die Prüfung von Bridges, Bonds, VLANs und Portkollisionen. Die aktuelle Portersetzung ist textbasiert, kein vollständiger Parser für Netzwerksemantik.

Erforderlich: eindeutige Portzuordnung, Prüfung der abhängigen Netzwerkstruktur und Syntax; vor Aktivierung einen lokalen, vom SSH-Kanal unabhängigen Rückfall vorbereiten. Erst nach erneuter Verbindung zum identifizierten Ziel und ausdrücklicher Bestätigung die Änderung dauerhaft freigeben. Bis zur realen Abnahme bleibt die Aktivierung manuell über Konsole.

### 6. Inventarfehler können wie fehlende Ausstattung aussehen

`command()` liefert bei nicht erfolgreicher Ausführung oder Timeout einen leeren String. Einige Inventarfelder werden dadurch zu leeren Listen, ohne Fehlermeldung. Fehlende erforderliche Erkenntnisse müssen als „unbekannt/Abfrage fehlgeschlagen“ erscheinen, nicht als „nicht vorhanden“.

Erforderlich: Ergebnistypen für erfolgreich, nicht anwendbar, fehlgeschlagen und abgeschnitten; benötigte Inventarfelder je Szenario festlegen und bei unbekanntem Zustand blockieren. Nicht jedes nicht installierte Werkzeug ist ein Fehler, etwa Ceph auf einem Host ohne Ceph.

### 7. Storage- und Identitätszuordnungen sind noch keine ausführbaren Regeln

`Mapping.Storage`, `Mapping.Hostname` und `Mapping.Address` sind im Datenmodell vorhanden, werden aber bei der Dateivorbereitung nicht umgesetzt. Bootmodus, UUIDs, ZFS-/LVM-/Mounts, PCI-Zuordnung und Paketabhängigkeiten bleiben manuelle Prüfungen. Deshalb darf ein Hardwarewechselplan noch nicht als automatisch wiederherstellbar gelten.

Erforderlich: getrennte, getestete Regeln für jedes unterstützte Ziel; vorhandene Datenträger nie anhand eines Gerätenamens formatieren. Frische Boot-/Hardwarekonfiguration erhalten, vorhandenen Storage identifizieren, Hostidentität bewusst übernehmen oder ersetzen und Geheimnisse entsprechend behandeln.

## Zielablauf mit möglichst wenig Bedienung

1. **Quelle und Fall wählen:** Einzelconfig, Standalone-Wiederaufbau, Ersatznode oder Clusterverlust. Anker erklärt unmittelbar, ob automatische Übernahme oder geführter Export unterstützt ist.
2. **Ziel prüfen:** Identität, Version, vollständiges erforderliches Inventar, Eigentümer, Netzwerk und Storage. Unklare Zuordnungen erscheinen als wenige konkrete Entscheidungen.
3. **Änderungen ansehen:** tatsächlicher Zielstand gegen vorbereiteten Inhalt, einschließlich Metadaten und nötiger Dienstaktionen. Der heutige Diff zeigt nur Quellinhalt gegen vorbereiteten Inhalt; die Oberfläche benennt diese Grenze bereits.
4. **Übernahme absichern:** Zielzustand und Rollback sichern, alle Dateien vorab bereitstellen, dauerhafte Recovery-ID auf Ziel und Zentrale speichern, abhängige Dienste kontrollieren.
5. **Anwenden und prüfen:** schrittweise protokollieren, Inhalte und Metadaten nachlesen, Syntax/Dienste/Erreichbarkeit prüfen. Netzwerk nur mit lokalem Rückweg aktivieren. Unklare SSH-Ergebnisse über das Hostjournal abgleichen.
6. **Abschließen:** „Dateien übernommen“ von „Dienste geprüft“ und „Neustart geprüft“ trennen. Gesamt-Recovery erst nach den erforderlichen Prüfungen als abgeschlossen markieren; Export und manuelle Rücksetzung bleiben zugänglich.

Ein gesunder Cluster übernimmt seinen aktuellen gemeinsamen Zustand beim Wiederbeitritt eines Ersatznodes. Bei vollständigem Clusterverlust wird dagegen ein isolierter gemeinsamer Ausgangsstand benötigt. Alte Node-Sicherungen nacheinander in einen lebenden Cluster einzuspielen ist kein geeigneter Wiederherstellungsablauf.

## Erforderliche Abnahme

- Echter Proxmox-Standalone mit Einzelconfig und mehreren abhängigen Dateien; unveränderte sowie andere UID/GID und Ziel-ACLs.
- Geänderte Hardware mit BIOS/UEFI, anderen NICs, Bonds/VLANs, Storage-IDs und UUIDs; vorhandene Daten bleiben erhalten.
- Ersatznode bei gesundem Cluster; getrennt davon vollständiger Verlust im isolierten Netz mit HA-/Storage-Prüfung.
- Abbruch vor dem Schreiben, zwischen Dateien und nach letztem Austausch vor Antwort; SSH-Ausfall, Prozessabbruch, Reboot und Stromverlust.
- Fehlender Platz oder Inodes, beschädigte Sicherung, fehlerhafte Configsyntax und externe Änderungen während der Ausführung.
- Netzwerkverlust mit belegtem Rückweg; Wiederherstellung ohne laufendes Anker ausschließlich aus Export und Anleitung.

Lokaler Nachweis dieser Prüfung: 17 bestehende Python-Helfertests bestanden; gezielte Go-Plan-/Drift-/Wiederanlauf-/Berechtigungsprüfungen mit Race Detector bestanden. Zwei zusätzliche isolierte Fehlerexperimente bestätigten die oben beschriebenen Probleme. Die Experimente nutzten nur temporäre lokale Verzeichnisse und injizierte Fehler, keine Proxmox-Hosts. Sie sind keine reale Recovery-Abnahme.

Offizielle Grundlagen: [Proxmox pmxcfs und Recovery](https://github.com/proxmox/pve-docs/blob/master/pmxcfs.adoc), [Proxmox Clusterverwaltung](https://github.com/proxmox/pve-docs/blob/master/pvecm.adoc). Insbesondere ist `/etc/pve` ein datenbankgestütztes Clusterdateisystem; der Clusterbeitritt überschreibt seine bestehende Konfiguration.

## Nachbesserung für 0.2.8

Die Befunde 1–4 und 6 sind mit lokaler Helfersperre, unmittelbarer Inhalts-/Metadatenprüfung, vollständigem Staging, synchronisiertem Recoveryjournal, kontrollierter Rücksetzung, erhaltenen Fehlerursachen, Benutzeridentitäten und expliziten Inventarabfragezuständen bearbeitet. Fremde Schreiber bleiben eine betriebliche Grenze: betroffene Dienste während der Übernahme anhalten. Es gibt keine globale atomare Hosttransaktion.

Zu Befund 5: Nur benötigte physische Ports werden zugeordnet; Bridges, Bonds, VLANs und numerische Aliase werden berücksichtigt, doppelte Ziele und unbekannte Referenzen blockieren. Netzwerk wird ausschließlich manuell vorbereitet und exportiert. Die automatische Übernahme bleibt gesperrt, bis ein unabhängiger Rückfallwächter und die reale Abnahme vorliegen.

Zu Befund 7: Referenzierte Storage-IDs und manuelle Identitätsentscheidungen ersetzen irrelevante Laufwerkslisten. Diese Entscheidungen werden sichtbar im Plan dokumentiert; automatische Storage-/Boot-/Clusterregeln sind weiter offen.

Die unabhängige Integrationsprüfung fand zusätzlich verlorene Disk-/Adapterfelder, mögliche Selbstabschaltung durch Restore der Zugangskonfiguration, teilweise dekodierte ungültige Hostantworten, geerbte Staging-ACLs und eine fehlerhafte UI-Sperre nach abgewiesenem Auftrag. Diese Fälle wurden mit Regressionen korrigiert. Echte Proxmox-Hardware-, Cluster-, Reboot- und Stromausfallabnahme bleibt erforderlich.
