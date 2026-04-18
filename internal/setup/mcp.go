package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// CheckMCPInstalled checks if vela is configured in OpenCode or Claude Desktop MCP settings.
func CheckMCPInstalled() bool {
	// Check OpenCode first
	if checkOpenCode() {
		return true
	}
	// Fallback to Claude Desktop
	return checkClaudeDesktop()
}

// UninstallMCP removes Vela's MCP registration from supported client configs.
func UninstallMCP() error {
	for _, path := range []string{getOpenCodeConfigPath(), getClaudeDesktopConfigPath()} {
		if path == "" {
			continue
		}
		if err := removeServerEntry(path, "vela"); err != nil {
			return err
		}
	}

	claudeCodePath := getClaudeCodeConfigPath()
	if claudeCodePath != "" {
		if err := os.Remove(claudeCodePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing Claude Code MCP config: %w", err)
		}
	}

	return nil
}

func removeServerEntry(configPath, serverName string) error {
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading MCP config %s: %w", configPath, err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing MCP config %s: %w", configPath, err)
	}

	mcpServers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		return nil
	}
	if _, exists := mcpServers[serverName]; !exists {
		return nil
	}

	delete(mcpServers, serverName)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding MCP config %s: %w", configPath, err)
	}
	if err := os.WriteFile(configPath, updated, 0644); err != nil {
		return fmt.Errorf("writing MCP config %s: %w", configPath, err)
	}

	return nil
}

func checkOpenCode() bool {
	configPath := getOpenCodeConfigPath()
	if configPath == "" {
		return false
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}

	// Check if mcpServers.vela exists
	mcpServers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		return false
	}

	_, velaExists := mcpServers["vela"]
	return velaExists
}

func checkClaudeDesktop() bool {
	configPath := getClaudeDesktopConfigPath()
	if configPath == "" {
		return false
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}

	mcpServers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		return false
	}

	_, velaExists := mcpServers["vela"]
	return velaExists
}

func getOpenCodeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// OpenCode uses different config structure than Claude Code
	switch runtime.GOOS {
	case "linux":
		return filepath.Join(home, ".config", "opencode", "mcp_settings.json")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "OpenCode", "mcp_settings.json")
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "OpenCode", "mcp_settings.json")
	default:
		return ""
	}
}

func getClaudeCodeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Claude Code uses ~/.claude/mcp/ directory for MCP server configs
	// Each server gets its own JSON file (e.g., vela.json)
	return filepath.Join(home, ".claude", "mcp", "vela.json")
}

func getClaudeDesktopConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	switch runtime.GOOS {
	case "linux":
		return filepath.Join(home, ".config", "claude", "claude_desktop_config.json")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "Claude", "claude_desktop_config.json")
	default:
		return ""
	}
}
