package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/retrieval"
	_ "modernc.org/sqlite"
)

func withServerStubEmbeddings(t *testing.T) {
	t.Helper()
	restore := retrieval.SetEmbedTextsForTesting(func(texts []string) ([][]float32, error) {
		out := make([][]float32, 0, len(texts))
		for _, text := range texts {
			vector := []float32{0, 0}
			lower := strings.ToLower(text)
			if strings.Contains(lower, "auth") {
				vector[0] = 1
			}
			if strings.Contains(lower, "federated") || strings.Contains(lower, "retriev") {
				vector[1] = 1
			}
			if vector[0] == 0 && vector[1] == 0 {
				vector[0] = 0.1
			}
			out = append(out, vector)
		}
		return out, nil
	})
	t.Cleanup(restore)
}

func seedServerAncoraDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ancora.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		type TEXT,
		workspace TEXT,
		visibility TEXT NOT NULL DEFAULT 'work',
		organization TEXT,
		topic_key TEXT,
		"references" TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deleted_at TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO observations (title, content, type, workspace, visibility, organization, topic_key, "references", created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "Federated retriever architecture", "Ancora feeds memory context and Vela fuses it with graph nodes.", "architecture", "vela", "work", "syfra", "vela/federated-retriever", nil, now, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	return dbPath
}

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
	s := New(buildTestEngine(t), seedServerAncoraDB(t), 7700)
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
	s := New(buildTestEngine(t), seedServerAncoraDB(t), 7700)
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
	s := New(buildTestEngine(t), seedServerAncoraDB(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/auth", nil)
	req.URL.Path = "/node/auth"
	s.handleNode(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleNode_NotFound(t *testing.T) {
	s := New(buildTestEngine(t), seedServerAncoraDB(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/ghost", nil)
	req.URL.Path = "/node/ghost"
	s.handleNode(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandlePath(t *testing.T) {
	s := New(buildTestEngine(t), seedServerAncoraDB(t), 7700)
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
	s := New(buildTestEngine(t), seedServerAncoraDB(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/path", nil)
	s.handlePath(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleSearch(t *testing.T) {
	withServerStubEmbeddings(t)
	s := New(buildTestEngine(t), seedServerAncoraDB(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search?q=federated%20retriever&limit=5", nil)
	s.handleSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp query.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Hits) == 0 {
		t.Fatal("expected search hits")
	}
}

func TestHandleSearchProfileLexical(t *testing.T) {
	withServerStubEmbeddings(t)
	s := New(buildTestEngine(t), seedServerAncoraDB(t), 7700)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search?q=auth&profile=lexical", nil)
	s.handleSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var run query.SearchRun
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	if run.Profile != query.SearchProfileLexical {
		t.Fatalf("profile = %q, want lexical", run.Profile)
	}
}
