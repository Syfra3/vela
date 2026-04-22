# Ralph Workflow

This repository carries a repo-local Ralph workflow for the layered retrieval
feature on branch `feature/ralph-full-architecture-workflow`.

The workflow assets live under `workflows/ralph/`:

- `prd.json` is the ordered story contract.
- `progress.txt` is the append-only execution log.
- `ralph.sh` is the runner for status, single-story execution, resume, and full
  story loops.

## Setup Expectations

Before running the workflow, verify these basics:

- You are in the `vela` repository root.
- Your current branch is `feature/ralph-full-architecture-workflow`.
- `jq` is installed because `ralph.sh` reads and updates `prd.json` with it.
- At least one supported agent CLI is installed: `opencode`, `claude`, or
  `amp`.
- The repo already contains the architecture implementation for stories
  `US-001` through `US-011`; this workflow is for executing and resuming that
  package, not inventing a second implementation path.

## Runner Usage

Run from the repo root:

```bash
./workflows/ralph/ralph.sh --status
./workflows/ralph/ralph.sh
./workflows/ralph/ralph.sh --story 12
./workflows/ralph/ralph.sh --all
```

Behavior:

- `--status` shows the current story matrix from `prd.json`.
- No argument resumes from the first story where `passes=false`.
- `--story <priority>` targets one ordered story directly.
- `--all` keeps advancing until no pending stories remain.
- The runner enforces the branch recorded in `prd.json`, builds the prompt from
  the selected story, appends the full session output to `progress.txt`, and
  marks the story as passed only when the agent prints the required
  `STORY_COMPLETE: US-xxx` marker.

## Validation Contract

The workflow is only credible if maintainers can verify the architecture from
multiple product surfaces instead of trusting one internal package. Use this as
the acceptance checklist.

### 1. CLI behavior

Purpose: prove extraction, routing, explainability, and benchmark tooling are
usable from the terminal.

Primary automated coverage:

- `go test ./cmd/vela ./internal/query`
- `cmd/vela/search_test.go`
- `cmd/vela/retrieval_bench_test.go`
- `internal/query/query_test.go`
- `internal/query/search_test.go`

Acceptance scenarios:

- `vela extract` reports a layer summary containing repo, contract, workspace,
  and memory when present.
- `vela search billing` shows workspace routing reasons before repo-deep hits.
- `vela explain <node>` exposes layer and evidence metadata instead of a flat
  unlabeled edge dump.
- `vela bench retrieval --suite bench/retrieval/vela-curated.json --json`
  evaluates the curated architecture scenarios and persists a benchmark
  snapshot.

### 2. TUI behavior

Purpose: prove maintainers can inspect setup and validation paths interactively,
not only through raw commands.

Primary automated coverage:

- `go test ./internal/tui`
- `internal/tui/doctor_test.go`
- `internal/tui/query_test.go`
- `internal/tui/projects_test.go`

Acceptance scenarios:

- Opening the doctor screen starts checks immediately.
- The doctor view reports the configured provider and integration sections.
- Query and project screens continue to operate against the layered graph data.

### 3. Doctor and install flow

Purpose: prevent fake confidence from config-only checks.

Primary automated coverage:

- `go test ./internal/tui ./cmd/vela`
- `internal/tui/query_test.go`
- `cmd/vela/main_test.go`

Acceptance scenarios:

- Classic TUI exposes extraction, graph status, and query over the new backend.
- `vela serve` exposes query-only MCP/HTTP transports.
- CLI help omits removed watch/search/provider-era flags.

### 4. Indexing and extraction

Purpose: prove graph materialization stays grounded in repo-local correctness.

Primary automated coverage:

- `go test ./internal/extract`
- `internal/extract/extract_test.go`
- `internal/extract/contract_test.go`

Acceptance scenarios:

- Extraction stamps layer and evidence metadata consistently across code,
  contract, and project inputs.
- Unsupported doc/LLM paths stay out of the active build surface.

### 5. Retrieval and graph-layer behavior

Purpose: prove the architecture works as a layered system instead of a pile of
disconnected indexes.

Primary automated coverage:

- `go test ./pkg/types ./internal/graph ./internal/query`
- `pkg/types/architecture_test.go`
- `internal/graph/contract_test.go`
- `internal/graph/workspace_test.go`

Benchmark scenarios:

- Workspace routing beats undifferentiated repo search for service-scoped
  queries such as `billing`.
- Contract truth beats weaker derived edges when the same relation appears in
  multiple layers.
- Query output stays grounded in workspace, contract, and repo evidence.

## Architecture-Shaped Benchmark Examples

Use these examples when validating the feature manually or when interpreting the
benchmark suite in `bench/retrieval/vela-curated.json`:

- Query: `billing`
  Expectation: workspace routing selects `billing-api` before repo-deep hits and
  the result set includes workspace, contract, and repo evidence.
- Query: `federated retriever`
  Expectation: memory observations and repo graph hits fuse under one canonical
  identity with provenance from both sources.
- Query: a renamed or moved file referenced by memory
  Expectation: explain/bindings output shows `redirected` or `stale`, never a
  silent success.
- Query: a declared service or endpoint
  Expectation: contract evidence outranks weaker inferred relations on the same
  triple.

## Recommended Verification Commands

Use the focused suites first, then the repo-wide pass that is feasible for this
branch:

```bash
go test ./cmd/vela ./internal/doctor ./internal/setup ./internal/tui ./internal/extract ./internal/retrieval ./internal/graph ./internal/query ./internal/reconcile ./pkg/types
go test ./cmd/... ./internal/... ./pkg/...
go test ./...
```

Current branch note:

- `go test ./...` is expected to keep reporting the pre-existing empty fixture
  packages under `tests/fixtures/detect/**` (`expected 'package', found 'EOF'`).
  Treat that as a repo baseline issue unless the branch changes those fixtures.
