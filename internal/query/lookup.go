package query

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/pkg/types"
)

// LookupCandidate is a ranked node suggestion for broad discovery queries.
type LookupCandidate struct {
	Node  types.Node
	Score int
}

// Lookup returns ranked node candidates for a broad term without pretending to
// answer a structural graph question directly.
func (e *Engine) Lookup(term string, limit int) []LookupCandidate {
	if e == nil || e.graph == nil {
		return nil
	}
	term = strings.TrimSpace(term)
	if term == "" {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}

	normalized := strings.ToLower(term)
	tokens := lookupTokens(normalized)
	if len(tokens) == 0 {
		return nil
	}

	results := make([]LookupCandidate, 0, limit)
	for _, node := range e.graph.Nodes {
		score := lookupScore(node, normalized, tokens)
		if score == 0 {
			continue
		}
		results = append(results, LookupCandidate{Node: node, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].Node.Label == results[j].Node.Label {
				return results[i].Node.ID < results[j].Node.ID
			}
			return results[i].Node.Label < results[j].Node.Label
		}
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// RenderLookup formats ranked candidates with enough metadata to pick an exact
// subject for follow-up structural graph queries.
func (e *Engine) RenderLookup(term string, limit int) string {
	results := e.Lookup(term, limit)
	if len(results) == 0 {
		return fmt.Sprintf("No candidates found for %q.", term)
	}

	lines := []string{fmt.Sprintf("Candidates for %q:", term), ""}
	for i, candidate := range results {
		node := candidate.Node
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, describeNode(node)))
		lines = append(lines, fmt.Sprintf("   id: %s", node.ID))
		if file := strings.TrimSpace(node.SourceFile); file != "" && file != node.Label {
			lines = append(lines, fmt.Sprintf("   file: %s", file))
		}
	}

	best := results[0].Node.Label
	if strings.TrimSpace(best) == "" {
		best = results[0].Node.ID
	}
	lines = append(lines, "", "Next steps:")
	lines = append(lines, fmt.Sprintf("- vela search \"explain %s\"", best))
	lines = append(lines, fmt.Sprintf("- vela search \"who uses %s\"", best))
	return strings.Join(lines, "\n")
}

// RenderExplore resolves broad natural language to candidate graph subjects and
// cites only graph relationships as proof for the returned context.
func (e *Engine) RenderExplore(request string, limit int) string {
	if routed := e.renderRouteFirstExplore(request, limit); routed != "" {
		return routed
	}

	results := e.Lookup(request, limit)
	if len(results) == 0 {
		return fmt.Sprintf("No graph-backed candidates found for %q.", request)
	}
	if len(results) > 1 {
		return renderAmbiguousExplore(request, results)
	}

	lines := []string{fmt.Sprintf("Resolved candidates for %q:", request), ""}
	candidateIDs := make(map[string]struct{}, len(results))
	for i, candidate := range results {
		node := candidate.Node
		candidateIDs[node.ID] = struct{}{}
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, describeNode(node)))
		lines = append(lines, fmt.Sprintf("   id: %s", node.ID))
	}

	facts := make([]string, 0)
	seen := map[string]struct{}{}
	for _, edge := range e.graph.Edges {
		if _, ok := candidateIDs[edge.Source]; !ok {
			if _, ok := candidateIDs[edge.Target]; !ok {
				continue
			}
		}
		fact := e.formatExplainEdge(edge)
		if _, ok := seen[fact]; ok {
			continue
		}
		seen[fact] = struct{}{}
		facts = append(facts, fact)
	}

	lines = append(lines, "", "Graph facts used:")
	if len(facts) == 0 {
		lines = append(lines, "  none yet; choose a candidate and run a structural query")
	} else {
		for _, fact := range facts {
			lines = append(lines, "  "+fact)
		}
	}
	lines = append(lines, "", "Free-text matching is candidate discovery only, not proof.")
	return strings.Join(lines, "\n")
}

func (e *Engine) renderRouteFirstExplore(request string, limit int) string {
	if e == nil || e.graph == nil {
		return ""
	}
	workspace := igraph.LoadWorkspace(e.graph.Nodes, e.graph.Edges)
	routes := workspace.SelectRepos(routeTokens(request), limit)
	if len(routes) == 0 {
		return ""
	}

	lines := []string{fmt.Sprintf("Workspace routes for %q:", request)}
	if len(routes) > 1 {
		lines = append(lines, "Route ambiguity: multiple workspace routes match; refine the route before making a strong cross-repo claim.")
	}
	for _, route := range routes {
		line := fmt.Sprintf("  %s score=%.2f", route.Repo, route.Score)
		if len(route.Reasons) > 0 {
			line += " reasons=" + strings.Join(route.Reasons, ",")
		}
		lines = append(lines, line)
	}

	lines = append(lines, "", "Workspace routing facts:")
	for _, fact := range e.workspaceRoutingFacts(routes) {
		lines = append(lines, "  "+fact)
	}
	lines = append(lines, "Selected workspace routes are routing/topology truth, not deep code truth.")

	deep := e.deepLookupCandidates(request, limit)
	lines = append(lines, "", "Deep graph retrieval candidates:")
	if len(deep) == 0 {
		lines = append(lines, "  none yet; route is known but deep graph candidates were not resolved")
	} else {
		for i, candidate := range deep {
			node := candidate.Node
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, describeNode(node)))
			lines = append(lines, fmt.Sprintf("   id: %s", node.ID))
			if file := strings.TrimSpace(node.SourceFile); file != "" {
				lines = append(lines, fmt.Sprintf("   file: %s", file))
			}
		}
	}
	lines = append(lines, "", "Free-text matching is candidate discovery only, not proof.")
	return strings.Join(lines, "\n")
}

func (e *Engine) workspaceRoutingFacts(routes []igraph.RepoRouteHit) []string {
	routeRepos := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		routeRepos[igraph.WorkspaceRepoID(route.Repo)] = struct{}{}
	}
	routeServices := make(map[string]struct{})
	for _, edge := range e.graph.Edges {
		if edge.Metadata["layer"] != string(types.LayerWorkspace) {
			continue
		}
		if _, ok := routeRepos[edge.Source]; ok && edge.Relation == igraph.WorkspaceRelExposes {
			routeServices[edge.Target] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	facts := make([]string, 0)
	for _, edge := range e.graph.Edges {
		if edge.Metadata["layer"] != string(types.LayerWorkspace) {
			continue
		}
		_, sourceRepo := routeRepos[edge.Source]
		_, sourceService := routeServices[edge.Source]
		_, targetService := routeServices[edge.Target]
		if !sourceRepo && !(sourceService && targetService) {
			continue
		}
		fact := e.formatExplainEdge(edge)
		if _, ok := seen[fact]; ok {
			continue
		}
		seen[fact] = struct{}{}
		facts = append(facts, fact)
	}
	if len(facts) == 0 {
		return []string{"none"}
	}
	return facts
}

func (e *Engine) deepLookupCandidates(request string, limit int) []LookupCandidate {
	results := e.Lookup(request, limit)
	deep := make([]LookupCandidate, 0, len(results))
	for _, candidate := range results {
		if nodeLayer(candidate.Node) == types.LayerWorkspace {
			continue
		}
		deep = append(deep, candidate)
	}
	return deep
}

func renderAmbiguousExplore(request string, results []LookupCandidate) string {
	lines := []string{fmt.Sprintf("Ambiguous explore query for %q", request), "", "Candidate nodes:"}
	for i, candidate := range results {
		node := candidate.Node
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, describeNode(node)))
		lines = append(lines, fmt.Sprintf("   id: %s", node.ID))
		if file := strings.TrimSpace(node.SourceFile); file != "" {
			lines = append(lines, fmt.Sprintf("   file: %s", file))
		}
	}
	lines = append(lines, "", fmt.Sprintf("Refine the request or run `vela lookup %q` before asking for a strong graph claim.", request))
	return strings.Join(lines, "\n")
}

func lookupTokens(input string) []string {
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '/' && r != '.' && r != '_' && r != '-'
	})
	unique := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		unique = append(unique, part)
	}
	return unique
}

func lookupScore(node types.Node, term string, tokens []string) int {
	label := strings.ToLower(strings.TrimSpace(node.Label))
	id := strings.ToLower(strings.TrimSpace(node.ID))
	file := strings.ToLower(strings.TrimSpace(node.SourceFile))
	description := strings.ToLower(strings.TrimSpace(node.Description))

	score := 0
	switch {
	case label == term || id == term:
		score += 100
	case file == term:
		score += 95
	case canonicalPathSuffix(file) == canonicalPathSuffix(term) && canonicalPathSuffix(term) != "":
		score += 85
	case strings.Contains(label, term):
		score += 70
	case strings.Contains(file, term):
		score += 65
	case strings.Contains(id, term):
		score += 60
	}

	matchedTokens := 0
	for _, token := range tokens {
		tokenMatched := false
		switch {
		case strings.Contains(label, token):
			score += 18
			tokenMatched = true
		case strings.Contains(file, token):
			score += 16
			tokenMatched = true
		case strings.Contains(id, token):
			score += 14
			tokenMatched = true
		case strings.Contains(description, token):
			score += 6
			tokenMatched = true
		}
		if tokenMatched {
			matchedTokens++
		}
	}
	if matchedTokens == len(tokens) && len(tokens) > 1 {
		score += 20
	}
	if matchedTokens == 0 {
		return 0
	}

	switch node.NodeType {
	case string(types.NodeTypeFile):
		if looksPathLike(term) {
			score += 12
		}
	default:
		if !looksPathLike(term) {
			score += 8
		}
	}

	if strings.Contains(file, "/test") || strings.Contains(file, ".test.") || strings.Contains(file, "_test.") {
		score -= 10
	}
	return score
}

func looksPathLike(input string) bool {
	return strings.Contains(input, "/") || strings.Contains(input, ".")
}

func canonicalPathSuffix(input string) string {
	input = filepath.ToSlash(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	parts := strings.Split(input, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return input
}
