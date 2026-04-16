package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		"edges": []map[string]interface{}{
			{"from": "auth", "to": "Database", "kind": "uses"},
		},
		"meta": map[string]interface{}{"nodeCount": 2, "edgeCount": 1},
	}
	data, _ := json.MarshalIndent(g, "", "  ")
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
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
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", resp["status"])
	}
}

func TestHandleGraph(t *testing.T) {
	s := New(buildTestEngine(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/graph", nil)
	s.handleGraph(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("expected Content-Type: application/json")
	}
}

func TestHandleNode_Found(t *testing.T) {
	s := New(buildTestEngine(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/auth", nil)
	req.URL.Path = "/node/auth"
	s.handleNode(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleNode_NotFound(t *testing.T) {
	s := New(buildTestEngine(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/ghost", nil)
	req.URL.Path = "/node/ghost"
	s.handleNode(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandlePath(t *testing.T) {
	s := New(buildTestEngine(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/path?from=AuthService&to=Database", nil)
	s.handlePath(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp["path"]; !ok {
		t.Error("expected 'path' key in response")
	}
}

func TestHandlePath_MissingParams(t *testing.T) {
	s := New(buildTestEngine(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/path", nil)
	s.handlePath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}
