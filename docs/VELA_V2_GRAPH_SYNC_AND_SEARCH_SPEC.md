# Vela v2 Graph Sync and Search Spec

Related docs:

- `docs/VELA_V2_GRAPH_SYNC_AND_SEARCH_PLAN.md`
- `docs/VELA_V2_GRAPH_SYNC_AND_SEARCH_TASKS.md`

## Status

Draft implementation spec for the CLI-first graph freshness and graph-first search initiative.

## Scope

This spec covers:

- local graph freshness under `.vela/`
- incremental update behavior
- local watch mode
- git hook sync for CLI workflows
- CLI retrieval/search improvements
- CLI-oriented agent instructions that prefer graph-first navigation

This spec does not cover:

- MCP tools
- HTTP server endpoints
- remote services
- non-code semantic ingestion beyond future compatibility hooks

## Product Intent

Vela should stop behaving like a tool that only knows how to rebuild everything and then offer a weak label search over the result.

The target product behavior is:

1. Build a graph once.
2. Keep it fresh with low-friction local mechanisms.
3. Offer CLI retrieval that answers structural questions better than blind grep.
4. Push coding agents to use the graph as the first navigation surface.

## Existing Baseline

Current strong surfaces:

- `internal/pipeline/build.go` owns graph build and persistence
- `internal/app/build.go` is the right orchestration boundary
- `internal/query/query.go` already supports graph-truth queries
- `internal/report/report.go` already writes `GRAPH_REPORT.md`

Current gaps:

- freshness is coarse and graph-file/modtime driven
- watch mode is not active in the root implementation
- search quality is limited to lexical node scoring
- agent behavior is not yet strongly graph-first in CLI workflows

## Output Contract

Vela will standardize on these local outputs inside `.vela/`:

- `.vela/graph.json` — persisted graph truth
- `.vela/GRAPH_REPORT.md` — human-readable graph overview
- `.vela/graph.html` — optional visual export
- `.vela/manifest.json` — file-level freshness metadata

Future-compatible but out of scope for initial implementation:

- `.vela/cache/`
- `.vela/retrieval.db`

## Manifest Contract

### Purpose

The manifest exists to decide whether Vela can reuse prior graph work safely and to identify the minimum set of files affected by local changes.

### Required fields

Top-level shape:

```json
{
  "version": 1,
  "repo_root": "/abs/path/or-normalized-root",
  "generated_at": "2026-04-23T00:00:00Z",
  "extractor_fingerprint": "vela-build-v2",
  "files": [
    {
      "path": "internal/query/query.go",
      "sha256": "...",
      "size": 12345,
      "mod_time_utc": "2026-04-23T00:00:00Z",
      "language": "go",
      "status": "active"
    }
  ]
}
```

### Semantics

- `version` changes when the manifest schema changes.
- `repo_root` is informational and should not be the only identity key.
- `extractor_fingerprint` changes when extraction logic changes in a way that invalidates prior file-level reuse.
- `files[].path` must be repo-relative.
- `files[].sha256` is the primary change detector.
- `files[].status` starts with `active`; deletion may be represented by omission plus diffing rather than persisted tombstones.

### Compatibility rules

Fallback to full rebuild when:

- manifest is missing
- manifest is unreadable
- manifest version is unsupported
- extractor fingerprint changed
- graph output is missing or unreadable

## Incremental Update Rules

### File classification

Each build or update run must classify files into exactly one bucket:

- `new`
- `changed`
- `unchanged`
- `deleted`

### Default rules

- `new`: present in current detect output, absent from prior manifest
- `changed`: present in both, but hash changed
- `unchanged`: present in both, same hash
- `deleted`: absent from current detect output, present in prior manifest

### Rebuild policy

Initial implementation should support this behavior:

1. If everything is `unchanged`, reuse graph outputs.
2. If one or more files are `new` or `changed`, rerun extraction for affected code inputs.
3. If one or more files are `deleted`, remove their graph-owned nodes and edges before persist.
4. If incremental merge cannot be performed safely, fall back to full rebuild.

### Safety over cleverness

The first implementation should prefer a correct fallback full rebuild over a broken partial merge.

## Build Command Contract

### `vela build`

Purpose:

- produce `.vela/graph.json`
- produce `.vela/GRAPH_REPORT.md`
- update `.vela/manifest.json`

Behavior:

- if manifest and graph outputs are current, report cached or reused result
- otherwise run detect, extract, merge, cluster, persist, and report generation

Exit behavior:

- `0` on success
- non-zero on build failure

### `vela update`

Purpose:

- perform an incremental freshness pass against existing `.vela/` outputs

Behavior:

- load manifest and current graph outputs
- detect changed/deleted files
- apply incremental update when safe
- fall back to `vela build` semantics when incremental preconditions fail

Exit behavior:

- `0` on success
- non-zero on failure

### `vela watch`

Purpose:

- keep `.vela/` outputs current during active development

Behavior:

- subscribe to file changes under repo root
- reuse detect ignore semantics
- debounce bursts
- trigger `update` behavior on code changes
- print concise status

Non-goals for first iteration:

- background daemon management
- cross-terminal coordination
- doc/image semantic refresh

## Watch Mode Semantics

### Included paths

Watch mode must use the same effective filtering as the build detect phase for supported code inputs.

### Debounce

Minimum behavior:

- collapse a burst of filesystem events into one update run
- avoid parallel overlapping rebuilds

### Rebuild serialization

If a rebuild is already running:

- queue one pending rerun, or
- collapse further events into a single follow-up rebuild

The implementation must avoid concurrent writes to `.vela/graph.json` and `.vela/manifest.json`.

## Git Hook Contract

### Supported hooks

- `post-commit`
- `post-checkout`

### CLI surface

Expected command family:

- `vela hooks install`
- `vela hooks uninstall`
- `vela hooks status`

### Behavior

- hooks call the normal Vela CLI path rather than custom internal scripts
- hook installation must be idempotent
- hook errors must be visible

## Retrieval Contract

### Design split

`internal/query` remains responsible for graph reasoning:

- find node
- neighbors
- path
- explain
- stats

`internal/retrieval` owns search candidate generation and ranking.

### Initial retrieval sources

The first iteration should search over:

- node label
- node description
- source file path
- relevant metadata fields

If chunking exists during implementation, chunk text may be added behind the same retrieval boundary.

### Initial ranking signals

Rank using a combination of:

- lexical match strength
- node type importance
- graph connectivity or local expansion
- evidence quality from architecture contracts
- source proximity when a result belongs to a focused file/module area

### CLI output contract

Results should be compact and agent-usable.

Each result should include at minimum:

- label
- node type
- file/path when available
- short reason or score basis

## Agent Behavior Contract

The goal is not to ban grep. The goal is to stop using grep as the default architecture navigation model.

### Graph-first questions

Agents should prefer Vela outputs and CLI commands for:

- architecture questions
- dependency flow
- impact exploration
- subsystem navigation
- cross-file connection questions

### Grep-first questions

Agents should prefer grep for:

- exact strings
- log lines
- env vars
- literal snippets
- highly specific text presence checks

### Repo-local instruction requirement

The repo should include instructions telling agents:

1. If `.vela/GRAPH_REPORT.md` exists, read it first for architecture context.
2. Prefer Vela CLI graph/retrieval commands before raw grep for structural questions.
3. Fall back to grep for exact-text questions.

## Acceptance Criteria

### Freshness

- unchanged repos avoid unnecessary rebuild work
- changed files are detected by file hash, not only graph timestamp
- deleted files disappear from graph outputs after update/build

### Watch mode

- watch mode respects ignore semantics
- rapid edit bursts do not cause rebuild storms
- graph and manifest outputs remain consistent during repeated edits

### Hooks

- hooks can be installed repeatedly without corruption
- graph freshness survives commit and branch switch workflows

### Retrieval

- search quality is materially better than current lexical node-only scoring
- structural questions can often be answered without blind raw grep

### Agent behavior

- graph-first usage is documented locally
- CLI workflows become the default graph navigation surface

## Risks

### Risk 1: Broken partial merge logic

Mitigation:

- prefer full rebuild fallback when incremental safety is uncertain

### Risk 2: Watch/detect drift

Mitigation:

- centralize ignore and path filtering semantics

### Risk 3: Retrieval layer grows into query layer

Mitigation:

- enforce package boundary between `internal/retrieval` and `internal/query`

### Risk 4: Platform-specific agent glue consumes the project

Mitigation:

- keep agent integration lightweight and CLI-first
- prioritize repo-local instructions over heavy platform-specific automation

## Recommended Implementation Sequence

1. manifest contract
2. file-level incremental diffing
3. deletion pruning
4. watch runtime
5. git hooks
6. retrieval package
7. CLI search commands
8. repo-local agent instructions

## Final Principle

The implementation is successful if Vela becomes a trustworthy local graph navigation tool for CLI workflows.

That requires three things together:

- fresh graph state
- useful retrieval
- disciplined graph-first agent behavior

Anything less is half-built architecture theater.
