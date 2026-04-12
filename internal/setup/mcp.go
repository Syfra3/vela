package setup

import (
	"encoding/json"
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
