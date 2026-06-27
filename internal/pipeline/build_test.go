package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/export"
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/query"
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

func writeTestFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

// REQ-002/REQ-003 → SCN-004 → TestSCN004_BuildCreatesRuntimeAndGeneratedArtifacts
func TestSCN004_BuildCreatesRuntimeAndGeneratedArtifacts(t *testing.T) {
	// Scenario: Build creates runtime and generated graph artifacts with SQLite as truth.
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package main\n")

	builder := NewBuilder(Config{
		Detect: func(string) ([]string, error) {
			return []string{filepath.Join(repoRoot, "main.go")}, nil
		},
		Scanner: &fakeScanner{
			nodes: []types.Node{{ID: "main", Label: "main", NodeType: "package", SourceFile: "main.go"}},
			edges: []types.Edge{},
		},
		GraphBuilder: igraph.Build,
		OutDir:       outDir,
	})

	result, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, path := range []string{
		filepath.Join(outDir, "graph.db"),
		filepath.Join(outDir, "graph.json"),
		filepath.Join(outDir, "manifest.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected build artifact %s: %v", path, err)
		}
	}
	graphDB, err := os.ReadFile(filepath.Join(outDir, "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(graphDB), "SQLite format 3\x00") {
		t.Fatal("graph.db is not a SQLite runtime store")
	}
	if result.GraphPath != filepath.Join(outDir, "graph.json") {
		t.Fatalf("GraphPath = %q, want generated debug graph.json path", result.GraphPath)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	var generated map[string]any
	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatalf("graph.json is invalid debug JSON: %v", err)
	}
}

// REQ-014 → SCN-021 → TestSCN021_SingleRepoSQLiteFixturePersistsQueryableGraphFacts
func TestSCN021_SingleRepoSQLiteFixturePersistsQueryableGraphFacts(t *testing.T) {
	// Scenario: Single-repo fixture proves SQLite graph build and symbol dependency queries.
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	writeTestFile(t, filepath.Join(repoRoot, "handler.go"), "package fixture\nfunc Handler() { Store() }\n")
	writeTestFile(t, filepath.Join(repoRoot, "store.go"), "package fixture\nfunc Store() {}\n")

	builder := NewBuilder(Config{
		Detect: func(string) ([]string, error) {
			return []string{filepath.Join(repoRoot, "handler.go"), filepath.Join(repoRoot, "store.go")}, nil
		},
		Scanner: &fakeScanner{
			nodes: []types.Node{
				{ID: "fixture:file:handler.go", Label: "handler.go", NodeType: string(types.NodeTypeFile), SourceFile: "handler.go"},
				{ID: "fixture:file:store.go", Label: "store.go", NodeType: string(types.NodeTypeFile), SourceFile: "store.go"},
				{ID: "fixture:handler.go:Handler", Label: "Handler", NodeType: string(types.NodeTypeFunction), SourceFile: "handler.go"},
				{ID: "fixture:store.go:Store", Label: "Store", NodeType: string(types.NodeTypeFunction), SourceFile: "store.go"},
			},
			edges: []types.Edge{
				{Source: "fixture:file:handler.go", Target: "fixture:handler.go:Handler", Relation: string(types.FactKindContains)},
				{Source: "fixture:file:store.go", Target: "fixture:store.go:Store", Relation: string(types.FactKindContains)},
				{Source: "fixture:handler.go:Handler", Target: "fixture:store.go:Store", Relation: string(types.FactKindCalls), Confidence: string(types.ConfidenceExtracted)},
			},
		},
		GraphBuilder: igraph.Build,
		OutDir:       outDir,
	})

	if _, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(outDir, "graph.db"))
	if err != nil {
		t.Fatalf("Open(graph.db) error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertSQLiteScalar(t, db, `SELECT COUNT(*) FROM nodes WHERE canonical_key = 'fixture:handler.go:Handler' AND kind = 'function' AND label = 'Handler'`, 1)
	assertSQLiteScalar(t, db, `SELECT COUNT(*) FROM edges WHERE from_node_id = 'fixture:handler.go:Handler' AND to_node_id = 'fixture:store.go:Store' AND kind = 'calls'`, 1)
	assertSQLiteScalar(t, db, `SELECT COUNT(*) FROM pragma_index_list('nodes') WHERE [unique] = 1 AND name = 'nodes_canonical_key_idx'`, 1)
}

// REQ-009/REQ-010/REQ-015 → SCN-012 → TestSCN012_InterfaceEvidenceFixturePreservesClaimStatuses
func TestSCN012_InterfaceEvidenceFixturePreservesClaimStatuses(t *testing.T) {
	// Scenario: Interface evidence fixture preserves declared, extracted, inferred, and ambiguous facts.
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package fixture\n")

	nodes := []types.Node{
		{ID: "service:frontend", Label: "frontend", NodeType: string(types.NodeTypeService)},
		{ID: "service:billing", Label: "billing", NodeType: string(types.NodeTypeService)},
	}
	edges := []types.Edge{
		interfaceEvidenceEdge("OpenAPIProvider", "declared", "contracts/openapi.yaml"),
		interfaceEvidenceEdge("ProtoProvider", "declared", "proto/billing.proto"),
		interfaceEvidenceEdge("FrameworkRoutesProvider", "extracted", "cmd/server/routes.go"),
		interfaceEvidenceEdge("HttpClientProvider", "extracted", "web/client.ts"),
		interfaceEvidenceEdge("ManifestProvider", "inferred", "package.json"),
		interfaceEvidenceEdge("WorkspaceHintsProvider", "declared_hint", ".vela/workspace.yaml"),
		interfaceEvidenceEdge("NamingHeuristicsProvider", "ambiguous", "services/billing"),
	}

	builder := NewBuilder(Config{
		Detect: func(string) ([]string, error) {
			return []string{filepath.Join(repoRoot, "main.go")}, nil
		},
		Scanner:      &fakeScanner{nodes: nodes, edges: edges},
		GraphBuilder: igraph.Build,
		OutDir:       outDir,
	})

	if _, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(outDir, "graph.db"))
	if err != nil {
		t.Fatalf("Open(graph.db) error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	assertInterfaceEvidenceStatus(t, db, "OpenAPIProvider", "declared")
	assertInterfaceEvidenceStatus(t, db, "ProtoProvider", "declared")
	assertInterfaceEvidenceStatus(t, db, "FrameworkRoutesProvider", "extracted")
	assertInterfaceEvidenceStatus(t, db, "HttpClientProvider", "extracted")
	assertInterfaceEvidenceStatus(t, db, "ManifestProvider", "inferred")
	assertInterfaceEvidenceStatus(t, db, "WorkspaceHintsProvider", "declared_hint")
	assertInterfaceEvidenceStatus(t, db, "NamingHeuristicsProvider", "ambiguous")
}

// REQ-011 → SCN-014 → TestSCN014_WorkspaceYAMLDeclaresMultiCodebaseTopology
func TestSCN014_WorkspaceYAMLDeclaresMultiCodebaseTopology(t *testing.T) {
	// Scenario: Workspace YAML declares multi-codebase topology.
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package fixture\n")
	writeTestFile(t, filepath.Join(repoRoot, ".vela", "workspace.yaml"), `organization:
  name: acme
repositories:
  - name: billing-api
    services:
      - name: billing
        kind: api
interfaces:
  - name: billing-http
    service: billing
    kind: http
known_links:
  - from: checkout
    to: billing
    interface: billing-http
`)

	builder := NewBuilder(Config{
		Detect: func(string) ([]string, error) {
			return []string{filepath.Join(repoRoot, "main.go")}, nil
		},
		Scanner:      &fakeScanner{},
		GraphBuilder: igraph.Build,
		OutDir:       outDir,
	})

	result, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	assertGraphHasWorkspaceNode(t, result.Graph, "workspace:repo:billing-api", "repo")
	assertGraphHasWorkspaceNode(t, result.Graph, "workspace:service:billing", "service")
	assertGraphHasWorkspaceNode(t, result.Graph, "workspace:interface:billing-http", "interface")
	assertGraphHasWorkspaceEdge(t, result.Graph, "workspace:repo:billing-api", "billing", "exposes")
	assertGraphHasWorkspaceEdge(t, result.Graph, "workspace:service:billing", "billing-http", "exposes")
	assertGraphHasWorkspaceEdge(t, result.Graph, "workspace:service:checkout", "billing", "uses")

	db, err := sql.Open("sqlite", filepath.Join(outDir, "graph.db"))
	if err != nil {
		t.Fatalf("Open(graph.db) error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	assertSQLiteScalar(t, db, `SELECT COUNT(*) FROM workspace_facts WHERE fact_kind = 'exposes' AND subject_key = 'workspace:repo:billing-api' AND object_key = 'workspace:service:billing' AND confidence = 'declared_hint' AND source_id = '.vela/workspace.yaml'`, 1)
	assertSQLiteScalar(t, db, `SELECT COUNT(*) FROM workspace_facts WHERE fact_kind = 'uses' AND subject_key = 'workspace:service:checkout' AND object_key = 'workspace:service:billing' AND confidence = 'declared_hint' AND source_id = '.vela/workspace.yaml'`, 1)
}

// REQ-011/REQ-015 → SCN-015 → TestSCN015_InvalidWorkspaceYAMLReturnsValidationDiagnostic
func TestSCN015_InvalidWorkspaceYAMLReturnsValidationDiagnostic(t *testing.T) {
	// Scenario: Invalid workspace YAML fails with actionable validation diagnostics.
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package fixture\n")
	writeTestFile(t, filepath.Join(repoRoot, ".vela", "workspace.yaml"), `organization:
  name: acme
repositories:
  - services:
      - name: billing
        kind: api
`)

	builder := NewBuilder(Config{
		Detect: func(string) ([]string, error) {
			return []string{filepath.Join(repoRoot, "main.go")}, nil
		},
		Scanner:      &fakeScanner{},
		GraphBuilder: igraph.Build,
		OutDir:       outDir,
	})

	_, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err == nil {
		t.Fatal("Build() error = nil, want workspace validation error")
	}
	if got := err.Error(); !strings.Contains(got, "workspace validation") || !strings.Contains(got, ".vela/workspace.yaml repositories[0].name") {
		t.Fatalf("Build() error = %q, want actionable repositories[0].name validation diagnostic", got)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "graph.db")); !os.IsNotExist(statErr) {
		t.Fatalf("graph.db stat error = %v, want not exist after invalid topology", statErr)
	}
}

// REQ-014 → SCN-022 → TestSCN022_MultiRepoFixtureRoutesThroughDeclaredWorkspaceTopology
func TestSCN022_MultiRepoFixtureRoutesThroughDeclaredWorkspaceTopology(t *testing.T) {
	// Scenario: Multi-repo fixture proves declared workspace routing.
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package fixture\n")
	writeTestFile(t, filepath.Join(repoRoot, ".vela", "workspace.yaml"), `organization:
  name: acme
repositories:
  - name: billing-api
    services:
      - name: billing
        kind: api
  - name: checkout-web
    services:
      - name: checkout
        kind: web
interfaces:
  - name: billing-http
    service: billing
    kind: http
known_links:
  - from: checkout
    to: billing
    interface: billing-http
`)

	builder := NewBuilder(Config{
		Detect: func(string) ([]string, error) {
			return []string{filepath.Join(repoRoot, "main.go")}, nil
		},
		Scanner:      &fakeScanner{},
		GraphBuilder: igraph.Build,
		OutDir:       outDir,
	})

	result, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	engine, err := query.LoadFromFile(result.GraphPath)
	if err != nil {
		t.Fatalf("LoadFromFile(%s) error = %v", result.GraphPath, err)
	}

	output := engine.RenderExplore("checkout billing", 5)
	for _, want := range []string{
		"Workspace routes for \"checkout billing\":",
		"billing-api score=",
		"checkout-web score=",
		"billing-api [workspace/repo] --[exposes]--> billing [workspace/service]",
		"checkout-web [workspace/repo] --[exposes]--> checkout [workspace/service]",
		"checkout [workspace/service] --[uses]--> billing [workspace/service]",
		"artifact=.vela/workspace.yaml",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected explore output to contain %q, got %q", want, output)
		}
	}
}

func assertGraphHasWorkspaceNode(t *testing.T, g *types.Graph, id, kind string) {
	t.Helper()
	for _, node := range g.Nodes {
		if node.ID == id && node.NodeType == kind && node.SourceFile == ".vela/workspace.yaml" && node.Metadata["layer"] == "workspace" && node.Metadata["evidence_type"] == "routing" {
			return
		}
	}
	t.Fatalf("workspace node %s/%s with routing provenance not found", id, kind)
}

func assertGraphHasWorkspaceEdge(t *testing.T, g *types.Graph, source, target, relation string) {
	t.Helper()
	for _, edge := range g.Edges {
		if edge.Source == source && edge.Target == target && edge.Relation == relation && edge.SourceFile == ".vela/workspace.yaml" && edge.Metadata["layer"] == "workspace" && edge.Metadata["evidence_type"] == "routing" {
			return
		}
	}
	t.Fatalf("workspace edge %s -[%s]-> %s with routing provenance not found", source, relation, target)
}

func interfaceEvidenceEdge(provider, claimStatus, sourceArtifact string) types.Edge {
	return types.Edge{
		Source:     "service:frontend",
		Target:     "service:billing",
		Relation:   "interface_fact",
		SourceFile: sourceArtifact,
		Metadata: map[string]interface{}{
			"interface_provider":       provider,
			"interface_kind":           "http",
			"interface_name":           provider + " billing link",
			"interface_route":          "/billing",
			"interface_method":         "GET",
			"evidence_confidence":      claimStatus,
			"claim_status":             claimStatus,
			"evidence_source_artifact": sourceArtifact,
		},
	}
}

func assertInterfaceEvidenceStatus(t *testing.T, db *sql.DB, provider, wantStatus string) {
	t.Helper()
	var got string
	err := db.QueryRow(`SELECT claim_status FROM interface_facts WHERE provider = ?`, provider).Scan(&got)
	if err != nil {
		t.Fatalf("interface fact provider %s missing: %v", provider, err)
	}
	if got != wantStatus {
		t.Fatalf("interface fact provider %s claim_status = %q, want %q", provider, got, wantStatus)
	}
}

func assertSQLiteScalar(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("QueryRow(%q) error = %v", query, err)
	}
	if got != want {
		t.Fatalf("QueryRow(%q) = %d, want %d", query, got, want)
	}
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

type fakeClusterer struct {
	partition map[string]int
	err       error
	called    int
}

func (c *fakeClusterer) Run(*igraph.Graph) (map[string]int, error) {
	c.called++
	if c.err != nil {
		return nil, c.err
	}
	return c.partition, nil
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
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(repoRoot, "README.md"), "# test\n")
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
	clusterer := &fakeClusterer{partition: map[string]int{"svc": 7, "db": 9}}
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
		Cluster:      clusterer.Run,
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
	if clusterer.called != 1 {
		t.Fatalf("clusterer called = %d, want 1", clusterer.called)
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
	if result.Graph.Nodes[0].Community == 0 && result.Graph.Nodes[1].Community == 0 {
		t.Fatal("expected clustering to annotate graph nodes before persistence")
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
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package main\n")
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
	manifest, err := buildManifest(repoRoot, []string{filepath.Join(repoRoot, "main.go")})
	if err != nil {
		t.Fatalf("buildManifest() error = %v", err)
	}
	manifest.GeneratedAt = time.Now().UTC()
	if err := export.WriteManifestAtomic(manifest, outDir); err != nil {
		t.Fatalf("WriteManifestAtomic() error = %v", err)
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
	manifest, err := buildManifest(repoRoot, []string{filepath.Join(repoRoot, "main.go")})
	if err != nil {
		t.Fatalf("buildManifest() error = %v", err)
	}
	manifest.GeneratedAt = time.Now().UTC()
	if err := export.WriteManifestAtomic(manifest, outDir); err != nil {
		t.Fatalf("WriteManifestAtomic() error = %v", err)
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

	_, err = builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if scanner.gotRoot != repoRoot {
		t.Fatal("expected full rebuild when executable is newer than cached graph")
	}
}

func TestBuilderBuild_FallsBackToFullRebuildWhenManifestMissing(t *testing.T) {
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
	currentExecutableChange = func() (time.Time, error) { return time.Time{}, nil }

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
		t.Fatal("expected full rebuild when manifest is missing")
	}
}

func TestBuilderBuild_FallsBackToFullRebuildWhenManifestHashChanges(t *testing.T) {
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	mainFile := filepath.Join(repoRoot, "main.go")
	if err := os.WriteFile(mainFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	if err := export.WriteJSONAtomic(&types.Graph{}, outDir); err != nil {
		t.Fatalf("WriteJSONAtomic() error = %v", err)
	}
	manifest, err := buildManifest(repoRoot, []string{mainFile})
	if err != nil {
		t.Fatalf("buildManifest() error = %v", err)
	}
	manifest.GeneratedAt = time.Now().UTC()
	if err := export.WriteManifestAtomic(manifest, outDir); err != nil {
		t.Fatalf("WriteManifestAtomic() error = %v", err)
	}
	if err := os.WriteFile(mainFile, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go updated) error = %v", err)
	}

	origRepoChange := latestRelevantRepoChange
	origExeChange := currentExecutableChange
	t.Cleanup(func() {
		latestRelevantRepoChange = origRepoChange
		currentExecutableChange = origExeChange
	})
	latestRelevantRepoChange = func(string) (time.Time, error) { return time.Time{}, nil }
	currentExecutableChange = func() (time.Time, error) { return time.Time{}, nil }

	scanner := &fakeScanner{nodes: []types.Node{{ID: "svc", Label: "svc", NodeType: "function"}}}
	builder := NewBuilder(Config{
		Detect:       func(string) ([]string, error) { return []string{mainFile}, nil },
		Scanner:      scanner,
		GraphBuilder: igraph.Build,
		Persist:      func(*types.Graph, string) error { return nil },
		OutDir:       outDir,
	})

	_, err = builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if scanner.gotRoot != repoRoot {
		t.Fatal("expected full rebuild when manifest hash no longer matches current file")
	}
}

func TestManifestReuseMode_ClassifiesDeletedOnlyDiff(t *testing.T) {
	saved := &types.Manifest{
		Version:              manifestVersion,
		ExtractorFingerprint: extractorFingerprint,
		Files: []types.ManifestFile{
			{Path: "cmd/vela/main.go", SHA256: "a"},
			{Path: "internal/query/query.go", SHA256: "b"},
		},
	}
	current := &types.Manifest{
		Version:              manifestVersion,
		ExtractorFingerprint: extractorFingerprint,
		Files: []types.ManifestFile{
			{Path: "cmd/vela/main.go", SHA256: "a"},
		},
	}

	mode, diff := manifestReuseMode(saved, current)
	if mode != cacheReuseDeletedOnly {
		t.Fatalf("reuse mode = %q, want %q", mode, cacheReuseDeletedOnly)
	}
	if len(diff.DeletedFiles) != 1 || diff.DeletedFiles[0].Path != "internal/query/query.go" {
		t.Fatalf("deleted files = %+v", diff.DeletedFiles)
	}
	if len(diff.NewFiles) != 0 || len(diff.ChangedFiles) != 0 {
		t.Fatalf("unexpected manifest diff = %+v", diff)
	}
}

func TestPruneGraphForDeletedFiles_RemovesOwnedNodesAndEdges(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{ID: "repo:file:cmd/vela/main.go", Label: "cmd/vela/main.go", NodeType: string(types.NodeTypeFile), SourceFile: "cmd/vela/main.go"},
			{ID: "repo:cmd/vela/main.go:main", Label: "main", NodeType: string(types.NodeTypeFunction), SourceFile: "cmd/vela/main.go"},
			{ID: "repo:file:internal/query/query.go", Label: "internal/query/query.go", NodeType: string(types.NodeTypeFile), SourceFile: "internal/query/query.go"},
			{ID: "repo:internal/query/query.go:Search", Label: "Search", NodeType: string(types.NodeTypeFunction), SourceFile: "internal/query/query.go"},
		},
		Edges: []types.Edge{
			{Source: "repo:cmd/vela/main.go:main", Target: "repo:internal/query/query.go:Search", Relation: string(types.FactKindCalls), SourceFile: "cmd/vela/main.go"},
			{Source: "repo:internal/query/query.go:Search", Target: "repo:file:internal/query/query.go", Relation: string(types.FactKindContains), SourceFile: "internal/query/query.go"},
		},
	}

	pruned := pruneGraphForDeletedFiles(g, []string{"internal/query/query.go"})
	if len(pruned.Nodes) != 2 {
		t.Fatalf("pruned nodes = %d, want 2", len(pruned.Nodes))
	}
	for _, node := range pruned.Nodes {
		if strings.Contains(node.ID, "internal/query/query.go") || node.SourceFile == "internal/query/query.go" {
			t.Fatalf("unexpected node retained after prune: %+v", node)
		}
	}
	if len(pruned.Edges) != 0 {
		t.Fatalf("pruned edges = %d, want 0", len(pruned.Edges))
	}
}

func TestBuilderBuild_PrunesDeletedFilesFromCachedGraph(t *testing.T) {
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, ".vela")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	mainFile := filepath.Join(repoRoot, "main.go")
	deletedFile := filepath.Join(repoRoot, "old.go")
	writeTestFile(t, mainFile, "package main\n")
	writeTestFile(t, deletedFile, "package main\nfunc old() {}\n")
	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "repo:file:main.go", Label: "main.go", NodeType: string(types.NodeTypeFile), SourceFile: "main.go"},
			{ID: "repo:main.go:main", Label: "main", NodeType: string(types.NodeTypeFunction), SourceFile: "main.go"},
			{ID: "repo:file:old.go", Label: "old.go", NodeType: string(types.NodeTypeFile), SourceFile: "old.go"},
			{ID: "repo:old.go:old", Label: "old", NodeType: string(types.NodeTypeFunction), SourceFile: "old.go"},
		},
		Edges: []types.Edge{{Source: "repo:main.go:main", Target: "repo:old.go:old", Relation: string(types.FactKindCalls), SourceFile: "main.go"}},
	}
	if err := export.WriteJSONAtomic(graph, outDir); err != nil {
		t.Fatalf("WriteJSONAtomic() error = %v", err)
	}
	savedManifest, err := buildManifest(repoRoot, []string{mainFile, deletedFile})
	if err != nil {
		t.Fatalf("buildManifest(saved) error = %v", err)
	}
	savedManifest.GeneratedAt = time.Now().UTC()
	if err := export.WriteManifestAtomic(savedManifest, outDir); err != nil {
		t.Fatalf("WriteManifestAtomic() error = %v", err)
	}
	if err := os.Remove(deletedFile); err != nil {
		t.Fatalf("Remove(old.go) error = %v", err)
	}

	origRepoChange := latestRelevantRepoChange
	origExeChange := currentExecutableChange
	t.Cleanup(func() {
		latestRelevantRepoChange = origRepoChange
		currentExecutableChange = origExeChange
	})
	latestRelevantRepoChange = func(string) (time.Time, error) { return time.Time{}, nil }
	currentExecutableChange = func() (time.Time, error) { return time.Time{}, nil }

	builder := NewBuilder(Config{
		Detect:       func(string) ([]string, error) { return []string{mainFile}, nil },
		Scanner:      &fakeScanner{nodes: []types.Node{{ID: "should-not-run", Label: "should-not-run", NodeType: "function"}}},
		GraphBuilder: igraph.Build,
		Persist:      export.WriteJSONAtomic,
		OutDir:       outDir,
	})

	result, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Graph.Nodes) != 2 {
		t.Fatalf("result graph nodes = %d, want 2 after deleted-file prune", len(result.Graph.Nodes))
	}
	for _, node := range result.Graph.Nodes {
		if node.SourceFile == "old.go" || strings.Contains(node.ID, "old.go") {
			t.Fatalf("deleted-file node retained in result: %+v", node)
		}
	}
	if len(result.Graph.Edges) != 0 {
		t.Fatalf("result graph edges = %d, want 0 after prune", len(result.Graph.Edges))
	}
	reloaded, err := export.LoadJSON(filepath.Join(outDir, "graph.json"))
	if err != nil {
		t.Fatalf("LoadJSON() error = %v", err)
	}
	if len(reloaded.Nodes) != 2 {
		t.Fatalf("reloaded graph nodes = %d, want 2 after persisted prune", len(reloaded.Nodes))
	}
	reloadedManifest, err := export.LoadManifest(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if len(reloadedManifest.Files) != 1 || reloadedManifest.Files[0].Path != "main.go" {
		t.Fatalf("persisted manifest files = %+v", reloadedManifest.Files)
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
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package main\n")
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
	writeTestFile(t, filepath.Join(repoRoot, "index.ts"), "export const value = 1;\n")
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
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package main\n")
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

func TestBuilderBuild_WarnsAndContinuesWhenClusteringFails(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, filepath.Join(repoRoot, "main.go"), "package main\n")
	clusterer := &fakeClusterer{err: igraph.ErrGraspologicMissing}
	builder := NewBuilder(Config{
		Detect:       func(string) ([]string, error) { return []string{filepath.Join(repoRoot, "main.go")}, nil },
		Scanner:      &fakeScanner{nodes: []types.Node{{ID: "svc", Label: "svc", NodeType: "function"}}},
		GraphBuilder: igraph.Build,
		Cluster:      clusterer.Run,
		Persist:      func(*types.Graph, string) error { return nil },
	})

	result, err := builder.Build(context.Background(), types.BuildRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Build() error = %v, want warning-only degrade", err)
	}
	if clusterer.called != 1 {
		t.Fatalf("clusterer called = %d, want 1", clusterer.called)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1", len(result.Warnings))
	}
	if got := result.Warnings[0]; !strings.Contains(got, "Community detection unavailable") || !strings.Contains(got, "graspologic") {
		t.Fatalf("warning = %q", got)
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
