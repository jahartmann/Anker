package anker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"syscall"
	"time"
)

func (s *Service) Jobs() ([]Job, error) {
	jobs, err := records[Job](s.Store, "jobs")
	for i := range jobs {
		if jobs[i].Error != "" {
			jobs[i].Issues = backupIssues([]string{jobs[i].Error})
		}
	}
	return jobs, err
}
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
	if j.Error != "" {
		j.Issues = backupIssues([]string{j.Error})
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
	// Keep a minimal admission tombstone: deleting history must not authorize
	// replaying a still-pending durable batch after a lost response.
	tx, err := s.Store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = txPut(tx, "deleted-job-requests", j.ID, map[string]string{"host_id": j.HostID, "kind": j.Kind}); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM records WHERE bucket='jobs' AND id=?", id); err != nil {
		return err
	}
	return tx.Commit()
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
		return s.queueAtForSetRetry(j.HostID, "backup", nil, "", j.ID, s.backupRunner(j.HostID))
	case "backup_verify":
		return s.queueAtForSetRetry(j.HostID, j.Kind, nil, "", j.ID, s.remediationRunner(j.HostID))
	case "verify":
		if j.ResultID == "" {
			return Job{}, fail(409, "Sicherung aus der Hostauswahl erneut prüfen")
		}
		return s.queueAtForSetRetry(j.HostID, j.Kind, nil, "", j.ID, s.verificationRunner(j.ResultID))
	case "drift":
		return s.queueAtForSetRetry(j.HostID, j.Kind, nil, "", j.ID, s.driftRunner(j.HostID))
	case "probe":
		return s.queueAtForSetRetry(j.HostID, j.Kind, nil, "", j.ID, s.probeRunner(j.HostID))
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
			previousState := j.State
			j.State = "interrupted"
			finishJobStage(&j, "interrupted")
			if j.Kind == "backup" || j.Kind == "backup_verify" || j.Kind == "probe" || j.Kind == "drift" || j.Kind == "verify" {
				if previousState == "queued" {
					j.Message = "Nach Dienstneustart beendet: Auftrag war noch in der Warteschlange. Bei Bedarf gezielt erneut starten."
				} else {
					j.Message = "Durch Dienstneustart unterbrochen. Abschluss nicht bestätigt; Sicherungsverlauf prüfen und bei Bedarf erneut starten."
				}
			} else {
				j.Message = "Dienstneustart: tatsächlichen Hostzustand und Hostjournal vor weiteren Schritten prüfen."
			}
			j.Error = j.Message
			j.RetryAt = ""
			j.Revision++
			j.FinishedAt = now()
			if err = s.Store.saveJob(j); err != nil {
				return err
			}
		}
	}
	plans, err := s.Plans()
	if err != nil {
		return err
	}
	for _, p := range plans {
		if p.State == "applying" || p.State == "rolling_back" {
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
	return s.queueAt(hostID, "backup", at, s.backupRunner(hostID))
}
func (s *Service) backupRunner(hostID string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		b, err := s.CreateBackup(ctx, hostID)
		if err != nil {
			// A late failure after publishing a capture must retain its result and
			// must never start another capture in the same retry loop.
			if b.ID != "" {
				if stored, e := s.Backup(b.ID); e == nil && stored.HostID == hostID {
					return b.ID, err
				}
			}
			return "", err
		}
		if b.Status == "partial" {
			issues := backupIssues(b.Warnings)
			if len(issues) > 0 {
				return b.ID, fmt.Errorf("Sicherung unvollständig: %s", issues[0].Title)
			}
			return b.ID, errors.New("Sicherung unvollständig; Erfassung und Prüfergebnisse ansehen")
		}
		return b.ID, err
	}
}
func (s *Service) QueueProbe(hostID string) (Job, error) {
	return s.queue(hostID, "probe", s.probeRunner(hostID))
}
func (s *Service) probeRunner(hostID string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		h, err := s.Host(hostID)
		if err != nil {
			return "", err
		}
		if err := reportJobStage(ctx, "probe", "Host untersuchen", "Verbindung und Systeminventar des Hosts abfragen.", 0, 0); err != nil {
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
		if err == nil {
			if progressErr := reportJobStage(ctx, "catalog", "Inventar speichern", "Erkannte Systemdaten im Katalog aktualisieren.", 0, 0); progressErr != nil {
				return "", progressErr
			}
		}
		saveErr := s.updateHostInventory(h, inventory, probeError)
		if err == nil {
			err = saveErr
		}
		if err != nil {
			return "", err
		}
		return h.ID, nil
	}
}
func (s *Service) QueueApply(id, confirmation string) (Job, error) {
	p, err := s.Plan(id)
	if err != nil {
		return Job{}, err
	}
	if p.SchemaVersion != 2 || len(p.Blockers) > 0 || p.State != "ready" || confirmation != id {
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
var errJobRequestConsumed = errors.New("Auftrag bereits verbucht; Historieneintrag wurde entfernt")

func (s *Service) queueAt(hostID, kind string, at *time.Time, run func(context.Context) (string, error)) (Job, error) {
	return s.queueAtForSet(hostID, kind, at, "", run)
}

func (s *Service) queueAtForSet(hostID, kind string, at *time.Time, setID string, run func(context.Context) (string, error)) (Job, error) {
	return s.queueAtForSetRetry(hostID, kind, at, setID, "", run)
}
func (s *Service) queueAtForSetRetry(hostID, kind string, at *time.Time, setID, retryOf string, run func(context.Context) (string, error)) (Job, error) {
	return s.queueOperation(hostID, kind, at, setID, retryOf, "", run)
}
func (s *Service) queueOperation(hostID, kind string, at *time.Time, setID, retryOf, requestedID string, run func(context.Context) (string, error), checks ...func() error) (Job, error) {
	return s.queueTriggeredOperation(hostID, kind, at, setID, retryOf, requestedID, "manual", run, checks...)
}
func (s *Service) queueTriggeredOperation(hostID, kind string, at *time.Time, setID, retryOf, requestedID, trigger string, run func(context.Context) (string, error), checks ...func() error) (Job, error) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	jobs, err := s.Jobs()
	if err != nil {
		return Job{}, err
	}
	if requestedID != "" {
		if !validID(requestedID) {
			return Job{}, fail(400, "Ungültige Auftragskennung")
		}
		for _, previous := range jobs {
			if previous.ID == requestedID {
				if previous.HostID != hostID || previous.Kind != kind {
					return Job{}, fail(409, "Auftragskennung passt nicht zum Host")
				}
				return previous, nil
			}
		}
	}
	if requestedID != "" {
		var marker map[string]string
		err := s.Store.Get("deleted-job-requests", requestedID, &marker)
		if err == nil {
			if marker["host_id"] != hostID || marker["kind"] != kind {
				return Job{}, fail(409, "Auftragszuordnung widersprüchlich")
			}
			return Job{}, errJobRequestConsumed
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Job{}, err
		}
	}
	if retryOf != "" {
		for _, previous := range jobs {
			if previous.RetryOf == retryOf {
				return previous, nil
			}
		}
	}
	if _, err := s.Host(hostID); err != nil {
		return Job{}, err
	}
	if s.maintenance {
		return Job{}, errors.New("Anker wird aktualisiert; neue Aufträge sind vorübergehend gesperrt")
	}
	if s.reconciling[hostID] {
		return Job{}, fail(409, "Hostjournal wird abgeglichen; Auftrag anschließend starten")
	}
	if s.connectingHostID == hostID {
		return Job{}, fail(409, "Hostzugang wird eingerichtet; Auftrag anschließend starten")
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
		if !h.Enabled || hostMaintenance(h, *at) || local.Before(due) {
			return Job{}, errJobNotDue
		}
		day = local.Format("2006-01-02")
	}
	for _, j := range jobs {
		if j.HostID == hostID && (j.State == "queued" || j.State == "running") {
			return Job{}, errHostJobActive
		}
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return Job{}, err
		}
	}
	jobID := requestedID
	if jobID == "" {
		jobID = ID()
	}
	j := Job{ID: jobID, Revision: 1, RetryOf: retryOf, Message: "Wartet auf einen freien Ausführungsplatz.", SourceSetID: setID, HostID: hostID, Kind: kind, State: "queued", CreatedAt: now(), Trigger: "manual", ScheduledDay: day}
	if day != "" || trigger == "scheduled" {
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
		delete(s.stopReasons, j.ID)
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
			if j.Trigger == "scheduled" {
				h, e := s.Host(j.HostID)
				if e != nil {
					s.jobMu.Unlock()
					j.State, j.FinishedAt, j.Error = "failed", now(), "Hosteinstellungen nicht lesbar"
					s.persistJob(j)
					return
				}
				if !h.Enabled || hostMaintenance(h, time.Now()) || (j.Kind == "drift" && h.DriftMinutes < 15) {
					s.jobMu.Unlock()
					j.State, j.FinishedAt, j.Message = "cancelled", now(), "Automatische Ausführung vor dem Start wegen Hostwartung oder pausierter Automatik beendet."
					s.persistJob(j)
					return
				}
			}
			j.State, j.StartedAt, j.Attempts = "running", now(), 1
			j.MaxAttempts = 1
			if j.Kind == "backup" || j.Kind == "backup_verify" || j.Kind == "probe" || j.Kind == "drift" {
				j.MaxAttempts += settings.Retries
			}
			j.Message = "Hostoperation läuft."
			err = s.saveJobProgress(&j)
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
	if j.Kind == "backup" || j.Kind == "backup_verify" || j.Kind == "probe" || j.Kind == "drift" {
		retries = settings.Retries
	}
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			break
		}
		j.Attempts = attempt + 1
		j.RetryAt = ""
		j.Message = fmt.Sprintf("Versuch %d von höchstens %d läuft.", j.Attempts, j.MaxAttempts)
		if err = s.saveJobProgress(&j); err != nil {
			break
		}
		j.ResultID, err = run(s.jobContext(ctx, &j))
		if err != nil && ctx.Err() == nil {
			finishJobStage(&j, "failed")
		}
		if err == nil || ctx.Err() != nil || j.ResultID != "" || !transientConnection(err) {
			break
		}
		if attempt < retries {
			delay := time.Duration(1<<attempt) * time.Second
			j.LastError = err.Error()
			j.RetryAt = time.Now().UTC().Add(delay).Format(time.RFC3339Nano)
			j.Message = fmt.Sprintf("Verbindung vorübergehend gestört. Versuch %d von höchstens %d folgt automatisch; gleicher Auftrag.", j.Attempts+1, j.MaxAttempts)
			if saveErr := s.saveJobProgress(&j); saveErr != nil {
				err = saveErr
				break
			}
			select {
			case <-ctx.Done():
			case <-time.After(delay):
			}
		}
	}
	j.State = "successful"
	j.RetryAt = ""
	j.Message = fmt.Sprintf("Erfolgreich nach %d Versuch(en) abgeschlossen.", j.Attempts)
	if err != nil {
		j.State = "failed"
		j.Error = err.Error()
		j.Message = "Beendet: Fehler benötigt eine Prüfung; keine automatische Wiederholung."
		if j.ResultID != "" {
			j.Message = "Ergebnis bereits vorhanden; Abschluss nicht vollständig bestätigt. Vor weiteren Sicherungen das Ergebnis prüfen."
		} else if (j.Kind == "backup" || j.Kind == "backup_verify" || j.Kind == "probe" || j.Kind == "drift") && transientConnection(err) {
			j.Message = fmt.Sprintf("Verbindung nach %d Versuchen weiterhin gestört. Wiederholungslimit erreicht.", j.Attempts)
		}
	}
	if ctx.Err() != nil {
		j.State = "cancelled"
		j.Error = "Auftrag abgebrochen; tatsächlichen Zustand prüfen"
		j.Message = "Abgebrochen. Ausstehende Wiederholungen wurden beendet."
	}
	j.FinishedAt = now()
	finishJobStage(&j, j.State)
	if s.persistJob(j) {
		s.notifyJob(j)
	}
}
func (s *Service) saveJobProgress(j *Job) error {
	j.Revision++
	return s.Store.Put("jobs", j.ID, j)
}
func (s *Service) persistJob(j Job) bool {
	s.mu.Lock()
	stopReason := s.stopReasons[j.ID]
	s.mu.Unlock()
	if j.State == "cancelled" && stopReason != "" {
		j.Message = stopReason
		j.Error = stopReason
	}
	j.Revision++
	if finishedJob(j) {
		j.RetryAt = ""
		if j.State == "cancelled" && j.Message == "Wartet auf einen freien Ausführungsplatz." {
			j.Message = "Vor der Ausführung abgebrochen. Es wird kein Wiederholungsversuch gestartet."
		}
		if j.Error != "" && (j.Message == "Wartet auf einen freien Ausführungsplatz." || j.Message == "Hostoperation läuft.") {
			j.Message = j.Error
		}
	}
	if err := s.Store.saveJob(j); err != nil {
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
	minimum, err := PasswordMinimum(v)
	if err != nil {
		return err
	}
	v.PasswordMinLength = minimum
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
	if s.stopReasons == nil {
		s.stopReasons = map[string]string{}
	}
	for id, cancel := range s.cancels {
		s.stopReasons[id] = "Dienst wurde beendet. Auftrag gestoppt; nach Neustart Sicherungsverlauf prüfen. Keine automatische Fortsetzung."
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

// Classification is structural: disk, authorization, host identity and invalid
// content errors must not turn into retries merely because their text matches.
var errTemporaryConnection = errors.New("Vorübergehender SSH-Verbindungsfehler")

func transientConnection(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, errTemporaryConnection) {
		return true
	}
	var op *net.OpError
	if !errors.As(err, &op) {
		return false
	}
	return op.Timeout() || errors.Is(op, syscall.ECONNRESET) || errors.Is(op, syscall.ECONNREFUSED) || errors.Is(op, syscall.EPIPE) || errors.Is(op, syscall.ENETUNREACH) || errors.Is(op, syscall.EHOSTUNREACH)
}
