# Vela Agent Instructions

## Purpose

Vela exposes graph-truth dependency queries over `.vela/graph.json`. Treat it as a structural graph tool, not as free-text or keyword search.

## Hard Rules

- Do not send bag-of-words or feature descriptions directly to `vela search`.
- Do not use `vela search` as a grep replacement.
- Do not guess generic node names like `movement`, `transaction`, `service`, or `handler` unless the exact node label is already known.
- If the user asks a broad product or feature question, discover concrete files, symbols, types, DTOs, or module names first.
- If the exact node is still unclear after discovery, use `vela lookup <term>` before structural graph queries.

## Valid `vela search` Forms

Use only these structural forms:

- `who uses X`
- `what uses X`
- `where is X used?`
- `what does X depend on`
- `dependencies of X`
- `impact of X`
- `what breaks if X changes?`
- `path A -> B`
- `path from A to B`
- `how does A reach B?`
- `explain X`

If the query does not fit one of those forms, do not call `vela search` yet.

## Required Workflow

1. Start broad questions with discovery, not structural search.
2. Find the real node candidates first: file paths, interfaces, DTOs, types, services, handlers, or modules.
3. Pick the most specific exact label or ID available.
4. Use `vela lookup <term>` when the exact node is still ambiguous.
5. Run `vela search` only after you have a concrete subject or path endpoints.
6. If the subject is ambiguous even after lookup, list candidates or ask a clarifying question instead of guessing.

## Routing Guidance

Use `vela search` first only for clearly structural questions such as:

- "who uses `TransactionMapper`?"
- "what does `WalletController` depend on?"
- "what breaks if `MovementStatus` changes?"
- "path `MovementController` -> `MovementStatusDto`"
- "explain `Transaction`"

Do not use `vela search` first for prompts like:

- "add movement status to the mobile app"
- "print the movement extract"
- "change the card contract"

For those prompts, first identify the concrete repos/modules/contracts involved, then query the graph with exact node names.

## Good vs Bad Examples

Bad:

- `vela search "movement transaction status dto contract enum mapping"`
- `vela search "billing"`
- `vela search "movement list transaction status render module api client"`

Good:

- `vela lookup "transaction"`
- `vela search "explain Transaction"`
- `vela search "who uses Transaction"`
- `vela search "impact of MovementStatusDto"`
- `vela search "path MovementController -> TransactionMapper"`

## Expected Behavior For Feature Planning

When the user asks for a feature plan, the agent should:

1. Discover the likely producer and consumer repos/modules.
2. Identify the contract or DTO types involved.
3. Use `vela lookup` if the exact node labels are still unclear.
4. Use `vela search` on those exact nodes to inspect dependencies, impact, and paths.
5. Produce a plan that separates producer changes, contract changes, consumer changes, and rollout risks.
