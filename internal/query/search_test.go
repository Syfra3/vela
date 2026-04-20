package query

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Syfra3/vela/internal/retrieval"
	"github.com/Syfra3/vela/pkg/types"
	_ "modernc.org/sqlite"
)

func withStubRetrieverEmbeddings(t *testing.T) {
	t.Helper()
	restore := retrieval.SetEmbedTextsForTesting(func(texts []string) ([][]float32, error) {
		out := make([][]float32, 0, len(texts))
		for _, text := range texts {
			vector := []float32{0, 0}
			lower := strings.ToLower(text)
			if strings.Contains(lower, "rank") {
				vector[0] = 1
			}
			if strings.Contains(lower, "retriev") || strings.Contains(lower, "federated") {
				vector[1] = 1
			}
			if vector[0] == 0 && vector[1] == 0 {
				vector[0] = 0.1
			}
			out = append(out, vector)
		}
		return out, nil
	})
	t.Cleanup(restore)
}

func seedSearchAncoraDB(t *testing.T, rows []map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ancora.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		type TEXT,
		workspace TEXT,
		visibility TEXT NOT NULL DEFAULT 'work',
		organization TEXT,
		topic_key TEXT,
		"references" TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		deleted_at TEXT
	)`)
	if err != nil {
		t.Fatalf("create observations: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, row := range rows {
		_, err = db.Exec(`
			INSERT INTO observations (title, content, type, workspace, visibility, organization, topic_key, "references", created_at, updated_at, deleted_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row["title"], row["content"], row["type"], row["workspace"], row["visibility"], row["organization"], row["topic_key"], row["references"], now, now, row["deleted_at"],
		)
		if err != nil {
			t.Fatalf("insert observation: %v", err)
		}
	}

	return dbPath
}

func buildSearchEngine(t *testing.T) *Engine {
	t.Helper()
	g := map[string]any{
		"nodes": []map[string]any{
			{
				"id":          "project:billing-api",
				"label":       "billing-api",
				"kind":        "project",
				"file":        "project:billing-api",
				"description": "Billing API repository.",
				"source_type": "codebase",
				"source_name": "billing-api",
				"metadata":    map[string]any{"layer": "repo"},
			},
			{
				"id":          "project:docs-site",
				"label":       "docs-site",
				"kind":        "project",
				"file":        "project:docs-site",
				"description": "Docs site repository.",
				"source_type": "codebase",
				"source_name": "docs-site",
				"metadata":    map[string]any{"layer": "repo"},
			},
			{
				"id":    "workspace:repo:billing-api",
				"label": "billing-api",
				"kind":  "repo",
				"file":  "workspace:repo:billing-api",
				"metadata": map[string]any{
					"layer":               "workspace",
					"evidence_type":       "routing",
					"evidence_confidence": "extracted",
				},
			},
			{
				"id":    "workspace:repo:docs-site",
				"label": "docs-site",
				"kind":  "repo",
				"file":  "workspace:repo:docs-site",
				"metadata": map[string]any{
					"layer":               "workspace",
					"evidence_type":       "routing",
					"evidence_confidence": "extracted",
				},
			},
			{
				"id":    "workspace:service:billing",
				"label": "billing",
				"kind":  "service",
				"file":  "workspace:service:billing",
				"metadata": map[string]any{
					"layer":               "workspace",
					"evidence_type":       "routing",
					"evidence_confidence": "extracted",
				},
			},
			{
				"id":          "contract:service:billing",
				"label":       "billing",
				"kind":        "service",
				"file":        "openapi.yaml",
				"description": "Declared billing service.",
				"metadata": map[string]any{
					"layer":               "contract",
					"evidence_type":       "openapi",
					"evidence_confidence": "declared",
				},
			},
			{
				"id":          "ancora:obs:1",
				"label":       "Federated retriever architecture",
				"kind":        "observation",
				"file":        "ancora:obs:1",
				"description": "Ancora is the source of truth while Vela projects and ranks graph-aware results.",
				"metadata": map[string]any{
					"workspace": "vela",
					"obs_type":  "architecture",
				},
			},
			{
				"id":          "code:retriever",
				"label":       "FederatedRetriever",
				"kind":        "struct",
				"file":        "internal/query/search.go",
				"description": "Combines graph and memory retrieval into one ranked result set.",
				"source_type": "codebase",
				"source_name": "billing-api",
				"metadata": map[string]any{
					"workspace": "vela",
				},
			},
			{
				"id":          "code:ranker",
				"label":       "ResultRanker",
				"kind":        "struct",
				"file":        "internal/query/search.go",
				"description": "Ranks structural graph expansions for retrieval.",
				"source_type": "codebase",
				"source_name": "billing-api",
				"metadata": map[string]any{
					"workspace": "vela",
				},
			},
			{
				"id":          "code:printer",
				"label":       "SearchPrinter",
				"kind":        "struct",
				"file":        "cmd/vela/main.go",
				"description": "Prints retrieval output and metrics.",
				"source_type": "codebase",
				"source_name": "docs-site",
				"metadata": map[string]any{
					"workspace": "vela",
				},
			},
		},
		"edges": []map[string]any{
			{
				"from": "workspace:repo:billing-api",
				"to":   "workspace:service:billing",
				"kind": "exposes",
				"metadata": map[string]any{
					"layer":               "workspace",
					"evidence_type":       "routing",
					"evidence_confidence": "extracted",
				},
			},
			{
				"from": "contract:service:billing",
				"to":   "project:billing-api",
				"kind": "declared_in",
				"metadata": map[string]any{
					"layer":               "contract",
					"evidence_type":       "openapi",
					"evidence_confidence": "declared",
				},
			},
			{
				"from": "code:retriever",
				"to":   "code:ranker",
				"kind": "calls",
				"metadata": map[string]any{
					"evidence_type":            "ast",
					"evidence_confidence":      "extracted",
					"evidence_source_artifact": "internal/query/search.go",
				},
			},
			{
				"from": "code:ranker",
				"to":   "code:printer",
				"kind": "uses",
			},
		},
		"meta": map[string]any{"generatedAt": time.Now().UTC().Format(time.RFC3339)},
	}
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write graph: %v", err)
	}
	eng, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}
	return eng
}

func TestSearcherSearch_FederatesAndMeasures(t *testing.T) {
	withStubRetrieverEmbeddings(t)
	eng := buildSearchEngine(t)
	dbPath := seedSearchAncoraDB(t, []map[string]any{
		{
			"title":        "Federated retriever architecture",
			"content":      "Ancora feeds memory context and Vela fuses it with graph nodes.",
			"type":         "architecture",
			"workspace":    "vela",
			"visibility":   "work",
			"organization": "syfra",
			"topic_key":    "vela/federated-retriever",
			"references":   nil,
			"deleted_at":   nil,
		},
	})

	searcher := NewSearcher(eng, dbPath)
	resp, err := searcher.Search("federated retriever", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(resp.Hits) < 2 {
		t.Fatalf("len(resp.Hits) = %d, want at least 2", len(resp.Hits))
	}
	if resp.Metrics.AncoraOnly.Returned == 0 {
		t.Fatalf("AncoraOnly.Returned = 0, want baseline hits")
	}
	if resp.Metrics.Comparison.AddedByFederated == 0 {
		t.Fatalf("AddedByFederated = 0, want graph-only expansion")
	}
	if resp.Metrics.Comparison.AddedBySource[searchSourceGraph] == 0 {
		t.Fatalf("AddedBySource[%q] = 0, want graph contribution", searchSourceGraph)
	}
	if resp.Metrics.Federated.SourceContribution[searchSourceGraph] == 0 {
		t.Fatalf("Federated.SourceContribution missing graph results: %#v", resp.Metrics.Federated.SourceContribution)
	}
	if resp.Metrics.Federated.SignalContribution[searchSignalStructural] == 0 {
		t.Fatalf("Federated.SignalContribution missing structural signal: %#v", resp.Metrics.Federated.SignalContribution)
	}
	if resp.Hits[0].PrimarySource == "" {
		t.Fatal("top hit missing primary source")
	}

	foundStructural := false
	for _, hit := range resp.Hits {
		if hit.ID != "code:ranker" {
			continue
		}
		if hit.Signals[searchSignalStructural] == 0 {
			t.Fatalf("code:ranker missing structural signal: %#v", hit.Signals)
		}
		if len(hit.Support) == 0 {
			t.Fatal("code:ranker missing support context")
		}
		if hit.SupportGraph == nil {
			t.Fatal("code:ranker missing support graph")
		}
		if len(hit.SupportGraph.Nodes) == 0 || len(hit.SupportGraph.Edges) == 0 {
			t.Fatalf("support graph missing structure: %#v", hit.SupportGraph)
		}
		if hit.SupportGraph.Edges[0].EvidenceType != "ast" {
			t.Fatalf("support edge evidence_type = %q, want ast", hit.SupportGraph.Edges[0].EvidenceType)
		}
		foundStructural = true
	}
	if !foundStructural {
		t.Fatal("expected structural graph expansion hit for code:ranker")
	}
}

func TestSearcherTraversalControls_MaxHopsOneStopsSecondHop(t *testing.T) {
	withStubRetrieverEmbeddings(t)
	eng := buildSearchEngine(t)
	searcher := NewSearcher(eng, "").WithTraversal(retrieval.TraversalOptions{MaxHops: 1, MaxExpansions: 8})

	// Reuse graph-side search directly so Ancora setup is irrelevant.
	hits, _, err := searchGraph(searcher.engine, "federated retriever", 5, searcher.traversal, defaultGraphSignals)
	if err != nil {
		t.Fatalf("searchGraph() error = %v", err)
	}
	for _, hit := range hits {
		if hit.ID != "code:ranker" {
			continue
		}
		if hit.SupportGraph == nil {
			t.Fatal("code:ranker missing support graph")
		}
		for _, edge := range hit.SupportGraph.Edges {
			if edge.Hop > 1 {
				t.Fatalf("edge hop = %d, want bounded to 1", edge.Hop)
			}
		}
		return
	}
	t.Fatal("expected code:ranker hit")
}

func TestSearcherSearch_VectorSignalAddsGraphHit(t *testing.T) {
	withStubRetrieverEmbeddings(t)
	eng := buildSearchEngine(t)
	dbPath := seedSearchAncoraDB(t, nil)

	searcher := NewSearcher(eng, dbPath)
	resp, err := searcher.Search("ranking", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(resp.Hits) == 0 {
		t.Fatal("len(resp.Hits) = 0, want graph vector hit")
	}
	if resp.Metrics.Federated.SignalContribution[searchSignalVector] == 0 {
		t.Fatalf("missing vector signal contribution: %#v", resp.Metrics.Federated.SignalContribution)
	}
	found := false
	for _, hit := range resp.Hits {
		if hit.ID != "code:ranker" {
			continue
		}
		if hit.Signals[searchSignalVector] == 0 {
			t.Fatalf("code:ranker missing vector signal: %#v", hit.Signals)
		}
		found = true
	}
	if !found {
		t.Fatal("expected vector-retrieved graph hit for code:ranker")
	}
}

func TestSearcherSearch_RoutesWorkspaceBeforeRepoDeepRetrieval(t *testing.T) {
	withStubRetrieverEmbeddings(t)
	eng := buildSearchEngine(t)
	resp, err := NewSearcher(eng, seedSearchAncoraDB(t, nil)).Search("billing", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(resp.Routing.RoutedRepos) == 0 {
		t.Fatal("expected routed repos for billing query")
	}
	if resp.Routing.RoutedRepos[0].Repo != "billing-api" {
		t.Fatalf("top routed repo = %q, want billing-api", resp.Routing.RoutedRepos[0].Repo)
	}
	if len(resp.Routing.RoutedRepos[0].Reasons) == 0 {
		t.Fatal("expected routing reasons for billing repo")
	}
	foundRepo := false
	foundWorkspace := false
	foundContract := false
	for _, hit := range resp.Hits {
		if hit.ID == "code:printer" {
			t.Fatalf("unexpected docs-site repo-deep hit in routed billing search: %+v", hit)
		}
		switch hit.PrimaryLayer {
		case string(types.LayerRepo):
			foundRepo = true
		case string(types.LayerWorkspace):
			foundWorkspace = true
		case string(types.LayerContract):
			foundContract = true
		}
	}
	if !foundRepo || !foundWorkspace || !foundContract {
		t.Fatalf("expected repo/workspace/contract coverage, got hits=%+v", resp.Hits)
	}
}

func TestSearcherSearch_SpanishPluralMatchesSingularAncora(t *testing.T) {
	dbPath := seedSearchAncoraDB(t, []map[string]any{
		{
			"title":        "Gasto D1 - 15 de abril",
			"content":      "Registro de gasto de comida en D1.",
			"type":         "manual",
			"workspace":    "personal-finance",
			"visibility":   "personal",
			"organization": "syfra",
			"topic_key":    "finanzas/gasto-d1-2026-04-15",
			"references":   nil,
			"deleted_at":   nil,
		},
	})

	searcher := NewSearcher(nil, dbPath)
	resp, err := searcher.Search("Gastos", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if resp.Metrics.AncoraOnly.Returned == 0 {
		t.Fatal("AncoraOnly.Returned = 0, want singular gasto match for plural query")
	}
	if len(resp.Hits) == 0 {
		t.Fatal("len(resp.Hits) = 0, want matching ancora hit")
	}
	if resp.Hits[0].PrimarySource != searchSourceAncora {
		t.Fatalf("PrimarySource = %q, want %q", resp.Hits[0].PrimarySource, searchSourceAncora)
	}
	if resp.Hits[0].Label != "Gasto D1 - 15 de abril" {
		t.Fatalf("Label = %q, want singular gasto observation", resp.Hits[0].Label)
	}
}

func TestFuseHits_MergesObservationAliasesByCanonicalID(t *testing.T) {
	merged := fuseHits([]SearchHit{
		{
			ID:            "ancora:obs:7",
			CanonicalID:   "memory:observation:ancora:obs:7",
			Label:         "Identity resolver decision",
			PrimarySource: searchSourceAncora,
			Score:         4,
		},
	}, []SearchHit{
		{
			ID:            "memory:observation:7",
			CanonicalID:   "memory:observation:ancora:obs:7",
			Label:         "Identity resolver decision",
			PrimarySource: searchSourceGraph,
			Score:         3,
		},
	}, 5)

	if len(merged) != 1 {
		t.Fatalf("len(merged) = %d, want 1", len(merged))
	}
	if len(merged[0].Sources) != 2 {
		t.Fatalf("sources = %#v, want merged ancora+graph provenance", merged[0].Sources)
	}
	if merged[0].CanonicalID != "memory:observation:ancora:obs:7" {
		t.Fatalf("canonical_id = %q", merged[0].CanonicalID)
	}
}

func TestWriteMetricsSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := WriteMetricsSnapshot(SearchMetrics{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Query:       "federated retriever",
		Limit:       5,
	})
	if err != nil {
		t.Fatalf("WriteMetricsSnapshot() error = %v", err)
	}
	if filepath.Dir(path) != filepath.Join(home, ".vela", "retrieval-history") {
		t.Fatalf("snapshot dir = %q", filepath.Dir(path))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("os.Stat(%q) error = %v", path, err)
	}
}

func TestSearcherRunProfile(t *testing.T) {
	withStubRetrieverEmbeddings(t)
	eng := buildSearchEngine(t)
	dbPath := seedSearchAncoraDB(t, []map[string]any{{
		"title":        "Federated retriever architecture",
		"content":      "Ancora feeds memory context and Vela fuses it with graph nodes.",
		"type":         "architecture",
		"workspace":    "vela",
		"visibility":   "work",
		"organization": "syfra",
		"topic_key":    "vela/federated-retriever",
		"references":   nil,
		"deleted_at":   nil,
	}})
	searcher := NewSearcher(eng, dbPath)

	tests := []struct {
		name    string
		profile SearchProfile
		wantID  string
	}{
		{name: "ancora", profile: SearchProfileAncora, wantID: "ancora:obs:1"},
		{name: "graph", profile: SearchProfileGraph, wantID: "code:retriever"},
		{name: "graph-hybrid", profile: SearchProfileGraphHybrid, wantID: "code:retriever"},
		{name: "lexical", profile: SearchProfileLexical, wantID: "code:retriever"},
		{name: "structural", profile: SearchProfileStructural, wantID: "code:ranker"},
		{name: "vector", profile: SearchProfileVector, wantID: "code:retriever"},
		{name: "federated", profile: SearchProfileFederated, wantID: "ancora:obs:1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, err := searcher.RunProfile("federated retriever", 5, tt.profile)
			if err != nil {
				t.Fatalf("RunProfile() error = %v", err)
			}
			if run.Profile != tt.profile && !(tt.profile == SearchProfileFederated && run.Profile == SearchProfileFederated) {
				t.Fatalf("Profile = %q, want %q", run.Profile, tt.profile)
			}
			if len(run.Hits) == 0 {
				t.Fatal("len(run.Hits) = 0")
			}
			if run.Hits[0].ID != tt.wantID {
				t.Fatalf("top hit = %q, want %q", run.Hits[0].ID, tt.wantID)
			}
		})
	}
}
