# Speicher für Anker

Der Bereich Einstellungen → Speicher zeigt ausschließlich die Maschine, in der Anker läuft, und deren eingebundene Laufwerke. Andere Proxmox-Gäste werden nicht überwacht oder verändert. Die produktive Installation beginnt ohne erfundene Messwerte.

## Anzeige und Verlauf

Linux-Mounts und Blockgeräte werden lesend ermittelt. Virtuelle Systemdateisysteme und einzelne Dateibindmounts werden ausgeblendet; System und Backup-Ablage bleiben auch in Containern sichtbar. Bindmounts desselben Dateisystems zählen einmal. Pro Volume werden Kapazität, tatsächlich belegter Platz, verfügbarer Platz, reservierter Platz, Inodes, Dateisystem, Gerät und zugehörige Mounts angezeigt. Fehler werden je Volume sichtbar, statt unbekannte Werte als null Prozent auszugeben. Container/VM-Erkennung bleibt best effort.

Eine echte Messung pro Stunde bleibt bis zu 90 Tage im bestehenden Katalog. Prognosen verwenden die letzten 14 Tage derselben Kapazität, mindestens 24 Messungen und drei Tage Messdauer. Wachstum wird anhand täglicher Medianwerte geschätzt. Unruhige, alte, fallende und zu kurze Reihen erhalten keine erfundene Voll-Datum-Angabe. Kapazitäts- oder Gerätewechsel beginnen eine neue Reihe. Die Anzeige erklärt, dass Aufbewahrung und andere Programme die Prognose beeinflussen.

## Erweiterung

Ein Assistent zeigt zuerst den erkannten Aufbau und passende Schritte. Bei LXC ist die Volumenerweiterung eine Aufgabe des Proxmox-Hosts; der Assistent erzeugt aus bewusst eingegebener CT-ID, rootfs/mp-Nummer und zusätzlicher Größe einen kopierbaren pct-Befehl. VM-, LVM-, Netzwerk- und neue Laufwerke erhalten prüfbare Hinweise. Neue Laufwerke werden niemals automatisch formatiert oder vorhandene Daten gelöscht.

Bereits vergrößerte ext4-/XFS-Blockgeräte für System bzw. Anker-Daten können nach aktueller Prüfung und ausdrücklicher Bestätigung des Mounts übernommen werden. Die root-eigene Verwaltung wählt ausschließlich das Systemdateisystem, /srv/anker und echte Linux-Mounts unter /srv/anker aus der laufenden Maschine; Webanfragen enthalten keine Gerätepfade oder Shellbefehle. Datei-, Geräte-, Kapazitäts- und Planidentität werden unmittelbar vor dem Eingriff erneut geprüft. Container, unbekannte Umgebungen, Schreibschutz, unbekannte Geräte und unveränderter Platz sperren die Aktion. Ausschließlich Wachstum, keine Partitionstabellen-/LVM-/Formatierungsänderung.

Ein dauerhafter Operationszustand zeigt Erfolg, Fehler oder Unterbrechung nach Neustart. Einrichtung, Update und Zertifikatsänderung schließen die Aktion aus. Der normale Webdienst bleibt unprivilegiert. CLI und Web verwenden dieselben administrativen Schnittstellen. Demoaktionen sind gesperrt. Die Dokumentation grenzt die echte automatische Dateisystemerweiterung von den angeleiteten Host-/Partition-/LVM-Schritten ab.

## Nachweise

Tests prüfen Mount-Deduplikation, Sonderzeichen, Verbrauch/Inodes, Kapazitätswechsel, kurze/unruhige/alte Reihen, Rechte/Demo, gefälschte Pläne, Schreibschutz/Container, Konkurrenz, Operationspersistenz und Browserfehler. Linux-CI prüft echte ext4-Vergrößerung auf einem ausschließlich für den Test erstellten Loopgerät.
