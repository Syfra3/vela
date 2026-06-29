# Vela v0.4 Archive Report

## Archive Status

- Version: `vela-v0.4`
- Workflow: Bob final review archive
- Final judge verdict: pass with warnings
- Release blockers: none under the active final gate definition
- Scenario traceability: SCN-001 through SCN-026 complete and traceable
- Archive result: complete

## Release Scope

Vela v0.4 ships the truth graph with evidence release scope: `.vela/graph.db` is the required runtime query store, `.vela/graph.json` and `.vela/GRAPH_REPORT.md` remain generated/debug artifacts, and CLI plus MCP query surfaces share the same core result semantics. The release adds graph-backed natural-language exploration, structured diagnostics, evidence confidence handling, declared workspace topology, freshness/status/update/watch behavior, MCP tool support, fixture proof, and one redacted real-workspace smoke proof.

Primary product contract:

- Important answers must be grounded in graph facts.
- Evidence, provenance, confidence, freshness, and diagnostics must be preserved where available.
- Vela must not invent graph truth from free text, stale JSON, or unqualified inferred evidence.
- CLI and MCP adapters must not diverge in core behavior.

## Completed Scenarios

SCN-001 through SCN-026 are implemented and traceable across:

- `specs/.implementation-complete`
- `features/vela-v0.4-truth-graph.feature`
- `N/A` (legacy workflow artifacts were pruned)
- `reports/judge_report.md`

Scenario coverage is recorded as 100% in the final judge report. The implemented scenario set covers:

- Proof-backed answers and no speculative answers: SCN-001, SCN-002
- SQLite runtime truth and generated artifacts: SCN-003, SCN-004
- CLI/MCP shared result behavior: SCN-005, SCN-008, SCN-009, SCN-023, SCN-024
- Graph-backed exploration and ambiguity handling: SCN-006, SCN-007, SCN-026
- CLI command surface: SCN-011
- Interface evidence, ranking, and cross-repo bridge confidence: SCN-012, SCN-013, SCN-017
- Workspace topology and route-first multi-codebase behavior: SCN-014, SCN-015, SCN-016, SCN-022
- Freshness, status, safe update, and watch debounce: SCN-010, SCN-018, SCN-019, SCN-020
- Fixture and release smoke proof: SCN-021, SCN-022, SCN-023, SCN-024, SCN-025

## Gate Results

Active gate source: `N/A` (legacy gate file pruned).

| Gate command | Result |
| --- | --- |
| `go test ./cmd/... ./internal/... ./pkg/... ./scripts ./tests/fixtures/extract -count=1` | pass |
| `go vet ./cmd/... ./internal/... ./pkg/... ./scripts ./tests/fixtures/extract` | pass |
| `go mod tidy -diff` | pass |
| `go test ./cmd/vela -run 'TestSCN025_' -count=1` | pass |

The full `go test ./... -count=1` command remains a documented non-active gate because malformed detect fixture packages contain empty `.go` files that fail before package tests can run. This is tracked as a fixture issue rather than a Vela v0.4 release blocker.

## SCN-025 Real Workspace Smoke

SCN-025 passed against a maintainer-selected real workspace with no-secrets redaction. Persisted archive evidence must continue to use `<REAL_WORKSPACE>` rather than the raw absolute workspace path.

Recorded smoke proof in `reports/SCN-025-real-workspace-smoke.md` includes:

- `vela build <REAL_WORKSPACE>` passed.
- `.vela/graph.json`, `.vela/graph.db`, and `.vela/manifest.json` were present under `<REAL_WORKSPACE>`.
- `vela status --graph <REAL_WORKSPACE>/.vela/graph.json --baseline ""` reported fresh graph state.
- `vela lookup "ExecuteAugusteTool" --graph <REAL_WORKSPACE>/.vela/graph.json --limit 5` returned graph-backed candidates.
- `vela explain "ExecuteAugusteToolUseCase" --graph <REAL_WORKSPACE>/.vela/graph.json --format json` returned evidence-bearing structured output.
- `VELA_SCN025_WORKSPACE=<REAL_WORKSPACE> go test ./cmd/vela -run TestSCN025_RealWorkspaceSmokeHarness -count=1` passed and exercised MCP `vela_explain` with structured `query.Result` content.

No environment files, tokens, credentials, private keys, or secret-like values are included in the persisted smoke report or this archive.

## Final Review Warnings

The final Bob judge report recorded no critical, high, or medium findings. Remaining warnings are low-severity release notes and hardening follow-ups:

- SCN-019 proof remains focused on failed update rollback and preservation behavior. Future hardening should add success-path stale-to-fresh proof and direct restoration assertions for `.vela/graph.db` and `.vela/manifest.json`.
- SCN-025 real workspace smoke is intentionally narrow. It proves one maintainer-selected real workspace and one evidence-bearing subject, not broad corpus determinism.
- Full `go test ./...` remains unavailable as an active gate until malformed detect fixture packages with empty `.go` files are cleaned or isolated.

## Next Hardening Tasks

Recommended post-release or next-workflow tasks:

1. Expand SCN-019 coverage with a success-path stale-to-fresh update proof and byte-for-byte restoration checks for generated runtime artifacts.
2. Broaden SCN-025 into a small, repeatable real-workspace smoke matrix while preserving no-secrets redaction and `<REAL_WORKSPACE>` path masking in persisted reports.
3. Clean or restructure malformed detect fixture packages so `go test ./... -count=1` can be promoted back to an active gate.
4. Add release-scale performance and determinism checks for larger SQLite graphs, evidence joins, and MCP structured responses.

## Archive Evidence Index

- `reports/judge_report.md` - Final Bob review verdict, active gate results, scenario traceability, warnings, and release blocker status.
- `reports/SCN-025-real-workspace-smoke.md` - Redacted real-workspace smoke evidence using `<REAL_WORKSPACE>`.
- `specs/.implementation-complete` - Implementation marker for SCN-001 through SCN-026.
- `N/A` (legacy gate file pruned) - Active final gate definition and known non-gate blocker.
- `N/A` (legacy workflow artifacts pruned) - RED/GREEN/REFACTOR evidence for implemented scenarios.
- `specs/vela-v0.4-hard-spec.md` - Approved hard spec and requirement contract.
- `features/vela-v0.4-truth-graph.feature` - Gherkin scenario contract.
- `docs/VELA_V0_4_TECHNICAL_DESIGN.md` - Technical design and scenario mapping.
- `docs/VELA_V0_4_TASK_BREAKDOWN.md` - Work-unit plan and review workload forecast.

## Archive Decision

Vela v0.4 is archived as feature complete under the active final gate definition. The release carries only low-severity warnings for future hardening, with no release blockers and with SCN-001 through SCN-026 implemented and traceable.

skill_resolution: none
