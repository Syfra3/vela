package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/internal/pipeline"
	"github.com/Syfra3/vela/internal/registry"
	"github.com/Syfra3/vela/pkg/types"
)

// Scenario: Publication never mixes generations. Call only the snapshot helper
// and temp publishers, never commands, registry/config, SCIP, or installed tools.
func TestHarnessReliabilityRollbackDoesNotOverwritePublication(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		t.Run(map[bool]string{true: "legacy_snapshot", false: "generation_snapshot"}[legacy], func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, ".vela")
			publish := func(id string) *export.Snapshot {
				t.Helper()
				m, _, err := export.Inventory(root, types.ManifestRequest{})
				if err != nil {
					t.Fatal(err)
				}
				w, err := export.AcquireWriter(context.Background(), out)
				if err != nil {
					t.Fatal(err)
				}
				defer w.Close()
				s, err := w.Publish(&types.Graph{Nodes: []types.Node{{ID: id, Label: id, NodeType: "file"}}}, m, nil)
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
			if legacy {
				if err := export.WriteJSONAtomic(&types.Graph{}, out); err != nil {
					t.Fatal(err)
				}
			} else {
				publish("A")
			}
			restore, err := snapshotGeneratedState(root, out)
			if err != nil {
				t.Fatal(err)
			}
			b := publish("B")
			if err := restore(); err != nil {
				t.Fatal(err)
			}
			selected, err := export.ResolveGeneration(out)
			if err != nil {
				t.Fatal(err)
			}
			if selected.ID != b.ID {
				t.Fatal("rollback overwrote concurrent publication")
			}
			if _, err := os.Readlink(filepath.Join(out, "graph.db")); err != nil {
				t.Fatal("rollback replaced compatibility link")
			}
		})
	}
}

func TestHarnessReliabilityAdoptedCLISelectionAndRefusal(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	if err := os.MkdirAll(filepath.Join(out, ".generations"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".generations/g-missing", filepath.Join(out, ".current")); err != nil {
		t.Fatal(err)
	}
	if path, ok := activeWorkspaceGraphFile(root); !ok || path != filepath.Join(out, "graph.json") {
		t.Fatal("dangling active corpus skipped")
	}
	if _, err := initializeProjectGraph(root); err == nil {
		t.Fatal("initializer repaired adopted corpus")
	}
	if err := purgeRegistryEntryIndex(registry.Entry{RepoRoot: root}); err == nil {
		t.Fatal("purge accepted adopted target")
	}
	if _, err := os.Lstat(filepath.Join(out, ".current")); err != nil {
		t.Fatal("refusal changed selection")
	}
}

func TestHarnessReliabilityDefaultCLIEmptyDriverPlan(t *testing.T) {
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
		r, err := runPipelineBuild(context.Background(), filepath.Join(root, ".vela"), types.BuildRequest{RepoRoot: root}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if r.Snapshot.Manifest.SourceBinding != generation.BoundGoProfile || i == 1 && r.CacheHits != 1 {
			t.Fatal("CLI production builder did not use empty participating plan")
		}
	}
}
