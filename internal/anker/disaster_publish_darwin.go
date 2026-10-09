package anker

import "golang.org/x/sys/unix"

func disasterPublish(fd int, from, to string) error {
	return unix.RenameatxNp(fd, from, fd, to, unix.RENAME_EXCL)
}
