package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHarnessReliabilityDanglingLocalNeverFallsBack(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{filepath.Join(root, ".vela"), filepath.Join(home, ".vela")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".vela", "graph.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".generations/g-missing", filepath.Join(root, ".vela", ".current")); err != nil {
		t.Fatal(err)
	}
	if path, err := FindGraphFile(root); err == nil {
		t.Fatalf("fell back to %s", path)
	}
}
