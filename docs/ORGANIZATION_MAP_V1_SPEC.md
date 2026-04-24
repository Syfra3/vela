# Organization Map V1 Spec

## Purpose

`Organization Map` is Vela's documentation-oriented graph artifact for modeling
cross-repository technical topology.

It is intentionally separate from the repo-local `Repo Graph` stored in
`.vela/graph`.

- `Repo Graph` answers code retrieval questions inside one repository.
- `Organization Map` answers architecture and topology questions across
  repositories, services, and resources.

`Organization Map` is documentation truth, not code truth.

## Separation From Repo Graph

The two artifacts must stay isolated.

### Repo Graph

- storage: `.vela/graph`
- scope: one repository
- purpose: retrieval, navigation, impact analysis
- truth model: extracted code facts

### Organization Map

- storage: `.vela/org-map/` or a future org-shared Vela store
- scope: organization or workspace
- purpose: technical documentation and topology visualization
- truth model: declared architecture facts, imported infrastructure facts, and
  explicit bridge relations

Do not mix code symbols, files, and package edges from the repo graph into the
default organization map views.

## Core Principles

1. Declared documentation beats inference.
2. Imported infrastructure facts beat guessed infrastructure facts.
3. Cross-repo relations must carry provenance and confidence.
4. Visualization defaults to stable architecture nouns, not implementation
   noise.
5. The first version optimizes for useful topology, not ontology completeness.

## Artifact Shape

An `Organization Map` artifact contains:

- top-level metadata
- a list of nodes
- a list of edges
- provenance on every node and edge
- optional view presets for visualization

Suggested canonical materialization path:

```text
.vela/org-map/map.json
```

Suggested supporting paths:

```text
.vela/org-map/sources/
.vela/org-map/views/
.vela/org-map/provenance/
```

## Node Types

V1 supports these node types only:

1. `organization`
2. `workspace`
3. `repository`
4. `service`
5. `resource`
6. `document`

### Node Requirements

Every node must include:

- `id`
- `type`
- `name`
- `provenance`
- `confidence`
- `source_ref`
- `observed_at`

Optional common fields:

- `description`
- `labels`
- `tags`
- `external_ref`
- `notes`
- `resource_kind` for `resource` nodes only

### Resource Kinds

`resource` nodes must use one of these `resource_kind` values:

- `database`
- `queue`
- `bucket`
- `topic`
- `cache`
- `secret_store`
- `dns`
- `api_gateway`
- `function`
- `object_store`
- `vendor_api`
- `kubernetes_cluster`
- `namespace`
- `network`
- `other`

## Edge Types

V1 supports these edge types only:

1. `contains`
2. `owns`
3. `deploys_to`
4. `uses`
5. `calls`
6. `publishes_to`
7. `consumes_from`
8. `exposes`
9. `documents`
10. `bridges`

### Edge Requirements

Every edge must include:

- `id`
- `type`
- `from`
- `to`
- `provenance`
- `confidence`
- `source_ref`
- `observed_at`

Optional fields:

- `directional` default `true`
- `notes`
- `labels`
- `external_ref`

## Recommended Semantics

### `contains`

Use for structural containment.

Examples:

- `organization -> workspace`
- `workspace -> repository`
- `workspace -> resource`

### `owns`

Use when a repository is the primary owner or implementation home of a service.

Example:

- `repository -> service`

### `deploys_to`

Use for runtime landing targets.

Examples:

- `service -> kubernetes_cluster`
- `service -> namespace`
- `service -> function`

### `uses`

Use for technical dependencies.

Examples:

- `service -> database`
- `repository -> vendor_api`

### `calls`

Use for service-to-service interaction.

Example:

- `service -> service`

### `publishes_to` and `consumes_from`

Use for event and queue interactions.

Examples:

- `service -> topic`
- `service -> queue`

### `exposes`

Use when a service is exposed through a gateway, DNS entry, or public endpoint.

### `documents`

Use when a document is the source of declared architecture truth.

Examples:

- `document -> workspace`
- `document -> service`
- `document -> resource`

### `bridges`

Use for explicit cross-layer links the UI should highlight.

Examples:

- `repository -> service`
- `repository -> resource`
- `service -> repository`

## Source Of Truth Rules

### Declared

Use for:

- workspace membership
- service existence
- service ownership
- intended cross-repo relationships
- architecture intent

Sources:

- architecture docs
- ADRs
- repo manifests
- manually curated map files

### IaC

Use for:

- resource inventory
- deploy targets
- infrastructure-linked dependencies

Sources:

- Terraform
- Pulumi
- Kubernetes manifests
- cloud descriptors

### Repo Metadata

Use for:

- repository identity
- repository role
- declared service ownership
- declared dependencies

### Inferred

Use only for candidate relations.

Inference must never become silent truth. Inferred edges remain lower confidence
and should be hidden by default in stable architecture views.

## Provenance And Confidence

Every node and edge must include:

- `provenance`
- `confidence`
- `source_ref`
- `observed_at`

Allowed `provenance` values:

- `declared`
- `iac`
- `repo_metadata`
- `inferred`
- `manual_override`

Allowed `confidence` values:

- `high`
- `medium`
- `low`

Recommended policy:

- `manual_override` -> `high`
- `declared` -> `high`
- `iac` -> `high`
- `repo_metadata` -> `medium` or `high`
- `inferred` -> `low`, promoted to `medium` only when corroborated by multiple
  independent sources

## Default Views

### Workspace View

Default nodes:

- `workspace`
- `repository`
- declared `service`

Default edges:

- `contains`
- `owns`
- high-confidence `calls`

Goal:

- explain repo and service boundaries
- explain which repos belong to which workspace
- explain high-confidence service interactions

### Resource View

Default nodes:

- `service`
- `resource`

Default edges:

- `deploys_to`
- `uses`
- `publishes_to`
- `consumes_from`
- `exposes`

Goal:

- explain runtime topology
- explain infrastructure dependencies
- explain blast radius around critical resources

### Bridge Relations

Default nodes:

- `repository`
- `service`
- `resource`

Default edges:

- `bridges`
- supporting `owns`, `uses`, and `deploys_to` when needed for context

Goal:

- explain how repo ownership maps to deployable services
- explain how services map to resources
- expose cross-layer relationships without mixing in code-level detail

## Hidden By Default

Do not visualize these by default:

- code symbols from `.vela/graph`
- file-level or package-level nodes
- low-confidence inferred edges
- raw cloud implementation noise such as every subnet, IAM policy, or pod
- stale relationships
- document nodes in the main view unless provenance mode is enabled

## Minimal Viable Version

V1 should ship with only these node types:

- `organization`
- `workspace`
- `repository`
- `service`
- `resource`

And only these edge types:

- `contains`
- `owns`
- `uses`
- `calls`
- `deploys_to`
- `bridges`

`document`, `publishes_to`, `consumes_from`, and `exposes` are still valid in
the schema, but the first usable implementation can keep the imported and
rendered subset smaller.

## Future Expansion

Add only when demanded by real workflows:

- `environment` nodes
- `domain` or `bounded_context` nodes
- `team` ownership nodes
- `capability` nodes
- API surface nodes
- temporal versioning and drift tracking

## Naming

User-facing terms:

- `Repo Graph`
- `Organization Map`

Organization Map view names:

- `Workspace View`
- `Resource View`
- `Bridge Relations`

These names are preferred over `Architecture Graph`, which is too vague, and
over reusing `.vela/graph`, which would conflate retrieval artifacts with
documentation artifacts.
