# Ancora and Vela Contract

## Purpose

Define a single boundary between Ancora and Vela so memory, retrieval, and integration do not overlap.

## Ownership

### Ancora owns

- Canonical durable memory storage
- Observations, decisions, bug fixes, sessions, and prompts
- Memory write semantics
- Memory read APIs

### Vela owns

- Repo, workspace, and contract graph retrieval
- Query orchestration and ranking
- Structural search such as graph traversal and path lookup
- Query-time fusion of code context and memory context

## Hard Rules

1. Ancora is the source of truth for durable memory.
2. Vela must not create a second canonical memory store.
3. Vela may index or transform Ancora memory for retrieval, but only as a cache or view.
4. If memory is written, the canonical write path is Ancora.
5. Skills, plugins, and MCP tools must not hide a duplicate write path.

## Integration Boundary

### What Vela reads from Ancora

- Observations
- Decisions
- Bug fixes
- Session summaries
- References from memory to repos, files, symbols, and concepts

### What Vela returns

- Ranked retrieval results
- Graph paths and structural context
- Fused answers combining code graph and memory graph evidence

## User-Facing Tool Boundary

- Ancora tools: save and retrieve memory
- Vela tools: retrieve and explain code or graph context

The user should not need to understand the internal boundary, but the implementation must keep it strict.

## Failure Mode Without Ancora

If Ancora is not installed or unavailable:

- Vela still works for repo, workspace, and contract retrieval
- Vela returns code and graph results only
- Vela disables memory-backed retrieval features
- Vela does not create fallback durable memory storage silently

## Decision

Use Ancora as the canonical memory backend.

Use Vela as the retrieval layer over code graphs and Ancora-backed memory.
