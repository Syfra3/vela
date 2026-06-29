# Hard Spec: Vela Agent Explore Runtime Contract (Phase 0 + Phase 1)

## Adversarial Pre-Mortem
- Failure mode 1: `vela explore` routes a broad natural-language prompt to the wrong low-level primitive and returns a confident-looking answer that is not actually supported by graph facts.
- Failure mode 2: Runtime query paths silently fall back to `.vela/graph.json` or stale generated artifacts when `.vela/graph.db` is missing, causing agents to trust data that is not the runtime source of truth.
- Failure mode 3: Layered evidence collapses memory, contract, workspace, and repo/code facts into one undifferentiated answer, making prior decisions or inferred topology look like executable code truth.
- Failure mode 4: MCP connect-time catch-up blocks agent startup or hides warming state, causing clients either to hang or to proceed without knowing whether the graph is fresh.
- Failure mode 5: Vela ships `vela_explore` but leaves installation to manual config editing, so users never get the MCP tool wired into their coding agents.

## Hidden Assumptions
- Existing graph query primitives can already provide enough lookup, explain, dependency, reverse-dependency, path, impact, status, and workspace/contract evidence to support the Phase 1 shell without a query-engine rewrite.
- Runtime graph answers can be read from `.vela/graph.db`; `.vela/graph.json` remains only a generated/debug/export artifact for runtime-query purposes.
- CLI and MCP can share one internal explore result envelope while rendering it differently: human-readable ordered sections for CLI, structured content for MCP.
- MCP can expose `vela_explore` without removing or weakening the existing low-level MCP tools used by maintainers and tests.
- Connect-time catch-up or freshness probing can be represented as `warming` without implementing the later watcher/debounce/auto-sync phase.
- Memory evidence is available only when the planner identifies a decision/history/why/previous-work intent; normal code-structure prompts do not request memory.
- Agent instruction text can be updated as part of the Phase 1 shell, but no production code, tests, watcher, daemon, debounce, or auto-sync implementation is part of this spec-writing pass.
- Existing CLI installer behavior for OpenCode and Claude Code can be reused by the TUI instead of duplicating agent config writing in the presentation layer.

## Alternatives Considered
| Approach | Reason Rejected |
|----------|----------------|
| Keep agents on low-level commands as the primary workflow | Rejected because the PRD's core problem is that agents must choose exact query forms before they have enough context. |
| Let runtime queries fall back from `.vela/graph.db` to `.vela/graph.json` | Rejected because runtime truth must be SQLite-backed; JSON fallback would hide missing DB setup and stale generated artifacts. |
| Implement watcher/debounce/auto-sync before stabilizing explore | Rejected for the first slice because a fresh graph is not useful until the answer contract and routing behavior are stable. |
| Always include memory observations in explore responses | Rejected because memory can add noise and privacy risk; it must be opt-in by query intent. |
| Block every MCP first query until catch-up completes | Rejected because connect-time catch-up can be slow; agents need an explicit `warming` state and retry guidance rather than an opaque wait. |
| Replace existing specialized commands with `explore` | Rejected because specialized commands remain valuable for humans, tests, debugging, and precise low-level queries. |

## Summary
Vela will define and implement, in the first implementation slice only after human approval, a stable agent-facing exploration contract for `vela explore` and MCP `vela_explore`. The Phase 0 + Phase 1 scope is the exact input/output contract, a planner/router over existing primitives, runtime DB-required diagnostics, freshness/warming reporting, layered evidence labels, memory evidence opt-in by query intent, and agent instruction shell updates. This scope explicitly excludes watcher/debounce/auto-sync implementation, new language extractors, benchmark harness work, and any removal of existing low-level commands.

## Requirements

### REQ-001: Phase 0 + Phase 1 boundary is explicit
**Description:** This spec covers only contract design and the initial explore CLI/MCP shell. Implementation must not start until the orchestrator receives explicit user approval.
**Acceptance Criteria:**
- The implementation slice includes `vela explore`, MCP `vela_explore`, the shared result contract, intent planning/routing over existing primitives, DB-required diagnostics, freshness/warming reporting, layered evidence labeling, memory opt-in, and Phase 1 agent instruction text.
- The implementation slice does not include watcher/debounce/auto-sync behavior, global daemon behavior, new extractors, benchmark implementation, or broad query-engine rewrites.
- The implementation slice includes a TUI wizard that makes the existing agent installer discoverable and safe for local users.
- The orchestrator asks the user before TDD implementation begins.
- Every scenario in `features/vela-agent-explore-runtime.feature` is suitable for one-scenario-at-a-time TDD.
**Edge Cases:**
- If an existing primitive cannot support a requested intent family, explore must return a qualified partial/unavailable section instead of inventing a result or expanding scope.
- If an implementation task would require watcher/debounce/auto-sync code, it must be deferred to a later phase.
**Out of Scope:**
- Starting implementation or writing tests during this spec pass.
- Implementing Phase 2+ PRD work.

### REQ-002: CLI exposes `vela explore` as the primary structural entrypoint
**Description:** The CLI must accept natural-language structural, architectural, flow, dependency, ownership, and impact questions through `vela explore` without requiring the caller to choose a low-level query form first.
**Acceptance Criteria:**
- `vela explore "<question>"` accepts a non-empty natural-language question.
- The CLI renders the stable explore response sections in a human-readable order.
- The CLI reports the interpreted intent and confidence when the planner can determine them.
- The CLI returns an actionable diagnostic for empty questions.
- The CLI uses `.vela/graph.db` as runtime truth for graph-backed answers and does not silently read `.vela/graph.json` as a fallback.
**Edge Cases:**
- Ambiguous prompts return candidates or clarification guidance instead of a strong claim.
- Broad area-survey prompts may return partial context with limits and suggested next queries.
**Out of Scope:**
- Making `vela explore` a free-text grep replacement.
- Removing `vela lookup`, `vela search`, `vela explain`, `vela impact`, or `vela path`.

### REQ-003: MCP exposes `vela_explore` with the same core semantics
**Description:** The MCP server must expose `vela_explore` as the primary agent-facing tool while preserving existing specialized MCP tools.
**Acceptance Criteria:**
- MCP tool listing includes `vela_explore`.
- `vela_explore` accepts at minimum `query: string` and may accept non-semantic output controls such as result limits.
- `vela_explore` returns structured content using the shared explore result envelope.
- For the same graph state and question, CLI and MCP outputs preserve the same core status, freshness, intent, evidence, diagnostics, and suggested next-query semantics.
- Existing low-level MCP tools remain available for maintainers, tests, and exact operations.
**Edge Cases:**
- Invalid MCP arguments return structured validation diagnostics rather than malformed tool output.
- If runtime graph state is unavailable, the MCP tool returns a structured unavailable result instead of transport-level failure unless the server itself is unhealthy.
**Out of Scope:**
- Removing existing MCP tools or requiring agents to memorize them as the default workflow.

### REQ-004: Explore responses use a stable versioned envelope
**Description:** CLI and MCP must be backed by a single stable response envelope so agents can rely on consistent sections across intents and degraded states.
**Acceptance Criteria:**
- Every explore response includes `schema_version` with initial value `vela.explore.v1`.
- Every response includes a `status` of `ok`, `partial`, `ambiguous`, `unavailable`, or `error`.
- Every response includes these user-facing sections, even when a section is marked empty, not relevant, unavailable, or not requested: `Answer`, `Freshness`, `Relevant source`, `Paths and relationships`, `Impact radius`, `Layered evidence`, `Confidence and limits`, and `Suggested next queries`.
- Structured responses expose equivalent fields: `answer`, `freshness`, `relevant_source`, `paths_and_relationships`, `impact_radius`, `layered_evidence`, `confidence_and_limits`, `suggested_next_queries`, `diagnostics`, and `interpreted_intent`.
- Diagnostics include stable machine-readable codes and actionable messages.
**Edge Cases:**
- A path query with no supported path returns a no-path answer with evidence/limits, not a generic failure.
- An impact query with no reachable impact returns an explicit empty impact radius.
**Out of Scope:**
- Freezing every wording detail of the CLI text renderer.
- Guaranteeing snippets for graph facts that do not carry line/source metadata.

### REQ-005: Runtime graph DB is required and missing DB diagnostics are actionable
**Description:** Runtime explore paths must require `.vela/graph.db` and must not silently fall back to `.vela/graph.json`.
**Acceptance Criteria:**
- If `.vela/graph.db` is missing, unreadable, or invalid for runtime queries, explore returns freshness state `unavailable`.
- Missing DB diagnostics state that `.vela/graph.db` is required for runtime graph answers.
- Missing DB diagnostics include next actions: `vela build`, `vela update`, and `vela status`.
- If `.vela/graph.json` exists but `.vela/graph.db` is missing, explore does not use JSON as graph truth.
- CLI missing-DB behavior fails command execution; MCP missing-DB behavior returns a structured unavailable tool result.
**Edge Cases:**
- If DB path exists but cannot be opened due to permissions or corruption, diagnostics identify the affected path without exposing sensitive file contents.
- If the DB is valid but contains no matching facts, the response is `ok` or `partial` with no-result diagnostics, not `unavailable`.
**Out of Scope:**
- Implementing DB rebuild or repair inside `vela explore`.

### REQ-006: Freshness and warming states are reported in every explore result
**Description:** Explore must expose query-time freshness so agents know when to trust the graph, read files directly, wait, or run update/build commands.
**Acceptance Criteria:**
- Every explore result includes exactly one freshness state: `fresh`, `pending`, `warming`, `stale`, or `unavailable`.
- `fresh` means graph state is considered trustworthy for known indexed files.
- `pending` means known file events or pending update state may affect the answer; the response names affected files/scopes when known and tells agents when direct reads are appropriate.
- `stale` means known drift may affect the answer; the response recommends `vela update` or `vela build` and names stale files/scopes when known.
- `warming` means MCP connect-time catch-up or initial freshness work is running; the response may include partial/no graph answer and must tell the agent to retry or inspect status.
- `unavailable` means the runtime DB cannot be used; no graph-backed answer is presented as truth.
**Edge Cases:**
- If MCP starts and the DB is already fresh, the first `vela_explore` call returns `fresh`, not `warming`.
- If MCP starts catch-up and cannot finish before the first query, the first `vela_explore` call returns `warming` with partial/no graph answer rather than blocking indefinitely.
**Out of Scope:**
- Implementing watcher/debounce/auto-sync that creates `pending` states from live file events.

### REQ-007: Planner classifies intent and routes over existing primitives
**Description:** `vela explore` must interpret the user's question into an intent family and route to existing deterministic graph primitives before considering any future semantic retrieval layer.
**Acceptance Criteria:**
- Supported intent families are `explain`, `usage`, `dependency`, `path`, `impact`, `area_survey`, `workspace_domain`, `contract`, `memory`, and `unknown`.
- Explain prompts route to lookup/explain-style primitives.
- Usage/reverse-dependency prompts route to reverse-dependency or `who uses` primitives.
- Dependency/callee prompts route to dependency/callee primitives.
- Path/flow prompts route to path primitives with resolved endpoints.
- Impact prompts route to impact or bounded reverse-reachability primitives.
- Area survey prompts use deterministic candidate resolution and graph relevance over existing facts; they do not present free-text matching alone as proof.
- Workspace/domain prompts use workspace routing evidence when available and label it separately from code truth.
- Contract prompts use contract evidence when available and label it separately from code truth.
- Memory prompts include memory evidence only when the query intent asks about decisions, history, why, previous work, or equivalent prior-context language.
**Edge Cases:**
- Low-confidence intent detection returns `ambiguous` or `partial` with clarification candidates.
- Missing primitives or missing graph families produce limits/diagnostics rather than scope expansion.
**Out of Scope:**
- Implementing personalized PageRank, embeddings, LLM retrieval, SCIP parsing, or new extractors in this slice.

### REQ-008: Ambiguity and confidence are visible, not hidden
**Description:** Explore must make ambiguity, unresolved labels, and confidence limits explicit so agents do not over-trust weak routing.
**Acceptance Criteria:**
- Each result includes interpreted intent confidence of `high`, `medium`, or `low`.
- Ambiguous subject resolution returns candidate labels with distinguishing metadata when available.
- Low-confidence routing includes a clarification prompt or suggested exact follow-up queries.
- A strong answer is not returned when required endpoints or subjects are unresolved.
- Confidence and limits explain missing graph families, stale files, unavailable snippets, or unsupported relation types relevant to the answer.
**Edge Cases:**
- Multiple candidates with the same display name must be disambiguated by file path, repository, symbol kind, graph family, or stable ID when available.
- If no candidates exist, the result gives no-result diagnostics and suggested lookup/build/status actions as appropriate.
**Out of Scope:**
- Perfect natural-language understanding.

### REQ-009: Layered evidence labels preserve graph-family boundaries
**Description:** Explore output must preserve Vela's layered graph model instead of flattening all evidence into code facts.
**Acceptance Criteria:**
- Evidence items are labeled with one graph family: `repo_code`, `workspace`, `contract`, `memory`, or `resource`.
- Repo/code evidence includes symbols, files, calls, imports, dependencies, routes, or equivalent code graph facts.
- Workspace evidence is presented as routing/topology/ownership context, not deep code truth.
- Contract evidence is presented as public-interface or behavior-contract context, not inferred executable code truth.
- Memory evidence is presented as prior decision/history context, not repo graph truth.
- Missing relevant graph families are represented as `unavailable`, `not_configured`, or `not_requested` rather than silently omitted when the question makes them relevant.
**Edge Cases:**
- Conflicting evidence across families must be preserved with confidence/limits instead of silently merged.
- Resource graph evidence may be represented as unavailable/future seam if not implemented.
**Out of Scope:**
- Completing the Resource Graph implementation.

### REQ-010: Memory evidence is opt-in by query intent
**Description:** Explore must include memory evidence only when the user's query asks for prior decisions, history, rationale, previous work, or equivalent memory-aware context.
**Acceptance Criteria:**
- Normal structural prompts such as “explain X”, “who uses X”, or “how does A reach B” do not include memory evidence by default.
- Memory-aware prompts such as “what did we decide about X”, “why did we choose X”, or “previous work on X” include memory evidence when available.
- Memory evidence, when included, is labeled as `memory` and separated from repo/code, workspace, and contract evidence.
- If memory is requested but unavailable, the response states that memory evidence is unavailable or not configured.
**Edge Cases:**
- Queries containing “why” about a call path may require both graph evidence and memory evidence; each must be labeled separately.
- Memory evidence must not override contradicting current repo/code graph facts.
**Out of Scope:**
- Making memory retrieval mandatory for all explore queries.
- Merging memory observations into the runtime repo graph truth.

### REQ-011: Agent instructions prefer `vela_explore` first while preserving fallbacks
**Description:** Phase 1 agent instructions must steer coding agents to start with `vela_explore` for structural questions while preserving raw grep/read and low-level tool fallbacks for appropriate cases.
**Acceptance Criteria:**
- MCP initialize/instructions or equivalent agent-facing text states: for structural, architectural, flow, dependency, ownership, or impact questions, call `vela_explore` first.
- Instructions tell agents to treat returned source snippets and graph paths as already-read evidence.
- Instructions preserve raw grep/read for exact text lookup, stale files named by Vela, unavailable/unindexed projects, or verification of latest source.
- Instructions do not ask agents to memorize low-level structural query syntax as the default workflow.
- Instructions do not promise watcher/debounce/auto-sync behavior in the Phase 1 shell.
**Edge Cases:**
- If graph state is `warming`, `stale`, `pending`, or `unavailable`, instructions permit direct reads of named files or relevant setup commands.
- Maintainer/debugging docs may still mention specialized commands.
**Out of Scope:**
- Rewriting every repository document that mentions older Vela workflows during this slice.

### REQ-012: Backward compatibility preserves specialized commands and tools
**Description:** `vela explore` becomes the default agent entrypoint, not the only graph capability.
**Acceptance Criteria:**
- Existing low-level CLI commands and query forms remain available for humans, tests, debugging, and exact operations.
- Existing low-level MCP tools remain available unless separately deprecated in a future approved spec.
- Explore may call existing primitives internally but must not change their documented behavior as part of this slice.
- Existing specialized tests should not need to migrate to `vela explore` unless they are testing the new explore surface.
**Edge Cases:**
- If explore wraps a primitive that returns a diagnostic, explore preserves or translates the diagnostic without hiding it.
- If a low-level command supports a JSON-only fixture/test path, that path must not become a runtime-query fallback for `vela explore`.
**Out of Scope:**
- Deprecating or removing specialized graph commands.

### REQ-013: TUI exposes an agent integration installer wizard
**Description:** The TUI must provide a first-class guided path for installing Vela MCP and instructions into supported coding agents.
**Acceptance Criteria:**
- The main TUI menu includes `Install Agent Integration` with copy that explains it configures Vela MCP for coding agents.
- Selecting the menu item opens a wizard instead of requiring users to discover CLI flags manually.
- The wizard defaults to the current project and can present tracked project context when available.
- The wizard lists OpenCode and Claude Code as MVP supported targets and shows unsupported detected agents as guidance-only, not installable.
- The wizard displays the config path that will be touched for the selected target before writing.
**Edge Cases:**
- If no supported agent config is detected, the wizard still explains how to install by choosing a target/path manually in a later release and points to the equivalent CLI command.
- Canceling from the wizard returns to the main menu without writing files.
**Out of Scope:**
- Cursor, Codex, Gemini, or other unsupported agent installation.
- A global daemon or watcher setup.

### REQ-014: TUI installer previews and confirms writes before installation
**Description:** The wizard must show a dry-run preview and require explicit confirmation before modifying agent config or instruction files.
**Acceptance Criteria:**
- The preview lists the project path, selected agent, MCP config file, instruction file, and graph DB path.
- The preview states that unrelated config is preserved and reruns are idempotent.
- Confirmation runs the same backend installer used by `vela install`; the TUI must not duplicate OpenCode or Claude config writing logic.
- Canceling at preview exits without writing files.
**Edge Cases:**
- If the backend installer returns an error, the wizard shows an actionable error and does not claim success.
- If `.vela/graph.db` is missing, the install path initializes or reports the graph DB through the shared installer behavior.
**Out of Scope:**
- Editing arbitrary custom agent config formats.

### REQ-015: TUI installer verifies the completed agent setup
**Description:** After confirmation, the wizard must show a concise verification summary that the user can act on immediately.
**Acceptance Criteria:**
- Success output says MCP config was written.
- Success output says agent instructions mention `vela_explore` first for structural questions.
- Success output says `.vela/graph.db` exists or gives actionable `vela build`, `vela update`, or `vela status` guidance.
- Success output includes a suggested local smoke prompt for the coding agent.
- Re-running the same wizard target updates Vela-managed files without duplicate entries.
**Edge Cases:**
- If verification is partial, the wizard labels missing pieces as warnings rather than hiding them.
- Existing unrelated config in the target directory is preserved.
**Out of Scope:**
- Launching or restarting the external coding agent process.

## Open Questions
- None for the Phase 0 + Phase 1 implementation contract plus the TUI agent installer wizard slice. Later-phase details such as watcher state storage, debounce timing, benchmark corpus, relation weights, Rust/JVM extractor strategy, SCIP/LSP ingestion, unsupported agent installers, and auto-sync implementation remain intentionally out of scope for this slice.

## Open Assumptions
- The initial CLI renderer can be human-readable by default while MCP returns structured content from the same envelope.
- Any optional CLI JSON rendering, if implemented, must mirror the shared envelope and must not become a separate contract.
- Existing freshness/status primitives are sufficient to report `fresh`, `stale`, `unavailable`, and MCP `warming`; live `pending` from file events may be reported only if existing state exists, because watcher/debounce implementation is deferred.
- The first TDD implementation can add only the minimum planner/router needed for the accepted scenarios and does not need full graph relevance ranking.
- The TUI wizard can start with keyboard-only Bubble Tea navigation and does not need a graphical diff viewer as long as the preview lists files and actions before writes.

## Out of Scope
- Watcher/debounce/auto-sync implementation.
- Global daemon behavior.
- Benchmark harness implementation or CodeGraph comparison execution.
- New language extractor work, including Rust, Java, or Kotlin extractors.
- SCIP/LSP parsing or compiler-backed semantic graph facts.
- LLM or embedding-based graph truth.
- Resource Graph completion.
- Replacing or removing existing low-level commands and MCP tools.
- Installing unsupported coding agents beyond OpenCode and Claude Code.
- Restarting or controlling external coding-agent processes.
- Production or test code changes during this spec pass.

## Trade-offs
- Requiring `.vela/graph.db` can make JSON-only or partially initialized projects fail earlier, but the failure is safer than serving stale/generated artifacts as runtime truth.
- Returning `warming` instead of blocking MCP first queries may give agents partial/no graph answers at connect time, but it avoids hangs and makes catch-up state explicit.
- Memory opt-in reduces surprise and noise, but agents must phrase prior-work questions clearly to receive memory evidence.
- Keeping low-level commands increases surface area, but preserves maintainer workflows and lets `explore` reuse proven primitives incrementally.
- Deferring watcher/debounce means Phase 1 does not fully solve active-session freshness, but it keeps the first implementation slice reviewable and anchored on the response contract.
- Adding a TUI wizard increases surface area, but it removes a practical adoption blocker: users should not have to hand-edit MCP config to use `vela_explore`.

## Risk Level
medium — Justification: The slice avoids destructive operations and watcher complexity, but it changes the primary agent-facing contract, introduces runtime DB-required failure behavior, and adds local agent config writes that must be previewed, confirmed, idempotent, and bounded to Vela-managed files.
