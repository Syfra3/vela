# Architecture Decision Records (ADR)

## ADR-001: Standardize on llama3 for LLM Extraction

**Date:** 2024-04-12  
**Status:** Accepted  
**Context:** Multi-model selection framework vs single default model

### Decision

Vela will standardize on **llama3 (8B)** as the default and recommended model for knowledge graph extraction.

### Rationale

**Why llama3:**
1. **Balanced Performance** - Sweet spot between speed (25 tok/s) and quality
2. **Reasonable Requirements** - 8GB RAM (most development machines)
3. **Syfra-Verified** - Tested in production with Syfra ecosystem
4. **Good Structured Output** - Reliable JSON generation for graph extraction
5. **Sufficient Context** - 8192 token window handles most code files
6. **Community Support** - Well-documented, active development

**Why NOT multi-model selection (for now):**
1. **Complexity vs Value** - Model picker adds UI complexity without clear user benefit
2. **Decision Fatigue** - Most users don't know which model to choose
3. **Support Burden** - Multiple models = multiple failure modes
4. **YAGNI Principle** - Can add later if users request it
5. **llama3 Works** - No complaints, good results, proven track record

### Alternatives Considered

**Option A: Multi-model picker** (rejected)
- Pros: User choice, flexibility
- Cons: UI complexity, decision fatigue, support burden
- Complexity: LOW (framework already exists in `models.go`)
- Verdict: Deferred until user demand is clear

**Option B: Auto-detect best model** (rejected)  
- Pros: Automatic optimization
- Cons: Opaque behavior, hard to debug, premature optimization
- Complexity: MEDIUM (need RAM detection, model benchmarking)
- Verdict: Not worth complexity for MVP

**Option C: llama3 default with advanced override** (future consideration)
- Pros: Simple default, expert escape hatch
- Cons: Still need validation/testing for non-standard models
- Complexity: LOW (just add config option)
- Verdict: Consider for v2 if requested

### Implementation

**Setup Wizard:**
- Hardcoded `selectedModel = "llama3"`
- No model selection UI
- Direct pull: `ollama pull llama3`

**Config File:**
- Default: `model: llama3`
- Users can manually edit `~/.vela/config.yaml` if needed (undocumented feature)

**Preserved for Future:**
- Model metadata framework (`internal/setup/models.go`)
- Curated model list (Syfra-verified flags)
- Performance characteristics (RAM, speed, context)
- If demand emerges, framework is ready

### Consequences

**Positive:**
- Simple, predictable user experience
- Lower support burden (one model to debug)
- Faster setup (no decision step)
- Clear documentation (just use llama3)

**Negative:**
- Power users can't easily switch models in UI
- Resource-constrained users might want llama3.2 (3B)
- Code-heavy users might want codellama

**Mitigation:**
- Document manual config override in `~/.vela/config.yaml`
- Monitor user requests for model flexibility
- Model selection framework preserved in codebase (easy to enable later)

### Related Decisions

- **ADR-002**: Vela/Ancora integration architecture (data-level compatibility, model-level independence)
- See: `docs/SYFRA_ECOSYSTEM_INTEGRATION.md`

---

## ADR-002: Separate LLM Pipelines for Vela and Ancora

**Date:** 2024-04-12  
**Status:** Accepted  
**Context:** Integration strategy between Vela (extraction) and Ancora (search)

### Decision

Vela and Ancora maintain **separate, independent LLM/embedding pipelines** that integrate at the **data level** (graph JSON), not the model level.

### Rationale

**Different Problems, Different Tools:**

| Concern | Vela | Ancora |
|---------|------|--------|
| **Purpose** | Graph extraction (reasoning) | Semantic search (similarity) |
| **Model Type** | Text generation (4-8B params) | Embedding (137M params) |
| **Runtime** | Ollama (HTTP API) | llama.cpp (CLI subprocess) |
| **Model** | llama3, mistral, qwen | nomic-embed-text |
| **Output** | Graph JSON (structured) | float32[768] vectors |
| **Context** | Full context (reasoning) | Query similarity (matching) |

**Why NOT unified model:**
1. **Optimization Goals Differ** - Generation ≠ Embedding
2. **Scale Mismatch** - 8B params vs 137M params
3. **Performance Trade-offs** - Speed vs quality targets differ
4. **Licensing** - Different model licenses/providers
5. **Independence** - Vela works without Ancora, and vice versa

### Integration Point: Graph JSON

**Vela Output:**
```json
{
  "nodes": [{"id": "user_service", "label": "UserService", "type": "class"}],
  "edges": [{"source": "api", "target": "user_service", "relation": "calls"}]
}
```

**Ancora Input:** Any JSON graph (model-agnostic)

**Compatibility:** Data format, not model architecture

### Consequences

**Positive:**
- Clear separation of concerns
- Independent optimization
- Each tool uses best model for its purpose
- No cross-contamination of requirements

**Negative:**
- Two separate setup processes (Ollama + llama.cpp)
- Users need to understand difference
- Disk space: ~5GB (Ollama models) + ~270MB (embedding model)

**Mitigation:**
- Document integration in `SYFRA_ECOSYSTEM_INTEGRATION.md`
- Setup wizards handle dependencies separately
- Clear messaging: "Vela extracts, Ancora searches"

### Related Decisions

- **ADR-001**: llama3 standardization
- See: `docs/SYFRA_ECOSYSTEM_INTEGRATION.md`

---

## Future ADRs

- ADR-003: Neo4j integration strategy (optional graph database)
- ADR-004: Remote LLM provider priority (Anthropic vs OpenAI)
- ADR-005: Code language detection approach
- ADR-006: Incremental extraction strategy (watch mode)
