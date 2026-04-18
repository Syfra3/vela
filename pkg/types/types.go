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
	// Source identifies where this node came from (codebase, memory, webhook).
	// Nodes from the same extraction share the same Source.
	Source *Source `json:"source,omitempty"`
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

// ---------------------------------------------------------------------------
// Knowledge node types (Ancora integration — spec §7)
// ---------------------------------------------------------------------------

// NodeType is a discriminated string for all node varieties in the graph.
type NodeType string

const (
	// Code-derived node types (existing conventions)
	NodeTypeFunction  NodeType = "function"
	NodeTypeStruct    NodeType = "struct"
	NodeTypeInterface NodeType = "interface"
	NodeTypeFile      NodeType = "file"
	NodeTypePackage   NodeType = "package"

	// Knowledge node types (Ancora integration)
	NodeTypeObservation  NodeType = "observation"
	NodeTypeConcept      NodeType = "concept"
	NodeTypeWorkspace    NodeType = "workspace"
	NodeTypeVisibility   NodeType = "visibility"
	NodeTypeOrganization NodeType = "organization"
)

// EdgeType is a discriminated string for graph relationships.
type EdgeType string

const (
	// Code-derived edge types (existing conventions)
	EdgeTypeCalls      EdgeType = "calls"
	EdgeTypeImports    EdgeType = "imports"
	EdgeTypeImplements EdgeType = "implements"

	// Knowledge edge types (Ancora integration)
	EdgeTypeReferences EdgeType = "references" // obs -> file/function (generic fallback)
	EdgeTypeRelatedTo  EdgeType = "related_to" // obs -> obs
	EdgeTypeDefines    EdgeType = "defines"    // obs -> concept
	EdgeTypeBelongsTo  EdgeType = "belongs_to" // obs -> workspace/visibility

	// Typed cross-source relations (derived from observation ObsType)
	EdgeTypeDocuments   EdgeType = "documents"   // architecture/discovery obs -> code
	EdgeTypeDecidesOn   EdgeType = "decides_on"  // decision obs -> code
	EdgeTypeConstrains  EdgeType = "constrains"  // bugfix obs -> code
	EdgeTypeExemplifies EdgeType = "exemplifies" // pattern obs -> code
	EdgeTypeDeprecates  EdgeType = "deprecates"  // deprecation obs -> code
)

// ---------------------------------------------------------------------------
// Source attribution — where knowledge came from
// ---------------------------------------------------------------------------

// SourceType discriminates knowledge sources.
type SourceType string

const (
	// SourceTypeCodebase is a local or git-tracked codebase (files, functions, deps).
	SourceTypeCodebase SourceType = "codebase"
	// SourceTypeMemory is Ancora persistent memory (observations, decisions).
	SourceTypeMemory SourceType = "memory"
	// SourceTypeWebhook is external event sources (future: Jira, GitHub, etc.).
	SourceTypeWebhook SourceType = "webhook"
)

// Source describes where a node or extraction result originated.
// For codebases: detected from git remote or folder name.
// For memory: always "ancora" with workspace/visibility metadata on nodes.
type Source struct {
	// Type discriminates the source category.
	Type SourceType `json:"type"`
	// Name is the project/repo name (e.g., "vela", "glim", "ancora").
	Name string `json:"name"`
	// Path is the local filesystem path (codebase only).
	Path string `json:"path,omitempty"`
	// Remote is the git remote URL if detected (codebase only).
	Remote string `json:"remote,omitempty"`
}

// NodeTypeProject is the root node for a codebase extraction.
const NodeTypeProject NodeType = "project"

// NodeTypeMemorySource is the root node for all ancora memory.
const NodeTypeMemorySource NodeType = "memory_source"

// ObsReference describes a relationship from an observation to another artifact.
// It mirrors the wire format used by Ancora but lives in the Vela type system.
type ObsReference struct {
	// Type is one of: "file", "observation", "concept", "function"
	Type string `json:"type"`
	// Target is the referenced artifact (path, ID, name, etc.)
	Target string `json:"target"`
}

// ObservationNode represents an Ancora memory observation as a graph node.
type ObservationNode struct {
	// ID is the Vela node ID — "ancora:obs:<ancoraID>"
	ID       string
	NodeType NodeType
	// AncoraID is the original Ancora observation primary key.
	AncoraID int64
	Title    string
	Content  string
	// ObsType is the Ancora observation category: "decision", "bugfix", etc.
	ObsType      string
	Workspace    string
	Visibility   string
	Organization string
	TopicKey     string
	References   []ObsReference
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// Source identifies this node as memory-derived (set by the patcher/extractor).
	Source *Source
}

// ToNode converts an ObservationNode to a generic types.Node for graph storage.
func (o *ObservationNode) ToNode() Node {
	metadata := map[string]interface{}{
		"ancora_id":  o.AncoraID,
		"obs_type":   o.ObsType,
		"workspace":  o.Workspace,
		"visibility": o.Visibility,
		"content":    o.Content,
	}
	if o.Organization != "" {
		metadata["organization"] = o.Organization
	}
	if o.TopicKey != "" {
		metadata["topic_key"] = o.TopicKey
	}
	if !o.CreatedAt.IsZero() {
		metadata["created_at"] = o.CreatedAt.Format("2006-01-02")
	}

	return Node{
		ID:          o.ID,
		Label:       o.Title,
		NodeType:    string(o.NodeType),
		SourceFile:  o.ID,
		Description: o.Content,
		Source:      o.Source,
		Metadata:    metadata,
	}
}

// ---------------------------------------------------------------------------
// Watch / Daemon configuration (spec §8)
// ---------------------------------------------------------------------------

// WatchSourceConfig configures a single event source for the daemon.
type WatchSourceConfig struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`   // "syfra" | "webhook"
	Socket string `yaml:"socket"` // path for syfra sources
}

// ReconcilerConfig controls event batching behaviour.
type ReconcilerConfig struct {
	DebounceMs   int `yaml:"debounce_ms"`
	MaxBatchSize int `yaml:"max_batch_size"`
}

// ExtractorConfig controls LLM-based reference extraction.
type ExtractorConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Workers   int    `yaml:"workers"`
	WriteBack bool   `yaml:"write_back"`
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
}

// WatchConfig is the top-level daemon watch configuration.
type WatchConfig struct {
	Enabled    bool                `yaml:"enabled"`
	Sources    []WatchSourceConfig `yaml:"sources"`
	Reconciler ReconcilerConfig    `yaml:"reconciler"`
	Extractor  ExtractorConfig     `yaml:"extractor"`
}

// DaemonConfig configures the background daemon process.
type DaemonConfig struct {
	PIDFile    string `yaml:"pid_file"`
	LogFile    string `yaml:"log_file"`
	LogLevel   string `yaml:"log_level"`
	StatusFile string `yaml:"status_file"`
}

// DaemonSourceStatus holds per-source connectivity info written to the status file.
type DaemonSourceStatus struct {
	Connected  bool  `json:"connected"`
	EventCount int64 `json:"event_count"`
}

// DaemonStatus is written by the daemon to StatusFile so the CLI can read
// source connectivity without cross-process registry access.
type DaemonStatus struct {
	PID            int                           `json:"pid"`
	Sources        map[string]DaemonSourceStatus `json:"sources"` // name -> status
	UpdatedAt      string                        `json:"updated_at"`
	LastGraphFlush string                        `json:"last_graph_flush,omitempty"` // RFC3339 timestamp of last graph.json write
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

// ObsidianConfig controls automatic Obsidian vault sync.
// When AutoSync is true the daemon writes the vault after every successful
// reconcile. VaultDir is the vault root and Vela writes into
// <VaultDir>/obsidian/ mirroring the manual `vela extract` layout.
type ObsidianConfig struct {
	AutoSync bool   `yaml:"auto_sync"`
	VaultDir string `yaml:"vault_dir"` // e.g. "~/Documents/vela"
}

// GraphConfig controls how the daemon persists graph.json to disk.
// When AutoPersist is true the daemon flushes its in-memory graph back to
// graph.json after every reconcile cycle (debounced by FlushInterval).
// This keeps graph.json — the canonical source of truth — up-to-date with
// memory events received via the Ancora socket.
type GraphConfig struct {
	AutoPersist   bool          `yaml:"auto_persist"`
	FlushInterval time.Duration `yaml:"flush_interval"`
}

// Config is the global configuration for Vela
type Config struct {
	LLM        LLMConfig        `yaml:"llm"`
	Extraction ExtractionConfig `yaml:"extraction"`
	UI         UIConfig         `yaml:"ui"`
	Watch      WatchConfig      `yaml:"watch"`
	Daemon     DaemonConfig     `yaml:"daemon"`
	Obsidian   ObsidianConfig   `yaml:"obsidian"`
	Graph      GraphConfig      `yaml:"graph"`
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
