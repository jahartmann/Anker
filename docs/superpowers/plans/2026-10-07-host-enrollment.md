# Automatische Hostanbindung – Umsetzung

**Ziel:** Einmal SSH-Zugang eingeben; Identität bestätigen; Helfer und eingeschränkte Schlüssel automatisch einrichten. Nutzer hat die Umsetzung ausdrücklich beauftragt.
**Architektur:** Eingebettete vorhandene Hostinstaller, Go-SSH-Client und fester root-Helfer. Webdienst übernimmt Berechtigungen und Katalog; Web/Terminal sind getrennte Verbraucher derselben API.
**Spec:** ../specs/2026-10-07-host-enrollment.md

## Schnittstellen

POST /api/hosts/connection/inspect: {address,port} → {address,port,fingerprint,key_type,known,changed}.
POST /api/hosts/connection/enroll: {host: Host,username,password,fingerprint,confirmed} → vollständig registrierter Host mit geprüftem Inventar. Passwort und Bootstrap-Benutzer gehören nicht ins Hostmodell.
Root-Helfer POST /hosts/inspect und /hosts/enroll: Inspect wie oben; Enroll nur {address,port,username,password,fingerprint,confirmed}. Ergebnis enthält address,port,fingerprint,backup_user,restore_user,backup_key_path,restore_key_path,known_hosts_path,inventory. inventory ist ein JSON-Objekt im bestehenden Inventarformat.

## Aufgaben

- [x] SSH-Kern in internal/hostconnect: echte lokale SSH-Servertests zuerst für Inspect ohne Auth, falsche Kennwörter, Schlüsselwechsel ohne Passwortübermittlung, feste TAR-Dateien und Rollen, erneute Einrichtung, Zeitlimit/Abbruch und Fehlerredaktion. Bestehende Installer direkt via host/embed.go und scripts/embed.go einbetten; getrennte root-kontrollierte Schlüssel verwenden, private Schlüssel nicht hochladen. SSH-Passwort und PAM-Keyboard-Interactive unterstützen. Installer/Probe mit Go-Sessions; Ausgabe begrenzen und keine Zugangsdaten weiterreichen.
- [x] Root-Helfer/API: Validierung, gemeinsame Systemaktions-/Setup-Sperren und Administrator-/Demo-/CSRF-Prüfung; Namen/Katalog validieren bevor Remoteänderungen starten. Konflikte und doppelte Hosts verhindern; nur nach beiden erfolgreichen Schlüsselproben speichern. Keine Kennwörter in Katalog, Jobs, Audit oder Fehlern. Tests zuerst für Rolle, ungültige Daten, Konflikte, Erfolg/Fehlschlag und Geheimnisfreiheit.
- [x] Web: neue Hosts automatisch anbinden, manuelle Methode optional; SSH-Benutzer root und Passwort vorne anzeigen, weitere Angaben optional. Inspection und ausdrückliche Identitätsbestätigung; Eingabeänderung verwirft alte Prüfung, busy verhindert doppelte Einreichung, Passwort nach Erfolg/Abbruch löschen. Browsertests zuerst für Phasen, Payload, Fehler, Sperren und bisherigen manuellen Weg.
- [x] Terminal: a öffnet automatische Anbindung; manuelle Anlage erreichbar. Verdecktes Passwort; Inspect → Fingerprintbestätigung → Enroll. Polling/Resize dürfen Entwurf/Identität nicht vermischen. Tests zuerst mit Service/API und PTY; bisherigen manuellen PTY-Test entsprechend ausdrücklich auswählen.
- [ ] Gemeinsame Verifikation: Go race/vet, Python und Browser; isolierter Linux-SSH-Zielcontainer für tatsächlichen Installer und beide eingeschränkten Zugänge, root und sudo, wiederholte Einrichtung, Passwortfreiheit und Fehler. README/Handbuch und Changelog aktualisieren, geprüften Stand und signiertes Release veröffentlichen.

## Reviewfokus

Bekannter Schlüssel wurde zwischen Prüfung und Anmeldung ausgetauscht; fehlendes sudo oder nicht erreichbare APT-Quellen; Passwort wird von einer fehlerhaften Gegenstelle ausgegeben; Abbruch nach teilweise installierten Rollen; paralleler Hostedit/Update während der Anbindung. Keiner dieser Fälle darf eine ungeprüfte Identität oder einen scheinbar fertig registrierten Host erzeugen.

## Lokale Prüfung und Review

Go-Tests einschließlich Race-Prüfung, vet, Python-Suiten (10 Host- und 14 Installerfälle), reale Terminal-PTY-Prüfung und 55 integrierte Browserfälle geprüft. Die neue Root-/sudo-SSH-Integration läuft ausschließlich auf dem isolierten Linux-CI-Runner. Unabhängiger Review korrigiert sudo-Cache-PID-Wechsel, übergroße Wiederanbindungs-Payloads und den Inventar-Antwortpuffer; vorhandene Schlüssel werden semantisch statt nach Zeilenreihenfolge geprüft.

Erster Linux-Prüflauf fand einen Eigentümerkonflikt: Die bestehende Servereinrichtung hält private SSH-Rollenschlüssel als `anker:anker` mit 0600, während der neue Connector zunächst ausschließlich root-eigene Dateien erwartete. Die Schlüssel werden weiterverwendet; nur die private Schlüsseldatei akzeptiert zusätzlich die intern ermittelte Dienst-UID. Verzeichnisse und Vertrauensdatei bleiben root-kontrolliert. Erneuter vollständiger Linux-Nachweis vor Veröffentlichung erforderlich.

Linux-Abnahme nach Korrektur erfolgreich: https://github.com/jahartmann/Anker/actions/runs/37597961722 . Tatsächlich ausgeführt: Web → root-Helfer → OpenSSH auf Debian 13, falsches Passwort ohne Katalog-/Vertrauensänderung, Installation von fehlendem sudo, beide getrennten Schlüsselrollen, Wiederanbindung mit Passwort-sudo, Profile/fremde Schlüssel unverändert, eingeschränkter Befehl und schreibgeschützte Backuprolle, Credentialfreiheit und unveränderte private Schlüsselrechte. 55 Browserfälle, vollständige Go-Race-/vet- und Python-Prüfungen sowie systemd-Update-/Rollback- und ext4-Prüfungen erfolgreich. Reales Proxmox-Labor und physischer Reboot bleiben separate Abnahmen.
