package anker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func (s *Service) ApplyPlan(ctx context.Context, id, confirmation string) (Plan, error) {
	lock := s.planLock(id)
	lock.Lock()
	defer lock.Unlock()
	p, err := s.Plan(id)
	if err != nil {
		return p, err
	}
	if confirmation != id {
		return p, errors.New("Plan-ID als ausdrückliche Bestätigung erforderlich")
	}
	if p.SchemaVersion != 2 {
		return p, errors.New("Alter Plan bleibt lesbar; für eine neue Übernahme Vergleich und Plan neu erstellen")
	}
	if len(p.Blockers) > 0 {
		return p, errors.New("Plan hat ungeklärte Blocker")
	}
	if p.State != "ready" {
		return p, errors.New("Plan ist nicht zur automatischen Ausführung freigegeben")
	}
	if err = s.acquire(p.TargetID); err != nil {
		return p, err
	}
	defer s.release(p.TargetID)
	if err = s.VerifyBackup(p.BackupID); err != nil {
		return p, err
	}
	if err = s.verifyPlanFiles(p); err != nil {
		return p, err
	}
	h, err := s.Host(p.TargetID)
	if err != nil {
		return p, err
	}
	snapshot, err := s.loadPlanSnapshot(p)
	if err != nil {
		return p, err
	}
	connection, err := s.recoveryConnectionSHA(h)
	if err != nil {
		return p, err
	}
	if connection != snapshot.ConnectionSHA {
		return p, errors.New("Zielverbindung wurde geändert; Vergleich wiederholen")
	}
	inv, err := s.Collector.Probe(ctx, h)
	if err != nil {
		return p, err
	}
	activePaths := []string{}
	for _, step := range p.Steps {
		if step.Action == "apply" {
			activePaths = append(activePaths, step.Path)
		}
	}
	if len(activePaths) == 0 {
		return p, errors.New("Plan enthält keine freigegebenen Schreibaktionen")
	}
	if recoveryPlanFingerprint(inv, activePaths, nil) != recoveryPlanFingerprint(p.Target, activePaths, nil) {
		return p, errors.New("Ziel hat sich seit der Planung geändert; Plan neu erstellen")
	}
	manifest, err := s.Manifest(p.BackupID)
	if err != nil {
		return p, err
	}
	if err = validateDecisionSteps(p, manifest, snapshot, inv); err != nil {
		return p, err
	}
	if _, real := s.Collector.(SSHCollector); real {
		var capability struct {
			Protocol int  `json:"restore_protocol"`
			Journal  bool `json:"journal"`
		}
		if json.Unmarshal(inv.Details["capabilities"], &capability) != nil || capability.Protocol != 2 || !capability.Journal {
			return p, errors.New("Hosthelfer unterstützt die abgesicherte Wiederherstellung noch nicht; Verbindung am Host neu einrichten")
		}
		manifest, readErr := s.Manifest(p.BackupID)
		if readErr != nil {
			return p, readErr
		}
		entries := map[string]Entry{}
		for _, entry := range manifest.Entries {
			entries[entry.Path] = entry
		}
		for _, step := range p.Steps {
			if step.Action != "apply" {
				continue
			}
			entry, ok := entries[step.Path]
			if !ok || fileManualReason(entry, p.Source, inv) != "" {
				return p, errors.New("Plan enthält inzwischen nur manuell freigegebene Dateien; Plan neu erstellen")
			}
		}
	}
	for _, step := range p.Steps {
		if step.Action != "apply" {
			continue
		}
		file, err := safeJoin(filepath.Join(s.Root, "plans", id, "prepared-files"), step.Path)
		if err != nil {
			return p, err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return p, err
		}
		if Hash(data) != step.PreparedSHA {
			return p, errors.New("vorbereitete Datei wurde verändert")
		}
	}
	current, err := s.Host(p.TargetID)
	if err != nil {
		return p, err
	}
	currentConnection, err := s.recoveryConnectionSHA(current)
	if err != nil {
		return p, err
	}
	if currentConnection != snapshot.ConnectionSHA {
		return p, errors.New("Zielverbindung während Vorprüfung geändert; Vergleich wiederholen")
	}
	p.State = "applying"
	if err = s.savePlan(p); err != nil {
		return p, err
	}
	helperPlan := p
	helperPlan.Target = inv
	helperPlan.Target.Fingerprint = Fingerprint(inv)
	result, err := s.Collector.Apply(ctx, h, helperPlan, filepath.Join(s.Root, "plans", id))
	if _, real := s.Collector.(SSHCollector); real && result.OperationID != "" {
		if validationErr := validateHostResult(p, result); validationErr != nil {
			result = ApplyResult{State: "interrupted", OperationID: p.ID, Error: validationErr.Error()}
			err = errors.Join(err, validationErr)
		}
	}
	if _, real := s.Collector.(SSHCollector); real && err == nil && result.OperationID == "" {
		err = errors.New("Hostantwort bestätigt keine Recovery-ID; tatsächlichen Zustand über Hostprotokoll prüfen")
	}
	if _, real := s.Collector.(SSHCollector); real && err == nil && result.State != "applied" {
		err = errors.New("Host bestätigt keine erfolgreiche Dateiübernahme; Hostprotokoll prüfen")
	}
	setRecoveryResult(&p, result, err)
	if err != nil {
		p.Manual = append(p.Manual, "Ausführung unterbrochen; tatsächlichen Zielzustand und Host-Rollbackprotokoll prüfen. Nicht ungeprüft wiederholen.")
		if saveErr := s.savePlan(p); saveErr != nil {
			return p, errors.Join(err, saveErr)
		}
		return p, err
	}
	if s.Demo {
		p.State = "demo_applied"
	}
	return p, s.savePlan(p)
}
