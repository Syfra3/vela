# Vela - Knowledge Explorer & Graph Builder

A high-performance, privacy-first knowledge graph builder for codebases, documentation, and technical content. Built in Go with pluggable LLM providers (local or remote) for graph extraction and analysis.

## Why Vela?

- **Privacy-First**: Run entirely locally, no data sent to cloud by default
- **Flexible LLM**: Pluggable providers (Ollama, llama.cpp, Anthropic Claude, OpenAI GPT-4o)
- **High Performance**: Go-native AST parsing, graph construction, and clustering
- **Beautiful TUI**: Interactive Bubbletea UI with real-time extraction progress
- **Microservice Mapping**: Perfect for understanding multi-repo architectures
- **Zero Token Waste**: Use local 8B models for free graph extraction

## Features

- **Code Extraction**: Tree-sitter AST parsing for 22 languages (Go, Python, TypeScript, Rust, Java, etc.)
- **Doc Extraction**: LLM-powered Named Entity Recognition & Relationship Extraction from markdown, PDFs, and comments
- **Graph Building**: Automatic construction of knowledge graph with gonum
- **Community Detection**: Leiden clustering (via graspologic Python wrapper) to find logical groupings
- **Interactive TUI**: Real-time progress tracking, estimated time, extraction percentage
- **Multiple Outputs**: graph.json (queryable), graph.html (interactive), GRAPH_REPORT.md (human-readable)
- **Caching**: SHA256-based incremental updates
- **Pluggable LLMs**: Use local models (Ollama, llama.cpp) or remote APIs (Claude, GPT-4o)

## Quick Start

```bash
# Build Vela
go build -o vela ./cmd/vela

# Extract knowledge graph from a folder
./vela extract ./my-repo

# Start interactive TUI to monitor extraction
./vela extract ./my-repo --tui

# Configure LLM provider
VELA_LLM_PROVIDER=local ./vela extract ./my-repo
VELA_LLM_MODEL=llama2 ./vela extract ./my-repo
VELA_LLM_ENDPOINT=http://localhost:11434 ./vela extract ./my-repo
```

## Architecture

### Hybrid Go + Python Design

**Go Layer (90%)**:
- CLI/TUI (Bubbletea)
- File detection & traversal
- Tree-sitter AST parsing (via `go-tree-sitter`)
- Graph construction (via `gonum/graph`)
- Export to JSON/HTML/Obsidian
- LLM client management
- Progress tracking & worker pools

**Python Layer (10%)**:
- Leiden community detection (graspologic)
- Specialized extractors (if needed)
- Runs as subprocess, called only when necessary

### Pluggable LLM Interface

```go
type LLMProvider interface {
    ExtractGraph(ctx context.Context, text string) (Nodes, Edges, error)
    Health(ctx context.Context) error
}
```

Providers:
- **Local**: Ollama or llama.cpp (0 tokens, 0 cost)
- **Remote**: Anthropic Claude (flexible, powerful)
- **Remote**: OpenAI GPT-4o (expensive, excellent quality)

## Configuration

Create `~/.vela/config.yaml`:

```yaml
llm:
  provider: "local"  # local | anthropic | openai
  model: "llama2"    # depends on provider
  endpoint: "http://localhost:11434"  # for local providers
  api_key: ""        # for remote providers

extraction:
  code_languages: ["go", "python", "typescript", "rust"]
  include_docs: true
  include_images: true
  chunk_size: 8000   # tokens per chunk for large docs

ui:
  theme: "dark"
  show_progress: true
```

## Repository Structure

```
vela/
├── cmd/vela/
│   └── main.go                 # CLI entry point
├── internal/
│   ├── detect/
│   │   └── detect.go           # File collection & filtering
│   ├── extract/
│   │   ├── extract.go          # Dispatcher
│   │   ├── code.go             # Tree-sitter AST extraction
│   │   ├── doc.go              # LLM-based doc extraction
│   │   ├── pdf.go              # PDF text extraction
│   │   └── schema.go           # Extraction result types
│   ├── graph/
│   │   ├── build.go            # Graph construction (gonum)
│   │   ├── cluster.go          # Community detection wrapper
│   │   ├── analyze.go          # God nodes + surprises
│   │   └── types.go            # Graph node/edge types
│   ├── llm/
│   │   ├── client.go           # LLM interface
│   │   ├── local.go            # Ollama/llama.cpp provider
│   │   ├── anthropic.go        # Claude provider
│   │   └── openai.go           # GPT-4o provider
│   ├── tui/
│   │   ├── app.go              # Bubbletea app
│   │   ├── progress.go         # Progress tracking component
│   │   └── styles.go           # UI styling
│   ├── report/
│   │   └── report.go           # GRAPH_REPORT.md generation
│   ├── export/
│   │   ├── json.go             # graph.json export
│   │   ├── html.go             # Interactive visualization
│   │   └── obsidian.go         # Obsidian vault export
│   ├── cache/
│   │   └── cache.go            # SHA256-based caching
│   └── security/
│       └── security.go         # Input validation
├── pkg/
│   └── types/
│       └── types.go            # Shared types
├── tests/
│   └── fixtures/               # Test data
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

## Development Status

- [ ] Phase 0: PoC (file detect + Go AST parsing + basic graph)
- [ ] Phase 1: Multi-language extraction + LLM client interface
- [ ] Phase 2: Community detection + analysis + exports
- [ ] Phase 3: Bubbletea TUI with progress tracking
- [ ] Phase 4: Advanced features (watch, incremental updates)

## License

MIT
