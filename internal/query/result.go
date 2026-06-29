package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Syfra3/vela/pkg/types"
)

// ResultStatus classifies the shared core query result before adapter rendering.
type ResultStatus string

const (
	ResultStatusOK          ResultStatus = "ok"
	ResultStatusPartial     ResultStatus = "partial"
	ResultStatusUnavailable ResultStatus = "unavailable"
	ResultStatusAmbiguous   ResultStatus = "ambiguous"
	ResultStatusUnresolved  ResultStatus = "unresolved"
	ResultStatusError       ResultStatus = "error"
)

// FreshnessStatus qualifies the runtime graph state used by a result.
type FreshnessStatus string

const (
	FreshnessUnknown FreshnessStatus = "unknown"
	FreshnessFresh   FreshnessStatus = "fresh"
	FreshnessWarming FreshnessStatus = "warming"
	FreshnessStale   FreshnessStatus = "stale"
)

// Result is the minimal adapter-independent envelope for graph-backed answers.
type Result struct {
	SchemaVersion         string           `json:"schema_version"`
	QueryKind             string           `json:"query_kind"`
	Status                ResultStatus     `json:"status"`
	ResolvedSubjects      []Subject        `json:"resolved_subjects,omitempty"`
	Facts                 []Fact           `json:"facts,omitempty"`
	Evidence              []types.Evidence `json:"evidence,omitempty"`
	Confidence            types.Confidence `json:"confidence,omitempty"`
	Freshness             Freshness        `json:"freshness"`
	Diagnostics           []Diagnostic     `json:"diagnostics"`
	Answer                string           `json:"answer,omitempty"`
	RelevantSource        []string         `json:"relevant_source,omitempty"`
	PathsAndRelationships []Fact           `json:"paths_and_relationships,omitempty"`
	ImpactRadius          string           `json:"impact_radius,omitempty"`
	LayeredEvidence       []Fact           `json:"layered_evidence,omitempty"`
	ConfidenceAndLimits   string           `json:"confidence_and_limits,omitempty"`
	SuggestedNextQueries  []string         `json:"suggested_next_queries,omitempty"`
	InterpretedIntent     string           `json:"interpreted_intent,omitempty"`
}

// Subject is a resolved graph subject included in a shared Result.
type Subject struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

// Fact is a graph-backed claim used by a query answer.
type Fact struct {
	Subject    string           `json:"subject"`
	Predicate  string           `json:"predicate"`
	Object     string           `json:"object"`
	Evidence   []types.Evidence `json:"evidence,omitempty"`
	Confidence types.Confidence `json:"confidence,omitempty"`
	Source     string           `json:"source,omitempty"`
	Layer      types.Layer      `json:"layer,omitempty"`
	Metadata   map[string]any   `json:"metadata,omitempty"`
}

// Freshness describes whether graph data used by a Result is fresh enough to trust.
type Freshness struct {
	Status             FreshnessStatus `json:"status"`
	StaleFiles         []string        `json:"stale_files,omitempty"`
	RecommendedActions []string        `json:"recommended_actions,omitempty"`
}

// Diagnostic carries structured warnings or errors without hiding available facts.
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// StatusResult returns structured runtime graph status for adapter responses.
func (e *Engine) StatusResult() Result {
	return Result{
		SchemaVersion: "vela.query.v1",
		QueryKind:     "status",
		Status:        ResultStatusOK,
		Freshness:     e.Freshness(),
	}
}

// DiagnosticResult returns structured adapter diagnostics when a tool cannot run.
func (e *Engine) DiagnosticResult(kind, code, message string) Result {
	return Result{
		SchemaVersion: "vela.query.v1",
		QueryKind:     strings.TrimSpace(kind),
		Status:        ResultStatusError,
		Freshness:     e.Freshness(),
		Diagnostics:   []Diagnostic{{Code: code, Message: message}},
	}
}

// LookupResult returns structured candidate resolution data for MCP agents.
func (e *Engine) LookupResult(term string, limit int) Result {
	result := Result{SchemaVersion: "vela.query.v1", QueryKind: "lookup", Status: ResultStatusOK, Freshness: e.Freshness()}
	for _, candidate := range e.Lookup(term, limit) {
		result.ResolvedSubjects = append(result.ResolvedSubjects, subjectFromNode(candidate.Node))
	}
	if len(result.ResolvedSubjects) == 0 {
		result.Status = ResultStatusUnresolved
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "NO_LOOKUP_CANDIDATES", Message: "no graph-backed lookup candidates were available"})
	}
	return result
}

// ExploreResult returns structured broad-request resolution data for MCP agents.
func (e *Engine) ExploreResult(request string, limit int) Result {
	result := Result{
		SchemaVersion:       "vela.explore.v1",
		QueryKind:           "explore",
		Status:              ResultStatusOK,
		Freshness:           e.Freshness(),
		InterpretedIntent:   exploreIntent(request),
		ImpactRadius:        "not calculated for this explore result",
		ConfidenceAndLimits: "Free-text matching is candidate discovery only, not proof.",
	}
	results := e.Lookup(request, limit)
	nodeSet := make(map[string]bool, len(results))
	for _, candidate := range results {
		result.ResolvedSubjects = append(result.ResolvedSubjects, subjectFromNode(candidate.Node))
		nodeSet[candidate.Node.ID] = true
		if strings.TrimSpace(candidate.Node.SourceFile) != "" {
			result.RelevantSource = append(result.RelevantSource, candidate.Node.SourceFile)
		}
	}
	if len(results) == 0 {
		result.Status = ResultStatusUnresolved
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "NO_GRAPH_BACKED_CANDIDATES", Message: "no graph-backed candidates were available for the request"})
		appendFreshnessDiagnostics(&result)
		return result
	}
	result.Facts = e.exploreFactsForNodeSet(nodeSet)
	result.PathsAndRelationships = result.Facts
	result.LayeredEvidence = result.Facts
	result.Answer = fmt.Sprintf("Resolved graph-backed candidates for %q", request)
	if len(results) > 0 {
		best := results[0].Node.Label
		if strings.TrimSpace(best) == "" {
			best = results[0].Node.ID
		}
		result.SuggestedNextQueries = []string{fmt.Sprintf("vela search \"explain %s\"", best), fmt.Sprintf("vela search \"who uses %s\"", best)}
	}
	if len(results) == 1 {
		appendFreshnessDiagnostics(&result)
		return result
	}
	result.Status = ResultStatusAmbiguous
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "AMBIGUOUS_SUBJECT", Message: fmt.Sprintf("ambiguous subject resolution for %q; refine the request or run `vela lookup`", request)})
	appendFreshnessDiagnostics(&result)
	return result
}

func (e *Engine) exploreFactsForNodeSet(nodeSet map[string]bool) []Fact {
	var facts []Fact
	for _, edge := range e.graph.Edges {
		if !nodeSet[edge.Source] && !nodeSet[edge.Target] {
			continue
		}
		fact := factFromEdge(edge)
		fact.Subject = e.nodeDisplayName(edge.Source)
		fact.Object = e.nodeDisplayName(edge.Target)
		facts = append(facts, fact)
	}
	return facts
}

func (e *Engine) nodeDisplayName(id string) string {
	node, ok := e.nodeByID[id]
	if !ok || strings.TrimSpace(node.Label) == "" {
		return id
	}
	return node.Label
}

// ImpactResult returns structured reverse-dependency facts for an MCP impact query.
func (e *Engine) ImpactResult(subject string, limit int) Result {
	result := Result{SchemaVersion: "vela.query.v1", QueryKind: "impact", Status: ResultStatusOK, Freshness: e.Freshness()}
	nodeIDs := e.resolveNodeIDs(subject)
	if len(nodeIDs) == 0 {
		result.Status = ResultStatusUnresolved
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "SUBJECT_NOT_FOUND", Message: "node not found"})
		return result
	}
	for _, id := range nodeIDs {
		if node, ok := e.nodeByID[id]; ok {
			result.ResolvedSubjects = append(result.ResolvedSubjects, subjectFromNode(node))
		}
	}
	nodeSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}
	for _, edge := range e.graph.Edges {
		if nodeSet[edge.Target] {
			result.Facts = append(result.Facts, factFromEdge(edge))
		}
		if limit > 0 && len(result.Facts) >= limit {
			break
		}
	}
	if len(result.Facts) == 0 {
		result.Status = ResultStatusUnresolved
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "NO_IMPACT_FACTS", Message: "no graph-backed impact facts were available"})
	}
	return result
}

// PathResult returns structured endpoint and path facts for an MCP path query.
func (e *Engine) PathResult(from, to string) Result {
	result := Result{SchemaVersion: "vela.query.v1", QueryKind: "path", Status: ResultStatusOK, Freshness: e.Freshness()}
	fromInt, fromOK := e.resolveToInt(from)
	toInt, toOK := e.resolveToInt(to)
	if !fromOK || !toOK {
		result.Status = ResultStatusUnresolved
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "PATH_ENDPOINT_NOT_FOUND", Message: "one or more path endpoints were not found"})
		return result
	}
	fromID := e.intToID[fromInt]
	toID := e.intToID[toInt]
	for _, id := range []string{fromID, toID} {
		if node, ok := e.nodeByID[id]; ok {
			result.ResolvedSubjects = append(result.ResolvedSubjects, subjectFromNode(node))
		}
	}
	if edge, ok := e.edgeBetween(fromID, toID); ok {
		result.Facts = append(result.Facts, factFromEdge(edge))
		return result
	}
	result.Status = ResultStatusUnresolved
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "NO_GRAPH_BACKED_PATH", Message: "no graph-backed path was available"})
	return result
}

// ExplainResult returns a shared core result for explain answers with proof metadata.
func (e *Engine) ExplainResult(label string) Result {
	result := Result{
		SchemaVersion: "vela.query.v1",
		QueryKind:     "explain",
		Status:        ResultStatusOK,
		Freshness:     e.Freshness(),
	}

	nodeIDs := e.resolveNodeIDs(label)
	if len(nodeIDs) == 0 {
		result.Status = ResultStatusUnresolved
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "SUBJECT_NOT_FOUND", Message: "node not found"})
		return result
	}

	nodeSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeSet[id] = true
		if node, ok := e.nodeByID[id]; ok {
			result.ResolvedSubjects = append(result.ResolvedSubjects, Subject{ID: node.ID, Label: node.Label, Kind: node.NodeType})
		}
	}

	for _, edge := range e.graph.Edges {
		if !nodeSet[edge.Source] && !nodeSet[edge.Target] {
			continue
		}
		evidence := types.EdgeEvidence(edge)
		evidence.Confidence = runtimeCommonIRConfidence(edge.Metadata, evidence.Confidence)
		fact := Fact{
			Subject:    edge.Source,
			Predicate:  edge.Relation,
			Object:     edge.Target,
			Confidence: evidence.Confidence,
			Source:     evidence.SourceArtifact,
			Layer:      evidence.Layer,
			Metadata:   runtimeCommonIRMetadata(edge.Metadata),
		}
		if hasEvidence(evidence) {
			fact.Evidence = []types.Evidence{evidence}
			result.Evidence = append(result.Evidence, evidence)
		}
		if result.Confidence == "" {
			result.Confidence = evidence.Confidence
		}
		result.Facts = append(result.Facts, fact)
	}
	if len(result.Facts) == 0 {
		result.Status = ResultStatusUnresolved
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "NO_GRAPH_BACKED_ANSWER", Message: "no graph-backed answer was available for the requested claim"})
	} else {
		rankInterfaceConflicts(&result)
	}
	appendMissingTestUsageDiagnostics(&result, e, nodeSet)
	appendFreshnessDiagnostics(&result)

	return result
}

func appendFreshnessDiagnostics(result *Result) {
	if result == nil || result.Freshness.Status != FreshnessStale {
		return
	}
	if result.Status == ResultStatusOK {
		result.Status = ResultStatusPartial
	}
	result.Confidence = ""
	message := "runtime graph is stale; run `vela update` or `vela build`"
	if len(result.Freshness.StaleFiles) > 0 {
		message = fmt.Sprintf("runtime graph is stale for %s; run `vela update` or `vela build`", strings.Join(result.Freshness.StaleFiles, ", "))
	}
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "STALE_GRAPH", Message: message})
}

func appendMissingTestUsageDiagnostics(result *Result, e *Engine, nodeSet map[string]bool) {
	if result == nil || e == nil || !explainedNodeSetContainsKind(e, nodeSet, "schema") {
		return
	}
	if graphHasNodeKind(e.graph, "testusage") || graphHasNodeKind(e.graph, "test_usage") || graphHasCoverageEdge(e.graph, nodeSet) {
		return
	}
	if result.Status == ResultStatusOK {
		result.Status = ResultStatusPartial
	}
	result.Diagnostics = append(result.Diagnostics, Diagnostic{
		Code:    "TEST_USAGE_NOT_INDEXED",
		Message: "TestUsage category is unknown/not indexed for this runtime graph; request targeted exploration or rebuild with TestUsage extraction before claiming test coverage is known-empty",
	})
}

func explainedNodeSetContainsKind(e *Engine, nodeSet map[string]bool, kind string) bool {
	for id := range nodeSet {
		node, ok := e.nodeByID[id]
		if ok && strings.EqualFold(node.NodeType, kind) {
			return true
		}
	}
	return false
}

func graphHasNodeKind(g *types.Graph, kind string) bool {
	if g == nil {
		return false
	}
	for _, node := range g.Nodes {
		if strings.EqualFold(node.NodeType, kind) {
			return true
		}
	}
	return false
}

func graphHasCoverageEdge(g *types.Graph, nodeSet map[string]bool) bool {
	if g == nil {
		return false
	}
	for _, edge := range g.Edges {
		if !nodeSet[edge.Source] && !nodeSet[edge.Target] {
			continue
		}
		if strings.EqualFold(edge.Relation, "covered_by_test") || strings.EqualFold(metadataValue(edge.Metadata, "ir_kind"), "COVERED_BY_TEST") {
			return true
		}
	}
	return false
}

func (e *Engine) factsForNodeSet(nodeSet map[string]bool) []Fact {
	var facts []Fact
	for _, edge := range e.graph.Edges {
		if !nodeSet[edge.Source] && !nodeSet[edge.Target] {
			continue
		}
		facts = append(facts, factFromEdge(edge))
	}
	return facts
}

func subjectFromNode(node types.Node) Subject {
	return Subject{ID: node.ID, Label: node.Label, Kind: node.NodeType}
}

func factFromEdge(edge types.Edge) Fact {
	evidence := types.EdgeEvidence(edge)
	evidence.Confidence = runtimeCommonIRConfidence(edge.Metadata, evidence.Confidence)
	fact := Fact{Subject: edge.Source, Predicate: edge.Relation, Object: edge.Target, Confidence: evidence.Confidence, Source: evidence.SourceArtifact, Layer: evidence.Layer, Metadata: runtimeCommonIRMetadata(edge.Metadata)}
	if hasEvidence(evidence) {
		fact.Evidence = []types.Evidence{evidence}
	}
	return fact
}

func runtimeCommonIRMetadata(metadata map[string]any) map[string]any {
	if metadata == nil || metadata["common_ir"] != true {
		return metadata
	}
	normalized := make(map[string]any, len(metadata))
	for key, value := range metadata {
		normalized[key] = value
	}
	if origin := normalizeCommonIROrigin(metadataValue(metadata, "ir_origin")); origin != "" {
		normalized["ir_origin"] = origin
	}
	if confidence := normalizeCommonIRConfidence(metadataValue(metadata, "evidence_confidence")); confidence != "" {
		normalized["evidence_confidence"] = confidence
	}
	return normalized
}

func runtimeCommonIRConfidence(metadata map[string]any, confidence types.Confidence) types.Confidence {
	if metadata == nil || metadata["common_ir"] != true {
		return confidence
	}
	if normalized := normalizeCommonIRConfidence(string(confidence)); normalized != "" {
		return types.Confidence(normalized)
	}
	return confidence
}

func normalizeCommonIROrigin(origin string) string {
	switch origin {
	case "deterministic_extractor", "deterministic":
		return "deterministic"
	case "exploration_enriched", "inferred":
		return origin
	default:
		return origin
	}
}

func normalizeCommonIRConfidence(confidence string) string {
	switch confidence {
	case "declared", "extracted", "high":
		return "high"
	case "inferred", "medium":
		return "medium"
	case "ambiguous", "legacy", "low":
		return "low"
	default:
		return confidence
	}
}

func rankInterfaceConflicts(result *Result) {
	if result == nil || len(result.Facts) < 2 {
		return
	}

	type groupedFact struct {
		index int
		fact  Fact
	}
	groups := make(map[string][]groupedFact)
	for i, fact := range result.Facts {
		key := interfaceConflictKey(fact)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], groupedFact{index: i, fact: fact})
	}

	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		best := group[0].fact
		for _, candidate := range group[1:] {
			if factWeight(candidate.fact) > factWeight(best) {
				best = candidate.fact
			}
		}
		for _, candidate := range group {
			if candidate.fact.Object == best.Object {
				continue
			}
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:    "EVIDENCE_CONFLICT",
				Message: fmt.Sprintf("%s interface evidence outranks conflicting %s evidence for %s", claimStatus(best), claimStatus(candidate.fact), interfaceName(best)),
			})
			break
		}
	}

	sort.SliceStable(result.Facts, func(i, j int) bool {
		return factWeight(result.Facts[i]) > factWeight(result.Facts[j])
	})
	result.Confidence = result.Facts[0].Confidence
}

func interfaceConflictKey(fact Fact) string {
	if fact.Metadata == nil || metadataValue(fact.Metadata, "interface_provider") == "" {
		return ""
	}
	name := interfaceName(fact)
	if name == "" {
		return ""
	}
	return fact.Subject + "|" + fact.Predicate + "|" + name + "|" + metadataValue(fact.Metadata, "interface_route") + "|" + metadataValue(fact.Metadata, "interface_method")
}

func interfaceName(fact Fact) string {
	if fact.Metadata == nil {
		return "interface relationship"
	}
	if name := metadataValue(fact.Metadata, "interface_name"); name != "" {
		return name
	}
	return "interface relationship"
}

func claimStatus(fact Fact) string {
	if fact.Metadata == nil {
		return string(fact.Confidence)
	}
	if status := metadataValue(fact.Metadata, "claim_status"); status != "" {
		return status
	}
	return string(fact.Confidence)
}

func metadataValue(metadata map[string]any, key string) string {
	if value, ok := metadata[key].(string); ok {
		return value
	}
	return ""
}

func factWeight(fact Fact) float64 {
	weight := types.Evidence{Confidence: fact.Confidence}.Weight()
	if fact.Metadata == nil {
		return weight
	}
	switch normalizeCommonIROrigin(metadataValue(fact.Metadata, "ir_origin")) {
	case "deterministic":
		weight += 0.20
	case "exploration_enriched":
		weight -= 0.10
	}
	switch metadataValue(fact.Metadata, "claim_status") {
	case "authoritative":
		weight += 0.10
	case "conflict":
		weight -= 0.10
	}
	return weight
}

// Freshness reports the runtime graph freshness state attached at load time.
func (e *Engine) Freshness() Freshness {
	if e == nil || e.graph == nil || e.graph.Metadata == nil {
		return Freshness{Status: FreshnessUnknown}
	}
	freshness := Freshness{Status: FreshnessUnknown}
	if status, ok := e.graph.Metadata["freshness_status"].(string); ok {
		switch FreshnessStatus(status) {
		case FreshnessFresh, FreshnessStale:
			freshness.Status = FreshnessStatus(status)
		}
	}
	freshness.StaleFiles = metadataStringSlice(e.graph.Metadata["stale_files"])
	freshness.RecommendedActions = metadataStringSlice(e.graph.Metadata["recommended_actions"])
	return freshness
}

func hasEvidence(evidence types.Evidence) bool {
	return evidence.Layer != "" || evidence.Type != "" || evidence.SourceArtifact != "" || evidence.Confidence != "" || evidence.Verification != "" || evidence.Score != 0
}
