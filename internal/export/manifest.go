package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Syfra3/vela/pkg/types"
)

// WriteManifestAtomic writes <outDir>/manifest.json atomically.
func WriteManifestAtomic(m *types.Manifest, outDir string) error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", outDir, err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling manifest: %w", err)
	}
	outPath := filepath.Join(outDir, "manifest.json")
	tmp := outPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// LoadManifest reads a manifest.json file written by WriteManifestAtomic.
func LoadManifest(path string) (*types.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m types.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshalling manifest: %w", err)
	}
	return &m, nil
}
