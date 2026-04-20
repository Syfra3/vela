package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Syfra3/vela/internal/config"
)

const defaultEmbeddingModel = "nomic-embed-text"
const defaultEmbeddingProvider = "ollama"
const vectorSearchMode = "in_process_cosine"
const vectorIndexBackend = "sqlite_blob"
const sqliteVecRequestedBackend = "sqlite-vec"
const sqliteCosineBackend = "sqlite-cosine"

// EmbeddingRuntime captures runtime/provenance for persisted vectors.
type EmbeddingRuntime struct {
	Provider               string
	Model                  string
	Endpoint               string
	VectorSearchMode       string
	VectorIndex            string
	RequestedVectorBackend string
	SQLiteVecPath          string
	SQLiteVecEnabled       bool
	SQLiteVecReason        string
}

type embeddingBackend interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
	Runtime() EmbeddingRuntime
}

var embeddingBackendFactory = newOllamaEmbeddingBackend
var embedTextsOverride func([]string) ([][]float32, error)

type ollamaEmbeddingBackend struct {
	endpoint string
	model    string
	client   *http.Client
}

type ollamaEmbedRequest struct {
	Model  string      `json:"model"`
	Input  interface{} `json:"input,omitempty"`
	Prompt string      `json:"prompt,omitempty"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings,omitempty"`
	Embedding  []float32   `json:"embedding,omitempty"`
	Error      string      `json:"error,omitempty"`
}

func newOllamaEmbeddingBackend() (embeddingBackend, error) {
	runtime, timeout, err := currentEmbeddingRuntime()
	if err != nil {
		return nil, err
	}
	if runtime.Provider != defaultEmbeddingProvider {
		return nil, fmt.Errorf("unsupported embedding provider: %s", runtime.Provider)
	}
	return &ollamaEmbeddingBackend{
		endpoint: strings.TrimRight(runtime.Endpoint, "/"),
		model:    runtime.Model,
		client:   &http.Client{Timeout: timeout},
	}, nil
}

func currentEmbeddingRuntime() (EmbeddingRuntime, time.Duration, error) {
	runtime := EmbeddingRuntime{
		Provider:               defaultEmbeddingProvider,
		Model:                  defaultEmbeddingModel,
		Endpoint:               "http://localhost:11434",
		VectorSearchMode:       vectorSearchMode,
		VectorIndex:            vectorIndexBackend,
		RequestedVectorBackend: sqliteCosineBackend,
		SQLiteVecEnabled:       false,
		SQLiteVecReason:        "sqlite-cosine default backend",
	}
	timeout := 60 * time.Second
	if cfg, err := config.Load(); err == nil && cfg != nil {
		if strings.TrimSpace(cfg.Embedding.Provider) != "" {
			runtime.Provider = strings.TrimSpace(cfg.Embedding.Provider)
		}
		if strings.TrimSpace(cfg.Embedding.Model) != "" {
			runtime.Model = strings.TrimSpace(cfg.Embedding.Model)
		}
		if strings.TrimSpace(cfg.Embedding.Endpoint) != "" {
			runtime.Endpoint = strings.TrimSpace(cfg.Embedding.Endpoint)
		} else if strings.TrimSpace(cfg.LLM.Endpoint) != "" {
			runtime.Endpoint = strings.TrimSpace(cfg.LLM.Endpoint)
		}
		if cfg.Embedding.Timeout > 0 {
			timeout = cfg.Embedding.Timeout
		} else if cfg.LLM.Timeout > 0 {
			timeout = cfg.LLM.Timeout
		}
		if strings.TrimSpace(cfg.Embedding.VectorBackend) != "" {
			runtime.RequestedVectorBackend = strings.TrimSpace(cfg.Embedding.VectorBackend)
		}
		if strings.TrimSpace(cfg.Embedding.SQLiteVecPath) != "" {
			runtime.SQLiteVecPath = strings.TrimSpace(cfg.Embedding.SQLiteVecPath)
		}
	}
	if value := strings.TrimSpace(os.Getenv("VELA_EMBED_PROVIDER")); value != "" {
		runtime.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("VELA_EMBED_MODEL")); value != "" {
		runtime.Model = value
	}
	if value := strings.TrimSpace(os.Getenv("VELA_EMBED_ENDPOINT")); value != "" {
		runtime.Endpoint = value
	}
	if value := strings.TrimSpace(os.Getenv("VELA_VECTOR_BACKEND")); value != "" {
		runtime.RequestedVectorBackend = value
	}
	if value := strings.TrimSpace(os.Getenv("VELA_SQLITE_VEC_PATH")); value != "" {
		runtime.SQLiteVecPath = value
	}
	resolveVectorBackend(&runtime)
	return runtime, timeout, nil
}

func resolveVectorBackend(runtime *EmbeddingRuntime) {
	requested := strings.ToLower(strings.TrimSpace(runtime.RequestedVectorBackend))
	if requested == "" {
		requested = sqliteCosineBackend
	}
	runtime.RequestedVectorBackend = requested
	runtime.VectorSearchMode = vectorSearchMode
	runtime.VectorIndex = vectorIndexBackend
	runtime.SQLiteVecEnabled = false
	switch requested {
	case sqliteCosineBackend:
		runtime.SQLiteVecReason = "using sqlite BLOB storage with in-process cosine"
	case sqliteVecRequestedBackend:
		if strings.TrimSpace(runtime.SQLiteVecPath) == "" {
			runtime.SQLiteVecReason = "sqlite-vec requested but sqlite_vec_path is not configured"
			return
		}
		if _, err := os.Stat(runtime.SQLiteVecPath); err != nil {
			runtime.SQLiteVecReason = fmt.Sprintf("sqlite-vec requested but extension path is unavailable: %v", err)
			return
		}
		runtime.SQLiteVecReason = "sqlite-vec extension present on disk, but current modernc.org/sqlite database/sql path does not expose a clean production-safe load-extension integration"
	default:
		runtime.SQLiteVecReason = fmt.Sprintf("unknown vector backend %q; falling back to sqlite-cosine", requested)
	}
}

func embedTexts(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if embedTextsOverride != nil {
		return embedTextsOverride(texts)
	}
	backend, err := embeddingBackendFactory()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return backend.EmbedTexts(ctx, texts)
}

func CurrentEmbeddingRuntime() (EmbeddingRuntime, error) {
	runtime, _, err := currentEmbeddingRuntime()
	return runtime, err
}

// SetEmbedTextsForTesting overrides embedding generation inside tests.
func SetEmbedTextsForTesting(fn func([]string) ([][]float32, error)) func() {
	previous := embedTextsOverride
	embedTextsOverride = fn
	return func() {
		embedTextsOverride = previous
	}
}

func (b *ollamaEmbeddingBackend) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	batch, err := b.embedBatch(ctx, texts)
	if err == nil {
		return batch, nil
	}
	if !strings.Contains(err.Error(), "404") {
		return nil, err
	}
	results := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vector, legacyErr := b.embedLegacy(ctx, text)
		if legacyErr != nil {
			return nil, legacyErr
		}
		results = append(results, vector)
	}
	return results, nil
}

func (b *ollamaEmbeddingBackend) Runtime() EmbeddingRuntime {
	runtime, _, _ := currentEmbeddingRuntime()
	runtime.Provider = defaultEmbeddingProvider
	runtime.Model = b.model
	runtime.Endpoint = b.endpoint
	return runtime
}

func (b *ollamaEmbeddingBackend) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	payload, err := json.Marshal(ollamaEmbedRequest{Model: b.model, Input: texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint+"/api/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed returned %d: %s", resp.StatusCode, raw)
	}
	var parsed ollamaEmbedResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parsing ollama embed response: %w", err)
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("ollama embed error: %s", parsed.Error)
	}
	if len(parsed.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed returned %d vectors for %d texts", len(parsed.Embeddings), len(texts))
	}
	return parsed.Embeddings, nil
}

func (b *ollamaEmbeddingBackend) embedLegacy(ctx context.Context, text string) ([]float32, error) {
	payload, err := json.Marshal(ollamaEmbedRequest{Model: b.model, Prompt: text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint+"/api/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama legacy embed request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama legacy embed returned %d: %s", resp.StatusCode, raw)
	}
	var parsed ollamaEmbedResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parsing ollama legacy embed response: %w", err)
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("ollama legacy embed error: %s", parsed.Error)
	}
	if len(parsed.Embedding) == 0 {
		return nil, fmt.Errorf("ollama legacy embed returned empty embedding")
	}
	return parsed.Embedding, nil
}
