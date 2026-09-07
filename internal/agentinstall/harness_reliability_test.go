package agentinstall

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHarnessReliabilityInitializerRefusesBeforeIntegrationWrites(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	if err := os.MkdirAll(filepath.Join(out, ".generations"), 0755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "integration")
	if _, err := Install(Request{ProjectDir: root, Agent: "claude", ConfigDir: config}); err == nil {
		t.Fatal("adopted initializer accepted")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("integration changed before refusal")
	}
	if _, err := os.Stat(filepath.Join(out, "graph.db")); !os.IsNotExist(err) {
		t.Fatal("placeholder repaired adoption")
	}
}
