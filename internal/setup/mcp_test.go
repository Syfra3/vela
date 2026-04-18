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

func TestUninstallMCPRemovesVelaEntries(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	openCodePath := filepath.Join(tmpDir, ".config", "opencode", "mcp_settings.json")
	claudeDesktopPath := filepath.Join(tmpDir, ".config", "claude", "claude_desktop_config.json")
	claudeCodePath := filepath.Join(tmpDir, ".claude", "mcp", "vela.json")

	for _, path := range []string{openCodePath, claudeDesktopPath, claudeCodePath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	configWithExtra := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"vela":  map[string]interface{}{"command": "vela", "args": []string{"serve"}},
			"other": map[string]interface{}{"command": "other"},
		},
	}
	data, _ := json.Marshal(configWithExtra)
	if err := os.WriteFile(openCodePath, data, 0o644); err != nil {
		t.Fatalf("write opencode config: %v", err)
	}
	if err := os.WriteFile(claudeDesktopPath, data, 0o644); err != nil {
		t.Fatalf("write claude desktop config: %v", err)
	}
	if err := os.WriteFile(claudeCodePath, []byte(`{"command":"vela"}`), 0o644); err != nil {
		t.Fatalf("write claude code config: %v", err)
	}

	if err := UninstallMCP(); err != nil {
		t.Fatalf("UninstallMCP() error = %v", err)
	}

	for _, path := range []string{openCodePath, claudeDesktopPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var cfg map[string]interface{}
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		mcpServers := cfg["mcpServers"].(map[string]interface{})
		if _, exists := mcpServers["vela"]; exists {
			t.Fatalf("vela entry still present in %s", path)
		}
		if _, exists := mcpServers["other"]; !exists {
			t.Fatalf("other entry missing from %s", path)
		}
	}

	if _, err := os.Stat(claudeCodePath); !os.IsNotExist(err) {
		t.Fatalf("expected Claude Code MCP file to be removed, stat err = %v", err)
	}
}
