package export

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/detect"
	"github.com/Syfra3/vela/internal/generation"
	"github.com/Syfra3/vela/pkg/types"
)

func harnessGraph(id string) *types.Graph {
	return &types.Graph{Nodes: []types.Node{{ID: id, Label: id, NodeType: "file"}}}
}

func TestHarnessReliabilityCallbackFullExportSemantics(t *testing.T) {
	for _, change := range []string{"description", "community", "source_path", "edge_source_file", "edge_score", "graph_node_count", "graph_edge_count", "unknown_graph_field"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, ".vela")
			w, err := AcquireWriter(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			g := harnessGraph("original")
			g.Nodes[0].Description = "original description"
			g.Nodes[0].Source = &types.Source{ID: "fixture", Name: "fixture", Path: root, Type: types.SourceTypeCodebase}
			g.Edges = []types.Edge{{Source: "original", Target: "original", Relation: "calls", SourceFile: "original.go", Score: 0.5}}
			old, err := w.Publish(g, harnessManifest(t, root), nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = w.Publish(g, harnessManifest(t, root), func(copy *types.Graph, scratch string) error {
				switch change {
				case "description":
					copy.Nodes[0].Description = "changed"
				case "community":
					copy.Nodes[0].Community = 42
				case "source_path":
					copy.Nodes[0].Source.Path = filepath.Join(root, "not-original")
				case "edge_source_file":
					copy.Edges[0].SourceFile = "not-original.go"
				case "edge_score":
					copy.Edges[0].Score = 0.75
				}
				data, err := marshalGraph(copy)
				if err != nil {
					return err
				}
				var raw map[string]interface{}
				if err := json.Unmarshal(data, &raw); err != nil {
					return err
				}
				switch change {
				case "graph_node_count":
					raw["meta"].(map[string]interface{})["nodeCount"] = 999
				case "graph_edge_count":
					raw["meta"].(map[string]interface{})["edgeCount"] = 999
				case "unknown_graph_field":
					raw["extra_graph_semantics"] = map[string]interface{}{"changed": true}
				}
				data, err = json.Marshal(raw)
				if err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(scratch, "graph.json"), data, 0600)
			})
			if err == nil {
				t.Error("callback changed full exported semantics without rejection")
			}
			selected, err := ResolveGeneration(out)
			if err != nil || selected.ID != old.ID {
				t.Error("invalid callback changed previous selection")
			}
		})
	}
}

func TestHarnessReliabilityCallbackExportNormalization(t *testing.T) {
	for _, mode := range []string{"standard_callback", "equivalent_export_defaults"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			w, err := AcquireWriter(context.Background(), filepath.Join(root, ".vela"))
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			g := harnessGraph("original")
			persist := WriteJSONAtomic
			if mode == "equivalent_export_defaults" {
				persist = func(copy *types.Graph, scratch string) error {
					data, err := marshalGraph(copy)
					if err != nil {
						return err
					}
					var raw map[string]interface{}
					if err := json.Unmarshal(data, &raw); err != nil {
						return err
					}
					raw["meta"].(map[string]interface{})["generatedAt"] = "2000-01-01T00:00:00Z"
					raw["nodes"].([]interface{})[0].(map[string]interface{})["metadata"] = map[string]interface{}{}
					raw["edges"] = nil
					data, err = json.Marshal(raw)
					if err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(scratch, "graph.json"), data, 0600)
				}
			}
			if _, err := w.Publish(g, harnessManifest(t, root), persist); err != nil {
				t.Fatalf("legitimate callback semantics rejected: %v", err)
			}
		})
	}
}

// A hung RED runs only in a child TEST executable, never the product CLI. The
// parent kills/reaps it on deadline; all inputs, output and TMPDIR are isolated.
func TestHarnessReliabilityRemovedScratchFailureBounded(t *testing.T) {
	const childEnv = "VELA_HARNESS_REMOVED_SCRATCH_CHILD"
	if os.Getenv(childEnv) == "1" {
		root := os.Getenv("VELA_HARNESS_REMOVED_SCRATCH_ROOT")
		out := filepath.Join(root, ".vela")
		w, err := AcquireWriter(context.Background(), out)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		old, err := w.Publish(harnessGraph("prior"), harnessManifest(t, root), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, failure := range []error{errors.New("callback removed scratch"), context.Canceled} {
			_, err := w.Publish(harnessGraph("new"), harnessManifest(t, root), func(_ *types.Graph, scratch string) error {
				if err := os.RemoveAll(scratch); err != nil {
					return err
				}
				return failure
			})
			if !errors.Is(err, failure) {
				t.Fatalf("callback error not returned: %v", err)
			}
			selected, err := ResolveGeneration(out)
			if err != nil || selected.ID != old.ID {
				t.Fatal("removed-scratch failure changed selection")
			}
		}
		return
	}
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestHarnessReliabilityRemovedScratchFailureBounded$", "-test.count=1")
	cmd.Env = append(os.Environ(), childEnv+"=1", "VELA_HARNESS_REMOVED_SCRATCH_ROOT="+root, "TMPDIR="+root)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("publisher deadlocked after private scratch removal (child killed and reaped): %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("child fixture: %v\n%s", err, output)
	}
}

func TestHarnessReliabilityLegacyArtifactAliasesCannotMutateSeal(t *testing.T) {
	for _, name := range []string{"graph.json", "graph.json.tmp", "graph.db.tmp", "manifest.json.tmp"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, ".vela")
			w, err := AcquireWriter(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			s, err := w.Publish(harnessGraph("sealed"), harnessManifest(t, root), nil)
			w.Close()
			if err != nil {
				t.Fatal(err)
			}
			legacy := t.TempDir()
			if err := os.Symlink(filepath.Join(s.Dir, "graph.json"), filepath.Join(legacy, name)); err != nil {
				t.Fatal(err)
			}
			var writeErr error
			switch name {
			case "graph.json":
				writeErr = WriteJSON(harnessGraph("wrong"), legacy)
			case "graph.json.tmp":
				writeErr = WriteJSONAtomic(harnessGraph("wrong"), legacy)
			case "graph.db.tmp":
				writeErr = WriteSQLiteGraphAtomic(harnessGraph("wrong"), legacy)
			case "manifest.json.tmp":
				writeErr = WriteManifestAtomic(harnessManifest(t, root), legacy)
			}
			if writeErr == nil {
				t.Error("independent alias write accepted")
			}
			if _, err := ValidateGeneration(s.Dir); err != nil {
				t.Errorf("sealed artifact changed: %v", err)
			}
		})
	}
}

func TestHarnessReliabilityWorkspaceFailureRetainsPartialInventory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a"), 0600); err != nil {
		t.Fatal(err)
	}
	m := harnessManifest(t, root)
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".vela", "workspace.yaml"), 0700); err != nil {
		t.Fatal(err)
	}
	stale, err := generation.VerifySources(m)
	if err == nil || len(stale) != 1 || stale[0] != "a.go" {
		t.Fatalf("known change lost after topology failure: %v %v", stale, err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a"), 0600); err != nil {
		t.Fatal(err)
	}
	stale, err = generation.VerifySources(m)
	if err == nil || len(stale) != 0 {
		t.Fatalf("only unreadable topology must remain unknown: %v %v", stale, err)
	}
}

// Only temporary graph artifacts are written. No extractor, driver, CLI,
// registry or live database participates in this filename/atomicity regression.
func TestHarnessReliabilitySQLiteWriteSpecialPaths(t *testing.T) {
	for _, name := range []string{"question?mark", "hash#mark", "percent%mark", "spaces and ñλ", "all ?#% ñλ", "literal %2F%3F"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, name)
			if err := os.Mkdir(out, 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(out, "graph.db")
			previous := []byte("previous placeholder remains until successful rename")
			if err := os.WriteFile(target, previous, 0600); err != nil {
				t.Fatal(err)
			}
			bad := harnessGraph("bad")
			bad.Nodes[0].Metadata = map[string]interface{}{"unsupported": make(chan int)}
			if err := WriteSQLiteGraphAtomic(bad, out); err == nil {
				t.Fatal("expected failed metadata serialization")
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != string(previous) {
				t.Fatalf("failed write replaced target: %q %v", data, err)
			}
			if err := WriteSQLiteGraphAtomic(harnessGraph("roundtrip"), out); err != nil {
				t.Fatal(err)
			}
			uri, err := generation.ReadOnlyURI(target)
			if err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", uri)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var id string
			if err := db.QueryRow("SELECT id FROM nodes").Scan(&id); err != nil || id != "roundtrip" {
				t.Fatalf("filename roundtrip: %q %v", id, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != name || !entries[0].IsDir() {
				t.Fatalf("created a truncated/decoded sibling path: %v", entries)
			}
			entries, err = os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() != "graph.db" && entry.Name() != ".writer.lock" {
					t.Fatalf("unexpected temporary/database artifact: %s", entry.Name())
				}
			}
		})
	}
}

func TestHarnessReliabilityAggregateLabelResolutionAndTamper(t *testing.T) {
	root := t.TempDir()
	children := []string{filepath.Join(root, "x"), filepath.Join(root, "y")}
	m := harnessManifest(t, root)
	m.Aggregate = &types.AggregateManifest{ParentRoot: root, DiscoveryFingerprint: ChildDiscoveryFingerprint, ChildRoots: children}
	aggregate := &types.Graph{}
	for _, childRoot := range children {
		if err := os.MkdirAll(filepath.Join(childRoot, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		child := &types.Graph{Nodes: []types.Node{{ID: "a", Label: "Source", NodeType: "file"}, {ID: "b", Label: "Target", NodeType: "file"}}, Edges: []types.Edge{{Source: "a", Target: "Target", Relation: "calls"}}}
		cm := harnessManifest(t, childRoot)
		cm.Generation = "g-fixture"
		m.Aggregate.Children = append(m.Aggregate.Children, types.ChildSnapshot{Root: childRoot, Generation: cm.Generation, GraphDigest: GraphDigest(child), Manifest: *cm})
		namespaced := NamespaceChild(childRoot, child)
		aggregate.Nodes = append(aggregate.Nodes, namespaced.Nodes...)
		aggregate.Edges = append(aggregate.Edges, namespaced.Edges...)
	}
	w, err := AcquireWriter(context.Background(), filepath.Join(root, ".vela"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	s, err := w.Publish(aggregate, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(s.Dir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, child := range children {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM edges WHERE from_node_id=? AND to_node_id=?", ChildNamespace(child)+"a", ChildNamespace(child)+"b").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatal("cross-child label resolution")
		}
	}
	m.Aggregate.Children[1].GraphDigest = m.Aggregate.Children[0].GraphDigest // same graph is intentional
	m.Aggregate.Children[1].Manifest.Files = append(m.Aggregate.Children[1].Manifest.Files, types.ManifestFile{Path: "../outside.go", SHA256: string(make([]byte, 64))})
	if _, err := w.Publish(aggregate, m, nil); err == nil {
		t.Fatal("invalid embedded child inventory accepted")
	}
	if _, err := AcquireWriter(context.Background(), s.Dir); err == nil {
		t.Fatal("immutable generation accepted as writer output")
	}
}

func TestHarnessReliabilityInitialPublicationRestart(t *testing.T) {
	finalRenamed := false
	for _, boundary := range PublicationBoundaries {
		if boundary == "generations_directory_sync" {
			finalRenamed = true
		}
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, ".vela")
			w, err := AcquireWriter(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			w.Fault = func(at string) error {
				if at == boundary {
					return errors.New("injected")
				}
				return nil
			}
			if _, err := w.Publish(harnessGraph("A"), harnessManifest(t, root), nil); err == nil {
				t.Fatal("fault not executed")
			}
			w.Close()
			if s, err := ResolveGeneration(out); err == nil && s != nil && s.Graph.Nodes[0].ID != "A" {
				t.Fatal("partial generation selected")
			}
			w, err = AcquireWriter(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			s, err := w.Recover()
			if finalRenamed && err != nil {
				t.Fatalf("validated final generation not recovered: %v", err)
			}
			if !finalRenamed && err == nil {
				t.Fatal("incomplete staging was recovered")
			}
			if err == nil {
				if s == nil || s.Graph.Nodes[0].ID != "A" {
					t.Fatal("bad recovery")
				}
				if _, err := ValidateGeneration(s.Dir); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestHarnessReliabilityWriterLockCancellationAndAliases(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	w, err := AcquireWriter(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(out, alias); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if other, err := AcquireWriter(ctx, alias); !errors.Is(err, context.DeadlineExceeded) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("same output aliases did not coordinate: %v", err)
	}
	if _, err := w.Publish(harnessGraph("A"), harnessManifest(t, root), nil); err != nil {
		t.Fatal(err)
	}
}

func TestHarnessReliabilityDiscoveryMatchesIgnoreRules(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{"package.json": "{}", "a.ts": "a", "generated.ts": "ignored", "node_modules/dependency.ts": "ignored", "sub/.gitignore": "hidden.ts\n", "sub/hidden.ts": "ignored", "sub/visible.ts": "visible", ".velignore": "generated.ts\n"} {
		p := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, files, err := Inventory(root, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want, err := detect.Collect(root, []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("discovery mismatch: %v %v", files, want)
	}
	if err := os.WriteFile(filepath.Join(root, "generated.ts"), []byte("ignored edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if stale, err := CheckInventory(m); err != nil || len(stale) > 0 {
		t.Fatalf("excluded source changed freshness: %v %v", stale, err)
	}
	// Invalid ignore-file type reliably exercises unreadable configuration even
	// under privileged test runners (unlike chmod-only permission fixtures).
	if err := os.Remove(filepath.Join(root, ".velignore")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".velignore"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckInventory(m); err == nil {
		t.Fatal("unreadable ignore configuration accepted")
	}
}

func TestHarnessReliabilityRejectsArtifactDisagreementAndCorruption(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	w, err := AcquireWriter(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	a, err := w.Publish(harnessGraph("A"), harnessManifest(t, root), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Publish(harnessGraph("B"), harnessManifest(t, root), func(_ *types.Graph, stage string) error { return writeJSONAtomic(harnessGraph("wrong"), stage) }); err == nil {
		t.Fatal("disagreeing JSON/SQLite accepted")
	}
	b, err := w.Publish(harnessGraph("B"), harnessManifest(t, root), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.Dir, "manifest.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveGeneration(out); err == nil {
		t.Fatal("corrupt selection accepted")
	}
	recovered, err := w.Recover()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ID != a.ID {
		t.Fatal("recovered corrupted generation")
	}
}
func harnessManifest(t *testing.T, root string) *types.Manifest {
	t.Helper()
	m, _, err := Inventory(root, types.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Scenario: Publication never mixes generations, including restart/recovery.
// Fault hooks and all files are per-writer/per-test; no global hooks or live DBs.
func TestHarnessReliabilityPublicationFaults(t *testing.T) {
	for _, boundary := range PublicationBoundaries {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, ".vela")
			w, err := AcquireWriter(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			a, err := w.Publish(harnessGraph("A"), harnessManifest(t, root), nil)
			if err != nil {
				t.Fatal(err)
			}
			w.Fault = func(at string) error {
				if at == boundary {
					return errors.New("injected " + at)
				}
				return nil
			}
			if _, err := w.Publish(harnessGraph("B"), harnessManifest(t, root), nil); err == nil {
				t.Fatal("fault did not execute")
			}
			w.Close()
			if a.Graph.Nodes[0].ID != "A" || a.Manifest.Generation != a.ID {
				t.Fatal("pinned A changed")
			}
			if _, err := ValidateGeneration(a.Dir); err != nil {
				t.Fatal(err)
			}
			selected, err := ResolveGeneration(out)
			if err != nil {
				t.Fatal(err)
			}
			if selected.Graph.Nodes[0].ID != "A" && selected.Graph.Nodes[0].ID != "B" {
				t.Fatal("mixed selection")
			}
			w, err = AcquireWriter(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			if _, err := w.Recover(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHarnessReliabilityInvalidSelectionAndPartialStaging(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	w, err := AcquireWriter(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	a, err := w.Publish(harnessGraph("A"), harnessManifest(t, root), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(out, ".generations", ".stage-partial"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, ".current")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../escape", filepath.Join(out, ".current")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveGeneration(out); err == nil {
		t.Fatal("invalid pointer accepted")
	}
	recovered, err := w.Recover()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ID != a.ID {
		t.Fatal("recovered partial staging")
	}
}

func TestHarnessReliabilityInventoryChanges(t *testing.T) {
	for _, change := range []string{"add", "edit", "delete", "rename", "exclude", "config", "fingerprint", "unreadable"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "a.go")
			if err := os.WriteFile(path, []byte("package a"), 0600); err != nil {
				t.Fatal(err)
			}
			m := harnessManifest(t, root)
			var err error
			switch change {
			case "add":
				err = os.WriteFile(filepath.Join(root, "b.go"), []byte("package b"), 0600)
			case "edit":
				err = os.WriteFile(path, []byte("package changed"), 0600)
			case "delete":
				err = os.Remove(path)
			case "rename":
				err = os.Rename(path, filepath.Join(root, "b.go"))
			case "exclude":
				err = os.WriteFile(filepath.Join(root, ".velignore"), []byte("a.go\n"), 0600)
			case "config":
				err = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture"), 0600)
			case "fingerprint":
				m.ExtractorFingerprint = "old"
			case "unreadable":
				err = os.Symlink("missing", filepath.Join(root, "broken.go"))
			}
			if err != nil {
				t.Fatal(err)
			}
			stale, err := CheckInventory(m)
			if change == "unreadable" {
				if err == nil {
					t.Fatal("incomplete check succeeded")
				}
			} else if err != nil || len(stale) == 0 {
				t.Fatalf("stale=%v err=%v", stale, err)
			}
		})
	}
}

func TestHarnessReliabilityEmptyMetadataSemantics(t *testing.T) {
	root := t.TempDir()
	w, err := AcquireWriter(context.Background(), filepath.Join(root, ".vela"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	g := &types.Graph{Nodes: []types.Node{{ID: "a", Label: "a", NodeType: "file", Metadata: map[string]interface{}{}}, {ID: "b", Label: "b", NodeType: "file"}}, Edges: []types.Edge{{Source: "a", Target: "b", Relation: "uses", Metadata: map[string]interface{}{}}}}
	if _, err := w.Publish(g, harnessManifest(t, root), nil); err != nil {
		t.Fatal(err)
	}
}

func TestHarnessReliabilityPartialInventoryKeepsProvenChange(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.go", "z.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package a"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := harnessManifest(t, root)
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "z.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "z.go")); err != nil {
		t.Fatal(err)
	}
	stale, err := CheckInventory(m)
	if err == nil || len(stale) != 1 || stale[0] != "a.go" {
		t.Fatalf("known stale evidence lost: %v %v", stale, err)
	}
}
