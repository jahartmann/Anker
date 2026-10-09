package anker

import "golang.org/x/sys/unix"

func disasterPublish(fd int, from, to string) error {
	return unix.Renameat2(fd, from, fd, to, unix.RENAME_NOREPLACE)
}
