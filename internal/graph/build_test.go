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
	// Node count should be 2 (the dangling edge doesn't add a node)
	if g.NodeCount() != 2 {
		t.Errorf("expected 2 nodes, got %d", g.NodeCount())
	}
	// Gonum edge count is 0 (unresolved target)
	if g.EdgeCount() != 0 {
		t.Errorf("expected 0 gonum edges (dangling ref), got %d", g.EdgeCount())
	}
	// But EdgeList still holds all edges for export
	if len(g.EdgeList) != 1 {
		t.Errorf("expected 1 edge in EdgeList, got %d", len(g.EdgeList))
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
	if len(tg.Edges) != 1 {
		t.Errorf("expected 1 edge in types.Graph, got %d", len(tg.Edges))
	}
}
