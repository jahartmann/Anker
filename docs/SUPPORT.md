# Unterstützungsstand

| Bereich | Implementiert | Lokal geprüft | Reales Proxmox-Labor |
|---|---|---|---|
| Lesbare versionierte Sicherungen und Prüfsummen | Ja | Go-Tests, Demo | Offen |
| SQLite-Snapshot und pmxcfs-Dekodierung | Ja | Python-Fixture mit konsistentem Baum | Offen |
| SSH mit geprüftem Hostschlüssel und festem Helfer | Ja | Argument-/Protokollprüfungen | Offen |
| Einzeldateiübernahme mit Driftprüfung und Rollbackkopie | Ja | Go-/Python-Tests, Demo-Browserablauf | Offen |
| Netzwerkportzuordnung | Ja | Transformation und fehlende Zuordnung | Verkabelung/Erreichbarkeit offen |
| Neue Hardware, Cluster, Versionen, Topologie | Manuelle Pläne und Exporte | Entscheidungsregeln und Dokumente | Offen; automatische Gesamtausführung gesperrt |
| Web, CLI und Terminal | Ja | Build, Go-Tests, Browserabläufe | Betrieb offen |
| Aufbewahrung, Archivierung, Scheduler | Ja | Go-Tests einschließlich DST und Archivprüfung | Langzeitbetrieb offen |
| Linux-/systemd-/SFTP-Installation | Skripte und Anleitung | Syntax und Crossbuild | Ausführung offen |

Der Versionsrahmen für automatische Einzeldateipläne umfasst gleiche Proxmox-Major-Versionen 8 oder 9. Das ist eine Entscheidungsregel, keine Zertifizierung jedes Minorstands. Hardware-, Cluster- und Versionskombinationen müssen vor Freigabe separat dokumentiert werden.

Erforderliche Abnahme: mindestens ein Standalone-Host, ein Node im gesunden Cluster und ein vollständiger Clusterverlust im isolierten Netz; neue Hardware mit anderen NIC-Namen und Diskkennungen; PBS-Verbindung und Gastrestores; Neustart; Wiederherstellung ohne Anker über SFTP; unterbrochene Übertragung und Übernahme; knappes Dateisystem; Linux-Servicehärtung und effektive SSH-Berechtigungen.

Grenzen: keine Gastdaten, kein Image-/Bootloaderrestore, keine automatische Quorumkorrektur, kein Ceph-Wiederaufbau, kein In-place-Upgrader, kein automatisch bestätigter Reboot, keine atomische Gesamtrückspielung und keine automatisierte Metadatenübernahme für Extended Attributes/Links. Diese Bereiche erscheinen als manuelle Schritte und bleiben exportierbar.
