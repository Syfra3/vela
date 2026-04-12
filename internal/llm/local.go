package llm

import (
	"context"
	"fmt"

	"github.com/Syfra3/vela/pkg/types"
)

// LocalProvider implements types.LLMProvider for local models (Ollama, llama.cpp)
type LocalProvider struct {
	config *types.LLMConfig
}

// NewLocalProvider creates a new local LLM provider
func NewLocalProvider(config *types.LLMConfig) (*LocalProvider, error) {
	if config.Endpoint == "" {
		config.Endpoint = "http://localhost:11434" // Ollama default
	}
	if config.Model == "" {
		config.Model = "llama3" // Default model
	}
	return &LocalProvider{config: config}, nil
}

// ExtractGraph implements types.LLMProvider
// Sends text to Ollama/llama.cpp and enforces JSON schema output
func (p *LocalProvider) ExtractGraph(ctx context.Context, text string, schema string) (*types.ExtractionResult, error) {
	// TODO Phase 1: POST to {endpoint}/v1/chat/completions (OpenAI-compatible)
	// Use JSON schema enforcement to force structured output
	// Parse response into ExtractionResult
	return nil, fmt.Errorf("not implemented yet")
}

// Health checks if the local endpoint is accessible
func (p *LocalProvider) Health(ctx context.Context) error {
	// TODO Phase 1: GET {endpoint}/api/tags or /health
	return nil
}

// Name returns provider identifier
func (p *LocalProvider) Name() string {
	return fmt.Sprintf("local[%s @ %s]", p.config.Model, p.config.Endpoint)
}
