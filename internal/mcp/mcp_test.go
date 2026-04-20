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
	return queryTestGraph(t, dir)
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

func TestNewServerRegistersApprovedTools(t *testing.T) {
	srv := NewServer(newTestEngine(t))
	tools := srv.ListTools()
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools))
	}
	want := map[string]bool{
		"vela_query_graph":      true,
		"vela_shortest_path":    true,
		"vela_get_node":         true,
		"vela_get_neighbors":    true,
		"vela_graph_stats":      true,
		"vela_explain_graph":    true,
		"vela_federated_search": true,
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

func TestHandleGetNode(t *testing.T) {
	h := handleGetNode(newTestEngine(t))
	res, err := h(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"label": "AuthService"}}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := callResultText(t, res)
	if !strings.Contains(text, "AuthService") {
		t.Fatalf("expected AuthService in result, got %q", text)
	}
}

func TestHandleShortestPath(t *testing.T) {
	h := handleShortestPath(newTestEngine(t))
	res, err := h(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"source": "AuthService", "target": "Database"}}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := callResultText(t, res)
	if !strings.Contains(text, "AuthService") || !strings.Contains(text, "Database") {
		t.Fatalf("expected path output, got %q", text)
	}
}

func TestHandleGraphStats(t *testing.T) {
	h := handleGraphStats(newTestEngine(t))
	res, err := h(context.Background(), mcppkg.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := callResultText(t, res)
	if !strings.Contains(text, "Nodes: 3") || !strings.Contains(text, "Edges: 3") {
		t.Fatalf("expected stats output, got %q", text)
	}
}

func queryTestGraph(t *testing.T, dir string) string {
	t.Helper()
	g := map[string]interface{}{
		"nodes": []map[string]interface{}{
			{"id": "auth", "label": "AuthService", "kind": "struct", "file": "auth.go"},
			{"id": "db", "label": "Database", "kind": "struct", "file": "db.go"},
			{"id": "user", "label": "UserRepo", "kind": "struct", "file": "user.go"},
		},
		"edges": []map[string]interface{}{
			{"from": "auth", "to": "db", "kind": "uses"},
			{"from": "auth", "to": "user", "kind": "uses"},
			{"from": "user", "to": "db", "kind": "uses"},
		},
	}
	data, _ := json.MarshalIndent(g, "", "  ")
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
