package setup

import (
	"testing"
)

func TestCheckOllama(t *testing.T) {
	// Basic smoke test — actual result depends on system state
	installed, running, path, err := CheckOllama()

	if err != nil {
		t.Logf("CheckOllama error: %v", err)
	}

	t.Logf("Ollama installed: %v, running: %v, path: %s", installed, running, path)

	if installed && path == "" {
		t.Error("Ollama reported as installed but path is empty")
	}
}
