---
name: vela-graph-query
description: >
  Plans and executes Vela graph queries with the correct structural syntax.
  Trigger: when using `vela search`, tracing dependencies/impact/paths, or turning a broad feature question into exact Vela queries.
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## When to Use

- The user wants to use `vela search` or `vela lookup`.
- The task is graph-oriented: ownership, dependencies, impact, reverse dependencies, or path tracing.
- The user asks a broad feature question and the agent must translate it into valid Vela queries.
- A coding agent is about to mix Vela with `grep`, `read`, or targeted bash commands.

## Critical Patterns

- Treat Vela as a structural graph tool, not as grep and not as semantic search.
- Never send bag-of-words or feature descriptions directly to `vela search`.
- If the exact node name is unknown, start with discovery or `vela lookup`.
- Use `vela search` only with valid structural forms.
- After graph results, switch to `grep` and `read` for semantic gaps, runtime behavior, and sparse graph coverage.

## Query Ladder

1. Start from the user goal.
2. Extract concrete candidates: repo names, file names, DTOs, types, services, handlers, components.
3. If the exact node label is unknown, run `vela lookup "term"`.
4. Pick the most specific exact node label or ID.
5. Convert the goal into one of the allowed structural queries.
6. Run `vela search`.
7. If graph coverage is thin, fall back to `grep` and `read` to verify behavior.

## Allowed `vela search` Forms

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

If the planned query does not match one of those forms, do not run `vela search` yet.

## Translation Rules

Broad question -> Vela-ready query sequence

- "Where does movement status come from?"
  - `vela lookup "Movement"`
  - `vela lookup "Activity"`
  - `vela search "who uses MovementCardRowProps"`
  - `vela search "explain ActivityController"`
  - `vela search "what does ActivityService depend on"`

- "What breaks if we change a DTO?"
  - `vela lookup "ActivityFormattedResponseDto"`
  - `vela search "impact of ActivityFormattedResponseDto"`

- "How does controller output reach a DTO?"
  - `vela search "path ActivityController -> ActivityFormattedResponseDto"`

## Good vs Bad

Bad:

- `vela search "movement status dto mapping screen pending approved rejected"`
- `vela search "wallet wrapper activity transactions"`
- `vela search "billing"`

Good:

- `vela lookup "MovementCards"`
- `vela lookup "ActivityFormatted"`
- `vela search "who uses MovementCardRowProps"`
- `vela search "what does ActivityService depend on"`
- `vela search "path ActivityController -> ActivityFormattedResponseDto"`

## Workflow With Other Tools

Use this order:

1. `vela lookup` for exact symbols when needed.
2. `vela search` for graph truth.
3. `grep` for string-based discovery and endpoint usage.
4. `read` for final proof in source files.

Use `grep` first instead of Vela when the question is mainly textual or behavioral, for example:

- searching raw endpoint strings like `/activity`
- finding enum values or status literals
- locating hooks, screens, or config files by text
- confirming UI policy implemented in code branches

## Agent Output Contract

When using this skill, the agent should report:

1. The user goal in one line.
2. The exact Vela node candidates discovered.
3. The structural queries chosen.
4. What the graph proved.
5. What still required `grep` or `read` verification.

## Commands

```bash
vela lookup "Movement"
vela search "who uses MovementCardRowProps" --graph "/path/to/.vela/graph.json"
vela search "what does ActivityService depend on" --graph "/path/to/.vela/graph.json"
vela search "path ActivityController -> ActivityFormattedResponseDto" --graph "/path/to/.vela/graph.json"
```
