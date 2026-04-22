package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/internal/query"
)

func buildTestEngine(t *testing.T) *query.Engine {
	t.Helper()
	g := map[string]interface{}{
		"nodes": []map[string]interface{}{
			{"id": "auth", "label": "AuthService", "kind": "struct", "file": "auth.go"},
			{"id": "db", "label": "Database", "kind": "struct", "file": "db.go"},
		},
		"edges": []map[string]interface{}{{"from": "auth", "to": "db", "kind": "uses"}},
	}
	data, _ := json.MarshalIndent(g, "", "  ")
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	eng, err := query.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return eng
}

func TestHandleHealth(t *testing.T) {
	s := New(buildTestEngine(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	s.handleHealth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleGraph(t *testing.T) {
	s := New(buildTestEngine(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/graph", nil)
	s.handleGraph(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleQueryRunsGraphTruthRequest(t *testing.T) {
	s := New(buildTestEngine(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/query?kind=dependencies&subject=AuthService&limit=5", nil)
	s.handleQuery(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal query response: %v", err)
	}
	output, _ := payload["output"].(string)
	if !strings.Contains(output, "Dependencies for \"AuthService\"") {
		t.Fatalf("unexpected query response %q", output)
	}
}

func TestHandleQueryRejectsUnsupportedLegacySearchEndpoint(t *testing.T) {
	s := New(buildTestEngine(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search?q=auth", nil)
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for removed search endpoint, got %d", rec.Code)
	}
}
