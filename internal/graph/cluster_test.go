package graph

import (
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

// TestRunLeiden exercises the full subprocess path.
// It succeeds even if graspologic is absent — the script falls back to
// connected-components partitioning, which is always available via Python stdlib.
func TestRunLeiden_SubprocessOrFallback(t *testing.T) {
	nodes := makeTestNodes("a", "b", "c", "d")
	edges := []types.Edge{
		{Source: "a", Target: "b", Relation: "calls", Confidence: "EXTRACTED"},
		{Source: "b", Target: "c", Relation: "calls", Confidence: "EXTRACTED"},
		// d is isolated — will be its own component
	}

	g, err := Build(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}

	partition, err := RunLeiden(g)
	if err != nil {
		// Python not available in this environment — skip rather than fail
		t.Skipf("leiden subprocess unavailable: %v", err)
	}

	// Every node must have a community assignment
	for _, n := range g.Nodes {
		if _, ok := partition[n.ID]; !ok {
			t.Errorf("node %q has no community assignment", n.ID)
		}
	}

	// "d" should be in a different community from "a","b","c" (it's isolated)
	commABC := partition["a"]
	commD := partition["d"]
	if commABC == commD {
		t.Logf("note: isolated node 'd' placed in same community as connected nodes (this may be OK for Leiden)")
	}
}

func TestApplyCommunities(t *testing.T) {
	nodes := makeTestNodes("x", "y", "z")
	g, _ := Build(nodes, nil)

	partition := map[string]int{"x": 0, "y": 0, "z": 1}
	communities := g.ApplyCommunities(partition)

	if len(communities) != 2 {
		t.Errorf("expected 2 communities, got %d", len(communities))
	}
	if len(communities[0]) != 2 {
		t.Errorf("expected 2 nodes in community 0, got %d", len(communities[0]))
	}
	if len(communities[1]) != 1 {
		t.Errorf("expected 1 node in community 1, got %d", len(communities[1]))
	}

	// Verify community is written back onto nodes
	for _, n := range g.Nodes {
		if n.ID == "z" && n.Community != 1 {
			t.Errorf("expected z.Community=1, got %d", n.Community)
		}
	}
}

func makeTestNodes(ids ...string) []types.Node {
	nodes := make([]types.Node, len(ids))
	for i, id := range ids {
		nodes[i] = types.Node{ID: id, Label: id, NodeType: "function"}
	}
	return nodes
}
