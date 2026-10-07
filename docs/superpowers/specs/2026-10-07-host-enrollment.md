# Automatische Hostanbindung

Anker soll Proxmox-Hosts über Adresse, SSH-Benutzer und ein einmaliges Passwort anbinden. Web und Terminal verwenden denselben Ablauf. root ist der Standard; andere Benutzer benötigen sudo ohne Passwort oder mit dem eingegebenen SSH-Passwort. Die vorhandene manuelle Anbindung bleibt erreichbar.

1. Ohne Passwortauthentifizierung den Ed25519-Hostschlüssel abfragen und bekannte Identität prüfen.
2. Fingerprint vor dem ersten Zugriff anzeigen und bestätigen. Bei geändertem bekannten Schlüssel stoppen; eine Identitätsänderung muss ausdrücklich über die vorhandene Schlüsselprüfung vorgenommen werden.
3. Per gepinntem SSH-Zugang ausschließlich den mitgelieferten Installer/Hosthelfer und die beiden öffentlichen Schlüssel übertragen. Anker verwendet hierfür den bestehenden root-Helfer; Anfragen enthalten keine lokalen Pfade oder Shellbefehle. Fehlendes sudo auf dem Ziel nach Bedarf über dessen APT installieren.
4. Eingeschränkte Konten anker/anker-restore und feste Helferkommandos einrichten. Vorhandene Profile und fremde Schlüssel behalten. Private Schlüssel bleiben zentral.
5. Beide Schlüsselzugänge ohne Passwort mit probe prüfen, verifizierten Hostschlüssel speichern und den Host erst dann registrieren. Gespeichert werden Konfiguration und Inventar, niemals das Einrichtungskennwort. Abbrüche lassen sich wiederholen; bereits installierte sichere Helfer/Konten bleiben erhalten.

Administratorrechte, CSRF-Schutz, Produktionsmodus, begrenzte Körper/Ausgaben und Zeitlimits sind Pflicht. Ein laufendes Update, eine offene Einrichtung oder andere privilegierte Systemaktion verhindert gleichzeitige Anbindung. Fehler melden die Phase ohne Passwort oder fremde SSH-Banner zurückzugeben.
