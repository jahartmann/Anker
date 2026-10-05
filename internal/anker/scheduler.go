package anker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
	_ "time/tzdata"
)

type Scheduler struct{ s *Service }

func NewScheduler(s *Service) *Scheduler { return &Scheduler{s: s} }
func (sc *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	sc.Tick(time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-ticker.C:
			sc.Tick(at)
		}
	}
}
func (sc *Scheduler) Tick(at time.Time) error {
	settings, err := sc.s.Settings()
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return err
	}
	local := at.In(loc)
	day := local.Format("2006-01-02")
	hosts, err := sc.s.Hosts()
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if !h.Enabled {
			continue
		}
		schedule := h.Schedule
		if schedule == "" {
			schedule = settings.Schedule
		}
		t, err := time.Parse("15:04", schedule)
		if err != nil {
			continue
		}
		jitter, _ := strconv.ParseInt(Hash([]byte(h.ID))[:4], 16, 64)
		due := time.Date(local.Year(), local.Month(), local.Day(), t.Hour(), t.Minute(), 0, 0, loc).Add(time.Duration(jitter%60) * time.Minute)
		if local.Before(due) {
			continue
		}
		var previous string
		sc.s.Store.Get("schedule", h.ID, &previous)
		if previous == day {
			continue
		}
		if _, err = sc.s.QueueBackup(h.ID); err == nil {
			sc.s.Store.Put("schedule", h.ID, day)
		}
	}
	var maintained string
	sc.s.Store.Get("maintenance", "day", &maintained)
	if maintained != day {
		if err = sc.s.Maintain(at); err == nil {
			sc.s.Store.Put("maintenance", "day", day)
		}
	}
	return nil
}
func (s *Service) Maintain(at time.Time) error {
	settings, err := s.Settings()
	if err != nil {
		return err
	}
	hosts, err := s.Hosts()
	if err != nil {
		return err
	}
	plans, err := s.Plans()
	if err != nil {
		return err
	}
	protected := map[string]bool{}
	for _, p := range plans {
		protected[p.BackupID] = true
	}
	loc, _ := time.LoadLocation(settings.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	for _, h := range hosts {
		backups, err := s.ListBackups(h.ID)
		if err != nil {
			return err
		}
		keep := map[string]bool{}
		days := map[string]bool{}
		weeks := map[string]bool{}
		months := map[string]bool{}
		latest := ""
		for _, b := range backups {
			if b.Status != "successful" {
				keep[b.ID] = true
				continue
			}
			if latest == "" {
				latest = b.ID
				keep[b.ID] = true
			}
			date, err := time.Parse(time.RFC3339Nano, b.CreatedAt)
			if err != nil {
				keep[b.ID] = true
				continue
			}
			date = date.In(loc)
			day := date.Format("2006-01-02")
			year, week := date.ISOWeek()
			wk := fmt.Sprintf("%d-%02d", year, week)
			mo := date.Format("2006-01")
			if !days[day] && len(days) < settings.Daily {
				keep[b.ID] = true
				days[day] = true
			}
			if !weeks[wk] && len(weeks) < settings.Weekly {
				keep[b.ID] = true
				weeks[wk] = true
			}
			if !months[mo] && len(months) < settings.Monthly {
				keep[b.ID] = true
				months[mo] = true
			}
			if b.Pinned || protected[b.ID] {
				keep[b.ID] = true
			}
		}
		for _, b := range backups {
			if !keep[b.ID] {
				lock := s.backupLock(b.ID)
				lock.Lock()
				current, _ := s.Backup(b.ID)
				referenced := current.Pinned
				freshPlans, _ := s.Plans()
				for _, p := range freshPlans {
					if p.BackupID == b.ID {
						referenced = true
					}
				}
				if referenced {
					lock.Unlock()
					continue
				}
				err = os.RemoveAll(s.backupDir(b))
				if err == nil {
					err = s.Store.Delete("backups", b.ID)
				}
				lock.Unlock()
				if err != nil {
					return err
				}
				continue
			}
			date, _ := time.Parse(time.RFC3339Nano, b.CreatedAt)
			if settings.ArchiveDays > 0 && !b.Pinned && !protected[b.ID] && !b.Archived && at.Sub(date) > time.Duration(settings.ArchiveDays)*24*time.Hour {
				if s.CheckNonessentialSpace() == nil {
					s.ArchiveBackup(b.ID)
				}
			}
		}
	}
	return nil
}
func (s *Service) Reindex() (int, error) {
	files, err := filepath.Glob(filepath.Join(s.Root, "hosts", "*", "backups", "*", "manifest.json"))
	if err != nil {
		return 0, err
	}
	sort.Strings(files)
	count := 0
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return count, err
		}
		var m Manifest
		if err = decodeJSON(data, &m); err != nil {
			return count, err
		}
		if m.Version != FormatVersion || !validID(m.ID) || !validID(m.HostID) || filepath.Base(filepath.Dir(p)) != m.ID || filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(p)))) != m.HostID {
			return count, fmt.Errorf("ungültiges Manifest %s", p)
		}
		b := Backup{ID: m.ID, HostID: m.HostID, HostName: m.Inventory.Hostname, CreatedAt: m.CreatedAt, Status: m.Status, Files: len(m.Entries), ManifestSHA: Hash(data), Warnings: m.Warnings}
		if previous, getErr := s.Backup(b.ID); getErr == nil {
			if previous.ManifestSHA != b.ManifestSHA {
				return count, fmt.Errorf("Manifest-Prüfsumme weicht vom bekannten Stand ab: %s", b.ID)
			}
			b.Pinned = previous.Pinned
		}
		for _, e := range m.Entries {
			b.Size += e.Size
		}
		_, err = os.Stat(filepath.Join(filepath.Dir(p), "files"))
		b.Archived = os.IsNotExist(err)
		lock := s.backupLock(b.ID)
		lock.Lock()
		if current, e := s.Backup(b.ID); e == nil {
			b.Pinned = current.Pinned
		}
		_, err = os.Stat(filepath.Join(filepath.Dir(p), "files"))
		b.Archived = os.IsNotExist(err)
		if b.Archived {
			var temp string
			temp, err = s.extractArchive(b)
			if temp != "" {
				os.RemoveAll(temp)
			}
		} else {
			err = s.verifyAtRecord(b, filepath.Dir(p))
		}
		if err == nil {
			err = s.Store.Put("backups", b.ID, b)
		}
		lock.Unlock()
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
