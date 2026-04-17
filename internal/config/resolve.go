package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const graphFileName = "graph.json"

// velaHome returns ~/.vela — the single global store for all vela output,
// mirroring how ancora uses ~/.ancora/.
func velaHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".vela"), nil
}

const outSubDir = "vela-out"

// OutDir returns the output directory for a given project root.
// Output is stored at ~/.vela/vela-out/ so all graphs live in one place.
func OutDir(root string) string {
	return filepath.Join(velaHomeOrFallback(), outSubDir)
}

// GraphFilePath returns the canonical graph.json path inside an out directory.
func GraphFilePath(outDir string) string {
	return filepath.Join(outDir, graphFileName)
}

// FindGraphFile resolves graph.json for the current working directory.
// Search order:
//  1. ~/.vela/<cwd-basename>/graph.json  — preferred (global store by project name)
//  2. ~/.vela/graph.json                 — fallback for single-project setups
//  3. ./.vela/graph.json                 — local override
//  4. ./vela-out/graph.json              — legacy backward-compat
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

	// 1. ~/.vela/vela-out/graph.json
	named := filepath.Join(home, outSubDir, graphFileName)
	if _, err := os.Stat(named); err == nil {
		return named, nil
	}

	// 2. ./.vela/graph.json
	local := filepath.Join(abs, ".vela", graphFileName)
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}

	// 4. legacy ./vela-out/graph.json
	legacy := filepath.Join(abs, "vela-out", graphFileName)
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

// DefaultVaultDir returns ~/Documents/vela — a visible location Obsidian can open directly.
func DefaultVaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "vela-vault"
	}
	return filepath.Join(home, "Documents", "vela")
}
