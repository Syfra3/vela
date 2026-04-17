package extract_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/extract"
	_ "modernc.org/sqlite"
)

// seedAncoraDB creates a minimal ancora.db with test observations.
func seedAncoraDB(t *testing.T, rows []map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ancora.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE observations (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		sync_id      TEXT,
		session_id   TEXT,
		type         TEXT,
		title        TEXT NOT NULL,
		content      TEXT NOT NULL DEFAULT '',
		tool_name    TEXT,
		workspace    TEXT,
		visibility   TEXT NOT NULL DEFAULT 'work',
		organization TEXT,
		topic_key    TEXT,
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
		_, err = db.Exec(`
			INSERT INTO observations
				(title, content, type, workspace, visibility, organization, topic_key, "references", created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row["title"], row["content"], row["type"],
			row["workspace"], row["visibility"], row["organization"],
			row["topic_key"], row["references"],
			now, now, row["deleted_at"],
		)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return dbPath
}

func TestExtractAncora_EmptyDB(t *testing.T) {
	dbPath := seedAncoraDB(t, nil)
	nodes, edges, err := extract.ExtractAncora(dbPath, nil, 8000, nil)
	if err != nil {
		t.Fatalf("ExtractAncora: %v", err)
	}
	if len(nodes) != 0 || len(edges) != 0 {
		t.Errorf("expected empty graph, got %d nodes %d edges", len(nodes), len(edges))
	}
}

func TestExtractAncora_HierarchyNodes(t *testing.T) {
	t.Parallel()
	dbPath := seedAncoraDB(t, []map[string]any{
		{
			"title": "Fix auth bug", "content": "root cause was X",
			"type": "bugfix", "workspace": "vela", "visibility": "work",
			"organization": "glim", "topic_key": nil, "references": nil, "deleted_at": nil,
		},
		{
			"title": "Decision: use JWT", "content": "chose JWT for statelessness",
			"type": "decision", "workspace": "ancora", "visibility": "personal",
			"organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil,
		},
	})

	nodes, edges, err := extract.ExtractAncora(dbPath, nil, 8000, nil)
	if err != nil {
		t.Fatalf("ExtractAncora: %v", err)
	}

	// Build ID index for assertions.
	nodeByID := make(map[string]bool)
	for _, n := range nodes {
		nodeByID[n.ID] = true
	}

	// Expect hierarchy nodes.
	for _, id := range []string{
		"ancora:workspace:vela",
		"ancora:workspace:ancora",
		"ancora:visibility:work",
		"ancora:visibility:personal",
		"ancora:org:glim",
	} {
		if !nodeByID[id] {
			t.Errorf("missing hierarchy node %q", id)
		}
	}

	// Expect observation nodes.
	obsCount := 0
	for _, n := range nodes {
		if n.NodeType == "observation" {
			obsCount++
		}
	}
	if obsCount != 2 {
		t.Errorf("observation nodes = %d, want 2", obsCount)
	}

	// Expect structural edges (obs → workspace, obs → visibility).
	edgeRelations := make(map[string]int)
	for _, e := range edges {
		edgeRelations[e.Relation]++
	}
	if edgeRelations["belongs_to"] == 0 {
		t.Error("no belongs_to edges")
	}
	if edgeRelations["scoped_to"] == 0 {
		t.Error("no scoped_to edges")
	}
	if edgeRelations["contains"] == 0 {
		t.Error("no contains edges (workspace→visibility)")
	}
}

func TestExtractAncora_ExplicitReferences(t *testing.T) {
	t.Parallel()
	dbPath := seedAncoraDB(t, []map[string]any{
		{
			"title": "Store bugfix", "content": "fixed N+1",
			"type": "bugfix", "workspace": "vela", "visibility": "work",
			"organization": nil, "topic_key": nil,
			"references": `[{"type":"file","target":"internal/store/store.go"},{"type":"observation","target":"ancora:obs:5"}]`,
			"deleted_at": nil,
		},
	})

	_, edges, err := extract.ExtractAncora(dbPath, nil, 8000, nil)
	if err != nil {
		t.Fatalf("ExtractAncora: %v", err)
	}

	relationsByTarget := make(map[string]string)
	for _, e := range edges {
		switch e.Target {
		case "internal/store/store.go", "ancora:obs:5":
			relationsByTarget[e.Target] = e.Relation
		}
	}
	if got := relationsByTarget["internal/store/store.go"]; got != "constrains" {
		t.Errorf("file reference relation = %q, want %q", got, "constrains")
	}
	if got := relationsByTarget["ancora:obs:5"]; got != "related_to" {
		t.Errorf("observation reference relation = %q, want %q", got, "related_to")
	}
}

func TestExtractAncora_ProgressCallback(t *testing.T) {
	t.Parallel()
	dbPath := seedAncoraDB(t, []map[string]any{
		{"title": "obs1", "content": "c", "type": "manual", "workspace": "w", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
		{"title": "obs2", "content": "c", "type": "manual", "workspace": "w", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
	})

	calls := 0
	progress := func(done, total int, current string) {
		calls++
	}

	_, _, err := extract.ExtractAncora(dbPath, nil, 8000, progress)
	if err != nil {
		t.Fatalf("ExtractAncora: %v", err)
	}
	if calls == 0 {
		t.Error("progress callback never called")
	}
}

func TestExtractAncora_DeletedObsExcluded(t *testing.T) {
	t.Parallel()
	dbPath := seedAncoraDB(t, []map[string]any{
		{"title": "alive", "content": "x", "type": "manual", "workspace": "w", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
		{"title": "deleted", "content": "x", "type": "manual", "workspace": "w", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": "2026-01-01T00:00:00Z"},
	})

	nodes, _, err := extract.ExtractAncora(dbPath, nil, 8000, nil)
	if err != nil {
		t.Fatalf("ExtractAncora: %v", err)
	}
	obsCount := 0
	for _, n := range nodes {
		if n.NodeType == "observation" {
			obsCount++
		}
	}
	if obsCount != 1 {
		t.Errorf("observation nodes = %d, want 1 (deleted excluded)", obsCount)
	}
}
