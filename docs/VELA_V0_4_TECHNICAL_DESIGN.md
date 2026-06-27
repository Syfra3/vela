# Vela v0.4 Technical Design

Vela v0.4 makes `.vela/graph.db` the required runtime truth store and exposes one graph-backed query engine through CLI and MCP adapters. Natural language is allowed only as a resolver into exact graph nodes, workspace routes, interfaces, and structural traversals; final answers must be backed by graph facts with evidence, provenance, confidence, freshness, and diagnostics.

## Design Goals

| Goal | Decision |
| --- | --- |
| Runtime truth | SQLite is authoritative for query/runtime behavior. JSON is export/debug only. |
| Adapter parity | CLI and MCP share one core engine and one result schema. |
| Agent safety | Ambiguity, staleness, and missing evidence are explicit result states. |
| Interface truth | Dynamic Interface Evidence Layer normalizes declared, extracted, inferred, and ambiguous interface facts. |
| Workspace reasoning | `.vela/workspace.yaml` declares topology used for route-first, retrieve-deeply-second behavior. |

## Non-Goals

- Do not keep JSON-only runtime compatibility for v0.4.
- Do not add MCP-specific query logic that bypasses the core engine.
- Do not treat workspace topology as deep code truth.
- Do not present inferred or extracted interface facts as declared contracts.
- Do not answer broad free-text prompts without resolving concrete graph-backed subjects first.

## Architecture Overview

### Runtime Shape

```text
CLI commands          MCP tools
     |                   |
     v                   v
CLI adapter          MCP adapter
     |                   |
     +--------+----------+
              |
              v
      Core query engine
              |
              v
     SQLite graph runtime
              |
              v
  Repo Graph + Organization Map + Interface Evidence + Freshness Manifest
```

### Core Engine as Source of Behavior

The core engine owns all query semantics:

- graph availability checks
- freshness checks and warning attachment
- natural-language candidate resolution
- exact subject resolution
- structural traversals for explain, impact, and path
- workspace route selection
- evidence ranking and conflict diagnostics
- shared result construction

The engine reads from `.vela/graph.db` only. If `.vela/graph.db` is missing, incompatible, or unreadable, the engine returns a structured runtime-unavailable diagnostic. It must not fall back to `.vela/graph.json` for query truth.

### CLI Adapter

The CLI adapter validates command-line arguments, calls the core engine, renders human-readable output, and maps diagnostics to process exit status. CLI rendering may summarize fields but must not add facts that are absent from the core result.

Required command surface:

- `vela build`
- `vela update`
- `vela watch`
- `vela status`
- `vela lookup`
- `vela explore`
- `vela serve --mcp`
- structural explain, impact, and path commands or equivalent structural search forms

### MCP Adapter

The MCP adapter starts with `vela serve --mcp`, registers the required tools, validates structured arguments, calls the same core engine, and returns structured responses preserving the core result schema.

Required tool surface:

- `vela_explore`
- `vela_lookup`
- `vela_explain`
- `vela_impact`
- `vela_path`
- `vela_status`

OpenCode and Claude Code compatibility means a standard MCP client can start the server, list tools, call each tool, and receive structured JSON responses without client-specific server branches.

### Shared Result Schema

All query paths return the same conceptual core result before rendering:

| Field | Purpose |
| --- | --- |
| `schema_version` | Version of the result contract. |
| `query` | Query kind, raw input, normalized input, and options. |
| `status` | `ok`, `ambiguous`, `unresolved`, `degraded`, or `error`. |
| `resolved_subjects` | Exact graph nodes, routes, services, interfaces, files, or selected candidates. |
| `facts` | Nodes, edges, interface facts, workspace facts, and file facts used by the answer. |
| `paths` | Ordered graph paths for path queries and routed cross-repo answers. |
| `relationships` | Neighbor, dependency, impact, or ownership edges relevant to the query. |
| `evidence` | Provenance records backing facts and relationships. |
| `confidence` | Overall confidence plus per-fact confidence tiers. |
| `freshness` | Runtime graph freshness status and affected scopes/files. |
| `diagnostics` | Structured warnings/errors with codes and recommended next actions. |
| `ambiguity` | Candidate list and refinement guidance when resolution is not unique. |
| `render_hints` | Optional hints for CLI truncation, table grouping, or agent display. |

Result discipline:

- A result can include useful lower-confidence facts only when they are qualified.
- Multiple diagnostics are preserved together.
- Ambiguous results list candidates instead of choosing silently.
- Missing evidence is reported as unavailable, not implied.

## SQLite Runtime Schema

### Storage Principles

- `.vela/graph.db` is required for all runtime queries.
- `.vela/graph.json` is generated export/debug output only.
- `.vela/GRAPH_REPORT.md` is generated human-readable report output only.
- `.vela/manifest.json` records build inputs and freshness metadata.
- Build/update writes should preserve the last valid runtime graph until a replacement is complete.
- Schema changes use direct migration because v0.4 has no compatibility obligation to JSON-only runtime users.

### Required Tables

| Table | Purpose | Key fields |
| --- | --- | --- |
| `schema_meta` | Runtime schema identity and migration state. | `schema_version`, `created_at`, `updated_at`, `vela_version` |
| `workspaces` | Declared organization/workspace roots. | `id`, `organization_id`, `name`, `root_path`, `source_uri`, `source_hash` |
| `repositories` | Repo boundaries for deep code truth. | `id`, `workspace_id`, `name`, `root_path`, `language_summary`, `source_uri` |
| `services` | Declared or discovered service identities. | `id`, `workspace_id`, `repo_id`, `name`, `kind`, `confidence`, `source_id` |
| `files` | Tracked file inputs and source locations. | `id`, `repo_id`, `path`, `language`, `sha256`, `size`, `mtime_utc`, `status` |
| `nodes` | Canonical graph nodes across repo and organization domains. | `id`, `canonical_key`, `kind`, `label`, `repo_id`, `service_id`, `file_id`, `metadata_json` |
| `edges` | Canonical graph relationships. | `id`, `from_node_id`, `to_node_id`, `kind`, `layer`, `confidence`, `metadata_json` |
| `evidence` | Provenance for facts, nodes, edges, and interface facts. | `id`, `subject_type`, `subject_id`, `source_type`, `source_uri`, `source_span_json`, `confidence`, `claim_status` |
| `interface_facts` | Normalized provider output for interfaces and service relationships. | `id`, `provider`, `interface_kind`, `name`, `source_service_id`, `target_service_id`, `source_node_id`, `target_node_id`, `route`, `method`, `protocol`, `confidence`, `claim_status`, `metadata_json` |
| `workspace_facts` | Declared topology facts from `.vela/workspace.yaml`. | `id`, `workspace_id`, `fact_kind`, `subject_key`, `object_key`, `confidence`, `source_id`, `metadata_json` |
| `manifests` | Build-level freshness and compatibility metadata mirrored from `.vela/manifest.json`. | `id`, `workspace_id`, `graph_hash`, `built_at`, `extractor_fingerprint`, `status`, `manifest_version` |
| `manifest_files` | File-level freshness inputs for diffing. | `manifest_id`, `repo_id`, `path`, `sha256`, `size`, `mtime_utc`, `input_kind` |
| `diagnostic_events` | Optional persisted build/update diagnostics for status. | `id`, `manifest_id`, `code`, `severity`, `scope`, `message`, `created_at` |

### Required Indexes

| Index | Reason |
| --- | --- |
| Unique `nodes(canonical_key)` | Exact node lookup and stable identity. |
| `nodes(kind, label)` | Lookup candidate generation. |
| `nodes(repo_id, kind)` | Repo-scoped structural traversal. |
| `nodes(file_id)` | Source-location queries and stale file impact. |
| `edges(from_node_id, kind)` | Explain/dependency expansion. |
| `edges(to_node_id, kind)` | Reverse impact traversal. |
| `edges(from_node_id, to_node_id, kind)` | Duplicate prevention and path traversal. |
| `evidence(subject_type, subject_id)` | Attach proof metadata to results. |
| `evidence(source_uri)` | Trace facts back to artifacts. |
| `interface_facts(source_service_id, target_service_id)` | Cross-service paths and impact. |
| `interface_facts(name, protocol, route)` | Interface lookup and conflict detection. |
| `workspace_facts(workspace_id, fact_kind)` | Route-first query planning. |
| `manifest_files(repo_id, path)` | Freshness diffing. |
| `manifest_files(path, sha256)` | Change detection. |

### Node Model

Nodes use stable canonical keys from existing architecture guidance:

- Repo Graph identities: `repo:<repo-name>`, `repo:<repo-name>:file:<path>`, `repo:<repo-name>:symbol:<qualified-name>`, `repo:<repo-name>:chunk:<stable-id>`
- Organization Map identities: `organization:<name>`, `workspace:<name>`, `repository:<name>`, `service:<name>`, `interface:<name>`

Each node records its truth domain through `kind`, `repo_id`, `service_id`, and metadata. Repo Graph nodes represent code truth. Organization/workspace/service/interface nodes represent topology and declared/documented truth.

### Edge Model

Edges record structural relationships and are qualified by layer and confidence. Allowed edge families should follow the existing domain split:

- Repo Graph: `contains`, `defines`, `imports`, `calls`, `references`, `implements`, `depends_on`, `chunk_of`
- Organization Map: `contains`, `owns`, `calls`, `uses`, `exposes`, `publishes_to`, `consumes_from`, `documents`, `bridges`
- Interface bridges: graph edges derived from `interface_facts`, always preserving evidence and confidence

### Evidence Model

Evidence is attached to nodes, edges, interface facts, workspace facts, and build diagnostics. It answers four questions:

- What source supports this fact?
- How directly was the fact observed?
- Is it declared, extracted, inferred, ambiguous, or conflicting?
- Is the source fresh relative to the runtime graph?

Confidence order is:

1. `declared`
2. `declared_hint`
3. `extracted`
4. `inferred`
5. `ambiguous`

Conflicts do not delete lower-confidence evidence. They produce conflict diagnostics and preserve all provenance.

### Migration Stance

v0.4 should use direct migration:

- `vela build` creates `.vela/graph.db` as the required runtime store.
- Runtime query commands fail clearly when `.vela/graph.db` is unavailable.
- Existing JSON persistence can be used as an implementation input during migration work only if needed, but runtime query behavior must not read JSON as truth.
- Generated JSON and reports are updated by build/update/export flows, not query-only flows.

## Query Pipeline

### Common Pipeline

Every query follows the same high-level pipeline:

1. Validate adapter input.
2. Open `.vela/graph.db` and check schema compatibility.
3. Load manifest/freshness summary.
4. Resolve raw input into exact node, route, interface, file, or query plan candidates.
5. If resolution is ambiguous, return structured ambiguity unless the query explicitly allows candidate exploration.
6. Execute structural graph traversal against SQLite.
7. Attach evidence, confidence, provenance, freshness, and diagnostics.
8. Return a shared core result to the adapter.

### `lookup`

Purpose: resolve terms into exact graph subjects.

Inputs:

- term
- optional kind filters
- optional workspace/repo/service scope
- optional max candidates

Behavior:

- Searches canonical keys, labels, source paths, service names, interface names, and provider-specific aliases.
- Returns candidates with distinguishing fields: kind, canonical key, repo, service, file, source location, confidence, and evidence summary.
- Does not traverse deeply or produce impact claims.

Ambiguity:

- Multiple plausible candidates return `status: ambiguous`.
- No candidate returns `status: unresolved` with suggested lookup terms when possible.

### `explain`

Purpose: explain one resolved subject.

Behavior:

- Resolves the subject using lookup semantics if an exact ID is not provided.
- Returns the subject node, direct relationships, interface facts, workspace facts, and supporting evidence.
- Includes conflict diagnostics when different evidence sources disagree.

Examples of facts:

- file defines symbol
- symbol imports package
- service owns repository
- service exposes route
- interface fact links caller to provider

### `impact`

Purpose: identify what may be affected if a subject changes.

Behavior:

- Resolves the subject exactly.
- Traverses reverse dependencies and configured impact edge types.
- Separates direct code impact from workspace/interface impact.
- Marks cross-repo impact with the confidence of interface/workspace bridge evidence.

Safety:

- Ambiguous subjects do not produce a strong impact answer.
- Inferred interface bridges are reported as inferred, not declared blast radius.

### `path`

Purpose: find graph-backed paths between two exact subjects.

Behavior:

- Resolves both endpoints.
- Uses repo-local edges and cross-repo bridge edges from workspace/interface facts.
- Returns ordered paths with per-hop edge type, domain, confidence, and evidence.

Safety:

- If either endpoint is ambiguous, return endpoint-specific candidates.
- If the only bridge is inferred, the path is valid as an inferred path but not a declared contract path.

### `explore`

Purpose: accept broader natural language while preserving graph-truth behavior.

Behavior:

1. Extract candidate terms, scopes, service names, interface names, and structural intent from the prompt.
2. Route first using workspace facts when the query is broad or cross-service.
3. Run lookup against candidate nodes/routes/interfaces.
4. Build a structural plan such as explain, impact, path, or candidate map.
5. Execute only graph-backed traversals.
6. Return facts and candidates used; never present free-text matches as proof.

Natural-language resolution can use lexical matching only to propose candidates. It cannot be the final evidence for an answer.

### Ambiguous Result Shape

Ambiguity is a first-class result state:

| Field | Meaning |
| --- | --- |
| `status` | `ambiguous` |
| `ambiguity.reason` | Why the engine cannot choose safely. |
| `ambiguity.candidates[]` | Candidate subjects or routes. |
| `ambiguity.candidates[].distinguishers` | Repo, service, path, kind, source, confidence. |
| `diagnostics[]` | `AMBIGUOUS_SUBJECT`, `AMBIGUOUS_ROUTE`, or related codes. |
| `recommended_next_actions[]` | Use `vela lookup`, refine scope, pass exact canonical key, or select candidate. |

## Dynamic Interface Evidence Layer

### Provider Interface Concept

Providers inspect different source types and emit normalized `InterfaceFact` records. Providers do not write final claims directly into user-facing results. The evidence layer normalizes, deduplicates, ranks, and links provider facts into the graph.

Provider responsibilities:

- discover facts from one source family
- record provenance and extraction confidence
- mark partial facts as partial or ambiguous
- avoid promoting extracted/inferred facts to declared truth

### Required Providers

| Provider | Source | Default claim status |
| --- | --- | --- |
| `OpenAPIProvider` | OpenAPI/Swagger artifacts | `declared` |
| `ProtoProvider` | protobuf/gRPC artifacts | `declared` |
| `FrameworkRoutesProvider` | framework route definitions | `extracted` |
| `HttpClientProvider` | HTTP/gRPC/client calls in code | `extracted` or `inferred` |
| `ManifestProvider` | package/service manifests | `inferred` unless explicit relationship is declared |
| `WorkspaceHintsProvider` | `.vela/workspace.yaml` hints | `declared_hint` |

Future providers can add AsyncAPI, GraphQL schemas, IaC service discovery, queue/topic declarations, or service mesh config without changing query behavior.

### Normalized `InterfaceFact` Shape

Conceptual fields:

| Field | Meaning |
| --- | --- |
| `id` | Stable identity derived from normalized endpoints and source. |
| `provider` | Provider that emitted the fact. |
| `interface_kind` | `http`, `grpc`, `event`, `package`, `resource`, or provider-specific extension. |
| `name` | Human-readable interface or operation name when known. |
| `protocol` | `http`, `grpc`, `amqp`, `kafka`, etc. when known. |
| `method` | HTTP method, RPC method, event action, or equivalent. |
| `route` | Route/path/topic/resource identifier when known. |
| `source_service` | Calling/owning service if known. |
| `target_service` | Serving/target service if known. |
| `source_node` | Repo Graph node that produced the fact when known. |
| `target_node` | Repo Graph node that produced the target when known. |
| `source_artifact` | File, contract, manifest, or workspace config path. |
| `source_span` | Line/column/range when available. |
| `claim_status` | `declared`, `declared_hint`, `extracted`, `inferred`, `ambiguous`, or `conflict`. |
| `confidence` | Rankable confidence tier. |
| `completeness` | `complete`, `partial`, or `unknown`. |
| `metadata` | Provider-specific normalized details. |

### Evidence Ranking

Ranking is deterministic and visible:

1. Declared contracts: OpenAPI, Proto, future AsyncAPI.
2. Declared workspace hints: explicit topology and known links in `.vela/workspace.yaml`.
3. Extracted framework route facts.
4. Extracted client call facts with concrete target evidence.
5. Inferred client/manifest/package facts.
6. Ambiguous naming/path heuristics.

Declared contracts outrank declared hints for interface semantics. Declared hints can still outrank extracted facts for route selection because they are explicit workspace topology, not deep code behavior.

### Conflict Handling

When providers report overlapping or contradictory facts:

- Preserve all facts and evidence.
- Select the highest-ranked interpretation as the primary fact.
- Attach conflict diagnostics naming the lower-ranked contradictory evidence.
- Avoid merging incompatible fields silently.
- Mark partially extracted fields as partial instead of completing them from heuristics.

### Claim Discipline Rule

Vela must never present inferred facts as declared truth. User-facing and MCP results must distinguish:

- declared contract truth
- declared workspace routing hints
- extracted code facts
- inferred relationships
- ambiguous heuristics
- conflicting evidence

## Workspace Topology

### `.vela/workspace.yaml` Schema

Proposed conceptual schema:

```yaml
version: 1
organization:
  id: glim
  name: Glim
workspace:
  id: core-platform
  name: Core Platform
  root: .
repositories:
  - id: api
    name: glim-api
    path: services/api
    languages: [go]
    services: [api]
  - id: web
    name: glim-web
    path: apps/web
    languages: [typescript]
    services: [web]
services:
  - id: api
    name: API
    repository: api
    kind: backend
    interfaces: [api-http]
  - id: web
    name: Web
    repository: web
    kind: frontend
interfaces:
  - id: api-http
    name: API HTTP
    kind: http
    owner_service: api
    contract: contracts/openapi.yaml
contracts:
  - id: api-openapi
    kind: openapi
    path: contracts/openapi.yaml
    owner_service: api
links:
  - from: web
    to: api
    kind: calls
    via: api-http
    confidence: declared_hint
```

### Topology Model

The schema must support:

- organization identity
- workspace identity
- repositories
- services
- contracts/interfaces
- known links between services, repos, and interfaces
- source provenance for every declared topology fact

Workspace facts are routing/topology truth. They do not prove that a particular code path calls a particular route unless linked to code-derived interface facts.

### Route-First, Retrieve-Deeply-Second

Broad and cross-codebase queries follow this flow:

1. Resolve workspace, repo, service, and interface routes from declared topology.
2. Report route ambiguity if multiple routes match.
3. Retrieve deeply only inside selected repo/service graph scopes.
4. Bridge cross-repo traversals through interface facts and workspace links.
5. Preserve the boundary between topology facts and code facts in results.

### Multi-Repo Fixture Strategy

The v0.4 fixture suite should include a small workspace with:

- two or three repositories under one `.vela/workspace.yaml`
- one backend service with a declared OpenAPI or Proto contract
- one consumer service with extracted client calls
- one explicit workspace link
- one ambiguous service/interface name to prove candidate handling
- one repo missing a local graph to prove route-known/deep-graph-unavailable diagnostics

## Freshness Architecture

### Manifest Model

`.vela/manifest.json` is the portable freshness artifact. SQLite mirrors relevant manifest data for fast status and query warnings.

Required conceptual fields:

| Field | Meaning |
| --- | --- |
| `version` | Manifest schema version. |
| `workspace_root` | Normalized root path. |
| `workspace_config_hash` | Hash of `.vela/workspace.yaml` when present. |
| `graph_db_hash` | Hash/fingerprint of the built runtime graph. |
| `generated_at` | Build/update timestamp. |
| `extractor_fingerprint` | Version of extraction logic and provider configuration. |
| `schema_version` | Runtime DB schema version. |
| `repos[]` | Repo paths, roots, and file inventories. |
| `files[]` | Path, hash, size, mtime, language/input kind, and repo. |
| `providers[]` | Provider fingerprints and input artifacts. |
| `outputs[]` | Generated artifacts and hashes. |

Freshness-relevant inputs include source files, workspace config, declared contracts, manifests, provider config, and extractor/schema fingerprints.

### `build`

`vela build` creates a complete runtime graph:

1. Validate workspace config if present.
2. Detect repos and source inputs.
3. Extract Repo Graph facts.
4. Run interface evidence providers.
5. Normalize workspace and interface facts.
6. Assemble graph and evidence.
7. Write a new SQLite runtime graph safely.
8. Write manifest.
9. Generate report and optional JSON export.

If report or JSON export generation fails after SQLite succeeds, the build should report artifact failure without corrupting the runtime graph.

### `update`

`vela update` compares current inputs with the prior manifest:

- unchanged inputs reuse existing graph facts
- changed/new files trigger incremental extraction where safe
- deleted files prune graph-owned nodes and edges where safe
- changed workspace config, schema version, extractor fingerprint, or provider inputs can force full rebuild
- unsafe incremental states fall back to full rebuild

Updates should write to a temporary graph or transactionally staged state before replacing the active runtime graph.

### `watch`

`vela watch` observes freshness-relevant inputs with the same filtering as build/update, debounces bursts, and serializes rebuilds. If an update is running, further changes should collapse into one queued follow-up update instead of overlapping writes.

### `status`

`vela status` reports:

- `fresh`
- `stale`
- `missing`
- `incompatible`
- `unknown`

Status includes stale files/scopes when known, schema compatibility, manifest health, generated artifact state, and recommended next actions.

### MCP Startup Behavior

`vela serve --mcp` should:

1. Open or inspect `.vela/graph.db`.
2. Read `.vela/manifest.json` and compare current inputs.
3. If graph is fresh, start normally.
4. If safe update is possible, perform it before serving or report that it was performed.
5. If update is unsafe or fails, start only when it can safely serve stale-but-marked results; otherwise fail clearly or start status-only degraded mode.

### Stale Warnings in Tool Responses

Every core result includes freshness. If the graph is stale or unknown, relevant query responses include warnings such as:

- stale source files may affect this answer
- workspace topology changed after build
- interface provider inputs changed after build
- graph schema is incompatible
- freshness cannot be determined because manifest is missing or unreadable

### Safe Update and Fallback Rebuild

Safety rules:

- Prefer full rebuild over questionable incremental merge.
- Never mark a graph fresh after failed or interrupted update.
- Preserve the previous valid graph if replacement fails.
- Treat deleted, renamed, workspace config, and contract changes as high-risk incremental cases unless explicitly supported.

## MCP Design

### Startup Command

The canonical command is:

```text
vela serve --mcp
```

No alternative MCP runtime should own separate query behavior.

### Tools

| Tool | Purpose | Core operation |
| --- | --- | --- |
| `vela_explore` | Broad graph-backed exploration. | `explore` |
| `vela_lookup` | Candidate resolution. | `lookup` |
| `vela_explain` | Explain exact subject. | `explain` |
| `vela_impact` | Reverse dependency/blast-radius query. | `impact` |
| `vela_path` | Path between two subjects. | `path` |
| `vela_status` | Runtime graph and freshness status. | `status` |

### Conceptual Tool Schemas

`vela_lookup` input:

- `term` string, required
- `kind` string array, optional
- `scope` object, optional: workspace, repo, service
- `limit` integer, optional

`vela_explain` input:

- `subject` string, required
- `scope` object, optional
- `include_evidence` boolean, optional default true

`vela_impact` input:

- `subject` string, required
- `scope` object, optional
- `max_depth` integer, optional
- `include_inferred` boolean, optional default true but qualified

`vela_path` input:

- `from` string, required
- `to` string, required
- `scope` object, optional
- `max_paths` integer, optional

`vela_explore` input:

- `query` string, required
- `scope` object, optional
- `mode` optional enum: `route`, `explain`, `impact`, `path`, `auto`
- `limit` integer, optional

`vela_status` input:

- `scope` object, optional
- `include_files` boolean, optional

All tool outputs return the shared core result envelope. Validation failures return structured diagnostics naming invalid fields.

### OpenCode and Claude Code Compatibility

Compatibility requirements:

- standard MCP server startup over stdio or supported transport selected by existing CLI conventions
- stable tool names listed above
- JSON-schema-compatible input definitions
- structured JSON output, not text-only responses
- no adapter-specific business logic branches
- clear startup errors for missing graph or incompatible schema
- degraded/status-only behavior is explicit if chosen

## Testing and Acceptance Mapping

### Scenario Mapping

| Scenario | Design components |
| --- | --- |
| SCN-001 | Shared result schema, evidence table, freshness attachment, explain pipeline. |
| SCN-002 | Core diagnostics, unresolved status, no speculative answer rule. |
| SCN-003 | SQLite runtime requirement, missing graph diagnostic, no JSON fallback. |
| SCN-004 | Build flow, graph.db, manifest, generated artifact role. |
| SCN-005 | Core engine, shared result schema, CLI/MCP adapters. |
| SCN-006 | Explore resolver, candidate resolution, graph-backed structural plan. |
| SCN-007 | Ambiguous result shape, lookup candidates, refinement guidance. |
| SCN-008 | MCP command and required tool registration. |
| SCN-009 | MCP structured outputs and OpenCode/Claude Code compatibility. |
| SCN-010 | MCP startup freshness behavior and stale warnings. |
| SCN-011 | CLI command surface and structural query support. |
| SCN-012 | Dynamic Interface Evidence Layer providers and confidence tiers. |
| SCN-013 | Evidence ranking, conflict diagnostics, no silent merge. |
| SCN-014 | Workspace YAML schema, workspace facts, topology provenance. |
| SCN-015 | Workspace YAML validation diagnostics. |
| SCN-016 | Route-first, retrieve-deeply-second flow. |
| SCN-017 | Cross-repo path through interface bridge evidence. |
| SCN-018 | Manifest diffing and status stale file reporting. |
| SCN-019 | Safe update, fallback rebuild, corruption prevention. |
| SCN-020 | Watch debounce and serialized updates. |
| SCN-021 | Single-repo fixture and SQLite symbol/dependency queries. |
| SCN-022 | Multi-repo fixture and declared workspace routing. |
| SCN-023 | MCP fixture and tool call diagnostics. |
| SCN-024 | CLI/MCP equivalence fixture and expected core result. |
| SCN-025 | Real workspace smoke test. |
| SCN-026 | Multiple diagnostics in degraded results. |

### Fixture Plan

| Fixture | Purpose |
| --- | --- |
| Single repo | Build `.vela/graph.db`, query symbols, explain dependencies, prove no JSON fallback. |
| Multi repo | Validate `.vela/workspace.yaml`, route across repos/services, prove topology provenance. |
| Interface evidence | Include OpenAPI/Proto, framework routes, HTTP clients, manifests, workspace hints, naming ambiguity, and conflicts. |
| MCP | Start `vela serve --mcp`, list tools, call all tools, validate structured diagnostics. |
| Freshness | Change, delete, rename files; change workspace config and contract artifacts; prove status/update/watch behavior. |
| CLI/MCP equivalence | Run paired CLI/MCP calls and compare core result semantics. |
| Real workspace smoke | Maintainer-selected workspace proving build/update, status, CLI evidence answer, and MCP evidence answer. |

## Risks and Constraints

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Scope risk | v0.4 combines storage migration, MCP, freshness, workspace topology, and evidence providers. | Implement in TDD slices by SCN, keep adapters thin, defer non-required providers/features. |
| Performance risk | SQLite traversals, evidence joins, and cross-repo paths may be slow on large workspaces. | Add required indexes early, keep path depth bounded, fixture benchmark critical traversals, avoid loading full graph into memory for normal queries. |
| Extraction confidence risk | Providers may overclaim incomplete facts. | Normalize claim status, require provenance, preserve partial facts, rank evidence explicitly, test conflicts. |
| MCP setup risk | OpenCode and Claude Code may differ in MCP startup/tool expectations. | Use standard MCP schemas, keep transport conventional, test both clients or compatibility harnesses in fixtures. |
| Stale graph safety risk | Auto-update or failed update could corrupt graph or hide staleness. | Transactional/staged writes, previous-valid-graph preservation, explicit stale/unknown statuses, rebuild fallback. |
| Schema churn risk | Early SQLite schema changes can block TDD progress. | Keep schema versioned, use direct migration during v0.4, avoid compatibility layers until external users exist. |
| Ambiguity UX risk | Users may find ambiguity responses too cautious. | Include useful candidates, distinguishers, and next actions; allow exact canonical-key follow-up. |

## Implementation Order Recommendation

The implementation should proceed scenario-by-scenario with tests written first for each slice.

### Phase 1: Runtime Foundation

1. Define the SQLite schema and core result schema.
2. Implement `vela build` path to create `.vela/graph.db` and `.vela/manifest.json` for a single-repo fixture.
3. Enforce missing graph behavior and no JSON runtime fallback.
4. Implement basic `lookup` and `explain` through the core engine.

First scenarios:

- SCN-003: SQLite graph database is required for runtime queries.
- SCN-004: Build creates runtime and generated graph artifacts with SQLite as truth.
- SCN-001: Important answers include proof metadata when evidence is available.
- SCN-002: Vela refuses to invent an answer when no graph-backed fact exists.
- SCN-021: Single-repo fixture proves SQLite graph build and symbol dependency queries.

### Phase 2: Shared Query Semantics

1. Complete lookup, explain, impact, path, and explore core operations.
2. Add ambiguity result handling.
3. Add CLI rendering for all query kinds.
4. Add CLI/MCP core-result equivalence tests using adapter-independent fixtures.

Target scenarios: SCN-005, SCN-006, SCN-007, SCN-011, SCN-024, SCN-026.

### Phase 3: Interface Evidence Layer

1. Add provider interface and normalized `InterfaceFact` storage.
2. Implement minimum providers required by fixtures.
3. Add evidence ranking and conflict diagnostics.
4. Link interface facts into explain, impact, path, and explore results.

Target scenarios: SCN-012, SCN-013, SCN-017.

### Phase 4: Workspace Topology

1. Add `.vela/workspace.yaml` validation and ingestion.
2. Persist workspace facts with provenance.
3. Implement route-first exploration.
4. Add multi-repo fixture and route-known/deep-graph-unavailable diagnostics.

Target scenarios: SCN-014, SCN-015, SCN-016, SCN-022.

### Phase 5: Freshness, Update, and Watch

1. Extend manifest model for workspace config, contracts, provider inputs, and schema fingerprints.
2. Implement status stale detection.
3. Implement safe update with fallback rebuild.
4. Implement watch debounce and serialized updates.

Target scenarios: SCN-018, SCN-019, SCN-020.

### Phase 6: MCP Adapter and Release Fixtures

1. Add `vela serve --mcp` startup and tool registration.
2. Route all tools through the core engine.
3. Add MCP startup freshness behavior.
4. Prove OpenCode and Claude Code compatibility.
5. Run the real workspace smoke test.

Target scenarios: SCN-008, SCN-009, SCN-010, SCN-023, SCN-025.

## Open Questions

- Should v0.4 expose explain/impact/path as first-class CLI subcommands, or preserve structural search forms while adding aliases? The design allows either, but implementation should choose one primary user-facing route.
- Should MCP startup default to fail-fast on missing graph, or start in status-only degraded mode? Both satisfy the spec if diagnostics are clear; fail-fast is simpler, status-only is friendlier for agents.
- Which framework route extractors are required for the first fixture beyond the language/frameworks already present in Vela's own codebase?

## Review Checklist

- SQLite is the only runtime query source.
- CLI and MCP use the same core engine and result schema.
- Natural-language explore resolves candidates before structural traversal.
- Interface facts retain source, confidence, and claim status.
- Workspace topology is routing truth, not deep code truth.
- Freshness warnings flow through status, CLI queries, and MCP tools.
- Every SCN-001 through SCN-026 maps to a design component and fixture strategy.
