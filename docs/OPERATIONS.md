# Betrieb

[Doku-Übersicht](README.md) · [Schnellstart](HANDBOOK.md#installation) · [Wiederherstellung](RECOVERY.md)

## Erstprüfung und Regelbetrieb

Im normalen Betrieb reicht die Weboberfläche:

| Prüfen | Wo |
| --- | --- |
| Welche Hosts Handlungsbedarf haben | **Übersicht** |
| Ob ein Auftrag erfolgreich war und was er gesichert hat | **Aufträge → Ablauf ansehen** |
| Ob `/srv/anker` auf dem vorgesehenen Dateisystem liegt und genug Platz hat | **Einstellungen → Betrieb → Speicher** |

Je Host prüfen, ob der Stand vollständig ist, die benötigten Dateien enthält und die Integritätsprüfung besteht. Datum und Sicherungs-ID für den Wiederherstellungstest festhalten.

<details>
<summary>Prüfen, wenn die Weboberfläche nicht erreichbar ist</summary>

```sh
sudo systemctl status anker anker-updater --no-pager
findmnt --target /srv/anker
df -h /srv/anker
sudo anker jobs
```

Wenn `/srv/anker` ein eigener Mount sein soll, Quelle und Ziel mit der Planung vergleichen. Bei einer Abweichung vor der ersten Sicherung den Mount korrigieren.

</details>

## Zeitplan und Aufträge

| Einstellung | Standard |
| --- | --- |
| Tägliche Sicherung | Ab 02:00 Uhr, Europe/Berlin. Der Start pro Host ist um weniger als eine Stunde versetzt. |
| Parallele Aufträge | Vier insgesamt, höchstens einer pro Host. |
| Aufbewahrung | 30 Tages-, 12 Wochen- und 12 Monatsstände. |
| Archivierung | Nach 90 Tagen. |

Im Hostformular einen eigenen Zeitplan setzen. Für eine befristete Pause **Wartung** verwenden. Manuelle Sicherungen bleiben möglich. [Zeitplan und Aufträge im Detail](reference/OPERATIONS.md#zeitplan-und-aufträge) · [Aufbewahrung und Speicher](reference/OPERATIONS.md#aufbewahrung-und-speicher)

## Störungen eingrenzen

| Befund | Nächste Prüfung |
| --- | --- |
| Kein neuer Stand | Wartung, Automatik und letzten Auftrag prüfen. |
| Sicherung unvollständig | Gemeldeten Pfad, Leserechte und Hostprofil prüfen. Danach neu sichern. |
| Prüfsummenfehler | Letzten intakten Stand bestimmen und den Speicherpfad prüfen. |
| Dienst startet nicht | Journal prüfen. Nach einem Update zuerst den [Rückfallstatus](UPDATES.md#fehler-und-rückfall) abfragen. |
| SSH bricht während der Dateiübernahme ab | **Hostzustand abgleichen** öffnen und das Ziel prüfen. [Unterbrechung und Rücksetzung](RECOVERY.md#unterbrechung-und-rücksetzung) |

```sh
sudo journalctl -u anker -u anker-updater -b -n 100 --no-pager
sudo anker job show AUFTRAGS-ID
```

Für die Diagnose Host, Zeitpunkt, Auftrags-ID und relevante Fehlermeldung festhalten. Zugangsdaten und private Sicherungsinhalte bleiben auf dem Server.

## Kaltes, verschlüsseltes Disaster-Paket

Ein Ausfall des Anker-Servers ist ein eigener Fall. Den verschlüsselten Cold-Export bei gestopptem Anker-Dienst erstellen, prüfen und getrennt aufbewahren. Passphrase separat verwahren. Import und Prüfung zuerst auf einem isolierten Ziel durchführen. [Export, Prüfung und Import](reference/DISASTER.md)

## Weitere Betriebsaufgaben

| Aufgabe | Anleitung |
| --- | --- |
| Helferversion oder Liveabweichung prüfen | [Helfer und Livevergleich](reference/OPERATIONS.md#ursachenverlauf-helfer-und-livevergleich) |
| Speicher erweitern oder umziehen | [Speicher und Erweiterung](reference/OPERATIONS.md#speicheranzeige-und-erweiterung) |
| Sicherungsordner über SFTP exportieren | [SFTP-Export](reference/OPERATIONS.md#optionaler-sftp-export-ohne-zweite-kopie) |
| Zugang und Passwörter verwalten | [Zugänge](reference/OPERATIONS.md#zugänge) |
| Mehrere Hosts bearbeiten oder pausieren | [Sammelaktionen und Wartung](reference/OPERATIONS.md#gemeinsame-hostaktionen-und-befristete-wartung) |
| <a name="interner-testbetrieb"></a>Historischen internen Kandidaten prüfen | [Interner Testbetrieb](reference/OPERATIONS.md#interner-testbetrieb) |
