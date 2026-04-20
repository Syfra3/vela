package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/query"
	mcppkg "github.com/mark3labs/mcp-go/mcp"
	_ "modernc.org/sqlite"
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
	srv := NewServer(newTestEngine(t), nil)
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

func TestHandleFederatedSearchUsesAncoraSearcher(t *testing.T) {
	dbPath := seedAncoraDB(t, []map[string]any{{
		"title":        "Identity resolver decision",
		"content":      "Use canonical observation ids when fusing ancora and graph hits.",
		"type":         "decision",
		"workspace":    "vela",
		"visibility":   "work",
		"organization": "syfra",
		"topic_key":    "vela/identity-resolver",
		"references":   nil,
		"deleted_at":   nil,
	}})

	h := handleFederatedSearch(query.NewSearcher(newTestEngine(t), dbPath))
	res, err := h(context.Background(), mcppkg.CallToolRequest{Params: mcppkg.CallToolParams{Arguments: map[string]any{"query": "identity resolver", "limit": 5}}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := callResultText(t, res)
	if !strings.Contains(text, "Identity resolver decision") {
		t.Fatalf("expected Ancora-backed federated hit, got %q", text)
	}
	if !strings.Contains(text, "source=ancora") {
		t.Fatalf("expected federated hit to report ancora source, got %q", text)
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

func seedAncoraDB(t *testing.T, rows []map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ancora.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE observations (
		id         INTEGER PRIMARY KEY,
		sync_id    TEXT,
		session_id TEXT,
		type       TEXT,
		title      TEXT NOT NULL,
		content    TEXT NOT NULL DEFAULT '',
		tool_name  TEXT,
		workspace  TEXT,
		visibility TEXT NOT NULL DEFAULT 'work',
		organization TEXT,
		topic_key  TEXT,
		"references" TEXT,
		revision_count  INTEGER DEFAULT 0,
		duplicate_count INTEGER DEFAULT 0,
		last_seen_at    TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deleted_at TEXT
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, row := range rows {
		_, err = db.Exec(`INSERT INTO observations
			(title, content, type, workspace, visibility, organization, topic_key, "references", created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row["title"], row["content"], row["type"],
			row["workspace"], row["visibility"], row["organization"],
			row["topic_key"], row["references"],
			now, now, row["deleted_at"],
		)
		if err != nil {
			t.Fatalf("insert row: %v", err)
		}
	}

	return dbPath
}
