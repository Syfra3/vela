# Graph Domain Schemas

## Purpose

This document defines the domain boundary of each Vela graph artifact or graph
view.

It does not define raw storage fields. That already exists in JSON Schema.

This document defines:

- what each graph exists to represent
- what kind of truth it contains
- which entities belong inside it
- which relations are allowed
- which invariants must stay true
- what is explicitly out of scope

If these boundaries are not enforced, every graph turns into a noisy bucket of
half-true edges. That is how graph products die.

## Naming

Use these user-facing names:

- `Repo Graph`
- `Organization Map`
- `Workspace View`
- `Resource View`
- `Bridge Relations`

`Workspace View`, `Resource View`, and `Bridge Relations` are views over the
same `Organization Map` domain model. They are not independent truth systems.

`Repo Graph` is different. It is its own truth system.

## 1. Repo Graph

### Purpose

Represent repo-local code structure for retrieval, navigation, and impact
analysis.

### Truth Model

Code truth.

Facts in this graph are extracted from repository contents. This graph should be
as close as possible to what the code actually says.

### Aggregate Roots

- `repository`
- `subsystem`

The repository is the correctness boundary. Subsystems are the coarse internal
boundary used to keep retrieval explainable in large repos.

### Allowed Node Types

- `repository`
- `subsystem`
- `file`
- `symbol`
- `chunk`

Optional later:

- `package`
- `module`
- `test_symbol`

### Allowed Edge Types

- `contains`
- `defines`
- `imports`
- `calls`
- `references`
- `implements`
- `depends_on`
- `chunk_of`

### Invariants

1. Everything belongs to exactly one repository boundary.
2. Symbol and file nodes must be traceable to real source locations.
3. Edges must be extractable or directly derivable from code artifacts.
4. This graph must not claim business or runtime truth it cannot prove.
5. Cross-repo architecture claims do not belong here.

### Out Of Scope

- business domains
- service ownership by organization
- infrastructure resources
- architecture intent from docs
- onboarding narratives

### Canonical Identity

Recommended identity pattern:

- `repo:<repo-name>`
- `repo:<repo-name>:subsystem:<name>`
- `repo:<repo-name>:file:<path>`
- `repo:<repo-name>:symbol:<qualified-name>`
- `repo:<repo-name>:chunk:<stable-hash-or-offset>`

### Typical Questions

- Where is this implemented?
- What calls this symbol?
- Which files depend on this module?
- What breaks if I change this package?

## 2. Organization Map

### Purpose

Represent organization-level technical topology across repositories, services,
and resources.

### Truth Model

Documentation truth.

This is not code truth. It combines declared architectural facts, imported
infrastructure facts, and explicit cross-layer relations.

### Aggregate Roots

- `organization`
- `workspace`

Everything in the Organization Map must belong to an organization scope, and in
normal usage it should belong to a workspace scope.

### Allowed Node Types

- `organization`
- `workspace`
- `repository`
- `service`
- `resource`
- `document`

### Allowed Edge Types

- `contains`
- `owns`
- `deploys_to`
- `uses`
- `calls`
- `publishes_to`
- `consumes_from`
- `exposes`
- `documents`
- `bridges`

### Invariants

1. Declared truth beats inferred truth.
2. Imported IaC beats guessed resource relationships.
3. Every edge carries provenance and confidence.
4. Low-confidence inferred topology must remain suppressible.
5. Organization Map must not duplicate repo-internal symbol graphs.

### Out Of Scope

- file-level code navigation
- symbol-level retrieval
- package dependency graphs
- deep AST facts

### Canonical Identity

Recommended identity pattern:

- `organization:<name>`
- `workspace:<name>`
- `repository:<name>`
- `service:<name>`
- `resource:<name>`
- `document:<name>`

If multiple environments later exist, add environment qualification only when
needed. Do not force environment into V1 identities unless the product actually
needs multi-environment resource separation.

### Typical Questions

- Which repositories belong to this workspace?
- Which service owns this capability?
- Which resources does this service depend on?
- What is connected to this repository outside its code?

## 3. Workspace View

### Purpose

Show the organizational and service topology of a workspace.

This is the main view for understanding technical structure across repositories.

### Truth Model

Organization Map truth filtered toward repo and service boundaries.

### Core Entities

- `workspace`
- `repository`
- `service`

`workspace` is the aggregate root for this view.

### Allowed Node Types

- `workspace`
- `repository`
- `service`
- `document` optional, hidden by default

### Allowed Edge Types

- `contains`
- `owns`
- `calls`
- `documents` optional, hidden by default

### Invariants

1. Every repository shown must belong to a workspace.
2. Services shown here should be declared or high-confidence entities.
3. Service-to-service calls must not be inferred from code alone without
   provenance.
4. Repo internals never appear here.

### Out Of Scope

- files
- symbols
- resource internals
- cloud noise

### Canonical Identity

Reuse Organization Map identities.

This view must not mint its own node IDs.

### Typical Questions

- What repositories are part of this workspace?
- Which repo owns this service?
- Which services interact with each other?
- Where should I start looking for a change in this domain?

## 4. Resource View

### Purpose

Show runtime and infrastructure topology for services.

### Truth Model

Organization Map truth filtered toward service and resource relations.

### Core Entities

- `service`
- `resource`

`service` is the natural operational anchor of this view.

### Allowed Node Types

- `service`
- `resource`
- `document` optional, hidden by default

### Allowed Edge Types

- `deploys_to`
- `uses`
- `publishes_to`
- `consumes_from`
- `exposes`
- `documents` optional, hidden by default

### Invariants

1. Resource nodes should come from IaC, curated declarations, or validated
   imports.
2. A resource relationship must not be shown as high confidence unless it has a
   real source.
3. This view stays service-centric rather than cloud-provider-centric.
4. Raw infra primitives that explode the graph stay out by default.

### Out Of Scope

- file-to-resource guesses
- every pod, subnet, or IAM policy
- internal code dependencies

### Canonical Identity

Reuse Organization Map identities.

Suggested resource identity form:

- `resource:<logical-name>`

If later needed:

- `resource:<environment>:<logical-name>`

### Typical Questions

- Which database does this service use?
- Which queue does this worker consume from?
- Where is this service deployed?
- What infrastructure is in the blast radius of this service?

## 5. Bridge Relations

### Purpose

Make cross-layer relations explicit.

This is the view that explains how repositories map to services and how services
map to resources.

Without this view, the user sees disconnected worlds.

### Truth Model

Organization Map truth filtered toward cross-layer joins.

### Core Entities

- `repository`
- `service`
- `resource`

This view has no independent aggregate root. It reuses Organization Map
entities.

### Allowed Node Types

- `repository`
- `service`
- `resource`

### Allowed Edge Types

- `bridges`
- `owns`
- `uses`
- `deploys_to`

### Invariants

1. Bridge Relations must reuse canonical identities from the Organization Map.
2. Bridge Relations must never create bridge-only node identities.
3. A bridge edge exists to expose a meaningful cross-layer explanation, not to
   restate every ordinary edge in the system.
4. The view should stay sparse and explanatory.

### Out Of Scope

- repo internals
- symbol-level links
- full cloud topology
- every service-to-service edge unless needed for explanation

### Canonical Identity

Reuse Organization Map identities exactly.

Do not invent IDs such as `bridge:repo-a-to-service-b` for nodes. Only edges may
carry bridge-specific IDs.

### Typical Questions

- Which repo implements this service?
- Which service is backed by this repository?
- Which resources are attached to the service owned by this repo?
- What is the technical path from repo to runtime dependency?

## Final Recommendation

Use `graph` only where there is a distinct truth system.

That means:

- `Repo Graph` is a graph.
- `Organization Map` is a documentation graph artifact.
- `Workspace View`, `Resource View`, and `Bridge Relations` are views over the
  Organization Map.

User-facing language should favor:

- `Repo Graph`
- `Organization Map`
- `Workspace View`
- `Resource View`
- `Bridge Relations`

Do not tell users they are dealing with four independent graphs if three of them
are only filtered lenses over the same documentation model. That is fake
complexity.
