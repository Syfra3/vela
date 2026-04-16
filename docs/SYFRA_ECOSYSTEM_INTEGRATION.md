# Syfra Ecosystem Integration

## Overview

Vela and Ancora are complementary tools in the Syfra ecosystem with **separate but compatible** LLM/embedding pipelines.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Syfra Ecosystem                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐           ┌──────────────┐              │
│  │    VELA      │           │   ANCORA     │              │
│  │              │           │              │              │
│  │  Knowledge   │  Graph    │   Memory &   │              │
│  │  Extraction  │  ────────▶│   Search     │              │
│  │              │   JSON    │              │              │
│  └──────────────┘           └──────────────┘              │
│         │                          │                       │
│         │                          │                       │
│    ┌────▼────┐              ┌─────▼──────┐               │
│    │ Ollama  │              │ llama.cpp  │               │
│    │ Models  │              │ embeddings │               │
│    └─────────┘              └────────────┘               │
│         │                          │                       │
│    LLM for                   Embeddings for              │
│    Extraction                Semantic Search             │
│    (4-8B params)             (nomic-embed-text)          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Vela: Knowledge Extraction

**Purpose:** Extract structured knowledge graphs from code/docs

**LLM Stack:**
- Runtime: Ollama (http://localhost:11434)
- Models: Llama 3, Mistral, Qwen, etc.
- Task: Graph extraction (NER + Relation Extraction)
- Output: JSON graph (nodes + edges)

**Recommended Models:**
1. **llama3** (8B) - Default, balanced
2. **llama3.2** (3B) - Fast, lightweight
3. **mistral** (7B) - High quality
4. **qwen2.5** (7B) - Large context window

**Why these models?**
- Structured output (JSON) capability
- Good reasoning for entity/relation extraction
- Local execution (privacy-first)
- 4-8GB RAM requirement

## Ancora: Memory & Semantic Search

**Purpose:** Store observations with semantic search

**Embedding Stack:**
- Runtime: llama.cpp (CLI tool)
- Model: nomic-embed-text-v1.5 (GGUF)
- Dimensions: 768
- Task: Text → vector embeddings
- Storage: SQLite with vector extension

**Why nomic-embed-text?**
- Apache 2.0 license (free for commercial use)
- 768-dim balanced quality/speed
- Quantized (Q4_K_M) - only 270MB
- Optimized for semantic similarity
- Pure Go + subprocess (no CGO)

## Compatibility Matrix

| Concern | Vela | Ancora | Compatible? |
|---------|------|--------|-------------|
| **LLM Runtime** | Ollama | llama.cpp | ✗ (different tools) |
| **Model Type** | Text generation (4-8B) | Embedding (137M) | ✗ (different purposes) |
| **Model Arch** | Llama/Mistral/Qwen | Nomic-embed | ✗ (different architectures) |
| **Embedding Dims** | 4096 (internal) | 768 | ✗ (different dimensions) |
| **Output Format** | JSON graph | float32[768] | ✗ (different outputs) |
| **Data Exchange** | Graph JSON | Graph JSON | ✓ (COMPATIBLE) |

## Where Compatibility Matters

### ✓ Data Level (Compatible)

Vela outputs graph JSON:
```json
{
  "nodes": [
    {"id": "user_service", "label": "UserService", "type": "class"}
  ],
  "edges": [
    {"source": "api", "target": "user_service", "relation": "calls"}
  ]
}
```

Ancora can index ANY graph JSON for semantic search. The embedding model is Ancora's internal concern.

### ✗ Model Level (Incompatible - by design)

**Vela models (Ollama):**
- Purpose: Generate structured text (graph extraction)
- Interface: HTTP API (POST /api/generate)
- Model: Full LLM (billions of parameters)

**Ancora models (llama.cpp):**
- Purpose: Convert text → vectors (semantic search)
- Interface: CLI subprocess (llama-embedding)
- Model: Embedding-only (millions of parameters)

These are **intentionally different** - they solve different problems.

## Integration Workflow

### 1. Vela Extracts Graph
```bash
vela extract /path/to/code
# Uses Ollama (llama3) to extract entities and relationships
# Outputs: vela-out/graph.json
```

### 2. Ancora Indexes Graph
```bash
ancora import vela-out/graph.json
# Uses nomic-embed-text to create semantic embeddings
# Stores: SQLite with FTS5 + vector search
```

### 3. Search Across Graph
```bash
ancora search "authentication flow"
# Uses nomic-embed-text embeddings for semantic similarity
# Returns: relevant nodes/edges from Vela's extracted graph
```

## Multi-Model Selection Complexity

### Simple Approach (Current)
- Default: `llama3` (hardcoded in wizard)
- Reason: Works well, trusted, 8B sweet spot
- Setup: One-click pull during wizard

### Complex Approach (Your Question)

**Complexity Analysis:**

1. **Model Curation** (Medium complexity)
   - Maintain list of tested models ✓ (done in `models.go`)
   - Compatibility flags (Syfra-verified) ✓
   - Performance metadata (RAM, speed) ✓

2. **Dynamic Model List** (Low complexity)
   - Call `ollama list` to get installed models ✓ (already implemented)
   - Merge with curated recommendations ✓
   - Filter by system RAM ~

3. **User Selection UI** (Low complexity)
   - TUI list with cursor navigation ✓ (already have pattern)
   - Show: name, size, RAM requirement, description ✓
   - Highlight Syfra-verified models ✓

4. **Ancora Embedding Compatibility** (No action needed)
   - Ancora uses separate embedding model (nomic-embed-text)
   - Vela model choice does NOT affect Ancora embeddings
   - Compatibility is at data level (graph JSON), not model level

**Total Complexity: LOW to MEDIUM**

The hard part is already done:
- Model metadata structure ✓ (`RecommendedModel`)
- Curated list with Syfra verification ✓
- Performance characteristics ✓
- Ollama integration ✓

What's left:
- Add TUI model picker to wizard
- Filter by available RAM (optional)
- Allow user to type custom model name

## Recommendation

**For MVP:**
- Keep default `llama3` - works well, trusted
- Add "Advanced: Choose different model" option
- Show curated list of 5-6 Syfra-verified models
- Allow skip/custom entry for power users

**For v2:**
- Auto-detect system RAM
- Filter recommendations by available memory
- Show performance estimates (tokens/sec)
- Allow A/B testing between models

## Key Insight

The "embedding compatibility" concern is a **non-issue** because:

1. **Vela** uses Ollama models for **extraction** (text → graph JSON)
2. **Ancora** uses nomic-embed-text for **search** (text → vector embeddings)
3. They communicate via **graph JSON** (universal format)
4. Model architectures don't need to match - data format does

**Analogy:**
- Vela is a translator (code → graph)
- Ancora is a librarian (graph → searchable index)
- Translator's language ≠ librarian's indexing system
- They just need to agree on the book format (JSON)

## References

- Vela LLM: `internal/llm/local.go`
- Ollama setup: `internal/setup/ollama.go`
- Model metadata: `internal/setup/models.go`
- Ancora embeddings: `~/Documents/personal/ancora/internal/embed/embed.go`
- Nomic model: https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF
