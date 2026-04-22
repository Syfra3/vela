# Vela Architecture — Graph-Truth Query Model

Vela is a local-first graph builder centered on code-truth facts. The active
product keeps only the layers needed for deterministic build and query flows.

## Topology

```
organization -> workspace -> repo -> subsystem -> node/chunk
```

- **organization** — the outermost container (optional, used when multiple
  workspaces coexist).
- **workspace** — a named grouping of repos, services, and dependencies used
  for routing.
- **repo** — a single code repository; the correctness boundary for deep
  retrieval.
- **subsystem** — a coarse grouping inside a repo (module, package cluster,
  product area).
- **node/chunk** — the smallest retrievable unit: a file, symbol, or chunked
  slice of content.

## Layers and Responsibilities

| Layer     | Responsibility                                                                                   | Primary Evidence                    |
| --------- | ------------------------------------------------------------------------------------------------ | ----------------------------------- |
| Repo      | Repo-local code, files, symbols, and chunks. Deep retrieval correctness path.                    | `extracted` from code (AST, imports) |
| Contract  | Declared service and interface truth from artifacts (OpenAPI, proto, manifests).                 | `declared` from artifacts           |
| Workspace | Lightweight routing over repos, services, domains, packages, and dependencies.                   | `inferred` routing metadata          |

### Invariants

1. **Declared beats derived.** Contract evidence outranks workspace and
   repo-inferred signals during fusion.
2. **Workspace is routing truth, not code truth.** Workspace edges must not
   be treated as deep code relationships.
3. **Identity is not layer-local.** Each cross-layer join goes through the
   identity resolver rather than each layer inventing its own keys.
4. **Evidence is typed.** Every cross-layer edge carries `Evidence` describing
   the source artifact and confidence, so ranking and explainability stay
   principled.

## Shared Types

The architecture-facing types that stabilize these contracts live in
`pkg/types/architecture.go`:

- `Layer` — `repo`, `contract`, `workspace`, `memory` (memory remains a legacy
  type but is no longer part of the active product surface).
- `CanonicalKey` — layer-aware identity (`<layer>:<kind>:<key>`) produced by
  the identity resolver.
- `Evidence` — typed provenance carrying layer, evidence type, source
  artifact, confidence, and verification state.
- `Confidence` — `declared` > `extracted` > `inferred` > `ambiguous`.
- `VerificationState` — `current`, `redirected`, `stale`, `ambiguous` for
  memory-reference binding health.
- `RoutingMetadata` — workspace-layer routing hints (repos, services,
  domains, packages, dependencies).
- Topology node types — `NodeTypeRepo`, `NodeTypeSubsystem`,
  `NodeTypeChunk`, `NodeTypeService`, `NodeTypeContract`, plus the existing
  `NodeTypeWorkspace`, `NodeTypeOrganization`, `NodeTypeObservation`, etc.

## Integration Surfaces

- `pkg/types` — shared contracts for all layers.
- `internal/graph` — graph assembly and topology wiring.
- `internal/query` — query engine and graph-truth query entrypoints.
- `internal/extract` — layer-agnostic extraction that tags outputs with source
   and layer metadata.
- `internal/tui` — classic local UI for extraction, graph status, and query.

## Retrieval Flow (target behavior)

1. Classify the query scope (repo-local, workspace-wide, contract-focused).
2. Use the workspace layer to pick candidate repos and services.
3. Run graph-truth queries against the persisted graph.
4. Join contract evidence where declared artifacts exist.
5. Return results with per-layer provenance.

Route first, retrieve deeply second.
