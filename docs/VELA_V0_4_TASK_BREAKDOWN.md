# Vela v0.4 Task Breakdown

This plan translates the approved v0.4 hard spec, feature scenarios, and technical design into strict TDD implementation work units. It is a planning artifact only; production code should be changed in the apply phase.

## Work Unit Order

1. **Test and Fixture Baseline** - Establish deterministic fixture and test harness foundations before changing runtime behavior.
2. **SQLite Runtime Schema and Persistence** - Make `.vela/graph.db` the runtime source of truth and preserve generated artifacts as debug outputs.
3. **Shared Result Schema and Proof Metadata** - Define the adapter-independent core result envelope before adding more query surfaces.
4. **DB-Backed Structural Queries and CLI Commands** - Implement lookup, explain, impact, and path against SQLite, including first-class CLI commands.
5. **Explore Resolver** - Add natural-language-to-structure resolution after exact structural operations exist.
6. **Dynamic Interface Evidence Layer** - Add normalized interface facts, provider outputs, ranking, and conflict handling.
7. **Workspace YAML Topology and Route-First Flow** - Add declared topology and multi-codebase routing once interface bridges can be represented.
8. **Freshness, Update, Watch, and Status** - Extend manifest-driven freshness and safe update behavior across CLI and core results.
9. **MCP Adapter and Agent Compatibility** - Expose the shared engine through MCP with degraded missing-graph behavior.
10. **CLI/MCP Equivalence and Real Workspace Smoke** - Prove release readiness across fixtures and a real workspace.

## Scenario Mapping

| Scenario | Primary work unit(s) |
| --- | --- |
| SCN-001 | WU-03, WU-04 |
| SCN-002 | WU-03, WU-04 |
| SCN-003 | WU-02, WU-09 |
| SCN-004 | WU-02, WU-01 |
| SCN-005 | WU-03, WU-09, WU-10 |
| SCN-006 | WU-05 |
| SCN-007 | WU-05, WU-03 |
| SCN-008 | WU-09 |
| SCN-009 | WU-09, WU-10 |
| SCN-010 | WU-08, WU-09 |
| SCN-011 | WU-04, WU-08, WU-09 |
| SCN-012 | WU-06 |
| SCN-013 | WU-06, WU-03 |
| SCN-014 | WU-07 |
| SCN-015 | WU-07, WU-08 |
| SCN-016 | WU-07, WU-05 |
| SCN-017 | WU-06, WU-07, WU-04 |
| SCN-018 | WU-08 |
| SCN-019 | WU-08 |
| SCN-020 | WU-08 |
| SCN-021 | WU-01, WU-02, WU-04 |
| SCN-022 | WU-07, WU-10 |
| SCN-023 | WU-09, WU-10 |
| SCN-024 | WU-03, WU-09, WU-10 |
| SCN-025 | WU-10 |
| SCN-026 | WU-03, WU-05, WU-08 |

## WU-01: Test and Fixture Baseline

**Goal:** Create or clean the v0.4 fixture layout and test harness so every later slice can begin with a failing scenario test.

**Scenarios covered:** SCN-004, SCN-021.

**Red tests to write first:**
- A single-repo fixture acceptance test that expects `vela build` to produce `.vela/graph.db` and `.vela/manifest.json`.
- A fixture query test that expects symbol and dependency queries to read from SQLite runtime truth once available.
- A fixture integrity test that fails if generated debug artifacts are treated as query inputs.

**Expected implementation areas:** fixture directories, acceptance test harness, CLI integration test helpers, graph build test utilities.

**Verification commands:** `go test ./...`, plus any existing CLI fixture test command used by the repo.

**Acceptance criteria:** Fixtures are deterministic, scenario tags are traceable, tests fail for missing v0.4 behavior before implementation, and no production behavior is changed in this baseline except test-only scaffolding if needed.

**Risk notes:** Overbuilding fixtures early can hide design flaws. Keep the baseline minimal and expand fixtures only when a work unit needs them.

## WU-02: SQLite Runtime Schema and Persistence

**Goal:** Promote `.vela/graph.db` to required runtime query storage and write manifest/debug artifacts through build/update flows only.

**Scenarios covered:** SCN-003, SCN-004, SCN-021.

**Red tests to write first:**
- Missing `.vela/graph.db` with existing `.vela/graph.json` returns runtime-unavailable and recommends `vela build` or `vela update`.
- `vela build` creates required SQLite tables, indexes, schema metadata, and `.vela/manifest.json`.
- Runtime queries do not fall back to JSON when DB is missing or unreadable.
- SQLite-backed symbol/dependency fixture queries return expected nodes and relationships.

**Expected implementation areas:** build pipeline, SQLite schema/migrations, graph persistence writer, manifest writer, runtime graph opener, generated report/export flow.

**Verification commands:** `go test ./...`; fixture command: `vela build` against the single-repo fixture followed by runtime query tests.

**Acceptance criteria:** SQLite is the only runtime query store; JSON/report are generated artifacts; build failures do not corrupt an existing valid DB; missing or incompatible DB produces structured diagnostics.

**Risk notes:** Schema churn can block downstream work. Use direct v0.4 migration but keep schema versioning explicit from the first DB write.

## WU-03: Shared Result Schema and Proof Metadata

**Goal:** Define the core result contract used by CLI and MCP before adapter rendering.

**Scenarios covered:** SCN-001, SCN-002, SCN-005, SCN-007, SCN-013, SCN-024, SCN-026.

**Red tests to write first:**
- Explain result includes resolved subject, facts, evidence/provenance, confidence, freshness, and diagnostics when evidence exists.
- Unsupported claims return `unresolved` or no-result with no speculative facts.
- Multiple diagnostics, such as stale graph plus ambiguous subject, are preserved together.
- Ambiguous candidate result includes distinguishers and recommended next actions.
- Core result golden tests compare adapter-independent JSON semantics.

**Expected implementation areas:** core query result types, diagnostics model, evidence attachment helpers, confidence/freshness fields, CLI render contract tests, adapter-independent golden fixtures.

**Verification commands:** `go test ./...`; golden result update command if the repo already has one.

**Acceptance criteria:** One schema represents query kind, subjects, facts, relationships, paths, evidence, confidence, freshness, diagnostics, ambiguity, and render hints; adapters cannot invent claims outside the core result.

**Risk notes:** A too-large schema can slow TDD. Implement the full envelope shape early, but allow fields to be empty until later work units populate them.

## WU-04: DB-Backed Structural Queries and CLI Commands

**Goal:** Implement SQLite-backed `lookup`, `explain`, `impact`, and `path`, and expose first-class CLI commands `vela explain`, `vela impact`, and `vela path` while keeping structural `vela search` forms as compatibility/advanced mode.

**Scenarios covered:** SCN-001, SCN-002, SCN-011, SCN-017, SCN-021.

**Red tests to write first:**
- `vela lookup` returns candidates with canonical key, kind, repo/service/file distinguishers, confidence, and evidence summary.
- `vela explain <subject>` returns direct relationships and proof metadata from SQLite.
- `vela impact <subject>` traverses reverse dependencies and refuses ambiguous strong answers.
- `vela path <from> <to>` returns ordered paths with per-hop type, domain, confidence, and evidence.
- CLI help exposes `explore`, `lookup`, `status`, `build`, `update`, `watch`, `serve --mcp`, `explain`, `impact`, and `path`.
- Structural `vela search` forms still map to the same core operations, while bag-of-words forms are refused or clarified.

**Expected implementation areas:** core query engine, SQLite graph reader, CLI command registration, CLI renderers, search compatibility parser.

**Verification commands:** `go test ./...`; CLI fixture runs for `lookup`, `explain`, `impact`, `path`, and structural `search` compatibility.

**Acceptance criteria:** Structural queries are DB-backed, return shared core results before rendering, preserve proof metadata, and use non-zero exits for invalid arguments or unavailable runtime graph.

**Risk notes:** Path traversal can become expensive. Start with bounded paths and required indexes rather than loading the full graph into memory.

## WU-05: Explore Resolver

**Goal:** Make `explore` accept broad natural language only as a resolver into exact graph subjects, routes, interfaces, or structural query plans.

**Scenarios covered:** SCN-006, SCN-007, SCN-016, SCN-026.

**Red tests to write first:**
- Broad terms return candidate nodes/routes before any final answer.
- Ambiguous terms return candidate lists with distinguishers and refinement guidance.
- Final explore answers cite graph facts used and never cite free-text matches as proof.
- Combined stale and ambiguity diagnostics are both present in degraded explore output.

**Expected implementation areas:** explore resolver, term extraction, candidate ranking, structural plan builder, ambiguity diagnostics, CLI explore renderer.

**Verification commands:** `go test ./...`; CLI fixture command for `vela explore` broad and ambiguous prompts.

**Acceptance criteria:** Free-text matching is candidate discovery only; final answers are graph-backed; broad multi-repo requests route before deep retrieval once topology exists.

**Risk notes:** Avoid semantic-search creep. Keep resolver output explainable: candidate terms, scopes, chosen structural operation, and facts used.

## WU-06: Dynamic Interface Evidence Layer

**Goal:** Normalize interface facts from provider outputs and enforce confidence ranking and claim discipline.

**Scenarios covered:** SCN-012, SCN-013, SCN-017.

**Red tests to write first:**
- Interface fixture marks declared contracts, framework routes, HTTP clients, manifests, workspace hints, and naming heuristics with the correct claim status.
- Conflicting declared and inferred/ambiguous interface facts preserve all evidence and emit a conflict diagnostic.
- Cross-repo path through an inferred interface bridge marks the bridge as inferred and does not present it as declared contract truth.
- Go route extraction fixture emits extracted route facts.
- TypeScript/JavaScript client fixture emits extracted or inferred client-call facts.

**Expected implementation areas:** provider interface, `InterfaceFact` model, evidence normalization, ranking/deduplication, interface graph linking, Go route provider, TypeScript/JavaScript client provider fixtures.

**Verification commands:** `go test ./...`; interface evidence fixture verification.

**Acceptance criteria:** Required provider roles exist or are represented equivalently; facts preserve provenance, confidence, claim status, partiality, and conflicts; inferred/extracted facts are never promoted to declared truth.

**Risk notes:** Provider scope can explode. First route/client providers are Go plus TypeScript/JavaScript fixtures; defer additional framework depth unless needed for scenarios.

## WU-07: Workspace YAML Topology and Route-First Flow

**Goal:** Ingest `.vela/workspace.yaml` as declared topology and support route-first, retrieve-deeply-second multi-codebase behavior.

**Scenarios covered:** SCN-014, SCN-015, SCN-016, SCN-017, SCN-022.

**Red tests to write first:**
- Valid workspace YAML creates workspace topology facts with provenance to `.vela/workspace.yaml`.
- Invalid workspace YAML returns actionable validation diagnostics with field/path and does not create routing truth.
- Multi-repo explore identifies candidate repositories, services, or interfaces before deep graph retrieval.
- Cross-repo path uses workspace/interface bridge facts and labels topology facts separately from code facts.
- Route-known/deep-graph-unavailable diagnostic appears when topology knows a service but the repo graph is unavailable.

**Expected implementation areas:** workspace YAML parser/validator, topology model, workspace fact persistence, route resolver, multi-repo fixture, cross-repo bridge integration.

**Verification commands:** `go test ./...`; multi-repo fixture verification.

**Acceptance criteria:** Workspace topology is declared routing truth with provenance; it is not treated as deep code truth; route ambiguity is explicit.

**Risk notes:** YAML schema choices affect users and fixtures. Lock only the fields needed for v0.4 and keep diagnostics precise.

## WU-08: Freshness, Update, Watch, and Status

**Goal:** Make freshness explicit across build, status, update, watch, CLI queries, and MCP startup/tool responses.

**Scenarios covered:** SCN-010, SCN-011, SCN-015, SCN-018, SCN-019, SCN-020, SCN-026.

**Red tests to write first:**
- `vela status` reports `fresh`, `stale`, `missing`, `incompatible`, or `unknown` and includes recommended next actions.
- Changed source files after build produce stale status and list stale files/scopes when known.
- Changed `.vela/workspace.yaml` and contract/provider inputs are freshness-relevant.
- `vela update` safely refreshes stale graph or falls back to rebuild and never marks a failed/interrupted update fresh.
- `vela watch` debounces bursts into one safe update cycle and serializes overlapping updates.
- Stale or unknown freshness attaches warnings to relevant query results.

**Expected implementation areas:** manifest model, freshness diffing, status command, update orchestration, watch debounce, staged/transactional graph replacement, freshness diagnostics.

**Verification commands:** `go test ./...`; freshness fixture for change/delete/rename/workspace-contract changes; watch debounce integration test if supported.

**Acceptance criteria:** Freshness is deterministic where manifest data exists, unknown otherwise; query surfaces qualify stale results; failed updates preserve previous valid graph or report graph unavailable.

**Risk notes:** Filesystem watch tests can be flaky. Prefer injectable clocks/debounce hooks and reserve one integration smoke for the real watcher.

## WU-09: MCP Adapter and Agent Compatibility

**Goal:** Add `vela serve --mcp` with required tools routed through the shared engine and explicit degraded behavior for missing runtime graphs.

**Scenarios covered:** SCN-003, SCN-005, SCN-008, SCN-009, SCN-010, SCN-011, SCN-023, SCN-024.

**Red tests to write first:**
- `vela serve --mcp` registers `vela_explore`, `vela_lookup`, `vela_explain`, `vela_impact`, `vela_path`, and `vela_status`.
- MCP tool schemas validate required fields and return structured validation diagnostics.
- With a fresh graph, each tool returns the shared core result envelope.
- With a missing graph, server starts in degraded mode: `vela_status` works; all other tools return actionable `run vela build` diagnostics and do not silently build unknown workspaces.
- With stale graph and unsafe update, status and tool responses include stale warnings.
- OpenCode-compatible and Claude Code-compatible harnesses can list and call tools without adapter-specific branches.

**Expected implementation areas:** MCP server startup, tool registration, tool argument validation, core engine adapter, degraded-mode handling, MCP compatibility fixtures.

**Verification commands:** `go test ./...`; MCP fixture harness for list-tools and tool calls; manual or automated OpenCode/Claude Code compatibility checks when available.

**Acceptance criteria:** MCP responses are structured, preserve diagnostics/proof metadata, and share core semantics with CLI; missing graph behavior is degraded/status-only rather than silent build.

**Risk notes:** MCP client differences can cause late failures. Keep schemas JSON-schema-compatible and avoid client-specific logic.

## WU-10: CLI/MCP Equivalence and Real Workspace Smoke

**Goal:** Prove v0.4 is release-ready through fixture equivalence and one maintainer-selected real workspace smoke test.

**Scenarios covered:** SCN-005, SCN-009, SCN-021, SCN-022, SCN-023, SCN-024, SCN-025.

**Red tests to write first:**
- CLI and MCP equivalent explain query match expected core result semantics and neither adapter adds unsupported claims.
- Release fixture suite runs single-repo, multi-repo, interface evidence, MCP, freshness, and equivalence fixtures.
- Real workspace smoke script requires build/update, status, one evidence-bearing CLI query, and one evidence-bearing MCP call.

**Expected implementation areas:** release verification harness, adapter equivalence tests, smoke test script/docs, fixture orchestration, CI wiring if available.

**Verification commands:** `go test ./...`; fixture verification command; real workspace smoke command selected by maintainer.

**Acceptance criteria:** Fixture failures block release; real workspace smoke proves behavior outside toy fixtures; redacted smoke output still proves graph build/update, freshness, CLI query, MCP query, and evidence-bearing answers.

**Risk notes:** The real workspace path and acceptable redaction policy are maintainer-dependent. Do not make release claims until this smoke passes.

## Recommended First 5 TDD Scenarios

1. **SCN-003: SQLite graph database is required for runtime queries** - Establishes the most important safety boundary: no JSON fallback and clear build/update diagnostics.
2. **SCN-004: Build creates runtime and generated graph artifacts with SQLite as truth** - Creates the runtime foundation needed by every query and fixture.
3. **SCN-001: Important answers include proof metadata when evidence is available** - Forces the shared result schema to carry evidence, confidence, provenance, and freshness from the start.
4. **SCN-002: Vela refuses to invent an answer when no graph-backed fact exists** - Locks the truth-graph product contract before adding explore or MCP surfaces.
5. **SCN-021: Single-repo fixture proves SQLite graph build and symbol dependency queries** - Provides the first end-to-end proof that SQLite-backed graph facts are queryable.

## Review Workload Forecast

| Work unit | Estimated changed lines | Notes |
| --- | ---: | --- |
| WU-01 Test and Fixture Baseline | 250-450 | Mostly tests/fixtures; may exceed budget if fixture harness is new. |
| WU-02 SQLite Runtime Schema and Persistence | 600-900 | High-risk storage and build slice. |
| WU-03 Shared Result Schema and Proof Metadata | 350-550 | Core types plus golden tests. |
| WU-04 DB-Backed Structural Queries and CLI Commands | 600-900 | Query engine plus CLI command surface. |
| WU-05 Explore Resolver | 350-550 | Resolver and ambiguity behavior. |
| WU-06 Dynamic Interface Evidence Layer | 800-1200 | Providers, fixtures, ranking, conflicts. |
| WU-07 Workspace YAML Topology and Route-First Flow | 600-900 | YAML model, validation, routing, multi-repo fixture. |
| WU-08 Freshness, Update, Watch, and Status | 700-1000 | Manifest diffing, safe update, watcher behavior. |
| WU-09 MCP Adapter and Agent Compatibility | 500-800 | MCP server/tool schemas and degraded mode. |
| WU-10 CLI/MCP Equivalence and Real Workspace Smoke | 250-450 | Verification harness and smoke docs/scripts. |

**Chained PRs recommended:** Yes. This change is materially over the 400-line review budget and spans storage, query semantics, CLI, MCP, freshness, workspace topology, and providers.

**400-line budget risk:** High for WU-02, WU-04, WU-06, WU-07, WU-08, and WU-09. Medium for WU-01, WU-03, WU-05, and WU-10.

**Decision needed before apply:** Yes. The apply phase should choose chained/stacked PRs or record an explicit `size:exception`. Recommended chaining is one PR per work unit, with WU-06 and WU-08 split further if they exceed reviewable size.

## Explicit Non-Goals

- Do not implement production code during task planning.
- Do not preserve JSON-only runtime compatibility for v0.4 query behavior.
- Do not add MCP-specific query logic that bypasses the core engine.
- Do not silently build unknown workspaces from MCP degraded mode.
- Do not present broad free-text matches as graph-backed proof.
- Do not treat workspace topology as proof of deep code behavior.
- Do not present extracted, inferred, or ambiguous interface facts as declared contracts.
- Do not require all codebases to publish OpenAPI specs.
- Do not automatically clone or mutate external repositories for multi-codebase routing.
- Do not claim v0.4 complete without fixture coverage and real workspace smoke.

## Open Questions

- What is the exact existing verification command for the full fixture suite, if different from `go test ./...`?
- Which maintainer-selected real workspace should be used for SCN-025, and what redaction rules apply to smoke output?
- Should WU-06 be split into separate provider PRs after the shared provider interface lands, or is one larger evidence-layer PR acceptable with `size:exception`?
- Should `vela search` compatibility be hidden from primary help while `vela explain`, `vela impact`, and `vela path` become the documented first-class route?
