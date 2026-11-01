package generation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/pkg/types"
)

func TestInventoryIgnoredConfigSymlink(t *testing.T) {
	for _, tc := range []struct {
		name, ignore string
		reject       bool
	}{
		{"default node_modules", "", false},
		{"explicit ignored directory", "node_modules/\n", false},
		{"included config", "", true},
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
			dir := root
			if !tc.reject {
				dir = filepath.Join(root, "node_modules")
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(filepath.Join(root, "package.json"), filepath.Join(dir, "tsconfig-linked.json")); err != nil {
				t.Fatal(err)
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
