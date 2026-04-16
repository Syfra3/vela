package daemon

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestPIDFileReadWriteRemoveLifecycle(t *testing.T) {
	t.Parallel()

	pf, err := NewPIDFile(filepath.Join(t.TempDir(), "vela", "watch.pid"))
	if err != nil {
		t.Fatalf("NewPIDFile() error = %v", err)
	}

	if err := pf.WritePID(12345); err != nil {
		t.Fatalf("WritePID() error = %v", err)
	}
	pid, err := pf.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if pid != 12345 {
		t.Fatalf("Read() pid = %d, want 12345", pid)
	}

	if err := pf.Remove(); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	_, err = pf.Read()
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Read() after remove error = %v, want ErrNotRunning", err)
	}
}

func TestPIDFileIsAliveRemovesStalePIDFile(t *testing.T) {
	t.Parallel()

	pf, err := NewPIDFile(filepath.Join(t.TempDir(), "watch.pid"))
	if err != nil {
		t.Fatalf("NewPIDFile() error = %v", err)
	}

	if err := pf.WritePID(999999); err != nil {
		t.Fatalf("WritePID() error = %v", err)
	}

	alive, pid, err := pf.IsAlive()
	if err != nil {
		t.Fatalf("IsAlive() error = %v", err)
	}
	if alive || pid != 0 {
		t.Fatalf("IsAlive() = (%v, %d), want (false, 0)", alive, pid)
	}
	_, err = pf.Read()
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Read() after stale cleanup error = %v, want ErrNotRunning", err)
	}
}
