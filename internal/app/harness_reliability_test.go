package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/generation"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/pipeline"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/registry"
	"github.com/Syfra3/vela/pkg/types"
)

func harnessChildren(t *testing.T) (string, []string) {
	t.Helper()
	root := t.TempDir()
	roots := []string{filepath.Join(root, "x", "repo"), filepath.Join(root, "y", "repo")}
	for _, r := range roots {
		if err := os.MkdirAll(filepath.Join(r, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(r, "a.go"), []byte("package fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, roots
}
func harnessChild(t *testing.T, root, label string) pipeline.Result {
	t.Helper()
	m, _, err := export.Inventory(root, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	g := &types.Graph{Nodes: []types.Node{{ID: "same-id", Label: label, NodeType: "file", SourceFile: "a.go"}}}
	w, err := export.AcquireWriter(context.Background(), filepath.Join(root, ".vela"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	s, err := w.Publish(g, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pipeline.Result{Graph: s.Graph, Snapshot: s, GraphPath: filepath.Join(root, ".vela", "graph.json")}
}
func harnessService() BuildService {
	return BuildService{WriteHTML: func(*types.Graph, string) error { return nil }, WriteReport: func(*types.Graph, string) error { return nil }}
}

// Call runMultiRepo directly to avoid Git detection subprocesses. Fake child
// results are real temporary validated generations, not live indexing outputs.
func TestHarnessReliabilityAggregatePinnedChildVector(t *testing.T) {
	root, roots := harnessChildren(t)
	a := harnessChild(t, roots[0], "A")
	b := harnessChild(t, roots[1], "B")
	service := harnessService()
	result, err := service.runMultiRepo(context.Background(), BuildRequest{RepoRoot: root}, roots, func(_ context.Context, _ string, req types.BuildRequest, _ pipeline.Observer) (pipeline.Result, error) {
		if req.RepoRoot == roots[0] {
			return a, nil
		}
		harnessChild(t, roots[0], "new-A")
		return b, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Graph.Nodes) != 2 || result.Graph.Nodes[0].ID == result.Graph.Nodes[1].ID {
		t.Fatal("same-basename child identities conflated")
	}
	m := result.Snapshot.Manifest
	if m.Aggregate.Children[0].Generation != a.Snapshot.ID {
		t.Fatal("mutable child selection reread")
	}
	if stale, err := export.CheckInventory(m); err != nil || len(stale) > 0 {
		t.Fatalf("unchanged child sources: %v %v", stale, err)
	}
	// Provenance is self-contained: validation does not consult child outputs.
	if err := os.RemoveAll(filepath.Join(roots[0], ".vela")); err != nil {
		t.Fatal(err)
	}
	if _, err := export.ValidateGeneration(result.Snapshot.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roots[0], "a.go"), []byte("package changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if stale, err := export.CheckInventory(m); err != nil || len(stale) == 0 {
		t.Fatalf("changed child sources: %v %v", stale, err)
	}
	if err := os.Symlink("missing", filepath.Join(roots[1], "broken.go")); err != nil {
		t.Fatal(err)
	}
	if stale, err := export.CheckInventory(m); err == nil || len(stale) == 0 {
		t.Fatalf("established stale survives incomplete child: %v %v", stale, err)
	}
}

func TestHarnessReliabilityAggregateFaultAndMissingChild(t *testing.T) {
	for _, boundary := range append([]string{"missing_child", "wrong_child"}, export.PublicationBoundaries...) {
		t.Run(boundary, func(t *testing.T) {
			root, roots := harnessChildren(t)
			a := harnessChild(t, roots[0], "A")
			b := harnessChild(t, roots[1], "B")
			service := harnessService()
			run := func(_ context.Context, _ string, req types.BuildRequest, _ pipeline.Observer) (pipeline.Result, error) {
				if req.RepoRoot == roots[0] {
					return a, nil
				}
				return b, nil
			}
			old, err := service.runMultiRepo(context.Background(), BuildRequest{RepoRoot: root}, roots, run)
			if err != nil {
				t.Fatal(err)
			}
			if boundary == "missing_child" {
				b.Snapshot = nil
			} else if boundary == "wrong_child" {
				b.Graph = &types.Graph{}
			} else {
				service.PublicationFault = func(at string) error {
					if at == boundary {
						return errors.New("injected")
					}
					return nil
				}
			}
			if _, err := service.runMultiRepo(context.Background(), BuildRequest{RepoRoot: root}, roots, run); err == nil {
				t.Fatal("invalid publication succeeded")
			}
			if _, err := export.ValidateGeneration(old.Snapshot.Dir); err != nil {
				t.Fatal(err)
			}
			selected, err := export.ResolveGeneration(filepath.Join(root, ".vela"))
			if err != nil {
				t.Fatal(err)
			}
			if boundary != "selected_directory_sync" && selected.ID != old.Snapshot.ID {
				t.Fatal("failed publication replaced prior selection")
			}
		})
	}
}

func TestHarnessReliabilityAggregateChildSetAndIncomplete(t *testing.T) {
	root, roots := harnessChildren(t)
	a := harnessChild(t, roots[0], "A")
	b := harnessChild(t, roots[1], "B")
	service := harnessService()
	result, err := service.runMultiRepo(context.Background(), BuildRequest{RepoRoot: root}, roots, func(_ context.Context, _ string, r types.BuildRequest, _ pipeline.Observer) (pipeline.Result, error) {
		if r.RepoRoot == roots[0] {
			return a, nil
		}
		return b, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "new", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if stale, err := export.CheckInventory(result.Snapshot.Manifest); err != nil || len(stale) == 0 {
		t.Fatalf("child add: %v %v", stale, err)
	}
	if err := os.RemoveAll(filepath.Join(root, "new")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(roots[0], "broken.go")); err != nil {
		t.Fatal(err)
	}
	if stale, err := export.CheckInventory(result.Snapshot.Manifest); err == nil || len(stale) != 0 {
		t.Fatalf("incomplete child: %v %v", stale, err)
	}
}

func TestHarnessReliabilityAggregateActualAncillaryConsumers(t *testing.T) {
	root, roots := harnessChildren(t)
	vault := t.TempDir()
	children := map[string]pipeline.Result{}
	for _, r := range roots {
		src := &types.Source{Type: types.SourceTypeCodebase, ID: r, Name: "repo", Path: r}
		g := &types.Graph{Nodes: []types.Node{{ID: "a", Label: "AFile", NodeType: "file", SourceFile: "a.go", Source: src}, {ID: "b", Label: "BFile", NodeType: "file", SourceFile: "b.go", Source: src}}, Edges: []types.Edge{{Source: "a", Target: "BFile", Relation: "calls", SourceFile: "a.go"}}}
		m, _, err := export.Inventory(r, types.ManifestRequest{})
		if err != nil {
			t.Fatal(err)
		}
		w, err := export.AcquireWriter(context.Background(), filepath.Join(r, ".vela"))
		if err != nil {
			t.Fatal(err)
		}
		s, err := w.Publish(g, m, nil)
		w.Close()
		if err != nil {
			t.Fatal(err)
		}
		children[r] = pipeline.Result{Graph: s.Graph, Snapshot: s, GraphPath: filepath.Join(r, ".vela", "graph.json")}
	}
	svc := BuildService{WriteHTML: func(*types.Graph, string) error { return nil }, WriteObsidian: func(g *types.Graph, dir string) error {
		built, err := igraph.Build(g.Nodes, g.Edges)
		if err != nil {
			return err
		}
		if len(built.ResolvedEdges) != 2 {
			t.Errorf("ancillary consumer lost child edges: %d", len(built.ResolvedEdges))
		}
		return export.WriteObsidian(g, dir)
	}}
	r, err := svc.runMultiRepo(context.Background(), BuildRequest{RepoRoot: root, Obsidian: types.ObsidianConfig{AutoSync: true, VaultDir: vault}}, roots, func(_ context.Context, _ string, req types.BuildRequest, _ pipeline.Observer) (pipeline.Result, error) {
		return children[req.RepoRoot], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) > 0 {
		t.Fatalf("ancillary warnings: %v", r.Warnings)
	}
	if _, err := os.Stat(r.ReportPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(vault, "obsidian")); err != nil {
		t.Fatal(err)
	}
	metrics, err := igraph.LoadHealthMetrics(r.GraphPath, 5)
	if err != nil || metrics.BrokenEdges != 0 || metrics.Edges != 2 {
		t.Fatalf("health lost namespaced label semantics: %+v %v", metrics, err)
	}
	e, err := query.LoadFromFile(r.GraphPath)
	if err != nil {
		t.Fatal(err)
	}
	if e.Freshness().Status != query.FreshnessUnknown {
		t.Fatal("aggregate upgraded unbound children")
	}
}

func TestHarnessReliabilityNestedReturnedGraphMutationRejected(t *testing.T) {
	root, roots := harnessChildren(t)
	a := harnessChild(t, roots[0], "A")
	b := harnessChild(t, roots[1], "B")
	// Add nested data before sealing, then mutate the same returned Graph object.
	g := a.Graph
	g.Nodes[0].Metadata = map[string]interface{}{"nested": map[string]interface{}{"value": "original"}}
	m, _, err := export.Inventory(roots[0], types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	w, err := export.AcquireWriter(context.Background(), filepath.Join(roots[0], ".vela"))
	if err != nil {
		t.Fatal(err)
	}
	a.Snapshot, err = w.Publish(g, m, nil)
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	a.Graph = a.Snapshot.Graph
	a.Graph.Nodes[0].Metadata["nested"].(map[string]interface{})["value"] = "mutated"
	svc := harnessService()
	if _, err := svc.runMultiRepo(context.Background(), BuildRequest{RepoRoot: root}, roots, func(_ context.Context, _ string, req types.BuildRequest, _ pipeline.Observer) (pipeline.Result, error) {
		if req.RepoRoot == roots[0] {
			return a, nil
		}
		return b, nil
	}); err == nil {
		t.Fatal("nested graph mutation accepted as matching snapshot")
	}
}

func TestHarnessReliabilityDefaultAppPlanAndRealReport(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc A(){}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	original := defaultPipelineConfig
	t.Cleanup(func() { defaultPipelineConfig = original })
	defaultPipelineConfig = func() (pipeline.Config, error) {
		cfg, err := original()
		if err != nil {
			return cfg, err
		}
		cfg.Source = func(root string) *types.Source {
			return &types.Source{ID: "fixture", Name: "fixture", Path: root, Type: types.SourceTypeCodebase}
		}
		cfg.Cluster = nil
		return cfg, nil
	}
	for i := 0; i < 2; i++ {
		r, err := defaultRunPipeline(context.Background(), filepath.Join(root, ".vela"), types.BuildRequest{RepoRoot: root}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if r.Snapshot.Manifest.SourceBinding != generation.BoundGoProfile {
			t.Fatal("normal nonnil registry blocked bound app profile")
		}
		if i == 1 && r.CacheHits != 1 {
			t.Fatal("app production route did not reuse owned extraction")
		}
		if err := defaultWriteReport(r.Graph, filepath.Dir(r.GraphPath)); err != nil {
			t.Fatal(err)
		}
		status := igraph.LoadRegistryStatusSnapshot([]registry.Entry{{RepoRoot: root, GraphPath: r.GraphPath}}, 5)
		if status.Summary.MissingReport != 0 || !status.Repos[0].Snapshot.Freshness.ReportPresent {
			t.Fatalf("real report lost after pin: %+v", status.Summary)
		}
	}
}
