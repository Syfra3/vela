package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/extract"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/listener"
	"github.com/Syfra3/vela/internal/reconcile"
	"github.com/Syfra3/vela/pkg/types"
)

// Daemon orchestrates the Vela watch subsystem: it manages listener lifecycle,
// feeds events through the reconciler pipeline, and drives LLM extraction.
type Daemon struct {
	cfgMu   sync.RWMutex
	cfg     *types.Config
	pidFile *PIDFile

	registry *listener.Registry
	queue    *reconcile.Queue
	differ   *reconcile.Differ
	patcher  *reconcile.Patcher
	graph    *igraph.Graph
	logFile  *os.File

	stopCh    chan struct{}
	stoppedCh chan struct{}
}

// New creates a Daemon from the provided configuration and graph.
// The graph is patched in-place as events arrive.
func New(cfg *types.Config, g *igraph.Graph) (*Daemon, error) {
	pidFile, err := NewPIDFile(cfg.Daemon.PIDFile)
	if err != nil {
		return nil, fmt.Errorf("pid file: %w", err)
	}

	debounce := time.Duration(cfg.Watch.Reconciler.DebounceMs) * time.Millisecond
	queue := reconcile.NewQueue(debounce, cfg.Watch.Reconciler.MaxBatchSize)
	patcher := reconcile.NewPatcher(g, 128)

	return &Daemon{
		cfg:       cfg,
		pidFile:   pidFile,
		registry:  listener.NewRegistry(),
		queue:     queue,
		differ:    reconcile.NewDiffer(),
		patcher:   patcher,
		graph:     g,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}, nil
}

// Start launches the daemon. It returns ErrAlreadyRunning if a daemon is
// already running (PID file exists and process is alive).
func (d *Daemon) Start(ctx context.Context) error {
	alive, pid, err := d.pidFile.IsAlive()
	if err != nil {
		return fmt.Errorf("checking pid: %w", err)
	}
	if alive {
		return fmt.Errorf("%w (pid %d)", ErrAlreadyRunning, pid)
	}

	// Write our PID before starting goroutines so a concurrent Start sees it.
	if err := d.pidFile.Write(); err != nil {
		return fmt.Errorf("writing pid: %w", err)
	}

	// Open log file if configured.
	if d.cfg.Daemon.LogFile != "" {
		lf, err := openAppend(d.cfg.Daemon.LogFile)
		if err != nil {
			log.Printf("WARN: cannot open log file %s: %v", d.cfg.Daemon.LogFile, err)
		} else {
			d.logFile = lf
			log.SetOutput(lf)
		}
	}

	// Connect all configured sources.
	for _, srcCfg := range d.cfg.Watch.Sources {
		src := d.buildSource(srcCfg)
		if err := d.registry.Add(ctx, src); err != nil {
			log.Printf("WARN: failed to connect source %s: %v", srcCfg.Name, err)
			// Continue — source may become available after reconnect.
		}
	}

	// Ingest loop: push incoming events into the queue.
	go d.ingestLoop(ctx)

	// Reconcile loop: drain the queue and apply changesets to the graph.
	go d.reconcileLoop(ctx)

	// LLM extractor loop (if enabled).
	if d.cfg.Watch.Extractor.Enabled {
		extCfg := &types.LLMConfig{
			Provider: d.cfg.Watch.Extractor.Provider,
			Model:    d.cfg.Watch.Extractor.Model,
			Endpoint: d.cfg.LLM.Endpoint,
			Timeout:  60 * time.Second,
		}
		ext, err := extract.NewLLMExtractor(extCfg, d.cfg.Watch.Extractor.Workers, nil)
		if err != nil {
			log.Printf("WARN: LLM extractor unavailable: %v", err)
		} else {
			go ext.Start(ctx, d.patcher.LLMQueue())
		}
	}

	// Status writer: periodically flush source connectivity to disk so
	// the CLI can read it without cross-process registry access.
	go d.statusLoop(ctx)

	// SIGHUP handler: reload config without restarting the daemon.
	go d.sighupLoop(ctx)

	log.Printf("INFO: vela watch daemon started (pid %d)", os.Getpid())
	return nil
}

// Stop shuts the daemon down cleanly and removes the PID file.
func (d *Daemon) Stop() error {
	alive, pid, err := d.pidFile.IsAlive()
	if err != nil {
		return err
	}
	if !alive {
		return ErrNotRunning
	}

	// Signal the running process to stop. If it's us, close stopCh.
	if pid == os.Getpid() {
		close(d.stopCh)
		<-d.stoppedCh
		d.registry.Close()
		_ = d.pidFile.Remove()
		if d.logFile != nil {
			_ = d.logFile.Close()
		}
		return nil
	}

	// Cross-process stop: send SIGTERM.
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(os.Interrupt)
}

// Status returns a human-readable status string for the daemon.
// When called cross-process it reads the status file written by statusLoop.
func (d *Daemon) Status() string {
	alive, pid, err := d.pidFile.IsAlive()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	if !alive {
		// Remove stale status file so next start is clean.
		if d.cfg.Daemon.StatusFile != "" {
			_ = os.Remove(d.cfg.Daemon.StatusFile)
		}
		return "stopped"
	}

	// Same process: read in-memory registry (accurate, immediate).
	if pid == os.Getpid() {
		statuses := d.registry.Statuses()
		connected := 0
		for _, s := range statuses {
			if s.Connected {
				connected++
			}
		}
		return fmt.Sprintf("running (pid %d) — %d/%d sources connected", pid, connected, len(statuses))
	}

	// Cross-process: read status file written by the daemon's statusLoop.
	if d.cfg.Daemon.StatusFile != "" {
		if data, err := os.ReadFile(d.cfg.Daemon.StatusFile); err == nil {
			var ds types.DaemonStatus
			if json.Unmarshal(data, &ds) == nil {
				connected := 0
				for _, s := range ds.Sources {
					if s.Connected {
						connected++
					}
				}
				return fmt.Sprintf("running (pid %d) — %d/%d sources connected", pid, connected, len(ds.Sources))
			}
		}
	}

	// Status file not available yet (daemon just started).
	return fmt.Sprintf("running (pid %d) — connecting...", pid)
}

// Registry returns the listener registry for status queries.
func (d *Daemon) Registry() *listener.Registry {
	return d.registry
}

// ---------------------------------------------------------------------------
// Internal loops
// ---------------------------------------------------------------------------

func (d *Daemon) ingestLoop(ctx context.Context) {
	events := d.registry.Events()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			key := reconcile.EntityKey(ev)
			d.queue.Push(ev, key)
		case <-d.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (d *Daemon) reconcileLoop(ctx context.Context) {
	defer close(d.stoppedCh)
	for {
		select {
		case <-d.queue.Ready():
			batch := d.queue.Drain()
			if len(batch) == 0 {
				continue
			}
			cs, err := d.differ.Diff(batch)
			if err != nil {
				log.Printf("ERROR: differ: %v", err)
				continue
			}
			if cs.IsEmpty() {
				continue
			}
			if err := d.patcher.Apply(cs); err != nil {
				log.Printf("ERROR: patcher: %v", err)
			} else {
				log.Printf("INFO: reconcile: +%d ~%d -%d",
					len(cs.Added), len(cs.Updated), len(cs.Deleted))
				d.obsidianSync()
			}
		case <-d.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// buildSource constructs an EventSource from a WatchSourceConfig.
func (d *Daemon) buildSource(cfg types.WatchSourceConfig) listener.EventSource {
	// Currently only "syfra" sources are supported (Ancora-compatible sockets).
	// Future: "webhook" type for GitLab/GitHub.
	home, _ := os.UserHomeDir()
	socketPath := expandHome(cfg.Socket, home)
	return listener.NewAncoraListener(socketPath, "")
}

func openAppend(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

// statusLoop writes source connectivity to the status file every 2s so the
// CLI can display accurate cross-process status without IPC.
func (d *Daemon) statusLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	// Write once immediately on start.
	d.writeStatus()
	for {
		select {
		case <-ticker.C:
			d.writeStatus()
		case <-d.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (d *Daemon) writeStatus() {
	if d.cfg.Daemon.StatusFile == "" {
		return
	}
	statuses := d.registry.Statuses()
	sources := make(map[string]types.DaemonSourceStatus, len(statuses))
	for name, s := range statuses {
		sources[name] = types.DaemonSourceStatus{
			Connected:  s.Connected,
			EventCount: s.EventCount,
		}
	}
	ds := types.DaemonStatus{
		PID:       os.Getpid(),
		Sources:   sources,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	data, err := json.Marshal(ds)
	if err != nil {
		return
	}
	_ = os.WriteFile(d.cfg.Daemon.StatusFile, data, 0644)
}

func expandHome(path, home string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		return home + path[1:]
	}
	return path
}

// ---------------------------------------------------------------------------
// Obsidian auto-sync
// ---------------------------------------------------------------------------

// obsidianSync writes the current in-memory graph to the Obsidian vault if
// auto_sync is enabled in config. Called after every successful reconcile.
func (d *Daemon) obsidianSync() {
	d.cfgMu.RLock()
	obs := d.cfg.Obsidian
	d.cfgMu.RUnlock()

	if !obs.AutoSync || obs.VaultDir == "" {
		return
	}

	tg := d.graph.ToTypes()
	if err := export.WriteObsidian(tg, obs.VaultDir); err != nil {
		log.Printf("WARN: obsidian auto-sync: %v", err)
		return
	}
	log.Printf("INFO: obsidian synced → %s/obsidian/", obs.VaultDir)
}

// ---------------------------------------------------------------------------
// SIGHUP hot-reload
// ---------------------------------------------------------------------------

// sighupLoop listens for SIGHUP and reloads config from disk without
// restarting the daemon. Follows the same pattern as nginx -s reload.
func (d *Daemon) sighupLoop(ctx context.Context) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	defer signal.Stop(ch)

	for {
		select {
		case <-ch:
			d.reloadConfig()
		case <-d.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// reloadConfig reads ~/.vela/config.yaml and hot-swaps the cfg pointer.
// Uses cfgMu so concurrent readers (obsidianSync) see a consistent view.
func (d *Daemon) reloadConfig() {
	// Import config inline to avoid circular import — config is a sibling
	// package so we just re-read the file via os + yaml directly.
	// Simpler: delegate to the config package via a loader func.
	// The daemon package already imports config indirectly through main.go,
	// but to keep daemon self-contained we use a package-level loader var.
	if configLoader == nil {
		log.Printf("WARN: SIGHUP received but no config loader registered")
		return
	}
	newCfg, err := configLoader()
	if err != nil {
		log.Printf("WARN: SIGHUP config reload failed: %v", err)
		return
	}
	d.cfgMu.Lock()
	d.cfg = newCfg
	d.cfgMu.Unlock()
	log.Printf("INFO: config reloaded (obsidian.auto_sync=%v vault_dir=%s)",
		newCfg.Obsidian.AutoSync, newCfg.Obsidian.VaultDir)
}

// configLoader is set by the caller (main.go) to avoid circular imports.
// It should return the current config from disk.
var configLoader func() (*types.Config, error)

// SetConfigLoader registers the function the daemon uses to reload config on
// SIGHUP. Call this before daemon.Start(). Keeps daemon package free of a
// direct import on internal/config (avoids circular dependency risk).
func SetConfigLoader(fn func() (*types.Config, error)) {
	configLoader = fn
}
