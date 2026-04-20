package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	ancoradb "github.com/Syfra3/vela/internal/ancora"
	"github.com/Syfra3/vela/internal/config"
	"github.com/Syfra3/vela/internal/query"
	"github.com/Syfra3/vela/internal/retrieval"
)

type retrievalBenchmarkSuite struct {
	Name  string                   `json:"name"`
	Limit int                      `json:"limit,omitempty"`
	Cases []retrievalBenchmarkCase `json:"cases"`
}

type retrievalBenchmarkCase struct {
	Name           string             `json:"name"`
	Query          string             `json:"query"`
	RelevantID     []string           `json:"relevant_ids"`
	RelevantLabels []string           `json:"relevant_labels,omitempty"`
	Relevance      map[string]float64 `json:"relevance,omitempty"`
}

type retrievalBenchmarkResult struct {
	GeneratedAt  string                             `json:"generated_at"`
	Suite        string                             `json:"suite"`
	SuitePath    string                             `json:"suite_path"`
	GraphPath    string                             `json:"graph_path"`
	Limit        int                                `json:"limit"`
	Profiles     []query.SearchProfile              `json:"profiles"`
	Settings     retrievalBenchmarkSettings         `json:"settings"`
	BaselinePath string                             `json:"baseline_path,omitempty"`
	Summary      map[string]retrievalProfileSummary `json:"summary"`
	Deltas       map[string]retrievalProfileDelta   `json:"deltas,omitempty"`
	Queries      []retrievalQueryEvaluation         `json:"queries"`
}

type retrievalBenchmarkSettings struct {
	MaxHops       int      `json:"max_hops,omitempty"`
	MaxExpansions int      `json:"max_expansions,omitempty"`
	Relations     []string `json:"relations,omitempty"`
}

type retrievalProfileSummary struct {
	Queries                int     `json:"queries"`
	RecallAtK              float64 `json:"recall_at_k"`
	MRR                    float64 `json:"mrr"`
	NDCGAtK                float64 `json:"ndcg_at_k,omitempty"`
	AvgLatencyMs           float64 `json:"avg_latency_ms"`
	P95LatencyMs           int64   `json:"p95_latency_ms"`
	AvgOverlapWithAncora   float64 `json:"avg_overlap_with_ancora,omitempty"`
	AvgAddedVsAncora       float64 `json:"avg_added_vs_ancora,omitempty"`
	AvgReturned            float64 `json:"avg_returned"`
	QueriesWithAnyRelevant int     `json:"queries_with_any_relevant"`
}

type retrievalProfileDelta struct {
	RecallAtKDelta            float64 `json:"recall_at_k_delta,omitempty"`
	MRRDelta                  float64 `json:"mrr_delta,omitempty"`
	NDCGAtKDelta              float64 `json:"ndcg_at_k_delta,omitempty"`
	AvgLatencyMsDelta         float64 `json:"avg_latency_ms_delta,omitempty"`
	P95LatencyMsDelta         int64   `json:"p95_latency_ms_delta,omitempty"`
	AvgOverlapWithAncoraDelta float64 `json:"avg_overlap_with_ancora_delta,omitempty"`
	AvgAddedVsAncoraDelta     float64 `json:"avg_added_vs_ancora_delta,omitempty"`
	AvgReturnedDelta          float64 `json:"avg_returned_delta,omitempty"`
}

type retrievalQueryEvaluation struct {
	Name           string                           `json:"name,omitempty"`
	Query          string                           `json:"query"`
	Relevant       []string                         `json:"relevant_ids"`
	RelevantLabels []string                         `json:"relevant_labels,omitempty"`
	Profiles       map[string]retrievalQueryProfile `json:"profiles"`
}

type retrievalQueryProfile struct {
	LatencyMs         int64    `json:"latency_ms"`
	RecallAtK         float64  `json:"recall_at_k"`
	MRR               float64  `json:"mrr"`
	NDCGAtK           float64  `json:"ndcg_at_k,omitempty"`
	Returned          int      `json:"returned"`
	OverlapWithAncora int      `json:"overlap_with_ancora,omitempty"`
	AddedVsAncora     int      `json:"added_vs_ancora,omitempty"`
	TopIDs            []string `json:"top_ids"`
}

func retrievalBenchCmd() *cobra.Command {
	var graphFile string
	var ancoraDB string
	var suitePath string
	var limit int
	var maxHops int
	var maxExpansions int
	var relationFilter string
	var profilesCSV string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "retrieval",
		Short: "Run retrieval benchmark suite against search profiles",
		Long:  "Run retrieval evaluation from a local JSON suite. Use the curated starter suite at bench/retrieval/vela-curated.json.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if suitePath == "" {
				return fmt.Errorf("--suite is required")
			}
			if graphFile == "" {
				var err error
				graphFile, err = config.FindGraphFile(".")
				if err != nil {
					return err
				}
			}
			if ancoraDB == "" {
				var err error
				ancoraDB, err = ancoradb.DefaultDBPath()
				if err != nil {
					return err
				}
			}
			suite, err := loadRetrievalBenchmarkSuite(suitePath)
			if err != nil {
				return err
			}
			profiles, err := parseBenchmarkProfiles(profilesCSV)
			if err != nil {
				return err
			}
			eng, err := loadEngine(graphFile)
			if err != nil {
				return err
			}
			searcher := query.NewSearcher(eng, ancoraDB).WithTraversal(retrieval.TraversalOptions{MaxHops: maxHops, MaxExpansions: maxExpansions, AllowedRelations: splitCSV(relationFilter)})
			benchLimit := limit
			if benchLimit <= 0 {
				benchLimit = suite.Limit
			}
			if benchLimit <= 0 {
				benchLimit = 5
			}
			result, err := runRetrievalBenchmark(searcher, suite, suitePath, graphFile, benchLimit, profiles, retrievalBenchmarkSettings{MaxHops: maxHops, MaxExpansions: maxExpansions, Relations: splitCSV(relationFilter)})
			if err != nil {
				return err
			}
			historyDir, err := retrievalBenchHistoryDir(suitePath)
			if err == nil {
				baselinePath, baselineResult, baselineErr := latestRetrievalBenchSnapshot(historyDir)
				if baselineErr == nil && baselineResult != nil {
					result.BaselinePath = baselinePath
					result.Deltas = buildRetrievalDeltas(result.Summary, baselineResult.Summary)
				}
				if _, writeErr := writeRetrievalBenchSnapshot(historyDir, result); writeErr != nil {
					fmt.Fprintf(os.Stderr, "warning: cannot persist retrieval benchmark snapshot: %v\n", writeErr)
				}
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			printRetrievalBenchmark(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&graphFile, "graph", "", "Path to graph.json (default: ~/.vela/graph.json)")
	cmd.Flags().StringVar(&ancoraDB, "ancora-db", "", "Path to ancora.db (default: ~/.ancora/ancora.db)")
	cmd.Flags().StringVar(&suitePath, "suite", filepath.Join("bench", "retrieval", "vela-curated.json"), "Path to benchmark suite JSON file")
	cmd.Flags().IntVar(&limit, "limit", 0, "Override benchmark result limit (defaults to suite limit or 5)")
	cmd.Flags().IntVar(&maxHops, "max-hops", 2, "Maximum graph traversal hops for benchmarked search")
	cmd.Flags().IntVar(&maxExpansions, "max-expansions", 24, "Maximum graph expansions for benchmarked search")
	cmd.Flags().StringVar(&relationFilter, "relations", "", "Comma-separated edge relations allowed during benchmarked search")
	cmd.Flags().StringVar(&profilesCSV, "profiles", "federated,ancora,graph,graph-hybrid,lexical,structural,vector", "Comma-separated benchmark profiles")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output benchmark result as JSON")
	return cmd
}

func loadRetrievalBenchmarkSuite(path string) (retrievalBenchmarkSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return retrievalBenchmarkSuite{}, fmt.Errorf("read benchmark suite: %w", err)
	}
	var suite retrievalBenchmarkSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return retrievalBenchmarkSuite{}, fmt.Errorf("parse benchmark suite: %w", err)
	}
	if len(suite.Cases) == 0 {
		return retrievalBenchmarkSuite{}, fmt.Errorf("benchmark suite has no cases")
	}
	for i, c := range suite.Cases {
		if strings.TrimSpace(c.Query) == "" {
			return retrievalBenchmarkSuite{}, fmt.Errorf("benchmark case %d has empty query", i)
		}
	}
	if strings.TrimSpace(suite.Name) == "" {
		suite.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return suite, nil
}

func parseBenchmarkProfiles(input string) ([]query.SearchProfile, error) {
	parts := splitCSV(input)
	if len(parts) == 0 {
		return nil, fmt.Errorf("no benchmark profiles selected")
	}
	profiles := make([]query.SearchProfile, 0, len(parts))
	for _, part := range parts {
		profile := query.SearchProfile(strings.ToLower(part))
		switch profile {
		case query.SearchProfileFederated, query.SearchProfileAncora, query.SearchProfileGraph, query.SearchProfileGraphHybrid, query.SearchProfileLexical, query.SearchProfileStructural, query.SearchProfileVector:
			profiles = append(profiles, profile)
		default:
			return nil, fmt.Errorf("unknown benchmark profile: %s", part)
		}
	}
	return profiles, nil
}

func runRetrievalBenchmark(searcher *query.Searcher, suite retrievalBenchmarkSuite, suitePath, graphPath string, limit int, profiles []query.SearchProfile, settings retrievalBenchmarkSettings) (retrievalBenchmarkResult, error) {
	result := retrievalBenchmarkResult{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Suite:       suite.Name,
		SuitePath:   suitePath,
		GraphPath:   graphPath,
		Limit:       limit,
		Profiles:    profiles,
		Settings:    settings,
		Summary:     map[string]retrievalProfileSummary{},
		Deltas:      map[string]retrievalProfileDelta{},
		Queries:     make([]retrievalQueryEvaluation, 0, len(suite.Cases)),
	}
	type agg struct {
		recall                 float64
		mrr                    float64
		ndcg                   float64
		latencies              []int64
		returned               int
		overlap                float64
		added                  float64
		queriesWithAnyRelevant int
	}
	aggs := map[query.SearchProfile]*agg{}
	for _, profile := range profiles {
		aggs[profile] = &agg{}
	}

	for _, testCase := range suite.Cases {
		caseResult := retrievalQueryEvaluation{
			Name:           testCase.Name,
			Query:          testCase.Query,
			Relevant:       append([]string(nil), testCase.RelevantID...),
			RelevantLabels: append([]string(nil), testCase.RelevantLabels...),
			Profiles:       map[string]retrievalQueryProfile{},
		}
		rel := normalizedRelevance(testCase)
		baselineRefs := map[string]struct{}{}
		for _, profile := range profiles {
			run, err := searcher.RunProfile(testCase.Query, limit, profile)
			if err != nil {
				return retrievalBenchmarkResult{}, err
			}
			topRefs := hitRefs(run.Hits, limit)
			profileResult := retrievalQueryProfile{
				LatencyMs: run.Metrics.LatencyMs,
				RecallAtK: recallAtK(topRefs, rel),
				MRR:       reciprocalRank(topRefs, rel),
				NDCGAtK:   ndcgAtK(topRefs, rel),
				Returned:  len(run.Hits),
				TopIDs:    hitIDs(run.Hits, limit),
			}
			if profile == query.SearchProfileAncora {
				baselineRefs = toSet(topRefs)
			} else if len(baselineRefs) > 0 {
				profileResult.OverlapWithAncora = overlapAtK(topRefs, baselineRefs)
				profileResult.AddedVsAncora = addedVsBaseline(topRefs, baselineRefs)
			}
			caseResult.Profiles[string(profile)] = profileResult

			a := aggs[profile]
			a.recall += profileResult.RecallAtK
			a.mrr += profileResult.MRR
			a.ndcg += profileResult.NDCGAtK
			a.latencies = append(a.latencies, profileResult.LatencyMs)
			a.returned += profileResult.Returned
			if len(rel) > 0 {
				a.queriesWithAnyRelevant++
			}
			if profile != query.SearchProfileAncora && len(baselineRefs) > 0 {
				a.overlap += float64(profileResult.OverlapWithAncora)
				a.added += float64(profileResult.AddedVsAncora)
			}
		}
		result.Queries = append(result.Queries, caseResult)
	}

	queryCount := float64(len(suite.Cases))
	for _, profile := range profiles {
		a := aggs[profile]
		summary := retrievalProfileSummary{
			Queries:                len(suite.Cases),
			RecallAtK:              a.recall / queryCount,
			MRR:                    a.mrr / queryCount,
			NDCGAtK:                a.ndcg / queryCount,
			AvgLatencyMs:           averageInt64(a.latencies),
			P95LatencyMs:           percentile95(a.latencies),
			AvgReturned:            float64(a.returned) / queryCount,
			QueriesWithAnyRelevant: a.queriesWithAnyRelevant,
		}
		if profile != query.SearchProfileAncora {
			summary.AvgOverlapWithAncora = a.overlap / queryCount
			summary.AvgAddedVsAncora = a.added / queryCount
		}
		result.Summary[string(profile)] = summary
	}
	return result, nil
}

func normalizedRelevance(testCase retrievalBenchmarkCase) map[string]float64 {
	rel := map[string]float64{}
	for _, id := range testCase.RelevantID {
		rel[id] = 1
	}
	for _, label := range testCase.RelevantLabels {
		rel[normalizeRelevantRef(label)] = 1
	}
	for id, score := range testCase.Relevance {
		rel[normalizeRelevantRef(id)] = score
	}
	return rel
}

func hitRefs(hits []query.SearchHit, limit int) []string {
	if limit > len(hits) {
		limit = len(hits)
	}
	refs := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		refs = append(refs, normalizeRelevantRef(hits[i].ID))
		refs = append(refs, normalizeRelevantRef(hits[i].Label))
	}
	return uniqueStrings(refs)
}

func hitIDs(hits []query.SearchHit, limit int) []string {
	if limit > len(hits) {
		limit = len(hits)
	}
	ids := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		ids = append(ids, hits[i].ID)
	}
	return ids
}

func recallAtK(topIDs []string, relevance map[string]float64) float64 {
	if len(relevance) == 0 {
		return 0
	}
	hits := 0
	for _, id := range topIDs {
		if relevance[id] > 0 {
			hits++
		}
	}
	return float64(hits) / float64(len(relevance))
}

func reciprocalRank(topIDs []string, relevance map[string]float64) float64 {
	for i, id := range topIDs {
		if relevance[id] > 0 {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

func ndcgAtK(topIDs []string, relevance map[string]float64) float64 {
	if len(relevance) == 0 {
		return 0
	}
	dcg := 0.0
	for i, id := range topIDs {
		rel := relevance[id]
		if rel <= 0 {
			continue
		}
		dcg += (math.Pow(2, rel) - 1) / math.Log2(float64(i+2))
	}
	ideal := make([]float64, 0, len(relevance))
	for _, rel := range relevance {
		ideal = append(ideal, rel)
	}
	sort.Slice(ideal, func(i, j int) bool { return ideal[i] > ideal[j] })
	idcg := 0.0
	for i, rel := range ideal {
		if i >= len(topIDs) {
			break
		}
		idcg += (math.Pow(2, rel) - 1) / math.Log2(float64(i+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func toSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func normalizeRelevantRef(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func overlapAtK(topIDs []string, baseline map[string]struct{}) int {
	count := 0
	for _, id := range topIDs {
		if _, ok := baseline[id]; ok {
			count++
		}
	}
	return count
}

func addedVsBaseline(topIDs []string, baseline map[string]struct{}) int {
	count := 0
	for _, id := range topIDs {
		if _, ok := baseline[id]; !ok {
			count++
		}
	}
	return count
}

func averageInt64(values []int64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := int64(0)
	for _, value := range values {
		total += value
	}
	return float64(total) / float64(len(values))
}

func percentile95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]int64(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	idx := int(math.Ceil(float64(len(copyValues))*0.95)) - 1
	if idx < 0 {
		idx = 0
	}
	return copyValues[idx]
}

func retrievalBenchHistoryDir(suitePath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	absPath, err := filepath.Abs(suitePath)
	if err != nil {
		absPath = suitePath
	}
	base := sanitizeHistoryName(strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath)))
	if base == "" {
		base = "retrieval-suite"
	}
	sum := sha1.Sum([]byte(absPath))
	hash := hex.EncodeToString(sum[:])[:10]
	return filepath.Join(home, ".vela", "retrieval-bench-history", base+"-"+hash), nil
}

func writeRetrievalBenchSnapshot(historyDir string, result retrievalBenchmarkResult) (string, error) {
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		return "", err
	}
	parsed, err := time.Parse(time.RFC3339, result.GeneratedAt)
	if err != nil {
		parsed = time.Now().UTC()
	}
	base := "retrieval-bench-" + parsed.UTC().Format("20060102T150405Z")
	path := filepath.Join(historyDir, base+".json")
	for i := 1; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		path = filepath.Join(historyDir, fmt.Sprintf("%s-%d.json", base, i))
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func latestRetrievalBenchSnapshot(historyDir string) (string, *retrievalBenchmarkResult, error) {
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		files = append(files, entry.Name())
	}
	if len(files) == 0 {
		return "", nil, nil
	}
	sort.Strings(files)
	latest := filepath.Join(historyDir, files[len(files)-1])
	data, err := os.ReadFile(latest)
	if err != nil {
		return "", nil, err
	}
	var result retrievalBenchmarkResult
	if err := json.Unmarshal(data, &result); err != nil {
		return "", nil, err
	}
	return latest, &result, nil
}

func buildRetrievalDeltas(current, previous map[string]retrievalProfileSummary) map[string]retrievalProfileDelta {
	deltas := map[string]retrievalProfileDelta{}
	for profile, cur := range current {
		prev, ok := previous[profile]
		if !ok {
			continue
		}
		deltas[profile] = retrievalProfileDelta{
			RecallAtKDelta:            cur.RecallAtK - prev.RecallAtK,
			MRRDelta:                  cur.MRR - prev.MRR,
			NDCGAtKDelta:              cur.NDCGAtK - prev.NDCGAtK,
			AvgLatencyMsDelta:         cur.AvgLatencyMs - prev.AvgLatencyMs,
			P95LatencyMsDelta:         cur.P95LatencyMs - prev.P95LatencyMs,
			AvgOverlapWithAncoraDelta: cur.AvgOverlapWithAncora - prev.AvgOverlapWithAncora,
			AvgAddedVsAncoraDelta:     cur.AvgAddedVsAncora - prev.AvgAddedVsAncora,
			AvgReturnedDelta:          cur.AvgReturned - prev.AvgReturned,
		}
	}
	return deltas
}

func printRetrievalBenchmark(result retrievalBenchmarkResult) {
	fmt.Println()
	fmt.Printf("  VELA RETRIEVAL BENCHMARK\n")
	fmt.Printf("  suite: %s\n", result.Suite)
	fmt.Printf("  generated %s\n", result.GeneratedAt)
	fmt.Printf("  source: %s\n", result.SuitePath)
	fmt.Printf("  graph: %s\n", result.GraphPath)
	fmt.Printf("  settings: max_hops=%d max_expansions=%d", result.Settings.MaxHops, result.Settings.MaxExpansions)
	if len(result.Settings.Relations) > 0 {
		fmt.Printf(" relations=%s", strings.Join(result.Settings.Relations, ","))
	}
	fmt.Println()
	if result.BaselinePath != "" {
		fmt.Printf("  baseline: %s\n", result.BaselinePath)
	}
	fmt.Println()
	for _, profile := range result.Profiles {
		s := result.Summary[string(profile)]
		delta := result.Deltas[string(profile)]
		fmt.Printf("  [%s]\n", profile)
		fmt.Printf("    recall@%d  %.3f%s\n", result.Limit, s.RecallAtK, formatFloatDelta(delta.RecallAtKDelta))
		fmt.Printf("    mrr        %.3f%s\n", s.MRR, formatFloatDelta(delta.MRRDelta))
		fmt.Printf("    ndcg@%d    %.3f%s\n", result.Limit, s.NDCGAtK, formatFloatDelta(delta.NDCGAtKDelta))
		fmt.Printf("    latency    avg %.1fms%s  p95 %dms%s\n", s.AvgLatencyMs, formatFloatDelta(delta.AvgLatencyMsDelta), s.P95LatencyMs, formatIntDelta(delta.P95LatencyMsDelta))
		fmt.Printf("    returned   avg %.2f%s\n", s.AvgReturned, formatFloatDelta(delta.AvgReturnedDelta))
		if profile != query.SearchProfileAncora {
			fmt.Printf("    overlap    avg %.2f with ancora%s\n", s.AvgOverlapWithAncora, formatFloatDelta(delta.AvgOverlapWithAncoraDelta))
			fmt.Printf("    added      avg %.2f vs ancora%s\n", s.AvgAddedVsAncora, formatFloatDelta(delta.AvgAddedVsAncoraDelta))
		}
		fmt.Println()
	}
}

func formatFloatDelta(delta float64) string {
	if delta == 0 {
		return ""
	}
	return fmt.Sprintf(" (%+.3f)", delta)
}

func formatIntDelta(delta int64) string {
	if delta == 0 {
		return ""
	}
	return fmt.Sprintf(" (%+d)", delta)
}
