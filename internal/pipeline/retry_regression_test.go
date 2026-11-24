package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/pkg/types"
)

func TestBuildRetriesInterruptedFirstPublication(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	config := Config{Scanner: &harnessScanner{}, Source: func(string) *types.Source { return &types.Source{Name: "fixture"} }}
	config.Persist = func(*types.Graph, string) error { return errors.New("injected persist failure") }
	req := types.BuildRequest{RepoRoot: root}
	if _, err := NewBuilder(config).Build(context.Background(), req); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := export.ResolveGeneration(out); err == nil {
		t.Fatal("reader accepted incomplete publication")
	}
	config.Persist = nil
	result, err := NewBuilder(config).Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := export.ResolveGeneration(out)
	if err != nil || selected.ID != result.Snapshot.ID {
		t.Fatalf("selection: %v", err)
	}
	if err := os.Remove(filepath.Join(out, ".current")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewBuilder(config).Build(context.Background(), req); err == nil {
		t.Fatal("missing pointer with sealed history accepted")
	}
	if err := os.Symlink(".generations/g-missing", filepath.Join(out, ".current")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewBuilder(config).Build(context.Background(), req); err == nil {
		t.Fatal("corrupt pointer accepted")
	}
	target, err := os.Readlink(filepath.Join(out, ".current"))
	if err != nil || target != ".generations/g-missing" {
		t.Fatal("pointer changed")
	}
}
