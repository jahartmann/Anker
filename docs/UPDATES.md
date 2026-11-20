# Installation und Updates

[Doku-Übersicht](README.md) · [Betrieb](OPERATIONS.md) · [Releases](https://github.com/jahartmann/Anker/releases)

## Erstinstallation aus einem Release

Die [vier Schritte im Handbuch](HANDBOOK.md#installation) führen von der Installation bis zur ersten geprüften Sicherung. Der Standardweg verwendet das signierte Linux-Paket.

## Weitere Installationswege

| Fall | Anleitung |
| --- | --- |
| Aus einem Git-Checkout bauen | [Quellcode installieren](reference/SETUP.md#installation) |
| Privates Repository oder Paket manuell prüfen | [Installationsvarianten und Signaturprüfung](reference/UPDATES.md#erstinstallation-aus-einem-release) |
| Setup nach Abbruch fortsetzen | [Einrichtung wiederholen](reference/SETUP.md#erste-einrichtung) |
| Release erstellen und signieren | [Repository vorbereiten](reference/UPDATES.md#repository-vorbereiten) · [Release erstellen](reference/UPDATES.md#release-erstellen) |

## Update installieren

1. Unter **Einstellungen → Updates** die Quelle und angebotene Version prüfen. Falls noch keine Quelle eingerichtet ist, **Updatequelle einrichten** wählen und den Fingerprint prüfen.
2. Laufende Sicherungen und Wiederherstellungen abwarten. Das Update starten und den Status beobachten.
3. Nach dem Neustart laufende Version und Dienststatus prüfen. Anschließend den nächsten Sicherungslauf kontrollieren.

```sh
sudo anker update check
sudo anker update install
sudo anker update status
```

Der Updater prüft Signatur und Prüfsumme, sichert Programm und Katalog und prüft den Dienststart. Die Hosthelfer werden dabei nicht ausgetauscht. [Details zum Updateablauf](reference/UPDATES.md#update-installieren) · [Helferstand prüfen](OPERATIONS.md#weitere-betriebsaufgaben)

## Fehler und Rückfall

```sh
sudo anker update status
sudo journalctl -u anker -u anker-updater -n 100 --no-pager
```

Bei einem fehlgeschlagenen Start versucht Anker, Programm und Katalog zurückzusetzen. Bleibt auch der Start nach dem Rückfall aus, zuerst Journal, Speicher und Katalog prüfen. Das Updatejournal für den Wiederanlauf erhalten.

- [Unterbrochenes Update und Wiederanlauf](reference/UPDATES.md#fehler-und-rückfall)
- [GitHub 403 oder 429 einordnen](reference/UPDATES.md#github-meldet-http-403)
