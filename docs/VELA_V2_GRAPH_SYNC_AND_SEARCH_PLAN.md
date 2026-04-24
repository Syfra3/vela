# Vela v2 Graph Sync and Search Plan

Related docs:

- `docs/VELA_V2_GRAPH_SYNC_AND_SEARCH_SPEC.md`
- `docs/VELA_V2_GRAPH_SYNC_AND_SEARCH_TASKS.md`

## Goal

Bring the strongest ideas from `graphify` into Vela v2 while keeping the implementation focused on the local CLI workflow.

This plan is intentionally scoped to:

- CLI commands
- local graph freshness
- local retrieval/search quality
- agent instructions that improve CLI-driven usage

This plan explicitly does **not** include MCP or HTTP server work.

## What Graphify Gets Right

`graphify` solves two separate problems:

1. It keeps the graph updated as the codebase changes.
2. It changes the agent's starting point from raw file search to graph-first navigation.

The important lesson is not "replace grep with magic." The real move is:

- keep the graph fresh enough to trust
- make agents consult graph structure before raw text search

## Relevant Graphify Patterns

### 1. Runtime sync, not manual discipline

Graph freshness comes from runtime mechanisms:

- watch mode for code changes
- incremental update flow
- content-addressed cache
- git hooks after commit and checkout

### 2. File-level incremental detection

Graphify tracks changed, unchanged, and deleted files rather than only checking one graph file timestamp.

That is the correct model for Vela too. A single graph-level modtime check is too coarse.

### 3. Graph-first agent behavior

Graphify does not literally remove grep. It changes search order:

- read `GRAPH_REPORT.md` first
- use graph query/path/explain tools for structural questions
- use raw grep only when exact text lookup is needed

That is the behavior Vela should copy.

## Current Vela v2 Reality

### Strong foundations already present

- `internal/pipeline/build.go` already owns the build pipeline and graph persistence.
- `internal/app/build.go` is the correct application boundary for rebuild orchestration.
- `internal/query/query.go` already supports graph-truth queries.
- `docs/VELA_ARCHITECTURE.md` already defines the intended routing and retrieval direction.
- `internal/report/report.go` already generates `GRAPH_REPORT.md`.

### Current gaps

- graph freshness is still coarse and mostly graph-file/modtime based
- live watch behavior is not implemented in the active root path
- current search is only lightweight lexical ranking over graph nodes
- Vela does not yet strongly steer CLI agents into graph-first navigation

## Scope Decision

This implementation should focus on CLI-first behavior.

In scope:

- `vela build`
- `vela update`
- `vela watch`
- local graph/report outputs in `.vela/`
- retrieval/search used by CLI-facing agent workflows
- repo-local instructions/hooks that push agents toward graph-first search

Out of scope:

- MCP tools
- HTTP server endpoints
- remote sync or hosted graph infrastructure

## Target Outcome

After this work, the intended workflow should be:

1. Developer runs `vela build` once.
2. Vela writes `.vela/graph.json` and `.vela/GRAPH_REPORT.md`.
3. Developer can run `vela watch` during active work.
4. Incremental updates keep the graph current without full rebuilds whenever possible.
5. CLI-oriented coding agents are instructed to read the graph report first.
6. Raw grep remains available, but becomes a fallback for exact-text lookup rather than the primary navigation tool.

## Implementation Plan

## Phase 1: Incremental Graph Freshness

### Objective

Replace coarse graph reuse logic with file-level change tracking.

### Work

1. Add a manifest file under `.vela/`.
2. Track, per file:
   - relative path
   - content hash
   - extraction fingerprint/version
   - last processed time
3. Extend the build pipeline to classify files as:
   - new
   - changed
   - unchanged
   - deleted
4. Rebuild only affected code-derived graph portions when possible.
5. Prune graph artifacts for deleted files.

### Primary seams

- `internal/pipeline/build.go`
- `internal/detect/detect.go`
- `internal/detect/walker.go`
- `internal/export/json.go`

### Acceptance criteria

- unchanged repos do not trigger unnecessary rebuilds
- changed files are detected at file granularity
- deleted files are removed from persisted graph outputs
- rebuild decisions no longer depend only on `graph.json` modtime

## Phase 2: Watch Mode

### Objective

Add a local runtime that keeps `.vela/graph.json` fresh while the developer edits code.

### Work

1. Implement `internal/watch` in the active root codepath.
2. Reuse the same ignore semantics as `detect/walker.go`.
3. Debounce file events to avoid rebuild storms.
4. Route watch-triggered rebuilds through `internal/app/build.go`.
5. Add CLI support for `vela watch`.

### Primary seams

- `internal/watch`
- `internal/app/build.go`
- `cmd/vela/main.go`
- `cmd/vela/cutover.go`

### Acceptance criteria

- code edits trigger incremental rebuild attempts
- ignored files do not trigger rebuilds
- watch mode reuses pipeline logic instead of duplicating it
- graph/report outputs stay consistent with normal build output

## Phase 3: Git Hook Sync

### Objective

Keep the graph aligned with commit and branch transitions even when watch mode is not running.

### Work

1. Add a CLI command for hook installation.
2. Install hooks for:
   - `post-commit`
   - `post-checkout`
3. Make hooks call the standard CLI update/build path.
4. Fail visibly when the rebuild fails.

### Primary seams

- `cmd/vela/main.go`
- a new hook installer package under `internal/`

### Acceptance criteria

- branch switches refresh stale graph outputs
- commits refresh graph outputs automatically
- hooks do not bypass normal build rules

## Phase 4: CLI Retrieval Layer

### Objective

Make Vela search good enough that agents stop leaning on grep for every non-trivial question.

### Work

1. Create `internal/retrieval` as a separate concern from `internal/query`.
2. Start with lexical retrieval over symbols, files, and chunks.
3. Expand lexical matches through graph neighbors and layer-aware joins.
4. Rank results using graph provenance and evidence strength.
5. Return focused result sets suitable for CLI output and agent consumption.

### Why this separation matters

`internal/query` should remain responsible for graph-truth reasoning such as:

- path lookup
- neighbors
- explain
- impact

`internal/retrieval` should own candidate generation and ranking.

Mixing both into one package is bad architecture.

### Primary seams

- `internal/retrieval`
- `internal/app/query.go`
- `internal/query/query.go`

### Acceptance criteria

- search quality is materially better than exact/fuzzy node label matching
- results include provenance
- structural questions can be answered from graph-first retrieval before raw file search

## Phase 5: CLI Agent Search Behavior

### Objective

Change agent behavior so graph-first navigation becomes the default in CLI-driven workflows.

### Work

1. Add repo-local instructions for supported CLI agents.
2. Teach agents to:
   - read `.vela/GRAPH_REPORT.md` first for architecture questions
   - prefer Vela CLI graph commands for structural questions
   - use grep only for exact string lookup
3. Add platform-specific hooks or rule files where available.
4. Keep the fallback simple: if no graph exists, agents can search raw files.

### Behavioral contract

Use graph-first for:

- architecture questions
- dependency flow
- ownership and subsystem mapping
- impact exploration
- "what connects X to Y"

Use grep-first for:

- exact string lookup
- log message search
- env var names
- literal snippets

### Acceptance criteria

- agents no longer start with blind grep for architectural questions
- the graph report becomes the default high-level map
- CLI commands become the preferred deep navigation surface

## Recommended Order

1. Phase 1: incremental manifest and file-level freshness
2. Phase 2: watch mode
3. Phase 3: git hooks
4. Phase 4: retrieval layer
5. Phase 5: CLI agent behavior changes

## Why This Order

- If the graph is stale, graph-first search is untrustworthy.
- If retrieval remains weak, telling agents to "use the graph" is mostly theater.
- Freshness must come first, then retrieval quality, then agent behavior.

## File-Level Starting Points

- `internal/pipeline/build.go` — replace coarse cache/freshness checks with file-level incremental logic
- `internal/detect/walker.go` — reuse ignore rules for both detect and watch mode
- `internal/app/build.go` — application entrypoint for normal and watch-triggered rebuilds
- `cmd/vela/main.go` — add CLI commands for update/watch/hooks
- `internal/retrieval` — new package for candidate generation and ranking
- `internal/query/query.go` — keep focused on graph-truth operations, not generic search overload
- `internal/report/report.go` — preserve report generation as the human-readable entrypoint for agents

## Non-Goals

- do not overload `internal/query/query.go` with retrieval, indexing, and watch logic
- do not introduce server or MCP work in this implementation slice
- do not promise semantic document ingestion unless explicitly scoped later

## Final Principle

The Vela v2 upgrade should not be framed as "remove grep."

It should be framed correctly:

- keep the graph fresh
- improve local retrieval
- change agent behavior so raw grep becomes a fallback instead of the default navigation model
