package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/pkg/types"
)

func buildTestGraph(t *testing.T) *igraph.Graph {
	t.Helper()
	nodes := []types.Node{
		{ID: "auth", Label: "auth", NodeType: "function", Community: 0},
		{ID: "payment", Label: "payment", NodeType: "function", Community: 1},
		{ID: "user", Label: "user", NodeType: "struct", Community: 0},
	}
	edges := []types.Edge{
		{Source: "auth", Target: "user", Relation: "calls", Confidence: "EXTRACTED"},
		{Source: "payment", Target: "user", Relation: "calls", Confidence: "EXTRACTED"},
	}
	g, err := igraph.Build(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGenerate_CreatesFile(t *testing.T) {
	g := buildTestGraph(t)
	outDir := t.TempDir()

	if err := Generate(g, outDir); err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	path := filepath.Join(outDir, "GRAPH_REPORT.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("GRAPH_REPORT.md not created: %v", err)
	}
	content := string(data)

	// Must contain expected sections
	for _, section := range []string{
		"# Vela Graph Report",
		"## Summary",
		"## God Nodes",
		"## Communities",
		"## Suggested Questions",
	} {
		if !strings.Contains(content, section) {
			t.Errorf("missing section %q in report", section)
		}
	}
}

func TestGenerate_NodeCountInSummary(t *testing.T) {
	g := buildTestGraph(t)
	outDir := t.TempDir()

	if err := Generate(g, outDir); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(outDir, "GRAPH_REPORT.md"))
	// Summary table should mention node count
	if !strings.Contains(string(data), "| Nodes |") {
		t.Error("report summary missing node count row")
	}
}
