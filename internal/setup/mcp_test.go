package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckMCPInstalled(t *testing.T) {
	// Create temp dir for test configs
	tmpDir := t.TempDir()

	// Create OpenCode config with vela
	openCodeDir := filepath.Join(tmpDir, ".config", "opencode")
	if err := os.MkdirAll(openCodeDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"vela": map[string]interface{}{
				"command": "vela",
				"args":    []string{"serve"},
			},
		},
	}

	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(openCodeDir, "mcp_settings.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Mock home directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test
	if !CheckMCPInstalled() {
		t.Error("Expected vela to be detected as installed")
	}
}

func TestCheckMCPNotInstalled(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty config
	openCodeDir := filepath.Join(tmpDir, ".config", "opencode")
	if err := os.MkdirAll(openCodeDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := map[string]interface{}{
		"mcpServers": map[string]interface{}{},
	}

	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(openCodeDir, "mcp_settings.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	if CheckMCPInstalled() {
		t.Error("Expected vela to NOT be detected")
	}
}
