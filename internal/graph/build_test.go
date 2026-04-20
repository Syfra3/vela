package graph

import (
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func makeNodes(ids ...string) []types.Node {
	nodes := make([]types.Node, len(ids))
	for i, id := range ids {
		nodes[i] = types.Node{ID: id, Label: id, NodeType: "function"}
	}
	return nodes
}

func TestBuild_NodeCount(t *testing.T) {
	nodes := makeNodes("a", "b", "c")
	edges := []types.Edge{
		{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED"},
		{Source: "b", Target: "c", Relation: "calls", Confidence: "EXTRACTED"},
	}

	g, err := Build(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	if g.NodeCount() != 3 {
		t.Errorf("expected 3 nodes, got %d", g.NodeCount())
	}
	if g.EdgeCount() != 2 {
		t.Errorf("expected 2 edges, got %d", g.EdgeCount())
	}
}

func TestBuild_DanglingEdge(t *testing.T) {
	nodes := makeNodes("a", "b")
	edges := []types.Edge{
		{Source: "a", Target: "nonexistent", Relation: "calls", Confidence: "EXTRACTED"},
	}

	g, err := Build(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	// Node count should be 2
	if g.NodeCount() != 2 {
		t.Errorf("expected 2 nodes, got %d", g.NodeCount())
	}
	// Dangling edges are now dropped entirely — both EdgeCount and ResolvedEdges must be 0.
	if g.EdgeCount() != 0 {
		t.Errorf("expected 0 resolved edges (dangling ref dropped), got %d", g.EdgeCount())
	}
	if len(g.ResolvedEdges) != 0 {
		t.Errorf("expected empty ResolvedEdges, got %d", len(g.ResolvedEdges))
	}
}

func TestBuild_Degree(t *testing.T) {
	nodes := makeNodes("a", "b", "c")
	edges := []types.Edge{
		{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED"},
		{Source: "a", Target: "c", Relation: "calls", Confidence: "EXTRACTED"},
	}

	g, err := Build(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	// a has out-degree 2, b and c have in-degree 1 each
	if g.Degree("a") != 2 {
		t.Errorf("expected degree 2 for 'a', got %d", g.Degree("a"))
	}
}

func TestBuild_ToTypes(t *testing.T) {
	nodes := makeNodes("x", "y")
	edges := []types.Edge{
		{Source: "x", Target: "y", Relation: "calls", Confidence: "EXTRACTED"},
	}

	g, err := Build(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}

	tg := g.ToTypes()
	if len(tg.Nodes) != 2 {
		t.Errorf("expected 2 nodes in types.Graph, got %d", len(tg.Nodes))
	}
	// Only resolved edges in output — dangling are excluded.
	if len(tg.Edges) != 1 {
		t.Errorf("expected 1 edge in types.Graph, got %d", len(tg.Edges))
	}
}

func TestBuild_LabelResolution(t *testing.T) {
	// Simulate real extraction: Source = "<file>:<func>", Target = bare label.
	nodes := []types.Node{
		{ID: "pkg/a.go:Foo", Label: "Foo", NodeType: "function", SourceFile: "pkg/a.go"},
		{ID: "pkg/b.go:Bar", Label: "Bar", NodeType: "function", SourceFile: "pkg/b.go"},
	}
	edges := []types.Edge{
		// Target is a bare label (as produced by calleeLabel in code.go)
		{Source: "pkg/a.go:Foo", Target: "Bar", Relation: "calls", Confidence: "EXTRACTED"},
		// Dangling — no node with label or ID "external"
		{Source: "pkg/a.go:Foo", Target: "external", Relation: "calls", Confidence: "EXTRACTED"},
	}

	g, err := Build(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	if g.EdgeCount() != 1 {
		t.Errorf("expected 1 resolved edge, got %d", g.EdgeCount())
	}
	// Resolved edge target should be the label of the resolved node
	if g.ResolvedEdges[0].Target != "Bar" {
		t.Errorf("expected resolved target 'Bar', got %q", g.ResolvedEdges[0].Target)
	}
}

func TestBuild_DeduplicatesNodeIDsAndDropsOrphanEdges(t *testing.T) {
	nodes := []types.Node{
		{ID: "project:vela", Label: "vela", NodeType: string(types.NodeTypeProject)},
		{ID: "project:vela", Label: "vela duplicate", NodeType: string(types.NodeTypeProject)},
		{ID: "memory:observation:1", Label: "obs", NodeType: string(types.NodeTypeObservation)},
	}
	edges := []types.Edge{
		{Source: "memory:observation:1", Target: "project:vela", Relation: "mentions", Confidence: "INFERRED"},
		{Source: "memory:observation:1", Target: "project:vela", Relation: "mentions", Confidence: "INFERRED"},
		{Source: "missing", Target: "project:vela", Relation: "mentions", Confidence: "INFERRED"},
	}

	g, err := Build(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}

	if g.NodeCount() != 2 {
		t.Fatalf("expected 2 canonical nodes, got %d", g.NodeCount())
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("exported node count = %d, want 2", len(g.Nodes))
	}
	if g.Nodes[0].Label != "vela" {
		t.Fatalf("first node label = %q, want original node preserved", g.Nodes[0].Label)
	}
	if g.EdgeCount() != 1 {
		t.Fatalf("expected 1 canonical edge, got %d", g.EdgeCount())
	}
	if g.ResolvedEdges[0].Source != "memory:observation:1" || g.ResolvedEdges[0].Target != "vela" {
		t.Fatalf("resolved edge = %+v", g.ResolvedEdges[0])
	}

	tg := g.ToTypes()
	if len(tg.Nodes) != 2 {
		t.Fatalf("types.Graph nodes = %d, want 2", len(tg.Nodes))
	}
	if len(tg.Edges) != 1 {
		t.Fatalf("types.Graph edges = %d, want 1", len(tg.Edges))
	}
}
