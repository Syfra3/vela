//go:build !windows

package generation

import (
	"os"
	"syscall"
)

type lockMode int

const (
	lockShared    lockMode = syscall.LOCK_SH
	lockExclusive lockMode = syscall.LOCK_EX
)

func lockFile(f *os.File, mode lockMode) error {
	return syscall.Flock(int(f.Fd()), int(mode)|syscall.LOCK_NB)
}

func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

func lockUnavailable(err error) bool {
	return err == syscall.EWOULDBLOCK || err == syscall.EAGAIN
}

func openDirectory(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
