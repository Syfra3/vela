package query

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func seedSearchAncoraDB(t *testing.T, rows []map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ancora.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
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
		t.Fatalf("create observations: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, row := range rows {
		_, err = db.Exec(`
			INSERT INTO observations (title, content, type, workspace, visibility, organization, topic_key, "references", created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row["title"], row["content"], row["type"], row["workspace"], row["visibility"], row["organization"], row["topic_key"], row["references"], now, now, row["deleted_at"],
		)
		if err != nil {
			t.Fatalf("insert observation: %v", err)
		}
	}

	return dbPath
}

func buildSearchEngine(t *testing.T) *Engine {
	t.Helper()
	g := map[string]any{
		"nodes": []map[string]any{
			{
				"id":          "ancora:obs:1",
				"label":       "Federated retriever architecture",
				"kind":        "observation",
				"file":        "ancora:obs:1",
				"description": "Ancora is the source of truth while Vela projects and ranks graph-aware results.",
				"metadata": map[string]any{
					"workspace": "vela",
					"obs_type":  "architecture",
				},
			},
			{
				"id":          "code:retriever",
				"label":       "FederatedRetriever",
				"kind":        "struct",
				"file":        "internal/query/search.go",
				"description": "Combines graph and memory retrieval into one ranked result set.",
				"metadata": map[string]any{
					"workspace": "vela",
				},
			},
		},
		"edges": []map[string]any{},
		"meta":  map[string]any{"generatedAt": time.Now().UTC().Format(time.RFC3339)},
	}
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write graph: %v", err)
	}
	eng, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}
	return eng
}

func TestSearcherSearch_FederatesAndMeasures(t *testing.T) {
	eng := buildSearchEngine(t)
	dbPath := seedSearchAncoraDB(t, []map[string]any{
		{
			"title":        "Federated retriever architecture",
			"content":      "Ancora feeds memory context and Vela fuses it with graph nodes.",
			"type":         "architecture",
			"workspace":    "vela",
			"visibility":   "work",
			"organization": "syfra",
			"topic_key":    "vela/federated-retriever",
			"references":   nil,
			"deleted_at":   nil,
		},
	})

	searcher := NewSearcher(eng, dbPath)
	resp, err := searcher.Search("federated retriever", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(resp.Hits) < 2 {
		t.Fatalf("len(resp.Hits) = %d, want at least 2", len(resp.Hits))
	}
	if resp.Metrics.AncoraOnly.Returned == 0 {
		t.Fatalf("AncoraOnly.Returned = 0, want baseline hits")
	}
	if resp.Metrics.Comparison.AddedByFederated == 0 {
		t.Fatalf("AddedByFederated = 0, want graph-only expansion")
	}
	if resp.Metrics.Comparison.AddedBySource[searchSourceGraph] == 0 {
		t.Fatalf("AddedBySource[%q] = 0, want graph contribution", searchSourceGraph)
	}
	if resp.Metrics.Federated.SourceContribution[searchSourceGraph] == 0 {
		t.Fatalf("Federated.SourceContribution missing graph results: %#v", resp.Metrics.Federated.SourceContribution)
	}
	if resp.Hits[0].PrimarySource == "" {
		t.Fatal("top hit missing primary source")
	}
}

func TestSearcherSearch_SpanishPluralMatchesSingularAncora(t *testing.T) {
	dbPath := seedSearchAncoraDB(t, []map[string]any{
		{
			"title":        "Gasto D1 - 15 de abril",
			"content":      "Registro de gasto de comida en D1.",
			"type":         "manual",
			"workspace":    "personal-finance",
			"visibility":   "personal",
			"organization": "syfra",
			"topic_key":    "finanzas/gasto-d1-2026-04-15",
			"references":   nil,
			"deleted_at":   nil,
		},
	})

	searcher := NewSearcher(nil, dbPath)
	resp, err := searcher.Search("Gastos", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if resp.Metrics.AncoraOnly.Returned == 0 {
		t.Fatal("AncoraOnly.Returned = 0, want singular gasto match for plural query")
	}
	if len(resp.Hits) == 0 {
		t.Fatal("len(resp.Hits) = 0, want matching ancora hit")
	}
	if resp.Hits[0].PrimarySource != searchSourceAncora {
		t.Fatalf("PrimarySource = %q, want %q", resp.Hits[0].PrimarySource, searchSourceAncora)
	}
	if resp.Hits[0].Label != "Gasto D1 - 15 de abril" {
		t.Fatalf("Label = %q, want singular gasto observation", resp.Hits[0].Label)
	}
}

func TestWriteMetricsSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := WriteMetricsSnapshot(SearchMetrics{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Query:       "federated retriever",
		Limit:       5,
	})
	if err != nil {
		t.Fatalf("WriteMetricsSnapshot() error = %v", err)
	}
	if filepath.Dir(path) != filepath.Join(home, ".vela", "retrieval-history") {
		t.Fatalf("snapshot dir = %q", filepath.Dir(path))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("os.Stat(%q) error = %v", path, err)
	}
}
