# When Does the LLM Model Run?

## TL;DR

**Ollama server:** Runs in background ALL THE TIME (like a database)  
**llama3 model:** Loaded ONLY when you extract data  
**Reading graph:** NO MODEL NEEDED (just reads JSON files)

---

## Complete Flow

### 1. Ollama Server (Background Service)

**Status:** Running continuously in background (daemon)

**How it starts:**
```bash
# macOS
brew services start ollama

# Linux
systemctl start ollama

# Manual
ollama serve  # Runs in background
```

**What it does:**
- Listens on `http://localhost:11434`
- Waits for API requests
- Does NOT load models yet
- Uses ~100MB RAM (idle)

**Like a database server:**
- PostgreSQL runs all time → Ollama runs all time
- PostgreSQL loads data when queried → Ollama loads model when requested

---

### 2. Model Loading (On-Demand)

**When:** ONLY during `vela extract`  
**Duration:** Loaded per extraction session  
**Trigger:** First LLM API call

**Flow:**
```
$ vela extract /code
  ↓
  Ollama server receives request
  ↓
  Loads llama3 model into RAM (~4.7GB)
  ↓
  Processes extraction requests
  ↓
  Keeps model in RAM for ~5 minutes (cache)
  ↓
  Unloads if no more requests
```

**Memory usage during extraction:**
- Ollama idle: ~100MB
- Ollama + llama3 loaded: ~5GB
- After 5min idle: model unloaded, back to ~100MB

---

### 3. Extraction Process (Model Active)

**Command:** `vela extract /path/to/code`

**What happens:**

```
┌─────────────────────────────────────────────────┐
│ vela extract /code                              │
└─────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│ 1. Scan files (.go, .py, .ts, .md, etc.)       │
│    → Found 100 files                            │
└─────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│ 2. For each file:                               │
│    ┌────────────────────────────────────┐       │
│    │ a. Check cache (skip if unchanged) │       │
│    │ b. Read file content               │       │
│    │ c. Chunk if too large (8000 tokens)│       │
│    │ d. Send to Ollama API ────────┐    │       │
│    └────────────────────────────────│────┘       │
│                                     │            │
│                                     ▼            │
│                          ┌──────────────────┐    │
│                          │ Ollama Server    │    │
│                          │ (localhost:11434)│    │
│                          │                  │    │
│                          │ Loads llama3     │    │
│                          │ Runs inference   │    │
│                          │ Returns JSON     │    │
│                          └──────────────────┘    │
│                                     │            │
│    ┌────────────────────────────────┘            │
│    ▼                                             │
│    e. Parse JSON (nodes + edges)                │
│    f. Add to graph                              │
│    └─────────────────────────────────           │
└─────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│ 3. Build final graph                            │
│    → Merge all nodes/edges                      │
│    → Run clustering (optional)                  │
│    → Save to vela-out/graph.json                │
└─────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│ 4. Model unloads (after ~5min idle)             │
│    → Back to ~100MB RAM                         │
└─────────────────────────────────────────────────┘
```

**Model is used:** Line 2d (send to Ollama API)  
**Number of calls:** One per file chunk (100 files = ~100 API calls)  
**Duration:** ~2-4 seconds per file  
**Total time:** 100 files × 3 sec = ~5 minutes

---

### 4. Reading Graph (NO MODEL)

**Command:** `vela query`, `vela path`, `vela explain`

**What happens:**

```
┌─────────────────────────────────────────────────┐
│ vela query "authentication"                     │
└─────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│ 1. Read vela-out/graph.json                     │
│    → Pure file I/O, no LLM needed              │
└─────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│ 2. Search in-memory graph                       │
│    → String matching, no AI                    │
└─────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│ 3. Return results                               │
│    → Fast (milliseconds)                       │
└─────────────────────────────────────────────────┘
```

**Model is used:** NEVER  
**Speed:** Instant (just reads JSON)  
**RAM:** ~10MB (graph in memory)

---

## Code Trace

### Where model is called (internal/llm/local.go):

```go
func (p *LocalProvider) ExtractGraph(ctx context.Context, text string, schema string) (*types.ExtractionResult, error) {
    // Build prompt
    prompt := buildExtractionPrompt(text, schema)
    
    // Send to Ollama
    req := ollamaRequest{
        Model:  "llama3",              // ← Model name
        Prompt: prompt,                // ← Code/doc text
        Format: "json",                // ← Force JSON output
    }
    
    // HTTP POST to localhost:11434/api/generate
    resp := http.Post(ollama_url, req)
    
    // Parse JSON response
    return parseExtractionJSON(resp.Body)
}
```

**This is called from:** `internal/extract/doc.go`, `internal/extract/pdf.go`  
**Frequency:** Once per file chunk during `vela extract`

---

## Summary Table

| Command | Ollama Server | Model Loaded | LLM Used | Speed |
|---------|---------------|--------------|----------|-------|
| `ollama serve` | ✓ Running | ✗ Not loaded | ✗ Idle | N/A |
| `vela extract` | ✓ Running | ✓ Loaded | ✓ Active | 2-4s per file |
| `vela query` | ✓ Running | ✗ Not loaded | ✗ Not used | <10ms |
| `vela path` | ✓ Running | ✗ Not loaded | ✗ Not used | <10ms |
| `vela explain` | ✓ Running | ✗ Not loaded | ✗ Not used | <10ms |
| `vela serve` (MCP) | ✓ Running | ✗ Not loaded | ✗ Not used | <10ms |

---

## Memory Usage Over Time

```
RAM Usage (GB)
    │
 8  │                 ┌─────┐
    │                 │     │
 6  │                 │     │
    │                 │     │ ← Model loaded during extraction
 4  │                 │     │
    │                 │     │
 2  │                 │     │
    │ ────────────────┘     └──────────────────
 0  │─────────────────────────────────────────────→ Time
    │     ^           ^     ^         ^
    │     │           │     │         │
    │   Idle      Extract  Wait    Unload
    │              starts  5min    (idle)
```

**Breakdown:**
- **Idle:** Ollama server only (~100MB)
- **Extract:** Server + llama3 model (~5GB)
- **Wait:** Model stays loaded (5min timeout)
- **Unload:** Back to idle (~100MB)

---

## Common Questions

### Q: Is Ollama always running?
**A:** Yes, as a background service (like PostgreSQL or Docker). Uses ~100MB RAM when idle.

### Q: Is llama3 always in memory?
**A:** No. Loaded during extraction, unloaded after ~5min idle.

### Q: Can I stop Ollama when not extracting?
**A:** Yes:
```bash
brew services stop ollama  # macOS
systemctl stop ollama      # Linux
```

But then `vela extract` will fail. Recommended: leave it running (minimal RAM when idle).

### Q: Does querying the graph use the model?
**A:** No. Graph is pure JSON. Query is keyword search (no AI).

### Q: Does the MCP server use the model?
**A:** No. MCP serves the graph JSON over HTTP (no AI).

### Q: Can multiple extractions run in parallel?
**A:** Yes. Ollama handles concurrent requests, but shares one model instance.

### Q: What if I have 1000 files?
**A:** Model stays loaded entire time (~1000 API calls, ~50min total). RAM usage constant (~5GB).

---

## Performance Tips

### 1. Use Cache
Vela caches extraction results. Re-running on unchanged files is instant (skips LLM).

```bash
vela extract /code  # First run: 5min
# (edit one file)
vela extract /code  # Second run: 5sec (only re-extracts changed file)
```

### 2. Stop Ollama when done
If RAM is tight:
```bash
brew services stop ollama  # Frees ~5GB if model loaded
```

### 3. Use smaller model for large codebases
Edit `~/.vela/config.yaml`:
```yaml
llm:
  model: llama3.2  # 3B instead of 8B (~2GB vs ~5GB)
```

---

## Related Docs

- `docs/SYFRA_ECOSYSTEM_INTEGRATION.md` - Vela vs Ancora architecture
- `docs/DECISIONS.md` - Why llama3 is default
- `internal/llm/local.go` - Ollama integration code
