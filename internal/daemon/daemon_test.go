package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/pkg/types"
)

func TestDaemonStartStopLifecycle(t *testing.T) {
	t.Parallel()

	d := newTestDaemon(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if status := d.Status(); !strings.Contains(status, "running") {
		t.Fatalf("Status() = %q, want running", status)
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if status := d.Status(); status != "stopped" {
		t.Fatalf("Status() after stop = %q, want stopped", status)
	}
}

func TestDaemonStartReturnsAlreadyRunning(t *testing.T) {
	t.Parallel()

	d := newTestDaemon(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer func() { _ = d.Stop() }()

	err := d.Start(ctx)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Start() error = %v, want ErrAlreadyRunning", err)
	}
}

func TestDaemonStopWhenNotRunning(t *testing.T) {
	t.Parallel()

	d := newTestDaemon(t)
	err := d.Stop()
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Stop() error = %v, want ErrNotRunning", err)
	}
}

func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()

	g, err := igraph.Build(nil, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	cfg := &types.Config{
		LLM: types.LLMConfig{},
		Watch: types.WatchConfig{
			Enabled: false,
			Sources: nil,
			Reconciler: types.ReconcilerConfig{
				DebounceMs:   0,
				MaxBatchSize: 10,
			},
			Extractor: types.ExtractorConfig{Enabled: false},
		},
		Daemon: types.DaemonConfig{
			PIDFile: filepath.Join(t.TempDir(), "watch.pid"),
			LogFile: "",
		},
	}

	d, err := New(cfg, g)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return d
}
