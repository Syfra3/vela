package types

import (
	"context"
	"time"
)

// Node represents a concept, class, function, or entity in the knowledge graph
type Node struct {
	ID             string                 `json:"id"`
	Label          string                 `json:"label"`
	NodeType       string                 `json:"type"` // function, class, concept, file, etc.
	SourceFile     string                 `json:"source_file,omitempty"`
	SourceLocation string                 `json:"source_location,omitempty"` // L42:C5
	Community      int                    `json:"community,omitempty"`
	Degree         int                    `json:"degree,omitempty"`
	Description    string                 `json:"description,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// Edge represents a relationship between two nodes
type Edge struct {
	Source     string                 `json:"source"`
	Target     string                 `json:"target"`
	Relation   string                 `json:"relation"`        // calls, imports, uses, describes, etc.
	Confidence string                 `json:"confidence"`      // EXTRACTED, INFERRED, AMBIGUOUS
	Score      float64                `json:"score,omitempty"` // 0.0-1.0
	SourceFile string                 `json:"source_file,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ExtractionResult is returned by extractors (code, doc, pdf)
type ExtractionResult struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Graph represents the complete knowledge graph
type Graph struct {
	Nodes       []Node                 `json:"nodes"`
	Edges       []Edge                 `json:"edges"`
	Communities map[int][]string       `json:"communities"`
	Metadata    map[string]interface{} `json:"metadata"`
	ExtractedAt time.Time              `json:"extracted_at"`
}

// LLMProvider defines the interface for pluggable LLM backends
type LLMProvider interface {
	// ExtractGraph sends text to the LLM and requests structured graph extraction
	ExtractGraph(ctx context.Context, text string, schema string) (*ExtractionResult, error)

	// Health checks if the LLM provider is accessible
	Health(ctx context.Context) error

	// Name returns the provider name for logging
	Name() string
}

// LLMConfig holds configuration for LLM providers
type LLMConfig struct {
	Provider       string        `yaml:"provider"` // local, anthropic, openai
	Model          string        `yaml:"model"`
	Endpoint       string        `yaml:"endpoint"` // for local providers
	APIKey         string        `yaml:"api_key"`  // for remote providers
	Timeout        time.Duration `yaml:"timeout"`
	MaxChunkTokens int           `yaml:"max_chunk_tokens"`
}

// ExtractionConfig holds configuration for extraction behavior
type ExtractionConfig struct {
	CodeLanguages []string `yaml:"code_languages"`
	IncludeDocs   bool     `yaml:"include_docs"`
	IncludeImages bool     `yaml:"include_images"`
	ChunkSize     int      `yaml:"chunk_size"` // tokens
	CacheDir      string   `yaml:"cache_dir"`
}

// UIConfig holds configuration for TUI behavior
type UIConfig struct {
	Theme        string `yaml:"theme"`
	ShowProgress bool   `yaml:"show_progress"`
	EnableColors bool   `yaml:"enable_colors"`
}

// Config is the global configuration for Vela
type Config struct {
	LLM        LLMConfig        `yaml:"llm"`
	Extraction ExtractionConfig `yaml:"extraction"`
	UI         UIConfig         `yaml:"ui"`
}

// ExtractionProgress tracks the progress of document extraction
type ExtractionProgress struct {
	TotalFiles      int
	ProcessedFiles  int
	TotalChunks     int
	ProcessedChunks int
	CurrentFile     string
	CurrentChunk    int
	StartTime       time.Time
	LastUpdateTime  time.Time
}

// ProgressUpdate is sent on a channel to update UI with extraction progress
type ProgressUpdate struct {
	Progress         ExtractionProgress
	Error            error
	IsComplete       bool
	EstimatedSeconds int
}

// Percentage returns the extraction progress as a percentage (0-100)
func (p *ExtractionProgress) Percentage() int {
	if p.TotalChunks == 0 {
		return 0
	}
	return int((float64(p.ProcessedChunks) / float64(p.TotalChunks)) * 100)
}

// ElapsedSeconds returns seconds since extraction started
func (p *ExtractionProgress) ElapsedSeconds() int {
	return int(time.Since(p.StartTime).Seconds())
}

// EstimatedTotalSeconds returns the estimated total time in seconds
func (p *ExtractionProgress) EstimatedTotalSeconds() int {
	if p.ProcessedChunks == 0 {
		return 0
	}
	elapsed := time.Since(p.StartTime).Seconds()
	rate := elapsed / float64(p.ProcessedChunks)
	return int(rate * float64(p.TotalChunks))
}

// EstimatedRemainingSeconds returns estimated remaining time in seconds
func (p *ExtractionProgress) EstimatedRemainingSeconds() int {
	total := p.EstimatedTotalSeconds()
	elapsed := p.ElapsedSeconds()
	if elapsed > total {
		return 0
	}
	return total - elapsed
}
