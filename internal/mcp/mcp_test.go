package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	graphExport "github.com/Syfra3/vela/internal/export"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/pkg/types"
	mcppkg "github.com/mark3labs/mcp-go/mcp"
)

func newTestEngine(t *testing.T) *query.Engine {
	t.Helper()
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, err := query.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	return eng
}

func writeTestGraph(t *testing.T, dir string) string {
	t.Helper()
	g := map[string]interface{}{
		"nodes": []map[string]interface{}{
			{"id": "auth", "label": "AuthService", "kind": "struct", "file": "auth.go"},
			{"id": "db", "label": "Database", "kind": "struct", "file": "db.go"},
			{"id": "user", "label": "UserRepo", "kind": "struct", "file": "user.go"},
		},
		"edges": []map[string]interface{}{{"from": "auth", "to": "db", "kind": "uses"}, {"from": "user", "to": "db", "kind": "uses"}},
	}
	data, _ := json.MarshalIndent(g, "", "  ")
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func callResultText(t *testing.T, res *mcppkg.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("expected non-empty result")
	}
	text, ok := mcppkg.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("expected text content")
	}
	return text.Text
}

func TestNewServerRegistersQueryToolSurface(t *testing.T) {
	srv := NewServer(newTestEngine(t))
	tools := srv.ListTools()
	if len(tools) != 8 {
		t.Fatalf("expected 8 tools, got %d", len(tools))
	}
	want := map[string]bool{
		"vela_explore":              true,
		"vela_lookup":               true,
		"vela_dependencies":         true,
		"vela_reverse_dependencies": true,
		"vela_impact":               true,
		"vela_path":                 true,
		"vela_explain":              true,
		"vela_status":               true,
	}
	for _, tool := range tools {
		if !want[tool.Tool.Name] {
			t.Fatalf("unexpected tool registered: %s", tool.Tool.Name)
		}
		delete(want, tool.Tool.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing tools: %v", want)
	}
}

// REQ-006 → SCN-008 → TestSCN008_MCPServerExposesRequiredV04Toolset
func TestSCN008_MCPServerExposesRequiredV04Toolset(t *testing.T) {
	// Scenario: MCP server exposes the required v0.4 toolset.
	srv := NewServer(newTestEngine(t))
	tools := srv.ListTools()

	want := map[string]bool{
		"vela_explore": true,
		"vela_lookup":  true,
		"vela_explain": true,
		"vela_impact":  true,
		"vela_path":    true,
		"vela_status":  true,
	}
	for _, tool := range tools {
		delete(want, tool.Tool.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing v0.4 MCP tools: %v", want)
	}
}

// REQ-006 → SCN-009 → TestSCN009_CompatibleMCPClientsReceiveStructuredCoreResponses
func TestSCN009_CompatibleMCPClientsReceiveStructuredCoreResponses(t *testing.T) {
	// Scenario: OpenCode and Claude-compatible MCP clients can call Vela tools.
	eng := newTestEngine(t)
	toolHandlers := map[string]serverCall{
		"vela_explore": {handler: handleExploreTool(eng), arguments: map[string]any{"query": "AuthService", "limit": 5}, wantKind: "explore"},
		"vela_lookup":  {handler: handleLookupTool(eng), arguments: map[string]any{"term": "AuthService", "limit": 5}, wantKind: "lookup"},
		"vela_explain": {handler: handleQueryTool(eng, "explain"), arguments: map[string]any{"subject": "AuthService", "limit": 5}, wantKind: "explain"},
		"vela_impact":  {handler: handleQueryTool(eng, "impact"), arguments: map[string]any{"subject": "Database", "limit": 5}, wantKind: "impact"},
		"vela_path":    {handler: handleQueryTool(eng, "path"), arguments: map[string]any{"subject": "AuthService", "target": "Database", "limit": 5}, wantKind: "path"},
		"vela_status":  {handler: handleStatusTool(eng), arguments: map[string]any{}, wantKind: "status"},
	}
	clientCalls := []struct {
		name string
	}{
		{name: "opencode-compatible"},
		{name: "claude-compatible"},
	}

	for _, client := range clientCalls {
		for toolName, tool := range toolHandlers {
			t.Run(client.name+"/"+toolName, func(t *testing.T) {
				res, err := tool.handler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: tool.arguments}})
				if err != nil {
					t.Fatalf("handler error: %v", err)
				}
				core := requireStructuredCoreResult(t, res)
				if core.SchemaVersion == "" || core.QueryKind != tool.wantKind || core.Status == "" {
					t.Fatalf("core result header = (%q, %q, %q), want schema version, %s, status", core.SchemaVersion, core.QueryKind, core.Status, tool.wantKind)
				}
				if core.Freshness.Status == "" {
					t.Fatalf("core result missing freshness required for agents: %+v", core)
				}
				if core.Status == query.ResultStatusOK && tool.wantKind != "status" && len(core.ResolvedSubjects) == 0 && len(core.Facts) == 0 {
					t.Fatalf("successful core result has no structured subjects or facts for agents: %+v", core)
				}
			})
		}
	}

	t.Run("structured validation diagnostic", func(t *testing.T) {
		res, err := handleQueryTool(eng, "path")(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService"}}})
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		core := requireStructuredCoreResult(t, res)
		if core.Status != query.ResultStatusError || !diagnosticContains(core.Diagnostics, "VALIDATION_ERROR", "target") {
			t.Fatalf("validation result = %+v, want structured target diagnostic", core)
		}
	})
}

type serverCall struct {
	handler   func(context.Context, mcppkg.CallToolRequest) (*mcppkg.CallToolResult, error)
	arguments map[string]any
	wantKind  string
}

func requireStructuredCoreResult(t *testing.T, res *mcppkg.CallToolResult) query.Result {
	t.Helper()
	core, ok := res.StructuredContent.(query.Result)
	if !ok {
		t.Fatalf("structured content = %T, want query.Result", res.StructuredContent)
	}
	var fallback query.Result
	if err := json.Unmarshal([]byte(callResultText(t, res)), &fallback); err != nil {
		t.Fatalf("text fallback was not core result JSON: %v", err)
	}
	if fallback.SchemaVersion != core.SchemaVersion || fallback.QueryKind != core.QueryKind || fallback.Status != core.Status {
		t.Fatalf("text fallback header = (%q, %q, %q), structured header = (%q, %q, %q)", fallback.SchemaVersion, fallback.QueryKind, fallback.Status, core.SchemaVersion, core.QueryKind, core.Status)
	}
	return core
}

// REQ-007/REQ-013 → SCN-010 → TestSCN010_MCPStartupReportsStaleGraphState
func TestSCN010_MCPStartupReportsStaleGraphState(t *testing.T) {
	// Scenario: MCP startup reports stale graph state when safe update is unavailable.
	repoRoot := t.TempDir()
	sourcePath := filepath.Join(repoRoot, "main.go")
	if err := os.WriteFile(sourcePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(repoRoot, ".vela")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	graphJSON := filepath.Join(outDir, "graph.json")
	if err := os.WriteFile(graphJSON, []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := graphExport.WriteSQLiteGraphAtomic(&types.Graph{
		Nodes: []types.Node{{ID: "auth", Label: "AuthService", NodeType: "struct", SourceFile: "main.go"}},
		Edges: []types.Edge{{Source: "auth", Target: "auth", Relation: "self", Metadata: map[string]interface{}{"evidence_confidence": "extracted"}}},
	}, outDir); err != nil {
		t.Fatalf("WriteSQLiteGraphAtomic error: %v", err)
	}
	manifest := types.Manifest{
		Version:     1,
		RepoRoot:    repoRoot,
		GeneratedAt: time.Now().UTC(),
		Files:       []types.ManifestFile{{Path: "main.go", SHA256: sha256Hex([]byte("package main\n"))}},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), manifestData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("package main\n// stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	eng, err := query.LoadFromFile(graphJSON)
	if err != nil {
		t.Fatalf("LoadFromFile(%s) error = %v", graphJSON, err)
	}
	statusHandler := handleStatusTool(eng)
	statusRes, err := statusHandler(context.Background(), mcppkg.CallToolRequest{})
	if err != nil {
		t.Fatalf("status handler error: %v", err)
	}
	statusText := callResultText(t, statusRes)
	for _, want := range []string{"stale", "main.go", "vela update"} {
		if !strings.Contains(statusText, want) {
			t.Fatalf("expected MCP status to contain %q, got %q", want, statusText)
		}
	}

	explainHandler := handleQueryTool(eng, "explain")
	explainRes, err := explainHandler(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService"}}})
	if err != nil {
		t.Fatalf("explain handler error: %v", err)
	}
	core, ok := explainRes.StructuredContent.(query.Result)
	if !ok {
		t.Fatalf("structured content = %T, want query.Result", explainRes.StructuredContent)
	}
	if core.Freshness.Status != query.FreshnessStale {
		t.Fatalf("MCP explain freshness = %q, want stale", core.Freshness.Status)
	}
	if !diagnosticContains(core.Diagnostics, "STALE_GRAPH", "main.go") {
		t.Fatalf("MCP explain diagnostics = %+v, want stale graph warning naming main.go", core.Diagnostics)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func diagnosticContains(diagnostics []query.Diagnostic, code, text string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && strings.Contains(diagnostic.Message, text) {
			return true
		}
	}
	return false
}

func TestDependenciesToolRunsQueryRequest(t *testing.T) {
	h := handleQueryTool(newTestEngine(t), "dependencies")
	res, err := h(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService", "limit": 5}}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := callResultText(t, res)
	if !strings.Contains(text, "Dependencies for \"AuthService\"") {
		t.Fatalf("unexpected text %q", text)
	}
}

// REQ-004 → SCN-005 → TestSCN005_CLIAndMCPExplainShareCoreResultFields
func TestSCN005_CLIAndMCPExplainShareCoreResultFields(t *testing.T) {
	// Scenario: CLI and MCP return equivalent core results for the same explain query.
	eng := newTestEngine(t)
	expected := eng.ExplainResult("AuthService")
	cliText, err := eng.RunRequest(queryRequestExplain("AuthService"))
	if err != nil {
		t.Fatalf("RunRequest(explain) error = %v", err)
	}

	h := handleQueryTool(eng, "explain")
	res, err := h(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService", "limit": 5}}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var got query.Result
	if err := json.Unmarshal([]byte(callResultText(t, res)), &got); err != nil {
		t.Fatalf("MCP explain result was not shared core JSON: %v", err)
	}
	if got.SchemaVersion != expected.SchemaVersion || got.QueryKind != expected.QueryKind || got.Status != expected.Status {
		t.Fatalf("MCP core result header = (%q, %q, %q), want (%q, %q, %q)", got.SchemaVersion, got.QueryKind, got.Status, expected.SchemaVersion, expected.QueryKind, expected.Status)
	}
	if len(got.ResolvedSubjects) != len(expected.ResolvedSubjects) || got.ResolvedSubjects[0].ID != expected.ResolvedSubjects[0].ID {
		t.Fatalf("MCP resolved subjects = %+v, want %+v", got.ResolvedSubjects, expected.ResolvedSubjects)
	}
	if len(got.Facts) != len(expected.Facts) || got.Facts[0].Subject != expected.Facts[0].Subject || got.Facts[0].Predicate != expected.Facts[0].Predicate || got.Facts[0].Object != expected.Facts[0].Object {
		t.Fatalf("MCP facts = %+v, want %+v", got.Facts, expected.Facts)
	}
	for _, want := range []string{expected.ResolvedSubjects[0].Label, expected.Facts[0].Predicate, "Database"} {
		if !strings.Contains(cliText, want) {
			t.Fatalf("CLI explain output %q did not preserve core field %q", cliText, want)
		}
	}
}

func queryRequestExplain(subject string) types.QueryRequest {
	return types.QueryRequest{Kind: types.QueryKindExplain, Subject: subject, Limit: 5}
}

func TestPathToolRequiresTarget(t *testing.T) {
	h := handleQueryTool(newTestEngine(t), "path")
	res, err := h(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"subject": "AuthService"}}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := callResultText(t, res)
	if !strings.Contains(strings.ToLower(text), "target") {
		t.Fatalf("expected target validation error, got %q", text)
	}
}
