# Anker-Dokumentation

[Projektübersicht](../README.md) · [Handbuch](HANDBOOK.md)

## Erste Inbetriebnahme

1. Anker auf einem eigenen Linux-System mit systemd installieren. Für ein Release Paket und Signatur prüfen. [Installation und Updates](UPDATES.md#erstinstallation-aus-einem-release)
2. Webzugriff festlegen, Administrator anlegen und beide Dienste prüfen. [Erste Einrichtung](HANDBOOK.md#erste-einrichtung) · [Webzugriff und TLS](HANDBOOK.md#webzugriff-und-tls)
3. Einen PVE- oder PBS-Host anbinden. Den SSH-Fingerprint an der Hostkonsole ablesen und vor der Anmeldung vergleichen. [Hosts anbinden](HANDBOOK.md#proxmox-hosts-anbinden)
4. Den ersten Stand sichern. Unter **Aufträge** den Abschluss prüfen. Danach Integrität und die benötigten Pfade im Sicherungsstand kontrollieren. [Erstprüfung](OPERATIONS.md#erstprüfung-und-regelbetrieb)

## Anleitungen nach Aufgabe

| Aufgabe | Anleitung |
| --- | --- |
| Einrichten, anmelden, Hosts und Sicherungen verwalten | [Handbuch](HANDBOOK.md) |
| Aufträge, Speicher, Schlüssel und Wartung betreiben | [Betrieb](OPERATIONS.md) |
| Installation, Signaturen und Update-Rückfall prüfen | [Installation und Updates](UPDATES.md) |
| Dateien auf einem Host übernehmen oder zurücksetzen | [Wiederherstellung](RECOVERY.md) |
| PBS-Konfiguration auf einem anderen PBS wiederherstellen | [PBS-Ablauf](HANDBOOK.md#pbs-konfiguration-geführt-wiederherstellen) |
| Zentralen Anker-Server nach einem Ausfall wiederherstellen | [Verschlüsseltes Disaster-Paket](OPERATIONS.md#kaltes-verschlüsseltes-disaster-paket) |
| Testumfang und offene Abnahmen prüfen | [Unterstützungsstand](SUPPORT.md) |

## Vor einer Wiederherstellung

Quelle, Ziel und Dateiauswahl im Plan prüfen. Einen Wiederherstellungsversuch zuerst auf einem isolierten Testhost durchführen. Der Dateivergleich bestätigt gespeicherte Inhalte. Dienste, Netzwerk, Storage und Clusterzustand anschließend am Ziel prüfen. Für VM- und Containerdaten die vorhandenen Gast-Backups verwenden.

Für einen Labortest die Anleitung und die Ergebnisse getrennt halten. Die tatsächliche PVE-Abnahme ist im [Unterstützungsstand](SUPPORT.md) dokumentiert. Das [Abnahmekit für PVE](https://github.com/jahartmann/Anker/tree/main/docs/recovery-lab) enthält die Szenarien und die Anforderungen an die Nachweise.
