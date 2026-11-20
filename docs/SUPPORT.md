# Unterstützungsstand

[Doku-Übersicht](README.md) · [Wiederherstellung](RECOVERY.md) · [Prüfberichte und frühere Nachweise](reference/SUPPORT.md)

| Bereich | Umfang |
| --- | --- |
| Proxmox VE und PBS | Sicherung der unterstützten Hostkonfigurationen und Inventardaten. Gastdatenträger und PBS-Datastore-Inhalte separat sichern. |
| Einzeldateien | Plan, Zielprüfung, Übernahme, Journal und Rücksetzung für unterstützte Dateien. Geschützte Dateien erfordern eine eigene Bestätigung. |
| PBS-Wiederherstellung | Unterstützte Konfiguration auf einem PBS derselben Hauptversion. [Voraussetzungen und Einschränkungen](reference/HOSTS.md#pbs-konfiguration-geführt-wiederherstellen) |
| Host- und Cluster-Recovery | Anker übernimmt freigegebene Dateien. Boot, Netzwerk, Storage, Quorum und Dienste am Ziel separat abnehmen. |
| Release-Build | Go-, Python- und Browserprüfungen, Linux-systemd-Test und Prüfung der signierten Pakete für beide Architekturen. [Prüfungen für v0.2.23](https://github.com/jahartmann/Anker/actions/runs/44688764085) |

## Reale Abnahme

Die automatisierten Prüfungen ersetzen keinen Wiederherstellungstest auf einem realen PVE- oder PBS-System. Die dokumentierte Laborabnahme bleibt offen. Für einen eigenen Test isoliert starten und Sicherungs-ID, Ziel, Plan-ID, Ergebnisprüfung und Rücksetzung festhalten.

[Nachweise und Grenzen](reference/SUPPORT.md#offline-recovery-und-laborbelege) · [PVE-Abnahmekit](https://github.com/jahartmann/Anker/tree/main/docs/recovery-lab)
