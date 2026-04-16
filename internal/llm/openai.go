package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Syfra3/vela/pkg/types"
)

// OpenAIProvider implements types.LLMProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	config *types.LLMConfig
	client *http.Client
	apiKey string
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(config *types.LLMConfig) (*OpenAIProvider, error) {
	key := apiKeyFromEnv(config.APIKey, "OPENAI_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required for openai provider")
	}
	if config.Model == "" {
		config.Model = "gpt-4o"
	}
	if config.Endpoint == "" {
		config.Endpoint = "https://api.openai.com/v1"
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &OpenAIProvider{
		config: config,
		client: &http.Client{Timeout: timeout},
		apiKey: key,
	}, nil
}

// openAIRequest is the chat completions request body.
type openAIRequest struct {
	Model          string            `json:"model"`
	Messages       []openAIMessage   `json:"messages"`
	ResponseFormat *openAIRespFormat `json:"response_format,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRespFormat struct {
	Type       string            `json:"type"`
	JSONSchema *openAIJSONSchema `json:"json_schema,omitempty"`
}

type openAIJSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ExtractGraph sends text to the OpenAI chat completions endpoint with JSON
// schema enforcement and parses the structured extraction result.
func (p *OpenAIProvider) ExtractGraph(ctx context.Context, text string, schema string) (*types.ExtractionResult, error) {
	systemMsg := fmt.Sprintf(`You are a knowledge graph extraction engine.
Extract concepts and relationships from text and output ONLY valid JSON matching the provided schema.
Do not include any explanation or markdown outside the JSON.

Schema:
%s`, schema)

	reqBody, err := json.Marshal(openAIRequest{
		Model: p.config.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: text},
		},
		ResponseFormat: &openAIRespFormat{
			Type: "json_schema",
			JSONSchema: &openAIJSONSchema{
				Name:   "extraction_result",
				Schema: json.RawMessage(schema),
				Strict: true,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.config.Endpoint+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai returned %d: %s", resp.StatusCode, raw)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing openai response: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("openai error: %s", apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from openai")
	}

	return parseExtractionJSON([]byte(apiResp.Choices[0].Message.Content))
}

// Health checks if the OpenAI-compatible endpoint is reachable.
func (p *OpenAIProvider) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.config.Endpoint+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("openai endpoint unreachable at %s: %w", p.config.Endpoint, err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("openai health check failed: status %d", resp.StatusCode)
	}
	return nil
}

// Name returns the provider identifier.
func (p *OpenAIProvider) Name() string {
	return fmt.Sprintf("openai[%s]", p.config.Model)
}
