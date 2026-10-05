package anker

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"sync"
)

// InstanceLock excludes all other daemons and initialization on the same data root.
func InstanceLock(root string) (func(), error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	fd, err := unix.Open(filepath.Join(root, "service.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		unix.Close(fd)
		return nil, errors.New("Anker-Dienst läuft bereits für dieses Datenverzeichnis")
	}
	return func() { unix.Flock(fd, unix.LOCK_UN); unix.Close(fd) }, nil
}
func (s *Service) backupLock(id string) *sync.RWMutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backupLocks == nil {
		s.backupLocks = map[string]*sync.RWMutex{}
	}
	if s.backupLocks[id] == nil {
		s.backupLocks[id] = &sync.RWMutex{}
	}
	return s.backupLocks[id]
}

// A readable lease prevents archiving or pruning until the caller releases it.
func (s *Service) readableLease(id string) (func(), error) {
	lock := s.backupLock(id)
	for {
		lock.RLock()
		b, err := s.Backup(id)
		if err != nil {
			lock.RUnlock()
			return nil, err
		}
		if !b.Archived {
			return lock.RUnlock, nil
		}
		lock.RUnlock()
		lock.Lock()
		err = s.ensureReadableUnlocked(id)
		lock.Unlock()
		if err != nil {
			return nil, err
		}
	}
}
