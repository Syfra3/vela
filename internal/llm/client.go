package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

// Client manages LLM provider selection and extraction
type Client struct {
	config   *types.LLMConfig
	provider types.LLMProvider
}

// NewClient creates a new LLM client with the specified provider
func NewClient(config *types.LLMConfig) (*Client, error) {
	var provider types.LLMProvider
	var err error

	switch config.Provider {
	case "local":
		provider, err = NewLocalProvider(config)
	case "anthropic":
		provider, err = NewAnthropicProvider(config)
	case "openai":
		provider, err = NewOpenAIProvider(config)
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", config.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize %s provider: %w", config.Provider, err)
	}

	return &Client{
		config:   config,
		provider: provider,
	}, nil
}

// ExtractGraph extracts graph from text using the configured LLM provider
func (c *Client) ExtractGraph(ctx context.Context, text string) (*types.ExtractionResult, error) {
	// Define the JSON schema for extraction
	schema := extractionSchema()

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	return c.provider.ExtractGraph(ctx, text, schema)
}

// Health checks if the LLM provider is accessible
func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return c.provider.Health(ctx)
}

// Provider returns the current LLM provider name
func (c *Client) Provider() string {
	return c.provider.Name()
}

// extractionSchema returns the JSON schema for graph extraction
// This ensures the LLM outputs structured data that we can parse
func extractionSchema() string {
	return `{
  "type": "object",
  "properties": {
    "nodes": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "label": {"type": "string"},
          "type": {"type": "string", "enum": ["function", "class", "concept", "file", "module", "interface", "struct", "constant"]},
          "description": {"type": "string"}
        },
        "required": ["id", "label", "type"]
      }
    },
    "edges": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "source": {"type": "string"},
          "target": {"type": "string"},
          "relation": {"type": "string", "enum": ["calls", "imports", "uses", "implements", "extends", "describes", "related_to"]},
          "confidence": {"type": "string", "enum": ["EXTRACTED", "INFERRED", "AMBIGUOUS"]},
          "score": {"type": "number", "minimum": 0, "maximum": 1}
        },
        "required": ["source", "target", "relation", "confidence"]
      }
    }
  },
  "required": ["nodes", "edges"]
}`
}
