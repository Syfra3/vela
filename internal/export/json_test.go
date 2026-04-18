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
				ID:         "a",
				Label:      "A",
				NodeType:   "function",
				SourceFile: "main.go",
				Metadata:   map[string]interface{}{"path": "/tmp/project", "remote": "https://github.com/Syfra3/vela.git"},
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
			{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED"},
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
	if got, _ := parsed.Nodes[0].Metadata["path"].(string); got != "/tmp/project" {
		t.Fatalf("node metadata path = %q, want %q", got, "/tmp/project")
	}
	if len(parsed.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(parsed.Edges))
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
