package agentinstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCodeInstallMergesRealConfigMCPShape(t *testing.T) {
	projectDir := t.TempDir()
	opencodeDir := t.TempDir()
	configPath := filepath.Join(opencodeDir, "opencode.json")
	if err := os.WriteFile(configPath, []byte(`{"username":"geen","mcp":{"playwright":{"type":"local","command":["npx","@playwright/mcp"],"enabled":true}},"instructions":["AGENTS.md"]}`), 0o644); err != nil {
		t.Fatalf("write opencode config: %v", err)
	}

	result, err := Install(Request{ProjectDir: projectDir, Agent: "opencode", ConfigDir: opencodeDir})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.MCPConfigPath != configPath {
		t.Fatalf("MCPConfigPath = %q, want %q", result.MCPConfigPath, configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read opencode config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("merged config is not JSON: %v\n%s", err, data)
	}
	if cfg["username"] != "geen" {
		t.Fatalf("existing config was not preserved: %v", cfg)
	}
	mcp := cfg["mcp"].(map[string]any)
	if _, ok := mcp["playwright"]; !ok {
		t.Fatalf("existing MCP entry was not preserved: %v", mcp)
	}
	vela := mcp["vela"].(map[string]any)
	if vela["type"] != "local" || vela["enabled"] != true {
		t.Fatalf("vela MCP entry = %#v, want enabled local", vela)
	}
	command := vela["command"].([]any)
	if len(command) != 2 || command[0] != "vela" || command[1] != "serve" {
		t.Fatalf("vela command = %#v, want [vela serve]", command)
	}
	if _, err := os.Stat(filepath.Join(opencodeDir, "instructions.md")); err != nil {
		t.Fatalf("expected instructions file: %v", err)
	}
}

func TestOpenCodeInstallIsIdempotentInConfig(t *testing.T) {
	projectDir := t.TempDir()
	opencodeDir := t.TempDir()
	for i := 0; i < 2; i++ {
		if _, err := Install(Request{ProjectDir: projectDir, Agent: "opencode", ConfigDir: opencodeDir}); err != nil {
			t.Fatalf("Install run %d error = %v", i, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(opencodeDir, "opencode.json"))
	if err != nil {
		t.Fatalf("read opencode config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	instructions := cfg["instructions"].([]any)
	var count int
	for _, item := range instructions {
		if item == "instructions.md" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("instructions.md count = %d, want 1 in %v", count, instructions)
	}
}
