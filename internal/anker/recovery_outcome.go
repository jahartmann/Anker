package anker

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"unicode/utf8"
)

type RecoveryOutcomeFile struct {
	Path          string          `json:"path"`
	State         string          `json:"state"`
	Expected      string          `json:"expected"`
	Observed      string          `json:"observed"`
	ExpectedSHA   string          `json:"expected_sha,omitempty"`
	ObservedSHA   string          `json:"observed_sha,omitempty"`
	ContentState  string          `json:"content_state"`
	MetadataState string          `json:"metadata_state"`
	Reason        string          `json:"reason"`
	Comparison    *FileComparison `json:"comparison,omitempty"`
}
type RecoveryOutcomeSetting struct {
	Setting       TrackedSetting `json:"setting"`
	State         string         `json:"state"`
	ExpectedValue string         `json:"expected_value,omitempty"`
	ObservedValue string         `json:"observed_value,omitempty"`
	ObservedState string         `json:"observed_state"`
	Sources       []string       `json:"sources"`
	Reason        string         `json:"reason"`
}
type RecoveryOutcome struct {
	ID                string                   `json:"id"`
	PlanID            string                   `json:"plan_id"`
	StartedAt         string                   `json:"started_at"`
	FinishedAt        string                   `json:"finished_at,omitempty"`
	Direction         string                   `json:"direction"`
	Origin            string                   `json:"origin"`
	State             string                   `json:"state"`
	Reason            string                   `json:"reason"`
	SourceManifestSHA string                   `json:"source_manifest_sha,omitempty"`
	SnapshotSHA       string                   `json:"snapshot_sha"`
	Files             []RecoveryOutcomeFile    `json:"files"`
	Settings          []RecoveryOutcomeSetting `json:"settings"`
}

func outcomeDirection(p Plan) string {
	if p.State == "rolled_back" {
		return "rollback"
	}
	return "apply"
}
func outcomeEligible(p Plan) bool {
	return p.State == "applied" || p.State == "checks_pending" || p.State == "demo_applied" || p.State == "rolled_back"
}

// Explicit refresh is read-only at the host. It creates no recovery job or plan.
func (s *Service) VerifyRecoveryOutcome(ctx context.Context, id string) (Plan, error) {
	lock := s.planLock(id)
	lock.Lock()
	defer lock.Unlock()
	p, err := s.Plan(id)
	if err != nil {
		return p, err
	}
	if !outcomeEligible(p) {
		return p, errors.New("Abgeschlossene Dateiübernahme oder Rücksetzung erforderlich")
	}
	if err = s.acquire(p.TargetID); err != nil {
		return p, err
	}
	defer s.release(p.TargetID)
	return s.verifyRecoveryOutcomeLocked(ctx, p)
}

// Caller holds the plan and target locks, including when called directly after apply/rollback.
func (s *Service) verifyRecoveryOutcomeLocked(ctx context.Context, p Plan) (Plan, error) {
	if len(p.OutcomeChecks) >= 20 {
		p.OutcomeChecks = append([]RecoveryOutcome{}, p.OutcomeChecks[len(p.OutcomeChecks)-19:]...)
	}
	a := RecoveryOutcome{ID: ID(), PlanID: p.ID, StartedAt: now(), Direction: outcomeDirection(p), Origin: "host-read", State: "running", Reason: "Aktuelle Zielprüfung noch nicht abgeschlossen", SnapshotSHA: p.TargetSnapshotSHA, Files: []RecoveryOutcomeFile{}, Settings: []RecoveryOutcomeSetting{}}
	if s.Demo {
		a.Origin = "demo-read"
	}
	p.OutcomeChecks = append(p.OutcomeChecks, a)
	save := func() error {
		p.OutcomeChecks[len(p.OutcomeChecks)-1] = a
		return s.Store.Put("recovery-outcomes", p.ID, p.OutcomeChecks)
	}
	if err := save(); err != nil {
		return p, err
	}
	run := func() error {
		if !outcomeEligible(p) || p.SchemaVersion != 2 {
			return errors.New("Kein abgeschlossener Plan mit prüfbarem Zielnachweis")
		}
		expectedResult := "applied"
		if a.Direction == "rollback" {
			expectedResult = "rolled_back"
		}
		if p.Result == nil || p.Result.OperationID != p.ID || p.Result.State != expectedResult {
			return errors.New("Abgeschlossenes passendes Hostjournal nicht bestätigt")
		}
		if err := validateHostResult(p, *p.Result); err != nil {
			return errors.New("Hostjournal deckt die Dateiaktionen nicht vollständig ab")
		}
		release, err := s.readableLease(p.BackupID)
		if err != nil {
			return err
		}
		defer release()
		check := func() (RecoverySnapshot, error) {
			if err := s.checkBackupUnlocked(p.BackupID); err != nil {
				return RecoverySnapshot{}, err
			}
			if err := s.verifyPlanFiles(p); err != nil {
				return RecoverySnapshot{}, err
			}
			snap, err := s.loadPlanSnapshot(p)
			if err != nil {
				return snap, err
			}
			h, err := s.Host(p.TargetID)
			if err != nil {
				return snap, err
			}
			if err = s.checkRecoveryTarget(p, h); err != nil {
				return snap, err
			}
			return snap, nil
		}
		snap, err := check()
		if err != nil {
			return err
		}
		a.SourceManifestSHA = snap.SourceManifestSHA
		reader, ok := s.Collector.(RecoveryReader)
		if !ok {
			return errors.New("Hostzugang unterstützt keine aktuelle Dateiprüfung")
		}
		manifest, err := s.Manifest(p.BackupID)
		if err != nil {
			return err
		}
		expected := map[string]RecoveryReadFile{}
		entries := map[string]Entry{}
		for _, e := range manifest.Entries {
			entries[e.Path] = e
		}
		before := map[string]RecoveryReadFile{}
		for _, f := range snap.Files {
			before[f.Path] = f
		}
		for _, step := range p.Steps {
			if step.Action != "apply" && step.Action != "remove" {
				continue
			}
			if a.Direction == "rollback" {
				f, ok := before[step.Path]
				if !ok {
					return errors.New("Ursprünglicher Zielnachweis fehlt")
				}
				expected[step.Path] = f
				continue
			}
			f := RecoveryReadFile{Path: step.Path, State: "missing"}
			if step.Action == "apply" {
				e, ok := entries[step.Path]
				if !ok {
					return errors.New("Quellmetadaten fehlen")
				}
				full, err := safeJoin(filepath.Join(s.Root, "plans", p.ID, "prepared-files"), step.Path)
				if err != nil {
					return err
				}
				data, err := readSnapshotBytes(full, 1<<20)
				if err != nil || Hash(data) != step.PreparedSHA {
					return errors.New("Vorbereiteter Sollinhalt nicht bestätigt")
				}
				e.SHA256 = step.PreparedSHA
				e.Size = int64(len(data))
				f.State = "read"
				f.Content = data
				f.SHA256 = e.SHA256
				f.Metadata = &e
			}
			expected[step.Path] = f
		}
		if len(expected) == 0 || len(expected) > 128 {
			return errors.New("Prüfung benötigt 1 bis 128 freigegebene Dateiaktionen")
		}
		h, err := s.Host(p.TargetID)
		if err != nil {
			return err
		}
		inv, err := s.Collector.Probe(ctx, h)
		if err != nil {
			return errors.New("Aktueller Zielzustand nicht erreichbar")
		}
		baseID, _ := verificationBaseline(snap.Target)
		currentID, _ := verificationBaseline(inv)
		if baseID != "" && (currentID == "" || baseID != currentID) {
			return errors.New("Aktuelle Zielidentität nicht bestätigt")
		}
		paths := outcomePaths(expected, inv)
		if len(paths) > 128 {
			return errors.New("Datei-/Einstellungsumfang überschreitet das Prüflimit")
		}
		result, err := reader.ReadRecoveryFiles(ctx, h, paths, []string{})
		if err != nil {
			return errors.New("Aktuelle Dateien konnten nicht vollständig abgefragt werden")
		}
		if err = validateRecoveryRead(result, paths, []string{}); err != nil {
			return errors.New("Unvollständige oder ungültige Dateiprüfantwort")
		}
		if err = validateSnapshotProbe(inv, result); err != nil {
			return errors.New("Dateien während der Prüfung verändert oder Inventar unvollständig")
		}
		afterInv, err := s.Collector.Probe(ctx, h)
		if err != nil {
			return errors.New("Abschließende Zielprüfung fehlgeschlagen")
		}
		if !reflect.DeepEqual(paths, outcomePaths(expected, afterInv)) || recoveryPlanFingerprint(inv, paths, nil) != recoveryPlanFingerprint(afterInv, paths, nil) {
			return errors.New("Zielbestand während der Prüfung geändert; erneut prüfen")
		}
		if _, err = check(); err != nil {
			return err
		}
		observed := map[string]RecoveryReadFile{}
		for _, f := range result.Files {
			observed[f.Path] = f
		}
		keys := []string{}
		for path := range expected {
			keys = append(keys, path)
		}
		sort.Strings(keys)
		a.State = "passed"
		a.Reason = "Geprüfte Dateiaktionen stimmen mit dem Sollstand überein"
		diffBudget := 64 << 10
		for _, path := range keys {
			f := compareOutcomeFile(expected[path], observed[path])
			if f.Comparison != nil {
				raw, _ := json.Marshal(f.Comparison)
				if len(raw) > diffBudget {
					f.Comparison = nil
				} else {
					diffBudget -= len(raw)
				}
			}
			a.Files = append(a.Files, f)
			if f.State == "different" {
				a.State = "different"
			} else if f.State != "passed" && a.State != "different" {
				a.State = "unknown"
			}
		}
		a.Settings = outcomeSettings(expected, observed)
		for _, v := range a.Settings {
			if v.State == "different" {
				a.State = "different"
			} else if v.State != "passed" && a.State != "different" {
				a.State = "unknown"
			}
		}
		if a.State == "different" {
			a.Reason = "Abweichungen am aktuell gelesenen Ziel festgestellt"
		} else if a.State == "unknown" {
			a.Reason = "Ein Teil des Ergebnisses ist nicht eindeutig nachgewiesen"
		}
		return nil
	}
	if err := run(); err != nil {
		a.State = "unknown"
		a.Reason = err.Error()
		a.Files = []RecoveryOutcomeFile{}
		a.Settings = []RecoveryOutcomeSetting{}
	}
	a.FinishedAt = now()
	err := save()
	return p, err
}
func outcomePaths(expected map[string]RecoveryReadFile, inv Inventory) []string {
	paths := map[string]bool{}
	for p := range expected {
		paths[p] = true
	}
	for _, setting := range trackedSettings {
		touched := false
		for p := range expected {
			touched = touched || settingPath(setting, p)
		}
		if !touched {
			continue
		}
		for p := range fileHashes(inv) {
			if settingPath(setting, p) {
				paths[p] = true
			}
		}
	}
	out := []string{}
	for p := range paths {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
func compareOutcomeFile(expected, observed RecoveryReadFile) RecoveryOutcomeFile {
	f := RecoveryOutcomeFile{Path: expected.Path, State: "unknown", Expected: expected.State, Observed: observed.State, ExpectedSHA: expected.SHA256, ObservedSHA: observed.SHA256, ContentState: "unknown", MetadataState: "unknown", Reason: "Zieldatei nicht vollständig lesbar"}
	if (expected.State != "read" && expected.State != "missing") || (observed.State != "read" && observed.State != "missing") {
		return f
	}
	f.State = "passed"
	f.Reason = "Inhalt und Dateieigenschaften stimmen überein"
	f.ContentState = "passed"
	f.MetadataState = "passed"
	if expected.State != observed.State {
		f.State = "different"
		f.ContentState = "different"
		f.MetadataState = "different"
		if expected.State == "missing" {
			f.Reason = "Datei ist weiterhin vorhanden, obwohl sie fehlen soll"
		} else {
			f.Reason = "Erwartete Datei fehlt am Ziel"
		}
	} else if expected.State == "read" {
		if expected.SHA256 != observed.SHA256 {
			f.State = "different"
			f.ContentState = "different"
			f.Reason = "Dateiinhalt weicht vom Sollstand ab"
		}
		if expected.Metadata == nil || observed.Metadata == nil {
			f.State = "unknown"
			f.MetadataState = "unknown"
			f.Reason = "Dateieigenschaften nicht vollständig bestätigt"
		} else {
			a, b := expected.Metadata, observed.Metadata
			if a.Type != b.Type || a.Mode != b.Mode || a.UID != b.UID || a.GID != b.GID || a.Link != b.Link || !equalXAttrs(a.XAttrs, b.XAttrs) {
				f.State = "different"
				f.MetadataState = "different"
				f.Reason = "Dateieigenschaften weichen ab"
				if f.ContentState == "different" {
					f.Reason = "Inhalt und Dateieigenschaften weichen ab"
				}
			}
		}
	}
	if expected.State == "missing" && observed.State == "missing" {
		f.Reason = "Datei fehlt wie vorgesehen"
	}
	if f.State == "different" && !comparisonSecret(expected.Content, expected.Metadata) && !comparisonSecret(observed.Content, observed.Metadata) {
		c := compareSides(comparisonSide{data: expected.Content, entry: expected.Metadata, state: expected.State}, comparisonSide{data: observed.Content, entry: observed.Metadata, state: observed.State}, "Soll laut Plan", "Aktuell am Ziel", expected.Path, false)
		raw, _ := json.Marshal(c)
		if len(raw) <= 16<<10 {
			f.Comparison = &c
		}
	}
	return f
}
func outcomeValue(setting TrackedSetting, files map[string]RecoveryReadFile) (string, string, []string) {
	value := ""
	sources := []string{}
	total := 0
	paths := []string{}
	for p := range files {
		if settingPath(setting, p) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	if len(paths) > 24 {
		return "", "limited", sources
	}
	for _, p := range paths {
		f := files[p]
		if f.State == "missing" {
			continue
		}
		total += len(f.Content)
		if f.State != "read" || f.Metadata == nil || len(f.Content) > 64<<10 || total > 256<<10 || !utf8.Valid(f.Content) {
			return "", "unavailable", sources
		}
		if comparisonSecret(f.Content, f.Metadata) {
			return "", "protected", []string{}
		}
		v, state := parseTrackedSetting(setting, string(f.Content))
		if state == "not_recorded" {
			continue
		}
		if state != "value" {
			return "", state, sources
		}
		sources = append(sources, p)
		if value != "" && value != v {
			return "", "ambiguous", sources
		}
		value = v
	}
	if value == "" {
		return "", "not_recorded", sources
	}
	return value, "value", sources
}
func outcomeSettings(expected, observed map[string]RecoveryReadFile) []RecoveryOutcomeSetting {
	out := []RecoveryOutcomeSetting{}
	for _, s := range trackedSettings {
		value, state, _ := outcomeValue(s, expected)
		if state == "not_recorded" {
			continue
		}
		current, currentState, sources := outcomeValue(s, observed)
		v := RecoveryOutcomeSetting{Setting: s, State: "unknown", ObservedState: currentState, Sources: sources, Reason: "Gespeicherte Deklaration nicht eindeutig nachgewiesen"}
		if state == "value" {
			v.ExpectedValue = value
			if currentState == "value" {
				v.ObservedValue = current
				v.State = "passed"
				v.Reason = "Deklaration entspricht dem Plan"
				if current != value {
					v.State = "different"
					v.Reason = "Deklarierter Wert weicht vom Plan ab"
				}
			} else if currentState == "ambiguous" {
				v.State = "different"
				v.Reason = "Widersprüchliche Deklarationen im geprüften Zielbestand"
			} else if currentState == "not_recorded" {
				v.State = "different"
				v.Reason = "Gewünschte Deklaration fehlt"
			}
		}
		out = append(out, v)
	}
	return out
}
