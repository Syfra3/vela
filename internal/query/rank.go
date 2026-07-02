package query

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

const (
	DefaultRankLimit       = 10
	DefaultRankExamples    = 3
	DefaultSummaryExamples = 5
)

// RankResult returns a compact scoped ranking over graph nodes/files/modules.
func (e *Engine) RankResult(scope, metric string, limit, examples int) Result {
	if limit <= 0 {
		limit = DefaultRankLimit
	}
	if examples <= 0 {
		examples = DefaultRankExamples
	}
	metric = normalizeRankMetric(metric)
	result := Result{SchemaVersion: "vela.query.v1", QueryKind: "rank", Status: ResultStatusOK, Freshness: e.Freshness(), InterpretedIntent: metric}
	nodes := e.scopedRankNodes(scope)
	if len(nodes) == 0 {
		result.Status = ResultStatusUnresolved
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "NO_RANK_CANDIDATES", Message: "no graph-backed rank candidates matched the requested scope"})
		appendFreshnessDiagnostics(&result)
		return result
	}
	candidates := make([]RankCandidate, 0, len(nodes))
	for _, node := range nodes {
		metrics := e.rankMetrics(node.ID)
		candidates = append(candidates, RankCandidate{
			Subject:         subjectFromNode(node),
			Path:            node.SourceFile,
			Metrics:         metrics,
			OptionalMetrics: map[string]string{"cross_package_consumers": "unavailable", "cross_app_consumers": "unavailable"},
			Examples:        e.rankExamples(node.ID, examples),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := metricValue(candidates[i].Metrics, metric), metricValue(candidates[j].Metrics, metric)
		if left != right {
			return left > right
		}
		if candidates[i].Subject.Label != candidates[j].Subject.Label {
			return candidates[i].Subject.Label < candidates[j].Subject.Label
		}
		return candidates[i].Subject.ID < candidates[j].Subject.ID
	})
	if len(candidates) > limit {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "RANK_TRUNCATED", Message: fmt.Sprintf("rank output truncated to %d candidates; pass a larger limit only when explicitly needed", limit)})
		candidates = candidates[:limit]
	}
	result.Rankings = candidates
	result.Answer = fmt.Sprintf("Ranked %d graph candidates by %s; metrics are split into in_degree, out_degree, total_degree, and downstream_count.", len(candidates), metric)
	result.Gaps = []string{"HTTP route/client extraction is not implemented in this iteration", "cross_package_consumers and cross_app_consumers are unavailable for this graph"}
	appendFreshnessDiagnostics(&result)
	return result
}

// HotspotsResult maps ergonomic hotspot intent onto RankResult without hiding metric ambiguity.
func (e *Engine) HotspotsResult(intent, scope string, limit, examples int) Result {
	metric := hotspotMetric(intent)
	result := e.RankResult(scope, metric, limit, examples)
	result.QueryKind = "hotspots"
	result.InterpretedIntent = strings.TrimSpace(intent)
	if result.InterpretedIntent == "" {
		result.InterpretedIntent = "highest impact"
	}
	ambiguity := "Hotspot is ambiguous: highest impact may mean in_degree (depended-on), out_degree (many dependencies), total_degree (centrality), or downstream_count (blast radius). This result ranks by " + metric + " and exposes every metric separately."
	if result.Answer == "" {
		result.Answer = ambiguity
	} else {
		result.Answer += " " + ambiguity
	}
	return result
}

// ModuleSummaryResult returns compact counts and bounded examples for an exact node/path.
func (e *Engine) ModuleSummaryResult(target string, examples int) Result {
	if examples <= 0 {
		examples = DefaultSummaryExamples
	}
	result := Result{SchemaVersion: "vela.query.v1", QueryKind: "module_summary", Status: ResultStatusOK, Freshness: e.Freshness()}
	target = strings.TrimSpace(target)
	nodes := e.summaryCandidates(target)
	if len(nodes) == 0 {
		result.Status = ResultStatusUnresolved
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "SUBJECT_NOT_FOUND", Message: "node or path not found"})
		return result
	}
	if len(nodes) > 1 {
		result.Status = ResultStatusAmbiguous
		for _, node := range nodes {
			result.ResolvedSubjects = append(result.ResolvedSubjects, subjectFromNode(node))
		}
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "AMBIGUOUS_SUBJECT", Message: "summary target matched multiple candidates; choose an exact id or path"})
		return result
	}
	node := nodes[0]
	metrics := e.rankMetrics(node.ID)
	result.ResolvedSubjects = []Subject{subjectFromNode(node)}
	result.Metrics = &metrics
	result.Examples = e.rankExamples(node.ID, examples)
	result.Facts = result.Examples
	result.ConfidenceAndLimits = "confidence: graph-backed structural counts; examples are bounded and full edge dumps require explicit explain/full-edge request"
	result.Gaps = []string{"HTTP route/client extraction is documented for future implementation but not extracted here", "optional cross-package/app consumer counts unavailable unless present in graph metadata"}
	result.Answer = fmt.Sprintf("Summary for %s: in_degree=%d out_degree=%d total_degree=%d downstream_count=%d; examples=%d/%d.", node.Label, metrics.InDegree, metrics.OutDegree, metrics.TotalDegree, metrics.DownstreamCount, len(result.Examples), examples)
	appendFreshnessDiagnostics(&result)
	return result
}

func (e *Engine) scopedRankNodes(scope string) []types.Node {
	scope = strings.TrimSpace(scope)
	var nodes []types.Node
	for _, node := range e.graph.Nodes {
		if scope == "" || nodeMatchesScope(node, scope) {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func nodeMatchesScope(node types.Node, scope string) bool {
	path := strings.TrimSpace(node.SourceFile)
	if path == "" {
		path = strings.TrimSpace(node.ID)
	}
	if path == scope || node.ID == scope || node.Label == scope {
		return true
	}
	normalized := filepath.ToSlash(scope)
	if scopeHasGlob(normalized) {
		return globMatches(normalized, filepath.ToSlash(path))
	}
	return strings.Contains(strings.ToLower(filepath.ToSlash(path)), strings.ToLower(filepath.ToSlash(normalized)))
}

func scopeHasGlob(scope string) bool {
	return strings.ContainsAny(scope, "*?[")
}

func globMatches(pattern, path string) bool {
	expr := globRegexp(pattern)
	matched, err := regexp.MatchString(expr, path)
	return err == nil && matched
}

func globRegexp(pattern string) string {
	var builder strings.Builder
	builder.WriteString("^")
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				index++
				if index+1 < len(pattern) && pattern[index+1] == '/' {
					index++
					builder.WriteString("(?:.*/)?")
				} else {
					builder.WriteString(".*")
				}
				continue
			}
			builder.WriteString("[^/]*")
		case '?':
			builder.WriteString("[^/]")
		case '[':
			end := index + 1
			for end < len(pattern) && pattern[end] != ']' {
				end++
			}
			if end < len(pattern) {
				class := pattern[index+1 : end]
				if strings.HasPrefix(class, "!") {
					class = "^" + class[1:]
				}
				builder.WriteString("[")
				builder.WriteString(class)
				builder.WriteString("]")
				index = end
			} else {
				builder.WriteString(regexp.QuoteMeta(string(pattern[index])))
			}
		default:
			builder.WriteString(regexp.QuoteMeta(string(pattern[index])))
		}
	}
	builder.WriteString("$")
	return builder.String()
}

func (e *Engine) rankMetrics(id string) RankMetrics {
	metrics := RankMetrics{}
	for _, edge := range e.graph.Edges {
		if edge.Source == id {
			metrics.OutDegree++
		}
		if edge.Target == id {
			metrics.InDegree++
		}
	}
	metrics.TotalDegree = metrics.InDegree + metrics.OutDegree
	metrics.DownstreamCount = e.downstreamCount(id)
	return metrics
}

func (e *Engine) downstreamCount(id string) int {
	seen := map[string]bool{id: true}
	queue := []string{id}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range e.graph.Edges {
			if edge.Source != current || seen[edge.Target] {
				continue
			}
			seen[edge.Target] = true
			queue = append(queue, edge.Target)
		}
	}
	return len(seen) - 1
}

func (e *Engine) rankExamples(id string, limit int) []Fact {
	if limit <= 0 {
		return nil
	}
	facts := make([]Fact, 0, limit)
	for _, edge := range e.graph.Edges {
		if edge.Source == id || edge.Target == id {
			facts = append(facts, factFromEdge(edge))
			if len(facts) >= limit {
				break
			}
		}
	}
	return facts
}

func normalizeRankMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "in", "incoming", "depended-on", "depended_on", "in_degree":
		return "in_degree"
	case "out", "outgoing", "dependencies", "out_degree":
		return "out_degree"
	case "blast", "blast_radius", "downstream", "downstream_count", "impact":
		return "downstream_count"
	case "total", "central", "centrality", "total_degree":
		return "total_degree"
	default:
		return "downstream_count"
	}
}

func hotspotMetric(intent string) string {
	text := strings.ToLower(intent)
	switch {
	case strings.Contains(text, "depended") || strings.Contains(text, "used by"):
		return "in_degree"
	case strings.Contains(text, "dependencies") || strings.Contains(text, "depends on"):
		return "out_degree"
	case strings.Contains(text, "central"):
		return "total_degree"
	default:
		return "downstream_count"
	}
}

func metricValue(metrics RankMetrics, metric string) int {
	switch metric {
	case "in_degree":
		return metrics.InDegree
	case "out_degree":
		return metrics.OutDegree
	case "total_degree":
		return metrics.TotalDegree
	default:
		return metrics.DownstreamCount
	}
}

func (e *Engine) summaryCandidates(target string) []types.Node {
	if target == "" {
		return nil
	}
	if node, ok := e.nodeByID[target]; ok {
		return []types.Node{node}
	}
	var matches []types.Node
	for _, node := range e.graph.Nodes {
		if node.Label == target || node.SourceFile == target {
			matches = append(matches, node)
		}
	}
	if len(matches) > 0 {
		return matches
	}
	return e.nodeCandidates(target)
}
