package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func TestIntegrationChecks_AllStagesReported(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	socketPath := filepath.Join(tmp, "ancora.sock")
	if err := os.WriteFile(socketPath, nil, 0o644); err != nil {
		t.Fatalf("write socket stub: %v", err)
	}

	statusPath := filepath.Join(tmp, "watch-status.json")
	statusJSON := `{"pid":123,"sources":{"ancora":{"connected":true,"event_count":2}},"updated_at":"2026-04-17T00:00:00Z"}`
	if err := os.WriteFile(statusPath, []byte(statusJSON), 0o644); err != nil {
		t.Fatalf("write status file: %v", err)
	}

	graphPath := filepath.Join(tmp, "graph.json")
	graphJSON := `{"nodes":[{"id":"ancora:obs:1","label":"Realtime memory","kind":"observation","source_type":"memory","source_name":"ancora"}],"edges":[],"meta":{"generatedAt":"2026-04-17T00:00:00Z"}}`
	if err := os.WriteFile(graphPath, []byte(graphJSON), 0o644); err != nil {
		t.Fatalf("write graph file: %v", err)
	}

	obsNote := filepath.Join(tmp, "obsidian", "Memories", "vela", "work", "Realtime memory.md")
	if err := os.MkdirAll(filepath.Dir(obsNote), 0o755); err != nil {
		t.Fatalf("mkdir obsidian: %v", err)
	}
	if err := os.WriteFile(obsNote, []byte("# Realtime memory\n"), 0o644); err != nil {
		t.Fatalf("write obsidian note: %v", err)
	}

	cfg := &types.Config{
		Watch: types.WatchConfig{
			Sources: []types.WatchSourceConfig{{Name: "ancora", Socket: socketPath}},
		},
		Daemon:   types.DaemonConfig{StatusFile: statusPath},
		Obsidian: types.ObsidianConfig{AutoSync: true, VaultDir: tmp},
	}

	steps := IntegrationChecks(cfg, graphPath)
	if len(steps) != 4 {
		t.Fatalf("steps = %d, want 4", len(steps))
	}
	for _, step := range steps {
		if step.Status != StepOK {
			t.Fatalf("step %q status = %s, want ok (%s)", step.Name, step.Status, step.Detail)
		}
	}
}
