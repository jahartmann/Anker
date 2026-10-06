package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"anker/internal/updater"
	"golang.org/x/sys/unix"
)

var setupConfigNames = []string{"service.env", "tls-renewal.json", "update.json", "setup-complete"}

type setupSavedFile struct {
	Name     string
	Exists   bool
	Data     []byte
	Mode     os.FileMode
	UID, GID int
}
type setupChange struct {
	Version                     int
	Files                       []setupSavedFile
	Created                     []string
	MainRunning, UpdaterRunning bool
}

func setupDisposable(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil || filepath.Clean(path) != path {
		return false
	}
	return filepath.Dir(rel) == "tls" && strings.HasPrefix(filepath.Base(rel), "server-") && (strings.HasSuffix(rel, ".crt") || strings.HasSuffix(rel, ".key")) || filepath.Dir(rel) == "." && strings.HasPrefix(rel, "github-token-")
}

// Only configuration and newly planned TLS/token files belong to this change.
// Users, backups and SSH identities are deliberately outside the rollback.
func setupBeginChange(dir string, created []string, mainRunning, updaterRunning bool) (*setupChange, error) {
	if old, err := setupLoadChange(dir); err != nil {
		return nil, err
	} else if old != nil {
		return nil, errors.New("Unterbrochene Einrichtung zuerst wiederherstellen")
	}
	change := &setupChange{Version: 1, Created: created, MainRunning: mainRunning, UpdaterRunning: updaterRunning}
	for _, path := range created {
		if !setupDisposable(dir, path) {
			return nil, errors.New("Ungültiger temporärer Einrichtungspfad")
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return nil, fmt.Errorf("Neue Einrichtungsdatei existiert bereits oder ist nicht prüfbar: %s", path)
		}
	}
	for _, name := range setupConfigNames {
		saved := setupSavedFile{Name: name}
		info, err := os.Lstat(filepath.Join(dir, name))
		if err == nil {
			if !info.Mode().IsRegular() || info.Size() > 1<<20 {
				return nil, fmt.Errorf("Konfigurationsdatei ungültig: %s", name)
			}
			saved.Exists = true
			saved.Mode = info.Mode().Perm()
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return nil, errors.New("Dateirechte nicht lesbar")
			}
			saved.UID, saved.GID = int(stat.Uid), int(stat.Gid)
			saved.Data, err = os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		change.Files = append(change.Files, saved)
	}
	b, err := json.Marshal(change)
	if err != nil {
		return nil, err
	}
	if err = setupWrite(filepath.Join(dir, "setup-pending.json"), b, 0600, os.Geteuid(), os.Getegid()); err != nil {
		return nil, err
	}
	return change, nil
}

func setupLoadChange(dir string) (*setupChange, error) {
	path := filepath.Join(dir, "setup-pending.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8<<20 {
		return nil, errors.New("Wiederherstellungsdatei ungültig oder ungeschützt")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var change setupChange
	if err = json.Unmarshal(b, &change); err != nil {
		return nil, fmt.Errorf("Wiederherstellungsdatei nicht lesbar: %w", err)
	}
	if change.Version != 1 || len(change.Files) != len(setupConfigNames) {
		return nil, errors.New("Wiederherstellungsdatei unvollständig")
	}
	for i, file := range change.Files {
		if file.Name != setupConfigNames[i] || file.Mode&^0777 != 0 || len(file.Data) > 1<<20 || file.UID < 0 || file.GID < 0 {
			return nil, errors.New("Wiederherstellungsdatei enthält ungültige Konfiguration")
		}
	}
	for _, path := range change.Created {
		if !setupDisposable(dir, path) {
			return nil, errors.New("Wiederherstellungsdatei enthält ungültigen Pfad")
		}
	}
	return &change, nil
}

func setupRemoveSynced(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (change *setupChange) rollback(dir string) error {
	// Restore supporting policy before selecting the old web configuration.
	for i := len(change.Files) - 1; i >= 0; i-- {
		file := change.Files[i]
		path := filepath.Join(dir, file.Name)
		if file.Exists {
			if err := setupWrite(path, file.Data, file.Mode, file.UID, file.GID); err != nil {
				return err
			}
		} else if err := setupRemoveSynced(path); err != nil {
			return err
		}
	}
	for _, path := range change.Created {
		if err := setupRemoveSynced(path); err != nil {
			return err
		}
	}
	return change.commit(dir)
}
func (change *setupChange) commit(dir string) error {
	return setupRemoveSynced(filepath.Join(dir, "setup-pending.json"))
}

// The setup lock is already held. Stop both services and take the same update
// lock as normal setup before touching any configuration from an interrupted run.
func setupRecoverPending(dir string) (bool, error) {
	change, err := setupLoadChange(dir)
	if err != nil || change == nil {
		return false, err
	}
	display := newSetupDisplay()
	display.Note("Unterbrochene Einrichtung erkannt. Letzten Konfigurationsstand wiederherstellen.")
	if err = setupCommand("systemctl", "stop", "anker-updater.service"); err != nil {
		return false, err
	}
	lock, err := os.OpenFile(filepath.Join(updater.StateDir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return false, err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return false, errors.New("Updater ist aktiv; Wiederherstellung später erneut starten")
	}
	if err = setupUpdateIdle(); err != nil {
		return false, err
	}
	if err = setupCommand("systemctl", "stop", "anker.service"); err != nil {
		return false, err
	}
	if err = change.rollback(dir); err != nil {
		return false, fmt.Errorf("Wiederherstellung nicht abgeschlossen; anker setup erneut ausführen: %w", err)
	}
	if change.MainRunning {
		if err = setupCommand("systemctl", "start", "anker.service"); err != nil {
			return false, err
		}
	}
	lock.Close()
	if change.UpdaterRunning {
		if err = setupCommand("systemctl", "start", "anker-updater.service"); err != nil {
			return false, err
		}
	}
	display.Note("Konfiguration wiederhergestellt. Vorhandene Benutzer und Schlüssel bleiben erhalten.")
	return true, nil
}
