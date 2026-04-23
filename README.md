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

# Run the local PR quality gate
make verify

# Extract knowledge graph from a folder
./vela extract ./my-repo

# Start interactive TUI to monitor extraction
./vela extract ./my-repo --tui

# Configure LLM provider
VELA_LLM_PROVIDER=local ./vela extract ./my-repo
VELA_LLM_MODEL=llama2 ./vela extract ./my-repo
VELA_LLM_ENDPOINT=http://localhost:11434 ./vela extract ./my-repo
```

## Installation

### What Vela is for

Vela is the **graph extraction and retrieval** layer.

Use Vela when you want to:
- extract a graph from a repo or workspace
- query graph structure with MCP tools
- run Vela by itself without Ancora

Do **not** use Vela as your durable memory database. That is Ancora's job.

### Install from source

```bash
git clone https://github.com/Syfra3/vela.git
cd vela
go build -o vela ./cmd/vela

# Optional: move it somewhere in PATH
sudo mv vela /usr/local/bin/

# Verify
vela --help
```

### Local quality gate

Use the same gate locally that CI runs on pull requests:

```bash
make verify
```

Install repository-managed hooks to catch issues before push:

```bash
make hooks-install
```

Notes:
- `make lint` bootstraps the pinned `golangci-lint` version automatically.
- `make verify` uses `gotestsum` when available and falls back to `go test -v`.
- Vela intentionally scopes automated tests to `./cmd/... ./internal/... ./pkg/...` because `tests/fixtures/detect/**` contains fixture files that are expected to break raw `go test ./...` analysis.

### MCP usage

Vela now exposes a real **stdio MCP server**.

```bash
# Start Vela as an MCP server over stdio
vela serve
```

That command is for MCP clients.
It does **not** print a normal terminal UI.

If you want the old HTTP server for debugging or legacy use:

```bash
vela serve --http --port 7700
```

### Three simple ways to use Vela

#### 1. Vela only

Use this if you only want graph extraction and graph retrieval.

```bash
# Build graph
vela extract ./my-repo

# Let an MCP client talk to Vela
vela serve
```

#### 2. Ancora + Vela

Use this if you want:
- Ancora for long-term memory
- Vela for graph retrieval
- one primary MCP surface from Ancora

In this setup:
- Ancora stays the main MCP server
- Vela runs behind Ancora for `vela_*` graph tools
- memory writes still belong to Ancora

#### 3. Local CLI only

Use this if you just want to generate and inspect graphs yourself.

```bash
vela extract ./my-repo
vela query "nodes"
vela path AuthService Database
vela explain AuthService
```

### Example MCP config

If you want to register Vela directly as its own MCP server:

#### Claude Code

Create `~/.claude/mcp/vela.json`:

```json
{
  "command": "vela",
  "args": ["serve"]
}
```

#### OpenCode

Add to `~/.config/opencode/mcp_settings.json`:

```json
{
  "mcpServers": {
    "vela": {
      "command": "vela",
      "args": ["serve"]
    }
  }
}
```

### Which tool should I install?

- Install **Ancora only** if you want memory.
- Install **Vela only** if you want graph extraction and graph retrieval.
- Install **both** if you want memory + graph retrieval together.

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
- Community detection (Leiden via `graspologic` when available, NetworkX modularity fallback otherwise)
- Specialized extractors (if needed)
- Runs as subprocess, called only when necessary

Install the clustering dependency explicitly if you want community detection to run:

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements-clustering.txt
```

If you specifically want the Leiden backend, install `graspologic` into that same virtualenv when it supports your Python version.

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
