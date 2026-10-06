package main

import (
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"strings"
	"syscall"
)

func setupUpdaterUnitText(original []byte) ([]byte, bool, error) {
	lines := strings.Split(string(original), "\n")
	managed := false
	changed := false
	var output []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "ExecStart=/usr/local/libexec/anker-updater updater-serve" {
			managed = true
		}
		if strings.TrimSpace(line) == "ConditionPathExists=/etc/anker/update.json" {
			changed = true
			continue
		}
		output = append(output, line)
	}
	if !changed {
		return original, false, nil
	}
	if !managed {
		return nil, false, errors.New("Abweichende anker-updater.service; Einrichtungsbedingung manuell prüfen")
	}
	return []byte(strings.Join(output, "\n")), true, nil
}

func setupMigrateUpdaterUnit() error {
	const path = "/etc/systemd/system/anker-updater.service"
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Size() > 64<<10 || !ok || owner.Uid != 0 || info.Mode().Perm()&0022 != 0 {
		return errors.New("Updater-Unit muss eine root-eigene, für andere nicht schreibbare Datei sein")
	}
	original, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return err
	}
	if len(original) > 64<<10 {
		return errors.New("Updater-Unit zu groß")
	}
	updated, changed, err := setupUpdaterUnitText(original)
	if err != nil || !changed {
		return err
	}
	if err = setupWrite(path, updated, info.Mode().Perm(), int(owner.Uid), int(owner.Gid)); err != nil {
		return err
	}
	return setupCommand("systemctl", "daemon-reload")
}
