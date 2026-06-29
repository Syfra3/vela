# Hard Spec: Vela MCP Stock-Chef Graph Correctness — Slice A+B+C

## Adversarial Pre-Mortem
- Failure mode 1: MCP status/explore selects a stale or wrong graph artifact while the CLI status path selects the fresh stock-chef graph, causing agents to recommend unnecessary rebuilds or trust the wrong corpus.
- Failure mode 2: MCP registers tools with a duplicated server prefix such as `vela_vela_explain`, breaking client expectations and masking the intended raw MCP local tool names such as `explain`.
- Failure mode 3: MCP answers silently merge facts from multiple stock-chef corpora, especially a real active workspace and a benchmark or dependency-evaluation corpus with the same display name.

## Hidden Assumptions
- The CLI status implementation already has the authoritative semantics for selecting graph source and freshness for a concrete workspace.
- The MCP server can observe or be configured with the active workspace/corpus root used by a real coding-agent session.
- Multiple corpora can share the same human display name, so repo name alone is not a safe routing key.
- MCP tool names are part of the external client contract and must remain stable once exposed.
- It is safer to return an explicit ambiguity/unavailable diagnostic than to synthesize a mixed-corpus graph answer.

## Alternatives Considered
| Approach | Reason Rejected |
|----------|----------------|
| Keep MCP source selection separate from CLI status | Rejected because it already produced contradictory freshness/source answers for stock-chef; one semantic source is required. |
| Always recommend `vela build` when MCP sees any stale stock-chef corpus | Rejected because guidance must be conditional on the selected active graph, not unrelated corpora. |
| Accept duplicated MCP tool names for backward compatibility | Rejected because duplicated server prefixes are client-visible defects; compatibility belongs to aliases only if explicitly supported later. |
| Merge all corpora with the same repo/display name | Rejected because it creates false evidence and makes benchmark corpora indistinguishable from the active workspace. |
| Solve broad architecture/explain quality in the same slice | Rejected because this slice is limited to MCP graph source correctness, tool naming, and workspace/corpus scoping. |

## Summary
Vela will correct the MCP stock-chef graph contract for Slice A+B+C: MCP graph source and freshness must match CLI status semantics, stale/build guidance must be emitted only for the selected graph, exposed MCP tool names must not duplicate the server prefix, and MCP must scope answers to the active workspace/corpus for a real stock-chef session. When the active corpus cannot be selected unambiguously, MCP must return an explicit ambiguity diagnostic instead of mixing corpora.

## Requirements

### REQ-001: MCP selected graph source must match CLI status semantics
**Description:** For the same working directory/root and available graph artifacts, MCP must select the same graph source and freshness state that `vela status` would select.
**Acceptance Criteria:**
- Given a real stock-chef working directory with a fresh graph selected by CLI status, MCP reports the same selected graph source.
- MCP freshness state for the selected graph matches CLI status freshness state.
- MCP does not let another corpus with the same display name override the active workspace graph.
- MCP diagnostics name the selected graph source or root clearly enough to distinguish it from similarly named corpora.
**Edge Cases:**
- Missing, unreadable, or invalid selected graph artifacts return structured unavailable diagnostics rather than falling back to unrelated corpora.
- Multiple candidate graphs with the same display name require root/corpus disambiguation before an answer is trusted.
**Out of Scope:**
- Redesigning all CLI status output.
- Changing graph build/index formats beyond what source selection needs.

### REQ-002: MCP stale/build guidance must be conditional on selected graph freshness
**Description:** MCP must recommend build/update actions only when the graph selected for the active request is missing, stale, or unavailable.
**Acceptance Criteria:**
- MCP does not recommend `vela build` or `vela update` when the selected active stock-chef graph is fresh.
- MCP may mention stale unrelated corpora only as non-selected candidates, not as guidance for the active request.
- Stale guidance includes the selected graph root/source it applies to.
- Fresh responses avoid stale diagnostics inherited from non-selected corpora.
**Edge Cases:**
- If selected graph freshness cannot be determined, MCP returns an explicit freshness-unavailable diagnostic with status guidance.
- If CLI status would mark the selected graph stale, MCP emits equivalent stale guidance.
**Out of Scope:**
- Implementing file watching, debounce, or automatic graph rebuild.

### REQ-003: MCP exposed tool names must not duplicate the server prefix
**Description:** Raw MCP local tool names exposed by the Vela server must prefer unprefixed names and must not contain an accidental duplicated server prefix.
**Acceptance Criteria:**
- MCP tool listing exposes preferred raw MCP local tool names such as `explore`, `lookup`, `status`, `explain`, `impact`, and `path`.
- If a client displays tool names with the server name prefixed once, names such as `vela_explore` may appear in that client display, but duplicated names such as `vela_vela_explore` must not appear.
- MCP tool listing does not expose duplicated names such as `vela_vela_explain`, `vela_vela_impact`, or `vela_vela_path`.
- Tool-call dispatch accepts the canonical names shown in the listing.
- Tool instructions/examples refer to canonical names only.
**Edge Cases:**
- If compatibility aliases are ever introduced, they must not appear as the primary advertised names in this slice.
- Unknown duplicated tool names return normal unknown-tool diagnostics instead of silently dispatching to a different tool.
**Out of Scope:**
- Renaming the MCP server itself.
- Removing separately approved legacy aliases outside this slice.

### REQ-004: MCP must prefer active workspace graph for real stock-chef cwd/root
**Description:** When MCP runs in or is configured for a real stock-chef workspace, that active workspace graph must take precedence over benchmark, fixture, dependency-evaluation, or other same-name corpora.
**Acceptance Criteria:**
- Active working directory/root is part of MCP graph selection.
- A real stock-chef workspace graph is selected before same-name corpora outside that root.
- MCP answers identify the active workspace/corpus used for graph evidence.
- Same-name non-active corpora do not contribute graph facts to the active workspace answer.
**Edge Cases:**
- If the active root has no usable graph and another same-name graph exists elsewhere, MCP returns unavailable/ambiguous guidance rather than silently using the other graph.
- If root identity cannot be established, MCP reports ambiguity and asks for a root/corpus selection.
**Out of Scope:**
- TypeScript dependency extraction improvements for stock-chef internals.
- Broad multi-repo architecture synthesis.

### REQ-005: MCP must return explicit ambiguity instead of silently mixing corpora
**Description:** When more than one plausible corpus could satisfy an MCP graph request and the active corpus cannot be chosen safely, MCP must return an explicit ambiguous result with candidates.
**Acceptance Criteria:**
- Ambiguous same-name corpora produce a structured ambiguity status/diagnostic.
- Ambiguity diagnostics include candidate roots or stable corpus identifiers when available.
- MCP does not merge nodes, edges, snippets, freshness states, or diagnostics from multiple unresolved corpora into one answer.
- The response tells the caller how to disambiguate, such as by running from the target root or providing an explicit corpus/root.
**Edge Cases:**
- Candidates with identical display names must be distinguished by root path, graph path, corpus ID, or source type.
- If no usable candidates exist, MCP returns unavailable/no-graph diagnostics rather than ambiguity.
**Out of Scope:**
- Explain-answer pollution unrelated to corpus mixing.
- New broad architecture query synthesis.

## Open Questions
- None for Slice A+B+C. The accepted implementation scope is limited to MCP source/freshness parity with CLI status, canonical MCP tool naming, and active workspace/corpus scoping for stock-chef.

## Trade-offs
- Reusing CLI status semantics for MCP reduces divergent behavior but may require a shared selection path rather than MCP-local shortcuts.
- Rejecting ambiguous same-name corpora can produce fewer answers, but prevents false mixed-corpus evidence.
- Fixing advertised tool names can expose clients that depended on accidental duplicated names; this slice prioritizes the canonical MCP contract.
- Deferring broad explain/architecture and TypeScript extraction fixes keeps Slice A+B+C reviewable and focused on graph correctness.

## Risk Level
medium — Justification: The slice is behaviorally narrow and avoids production-code implementation during this spec pass, but it changes client-visible MCP tool names and graph-source selection semantics that agents rely on for correctness.
