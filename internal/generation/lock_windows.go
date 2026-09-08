//go:build windows

package generation

import (
	"os"

	"golang.org/x/sys/windows"
)

type lockMode uint32

const (
	lockShared    lockMode = 0
	lockExclusive lockMode = 2 // LOCKFILE_EXCLUSIVE_LOCK
	lockImmediate          = 1 // LOCKFILE_FAIL_IMMEDIATELY
)

func lockFile(f *os.File, mode lockMode) error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(windows.Handle(f.Fd()), uint32(mode)|lockImmediate, 0, 1, 0, &overlapped)
}

func unlockFile(f *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped)
}

func lockUnavailable(err error) bool {
	return err == windows.ERROR_LOCK_VIOLATION
}

func openDirectory(path string) (*os.File, error) {
	return os.Open(path)
}
