# Ancora and Vela Implementation Plan

## Goal

Implement the approved boundary so Ancora remains the canonical memory layer and Vela becomes the graph extraction and retrieval engine with a clean user-facing MCP experience.

## Tasks

### 1. Build a real stdio MCP server in Vela

- Replace or complement the current HTTP-only `vela serve` flow with a real stdio MCP server.
- Expose only `vela_*` graph and retrieval tools.
- Do not expose any memory write tools.

### 2. Define and implement the Vela MCP tool handlers

- Implement these first-class tools:
  - `vela_query_graph`
  - `vela_shortest_path`
  - `vela_get_node`
  - `vela_get_neighbors`
  - `vela_graph_stats`
  - `vela_explain_graph`
  - `vela_federated_search`
- Map each tool to an existing Vela retrieval/query path where possible.

### 3. Make Ancora detect and forward Vela tools

- Add optional Vela integration inside Ancora.
- When Vela is available, Ancora can expose forwarded `vela_*` tools on the primary MCP surface.
- Forwarding must call Vela internally and return results without changing tool ownership or names.

### 4. Keep memory ownership strict in code

- Audit Vela setup, MCP, and CLI paths to ensure Vela does not write canonical memory.
- If Vela needs memory context, it must read from Ancora APIs or indexed views only.
- Reject any duplicate tools such as `vela_save_memory` or `vela_recall_memory`.

### 5. Implement transparent install modes

- `Ancora only`: install only Ancora MCP and memory tools.
- `Vela only`: install only Vela MCP and graph/retrieval tools.
- `Ancora + Vela`: install Ancora as the primary MCP surface and enable forwarded `vela_*` tools when Vela is detected.

### 6. Update setup and docs together

- Update setup flows so config matches real transport and tool behavior.
- Remove any setup path that registers Vela as MCP before a real stdio MCP exists.
- Document the three install modes and the visible tool surface for each one.

### 7. Verify with end-to-end integration tests

- Test `Ancora only`, `Vela only`, and `Ancora + Vela` setups.
- Verify tool visibility, correct forwarding, and absence of duplicate memory writes.
- Verify Vela still works without Ancora, but only as graph retrieval.

## Order

Build in this order:

1. Vela stdio MCP server
2. Vela tool handlers
3. Ancora forwarding layer
4. Setup/install logic
5. Integration tests
6. Documentation cleanup

## Done Criteria

- Vela exposes a real stdio MCP server with `vela_*` tools.
- Ancora remains the only canonical memory write surface.
- Combined install exposes one primary MCP surface without duplicate memory tools.
- All three install modes work as documented.
