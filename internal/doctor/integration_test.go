package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func TestIntegrationChecks_AllStagesReported(t *testing.T) {
	originalLLMHealthCheck := llmHealthCheck
	originalCheckOllama := checkOllama
	originalGetOllamaModels := getOllamaModels
	originalCheckMCPInstalled := checkMCPInstalled
	t.Cleanup(func() {
		llmHealthCheck = originalLLMHealthCheck
		checkOllama = originalCheckOllama
		getOllamaModels = originalGetOllamaModels
		checkMCPInstalled = originalCheckMCPInstalled
	})

	llmHealthCheck = func(ctx context.Context, cfg *types.LLMConfig) error { return nil }
	checkOllama = func() (bool, bool, string, error) { return true, true, "/usr/bin/ollama", nil }
	getOllamaModels = func() ([]string, error) { return []string{"llama3:latest", "nomic-embed-text:latest"}, nil }
	checkMCPInstalled = func() bool { return true }

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
		LLM: types.LLMConfig{Provider: "local", Model: "llama3"},
		Watch: types.WatchConfig{
			Sources: []types.WatchSourceConfig{{Name: "ancora", Socket: socketPath}},
		},
		Daemon:   types.DaemonConfig{StatusFile: statusPath},
		Obsidian: types.ObsidianConfig{AutoSync: true, VaultDir: tmp},
	}

	steps := IntegrationChecks(cfg, graphPath)
	if len(steps) != 6 {
		t.Fatalf("steps = %d, want 6", len(steps))
	}
	for _, step := range steps {
		if step.Status != StepOK {
			t.Fatalf("step %q status = %s, want ok (%s)", step.Name, step.Status, step.Detail)
		}
	}
}

func TestIntegrationChecks_FailsWhenConfiguredLocalModelMissing(t *testing.T) {
	originalLLMHealthCheck := llmHealthCheck
	originalCheckOllama := checkOllama
	originalGetOllamaModels := getOllamaModels
	originalCheckMCPInstalled := checkMCPInstalled
	t.Cleanup(func() {
		llmHealthCheck = originalLLMHealthCheck
		checkOllama = originalCheckOllama
		getOllamaModels = originalGetOllamaModels
		checkMCPInstalled = originalCheckMCPInstalled
	})

	llmHealthCheck = func(ctx context.Context, cfg *types.LLMConfig) error { return nil }
	checkOllama = func() (bool, bool, string, error) { return true, true, "/usr/bin/ollama", nil }
	getOllamaModels = func() ([]string, error) { return []string{"mistral:latest"}, nil }
	checkMCPInstalled = func() bool { return false }

	cfg := &types.Config{LLM: types.LLMConfig{Provider: "local", Model: "llama3"}}
	steps := IntegrationChecks(cfg, filepath.Join(t.TempDir(), "missing-graph.json"))

	if got := steps[2]; got.Name != "LLM runtime" || got.Status != StepFail {
		t.Fatalf("LLM step = %+v, want fail", got)
	}
	if got := steps[4]; got.Name != "MCP registration" || got.Status != StepWarn {
		t.Fatalf("MCP step = %+v, want warn", got)
	}
}

func TestIntegrationChecks_FailsWhenEmbeddingModelMissing(t *testing.T) {
	originalLLMHealthCheck := llmHealthCheck
	originalCheckOllama := checkOllama
	originalGetOllamaModels := getOllamaModels
	originalCheckMCPInstalled := checkMCPInstalled
	t.Cleanup(func() {
		llmHealthCheck = originalLLMHealthCheck
		checkOllama = originalCheckOllama
		getOllamaModels = originalGetOllamaModels
		checkMCPInstalled = originalCheckMCPInstalled
	})

	llmHealthCheck = func(ctx context.Context, cfg *types.LLMConfig) error { return nil }
	checkOllama = func() (bool, bool, string, error) { return true, true, "/usr/bin/ollama", nil }
	getOllamaModels = func() ([]string, error) { return []string{"llama3:latest"}, nil }
	checkMCPInstalled = func() bool { return true }

	cfg := &types.Config{LLM: types.LLMConfig{Provider: "local", Model: "llama3"}}
	steps := IntegrationChecks(cfg, filepath.Join(t.TempDir(), "missing-graph.json"))

	if got := steps[2]; got.Name != "LLM runtime" || got.Status != StepFail {
		t.Fatalf("LLM step = %+v, want fail", got)
	}
	if got := steps[2].Detail; got != "Ollama embedding model \"nomic-embed-text\" is not available" {
		t.Fatalf("LLM detail = %q", got)
	}
}
