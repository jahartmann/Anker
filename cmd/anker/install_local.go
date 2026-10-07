package main

import (
	"anker/internal/buildinfo"
	"anker/internal/updater"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// This command is only invoked by the local installer, never over HTTP. The
// administrator explicitly runs a locally built or verified release executable.
func runLocalInstall() (result error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return errors.New("Serverinstallation benötigt root und Linux mit systemd")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self == updater.BinaryPath || self == updater.HelperPath {
		return errors.New("Installation aus der neu gebauten Datei ausführen")
	}
	info, err := os.Stat(self)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > updater.MaxBinary {
		return errors.New("Installationsdatei ungültig")
	}
	content, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	lock, err := os.OpenFile("/etc/anker/setup.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("Einrichtung oder Systemänderung läuft bereits")
	}
	if _, err = setupRecoverPending("/etc/anker"); err != nil {
		return err
	}
	if err = setupHelperIdle(); err != nil {
		return err
	}
	// Stop the helper before taking its process lock. A persisted update is
	// recovered below with the same journal as an ordinary OTA restart.
	helperRunning := exec.Command("systemctl", "is-active", "--quiet", "anker-updater.service").Run() == nil
	defer func() {
		if helperRunning {
			if _, pendingErr := os.Stat(filepath.Join(updater.StateDir, "pending.json")); !os.IsNotExist(pendingErr) {
				return
			}
			if startErr := setupCommand("systemctl", "start", "anker-updater.service"); startErr != nil {
				result = errors.Join(result, startErr)
			}
		}
	}()
	if err = setupCommand("systemctl", "stop", "anker-updater.service"); err != nil {
		return err
	}
	if err = os.MkdirAll(updater.StateDir, 0700); err != nil {
		return err
	}
	updateLock, err := os.OpenFile(filepath.Join(updater.StateDir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { updateLock.Close() }()
	if err = unix.Flock(int(updateLock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("Updater ist noch aktiv; Installation nicht verändert")
	}
	if _, pendingErr := os.Stat(filepath.Join(updater.StateDir, "pending.json")); pendingErr == nil {
		recovery := updater.Installer{Binary: updater.BinaryPath, Helper: updater.HelperPath, Data: updater.DataDir, StateDir: updater.StateDir, Control: updater.SystemControl{Socket: filepath.Join(updater.DataDir, "anker.sock")}}
		fmt.Println("Unterbrochene Aktualisierung wiederherstellen.")
		if err = recovery.Recover(context.Background()); err != nil {
			return err
		}
	} else if !os.IsNotExist(pendingErr) {
		return pendingErr
	}
	if _, guardErr := os.Stat(updater.Maintenance); guardErr == nil {
		control := localInstallControl{SystemControl: updater.SystemControl{Socket: filepath.Join(updater.DataDir, "anker.sock")}, online: true}
		if err = control.Release(context.Background()); err != nil {
			return err
		}
	} else if !os.IsNotExist(guardErr) {
		return guardErr
	}
	if err = setupUpdateIdle(); err != nil {
		return err
	}
	running := exec.Command("systemctl", "is-active", "--quiet", "anker.service").Run() == nil
	localLock, err := os.OpenFile(updater.LocalInstallLock, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer localLock.Close()
	if err = unix.Flock(int(localLock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("Eine lokale Installation läuft bereits")
	}
	control := &localInstallControl{SystemControl: updater.SystemControl{Socket: filepath.Join(updater.DataDir, "anker.sock")}, online: running}
	lockReleased := false
	control.beforeStop = func() error {
		if !lockReleased {
			return nil
		}
		if err := setupCommand("systemctl", "stop", "anker-updater.service"); err != nil {
			return err
		}
		lock, err := os.OpenFile(filepath.Join(updater.StateDir, "lock"), os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
			lock.Close()
			return err
		}
		updateLock = lock
		lockReleased = false
		return nil
	}
	sum := sha256.Sum256(content)
	release := updater.Release{Version: buildinfo.Version, Artifact: updater.Artifact{Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}}
	installer := updater.Installer{Binary: updater.BinaryPath, Helper: updater.HelperPath, Data: updater.DataDir, StateDir: updater.StateDir, Control: control, AllowMissingCatalog: !running, Offline: !running, Coordinated: true}
	installer.AfterHealth = func(ctx context.Context) error {
		if !helperRunning {
			return nil
		}
		if err := updateLock.Close(); err != nil {
			return err
		}
		lockReleased = true
		if err := setupCommand("systemctl", "start", "anker-updater.service"); err != nil {
			return err
		}
		return setupReady(updater.Socket, "/status")
	}
	fmt.Println("Anker · bestehende Installation aktualisieren")
	fmt.Println("Programme und vorhandener Katalog werden vor dem Austausch gesichert.")
	if err = installer.Install(context.Background(), release, content); err != nil {
		return err
	}
	// Main startup and both executable replacements have been verified. The
	// following setup performs the full configuration and helper readiness check.
	if !lockReleased {
		if err = updateLock.Close(); err != nil {
			return err
		}
		lockReleased = true
	}
	fmt.Println("Programm aktualisiert. Vorhandene Benutzer, Schlüssel und Einstellungen bleiben erhalten.")
	return nil
}

type localInstallControl struct {
	updater.SystemControl
	online     bool
	beforeStop func() error
}

func (c localInstallControl) Prepare(ctx context.Context) (string, error) {
	if c.online {
		return c.SystemControl.Prepare(ctx)
	}
	out, err := exec.CommandContext(ctx, updater.BinaryPath, "version").Output()
	if err != nil {
		return "", fmt.Errorf("Bisheriges Programm nicht prüfbar: %w", err)
	}
	version := strings.TrimSpace(strings.TrimPrefix(string(out), "Anker "))
	if version == "" {
		return "", errors.New("Bisherige Programmversion fehlt")
	}
	return version, nil
}
func (c localInstallControl) Release(ctx context.Context) error {
	if !c.online {
		return nil
	}
	// Wait before releasing maintenance when Type=simple has just started.
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for {
		err := c.SystemControl.Release(bounded)
		if err == nil {
			return nil
		}
		select {
		case <-bounded.Done():
			return err
		case <-time.After(time.Second):
		}
	}
}
func (c localInstallControl) Stop(ctx context.Context) error {
	if c.beforeStop != nil {
		if err := c.beforeStop(); err != nil {
			return err
		}
	}
	return c.SystemControl.Stop(ctx)
}
func (c localInstallControl) Start(ctx context.Context) error {
	if c.online {
		return c.SystemControl.Start(ctx)
	}
	return nil
}
func (c localInstallControl) Health(ctx context.Context, want string) error {
	if c.online {
		return c.SystemControl.Health(ctx, want)
	}
	out, err := exec.CommandContext(ctx, updater.BinaryPath, "version").Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "Anker "+want {
		return errors.New("Installiertes Programm besteht die Versionsprüfung nicht")
	}
	return nil
}
