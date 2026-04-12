# PRD: Vela — Knowledge Explorer & Graph Builder

**Project**: Vela  
**Repo**: github.com/Syfra3/vela  
**Author**: G33N / Syfra3  
**Date**: 2026-04-12  
**Status**: Draft  

---

## 1. Problem Statement

Modern software engineering increasingly relies on multi-repo microservice architectures. These systems suffer from a fundamental knowledge problem:

- **No one knows what depends on what.** Dependencies between services live in runtime behaviour, not documentation.
- **Documentation rots.** README files, architecture diagrams, and ADRs fall out of sync with the code.
- **Onboarding takes months.** New engineers spend weeks just mapping the system before they can contribute.
- **Breaking changes are invisible.** There is no automated way to ask "if I change this interface, what services break?"
- **Existing tools require cloud or Python.** Tools like Graphify (Python) work well but require a Python runtime and send documentation content to cloud LLMs, which is unacceptable for private or enterprise codebases.

---

## 2. Solution

**Vela** is a Go-native, privacy-first knowledge graph builder for codebases and technical documentation.

Vela automatically extracts concepts, relationships, and dependencies from code, markdown, PDFs, and configuration files across multiple repositories, and builds a queryable knowledge graph that can be explored interactively via a Bubbletea TUI or exported to JSON/HTML/Obsidian.

### Core Insight
- **Code extraction is 100% local.** Tree-sitter AST parsing runs entirely on-device with no LLM calls. Zero cost, zero latency, zero privacy risk.
- **Document extraction uses a pluggable LLM.** Markdown/PDF/comment extraction uses an LLM for Named Entity Recognition and Relationship Extraction — but the provider is configurable: local (Ollama, llama.cpp) or remote (Claude, GPT-4o).
- **Clustering is Go + Python.** Graph construction and analysis run in Go (gonum). Community detection uses Leiden (graspologic via Python subprocess) — the best clustering algorithm, called only once at the end.
- **Single binary.** Unlike Graphify, Vela ships as a single Go binary. No Python runtime required by the end user.

---

## 3. Target Users

| User | Pain | How Vela Helps |
|------|------|----------------|
| **Platform Engineer** | No map of 30+ microservices and their dependencies | `vela extract ./repos --directed` → visual graph of all service dependencies |
| **New Developer** | 3-month onboarding to understand system architecture | `GRAPH_REPORT.md` shows god nodes, communities, and entry points in minutes |
| **Tech Lead** | Can't assess the blast radius of a refactor | `vela query path "UserSchema" "PaymentController"` → exact dependency chain |
| **Security Engineer** | Unknown data flows between services | Graph shows all service-to-service data flows extracted from docs + code |
| **DevOps / SRE** | Architecture docs 6 months out of date | `vela extract --watch` auto-updates graph as code changes |

---

## 4. Architecture

### Hybrid: Go Core + Python Subprocess

```
┌─────────────────────────────────────────────────────────┐
│                     Vela CLI / TUI                       │
│                      (Go / Cobra)                        │
└────────────────────────┬────────────────────────────────┘
                         │
         ┌───────────────┼───────────────┐
         ▼               ▼               ▼
   ┌───────────┐  ┌────────────┐  ┌──────────────┐
   │  Detect   │  │  Extract   │  │    Export    │
   │ (Go/fs)   │  │ (Go+LLM)  │  │  (Go/JSON    │
   └───────────┘  └─────┬──────┘  │   HTML/Obs)  │
                        │         └──────────────┘
              ┌─────────┴──────────┐
              ▼                    ▼
        ┌──────────┐        ┌────────────┐
        │   Code   │        │    Docs    │
        │ (Tree-   │        │   (LLM     │
        │ sitter)  │        │  Provider) │
        └──────────┘        └─────┬──────┘
                                  │
                    ┌─────────────┼──────────────┐
                    ▼             ▼               ▼
             ┌──────────┐ ┌────────────┐ ┌────────────┐
             │  Local   │ │ Anthropic  │ │   OpenAI   │
             │ (Ollama/ │ │  (Claude)  │ │  (GPT-4o)  │
             │ llama.cpp│ └────────────┘ └────────────┘
             └──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │   Graph (gonum)     │
              │   Build + Analyze   │
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │ Leiden Clustering   │
              │ (Python subprocess  │
              │  graspologic)       │
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │  Bubbletea TUI      │
              │  (Progress / Query) │
              └─────────────────────┘
```

### Directory Structure

```
vela/
├── cmd/vela/main.go              # CLI entry point
├── internal/
│   ├── detect/detect.go          # File collection + .velignore
│   ├── extract/
│   │   ├── extract.go            # Dispatcher (code vs doc)
│   │   ├── code.go               # Tree-sitter AST extraction
│   │   ├── doc.go                # LLM-based doc/PDF extraction
│   │   └── schema.go             # Extraction result types
│   ├── graph/
│   │   ├── build.go              # Graph construction (gonum)
│   │   ├── cluster.go            # Leiden wrapper (Python subprocess)
│   │   ├── analyze.go            # God nodes + surprises + questions
│   │   └── types.go              # Graph node/edge types
│   ├── llm/
│   │   ├── client.go             # Provider dispatcher
│   │   ├── local.go              # Ollama / llama.cpp
│   │   ├── anthropic.go          # Claude
│   │   └── openai.go             # GPT-4o (OpenAI-compatible)
│   ├── tui/
│   │   ├── app.go                # Bubbletea model
│   │   ├── progress.go           # Progress renderer (%, ETA, file)
│   │   └── styles.go             # Lipgloss styles
│   ├── report/report.go          # GRAPH_REPORT.md generation
│   ├── export/
│   │   ├── json.go               # graph.json
│   │   ├── html.go               # Interactive vis.js HTML
│   │   └── obsidian.go           # Obsidian vault
│   ├── cache/cache.go            # SHA256 incremental cache
│   └── security/security.go      # Input validation
├── pkg/types/types.go            # Shared types + interfaces
└── go.mod
```

---

## 5. Features

### Phase 0 — PoC (must have before anything else)
| Feature | Description |
|---------|-------------|
| File detection | Walk directory, filter by extension, respect `.velignore` |
| Go AST extraction | Tree-sitter parses Go files → functions, structs, interfaces, calls |
| Basic graph | gonum graph from extracted nodes/edges |
| JSON export | Output `vela-out/graph.json` |
| CLI | `vela extract <path>` command via Cobra |

**Success gate**: `vela extract .` on the Vela repo itself produces a valid `graph.json`.

### Phase 1 — Core Extraction
| Feature | Description |
|---------|-------------|
| Multi-language AST | Add Python, TypeScript, JavaScript, Java, Rust |
| LLM doc extraction | Markdown, `.txt`, inline comments with LLM provider |
| PDF extraction | ledongthuc/pdf → text → LLM extraction |
| LLM providers | local (Ollama/llama.cpp), anthropic (Claude), openai (GPT-4o) |
| JSON Schema enforcement | Force structured output from local models (GBNF grammar) |
| Document chunking | Split large docs for local models with limited context |
| SHA256 caching | Skip unchanged files on re-runs |
| Input validation | Path containment, label sanitization |

### Phase 2 — Analysis & Exports
| Feature | Description |
|---------|-------------|
| Leiden clustering | Python subprocess → graspologic → community assignments |
| God nodes | Highest-degree concepts in the graph |
| Surprise edges | High-scoring cross-file/cross-domain connections |
| Suggested questions | 4-5 questions the graph is uniquely positioned to answer |
| GRAPH_REPORT.md | Human-readable summary report |
| HTML export | Interactive vis.js visualization (`graph.html`) |
| Obsidian export | Vault with wikilinks matching graph structure |

### Phase 3 — Bubbletea TUI
| Feature | Description |
|---------|-------------|
| Real-time progress | File name, chunks processed/total, %, elapsed, ETA |
| Worker pool | Configurable concurrent LLM requests (1-N, respects local hardware) |
| Provider health check | Shows LLM provider status before starting extraction |
| Interactive query | Query the graph from TUI: `path`, `query`, `explain` |
| Community explorer | Browse communities and god nodes interactively |

### Phase 4 — Advanced (Future)
| Feature | Description |
|---------|-------------|
| `--watch` mode | fsnotify-based auto-update as files change |
| `--update` mode | Incremental re-extraction (changed files only) |
| MCP server | `vela serve graph.json` — structured graph access via MCP protocol |
| Neo4j export | `--neo4j-push bolt://localhost:7687` |
| Git hooks | `vela hook install` — rebuild graph on commit/branch switch |
| Ancora integration | `vela extract → graph nodes linked to Ancora observations` |

---

## 6. LLM Provider Strategy

### Why pluggable?

The extraction quality vs. cost/privacy tradeoff is different for every user:

| Provider | Quality | Cost | Privacy | Best For |
|----------|---------|------|---------|----------|
| **local (Ollama)** | Good (8B models) | Free | 100% local | Private codebases, cost-sensitive |
| **local (llama.cpp)** | Good | Free | 100% local | Offline, custom models |
| **anthropic (Claude)** | Excellent | $$$ | Cloud | Best quality, non-sensitive docs |
| **openai (GPT-4o)** | Excellent | $$$ | Cloud | Best quality, OpenAI-compatible APIs |

### Local LLM Engineering Constraints

When using local models, Vela must handle:
1. **JSON Schema enforcement** — GBNF grammar forces structured output at token level (no freeform text)
2. **Document chunking** — Split docs into 8k-token chunks for models with limited context windows
3. **Worker pool** — 1-2 concurrent requests (local hardware bottleneck), shown in TUI
4. **Few-shot prompting** — Optimized system prompt for 8B models to extract implicit relationships

---

## 7. TUI Progress Requirements

The TUI is critical for user confidence during long extraction runs. Users MUST be able to:

1. **See what is being processed**: current file name (truncated to fit)
2. **See completion**: `chunks processed / total chunks` + percentage bar
3. **Estimate duration**: elapsed time + ETA in `Xm Ys` format
4. **Know the LLM provider**: name + health status (`ready` / `offline`)
5. **NOT kill the process**: confidence that it's running, not stuck

The progress state is driven by `types.ExtractionProgress` which tracks files, chunks, and time. The `EstimatedRemainingSeconds()` method gives real-time ETA from the current processing rate.

---

## 8. Configuration

`~/.vela/config.yaml`:
```yaml
llm:
  provider: "local"        # local | anthropic | openai
  model: "llama3"          # depends on provider
  endpoint: "http://localhost:11434"  # for local providers
  api_key: ""              # for remote providers
  timeout: 60s
  max_chunk_tokens: 8000

extraction:
  code_languages: ["go", "python", "typescript", "rust", "java"]
  include_docs: true
  include_images: false    # requires vision model
  chunk_size: 8000
  cache_dir: "~/.vela/cache"

ui:
  theme: "dark"
  show_progress: true
  enable_colors: true
```

Also: `.velignore` file (same syntax as `.gitignore`) for excluding paths.

---

## 9. CLI Interface

```bash
# Core
vela extract <path>                     # extract graph from folder
vela extract <path> --directed          # preserve edge direction
vela extract <path> --no-viz            # skip HTML, JSON only
vela extract <path> --provider local    # override LLM provider
vela extract <path> --model llama3      # override model

# Query (Phase 2+)
vela query "what connects auth to payments?"
vela path "UserSchema" "PaymentController"
vela explain "AuthService"

# Config
vela config init                        # create ~/.vela/config.yaml
vela doctor                             # check LLM provider health

# Advanced (Phase 4)
vela extract <path> --watch             # auto-update on file changes
vela extract <path> --update            # incremental update
vela serve vela-out/graph.json          # start MCP server
vela hook install                       # install git hooks
```

---

## 10. Success Metrics

| Phase | Success Criteria |
|-------|-----------------|
| Phase 0 | `vela extract .` on Vela repo produces valid `graph.json` with nodes and edges |
| Phase 1 | Extract Go + TypeScript + Python + docs from a 5-repo microservice demo. LLM providers switchable via config. |
| Phase 2 | `GRAPH_REPORT.md` shows correct god nodes and community structure. HTML graph opens in browser. |
| Phase 3 | TUI shows real-time progress on a 50-file corpus without UX freezes. ETA within 20% accuracy. |
| Phase 4 | `--watch` detects file changes and updates graph within 5 seconds (code files, AST-only path). |

---

## 11. Non-Goals (Explicit)

- **Ancora integration** — deferred, not in scope for Phases 0-3
- **Video/audio transcription** — deferred (cgo complexity), Phase 4+
- **Managed cloud service** — Vela is a CLI tool, not SaaS
- **Replace Graphify** — Vela is inspired by Graphify but not a drop-in replacement

---

## 12. Risks

| Risk | Mitigation |
|------|-----------|
| Louvain vs Leiden quality gap | Using Python graspologic (Leiden) directly. No quality gap. |
| Local LLM extraction quality on 8B models | Few-shot prompts + GBNF JSON schema enforcement + tested on real corpora |
| go-tree-sitter bindings incomplete | Fallback: subprocess to Python tree-sitter for missing languages |
| Large graph performance (gonum) | Benchmark in Phase 0 on real corpus. gonum is compiled, expected fast. |
| Python subprocess reliability | Isolate to single `cluster.go` module. Wrap with clear error handling. |
