package query

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	igraph "github.com/Syfra3/vela/internal/graph"
	"gonum.org/v1/gonum/graph/path"
	"gonum.org/v1/gonum/graph/simple"

	"github.com/Syfra3/vela/pkg/types"
)

// Engine wraps a loaded graph and answers queries against it.
type Engine struct {
	graphPath     string
	graph         *types.Graph
	nodeByID      map[string]types.Node   // node ID → node
	nodeByLabel   map[string][]types.Node // label → nodes (may be multiple)
	canonicalToID map[string]string
	edgeByTriple  map[string]types.Edge
	directed      *simple.DirectedGraph
	idToInt       map[string]int64
	intToID       map[int64]string
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
			ID           string                 `json:"id"`
			Label        string                 `json:"label"`
			Kind         string                 `json:"kind"`
			File         string                 `json:"file"`
			Description  string                 `json:"description"`
			SourceType   string                 `json:"source_type"`
			SourceName   string                 `json:"source_name"`
			SourcePath   string                 `json:"source_path"`
			SourceRemote string                 `json:"source_remote"`
			Metadata     map[string]interface{} `json:"metadata"`
		} `json:"nodes"`
		Edges []struct {
			From       string                 `json:"from"`
			To         string                 `json:"to"`
			Kind       string                 `json:"kind"`
			File       string                 `json:"file"`
			Confidence string                 `json:"confidence"`
			Score      float64                `json:"score"`
			Metadata   map[string]interface{} `json:"metadata"`
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
		g.Nodes[i] = types.Node{
			ID:          n.ID,
			Label:       n.Label,
			NodeType:    n.Kind,
			SourceFile:  n.File,
			Description: n.Description,
			Metadata:    n.Metadata,
		}
		if n.SourceType != "" || n.SourceName != "" || n.SourcePath != "" || n.SourceRemote != "" {
			g.Nodes[i].Source = &types.Source{
				Type:   types.SourceType(n.SourceType),
				Name:   n.SourceName,
				Path:   n.SourcePath,
				Remote: n.SourceRemote,
			}
		}
	}
	for i, e := range raw.Edges {
		g.Edges[i] = types.Edge{Source: e.From, Target: e.To, Relation: e.Kind, SourceFile: e.File, Confidence: e.Confidence, Score: e.Score, Metadata: e.Metadata}
	}
	g.Nodes, g.Edges = igraph.Canonicalize(g.Nodes, g.Edges)

	eng := newEngine(g)
	eng.graphPath = graphPath
	return eng, nil
}

// newEngine builds indexes from a types.Graph.
func newEngine(g *types.Graph) *Engine {
	e := &Engine{
		graph:         g,
		nodeByID:      make(map[string]types.Node, len(g.Nodes)),
		nodeByLabel:   make(map[string][]types.Node),
		canonicalToID: make(map[string]string, len(g.Nodes)),
		edgeByTriple:  make(map[string]types.Edge, len(g.Edges)),
		directed:      simple.NewDirectedGraph(),
		idToInt:       make(map[string]int64, len(g.Nodes)),
		intToID:       make(map[int64]string, len(g.Nodes)),
	}

	for i, n := range g.Nodes {
		e.nodeByID[n.ID] = n
		e.nodeByLabel[n.Label] = append(e.nodeByLabel[n.Label], n)
		e.nodeByLabel[strings.ToLower(n.Label)] = append(e.nodeByLabel[strings.ToLower(n.Label)], n)
		if canonical := types.CanonicalKeyForNode(n); !canonical.IsZero() {
			e.canonicalToID[canonical.String()] = n.ID
		}

		id := int64(i + 1)
		e.idToInt[n.ID] = id
		e.intToID[id] = n.ID
		e.directed.AddNode(simple.Node(id))
	}

	normalizedEdges := make([]types.Edge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		normalized := edge
		normalized.Source = e.resolveEdgeNodeRef(edge.Source, edge)
		normalized.Target = e.resolveEdgeNodeRef(edge.Target, edge)
		triple := edgeTriple(normalized.Source, normalized.Target, normalized.Relation)
		if current, ok := e.edgeByTriple[triple]; !ok || types.PreferEdgeEvidence(normalized, current) {
			e.edgeByTriple[triple] = normalized
		}
		fromInt, fromOK := e.idToInt[normalized.Source]
		toInt, toOK := e.resolveToInt(normalized.Target)
		if fromOK && toOK && fromInt != toInt {
			if !e.directed.HasEdgeFromTo(fromInt, toInt) {
				e.directed.SetEdge(simple.Edge{F: simple.Node(fromInt), T: simple.Node(toInt)})
			}
		}
		normalizedEdges = append(normalizedEdges, normalized)
	}

	e.graph = &types.Graph{
		Nodes:       g.Nodes,
		Edges:       normalizedEdges,
		Communities: g.Communities,
		Metadata:    g.Metadata,
		ExtractedAt: g.ExtractedAt,
	}

	return e
}

func edgeTriple(source, target, relation string) string {
	return source + "|" + relation + "|" + target
}

// resolveToInt resolves a target (ID or label) to a gonum int64 node ID.
func (e *Engine) resolveToInt(target string) (int64, bool) {
	if id, ok := e.idToInt[target]; ok {
		return id, true
	}
	if canonical := types.CanonicalKeyForID(target, "", nil); !canonical.IsZero() {
		if id, ok := e.canonicalToID[canonical.String()]; ok {
			if intID, ok := e.idToInt[id]; ok {
				return intID, true
			}
		}
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

func (e *Engine) resolveEdgeNodeRef(ref string, edge types.Edge) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ref
	}
	if _, ok := e.nodeByID[ref]; ok {
		return ref
	}
	if id := e.resolveNodeID(ref, preferredLayer(edge)); id != "" {
		return id
	}
	return ref
}

func (e *Engine) resolveNodeID(ref string, preferred types.Layer) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if _, ok := e.nodeByID[ref]; ok {
		return ref
	}
	if canonical := types.CanonicalJoinKey(ref, "", nil); canonical != "" {
		if id, ok := e.canonicalToID[canonical]; ok {
			return id
		}
	}
	candidates := e.nodeCandidates(ref)
	if len(candidates) == 0 {
		return ""
	}
	if preferred != "" {
		preferredMatches := make([]types.Node, 0, len(candidates))
		for _, candidate := range candidates {
			if nodeLayer(candidate) == preferred {
				preferredMatches = append(preferredMatches, candidate)
			}
		}
		if len(preferredMatches) == 1 {
			return preferredMatches[0].ID
		}
		if len(preferredMatches) > 1 {
			candidates = preferredMatches
		}
	}
	return candidates[0].ID
}

func (e *Engine) nodeCandidates(ref string) []types.Node {
	if nodes, ok := e.nodeByLabel[ref]; ok && len(nodes) > 0 {
		return uniqueNodes(nodes)
	}
	if nodes, ok := e.nodeByLabel[strings.ToLower(ref)]; ok && len(nodes) > 0 {
		return uniqueNodes(nodes)
	}
	return nil
}

func uniqueNodes(nodes []types.Node) []types.Node {
	out := make([]types.Node, 0, len(nodes))
	seen := map[string]struct{}{}
	for _, node := range nodes {
		if _, ok := seen[node.ID]; ok {
			continue
		}
		seen[node.ID] = struct{}{}
		out = append(out, node)
	}
	return out
}

func preferredLayer(edge types.Edge) types.Layer {
	if ev := types.EdgeEvidence(edge); ev.Layer != "" {
		return ev.Layer
	}
	return ""
}

func nodeLayer(node types.Node) types.Layer {
	if key := types.CanonicalKeyForNode(node); !key.IsZero() && key.Layer != "" {
		return key.Layer
	}
	if node.Source != nil {
		return types.LayerOf(node.Source.Type)
	}
	return ""
}

// Path finds the shortest directed path from nodeA to nodeB.
// Returns a human-readable string.
func (e *Engine) Path(fromLabel, toLabel string) string {
	fromInt, fromOK := e.resolveToInt(fromLabel)
	toInt, toOK := e.resolveToInt(toLabel)

	if !fromOK {
		return degradedNoPathMessage(fromLabel, toLabel, fmt.Sprintf("source node %q is missing from the current graph", fromLabel))
	}
	if !toOK {
		return degradedNoPathMessage(fromLabel, toLabel, fmt.Sprintf("target node %q is missing from the current graph", toLabel))
	}
	if fromID, ok := e.intToID[fromInt]; ok {
		if toID, ok := e.intToID[toInt]; ok {
			fromNode, fromNodeOK := e.nodeByID[fromID]
			toNode, toNodeOK := e.nodeByID[toID]
			if fromNodeOK && toNodeOK && isFileNode(fromNode) && isFileNode(toNode) {
				return e.fileDependencyPath(fromID, toID, fromLabel, toLabel)
			}
		}
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
				labels[i] = describeNode(node)
			} else {
				labels[i] = nodeID
			}
		}
	}
	return strings.Join(labels, " → ")
}

func degradedNoPathMessage(fromLabel, toLabel, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Sprintf("no path found from %q to %q", fromLabel, toLabel)
	}
	return fmt.Sprintf("no path found from %q to %q\nreason: %s\nprovenance: degraded graph lookup", fromLabel, toLabel, reason)
}

func (e *Engine) fileDependencyPath(fromID, toID, fromLabel, toLabel string) string {
	pathIDs, ok := e.rankFileDependencyPath(fromID, toID)
	if !ok {
		return fmt.Sprintf("no path found from %q to %q", fromLabel, toLabel)
	}
	labels := make([]string, 0, len(pathIDs))
	for _, nodeID := range pathIDs {
		labels = append(labels, e.describeRef(nodeID))
	}
	return strings.Join(labels, " → ")
}

func (e *Engine) rankFileDependencyPath(fromID, toID string) ([]string, bool) {
	adjacency := make(map[string][]string)
	for _, edge := range e.graph.Edges {
		if !isFileDependencyEdge(edge, e.nodeByID) || edge.Source == edge.Target {
			continue
		}
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
	}
	for source := range adjacency {
		sort.Strings(adjacency[source])
	}

	shortest, ok := shortestFileDependencyPath(adjacency, fromID, toID)
	if !ok {
		return nil, false
	}
	maxDepth := len(shortest) + 3
	if maxDepth < len(shortest) {
		maxDepth = len(shortest)
	}
	if maxDepth > 8 {
		maxDepth = 8
	}

	best := shortest
	bestScore := e.rankFileDependencyPathScore(best)
	pathBuf := make([]string, 0, maxDepth)
	visited := map[string]bool{}

	var dfs func(string)
	dfs = func(current string) {
		if len(pathBuf) >= maxDepth {
			return
		}
		visited[current] = true
		pathBuf = append(pathBuf, current)
		if current == toID {
			candidate := append([]string(nil), pathBuf...)
			score := e.rankFileDependencyPathScore(candidate)
			if score > bestScore || (score == bestScore && preferRankedFilePath(candidate, best)) {
				best = candidate
				bestScore = score
			}
		} else {
			for _, next := range adjacency[current] {
				if visited[next] {
					continue
				}
				dfs(next)
			}
		}
		pathBuf = pathBuf[:len(pathBuf)-1]
		delete(visited, current)
	}

	dfs(fromID)
	return best, true
}

func shortestFileDependencyPath(adjacency map[string][]string, fromID, toID string) ([]string, bool) {
	prev := map[string]string{}
	queue := []string{fromID}
	visited := map[string]bool{fromID: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == toID {
			break
		}
		for _, next := range adjacency[current] {
			if visited[next] {
				continue
			}
			visited[next] = true
			prev[next] = current
			queue = append(queue, next)
		}
	}
	if !visited[toID] {
		return nil, false
	}
	pathIDs := []string{toID}
	for current := toID; current != fromID; {
		current = prev[current]
		pathIDs = append([]string{current}, pathIDs...)
	}
	return pathIDs, true
}

func preferRankedFilePath(candidate, current []string) bool {
	if len(candidate) != len(current) {
		return len(candidate) < len(current)
	}
	return strings.Join(candidate, "\x00") < strings.Join(current, "\x00")
}

func (e *Engine) rankFileDependencyPathScore(pathIDs []string) int {
	if len(pathIDs) < 2 {
		return 0
	}
	source, sourceOK := e.nodeByID[pathIDs[0]]
	target, targetOK := e.nodeByID[pathIDs[len(pathIDs)-1]]
	if !sourceOK || !targetOK {
		return -len(pathIDs)
	}
	sourcePath := normalizeRankedFilePath(source.SourceFile)
	targetPath := normalizeRankedFilePath(target.SourceFile)
	sourceRoot := rankedFilePathRoot(sourcePath)
	targetRoot := rankedFilePathRoot(targetPath)
	score := -len(pathIDs)
	if len(pathIDs) > 2 {
		score += 10
	}
	targetRootOnly := len(pathIDs) > 2
	for _, nodeID := range pathIDs[1 : len(pathIDs)-1] {
		node, ok := e.nodeByID[nodeID]
		if !ok {
			continue
		}
		path := normalizeRankedFilePath(node.SourceFile)
		base := filepath.Base(path)
		if targetRoot != "" && strings.HasPrefix(path, targetRoot) {
			score += 40
		} else {
			targetRootOnly = false
		}
		if strings.HasPrefix(base, "index.") {
			score += 30
		}
		switch base {
		case "config.go":
			score += 25
		case "types.go":
			score += 10
		}
		if sourceRoot != "" && targetRoot != "" && sourceRoot != targetRoot && strings.HasPrefix(path, sourceRoot) {
			score -= 20
		}
		if strings.Contains(path, "/domain/") {
			score -= 15
		}
		if strings.Contains(path, "/context/") && !strings.Contains(path, "/presentation/context/") && !strings.Contains(path, "/shared/contexts/") {
			score -= 10
		}
	}
	if targetRootOnly {
		score += 20
	}
	return score
}

func normalizeRankedFilePath(path string) string {
	return strings.ToLower(strings.TrimSpace(path))
}

func rankedFilePathRoot(path string) string {
	path = normalizeRankedFilePath(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "packages/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			return strings.Join(parts[:3], "/") + "/"
		}
	}
	if strings.HasPrefix(path, "internal/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return strings.Join(parts[:2], "/") + "/"
		}
	}
	if strings.HasPrefix(path, "cmd/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return strings.Join(parts[:2], "/") + "/"
		}
	}
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash == -1 {
		return ""
	}
	return path[:lastSlash+1]
}

// Explain returns all edges where the given node is source or target.
func (e *Engine) Explain(label string) string {
	nodeIDs := e.resolveNodeIDs(label)
	if len(nodeIDs) == 0 {
		return fmt.Sprintf("node %q not found", label)
	}

	nodeSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}

	var lines []string
	for _, edge := range e.graph.Edges {
		if nodeSet[edge.Source] || nodeSet[edge.Target] {
			lines = append(lines, "  "+e.formatExplainEdge(edge))
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

func (e *Engine) resolveNodeIDs(ref string) []string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	if _, ok := e.nodeByID[ref]; ok {
		return []string{ref}
	}
	if id, ok := e.canonicalToID[types.CanonicalJoinKey(ref, "", nil)]; ok {
		return []string{id}
	}
	if nodes, ok := e.nodeByLabel[ref]; ok && len(nodes) > 0 {
		return nodeIDs(nodes)
	}
	if nodes, ok := e.nodeByLabel[strings.ToLower(ref)]; ok && len(nodes) > 0 {
		return nodeIDs(nodes)
	}
	return nil
}

func nodeIDs(nodes []types.Node) []string {
	out := make([]string, 0, len(nodes))
	seen := map[string]struct{}{}
	for _, n := range nodes {
		if _, ok := seen[n.ID]; ok {
			continue
		}
		seen[n.ID] = struct{}{}
		out = append(out, n.ID)
	}
	return out
}

func (e *Engine) formatExplainEdge(edge types.Edge) string {
	line := fmt.Sprintf("%s --[%s]--> %s", e.describeRef(edge.Source), edge.Relation, e.describeRef(edge.Target))
	ev := types.EdgeEvidence(edge)
	parts := make([]string, 0, 5)
	if ev.Layer != "" {
		parts = append(parts, "layer="+string(ev.Layer))
	}
	if ev.Type != "" {
		parts = append(parts, "type="+ev.Type)
	}
	if ev.SourceArtifact != "" {
		parts = append(parts, "artifact="+ev.SourceArtifact)
	}
	if ev.Confidence != "" {
		parts = append(parts, "confidence="+string(ev.Confidence))
	}
	if ev.Verification != "" {
		parts = append(parts, "verification="+string(ev.Verification))
	}
	if reference, _ := edge.Metadata["reference_target"].(string); reference != "" {
		parts = append(parts, "reference="+reference)
	}
	if bound, _ := edge.Metadata["bound_target"].(string); bound != "" {
		parts = append(parts, "bound="+bound)
	}
	if binding, _ := edge.Metadata["binding_evidence"].(string); binding != "" {
		parts = append(parts, "binding="+binding)
	}
	if suggestions := metadataStringSlice(edge.Metadata["binding_suggestions"]); len(suggestions) > 0 {
		parts = append(parts, "suggestions="+strings.Join(suggestions, ","))
	}
	if len(parts) == 0 {
		return line
	}
	return line + " {" + strings.Join(parts, ", ") + "}"
}

func (e *Engine) describeRef(ref string) string {
	if node, ok := e.nodeByID[ref]; ok {
		return describeNode(node)
	}
	return ref
}

func describeNode(node types.Node) string {
	label := strings.TrimSpace(node.Label)
	if label == "" {
		label = node.ID
	}
	parts := make([]string, 0, 2)
	if layer := nodeLayer(node); layer != "" {
		parts = append(parts, string(layer))
	}
	if kind := strings.TrimSpace(node.NodeType); kind != "" {
		parts = append(parts, kind)
	}
	if len(parts) == 0 {
		return label
	}
	return fmt.Sprintf("%s [%s]", label, strings.Join(parts, "/"))
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (e *Engine) edgeEvidence(source, target, relation string) types.Evidence {
	if e == nil {
		return types.Evidence{}
	}
	if edge, ok := e.edgeByTriple[edgeTriple(source, target, relation)]; ok {
		return types.EdgeEvidence(edge)
	}
	return types.Evidence{}
}

// Route reports workspace-layer repo routing for a query.
func (e *Engine) Route(input string) string {
	if e == nil || e.graph == nil {
		return "no graph loaded"
	}
	routes := igraph.LoadWorkspace(e.graph.Nodes, e.graph.Edges).SelectRepos(routeTokens(input), 5)
	if len(routes) == 0 {
		return fmt.Sprintf("no workspace routes found for %q", input)
	}
	lines := []string{fmt.Sprintf("Workspace routes for %q:", input)}
	for _, route := range routes {
		line := fmt.Sprintf("  %s score=%.2f", route.Repo, route.Score)
		if len(route.Reasons) > 0 {
			line += " reasons=" + strings.Join(route.Reasons, ",")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func routeTokens(input string) []string {
	return strings.FieldsFunc(strings.ToLower(input), func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= '0' && r <= '9':
			return false
		default:
			return true
		}
	})
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
	case "bindings":
		if len(parts) < 2 {
			return "usage: bindings <node>"
		}
		return e.Bindings(strings.Join(parts[1:], " "))
	case "route":
		if len(parts) < 2 {
			return "usage: route <query>"
		}
		return e.Route(strings.Join(parts[1:], " "))
	case "lookup":
		if len(parts) < 2 {
			return "usage: lookup <term>"
		}
		return e.RenderLookup(strings.Join(parts[1:], " "), 5)
	case "nodes":
		return fmt.Sprintf("Total nodes: %d", len(e.graph.Nodes))
	case "edges":
		return fmt.Sprintf("Total edges: %d", len(e.graph.Edges))
	case "help":
		return "Commands:\n  path <from> <to>  — shortest dependency path\n  explain <node>    — all edges involving a node\n  bindings <node>   — memory reference binder state\n  route <query>     — workspace repo routing decision\n  lookup <term>     — candidate nodes for follow-up graph queries\n  nodes / edges     — graph stats\n  quit              — exit"
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
		hasNonZero := false
		for _, node := range e.graph.Nodes {
			if node.Community != 0 {
				hasNonZero = true
				communities[node.Community] = struct{}{}
			}
		}
		if hasNonZero {
			stats.CommunityCount = len(communities)
		}
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
