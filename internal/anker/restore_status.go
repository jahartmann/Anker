package anker

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	if err := s.disasterPlanAllowed(id); err != nil {
		return Plan{}, err
	}
	lock := s.planLock(id)
	lock.Lock()
	defer lock.Unlock()
	p, err := s.Plan(id)
	if err != nil {
		return p, err
	}
	ready := p.State == "ready"
	if (ready && (p.SchemaVersion != 2 || len(p.Blockers) > 0 || s.Demo)) || p.State == "manual" || p.State == "blocked" || p.State == "demo_applied" {
		return p, errors.New("Für diesen Plan ist keine ungeklärte Hostausführung gespeichert")
	}
	if ready {
		// Queue admission and this reservation have one order. A queued request can
		// still begin later, so the ordinary running-target lock alone is insufficient.
		if err = s.reserveReadyReconciliation(p.TargetID); err != nil {
			return p, err
		}
		defer func() { s.jobMu.Lock(); delete(s.reconciling, p.TargetID); s.jobMu.Unlock() }()
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
	var connection string
	if ready {
		snapshot, loadErr := s.loadPlanSnapshot(p)
		if loadErr != nil {
			return p, loadErr
		}
		connection, err = s.recoveryConnectionSHA(h)
		if err != nil {
			return p, err
		}
		if connection != snapshot.ConnectionSHA {
			return p, errors.New("Zielverbindung wurde geändert; Hostjournal an der ursprünglichen Zielkonsole prüfen")
		}
	}
	result, err := c.RestoreStatus(ctx, h, p)
	if err != nil {
		return p, err
	}
	if err = validateHostResult(p, result); err != nil {
		return p, err
	}
	if ready {
		current, loadErr := s.Host(p.TargetID)
		if loadErr != nil {
			return p, loadErr
		}
		currentConnection, connectionErr := s.recoveryConnectionSHA(current)
		if connectionErr != nil {
			return p, connectionErr
		}
		if currentConnection != connection {
			return p, errors.New("Zielverbindung während Journalabgleich geändert; Zielkonsole prüfen")
		}
		if result.State == "not_found" {
			// Absence at this instant is not a proof of a completed or globally unwritten
			// restore. Consume the uncertain plan; never reset it for another apply.
			result.Checks = []string{"Kein Journal für diese Recovery-ID gefunden. Dieser Plan bleibt gesperrt. Zielkonsole und laufende Aufträge prüfen; anschließend Vergleich und Plan neu vorbereiten."}
			result.RebootVerified = false
		}
	}
	setRecoveryResult(&p, result, nil)
	return p, s.savePlan(p)
}

func (s *Service) reserveReadyReconciliation(hostID string) error {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.reconciling[hostID] || s.connectingHostID == hostID {
		return errHostJobActive
	}
	jobs, err := s.Jobs()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.HostID == hostID && activeJob(j) {
			return errHostJobActive
		}
	}
	if s.reconciling == nil {
		s.reconciling = map[string]bool{}
	}
	s.reconciling[hostID] = true
	return nil
}

func (s *Service) RollbackPlan(ctx context.Context, id, confirmation string) (Plan, error) {
	if err := s.disasterPlanAllowed(id); err != nil {
		return Plan{}, err
	}
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

// recoveryActorKey cannot be set by deserializing a request. HTTP/CLI adapters
// supply their authenticated principal, never a caller-controlled actor field.
type recoveryActorKey struct{}

func WithRecoveryActor(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, recoveryActorKey{}, userID)
}

func operationRecoveryTasks(prerequisites []RecoveryTask) []RecoveryTask {
	requires := []string{}
	for _, task := range prerequisites {
		requires = append(requires, task.ID)
	}
	return []RecoveryTask{
		{ID: "check-services", Kind: "manual-check", Title: "Dienste prüfen", Instruction: "Übernommene Dateien und betroffene Dienste am Ziel prüfen; Ergebnis dokumentieren.", Requires: requires, State: "pending"},
		{ID: "check-reachability", Kind: "manual-check", Title: "Erreichbarkeit prüfen", Instruction: "Den vorgesehenen Zielendpunkt und die unabhängig geprüfte Hostidentität prüfen.", Requires: []string{"check-services"}, State: "pending"},
		{ID: "check-reboot", Kind: "manual-check", Title: "Neustart prüfen", Instruction: "Nach kontrolliertem Neustart Netzwerk, Storage und Gäste prüfen. Diese Benutzerbestätigung ist kein maschineller Neustartnachweis.", Requires: []string{"check-reachability"}, State: "pending"},
	}
}

func (s *Service) ConfirmRecoveryTask(ctx context.Context, planID, taskID, userID, evidence string) (Plan, error) {
	lock := s.planLock(planID)
	lock.Lock()
	defer lock.Unlock()
	p, err := s.Plan(planID)
	if err != nil {
		return p, err
	}
	actor, _ := ctx.Value(recoveryActorKey{}).(string)
	if actor == "" || actor != userID {
		return p, errors.New("authentifizierte Benutzeridentität erforderlich")
	}
	if p.SchemaVersion != 2 || len(p.Blockers) > 0 {
		return p, errors.New("Blocker oder alter Plan erlauben keine Aufgabenbestätigung")
	}
	if len(strings.TrimSpace(evidence)) == 0 || len(evidence) > 4096 {
		return p, errors.New("konkreten Prüfnachweis bis 4096 Zeichen angeben")
	}
	if err = s.verifyPlanFiles(p); err != nil {
		return p, err
	}
	index := -1
	tasks := map[string]RecoveryTask{}
	for i, task := range p.Tasks {
		tasks[task.ID] = task
		if task.ID == taskID {
			index = i
		}
	}
	if index < 0 {
		return p, errors.New("unbekannte Wiederherstellungsaufgabe")
	}
	task := p.Tasks[index]
	if task.State != "pending" {
		return p, errors.New("Aufgabe bereits bestätigt oder nicht manuell bestätigbar")
	}
	// Machine evidence is never user-confirmable. Preparation tasks and explicit
	// operational checks are manual attestations only.
	switch task.Kind {
	case "manual-check":
		if p.State == "ready" || p.State == "applying" || p.State == "interrupted" || p.State == "failed" || p.State == "rolling_back" || p.State == "rollback_conflict" {
			return p, errors.New("Dateiübernahme und Hostzustand zuerst klären")
		}
	case "backing", "bridge", "cluster", "file-type", "guest-collision", "guest-inventory", "guest-mac", "guest-node", "guest-syntax", "guest-volume", "hook", "hostname", "hostname-identity", "legacy-address", "mount", "network", "network-activation", "network-ambiguous", "network-field", "network-method", "network-netmask", "network-port-collision", "network-stanza", "network-syntax", "network-topology", "network-value", "pbs", "pci", "storage", "storage-content", "storage-nodes", "volume":
	default:
		return p, errors.New("Aufgabe ist nicht als manuelle Prüfung freigegeben")
	}
	for _, id := range task.Requires {
		dependency, ok := tasks[id]
		if !ok || dependency.State != "user_confirmed" {
			return p, errors.New("Voraussetzungen zuerst prüfen und bestätigen")
		}
	}
	p.Tasks[index].State = "user_confirmed"
	p.Tasks[index].ConfirmedBy = actor
	p.Tasks[index].ConfirmedAt = now()
	p.Tasks[index].Evidence = evidence
	// Preserve State/Result: human statements cannot grant apply or reboot proof.
	return p, s.savePlan(p)
}
