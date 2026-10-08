package anker

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func planComparisonState(e Entry, content []byte, before RecoveryReadFile) string {
	switch before.State {
	case "missing":
		return "missing_before"
	case "read":
		m := before.Metadata
		if m != nil && Hash(content) == before.SHA256 && e.Type == m.Type && e.Mode == m.Mode && e.UID == m.UID && e.GID == m.GID && e.Link == m.Link && equalXAttrs(e.XAttrs, m.XAttrs) {
			return "identical"
		}
		return "different"
	case "too_large":
		return "too_large"
	default:
		return "unavailable"
	}
}
func equalXAttrs(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || v != w {
			return false
		}
	}
	return true
}
func copyPlanSnapshot(root string, snapshot RecoverySnapshot) (string, error) {
	for _, f := range snapshot.Files {
		if f.State != "read" {
			continue
		}
		path, err := safeJoin(filepath.Join(root, "target-files"), f.Path)
		if err != nil {
			return "", err
		}
		if err = atomicWrite(path, f.Content, 0600); err != nil {
			return "", err
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	if err = atomicWrite(filepath.Join(root, "target-snapshot.json"), data, 0600); err != nil {
		return "", err
	}
	return Hash(data), nil
}

// Independent private evidence: the original inspection may have expired or been removed.
// This does not probe the host, so legacy/manual exports remain possible offline.
func (s *Service) loadPlanSnapshot(p Plan) (RecoverySnapshot, error) {
	var snapshot RecoverySnapshot
	if p.TargetSnapshotID == "" {
		if p.TargetSnapshotSHA != "" {
			return snapshot, errors.New("ungültiger Zielnachweis")
		}
		for _, step := range p.Steps {
			if step.Action == "apply" {
				return snapshot, errors.New("automatische Übernahme ohne Zielaufnahme")
			}
		}
		return snapshot, nil
	}
	root := filepath.Join(s.Root, "plans", p.ID)
	data, err := readSnapshotBytes(filepath.Join(root, "target-snapshot.json"), 32<<20)
	if err != nil {
		return snapshot, err
	}
	if Hash(data) != p.TargetSnapshotSHA {
		return snapshot, errors.New("Plan-Zielaufnahme wurde verändert")
	}
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return snapshot, err
	}
	situation, err := recoverySituation(p.Scenario)
	if err != nil {
		return snapshot, err
	}
	if snapshot.ID != p.TargetSnapshotID || snapshot.BackupID != p.BackupID || snapshot.TargetID != p.TargetID || snapshot.Scenario != p.Scenario || snapshot.Situation != situation || snapshot.Target.Fingerprint != p.Target.Fingerprint {
		return snapshot, errors.New("Plan-Zielkontext wurde verändert")
	}
	m, err := s.Manifest(p.BackupID)
	if err != nil {
		return snapshot, err
	}
	paths := []string{}
	for _, step := range p.Steps {
		paths = append(paths, step.Path)
	}
	r := PlanRequest{BackupID: p.BackupID, TargetID: p.TargetID, Scenario: p.Scenario, Files: paths, Mapping: p.Mapping}
	selection, err := scenarioSelection(m, r)
	if err != nil {
		return snapshot, err
	}
	if snapshot.SelectionHash != selectionSHA(selection, r) {
		return snapshot, errors.New("Plan-Dateiauswahl wurde verändert")
	}
	if err = validateSnapshotEvidenceSet(snapshot, selection); err != nil {
		return snapshot, err
	}
	for i, f := range snapshot.Files {
		if f.State != "read" {
			continue
		}
		path, joinErr := safeJoin(filepath.Join(root, "target-files"), f.Path)
		if joinErr != nil {
			return snapshot, joinErr
		}
		content, readErr := readSnapshotBytes(path, 1<<20)
		if readErr != nil {
			return snapshot, readErr
		}
		snapshot.Files[i].Content = content
	}
	ids := []string{}
	for id := range snapshot.StorageVolumes {
		ids = append(ids, id)
	}
	if err = validateRecoveryRead(RecoveryReadResult{Files: snapshot.Files, StorageVolumes: snapshot.StorageVolumes}, recoveryEvidencePaths(selection, snapshot.Target), ids); err != nil {
		return snapshot, err
	}
	if err = validateSnapshotProbe(snapshot.Target, RecoveryReadResult{Files: snapshot.Files}); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

// PlanContainsSecrets fails closed on integrity errors; includes supplemental target evidence.
// Callers must still enforce the existing full-export secrets permission unconditionally.
func (s *Service) PlanContainsSecrets(id string) (bool, error) {
	lock := s.planLock(id)
	lock.RLock()
	defer lock.RUnlock()
	p, err := s.Plan(id)
	if err != nil {
		return false, err
	}
	release, err := s.readableLease(p.BackupID)
	if err != nil {
		return false, err
	}
	defer release()
	if err = s.checkBackupUnlocked(p.BackupID); err != nil {
		return false, err
	}
	if err = s.verifyPlanFiles(p); err != nil {
		return false, err
	}
	m, err := s.Manifest(p.BackupID)
	if err != nil {
		return false, err
	}
	secret := false
	for _, e := range m.Entries {
		secret = secret || e.Secret || isSecret(e.Path)
	}
	if p.SchemaVersion == 2 {
		snapshot, err := s.loadPlanSnapshot(p)
		if err != nil {
			return false, err
		}
		for _, f := range snapshot.Files {
			if f.State == "read" {
				secret = secret || isSecret(f.Path) || secretContent(f.Content) || (f.Metadata != nil && f.Metadata.Secret)
			}
		}
	}
	return secret, nil
}

// Export names distinguish captured evidence from files prepared for installation.
func tarPlanTree(w *tar.Writer, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		st, err := d.Info()
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe export link")
		}
		h, err := tar.FileInfoHeader(st, "")
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		if h.Name == "target-files" {
			h.Name = "target-before"
		} else if strings.HasPrefix(h.Name, "target-files/") {
			h.Name = "target-before/" + strings.TrimPrefix(h.Name, "target-files/")
		}
		if err = w.WriteHeader(h); err != nil {
			return err
		}
		if st.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, err = w.Write(data)
			return err
		}
		return nil
	})
}

func validateDecisionSteps(p Plan, m Manifest, snapshot RecoverySnapshot, target Inventory) error {
	entries := map[string]Entry{}
	for _, e := range m.Entries {
		entries[e.Path] = e
	}
	before := map[string]RecoveryReadFile{}
	for _, f := range snapshot.Files {
		before[f.Path] = f
	}
	seen := map[string]bool{}
	for _, step := range p.Steps {
		if seen[step.Path] || p.Decisions[step.Path] != step.Decision {
			return errors.New("inkonsistente Dateientscheidung")
		}
		seen[step.Path] = true
		if step.Action != "apply" {
			continue
		}
		e, ok := entries[step.Path]
		f := before[step.Path]
		if !ok || fileManualReason(e, p.Source, target) != "" || step.Destination != step.Path || (step.Decision != DecisionBackup && step.Decision != DecisionPrepared) || step.ComparisonState == "identical" || (f.State != "read" && f.State != "missing") {
			return errors.New("Dateientscheidung erlaubt keine automatische Übernahme")
		}
		sha := f.SHA256
		if f.State == "missing" {
			sha = "missing"
		}
		if step.BeforeSHA != sha {
			return errors.New("Zielvorbedingung weicht von Aufnahme ab")
		}
	}
	if snapshot.Target.Fingerprint != p.Target.Fingerprint {
		return errors.New("Zielaufnahme fehlt")
	}
	return nil
}

// Retain host/account/topology checks while excluding kept/unselected file state.
// An empty selection means a scenario selection, so keep the conservative full set.
func recoveryPlanFingerprint(inv Inventory, paths []string, decisions map[string]FileDecision) string {
	if len(paths) == 0 {
		return Fingerprint(inv)
	}
	details := make(map[string]json.RawMessage, len(inv.Details))
	for k, v := range inv.Details {
		details[k] = v
	}
	for _, key := range []string{"file_hashes", "file_metadata"} {
		var all map[string]json.RawMessage
		if json.Unmarshal(details[key], &all) != nil {
			continue
		}
		relevant := map[string]json.RawMessage{}
		for _, path := range paths {
			if decisions[path] != DecisionKeep {
				if v, ok := all[path]; ok {
					relevant[path] = v
				}
			}
		}
		details[key], _ = json.Marshal(relevant)
	}
	inv.Details = details
	return Fingerprint(inv)
}

func planEvidenceSelection(snapshot RecoverySnapshot, r PlanRequest, selection RecoverySelection) ([]string, map[string]FileDecision) {
	paths := []string{}
	decisions := map[string]FileDecision{}
	network, guest := false, false
	for _, path := range selection.Paths {
		if r.Decisions[path] != DecisionKeep {
			network = network || strings.HasPrefix(path, "etc/network/")
			guest = guest || guestConfigPath(path)
		}
	}
	for _, f := range snapshot.Files {
		paths = append(paths, f.Path)
		if r.Decisions[f.Path] == DecisionKeep && !(network && networkEvidencePath(f.Path)) && !(guest && f.Path == "etc/pve/storage.cfg") {
			decisions[f.Path] = DecisionKeep
		}
	}
	return paths, decisions
}
