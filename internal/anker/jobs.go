package anker

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (s *Service) Jobs() ([]Job, error) { return records[Job](s.Store, "jobs") }
func (s *Service) RecoverJobs() error {
	jobs, err := s.Jobs()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.State == "running" || j.State == "queued" {
			j.State = "interrupted"
			j.Error = "Dienst wurde unterbrochen; tatsächlichen Hostzustand prüfen"
			j.FinishedAt = now()
			if err = s.Store.Put("jobs", j.ID, j); err != nil {
				return err
			}
		}
	}
	plans, err := s.Plans()
	if err != nil {
		return err
	}
	for _, p := range plans {
		if p.State == "applying" {
			p.State = "interrupted"
			if err = s.savePlan(p); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Service) QueueBackup(hostID string) (Job, error) {
	return s.queue(hostID, "backup", func(ctx context.Context) (string, error) {
		b, err := s.CreateBackup(ctx, hostID)
		if err == nil && b.Status == "partial" {
			return b.ID, errors.New("Sicherung hat Pflichtlücken; Warnungen im Sicherungsstand prüfen")
		}
		return b.ID, err
	})
}
func (s *Service) QueueProbe(hostID string) (Job, error) {
	return s.queue(hostID, "probe", func(ctx context.Context) (string, error) {
		h, err := s.Host(hostID)
		if err != nil {
			return "", err
		}
		inv, err := s.Collector.Probe(ctx, h)
		h.LastProbe = now()
		if err != nil {
			h.ProbeError = "Hostprüfung fehlgeschlagen; SSH-Zugang und Identität prüfen"
		} else {
			inv.Fingerprint = Fingerprint(inv)
			h.Inventory = &inv
			h.ProbeError = ""
		}
		saveErr := s.SaveHost(h)
		if err == nil {
			err = saveErr
		}
		return h.ID, err
	})
}
func (s *Service) QueueApply(id, confirmation string) (Job, error) {
	p, err := s.Plan(id)
	if err != nil {
		return Job{}, err
	}
	if p.State != "ready" || confirmation != id {
		return Job{}, errors.New("geprüften Plan mit seiner ID bestätigen")
	}
	return s.queue(p.TargetID, "restore", func(ctx context.Context) (string, error) {
		p, err := s.ApplyPlan(ctx, id, confirmation)
		return p.ID, err
	})
}
func (s *Service) queue(hostID, kind string, run func(context.Context) (string, error)) (Job, error) {
	if _, err := s.Host(hostID); err != nil {
		return Job{}, err
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	jobs, err := s.Jobs()
	if err != nil {
		return Job{}, err
	}
	for _, j := range jobs {
		if j.HostID == hostID && (j.State == "queued" || j.State == "running") {
			return Job{}, errors.New("Host hat bereits einen aktiven Auftrag")
		}
	}
	j := Job{ID: ID(), HostID: hostID, Kind: kind, State: "queued", CreatedAt: now()}
	if err = s.Store.Put("jobs", j.ID, j); err != nil {
		return j, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[j.ID] = cancel
	s.mu.Unlock()
	go s.executeJob(ctx, j, run)
	return j, nil
}
func (s *Service) executeJob(ctx context.Context, j Job, run func(context.Context) (string, error)) {
	defer func() { s.mu.Lock(); delete(s.cancels, j.ID); s.mu.Unlock() }()
	settings, _ := s.Settings()
	if settings.Parallel < 1 {
		settings.Parallel = 4
	}
	for {
		s.jobMu.Lock()
		jobs, err := s.Jobs()
		running := 0
		for _, job := range jobs {
			if job.State == "running" {
				running++
			}
		}
		if err == nil && running < settings.Parallel {
			j.State = "running"
			err = s.Store.Put("jobs", j.ID, j)
			s.jobMu.Unlock()
			if err != nil {
				return
			}
			break
		}
		s.jobMu.Unlock()
		select {
		case <-ctx.Done():
			j.State = "cancelled"
			j.FinishedAt = now()
			s.Store.Put("jobs", j.ID, j)
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	retries := 0
	if j.Kind == "backup" || j.Kind == "probe" {
		retries = settings.Retries
	}
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		j.Attempts = attempt + 1
		j.ResultID, err = run(ctx)
		if err == nil || ctx.Err() != nil || j.ResultID != "" {
			break
		}
		if attempt < retries {
			select {
			case <-ctx.Done():
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}
	j.State = "successful"
	if err != nil {
		j.State = "failed"
		j.Error = err.Error()
	}
	if ctx.Err() != nil {
		j.State = "cancelled"
		j.Error = "Auftrag abgebrochen; tatsächlichen Zustand prüfen"
	}
	j.FinishedAt = now()
	s.Store.Put("jobs", j.ID, j)
	s.notifyJob(j)
}
func (s *Service) CancelJob(id string) error {
	s.mu.Lock()
	cancel := s.cancels[id]
	s.mu.Unlock()
	if cancel == nil {
		return errors.New("Auftrag ist nicht aktiv")
	}
	cancel()
	return nil
}
func (s *Service) SaveSettings(v Settings) error {
	if v.Parallel < 1 || v.Parallel > 16 || v.Retries < 0 || v.Retries > 5 || v.Daily < 1 || v.Weekly < 1 || v.Monthly < 1 || v.StaleHours < 1 || v.ArchiveDays < 0 {
		return errors.New("ungültige Betriebswerte")
	}
	if _, err := time.LoadLocation(v.Timezone); err != nil {
		return fmt.Errorf("Zeitzone ungültig: %w", err)
	}
	if _, err := time.Parse("15:04", v.Schedule); err != nil {
		return errors.New("Zeitplan muss HH:MM sein")
	}
	return s.Store.Put("settings", "main", v)
}

// StopJobs cancels queued work and waits for workers to persist their final state.
func (s *Service) StopJobs(ctx context.Context) error {
	s.mu.Lock()
	for _, cancel := range s.cancels {
		cancel()
	}
	s.mu.Unlock()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		n := len(s.cancels)
		s.mu.Unlock()
		if n == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
