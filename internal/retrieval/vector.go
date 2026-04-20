package retrieval

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	vectorMinSimilarity = 0.18
)

// SearchVector runs local vector retrieval over stored node embeddings using
// default (unscoped) options.
func SearchVector(dbPath, input string, limit int) ([]Result, int, error) {
	return SearchVectorWithOptions(dbPath, input, SearchOptions{Limit: limit})
}

// SearchVectorWithOptions runs local vector retrieval with an optional repo
// scope applied to the stored node vectors. Embeddings are kept locally in
// SQLite and the repo filter ensures retrieval stays repo-scoped rather than
// querying a global deep index.
func SearchVectorWithOptions(dbPath, input string, opts SearchOptions) ([]Result, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 5
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open retrieval db: %w", err)
	}
	defer db.Close()

	filterSQL, filterArgs := searchScopeClause(opts, "n")
	countSQL := `
		SELECT COUNT(1)
		FROM node_vectors v
		JOIN nodes n ON n.id = v.id
		WHERE 1=1` + filterSQL
	var storedVectors int
	if err := db.QueryRow(countSQL, filterArgs...).Scan(&storedVectors); err != nil {
		return nil, 0, fmt.Errorf("count node vectors: %w", err)
	}
	if storedVectors == 0 {
		return nil, 0, nil
	}

	vectors, err := embedTexts([]string{indexText(input)})
	if err != nil {
		return nil, 0, nil
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil, 0, nil
	}
	query := vectors[0]

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
			v.embedding
		FROM node_vectors v
		JOIN nodes n ON n.id = v.id
		WHERE 1=1` + filterSQL
	rows, err := db.Query(selectSQL, filterArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query node vectors: %w", err)
	}
	defer rows.Close()

	var (
		results    []Result
		candidates int
	)
	for rows.Next() {
		var (
			item Result
			raw  []byte
		)
		if err := rows.Scan(
			&item.ID, &item.Label, &item.Kind, &item.Path, &item.Description, &item.MetadataText,
			&item.SourceType, &item.SourceName, &item.SourcePath, &item.SourceRemote,
			&raw,
		); err != nil {
			return nil, 0, fmt.Errorf("scan node vector: %w", err)
		}
		stored, err := decodeEmbedding(raw)
		if err != nil {
			return nil, 0, fmt.Errorf("decode node vector %q: %w", item.ID, err)
		}
		item.Score = cosineSimilarity(query, stored)
		if item.Score < vectorMinSimilarity {
			continue
		}
		candidates++
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate node vectors: %w", err)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return strings.ToLower(results[i].Label) < strings.ToLower(results[j].Label)
		}
		return results[i].Score > results[j].Score
	})
	if limit < len(results) {
		results = results[:limit]
	}
	return results, candidates, nil
}
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	dot := 0.0
	for i := range a {
		dot += float64(a[i] * b[i])
	}
	return dot
}

func encodeEmbedding(values []float32) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, len(values)*4))
	for _, value := range values {
		_ = binary.Write(buf, binary.LittleEndian, value)
	}
	return buf.Bytes()
}

func decodeEmbedding(raw []byte) ([]float32, error) {
	if len(raw)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding length %d", len(raw))
	}
	values := make([]float32, len(raw)/4)
	reader := bytes.NewReader(raw)
	for i := range values {
		if err := binary.Read(reader, binary.LittleEndian, &values[i]); err != nil {
			return nil, err
		}
	}
	return values, nil
}
