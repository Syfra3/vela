package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

func TestWriteHTML(t *testing.T) {
	g := &types.Graph{
		Nodes: []types.Node{
			{ID: "a", Label: "A", NodeType: "function", Community: 0},
			{ID: "b", Label: "B", NodeType: "struct", Community: 1},
		},
		Edges: []types.Edge{
			{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED"},
		},
		Communities: map[int][]string{},
		ExtractedAt: time.Now(),
	}

	outDir := t.TempDir()
	if err := WriteHTML(g, outDir); err != nil {
		t.Fatalf("WriteHTML error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "graph.html"))
	if err != nil {
		t.Fatalf("graph.html not created: %v", err)
	}
	content := string(data)

	// Must be valid HTML with vis.js and both nodes embedded
	for _, want := range []string{
		"vis-network",
		`"id":"a"`,
		`"id":"b"`,
		`"from":"a"`,
		"Vela Knowledge Graph",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("HTML missing expected content: %q", want)
		}
	}
}
