package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/query"
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

func TestNewServerRegistersOnlyQueryTools(t *testing.T) {
	srv := NewServer(newTestEngine(t))
	tools := srv.ListTools()
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}
	want := map[string]bool{
		"vela_dependencies":         true,
		"vela_reverse_dependencies": true,
		"vela_impact":               true,
		"vela_path":                 true,
		"vela_explain":              true,
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
