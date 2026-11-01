package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/internal/scip"
	"github.com/Syfra3/vela/pkg/types"
)

type harnessScanner struct{ calls int }

func (s *harnessScanner) Scan(root string, paths []string, src *types.Source) ([]types.Node, []types.Edge, error) {
	s.calls++
	var nodes []types.Node
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, types.Node{ID: rel, Label: rel, NodeType: "file", SourceFile: rel})
	}
	return nodes, nil, nil
}

// These builds use only a fake scanner/source and no registry, driver, cluster,
// subprocess, user config, or repository data. All input/output is t.TempDir.
func TestHarnessReliabilityUnboundDeletionAlwaysPublishes(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scanner := &harnessScanner{}
	b := NewBuilder(Config{Scanner: scanner, Source: func(string) *types.Source { return &types.Source{Name: "fixture"} }})
	a, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	built, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if scanner.calls != 2 || a.Snapshot.ID == built.Snapshot.ID {
		t.Fatalf("unchanged reuse: scans=%d", scanner.calls)
	}
	if err := os.Remove(filepath.Join(root, "b.go")); err != nil {
		t.Fatal(err)
	}
	pruned, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if scanner.calls != 3 || pruned.Snapshot.ID == a.Snapshot.ID || len(pruned.Graph.Nodes) != 1 {
		t.Fatalf("deletion reuse failed: scans=%d nodes=%d", scanner.calls, len(pruned.Graph.Nodes))
	}
	if pruned.Snapshot.Manifest.BuildMode != buildModeFullRebuild || pruned.Snapshot.Manifest.SourceBinding != "unbound" {
		t.Fatal("unbound deletion shortcut upgraded provenance")
	}
	if _, err := export.ValidateGeneration(a.Snapshot.Dir); err != nil {
		t.Fatal(err)
	}
	selected, err := export.ResolveGeneration(out)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != pruned.Snapshot.ID {
		t.Fatal("selection mismatch")
	}
	for i, change := range []string{"add", "edit", "rename", "config"} {
		switch change {
		case "add":
			err = os.WriteFile(filepath.Join(root, "c.go"), []byte("package c"), 0600)
		case "edit":
			err = os.WriteFile(filepath.Join(root, "a.go"), []byte("package edit"), 0600)
		case "rename":
			err = os.Rename(filepath.Join(root, "c.go"), filepath.Join(root, "d.go"))
		case "config":
			err = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture"), 0600)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err = b.Build(context.Background(), types.BuildRequest{RepoRoot: root}); err != nil {
			t.Fatal(err)
		}
		if scanner.calls != i+4 {
			t.Fatalf("%s did not conservatively rebuild", change)
		}
	}
}

func TestHarnessReliabilityBuildRejectsIncompleteDiscovery(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, detect := range []func(string) ([]string, error){func(string) ([]string, error) { return nil, nil }, func(string) ([]string, error) { return nil, fmt.Errorf("unreadable") }} {
		b := NewBuilder(Config{Detect: detect, Scanner: &harnessScanner{}, Source: func(string) *types.Source { return nil }})
		if _, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root}); err == nil {
			t.Fatal("incomplete detection accepted")
		}
	}
}

func TestHarnessReliabilityOpaqueScannerNeverReused(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a"), 0600); err != nil {
		t.Fatal(err)
	}
	scanner := &harnessScanner{}
	b := NewBuilder(Config{Scanner: scanner, Source: func(string) *types.Source { return &types.Source{Name: "fixture"} }})
	for i := 0; i < 2; i++ {
		result, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		if result.Snapshot.Manifest.BindingReason == "" {
			t.Error("opaque extraction has no unbound reason")
		}
	}
	if scanner.calls != 2 {
		t.Fatalf("unbound scanner reused: %d calls", scanner.calls)
	}
}

func TestHarnessReliabilityActualDriverPlanAndPersistCallback(t *testing.T) {
	root := t.TempDir()
	harnessPut(t, root, "a.go", "package a\nfunc A(){}\n")
	reg, err := scip.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(Config{Registry: reg, Source: harnessSource})
	r, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if r.Snapshot.Manifest.SourceBinding != generation.BoundGoProfile {
		t.Error("empty participating-driver plan prevented bound extraction")
	}
	b = NewBuilder(Config{Registry: reg, Source: harnessSource, Persist: export.WriteJSONAtomic})
	if _, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root}); err != nil {
		t.Fatalf("existing Persist callback rejected: %v", err)
	}
	selected, err := export.ResolveGeneration(filepath.Join(root, ".vela"))
	if err != nil {
		t.Fatal(err)
	}
	b = NewBuilder(Config{Registry: reg, Source: harnessSource, Persist: func(*types.Graph, string) error { return fmt.Errorf("callback failed") }})
	if _, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root}); err == nil {
		t.Fatal("callback failure accepted")
	}
	still, err := export.ResolveGeneration(filepath.Join(root, ".vela"))
	if err != nil || still.ID != selected.ID {
		t.Fatal("callback failure replaced prior selection")
	}
	b = NewBuilder(Config{Registry: reg, Source: harnessSource, Persist: func(g *types.Graph, out string) error {
		g.Nodes[0].Label = "not the extracted graph"
		return export.WriteJSONAtomic(g, out)
	}})
	if _, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root}); err == nil {
		t.Fatal("callback mutation bypassed publisher agreement validation")
	}
	still, err = export.ResolveGeneration(filepath.Join(root, ".vela"))
	if err != nil || still.ID != selected.ID {
		t.Fatal("invalid callback bytes replaced selection")
	}
}
