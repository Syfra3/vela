package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/generation"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/scip"
	"github.com/Syfra3/vela/pkg/types"
)

func harnessSource(root string) *types.Source {
	return &types.Source{ID: "fixture", Name: "fixture", Path: root, Type: types.SourceTypeCodebase}
}
func harnessPut(t *testing.T, root, path, body string) {
	t.Helper()
	p := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// The real default structural parser is local/in-process. There are no SCIP
// processes, installers, Git calls, user config, or non-temporary input files.
func TestHarnessReliabilityOwnedCacheMatchesFullSnapshotOracle(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, ".vela")
	oracleOut := t.TempDir()
	harnessPut(t, root, "go.mod", "module fixture\n")
	harnessPut(t, root, "a.go", "package fixture\nfunc A(){B()}\n")
	harnessPut(t, root, "b.go", "package fixture\nfunc B(){}\n")
	cached := NewBuilder(Config{Source: harnessSource})
	full := NewBuilder(Config{Source: harnessSource, OutDir: oracleOut, DisableExtractionCache: true})
	for _, change := range []string{"initial", "unchanged", "edit", "add", "rename", "delete", "configuration", "dependency", "corruption"} {
		t.Run(change, func(t *testing.T) {
			switch change {
			case "edit":
				harnessPut(t, root, "a.go", "package fixture\nfunc A(){B();B()}\n")
			case "add":
				harnessPut(t, root, "c.go", "package fixture\nfunc C(){A()}\n")
			case "rename":
				if err := os.Rename(filepath.Join(root, "c.go"), filepath.Join(root, "d.go")); err != nil {
					t.Fatal(err)
				}
			case "delete":
				if err := os.Remove(filepath.Join(root, "d.go")); err != nil {
					t.Fatal(err)
				}
			case "configuration":
				harnessPut(t, root, "go.mod", "module fixture\ngo 1.26\n")
			case "dependency":
				harnessPut(t, root, ".velignore", "ignored/\n")
			case "corruption":
				entries, err := os.ReadDir(filepath.Join(out, ".extraction-cache"))
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if err := os.WriteFile(filepath.Join(out, ".extraction-cache", entry.Name()), []byte("corrupt"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if change == "dependency" {
				harnessPut(t, root, "ignored/types.go", "package ignored\nfunc Hidden(){}\n")
				harnessPut(t, root, "a.go", "package fixture\nimport _ \"fixture/ignored\"\nfunc A(){B()}\n")
			}
			got, err := cached.Build(context.Background(), types.BuildRequest{RepoRoot: root})
			if err != nil {
				t.Fatal(err)
			}
			want, err := full.Build(context.Background(), types.BuildRequest{RepoRoot: root})
			if err != nil {
				t.Fatal(err)
			}
			if export.GraphDigest(got.Graph) != export.GraphDigest(want.Graph) {
				t.Fatal("cache/full snapshot oracle mismatch")
			}
			if got.Snapshot.Manifest.SourceBinding != generation.BoundGoProfile {
				t.Fatalf("not bound: %s", got.Snapshot.Manifest.BindingReason)
			}
			if change == "unchanged" && got.CacheHits != 2 {
				t.Fatalf("unchanged hits=%d", got.CacheHits)
			}
			if change == "edit" && got.CacheHits != 1 {
				t.Fatalf("file-owned edit hits=%d", got.CacheHits)
			}
			// Deleting the renamed addition restores an exact earlier dependency
			// universe; content-addressed artifacts for that universe remain valid.
			if change != "unchanged" && change != "edit" && change != "delete" && got.CacheHits != 0 {
				t.Fatalf("conservative invalidation hits=%d", got.CacheHits)
			}
			for _, n := range got.Graph.Nodes {
				if n.Source != nil && n.Source.Path != root {
					t.Fatalf("source identity leaked: %+v", n.Source)
				}
			}
			data, err := os.ReadFile(filepath.Join(got.Snapshot.Dir, "graph.json"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "vela-source-") {
				t.Fatal("private snapshot path leaked")
			}
			e, err := query.LoadFromFile(got.GraphPath)
			if err != nil {
				t.Fatal(err)
			}
			if e.Freshness().Status != query.FreshnessFresh {
				t.Fatalf("captured graph not fresh: %+v", e.Freshness())
			}
		})
	}
}

type harnessABAScanner struct{}

func (harnessABAScanner) Scan(root string, _ []string, src *types.Source) ([]types.Node, []types.Edge, error) {
	path := filepath.Join(root, "a.go")
	a, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(path, []byte("package fixture\nfunc B(){}\n"), 0600); err != nil {
		return nil, nil, err
	}
	consumed, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(path, a, 0600); err != nil {
		return nil, nil, err
	}
	return []types.Node{{ID: "live", Label: string(consumed), NodeType: "file", SourceFile: "a.go", Source: src}}, nil, nil
}
func TestHarnessReliabilityABASourceBinding(t *testing.T) {
	root := t.TempDir()
	original := "package fixture\nfunc A(){}\n"
	harnessPut(t, root, "a.go", original)
	b := NewBuilder(Config{Source: harnessSource, GraphBuilder: func(n []types.Node, e []types.Edge) (*igraph.Graph, error) {
		harnessPut(t, root, "a.go", original)
		return igraph.Build(n, e)
	}})
	b.afterCapture = func() { harnessPut(t, root, "a.go", "package fixture\nfunc B(){}\n") }
	bound, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range bound.Graph.Nodes {
		if n.Label == "B" {
			t.Fatal("live B substituted for captured A")
		}
	}
	if bound.Snapshot.Manifest.SourceBinding != generation.BoundGoProfile {
		t.Fatal("closed snapshot not bound")
	}
	live := NewBuilder(Config{Source: harnessSource, Scanner: harnessABAScanner{}})
	for i := 0; i < 2; i++ {
		result, err := live.Build(context.Background(), types.BuildRequest{RepoRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		if result.CacheHits != 0 || result.Snapshot.Manifest.SourceBinding != "unbound" {
			t.Fatal("opaque output reused/bound")
		}
		e, err := query.LoadFromFile(result.GraphPath)
		if err != nil {
			t.Fatal(err)
		}
		if e.Freshness().Status != query.FreshnessUnknown {
			t.Fatal("equal live hashes certified ABA output")
		}
	}
}

type harnessOutputDriver struct {
	root  string
	paths []string
}

func (d *harnessOutputDriver) Name() string         { return "fake" }
func (d *harnessOutputDriver) Language() string     { return "go" }
func (d *harnessOutputDriver) Supports(string) bool { return true }
func (d *harnessOutputDriver) Index(_ context.Context, r scip.Request) (scip.Result, error) {
	d.paths = append(d.paths, r.OutputPath)
	if r.RepoRoot != d.root {
		return scip.Result{}, os.ErrInvalid
	}
	if _, err := os.Stat(r.OutputPath); !os.IsNotExist(err) {
		return scip.Result{}, os.ErrExist
	}
	return scip.Result{}, os.WriteFile(r.OutputPath, []byte("fake"), 0600)
}
func TestHarnessReliabilityUnclosedAndDriversFallback(t *testing.T) {
	root := t.TempDir()
	harnessPut(t, root, "a.go", "package fixture\nfunc A(){}\n")
	if err := os.Symlink("missing", filepath.Join(root, "irrelevant-link")); err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(Config{Source: harnessSource})
	r, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if r.Snapshot.Manifest.SourceBinding != "unbound" || !strings.Contains(r.Snapshot.Manifest.BindingReason, "unclosed") {
		t.Fatal("unclosed capture certified")
	}
	if err := os.Remove(filepath.Join(root, "irrelevant-link")); err != nil {
		t.Fatal(err)
	}
	d := &harnessOutputDriver{root: root}
	reg, err := scip.NewRegistry(d)
	if err != nil {
		t.Fatal(err)
	}
	b = NewBuilder(Config{Source: harnessSource, Registry: reg})
	for i := 0; i < 2; i++ {
		r, err = b.Build(context.Background(), types.BuildRequest{RepoRoot: root, Languages: []string{"go"}})
		if err != nil {
			t.Fatal(err)
		}
		if r.Snapshot.Manifest.SourceBinding != "unbound" || r.CacheHits != 0 {
			t.Fatal("driver relocated/certified/reused")
		}
	}
	if len(d.paths) != 2 || d.paths[0] == d.paths[1] {
		t.Fatal("driver output reused")
	}
}

type harnessOpaquePatcher struct{}

func (harnessOpaquePatcher) Name() string { return "opaque" }
func (harnessOpaquePatcher) Patch(_ context.Context, _ types.BuildRequest, f []types.Fact) ([]types.Fact, error) {
	return f, nil
}
func TestHarnessReliabilityUnsupportedProfilesStayUnbound(t *testing.T) {
	for _, profile := range []string{"patcher", "non_go", "escaping_import"} {
		t.Run(profile, func(t *testing.T) {
			root := t.TempDir()
			harnessPut(t, root, "a.go", "package fixture\nfunc A(){}\n")
			cfg := Config{Source: harnessSource}
			req := types.BuildRequest{RepoRoot: root}
			switch profile {
			case "patcher":
				cfg.Patchers = map[string]Patcher{"opaque": harnessOpaquePatcher{}}
				req.Patchers = []string{"opaque"}
			case "non_go":
				harnessPut(t, root, "x.ts", "export const x = 1")
			case "escaping_import":
				harnessPut(t, root, "go.mod", "module fixture\n")
				harnessPut(t, root, "a.go", "package fixture\nimport _ \"fixture/../../escape\"\n")
			}
			b := NewBuilder(cfg)
			// Exercise closure rejection but replace live fallback to ensure this
			// escaping-path fixture never reads outside its temporary source root.
			if profile == "escaping_import" {
				b.scanner = &harnessScanner{}
			}
			for i := 0; i < 2; i++ {
				r, err := b.Build(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				if r.Snapshot.Manifest.SourceBinding != "unbound" || r.CacheHits != 0 || r.Snapshot.Manifest.BindingReason == "" {
					t.Fatal("unsupported output bound or reused")
				}
				e, err := query.LoadFromFile(r.GraphPath)
				if err != nil {
					t.Fatal(err)
				}
				if e.Freshness().Status != query.FreshnessUnknown {
					t.Fatal("unsupported output fresh")
				}
			}
		})
	}
}

func TestHarnessReliabilityIgnoredGoClosureChangesAreStale(t *testing.T) {
	root := t.TempDir()
	harnessPut(t, root, "go.mod", "module fixture\n")
	harnessPut(t, root, ".velignore", "ignored/\n")
	harnessPut(t, root, "a.go", "package fixture\nimport _ \"fixture/ignored\"\n")
	harnessPut(t, root, "ignored/types.go", "package ignored\n")
	b := NewBuilder(Config{Source: harnessSource})
	r, err := b.Build(context.Background(), types.BuildRequest{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range r.Snapshot.Manifest.ConsumedFiles {
		if f.Path == "ignored/types.go" {
			found = true
		}
	}
	if !found {
		t.Fatal("ignored resolver input not captured")
	}
	harnessPut(t, root, "ignored/config.go", "package ignored\n")
	e, err := query.LoadFromFile(r.GraphPath)
	if err != nil {
		t.Fatal(err)
	}
	if e.Freshness().Status != query.FreshnessStale {
		t.Fatalf("ignored resolution change not stale: %+v", e.Freshness())
	}
}
