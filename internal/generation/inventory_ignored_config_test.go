package generation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/pkg/types"
)

func TestInventoryIgnoreControlsValidatedAndFingerprinted(t *testing.T) {
	for _, name := range []string{".gitignore", ".velignore"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			control := filepath.Join(root, name)
			if err := os.WriteFile(control, []byte(name+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			m, _, err := generation.Inventory(root, types.ManifestRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(control, []byte(name+"\n# changed\n"), 0600); err != nil {
				t.Fatal(err)
			}
			stale, err := generation.CheckInventory(m)
			if err != nil || len(stale) != 1 || stale[0] != "configuration" {
				t.Fatalf("self-ignored control change: stale=%v err=%v", stale, err)
			}
			if err := os.Remove(control); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, "target")
			if err := os.WriteFile(target, []byte(name+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, control); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			if _, _, err := generation.Inventory(root, types.ManifestRequest{}); err == nil || !strings.Contains(err.Error(), "inventory requires regular file") {
				t.Fatalf("symlink control accepted: %v", err)
			}
		})
	}
}

func TestInventoryIgnoredConfigSymlink(t *testing.T) {
	for _, tc := range []struct {
		name, ignore string
		dir          string
		reject       bool
	}{
		{"default node_modules", "", "node_modules", false},
		{"explicit ignored directory", "ignored/\n", "ignored", false},
		{"included config", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"fixture"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.ignore != "" {
				if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(tc.ignore), 0600); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(root, tc.dir)
			if tc.dir != "" {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if tc.dir != "" {
				if err := os.Symlink(filepath.Join(root, "package.json"), filepath.Join(dir, ".gitignore")); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			if err := os.Symlink(filepath.Join(root, "package.json"), filepath.Join(dir, "tsconfig-linked.json")); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			m, _, err := generation.Inventory(root, types.ManifestRequest{})
			if tc.reject {
				if err == nil || !strings.Contains(err.Error(), "inventory requires regular file") {
					t.Fatalf("included symlink: manifest=%v err=%v", m, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !m.InventoryComplete {
				t.Fatal("inventory incomplete")
			}
			if stale, err := generation.CheckInventory(m); err != nil || len(stale) != 0 {
				t.Fatalf("ignored config affected freshness: %v %v", stale, err)
			}
		})
	}
}
