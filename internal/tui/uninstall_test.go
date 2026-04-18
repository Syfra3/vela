package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Syfra3/vela/internal/daemon"
)

func TestUninstallAllRemovesManagedData(t *testing.T) {
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})

	home := t.TempDir()
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}

	velaDir := filepath.Join(home, ".vela")
	// Default vault dir — the whole directory should be removed, not just the obsidian subdir.
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

	restore := stubUninstallDeps()
	defer restore()

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
	if len(result.Removed) != 2 {
		t.Fatalf("Removed = %v, want 2 entries", result.Removed)
	}
}

func TestUninstallAllOnlyRemovesObsidianSubdirForCustomVault(t *testing.T) {
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})

	home := t.TempDir()
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}

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

	restore := stubUninstallDeps()
	defer restore()

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

func stubUninstallDeps() func() {
	originalStop := uninstallStopDaemon
	originalSvc := uninstallRemoveSvc
	originalMCP := uninstallRemoveMCP
	originalTargets := uninstallTargetsFunc

	uninstallStopDaemon = func() error { return daemon.ErrNotRunning }
	uninstallRemoveSvc = func() error { return nil }
	uninstallRemoveMCP = func() error { return nil }
	uninstallTargetsFunc = uninstallTargets

	return func() {
		uninstallStopDaemon = originalStop
		uninstallRemoveSvc = originalSvc
		uninstallRemoveMCP = originalMCP
		uninstallTargetsFunc = originalTargets
	}
}

func TestUninstallModelEnterStartsRemoval(t *testing.T) {
	originalTargets := uninstallTargetsFunc
	t.Cleanup(func() {
		uninstallTargetsFunc = originalTargets
	})
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

func TestUninstallAllReturnsWarningsForIntegrationFailures(t *testing.T) {
	originalTargets := uninstallTargetsFunc
	originalStop := uninstallStopDaemon
	originalSvc := uninstallRemoveSvc
	originalMCP := uninstallRemoveMCP
	t.Cleanup(func() {
		uninstallTargetsFunc = originalTargets
		uninstallStopDaemon = originalStop
		uninstallRemoveSvc = originalSvc
		uninstallRemoveMCP = originalMCP
	})

	uninstallTargetsFunc = func() ([]string, error) { return []string{}, nil }
	uninstallStopDaemon = func() error { return errors.New("stop failed") }
	uninstallRemoveSvc = func() error { return errors.New("service failed") }
	uninstallRemoveMCP = func() error { return errors.New("mcp failed") }

	result, err := uninstallAll()
	if err != nil {
		t.Fatalf("uninstallAll() error = %v", err)
	}
	if len(result.Warnings) != 3 {
		t.Fatalf("Warnings = %v, want 3", result.Warnings)
	}
}
