package anker

import (
	"context"
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
	inv, err := s.Collector.Probe(ctx, h)
	if err != nil {
		return p, err
	}
	if Fingerprint(inv) != p.Target.Fingerprint {
		return p, errors.New("Ziel hat sich seit der Planung geändert; Plan neu erstellen")
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
	p.State = "applying"
	if err = s.savePlan(p); err != nil {
		return p, err
	}
	result, err := s.Collector.Apply(ctx, h, p, filepath.Join(s.Root, "plans", id))
	if err != nil {
		p.State = "failed"
		p.Manual = append(p.Manual, "Ausführung unterbrochen; tatsächlichen Zielzustand und Host-Rollbackprotokoll prüfen. Nicht ungeprüft wiederholen.")
		s.savePlan(p)
		return p, err
	}
	p.Result = &result
	p.State = "checks_pending"
	if s.Demo {
		p.State = "demo_applied"
	}
	return p, s.savePlan(p)
}
