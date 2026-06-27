# PRD: Vela Agent Explore and Runtime Parity

**Project**: Vela  
**Repo**: `github.com/Syfra3/vela`  
**Status**: Draft for review  
**Primary user**: AI coding agents using Vela through MCP or CLI  
**Secondary users**: Vela maintainers, local developers, architecture reviewers  
**Related docs**:

- `docs/VELA_ARCHITECTURE.md`
- `docs/GRAPH_DOMAIN_SCHEMAS.md`
- `docs/ORGANIZATION_MAP_V1_SPEC.md`
- `docs/VELA_V2_GRAPH_SYNC_AND_SEARCH_PLAN.md`
- `docs/BENCHMARK_DRIVEN_IMPROVEMENT_LOOP_PRD.md`
- `openspec/prd.md`

## 1. Decision

Vela should close the practical UX/runtime gaps exposed by CodeGraph without becoming a code-only graph clone.

The product move is:

1. Add one simple agent-facing exploration surface: `vela explore` / `vela_explore`.
2. Make the SQLite graph DB the trusted runtime source for all agent queries.
3. Add auto-sync and explicit freshness states so agents can trust, warn on, or reject graph answers.
4. Preserve Vela's moat: layered evidence across repo/code, workspace, contract, memory, and future resource graphs.

Vela should become an **agent-native system graph**: CodeGraph-like ergonomics plus Vela's broader system context.

## 2. Why this PRD exists

The previous comparison showed that CodeGraph currently wins on agent ergonomics:

- one strong MCP tool;
- source, call paths, and impact radius in one response;
- local SQLite runtime graph;
- auto-sync and stale-result warnings;
- explicit instructions that prevent agents from starting with blind grep/read loops.

Vela has stronger long-term architecture, but its current agent surface is harder to use:

- agents must know valid structural query forms;
- broad questions require manual discovery before graph queries;
- graph freshness is not yet productized as a runtime contract;
- MCP/CLI output is not yet shaped as a single surgical context packet;
- the broader graph families are valuable but not consistently visible in one response.

This PRD defines the product work needed to close those gaps.

## 3. Review of the first draft: improvements added

The initial draft had the right direction, but it needed sharper product boundaries. This version adds:

| Gap in first draft | Improvement in this PRD |
|---|---|
| Simple goal, but weak MVP boundary | Adds explicit MVP, later phases, and non-goals |
| Auto-sync mentioned, but not specified as a trust contract | Adds freshness states, query-time warnings, and missing-runtime diagnostics |
| `vela explore` named, but response contract too loose | Defines a stable explore response shape and evidence labels |
| Vela advantages named, but not protected by acceptance criteria | Adds graph-family evidence requirements and anti-clone constraints |
| Success metrics were broad | Adds measurable targets for tool calls, file reads, freshness, latency, and correctness |
| No rollout strategy | Adds phases, migration path, compatibility rules, and benchmark gates |
| No failure-mode handling | Adds risks for stale graphs, over-broad answers, source drift, MCP misuse, and background daemon complexity |

## 4. Problem statement

Vela asks agents to reason through the graph, but the current workflow still makes the agent do too much routing work.

Today an agent often has to:

1. discover concrete labels with normal file search;
2. choose between lookup/search/path/explain/impact-style operations;
3. format a structural query correctly;
4. separately inspect source, dependency paths, and architectural context;
5. infer whether the graph is fresh enough to trust.

That is fragile. Agents do not reliably follow multi-step graph protocols under context pressure. If Vela wants graph-first behavior, it must give agents a single safe starting point.

## 5. Product principle

> Agents should not need to understand Vela's internal query taxonomy before asking architectural questions.

Vela should expose advanced graph operations internally, but the default agent contract should be one high-level exploration tool.

## 6. Goals

- Make `vela explore` / `vela_explore` the default entrypoint for structural, architecture, flow, and impact questions.
- Return source snippets, graph paths, impact radius, and cross-layer evidence in one response.
- Keep graph freshness automatic during active agent sessions.
- Make stale or unavailable runtime graph state explicit and actionable.
- Preserve Vela's layered graph model instead of flattening all context into a code-only symbol graph.
- Reduce agent reliance on blind grep/read loops for indexed code paths.
- Provide benchmark evidence against CodeGraph-like and grep/read baselines.
- Define a clear multi-language roadmap that prioritizes Rust and Java/Kotlin because they are common in modern backend, platform, and mobile/enterprise codebases.

## 7. Non-goals

- Do not clone CodeGraph one-to-one.
- Do not remove specialized CLI commands used by humans or tests.
- Do not expose many narrow MCP tools as the primary agent surface.
- Do not silently fall back from `.vela/graph.db` to `.vela/graph.json` for runtime query truth.
- Do not mix memory observations directly into repo code graph truth.
- Do not claim CodeGraph-equivalent broad semantic language coverage until Vela has active extractors for those languages.
- Do not treat legacy LLM/embedding configuration as part of the current core repo graph build path.
- Do not claim SCIP/LSP-backed graph truth until SCIP/LSP artifacts are parsed into graph facts and wired into the build pipeline.
- Do not require a long-running global daemon for the MVP.
- Do not make Vela depend on a remote service.
- Do not treat grep/read as forbidden; they remain fallback tools for exact text lookup, stale files, or unindexed projects.

## 8. Users and jobs to be done

### Primary user: AI coding agent

Jobs:

- answer “how does this work?” without crawling files manually;
- inspect “how does A reach B?” with concrete paths;
- estimate “what breaks if X changes?” with impact radius;
- understand code in system context, not only local symbol context;
- know when the graph is fresh, stale, warming, or unavailable.

### Secondary user: local developer

Jobs:

- initialize Vela once and trust the graph during active work;
- run status/doctor commands when graph state is unhealthy;
- keep local-first behavior and privacy guarantees.

### Secondary user: Vela maintainer

Jobs:

- benchmark graph-first UX against grep/read and CodeGraph-like baselines;
- improve query quality without losing operational viability;
- preserve clean boundaries between graph families.

## 9. Current-state gap matrix

| Dimension | Current Vela gap | Target state |
|---|---|---|
| Agent entrypoint | Agents must choose/search/lookup/path/explain manually | One `vela explore` / `vela_explore` entrypoint |
| Query language | Current `vela search` requires exact structural forms | Natural-language prompt accepted; Vela plans/routes internally |
| Response shape | Source, path, impact, and context can be fragmented | One consistent surgical context packet |
| Freshness | Graph status exists, but query freshness is not a first-class runtime contract | Every query reports fresh/stale/pending/unavailable state |
| Auto-sync | Watch/update behavior exists as a direction, not a complete agent runtime promise | Session watcher plus debounced incremental update |
| Runtime truth | SQLite DB direction exists and must be completed | `.vela/graph.db` required for runtime queries |
| Vela advantage | Layered graph model exists but can be invisible to agents | Evidence labeled by repo, workspace, contract, memory, resource |
| Agent instruction | Agents can still start with blind grep or malformed graph queries | MCP/agent instructions say: use `vela_explore` first for structural questions |
| Benchmarking | Existing dep-eval focuses extraction/query quality | Add UX metrics: tool calls, file reads, answer completeness, freshness behavior |

### 9.1 Current technical reality to preserve

This PRD should compare Vela against CodeGraph without overclaiming Vela's current runtime.

| Dimension | CodeGraph direction to learn from | Current Vela reality | PRD implication |
|---|---|---|---|
| Main extraction tool | `web-tree-sitter` with WASM grammars | Go uses `go/ast`; TypeScript/JavaScript and Python use `go-tree-sitter` | Vela should keep Go-native precision for Go and add languages incrementally rather than rewrite extraction only for parser breadth |
| Language coverage | Broad multi-language parser story | Active semantic extractors are mainly Go, TypeScript/JavaScript, and Python | Do not claim broad semantic coverage; distinguish active graph extraction from tech detection or ignored-file support |
| Algorithm UX | Callers/callees, impact radius, deterministic graph relevance / PageRank-style ranking | Dijkstra shortest path, BFS-style reachability, Leiden/NetworkX clustering, lexical lookup scoring, degree metrics | Add graph relevance and caller/callee packaging to `vela explore`; keep existing path/reachability/community algorithms as primitives |
| LLM/embeddings | Not required for core graph creation | Legacy config/interfaces exist, but they are not active in the current core build path | Remove LLM/embedding claims from MVP; future semantic retrieval must be separate evidence, not repo graph truth |
| SCIP/LSP | No confirmed compiler/LSP extraction path for CodeGraph | SCIP drivers exist for Go/TS/Python, but current drivers do not parse SCIP output into graph facts | Treat SCIP as a future enrichment path after parser/ingestion work, not as current graph truth |

### 9.2 Current extraction coverage

Vela should not claim broad CodeGraph-equivalent semantic language coverage yet.

Current active repo/code extraction is:

- **Go**: parsed with Go `go/ast`; extracts functions, methods, structs, interfaces, calls, and import-derived file dependencies.
- **TypeScript/JavaScript**: parsed with `go-tree-sitter`; extracts functions, methods, classes, interfaces, calls, and import-derived file dependencies.
- **Python**: parsed with `go-tree-sitter`; extracts functions, classes, calls, and import-derived file dependencies.

Vela may detect or ignore additional ecosystems, but those capabilities must not be presented as semantic graph extraction until nodes and edges are built from those languages.

### 9.3 Priority multi-language roadmap

Rust and Java/Kotlin compatibility are important product goals because they cover high-value codebases that agents commonly need to inspect:

- **Rust**: systems, infrastructure, CLIs, crypto, performance-sensitive services, and modern backend components.
- **Java/Kotlin**: enterprise backends, Spring services, Android/mobile code, JVM platform teams, and large monorepos.

Vela should add language support by explicit evidence level instead of using a single vague “supported” label.

| Evidence level | Meaning | Agent behavior |
|---|---|---|
| Full semantic graph | files, symbols, imports, calls, type-like declarations, and enough relationships for impact/path queries | Safe for `vela explore`, impact, path, callers/callees, and architecture answers |
| Syntax graph | files, symbols, declarations, imports, and partial calls; limited type resolution | Useful for navigation and likely relationships; report confidence limits |
| File/dependency graph | files, package manifests, module dependencies, import edges, ownership | Useful for repo/package impact and routing; not enough for precise call paths |
| Detected only | language/framework detected but no graph facts extracted | Show as unavailable for semantic answers; suggest fallback reads or extractor setup |

Priority roadmap:

| Priority | Language family | Target evidence level | Rationale |
|---|---|---|---|
| P0 | Go, TypeScript/JavaScript, Python | Full semantic graph for current extractor scope | Preserve and improve current active coverage before broadening claims |
| P1 | Rust | Syntax graph first, then full semantic graph for functions, structs/enums/traits, modules, imports, and call-like relationships | Popular in infra/system tooling and increasingly common in agent/codebase benchmarks |
| P1 | Java/Kotlin | Syntax graph first, then full semantic graph for classes, interfaces, methods, annotations, packages, imports, and call-like relationships | High popularity in enterprise, Spring backends, Android, and JVM monorepos |
| P2 | C# | Syntax/file graph first | Common enterprise/backend language after JVM ecosystems |
| P2 | Ruby/PHP | File/dependency graph, then syntax graph | Valuable for web monoliths but lower priority than Rust/JVM for the first expansion |
| P3 | Swift | File/dependency graph, then syntax graph | Useful for iOS, but precise graph value may need deeper compiler/project-model integration |

Rust and Java/Kotlin should be planned as the first expansion after the `vela explore` runtime contract is stable. They should not block the MVP, but the PRD should reserve design space for them now:

- extractor interfaces must report language, evidence level, confidence, and unsupported edge types;
- `vela status` should show per-language extractor readiness;
- `vela explore` should include per-language limits when answering from syntax-only or file-only graphs;
- benchmarks should include at least one Rust fixture and one JVM fixture before declaring multi-language parity progress.

### 9.4 Algorithm direction for `vela explore`

The best CodeGraph idea to borrow is not the exact extraction stack. It is the **agent answer shape**: callers, callees, paths, impact, source evidence, and deterministic ranking in one response.

| Capability | Current Vela status | PRD direction |
|---|---|---|
| Caller/callee context | Available through call/dependency edges and dependency/reverse-dependency traversal, but not packaged into one explore answer | Make caller/callee neighborhoods a default `vela explore` section for symbol questions |
| Impact radius | Partially available; some paths use broad path checks, while structured impact can be limited to direct incoming facts | Implement bounded reverse reachability with distance, relation type, file/symbol grouping, confidence, and output caps |
| Path explanation | Available through Dijkstra shortest path | Add relation-aware weighting and optional alternate paths for ambiguous architecture questions |
| Graph relevance ranking | Not currently present beyond lexical lookup, workspace token scoring, and degree metrics | Add deterministic personalized PageRank or similar seeded graph ranking for broad explore prompts |
| Community detection | Leiden/NetworkX clustering available during build | Use communities to group and summarize output, not as the primary ranking signal |
| Degree/health metrics | Available for graph diagnostics | Use for architecture review and follow-up suggestions, not default surgical answers |
| LLM/embedding search | Legacy code/config may exist, but it is not active in core graph creation | Keep out of MVP claims; any future semantic retrieval must be labeled as separate evidence |
| SCIP enrichment | Driver wrappers exist, but facts are not ingested | Treat as future semantic-enrichment work after SCIP parsing is implemented |

### 9.5 LLM, embedding, SCIP, and LSP boundaries

The active repo graph should be described as deterministic graph truth built from parsers, static file dependencies, workspace topology, and persisted graph facts.

- LLMs and embeddings are not part of the current core repo graph build path.
- Legacy LLM/embedding configuration should be removed from this PRD's MVP narrative.
- Future memory or semantic retrieval may use LLMs/embeddings, but only as a separate evidence layer with explicit provenance.
- SCIP driver wrappers can invoke external SCIP tools and cache artifacts, but the PRD must not claim SCIP-backed graph extraction until SCIP output is parsed into graph facts.
- No LSP/compiler-API graph path should be claimed for CodeGraph or Vela without separate verification.

## 10. Functional requirements

### FR-1: Single primary explore surface

Vela must expose one primary agent-facing exploration surface.

CLI:

```bash
vela explore "How does billing webhook reach refund creation?"
```

MCP:

```txt
vela_explore
```

Requirements:

- accepts natural-language structural and architectural questions;
- detects likely query intent internally;
- can route to lookup, search, explain, path, impact, or layer-aware retrieval without requiring the agent to pick the operation;
- returns a stable response shape;
- reports freshness state in every response;
- uses `.vela/graph.db` as runtime truth.

### FR-2: Query planner and router

`vela explore` must include an internal planner that classifies prompts into at least these intent families:

| Intent family | Example |
|---|---|
| Explain | “explain RefundService” |
| Usage/reverse dependency | “who uses RefundStatus?” |
| Dependency/callee | “what does WebhookHandler depend on?” |
| Path/flow | “how does StripeWebhook reach RefundService?” |
| Impact | “what breaks if RefundStatus changes?” |
| Area survey | “how does the refund workflow work?” |
| Workspace/domain routing | “which service owns refunds?” |
| Contract-aware query | “where is the refund API contract enforced?” |
| Memory-aware query | “what did we decide about Stripe vs Adyen refunds?” |

The planner must make a best effort with ambiguous prompts and return clarification candidates when confidence is low.

The planner must prefer deterministic graph primitives before any future semantic retrieval layer:

1. resolve concrete candidates with lookup/workspace routing;
2. collect caller/callee neighborhoods for symbol questions;
3. use bounded reverse reachability for impact questions;
4. use relation-aware shortest paths for flow questions;
5. rank broad results with deterministic graph relevance such as personalized PageRank seeded by resolved candidates;
6. use communities only to group or summarize results after primary evidence is selected.

### FR-3: Stable explore response contract

Every explore response must follow this shape:

```txt
Answer
Freshness
Relevant source
Paths and relationships
Impact radius
Layered evidence
Confidence and limits
Suggested next queries
```

Minimum fields:

| Field | Required content |
|---|---|
| Answer | Short direct answer to the user's question |
| Freshness | graph state: fresh, pending, stale, warming, or unavailable |
| Relevant source | file paths, symbols, line ranges or snippets when available |
| Paths and relationships | call/import/dependency/ownership paths used as evidence |
| Impact radius | likely affected symbols/files/tests/packages when relevant |
| Layered evidence | evidence grouped by graph family |
| Confidence and limits | ambiguity, missing graph families, stale files, unresolved labels |
| Suggested next queries | concrete follow-ups using exact discovered names |

### FR-4: Layered evidence must be preserved

Vela must label evidence by graph family instead of flattening everything into a code-only answer.

| Graph family | Evidence examples |
|---|---|
| Repo/Code Graph | symbols, files, calls, imports, dependencies, routes |
| Workspace Graph | repos, services, packages, bounded contexts, ownership |
| Contract Graph | OpenAPI/proto/spec declarations, public interfaces, behavior contracts |
| Memory Graph | prior decisions, bug fixes, design notes, session summaries |
| Resource Graph | databases, queues, infra resources, external systems; future seam |

MVP can support only the graph families currently available, but the response shape must not block later families.

### FR-5: Auto-sync during active use

Vela must keep the local graph fresh during active use.

MVP requirement:

- when the MCP server is active, watch indexed project files;
- debounce file events;
- run incremental update after the debounce window;
- expose pending/stale files to query responses;
- perform connect-time catch-up before first query when possible.

Later requirement:

- optional CLI daemon or `vela watch` mode for non-MCP workflows;
- optional git hooks for checkout/commit transitions;
- status/doctor integration.

### FR-6: Freshness states

Every runtime query must return one of these states:

| State | Meaning | Agent behavior |
|---|---|---|
| `fresh` | Graph matches known file state | Trust graph answer |
| `pending` | File events detected, debounce/update not complete | Use graph with warning; read pending files directly for exact latest code |
| `warming` | Connect-time catch-up or initial build is running | Wait or return partial answer with warning |
| `stale` | Known graph drift could affect answer | Warn loudly and suggest `vela update` or direct read |
| `unavailable` | `.vela/graph.db` missing or unreadable | Do not answer from graph; show build/update instructions |

Example warning:

```txt
⚠️ Freshness: pending
The graph may be stale for:
- internal/query/query.go changed 1.8s ago
- internal/mcp/mcp.go changed 1.2s ago

The structural answer is based on the last indexed graph. Read those files directly if exact latest source is required.
```

### FR-7: Runtime DB contract

Runtime query paths must require `.vela/graph.db`.

Requirements:

- no silent fallback to `.vela/graph.json` for runtime query truth;
- missing DB returns a clear diagnostic;
- diagnostic includes next actions:

```bash
vela build
vela update
vela status
```

### FR-8: Agent instructions

MCP and repo-local instructions must be simplified around the new tool.

Required instruction:

```txt
For structural, architectural, flow, dependency, ownership, or impact questions, call `vela_explore` first. Treat returned source snippets and graph paths as already read. Use raw grep/read only for exact text lookup, stale files named by Vela, or projects without a usable graph.
```

Specialized commands can remain documented for maintainers, but agents should not be asked to memorize structural query syntax as their default workflow.

### FR-9: Backward compatibility

Existing specialized commands should remain available for humans, tests, and debugging:

- `vela lookup`
- `vela search`
- path/explain/impact query forms
- build/update/status/watch commands

`vela explore` becomes the default agent surface, not the only graph capability.

### FR-10: Benchmark and evaluation loop

The feature must be validated with a benchmark that compares:

1. `vela_explore`;
2. current Vela search/query workflow;
3. CodeGraph where available;
4. grep/read baseline.

Measure:

- tool calls;
- file reads;
- wall-clock time;
- answer correctness;
- source/path/impact completeness;
- freshness/staleness behavior;
- graph-family evidence coverage.

## 11. Non-functional requirements

| Requirement | Target |
|---|---|
| Local-first | No remote service required |
| Privacy | Source and graph data stay local |
| Response latency | Fast enough for interactive agent use; target p95 under 3 seconds for already-indexed medium repos |
| Watch debounce | Configurable; default should avoid rebuild storms |
| Failure clarity | Missing/stale graph states must be actionable |
| Output budget | Explore output should be focused; avoid dumping whole files unless explicitly requested |
| Determinism | Same query and graph state should produce stable evidence ordering where possible |
| Extensibility | New graph families can be added without breaking the response contract |

## 12. MVP scope

The MVP should prove the agent UX/runtime contract without attempting the full long-term graph vision.

In scope:

- `vela explore` CLI command;
- MCP `vela_explore` tool;
- internal intent planner for common structural queries;
- deterministic caller/callee, path, impact, and graph-relevance orchestration over current graph facts;
- response shape with answer, freshness, source, paths, impact, evidence, limits;
- `.vela/graph.db` runtime validation;
- MCP-session file watcher with debounced update or pending/stale warnings;
- simplified MCP/agent instructions;
- benchmark harness updates for agent UX metrics.

Out of MVP scope:

- full global daemon;
- hosted graph service;
- complete Resource Graph implementation;
- perfect natural-language understanding;
- broad language expansion beyond current active extractors;
- LLM/embedding-based repo graph creation;
- SCIP/LSP-backed semantic graph facts unless a parser/ingestion stage is explicitly added;
- replacing every specialized CLI command;
- broad rewrite of extraction internals unless required for runtime DB correctness.

Multi-language note: Rust and Java/Kotlin are explicit post-MVP compatibility priorities. The MVP should not implement them unless doing so is cheaper than expected, but it must avoid response schemas, storage assumptions, or extractor interfaces that would make those languages hard to add next.

## 13. User stories and acceptance criteria

### US-001: Explore structural questions from one tool

As an AI coding agent, I want one Vela tool for structural questions so I do not have to choose the correct low-level graph operation.

Acceptance criteria:

- [ ] `vela explore "explain X"` returns a direct explanation when X is known.
- [ ] `vela explore "who uses X"` returns reverse dependency evidence.
- [ ] `vela explore "how does A reach B?"` returns path evidence or a clear no-path answer.
- [ ] `vela explore "what breaks if X changes?"` returns impact evidence.
- [ ] MCP exposes equivalent `vela_explore` behavior.

### US-002: Return surgical context in one response

As an agent, I want the answer, relevant source, paths, impact, and limits together so I do not need a file crawl before acting.

Acceptance criteria:

- [ ] Response includes source references with file paths and symbols.
- [ ] Response includes graph paths or relationships used as evidence.
- [ ] Response includes impact radius when relevant.
- [ ] Response includes confidence/limits for ambiguous or partial results.
- [ ] Response includes suggested next queries using exact discovered labels.

### US-003: Preserve layered Vela evidence

As a Vela maintainer, I want explore output to preserve graph-family boundaries so Vela does not collapse into a code-only graph.

Acceptance criteria:

- [ ] Evidence is labeled by graph family.
- [ ] Repo/code graph evidence is separate from workspace evidence.
- [ ] Contract evidence is not treated as inferred code truth.
- [ ] Memory evidence is linked as prior context, not merged into code facts.
- [ ] Missing graph families are reported as unavailable rather than silently omitted when relevant.

### US-004: Trust freshness at query time

As an agent, I want every Vela answer to say whether the graph is fresh so I know when direct file reads are necessary.

Acceptance criteria:

- [ ] Query response reports `fresh`, `pending`, `warming`, `stale`, or `unavailable`.
- [ ] Pending/stale responses name affected files when known.
- [ ] Missing `.vela/graph.db` blocks graph answers and gives build/update/status instructions.
- [ ] Stale/pending responses tell the agent when to read files directly.

### US-005: Auto-sync during active agent sessions

As a developer, I want the graph to update while agents edit files so I do not have to manually run sync after every change.

Acceptance criteria:

- [ ] MCP server watches indexed source files during active sessions.
- [ ] File events are debounced.
- [ ] Incremental update runs through the standard build/update path.
- [ ] Ignored files do not trigger rebuilds.
- [ ] Query responses expose pending updates during the debounce window.

### US-006: Improve agent instructions

As a maintainer, I want Vela instructions to steer agents into `vela_explore` first so graph-first usage is reliable.

Acceptance criteria:

- [ ] MCP initialize/instructions mention `vela_explore` as the primary structural tool.
- [ ] Repo-local agent docs no longer force agents to memorize low-level query forms for normal use.
- [ ] Instructions preserve grep/read as fallback for exact strings, stale files, and unindexed projects.
- [ ] Existing low-level examples move to a maintainer/debugging section.

### US-007: Validate against CodeGraph-like UX metrics

As a reviewer, I want empirical evidence that the new surface reduces discovery overhead.

Acceptance criteria:

- [ ] Benchmark includes architecture questions on at least one Vela repo/corpus.
- [ ] Benchmark records tool calls, file reads, time, answer quality, and evidence coverage.
- [ ] `vela_explore` beats current Vela workflow on agent tool-call and file-read count.
- [ ] `vela_explore` does not regress query correctness versus existing specialized query commands.

### US-008: Plan popular multi-language compatibility

As a Vela maintainer, I want Rust and Java/Kotlin compatibility planned explicitly so Vela can support popular real-world backend, platform, and mobile codebases after the explore MVP.

Acceptance criteria:

- [ ] The PRD distinguishes full semantic, syntax, file/dependency, and detected-only support.
- [ ] Rust is listed as a P1 language expansion target.
- [ ] Java/Kotlin are listed as P1 language expansion targets.
- [ ] Extractor outputs include language, evidence level, confidence, and unsupported edge types.
- [ ] `vela explore` can report language-specific limits when a graph is syntax-only or file/dependency-only.
- [ ] Future benchmark planning includes at least one Rust fixture and one Java/Kotlin fixture.

## 14. Success metrics

| Metric | Target |
|---|---|
| Agent tool calls | 50%+ reduction versus current grep/read-heavy workflow for indexed architecture questions |
| File reads | Near-zero for fresh indexed code paths, excluding stale-file fallback reads |
| First-tool correctness | Agents choose `vela_explore` first for structural questions in guided tests |
| Query correctness | No regression versus existing Vela specialized query commands |
| Freshness reporting | 100% of explore responses include freshness state |
| Missing DB behavior | 100% of missing-runtime cases produce actionable diagnostics |
| Layered evidence | Responses include graph-family labels whenever non-code evidence participates |
| Watch behavior | Edits trigger pending state quickly and update after debounce without manual command |
| Multi-language readiness | Extractor contracts and response limits support Rust and Java/Kotlin as P1 post-MVP additions |

## 15. Rollout plan

### Phase 0: Contract design

Outputs:

- finalized `vela explore` response schema;
- intent families and routing rules;
- algorithm plan for caller/callee neighborhoods, bounded impact, relation-aware paths, and graph relevance ranking;
- explicit extraction coverage statement for Go, TypeScript/JavaScript, and Python;
- explicit multi-language expansion plan with Rust and Java/Kotlin as P1 post-MVP targets;
- explicit boundary statement for LLM/embeddings and SCIP/LSP;
- freshness state model;
- benchmark scenario list.

Exit criteria:

- maintainers approve the response contract and MVP boundary.

### Phase 1: Explore CLI and MCP shell

Outputs:

- `vela explore` command;
- `vela_explore` MCP tool;
- stable response shape;
- routing to existing query engine where possible.

Exit criteria:

- structural prompts work through one surface without requiring exact low-level syntax.

### Phase 2: Runtime DB and freshness contract

Outputs:

- `.vela/graph.db` runtime enforcement across explore;
- freshness state reporting;
- missing/stale diagnostics.

Exit criteria:

- no silent JSON fallback in runtime query paths;
- all explore responses include freshness.

### Phase 3: Auto-sync MVP

Outputs:

- MCP-session watcher;
- debounce and pending-file tracking;
- connect-time catch-up;
- query-time stale/pending warnings.

Exit criteria:

- edits made during an MCP session become visible to Vela or trigger explicit stale warnings.

### Phase 4: Layered evidence integration

Outputs:

- repo/code evidence in explore response;
- workspace/domain evidence where available;
- memory/contract evidence hooks where available;
- labels for unavailable graph families.

Exit criteria:

- explore output demonstrates Vela's system-graph advantage on representative questions.

### Phase 5: Benchmark and instruction cutover

Outputs:

- updated agent instructions;
- UX benchmark report;
- comparison against current Vela and grep/read baseline;
- optional CodeGraph comparison if installed.

Exit criteria:

- benchmark shows reduced discovery overhead without correctness regression.

### Phase 6: Rust and JVM compatibility planning

Outputs:

- Rust extractor design with target evidence level, supported node/edge types, and known limits;
- Java/Kotlin extractor design with target evidence level, supported node/edge types, and known limits;
- per-language status output and `vela explore` limit reporting;
- fixture benchmark plan for Rust and Java/Kotlin repos.

Exit criteria:

- maintainers approve the extractor contract and benchmark fixtures before implementation begins.

## 16. Risks and mitigations

| Risk | Failure mode | Mitigation |
|---|---|---|
| Over-broad explore output | Agent receives too much context and loses focus | Use ranked snippets, caps, and suggested follow-ups |
| Bad intent routing | Natural prompt maps to wrong operation | Show interpreted intent and confidence; ask for clarification when low confidence |
| Stale graph trust bug | Agent acts on outdated graph facts | Query-time freshness state; stale file list; direct-read fallback guidance |
| Watch complexity | Background runtime becomes flaky or resource-heavy | MVP watcher only during MCP session; optional daemon later |
| Layer collapse | Memory/contracts/workspace become mixed with code truth | Evidence labels and provenance boundaries are mandatory |
| Regression of low-level tools | Existing tests/maintainer workflows break | Keep specialized commands; route explore through existing primitives |
| Benchmark gaming | Lower file reads but worse answer quality | Gate on correctness and evidence quality, not only tool count |
| CodeGraph clone trap | Vela loses its differentiated system-graph direction | Acceptance criteria require graph-family evidence and layered boundaries |

## 17. Product decisions to confirm

1. **Name**: default to `vela explore` and MCP `vela_explore` unless maintainers prefer `vela ask`.
2. **Auto-sync MVP**: start with MCP-session watcher, not a global daemon.
3. **Stale behavior**: warn rather than block for `pending`; block graph answers for `unavailable`.
4. **Specialized commands**: keep them for humans/tests; demote them in agent instructions.
5. **Layered evidence**: response contract must support all graph families even if MVP populates only some.
6. **Algorithm priority**: ship caller/callee packaging, bounded impact, relation-aware paths, and deterministic graph relevance before considering embeddings.
7. **Extraction honesty**: market current semantic graph support as Go, TypeScript/JavaScript, and Python until more extractors are active.
8. **SCIP/LSP boundary**: keep SCIP as a future enrichment path unless its artifacts are parsed into graph facts.
9. **Multi-language priority**: Rust and Java/Kotlin are P1 post-MVP compatibility targets because of popularity and coverage of infra/backend/mobile/enterprise repos.

## 18. Open questions

1. Should `vela explore` return machine-readable JSON by default for MCP and human-readable Markdown for CLI?
2. What is the exact line/snippet budget per response before Vela should ask a follow-up?
3. Should connect-time catch-up block the first query, or return `warming` with partial results?
4. Should file watcher state live in `.vela/` or only in process memory for the MCP-session MVP?
5. Which corpus should be the first CodeGraph comparison target: `vela`, `stock-chef`, or a smaller fixture repo?
6. Should memory evidence be opt-in for privacy/noise control, or enabled automatically when query intent mentions decisions/history?
7. Should personalized PageRank be implemented in the runtime query layer, persisted during build, or both?
8. Which relation weights should `vela explore` use for paths: calls over imports, same-file over cross-file, high-confidence facts over inferred facts?
9. Should SCIP ingestion be a separate milestone after `vela explore`, or folded into extraction correctness work only when benchmarks show parser gaps?
10. Should Rust use tree-sitter first, `rust-analyzer`/compiler metadata later, or both behind separate evidence levels?
11. Should Java/Kotlin start with tree-sitter syntax extraction, language-server/compiler metadata, or build-tool-aware extraction from Maven/Gradle projects?
12. Should Rust and Java/Kotlin compatibility ship as one multi-language milestone or as separate reviewable slices?

## 19. Recommended next action

Approve Phase 0 and turn this PRD into an implementation spec.

The first implementation spec should define:

- exact `vela explore` input/output schema;
- mapping from prompt intent to existing query engine calls;
- freshness state calculation;
- MCP tool contract;
- minimal benchmark scenario set.

Do not start with auto-sync implementation before the explore response contract is stable. A fresh graph is useful only if the agent-facing answer shape is reliable.
