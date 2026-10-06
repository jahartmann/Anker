package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func Identity(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:16])
}
func unescapeMount(value string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, "\\").Replace(value)
}
func containsPath(root, path string) bool {
	return root == "/" || path == root || strings.HasPrefix(path, root+"/")
}
func option(value, key string) bool {
	for _, s := range strings.Split(value, ",") {
		if s == key {
			return true
		}
	}
	return false
}
func VolumesFromMounts(raw, dataPath string, stat func(string) (Stats, error)) ([]Volume, []string, error) {
	type mount struct{ id, root, path, fs, source, options string }
	mounts := []mount{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			return nil, nil, errors.New("Mountinformationen sind unvollständig")
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) < 6 || len(right) < 3 {
			return nil, nil, errors.New("Mountinformationen sind ungültig")
		}
		mounts = append(mounts, mount{left[2], unescapeMount(left[3]), unescapeMount(left[4]), right[0], unescapeMount(right[1]), right[2]})
	}
	if len(mounts) > 4096 {
		return nil, nil, errors.New("Zu viele Mounts")
	}
	// The last covering mount in mountinfo takes precedence for overmounts.
	dataIndex, systemIndex := -1, -1
	for i, m := range mounts {
		if m.path == "/" {
			systemIndex = i
		}
		if containsPath(m.path, dataPath) && (dataIndex < 0 || len(m.path) >= len(mounts[dataIndex].path)) {
			dataIndex = i
		}
	}
	if systemIndex < 0 || dataIndex < 0 {
		return nil, nil, errors.New("System oder Anker-Ablage konnte keinem Mount zugeordnet werden")
	}
	pseudo := map[string]bool{"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true, "tmpfs": true, "cgroup": true, "cgroup2": true, "mqueue": true, "securityfs": true, "debugfs": true, "tracefs": true, "pstore": true, "configfs": true, "hugetlbfs": true, "fusectl": true, "autofs": true, "binfmt_misc": true, "rpc_pipefs": true, "nsfs": true, "squashfs": true}
	out := []Volume{}
	indices := map[string]int{}
	warnings := []string{}
	visible := map[string]int{}
	for i, m := range mounts {
		visible[m.path] = i
	}
	for i, m := range mounts {
		if visible[m.path] != i {
			continue
		}
		relevant := i == dataIndex || i == systemIndex || strings.HasPrefix(m.path, dataPath+"/")
		if !relevant && (pseudo[m.fs] || strings.HasPrefix(m.path, "/run/") || strings.HasPrefix(m.source, "/dev/loop") || m.path == "/etc/hosts" || m.path == "/etc/hostname" || m.path == "/etc/resolv.conf") {
			continue
		}
		key := m.id + "|" + m.source + "|" + m.fs
		idx, exists := indices[key]
		if !exists {
			idx = len(out)
			indices[key] = idx
			out = append(out, Volume{ID: Identity(key), Mount: m.path, Paths: []string{}, Source: m.source, FSType: m.fs, ReadOnly: option(m.options, "ro")})
		}
		v := &out[idx]
		if m.root == "/" || relevant {
			found := false
			for _, p := range v.Paths {
				if p == m.path {
					found = true
				}
			}
			if !found {
				v.Paths = append(v.Paths, m.path)
			}
		}
		if i == systemIndex {
			v.IsSystem = true
			v.Mount = m.path
		}
		if i == dataIndex {
			v.IsData = true
			v.Mount = dataPath
			if m.path != dataPath {
				v.Paths = append(v.Paths, dataPath)
			}
		}
		if strings.HasPrefix(m.path, dataPath+"/") {
			if !v.IsData {
				v.Mount = m.path
			}
			v.IsData = true
		}
	}
	for i := range out {
		v := &out[i]
		stats, err := stat(v.Mount)
		if err != nil {
			v.Error = err.Error()
			continue
		}
		if stats.Total <= 0 || stats.Free < 0 || stats.Available < 0 || stats.Free > stats.Total || stats.Available > stats.Free || stats.InodesFree > stats.Inodes {
			v.Error = "Dateisystem meldet unplausible Werte"
			continue
		}
		v.Total, v.Used, v.Available, v.Reserved = stats.Total, stats.Total-stats.Free, stats.Available, stats.Free-stats.Available
		if denominator := v.Used + v.Available; denominator > 0 {
			v.UsedPercent = 100 * float64(v.Used) / float64(denominator)
		}
		v.Inodes, v.InodesUsed = stats.Inodes, stats.Inodes-stats.InodesFree
		sort.Strings(v.Paths)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsSystem != out[j].IsSystem {
			return out[i].IsSystem
		}
		if out[i].IsData != out[j].IsData {
			return out[i].IsData
		}
		return out[i].Mount < out[j].Mount
	})
	return out, warnings, nil
}

// Commands are fixed by the caller, bounded and never evaluated by a shell.
func Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	var output limitedBuffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return output.data, fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return output.data, nil
}

type limitedBuffer struct{ data []byte }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 1<<20 {
		return 0, errors.New("Werkzeugausgabe zu groß")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func boundedRun(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return Run(ctx, name, args...)
}
func FindDevice(devices []Device, name string) (Device, bool) {
	canonical := name
	if resolved, err := filepath.EvalSymlinks(name); err == nil {
		canonical = resolved
	}
	for _, d := range devices {
		if d.Name == name {
			return d, true
		}
		if resolved, err := filepath.EvalSymlinks(d.Name); err == nil && resolved == canonical {
			return d, true
		}
		if child, ok := FindDevice(d.Children, name); ok {
			return child, true
		}
	}
	return Device{}, false
}
