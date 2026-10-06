package updater

import (
	"errors"
	"golang.org/x/sys/unix"
	"unsafe"
)

func verifyStorageDevice(path, mount string, want int64) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return errors.New("Blockgerät ist nicht zugänglich")
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFBLK || stat.Uid != 0 || stat.Mode&0002 != 0 {
		return errors.New("Kein geschütztes lokales Blockgerät")
	}
	var target unix.Stat_t
	if err = unix.Stat(mount, &target); err != nil || target.Mode&unix.S_IFMT != unix.S_IFDIR || target.Dev != stat.Rdev {
		return errors.New("Mountpoint und Blockgerät gehören nicht zum selben Dateisystem; erneut prüfen")
	}
	var size uint64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.BLKGETSIZE64), uintptr(unsafe.Pointer(&size)))
	if errno != 0 || size != uint64(want) {
		return errors.New("Gerätegröße hat sich verändert; erneut prüfen")
	}
	return nil
}
