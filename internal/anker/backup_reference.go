package anker

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ManualDrill records an operator's account of an external exercise. It is
// testimony, not a result produced by Anker or permission to restore.
type ManualDrill struct {
	TestedAt    string `json:"tested_at"`
	Outcome     string `json:"outcome"`
	Evidence    string `json:"evidence"`
	Actor       string `json:"actor"`
	RecordedAt  string `json:"recorded_at"`
	BackupID    string `json:"backup_id"`
	ManifestSHA string `json:"manifest_sha"`
}

type BackupReference struct {
	HostID      string       `json:"host_id"`
	BackupID    string       `json:"backup_id"`
	ManifestSHA string       `json:"manifest_sha"`
	Name        string       `json:"name"`
	Reason      string       `json:"reason,omitempty"`
	UpdatedAt   string       `json:"updated_at"`
	UpdatedBy   string       `json:"updated_by"`
	Drill       *ManualDrill `json:"drill,omitempty"`
	State       string       `json:"state"`
}

type ReferenceChangeSummary struct {
	Reference BackupReference `json:"reference"`
	ToID      string          `json:"to_id"`
	Changed   int             `json:"changed"`
	Added     int             `json:"added"`
	Removed   int             `json:"removed"`
	Content   int             `json:"content"`
	Metadata  int             `json:"metadata"`
	MTimeOnly int             `json:"mtime_only"`
}

func (s *Service) ProtectedReference(hostID string) (BackupReference, error) {
	if !validID(hostID) {
		return BackupReference{}, fail(400, "Ungültige Host-ID")
	}
	ref := BackupReference{HostID: hostID, State: "missing"}
	err := s.Store.Get("backup_references", hostID, &ref)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupReference{HostID: hostID, State: "missing"}, nil
	}
	if err != nil {
		return BackupReference{}, err
	}
	ref.State = "unresolved"
	if ref.HostID != hostID {
		return ref, nil
	}
	b, err := s.Backup(ref.BackupID)
	if err != nil || b.HostID != hostID || b.ManifestSHA != ref.ManifestSHA || b.Status == "damaged" || b.VerificationError != "" {
		return ref, nil
	}
	if _, err = s.Manifest(b.ID); err == nil {
		ref.State = "valid"
	}
	return ref, nil
}

func (s *Service) SaveProtectedReference(hostID, backupID, name, reason string, drill *ManualDrill, actor string) (BackupReference, error) {
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	if !validID(backupID) {
		return BackupReference{}, fail(400, "Ungültige Sicherungs-ID")
	}
	lock := s.backupLock(backupID)
	lock.Lock()
	defer lock.Unlock()
	if _, err := s.Host(hostID); err != nil {
		return BackupReference{}, fail(404, "Host nicht gefunden")
	}
	b, err := s.Backup(backupID)
	if err != nil || b.HostID != hostID {
		return BackupReference{}, fail(400, "Sicherung gehört nicht zu diesem Host")
	}
	if b.Status == "damaged" || b.VerificationError != "" {
		return BackupReference{}, fail(409, "Beschädigte Sicherung kann keine Referenz sein")
	}
	if _, err = s.Manifest(b.ID); err != nil {
		return BackupReference{}, fail(409, "Quellmanifest nicht lesbar")
	}
	name, reason, actor = strings.TrimSpace(name), strings.TrimSpace(reason), strings.TrimSpace(actor)
	if name == "" || len(name) > 120 || len(reason) > 1000 || actor == "" {
		return BackupReference{}, fail(400, "Name, Grund oder Akteur ungültig")
	}
	ref := BackupReference{HostID: hostID, BackupID: backupID, ManifestSHA: b.ManifestSHA, Name: name, Reason: reason, UpdatedAt: now(), UpdatedBy: actor, State: "valid"}
	if drill == nil {
		var previous BackupReference
		if err := s.Store.Get("backup_references", hostID, &previous); err == nil && previous.BackupID == b.ID && previous.ManifestSHA == b.ManifestSHA {
			ref.Drill = previous.Drill
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return BackupReference{}, err
		}
	}
	if drill != nil {
		when, parseErr := time.Parse(time.RFC3339, drill.TestedAt)
		if parseErr != nil || when.After(time.Now()) || (drill.Outcome != "passed" && drill.Outcome != "failed") || strings.TrimSpace(drill.Evidence) == "" || len(drill.Evidence) > 2000 {
			return BackupReference{}, fail(400, "Offline-Probe benötigt Datum, Ergebnis und Beleg")
		}
		copy := *drill
		copy.Evidence = strings.TrimSpace(copy.Evidence)
		copy.Actor, copy.RecordedAt, copy.BackupID, copy.ManifestSHA = actor, now(), b.ID, b.ManifestSHA
		ref.Drill = &copy
	}
	if err = s.Store.Put("backup_references", hostID, ref); err != nil {
		return BackupReference{}, err
	}
	return ref, nil
}

func (s *Service) ClearProtectedReferenceDrill(hostID, actor string) (BackupReference, error) {
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	var ref BackupReference
	if err := s.Store.Get("backup_references", hostID, &ref); err != nil {
		return BackupReference{}, fail(404, "Referenz nicht gefunden")
	}
	if ref.HostID != hostID || strings.TrimSpace(actor) == "" {
		return BackupReference{}, fail(400, "Referenz oder Akteur ungültig")
	}
	ref.Drill, ref.UpdatedBy, ref.UpdatedAt = nil, actor, now()
	if err := s.Store.Put("backup_references", hostID, ref); err != nil {
		return BackupReference{}, err
	}
	return s.ProtectedReference(hostID)
}

func (s *Service) CompareProtectedReference(hostID, toID string) (ReferenceChangeSummary, error) {
	ref, err := s.ProtectedReference(hostID)
	if err != nil {
		return ReferenceChangeSummary{}, err
	}
	if ref.State != "valid" {
		return ReferenceChangeSummary{}, fail(409, "Referenz fehlt oder ist nicht auflösbar")
	}
	to, err := s.Backup(toID)
	if err != nil || to.HostID != hostID {
		return ReferenceChangeSummary{}, fail(400, "Vergleichsstand gehört nicht zu diesem Host")
	}
	rows, err := s.DiffBackups(ref.BackupID, toID, "")
	if err != nil {
		return ReferenceChangeSummary{}, err
	}
	out := ReferenceChangeSummary{Reference: ref, ToID: toID}
	for _, row := range rows {
		if row.MTimeOnly {
			out.MTimeOnly++
			continue
		}
		out.Changed++
		if row.Change == "added" {
			out.Added++
		}
		if row.Change == "removed" {
			out.Removed++
		}
		if row.ContentChanged {
			out.Content++
		}
		if row.MetadataChanged {
			out.Metadata++
		}
	}
	return out, nil
}
