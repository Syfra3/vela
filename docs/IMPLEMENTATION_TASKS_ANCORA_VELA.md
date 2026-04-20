# Vela Implementation Tasks for Ancora Integration

## Goal

Make Vela a real stdio MCP server for graph extraction and retrieval, without taking ownership of durable memory.

## Tasks

### 1. Replace fake MCP registration with a real MCP transport

- Update `cmd/vela/main.go` so `vela serve` starts a real stdio MCP server.
- Keep the current HTTP server only if it remains a separate explicit mode.
- Update `internal/server/server.go` only if parts of the current query handlers can be reused.

## 2. Add the Vela MCP tool surface

- Register these tools in the MCP server:
  - `vela_extract_graph`
  - `vela_query_graph`
  - `vela_shortest_path`
  - `vela_get_node`
  - `vela_get_neighbors`
  - `vela_graph_stats`
  - `vela_explain_graph`
  - `vela_federated_search`
- Put MCP-specific registration and handler glue in a dedicated package such as `internal/mcp/`.

## 3. Map tools onto existing Vela query paths

- Reuse existing graph/query logic from:
  - `internal/query/`
  - `internal/export/`
  - `internal/retrieval/`
  - `internal/server/` where useful
- Keep the MCP layer thin and move shared logic below it if needed.

## 4. Enforce the no-memory-write rule in Vela

- Audit CLI, server, and MCP code to ensure Vela does not expose canonical memory writes.
- Do not add tools such as `vela_save_memory`, `vela_update_memory`, or `vela_recall_memory`.
- If Vela reads memory context, treat it as read-only retrieval input.

## 5. Fix setup flow to match reality

- Update `internal/setup/wizard.go` and `internal/setup/mcp.go` so Vela MCP registration matches the actual stdio server command.
- Remove any setup messaging that implies MCP support before the stdio server exists.

## 6. Add contract tests

- Add tests for tool registration and handler behavior.
- Add tests proving Vela exposes only `vela_*` retrieval tools.
- Add tests proving Vela still works without Ancora.

## Primary Files

- `cmd/vela/main.go`
- `internal/mcp/` new package
- `internal/server/server.go`
- `internal/query/`
- `internal/setup/wizard.go`
- `internal/setup/mcp.go`

## Done Criteria

- `vela serve` is a real stdio MCP server.
- Only approved `vela_*` tools are exposed.
- No memory-write overlap exists.
- Setup/config registers a real working Vela MCP entry.
