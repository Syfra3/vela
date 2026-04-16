package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHookInstall verifies that a hook script can be written to a .git/hooks dir.
func TestHookInstall_WritesExecutable(t *testing.T) {
	// Simulate a minimal git repo structure
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}

	hookPath := filepath.Join(hooksDir, "post-commit")
	hookScript := "#!/bin/sh\nvela extract . --no-tui --no-viz --provider none 2>/dev/null || true\n"
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Verify file exists and is executable
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("hook file not created: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("hook file is not executable")
	}

	// Verify content
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "#!/bin/sh") {
		t.Error("hook file missing shebang")
	}
	if !strings.Contains(string(data), "vela extract") {
		t.Error("hook file missing vela extract command")
	}
}
