package storage

import (
	"encoding/json"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Inspect(dataPath string) (Report, error) {
	r := Report{CollectedAt: time.Now().UTC().Format(time.RFC3339), DataPath: dataPath, Environment: "unknown", Volumes: []Volume{}, Devices: []Device{}, Warnings: []string{}}
	raw, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return r, err
	}
	r.Volumes, r.Warnings, err = VolumesFromMounts(string(raw), dataPath, func(path string) (Stats, error) {
		return filesystemProbe.read(path, localStats)
	})
	if err != nil {
		return r, err
	}
	if output, e := boundedRun("systemd-detect-virt", "--container"); e == nil {
		r.Environment = "container:" + strings.TrimSpace(string(output))
	} else if output, e = boundedRun("systemd-detect-virt", "--vm"); e == nil {
		r.Environment = "vm:" + strings.TrimSpace(string(output))
	} else if output, e = boundedRun("systemd-detect-virt"); e != nil && strings.TrimSpace(string(output)) == "none" {
		r.Environment = "physical"
	}
	output, e := boundedRun("lsblk", "--json", "--bytes", "--paths", "--output", "NAME,TYPE,SIZE,FSTYPE,UUID,MOUNTPOINTS,PKNAME")
	if e != nil {
		r.Warnings = append(r.Warnings, "Blockgeräte konnten nicht vollständig gelesen werden; lsblk prüfen.")
	} else {
		var parsed struct {
			Devices []Device `json:"blockdevices"`
		}
		if e = json.Unmarshal(output, &parsed); e != nil {
			r.Warnings = append(r.Warnings, "Blockgeräteliste ist nicht lesbar.")
		} else {
			r.Devices = parsed.Devices
		}
	}
	for i := range r.Volumes {
		v := &r.Volumes[i]
		device := v.Source
		d, ok := FindDevice(r.Devices, device)
		if !ok {
			if resolved, e := filepath.EvalSymlinks(device); e == nil {
				d, ok = FindDevice(r.Devices, resolved)
			}
		}
		if ok && d.UUID != "" {
			v.UUID = d.UUID
			v.ID = Identity(v.Source + "|" + v.FSType + "|" + v.UUID)
		}
	}
	if strings.HasPrefix(r.Environment, "container:") {
		r.Warnings = append(r.Warnings, "Containerwerte gelten für sichtbare Dateisysteme/Quoten. Freier Platz des Proxmox-Speicherpools ist hier nicht messbar.")
	}
	return r, nil
}

func localStats(path string) (Stats, error) {
	info, e := os.Stat(path)
	if e != nil {
		return Stats{}, e
	}
	if !info.IsDir() {
		return Stats{}, os.ErrInvalid
	}
	var s unix.Statfs_t
	if e = unix.Statfs(path, &s); e != nil {
		return Stats{}, e
	}
	return Stats{int64(s.Blocks) * s.Bsize, int64(s.Bfree) * s.Bsize, int64(s.Bavail) * s.Bsize, int64(s.Files), int64(s.Ffree)}, nil
}
