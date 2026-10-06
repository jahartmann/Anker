//go:build !linux

package storage

import (
	"golang.org/x/sys/unix"
	"time"
)

func Inspect(dataPath string) (Report, error) {
	r := Report{CollectedAt: time.Now().UTC().Format(time.RFC3339), DataPath: dataPath, Environment: "unsupported", Volumes: []Volume{}, Devices: []Device{}, Warnings: []string{"Vollständige Laufwerkerkennung und Erweiterung benötigen Linux."}}
	var s unix.Statfs_t
	if err := unix.Statfs(dataPath, &s); err != nil {
		return r, err
	}
	total, free, available := int64(s.Blocks)*int64(s.Bsize), int64(s.Bfree)*int64(s.Bsize), int64(s.Bavail)*int64(s.Bsize)
	v := Volume{ID: Identity(dataPath), Mount: dataPath, Paths: []string{dataPath}, IsData: true, Total: total, Used: total - free, Available: available, Reserved: free - available, Inodes: int64(s.Files), InodesUsed: int64(s.Files) - int64(s.Ffree)}
	if v.Used+available > 0 {
		v.UsedPercent = 100 * float64(v.Used) / float64(v.Used+available)
	}
	r.Volumes = append(r.Volumes, v)
	return r, nil
}
