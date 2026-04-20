package retrieval

import (
	"path/filepath"
	"testing"

	"github.com/Syfra3/vela/pkg/types"
)

// twoRepoGraph returns a tiny fixture graph spanning two repos so that repo
// scoping behavior can be exercised without mocks.
func twoRepoGraph() *types.Graph {
	return &types.Graph{
		Nodes: []types.Node{
			{
				ID:          "vela:retriever",
				Label:       "FederatedRetriever",
				NodeType:    "struct",
				SourceFile:  "internal/query/search.go",
				Description: "Combines lexical and structural retrieval.",
				Source: &types.Source{
					Type: types.SourceTypeCodebase,
					Name: "vela",
					Path: "/repos/vela",
				},
			},
			{
				ID:          "ancora:retriever",
				Label:       "FederatedRetriever",
				NodeType:    "struct",
				SourceFile:  "internal/memory/search.go",
				Description: "Ranks federated retrieval hits across observations.",
				Source: &types.Source{
					Type: types.SourceTypeCodebase,
					Name: "ancora",
					Path: "/repos/ancora",
				},
			},
		},
	}
}

func TestSearchLexicalWithOptions_RepoScopeFiltersResults(t *testing.T) {
	withStubEmbedder(t)

	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(twoRepoGraph(), dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}

	unscoped, _, err := SearchLexicalWithOptions(dbPath, "federated retriever", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("SearchLexicalWithOptions() error = %v", err)
	}
	if len(unscoped) != 2 {
		t.Fatalf("unscoped len = %d, want 2", len(unscoped))
	}

	scoped, candidates, err := SearchLexicalWithOptions(dbPath, "federated retriever", SearchOptions{Repo: "vela", Limit: 5})
	if err != nil {
		t.Fatalf("SearchLexicalWithOptions(repo=vela) error = %v", err)
	}
	if candidates != 1 {
		t.Fatalf("candidates = %d, want 1 when repo-scoped", candidates)
	}
	if len(scoped) != 1 {
		t.Fatalf("scoped len = %d, want 1", len(scoped))
	}
	hit := scoped[0]
	if hit.ID != "vela:retriever" {
		t.Fatalf("hit.ID = %q, want vela:retriever", hit.ID)
	}
	if hit.SourceName != "vela" || hit.SourceType != string(types.SourceTypeCodebase) || hit.SourcePath != "/repos/vela" {
		t.Fatalf("source attribution missing: %+v", hit)
	}
}

func TestSearchVectorWithOptions_RepoScopeFiltersResults(t *testing.T) {
	withStubEmbedder(t)

	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(twoRepoGraph(), dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}

	unscoped, _, err := SearchVectorWithOptions(dbPath, "retriever", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("SearchVectorWithOptions() error = %v", err)
	}
	if len(unscoped) != 2 {
		t.Fatalf("unscoped len = %d, want 2", len(unscoped))
	}
	for _, hit := range unscoped {
		if hit.SourceName == "" {
			t.Fatalf("vector hit missing SourceName: %+v", hit)
		}
	}

	scoped, _, err := SearchVectorWithOptions(dbPath, "retriever", SearchOptions{Repo: "ancora", Limit: 5})
	if err != nil {
		t.Fatalf("SearchVectorWithOptions(repo=ancora) error = %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("scoped len = %d, want 1", len(scoped))
	}
	if scoped[0].SourceName != "ancora" {
		t.Fatalf("scoped hit SourceName = %q, want ancora", scoped[0].SourceName)
	}
}

func TestSearchLexicalWithOptions_SourceTypeFiltersNonCodebase(t *testing.T) {
	withStubEmbedder(t)

	graph := &types.Graph{Nodes: []types.Node{
		{ID: "vela:retriever", Label: "FederatedRetriever", NodeType: "struct", Description: "Code result", Source: &types.Source{Type: types.SourceTypeCodebase, Name: "vela"}},
		{ID: "memory:observation:1", Label: "Federated retriever note", NodeType: string(types.NodeTypeObservation), Description: "Memory result", Source: &types.Source{Type: types.SourceTypeMemory, Name: "ancora"}},
	}}
	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(graph, dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}

	hits, _, err := SearchLexicalWithOptions(dbPath, "federated retriever", SearchOptions{Limit: 5, SourceType: string(types.SourceTypeCodebase)})
	if err != nil {
		t.Fatalf("SearchLexicalWithOptions(source_type=codebase) error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("len(hits) = %d, want 1", len(hits))
	}
	if hits[0].ID != "vela:retriever" {
		t.Fatalf("hit.ID = %q, want vela:retriever", hits[0].ID)
	}
}

func TestEmbeddingEncodeDecodeRoundTrip(t *testing.T) {
	want := []float32{0.0, 1.0, -0.5, 3.14159, 42}
	encoded := encodeEmbedding(want)
	if len(encoded) != len(want)*4 {
		t.Fatalf("encoded length = %d, want %d", len(encoded), len(want)*4)
	}
	got, err := decodeEmbedding(encoded)
	if err != nil {
		t.Fatalf("decodeEmbedding() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("decoded length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("decoded[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if _, err := decodeEmbedding([]byte{1, 2, 3}); err == nil {
		t.Fatal("decodeEmbedding(invalid) error = nil, want error on truncated payload")
	}
}

func TestSyncGraphPersistsEmbeddingDims(t *testing.T) {
	withStubEmbedder(t)

	graph := &types.Graph{
		Nodes: []types.Node{
			{ID: "n1", Label: "FederatedRetriever", NodeType: "struct"},
			{ID: "n2", Label: "ResultRanker", NodeType: "struct", Description: "Ranks retrieval hits."},
		},
	}
	dbPath := filepath.Join(t.TempDir(), "retrieval.db")
	if err := SyncGraph(graph, dbPath); err != nil {
		t.Fatalf("SyncGraph() error = %v", err)
	}

	metadata, err := LoadMetadata(dbPath)
	if err != nil {
		t.Fatalf("LoadMetadata() error = %v", err)
	}
	// The stub embedder returns 2-D vectors, so embedding_dims must round-trip.
	if metadata.EmbeddingDims != 2 {
		t.Fatalf("EmbeddingDims = %d, want 2", metadata.EmbeddingDims)
	}
	if metadata.VectorIndex != vectorIndexBackend {
		t.Fatalf("VectorIndex = %q, want %q", metadata.VectorIndex, vectorIndexBackend)
	}
}
