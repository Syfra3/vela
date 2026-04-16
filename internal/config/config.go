package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Syfra3/vela/pkg/types"
)

const configFile = "config.yaml"

// defaults returns a Config populated with sensible defaults.
func defaults() *types.Config {
	home, _ := os.UserHomeDir()
	return &types.Config{
		LLM: types.LLMConfig{
			Provider:       "local",
			Model:          "llama3",
			Endpoint:       "http://localhost:11434",
			Timeout:        60 * time.Second,
			MaxChunkTokens: 8000,
		},
		Extraction: types.ExtractionConfig{
			CodeLanguages: []string{"go", "python", "typescript", "rust", "java"},
			IncludeDocs:   true,
			IncludeImages: false,
			ChunkSize:     8000,
			CacheDir:      filepath.Join(home, ".vela", "cache"),
		},
		UI: types.UIConfig{
			Theme:        "dark",
			ShowProgress: true,
			EnableColors: true,
		},
		Watch: types.WatchConfig{
			Enabled: false,
			Sources: []types.WatchSourceConfig{
				{
					Name:   "ancora",
					Type:   "syfra",
					Socket: filepath.Join(home, ".syfra", "ancora.sock"),
				},
			},
			Reconciler: types.ReconcilerConfig{
				DebounceMs:   100,
				MaxBatchSize: 50,
			},
			Extractor: types.ExtractorConfig{
				Enabled:   true,
				Workers:   2,
				WriteBack: true,
				Provider:  "local",
				Model:     "llama3",
			},
		},
		Daemon: types.DaemonConfig{
			PIDFile:    filepath.Join(home, ".vela", "watch.pid"),
			LogFile:    filepath.Join(home, ".vela", "watch.log"),
			LogLevel:   "info",
			StatusFile: filepath.Join(home, ".vela", "watch-status.json"),
		},
		Obsidian: types.ObsidianConfig{
			AutoSync: false,
			VaultDir: "vela-out",
		},
	}
}

// configDir returns the path to ~/.vela/
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".vela"), nil
}

// Load reads ~/.vela/config.yaml and returns a Config. If the file does not
// exist, the defaults are returned without error.
func Load() (*types.Config, error) {
	dir, err := configDir()
	if err != nil {
		return defaults(), nil
	}

	path := filepath.Join(dir, configFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaults(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	cfg := defaults()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return cfg, nil
}

// WriteDefault writes the default config to ~/.vela/config.yaml.
// Returns an error if the file already exists (use force=true to overwrite).
func WriteDefault(force bool) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}

	path := filepath.Join(dir, configFile)
	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, fmt.Errorf("config already exists at %s (use --force to overwrite)", path)
		}
	}

	cfg := defaults()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}
