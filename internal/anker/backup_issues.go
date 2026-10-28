package anker

import "strings"

// BackupIssue separates useful explanations from untrusted remote diagnostics.
// Original warnings stay in the manifest; public responses contain safe actions
// even when the original warning must be hidden by the credential filter.
type BackupIssue struct {
	Code          string `json:"code"`
	Title         string `json:"title"`
	Detail        string `json:"detail"`
	Action        string `json:"action"`
	Path          string `json:"path,omitempty"`
	PathProtected bool   `json:"path_protected,omitempty"`
}

func backupIssues(warnings []string) []BackupIssue {
	out := []BackupIssue{}
	for _, warning := range warnings {
		lower := strings.ToLower(warning)
		i := BackupIssue{Code: "capture_warning", Title: "Erfassung benötigt eine Prüfung", Detail: "Ein Teil der Konfiguration konnte nicht vollständig erfasst werden.", Action: "Host prüfen und danach eine neue Sicherung starten. Vorhandene Sicherungen bleiben unverändert."}
		candidate := ""
		switch {
		case strings.HasPrefix(warning, "Referenced config/secret is not captured: "), strings.HasPrefix(warning, "Referenced hookscript is not captured: "):
			i.Code, i.Title = "dependency_not_captured", "Verwendete Datei nicht in der Sicherung"
			i.Detail = "Eine Konfiguration verweist auf eine weitere Datei. Diese wurde nicht mitgesichert; ob sie auf dem Host existiert, ist damit noch nicht geklärt."
			i.Action = "Verweis auf dem Host prüfen. Wird die Datei benötigt, im Hostprofil /etc/anker-host.json aufnehmen und erneut sichern. Einen veralteten Verweis an seiner Quelle korrigieren."
			candidate = strings.SplitN(strings.SplitN(warning, ": ", 2)[1], " (", 2)[0]
		case strings.Contains(lower, "required path missing"), strings.Contains(lower, "pflichtpfad"), strings.Contains(lower, "required pbs configuration directory missing"):
			i.Code, i.Title = "required_path_missing", "Pflichtpfad fehlt"
			i.Detail = "Ein als erforderlich eingestufter Pfad wurde nicht erfasst. Ein falscher Pfad oder ein abweichendes Hostprofil kann die Ursache sein."
			i.Action = "Pfad und Hostprofil vergleichen. Eine veraltete Pflichtpfadangabe in den Host-Einstellungen korrigieren; tatsächlich benötigte Dateien auf dem Host wiederherstellen und erneut sichern."
			if parts := strings.SplitN(warning, ": ", 2); len(parts) == 2 {
				candidate = parts[1]
			}
		case strings.Contains(lower, "invalid path"), strings.Contains(lower, "invalid host profile"), strings.Contains(lower, "ungültiger relativer pfad"):
			i.Code, i.Title = "invalid_path", "Pfad konnte nicht verarbeitet werden"
			i.Detail = "Die Pfadangabe entspricht nicht dem erlaubten Format. Das ist kein Nachweis, dass die Datei fehlt."
			switch {
			case strings.Contains(lower, "empty or non-text"):
				i.Detail = "Eine Pfadangabe ist leer oder kein Text. Das ist kein Nachweis, dass eine Datei fehlt."
			case strings.Contains(lower, "expected a relative"):
				i.Detail = "An einer internen Stelle wird ein relativer Pfad erwartet, es wurde aber ein absoluter Pfad geliefert. Das ist kein Nachweis, dass die Datei fehlt."
			case strings.Contains(lower, "backslash"):
				i.Detail = "Eine Pfadangabe enthält einen Backslash. Für Linux-Pfade wird der Schrägstrich / verwendet."
			case strings.Contains(lower, "dot or parent"):
				i.Detail = "Eine Pfadangabe enthält leere Bestandteile, einen Punkt oder einen übergeordneten Pfad. Sie kann in diesem Format nicht verarbeitet werden."
			}
			i.Action = "Hostprofil und Pflichtpfade auf ungültige Bestandteile prüfen. Hosthelfer aktualisieren und erneut sichern; bleibt der Fehler bestehen, die technischen Angaben des Auftrags prüfen."
		case (strings.Contains(lower, "permission denied") || strings.Contains(lower, "nicht berechtigt")) && !strings.Contains(lower, "metadata capture incomplete") && !strings.Contains(lower, "config.db capture failed"):
			i.Code, i.Title = "permission_denied", "Zugriff wurde verweigert"
			i.Detail = "Bei einem Zugriff fehlte die erforderliche Berechtigung. Die Meldung allein unterscheidet nicht zwischen Anmeldung und Dateizugriff."
			if strings.HasPrefix(warning, "Cannot capture ") {
				i.Title = "Datei konnte nicht gelesen werden"
				i.Detail = "Dem Sicherungszugang fehlen Leserechte für einen Teil der Konfiguration."
			}
			i.Action = "Hostverbindung prüfen und gegebenenfalls über Verbindung einrichten aktualisieren. Dateirechte auf dem Host gezielt prüfen und anschließend erneut sichern."
		case strings.Contains(lower, "config.db capture failed"):
			i.Code, i.Title = "pve_database", "Proxmox-Konfigurationsdatenbank nicht erfasst"
			i.Detail = "Die lokale PVE-Konfigurationsdatenbank konnte nicht vollständig gelesen oder dekodiert werden."
			i.Action = "PVE-Dienste und /var/lib/pve-cluster/config.db auf dem Host prüfen. Hosthelfer aktualisieren und erneut sichern."
		case strings.Contains(lower, "pbs inventory query failed"), strings.Contains(lower, "keine proxmox-version erkannt"):
			i.Code, i.Title = "inventory_query_failed", "Hostangaben konnten nicht vollständig abgefragt werden"
			i.Detail = "Mindestens eine Inventarabfrage ist fehlgeschlagen. Das allein belegt keinen Verlust gesicherter Dateiinhalte."
			i.Action = "Hostverbindung, installierte PVE-/PBS-Werkzeuge und Dienstzustand prüfen. Danach mit aktuellem Hosthelfer erneut sichern."
		case strings.Contains(lower, "symlink parent or destination"):
			i.Code, i.Title = "symlink_path", "Pfad führt über einen symbolischen Link"
			i.Detail = "Der konfigurierte Pfad wird aus Sicherheitsgründen nicht über einen symbolischen Link verfolgt."
			i.Action = "Den tatsächlich benötigten Konfigurationspfad im Hostprofil ausdrücklich angeben. Datastore-Inhalte bleiben außerhalb des Sicherungsprofils."
		case strings.Contains(lower, "file exceeds 64 mib"):
			i.Code, i.Title = "file_size_limit", "Datei überschreitet die Erfassungsgrenze"
			i.Detail = "Eine Datei ist größer als die für Konfigurationsdateien erlaubten 64 MiB."
			i.Action = "Prüfen, ob es sich um Konfiguration oder Anwendungsdaten handelt. Daten separat sichern und das Konfigurationsprofil entsprechend korrigieren."
		case strings.Contains(lower, "file changing during capture"):
			i.Code, i.Title = "file_changing", "Datei hat sich während der Erfassung verändert"
			i.Detail = "Trotz mehrerer Leseversuche war kein stabiler Dateistand verfügbar."
			i.Action = "Eine ruhige Phase abwarten oder den schreibenden Vorgang kontrolliert beenden und eine neue Sicherung starten."
		case strings.Contains(lower, "unsupported file type"):
			i.Code, i.Title = "unsupported_file_type", "Dateityp wird nicht als Konfiguration erfasst"
			i.Detail = "Ein Eintrag ist keine unterstützte Konfigurationsdatei, kein Verzeichnis oder kein gespeicherter Link."
			i.Action = "Prüfen, ob der Eintrag benötigt wird oder nur zur Laufzeit entsteht. Nicht benötigte Laufzeitpfade aus dem Konfigurationsprofil nehmen."
		case strings.Contains(lower, "metadata capture incomplete"):
			i.Code, i.Title = "metadata_incomplete", "Dateieigenschaften unvollständig"
			i.Detail = "Nicht alle zusätzlichen Dateieigenschaften konnten erfasst werden. Das betrifft die belegbare Wiederherstellung der Eigenschaften."
			i.Action = "Unterstützung für Dateirechte und erweiterte Attribute auf dem Host prüfen und erneut sichern."
		case strings.Contains(lower, "dependency scan incomplete"):
			i.Code, i.Title = "dependency_scan_limit", "Abhängigkeitssuche hat eine Grenze erreicht"
			i.Detail = "Die begrenzte Suche konnte nicht alle Konfigurationsverweise prüfen. Weitere Abhängigkeiten können fehlen."
			i.Action = "Sicherungsprofil auf Konfiguration begrenzen und benötigte zusätzliche Pfade ausdrücklich aufnehmen."
		case strings.Contains(lower, "cannot capture"):
			i.Code, i.Title = "file_not_captured", "Datei oder Verzeichnis nicht erfasst"
			i.Detail = "Ein Teil der Konfiguration konnte nicht gelesen oder in die Sicherung übernommen werden."
			i.Action = "Existenz, Leserechte und Änderungen während der Erfassung auf dem Host prüfen und erneut sichern."
			candidate = strings.SplitN(strings.TrimPrefix(warning, "Cannot capture "), ": ", 2)[0]
		case strings.Contains(lower, "[verdeckter inhalt]"):
			i.Code, i.Title = "protected_diagnostic", "Technischer Hinweis enthält geschützte Angaben"
			i.Detail = "Der alte Hinweis lässt sich ohne Offenlegung geschützter Angaben nicht genauer zuordnen. Verdeckter Inhalt allein bedeutet nicht, dass eine Datei fehlt."
			i.Action = "Hosthelfer aktualisieren und eine neue Sicherung erstellen. Geschützte Inhalte bleiben geschützt."
		}
		if candidate == "" && strings.HasPrefix(warning, "Cannot capture ") {
			candidate = strings.SplitN(strings.TrimPrefix(warning, "Cannot capture "), ": ", 2)[0]
		}
		if candidate != "" {
			rel := strings.TrimPrefix(candidate, "/")
			// A pathname is metadata, not file content. The existing file browser
			// exposes names too; unknown/secret file contents remain protected.
			if len(candidate) <= 4096 && ValidPath(rel) == nil && !publicSecretText(candidate) && !strings.ContainsAny(candidate, "\r\n\t") {
				i.Path = "/" + rel
			} else {
				i.PathProtected = true
			}
		}
		out = append(out, i)
	}
	return out
}
