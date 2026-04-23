package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/export"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/scip"
	"github.com/Syfra3/vela/pkg/types"
)

type fakeScanner struct {
	gotRoot  string
	gotFiles []string
	gotSrc   *types.Source
	nodes    []types.Node
	edges    []types.Edge
	err      error
	order    *[]string
}

func (s *fakeScanner) Scan(root string, files []string, src *types.Source) ([]types.Node, []types.Edge, error) {
	s.gotRoot = root
	s.gotFiles = append([]string(nil), files...)
	s.gotSrc = src
	if s.order != nil {
		*s.order = append(*s.order, "scan")
	}
	return append([]types.Node(nil), s.nodes...), append([]types.Edge(nil), s.edges...), s.err
}

type fakeDriver struct {
	name         string
	language     string
	supported    bool
	result       scip.Result
	err          error
	called       int
	bootstrapped int
	order        *[]string
}

func (d *fakeDriver) Name() string { return d.name }

func (d *fakeDriver) Language() string { return d.language }

func (d *fakeDriver) Supports(string) bool { return d.supported }

func (d *fakeDriver) Index(context.Context, scip.Request) (scip.Result, error) {
	d.called++
	if d.order != nil {
		*d.order = append(*d.order, "index")
	}
	return d.result, d.err
}

func (d *fakeDriver) Bootstrap(context.Context, scip.Request) error {
	d.bootstrapped++
	if d.order != nil {
		*d.order = append(*d.order, "bootstrap")
	}
	return nil
}

type fakePatcher struct {
	name   string
	called int
	out    []types.Fact
	err    error
}

func (p *fakePatcher) Name() string { return p.name }

func (p *fakePatcher) Patch(context.Context, types.BuildRequest, []types.Fact) ([]types.Fact, error) {
	p.called++
	if p.err != nil {
		return nil, p.err
	}
	return append([]types.Fact(nil), p.out...), nil
}

func TestBuilderBuild_RunsDetectScanDriverPatchMergeAndPersist(t *testing.T) {
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela-test")
	scanner := &fakeScanner{
		nodes: []types.Node{{ID: "svc", Label: "svc", NodeType: "function"}},
	}
	driver := &fakeDriver{
		name:      "scip-go",
		language:  "go",
		supported: true,
		result: scip.Result{
			Driver:   "scip-go",
			Language: "go",
			Artifact: filepath.Join(repoRoot, ".vela", "scip", "go.scip"),
			Facts: []types.Fact{{
				Repo:     "vela",
				Language: "go",
				Kind:     types.FactKindDependsOn,
				From:     "svc",
				To:       "db",
				Provenance: []types.Provenance{{
					Stage:      string(types.BuildStageDrivers),
					Driver:     "scip-go",
					Source:     "scip",
					Confidence: types.ConfidenceDeclared,
					Artifact:   filepath.Join(repoRoot, ".vela", "scip", "go.scip"),
				}},
			}},
		},
	}
	registry, err := scip.NewRegistry(driver)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	patcher := &fakePatcher{
		name: "enrich-deps",
		out: []types.Fact{{
			Repo:     "vela",
			Language: "go",
			Kind:     types.FactKindDependsOn,
			From:     "svc",
			To:       "db",
			Provenance: []types.Provenance{{
				Stage:      string(types.BuildStagePatch),
				Driver:     "enrich-deps",
				Source:     "patcher",
				Confidence: types.ConfidenceExtracted,
			}},
		}},
	}

	var persisted *types.Graph
	var persistedPath string
	builder := NewBuilder(Config{
		Detect: func(string) ([]string, error) {
			return []string{filepath.Join(repoRoot, "main.go"), filepath.Join(repoRoot, "README.md")}, nil
		},
		Scanner:  scanner,
		Registry: registry,
		Patchers: map[string]Patcher{
			"enrich-deps": patcher,
		},
		GraphBuilder: igraph.Build,
		Persist: func(g *types.Graph, out string) error {
			persisted = g
			persistedPath = filepath.Join(out, "graph.json")
			return nil
		},
		OutDir: outDir,
	})

	result, err := builder.Build(context.Background(), types.BuildRequest{
		RepoRoot:  repoRoot,
		Languages: []string{"go"},
		Patchers:  []string{"enrich-deps"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if scanner.gotRoot != repoRoot {
		t.Fatalf("scanner root = %q, want %q", scanner.gotRoot, repoRoot)
	}
	if len(scanner.gotFiles) != 1 || filepath.Ext(scanner.gotFiles[0]) != ".go" {
		t.Fatalf("scanner files = %v, want only code files", scanner.gotFiles)
	}
	if scanner.gotSrc == nil || scanner.gotSrc.Name == "" {
		t.Fatalf("scanner source = %#v, want detected project source", scanner.gotSrc)
	}
	if driver.called != 1 {
		t.Fatalf("driver called = %d, want 1", driver.called)
	}
	if driver.bootstrapped != 1 {
		t.Fatalf("driver bootstrapped = %d, want 1", driver.bootstrapped)
	}
	if patcher.called != 1 {
		t.Fatalf("patcher called = %d, want 1", patcher.called)
	}
	if persisted == nil {
		t.Fatal("persisted graph = nil, want persisted graph")
	}
	if persistedPath != filepath.Join(outDir, "graph.json") {
		t.Fatalf("persisted path = %q, want %q", persistedPath, filepath.Join(outDir, "graph.json"))
	}
	if result.GraphPath != filepath.Join(outDir, "graph.json") {
		t.Fatalf("result graph path = %q", result.GraphPath)
	}
	if len(result.Facts) != 1 {
		t.Fatalf("facts len = %d, want 1", len(result.Facts))
	}
	if len(result.Graph.Nodes) != 2 {
		t.Fatalf("graph nodes = %d, want 2", len(result.Graph.Nodes))
	}
	if len(result.Graph.Edges) != 1 {
		t.Fatalf("graph edges = %d, want 1", len(result.Graph.Edges))
	}
	edge := result.Graph.Edges[0]
	if edge.Source != "svc" || edge.Target != "db" || edge.Relation != string(types.FactKindDependsOn) {
		t.Fatalf("merged edge = %+v", edge)
	}
	if got := edge.Metadata["evidence_type"]; got != "patcher" {
		t.Fatalf("edge evidence_type = %v, want patcher", got)
	}
	if got := edge.Metadata["evidence_confidence"]; got != string(types.ConfidenceExtracted) {
		t.Fatalf("edge evidence_confidence = %v, want %q", got, types.ConfidenceExtracted)
	}
	if len(result.StageReports) != 6 {
		t.Fatalf("stage reports len = %d, want 6", len(result.StageReports))
	}
}

func TestBuilderBuild_BootstrapsDriversBeforeScan(t *testing.T) {
	repoRoot := t.TempDir()
	order := make([]string, 0, 3)
	scanner := &fakeScanner{
		nodes: []types.Node{{ID: "svc", Label: "svc", NodeType: "function"}},
		order: &order,
	}
	driver := &fakeDriver{
		name:      "scip-go",
		language:  "go",
		supported: true,
		order:     &order,
		result:    scip.Result{Driver: "scip-go", Language: "go", Artifact: filepath.Join(repoRoot, ".vela", "scip", "go.scip")},
	}
	registry, err := scip.NewRegistry(driver)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	builder := NewBuilder(Config{
		Detect:       func(string) ([]string, error) { return []string{filepath.Join(repoRoot, "main.go")}, nil },
		Scanner:      scanner,
		Registry:     registry,
		GraphBuilder: igraph.Build,
		Persist:      func(*types.Graph, string) error { return nil },
	})

	_, err = builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot, Languages: []string{"go"}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := strings.Join(order, ","); got != "bootstrap,scan,index" {
		t.Fatalf("order = %q, want bootstrap,scan,index", got)
	}
}

func TestBuilderBuild_ReusesFreshPersistedGraphForDefaultBuild(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	graph := &types.Graph{
		Nodes: []types.Node{{ID: "file:main", Label: "main.go", NodeType: string(types.NodeTypeFile), SourceFile: "main.go"}},
		Edges: []types.Edge{{Source: "file:main", Target: "file:main", Relation: string(types.FactKindContains)}},
	}
	if err := export.WriteJSONAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteJSONAtomic() error = %v", err)
	}

	origRepoChange := latestRelevantRepoChange
	origExeChange := currentExecutableChange
	t.Cleanup(func() {
		latestRelevantRepoChange = origRepoChange
		currentExecutableChange = origExeChange
	})
	latestRelevantRepoChange = func(string) (time.Time, error) { return time.Time{}, nil }
	currentExecutableChange = func() (time.Time, error) { return time.Time{}, nil }

	scanner := &fakeScanner{nodes: []types.Node{{ID: "should-not-run", Label: "should-not-run", NodeType: "function"}}}
	builder := NewBuilder(Config{
		Detect: func(string) ([]string, error) {
			return []string{filepath.Join(repoRoot, "main.go")}, nil
		},
		Scanner:      scanner,
		GraphBuilder: igraph.Build,
		Persist:      func(*types.Graph, string) error { t.Fatal("persist should be skipped on cache hit"); return nil },
		OutDir:       outDir,
	})

	result, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if scanner.gotRoot != "" {
		t.Fatal("expected scanner to be skipped on fresh cache hit")
	}
	if result.GraphPath != filepath.Join(outDir, "graph.json") {
		t.Fatalf("GraphPath = %q, want cached graph path", result.GraphPath)
	}
	if len(result.Graph.Nodes) != 1 {
		t.Fatalf("cached graph nodes = %d, want 1", len(result.Graph.Nodes))
	}
	if len(result.StageReports) != 6 {
		t.Fatalf("stage reports len = %d, want 6", len(result.StageReports))
	}
}

func TestBuilderBuild_SkipsCacheWhenExecutableIsNewer(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	if err := export.WriteJSONAtomic(&types.Graph{}, outDir); err != nil {
		t.Fatalf("WriteJSONAtomic() error = %v", err)
	}

	origRepoChange := latestRelevantRepoChange
	origExeChange := currentExecutableChange
	t.Cleanup(func() {
		latestRelevantRepoChange = origRepoChange
		currentExecutableChange = origExeChange
	})
	latestRelevantRepoChange = func(string) (time.Time, error) { return time.Time{}, nil }
	currentExecutableChange = func() (time.Time, error) { return time.Now().Add(time.Minute), nil }

	scanner := &fakeScanner{nodes: []types.Node{{ID: "svc", Label: "svc", NodeType: "function"}}}
	builder := NewBuilder(Config{
		Detect:       func(string) ([]string, error) { return []string{filepath.Join(repoRoot, "main.go")}, nil },
		Scanner:      scanner,
		GraphBuilder: igraph.Build,
		Persist:      func(*types.Graph, string) error { return nil },
		OutDir:       outDir,
	})

	_, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if scanner.gotRoot != repoRoot {
		t.Fatal("expected full rebuild when executable is newer than cached graph")
	}
}

func TestBuilderBuild_FailsWhenNamedPatcherMissing(t *testing.T) {
	builder := NewBuilder(Config{
		Detect:       func(string) ([]string, error) { return nil, nil },
		Scanner:      &fakeScanner{},
		GraphBuilder: igraph.Build,
		Persist:      func(*types.Graph, string) error { return nil },
	})

	_, err := builder.Build(context.Background(), types.BuildRequest{
		RepoRoot: "/repo",
		Patchers: []string{"missing"},
	})
	if err == nil {
		t.Fatal("Build() error = nil, want missing patcher error")
	}
	if !errors.Is(err, ErrUnknownPatcher) {
		t.Fatalf("Build() error = %v, want ErrUnknownPatcher", err)
	}
}

func TestBuilderBuild_EmitsObserverStageEvents(t *testing.T) {
	repoRoot := t.TempDir()
	var events []StageEvent
	builder := NewBuilder(Config{
		Detect:       func(string) ([]string, error) { return []string{filepath.Join(repoRoot, "main.go")}, nil },
		Scanner:      &fakeScanner{nodes: []types.Node{{ID: "svc", Label: "svc", NodeType: "function"}}},
		GraphBuilder: igraph.Build,
		Persist:      func(*types.Graph, string) error { return nil },
		Observer: func(event StageEvent) {
			events = append(events, event)
		},
	})

	_, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(events) != 12 {
		t.Fatalf("observer events = %d, want 12", len(events))
	}
	if events[0].Stage != types.BuildStageDetect {
		t.Fatalf("first stage = %q, want %q", events[0].Stage, types.BuildStageDetect)
	}
	if events[0].Message != "starting detect stage" {
		t.Fatalf("first message = %q, want starting detect stage", events[0].Message)
	}
	if events[1].Message != "detected source files" {
		t.Fatalf("second message = %q, want detected source files", events[1].Message)
	}
	if events[len(events)-1].Stage != types.BuildStagePersist {
		t.Fatalf("last stage = %q, want %q", events[len(events)-1].Stage, types.BuildStagePersist)
	}
	if events[len(events)-1].Message != "persisted graph" {
		t.Fatalf("last message = %q, want persisted graph", events[len(events)-1].Message)
	}
}

func TestBuilderBuild_WarnsAndContinuesWhenDriverBinaryMissing(t *testing.T) {
	repoRoot := t.TempDir()
	driver := &fakeDriver{
		name:      "scip-typescript",
		language:  "typescript",
		supported: true,
		err:       &scip.MissingBinaryError{Driver: "scip-typescript", Command: "scip-typescript", RepoRoot: repoRoot, InstallHint: "npm install -g @sourcegraph/scip-typescript"},
	}
	registry, err := scip.NewRegistry(driver)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	builder := NewBuilder(Config{
		Detect:       func(string) ([]string, error) { return []string{filepath.Join(repoRoot, "index.ts")}, nil },
		Scanner:      &fakeScanner{nodes: []types.Node{{ID: "svc", Label: "svc", NodeType: "function"}}},
		Registry:     registry,
		GraphBuilder: igraph.Build,
		Persist:      func(*types.Graph, string) error { return nil },
	})

	result, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot, Languages: []string{"typescript"}})
	if err != nil {
		t.Fatalf("Build() error = %v, want warning-only degrade", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1", len(result.Warnings))
	}
	if result.Warnings[0] != "SCIP driver unavailable: scip-typescript is not installed. Install it with: npm install -g @sourcegraph/scip-typescript (repo: "+repoRoot+")" {
		t.Fatalf("warning = %q", result.Warnings[0])
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts len = %d, want 0 when driver is skipped", len(result.Facts))
	}
}

func TestBuilderBuild_WarnsAndContinuesWhenDriverExecutionFails(t *testing.T) {
	repoRoot := t.TempDir()
	driver := &fakeDriver{
		name:      "scip-go",
		language:  "go",
		supported: true,
		err:       errors.New("panic: nil pointer dereference"),
	}
	registry, err := scip.NewRegistry(driver)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	builder := NewBuilder(Config{
		Detect:       func(string) ([]string, error) { return []string{filepath.Join(repoRoot, "main.go")}, nil },
		Scanner:      &fakeScanner{nodes: []types.Node{{ID: "svc", Label: "svc", NodeType: "function"}}},
		Registry:     registry,
		GraphBuilder: igraph.Build,
		Persist:      func(*types.Graph, string) error { return nil },
	})

	result, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot, Languages: []string{"go"}})
	if err != nil {
		t.Fatalf("Build() error = %v, want warning-only degrade", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1", len(result.Warnings))
	}
	if got := result.Warnings[0]; !strings.Contains(got, "SCIP driver failed: scip-go: panic: nil pointer dereference") || !strings.Contains(got, "go install github.com/sourcegraph/scip-go/cmd/scip-go@latest") || !strings.Contains(got, repoRoot) {
		t.Fatalf("warning = %q", got)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts len = %d, want 0 when driver fails", len(result.Facts))
	}
	if driver.called != 1 {
		t.Fatalf("driver called = %d, want 1", driver.called)
	}
}

func TestProjectFileDependencyEdges_ProjectsSymbolCallsToFiles(t *testing.T) {
	nodes := []types.Node{
		{ID: "project:vela", Label: "vela", NodeType: string(types.NodeTypeProject), Source: &types.Source{Type: types.SourceTypeCodebase, Name: "vela"}},
		{ID: "vela:file:cmd/vela/main.go", Label: "cmd/vela/main.go", NodeType: string(types.NodeTypeFile), SourceFile: "cmd/vela/main.go", Source: &types.Source{Type: types.SourceTypeCodebase, Name: "vela"}},
		{ID: "vela:file:internal/config/config.go", Label: "internal/config/config.go", NodeType: string(types.NodeTypeFile), SourceFile: "internal/config/config.go", Source: &types.Source{Type: types.SourceTypeCodebase, Name: "vela"}},
		{ID: "vela:cmd/vela/main.go:main", Label: "main", NodeType: string(types.NodeTypeFunction), SourceFile: "cmd/vela/main.go", Source: &types.Source{Type: types.SourceTypeCodebase, Name: "vela"}},
		{ID: "vela:internal/config/config.go:Load", Label: "Load", NodeType: string(types.NodeTypeFunction), SourceFile: "internal/config/config.go", Source: &types.Source{Type: types.SourceTypeCodebase, Name: "vela"}},
	}
	edges := []types.Edge{{Source: "vela:cmd/vela/main.go:main", Target: "Load", Relation: string(types.FactKindCalls), Confidence: string(types.ConfidenceExtracted), SourceFile: "cmd/vela/main.go"}}

	projected := projectFileDependencyEdges(nodes, edges)
	if len(projected) != 1 {
		t.Fatalf("projected len = %d, want 1", len(projected))
	}
	if projected[0].Source != "vela:file:cmd/vela/main.go" || projected[0].Target != "vela:file:internal/config/config.go" || projected[0].Relation != string(types.FactKindDependsOn) {
		t.Fatalf("projected edge = %+v", projected[0])
	}
}
