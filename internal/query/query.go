package query

import (
	"encoding/json"
	"fmt"
	"os"
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

// Graph returns the underlying types.Graph (read-only reference).
func (e *Engine) Graph() *types.Graph {
	return e.graph
}

// NodeByID returns the node with the given ID string, if it exists.
func (e *Engine) NodeByID(id string) (types.Node, bool) {
	n, ok := e.nodeByID[id]
	return n, ok
}
