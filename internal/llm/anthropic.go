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

const anthropicAPIBase = "https://api.anthropic.com/v1"
const anthropicVersion = "2023-06-01"

// AnthropicProvider implements types.LLMProvider for Anthropic Claude.
type AnthropicProvider struct {
	config *types.LLMConfig
	client *http.Client
	apiKey string
}

// NewAnthropicProvider creates a new Anthropic Claude provider.
func NewAnthropicProvider(config *types.LLMConfig) (*AnthropicProvider, error) {
	key := apiKeyFromEnv(config.APIKey, "ANTHROPIC_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required for anthropic provider")
	}
	if config.Model == "" {
		config.Model = "claude-3-5-sonnet-20241022"
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &AnthropicProvider{
		config: config,
		client: &http.Client{Timeout: timeout},
		apiKey: key,
	}, nil
}

// anthropicRequest is the messages API request body.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ExtractGraph sends text to Claude and parses the structured extraction.
func (p *AnthropicProvider) ExtractGraph(ctx context.Context, text string, schema string) (*types.ExtractionResult, error) {
	system := fmt.Sprintf(`You are a knowledge graph extraction engine. 
Extract concepts and relationships and output ONLY valid JSON matching this schema:
%s
Do not include any explanation or markdown outside the JSON.`, schema)

	reqBody, err := json.Marshal(anthropicRequest{
		Model:     p.config.Model,
		MaxTokens: 4096,
		System:    system,
		Messages: []anthropicMessage{
			{Role: "user", Content: text},
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		anthropicAPIBase+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, raw)
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing anthropic response: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", apiResp.Error.Message)
	}
	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic")
	}

	return parseExtractionJSON([]byte(apiResp.Content[0].Text))
}

// Health performs a lightweight check against the Anthropic API.
func (p *AnthropicProvider) Health(ctx context.Context) error {
	// Anthropic has no public ping endpoint — use a minimal models list.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		anthropicAPIBase+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic unreachable: %w", err)
	}
	resp.Body.Close()
	// 200 or 404 (endpoint may not exist) both mean the API is reachable
	if resp.StatusCode >= 500 {
		return fmt.Errorf("anthropic health check failed: status %d", resp.StatusCode)
	}
	return nil
}

// Name returns the provider identifier.
func (p *AnthropicProvider) Name() string {
	return fmt.Sprintf("anthropic[%s]", p.config.Model)
}
