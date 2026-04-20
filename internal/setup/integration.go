package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	IntegrationModeAncoraOnly = "ancora-only"
	IntegrationModeAncoraVela = "ancora+vela"
	IntegrationModeVelaOnly   = "vela-only"

	IntegrationTargetClaudeCode = "claude-code"
	IntegrationTargetOpenCode   = "opencode"
	IntegrationTargetSkip       = "skip"
)

type IntegrationState struct {
	Mode      string `json:"mode"`
	MCPTarget string `json:"mcp_target,omitempty"`
	UpdatedBy string `json:"updated_by,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func integrationStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".syfra", "integration.json"), nil
}

func LoadIntegrationState() (*IntegrationState, error) {
	path, err := integrationStatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading integration state: %w", err)
	}
	var state IntegrationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing integration state: %w", err)
	}
	if !validIntegrationMode(state.Mode) {
		return nil, nil
	}
	return &state, nil
}

func SaveIntegrationState(state IntegrationState) error {
	if !validIntegrationMode(state.Mode) {
		return fmt.Errorf("invalid integration mode: %s", state.Mode)
	}
	if state.UpdatedAt == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if state.UpdatedBy == "" {
		state.UpdatedBy = "vela"
	}
	path, err := integrationStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating integration state dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding integration state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing integration state: %w", err)
	}
	return nil
}

func validIntegrationMode(mode string) bool {
	switch mode {
	case IntegrationModeAncoraOnly, IntegrationModeAncoraVela, IntegrationModeVelaOnly:
		return true
	default:
		return false
	}
}

func integrationModeLabel(mode string) string {
	switch mode {
	case IntegrationModeAncoraOnly:
		return "Ancora only"
	case IntegrationModeAncoraVela:
		return "Ancora + Vela"
	case IntegrationModeVelaOnly:
		return "Vela only"
	default:
		return "Unknown"
	}
}

func targetLabel(target string) string {
	switch target {
	case IntegrationTargetClaudeCode:
		return "Claude Code"
	case IntegrationTargetOpenCode:
		return "OpenCode"
	case IntegrationTargetSkip, "":
		return "Skip MCP setup"
	default:
		return target
	}
}
