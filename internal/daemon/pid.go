// Package daemon manages the Vela watch daemon lifecycle: PID file management,
// process detection, start/stop/status operations, and service installation.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ErrNotRunning is returned when a stop/status operation finds no running daemon.
var ErrNotRunning = errors.New("daemon is not running")

// ErrAlreadyRunning is returned when a start operation finds a daemon already running.
var ErrAlreadyRunning = errors.New("daemon is already running")

// PIDFile manages a file that stores the PID of the running daemon.
type PIDFile struct {
	path string
}

// NewPIDFile creates a PIDFile for the given path.
// The directory is created if it does not exist.
func NewPIDFile(path string) (*PIDFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating pid dir: %w", err)
	}
	return &PIDFile{path: path}, nil
}

// Write records the current process PID into the file.
func (p *PIDFile) Write() error {
	return p.WritePID(os.Getpid())
}

// WritePID records an arbitrary PID into the file (used when fork-exec'ing).
func (p *PIDFile) WritePID(pid int) error {
	return os.WriteFile(p.path, []byte(strconv.Itoa(pid)+"\n"), 0644)
}

// Read returns the PID stored in the file. Returns ErrNotRunning if the file
// does not exist.
func (p *PIDFile) Read() (int, error) {
	data, err := os.ReadFile(p.path)
	if os.IsNotExist(err) {
		return 0, ErrNotRunning
	}
	if err != nil {
		return 0, fmt.Errorf("reading pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid file content: %w", err)
	}
	return pid, nil
}

// Remove deletes the PID file (called on clean shutdown).
func (p *PIDFile) Remove() error {
	err := os.Remove(p.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// IsAlive returns true if the recorded PID corresponds to a running process.
func (p *PIDFile) IsAlive() (bool, int, error) {
	pid, err := p.Read()
	if errors.Is(err, ErrNotRunning) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		// On Unix, FindProcess always succeeds; error means invalid PID.
		_ = p.Remove()
		return false, 0, nil
	}
	// Signal 0 checks if the process exists without sending an actual signal.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process no longer exists — remove stale PID file.
		_ = p.Remove()
		return false, 0, nil
	}
	return true, pid, nil
}
