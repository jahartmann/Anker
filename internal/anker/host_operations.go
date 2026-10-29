package anker

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

func hostMaintenance(h Host, at time.Time) bool {
	if h.MaintenanceUntil == "" {
		return false
	}
	until, err := time.Parse(time.RFC3339Nano, h.MaintenanceUntil)
	return err != nil || until.After(at)
}
func hostRevision(h Host) string {
	raw, _ := json.Marshal(struct {
		ID, Name, Address, Group, Cluster, User, Key, Trust, RestoreUser, RestoreKey, Schedule, Maintenance string
		Port, Drift                                                                                         int
		Enabled                                                                                             bool
		Extra                                                                                               []string
	}{h.ID, h.Name, h.Address, h.Group, h.ClusterID, h.SSHUser, h.KeyPath, h.KnownHostsPath, h.RestoreSSHUser, h.RestoreKeyPath, h.Schedule, h.MaintenanceUntil, h.SSHPort, h.DriftMinutes, h.Enabled, h.ExtraPaths})
	return Hash(raw)
}

type HostOperationTarget struct {
	HostID   string `json:"host_id"`
	Revision string `json:"revision"`
	Name     string `json:"name,omitempty"`
	State    string `json:"state,omitempty"`
	Message  string `json:"message,omitempty"`
	JobID    string `json:"job_id,omitempty"`
}
type HostOperationRequest struct {
	RequestID string                `json:"request_id"`
	Action    string                `json:"action"`
	Targets   []HostOperationTarget `json:"targets"`
	Value     string                `json:"value,omitempty"`
	Until     string                `json:"until,omitempty"`
	Minutes   int                   `json:"minutes,omitempty"`
}
type HostOperationRun struct {
	ID        string                `json:"id"`
	ActorID   string                `json:"actor_id"`
	CreatedAt string                `json:"created_at"`
	Request   HostOperationRequest  `json:"request"`
	Targets   []HostOperationTarget `json:"targets"`
}

func hostOperationID(actor, nonce string) string {
	return Hash([]byte("host-operation\x00" + actor + "\x00" + nonce))[:24]
}
func operationalRead(action string) bool {
	return action == "backup" || action == "probe" || action == "verify" || action == "drift"
}
func (s *Service) HostOperationRun(actor, nonce string) (HostOperationRun, error) {
	var run HostOperationRun
	if !validID(nonce) {
		return run, fail(400, "Ungültige Sammelkennung")
	}
	err := s.Store.Get("host_operations", hostOperationID(actor, nonce), &run)
	if errors.Is(err, sql.ErrNoRows) {
		return run, fail(404, "Sammelauftrag nicht gefunden")
	}
	if err == nil && (run.ActorID != actor || run.ID != hostOperationID(actor, nonce) || run.Request.RequestID != nonce) {
		return HostOperationRun{}, fail(409, "Sammelauftrag widersprüchlich")
	}
	return run, err
}
func (s *Service) StartHostOperation(actor string, request HostOperationRequest) (HostOperationRun, error) {
	var run HostOperationRun
	if actor == "" || !validID(request.RequestID) || len(request.Targets) == 0 || len(request.Targets) > 1000 {
		return run, fail(400, "1 bis 1000 Hosts und eine Sammelkennung erforderlich")
	}
	switch request.Action {
	case "backup", "probe", "verify", "drift", "schedule", "group", "maintenance", "maintenance_end", "pause", "resume", "drift_schedule":
	default:
		return run, fail(400, "Unbekannte Sammelaktion")
	}
	if request.Action == "schedule" && request.Value != "" {
		if _, err := time.Parse("15:04", request.Value); err != nil {
			return run, fail(400, "Zeitplan benötigt HH:MM")
		}
	}
	if request.Action == "group" && len(request.Value) > 200 {
		return run, fail(400, "Gruppenname zu lang")
	}
	if request.Action == "drift_schedule" && request.Minutes != 0 && (request.Minutes < 15 || request.Minutes > 1440) {
		return run, fail(400, "Änderungsprüfung: aus oder 15 bis 1440 Minuten")
	}
	seen := map[string]bool{}
	for i := range request.Targets {
		request.Targets[i] = HostOperationTarget{HostID: request.Targets[i].HostID, Revision: request.Targets[i].Revision}
		target := request.Targets[i]
		if !validID(target.HostID) || len(target.Revision) != 64 || seen[target.HostID] {
			return run, fail(400, "Hostauswahl ungültig oder doppelt")
		}
		seen[target.HostID] = true
	}
	s.operationsMu.Lock()
	defer s.operationsMu.Unlock()
	id := hostOperationID(actor, request.RequestID)
	err := s.Store.Get("host_operations", id, &run)
	if err == nil {
		if !validHostOperation(run) {
			return run, fail(409, "Gespeicherte Sammelquittung ist widersprüchlich")
		}
		a, _ := json.Marshal(run.Request)
		b, _ := json.Marshal(request)
		if run.ID != id || run.ActorID != actor || string(a) != string(b) || len(run.Targets) != len(request.Targets) {
			return run, fail(409, "Kennung bereits anders verwendet oder Quittung beschädigt")
		}
		for i, target := range run.Targets {
			if target.HostID != request.Targets[i].HostID || target.Revision != request.Targets[i].Revision {
				return run, fail(409, "Gespeicherte Auswahl widersprüchlich")
			}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return run, err
	} else {
		if request.Action == "maintenance" {
			until, err := time.Parse(time.RFC3339Nano, request.Until)
			if err != nil || !until.After(time.Now()) || until.After(time.Now().Add(30*24*time.Hour)) {
				return run, fail(400, "Wartungsende muss in den nächsten 30 Tagen liegen")
			}
		}
		run = HostOperationRun{ID: id, ActorID: actor, CreatedAt: now(), Request: request, Targets: append([]HostOperationTarget(nil), request.Targets...)}
		for i := range run.Targets {
			run.Targets[i].State = "pending"
			run.Targets[i].JobID = ""
			run.Targets[i].Message = ""
		}
		if err := s.Store.Put("host_operations", id, run); err != nil {
			return run, err
		}
	}
	if !operationalRead(request.Action) {
		return s.applyHostSettings(run)
	}
	for i := range run.Targets {
		item := &run.Targets[i]
		if item.State != "pending" {
			continue
		}
		jobID := Hash([]byte("host-operation-job\x00" + id + "\x00" + item.HostID))[:24]
		if j, e := s.Job(jobID); e == nil {
			if j.HostID != item.HostID {
				return run, fail(409, "Auftragszuordnung widersprüchlich")
			}
			item.State, item.JobID = "queued", j.ID
		} else if !errors.Is(e, sql.ErrNoRows) {
			return run, e
		} else {
			h, e := s.Host(item.HostID)
			if e != nil {
				item.State, item.Message = "skipped", "Host nicht mehr vorhanden"
			} else if hostRevision(h) != item.Revision {
				item.State, item.Message = "skipped", "Hosteinstellungen inzwischen geändert; Auswahl neu laden"
			} else {
				item.Name = h.Name
				kind := request.Action
				var runner func(context.Context) (string, error)
				switch request.Action {
				case "backup":
					kind = "backup_verify"
					runner = s.remediationRunner(h.ID)
				case "probe":
					runner = s.probeRunner(h.ID)
				case "drift":
					runner = s.driftRunner(h.ID)
				case "verify":
					backups, e := s.ListBackups(h.ID)
					if e != nil {
						return run, e
					}
					if len(backups) == 0 {
						item.State, item.Message = "skipped", "Keine Sicherung zum Prüfen vorhanden"
						break
					}
					backupID := backups[0].ID
					runner = func(ctx context.Context) (string, error) {
						if err := reportJobStage(ctx, "integrity", "Sicherung verifizieren", "Gespeicherte Bytes und Manifest prüfen.", 0, 0); err != nil {
							return backupID, err
						}
						return backupID, s.VerifyBackup(backupID)
					}
				}
				if runner != nil {
					guard := func() error {
						current, e := s.Host(item.HostID)
						if e != nil {
							return e
						}
						if hostRevision(current) != item.Revision {
							return errRemediationChanged
						}
						return nil
					}
					j, e := s.queueOperation(item.HostID, kind, nil, "", "", jobID, runner, guard)
					if e == nil {
						item.State, item.JobID = "queued", j.ID
					} else if errors.Is(e, errJobRequestConsumed) {
						item.State, item.Message = "skipped", errJobRequestConsumed.Error()
					} else if errors.Is(e, errHostJobActive) {
						item.State, item.Message = "skipped", "Auftrag bereits aktiv"
					} else {
						item.State, item.Message = "skipped", "Nicht gestartet; Einstellungen oder Dienststatus inzwischen geändert"
					}
				}
			}
		}
		if err := s.Store.Put("host_operations", id, run); err != nil {
			return run, err
		}
	}
	return run, nil
}
func (s *Service) applyHostSettings(run HostOperationRun) (HostOperationRun, error) {
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.maintenance || s.connectingHostID != "" {
		return run, fail(409, "Systemänderung oder Hostanbindung läuft")
	}
	updates := map[string]Host{}
	for i := range run.Targets {
		target := &run.Targets[i]
		if target.State != "pending" {
			continue
		}
		h, err := s.Host(target.HostID)
		if err != nil || hostRevision(h) != target.Revision {
			target.State, target.Message = "skipped", "Host fehlt oder Einstellungen geändert"
			continue
		}
		if run.Request.Action == "maintenance" {
			until, e := time.Parse(time.RFC3339Nano, run.Request.Until)
			if e != nil || !until.After(time.Now()) {
				target.State, target.Message = "skipped", "Wartungszeitraum ist inzwischen abgelaufen"
				continue
			}
		}
		target.Name = h.Name
		switch run.Request.Action {
		case "schedule":
			h.Schedule = run.Request.Value
		case "group":
			h.Group = run.Request.Value
		case "maintenance":
			h.MaintenanceUntil = run.Request.Until
		case "maintenance_end":
			h.MaintenanceUntil = ""
		case "pause":
			h.Enabled = false
		case "resume":
			h.Enabled = true
		case "drift_schedule":
			h.DriftMinutes = run.Request.Minutes
		}
		if _, err := prepareHost(h); err != nil {
			return run, err
		}
		h.ConfigRevision = ""
		updates[h.ID] = h
		target.State = "applied"
		target.Message = "Einstellungen übernommen"
	}
	tx, err := s.Store.db.Begin()
	if err != nil {
		return run, err
	}
	defer tx.Rollback()
	for _, h := range updates {
		if err := txPut(tx, "hosts", h.ID, h); err != nil {
			return run, err
		}
	}
	if err := txPut(tx, "host_operations", run.ID, run); err != nil {
		return run, err
	}
	if err := tx.Commit(); err != nil {
		return run, err
	}
	return run, nil
}

func (s *Service) QueueDrift(hostID string) (Job, error) {
	return s.queue(hostID, "drift", s.driftRunner(hostID))
}
func (s *Service) queueDriftDue(h Host, at time.Time) (Job, error) {
	guard := func() error {
		current, err := s.Host(h.ID)
		if err != nil {
			return err
		}
		if !current.Enabled || hostMaintenance(current, at) || current.DriftMinutes < 15 {
			return errJobNotDue
		}
		if last, err := time.Parse(time.RFC3339Nano, current.LastProbe); err == nil && !last.After(at.Add(5*time.Minute)) && at.Sub(last) < time.Duration(current.DriftMinutes)*time.Minute {
			return errJobNotDue
		}
		return nil
	}
	return s.queueTriggeredOperation(h.ID, "drift", nil, "", "", "", "scheduled", s.driftRunner(h.ID), guard)
}

type DriftFile struct {
	Path  string `json:"path"`
	State string `json:"state"`
}
type DriftReport struct {
	State    string      `json:"state"`
	HostID   string      `json:"host_id"`
	BackupID string      `json:"backup_id,omitempty"`
	At       string      `json:"at,omitempty"`
	Files    []DriftFile `json:"files"`
	Unknown  int         `json:"unknown"`
	Detail   string      `json:"detail"`
}

func (s *Service) HostDrift(hostID string) (DriftReport, error) {
	h, err := s.Host(hostID)
	if err != nil {
		return DriftReport{}, err
	}
	backups, err := s.ListBackups(hostID)
	if err != nil {
		return DriftReport{}, err
	}
	return s.hostDrift(h, backups)
}
func (s *Service) hostDrift(h Host, backups []Backup) (DriftReport, error) {
	r := DriftReport{HostID: h.ID, State: "unknown", Files: []DriftFile{}, Detail: "Aktuelle Hostprüfung und vollständiger Vergleichsstand erforderlich."}
	if h.Inventory == nil || h.Inventory.Hostname == "" || h.Inventory.ProductVersion() == "" || h.LastProbe == "" || h.ProbeError != "" {
		return r, nil
	}
	var source *Backup
	for i := range backups {
		if backups[i].Status == "successful" && len(backups[i].Warnings) == 0 && backups[i].VerificationError == "" {
			source = &backups[i]
			break
		}
	}
	if source == nil {
		return r, nil
	}
	r.BackupID, r.At = source.ID, h.Inventory.CapturedAt
	probeAt, e1 := time.Parse(time.RFC3339Nano, h.Inventory.CapturedAt)
	m, err := s.Manifest(source.ID)
	if err != nil {
		return r, err
	}
	if m.Inventory.Hostname != h.Inventory.Hostname || m.Inventory.Platform() != h.Inventory.Platform() {
		r.Detail = "Hostidentität oder Servertyp weicht vom Sicherungsstand ab; neue Referenzsicherung erforderlich."
		return r, nil
	}
	backupAt, e2 := time.Parse(time.RFC3339Nano, m.CompletedAt)
	if e1 != nil || e2 != nil || probeAt.After(time.Now().Add(5*time.Minute)) {
		r.Detail = "Zeitangaben sind unplausibel; Hostuhr und Referenzzeit prüfen."
		return r, nil
	}
	if !probeAt.After(backupAt) {
		r.Detail = "Noch keine neuere lesende Hostprüfung nach diesem Sicherungsstand."
		return r, nil
	}
	var hashes map[string]string
	var queries map[string]struct {
		State string `json:"state"`
	}
	if json.Unmarshal(h.Inventory.Details["file_hashes"], &hashes) != nil || json.Unmarshal(h.Inventory.Details["command_results"], &queries) != nil || queries["file_inventory"].State != "ok" {
		r.Detail = "Die Live-Dateiinventarisierung ist unvollständig; keine Entwarnung möglich."
		return r, nil
	}
	if h.Inventory.Platform() == "pbs" && queries["pbs_datastore_identity"].State != "ok" {
		r.Detail = "Datastore-Abgrenzung wurde nicht vollständig bestätigt."
		return r, nil
	}
	inventoryAt, e := time.Parse(time.RFC3339Nano, h.LastProbe)
	if e != nil || inventoryAt.After(time.Now().Add(5*time.Minute)) {
		return r, nil
	}
	if time.Since(inventoryAt) > time.Duration(max(60, h.DriftMinutes*2))*time.Minute || time.Since(probeAt) > time.Duration(max(60, h.DriftMinutes*2))*time.Minute {
		r.State = "stale"
		r.Detail = "Die letzte Liveprüfung ist veraltet; erneut prüfen."
		return r, nil
	}
	seen := map[string]bool{}
	for _, entry := range m.Entries {
		if entry.Type == "directory" || excludedDriftPath(entry.Path, m.Inventory, *h.Inventory) {
			continue
		}
		seen[entry.Path] = true
		if !(strings.HasPrefix(entry.Path, "etc/") || strings.HasPrefix(entry.Path, "usr/local/")) {
			r.Unknown++
			continue
		}
		live, exists := hashes[entry.Path]
		if !exists {
			r.Files = append(r.Files, DriftFile{Path: entry.Path, State: "removed"})
			continue
		}
		expected := entry.SHA256
		if entry.Type == "symlink" {
			expected = "symlink:" + entry.Link
		}
		if !validDriftHash(live) {
			r.Unknown++
			continue
		}
		if live != expected {
			r.Files = append(r.Files, DriftFile{Path: entry.Path, State: "changed"})
		}
	}
	// Added paths are considered only under explicitly captured profile roots.
	roots := []string{}
	if m.Profile != nil {
		roots = m.Profile.Roots
	} else {
		r.Unknown++
	}
	for path, hash := range hashes {
		if seen[path] || excludedDriftPath(path, m.Inventory, *h.Inventory) {
			continue
		}
		for _, root := range roots {
			base := stringsTrimRoot(root)
			if path == base || len(path) > len(base) && path[:len(base)+1] == base+"/" {
				if ValidPath(path) == nil {
					if validDriftHash(hash) {
						r.Files = append(r.Files, DriftFile{Path: path, State: "added"})
					} else {
						r.Unknown++
					}
				}
				break
			}
		}
	}
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Path < r.Files[j].Path })
	r.State = "unchanged"
	r.Detail = "Keine Inhaltsänderung innerhalb des erfassten Konfigurationsumfangs beobachtet."
	if len(r.Files) > 0 {
		r.State = "changed"
		r.Detail = fmt.Sprintf("%d Konfigurationseinträge seit der Sicherung geändert; Liveprüfung vom %s.", len(r.Files), r.At)
	}
	if r.Unknown > 0 {
		r.State = "partial"
		r.Detail = fmt.Sprintf("%d Änderungen, %d nicht vergleichbare Einträge; keine vollständige Entwarnung.", len(r.Files), r.Unknown)
	}
	return r, nil
}
func stringsTrimRoot(root string) string {
	for len(root) > 0 && root[0] == '/' {
		root = root[1:]
	}
	for len(root) > 0 && root[len(root)-1] == '/' {
		root = root[:len(root)-1]
	}
	return root
}

func validDriftHash(hash string) bool {
	if strings.HasPrefix(hash, "symlink:") {
		return true
	}
	raw, err := hex.DecodeString(hash)
	return err == nil && len(raw) == 32
}
func excludedDriftPath(path string, inventories ...Inventory) bool {
	if ValidPath(path) != nil || strings.Contains("/"+path+"/", "/.chunks/") {
		return true
	}
	for _, inv := range inventories {
		for _, store := range inv.PBSDatastores {
			root := strings.Trim(store.Path, "/")
			if root != "" && (path == root || strings.HasPrefix(path, root+"/")) {
				return true
			}
		}
	}
	return false
}
func (s *Service) verificationRunner(id string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		if err := reportJobStage(ctx, "integrity", "Sicherung verifizieren", "Gespeicherte Bytes und Manifest prüfen.", 0, 0); err != nil {
			return id, err
		}
		return id, s.VerifyBackup(id)
	}
}
func validHostOperation(run HostOperationRun) bool {
	if !validID(run.Request.RequestID) || run.ActorID == "" || run.ID != hostOperationID(run.ActorID, run.Request.RequestID) || len(run.Targets) == 0 || len(run.Targets) > 1000 || len(run.Targets) != len(run.Request.Targets) {
		return false
	}
	for i, t := range run.Targets {
		if !validID(t.HostID) || len(t.Revision) != 64 || t.HostID != run.Request.Targets[i].HostID || t.Revision != run.Request.Targets[i].Revision {
			return false
		}
		switch t.State {
		case "pending", "queued", "applied", "skipped":
		default:
			return false
		}
	}
	return true
}

func (s *Service) driftRunner(hostID string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		id, e := s.probeRunner(hostID)(ctx)
		if e != nil {
			return id, e
		}
		if e = reportJobStage(ctx, "live_compare", "Livezustand vergleichen", "Aktuelles Dateiinventar mit der letzten vollständigen Konfigurationssicherung vergleichen.", 0, 0); e != nil {
			return id, e
		}
		report, e := s.HostDrift(hostID)
		if e != nil {
			return id, e
		}
		return id, reportJobStage(ctx, "live_result", "Vergleich ausgewertet", report.Detail, 0, 0)
	}
}
