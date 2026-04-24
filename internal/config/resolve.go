package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const graphFileName = "graph.json"
const legacyOutSubDir = "vela-out"

// velaHome returns ~/.vela — the single global store for all vela output,
// mirroring how ancora uses ~/.ancora/.
func velaHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".vela"), nil
}

// OutDir returns the output directory for a given project root.
// Output is stored directly at ~/.vela/ so graph data, cache, and runtime
// state all live under one canonical root.
func OutDir(root string) string {
	return velaHomeOrFallback()
}

// GraphFilePath returns the canonical graph.json path inside an out directory.
func GraphFilePath(outDir string) string {
	return filepath.Join(outDir, graphFileName)
}

// RegistryFilePath returns the global tracked-repository registry path.
func RegistryFilePath() string {
	return filepath.Join(velaHomeOrFallback(), "registry.json")
}

// FindGraphFile resolves graph.json for the current working directory.
// Search order:
//  1. ~/.vela/graph.json          — canonical global store
//  2. ~/.vela/vela-out/graph.json — legacy global store
//  3. ./.vela/graph.json          — local override
//  4. ./vela-out/graph.json       — legacy repo-local store
func FindGraphFile(startDir string) (string, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting working directory: %w", err)
		}
	}

	abs, err := filepath.Abs(startDir)
	if err != nil {
		abs = startDir
	}

	home := velaHomeOrFallback()

	// 1. ~/.vela/graph.json
	canonical := filepath.Join(home, graphFileName)
	if _, err := os.Stat(canonical); err == nil {
		return canonical, nil
	}

	// 2. ~/.vela/vela-out/graph.json
	legacyGlobal := filepath.Join(home, legacyOutSubDir, graphFileName)
	if _, err := os.Stat(legacyGlobal); err == nil {
		return legacyGlobal, nil
	}

	// 3. ./.vela/graph.json
	local := filepath.Join(abs, ".vela", graphFileName)
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}

	// 4. legacy ./vela-out/graph.json
	legacy := filepath.Join(abs, legacyOutSubDir, graphFileName)
	if _, err := os.Stat(legacy); err == nil {
		return legacy, nil
	}

	return "", fmt.Errorf("graph.json not found — run: vela extract <path>")
}

func velaHomeOrFallback() string {
	h, err := velaHome()
	if err != nil {
		return ".vela"
	}
	return h
}

// ResolveVaultDir maps an Obsidian vault_dir config value to the canonical
// destination. Empty and legacy repo-local values are migrated to
// ~/Documents/vela so old configs stop recreating ./vela-out/obsidian.
func ResolveVaultDir(raw string) string {
	if raw == "" {
		return DefaultVaultDir()
	}

	if filepath.Clean(raw) == legacyOutSubDir {
		return DefaultVaultDir()
	}

	return raw
}

// DefaultVaultDir returns ~/Documents/vela — a visible location Obsidian can open directly.
func DefaultVaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "vela-vault"
	}
	return filepath.Join(home, "Documents", "vela")
}
