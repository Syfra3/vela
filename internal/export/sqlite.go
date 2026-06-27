package export

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Syfra3/vela/pkg/types"
	_ "modernc.org/sqlite"
)

const runtimeSchemaVersion = "v0.4.0-minimal"

// WriteSQLiteGraphAtomic writes <outDir>/graph.db as the v0.4 runtime graph
// store. The first schema slice intentionally persists only the graph facts
// needed by the build/runtime artifact contract; later query scenarios can add
// traversal-specific tables without making graph.json runtime truth again.
func WriteSQLiteGraphAtomic(g *types.Graph, outDir string) error {
	if g == nil {
		return fmt.Errorf("graph is nil")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %s: %w", outDir, err)
	}
	outPath := filepath.Join(outDir, "graph.db")
	tmpPath := outPath + ".tmp"
	_ = os.Remove(tmpPath)

	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return fmt.Errorf("opening temp sqlite graph: %w", err)
	}
	defer func() {
		_ = db.Close()
		_ = os.Remove(tmpPath)
	}()

	if err := writeSQLiteGraph(db, g); err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("closing sqlite graph: %w", err)
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("renaming sqlite graph: %w", err)
	}
	return nil
}

func writeSQLiteGraph(db *sql.DB, g *types.Graph) error {
	if _, err := db.Exec(`
PRAGMA foreign_keys = ON;
CREATE TABLE schema_meta (
  schema_version TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  vela_version TEXT NOT NULL
);
CREATE TABLE nodes (
  id TEXT PRIMARY KEY,
  canonical_key TEXT NOT NULL,
  kind TEXT NOT NULL,
  label TEXT NOT NULL,
  file_path TEXT,
  metadata_json TEXT
);
CREATE UNIQUE INDEX nodes_canonical_key_idx ON nodes(canonical_key);
CREATE INDEX nodes_kind_label_idx ON nodes(kind, label);
CREATE TABLE edges (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  from_node_id TEXT NOT NULL,
  to_node_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  confidence TEXT,
  metadata_json TEXT
);
CREATE INDEX edges_from_kind_idx ON edges(from_node_id, kind);
CREATE INDEX edges_to_kind_idx ON edges(to_node_id, kind);
CREATE UNIQUE INDEX edges_unique_triple_idx ON edges(from_node_id, to_node_id, kind);
CREATE TABLE interface_facts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider TEXT NOT NULL,
  interface_kind TEXT,
  name TEXT,
  source_node_id TEXT,
  target_node_id TEXT,
  route TEXT,
  method TEXT,
  confidence TEXT,
  claim_status TEXT NOT NULL,
  source_artifact TEXT,
  metadata_json TEXT
);
CREATE INDEX interface_facts_provider_idx ON interface_facts(provider);
CREATE INDEX interface_facts_claim_status_idx ON interface_facts(claim_status);
CREATE TABLE workspace_facts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  fact_kind TEXT NOT NULL,
  subject_key TEXT NOT NULL,
  object_key TEXT,
  confidence TEXT,
  source_id TEXT,
  metadata_json TEXT
);
CREATE INDEX workspace_facts_kind_idx ON workspace_facts(fact_kind);
CREATE INDEX workspace_facts_subject_idx ON workspace_facts(subject_key);
`); err != nil {
		return fmt.Errorf("creating sqlite graph schema: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin sqlite graph transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO schema_meta(schema_version, created_at, updated_at, vela_version) VALUES (?, ?, ?, ?)`, runtimeSchemaVersion, now, now, runtimeSchemaVersion); err != nil {
		return fmt.Errorf("inserting schema metadata: %w", err)
	}
	nodeIDByLabel := make(map[string]string, len(g.Nodes))
	for _, node := range g.Nodes {
		if _, exists := nodeIDByLabel[node.Label]; !exists {
			nodeIDByLabel[node.Label] = node.ID
		}
		metadata, err := json.Marshal(node.Metadata)
		if err != nil {
			return fmt.Errorf("marshalling node metadata for %s: %w", node.ID, err)
		}
		if _, err := tx.Exec(`INSERT INTO nodes(id, canonical_key, kind, label, file_path, metadata_json) VALUES (?, ?, ?, ?, ?, ?)`, node.ID, node.ID, node.NodeType, node.Label, node.SourceFile, string(metadata)); err != nil {
			return fmt.Errorf("inserting node %s: %w", node.ID, err)
		}
	}
	for _, edge := range g.Edges {
		metadata, err := json.Marshal(edge.Metadata)
		if err != nil {
			return fmt.Errorf("marshalling edge metadata for %s -> %s: %w", edge.Source, edge.Target, err)
		}
		fromNodeID := sqliteNodeID(edge.Source, nodeIDByLabel)
		toNodeID := sqliteNodeID(edge.Target, nodeIDByLabel)
		if _, err := tx.Exec(`INSERT OR IGNORE INTO edges(from_node_id, to_node_id, kind, confidence, metadata_json) VALUES (?, ?, ?, ?, ?)`, fromNodeID, toNodeID, edge.Relation, edge.Confidence, string(metadata)); err != nil {
			return fmt.Errorf("inserting edge %s -> %s: %w", edge.Source, edge.Target, err)
		}
		if err := insertSQLiteInterfaceFact(tx, edge, fromNodeID, toNodeID, string(metadata)); err != nil {
			return err
		}
		if err := insertSQLiteWorkspaceFact(tx, edge, fromNodeID, toNodeID, string(metadata)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing sqlite graph transaction: %w", err)
	}
	return nil
}

func insertSQLiteWorkspaceFact(tx *sql.Tx, edge types.Edge, fromNodeID, toNodeID, metadata string) error {
	if metadataString(edge.Metadata, "layer") != string(types.LayerWorkspace) || metadataString(edge.Metadata, "evidence_type") != "routing" {
		return nil
	}
	confidence := metadataString(edge.Metadata, "evidence_confidence")
	if confidence == "" {
		confidence = edge.Confidence
	}
	sourceID := metadataString(edge.Metadata, "evidence_source_artifact")
	if sourceID == "" {
		sourceID = edge.SourceFile
	}
	if _, err := tx.Exec(`INSERT INTO workspace_facts(fact_kind, subject_key, object_key, confidence, source_id, metadata_json) VALUES (?, ?, ?, ?, ?, ?)`, edge.Relation, fromNodeID, toNodeID, confidence, sourceID, metadata); err != nil {
		return fmt.Errorf("inserting workspace fact %s %s -> %s: %w", edge.Relation, edge.Source, edge.Target, err)
	}
	return nil
}

func insertSQLiteInterfaceFact(tx *sql.Tx, edge types.Edge, fromNodeID, toNodeID, metadata string) error {
	provider := metadataString(edge.Metadata, "interface_provider")
	claimStatus := metadataString(edge.Metadata, "claim_status")
	if provider == "" || claimStatus == "" {
		return nil
	}
	confidence := metadataString(edge.Metadata, "evidence_confidence")
	if confidence == "" {
		confidence = edge.Confidence
	}
	sourceArtifact := metadataString(edge.Metadata, "evidence_source_artifact")
	if sourceArtifact == "" {
		sourceArtifact = edge.SourceFile
	}
	if _, err := tx.Exec(`INSERT INTO interface_facts(provider, interface_kind, name, source_node_id, target_node_id, route, method, confidence, claim_status, source_artifact, metadata_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		provider,
		metadataString(edge.Metadata, "interface_kind"),
		metadataString(edge.Metadata, "interface_name"),
		fromNodeID,
		toNodeID,
		metadataString(edge.Metadata, "interface_route"),
		metadataString(edge.Metadata, "interface_method"),
		confidence,
		claimStatus,
		sourceArtifact,
		metadata,
	); err != nil {
		return fmt.Errorf("inserting interface fact %s %s -> %s: %w", provider, edge.Source, edge.Target, err)
	}
	return nil
}

func metadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key].(string); ok {
		return value
	}
	return ""
}

func sqliteNodeID(ref string, nodeIDByLabel map[string]string) string {
	if id, ok := nodeIDByLabel[ref]; ok {
		return id
	}
	return ref
}
