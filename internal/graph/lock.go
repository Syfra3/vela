package graph

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// GraphLock holds an exclusive advisory lock on ~/.vela/graph.lock so the
// daemon and vela extract cannot write graph.json concurrently.
type GraphLock struct {
	f *os.File
}

// ErrGraphLocked is returned when another process holds the graph lock.
var ErrGraphLocked = errors.New("graph is locked by another process (daemon running?); stop the daemon first")

// AcquireGraphLock opens the lock file and acquires a non-blocking exclusive
// lock. Returns ErrGraphLocked if another process already holds the lock.
func AcquireGraphLock(lockPath string) (*GraphLock, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &GraphLock{f: f}, nil
}

// Release unlocks and closes the lock file. Safe to call on a nil receiver.
func (l *GraphLock) Release() {
	if l == nil || l.f == nil {
		return
	}
	_ = unlockFile(l.f)
	_ = l.f.Close()
	l.f = nil
}
