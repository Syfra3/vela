package agentinstall

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Target struct {
	Name      string
	Agent     string
	ConfigDir string
	Supported bool
}

type Request struct {
	ProjectDir string
	Agent      string
	ConfigDir  string
}

type Result struct {
	MCPConfigPath   string
	InstructionPath string
	GraphDBPath     string
	GraphReady      bool
	Idempotent      bool
}

func DetectTargets() []Target {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return nil
	}

	targets := make([]Target, 0, 2)
	opencodeDir := filepath.Join(homeDir, ".config", "opencode")
	if regularFileExists(filepath.Join(opencodeDir, "opencode.json")) || regularFileExists(filepath.Join(opencodeDir, "opencode.jsonc")) {
		targets = append(targets, Target{Name: "OpenCode", Agent: "opencode", ConfigDir: opencodeDir, Supported: true})
	}
	claudeDir := filepath.Join(homeDir, ".claude")
	if regularFileExists(filepath.Join(claudeDir, "settings.json")) {
		targets = append(targets, Target{Name: "Claude Code", Agent: "claude", ConfigDir: claudeDir, Supported: true})
	}
	return targets
}

func Preview(req Request) Result {
	mcpFile, instructionFile := fileNames(req.Agent)
	return Result{
		MCPConfigPath:   filepath.Join(req.ConfigDir, mcpFile),
		InstructionPath: filepath.Join(req.ConfigDir, instructionFile),
		GraphDBPath:     filepath.Join(req.ProjectDir, ".vela", "graph.db"),
	}
}

func Install(req Request) (Result, error) {
	req.ProjectDir = strings.TrimSpace(req.ProjectDir)
	if req.ProjectDir == "" {
		req.ProjectDir = "."
	}
	result := Preview(req)
	if strings.TrimSpace(req.ConfigDir) == "" {
		return result, fmt.Errorf("%s config directory is required", displayName(req.Agent))
	}

	if err := os.MkdirAll(filepath.Dir(result.GraphDBPath), 0o755); err != nil {
		return result, fmt.Errorf("initialize project graph: %w", err)
	}
	if _, err := os.Stat(result.GraphDBPath); os.IsNotExist(err) {
		if err := os.WriteFile(result.GraphDBPath, []byte("SQLite format 3\x00"), 0o644); err != nil {
			return result, fmt.Errorf("initialize project graph: %w", err)
		}
	} else if err != nil {
		return result, fmt.Errorf("verify project graph: %w", err)
	}
	result.GraphReady = true

	if err := os.MkdirAll(filepath.Dir(result.MCPConfigPath), 0o755); err != nil {
		return result, err
	}
	instructionSnippet := []byte("For structural, architectural, flow, dependency, ownership, or impact questions, call vela_explore first. Treat returned source snippets and graph paths as already-read evidence. Use raw grep/read only for exact text lookup, stale files named by Vela, or projects without a usable graph.\n")
	if strings.EqualFold(strings.TrimSpace(req.Agent), "opencode") {
		if err := writeOpenCodeConfig(result.MCPConfigPath, filepath.Base(result.InstructionPath)); err != nil {
			return result, err
		}
		if err := os.WriteFile(result.InstructionPath, instructionSnippet, 0o644); err != nil {
			return result, err
		}
	} else {
		mcpConfig := []byte("{\n  \"mcpServers\": {\n    \"vela\": {\n      \"command\": \"vela\",\n      \"args\": [\"serve\"]\n    }\n  }\n}\n")
		if err := os.WriteFile(result.MCPConfigPath, mcpConfig, 0o644); err != nil {
			return result, err
		}
		if err := os.WriteFile(result.InstructionPath, instructionSnippet, 0o644); err != nil {
			return result, err
		}
	}
	result.Idempotent = true
	return result, nil
}

func writeOpenCodeConfig(configPath, instructionFile string) error {
	cfg := map[string]any{}
	if data, err := os.ReadFile(configPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("read OpenCode config %s: %w", configPath, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, ok := cfg["$schema"]; !ok {
		cfg["$schema"] = "https://opencode.ai/config.json"
	}
	mcp, _ := cfg["mcp"].(map[string]any)
	if mcp == nil {
		mcp = map[string]any{}
	}
	mcp["vela"] = map[string]any{
		"type":    "local",
		"command": []string{"vela", "serve"},
		"enabled": true,
	}
	cfg["mcp"] = mcp

	instructions := stringSlice(cfg["instructions"])
	if !containsString(instructions, instructionFile) {
		instructions = append(instructions, instructionFile)
	}
	cfg["instructions"] = instructions

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(configPath, data, 0o644)
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func fileNames(agent string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "claude":
		return "vela-mcp.json", "vela-instructions.md"
	case "opencode":
		return "opencode.json", "instructions.md"
	default:
		return "vela-mcp.json", "vela-instructions.md"
	}
}

func displayName(agent string) string {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "claude":
		return "Claude Code"
	case "opencode":
		return "OpenCode"
	default:
		return "agent"
	}
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
