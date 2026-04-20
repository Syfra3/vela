package retrieval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

func withStubEmbedder(t *testing.T) {
	t.Helper()
	restore := SetEmbedTextsForTesting(func(texts []string) ([][]float32, error) {
		return stubEmbeddingBackend{}.EmbedTexts(context.Background(), texts)
	})
	t.Cleanup(restore)
}

func TestSyncGraphAndSearchLexical(t *testing.T) {
	withStubEmbedder(t)

	graph := &types.Graph{
		Nodes: []types.Node{
			{
				ID:          "code:retriever",
				Label:       "FederatedRetriever",
				NodeType:    "struct",
				SourceFile:  "internal/query/search.go",
				Description: "Combines graph and memory retrieval into one ranked result set.",
				Metadata: map[string]interface{}{
					"workspace": "vela",
					"topic_key": "vela/true-hybrid-retrieval",
				},
			},
			{
				ID:          "obs:sqlite",
				Label:       "SQLite substrate",
				NodeType:    "concept",
				Description: "Vela stores lexical retrieval units in SQLite FTS5.",
			},
		},
		Edges: []types.Edge{{Source: "code:retriever", Target: "obs:sqlite", Relation: "uses"}},
	}

	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(graph, dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}

	results, candidates, err := SearchLexical(dbPath, "federated retriever", 5)
	if err != nil {
		t.Fatalf("SearchLexical() error = %v", err)
	}
	if candidates != 1 {
		t.Fatalf("candidates = %d, want 1", candidates)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].ID != "code:retriever" {
		t.Fatalf("top hit = %q, want code:retriever", results[0].ID)
	}
	if results[0].Path != "internal/query/search.go" {
		t.Fatalf("top path = %q", results[0].Path)
	}
	metadata, err := LoadMetadata(dbPath)
	if err != nil {
		t.Fatalf("LoadMetadata() error = %v", err)
	}
	if metadata.EmbeddingProvider == "" || metadata.VectorSearchMode != vectorSearchMode || metadata.VectorIndex != vectorIndexBackend {
		t.Fatalf("unexpected retrieval metadata: %+v", metadata)
	}
	if metadata.RequestedVectorBackend != sqliteCosineBackend || metadata.SQLiteVecEnabled || metadata.SQLiteVecReason == "" {
		t.Fatalf("unexpected vector backend metadata: %+v", metadata)
	}
}

func TestEnsureGraphSyncCreatesDBWhenMissing(t *testing.T) {
	withStubEmbedder(t)

	graphPath := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[],"meta":{}}`), 0o644); err != nil {
		t.Fatalf("write graph: %v", err)
	}

	dbPath, err := EnsureGraphSync(graphPath, &types.Graph{})
	if err != nil {
		t.Fatalf("EnsureGraphSync() error = %v", err)
	}
	if filepath.Base(dbPath) != dbFileName {
		t.Fatalf("db path = %q, want %q", dbPath, dbFileName)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("os.Stat(%q) error = %v", dbPath, err)
	}
}

func TestEnsureGraphSyncRebuildsWhenEmbeddingModelChanges(t *testing.T) {
	withStubEmbedder(t)
	t.Setenv("VELA_EMBED_MODEL", "embed-a")

	graphPath := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(graphPath, []byte(`{"nodes":[],"edges":[],"meta":{}}`), 0o644); err != nil {
		t.Fatalf("write graph: %v", err)
	}
	dbPath, err := EnsureGraphSync(graphPath, &types.Graph{})
	if err != nil {
		t.Fatalf("EnsureGraphSync() error = %v", err)
	}
	metadata, err := LoadMetadata(dbPath)
	if err != nil {
		t.Fatalf("LoadMetadata() error = %v", err)
	}
	if metadata.EmbeddingModel != "embed-a" {
		t.Fatalf("EmbeddingModel = %q, want embed-a", metadata.EmbeddingModel)
	}

	t.Setenv("VELA_EMBED_MODEL", "embed-b")
	dbPath, err = EnsureGraphSync(graphPath, &types.Graph{})
	if err != nil {
		t.Fatalf("EnsureGraphSync() second error = %v", err)
	}
	metadata, err = LoadMetadata(dbPath)
	if err != nil {
		t.Fatalf("LoadMetadata() second error = %v", err)
	}
	if metadata.EmbeddingModel != "embed-b" {
		t.Fatalf("EmbeddingModel = %q, want embed-b", metadata.EmbeddingModel)
	}
}

func TestSyncGraphFallsBackWhenEmbeddingsUnavailable(t *testing.T) {
	restore := SetEmbedTextsForTesting(func([]string) ([][]float32, error) {
		return nil, errors.New("model \"nomic-embed-text\" not found")
	})
	t.Cleanup(restore)

	graph := &types.Graph{
		Nodes: []types.Node{{ID: "code:retriever", Label: "FederatedRetriever", NodeType: "struct", SourceFile: "internal/query/search.go"}},
	}
	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(graph, dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}

	results, candidates, err := SearchLexical(dbPath, "federated retriever", 5)
	if err != nil {
		t.Fatalf("SearchLexical() error = %v", err)
	}
	if candidates != 1 || len(results) != 1 {
		t.Fatalf("lexical fallback results = %d/%d", candidates, len(results))
	}

	vectorResults, vectorCandidates, err := SearchVector(dbPath, "federated retriever", 5)
	if err != nil {
		t.Fatalf("SearchVector() error = %v", err)
	}
	if vectorCandidates != 0 || len(vectorResults) != 0 {
		t.Fatalf("vector fallback = %d/%d, want 0/0", vectorCandidates, len(vectorResults))
	}

	metadata, err := LoadMetadata(dbPath)
	if err != nil {
		t.Fatalf("LoadMetadata() error = %v", err)
	}
	if metadata.EmbeddingDims != 0 {
		t.Fatalf("EmbeddingDims = %d, want 0", metadata.EmbeddingDims)
	}
	if !strings.Contains(metadata.SQLiteVecReason, "vector embeddings unavailable") {
		t.Fatalf("SQLiteVecReason = %q", metadata.SQLiteVecReason)
	}
}

func TestSyncGraphIgnoresDuplicateEdges(t *testing.T) {
	withStubEmbedder(t)

	graph := &types.Graph{
		Nodes: []types.Node{{ID: "code:retriever", Label: "FederatedRetriever", NodeType: "struct", SourceFile: "internal/query/search.go"}},
		Edges: []types.Edge{
			{Source: "code:retriever", Target: "memory:workspace:syfra", Relation: "references", SourceFile: "internal/query/search.go"},
			{Source: "code:retriever", Target: "memory:workspace:syfra", Relation: "references", SourceFile: "internal/query/search.go"},
		},
	}

	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(graph, dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}
	if _, _, err := SearchLexical(dbPath, "federated retriever", 5); err != nil {
		t.Fatalf("SearchLexical() error = %v", err)
	}
}

func TestSyncGraphDeduplicatesDuplicateNodeIDs(t *testing.T) {
	withStubEmbedder(t)

	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "vela:vela", Label: "vela", NodeType: "concept"},
			{ID: "vela:vela", Label: "vela duplicate", NodeType: "concept"},
			{ID: "code:retriever", Label: "FederatedRetriever", NodeType: "struct", SourceFile: "internal/query/search.go"},
		},
		Edges: []types.Edge{{Source: "code:retriever", Target: "vela:vela", Relation: "documents"}},
	}

	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(graph, dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}

	results, candidates, err := SearchLexical(dbPath, "federated retriever", 5)
	if err != nil {
		t.Fatalf("SearchLexical() error = %v", err)
	}
	if candidates != 1 || len(results) != 1 {
		t.Fatalf("results = %d/%d, want 1/1", candidates, len(results))
	}
}

func TestCurrentEmbeddingRuntimeSQLiteVecFallback(t *testing.T) {
	t.Setenv("VELA_VECTOR_BACKEND", "sqlite-vec")
	t.Setenv("VELA_SQLITE_VEC_PATH", "/tmp/does-not-exist-sqlite-vec.so")
	runtime, err := CurrentEmbeddingRuntime()
	if err != nil {
		t.Fatalf("CurrentEmbeddingRuntime() error = %v", err)
	}
	if runtime.RequestedVectorBackend != sqliteVecRequestedBackend {
		t.Fatalf("RequestedVectorBackend = %q", runtime.RequestedVectorBackend)
	}
	if runtime.SQLiteVecEnabled {
		t.Fatal("SQLiteVecEnabled = true, want fallback")
	}
	if runtime.VectorIndex != vectorIndexBackend || runtime.VectorSearchMode != vectorSearchMode {
		t.Fatalf("unexpected fallback runtime: %+v", runtime)
	}
	if runtime.SQLiteVecReason == "" {
		t.Fatal("expected sqlite-vec fallback reason")
	}
}

func TestExpandStructural_FromLexicalSeeds(t *testing.T) {
	withStubEmbedder(t)

	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "seed", Label: "FederatedRetriever", NodeType: "struct", Description: "Combines lexical and structural retrieval."},
			{ID: "neighbor", Label: "ResultRanker", NodeType: "struct", Description: "Ranks hybrid graph results."},
			{ID: "leaf", Label: "SearchCLI", NodeType: "file", Description: "Prints retrieval output."},
		},
		Edges: []types.Edge{
			{Source: "seed", Target: "neighbor", Relation: "calls"},
			{Source: "neighbor", Target: "leaf", Relation: "uses"},
		},
	}

	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(graph, dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}
	seeds, _, err := SearchLexical(dbPath, "federated retriever", 5)
	if err != nil {
		t.Fatalf("SearchLexical() error = %v", err)
	}

	results, candidates, err := ExpandStructural(dbPath, seeds, TraversalOptions{MaxHops: 2, MaxExpansions: 8, Limit: 5})
	if err != nil {
		t.Fatalf("ExpandStructural() error = %v", err)
	}
	if candidates == 0 {
		t.Fatal("candidates = 0, want structural neighbors")
	}
	if len(results) == 0 {
		t.Fatal("len(results) = 0, want structural results")
	}
	if results[0].ID != "neighbor" {
		t.Fatalf("top structural hit = %q, want neighbor", results[0].ID)
	}
	if len(results[0].Context) == 0 {
		t.Fatal("expected structural context on top result")
	}
	if results[0].Context[0].Relation != "calls" {
		t.Fatalf("top structural relation = %q, want calls", results[0].Context[0].Relation)
	}
	if results[0].Context[0].Hop != 1 {
		t.Fatalf("top structural hop = %d, want 1", results[0].Context[0].Hop)
	}
}

func TestSearchVector_FindsMorphologicalNeighbor(t *testing.T) {
	withStubEmbedder(t)

	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "retriever", Label: "FederatedRetriever", NodeType: "struct", Description: "Combines graph retrieval results."},
			{ID: "ranker", Label: "ResultRanker", NodeType: "struct", Description: "Ranks hybrid graph expansions for retrieval."},
			{ID: "printer", Label: "SearchPrinter", NodeType: "struct", Description: "Prints command output."},
		},
	}

	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(graph, dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}

	results, candidates, err := SearchVector(dbPath, "ranking", 5)
	if err != nil {
		t.Fatalf("SearchVector() error = %v", err)
	}
	if candidates == 0 {
		t.Fatal("candidates = 0, want vector matches")
	}
	if len(results) == 0 {
		t.Fatal("len(results) = 0, want vector results")
	}
	if results[0].ID != "ranker" {
		t.Fatalf("top vector hit = %q, want ranker", results[0].ID)
	}
	if results[0].Score <= 0 {
		t.Fatalf("top vector score = %f, want positive", results[0].Score)
	}
}
