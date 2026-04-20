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
			{"id": "workspace:repo:auth-api", "label": "auth-api", "kind": "repo", "file": "workspace:repo:auth-api", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"id": "workspace:service:auth", "label": "auth", "kind": "service", "file": "workspace:service:auth", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"id": "memory:observation:7", "label": "Auth note", "kind": "observation", "file": "ancora:obs:7"},
			{"id": "memory:observation:8", "label": "Config note", "kind": "observation", "file": "ancora:obs:8"},
		},
		"edges": []map[string]interface{}{
			{"from": "workspace:repo:auth-api", "to": "workspace:service:auth", "kind": "exposes", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"from": "auth", "to": "db", "kind": "uses", "confidence": "DECLARED", "metadata": map[string]interface{}{"evidence_type": "openapi", "evidence_confidence": "declared", "evidence_source_artifact": "openapi.yaml"}},
			{"from": "auth", "to": "user", "kind": "uses"},
			{"from": "user", "to": "db", "kind": "uses"},
			{"from": "memory:observation:7", "to": "auth", "kind": "documents", "metadata": map[string]interface{}{"layer": "memory", "evidence_type": "observation-reference", "evidence_confidence": "declared", "verification": "redirected", "reference_target": "repo:file:internal/legacy/auth.go", "bound_target": "auth", "binding_state": "redirected", "binding_evidence": "unique basename match"}},
			{"from": "memory:observation:8", "to": "repo:file:legacy/config.go", "kind": "constrains", "metadata": map[string]interface{}{"layer": "memory", "evidence_type": "observation-reference", "evidence_confidence": "declared", "verification": "ambiguous", "reference_target": "repo:file:legacy/config.go", "binding_state": "ambiguous", "binding_evidence": "multiple live files share the historical basename", "binding_suggestions": []string{"auth", "db"}}},
		},
		"meta": map[string]interface{}{"nodeCount": 7, "edgeCount": 6},
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
	if len(eng.graph.Nodes) != 7 {
		t.Errorf("expected 7 nodes, got %d", len(eng.graph.Nodes))
	}
	if len(eng.graph.Edges) != 6 {
		t.Errorf("expected 6 edges, got %d", len(eng.graph.Edges))
	}
	declaredFound := false
	for _, edge := range eng.graph.Edges {
		if edge.Confidence != "DECLARED" {
			continue
		}
		if got, _ := edge.Metadata["evidence_type"].(string); got == "openapi" {
			declaredFound = true
			break
		}
	}
	if !declaredFound {
		t.Fatal("expected declared openapi edge in graph")
	}
}

func TestExplain_ResolvesCanonicalIDAndShowsEvidence(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Explain("repo:struct:auth")
	if !strings.Contains(result, "AuthService [repo/struct]") {
		t.Fatalf("expected layer-aware node description in explain output, got: %q", result)
	}
	if !strings.Contains(result, "type=openapi") {
		t.Fatalf("expected evidence type in explain output, got: %q", result)
	}
	if !strings.Contains(result, "confidence=declared") {
		t.Fatalf("expected evidence confidence in explain output, got: %q", result)
	}
}

func TestExplain_ShowsBindingMetadata(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Explain("Auth note")
	if !strings.Contains(result, "Auth note [memory/observation]") {
		t.Fatalf("expected node layer/type label in explain output, got: %q", result)
	}
	if !strings.Contains(result, "reference=repo:file:internal/legacy/auth.go") {
		t.Fatalf("expected original reference target in explain output, got: %q", result)
	}
	if !strings.Contains(result, "binding=unique basename match") {
		t.Fatalf("expected binder evidence in explain output, got: %q", result)
	}
}

func TestExplain_ResolvesIncomingLabelTargetsAgainstLayer(t *testing.T) {
	dir := t.TempDir()
	g := map[string]interface{}{
		"nodes": []map[string]interface{}{
			{"id": "workspace:repo:billing-api", "label": "billing-api", "kind": "repo", "file": "workspace:repo:billing-api", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"id": "workspace:service:billing", "label": "billing", "kind": "service", "file": "workspace:service:billing", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
			{"id": "contract:service:billing", "label": "billing", "kind": "service", "file": "openapi.yaml", "metadata": map[string]interface{}{"layer": "contract", "evidence_type": "openapi", "evidence_confidence": "declared"}},
		},
		"edges": []map[string]interface{}{
			{"from": "workspace:repo:billing-api", "to": "billing", "kind": "exposes", "metadata": map[string]interface{}{"layer": "workspace", "evidence_type": "routing", "evidence_confidence": "extracted"}},
		},
	}
	data, _ := json.MarshalIndent(g, "", "  ")
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	eng, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	result := eng.Explain("workspace:service:billing")
	if !strings.Contains(result, "billing-api [workspace/repo] --[exposes]--> billing [workspace/service]") {
		t.Fatalf("expected workspace-layer target resolution, got: %q", result)
	}
	if strings.Contains(result, "contract/service") {
		t.Fatalf("expected explain to avoid resolving workspace edge to contract node, got: %q", result)
	}
}

func TestBindings_ReturnsBinderState(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Bindings("Config note")
	if !strings.Contains(result, "[ambiguous]") {
		t.Fatalf("expected ambiguous state in bindings output, got: %q", result)
	}
	if !strings.Contains(result, "suggestions=auth,db") {
		t.Fatalf("expected suggestions in bindings output, got: %q", result)
	}
}

func TestRoute_ReturnsWorkspaceRouting(t *testing.T) {
	dir := t.TempDir()
	path := writeTestGraph(t, dir)
	eng, _ := LoadFromFile(path)

	result := eng.Route("auth")
	if !strings.Contains(result, "score=") {
		t.Fatalf("expected scored route output, got: %q", result)
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
	if !strings.Contains(result, "[repo/struct]") {
		t.Errorf("expected layer-aware path output, got: %q", result)
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
		{"nodes", "7"},
		{"edges", "6"},
		{"help", "path"},
		{"path AuthService Database", "→"},
		{"explain AuthService", "AuthService"},
		{"bindings Config note", "ambiguous"},
		{"route auth", "score="},
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
