package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

// LocalProvider implements types.LLMProvider for Ollama / llama.cpp.
type LocalProvider struct {
	config *types.LLMConfig
	client *http.Client
}

// NewLocalProvider creates a new local LLM provider.
func NewLocalProvider(config *types.LLMConfig) (*LocalProvider, error) {
	if config.Endpoint == "" {
		config.Endpoint = "http://localhost:11434"
	}
	if config.Model == "" {
		config.Model = "llama3"
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &LocalProvider{
		config: config,
		client: &http.Client{Timeout: timeout},
	}, nil
}

// ollamaRequest is the request body for Ollama's /api/generate endpoint.
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format"` // "json" — forces JSON output
}

// ollamaResponse is the non-streaming response from Ollama.
type ollamaResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// ExtractGraph sends text to Ollama and parses the structured extraction result.
func (p *LocalProvider) ExtractGraph(ctx context.Context, text string, schema string) (*types.ExtractionResult, error) {
	prompt := buildExtractionPrompt(text, schema)

	body, err := json.Marshal(ollamaRequest{
		Model:  p.config.Model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.config.Endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, raw)
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(raw, &ollamaResp); err != nil {
		return nil, fmt.Errorf("parsing ollama response: %w", err)
	}
	if ollamaResp.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", ollamaResp.Error)
	}

	return parseExtractionJSON([]byte(ollamaResp.Response))
}

// Health checks if the Ollama endpoint is reachable.
func (p *LocalProvider) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.config.Endpoint+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable at %s: %w", p.config.Endpoint, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama health check failed: status %d", resp.StatusCode)
	}
	return nil
}

// Name returns the provider identifier.
func (p *LocalProvider) Name() string {
	return fmt.Sprintf("local[%s @ %s]", p.config.Model, p.config.Endpoint)
}

// buildExtractionPrompt constructs the few-shot NER+RE prompt.
func buildExtractionPrompt(text, schema string) string {
	return fmt.Sprintf(`You are a knowledge graph extraction engine. Extract concepts and relationships from the text below.

Output ONLY valid JSON matching this schema:
%s

Rules:
- Extract named entities: functions, classes, interfaces, modules, concepts, services
- Extract relationships: calls, imports, uses, implements, extends, describes, related_to
- Use confidence: EXTRACTED (direct), INFERRED (implied), AMBIGUOUS (unclear)
- IDs must be snake_case, unique, and derived from the label
- Do not include explanation text outside the JSON

Text to extract from:
---
%s
---

JSON output:`, schema, text)
}

// parseExtractionJSON parses the LLM JSON response into ExtractionResult.
func parseExtractionJSON(data []byte) (*types.ExtractionResult, error) {
	var result types.ExtractionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing extraction JSON: %w\nraw: %s", err, data)
	}
	return &result, nil
}

// apiKeyFromEnv returns the config API key or falls back to env var.
func apiKeyFromEnv(configKey, envVar string) string {
	if configKey != "" {
		return configKey
	}
	return os.Getenv(envVar)
}
