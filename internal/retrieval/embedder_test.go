package retrieval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubEmbeddingBackend struct{}

func (stubEmbeddingBackend) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vector := []float32{0, 0}
		if containsAny(text, "rank", "ranking") {
			vector[0] = 1
		}
		if containsAny(text, "retriev", "federated") {
			vector[1] = 1
		}
		if vector[0] == 0 && vector[1] == 0 {
			vector[0] = 0.1
		}
		out = append(out, vector)
	}
	return out, nil
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(strings.ToLower(text), needle) {
			return true
		}
	}
	return false
}

func TestOllamaEmbeddingBackendBatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("path = %q, want /api/embed", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{{1, 0}, {0, 1}}})
	}))
	defer server.Close()

	backend := &ollamaEmbeddingBackend{endpoint: server.URL, model: defaultEmbeddingModel, client: server.Client()}
	vectors, err := backend.EmbedTexts(context.Background(), []string{"one", "two"})
	if err != nil {
		t.Fatalf("EmbedTexts() error = %v", err)
	}
	if len(vectors) != 2 || len(vectors[0]) != 2 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}

func TestOllamaEmbeddingBackendLegacyFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embed":
			http.NotFound(w, r)
		case "/api/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float32{1, 0, 0}})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	backend := &ollamaEmbeddingBackend{endpoint: server.URL, model: defaultEmbeddingModel, client: server.Client()}
	vectors, err := backend.EmbedTexts(context.Background(), []string{"one"})
	if err != nil {
		t.Fatalf("EmbedTexts() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 3 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}
