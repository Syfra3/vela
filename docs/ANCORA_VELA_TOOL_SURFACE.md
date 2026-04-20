# Ancora and Vela Tool Surface

## Purpose

Define the exact user-facing tool boundary for Ancora-only, Vela-only, and combined setups.

## Principle

Expose tools by responsibility, not by overlapping implementation.

## Ancora-Owned Tools

These remain canonical and user-facing whenever Ancora is installed:

- `ancora_save`
- `ancora_search`
- `ancora_context`
- `ancora_get`
- `ancora_summarize`
- `ancora_update`
- `ancora_suggest_topic`
- `ancora_start`
- `ancora_end`
- `ancora_save_prompt`
- `ancora_capture`
- `ancora_timeline`
- `ancora_stats`

## Vela-Owned Tools

These belong to Vela and must never become canonical memory writes:

- `vela_extract_graph`
- `vela_query_graph`
- `vela_shortest_path`
- `vela_get_node`
- `vela_get_neighbors`
- `vela_graph_stats`
- `vela_explain_graph`
- `vela_federated_search`

## Combined Setup

When Ancora and Vela are both installed:

- Primary MCP surface is `ancora`
- Canonical memory tools stay under Ancora names
- Vela retrieval tools are exposed through the same primary surface as forwarded graph tools

## Forwarded Vela Tools Through Ancora

When both are installed, Ancora may expose these Vela-backed tools:

- `vela_query_graph`
- `vela_shortest_path`
- `vela_get_node`
- `vela_get_neighbors`
- `vela_graph_stats`
- `vela_explain_graph`
- `vela_federated_search`

Ancora forwards these calls to Vela internally. Ancora does not rebrand them as memory tools.

## Tools Not Allowed In Combined Setup

These must not exist as duplicate user-facing actions:

- `vela_save_memory`
- `vela_update_memory`
- `vela_recall_memory`
- any second save or recall tool that overlaps with Ancora ownership

## Install Behavior

### Ancora only

- Register only Ancora tools

### Vela only

- Register only Vela tools
- No durable memory write tools

### Ancora + Vela

- Register Ancora tools
- Register forwarded Vela graph tools on the same primary surface
- Do not require the user to choose between two memory systems

## Naming Rule

Memory tools use `ancora_*` names.

Graph and retrieval tools use `vela_*` names.

This keeps the boundary visible even when both are served through one primary MCP.

## Decision

In the combined setup, Ancora is the primary MCP surface and exposes both:

- canonical memory tools from Ancora
- graph retrieval tools powered by Vela
