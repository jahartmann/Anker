package anker

import "errors"

// RecoveryPreviewRequest compares candidate choices without creating a plan.
// Nonempty decisions are rejected: all selected paths participate in mapping validation.
type RecoveryPreviewRequest struct {
	PlanRequest
	ManifestSHA string `json:"manifest_sha"`
	Path        string `json:"path"`
	Basis       string `json:"basis"`
	Reveal      bool   `json:"reveal"`
}
type RecoveryCandidateAction struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}
type RecoveryPreview struct {
	SnapshotID     string                  `json:"snapshot_id"`
	ManifestSHA    string                  `json:"manifest_sha"`
	Comparison     FileComparison          `json:"comparison"`
	OriginalAction RecoveryCandidateAction `json:"original_action"`
	PreparedAction RecoveryCandidateAction `json:"prepared_action"`
}

// Shared with saved plans: equality includes writable metadata, and an otherwise
// eligible candidate still needs the plan's global checks before execution.
func recoveryCandidateAction(e Entry, prepared PreparedRecoveryFile, before RecoveryReadFile, snapshot RecoverySnapshot, decision FileDecision) RecoveryCandidateAction {
	if planComparisonState(e, prepared.Content, before) == "identical" {
		return RecoveryCandidateAction{"keep", "Inhalt und Schreibmetadaten identisch; Ziel behalten"}
	}
	reason := fileManualReason(e, snapshot.Source, snapshot.Target)
	if reason == "" && decision != DecisionManual && snapshot.ID != "" && (before.State == "read" || before.State == "missing") && len(prepared.ManualTasks) == 0 {
		return RecoveryCandidateAction{"apply", "Datei zur Übernahme vorbereiten; Planprüfungen bleiben erforderlich"}
	}
	if reason == "" {
		reason = "Manuelle Entscheidung oder ungeprüfte Voraussetzungen; keine automatische Übernahme"
	}
	return RecoveryCandidateAction{"manual", reason}
}
func (s *Service) PreviewRecoveryFile(in RecoveryPreviewRequest) (RecoveryPreview, error) {
	var out RecoveryPreview
	if err := ValidPath(in.Path); err != nil {
		return out, err
	}
	if in.Basis != "source-target" && in.Basis != "source" && in.Basis != "target" {
		return out, fail(400, "Ungültige Vergleichsbasis")
	}
	if in.SnapshotID == "" || in.ManifestSHA == "" {
		return out, fail(400, "Gebundene Zielaufnahme und Quellhash erforderlich")
	}
	if len(in.Decisions) > 0 {
		return out, fail(400, "Vorschau erwartet noch keine Dateientscheidungen")
	}
	release, err := s.readableLease(in.BackupID)
	if err != nil {
		return out, err
	}
	defer release()
	snapshot, err := s.loadRecoverySnapshotUnlocked(in.SnapshotID, in.PlanRequest)
	if err != nil {
		return out, err
	}
	b, err := s.Backup(in.BackupID)
	if err != nil {
		return out, err
	}
	if in.ManifestSHA != b.ManifestSHA || snapshot.SourceManifestSHA != b.ManifestSHA {
		return out, errors.New("Quellstand wurde geändert; Vergleich wiederholen")
	}
	m, err := s.Manifest(b.ID)
	if err != nil {
		return out, err
	}
	selected, err := recoverySelection(m, in.PlanRequest)
	if err != nil {
		return out, err
	}
	if !selected[in.Path] {
		return out, fail(400, "Datei nicht in gebundener Auswahl")
	}
	inspection, err := s.analyzeRecovery(b, m, snapshot.Target, in.PlanRequest)
	if err != nil {
		return out, err
	}
	resolved, blockers, _ := validateRecoveryMapping(inspection, in.PlanRequest)
	if len(blockers) > 0 {
		return out, fail(400, "Zuordnungen prüfen: "+blockers[0])
	}
	// Network keys must refer to selected source stanzas, even if unused by this file.
	for key := range in.Mapping.Network {
		found := false
		for _, stanza := range inspection.NetworkStanzas {
			if stanza.Key == key {
				found = true
				break
			}
		}
		if !found {
			return out, fail(400, "Unbekannte Netzwerk-Stanza")
		}
	}
	mapping := in.Mapping
	mapping.Interfaces = resolved
	source, err := s.backupComparisonSide(b.ID, in.Path)
	if err != nil {
		return out, err
	}
	if source.entry == nil || source.state != "read" {
		return out, errors.New("Quelldatei konnte nicht sicher gelesen werden")
	}
	before := RecoveryReadFile{Path: in.Path, State: "unavailable"}
	for _, f := range snapshot.Files {
		if f.Path == in.Path {
			before = f
			break
		}
	}
	target := comparisonSide{data: before.Content, entry: before.Metadata, state: before.State}
	prepared, err := prepareRecoveryFile(*source.entry, source.data, snapshot, mapping, in.Scenario)
	if err != nil {
		return out, err
	}
	e := *source.entry
	e.Size = int64(len(prepared.Content))
	e.SHA256 = Hash(prepared.Content)
	candidate := comparisonSide{data: prepared.Content, entry: &e, state: "read"}
	out.SnapshotID, out.ManifestSHA = snapshot.ID, b.ManifestSHA
	out.OriginalAction = recoveryCandidateAction(*source.entry, PreparedRecoveryFile{Content: source.data, Destination: in.Path}, before, snapshot, DecisionBackup)
	out.PreparedAction = recoveryCandidateAction(*source.entry, prepared, before, snapshot, DecisionPrepared)
	out.Comparison = compareRecoveryCandidates(source, target, candidate, in.Path, b.ID, in.Basis, in.Reveal, false)
	// Return no candidate-derived evidence through a hidden-side secret gate.
	if out.Comparison.Masked {
		out.OriginalAction = RecoveryCandidateAction{"unknown", "Geschützten Inhalt ausdrücklich freigeben und prüfen"}
		out.PreparedAction = out.OriginalAction
	}
	return out, nil
}

func compareRecoveryCandidates(source, target, prepared comparisonSide, path, backup, basis string, reveal, secret bool) FileComparison {
	a, b, al, bl := target, prepared, "Ziel vor Übernahme", "Vorbereitet"
	if basis == "source" {
		a = source
		al = "Sicherung " + backup
	}
	if basis == "source-target" {
		a, b = source, target
		al = "Sicherung " + backup
		bl = "Ziel vor Übernahme"
	}
	if !reveal && (secret || comparisonSecret(source.data, source.entry) || comparisonSecret(target.data, target.entry) || comparisonSecret(prepared.data, prepared.entry)) {
		return FileComparison{Path: path, BeforeLabel: al, AfterLabel: bl, State: "masked", Masked: true, MetadataChanges: []MetadataChange{}, Hunks: []DiffHunk{}}
	}
	if !reveal {
		for _, side := range []comparisonSide{source, target} {
			if side.state != "read" && side.state != "missing" {
				return compareSides(source, target, al, bl, path, false)
			}
		}
	}
	return compareSides(a, b, al, bl, path, reveal)
}
