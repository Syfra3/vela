# Ancora and Vela Integration Contract

## Purpose

Define the user-facing integration model after the ownership contract.

## Product Roles

- Ancora is the long-term memory persistence layer.
- Vela is the extraction, graph-building, and retrieval layer.

## Primary Rule

There must be only one primary MCP surface active for the user in a given setup.

This avoids duplicate tools, duplicate writes, and agent confusion.

## Install Modes

### 1. Ancora only

- Primary MCP: `ancora`
- Exposes: memory save, memory search, memory recall
- No Vela graph retrieval tools

### 2. Vela only

- Primary MCP: `vela`
- Exposes: extraction, graph retrieval, path search, graph explanation
- No durable memory write tools

### 3. Ancora + Vela

- Primary MCP: `ancora`
- Ancora remains the user-facing memory surface
- Vela runs as a local retrieval engine behind the scenes
- Graph retrieval tools may be exposed through the primary surface, but canonical memory tools remain Ancora-owned

## Tool Boundary

### Ancora tools

- Save memory
- Search memory
- Recall sessions, decisions, and observations

### Vela tools

- Extract graph from repos or content
- Query graph structure
- Find paths and relationships
- Fuse graph context with memory context at query time

## Hard Rules

1. Ancora never delegates canonical memory ownership.
2. Vela never exposes a second canonical memory write path.
3. When both are installed, the user should not need to choose between two memory tools.
4. When both are installed, graph retrieval may use Vela internally, but memory writes still resolve to Ancora.

## User Experience Goal

The system should feel simple:

- install Ancora for memory
- install Vela for graph extraction and retrieval
- install both for combined retrieval without duplicate ownership

## Decision

Use Ancora as the primary user-facing MCP when both systems are installed.

Use Vela as the standalone MCP only when Vela is installed without Ancora.
