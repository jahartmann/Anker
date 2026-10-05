package anker

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
)

type DiskUsage struct {
	Total       int64 `json:"total"`
	Free        int64 `json:"free"`
	UsedPercent int   `json:"used_percent"`
}

func (s *Service) DiskUsage() (DiskUsage, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(s.Root, &stat); err != nil {
		return DiskUsage{}, err
	}
	v := DiskUsage{Total: int64(stat.Blocks) * int64(stat.Bsize), Free: int64(stat.Bavail) * int64(stat.Bsize)}
	if v.Total > 0 {
		v.UsedPercent = int(100 * (v.Total - v.Free) / v.Total)
	}
	return v, nil
}
func (s *Service) CheckSpace(required int64) error {
	v, err := s.DiskUsage()
	if err != nil {
		return err
	}
	if required < 64<<20 {
		required = 64 << 20
	}
	if v.Free < required {
		return fmt.Errorf("Zu wenig freier Speicher; benötigt mindestens %d MiB", required>>20)
	}
	return nil
}
func (s *Service) CheckNonessentialSpace() error {
	v, err := s.DiskUsage()
	if err != nil {
		return err
	}
	if v.UsedPercent >= 90 {
		return errors.New("Ablage mindestens 90 % voll; nicht erforderlichen Vorgang zurückgestellt")
	}
	return s.CheckSpace(64 << 20)
}
