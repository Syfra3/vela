package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// RunRequest executes the reduced graph-truth query contract used by the new
// CLI, TUI, server, and MCP surfaces.
func (e *Engine) RunRequest(req types.QueryRequest) (string, error) {
	req = req.Normalize()
	if err := req.Validate(); err != nil {
		return "", err
	}

	subjectID := e.resolveNodeID(req.Subject, "")
	if subjectID == "" {
		return "", fmt.Errorf("node %q not found", req.Subject)
	}
	subjectNode, hasSubjectNode := e.nodeByID[subjectID]

	switch req.Kind {
	case types.QueryKindDependencies:
		return e.renderReachability(req.Subject, subjectID, req.Limit, outgoingEdges, "Dependencies", isFileNode(subjectNode) && hasSubjectNode), nil
	case types.QueryKindReverseDependencies:
		return e.renderReachability(req.Subject, subjectID, req.Limit, incomingEdges, "Reverse dependencies", isFileNode(subjectNode) && hasSubjectNode), nil
	case types.QueryKindImpact:
		return e.renderImpact(req.Subject, subjectID, req.Limit), nil
	case types.QueryKindPath:
		return e.Path(req.Subject, req.Target), nil
	case types.QueryKindExplain:
		return e.Explain(req.Subject), nil
	default:
		return "", fmt.Errorf("unsupported query kind %q", req.Kind)
	}
}

type edgeDirection int

const (
	outgoingEdges edgeDirection = iota
	incomingEdges
)

func (e *Engine) renderReachability(subject, subjectID string, limit int, direction edgeDirection, heading string, fileOnly bool) string {
	lines := []string{fmt.Sprintf("%s for %q:", heading, subject)}
	results := e.collectReachability(subjectID, limit, direction, fileOnly)
	if len(results) == 0 {
		return strings.Join(append(lines, "  (none)"), "\n")
	}
	for _, line := range results {
		lines = append(lines, "  - "+line)
	}
	return strings.Join(lines, "\n")
}

func (e *Engine) renderImpact(subject, subjectID string, limit int) string {
	results := e.collectImpact(subjectID, limit)
	lines := []string{fmt.Sprintf("Impact for %q:", subject)}
	if len(results) == 0 {
		return strings.Join(append(lines, "  (none)"), "\n")
	}
	for _, line := range results {
		lines = append(lines, "  - "+line)
	}
	return strings.Join(lines, "\n")
}

func (e *Engine) collectReachability(subjectID string, limit int, direction edgeDirection, fileOnly bool) []string {
	if limit <= 0 {
		limit = types.DefaultQueryLimit
	}
	if fileOnly {
		return e.collectFileReachability(subjectID, limit, direction)
	}
	visited := map[string]bool{subjectID: true}
	queue := []string{subjectID}
	results := make([]string, 0, limit)

	for len(queue) > 0 && len(results) < limit {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range e.graph.Edges {
			nextID, ok := traverseEdge(edge, current, direction)
			if !ok || visited[nextID] {
				continue
			}
			visited[nextID] = true
			queue = append(queue, nextID)
			results = append(results, fmt.Sprintf("%s via %s", e.describeRef(nextID), edge.Relation))
			if len(results) >= limit {
				break
			}
		}
	}

	sort.Strings(results)
	return results
}

func (e *Engine) collectFileReachability(subjectID string, limit int, direction edgeDirection) []string {
	type candidate struct {
		id    string
		edge  types.Edge
		label string
		score int
	}
	candidates := make([]candidate, 0)
	seen := map[string]bool{}
	for _, edge := range e.graph.Edges {
		if !isFileDependencyEdge(edge, e.nodeByID) {
			continue
		}
		nextID, ok := traverseEdge(edge, subjectID, direction)
		if !ok || seen[nextID] {
			continue
		}
		seen[nextID] = true
		candidates = append(candidates, candidate{
			id:    nextID,
			edge:  edge,
			label: fmt.Sprintf("%s via %s", e.describeRef(nextID), edge.Relation),
			score: fileReachabilityScore(nextID, edge, e.nodeByID),
		})
	}
	if len(candidates) == 0 {
		return nil
	}
	filtered := candidates
	static := make([]candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if isStaticFileDependency(candidate.edge) {
			static = append(static, candidate)
		}
	}
	if len(static) > 0 {
		filtered = static
	}
	nonTest := make([]candidate, 0, len(filtered))
	for _, candidate := range filtered {
		path := strings.ToLower(strings.TrimSpace(e.nodeByID[candidate.id].SourceFile))
		if strings.Contains(path, "_test.go") || strings.Contains(path, ".test.") || strings.Contains(path, ".spec.") || strings.Contains(path, "bench") {
			continue
		}
		nonTest = append(nonTest, candidate)
	}
	if len(nonTest) >= 2 {
		filtered = nonTest
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].score != filtered[j].score {
			return filtered[i].score > filtered[j].score
		}
		return filtered[i].label < filtered[j].label
	})
	results := make([]string, 0, limit)
	for _, candidate := range filtered {
		results = append(results, candidate.label)
		if len(results) >= limit {
			break
		}
	}
	return results
}

func (e *Engine) collectImpact(subjectID string, limit int) []string {
	if limit <= 0 {
		limit = types.DefaultQueryLimit
	}
	results := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, node := range e.graph.Nodes {
		if node.ID == subjectID || seen[node.ID] {
			continue
		}
		pathText := e.Path(node.ID, subjectID)
		if strings.Contains(pathText, "no path found") || strings.Contains(pathText, "not found") {
			continue
		}
		seen[node.ID] = true
		results = append(results, fmt.Sprintf("%s via %s", describeNode(node), pathText))
		if len(results) >= limit {
			break
		}
	}
	sort.Strings(results)
	return results
}

func traverseEdge(edge types.Edge, current string, direction edgeDirection) (string, bool) {
	switch direction {
	case outgoingEdges:
		if edge.Source == current {
			return edge.Target, true
		}
	case incomingEdges:
		if edge.Target == current {
			return edge.Source, true
		}
	}
	return "", false
}

func isFileNode(node types.Node) bool {
	return node.NodeType == string(types.NodeTypeFile)
}

func isFileDependencyEdge(edge types.Edge, nodeByID map[string]types.Node) bool {
	if edge.Relation == string(types.FactKindContains) {
		return false
	}
	src, srcOK := nodeByID[edge.Source]
	tgt, tgtOK := nodeByID[edge.Target]
	return srcOK && tgtOK && isFileNode(src) && isFileNode(tgt)
}

func isStaticFileDependency(edge types.Edge) bool {
	projectedFrom, _ := edge.Metadata["projected_from"].(string)
	return projectedFrom == "static_import" || projectedFrom == "workspace_package"
}

func fileReachabilityScore(nodeID string, edge types.Edge, nodeByID map[string]types.Node) int {
	node, ok := nodeByID[nodeID]
	if !ok {
		return 0
	}
	score := 0
	path := strings.ToLower(strings.TrimSpace(node.SourceFile))
	if isStaticFileDependency(edge) {
		score += 100
	}
	if strings.HasPrefix(path, "cmd/") {
		score += 50
	}
	if strings.Contains(path, "/server/") || strings.Contains(path, "/auth/") {
		score += 40
	}
	if strings.Contains(path, "/presentation/") || strings.Contains(path, "/shared/contexts/") {
		score += 35
	}
	for _, token := range []string{"context", "hook", "service", "domain", "reader", "main.go", "server.go"} {
		if strings.Contains(path, token) {
			score += 20
		}
	}
	for _, token := range []string{"/domain/", "/kitchen/", "/dashboard/"} {
		if strings.Contains(path, token) {
			score -= 15
		}
	}
	for _, token := range []string{"_test.go", ".spec.", ".test.", "/components/", "/page.tsx", "/index.tsx"} {
		if strings.Contains(path, token) {
			score -= 25
		}
	}
	if strings.HasPrefix(path, "apps/mobile/") {
		score += 15
	}
	if strings.HasPrefix(path, "apps/desktop-pos/") {
		score += 10
	}
	if strings.HasPrefix(path, "apps/web-portal/") {
		score -= 5
	}
	return score
}
