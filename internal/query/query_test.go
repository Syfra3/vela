package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestGraph(t *testing.T, dir string) string {
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
		"meta": map[string]interface{}{"nodeCount": 3, "edgeCount": 3},
	}
	data, _ := json.MarshalIndent(g, "", "  ")
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)

	eng, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}
	if len(eng.graph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(eng.graph.Nodes))
	}
	if len(eng.graph.Edges) != 3 {
		t.Errorf("expected 3 edges, got %d", len(eng.graph.Edges))
	}
}

func TestPath_DirectEdge(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Path("AuthService", "Database")
	if !strings.Contains(result, "AuthService") {
		t.Errorf("expected path containing AuthService, got: %q", result)
	}
	if !strings.Contains(result, "Database") {
		t.Errorf("expected path containing Database, got: %q", result)
	}
}

func TestPath_NoPath(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	// Database has no outgoing edges → no path to AuthService
	result := eng.Path("Database", "AuthService")
	if !strings.Contains(result, "no path") {
		t.Errorf("expected 'no path' message, got: %q", result)
	}
}

func TestPath_NodeNotFound(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Path("NonExistent", "Database")
	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found' message, got: %q", result)
	}
}

func TestExplain(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Explain("AuthService")
	if !strings.Contains(result, "AuthService") {
		t.Errorf("expected AuthService in explain result, got: %q", result)
	}
	// Should list at least the two outgoing edges
	if !strings.Contains(result, "uses") {
		t.Errorf("expected 'uses' relation in explain result, got: %q", result)
	}
}

func TestExplain_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Explain("Ghost")
	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found', got: %q", result)
	}
}

func TestQuery_Dispatcher(t *testing.T) {
	dir := t.TempDir()
	gpath := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(gpath)

	cases := []struct {
		input  string
		wantIn string
	}{
		{"nodes", "3"},
		{"edges", "3"},
		{"help", "path"},
		{"path AuthService Database", "→"},
		{"explain AuthService", "AuthService"},
		{"unknown cmd", "unknown command"},
	}

	for _, tc := range cases {
		result := eng.Query(tc.input)
		if !strings.Contains(result, tc.wantIn) {
			t.Errorf("query(%q): expected %q in result, got: %q", tc.input, tc.wantIn, result)
		}
	}
}

func TestFindNode_FuzzyLabel(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	node, ok := eng.FindNode("auth")
	if !ok {
		t.Fatal("expected fuzzy node match")
	}
	if node.Label != "AuthService" {
		t.Fatalf("expected AuthService, got %q", node.Label)
	}
}

func TestNeighbors(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	neighbors, err := eng.Neighbors("AuthService")
	if err != nil {
		t.Fatalf("Neighbors error: %v", err)
	}
	if len(neighbors) != 2 {
		t.Fatalf("expected 2 neighbors, got %d", len(neighbors))
	}
}

func TestStats(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	stats := eng.Stats()
	if stats.NodeCount != 3 {
		t.Fatalf("expected 3 nodes, got %d", stats.NodeCount)
	}
	if stats.EdgeCount != 3 {
		t.Fatalf("expected 3 edges, got %d", stats.EdgeCount)
	}
	if stats.NodeTypes["struct"] != 3 {
		t.Fatalf("expected 3 struct nodes, got %d", stats.NodeTypes["struct"])
	}
}
