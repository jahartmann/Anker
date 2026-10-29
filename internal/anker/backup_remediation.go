package anker

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

type RemediationSource struct {
	HostID   string `json:"host_id"`
	BackupID string `json:"backup_id"`
}
type RemediationTarget struct {
	RemediationSource
	HostName string   `json:"host_name"`
	Reasons  []string `json:"reasons"`
	State    string   `json:"state"`
	Message  string   `json:"message,omitempty"`
	JobID    string   `json:"job_id,omitempty"`
}
type RemediationPlan struct {
	Targets    []RemediationTarget `json:"targets"`
	OtherHosts int                 `json:"other_hosts"`
}
type RemediationRequest struct {
	RequestID     string              `json:"request_id"`
	Sources       []RemediationSource `json:"sources"`
	IncludePaused bool                `json:"include_paused"`
}
type RemediationRun struct {
	ID        string              `json:"id"`
	ActorID   string              `json:"actor_id"`
	CreatedAt string              `json:"created_at"`
	Request   RemediationRequest  `json:"request"`
	Targets   []RemediationTarget `json:"targets"`
}

// Classify only the previously diagnosed cases. A fresh capture must still
// supply the corroborating evidence before a warning can disappear.
func knownRemediationReasons(warnings []string) []string {
	found := map[string]bool{}
	for _, warning := range warnings {
		if strings.HasPrefix(warning, "Cannot capture /") {
			parts := strings.SplitN(strings.TrimPrefix(warning, "Cannot capture /"), ": ", 2)
			if len(parts) == 2 && systemdEscapedPath(parts[0]) && ValidPath(parts[0]) == nil && (parts[1] == "invalid path" || parts[1] == "invalid path: backslash is not supported") {
				found["systemd_path"] = true
			}
		}
		match := legacyDependencyWarning.FindStringSubmatch(warning)
		if match == nil {
			continue
		}
		if match[1] == "/etc/lynx/local.cfg" && match[2] == "etc/lynx/lynx.cfg" {
			found["lynx_optional"] = true
		}
		if match[1] == `/SYS\n` && strings.HasPrefix(match[2], "etc/powerman/") && strings.HasSuffix(match[2], ".dev") && !strings.Contains(strings.TrimPrefix(match[2], "etc/powerman/"), "/") {
			found["powerman_command"] = true
		}
	}
	out := []string{}
	for _, code := range []string{"systemd_path", "lynx_optional", "powerman_command"} {
		if found[code] {
			out = append(out, code)
		}
	}
	return out
}
func (s *Service) BackupRemediationPlan() (RemediationPlan, error) {
	plan := RemediationPlan{Targets: []RemediationTarget{}}
	hosts, err := s.Hosts()
	if err != nil {
		return plan, err
	}
	backups, err := s.ListBackups("")
	if err != nil {
		return plan, err
	}
	jobs, err := s.Jobs()
	if err != nil {
		return plan, err
	}
	latest := map[string]Backup{}
	for _, b := range backups {
		previous, ok := latest[b.HostID]
		if !ok || b.CreatedAt > previous.CreatedAt || (b.CreatedAt == previous.CreatedAt && b.ID > previous.ID) {
			latest[b.HostID] = b
		}
	}
	for _, host := range hosts {
		b, ok := latest[host.ID]
		if !ok {
			continue
		}
		reasons := knownRemediationReasons(b.Warnings)
		if len(reasons) == 0 {
			if b.Status != "successful" || len(b.Warnings) > 0 || b.VerificationError != "" {
				plan.OtherHosts++
			}
			continue
		}
		target := RemediationTarget{RemediationSource: RemediationSource{HostID: host.ID, BackupID: b.ID}, HostName: host.Name, Reasons: reasons, State: "ready"}
		if !host.Enabled {
			target.State = "paused"
			target.Message = "Automatischer Zeitplan pausiert; nur mit ausdrücklicher Einbeziehung starten."
		}
		if _, real := s.Collector.(SSHCollector); real && (host.KeyPath == "" || host.KnownHostsPath == "" || host.SSHUser == "") {
			target.State = "connection_missing"
			target.Message = "Gespeicherter SSH-Zugang ist unvollständig; es wird kein neuer Zugang eingerichtet."
		}
		for _, job := range jobs {
			if job.HostID == host.ID && activeJob(job) {
				target.State = "busy"
				target.JobID = job.ID
				target.Message = "Ein Auftrag ist bereits aktiv; kein zweiter Start."
				break
			}
		}
		plan.Targets = append(plan.Targets, target)
	}
	sort.Slice(plan.Targets, func(i, j int) bool { return plan.Targets[i].HostName < plan.Targets[j].HostName })
	return plan, nil
}
func remediationRunID(actor, request string) string {
	return Hash([]byte("backup-remediation\x00" + actor + "\x00" + request))[:24]
}
func remediationJobID(run, host string) string {
	return Hash([]byte("backup-remediation-job\x00" + run + "\x00" + host))[:24]
}
func validRemediationRun(run RemediationRun) bool {
	if run.ActorID == "" || !validID(run.Request.RequestID) || run.ID != remediationRunID(run.ActorID, run.Request.RequestID) || len(run.Targets) == 0 || len(run.Targets) > 1000 || len(run.Targets) != len(run.Request.Sources) {
		return false
	}
	seen := map[string]bool{}
	for i, target := range run.Targets {
		if !validID(target.HostID) || !validID(target.BackupID) || seen[target.HostID] || target.RemediationSource != run.Request.Sources[i] {
			return false
		}
		seen[target.HostID] = true
		switch target.State {
		case "pending", "ready", "paused", "busy", "connection_missing", "error", "skipped":
		case "queued":
			if target.JobID != remediationJobID(run.ID, target.HostID) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (s *Service) BackupRemediationRun(actor, request string) (RemediationRun, error) {
	var run RemediationRun
	if !validID(request) {
		return run, fail(400, "Ungültige Sammelauftragskennung")
	}
	err := s.Store.Get("backup_remediation", remediationRunID(actor, request), &run)
	if errors.Is(err, sql.ErrNoRows) {
		return run, fail(404, "Sammelauftrag noch nicht angelegt")
	}
	if err == nil && (!validRemediationRun(run) || run.ActorID != actor || run.Request.RequestID != request) {
		return RemediationRun{}, fail(409, "Sammelauftrag passt nicht zur Anmeldung")
	}
	return run, err
}
func (s *Service) remediationRunner(hostID string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		result, captureErr := s.backupRunner(hostID)(ctx)
		if result == "" {
			return result, captureErr
		}
		if err := reportJobStage(ctx, "integrity", "Neue Sicherung verifizieren", "Manifest und tatsächlich gespeicherte Sicherungsdateien auf Integrität prüfen.", 0, 0); err != nil {
			return result, errors.Join(captureErr, err)
		}
		verificationErr := s.VerifyBackup(result)
		if verificationErr == nil {
			// A partial capture can pass byte integrity. Preserve that successful check
			// and attribute remaining warnings to the completion verdict instead.
			if err := reportJobStage(ctx, "completion", "Abschluss bestätigen", "Erfassungsumfang und Integritätsnachweis gemeinsam auswerten.", 0, 0); err != nil {
				return result, errors.Join(captureErr, err)
			}
		}
		return result, errors.Join(captureErr, verificationErr)
	}
}

var errRemediationChanged = errors.New("Sicherungsstand inzwischen geändert")
var errRemediationPaused = errors.New("Zeitplan inzwischen pausiert")

// Runs under jobMu after checking all active jobs. A finished job published its
// backup before its terminal state, so this last read also closes the window
// between an older preview and a concurrently finishing capture.
func (s *Service) remediationAdmission(source RemediationSource, includePaused bool) func() error {
	return func() error {
		host, err := s.Host(source.HostID)
		if err != nil {
			return err
		}
		if !host.Enabled && !includePaused {
			return errRemediationPaused
		}
		backups, err := s.ListBackups(source.HostID)
		if err != nil {
			return err
		}
		var latest Backup
		for _, backup := range backups {
			if latest.ID == "" || backup.CreatedAt > latest.CreatedAt || (backup.CreatedAt == latest.CreatedAt && backup.ID > latest.ID) {
				latest = backup
			}
		}
		if latest.ID != source.BackupID || len(knownRemediationReasons(latest.Warnings)) == 0 {
			return errRemediationChanged
		}
		return nil
	}
}

func (s *Service) StartBackupRemediation(actor string, request RemediationRequest) (RemediationRun, error) {
	var run RemediationRun
	if actor == "" || !validID(request.RequestID) || len(request.Sources) == 0 || len(request.Sources) > 1000 {
		return run, fail(400, "Sammelauftrag benötigt eine Kennung und 1 bis 1000 betroffene Hosts")
	}
	seen := map[string]bool{}
	for _, source := range request.Sources {
		if !validID(source.HostID) || !validID(source.BackupID) || seen[source.HostID] {
			return run, fail(400, "Ungültige oder doppelte Hostauswahl")
		}
		seen[source.HostID] = true
	}
	s.remediationMu.Lock()
	defer s.remediationMu.Unlock()
	id := remediationRunID(actor, request.RequestID)
	err := s.Store.Get("backup_remediation", id, &run)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return run, err
	}
	if err == nil {
		if !validRemediationRun(run) {
			return run, fail(409, "Gespeicherter Sammelauftrag ist unvollständig oder widersprüchlich; es wird nichts gestartet")
		}
		if run.ActorID != actor || run.Request.RequestID != request.RequestID || run.Request.IncludePaused != request.IncludePaused || len(run.Request.Sources) != len(request.Sources) {
			return run, fail(409, "Kennung wurde bereits für eine andere Auswahl verwendet")
		}
		for i, source := range request.Sources {
			if run.Request.Sources[i] != source {
				return run, fail(409, "Kennung wurde bereits für eine andere Auswahl verwendet")
			}
		}
	} else {
		run = RemediationRun{ID: id, ActorID: actor, CreatedAt: now(), Request: request, Targets: []RemediationTarget{}}
		for _, source := range request.Sources {
			run.Targets = append(run.Targets, RemediationTarget{RemediationSource: source, Reasons: []string{}, State: "pending"})
		}
		if err := s.Store.Put("backup_remediation", id, run); err != nil {
			return run, err
		}
	}
	// A durable request and deterministic job IDs make a lost reply or a process
	// restart between queueing and saving the receipt safe to reconcile.
	for i := range run.Targets {
		target := &run.Targets[i]
		if target.State != "pending" {
			continue
		}
		jobID := remediationJobID(id, target.HostID)
		if existing, e := s.Job(jobID); e == nil {
			if existing.HostID != target.HostID || existing.Kind != "backup" {
				return run, fail(409, "Auftragskennung kollidiert")
			}
			target.State, target.JobID = "queued", existing.ID
		} else if !errors.Is(e, sql.ErrNoRows) {
			return run, e
		} else {
			plan, e := s.BackupRemediationPlan()
			if e != nil {
				return run, e
			}
			var current *RemediationTarget
			for index := range plan.Targets {
				if plan.Targets[index].HostID == target.HostID {
					current = &plan.Targets[index]
					break
				}
			}
			if current == nil || current.BackupID != target.BackupID {
				target.State = "skipped"
				target.Message = "Der aktuelle Sicherungsstand hat sich geändert oder benötigt diese Korrektur nicht mehr."
			} else {
				*target = *current
				if target.State == "ready" || (target.State == "paused" && request.IncludePaused) {
					job, e := s.queueOperation(target.HostID, "backup", nil, "", "", jobID, s.remediationRunner(target.HostID), s.remediationAdmission(target.RemediationSource, request.IncludePaused))
					if e == nil {
						target.State, target.JobID, target.Message = "queued", job.ID, "Neue Erfassung und anschließende Integritätsprüfung eingereiht."
					} else if errors.Is(e, errRemediationChanged) {
						target.State = "skipped"
						target.Message = "Inzwischen liegt ein anderer Sicherungsstand vor; kein erneuter Start."
					} else if errors.Is(e, errRemediationPaused) {
						target.State = "paused"
						target.Message = "Der Zeitplan wurde inzwischen pausiert; kein automatischer Start."
					} else if errors.Is(e, errHostJobActive) {
						target.State = "busy"
						target.Message = "Inzwischen ist ein anderer Auftrag aktiv; kein zweiter Start."
					} else {
						target.State = "error"
						target.Message = "Auftrag konnte nicht eingereiht werden; Dienststatus, Katalog oder Wartung prüfen."
					}
				}
			}
		}
		if err := s.Store.Put("backup_remediation", id, run); err != nil {
			return run, err
		}
	}
	return run, nil
}
