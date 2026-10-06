package anker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ResolveDataRoot resolves symlink aliases, including paths not created yet.
func ResolveDataRoot(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	ancestor := path
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			rel, err := filepath.Rel(ancestor, path)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, rel), nil
		}
		if !os.IsNotExist(err) || ancestor == filepath.Dir(ancestor) {
			return "", err
		}
		ancestor = filepath.Dir(ancestor)
	}
}

func dataMode(root string) (string, error) {
	fd, err := unix.Open(filepath.Join(root, ".anker-mode"), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("Betriebsart kann nicht geprüft werden: %w", err)
	}
	f := os.NewFile(uintptr(fd), ".anker-mode")
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("Betriebsart muss eine reguläre Datei sein")
	}
	b, err := io.ReadAll(io.LimitReader(f, 32))
	if err != nil {
		return "", err
	}
	if string(b) != "production\n" && string(b) != "demo\n" {
		return "", errors.New("Ungültige Betriebsart; Datenverzeichnis bleibt unverändert")
	}
	return string(b), nil
}

// EnsureDataMode must run under InstanceLock, before opening the catalog.
// Never convert a production catalog into a demo or run demo data with SSH.
func EnsureDataMode(root string, demo bool) error {
	root, err := ResolveDataRoot(root)
	if err != nil {
		return err
	}
	want := "production\n"
	if demo {
		want = "demo\n"
	}
	current := ""
	for parent := root; ; parent = filepath.Dir(parent) {
		mode, err := dataMode(parent)
		if err != nil {
			return err
		}
		if mode != "" && mode != want {
			return errors.New("Demo und Produktionsdaten müssen in getrennten Datenverzeichnissen liegen")
		}
		if demo && parent != root && mode == "" {
			// Old production installations have no marker yet. Their catalog
			// identifies an occupied root; a leftover lock alone does not.
			if _, err := os.Lstat(filepath.Join(parent, "catalog.db")); err == nil {
				return errors.New("Demo darf nicht in einem vorhandenen Produktionsdatenverzeichnis liegen")
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if parent == root {
			current = mode
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	if current == want {
		return nil
	}
	if demo {
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Name() != "service.lock" {
				return errors.New("Demo benötigt ein neues, leeres Datenverzeichnis; vorhandene Daten bleiben unverändert")
			}
		}
	} else {
		if _, err := os.Lstat(filepath.Join(root, "demo-hosts")); err == nil {
			return errors.New("Dieses Verzeichnis enthält eine frühere Demo; für Produktionsbetrieb ein separates Datenverzeichnis verwenden")
		} else if !os.IsNotExist(err) {
			return err
		}
		// Only an unmarked legacy production root needs this scan. Existing
		// marked roots reject conflicting descendants when those are created.
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				parts := strings.Split(rel, string(os.PathSeparator))
				backupPayload := len(parts) == 5 && parts[0] == "hosts" && parts[2] == "backups" && parts[4] == "files"
				planPayload := len(parts) == 3 && parts[0] == "plans" && parts[2] == "prepared-files"
				stagedPayload := len(parts) == 3 && parts[0] == "staging" && (parts[2] == "files" || parts[2] == "original-files")
				if backupPayload || planPayload || stagedPayload {
					return filepath.SkipDir
				}
			}
			if entry.Name() != ".anker-mode" {
				return nil
			}
			mode, err := dataMode(filepath.Dir(path))
			if err != nil {
				return err
			}
			if mode != want {
				return errors.New("Produktionsverzeichnis darf keinen vorhandenen Demo-Datenordner umschließen")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return atomicWrite(filepath.Join(root, ".anker-mode"), []byte(want), 0600)
}
