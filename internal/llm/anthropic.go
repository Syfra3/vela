package llm

import (
	"context"
	"fmt"

	"github.com/Syfra3/vela/pkg/types"
)

// AnthropicProvider implements types.LLMProvider for Anthropic Claude
type AnthropicProvider struct {
	config *types.LLMConfig
}

// NewAnthropicProvider creates a new Anthropic Claude provider
func NewAnthropicProvider(config *types.LLMConfig) (*AnthropicProvider, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required for anthropic provider")
	}
	if config.Model == "" {
		config.Model = "claude-3-5-sonnet-20241022"
	}
	return &AnthropicProvider{config: config}, nil
}

// ExtractGraph implements types.LLMProvider
// Sends text to Claude with structured output (tool_use / JSON schema)
func (p *AnthropicProvider) ExtractGraph(ctx context.Context, text string, schema string) (*types.ExtractionResult, error) {
	// TODO Phase 1: POST to https://api.anthropic.com/v1/messages
	// Use tool_use with the extraction schema to enforce structured output
	// Parse response into ExtractionResult
	return nil, fmt.Errorf("not implemented yet")
}

// Health checks the Anthropic API is accessible
func (p *AnthropicProvider) Health(ctx context.Context) error {
	// TODO Phase 1: simple models list or ping
	return nil
}

// Name returns provider identifier
func (p *AnthropicProvider) Name() string {
	return fmt.Sprintf("anthropic[%s]", p.config.Model)
}
