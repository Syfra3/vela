package graph

import (
	"time"

	"gonum.org/v1/gonum/graph/simple"

	"github.com/Syfra3/vela/pkg/types"
)

// Graph wraps a gonum directed graph with auxiliary indexes for fast lookup.
type Graph struct {
	Directed  *simple.DirectedGraph
	NodeIndex map[string]int64     // node ID string → gonum int64 ID
	NodeByID  map[int64]types.Node // gonum int64 ID → types.Node
	EdgeList  []types.Edge
	Nodes     []types.Node
}

// gonumNode is a minimal implementation of gonum's graph.Node.
type gonumNode struct {
	id int64
}

func (n gonumNode) ID() int64 { return n.id }

// Build constructs a Graph from the provided nodes and edges.
// Edges whose Target is not a known node ID are stored as dangling references
// (not an error) — they will still appear in EdgeList for export purposes.
func Build(nodes []types.Node, edges []types.Edge) (*Graph, error) {
	g := &Graph{
		Directed:  simple.NewDirectedGraph(),
		NodeIndex: make(map[string]int64, len(nodes)),
		NodeByID:  make(map[int64]types.Node, len(nodes)),
		Nodes:     nodes,
		EdgeList:  edges,
	}

	// Add all nodes
	for i, n := range nodes {
		id := int64(i + 1) // gonum IDs must be non-zero
		gn := gonumNode{id: id}
		g.Directed.AddNode(gn)
		g.NodeIndex[n.ID] = id
		g.NodeByID[id] = n
	}

	// Add edges where both endpoints are known nodes
	for _, e := range edges {
		fromID, fromOK := g.NodeIndex[e.Source]
		// Target is a label, not an ID — try to find matching node
		toID, toOK := resolveTarget(g, e.Target)

		if fromOK && toOK && fromID != toID {
			// Only add if not already present (simple graph forbids duplicates)
			if g.Directed.HasEdgeFromTo(fromID, toID) {
				continue
			}
			g.Directed.SetEdge(simple.Edge{
				F: gonumNode{id: fromID},
				T: gonumNode{id: toID},
			})
		}
	}

	return g, nil
}

// NodeCount returns the number of nodes in the graph.
func (g *Graph) NodeCount() int {
	return g.Directed.Nodes().Len()
}

// EdgeCount returns the number of edges added to the gonum graph
// (resolved edges only; dangling references are not counted here).
func (g *Graph) EdgeCount() int {
	return g.Directed.Edges().Len()
}

// Degree returns the total degree (in + out) of the node with the given ID string.
func (g *Graph) Degree(nodeID string) int {
	id, ok := g.NodeIndex[nodeID]
	if !ok {
		return 0
	}
	return g.Directed.From(id).Len() + g.Directed.To(id).Len()
}

// resolveTarget looks up a node whose Label matches target, falling back to
// an ID match. Returns (id, true) on success.
func resolveTarget(g *Graph, target string) (int64, bool) {
	// Try direct ID match first
	if id, ok := g.NodeIndex[target]; ok {
		return id, true
	}
	// Try label match
	for _, n := range g.Nodes {
		if n.Label == target {
			if id, ok := g.NodeIndex[n.ID]; ok {
				return id, true
			}
		}
	}
	return 0, false
}

// Communities returns a map of communityID → list of node IDs (empty until
// clustering is run).
func (g *Graph) Communities() map[int][]string {
	return make(map[int][]string)
}

// ToTypes converts this internal Graph to the types.Graph for serialisation.
func (g *Graph) ToTypes() *types.Graph {
	return &types.Graph{
		Nodes:       g.Nodes,
		Edges:       g.EdgeList,
		Communities: g.Communities(),
		Metadata: map[string]interface{}{
			"nodeCount": g.NodeCount(),
			"edgeCount": g.EdgeCount(),
		},
		ExtractedAt: time.Now(),
	}
}
