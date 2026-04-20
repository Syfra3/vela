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
	igraph "github.com/Syfra3/vela/internal/graph"
	"github.com/Syfra3/vela/internal/retrieval"
	"github.com/Syfra3/vela/pkg/types"
)

const (
	searchSourceAncora     = "ancora"
	searchSourceGraph      = "vela_graph"
	searchSignalLexical    = "lexical"
	searchSignalRouting    = "routing"
	searchSignalStructural = "structural"
	searchSignalVector     = "vector"
)

const (
	graphSeedLimit    = 5
	graphSupportLimit = 3
	graphRouteLimit   = 3
)

var defaultTraversalOptions = retrieval.TraversalOptions{
	MaxHops:       2,
	MaxExpansions: 24,
}

// Searcher runs Ancora-only and federated retrieval against the current graph.
type Searcher struct {
	engine       *Engine
	ancoraDBPath string
	traversal    retrieval.TraversalOptions
}

// SearchProfile selects which retrieval stack to run.
type SearchProfile string

const (
	SearchProfileFederated   SearchProfile = "federated"
	SearchProfileAncora      SearchProfile = "ancora"
	SearchProfileGraph       SearchProfile = "graph"
	SearchProfileGraphHybrid SearchProfile = "graph-hybrid"
	SearchProfileLexical     SearchProfile = "lexical"
	SearchProfileStructural  SearchProfile = "structural"
	SearchProfileVector      SearchProfile = "vector"
)

type graphSignalConfig struct {
	Lexical    bool
	Structural bool
	Vector     bool
}

var defaultGraphSignals = graphSignalConfig{Lexical: true, Structural: true, Vector: true}

// SearchRun is one retrieval execution for a selected profile.
type SearchRun struct {
	Profile SearchProfile   `json:"profile"`
	Query   string          `json:"query"`
	Hits    []SearchHit     `json:"hits"`
	Metrics StrategyMetrics `json:"metrics"`
}

// SearchHit is a ranked retrieval result.
type SearchHit struct {
	ID                 string             `json:"id"`
	CanonicalID        string             `json:"canonical_id,omitempty"`
	Label              string             `json:"label"`
	Kind               string             `json:"kind,omitempty"`
	Path               string             `json:"path,omitempty"`
	Snippet            string             `json:"snippet,omitempty"`
	Support            []string           `json:"support,omitempty"`
	SupportGraph       *SupportGraph      `json:"support_graph,omitempty"`
	Score              float64            `json:"score"`
	PrimarySource      string             `json:"primary_source"`
	PrimaryLayer       string             `json:"primary_layer,omitempty"`
	Sources            []string           `json:"sources"`
	Layers             []string           `json:"layers,omitempty"`
	Contributions      map[string]float64 `json:"contributions,omitempty"`
	Signals            map[string]float64 `json:"signals,omitempty"`
	LayerContributions map[string]float64 `json:"layer_contributions,omitempty"`
	Provenance         []SearchProvenance `json:"provenance,omitempty"`
}

type SearchProvenance struct {
	Layer   string   `json:"layer"`
	Source  string   `json:"source,omitempty"`
	Signal  string   `json:"signal,omitempty"`
	Repo    string   `json:"repo,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
}

type SearchRoute struct {
	Repo    string   `json:"repo"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons,omitempty"`
}

type SearchRouting struct {
	Tokens      []string      `json:"tokens,omitempty"`
	RoutedRepos []SearchRoute `json:"routed_repos,omitempty"`
	Fallback    bool          `json:"fallback"`
}

// SupportGraph is a compact structured subgraph attached to a hit.
type SupportGraph struct {
	Nodes []SupportNode `json:"nodes,omitempty"`
	Edges []SupportEdge `json:"edges,omitempty"`
}

// SupportNode is one supporting node in a result subgraph.
type SupportNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"`
	Path  string `json:"path,omitempty"`
}

// SupportEdge is one supporting edge in a result subgraph.
type SupportEdge struct {
	FromID         string `json:"from_id"`
	ToID           string `json:"to_id"`
	Relation       string `json:"relation"`
	Direction      string `json:"direction,omitempty"`
	Hop            int    `json:"hop,omitempty"`
	EvidenceType   string `json:"evidence_type,omitempty"`
	SourceArtifact string `json:"source_artifact,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	Verification   string `json:"verification,omitempty"`
}

// StrategyMetrics captures signals that matter when comparing retrieval modes.
type StrategyMetrics struct {
	LatencyMs          int64          `json:"latency_ms"`
	Candidates         int            `json:"candidates"`
	Returned           int            `json:"returned"`
	SourceContribution map[string]int `json:"source_contribution"`
	SignalContribution map[string]int `json:"signal_contribution,omitempty"`
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

type VectorRuntime struct {
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	SearchMode       string `json:"search_mode,omitempty"`
	IndexBackend     string `json:"index_backend,omitempty"`
	RequestedBackend string `json:"requested_backend,omitempty"`
	SQLiteVecEnabled bool   `json:"sqlite_vec_enabled,omitempty"`
	SQLiteVecReason  string `json:"sqlite_vec_reason,omitempty"`
	EmbeddingDims    int    `json:"embedding_dims,omitempty"`
}

// SearchMetrics records the baseline and federated retrieval comparison for one query.
type SearchMetrics struct {
	GeneratedAt   string            `json:"generated_at"`
	Query         string            `json:"query"`
	Limit         int               `json:"limit"`
	AncoraOnly    StrategyMetrics   `json:"ancora_only"`
	Federated     StrategyMetrics   `json:"federated"`
	Comparison    ComparisonMetrics `json:"comparison"`
	VectorRuntime *VectorRuntime    `json:"vector_runtime,omitempty"`
}

// SearchResponse is the federated result set plus comparison metrics.
type SearchResponse struct {
	Query   string        `json:"query"`
	Hits    []SearchHit   `json:"hits"`
	Routing SearchRouting `json:"routing,omitempty"`
	Metrics SearchMetrics `json:"metrics"`
}

type scoredHit struct {
	hit   SearchHit
	score float64
}

// NewSearcher wires a graph engine with an optional Ancora DB path.
func NewSearcher(engine *Engine, ancoraDBPath string) *Searcher {
	return &Searcher{engine: engine, ancoraDBPath: ancoraDBPath, traversal: defaultTraversalOptions}
}

// WithTraversal updates graph traversal controls for graph-side search.
func (s *Searcher) WithTraversal(opts retrieval.TraversalOptions) *Searcher {
	if opts.MaxHops <= 0 {
		opts.MaxHops = defaultTraversalOptions.MaxHops
	}
	if opts.MaxExpansions <= 0 {
		opts.MaxExpansions = defaultTraversalOptions.MaxExpansions
	}
	s.traversal = opts
	return s
}

// Search returns federated results and baseline comparison metrics.
func (s *Searcher) Search(input string, limit int) (SearchResponse, error) {
	comp, err := s.computeSearch(input, limit)
	if err != nil {
		return SearchResponse{}, err
	}
	return SearchResponse{
		Query:   comp.query,
		Hits:    comp.federatedHits,
		Routing: comp.routing,
		Metrics: comp.metrics,
	}, nil
}

// RunProfile runs one selected retrieval stack for evaluation/benchmarking.
func (s *Searcher) RunProfile(input string, limit int, profile SearchProfile) (SearchRun, error) {
	comp, err := s.computeSearch(input, limit)
	if err != nil {
		return SearchRun{}, err
	}
	switch profile {
	case SearchProfileAncora:
		return SearchRun{Profile: profile, Query: comp.query, Hits: comp.ancoraHits, Metrics: buildStrategyMetrics(comp.ancoraHits, comp.ancoraCandidates, comp.ancoraLatency)}, nil
	case SearchProfileGraph:
		return SearchRun{Profile: profile, Query: comp.query, Hits: comp.graphHits, Metrics: buildStrategyMetrics(comp.graphHits, comp.graphCandidates, comp.graphLatency)}, nil
	case SearchProfileGraphHybrid, SearchProfileLexical, SearchProfileStructural, SearchProfileVector:
		hits, candidates, latency, err := s.runGraphProfile(comp.query, limit, profile)
		if err != nil {
			return SearchRun{}, err
		}
		return SearchRun{Profile: profile, Query: comp.query, Hits: hits, Metrics: buildStrategyMetrics(hits, candidates, latency)}, nil
	case SearchProfileFederated, "":
		return SearchRun{Profile: SearchProfileFederated, Query: comp.query, Hits: comp.federatedHits, Metrics: buildStrategyMetrics(comp.federatedHits, comp.ancoraCandidates+comp.graphCandidates, comp.federatedLatency)}, nil
	default:
		return SearchRun{}, fmt.Errorf("unknown search profile: %s", profile)
	}
}

func (s *Searcher) runGraphProfile(input string, limit int, profile SearchProfile) ([]SearchHit, int, time.Duration, error) {
	start := time.Now()
	hits, candidates, err := searchGraph(s.engine, input, limit, s.traversal, graphSignalsForProfile(profile))
	return hits, candidates, time.Since(start), err
}

func graphSignalsForProfile(profile SearchProfile) graphSignalConfig {
	switch profile {
	case SearchProfileLexical:
		return graphSignalConfig{Lexical: true}
	case SearchProfileStructural:
		return graphSignalConfig{Structural: true}
	case SearchProfileVector:
		return graphSignalConfig{Vector: true}
	case SearchProfileGraphHybrid, SearchProfileGraph:
		return defaultGraphSignals
	default:
		return defaultGraphSignals
	}
}

type searchComputation struct {
	query string

	ancoraHits       []SearchHit
	ancoraCandidates int
	ancoraLatency    time.Duration

	graphHits       []SearchHit
	graphCandidates int
	graphLatency    time.Duration
	routing         SearchRouting

	federatedHits    []SearchHit
	federatedLatency time.Duration
	metrics          SearchMetrics
}

func (s *Searcher) computeSearch(input string, limit int) (searchComputation, error) {
	queryText := strings.TrimSpace(input)
	if queryText == "" {
		return searchComputation{}, fmt.Errorf("query cannot be empty")
	}
	if limit <= 0 {
		limit = 5
	}

	ancoraDBPath := s.ancoraDBPath
	if ancoraDBPath == "" {
		var err error
		ancoraDBPath, err = ancora.DefaultDBPath()
		if err != nil {
			return searchComputation{}, fmt.Errorf("resolve ancora db: %w", err)
		}
	}

	baselineStart := time.Now()
	ancoraHits, ancoraCandidates, err := searchAncora(ancoraDBPath, queryText, limit)
	if err != nil {
		return searchComputation{}, err
	}
	ancoraLatency := time.Since(baselineStart)

	routing := routeWorkspace(s.engine, queryText)

	graphStart := time.Now()
	graphHits, graphCandidates, err := searchGraph(s.engine, queryText, limit, s.traversal, defaultGraphSignals)
	if err != nil {
		return searchComputation{}, err
	}
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
	if runtime := loadVectorRuntime(s.engine); runtime != nil {
		metrics.VectorRuntime = runtime
	}

	return searchComputation{
		query:            queryText,
		ancoraHits:       ancoraHits,
		ancoraCandidates: ancoraCandidates,
		ancoraLatency:    ancoraLatency,
		graphHits:        graphHits,
		graphCandidates:  graphCandidates,
		graphLatency:     graphLatency,
		routing:          routing,
		federatedHits:    federatedHits,
		federatedLatency: federatedLatency,
		metrics:          metrics,
	}, nil
}

func loadVectorRuntime(engine *Engine) *VectorRuntime {
	if engine == nil || engine.graphPath == "" {
		return nil
	}
	metadata, err := retrieval.LoadMetadata(retrieval.DBPath(filepath.Dir(engine.graphPath)))
	if err != nil {
		return nil
	}
	return &VectorRuntime{
		Provider:         metadata.EmbeddingProvider,
		Model:            metadata.EmbeddingModel,
		SearchMode:       metadata.VectorSearchMode,
		IndexBackend:     metadata.VectorIndex,
		RequestedBackend: metadata.RequestedVectorBackend,
		SQLiteVecEnabled: metadata.SQLiteVecEnabled,
		SQLiteVecReason:  metadata.SQLiteVecReason,
		EmbeddingDims:    metadata.EmbeddingDims,
	}
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
				ID:                 fmt.Sprintf("ancora:obs:%d", obs.ID),
				CanonicalID:        types.CanonicalKey{Layer: types.LayerMemory, Kind: "observation", Key: fmt.Sprintf("ancora:obs:%d", obs.ID)}.String(),
				Label:              label,
				Kind:               obs.Type,
				Path:               obs.Workspace,
				Snippet:            buildSnippet(obs.Content, input),
				Score:              score,
				PrimarySource:      searchSourceAncora,
				PrimaryLayer:       string(types.LayerMemory),
				Sources:            []string{searchSourceAncora},
				Layers:             []string{string(types.LayerMemory)},
				Contributions:      map[string]float64{searchSourceAncora: score},
				LayerContributions: map[string]float64{string(types.LayerMemory): score},
				Provenance:         []SearchProvenance{{Layer: string(types.LayerMemory), Source: searchSourceAncora}},
			},
		})
	}

	sortScoredHits(scored)
	return trimHits(scored, limit), len(scored), nil
}

func searchGraph(engine *Engine, input string, limit int, traversal retrieval.TraversalOptions, signals graphSignalConfig) ([]SearchHit, int, error) {
	if engine == nil || engine.graph == nil {
		return nil, 0, nil
	}
	if engine.graphPath == "" {
		return nil, 0, nil
	}

	routing := routeWorkspace(engine, input)
	workspaceHits, workspaceCandidates := searchWorkspaceLayer(engine, input, limit, routing)
	contractHits, contractCandidates := searchContractLayer(engine, input, limit, routing)

	dbPath, err := retrieval.EnsureGraphSync(engine.graphPath, engine.graph)
	if err != nil {
		return nil, 0, fmt.Errorf("sync graph retrieval substrate: %w", err)
	}

	repoRoutes := routeMap(routing)
	repos := orderedRouteRepos(routing)
	if len(repos) == 0 {
		repos = allCodebaseRepos(engine)
	}
	collections := make([][]SearchHit, 0, len(repos)+2)
	if len(workspaceHits) > 0 {
		collections = append(collections, workspaceHits)
	}
	if len(contractHits) > 0 {
		collections = append(collections, contractHits)
	}
	totalCandidates := workspaceCandidates + contractCandidates
	for _, repo := range repos {
		var (
			results              []retrieval.Result
			fusedLexical         []retrieval.Result
			candidates           int
			vectorResults        []retrieval.Result
			vectorCandidates     int
			structural           []retrieval.StructuralResult
			structuralCandidates int
		)
		opts := retrieval.SearchOptions{Limit: limit, Repo: repo, SourceType: string(types.SourceTypeCodebase)}
		if signals.Lexical || signals.Structural {
			results, candidates, err = retrieval.SearchLexicalWithOptions(dbPath, input, opts)
			if err != nil {
				return nil, 0, fmt.Errorf("search repo retrieval substrate: %w", err)
			}
			if signals.Lexical {
				fusedLexical = results
			}
		}
		if signals.Vector {
			vectorResults, vectorCandidates, err = retrieval.SearchVectorWithOptions(dbPath, input, opts)
			if err != nil {
				return nil, 0, fmt.Errorf("search repo vector substrate: %w", err)
			}
		}
		if signals.Structural {
			seedResults := results
			if len(seedResults) > graphSeedLimit {
				seedResults = seedResults[:graphSeedLimit]
			}
			if len(seedResults) > 0 {
				traversal.Limit = max(limit*2, graphSeedLimit)
				structural, structuralCandidates, err = retrieval.ExpandStructural(dbPath, seedResults, traversal)
				if err != nil {
					return nil, 0, fmt.Errorf("expand repo retrieval substrate: %w", err)
				}
			}
		}
		repoHits, mergedCandidates := fuseGraphSignals(engine, fusedLexical, structural, vectorResults, input, limit, repo, repoRoutes[strings.ToLower(repo)])
		if len(repoHits) > 0 {
			collections = append(collections, repoHits)
		}
		totalCandidates += max(max(candidates, vectorCandidates), max(structuralCandidates, mergedCandidates))
	}
	return mergePreScoredHits(limit, collections...), totalCandidates, nil
}

func fuseGraphSignals(engine *Engine, lexical []retrieval.Result, structural []retrieval.StructuralResult, vector []retrieval.Result, input string, limit int, repo string, route SearchRoute) ([]SearchHit, int) {
	merged := make(map[string]SearchHit, len(lexical)+len(structural))
	merge := func(signal string, rank int, hit SearchHit, score float64) {
		key := joinKey(hit)
		current, ok := merged[key]
		bonus := 1.0 / float64(rank+10)
		contribution := score + bonus
		layer := hit.PrimaryLayer
		if !ok {
			hit.Score = contribution
			hit.PrimarySource = searchSourceGraph
			hit.Sources = []string{searchSourceGraph}
			hit.Layers = appendUnique(hit.Layers, layer)
			hit.Contributions = map[string]float64{searchSourceGraph: contribution}
			hit.Signals = map[string]float64{signal: contribution}
			hit.LayerContributions = map[string]float64{layer: contribution}
			hit.Provenance = appendProvenance(hit.Provenance, SearchProvenance{Layer: layer, Source: searchSourceGraph, Signal: signal, Repo: repo, Reasons: route.Reasons})
			merged[key] = hit
			return
		}
		current.Score += contribution
		if current.Contributions == nil {
			current.Contributions = map[string]float64{}
		}
		current.Contributions[searchSourceGraph] += contribution
		if current.Signals == nil {
			current.Signals = map[string]float64{}
		}
		current.Signals[signal] += contribution
		if current.LayerContributions == nil {
			current.LayerContributions = map[string]float64{}
		}
		current.LayerContributions[layer] += contribution
		current.Layers = appendUnique(current.Layers, layer)
		current.PrimaryLayer = primaryLayer(current.LayerContributions)
		if current.Snippet == "" && hit.Snippet != "" {
			current.Snippet = hit.Snippet
		}
		if current.Path == "" && hit.Path != "" {
			current.Path = hit.Path
		}
		if current.Kind == "" && hit.Kind != "" {
			current.Kind = hit.Kind
		}
		if len(hit.Support) > 0 {
			current.Support = appendUniqueStrings(current.Support, hit.Support, graphSupportLimit)
		}
		if hit.SupportGraph != nil {
			current.SupportGraph = mergeSupportGraphs(current.SupportGraph, hit.SupportGraph)
		}
		current.Provenance = appendProvenance(current.Provenance, SearchProvenance{Layer: layer, Source: searchSourceGraph, Signal: signal, Repo: repo, Reasons: route.Reasons})
		merged[key] = current
	}

	for rank, item := range lexical {
		snippetText := strings.TrimSpace(item.Description)
		if item.MetadataText != "" {
			snippetText = strings.TrimSpace(snippetText + " " + item.MetadataText)
		}
		merge(searchSignalLexical, rank, SearchHit{
			ID:           item.ID,
			CanonicalID:  canonicalSearchID(item.ID, item.Kind),
			Label:        item.Label,
			Kind:         item.Kind,
			Path:         item.Path,
			Snippet:      buildSnippet(snippetText, input),
			PrimaryLayer: string(types.LayerRepo),
		}, item.Score)
	}

	for rank, item := range structural {
		support := formatSupport(item.Context)
		snippetText := strings.TrimSpace(item.Description)
		if snippetText == "" && len(support) > 0 {
			snippetText = support[0]
		}
		merge(searchSignalStructural, rank, SearchHit{
			ID:           item.ID,
			CanonicalID:  canonicalSearchID(item.ID, item.Kind),
			Label:        item.Label,
			Kind:         item.Kind,
			Path:         item.Path,
			Snippet:      buildSnippet(snippetText, input),
			Support:      support,
			SupportGraph: buildSupportGraph(engine, item),
			PrimaryLayer: string(types.LayerRepo),
		}, item.Score)
	}

	for rank, item := range vector {
		snippetText := strings.TrimSpace(item.Description)
		if item.MetadataText != "" {
			snippetText = strings.TrimSpace(snippetText + " " + item.MetadataText)
		}
		merge(searchSignalVector, rank, SearchHit{
			ID:           item.ID,
			CanonicalID:  canonicalSearchID(item.ID, item.Kind),
			Label:        item.Label,
			Kind:         item.Kind,
			Path:         item.Path,
			Snippet:      buildSnippet(snippetText, input),
			PrimaryLayer: string(types.LayerRepo),
		}, item.Score)
	}

	scored := make([]scoredHit, 0, len(merged))
	for _, hit := range merged {
		hit.PrimarySource = searchSourceGraph
		hit.PrimaryLayer = primaryLayer(hit.LayerContributions)
		scored = append(scored, scoredHit{hit: hit, score: hit.Score})
	}
	sortScoredHits(scored)
	return trimHits(scored, limit), len(merged)
}

func fuseHits(ancoraHits, graphHits []SearchHit, limit int) []SearchHit {
	merged := make(map[string]SearchHit, len(ancoraHits)+len(graphHits))
	merge := func(source string, hits []SearchHit) {
		for rank, hit := range hits {
			key := joinKey(hit)
			current, ok := merged[key]
			rrf := 1.0 / float64(rank+10)
			contribution := hit.Score + rrf
			if !ok {
				hit.Score = contribution
				hit.PrimarySource = source
				hit.Sources = []string{source}
				hit.Contributions = map[string]float64{source: contribution}
				if hit.PrimaryLayer != "" {
					hit.Layers = appendUnique(hit.Layers, hit.PrimaryLayer)
					if hit.LayerContributions == nil {
						hit.LayerContributions = map[string]float64{hit.PrimaryLayer: contribution}
					}
				}
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
			current.Layers = appendUniqueStrings(current.Layers, hit.Layers, 0)
			if current.LayerContributions == nil {
				current.LayerContributions = map[string]float64{}
			}
			if hit.PrimaryLayer != "" {
				current.LayerContributions[hit.PrimaryLayer] += contribution
			}
			current.PrimaryLayer = primaryLayer(current.LayerContributions)
			current.Provenance = appendProvenance(current.Provenance, hit.Provenance...)
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

func canonicalSearchID(id, kind string) string { return types.CanonicalJoinKey(id, kind, nil) }

func joinKey(hit SearchHit) string {
	if hit.CanonicalID != "" {
		return hit.CanonicalID
	}
	if key := canonicalSearchID(hit.ID, hit.Kind); key != "" {
		return key
	}
	if hit.ID != "" {
		return hit.ID
	}
	return strings.ToLower(hit.Label)
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
		SignalContribution: map[string]int{},
		RankingComposition: make([]string, 0, len(hits)),
	}
	kinds := make(map[string]struct{})
	for _, hit := range hits {
		metrics.SourceContribution[hit.PrimarySource]++
		metrics.RankingComposition = append(metrics.RankingComposition, hit.PrimarySource)
		if hit.Kind != "" {
			kinds[hit.Kind] = struct{}{}
		}
		for signal := range hit.Signals {
			metrics.SignalContribution[signal]++
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
		baselineIDs[joinKey(hit)] = struct{}{}
	}

	overlap := 0
	addedBySource := map[string]int{}
	for _, hit := range federatedHits {
		if _, ok := baselineIDs[joinKey(hit)]; ok {
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

func appendUniqueStrings(values, additions []string, limit int) []string {
	for _, addition := range additions {
		values = appendUnique(values, addition)
		if limit > 0 && len(values) >= limit {
			return values[:limit]
		}
	}
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
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

func primaryLayer(contributions map[string]float64) string {
	best := ""
	bestScore := -1.0
	for layer, score := range contributions {
		if score > bestScore {
			best = layer
			bestScore = score
		}
	}
	return best
}

func routeWorkspace(engine *Engine, input string) SearchRouting {
	routing := SearchRouting{Tokens: tokenize(input), Fallback: true}
	if engine == nil || engine.graph == nil || len(routing.Tokens) == 0 {
		return routing
	}
	hits := igraph.LoadWorkspace(engine.graph.Nodes, engine.graph.Edges).SelectRepos(routing.Tokens, graphRouteLimit)
	if len(hits) == 0 {
		return routing
	}
	routing.Fallback = false
	routing.RoutedRepos = make([]SearchRoute, 0, len(hits))
	for _, hit := range hits {
		routing.RoutedRepos = append(routing.RoutedRepos, SearchRoute{Repo: hit.Repo, Score: hit.Score, Reasons: hit.Reasons})
	}
	return routing
}

func routeMap(routing SearchRouting) map[string]SearchRoute {
	indexed := make(map[string]SearchRoute, len(routing.RoutedRepos))
	for _, route := range routing.RoutedRepos {
		indexed[strings.ToLower(route.Repo)] = route
	}
	return indexed
}

func orderedRouteRepos(routing SearchRouting) []string {
	out := make([]string, 0, len(routing.RoutedRepos))
	for _, route := range routing.RoutedRepos {
		out = append(out, route.Repo)
	}
	return out
}

func allCodebaseRepos(engine *Engine) []string {
	if engine == nil || engine.graph == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var repos []string
	for _, node := range engine.graph.Nodes {
		name := ""
		if node.Source != nil && node.Source.Type == types.SourceTypeCodebase {
			name = strings.TrimSpace(node.Source.Name)
		}
		if name == "" && node.NodeType == string(types.NodeTypeProject) {
			name = strings.TrimSpace(node.Label)
		}
		if name == "" {
			continue
		}
		lookup := strings.ToLower(name)
		if _, ok := seen[lookup]; ok {
			continue
		}
		seen[lookup] = struct{}{}
		repos = append(repos, name)
	}
	sort.Strings(repos)
	return repos
}

func searchWorkspaceLayer(engine *Engine, input string, limit int, routing SearchRouting) ([]SearchHit, int) {
	routes := routeMap(routing)
	return searchLayerNodes(engine, input, limit, types.LayerWorkspace, func(node types.Node) (SearchProvenance, bool) {
		repo := strings.TrimSpace(node.Label)
		route, ok := routes[strings.ToLower(repo)]
		if ok {
			return SearchProvenance{Layer: string(types.LayerWorkspace), Source: searchSourceGraph, Signal: searchSignalRouting, Repo: route.Repo, Reasons: route.Reasons}, true
		}
		return SearchProvenance{Layer: string(types.LayerWorkspace), Source: searchSourceGraph, Signal: searchSignalRouting}, true
	})
}

func searchContractLayer(engine *Engine, input string, limit int, routing SearchRouting) ([]SearchHit, int) {
	routes := routeMap(routing)
	bindings := contractRepoBindings(engine)
	hasRoutes := len(routes) > 0
	return searchLayerNodes(engine, input, limit, types.LayerContract, func(node types.Node) (SearchProvenance, bool) {
		repos := bindings[node.ID]
		if hasRoutes {
			matched := false
			for _, repo := range repos {
				if _, ok := routes[strings.ToLower(repo)]; ok {
					matched = true
					break
				}
			}
			if !matched {
				return SearchProvenance{}, false
			}
		}
		for _, repo := range repos {
			if route, ok := routes[strings.ToLower(repo)]; ok {
				return SearchProvenance{Layer: string(types.LayerContract), Source: searchSourceGraph, Signal: searchSignalLexical, Repo: route.Repo, Reasons: route.Reasons}, true
			}
		}
		prov := SearchProvenance{Layer: string(types.LayerContract), Source: searchSourceGraph, Signal: searchSignalLexical}
		if len(repos) > 0 {
			prov.Repo = repos[0]
		}
		return prov, true
	})
}

func searchLayerNodes(engine *Engine, input string, limit int, layer types.Layer, provenance func(types.Node) (SearchProvenance, bool)) ([]SearchHit, int) {
	if engine == nil || engine.graph == nil {
		return nil, 0
	}
	var scored []scoredHit
	for _, node := range engine.graph.Nodes {
		if nodeLayer(node) != layer {
			continue
		}
		prov, ok := provenance(node)
		if !ok {
			continue
		}
		score := scoreQuery(input,
			weightedField{Text: node.Label, Weight: 5},
			weightedField{Text: node.NodeType, Weight: 3},
			weightedField{Text: node.SourceFile, Weight: 2},
			weightedField{Text: node.Description, Weight: 2},
			weightedField{Text: metadataSearchText(node.Metadata), Weight: 1},
		)
		if score <= 0 {
			continue
		}
		hit := SearchHit{
			ID:                 node.ID,
			CanonicalID:        canonicalSearchID(node.ID, node.NodeType),
			Label:              node.Label,
			Kind:               node.NodeType,
			Path:               node.SourceFile,
			Snippet:            buildSnippet(strings.TrimSpace(node.Description+" "+metadataSearchText(node.Metadata)), input),
			Score:              score,
			PrimarySource:      searchSourceGraph,
			PrimaryLayer:       string(layer),
			Sources:            []string{searchSourceGraph},
			Layers:             []string{string(layer)},
			Contributions:      map[string]float64{searchSourceGraph: score},
			Signals:            map[string]float64{prov.Signal: score},
			LayerContributions: map[string]float64{string(layer): score},
			Provenance:         []SearchProvenance{prov},
		}
		scored = append(scored, scoredHit{hit: hit, score: score})
	}
	sortScoredHits(scored)
	return trimHits(scored, limit), len(scored)
}

func nodeLayer(node types.Node) types.Layer {
	if key := types.CanonicalKeyForNode(node); !key.IsZero() {
		return key.Layer
	}
	if node.Source != nil {
		return types.LayerOf(node.Source.Type)
	}
	return ""
}

func metadataSearchText(metadata map[string]interface{}) string {
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

func contractRepoBindings(engine *Engine) map[string][]string {
	if engine == nil || engine.graph == nil {
		return nil
	}
	projectNames := map[string]string{}
	for _, node := range engine.graph.Nodes {
		if node.NodeType != string(types.NodeTypeProject) {
			continue
		}
		name := strings.TrimSpace(node.Label)
		if node.Source != nil && strings.TrimSpace(node.Source.Name) != "" {
			name = strings.TrimSpace(node.Source.Name)
		}
		if name != "" {
			projectNames[node.ID] = name
		}
	}
	serviceRepos := map[string][]string{}
	children := map[string][]string{}
	for _, edge := range engine.graph.Edges {
		switch edge.Relation {
		case "declared_in":
			if repo, ok := projectNames[edge.Target]; ok {
				serviceRepos[edge.Source] = appendUnique(serviceRepos[edge.Source], repo)
			}
		case "declares":
			children[edge.Source] = appendUnique(children[edge.Source], edge.Target)
		}
	}
	bindings := map[string][]string{}
	for service, repos := range serviceRepos {
		bindings[service] = append([]string(nil), repos...)
		for _, child := range children[service] {
			bindings[child] = appendUniqueStrings(bindings[child], repos, 0)
		}
	}
	return bindings
}

func mergePreScoredHits(limit int, collections ...[]SearchHit) []SearchHit {
	merged := make(map[string]SearchHit)
	for _, hits := range collections {
		for _, hit := range hits {
			key := joinKey(hit)
			current, ok := merged[key]
			if !ok {
				merged[key] = hit
				continue
			}
			current.Score += hit.Score
			current.Sources = appendUniqueStrings(current.Sources, hit.Sources, 0)
			current.Layers = appendUniqueStrings(current.Layers, hit.Layers, 0)
			mergeFloatMaps(current.Contributions, hit.Contributions)
			mergeFloatMaps(current.Signals, hit.Signals)
			mergeFloatMaps(current.LayerContributions, hit.LayerContributions)
			current.PrimarySource = primarySource(current.Contributions)
			current.PrimaryLayer = primaryLayer(current.LayerContributions)
			if current.Snippet == "" {
				current.Snippet = hit.Snippet
			}
			if current.Path == "" {
				current.Path = hit.Path
			}
			if current.Kind == "" {
				current.Kind = hit.Kind
			}
			current.Support = appendUniqueStrings(current.Support, hit.Support, graphSupportLimit)
			current.SupportGraph = mergeSupportGraphs(current.SupportGraph, hit.SupportGraph)
			current.Provenance = appendProvenance(current.Provenance, hit.Provenance...)
			merged[key] = current
		}
	}
	scored := make([]scoredHit, 0, len(merged))
	for _, hit := range merged {
		scored = append(scored, scoredHit{hit: hit, score: hit.Score})
	}
	sortScoredHits(scored)
	return trimHits(scored, limit)
}

func mergeFloatMaps(dst, src map[string]float64) {
	if dst == nil || src == nil {
		return
	}
	for key, value := range src {
		dst[key] += value
	}
}

func appendProvenance(values []SearchProvenance, additions ...SearchProvenance) []SearchProvenance {
	seen := map[string]struct{}{}
	for _, value := range values {
		seen[fmt.Sprintf("%s|%s|%s|%s|%s", value.Layer, value.Source, value.Signal, value.Repo, strings.Join(value.Reasons, ","))] = struct{}{}
	}
	for _, addition := range additions {
		key := fmt.Sprintf("%s|%s|%s|%s|%s", addition.Layer, addition.Source, addition.Signal, addition.Repo, strings.Join(addition.Reasons, ","))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, addition)
	}
	return values
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatSupport(context []retrieval.EdgeContext) []string {
	if len(context) == 0 {
		return nil
	}
	limit := min(len(context), graphSupportLimit)
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		edge := context[i]
		if edge.Direction == "in" {
			lines = append(lines, fmt.Sprintf("%s <-[%s]- %s (hop %d)", edge.FromLabel, edge.Relation, edge.ToLabel, edge.Hop))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s -[%s]-> %s (hop %d)", edge.FromLabel, edge.Relation, edge.ToLabel, edge.Hop))
	}
	return lines
}

func buildSupportGraph(engine *Engine, item retrieval.StructuralResult) *SupportGraph {
	if len(item.Context) == 0 {
		return nil
	}
	graph := &SupportGraph{}
	seenNodes := map[string]struct{}{}
	for i, edge := range item.Context {
		if i >= graphSupportLimit {
			break
		}
		if edge.FromID != "" {
			if _, ok := seenNodes[edge.FromID]; !ok {
				graph.Nodes = append(graph.Nodes, SupportNode{ID: edge.FromID, Label: edge.FromLabel})
				seenNodes[edge.FromID] = struct{}{}
			}
		}
		if edge.ToID != "" {
			if _, ok := seenNodes[edge.ToID]; !ok {
				kind := ""
				path := ""
				if edge.ToID == item.ID {
					kind = item.Kind
					path = item.Path
				}
				graph.Nodes = append(graph.Nodes, SupportNode{ID: edge.ToID, Label: edge.ToLabel, Kind: kind, Path: path})
				seenNodes[edge.ToID] = struct{}{}
			}
		}
		ev := types.Evidence{}
		if engine != nil {
			ev = engine.edgeEvidence(edge.FromID, edge.ToID, edge.Relation)
		}
		graph.Edges = append(graph.Edges, SupportEdge{
			FromID:         edge.FromID,
			ToID:           edge.ToID,
			Relation:       edge.Relation,
			Direction:      edge.Direction,
			Hop:            edge.Hop,
			EvidenceType:   ev.Type,
			SourceArtifact: ev.SourceArtifact,
			Confidence:     string(ev.Confidence),
			Verification:   string(ev.Verification),
		})
	}
	if len(graph.Nodes) == 0 && len(graph.Edges) == 0 {
		return nil
	}
	return graph
}

func mergeSupportGraphs(current, incoming *SupportGraph) *SupportGraph {
	if current == nil {
		return incoming
	}
	if incoming == nil {
		return current
	}
	merged := &SupportGraph{
		Nodes: append([]SupportNode{}, current.Nodes...),
		Edges: append([]SupportEdge{}, current.Edges...),
	}
	nodeSeen := make(map[string]struct{}, len(merged.Nodes))
	for _, node := range merged.Nodes {
		nodeSeen[node.ID] = struct{}{}
	}
	for _, node := range incoming.Nodes {
		if _, ok := nodeSeen[node.ID]; ok {
			continue
		}
		merged.Nodes = append(merged.Nodes, node)
		nodeSeen[node.ID] = struct{}{}
	}
	edgeSeen := make(map[string]struct{}, len(merged.Edges))
	for _, edge := range merged.Edges {
		edgeSeen[fmt.Sprintf("%s|%s|%s|%d", edge.FromID, edge.ToID, edge.Relation, edge.Hop)] = struct{}{}
	}
	for _, edge := range incoming.Edges {
		key := fmt.Sprintf("%s|%s|%s|%d", edge.FromID, edge.ToID, edge.Relation, edge.Hop)
		if _, ok := edgeSeen[key]; ok {
			continue
		}
		merged.Edges = append(merged.Edges, edge)
		edgeSeen[key] = struct{}{}
	}
	return merged
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
