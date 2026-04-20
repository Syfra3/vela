package graph

import (
	"time"

	"gonum.org/v1/gonum/graph/simple"

	"github.com/Syfra3/vela/pkg/types"
)

type edgeKey struct {
	source   string
	target   string
	relation string
	path     string
}

// Graph wraps a gonum directed graph with auxiliary indexes for fast lookup.
type Graph struct {
	Directed      *simple.DirectedGraph
	NodeIndex     map[string]int64     // node ID string → gonum int64 ID
	NodeByID      map[int64]types.Node // gonum int64 ID → types.Node
	ResolvedEdges []types.Edge         // only edges where both endpoints resolved
	Nodes         []types.Node
}

// gonumNode is a minimal implementation of gonum's graph.Node.
type gonumNode struct {
	id int64
}

func (n gonumNode) ID() int64 { return n.id }

// Build constructs a Graph from the provided nodes and edges.
//
// Edge resolution:
//   - e.Source must match a node ID exactly (it is always in "<file>:<name>" format).
//   - e.Target is first tried as an exact ID, then as a bare label via labelIndex.
//
// Only edges where BOTH endpoints resolve to known nodes are stored in
// ResolvedEdges and added to the gonum graph. Dangling edges (targeting
// stdlib/external symbols) are silently dropped — they add noise and break
// Obsidian graph view by creating isolated satellite dots.
func Build(nodes []types.Node, edges []types.Edge) (*Graph, error) {
	nodes, edges = Canonicalize(nodes, edges)

	g := &Graph{
		Directed:  simple.NewDirectedGraph(),
		NodeIndex: make(map[string]int64, len(nodes)),
		NodeByID:  make(map[int64]types.Node, len(nodes)),
		Nodes:     nodes,
	}

	// labelIndex maps bare label → first matching node ID.
	// When multiple nodes share the same label (e.g. "Load" in different
	// packages), the first one encountered wins. This is deterministic
	// because node order is stable (file walk order).
	labelIndex := make(map[string]string, len(nodes))

	for i, n := range nodes {
		id := int64(i + 1) // gonum IDs must be non-zero
		gn := gonumNode{id: id}
		g.Directed.AddNode(gn)
		g.NodeIndex[n.ID] = id
		g.NodeByID[id] = n

		if _, exists := labelIndex[n.Label]; !exists {
			labelIndex[n.Label] = n.ID
		}
	}

	// resolveTarget resolves a target string (bare label or full ID) to a
	// gonum int64 node ID. Returns (id, true) on success.
	resolveTarget := func(target string) (int64, bool) {
		// Try exact ID match first (handles fully-qualified targets).
		if id, ok := g.NodeIndex[target]; ok {
			return id, true
		}
		// Fall back to label match.
		if nodeID, ok := labelIndex[target]; ok {
			if id, ok2 := g.NodeIndex[nodeID]; ok2 {
				return id, true
			}
		}
		return 0, false
	}

	for _, e := range edges {
		fromID, fromOK := g.NodeIndex[e.Source]
		toID, toOK := resolveTarget(e.Target)

		if !fromOK || !toOK || fromID == toID {
			continue // drop dangling or self-referential edges
		}

		// Rewrite Target to the resolved node's label so downstream
		// consumers (graph.json, Obsidian) always get a consistent,
		// human-readable name that matches an actual node.
		resolvedNode := g.NodeByID[toID]
		resolvedEdge := e
		resolvedEdge.Target = resolvedNode.Label

		// Deduplicate in the gonum graph (simple graph forbids multi-edges).
		if !g.Directed.HasEdgeFromTo(fromID, toID) {
			g.Directed.SetEdge(simple.Edge{
				F: gonumNode{id: fromID},
				T: gonumNode{id: toID},
			})
		}

		g.ResolvedEdges = append(g.ResolvedEdges, resolvedEdge)
	}

	return g, nil
}

// Canonicalize removes duplicate node IDs and duplicate/orphaned edges while
// preserving the first occurrence order for stable graph exports.
func Canonicalize(nodes []types.Node, edges []types.Edge) ([]types.Node, []types.Edge) {
	seenNodes := make(map[string]bool, len(nodes))
	canonicalNodes := make([]types.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.ID == "" || seenNodes[n.ID] {
			continue
		}
		seenNodes[n.ID] = true
		canonicalNodes = append(canonicalNodes, n)
	}

	seenEdges := make(map[edgeKey]bool, len(edges))
	canonicalEdges := make([]types.Edge, 0, len(edges))
	for _, e := range edges {
		if e.Source == "" || e.Target == "" {
			continue
		}
		if !seenNodes[e.Source] {
			continue
		}
		key := edgeKey{source: e.Source, target: e.Target, relation: e.Relation, path: e.SourceFile}
		if seenEdges[key] {
			continue
		}
		seenEdges[key] = true
		canonicalEdges = append(canonicalEdges, e)
	}

	return canonicalNodes, canonicalEdges
}

// NodeCount returns the number of nodes in the graph.
func (g *Graph) NodeCount() int {
	return g.Directed.Nodes().Len()
}

// EdgeCount returns the number of resolved edges in the graph.
func (g *Graph) EdgeCount() int {
	return len(g.ResolvedEdges)
}

// Degree returns the total degree (in + out) of the node with the given ID string.
func (g *Graph) Degree(nodeID string) int {
	id, ok := g.NodeIndex[nodeID]
	if !ok {
		return 0
	}
	return g.Directed.From(id).Len() + g.Directed.To(id).Len()
}

// Communities returns a map of communityID → list of node IDs (empty until
// clustering is run).
func (g *Graph) Communities() map[int][]string {
	return make(map[int][]string)
}

// ToTypes converts this internal Graph to the types.Graph for serialisation.
// Only resolved edges are exported — dangling references are excluded.
func (g *Graph) ToTypes() *types.Graph {
	return &types.Graph{
		Nodes:       g.Nodes,
		Edges:       g.ResolvedEdges,
		Communities: g.Communities(),
		Metadata: map[string]interface{}{
			"nodeCount": g.NodeCount(),
			"edgeCount": g.EdgeCount(),
		},
		ExtractedAt: time.Now(),
	}
}
