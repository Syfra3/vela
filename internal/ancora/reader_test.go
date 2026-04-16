package ancora_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/ancora"
	_ "modernc.org/sqlite"
)

// seedDB creates a minimal ancora.db with the observations table and inserts
// test rows. Returns the path to the temp database.
func seedDB(t *testing.T, rows []map[string]any) string {
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

func TestReaderOpen_NotFound(t *testing.T) {
	_, err := ancora.Open("/tmp/nonexistent-vela-test-abc/ancora.db")
	if err == nil {
		t.Fatal("expected error for missing db, got nil")
	}
}

func TestReaderCount(t *testing.T) {
	t.Parallel()
	dbPath := seedDB(t, []map[string]any{
		{"title": "obs1", "content": "c1", "type": "decision", "workspace": "vela", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
		{"title": "obs2", "content": "c2", "type": "bugfix", "workspace": "ancora", "visibility": "personal", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
		// Soft-deleted — should NOT be counted.
		{"title": "gone", "content": "x", "type": "manual", "workspace": "vela", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": "2026-01-01T00:00:00Z"},
	})

	r, err := ancora.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	n, err := r.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Errorf("Count = %d, want 2 (deleted row must be excluded)", n)
	}
}

func TestReaderAllObservations_NoFilter(t *testing.T) {
	t.Parallel()
	dbPath := seedDB(t, []map[string]any{
		{"title": "A", "content": "ca", "type": "decision", "workspace": "vela", "visibility": "work", "organization": "glim", "topic_key": "arch/auth", "references": nil, "deleted_at": nil},
		{"title": "B", "content": "cb", "type": "bugfix", "workspace": "ancora", "visibility": "personal", "organization": nil, "topic_key": nil, "references": `[{"type":"file","target":"store.go"}]`, "deleted_at": nil},
	})

	r, _ := ancora.Open(dbPath)
	defer r.Close()

	obs, err := r.AllObservations("", "")
	if err != nil {
		t.Fatalf("AllObservations: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("got %d observations, want 2", len(obs))
	}

	// Verify field mapping for first row (A).
	a := obs[0]
	if a.Title != "A" {
		t.Errorf("Title = %q, want A", a.Title)
	}
	if a.Workspace != "vela" {
		t.Errorf("Workspace = %q, want vela", a.Workspace)
	}
	if a.Organization != "glim" {
		t.Errorf("Organization = %q, want glim", a.Organization)
	}
	if a.TopicKey != "arch/auth" {
		t.Errorf("TopicKey = %q, want arch/auth", a.TopicKey)
	}
	if a.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero — time parsing failed")
	}

	// Second row has references JSON.
	b := obs[1]
	if b.References == "" {
		t.Error("References is empty, want JSON string")
	}
}

func TestReaderAllObservations_WorkspaceFilter(t *testing.T) {
	t.Parallel()
	dbPath := seedDB(t, []map[string]any{
		{"title": "vela-obs", "content": "", "type": "manual", "workspace": "vela", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
		{"title": "ancora-obs", "content": "", "type": "manual", "workspace": "ancora", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
	})

	r, _ := ancora.Open(dbPath)
	defer r.Close()

	obs, err := r.AllObservations("vela", "")
	if err != nil {
		t.Fatalf("AllObservations(vela): %v", err)
	}
	if len(obs) != 1 || obs[0].Title != "vela-obs" {
		t.Errorf("got %v, want [vela-obs]", titlesOf(obs))
	}
}

func TestReaderAllObservations_VisibilityFilter(t *testing.T) {
	t.Parallel()
	dbPath := seedDB(t, []map[string]any{
		{"title": "work-obs", "content": "", "type": "manual", "workspace": "vela", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
		{"title": "personal-obs", "content": "", "type": "manual", "workspace": "vela", "visibility": "personal", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
	})

	r, _ := ancora.Open(dbPath)
	defer r.Close()

	obs, err := r.AllObservations("", "personal")
	if err != nil {
		t.Fatalf("AllObservations(personal): %v", err)
	}
	if len(obs) != 1 || obs[0].Title != "personal-obs" {
		t.Errorf("got %v, want [personal-obs]", titlesOf(obs))
	}
}

func TestReaderAllObservations_ExcludeDeleted(t *testing.T) {
	t.Parallel()
	dbPath := seedDB(t, []map[string]any{
		{"title": "alive", "content": "", "type": "manual", "workspace": "x", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": nil},
		{"title": "dead", "content": "", "type": "manual", "workspace": "x", "visibility": "work", "organization": nil, "topic_key": nil, "references": nil, "deleted_at": "2026-01-01T00:00:00Z"},
	})

	r, _ := ancora.Open(dbPath)
	defer r.Close()

	obs, _ := r.AllObservations("", "")
	if len(obs) != 1 || obs[0].Title != "alive" {
		t.Errorf("got %v, want [alive]", titlesOf(obs))
	}
}

func TestDefaultDBPath_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ANCORA_DATA_DIR", dir)

	path, err := ancora.DefaultDBPath()
	if err != nil {
		t.Fatalf("DefaultDBPath: %v", err)
	}
	if path != filepath.Join(dir, "ancora.db") {
		t.Errorf("path = %q, want %q", path, filepath.Join(dir, "ancora.db"))
	}
}

func TestDefaultDBPath_HomeDir(t *testing.T) {
	// Ensure env var is not set.
	t.Setenv("ANCORA_DATA_DIR", "")

	home, _ := os.UserHomeDir()
	path, err := ancora.DefaultDBPath()
	if err != nil {
		t.Fatalf("DefaultDBPath: %v", err)
	}
	expected := filepath.Join(home, ".ancora", "ancora.db")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}

func titlesOf(obs []ancora.Observation) []string {
	titles := make([]string, len(obs))
	for i, o := range obs {
		titles[i] = o.Title
	}
	return titles
}
