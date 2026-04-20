package retrieval

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/pkg/types"
	_ "modernc.org/sqlite"
)

const dbFileName = "retrieval.db"

// Result is one lexical hit returned from the SQLite substrate.
//
// Source* fields carry the repo-local provenance of the underlying node so
// that retrieval remains repo-scoped and explainable. Fused results can use
// them to attribute evidence back to the originating repo without re-reading
// graph.json.
type Result struct {
	ID           string
	Label        string
	Kind         string
	Path         string
	Description  string
	MetadataText string
	SourceType   string
	SourceName   string
	SourcePath   string
	SourceRemote string
	Score        float64
}

// SearchOptions narrows a repo-local retrieval query. The zero value runs an
// unscoped query, which matches the historical behavior of SearchLexical and
// SearchVector.
type SearchOptions struct {
	// Limit caps the number of returned hits. Zero falls back to a sensible
	// default (5) inside each search function.
	Limit int
	// Repo scopes the query to a single repo by matching the stored
	// source_name column. Empty means no repo filter.
	Repo string
	// SourceType scopes retrieval to one stored source type. Empty means no
	// source-type filter.
	SourceType string
}

// Metadata records retrieval substrate provenance and vector runtime details.
type Metadata struct {
	EmbeddingProvider      string
	EmbeddingModel         string
	EmbeddingEndpoint      string
	VectorSearchMode       string
	VectorIndex            string
	RequestedVectorBackend string
	SQLiteVecPath          string
	SQLiteVecEnabled       bool
	SQLiteVecReason        string
	EmbeddingDims          int
}

// DBPath returns the canonical retrieval DB path for an output directory.
func DBPath(outDir string) string {
	return filepath.Join(outDir, dbFileName)
}

// EnsureGraphSync refreshes the retrieval substrate when graph.json is newer or
// the SQLite DB does not exist yet.
func EnsureGraphSync(graphPath string, graph *types.Graph) (string, error) {
	if graphPath == "" {
		return "", nil
	}
	dbPath := DBPath(filepath.Dir(graphPath))
	graphInfo, err := os.Stat(graphPath)
	if err != nil {
		return "", fmt.Errorf("stat graph.json: %w", err)
	}
	dbInfo, err := os.Stat(dbPath)
	if err == nil && !dbInfo.ModTime().Before(graphInfo.ModTime()) {
		currentRuntime, runtimeErr := CurrentEmbeddingRuntime()
		if runtimeErr == nil {
			metadata, metaErr := LoadMetadata(dbPath)
			if metaErr == nil && metadata.matches(currentRuntime) {
				return dbPath, nil
			}
		}
	}
	if err := SyncGraph(graph, dbPath); err != nil {
		return "", err
	}
	return dbPath, nil
}

// SyncGraph materializes the graph into SQLite for lexical retrieval.
func SyncGraph(graph *types.Graph, dbPath string) error {
	if graph == nil {
		return fmt.Errorf("graph is nil")
	}
	nodes, edges := graphpkgCanonical(graph.Nodes, graph.Edges)
	graph = &types.Graph{
		Nodes:       nodes,
		Edges:       edges,
		Communities: graph.Communities,
		Metadata:    graph.Metadata,
		ExtractedAt: graph.ExtractedAt,
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create retrieval dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open retrieval db: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`); err != nil {
		return fmt.Errorf("configure retrieval db: %w", err)
	}
	if err := initSchema(db); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin retrieval tx: %w", err)
	}
	defer tx.Rollback()

	for _, stmt := range []string{
		`DELETE FROM node_fts`,
		`DELETE FROM retrieval_meta`,
		`DELETE FROM node_vectors`,
		`DELETE FROM edges`,
		`DELETE FROM nodes`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("reset retrieval tables: %w", err)
		}
	}

	nodeStmt, err := tx.Prepare(`
		INSERT INTO nodes (id, label, kind, path, description, metadata_text, source_type, source_name, source_path, source_remote)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare node insert: %w", err)
	}
	defer nodeStmt.Close()

	ftsStmt, err := tx.Prepare(`
		INSERT INTO node_fts (id, label, kind, path, description, metadata_text)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare fts insert: %w", err)
	}
	defer ftsStmt.Close()

	vectorStmt, err := tx.Prepare(`
		INSERT INTO node_vectors (id, dims, embedding)
		VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare vector insert: %w", err)
	}
	defer vectorStmt.Close()

	metaStmt, err := tx.Prepare(`INSERT INTO retrieval_meta (key, value) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare metadata insert: %w", err)
	}
	defer metaStmt.Close()

	edgeStmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO edges (source_id, target_id, relation, path)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare edge insert: %w", err)
	}
	defer edgeStmt.Close()

	for _, node := range graph.Nodes {
		metadataText := flattenMetadata(node.Metadata)
		indexedLabel := indexText(node.Label)
		indexedKind := indexText(node.NodeType)
		indexedPath := indexText(node.SourceFile)
		indexedDescription := indexText(node.Description)
		indexedMetadata := indexText(metadataText)
		sourceType, sourceName, sourcePath, sourceRemote := "", "", "", ""
		if node.Source != nil {
			sourceType = string(node.Source.Type)
			sourceName = node.Source.Name
			sourcePath = node.Source.Path
			sourceRemote = node.Source.Remote
		}
		if _, err := nodeStmt.Exec(node.ID, node.Label, node.NodeType, node.SourceFile, node.Description, metadataText, sourceType, sourceName, sourcePath, sourceRemote); err != nil {
			return fmt.Errorf("insert node %q: %w", node.ID, err)
		}
		if _, err := ftsStmt.Exec(node.ID, indexedLabel, indexedKind, indexedPath, indexedDescription, indexedMetadata); err != nil {
			return fmt.Errorf("insert node fts %q: %w", node.ID, err)
		}
	}

	vectorTexts := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		metadataText := flattenMetadata(node.Metadata)
		vectorTexts = append(vectorTexts, strings.TrimSpace(strings.Join([]string{
			indexText(node.Label),
			indexText(node.NodeType),
			indexText(node.SourceFile),
			indexText(node.Description),
			indexText(metadataText),
		}, " ")))
	}
	runtime, err := CurrentEmbeddingRuntime()
	if err != nil {
		return fmt.Errorf("resolve embedding runtime: %w", err)
	}
	vectors, err := embedTexts(vectorTexts)
	if err != nil {
		runtime.SQLiteVecReason = strings.TrimSpace(runtime.SQLiteVecReason + "; vector embeddings unavailable: " + err.Error())
		vectors = nil
	} else {
		if len(vectors) != len(graph.Nodes) {
			return fmt.Errorf("embed retrieval nodes returned %d vectors for %d nodes", len(vectors), len(graph.Nodes))
		}
		for i, node := range graph.Nodes {
			if _, err := vectorStmt.Exec(node.ID, len(vectors[i]), encodeEmbedding(vectors[i])); err != nil {
				return fmt.Errorf("insert node vector %q: %w", node.ID, err)
			}
		}
	}
	metadata := Metadata{
		EmbeddingProvider:      runtime.Provider,
		EmbeddingModel:         runtime.Model,
		EmbeddingEndpoint:      runtime.Endpoint,
		VectorSearchMode:       runtime.VectorSearchMode,
		VectorIndex:            runtime.VectorIndex,
		RequestedVectorBackend: runtime.RequestedVectorBackend,
		SQLiteVecPath:          runtime.SQLiteVecPath,
		SQLiteVecEnabled:       runtime.SQLiteVecEnabled,
		SQLiteVecReason:        runtime.SQLiteVecReason,
	}
	if len(vectors) > 0 {
		metadata.EmbeddingDims = len(vectors[0])
	}
	for key, value := range metadata.values() {
		if _, err := metaStmt.Exec(key, value); err != nil {
			return fmt.Errorf("insert retrieval metadata %q: %w", key, err)
		}
	}

	for _, edge := range graph.Edges {
		if _, err := edgeStmt.Exec(edge.Source, edge.Target, edge.Relation, edge.SourceFile); err != nil {
			return fmt.Errorf("insert edge %q -> %q: %w", edge.Source, edge.Target, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit retrieval tx: %w", err)
	}
	return nil
}

func graphpkgCanonical(nodes []types.Node, edges []types.Edge) ([]types.Node, []types.Edge) {
	return graph.Canonicalize(nodes, edges)
}

// SearchLexical runs FTS5 over the materialized graph nodes. It is a
// convenience wrapper that delegates to SearchLexicalWithOptions with default
// (unscoped) options.
func SearchLexical(dbPath, input string, limit int) ([]Result, int, error) {
	return SearchLexicalWithOptions(dbPath, input, SearchOptions{Limit: limit})
}

// SearchLexicalWithOptions runs FTS5 over the materialized graph nodes with
// an optional repo scope applied at the SQL layer. Repo scoping matches the
// stored source_name column so that retrieval stays repo-bounded.
func SearchLexicalWithOptions(dbPath, input string, opts SearchOptions) ([]Result, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}
	query := buildMatchQuery(input)
	if query == "" {
		return nil, 0, nil
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open retrieval db: %w", err)
	}
	defer db.Close()

	filterSQL, filterArgs := searchScopeClause(opts, "n")

	countSQL := `SELECT COUNT(*) FROM node_fts JOIN nodes n ON n.id = node_fts.id WHERE node_fts MATCH ?` + filterSQL
	countArgs := append([]any{query}, filterArgs...)
	var candidates int
	if err := db.QueryRow(countSQL, countArgs...).Scan(&candidates); err != nil {
		return nil, 0, fmt.Errorf("count lexical hits: %w", err)
	}

	selectSQL := `
		SELECT
			n.id,
			n.label,
			COALESCE(n.kind, ''),
			COALESCE(n.path, ''),
			COALESCE(n.description, ''),
			COALESCE(n.metadata_text, ''),
			COALESCE(n.source_type, ''),
			COALESCE(n.source_name, ''),
			COALESCE(n.source_path, ''),
			COALESCE(n.source_remote, ''),
			-bm25(node_fts, 8.0, 3.0, 2.0, 2.0, 1.0) AS score
		FROM node_fts
		JOIN nodes n ON n.id = node_fts.id
		WHERE node_fts MATCH ?` + filterSQL + `
		ORDER BY score DESC, lower(n.label) ASC
		LIMIT ?`
	queryArgs := append([]any{query}, filterArgs...)
	queryArgs = append(queryArgs, limit)
	rows, err := db.Query(selectSQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query lexical hits: %w", err)
	}
	defer rows.Close()

	results := make([]Result, 0, limit)
	for rows.Next() {
		var item Result
		if err := rows.Scan(
			&item.ID, &item.Label, &item.Kind, &item.Path, &item.Description, &item.MetadataText,
			&item.SourceType, &item.SourceName, &item.SourcePath, &item.SourceRemote,
			&item.Score,
		); err != nil {
			return nil, 0, fmt.Errorf("scan lexical hit: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate lexical hits: %w", err)
	}
	return results, candidates, nil
}

// searchScopeClause returns AND-clauses for retrieval filters and matching args.
func searchScopeClause(opts SearchOptions, alias string) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if repo := strings.TrimSpace(opts.Repo); repo != "" {
		clauses = append(clauses, alias+".source_name = ?")
		args = append(args, repo)
	}
	if sourceType := strings.TrimSpace(opts.SourceType); sourceType != "" {
		clauses = append(clauses, alias+".source_type = ?")
		args = append(args, sourceType)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func initSchema(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			kind TEXT,
			path TEXT,
			description TEXT,
			metadata_text TEXT,
			source_type TEXT,
			source_name TEXT,
			source_path TEXT,
			source_remote TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS edges (
			source_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			relation TEXT NOT NULL,
			path TEXT,
			PRIMARY KEY (source_id, target_id, relation, path)
		)`,
		`CREATE TABLE IF NOT EXISTS node_vectors (
			id TEXT PRIMARY KEY,
			dims INTEGER NOT NULL,
			embedding BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS retrieval_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS node_fts USING fts5(
			id UNINDEXED,
			label,
			kind,
			path,
			description,
			metadata_text,
			tokenize='unicode61 remove_diacritics 2'
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("init retrieval schema: %w", err)
		}
	}
	return nil
}

func LoadMetadata(dbPath string) (Metadata, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("open retrieval db: %w", err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT key, value FROM retrieval_meta`)
	if err != nil {
		return Metadata{}, fmt.Errorf("query retrieval metadata: %w", err)
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Metadata{}, fmt.Errorf("scan retrieval metadata: %w", err)
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return Metadata{}, fmt.Errorf("iterate retrieval metadata: %w", err)
	}
	metadata := Metadata{
		EmbeddingProvider:      values["embedding_provider"],
		EmbeddingModel:         values["embedding_model"],
		EmbeddingEndpoint:      values["embedding_endpoint"],
		VectorSearchMode:       values["vector_search_mode"],
		VectorIndex:            values["vector_index"],
		RequestedVectorBackend: values["requested_vector_backend"],
		SQLiteVecPath:          values["sqlite_vec_path"],
		SQLiteVecEnabled:       values["sqlite_vec_enabled"] == "true",
		SQLiteVecReason:        values["sqlite_vec_reason"],
	}
	if values["embedding_dims"] != "" {
		_, _ = fmt.Sscanf(values["embedding_dims"], "%d", &metadata.EmbeddingDims)
	}
	return metadata, nil
}

func (m Metadata) values() map[string]string {
	return map[string]string{
		"embedding_provider":       m.EmbeddingProvider,
		"embedding_model":          m.EmbeddingModel,
		"embedding_endpoint":       m.EmbeddingEndpoint,
		"vector_search_mode":       m.VectorSearchMode,
		"vector_index":             m.VectorIndex,
		"requested_vector_backend": m.RequestedVectorBackend,
		"sqlite_vec_path":          m.SQLiteVecPath,
		"sqlite_vec_enabled":       fmt.Sprintf("%t", m.SQLiteVecEnabled),
		"sqlite_vec_reason":        m.SQLiteVecReason,
		"embedding_dims":           fmt.Sprintf("%d", m.EmbeddingDims),
	}
}

func (m Metadata) matches(runtime EmbeddingRuntime) bool {
	return m.EmbeddingProvider == runtime.Provider &&
		m.EmbeddingModel == runtime.Model &&
		m.EmbeddingEndpoint == runtime.Endpoint &&
		m.VectorSearchMode == runtime.VectorSearchMode &&
		m.VectorIndex == runtime.VectorIndex &&
		m.RequestedVectorBackend == runtime.RequestedVectorBackend &&
		m.SQLiteVecPath == runtime.SQLiteVecPath &&
		m.SQLiteVecEnabled == runtime.SQLiteVecEnabled
}

func buildMatchQuery(input string) string {
	tokens := tokenize(input)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		variants := tokenVariants(token)
		clauses := make([]string, 0, len(variants))
		for _, variant := range variants {
			clauses = append(clauses, fmt.Sprintf("\"%s\"", strings.ReplaceAll(variant, `"`, `""`)))
		}
		if len(clauses) == 1 {
			parts = append(parts, clauses[0])
			continue
		}
		parts = append(parts, "("+strings.Join(clauses, " OR ")+")")
	}
	return strings.Join(parts, " AND ")
}

func tokenize(input string) []string {
	return strings.FieldsFunc(strings.ToLower(input), func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= '0' && r <= '9':
			return false
		default:
			return true
		}
	})
}

func tokenVariants(token string) []string {
	variants := []string{token}
	if len(token) > 3 && strings.HasSuffix(token, "es") {
		variants = append(variants, token[:len(token)-2])
	}
	if len(token) > 2 && strings.HasSuffix(token, "s") {
		variants = append(variants, token[:len(token)-1])
	}
	return variants
}

func flattenMetadata(metadata map[string]interface{}) string {
	if len(metadata) == 0 {
		return ""
	}
	parts := make([]string, 0, len(metadata))
	for key, value := range metadata {
		parts = append(parts, key+" "+fmt.Sprint(value))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func indexText(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	expanded := expandIdentifiers(input)
	if expanded == input {
		return input
	}
	return input + " " + expanded
}

func expandIdentifiers(input string) string {
	var out []rune
	var prev rune
	for i, r := range input {
		if isASCIIAlphaNum(r) {
			if i > 0 && shouldSplitIdentifier(prev, r) && len(out) > 0 && out[len(out)-1] != ' ' {
				out = append(out, ' ')
			}
			out = append(out, r)
			prev = r
			continue
		}
		if len(out) > 0 && out[len(out)-1] != ' ' {
			out = append(out, ' ')
		}
		prev = ' '
	}
	return strings.Join(strings.Fields(string(out)), " ")
}

func shouldSplitIdentifier(prev, current rune) bool {
	return (prev >= 'a' && prev <= 'z' && current >= 'A' && current <= 'Z') ||
		(prev >= '0' && prev <= '9' && ((current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z'))) ||
		(((prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z')) && current >= '0' && current <= '9')
}

func isASCIIAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
