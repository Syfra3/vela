package setup

import (
	"os"
	"testing"
)

func TestSaveAndLoadIntegrationState(t *testing.T) {
	home := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", originalHome)

	if err := SaveIntegrationState(IntegrationState{Mode: IntegrationModeAncoraVela, MCPTarget: IntegrationTargetClaudeCode}); err != nil {
		t.Fatalf("SaveIntegrationState() error = %v", err)
	}

	state, err := LoadIntegrationState()
	if err != nil {
		t.Fatalf("LoadIntegrationState() error = %v", err)
	}
	if state == nil {
		t.Fatal("expected integration state")
	}
	if state.Mode != IntegrationModeAncoraVela {
		t.Fatalf("Mode = %q, want %q", state.Mode, IntegrationModeAncoraVela)
	}
	if state.MCPTarget != IntegrationTargetClaudeCode {
		t.Fatalf("MCPTarget = %q, want %q", state.MCPTarget, IntegrationTargetClaudeCode)
	}
}
