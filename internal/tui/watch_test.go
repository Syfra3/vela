package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecoverRuntimeReportsAncoraOffline(t *testing.T) {
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})

	home := t.TempDir()
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}

	restore := stubWatchRecoveryDeps(false)
	defer restore()

	msg, err := recoverRuntime()
	if err != nil {
		t.Fatalf("recoverRuntime error: %v", err)
	}
	want := "Vela daemon restarted, but Ancora is still offline (start: ancora mcp)."
	if msg != want {
		t.Fatalf("unexpected message\nwant: %q\ngot:  %q", want, msg)
	}
}

func TestRecoverRuntimeReportsHealthyRuntime(t *testing.T) {
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})

	home := t.TempDir()
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}

	restore := stubWatchRecoveryDeps(true)
	defer restore()

	msg, err := recoverRuntime()
	if err != nil {
		t.Fatalf("recoverRuntime error: %v", err)
	}
	want := "Runtime recovered: Vela daemon restarted and Ancora socket is reachable."
	if msg != want {
		t.Fatalf("unexpected message\nwant: %q\ngot:  %q", want, msg)
	}
}

func TestWaitForDaemonStateTimesOut(t *testing.T) {
	pidFilePath := filepath.Join(t.TempDir(), "watch.pid")
	start := time.Now()
	err := waitForDaemonState(pidFilePath, true, 250*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if err.Error() != "daemon did not start before timeout" {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) < 250*time.Millisecond {
		t.Fatal("waitForDaemonState returned too early")
	}
}

func stubWatchRecoveryDeps(ancoraOnline bool) func() {
	originalStart := watchStartDaemon
	originalStop := watchStopDaemon
	originalWait := watchWaitForDaemonState
	originalProbe := watchSocketProbe

	watchStartDaemon = func() error { return nil }
	watchStopDaemon = func() error { return nil }
	watchWaitForDaemonState = func(string, bool, time.Duration) error { return nil }
	watchSocketProbe = func() bool { return ancoraOnline }

	return func() {
		watchStartDaemon = originalStart
		watchStopDaemon = originalStop
		watchWaitForDaemonState = originalWait
		watchSocketProbe = originalProbe
	}
}
