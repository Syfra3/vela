package llm

import (
	"context"
	"fmt"

	"github.com/Syfra3/vela/pkg/types"
)

// OpenAIProvider implements types.LLMProvider for OpenAI GPT models
type OpenAIProvider struct {
	config *types.LLMConfig
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(config *types.LLMConfig) (*OpenAIProvider, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required for openai provider")
	}
	if config.Model == "" {
		config.Model = "gpt-4o"
	}
	if config.Endpoint == "" {
		config.Endpoint = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{config: config}, nil
}

// ExtractGraph implements types.LLMProvider
// Sends text to GPT-4o with response_format JSON schema enforcement
func (p *OpenAIProvider) ExtractGraph(ctx context.Context, text string, schema string) (*types.ExtractionResult, error) {
	// TODO Phase 1: POST to {endpoint}/chat/completions
	// Use response_format: { type: "json_schema", json_schema: {...} }
	// This is the OpenAI-native JSON schema enforcement — zero freeform output
	return nil, fmt.Errorf("not implemented yet")
}

// Health checks the OpenAI API is accessible
func (p *OpenAIProvider) Health(ctx context.Context) error {
	// TODO Phase 1: GET {endpoint}/models
	return nil
}

// Name returns provider identifier
func (p *OpenAIProvider) Name() string {
	return fmt.Sprintf("openai[%s]", p.config.Model)
}
