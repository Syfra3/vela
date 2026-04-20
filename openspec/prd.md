# PRD: Vela Full Local-First Retrieval Architecture

**Project**: Vela
**Repo**: github.com/Syfra3/vela
**Branch**: `feature/ralph-full-architecture-workflow`
**Status**: Draft
**Primary User**: Local solo developer

## Problem

Vela needs to move from repo-local graph tooling into a full local-first retrieval system that can answer architectural questions across repositories, workspaces, contracts, and durable memory without collapsing everything into one noisy graph.

The current architecture direction is already defined in `syfra/VELA_ARCHITECTURE.md`, but the implementation workflow is not yet encoded as a practical, testable Ralph execution plan.

## Users

### Primary User

- local solo developer building and validating Vela on one machine

### Secondary Users

- developer exploring a large monolith through subsystem routing
- developer exploring several related repos through workspace routing
- maintainer validating installation, setup, doctor checks, and retrieval correctness

## Goals

- Implement the full layered architecture, not a reduced phase-only subset.
- Keep the core local-first and usable without cloud services.
- Preserve SQLite as the correctness-path baseline for indexing and retrieval.
- Support repo-local deep retrieval, workspace routing, contract truth, and memory-aware fusion.
- Add install validation, doctor validation, and an Ancora-like setup wizard.
- Make retrieval explainable with provenance and evidence-aware ranking.
- Encode the implementation into a Ralph workflow that can be run story by story.

## Non-Goals

- Replacing the SQLite baseline with a mandatory remote service
- Building one giant unified symbol graph across all repos
- Mixing memory observations directly into repo code graphs
- Treating derived workspace edges as contract truth
- Shipping an architecture that cannot be validated through tests, CLI flows, TUI flows, and install verification

## Scope

This PRD includes all major architecture components described in `VELA_ARCHITECTURE.md`:

- Repo Graph and repo-local indexes
- Contract Graph
- Workspace Graph
- Memory Graph
- Identity Resolver
- Memory Reference Binder
- Evidence Model
- Retrieval Orchestrator
- CLI commands
- TUI validation surfaces
- doctor and installation verification
- setup wizard
- Ralph workflow assets in `workflows/ralph/`

## Success Criteria

- `go test ./...` passes for the implemented feature set.
- extraction, search, path, explain, query, doctor, and related CLI flows pass smoke validation.
- doctor checks validate graph health, local dependencies, memory integration, and install readiness.
- the TUI doctor/setup flow can be validated interactively by a local terminal user.
- installation verification succeeds on a fresh local setup path.
- the setup wizard behaves like Ancora in spirit: guided, local-first, and able to verify readiness instead of only writing config.
- retrieval validation demonstrates route-first, retrieve-deeply-second behavior across memory, workspace, contract, and repo layers.
- workflow docs and Ralph assets are present, consistent, and runnable.

## Major Components

### Repo Graph

- per-repo extraction boundary
- repo-local lexical and vector retrieval
- deep code retrieval with bounded updates and explainable provenance

### Contract Graph

- declared service and interface truth from artifacts such as OpenAPI, proto, manifests, and config
- separate from repo code truth and workspace routing truth

### Workspace Graph

- lightweight routing graph over repos, services, domains, packages, and dependencies
- used to decide where to search next

### Memory Graph

- structured layer over Ancora-backed observations, decisions, bugs, preferences, and sessions
- linked to code entities by reference rather than duplication

### Identity Resolver

- produces canonical keys for cross-graph joins
- prevents each layer from inventing identity independently

### Memory Reference Binder

- keeps memory references live as code moves
- marks references as current, redirected, stale, or ambiguous

### Evidence Model

- typed evidence across all graph edges and entities
- supports confidence, provenance, and ranking policy

### Retrieval Orchestrator

- classifies query scope
- routes through memory, contract, workspace, and repo layers
- fuses evidence into explainable answers

### Local Validation Surfaces

- CLI commands in `cmd/vela`
- TUI flows in `internal/tui`
- doctor checks in `internal/doctor`
- setup wizard in `internal/setup`

## Validation

- unit and integration coverage for graph, retrieval, query, doctor, setup, TUI, extraction, and Ancora-backed memory flows
- CLI smoke validation for extraction and query-oriented commands
- TUI walkthrough validation for doctor/setup flows
- installation verification on a clean local path
- retrieval benchmark or scenario validation using architecture-shaped questions and routing expectations
- docs validation for workflow usage and local setup

## Risks

- the scope is large and can drift if stories are too fine-grained or poorly ordered
- canonical identity and rebinding can become brittle if evidence is underspecified
- retrieval can look structurally correct while still failing real query quality checks
- doctor and wizard flows can lie if they validate config presence instead of real runtime behavior
- optional accelerator support can accidentally fork the logical model if fallback parity is not enforced

## Ralph Workflow

The implementation is tracked in `workflows/ralph/prd.json` as 12 substantial stories grouped around architectural boundaries instead of dozens of tiny tasks.

Maintainer-facing workflow and validation guidance lives in `docs/RALPH_WORKFLOW.md`.

Run the loop with:

```bash
./workflows/ralph/ralph.sh --status
./workflows/ralph/ralph.sh
./workflows/ralph/ralph.sh --all
```

The loop reads `prd.json`, picks the next pending story, launches the configured coding agent, and appends progress to `progress.txt`. Use `docs/RALPH_WORKFLOW.md` as the source of truth for setup expectations, validation coverage, and architecture-shaped benchmark scenarios.
