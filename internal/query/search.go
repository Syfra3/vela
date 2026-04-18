package query

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Syfra3/vela/internal/ancora"
)

const (
	searchSourceAncora = "ancora"
	searchSourceGraph  = "vela_graph"
)

// Searcher runs Ancora-only and federated retrieval against the current graph.
type Searcher struct {
	engine       *Engine
	ancoraDBPath string
}

// SearchHit is a ranked retrieval result.
type SearchHit struct {
	ID            string             `json:"id"`
	Label         string             `json:"label"`
	Kind          string             `json:"kind,omitempty"`
	Path          string             `json:"path,omitempty"`
	Snippet       string             `json:"snippet,omitempty"`
	Score         float64            `json:"score"`
	PrimarySource string             `json:"primary_source"`
	Sources       []string           `json:"sources"`
	Contributions map[string]float64 `json:"contributions,omitempty"`
}

// StrategyMetrics captures signals that matter when comparing retrieval modes.
type StrategyMetrics struct {
	LatencyMs          int64          `json:"latency_ms"`
	Candidates         int            `json:"candidates"`
	Returned           int            `json:"returned"`
	SourceContribution map[string]int `json:"source_contribution"`
	RankingComposition []string       `json:"ranking_composition"`
	DistinctKinds      int            `json:"distinct_kinds"`
	HybridResults      int            `json:"hybrid_results,omitempty"`
}

// ComparisonMetrics highlights what federated retrieval changed vs the Ancora baseline.
type ComparisonMetrics struct {
	OverlapAtK       int            `json:"overlap_at_k"`
	OverlapRatio     float64        `json:"overlap_ratio"`
	AddedByFederated int            `json:"added_by_federated"`
	AddedBySource    map[string]int `json:"added_by_source"`
	AncoraLatencyGap int64          `json:"ancora_latency_gap_ms"`
	GraphLatencyMs   int64          `json:"graph_latency_ms"`
}

// SearchMetrics records the baseline and federated retrieval comparison for one query.
type SearchMetrics struct {
	GeneratedAt string            `json:"generated_at"`
	Query       string            `json:"query"`
	Limit       int               `json:"limit"`
	AncoraOnly  StrategyMetrics   `json:"ancora_only"`
	Federated   StrategyMetrics   `json:"federated"`
	Comparison  ComparisonMetrics `json:"comparison"`
}

// SearchResponse is the federated result set plus comparison metrics.
type SearchResponse struct {
	Query   string        `json:"query"`
	Hits    []SearchHit   `json:"hits"`
	Metrics SearchMetrics `json:"metrics"`
}

type scoredHit struct {
	hit   SearchHit
	score float64
}

// NewSearcher wires a graph engine with an optional Ancora DB path.
func NewSearcher(engine *Engine, ancoraDBPath string) *Searcher {
	return &Searcher{engine: engine, ancoraDBPath: ancoraDBPath}
}

// Search returns federated results and baseline comparison metrics.
func (s *Searcher) Search(input string, limit int) (SearchResponse, error) {
	queryText := strings.TrimSpace(input)
	if queryText == "" {
		return SearchResponse{}, fmt.Errorf("query cannot be empty")
	}
	if limit <= 0 {
		limit = 5
	}

	ancoraDBPath := s.ancoraDBPath
	if ancoraDBPath == "" {
		var err error
		ancoraDBPath, err = ancora.DefaultDBPath()
		if err != nil {
			return SearchResponse{}, fmt.Errorf("resolve ancora db: %w", err)
		}
	}

	baselineStart := time.Now()
	ancoraHits, ancoraCandidates, err := searchAncora(ancoraDBPath, queryText, limit)
	if err != nil {
		return SearchResponse{}, err
	}
	ancoraLatency := time.Since(baselineStart)

	graphStart := time.Now()
	graphHits, graphCandidates := searchGraph(s.engine, queryText, limit)
	graphLatency := time.Since(graphStart)

	federatedStart := time.Now()
	federatedHits := fuseHits(ancoraHits, graphHits, limit)
	federatedLatency := time.Since(federatedStart) + ancoraLatency + graphLatency

	metrics := SearchMetrics{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Query:       queryText,
		Limit:       limit,
		AncoraOnly:  buildStrategyMetrics(ancoraHits, ancoraCandidates, ancoraLatency),
		Federated:   buildStrategyMetrics(federatedHits, ancoraCandidates+graphCandidates, federatedLatency),
		Comparison:  buildComparisonMetrics(ancoraHits, federatedHits, ancoraLatency, federatedLatency, graphLatency, limit),
	}

	return SearchResponse{
		Query:   queryText,
		Hits:    federatedHits,
		Metrics: metrics,
	}, nil
}

func searchAncora(dbPath, input string, limit int) ([]SearchHit, int, error) {
	r, err := ancora.Open(dbPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open ancora db: %w", err)
	}
	defer r.Close()

	observations, err := r.AllObservations("", "")
	if err != nil {
		return nil, 0, fmt.Errorf("read observations: %w", err)
	}

	var scored []scoredHit
	for _, obs := range observations {
		score := scoreQuery(input,
			weightedField{Text: obs.Title, Weight: 5},
			weightedField{Text: obs.Type, Weight: 3},
			weightedField{Text: obs.Workspace, Weight: 2},
			weightedField{Text: obs.Organization, Weight: 2},
			weightedField{Text: obs.TopicKey, Weight: 2},
			weightedField{Text: obs.Content, Weight: 1},
		)
		if score <= 0 {
			continue
		}
		label := obs.Title
		if label == "" {
			label = fmt.Sprintf("obs-%d", obs.ID)
		}
		scored = append(scored, scoredHit{
			score: score,
			hit: SearchHit{
				ID:            fmt.Sprintf("ancora:obs:%d", obs.ID),
				Label:         label,
				Kind:          obs.Type,
				Path:          obs.Workspace,
				Snippet:       buildSnippet(obs.Content, input),
				Score:         score,
				PrimarySource: searchSourceAncora,
				Sources:       []string{searchSourceAncora},
				Contributions: map[string]float64{searchSourceAncora: score},
			},
		})
	}

	sortScoredHits(scored)
	return trimHits(scored, limit), len(scored), nil
}

func searchGraph(engine *Engine, input string, limit int) ([]SearchHit, int) {
	if engine == nil || engine.graph == nil {
		return nil, 0
	}

	var scored []scoredHit
	for _, node := range engine.graph.Nodes {
		score := scoreQuery(input,
			weightedField{Text: node.Label, Weight: 5},
			weightedField{Text: node.NodeType, Weight: 3},
			weightedField{Text: node.SourceFile, Weight: 2},
			weightedField{Text: node.Description, Weight: 2},
			weightedField{Text: flattenMetadata(node.Metadata), Weight: 1},
		)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredHit{
			score: score,
			hit: SearchHit{
				ID:            node.ID,
				Label:         node.Label,
				Kind:          node.NodeType,
				Path:          node.SourceFile,
				Snippet:       buildSnippet(node.Description, input),
				Score:         score,
				PrimarySource: searchSourceGraph,
				Sources:       []string{searchSourceGraph},
				Contributions: map[string]float64{searchSourceGraph: score},
			},
		})
	}

	sortScoredHits(scored)
	return trimHits(scored, limit), len(scored)
}

func fuseHits(ancoraHits, graphHits []SearchHit, limit int) []SearchHit {
	merged := make(map[string]SearchHit, len(ancoraHits)+len(graphHits))
	merge := func(source string, hits []SearchHit) {
		for rank, hit := range hits {
			key := hit.ID
			if key == "" {
				key = strings.ToLower(hit.Label)
			}
			current, ok := merged[key]
			rrf := 1.0 / float64(rank+10)
			contribution := hit.Score + rrf
			if !ok {
				hit.Score = contribution
				hit.Contributions = map[string]float64{source: contribution}
				merged[key] = hit
				continue
			}
			current.Score += contribution
			current.Sources = appendUnique(current.Sources, source)
			if current.Contributions == nil {
				current.Contributions = map[string]float64{}
			}
			current.Contributions[source] += contribution
			current.PrimarySource = primarySource(current.Contributions)
			if current.Snippet == "" && hit.Snippet != "" {
				current.Snippet = hit.Snippet
			}
			if current.Path == "" && hit.Path != "" {
				current.Path = hit.Path
			}
			if current.Kind == "" && hit.Kind != "" {
				current.Kind = hit.Kind
			}
			merged[key] = current
		}
	}

	merge(searchSourceAncora, ancoraHits)
	merge(searchSourceGraph, graphHits)

	scored := make([]scoredHit, 0, len(merged))
	for _, hit := range merged {
		hit.PrimarySource = primarySource(hit.Contributions)
		scored = append(scored, scoredHit{hit: hit, score: hit.Score})
	}
	sortScoredHits(scored)
	return trimHits(scored, limit)
}

type weightedField struct {
	Text   string
	Weight float64
}

func scoreQuery(input string, fields ...weightedField) float64 {
	query := normalizeText(input)
	if query == "" {
		return 0
	}
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return 0
	}

	total := 0.0
	matchedTokens := make(map[string]bool, len(tokens))
	for _, field := range fields {
		text := normalizeText(field.Text)
		if text == "" {
			continue
		}
		if strings.Contains(text, query) {
			total += 3 * field.Weight
		}
		for _, token := range tokens {
			if matchToken(text, token) {
				total += field.Weight
				matchedTokens[token] = true
			}
		}
	}
	if len(matchedTokens) == len(tokens) {
		total += 2
	}
	return total
}

func matchToken(text, token string) bool {
	for _, variant := range tokenVariants(token) {
		if strings.Contains(text, variant) {
			return true
		}
	}
	return false
}

func tokenVariants(token string) []string {
	variants := []string{token}
	if len(token) > 3 && strings.HasSuffix(token, "es") {
		variants = append(variants, token[:len(token)-2])
	}
	if len(token) > 2 && strings.HasSuffix(token, "s") {
		variants = append(variants, token[:len(token)-1])
	}
	return variants
}

func buildSnippet(text, input string) string {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return ""
	}
	const maxLen = 160
	if len(clean) <= maxLen {
		return clean
	}
	needle := ""
	for _, token := range tokenize(input) {
		if len(token) > len(needle) {
			needle = token
		}
	}
	if needle == "" {
		return clean[:maxLen]
	}
	idx := strings.Index(strings.ToLower(clean), strings.ToLower(needle))
	if idx < 0 {
		return clean[:maxLen]
	}
	start := idx - maxLen/3
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(clean) {
		end = len(clean)
	}
	return strings.TrimSpace(clean[start:end])
}

func buildStrategyMetrics(hits []SearchHit, candidates int, latency time.Duration) StrategyMetrics {
	metrics := StrategyMetrics{
		LatencyMs:          latency.Milliseconds(),
		Candidates:         candidates,
		Returned:           len(hits),
		SourceContribution: map[string]int{},
		RankingComposition: make([]string, 0, len(hits)),
	}
	kinds := make(map[string]struct{})
	for _, hit := range hits {
		metrics.SourceContribution[hit.PrimarySource]++
		metrics.RankingComposition = append(metrics.RankingComposition, hit.PrimarySource)
		if hit.Kind != "" {
			kinds[hit.Kind] = struct{}{}
		}
		if len(hit.Sources) > 1 {
			metrics.HybridResults++
		}
	}
	metrics.DistinctKinds = len(kinds)
	return metrics
}

func buildComparisonMetrics(ancoraHits, federatedHits []SearchHit, ancoraLatency, federatedLatency, graphLatency time.Duration, limit int) ComparisonMetrics {
	baselineIDs := make(map[string]struct{}, min(limit, len(ancoraHits)))
	for _, hit := range ancoraHits {
		baselineIDs[hit.ID] = struct{}{}
	}

	overlap := 0
	addedBySource := map[string]int{}
	for _, hit := range federatedHits {
		if _, ok := baselineIDs[hit.ID]; ok {
			overlap++
			continue
		}
		addedBySource[hit.PrimarySource]++
	}
	denom := min(limit, len(federatedHits))
	ratio := 0.0
	if denom > 0 {
		ratio = float64(overlap) / float64(denom)
	}
	return ComparisonMetrics{
		OverlapAtK:       overlap,
		OverlapRatio:     ratio,
		AddedByFederated: len(federatedHits) - overlap,
		AddedBySource:    addedBySource,
		AncoraLatencyGap: federatedLatency.Milliseconds() - ancoraLatency.Milliseconds(),
		GraphLatencyMs:   graphLatency.Milliseconds(),
	}
}

func trimHits(scored []scoredHit, limit int) []SearchHit {
	if limit > len(scored) {
		limit = len(scored)
	}
	hits := make([]SearchHit, 0, limit)
	for i := 0; i < limit; i++ {
		hits = append(hits, scored[i].hit)
	}
	return hits
}

func sortScoredHits(scored []scoredHit) {
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return strings.ToLower(scored[i].hit.Label) < strings.ToLower(scored[j].hit.Label)
		}
		return scored[i].score > scored[j].score
	})
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func primarySource(contributions map[string]float64) string {
	best := ""
	bestScore := -1.0
	for source, score := range contributions {
		if score > bestScore {
			best = source
			bestScore = score
		}
	}
	if best == "" && len(contributions) > 1 {
		return "hybrid"
	}
	return best
}

func normalizeText(input string) string {
	input = strings.ToLower(input)
	return strings.Join(tokenize(input), " ")
}

func tokenize(input string) []string {
	return strings.FieldsFunc(input, func(r rune) bool {
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

func flattenMetadata(metadata map[string]interface{}) string {
	if len(metadata) == 0 {
		return ""
	}
	parts := make([]string, 0, len(metadata))
	for key, value := range metadata {
		parts = append(parts, key+" "+fmt.Sprint(value))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// WriteMetricsSnapshot persists one retrieval comparison under ~/.vela/retrieval-history.
func WriteMetricsSnapshot(metrics SearchMetrics) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	dir := filepath.Join(home, ".vela", "retrieval-history")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating retrieval history dir: %w", err)
	}
	name := sanitizeSnapshotName(metrics.Query)
	if name == "" {
		name = "query"
	}
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.json", name, timestamp))
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal metrics: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write metrics snapshot: %w", err)
	}
	return path, nil
}

func sanitizeSnapshotName(input string) string {
	tokens := tokenize(strings.ToLower(input))
	if len(tokens) == 0 {
		return ""
	}
	return strings.Join(tokens, "-")
}
