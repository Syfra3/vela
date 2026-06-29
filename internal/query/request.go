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

type reachabilityCandidate struct {
	id    string
	edge  types.Edge
	label string
	score int
}

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
			results = append(results, e.formatReachabilityEdge(nextID, edge))
			if len(results) >= limit {
				break
			}
		}
	}

	sort.Strings(results)
	return results
}

func (e *Engine) collectFileReachability(subjectID string, limit int, direction edgeDirection) []string {
	candidates := make([]reachabilityCandidate, 0)
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
		candidates = append(candidates, reachabilityCandidate{
			id:    nextID,
			edge:  edge,
			label: e.formatReachabilityEdge(nextID, edge),
			score: fileReachabilityScore(nextID, edge, e.nodeByID),
		})
	}
	if len(candidates) == 0 {
		return nil
	}
	filtered := candidates
	static := make([]reachabilityCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if isStaticFileDependency(candidate.edge) {
			static = append(static, candidate)
		}
	}
	if len(static) > 0 {
		filtered = static
	}
	nonTest := make([]reachabilityCandidate, 0, len(filtered))
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
	if preferred := preferExternalBarrelCallers(subjectID, direction, filtered, e.nodeByID); len(preferred) >= 2 {
		filtered = preferred
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

func preferExternalBarrelCallers(subjectID string, direction edgeDirection, candidates []reachabilityCandidate, nodeByID map[string]types.Node) []reachabilityCandidate {
	if direction != incomingEdges {
		return nil
	}
	subject, ok := nodeByID[subjectID]
	if !ok {
		return nil
	}
	root := packageBarrelRoot(subject.SourceFile)
	if root == "" {
		return nil
	}
	preferred := make([]reachabilityCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		caller, ok := nodeByID[candidate.id]
		if !ok {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(caller.SourceFile)), root) {
			preferred = append(preferred, candidate)
		}
	}
	return preferred
}

func packageBarrelRoot(path string) string {
	path = strings.ToLower(strings.TrimSpace(path))
	if !strings.HasPrefix(path, "packages/") {
		return ""
	}
	for _, suffix := range []string{"/src/index.ts", "/src/index.tsx", "/src/index.js", "/src/index.jsx"} {
		if strings.HasSuffix(path, suffix) {
			trimmed := strings.TrimSuffix(path, suffix)
			if trimmed != "" {
				return trimmed + "/"
			}
		}
	}
	return ""
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
		line := fmt.Sprintf("%s via %s", describeNode(node), pathText)
		if edge, ok := e.edgeBetween(node.ID, subjectID); ok {
			line += formatImpactCompatibilityMetadata(node, edge)
		}
		results = append(results, line)
		if len(results) >= limit {
			break
		}
	}
	sort.Strings(results)
	return results
}

func formatImpactCompatibilityMetadata(node types.Node, edge types.Edge) string {
	if edge.Metadata == nil {
		return ""
	}
	parts := make([]string, 0, 5)
	if backing := edgeBackingLabel(edge); backing != "" {
		parts = append(parts, "backing="+backing)
	}
	if stableID := metadataValue(edge.Metadata, "stable_id"); stableID != "" {
		parts = append(parts, "stable_id="+stableID)
	}
	if strings.TrimSpace(node.ID) != "" {
		parts = append(parts, "impacted_id="+node.ID)
	}
	if artifact := types.EdgeEvidence(edge).SourceArtifact; artifact != "" {
		parts = append(parts, "source="+artifact)
	}
	if snippet := metadataValue(edge.Metadata, "evidence_snippet"); snippet != "" {
		parts = append(parts, "evidence="+snippet)
	}
	diagnostics := legacyUnavailableDiagnostics(edge.Metadata)
	if len(diagnostics) > 0 {
		parts = append(parts, "diagnostics="+strings.Join(diagnostics, "; "))
	}
	if len(parts) == 0 {
		return ""
	}
	return " {" + strings.Join(parts, ", ") + "}"
}

func legacyUnavailableDiagnostics(metadata map[string]interface{}) []string {
	keys := []string{"source_file_hash", "source_range", "extractor_version"}
	diagnostics := make([]string, 0, len(keys))
	for _, key := range keys {
		value := metadataValue(metadata, key)
		if strings.Contains(value, "legacy-unavailable") {
			diagnostics = append(diagnostics, value)
		}
	}
	return diagnostics
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

func (e *Engine) formatReachabilityEdge(nodeID string, edge types.Edge) string {
	line := fmt.Sprintf("%s via %s", e.describeRef(nodeID), edge.Relation)
	if edge.Metadata == nil {
		return line
	}
	parts := make([]string, 0, 4)
	if backing := edgeBackingLabel(edge); backing != "" {
		parts = append(parts, "backing="+backing)
	}
	if edge.Metadata["common_ir"] != true {
		if len(parts) == 0 {
			return line
		}
		return line + " {" + strings.Join(parts, ", ") + "}"
	}
	if kind, _ := edge.Metadata["ir_kind"].(string); kind != "" {
		parts = append(parts, "kind="+kind)
	}
	if origin := normalizeCommonIROrigin(metadataValue(edge.Metadata, "ir_origin")); origin != "" {
		parts = append(parts, "origin="+origin)
	}
	if confidence := normalizeCommonIRConfidence(metadataValue(edge.Metadata, "evidence_confidence")); confidence != "" {
		parts = append(parts, "confidence="+confidence)
	}
	if freshness, _ := edge.Metadata["freshness"].(string); freshness != "" {
		parts = append(parts, "freshness="+freshness)
	}
	if len(parts) == 0 {
		return line
	}
	return line + " {" + strings.Join(parts, ", ") + "}"
}

func formatPersistedEnrichmentReuseMetadata(metadata map[string]any) []string {
	if metadata == nil || metadata["common_ir"] != true || normalizeCommonIROrigin(metadataValue(metadata, "ir_origin")) != "exploration_enriched" {
		return nil
	}
	parts := make([]string, 0, 9)
	if kind := metadataValue(metadata, "ir_kind"); kind != "" {
		parts = append(parts, "kind="+kind)
	}
	parts = append(parts, "origin=exploration_enriched")
	if confidence := normalizeCommonIRConfidence(metadataValue(metadata, "evidence_confidence")); confidence != "" {
		parts = append(parts, "confidence="+confidence)
	}
	if freshness := metadataValue(metadata, "freshness"); freshness != "" {
		parts = append(parts, "freshness="+freshness)
		if freshness == "fresh" {
			parts = append(parts, "exploration_reused=true")
		}
	}
	if sourceHash := metadataValue(metadata, "source_file_hash"); sourceHash != "" {
		parts = append(parts, "source_hash="+sourceHash)
	}
	if lastSeen := metadataValue(metadata, "last_seen_at"); lastSeen != "" {
		parts = append(parts, "last_seen="+lastSeen)
	}
	if snippet := metadataValue(metadata, "evidence_snippet"); snippet != "" {
		parts = append(parts, "evidence="+snippet)
	}
	if sourceRange := metadataValue(metadata, "source_range"); sourceRange != "" {
		parts = append(parts, "range="+sourceRange)
	}
	if extractor := persistedEnrichmentExtractor(metadata); extractor != "" {
		parts = append(parts, "extractor="+extractor)
	}
	return parts
}

func persistedEnrichmentExtractor(metadata map[string]any) string {
	name := metadataValue(metadata, "extractor_name")
	version := metadataValue(metadata, "extractor_version")
	if name == "" {
		return version
	}
	if version == "" {
		return name
	}
	return name + "@" + version
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
