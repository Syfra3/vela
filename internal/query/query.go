package query

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"

	"github.com/Syfra3/vela/pkg/types"
)

// Engine wraps a loaded graph and answers queries against it.
type Engine struct {
	graph       *types.Graph
	nodeByID    map[string]types.Node   // node ID → node
	nodeByLabel map[string][]types.Node // label → nodes (may be multiple)
	directed    *simple.DirectedGraph
	idToInt     map[string]int64
	intToID     map[int64]string
}

// Neighbor describes a directly connected node and the edge relating it.
type Neighbor struct {
	Node      types.Node
	Edge      types.Edge
	Direction string
}

// GraphStats summarizes the graph contents for diagnostics and MCP output.
type GraphStats struct {
	NodeCount       int
	EdgeCount       int
	CommunityCount  int
	NodeTypes       map[string]int
	ConfidenceTypes map[string]int
}

// LoadFromFile reads graph.json and constructs a query Engine.
func LoadFromFile(graphPath string) (*Engine, error) {
	data, err := os.ReadFile(graphPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", graphPath, err)
	}

	// graph.json uses the export format from internal/export/json.go
	var raw struct {
		Nodes []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Kind  string `json:"kind"`
			File  string `json:"file"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Kind string `json:"kind"`
			File string `json:"file"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing graph.json: %w", err)
	}

	// Reconstruct types.Graph
	g := &types.Graph{
		Nodes: make([]types.Node, len(raw.Nodes)),
		Edges: make([]types.Edge, len(raw.Edges)),
	}
	for i, n := range raw.Nodes {
		g.Nodes[i] = types.Node{ID: n.ID, Label: n.Label, NodeType: n.Kind, SourceFile: n.File}
	}
	for i, e := range raw.Edges {
		g.Edges[i] = types.Edge{Source: e.From, Target: e.To, Relation: e.Kind, SourceFile: e.File}
	}

	return newEngine(g), nil
}

// newEngine builds indexes from a types.Graph.
func newEngine(g *types.Graph) *Engine {
	e := &Engine{
		graph:       g,
		nodeByID:    make(map[string]types.Node, len(g.Nodes)),
		nodeByLabel: make(map[string][]types.Node),
		directed:    simple.NewDirectedGraph(),
		idToInt:     make(map[string]int64, len(g.Nodes)),
		intToID:     make(map[int64]string, len(g.Nodes)),
	}

	for i, n := range g.Nodes {
		e.nodeByID[n.ID] = n
		e.nodeByLabel[n.Label] = append(e.nodeByLabel[n.Label], n)
		e.nodeByLabel[strings.ToLower(n.Label)] = append(e.nodeByLabel[strings.ToLower(n.Label)], n)

		id := int64(i + 1)
		e.idToInt[n.ID] = id
		e.intToID[id] = n.ID
		e.directed.AddNode(simple.Node(id))
	}

	for _, edge := range g.Edges {
		fromInt, fromOK := e.idToInt[edge.Source]
		// Target may be a label — resolve
		toInt, toOK := e.resolveToInt(edge.Target)
		if fromOK && toOK && fromInt != toInt {
			if !e.directed.HasEdgeFromTo(fromInt, toInt) {
				e.directed.SetEdge(simple.Edge{F: simple.Node(fromInt), T: simple.Node(toInt)})
			}
		}
	}

	return e
}

// resolveToInt resolves a target (ID or label) to a gonum int64 node ID.
func (e *Engine) resolveToInt(target string) (int64, bool) {
	if id, ok := e.idToInt[target]; ok {
		return id, true
	}
	// Try label match
	if nodes, ok := e.nodeByLabel[target]; ok && len(nodes) > 0 {
		if id, ok2 := e.idToInt[nodes[0].ID]; ok2 {
			return id, true
		}
	}
	if nodes, ok := e.nodeByLabel[strings.ToLower(target)]; ok && len(nodes) > 0 {
		if id, ok2 := e.idToInt[nodes[0].ID]; ok2 {
			return id, true
		}
	}
	return 0, false
}

// Path finds the shortest directed path from nodeA to nodeB.
// Returns a human-readable string.
func (e *Engine) Path(fromLabel, toLabel string) string {
	fromInt, fromOK := e.resolveToInt(fromLabel)
	toInt, toOK := e.resolveToInt(toLabel)

	if !fromOK {
		return fmt.Sprintf("node %q not found in graph", fromLabel)
	}
	if !toOK {
		return fmt.Sprintf("node %q not found in graph", toLabel)
	}

	shortest := path.DijkstraFrom(simple.Node(fromInt), e.directed)
	nodes, _ := shortest.To(toInt)

	if len(nodes) == 0 {
		return fmt.Sprintf("no path found from %q to %q", fromLabel, toLabel)
	}

	labels := make([]string, len(nodes))
	for i, n := range nodes {
		if nodeID, ok := e.intToID[n.ID()]; ok {
			if node, ok2 := e.nodeByID[nodeID]; ok2 {
				labels[i] = node.Label
			} else {
				labels[i] = nodeID
			}
		}
	}
	return strings.Join(labels, " → ")
}

// Explain returns all edges where the given node is source or target.
func (e *Engine) Explain(label string) string {
	nodes, ok := e.nodeByLabel[label]
	if !ok {
		nodes, ok = e.nodeByLabel[strings.ToLower(label)]
	}
	if !ok || len(nodes) == 0 {
		return fmt.Sprintf("node %q not found", label)
	}

	nodeIDs := make(map[string]bool)
	for _, n := range nodes {
		nodeIDs[n.ID] = true
	}

	var lines []string
	for _, edge := range e.graph.Edges {
		if nodeIDs[edge.Source] {
			lines = append(lines, fmt.Sprintf("  %s --[%s]--> %s", edge.Source, edge.Relation, edge.Target))
		} else if nodeIDs[edge.Target] {
			lines = append(lines, fmt.Sprintf("  %s --[%s]--> %s", edge.Source, edge.Relation, edge.Target))
		}
	}

	if len(lines) == 0 {
		return fmt.Sprintf("no edges found for %q", label)
	}

	// Deduplicate
	seen := make(map[string]bool)
	var deduped []string
	for _, l := range lines {
		if !seen[l] {
			seen[l] = true
			deduped = append(deduped, l)
		}
	}

	return fmt.Sprintf("Edges for %q:\n%s", label, strings.Join(deduped, "\n"))
}

// Query is a freeform dispatcher: parses "path A B", "explain X", etc.
func (e *Engine) Query(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "path":
		if len(parts) < 3 {
			return "usage: path <from> <to>"
		}
		return e.Path(parts[1], strings.Join(parts[2:], " "))
	case "explain":
		if len(parts) < 2 {
			return "usage: explain <node>"
		}
		return e.Explain(strings.Join(parts[1:], " "))
	case "nodes":
		return fmt.Sprintf("Total nodes: %d", len(e.graph.Nodes))
	case "edges":
		return fmt.Sprintf("Total edges: %d", len(e.graph.Edges))
	case "help":
		return "Commands:\n  path <from> <to>  — shortest dependency path\n  explain <node>    — all edges involving a node\n  nodes / edges     — graph stats\n  quit              — exit"
	default:
		return fmt.Sprintf("unknown command %q — type 'help' for available commands", cmd)
	}
}

// FindNode resolves a node by ID, exact label, or fuzzy label match.
func (e *Engine) FindNode(term string) (types.Node, bool) {
	if term == "" {
		return types.Node{}, false
	}
	if n, ok := e.nodeByID[term]; ok {
		return n, true
	}
	if nodes, ok := e.nodeByLabel[term]; ok && len(nodes) > 0 {
		return nodes[0], true
	}
	if nodes, ok := e.nodeByLabel[strings.ToLower(term)]; ok && len(nodes) > 0 {
		return nodes[0], true
	}
	results := e.Search(term, 1)
	if len(results) == 0 {
		return types.Node{}, false
	}
	return results[0], true
}

// Search performs a small lexical ranking over node fields.
func (e *Engine) Search(term string, limit int) []types.Node {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}

	type scoredNode struct {
		node  types.Node
		score int
	}

	var scored []scoredNode
	for _, node := range e.graph.Nodes {
		score := scoreNode(node, term)
		if score == 0 {
			continue
		}
		scored = append(scored, scoredNode{node: node, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].node.Label < scored[j].node.Label
		}
		return scored[i].score > scored[j].score
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}

	results := make([]types.Node, len(scored))
	for i, item := range scored {
		results[i] = item.node
	}
	return results
}

// Neighbors returns all incoming and outgoing edges for the given node.
func (e *Engine) Neighbors(label string) ([]Neighbor, error) {
	node, ok := e.FindNode(label)
	if !ok {
		return nil, fmt.Errorf("node %q not found", label)
	}

	neighbors := make([]Neighbor, 0)
	for _, edge := range e.graph.Edges {
		switch {
		case edge.Source == node.ID:
			target, ok := e.nodeByID[edge.Target]
			if !ok {
				continue
			}
			neighbors = append(neighbors, Neighbor{Node: target, Edge: edge, Direction: "outgoing"})
		case edge.Target == node.ID:
			source, ok := e.nodeByID[edge.Source]
			if !ok {
				continue
			}
			neighbors = append(neighbors, Neighbor{Node: source, Edge: edge, Direction: "incoming"})
		}
	}

	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].Direction == neighbors[j].Direction {
			return neighbors[i].Node.Label < neighbors[j].Node.Label
		}
		return neighbors[i].Direction < neighbors[j].Direction
	})

	return neighbors, nil
}

// Stats returns a structured graph summary.
func (e *Engine) Stats() GraphStats {
	stats := GraphStats{
		NodeCount:       len(e.graph.Nodes),
		EdgeCount:       len(e.graph.Edges),
		NodeTypes:       make(map[string]int),
		ConfidenceTypes: make(map[string]int),
	}

	communities := make(map[int]struct{})
	if len(e.graph.Communities) > 0 {
		stats.CommunityCount = len(e.graph.Communities)
	} else {
		for _, node := range e.graph.Nodes {
			if node.Community != 0 {
				communities[node.Community] = struct{}{}
			}
		}
		stats.CommunityCount = len(communities)
	}

	for _, node := range e.graph.Nodes {
		kind := node.NodeType
		if kind == "" {
			kind = "unknown"
		}
		stats.NodeTypes[kind]++
	}

	for _, edge := range e.graph.Edges {
		confidence := edge.Confidence
		if confidence == "" {
			confidence = "unknown"
		}
		stats.ConfidenceTypes[confidence]++
	}

	return stats
}

// Graph returns the underlying types.Graph (read-only reference).
func (e *Engine) Graph() *types.Graph {
	return e.graph
}

// NodeByID returns the node with the given ID string, if it exists.
func (e *Engine) NodeByID(id string) (types.Node, bool) {
	n, ok := e.nodeByID[id]
	return n, ok
}

func scoreNode(node types.Node, term string) int {
	label := strings.ToLower(node.Label)
	id := strings.ToLower(node.ID)
	sourceFile := strings.ToLower(node.SourceFile)
	description := strings.ToLower(node.Description)

	score := 0
	switch {
	case label == term || id == term:
		score += 10
	case strings.Contains(label, term):
		score += 6
	case strings.Contains(id, term):
		score += 5
	}
	if strings.Contains(sourceFile, term) {
		score += 3
	}
	if strings.Contains(description, term) {
		score += 2
	}
	return score
}
