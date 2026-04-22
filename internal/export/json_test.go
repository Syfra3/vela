package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

func TestWriteJSON(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{
				ID:          "a",
				Label:       "A",
				NodeType:    "function",
				SourceFile:  "main.go",
				Description: "primary entrypoint",
				Metadata:    map[string]interface{}{"path": "/tmp/project", "remote": "https://github.com/Syfra3/vela.git"},
				Source: &types.Source{
					Type:   types.SourceTypeCodebase,
					Name:   "vela",
					Path:   "/tmp/project",
					Remote: "https://github.com/Syfra3/vela.git",
				},
			},
			{ID: "b", Label: "B", NodeType: "struct", SourceFile: "main.go"},
		},
		Edges: []types.Edge{
			{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED", Metadata: map[string]interface{}{"layer": "repo", "evidence_type": "ast"}},
		},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteJSON(g, outDir); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(outDir, "graph.json")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}

	var parsed graphJSON
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if parsed.Meta.NodeCount != 2 {
		t.Errorf("expected nodeCount=2, got %d", parsed.Meta.NodeCount)
	}
	if parsed.Meta.EdgeCount != 1 {
		t.Errorf("expected edgeCount=1, got %d", parsed.Meta.EdgeCount)
	}
	if len(parsed.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(parsed.Nodes))
	}
	if parsed.Nodes[0].SourcePath != "/tmp/project" {
		t.Fatalf("node source_path = %q, want %q", parsed.Nodes[0].SourcePath, "/tmp/project")
	}
	if parsed.Nodes[0].SourceRemote != "https://github.com/Syfra3/vela.git" {
		t.Fatalf("node source_remote = %q", parsed.Nodes[0].SourceRemote)
	}
	if parsed.Nodes[0].Description != "primary entrypoint" {
		t.Fatalf("node description = %q", parsed.Nodes[0].Description)
	}
	if got, _ := parsed.Nodes[0].Metadata["path"].(string); got != "/tmp/project" {
		t.Fatalf("node metadata path = %q, want %q", got, "/tmp/project")
	}
	if len(parsed.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(parsed.Edges))
	}
	if got, _ := parsed.Edges[0].Metadata["evidence_type"].(string); got != "ast" {
		t.Fatalf("edge metadata evidence_type = %q, want %q", got, "ast")
	}

	if _, err := os.Stat(filepath.Join(outDir, "retrieval.db")); !os.IsNotExist(err) {
		t.Fatalf("expected retrieval.db to be removed from reduced export surface, err=%v", err)
	}
}

func TestWriteJSON_CreatesOutDir(t *testing.T) {
	base := t.TempDir()
	outDir := filepath.Join(base, "nested", "output")

	g := &types.Graph{
		Nodes:       []types.Node{{ID: "x", Label: "X", NodeType: "function"}},
		Edges:       []types.Edge{},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	if err := WriteJSON(g, outDir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "graph.json")); err != nil {
		t.Errorf("graph.json not created in nested dir: %v", err)
	}
}

func TestLoadJSON_RoundTripsExportFormat(t *testing.T) {
	outDir := t.TempDir()
	original := &types.Graph{
		Nodes: []types.Node{
			{
				ID:          "project:vela",
				Label:       "vela",
				NodeType:    string(types.NodeTypeProject),
				Description: "tracked repo",
				Community:   7,
				Metadata:    map[string]interface{}{"role": "root"},
				Source: &types.Source{
					Type:   types.SourceTypeCodebase,
					Name:   "vela",
					Path:   "/work/vela",
					Remote: "https://github.com/Syfra3/vela.git",
				},
			},
			{
				ID:          "memory:observation:1",
				Label:       "Fix persistence",
				NodeType:    string(types.NodeTypeObservation),
				Description: "daemon persistence note",
				Metadata:    map[string]interface{}{"type": "bugfix"},
				Source: &types.Source{
					Type: types.SourceTypeMemory,
					Name: "ancora",
				},
			},
		},
		Edges: []types.Edge{
			{
				Source:     "memory:observation:1",
				Target:     "project:vela",
				Relation:   "mentions",
				Confidence: "INFERRED",
				Score:      0.8,
				Metadata:   map[string]interface{}{"cross_layer": true},
			},
		},
	}

	if err := WriteJSON(original, outDir); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	loaded, err := LoadJSON(filepath.Join(outDir, "graph.json"))
	if err != nil {
		t.Fatalf("LoadJSON() error = %v", err)
	}

	if len(loaded.Nodes) != 2 {
		t.Fatalf("loaded node count = %d, want 2", len(loaded.Nodes))
	}
	if loaded.Nodes[0].Description != "tracked repo" {
		t.Fatalf("loaded description = %q", loaded.Nodes[0].Description)
	}
	if loaded.Nodes[0].Community != 7 {
		t.Fatalf("loaded community = %d, want 7", loaded.Nodes[0].Community)
	}
	if loaded.Nodes[0].Source == nil || loaded.Nodes[0].Source.Path != "/work/vela" {
		t.Fatalf("loaded source path = %#v", loaded.Nodes[0].Source)
	}
	if len(loaded.Edges) != 1 {
		t.Fatalf("loaded edge count = %d, want 1", len(loaded.Edges))
	}
	if loaded.Edges[0].Confidence != "INFERRED" {
		t.Fatalf("loaded edge confidence = %q", loaded.Edges[0].Confidence)
	}
	if loaded.Edges[0].Score != 0.8 {
		t.Fatalf("loaded edge score = %v, want 0.8", loaded.Edges[0].Score)
	}
}

func TestWriteJSON_DeduplicatesNodesBeforeRetrievalSync(t *testing.T) {
	t.Skip("legacy retrieval sync removed in reduced export surface")
	outDir := t.TempDir()
	g := &types.Graph{
		Nodes: []types.Node{
			{ID: "vela:vela", Label: "vela", NodeType: string(types.NodeTypeConcept)},
			{ID: "vela:vela", Label: "vela duplicate", NodeType: string(types.NodeTypeConcept)},
		},
	}

	if err := WriteJSON(g, outDir); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	loaded, err := LoadJSON(filepath.Join(outDir, "graph.json"))
	if err != nil {
		t.Fatalf("LoadJSON() error = %v", err)
	}
	if len(loaded.Nodes) != 1 {
		t.Fatalf("loaded nodes = %d, want 1", len(loaded.Nodes))
	}
}
