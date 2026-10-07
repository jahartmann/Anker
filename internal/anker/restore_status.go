package anker

import (
	"context"
	"errors"
	"fmt"
)

type recoveryJournalCollector interface {
	RestoreStatus(context.Context, Host, Plan) (ApplyResult, error)
	Rollback(context.Context, Host, Plan, string) (ApplyResult, error)
}

func validateHostResult(p Plan, result ApplyResult) error {
	if result.OperationID != p.ID {
		return errors.New("Hostprotokoll gehört nicht zu diesem Wiederherstellungsplan")
	}
	allowed := map[string]bool{}
	for _, step := range p.Steps {
		if step.Action == "apply" {
			allowed[step.Path] = true
		}
	}
	seen := map[string]bool{}
	for _, path := range result.Applied {
		if !allowed[path] || seen[path] {
			return errors.New("Hostprotokoll enthält unerwartete oder doppelte Dateien")
		}
		seen[path] = true
	}
	switch result.State {
	case "applied":
		if len(seen) != len(allowed) {
			return errors.New("Hostprotokoll bestätigt keine vollständige Dateiübernahme")
		}
	case "rolled_back", "failed", "interrupted", "rollback_conflict", "writing", "rolling_back", "not_found":
	default:
		return errors.New("Hostprotokoll meldet einen unbekannten Wiederherstellungszustand")
	}
	return nil
}

func setRecoveryResult(p *Plan, result ApplyResult, operationErr error) {
	p.Result = &result
	switch result.State {
	case "rolled_back":
		p.State = "rolled_back"
	case "rollback_conflict":
		p.State = "rollback_conflict"
	case "writing", "rolling_back", "interrupted", "not_found":
		p.State = "interrupted"
	case "failed":
		p.State = "failed"
	case "applied":
		p.State = "checks_pending"
	default:
		if operationErr != nil {
			p.State = "interrupted"
		} else {
			p.State = "checks_pending"
		}
	}
	if operationErr != nil && p.Result.Error == "" {
		p.Result.Error = operationErr.Error()
	}
}

// ReconcilePlan reads the host journal; it never repeats a write operation.
func (s *Service) ReconcilePlan(ctx context.Context, id string) (Plan, error) {
	lock := s.planLock(id)
	lock.Lock()
	defer lock.Unlock()
	p, err := s.Plan(id)
	if err != nil {
		return p, err
	}
	if p.State == "ready" || p.State == "manual" || p.State == "blocked" || p.State == "demo_applied" {
		return p, errors.New("Für diesen Plan ist keine ungeklärte Hostausführung gespeichert")
	}
	if err = s.acquire(p.TargetID); err != nil {
		return p, err
	}
	defer s.release(p.TargetID)
	c, ok := s.Collector.(recoveryJournalCollector)
	if !ok {
		return p, errors.New("Hostzugang unterstützt noch keine Recovery-Protokollabfrage; Verbindung neu einrichten")
	}
	h, err := s.Host(p.TargetID)
	if err != nil {
		return p, err
	}
	result, err := c.RestoreStatus(ctx, h, p)
	if err != nil {
		return p, err
	}
	if err = validateHostResult(p, result); err != nil {
		return p, err
	}
	setRecoveryResult(&p, result, nil)
	return p, s.savePlan(p)
}

func (s *Service) RollbackPlan(ctx context.Context, id, confirmation string) (Plan, error) {
	lock := s.planLock(id)
	lock.Lock()
	defer lock.Unlock()
	p, err := s.Plan(id)
	if err != nil {
		return p, err
	}
	if confirmation != id {
		return p, errors.New("Plan-ID als ausdrückliche Bestätigung der Rücksetzung erforderlich")
	}
	if p.State == "ready" || p.State == "manual" || p.State == "blocked" || p.State == "rolled_back" || p.State == "demo_applied" {
		return p, errors.New("Dieser Plan hat keine rücksetzbare reale Übernahme")
	}
	if err = s.acquire(p.TargetID); err != nil {
		return p, err
	}
	defer s.release(p.TargetID)
	c, ok := s.Collector.(recoveryJournalCollector)
	if !ok {
		return p, errors.New("Hostzugang unterstützt noch keine kontrollierte Rücksetzung; Verbindung neu einrichten")
	}
	h, err := s.Host(p.TargetID)
	if err != nil {
		return p, err
	}
	status, err := c.RestoreStatus(ctx, h, p)
	if err != nil {
		return p, fmt.Errorf("Hostprotokoll vor Rücksetzung nicht prüfbar: %w", err)
	}
	if err = validateHostResult(p, status); err != nil {
		return p, err
	}
	if status.State == "not_found" || status.State == "writing" || status.State == "rolling_back" || status.State == "rolled_back" {
		return p, errors.New("Hostzustand erlaubt derzeit keine Rücksetzung; Protokoll und laufende Ausführung prüfen")
	}
	p.State = "rolling_back"
	p.Result = &status
	if err = s.savePlan(p); err != nil {
		return p, err
	}
	result, err := c.Rollback(ctx, h, p, confirmation)
	if result.OperationID == "" {
		result.State = "interrupted"
		err = errors.Join(err, errors.New("Hostantwort bestätigt keine Recovery-ID; tatsächlichen Zustand über Hostprotokoll prüfen"))
	} else {
		if validationErr := validateHostResult(p, result); validationErr != nil {
			result = ApplyResult{OperationID: p.ID, State: "interrupted", Error: validationErr.Error()}
			err = errors.Join(err, validationErr)
		}
	}
	if err == nil && result.State != "rolled_back" {
		err = errors.New("Host bestätigt keine abgeschlossene Rücksetzung; Hostprotokoll prüfen")
	}
	setRecoveryResult(&p, result, err)
	if saveErr := s.savePlan(p); saveErr != nil {
		return p, errors.Join(err, saveErr)
	}
	return p, err
}

func (s *Service) QueueRollback(id, confirmation string) (Job, error) {
	p, err := s.Plan(id)
	if err != nil {
		return Job{}, err
	}
	if confirmation != id {
		return Job{}, errors.New("Plan-ID zur Rücksetzung bestätigen")
	}
	if p.State == "ready" || p.State == "manual" || p.State == "blocked" || p.State == "rolled_back" || p.State == "demo_applied" {
		return Job{}, errors.New("Dieser Plan hat keine rücksetzbare reale Übernahme")
	}
	return s.queue(p.TargetID, "rollback", func(ctx context.Context) (string, error) {
		p, err := s.RollbackPlan(ctx, id, confirmation)
		return p.ID, err
	})
}
