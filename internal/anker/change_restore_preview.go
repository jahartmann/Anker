package anker

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

type ChangeRestorePoint struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	ManifestSHA string `json:"manifest_sha"`
}
type ChangeRestoreValue struct {
	Setting TrackedSetting     `json:"setting"`
	Before  SettingObservation `json:"before"`
	After   SettingObservation `json:"after"`
}
type ChangeRestoreFile struct {
	Path       string         `json:"path"`
	Action     string         `json:"action"`
	Comparison FileComparison `json:"comparison"`
}
type ChangeRestorePreview struct {
	Basis      string               `json:"basis"`
	Source     ChangeRestorePoint   `json:"source"`
	Baseline   ChangeRestorePoint   `json:"baseline"`
	Selected   ChangeRestoreValue   `json:"selected"`
	Additional []ChangeRestoreValue `json:"additional"`
	Files      []ChangeRestoreFile  `json:"files"`
}

// A purely historical projection. It neither creates a plan nor reads/writes a
// remote target, and it never authorizes a parameter-only or live rollback.
func (s *Service) changeRestorePreviewHTTP(u User, w http.ResponseWriter, r *http.Request) (bool, error) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "backups" || parts[3] != "change-restore-preview" {
		return false, nil
	}
	if r.Method != "GET" {
		return true, fail(405, "GET erforderlich")
	}
	if err := require(u, "read"); err != nil {
		return true, err
	}
	sourceID, baselineID := parts[2], r.URL.Query().Get("baseline")
	if !validID(sourceID) || !validID(baselineID) || sourceID == baselineID {
		return true, fail(400, "Zwei unterschiedliche Sicherungsstände erforderlich")
	}
	source, err := s.Backup(sourceID)
	if err != nil {
		return true, fail(404, "Quellsicherung nicht gefunden")
	}
	baseline, err := s.Backup(baselineID)
	if err != nil {
		return true, fail(404, "Vergleichssicherung nicht gefunden")
	}
	if source.HostID != baseline.HostID {
		return true, fail(400, "Vorschau nur innerhalb derselben Node möglich")
	}
	oldTime, e1 := time.Parse(time.RFC3339Nano, source.CreatedAt)
	newTime, e2 := time.Parse(time.RFC3339Nano, baseline.CreatedAt)
	if e1 != nil || e2 != nil || !oldTime.Before(newTime) || newTime.After(time.Now().Add(5*time.Minute)) {
		return true, fail(409, "Zeitliche Reihenfolge der Änderung ist nicht belastbar")
	}
	if source.Status != "successful" || baseline.Status != "successful" || source.VerificationError != "" || baseline.VerificationError != "" {
		return true, fail(409, "Beide Stände müssen vollständig und ohne bekannte Integritätsfehler sein")
	}
	var setting TrackedSetting
	for _, candidate := range trackedSettings {
		if candidate.ID == r.URL.Query().Get("setting") {
			setting = candidate
		}
	}
	if setting.ID == "" {
		return true, fail(400, "Einstellung wird nicht unterstützt")
	}
	desired := s.observeSetting(source, setting)
	observed := s.observeSetting(baseline, setting)
	if desired.State != "value" || observed.State != "value" {
		return true, fail(409, "Die Einstellungswerte sind nicht eindeutig lesbar. Nachweise erneut prüfen.")
	}
	if desired.Value == observed.Value {
		return true, fail(409, "Zwischen diesen Ständen liegt kein unterschiedlicher Einstellungswert vor")
	}
	if desired.Value != r.URL.Query().Get("expected_source") || observed.Value != r.URL.Query().Get("expected_baseline") {
		return true, fail(409, "Die Werte passen nicht mehr zur ausgewählten Änderung. Änderung neu laden.")
	}
	out := ChangeRestorePreview{Basis: "saved-backup", Source: ChangeRestorePoint{source.ID, source.CreatedAt, source.ManifestSHA}, Baseline: ChangeRestorePoint{baseline.ID, baseline.CreatedAt, baseline.ManifestSHA}, Selected: ChangeRestoreValue{setting, observed, desired}, Additional: []ChangeRestoreValue{}, Files: []ChangeRestoreFile{}}
	paths := map[string]bool{}
	for _, p := range append(append([]string{}, desired.Sources...), observed.Sources...) {
		paths[p] = true
	}
	if len(paths) > 12 {
		return true, fail(409, "Zu viele Quelldateien für eine kompakte Änderungsvorschau. Die Wiederherstellung separat prüfen.")
	}
	ordered := []string{}
	for p := range paths {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)
	for _, p := range ordered {
		if err := r.Context().Err(); err != nil {
			return true, err
		}
		comparison, err := s.CompareBackupFile(baseline.ID, source.ID, p, false)
		if err != nil || comparison.Masked || comparison.State == "masked" || comparison.State == "unavailable" || comparison.State == "binary" || comparison.State == "too_large" {
			return true, fail(409, "Eine betroffene Datei ist nicht sicher vergleichbar. Keine vollständige Vorschau verfügbar.")
		}
		action := "replace"
		if comparison.State == "different" && !comparison.ContentChanged {
			action = "keep"
			for _, m := range comparison.MetadataChanges {
				if m.Field != "mtime" {
					action = "metadata"
				}
			}
		}
		switch comparison.State {
		case "identical":
			action = "keep"
		case "missing_before":
			action = "create"
		case "missing_after":
			action = "remove"
		}
		out.Files = append(out.Files, ChangeRestoreFile{p, action, comparison})
	}
	// Only describe collateral setting values when all of their declarations are
	// within the file scope. Other text changes remain visible in each file diff.
	for _, candidate := range trackedSettings {
		if candidate.ID == setting.ID {
			continue
		}
		if err := r.Context().Err(); err != nil {
			return true, err
		}
		old := s.observeSetting(source, candidate)
		recent := s.observeSetting(baseline, candidate)
		if old.State != "value" || recent.State != "value" || old.Value == recent.Value {
			continue
		}
		contained := true
		for _, p := range append(append([]string{}, old.Sources...), recent.Sources...) {
			if !paths[p] {
				contained = false
			}
		}
		if contained {
			out.Additional = append(out.Additional, ChangeRestoreValue{candidate, recent, old})
		}
	}
	// Reindexing or verification must not quietly mix a new source with earlier
	// evidence. No executable plan or token is issued by this endpoint.
	for _, original := range []Backup{source, baseline} {
		current, err := s.Backup(original.ID)
		if err != nil || current.ManifestSHA != original.ManifestSHA || current.HostID != original.HostID || current.CreatedAt != original.CreatedAt || current.Status != "successful" || current.VerificationError != "" {
			return true, fail(409, "Ein Sicherungsstand hat sich während der Vorschau geändert. Erneut prüfen.")
		}
	}
	return true, jsonOut(w, out)
}
