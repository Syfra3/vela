package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUninstallAllRemovesManagedData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	velaDir := filepath.Join(home, ".vela")
	defaultVaultDir := filepath.Join(home, "Documents", "vela")
	obsidianDir := filepath.Join(defaultVaultDir, "obsidian")
	for _, dir := range []string{velaDir, obsidianDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(velaDir, "graph.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write graph: %v", err)
	}
	if err := os.WriteFile(filepath.Join(obsidianDir, "note.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	result, err := uninstallAll()
	if err != nil {
		t.Fatalf("uninstallAll() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", result.Warnings)
	}
	for _, path := range []string{velaDir, defaultVaultDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err = %v", path, err)
		}
	}
}

func TestUninstallAllOnlyRemovesObsidianSubdirForCustomVault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	velaDir := filepath.Join(home, ".vela")
	customVault := filepath.Join(home, "vault")
	obsidianDir := filepath.Join(customVault, "obsidian")
	if err := os.MkdirAll(velaDir, 0o755); err != nil {
		t.Fatalf("mkdir vela dir: %v", err)
	}
	if err := os.MkdirAll(obsidianDir, 0o755); err != nil {
		t.Fatalf("mkdir obsidian dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customVault, "keep.md"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}
	configYAML := []byte("obsidian:\n  vault_dir: " + customVault + "\n")
	if err := os.WriteFile(filepath.Join(velaDir, "config.yaml"), configYAML, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := uninstallAll(); err != nil {
		t.Fatalf("uninstallAll() error = %v", err)
	}
	if _, err := os.Stat(obsidianDir); !os.IsNotExist(err) {
		t.Fatalf("expected obsidian dir removed, stat err = %v", err)
	}
	if _, err := os.Stat(customVault); err != nil {
		t.Fatalf("expected custom vault root to remain, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(customVault, "keep.md")); err != nil {
		t.Fatalf("expected non-Vela file to remain, stat err = %v", err)
	}
}

func TestUninstallModelEnterStartsRemoval(t *testing.T) {
	originalTargets := uninstallTargetsFunc
	t.Cleanup(func() { uninstallTargetsFunc = originalTargets })
	uninstallTargetsFunc = func() ([]string, error) { return []string{"/tmp/.vela"}, nil }

	model := NewUninstallModel()
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected uninstall command")
	}
	if updated.(UninstallModel).state != uninstallStateRunning {
		t.Fatalf("state = %v, want running", updated.(UninstallModel).state)
	}
}
