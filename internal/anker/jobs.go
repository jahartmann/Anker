package anker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

func (s *Service) Jobs() ([]Job, error) { return records[Job](s.Store, "jobs") }
func (s *Service) Job(id string) (Job, error) {
	var j Job
	if !validID(id) {
		return j, fail(400, "Ungültige Auftrags-ID")
	}
	if err := s.Store.Get("jobs", id, &j); err != nil {
		return j, err
	}
	if j.ID != id {
		return j, errors.New("Auftragseintrag ist ungültig")
	}
	return j, nil
}
func activeJob(j Job) bool { return j.State == "queued" || j.State == "running" }
func finishedJob(j Job) bool {
	return j.State == "successful" || j.State == "failed" || j.State == "cancelled" || j.State == "interrupted"
}
func (s *Service) DeleteJob(id string) error {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	j, err := s.Job(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	worker := s.cancels[id] != nil
	s.mu.Unlock()
	if !finishedJob(j) || worker {
		return fail(409, "Aktive Aufträge können nicht entfernt werden; erst Abschluss oder Abbruch abwarten")
	}
	return s.Store.Delete("jobs", id)
}
func (s *Service) RetryJob(id string) (Job, error) {
	j, err := s.Job(id)
	if err != nil {
		return Job{}, err
	}
	if !finishedJob(j) {
		return Job{}, fail(409, "Aktiven Auftrag erst beenden")
	}
	switch j.Kind {
	case "backup":
		return s.QueueBackup(j.HostID)
	case "probe":
		return s.QueueProbe(j.HostID)
	default:
		return Job{}, fail(409, "Wiederherstellung über einen frisch geprüften Plan mit neuer Bestätigung starten")
	}
}
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
	return s.queueBackup(hostID, nil)
}
func (s *Service) queueBackup(hostID string, at *time.Time) (Job, error) {
	return s.queueAt(hostID, "backup", at, func(ctx context.Context) (string, error) {
		b, err := s.CreateBackup(ctx, hostID)
		if err != nil {
			return "", err
		}
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
		var probeError string
		if err != nil {
			probeError = "Hostprüfung fehlgeschlagen; SSH-Zugang und Identität prüfen"
		} else {
			inv.Fingerprint = Fingerprint(inv)
		}
		var inventory *Inventory
		if err == nil {
			inventory = &inv
		}
		saveErr := s.updateHostInventory(h, inventory, probeError)
		if err == nil {
			err = saveErr
		}
		if err != nil {
			return "", err
		}
		return h.ID, nil
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
	return s.queueAt(hostID, kind, nil, run)
}

var errJobNotDue = errors.New("Zeitplan ist nicht fällig oder wurde heute bereits gestartet")
var errHostJobActive = errors.New("Host hat bereits einen aktiven Auftrag")

func (s *Service) queueAt(hostID, kind string, at *time.Time, run func(context.Context) (string, error)) (Job, error) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if _, err := s.Host(hostID); err != nil {
		return Job{}, err
	}
	if s.maintenance {
		return Job{}, errors.New("Anker wird aktualisiert; neue Aufträge sind vorübergehend gesperrt")
	}
	day := ""
	if at != nil {
		h, err := s.Host(hostID)
		if err != nil {
			return Job{}, err
		}
		settings, err := s.Settings()
		if err != nil {
			return Job{}, err
		}
		loc, err := time.LoadLocation(settings.Timezone)
		if err != nil {
			return Job{}, err
		}
		local := at.In(loc)
		due, err := scheduledTime(h, settings, local)
		if err != nil {
			return Job{}, err
		}
		if !h.Enabled || local.Before(due) {
			return Job{}, errJobNotDue
		}
		day = local.Format("2006-01-02")
	}
	jobs, err := s.Jobs()
	if err != nil {
		return Job{}, err
	}
	for _, j := range jobs {
		if j.HostID == hostID && (j.State == "queued" || j.State == "running") {
			return Job{}, errHostJobActive
		}
	}
	j := Job{ID: ID(), HostID: hostID, Kind: kind, State: "queued", CreatedAt: now(), Trigger: "manual", ScheduledDay: day}
	if day != "" {
		j.Trigger = "scheduled"
	}
	if err = s.Store.insertJob(j); err != nil {
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
	defer func() {
		s.mu.Lock()
		if cancel := s.cancels[j.ID]; cancel != nil {
			cancel()
		}
		delete(s.cancels, j.ID)
		s.mu.Unlock()
	}()
	var settings Settings
	for {
		if ctx.Err() != nil {
			j.State, j.FinishedAt = "cancelled", now()
			s.persistJob(j)
			return
		}
		s.jobMu.Lock()
		var err error
		settings, err = s.Settings()
		if err != nil || settings.Parallel < 1 || settings.Parallel > 16 || settings.Retries < 0 || settings.Retries > 5 {
			s.jobMu.Unlock()
			j.State, j.FinishedAt, j.Error = "failed", now(), "Betriebseinstellungen nicht lesbar oder ungültig; Katalog prüfen"
			s.persistJob(j)
			return
		}
		jobs, err := s.Jobs()
		if err != nil {
			s.jobMu.Unlock()
			j.State, j.FinishedAt, j.Error = "failed", now(), "Auftragsliste nicht lesbar; Katalog prüfen"
			s.persistJob(j)
			return
		}
		running := 0
		for _, job := range jobs {
			if job.State == "running" {
				running++
			}
		}
		if err == nil && running < settings.Parallel {
			if ctx.Err() != nil {
				s.jobMu.Unlock()
				continue
			}
			j.State, j.StartedAt, j.Attempts = "running", now(), 1
			err = s.Store.Put("jobs", j.ID, j)
			s.jobMu.Unlock()
			if err != nil {
				log.Printf("Anker Auftrag %s: Startzustand konnte nicht gespeichert werden: %v", j.ID, err)
				j.State, j.FinishedAt, j.Error = "failed", now(), "Auftragsstart konnte nicht gespeichert werden; Katalog und Speicher prüfen"
				s.persistJob(j)
				return
			}
			break
		}
		s.jobMu.Unlock()
		select {
		case <-ctx.Done():
			j.State = "cancelled"
			j.FinishedAt = now()
			s.persistJob(j)
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
		if ctx.Err() != nil {
			break
		}
		j.Attempts = attempt + 1
		if err = s.Store.Put("jobs", j.ID, j); err != nil {
			break
		}
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
	if s.persistJob(j) {
		s.notifyJob(j)
	}
}
func (s *Service) persistJob(j Job) bool {
	if err := s.Store.Put("jobs", j.ID, j); err != nil {
		log.Printf("Anker Auftrag %s: Status %s konnte nicht gespeichert werden: %v", j.ID, j.State, err)
		return false
	}
	return true
}
func (s *Service) CancelJob(id string) error {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	j, err := s.Job(id)
	if err != nil {
		return err
	}
	if !activeJob(j) {
		return fail(409, "Auftrag ist bereits beendet")
	}
	s.mu.Lock()
	cancel := s.cancels[id]
	s.mu.Unlock()
	if cancel == nil {
		return fail(409, "Auftrag hat keinen aktiven Worker; nach Dienstabbruch den gespeicherten Zustand prüfen")
	}
	cancel()
	return nil
}
func (s *Service) SaveSettings(v Settings) error {
	if v.SessionDays == 0 {
		v.SessionDays = 30
	}
	if v.SessionDays < 1 || v.SessionDays > 365 {
		return errors.New("Sitzungsdauer muss zwischen 1 und 365 Tagen liegen")
	}
	if v.Parallel < 1 || v.Parallel > 16 || v.Retries < 0 || v.Retries > 5 || v.Daily < 1 || v.Weekly < 1 || v.Monthly < 1 || v.StaleHours < 1 || v.ArchiveDays < 0 {
		return errors.New("ungültige Betriebswerte")
	}
	if _, err := time.LoadLocation(v.Timezone); err != nil {
		return fmt.Errorf("Zeitzone ungültig: %w", err)
	}
	if _, err := time.Parse("15:04", v.Schedule); err != nil {
		return errors.New("Zeitplan muss HH:MM sein")
	}
	if v.Webhook != "" {
		u, err := url.Parse(v.Webhook)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return errors.New("Webhook muss eine gültige HTTP- oder HTTPS-Adresse sein")
		}
	}
	if v.SMTPServer != "" {
		if _, _, err := net.SplitHostPort(v.SMTPServer); err != nil {
			return errors.New("SMTP-Server muss Host:Port enthalten")
		}
		for _, address := range []string{v.MailFrom, v.MailTo} {
			parsed, err := mail.ParseAddress(address)
			if err != nil || strings.ContainsAny(address, "\r\n") || parsed.Address != address {
				return errors.New("Gültige Absender- und Empfängeradresse ohne Anzeigenamen erforderlich")
			}
		}
	}
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
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
