//go:build windows

package graph

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

const (
	lockfileFailImmediately = 1
	lockfileExclusiveLock   = 2
)

func lockFile(f *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(f.Fd()), lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0, &overlapped)
	if err != nil {
		if err == windows.ERROR_LOCK_VIOLATION {
			return ErrGraphLocked
		}
		return fmt.Errorf("acquiring graph lock: %w", err)
	}
	return nil
}

func unlockFile(f *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped)
}
