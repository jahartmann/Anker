package anker

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

type IncidentHost struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	BackupID string `json:"backup_id,omitempty"`
	Muted    bool   `json:"muted"`
}
type Incident struct {
	ID             string         `json:"id"`
	Code           string         `json:"code"`
	Title          string         `json:"title"`
	Detail         string         `json:"detail"`
	Action         string         `json:"action"`
	Path           string         `json:"path,omitempty"`
	State          string         `json:"state"`
	FirstSeen      string         `json:"first_seen"`
	LastSeen       string         `json:"last_seen"`
	ResolvedAt     string         `json:"resolved_at,omitempty"`
	AcknowledgedBy string         `json:"acknowledged_by,omitempty"`
	ReopenCount    int            `json:"reopen_count"`
	Hosts          []IncidentHost `json:"hosts"`
}

// Reconciliation only closes a cause when every previously affected, still
// monitored host has newer successful evidence. Pausing is never a repair.
func (s *Service) Incidents() ([]Incident, error) {
	s.incidentsMu.Lock()
	defer s.incidentsMu.Unlock()
	hosts, e := s.Hosts()
	if e != nil {
		return nil, e
	}
	backups, e := s.ListBackups("")
	if e != nil {
		return nil, e
	}
	jobs, e := s.Jobs()
	if e != nil {
		return nil, e
	}
	settings, e := s.Settings()
	if e != nil {
		return nil, e
	}
	staleHours := settings.StaleHours
	if staleHours < 1 {
		staleHours = 26
	}
	previous, e := records[Incident](s.Store, "incidents")
	if e != nil {
		return nil, e
	}
	current := map[string]*Incident{}
	hostMap := map[string]Host{}
	latest := map[string]Backup{}
	backupsByHost := map[string][]Backup{}
	busy := map[string]bool{}
	for _, j := range jobs {
		if activeJob(j) {
			busy[j.HostID] = true
		}
	}
	for _, b := range backups {
		backupsByHost[b.HostID] = append(backupsByHost[b.HostID], b)
		if _, ok := latest[b.HostID]; !ok {
			latest[b.HostID] = b
		}
	}
	add := func(h Host, b Backup, issue BackupIssue) {
		key := Hash([]byte(issue.Code + "\x00" + issue.Path + "\x00" + issue.Title))[:24]
		v := current[key]
		if v == nil {
			v = &Incident{ID: key, Code: issue.Code, Title: issue.Title, Detail: issue.Detail, Action: issue.Action, Path: issue.Path, Hosts: []IncidentHost{}}
			current[key] = v
		}
		for _, old := range v.Hosts {
			if old.ID == h.ID {
				return
			}
		}
		v.Hosts = append(v.Hosts, IncidentHost{ID: h.ID, Name: h.Name, BackupID: b.ID, Muted: !h.Enabled || hostMaintenance(h, time.Now())})
	}
	for _, h := range hosts {
		hostMap[h.ID] = h
		b, exists := latest[h.ID]
		if !exists {
			if h.Enabled {
				add(h, b, BackupIssue{Code: "backup_missing", Title: "Noch keine Sicherung", Detail: "Für diesen Host fehlt ein Konfigurationsstand.", Action: "Erste Sicherung starten und verifizieren."})
			}
			continue
		}
		if b.Status == "damaged" || b.VerificationError != "" {
			add(h, b, BackupIssue{Code: "integrity", Title: "Integritätsprüfung fehlgeschlagen", Detail: "Gespeicherter Stand benötigt eine erneute Prüfung.", Action: "Sicherung verifizieren; bei bestätigtem Schaden neu sichern."})
		}
		for _, issue := range backupIssues(b.Warnings) {
			add(h, b, issue)
		}
		if b.Status != "successful" && len(b.Warnings) == 0 && b.Status != "damaged" {
			add(h, b, BackupIssue{Code: "backup_incomplete", Title: "Sicherung unvollständig", Detail: "Der letzte Erfassungsversuch ist nicht vollständig.", Action: "Auftrag prüfen und erneut sichern."})
		}
		if h.ProbeError != "" {
			add(h, b, BackupIssue{Code: "probe", Title: "Hostprüfung fehlgeschlagen", Detail: "Der aktuelle Hostzustand konnte nicht bestätigt werden.", Action: "SSH-Verbindung prüfen und Host erneut untersuchen."})
		}
		if h.DriftMinutes > 0 {
			if drift, err := s.hostDrift(h, backupsByHost[h.ID]); err == nil && len(drift.Files) > 0 {
				add(h, b, BackupIssue{Code: "live_changes", Title: "Ungesicherte Konfigurationsänderungen", Detail: "Die lesende Liveprüfung hat Inhaltsänderungen gegenüber der letzten vollständigen Sicherung erkannt.", Action: "Änderungen beim Host prüfen und anschließend sichern und verifizieren."})
			}
		}
		at, err := time.Parse(time.RFC3339Nano, b.CreatedAt)
		if h.Enabled && (err != nil || time.Since(at) > time.Duration(staleHours)*time.Hour) {
			add(h, b, BackupIssue{Code: "backup_overdue", Title: "Sicherung überfällig", Detail: "Der letzte Stand ist älter als die eingestellte Frist oder besitzt keine gültige Zeitangabe.", Action: "Zeitplan prüfen und neuen Stand sichern."})
		}
	}
	oldMap := map[string]Incident{}
	for _, old := range previous {
		oldMap[old.ID] = old
	}
	for id, v := range current {
		old, ok := oldMap[id]
		v.FirstSeen = old.FirstSeen
		v.LastSeen = old.LastSeen
		v.ReopenCount = old.ReopenCount
		v.AcknowledgedBy = old.AcknowledgedBy
		if v.FirstSeen == "" {
			v.FirstSeen = now()
		}
		v.State = "new"
		if old.State == "resolved" || old.State == "unmonitored" {
			v.ReopenCount++
			v.AcknowledgedBy = ""
			v.State = "reopened"
		} else if old.State == "reopened" {
			v.State = "reopened"
		}
		if v.AcknowledgedBy != "" {
			v.State = "acknowledged"
		}
		for _, h := range v.Hosts {
			if busy[h.ID] {
				v.State = "in_progress"
				break
			}
		}
		for _, affected := range old.Hosts {
			present := false
			for _, h := range v.Hosts {
				if h.ID == affected.ID {
					present = true
					break
				}
			}
			if !present {
				if h, exists := hostMap[affected.ID]; exists && !s.incidentHostProven(old, affected, h, latest[h.ID]) {
					affected.Name = h.Name
					affected.Muted = !h.Enabled || hostMaintenance(h, time.Now())
					v.Hosts = append(v.Hosts, affected)
				}
			}
		}
		sort.Slice(v.Hosts, func(i, j int) bool { return v.Hosts[i].ID < v.Hosts[j].ID })
		before, _ := json.Marshal(old.Hosts)
		after, _ := json.Marshal(v.Hosts)
		if !ok || string(before) != string(after) || v.State != old.State {
			v.LastSeen = now()
		}
	}
	for id, old := range oldMap {
		if current[id] != nil {
			continue
		}
		v := old
		allProven := true
		remaining := false
		for _, affected := range old.Hosts {
			h, exists := hostMap[affected.ID]
			if !exists {
				allProven = false
				continue
			}
			remaining = true
			if !s.incidentHostProven(old, affected, h, latest[h.ID]) {
				allProven = false
			}
		}
		if old.State != "resolved" && allProven {
			v.State = "resolved"
			v.ResolvedAt = now()
		} else if !remaining && old.State != "resolved" {
			v.State = "unmonitored"
		}
		current[id] = &v
	}
	out := []Incident{}
	for id, v := range current {
		old := oldMap[id]
		a, _ := json.Marshal(old)
		b, _ := json.Marshal(v)
		if string(a) != string(b) {
			if e := s.Store.Put("incidents", id, v); e != nil {
				return nil, e
			}
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].State == "resolved") != (out[j].State == "resolved") {
			return out[j].State == "resolved"
		}
		return out[i].LastSeen > out[j].LastSeen
	})
	return out, nil
}
func (s *Service) AcknowledgeIncident(id, actor string, ack bool) (Incident, error) {
	s.incidentsMu.Lock()
	defer s.incidentsMu.Unlock()
	var v Incident
	if !validID(id) {
		return v, fail(400, "Ungültige Problemkennung")
	}
	if e := s.Store.Get("incidents", id, &v); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return v, fail(404, "Problem nicht gefunden")
		}
		return v, e
	}
	if v.State == "resolved" || v.State == "unmonitored" {
		return v, fail(409, "Problem ist nicht mehr aktiv")
	}
	v.AcknowledgedBy = ""
	v.State = "new"
	if ack {
		v.AcknowledgedBy = actor
		v.State = "acknowledged"
	}
	return v, s.Store.Put("incidents", id, v)
}

func (s *Service) incidentHostProven(old Incident, affected IncidentHost, h Host, b Backup) bool {
	if old.Code == "probe" {
		return h.Inventory != nil && h.ProbeError == "" && h.LastProbe > old.LastSeen
	}
	if old.Code == "integrity" {
		return b.VerificationError == "" && b.VerificationAttempt != nil && b.VerificationAttempt.State == "ok" && b.VerifiedAt > old.LastSeen && (b.ID == affected.BackupID || b.Status == "successful")
	}
	if old.Code == "live_changes" {
		if drift, e := s.HostDrift(h.ID); e == nil && drift.State == "unchanged" && drift.At > old.LastSeen {
			return true
		}
	}
	if b.ID == "" || b.ID == affected.BackupID || b.Status != "successful" || len(b.Warnings) > 0 || b.VerificationError != "" {
		return false
	}
	m, e := s.Manifest(b.ID)
	if e != nil {
		return false
	}
	completed, e1 := time.Parse(time.RFC3339Nano, m.CompletedAt)
	seen, e2 := time.Parse(time.RFC3339Nano, old.LastSeen)
	return e1 == nil && e2 == nil && completed.After(seen) && !completed.After(time.Now().Add(5*time.Minute))
}
