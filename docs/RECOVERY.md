# Wiederherstellung

[Doku-Übersicht](README.md) · [PBS-Wiederherstellung](HANDBOOK.md#pbs-konfiguration-geführt-wiederherstellen) · [Unterstützungsstand](SUPPORT.md)

## Vor dem Start

Sicherungs-ID, Integrität und Zielhost prüfen. Bei Änderungen an Netzwerk oder Zugang die Zielkonsole bereithalten. Für einen Wiederherstellungstest einen isolierten Host mit unkritischen Testdateien verwenden.

## Fünf Schritte

| Schritt | Am Ziel zu prüfen |
| --- | --- |
| **1. Quelle und Situation wählen** | Richtiger Stand und passendes Szenario. |
| **2. Ziel prüfen** | Hostidentität, Dateibestand, Ports und Storage. |
| **3. Dateien entscheiden** | Original aus Sicherung, vorbereitete Fassung, Ziel behalten oder manuell bearbeiten. |
| **4. Plan prüfen** | Pfade, geänderte Inhalte, Eigentümer und Rechte. Geschützte Dateien separat bestätigen. |
| **5. Übernehmen und Ergebnis prüfen** | Plan-ID bestätigen, Dateiübernahme prüfen und betroffene Dienste am Ziel testen. |

Anker sichert den vorherigen Dateistand im Hostjournal. Netzwerk und Dienste anschließend gezielt am Ziel prüfen und aktivieren. [Plan und Zuordnungen im Detail](reference/RECOVERY.md#fünf-schritte)

> [!IMPORTANT]
> Änderungen unter `/etc/pve/` können sofort im gesamten Cluster wirksam werden.

## Unterbrechung und Rücksetzung

- Bei Abbruch oder verlorenem SSH die Übernahme nicht wiederholen. **Hostzustand abgleichen** liest das Journal. Danach den tatsächlichen Zustand am Ziel prüfen.
- **Dateien zurücksetzen** verlangt die Plan-ID erneut. Zwischenzeitliche Fremdänderungen bleiben erhalten und werden als Konflikt angezeigt.
- Sind die Dateien geprüft, folgt die Betriebsprüfung. Welche Dienste, Mounts oder Clusterfunktionen zu prüfen sind, hängt vom Plan ab.

[Journal, Abgleich und Rücksetzbedingungen](reference/RECOVERY.md#unterbrechung-und-rücksetzung)

## Sonderfälle

| Fall | Anleitung |
| --- | --- |
| Neuer Host oder geänderte Hardware | [Hostausfall und neue Hardware](reference/RECOVERY.md#hostausfall-oder-neue-hardware) |
| Cluster-Node ersetzen oder Cluster wiederaufbauen | [Cluster und Versionswechsel](reference/RECOVERY.md#cluster-und-versionswechsel) · [Cluster-Sicherungssets](reference/RECOVERY.md#koordinierte-cluster-sicherungssets) |
| PBS-Konfiguration bei erhaltenen Datastores | [PBS-Ablauf](reference/HOSTS.md#pbs-konfiguration-geführt-wiederherstellen) |
| Ohne laufenden Anker-Dienst arbeiten | [Sicherungen und exportierte Pläne prüfen](reference/RECOVERY.md#ohne-laufenden-anker-dienst) |
| Anker-Server selbst wiederherstellen | [Verschlüsseltes Disaster-Paket](reference/DISASTER.md) |

VM- und Containerdaten aus den vorhandenen Gast-Backups wiederherstellen. Die Dateiübernahme bestätigt noch keinen vollständigen Betrieb von Host oder Cluster.
