// Package ancora provides a read-only snapshot reader for the Ancora SQLite
// database. It does NOT import the ancora binary or its store package — it
// speaks directly to the DB file so vela stays independent.
package ancora

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // register "sqlite" driver
)

// DefaultDBPath returns the path to the ancora SQLite database.
// Respects ANCORA_DATA_DIR env var, otherwise uses ~/.ancora/ancora.db.
func DefaultDBPath() (string, error) {
	if dir := os.Getenv("ANCORA_DATA_DIR"); dir != "" {
		return filepath.Join(dir, "ancora.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".ancora", "ancora.db"), nil
}

// Observation is a minimal projection of ancora's observations table.
// Only the fields vela needs for graph extraction are included.
type Observation struct {
	ID           int64
	Title        string
	Content      string
	Type         string // "decision", "bugfix", "architecture", etc.
	Workspace    string // project/repo name — may be empty
	Visibility   string // "work" | "personal"
	Organization string // optional org scope
	TopicKey     string // optional dedup key
	References   string // JSON array string — may be empty
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Reader opens a read-only view of the ancora SQLite database.
type Reader struct {
	db *sql.DB
}

// Open opens the ancora database at dbPath in read-only WAL mode.
// The caller must call Close() when done.
func Open(dbPath string) (*Reader, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("ancora db not found at %s: %w", dbPath, err)
	}
	// Open read-only: immutable=1 prevents any write-ahead log writes.
	dsn := fmt.Sprintf("file:%s?mode=ro&_journal=WAL&immutable=1", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open ancora db: %w", err)
	}
	db.SetMaxOpenConns(1)
	return &Reader{db: db}, nil
}

// Close releases the database connection.
func (r *Reader) Close() error {
	return r.db.Close()
}

// AllObservations returns all non-deleted observations.
// workspace and visibility are optional filters ("" = all).
func (r *Reader) AllObservations(workspace, visibility string) ([]Observation, error) {
	q := `
		SELECT
			id,
			title,
			content,
			type,
			COALESCE(workspace, '')    AS workspace,
			COALESCE(visibility, '')   AS visibility,
			COALESCE(organization, '') AS organization,
			COALESCE(topic_key, '')    AS topic_key,
			COALESCE("references", '') AS refs,
			created_at,
			updated_at
		FROM observations
		WHERE deleted_at IS NULL
	`
	args := []any{}
	if workspace != "" {
		q += " AND workspace = ?"
		args = append(args, workspace)
	}
	if visibility != "" {
		q += " AND visibility = ?"
		args = append(args, visibility)
	}
	q += " ORDER BY created_at ASC"

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query observations: %w", err)
	}
	defer rows.Close()

	var out []Observation
	for rows.Next() {
		var o Observation
		var createdStr, updatedStr string
		if err := rows.Scan(
			&o.ID, &o.Title, &o.Content, &o.Type,
			&o.Workspace, &o.Visibility, &o.Organization,
			&o.TopicKey, &o.References,
			&createdStr, &updatedStr,
		); err != nil {
			return nil, fmt.Errorf("scan observation: %w", err)
		}
		o.CreatedAt = parseTime(createdStr)
		o.UpdatedAt = parseTime(updatedStr)
		out = append(out, o)
	}
	return out, rows.Err()
}

// Count returns the total number of non-deleted observations.
func (r *Reader) Count() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL`).Scan(&n)
	return n, err
}

// parseTime parses the RFC3339 / SQLite datetime string stored in ancora.
func parseTime(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
