# Sicherheitsmeldungen

Sicherheitslücken bitte über GitHubs „Report a vulnerability“ im Security-Bereich des Repositorys melden. Private vulnerability reporting muss der Repository-Eigentümer vor der Veröffentlichung aktivieren. Solange das nicht eingerichtet ist, keine Zugangsdaten oder ausnutzbaren Details in öffentliche Issues stellen.

Für eine Meldung reichen betroffene Version, notwendige Zugriffsrechte, Schritte zum Nachstellen und die erwartete Auswirkung. Hostbackups enthalten sensible Daten; bitte ausschließlich künstliche Beispieldaten verwenden.

Unterstützt wird der aktuelle stabile Release. Alte Releases erhalten keine zugesagten Sicherheitsupdates. Die Software ist noch in der Entwicklung; vollständige Host- und Clusterwiederherstellungen brauchen einen eigenen Labortest.

Release-Signierschlüssel gehören nicht auf den Anker-Server und nicht ins Git-Repository. Der Server benötigt nur den öffentlichen Schlüssel. Einen kompromittierten Signierschlüssel außer Betrieb nehmen und den neuen öffentlichen Schlüssel separat auf den Servern verteilen; ein gewöhnlicher OTA-Release darf den Vertrauensschlüssel nicht selbst ersetzen.
