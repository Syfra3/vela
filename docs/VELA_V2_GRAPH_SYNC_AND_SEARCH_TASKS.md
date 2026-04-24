# Vela v2 Graph Sync and Search Tasks

Related docs:

- `docs/VELA_V2_GRAPH_SYNC_AND_SEARCH_PLAN.md`
- `docs/VELA_V2_GRAPH_SYNC_AND_SEARCH_SPEC.md`

This task list implements the plan in `docs/VELA_V2_GRAPH_SYNC_AND_SEARCH_PLAN.md`.

Scope remains CLI-first only.

## Phase 1: Incremental Graph Freshness

### Task 1.1: Define `.vela` manifest contract

- Create a manifest structure for file-level freshness tracking.
- Include at minimum:
  - relative path
  - content hash
  - extractor/build version fingerprint
  - last processed timestamp
  - deleted marker or deletion handling strategy

Files:

- `internal/pipeline/build.go`
- `internal/export/json.go`
- `pkg/types`

Done when:

- Vela can persist and reload manifest metadata from `.vela/`
- manifest format is stable enough to support future watch/update logic

### Task 1.2: Replace coarse cache reuse with file-level diffing

- Stop relying only on `graph.json` modtime for rebuild decisions.
- Detect:
  - new files
  - changed files
  - unchanged files
  - deleted files
- Keep full rebuild as fallback when manifest is missing or incompatible.

Files:

- `internal/pipeline/build.go`
- `internal/detect/detect.go`
- `internal/detect/walker.go`

Done when:

- unchanged repos reuse prior outputs safely
- a single changed file does not invalidate the entire graph by default
- deleted files are explicitly detected

### Task 1.3: Prune deleted-file graph artifacts

- Remove nodes and edges that belong to deleted files.
- Ensure persisted graph outputs reflect current repo state.

Files:

- `internal/pipeline/build.go`
- `internal/graph`
- `internal/export/json.go`

Done when:

- deleting a source file removes its graph presence after update/build

### Task 1.4: Verify incremental freshness behavior

- Add tests for:
  - no-op rebuild
  - single file change
  - file deletion
  - manifest missing/corrupt fallback

Files:

- `internal/pipeline/*_test.go`
- `internal/detect/*_test.go`

Done when:

- pipeline behavior is covered by repeatable tests

## Phase 2: Watch Mode

### Task 2.1: Implement watcher runtime

- Create the active root implementation under `internal/watch`.
- Watch code files only for the first iteration.
- Debounce change bursts before rebuild.

Files:

- `internal/watch`

Done when:

- watcher can observe repo changes and emit rebuild triggers

### Task 2.2: Reuse detect ignore semantics

- Ensure watch filtering matches detect filtering.
- Avoid drift between what Vela scans and what Vela watches.

Files:

- `internal/watch`
- `internal/detect/walker.go`

Done when:

- ignored paths do not trigger watch rebuilds
- watched paths match build-relevant paths

### Task 2.3: Route watch rebuilds through app service

- Watch mode must call the normal build/update orchestration path.
- Do not duplicate pipeline logic inside watcher code.

Files:

- `internal/app/build.go`
- `internal/watch`

Done when:

- manual build and watch-triggered build share the same execution path

### Task 2.4: Add `vela watch` CLI command

- Expose watch mode through the CLI.
- Print concise rebuild status and errors.

Files:

- `cmd/vela/main.go`
- `cmd/vela/cutover.go`

Done when:

- `vela watch` runs locally and refreshes outputs as files change

### Task 2.5: Verify watch mode

- Add tests where feasible.
- Validate debounce and ignore behavior.

Files:

- `internal/watch/*_test.go`
- CLI tests if present in current command setup

Done when:

- watch mode does not rebuild on every noisy filesystem event

## Phase 3: Git Hook Sync

### Task 3.1: Create hook installer package

- Add a small internal package that installs and removes hooks.
- Keep shell generation minimal and deterministic.

Files:

- `internal/hooks` or similar new package

Done when:

- Vela can manage its own hook files safely

### Task 3.2: Implement post-commit and post-checkout hooks

- Hooks should call the normal Vela update/build path.
- Hook failures should be visible.

Files:

- `internal/hooks`

Done when:

- graph outputs refresh after commit and branch switch

### Task 3.3: Add CLI command for hook management

- Add install/uninstall/status commands.

Files:

- `cmd/vela/main.go`

Done when:

- developer can manage hooks without editing `.git/hooks` manually

### Task 3.4: Verify hook behavior

- Test generated scripts.
- Validate failure handling and idempotent install behavior.

Files:

- `internal/hooks/*_test.go`

Done when:

- repeated install does not corrupt hooks

## Phase 4: CLI Retrieval Layer

### Task 4.1: Create retrieval package boundary

- Introduce `internal/retrieval`.
- Keep it separate from `internal/query`.

Files:

- `internal/retrieval`
- `internal/app/query.go`

Done when:

- retrieval responsibilities are clearly separated from graph reasoning

### Task 4.2: Implement lexical candidate generation

- Index and search across at least:
  - symbols
  - files
  - descriptions
  - chunks if chunking already exists

Files:

- `internal/retrieval`
- possibly `pkg/types`

Done when:

- search quality is materially better than the current node label scorer

### Task 4.3: Add graph expansion and evidence-aware ranking

- Expand lexical hits through graph neighbors.
- Rank using evidence strength and provenance.
- Favor declared/extracted evidence over inferred where applicable.

Files:

- `internal/retrieval`
- `pkg/types/architecture.go`

Done when:

- retrieval returns structurally useful results, not just fuzzy text matches

### Task 4.4: Add CLI search entrypoints

- Expose retrieval in CLI commands.
- Keep output compact and agent-consumable.

Files:

- `cmd/vela/main.go`
- `internal/app/query.go`

Done when:

- developer can use Vela CLI instead of raw grep for graph-structural questions

### Task 4.5: Verify retrieval quality

- Add tests for ranking and result selection.
- Compare against current `internal/query/query.go` search behavior.

Files:

- `internal/retrieval/*_test.go`
- `internal/query/*_test.go`

Done when:

- retrieval quality regressions are detectable

## Phase 5: CLI Agent Graph-First Behavior

### Task 5.1: Define graph-first search rules

- Document when agents should prefer:
  - `.vela/GRAPH_REPORT.md`
  - Vela CLI graph commands
  - raw grep

Files:

- `AGENTS.md`
- relevant platform instruction files if already present

Done when:

- graph-first behavior is explicitly documented

### Task 5.2: Add CLI-oriented agent instructions

- Teach supported coding agents to:
  - read `GRAPH_REPORT.md` before architecture search
  - use Vela CLI commands before blind grep
  - fall back to grep for exact text lookup

Files:

- `AGENTS.md`
- platform-specific rule files if Vela already owns them

Done when:

- the repo itself nudges agents into the intended search order

### Task 5.3: Add optional hook/plugin integrations where cheap

- Only add platform-specific hook behavior if it stays lightweight.
- Do not let platform glue dominate the implementation.

Files:

- platform-specific config files as needed

Done when:

- supported agents get a reminder before raw search tools when graph outputs exist

## Cross-Cutting Constraints

- Do not merge retrieval, watch logic, and graph reasoning into one package.
- Do not introduce MCP or HTTP server work in this slice.
- Keep `.vela/GRAPH_REPORT.md` as the human-readable high-level map.
- Keep raw grep available for exact-text search.

## Suggested Execution Order

1. Task 1.1 through 1.4
2. Task 2.1 through 2.5
3. Task 3.1 through 3.4
4. Task 4.1 through 4.5
5. Task 5.1 through 5.3

## Definition Of Success

Vela v2 succeeds at this initiative when all of the following are true:

- local graph outputs stay fresh with minimal manual effort
- CLI update/watch workflows are trustworthy
- search quality is good enough for graph-structural questions
- agents default to graph-first navigation instead of blind grep
- grep remains a fallback, not the primary navigation model
