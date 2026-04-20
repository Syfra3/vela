package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutDirUsesVelaHomeRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	want := filepath.Join(tmp, ".vela")
	if got := OutDir("."); got != want {
		t.Fatalf("OutDir() = %q, want %q", got, want)
	}
}

func TestResolveVaultDirMigratesLegacyRepoLocalValue(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	want := filepath.Join(tmp, "Documents", "vela")
	for _, raw := range []string{"", "vela-out", "./vela-out", "vela-out/"} {
		if got := ResolveVaultDir(raw); got != want {
			t.Fatalf("ResolveVaultDir(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestLoadMigratesLegacyObsidianVaultDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfgDir := filepath.Join(tmp, ".vela")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	configYAML := []byte("obsidian:\n  auto_sync: true\n  vault_dir: vela-out\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), configYAML, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := filepath.Join(tmp, "Documents", "vela")
	if cfg.Obsidian.VaultDir != want {
		t.Fatalf("cfg.Obsidian.VaultDir = %q, want %q", cfg.Obsidian.VaultDir, want)
	}
}

func TestFindGraphFileReadsLegacyGlobalPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	legacyGraph := filepath.Join(tmp, ".vela", "vela-out", graphFileName)
	if err := os.MkdirAll(filepath.Dir(legacyGraph), 0o755); err != nil {
		t.Fatalf("mkdir legacy graph dir: %v", err)
	}
	if err := os.WriteFile(legacyGraph, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write legacy graph: %v", err)
	}

	got, err := FindGraphFile(tmp)
	if err != nil {
		t.Fatalf("FindGraphFile() error = %v", err)
	}
	if got != legacyGraph {
		t.Fatalf("FindGraphFile() = %q, want %q", got, legacyGraph)
	}
}

func TestLoadReadsEmbeddingConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfgDir := filepath.Join(tmp, ".vela")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	configYAML := []byte("embedding:\n  provider: ollama\n  model: all-minilm\n  endpoint: http://localhost:4242\n  vector_backend: sqlite-vec\n  sqlite_vec_path: /tmp/sqlite-vec.so\n")
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), configYAML, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Embedding.Provider != "ollama" || cfg.Embedding.Model != "all-minilm" || cfg.Embedding.Endpoint != "http://localhost:4242" || cfg.Embedding.VectorBackend != "sqlite-vec" || cfg.Embedding.SQLiteVecPath != "/tmp/sqlite-vec.so" {
		t.Fatalf("unexpected embedding config: %+v", cfg.Embedding)
	}
}
