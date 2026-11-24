package server

import (
	"encoding/json"
	"net"
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

func TestListenUsesLoopbackAndActualPort(t *testing.T) {
	listener, err := New(nil, 0).listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !addr.IP.IsLoopback() || addr.Port == 0 {
		t.Fatalf("listener address = %v, want loopback with allocated port", listener.Addr())
	}
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

func TestStatusDistinguishesUnavailableFromHealth(t *testing.T) {
	s := New(query.NewUnavailableEngine(query.UnavailableDiagnostic{Message: "missing"}), 0)
	status := httptest.NewRecorder()
	s.routes().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/status", nil))
	if !strings.Contains(status.Body.String(), `"status":"unavailable"`) {
		t.Fatal(status.Body.String())
	}
	health := httptest.NewRecorder()
	s.routes().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if !strings.Contains(health.Body.String(), `"status":"ok"`) {
		t.Fatal(health.Body.String())
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
