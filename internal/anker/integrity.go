package anker

import (
	"database/sql"
	"errors"
	"os"
	"sort"
	"syscall"
	"time"
)

// Environment failures are attempts without a verdict about stored content.
// Missing paths, invalid path types and links are evidence of an incomplete/invalid tree.
type deferredVerificationError struct{ error }

func (e deferredVerificationError) Unwrap() error { return e.error }
func verificationDeferred(err error) bool {
	if err == nil {
		return false
	}
	var deferred deferredVerificationError
	if errors.As(err, &deferred) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) && !errors.Is(err, syscall.EISDIR) && !errors.Is(err, syscall.ELOOP)
	}
	return false
}
func (s *Service) beginIntegrityOperation() (func(), error) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.maintenance {
		return nil, fail(503, "Anker wird aktualisiert; Prüfung zurückgestellt")
	}
	s.integrityBusy++
	return func() { s.jobMu.Lock(); s.integrityBusy--; s.jobMu.Unlock() }, nil
}

type integrityAttempt struct {
	At    string `json:"at"`
	State string `json:"state"`
}

func (s *Service) maintainIntegrity(b Backup, at time.Time) error {
	// Save admission before I/O: an interrupted or deferred older check must not monopolize rotation.
	attempt := integrityAttempt{At: at.UTC().Format(time.RFC3339Nano), State: "started"}
	if err := s.Store.Put("integrity_attempts", b.ID, attempt); err != nil {
		return err
	}
	err := s.VerifyBackup(b.ID)
	attempt.State = "ok"
	if err != nil {
		if verificationDeferred(err) {
			attempt.State = "deferred"
		} else {
			current, getErr := s.Backup(b.ID)
			if getErr != nil {
				return errors.Join(err, getErr)
			}
			if current.Status != "damaged" {
				return err
			}
			attempt.State = "failed"
		}
	}
	// Deferred/failed content checks are recorded, not retried on every scheduler minute.
	return s.Store.Put("integrity_attempts", b.ID, attempt)
}

type integritySweep struct {
	Attempted map[string]bool `json:"attempted"`
}

func (s *Service) rotateIntegrity(backups []Backup, keep, protected, checked map[string]bool, at time.Time) error {
	if len(backups) == 0 {
		return nil
	}
	var sweep integritySweep
	if err := s.Store.Get("integrity_sweeps", backups[0].HostID, &sweep); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if sweep.Attempted == nil {
		sweep.Attempted = map[string]bool{}
	}

	type candidate struct {
		b         Backup
		last      time.Time
		protected bool
	}
	var candidates []candidate
	for _, b := range backups {
		if !keep[b.ID] || checked[b.ID] {
			continue
		}
		var attempt integrityAttempt
		err := s.Store.Get("integrity_attempts", b.ID, &attempt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if attempt.At == "" {
			attempt.At = b.VerifiedAt
		}
		last, _ := time.Parse(time.RFC3339Nano, attempt.At)
		candidates = append(candidates, candidate{b, last, b.Pinned || protected[b.ID]})
	}
	// Protected sources lead each sweep. A source already attempted in this sweep
	// cannot displace an unattempted source, including after restart or deferral.
	pending := []candidate{}
	retained := map[string]bool{}
	for _, c := range candidates {
		retained[c.b.ID] = true
		if !sweep.Attempted[c.b.ID] {
			pending = append(pending, c)
		}
	}
	for id := range sweep.Attempted {
		if !retained[id] {
			delete(sweep.Attempted, id)
		}
	}
	if len(pending) > 0 {
		candidates = pending
	} else {
		sweep.Attempted = map[string]bool{}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.protected != b.protected {
			return a.protected
		}
		if !a.last.Equal(b.last) {
			return a.last.Before(b.last)
		}
		if a.b.CreatedAt != b.b.CreatedAt {
			return a.b.CreatedAt < b.b.CreatedAt
		}
		return a.b.ID < b.b.ID
	})
	if len(candidates) == 0 {
		return nil
	}
	selected := candidates[0].b
	sweep.Attempted[selected.ID] = true
	if err := s.Store.Put("integrity_sweeps", selected.HostID, sweep); err != nil {
		return err
	}
	return s.maintainIntegrity(selected, at)
}
