package anker

import (
	"encoding/json"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BackupReadiness is a set of independent source-bound facts, never a recovery verdict.
type BackupReadiness struct {
	Version      int                  `json:"version"`
	BackupID     string               `json:"backup_id"`
	Items        []ReadinessItem      `json:"items"`
	Coverage     []CoverageRoot       `json:"coverage"`
	Dependencies []DependencyEvidence `json:"dependencies"`
}
type ReadinessItem struct {
	ID         string `json:"id"`
	State      string `json:"state"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	At         string `json:"at,omitempty"`
	NextAction string `json:"next_action,omitempty"`
}
type CoverageRoot struct {
	Path      string `json:"path"`
	State     string `json:"state"`
	Reason    string `json:"reason"`
	Authority string `json:"authority"`
}

func (s *Service) BackupReadiness(id string) (BackupReadiness, error) {
	b, err := s.Backup(id)
	if err != nil {
		return BackupReadiness{}, err
	}
	r := BackupReadiness{Version: 1, BackupID: id, Items: []ReadinessItem{}, Coverage: []CoverageRoot{}, Dependencies: []DependencyEvidence{}}
	add := func(id, state, title, detail, at, action string) {
		r.Items = append(r.Items, ReadinessItem{id, state, title, detail, at, action})
	}
	if b.Status == "damaged" || b.VerificationError != "" {
		add("capture", "failed", "Erfassung", "Sicherung ist als beschädigt markiert.", b.CreatedAt, "Sicherung und Quelle prüfen")
	} else if b.Status == "successful" {
		add("capture", "ok", "Erfassung", "Erfassung wurde abgeschlossen.", b.CreatedAt, "")
	} else {
		add("capture", "missing", "Erfassung", "Erfassung ist unvollständig.", b.CreatedAt, "Warnungen prüfen und erneut sichern")
	}
	m, manifestErr := s.Manifest(id)
	if manifestErr != nil {
		add("manifest", "failed", "Manifest", "Manifest ist nicht lesbar oder stimmt nicht mit dem Katalog überein.", "", "Integrität prüfen")
	} else {
		add("manifest", "ok", "Manifest", "Manifest und Katalogeintrag stimmen überein.", m.CompletedAt, "")
		if m.Status != b.Status {
			r.Items[0].State = "unknown"
			r.Items[0].Detail = "Erfassungsstatus in Manifest und Katalog weicht ab."
			r.Items[0].NextAction = "Katalog und Sicherung prüfen"
		}
	}
	verification := "unknown"
	verificationDetail := "Keine erfolgreiche Integritätsprüfung belegt."
	verificationAction := "Sicherung prüfen"
	if b.VerificationError != "" || b.Status == "damaged" || manifestErr != nil {
		verification = "failed"
		verificationDetail = "Letzter Prüfversuch schlug fehl oder die Quelle ist beschädigt."
	} else if b.VerifiedAt != "" {
		verification = "ok"
		verificationDetail = "Letzter Integritätsprüfversuch war erfolgreich."
		verificationAction = ""
	}
	add("verification", verification, "Integrität", verificationDetail, b.VerifiedAt, verificationAction)
	settings, settingsErr := s.Settings()
	if at, parseErr := time.Parse(time.RFC3339, b.CreatedAt); parseErr == nil && settingsErr == nil && settings.StaleHours > 0 {
		if time.Since(at) > time.Duration(settings.StaleHours)*time.Hour {
			add("age", "stale", "Alter", "Sicherung überschreitet die eingestellte Aktualitätsgrenze.", b.CreatedAt, "Neuen Stand erfassen")
		} else {
			add("age", "ok", "Alter", "Sicherung liegt innerhalb der Aktualitätsgrenze.", b.CreatedAt, "")
		}
	} else {
		add("age", "unknown", "Alter", "Aktualität kann nicht bestimmt werden.", b.CreatedAt, "Zeitstempel und Einstellung prüfen")
	}
	// Separate facts remain open until later evidence producers bind their own proofs.
	add("independent_copy", "unknown", "Unabhängige Kopie", "Für diesen Stand liegt noch kein gebundener Prüfnachweis einer unabhängigen Kopie vor.", "", "Unabhängige Kopie prüfen")
	add("offline_drill", "unknown", "Offline-Probe", "Für diesen Stand liegt kein dokumentierter Offline-Test vor.", "", "Offline-Probe im isolierten Labor dokumentieren")
	if manifestErr != nil {
		for _, key := range []string{"pve_version", "users", "groups", "file_inventory"} {
			add("query."+key, "unknown", "Quellabfrage "+key, "Manifestbeleg nicht verfügbar.", "", "Manifest prüfen")
		}
		add("profile", "unknown", "Hostprofil", "Autorisiertes Hostprofil nicht belegbar.", "", "Quelle erneut sichern")
		add("helper", "unknown", "Helferfähigkeit", "Quellfähigkeiten nicht belegbar.", "", "Quelle erneut sichern")
		add("dependency_scan", "unknown", "Abhängigkeitssuche", "Manifestbeleg nicht verfügbar.", "", "Manifest prüfen")
		return r, nil
	}
	var queries map[string]struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(m.Inventory.Details["command_results"], &queries)
	for _, key := range []string{"pve_version", "users", "groups", "file_inventory"} {
		state := "unknown"
		if q, ok := queries[key]; ok && (q.State == "ok" || q.State == "failed") {
			state = q.State
		}
		if state == "ok" && !queryPayloadPresent(m.Inventory, m.Entries, key) {
			state = "unknown"
		}
		detail := "Quellabfrage nicht belegt."
		action := "Mit aktuellem Helfer erneut sichern"
		if state == "ok" {
			detail = "Quellabfrage erfolgreich belegt."
			action = ""
		}
		if state == "failed" {
			detail = "Quellabfrage schlug fehl."
			action = "Abfrage auf Quelle prüfen und erneut sichern"
		}
		add("query."+key, state, "Quellabfrage "+key, detail, m.Inventory.CapturedAt, action)
	}
	var capabilities struct {
		RestoreProtocol      int  `json:"restore_protocol"`
		Journal              bool `json:"journal"`
		RecoveryReadProtocol int  `json:"recovery_read_protocol"`
	}
	if err := json.Unmarshal(m.Inventory.Details["capabilities"], &capabilities); err == nil && capabilities.RestoreProtocol >= 2 && capabilities.Journal && capabilities.RecoveryReadProtocol >= 1 {
		add("helper", "ok", "Helferfähigkeit", "Erforderliche Quellfähigkeiten wurden bei der Aufnahme gemeldet.", m.Inventory.CapturedAt, "")
	} else {
		add("helper", "unknown", "Helferfähigkeit", "Quellfähigkeiten sind nicht vollständig belegt.", m.Inventory.CapturedAt, "Mit aktuellem Helfer erneut sichern")
	}
	if m.Profile == nil {
		add("profile", "unknown", "Hostprofil", "Tatsächliches autorisiertes Sammelprofil wurde nicht gespeichert.", "", "Mit aktuellem Helfer erneut sichern")
		add("dependency_scan", "unknown", "Abhängigkeitssuche", "Strukturierter Quellscan wurde nicht gespeichert.", "", "Mit aktuellem Helfer erneut sichern")
	} else {
		if m.Profile.Version != 1 || len(m.Profile.Roots) == 0 {
			add("profile", "unknown", "Hostprofil", "Gespeichertes Sammelprofil ist unvollständig oder unbekannt.", m.CompletedAt, "Mit aktuellem Helfer erneut sichern")
		} else {
			add("profile", "ok", "Hostprofil", "Autorisierte Sammelwurzeln stammen aus dieser Aufnahme.", m.CompletedAt, "")
			for _, root := range m.Profile.Roots {
				r.Coverage = append(r.Coverage, coverageFor(root, "collector", m.Entries))
			}
		}
		if m.Profile.Version != 1 || m.Profile.DependencyEntryLimit <= 0 || m.Profile.DependencyFileLimit <= 0 || m.Profile.DependencyReferenceLimit <= 0 || !m.Profile.DependencyScanComplete {
			add("dependency_scan", "unknown", "Abhängigkeitssuche", "Grenze der Abhängigkeitssuche erreicht oder Scan unvollständig.", m.CompletedAt, "Abhängigkeiten manuell prüfen")
		} else {
			add("dependency_scan", "ok", "Abhängigkeitssuche", "Begrenzte Abhängigkeitssuche abgeschlossen.", m.CompletedAt, "")
		}
	}
	for _, required := range m.RequiredPaths {
		r.Coverage = append(r.Coverage, coverageFor(required, "required", m.Entries))
	}
	for _, dep := range m.Dependencies {
		r.Dependencies = append(r.Dependencies, publicDependency(dep))
	}
	entryByPath := map[string]Entry{}
	for _, entry := range m.Entries {
		entryByPath[entry.Path] = entry
	}
	for _, entry := range m.Entries {
		if entry.Type != "symlink" {
			continue
		}
		target := entry.Link
		if !strings.HasPrefix(target, "/") {
			target = "/" + path.Clean(path.Join(path.Dir(entry.Path), target))
		}
		rel := strings.TrimPrefix(path.Clean(target), "/")
		state, reason := "missing", "link_target_uncaptured"
		if ValidPath(rel) != nil {
			state, reason = "unknown", "link_target_unresolved"
		} else if destination, ok := entryByPath[rel]; ok {
			if destination.Type == "symlink" {
				state, reason = "unknown", "link_target_chain"
			} else {
				state, reason = "captured", "link_target_captured"
			}
		}
		r.Dependencies = append(r.Dependencies, publicDependency(DependencyEvidence{Path: target, Source: entry.Path, State: state, Reason: reason}))
	}
	sort.Slice(r.Dependencies, func(i, j int) bool {
		if r.Dependencies[i].Source == r.Dependencies[j].Source {
			return r.Dependencies[i].Path < r.Dependencies[j].Path
		}
		return r.Dependencies[i].Source < r.Dependencies[j].Source
	})
	return r, nil
}

func queryPayloadPresent(inv Inventory, entries []Entry, key string) bool {
	if key == "pve_version" {
		return inv.PVEVersion != ""
	}
	if key == "users" || key == "groups" {
		var names map[string]string
		if json.Unmarshal(inv.Details[key], &names) != nil || len(names) == 0 {
			return false
		}
		for id, name := range names {
			numeric, err := strconv.ParseUint(id, 10, 32)
			if err != nil || strconv.FormatUint(numeric, 10) != id || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\x00\n\r") {
				return false
			}
		}
		for _, entry := range entries {
			owner := entry.UID
			if key == "groups" {
				owner = entry.GID
			}
			if owner < 0 || names[strconv.Itoa(owner)] == "" {
				return false
			}
		}
		return true
	}
	if key != "file_inventory" {
		return false
	}
	var hashes map[string]string
	var metadata map[string]json.RawMessage
	if json.Unmarshal(inv.Details["file_hashes"], &hashes) != nil || len(hashes) == 0 || json.Unmarshal(inv.Details["file_metadata"], &metadata) != nil || len(metadata) == 0 {
		return false
	}
	for file, hash := range hashes {
		if ValidPath(file) != nil || hash == "" {
			return false
		}
		if _, ok := metadata[file]; !ok {
			return false
		}
	}
	for file, raw := range metadata {
		if ValidPath(file) != nil {
			return false
		}
		var record struct {
			Type   string            `json:"type"`
			Mode   *uint32           `json:"mode"`
			UID    *int              `json:"uid"`
			GID    *int              `json:"gid"`
			XAttrs map[string]string `json:"xattrs"`
			Link   *string           `json:"link"`
		}
		if json.Unmarshal(raw, &record) != nil || record.Mode == nil || record.UID == nil || record.GID == nil || record.XAttrs == nil || *record.Mode > 07777 || *record.UID < 0 || *record.GID < 0 {
			return false
		}
		if record.Type != "file" && record.Type != "symlink" {
			return false
		}
		if record.Type == "symlink" && (record.Link == nil || *record.Link == "") {
			return false
		}
		hash, ok := hashes[file]
		if !ok || record.Type == "file" && !validSHA256(hash) || record.Type == "symlink" && hash != "symlink:"+*record.Link {
			return false
		}
	}
	for _, entry := range entries {
		if entry.Type == "directory" || !(strings.HasPrefix(entry.Path, "etc/") || strings.HasPrefix(entry.Path, "usr/local/")) {
			continue
		}
		hash, ok := hashes[entry.Path]
		if !ok {
			return false
		}
		var record struct {
			Type string `json:"type"`
			Link string `json:"link"`
		}
		if json.Unmarshal(metadata[entry.Path], &record) != nil || record.Type != entry.Type {
			return false
		}
		if entry.Type == "file" && !validSHA256(hash) {
			return false
		}
		if entry.Type == "symlink" && (hash != "symlink:"+entry.Link || record.Link != entry.Link) {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func coverageFor(root, authority string, entries []Entry) CoverageRoot {
	coverage := CoverageRoot{Path: root, State: "missing", Reason: "no_captured_entry", Authority: authority}
	rel := strings.TrimSuffix(strings.TrimPrefix(root, "/"), "/")
	for _, entry := range entries {
		if entry.Path == rel || strings.HasPrefix(entry.Path, rel+"/") {
			coverage.State = "captured"
			coverage.Reason = "entry_in_manifest"
			break
		}
	}
	return coverage
}

func publicDependency(dep DependencyEvidence) DependencyEvidence {
	switch dep.State {
	case "captured", "missing", "unknown":
	default:
		dep.State = "unknown"
	}
	switch dep.Reason {
	case "captured", "outside_capture", "unresolved_reference", "scan_limit", "link_target_uncaptured", "link_target_unresolved", "link_target_chain", "link_target_captured":
	default:
		dep.Reason = "unspecified"
	}
	if !strings.HasPrefix(dep.Path, "/") || publicSecretText(dep.Path) {
		dep.Path = "[Verdeckter Pfad]"
	}
	if publicSecretText(dep.Source) {
		dep.Source = "[Verdeckte Quelle]"
	}
	return dep
}
