# Vela Lookup and Ambiguity Handling Spec

## Status

Implemented for `vela lookup <term>`.

Future work remains for ambiguity-aware fuzzy fallback in structural queries and shorthand aliases such as `vela explain <node>`.

## Problem

`vela search` is a structural graph query tool, but users and agents keep treating it like keyword search.

That causes two predictable failures:

1. broad feature prompts get passed directly to `vela search`
2. generic node names such as `movement` or `transaction` get guessed without disambiguation

The current CLI surface overloads one command with two different jobs:

- discovery: finding likely nodes to inspect
- structural reasoning: dependencies, impact, explanation, and paths

Those jobs should be separated.

## Product Goal

Keep `vela search` trustworthy as a structural graph query surface, while adding a dedicated discovery command that helps humans and agents find the right node before they query it.

## Proposal

Add a new command:

```bash
vela lookup <term>
```

Purpose:

- return likely node candidates for a broad term
- support safe discovery before structural graph queries
- never pretend to answer dependency or path questions directly

Keep `vela search` structural-only.

## Command Surface

### `vela lookup <term>`

Examples:

```bash
vela lookup transaction
vela lookup movement status
vela lookup card contract
vela lookup transaction.mapper
```

Expected behavior:

- search graph labels, canonical keys, and file-path-like nodes
- return ranked candidates
- include node label, node kind, and light provenance metadata
- do not silently choose one candidate when ambiguity is high

Example output:

```text
Candidates for "transaction":

1. Transaction [repo/interface]
   id: repo:interface:Transaction
   evidence: declared

2. src/commons/mappers/transaction.mapper.ts [repo/file]
   id: repo:file:src/commons/mappers/transaction.mapper.ts
   evidence: filesystem

3. TransactionMapper [repo/class]
   id: repo:class:TransactionMapper
   evidence: declared

Next steps:
- vela search "explain Transaction"
- vela search "who uses Transaction"
- vela search "explain src/commons/mappers/transaction.mapper.ts"
```

### Future sugar commands

Once lookup exists, the CLI can safely grow simpler structural aliases:

```bash
vela explain <node>
vela uses <node>
vela deps <node>
vela impact <node>
vela path <a> <b>
```

These are UX improvements only. They should map to the same underlying structural query engine as `vela search`.

## Non-Goals

- Do not turn `vela search` into generic free-text retrieval.
- Do not silently fuzzy-resolve generic terms to arbitrary nodes.
- Do not merge discovery ranking with graph-truth reasoning in one opaque result.

## Lookup Ranking Rules

`vela lookup` should rank candidates using lightweight lexical and structural hints.

Suggested ranking order:

1. exact ID match
2. exact canonical key match
3. exact label match
4. case-insensitive exact label match
5. path suffix match
6. token overlap / partial lexical match

Tie-breakers:

- prefer declared symbols over inferred artifacts
- prefer current nodes over stale or redirected references
- prefer non-test nodes over tests when scores are similar
- prefer file-path matches only when the query looks path-like

## Ambiguity Rules

If multiple candidates are close in score, `lookup` must return candidates instead of choosing one.

If future fuzzy support is added to `vela search`, it must follow the same rule:

- if confidence is high, allow direct resolution
- if confidence is low or multiple candidates are close, return an ambiguity response

Example ambiguity response:

```text
Ambiguous subject "transaction". Candidates:

1. Transaction [repo/interface]
2. TransactionMapper [repo/class]
3. src/commons/mappers/transaction.mapper.ts [repo/file]

Try one of:
- vela search "explain Transaction"
- vela search "who uses Transaction"
- vela search "explain src/commons/mappers/transaction.mapper.ts"
```

## Agent Workflow Contract

Agents should follow this flow:

1. broad feature prompt arrives
2. use discovery to find candidate repos/modules/types/files
3. use `vela lookup` for ambiguous subjects
4. run structural graph queries on exact labels
5. produce plan from producer, contract, consumer, and risk

Example:

```text
User: add movement status to the mobile app

Agent flow:
1. discover likely repos and contracts
2. vela lookup "movement status"
3. vela lookup "transaction"
4. vela search "impact of MovementStatusDto"
5. vela search "who uses Transaction"
6. vela search "path TransactionMapper -> MovementStatusDto"
```

## Readme and Agent Docs

When `vela lookup` lands, docs should teach this split explicitly:

- `lookup` = discovery
- `search` = structural graph reasoning

Repo-local agent instructions should say:

- never pass bag-of-words directly to `vela search`
- use `vela lookup` when the exact node is not yet known
- use `search` only with exact subjects or path endpoints

## Implementation Notes

Likely ownership:

- candidate generation and ranking: `internal/retrieval`
- CLI command wiring: `cmd/vela`
- output rendering: existing text rendering patterns in query/CLI surfaces

Suggested initial implementation:

1. implement `vela lookup` against graph labels, canonical keys, and paths only
2. return top N candidates with ids and kinds
3. add explicit ambiguity responses for `search` when low-confidence fuzzy matching is attempted
4. later add command aliases such as `vela explain`, `vela uses`, and `vela deps`

## Acceptance Criteria

1. `vela lookup transaction` returns a ranked candidate list instead of failing.
2. `vela lookup movement status` returns candidate nodes when exact labels are unknown.
3. `vela search` remains structural-only and does not accept free-text feature descriptions.
4. ambiguous subjects do not silently resolve to arbitrary nodes.
5. docs clearly distinguish discovery from structural graph reasoning.

## Why This Is Better Than Blind Fuzzy Search

Blind fuzzy search feels convenient, but it breaks trust.

If Vela silently maps `transaction` to the wrong node, the user gets graph-shaped nonsense with false confidence.

`lookup` preserves trust because it says:

- here are the likely candidates
- now choose the exact thing you mean

That is the right contract for a graph-truth tool.
