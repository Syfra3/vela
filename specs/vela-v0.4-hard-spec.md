# Hard Spec: Vela v0.4 — Truth Graph with Evidence

## Adversarial Pre-Mortem
- Failure mode 1: Vela becomes a natural-language search wrapper that returns plausible answers without graph-backed proof. v0.4 must force natural-language exploration through graph node resolution, structural queries, and explicit evidence/provenance/confidence.
- Failure mode 2: CLI and MCP diverge into two product surfaces with different schemas, freshness behavior, and evidence handling. v0.4 must route both through the same core query engine and result schema.
- Failure mode 3: Interface/workspace facts are overclaimed. Extracted routes, client calls, manifests, and naming hints must never be presented as declared contracts unless their source is a declared contract artifact.

## Hidden Assumptions
- v0.4 has no external users depending on JSON-only query runtime compatibility, so `.vela/graph.db` can become required immediately.
- The real workspace smoke test can be run against at least one non-toy workspace available to the maintainer before release.
- Initial MCP compatibility means OpenCode and Claude Code can connect to `vela serve --mcp`, list tools, call each required tool, and receive structured JSON responses without adapter-specific code paths.
- `.vela/workspace.yaml` is authored or generated before multi-codebase routing is expected to be authoritative.
- Freshness can be checked deterministically from `.vela/manifest.json` plus current workspace file state.

## Alternatives Considered
| Approach | Reason Rejected |
|----------|-----------------|
| Keep `.vela/graph.json` as query/runtime truth | JSON is a debug/export artifact in v0.4; SQLite is required for reliable querying, indexing, freshness state, and future agent-scale use. |
| Ship only CLI support in v0.4 | The release positioning targets coding agents; MCP is a first-class adapter, not a later wrapper. |
| Build OpenAPI-only contract support | Many codebases lack OpenAPI specs; v0.4 needs a dynamic Interface Evidence Layer with source-aware confidence ranking. |
| Infer workspace topology from paths by default | Inference is lower-confidence and can route agents incorrectly; v0.4 uses explicit `.vela/workspace.yaml` as declared routing truth. |
| Let MCP tools answer directly from free text | Natural language is allowed only as a resolver into graph-backed structural answers; bag-of-words answers are not truth graph behavior. |
| Maintain JSON-only runtime compatibility | There are no users yet; migration cost is lower now and avoiding dual runtimes prevents divergent behavior. |

## Summary
Vela v0.4 makes Vela the truth graph for coding agents: graph-backed context with evidence, confidence, provenance, and freshness state. The release promotes `.vela/graph.db` to the required runtime query store, keeps JSON/report outputs as generated artifacts, adds a full initial MCP adapter for OpenCode and Claude Code, introduces explicit workspace topology through `.vela/workspace.yaml`, and replaces OpenAPI-only contract thinking with a dynamic Interface Evidence Layer. CLI and MCP must share the same core engine and result schema so humans, scripts, CI, and agents receive equivalent graph facts rendered through different adapters.

## Requirements

### REQ-001: Release Identity and Product Contract
**Description:** Vela v0.4 MUST position and behave as "the truth graph for coding agents — graph-backed context with evidence." Important answers MUST be backed by graph facts and include evidence, provenance, confidence, and freshness status where available.
**Acceptance Criteria:**
- The release version is identified as `v0.4` in release-facing artifacts and status output where version is shown.
- User-facing documentation, CLI help, or release text uses the approved positioning: "truth graph" / "truth with evidence".
- Important graph answers include evidence/provenance/confidence when those fields exist for the underlying facts.
- Answers that lack evidence for a claim explicitly say evidence is unavailable rather than implying proof.
**Edge Cases:**
- If a node is known but has partial evidence, the result includes the known evidence and clearly marks missing evidence fields.
- If a query produces no graph-backed answer, Vela returns a no-result/ambiguous result instead of a speculative answer.
**Out of Scope:**
- General-purpose semantic search that is not grounded in graph facts.

### REQ-002: SQLite Runtime Source of Truth
**Description:** `.vela/graph.db` MUST be the required query/runtime store for v0.4. `.vela/graph.json` MUST remain generated export/debug output only and MUST NOT be used as the runtime source of query truth.
**Acceptance Criteria:**
- `vela build` creates or refreshes `.vela/graph.db` for query/runtime use.
- Runtime query commands and MCP tools read graph truth from `.vela/graph.db` through the core graph/query engine.
- `.vela/graph.json`, when generated, is explicitly treated as export/debug output and not as the authoritative query store.
- If `.vela/graph.db` is missing, incompatible, or unreadable, query surfaces return a clear build/update-required error instead of silently falling back to JSON truth.
**Edge Cases:**
- If `.vela/graph.json` exists but `.vela/graph.db` is missing, Vela still reports runtime graph unavailable and instructs the caller to build/update.
- If both files exist but disagree, `.vela/graph.db` governs query results and freshness diagnostics identify stale/generated artifacts where detectable.
**Out of Scope:**
- JSON-only runtime compatibility for pre-v0.4 users.

### REQ-003: Generated Artifacts and Manifest
**Description:** v0.4 MUST maintain generated artifacts for humans/debugging while keeping SQLite as runtime truth.
**Acceptance Criteria:**
- `.vela/GRAPH_REPORT.md` remains the human-readable graph report output.
- `.vela/graph.json` remains available as a generated export/debug artifact when export is requested or configured.
- `.vela/manifest.json` records enough build inputs and graph metadata to support freshness/change detection.
- Generated artifacts are updated by build/update flows, not by query-only commands.
**Edge Cases:**
- If report or JSON export generation fails after a successful SQLite build, Vela reports the artifact failure without corrupting `.vela/graph.db`.
- If `.vela/manifest.json` is missing, freshness status degrades to unknown/stale and recommends rebuild/update.
**Out of Scope:**
- Treating generated report or JSON export as source input for graph queries.

### REQ-004: Shared Core Result Schema for CLI and MCP
**Description:** CLI commands and MCP tools MUST use the same internal core result schema. CLI renders the schema to text for humans; MCP returns structured results for agents.
**Acceptance Criteria:**
- The core result schema supports at minimum: query kind, resolved subject(s), graph facts, relationships/paths where applicable, evidence/provenance, confidence, freshness status, diagnostics/warnings, and ambiguity candidates.
- Equivalent CLI and MCP calls over the same graph state produce semantically equivalent core results before adapter rendering.
- CLI rendering may summarize or format text, but it must not add claims absent from the core result.
- MCP responses preserve structured fields needed by agents to inspect evidence, confidence, and freshness.
**Edge Cases:**
- If CLI output is truncated for readability, it includes a signal that structured/full data exists or that output was summarized.
- If MCP receives a query that maps to multiple candidates, it returns structured ambiguity data rather than choosing silently.
**Out of Scope:**
- Maintaining separate CLI-only and MCP-only query implementations.

### REQ-005: Graph-Backed Natural-Language Explore
**Description:** `explore` MAY accept natural language, but it MUST act only as a resolver into graph-backed structural answers. Vela MUST NOT use bag-of-words matching as if it were graph truth.
**Acceptance Criteria:**
- `vela explore` and `vela_explore` resolve broad terms to exact graph node candidates, workspace routes, interfaces, or structural query plans before answering.
- Ambiguous terms return candidates with enough identifying information for the caller to select or refine.
- The final answer cites the graph facts used or states that no graph-backed answer could be produced.
- Free-text matching may be used only to propose candidate nodes/routes, not as the proof for the final answer.
**Edge Cases:**
- If no exact node can be resolved, Vela returns an ambiguity/no-resolution response with suggested lookup terms.
- If multiple workspaces/repos match, Vela reports the routing ambiguity instead of querying an arbitrary repo.
**Out of Scope:**
- LLM-generated answers unsupported by graph facts.

### REQ-006: MCP Adapter and Toolset
**Description:** v0.4 MUST include a full initial MCP adapter started by `vela serve --mcp`, targeting OpenCode and Claude Code compatibility.
**Acceptance Criteria:**
- `vela serve --mcp` starts an MCP-compatible server process exposing the v0.4 toolset.
- The MCP toolset includes: `vela_explore`, `vela_lookup`, `vela_explain`, `vela_impact`, `vela_path`, and `vela_status`.
- OpenCode and Claude Code can list and call the tools using their MCP client flows.
- MCP tool responses use structured core result data and include diagnostics/warnings rather than plain text only.
**Edge Cases:**
- If startup cannot open the graph database, the server either fails with a clear error or starts in a status-only/degraded mode that reports the problem consistently.
- If a tool receives invalid arguments, it returns a structured validation error that names the invalid field(s).
**Out of Scope:**
- MCP-specific business logic that bypasses the core query engine.

### REQ-007: MCP Startup Freshness Behavior
**Description:** MCP startup MUST check graph freshness. When safe, it MAY update automatically; otherwise tool responses MUST include clear stale warnings.
**Acceptance Criteria:**
- On startup, `vela serve --mcp` checks `.vela/manifest.json` and current workspace inputs for freshness.
- If a safe incremental update or rebuild can be performed without user intervention, MCP startup performs it or clearly reports that it was performed.
- If safe update is not possible, the MCP server exposes freshness/staleness warnings in `vela_status` and relevant tool responses.
- Stale warnings identify pending stale files or stale scopes where available.
**Edge Cases:**
- If freshness cannot be determined, status is `unknown` and responses recommend `vela update` or `vela build`.
- If auto-update fails, Vela reports the failed update and continues only if it can safely serve stale-but-marked results.
**Out of Scope:**
- Unsafe background rewrites that can corrupt the runtime graph.

### REQ-008: CLI Command Surface
**Description:** v0.4 MUST expose CLI commands equivalent to the MCP tool surface plus build/freshness workflows.
**Acceptance Criteria:**
- CLI includes `vela explore`, `vela lookup`, `vela status`, `vela build`, `vela update`, `vela watch`, and `vela serve --mcp`.
- CLI supports structural search forms or equivalent commands for explain, impact, and path queries.
- CLI commands return non-zero exit status for invalid arguments, missing runtime graph, failed build/update, and incompatible runtime graph states.
- Human-readable CLI output preserves evidence/confidence/freshness signals from the core result.
**Edge Cases:**
- `vela watch` handles bursts of file changes through debouncing rather than rebuilding once per event.
- Structural search refuses or clarifies non-structural bag-of-words queries.
**Out of Scope:**
- GUI/TUI changes beyond what is necessary to avoid contradicting v0.4 behavior.

### REQ-009: Dynamic Interface Evidence Layer
**Description:** v0.4 MUST replace OpenAPI-only contract ingestion with a Dynamic Interface Evidence Layer. Providers emit normalized `InterfaceFact` records from multiple source types.
**Acceptance Criteria:**
- The evidence layer supports provider categories for OpenAPI, Proto, framework routes, HTTP client calls, manifests, workspace hints, and future provider extension.
- Provider candidates are represented as: `OpenAPIProvider`, `ProtoProvider`, `FrameworkRoutesProvider`, `HttpClientProvider`, `ManifestProvider`, and `WorkspaceHintsProvider` or equivalent names/roles.
- Each provider emits normalized `InterfaceFact` records with source/provenance, confidence tier, relationship endpoints where known, and enough identity to link into the graph.
- Interface facts are queryable through CLI/MCP explain/impact/path/explore surfaces where relevant.
**Edge Cases:**
- If two providers report the same interface relationship with different confidence, Vela retains provenance and presents the highest-confidence interpretation without discarding lower-confidence evidence.
- If a provider partially extracts a fact, the fact is marked partial/ambiguous rather than promoted to declared truth.
**Out of Scope:**
- Requiring all codebases to publish OpenAPI specs.

### REQ-010: Evidence Confidence Ranking and Claim Discipline
**Description:** Vela MUST rank interface evidence and must never claim inferred or extracted facts are declared contracts.
**Acceptance Criteria:**
- Declared contracts from OpenAPI/proto/AsyncAPI outrank extracted and inferred evidence.
- Framework route extraction is marked extracted, not declared.
- HTTP client call extraction is marked extracted or inferred according to certainty, not declared.
- Package/service manifests are marked inferred unless they explicitly declare an interface relationship.
- Workspace config hints are marked declared hint, not deep code truth.
- Naming/path heuristics are marked ambiguous.
- Tool and CLI results show the evidence source and confidence tier for interface facts.
**Edge Cases:**
- Conflicting declared and extracted facts are reported with conflict diagnostics rather than merged silently.
- Ambiguous heuristic matches cannot be used as sole proof for a strong answer.
**Out of Scope:**
- Collapsing all evidence into a single unqualified "dependency" label.

### REQ-011: Declared Workspace Source
**Description:** `.vela/workspace.yaml` MUST be the v0.4 declared workspace source for organization/workspace topology, repos, services, contracts/interfaces, and known links.
**Acceptance Criteria:**
- Vela reads `.vela/workspace.yaml` as the explicit workspace topology source when present.
- The workspace model supports organization/workspace identity, repositories, services, interfaces/contracts, and known links between them.
- Workspace facts are treated as routing/topology truth and carry provenance back to `.vela/workspace.yaml`.
- If `.vela/workspace.yaml` is missing, multi-codebase routing is unavailable, inferred, or lower-confidence according to explicit diagnostics.
**Edge Cases:**
- Invalid workspace YAML fails with actionable validation errors that identify the field/path.
- Stale workspace topology is included in freshness diagnostics when the workspace file changes after graph build.
**Out of Scope:**
- Treating workspace topology as proof of deep code-level behavior.

### REQ-012: Multi-Codebase Routing
**Description:** v0.4 MUST support route-first, retrieve-deeply-second multi-codebase reasoning based on declared workspace topology and graph facts.
**Acceptance Criteria:**
- For broad feature/context requests, Vela first identifies candidate workspace/repo/service/interface routes before deep graph retrieval.
- Results distinguish workspace topology facts from code graph facts.
- Routing decisions include evidence/provenance/confidence and expose ambiguity when multiple routes match.
- Impact/path/explore queries can cross repository/service boundaries when declared topology or interface facts connect them.
**Edge Cases:**
- If routing finds a service but no local graph is available for its repo, Vela reports route-known/deep-graph-unavailable.
- If a cross-repo path depends on inferred evidence, the path is marked with that confidence and not presented as declared truth.
**Out of Scope:**
- Automatically cloning or mutating external repositories.

### REQ-013: Freshness, Update, and Watch
**Description:** v0.4 MUST make freshness explicit across CLI and MCP. `vela status` reports graph freshness; `vela update` performs safe incremental update or fallback rebuild; `vela watch` debounces changes.
**Acceptance Criteria:**
- `vela status` reports whether the runtime graph is fresh, stale, missing, incompatible, or unknown.
- `vela status` lists pending stale files/scopes when known.
- `vela update` safely updates the runtime graph incrementally or falls back to rebuild when incremental update is unsafe/unavailable.
- `vela watch` observes relevant workspace changes, debounces bursts, and triggers safe update behavior.
- Query results include freshness/staleness status when results may be affected.
**Edge Cases:**
- Deleted files, renamed files, changed workspace config, and changed interface contract artifacts are freshness-relevant.
- Interrupted update/build leaves the previous valid runtime graph usable or reports graph unavailable; it must not leave a silently corrupt graph as fresh.
**Out of Scope:**
- Perfect real-time synchronization guarantees under all filesystem watcher edge cases.

### REQ-014: Fixture-Proven Definition of Done
**Description:** v0.4 MUST be proven through fixtures and one real workspace smoke test before release.
**Acceptance Criteria:**
- A single-repo fixture proves SQLite graph build and symbol/dependency queries.
- A multi-repo fixture proves `.vela/workspace.yaml` routing.
- An interface evidence fixture proves declared, extracted, inferred, and ambiguous/interface hint facts plus confidence ranking.
- An MCP fixture proves OpenCode/Claude-compatible tool serving.
- A freshness fixture proves stale detection or safe update.
- A CLI/MCP equivalence fixture proves both adapters use the same core result schema.
- One real workspace smoke test proves the release works outside toy fixtures.
**Edge Cases:**
- Fixture failures block release even if manual testing appears to work.
- Real workspace smoke may use redacted output, but must still prove build, freshness, CLI query, MCP query, and evidence-bearing answer behavior.
**Out of Scope:**
- Claiming v0.4 done based only on unit tests without fixtures/smoke coverage.

### REQ-015: Safe Degradation and Diagnostics
**Description:** When Vela cannot provide graph-backed truth, it MUST degrade explicitly through structured diagnostics rather than fabricating answers.
**Acceptance Criteria:**
- Missing graph, stale graph, ambiguous subject, invalid workspace config, provider extraction failure, and evidence conflict states have distinct diagnostics.
- Diagnostics are available in both CLI output and MCP structured responses.
- Degraded results include recommended next actions where possible, such as `vela build`, `vela update`, `vela lookup`, or workspace config fixes.
- Warnings do not suppress available lower-confidence facts; they qualify them.
**Edge Cases:**
- Multiple simultaneous diagnostics are returned together without losing the primary result.
- A lower-confidence answer cannot override a higher-confidence contradictory fact without a conflict warning.
**Out of Scope:**
- Silent fallback to stale JSON, unconstrained free-text search, or unqualified inferred answers.

## Open Questions
- None blocking for v0.4 implementation. Provider internals, exact SQLite schema, and exact YAML field names should be finalized in technical design without changing the behavioral contracts above.

## Trade-offs
- Direct SQLite migration reduces compatibility complexity now but requires all v0.4 query flows to handle missing/incompatible database states cleanly.
- Full MCP scope increases release surface area but is necessary for the agent-first product position.
- Explicit workspace YAML avoids unsafe topology inference but requires users/projects to declare workspace topology before high-confidence multi-codebase routing.
- Evidence ranking may produce more qualified answers than users expect, but this preserves Vela's truth-with-proof contract.
- Shared core schema constrains CLI/MCP adapter freedom but prevents behavior drift and simplifies test equivalence.

## Risk Level
high — Justification: v0.4 changes the runtime storage source of truth, adds an MCP server surface, introduces multi-codebase routing, and adds a multi-provider evidence model. The release is feasible, but correctness depends on strict fixture coverage, schema sharing, and disciplined evidence labeling.

## Requirement Trace Summary
- REQ-001: Release identity and proof-backed answers.
- REQ-002: SQLite runtime source of truth.
- REQ-003: Generated artifacts and manifest.
- REQ-004: Shared core result schema.
- REQ-005: Graph-backed natural-language explore.
- REQ-006: MCP adapter and toolset.
- REQ-007: MCP startup freshness behavior.
- REQ-008: CLI command surface.
- REQ-009: Dynamic Interface Evidence Layer.
- REQ-010: Evidence ranking and claim discipline.
- REQ-011: Declared workspace source.
- REQ-012: Multi-codebase routing.
- REQ-013: Freshness/update/watch.
- REQ-014: Fixture-proven definition of done.
- REQ-015: Safe degradation and diagnostics.
