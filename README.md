<div align="center">
  <img src="assets/vela-header.png" alt="Vela" width="100%"/>
</div>

# Vela - Graph-Truth for Codebases

Vela is a local-first graph-truth index for codebases: it builds evidence-backed dependency graphs and exposes read-only structural queries through the CLI and MCP.

## Quick Start

```bash
# Build the CLI from source.
go build -o vela ./cmd/vela

# Build graph outputs for a repo or workspace.
./vela build ./my-repo

# Check graph health and freshness.
./vela status --graph ./my-repo/.vela/graph.json

# Discover candidates before structural queries.
./vela explore --graph ./my-repo/.vela/graph.json "where is billing handled?"
./vela lookup --graph ./my-repo/.vela/graph.json "BillingService"

# Ask structural graph questions after you have exact node names.
./vela search --graph ./my-repo/.vela/graph.json "explain BillingService"
./vela search --graph ./my-repo/.vela/graph.json "who uses BillingService"
./vela search --graph ./my-repo/.vela/graph.json "path BillingController -> BillingService"

# Start the default stdio MCP server for coding agents.
./vela serve --graph ./my-repo/.vela/graph.json
```

Expected `build` output includes `graph:`, `html:`, `report:`, `files:`, and `facts:` lines. The primary generated files live under `<repo>/.vela/`.

## What Vela Is

| Vela does | Vela does not do |
| --- | --- |
| Build local code and workspace graphs from repository files. | Act as durable chat memory or a notes database. Use Ancora for that. |
| Answer dependency, impact, path, and explanation questions from graph facts. | Replace grep, ripgrep, or exact file reads for raw text lookup. |
| Preserve evidence, confidence, freshness, and diagnostics where available. | Invent architecture truth from broad free-text prompts. |
| Expose the same graph-query core through CLI, MCP, and legacy HTTP debug mode. | Treat stale `graph.json` as trusted runtime truth for repo-local graphs. |

The happy path is: build the graph, discover exact candidates, then run structural queries on those exact nodes.

## Build and Verify

Vela is a Go project. The required Go version is declared in `go.mod`.

```bash
# Build release-style local binary.
make build

# Install to ~/.local/bin.
make install

# Run the full local gate used by this repo.
make verify
```

`make verify` runs `fmt-check`, pinned `golangci-lint`, scoped tests, and `build`. Tests are intentionally scoped to `./cmd/... ./internal/... ./pkg/...` because fixture directories under `tests/fixtures/detect/**` are not valid standalone Go packages.

For quick command-surface checks during documentation or CLI work:

```bash
go run ./cmd/vela --help
go test ./cmd/vela -run TestRootCommandExposesReducedBuildAndQuerySurface -count=1
```

## Graph Lifecycle

| Step | Command | Behavior |
| --- | --- | --- |
| Build | `vela build <path>` | Scans source files, runs available SCIP drivers, merges graph facts, writes `.vela/graph.json`, `.vela/graph.db`, `.vela/manifest.json`, `.vela/graph.html`, and `.vela/GRAPH_REPORT.md`. |
| Rebuild safely | `vela update <path>` | Uses the same build path with manifest-aware generated-state snapshotting. If update fails, previous generated graph state is restored. |
| Watch | `vela watch <path>` | Watches `.go`, `.py`, `.ts`, `.tsx`, `.js`, and `.jsx` files and refreshes graph outputs after changes. |
| Check health | `vela status --graph <path>` | Reports freshness, counts, confidence distribution, graph quality, structure, communities, and top nodes. Alias: `vela bench`. |
| Keep fresh | `vela hooks install <path>` | Installs repo-local `post-commit` and `post-checkout` hook blocks for graph freshness. |

`vela extract <path>` still exists as a compatibility alias for `vela build <path>`.

## Runtime Storage

| File | Role |
| --- | --- |
| `.vela/graph.db` | Primary v0.4 runtime query store for repo-local graphs. Required for trusted answers when querying `.vela/graph.json`. |
| `.vela/graph.json` | Export/debug artifact and graph selection handle. For repo-local `.vela/graph.json`, Vela loads sibling `graph.db` as query truth. |
| `.vela/manifest.json` | Freshness metadata. Query results can report `fresh`, `stale`, `unknown`, or `warming` state. |
| `.vela/GRAPH_REPORT.md` | Human-readable graph report. |
| `.vela/graph.html` | Local visual graph export. |
| `~/.vela/registry.json` | Global tracked-repository registry used for project/corpus discovery and ambiguity diagnostics. |

Custom or legacy graph JSON files outside a repo-local `.vela/` directory can still be loaded as compatibility data, but the active product direction is SQLite-backed runtime truth.

## Inputs and Evidence

The active public build path supports code and workspace topology evidence.

| Input | Evidence |
| --- | --- |
| `.go`, `.py`, `.ts`, `.tsx`, `.js`, `.jsx` | Repo-layer file, symbol, call/reference, and projected file-dependency facts from local extraction. |
| SCIP drivers for Go, TypeScript, and Python | Optional semantic artifacts under `.vela/scip/*.scip` when drivers are available. Missing drivers are reported as warnings and structural extraction continues. |
| `.vela/workspace.yaml` | Declared workspace routing facts for organizations, repositories, services, interfaces, and known service links. |

Minimal workspace topology example:

```yaml
organization:
  name: Acme
repositories:
  - name: billing-api
    services:
      - name: billing
        kind: api
interfaces:
  - name: billing-http
    service: billing
    kind: http
known_links:
  - from: checkout
    to: billing
    interface: billing-http
```

## Query Model

Vela separates discovery from structural reasoning.

| Need | Use | Why |
| --- | --- | --- |
| Broad context or feature-planning prompt | `vela explore <request>` | Returns graph-backed candidates and suggested next queries. This is discovery, not proof. |
| Ambiguous term | `vela lookup <term>` | Finds exact node candidates before structural graph queries. |
| Natural-language structural question | `vela search <query>` | Parses valid structural forms and runs the matching graph query. |
| Explicit query kind | `vela query <kind> ...` or root `vela explain`, `vela impact`, `vela path` | Avoids query parsing when the kind is already known. |

Valid `vela search` forms:

```text
who uses X
what uses X
where is X used?
what does X depend on
dependencies of X
impact of X
what breaks if X changes?
what is affected by X?
path A -> B
path from A to B
how does A reach B?
explain X
```

Good examples:

```bash
vela lookup --graph ./my-repo/.vela/graph.json "transaction"
vela search --graph ./my-repo/.vela/graph.json "explain TransactionMapper"
vela search --graph ./my-repo/.vela/graph.json "who uses TransactionMapper"
vela search --graph ./my-repo/.vela/graph.json "impact of MovementStatusDto"
vela search --graph ./my-repo/.vela/graph.json "path TransactionController -> TransactionMapper"
```

Bad examples:

```bash
vela search --graph ./my-repo/.vela/graph.json "movement status mobile app dto contract"
vela search --graph ./my-repo/.vela/graph.json "billing"
vela search --graph ./my-repo/.vela/graph.json "print the movement extract"
```

Those are broad discovery prompts or keyword searches. Use `explore`, `lookup`, grep, or file reads first, then query exact graph nodes.

## CLI Surface

| Command | Purpose |
| --- | --- |
| `vela` or `vela tui` | Launch the interactive TUI when stdout is a TTY. |
| `vela build <path>` | Build graph-truth outputs. |
| `vela update <path>` | Refresh generated graph outputs with rollback on failed update. |
| `vela watch <path>` | Auto-refresh graph outputs on source changes. |
| `vela status [--graph path]` | Show graph health and freshness. Alias: `bench`. |
| `vela explore <request>` | Default broad agent surface for graph-backed structural context. |
| `vela lookup <term>` | Resolve a term into candidate nodes. |
| `vela search <query>` | Route supported structural natural-language forms to graph queries. |
| `vela query dependencies <subject>` | Show outgoing dependencies. |
| `vela query reverse_dependencies <subject>` | Show incoming dependencies and users. |
| `vela query impact <subject>` or `vela impact <subject>` | Show reverse-impact facts. Supports `--format json`. |
| `vela query path <subject> <target>` or `vela path <subject> <target>` | Find graph-backed path evidence between two endpoints. Supports `--format json`. |
| `vela query explain <subject>` or `vela explain <subject>` | Explain a node with graph facts and evidence. Supports `--format json`. |
| `vela serve [graph-file]` | Serve stdio MCP tools by default. Use `--http --port 7700` for legacy HTTP debug endpoints. |
| `vela install` | Initialize `.vela/graph.db` and optionally install OpenCode or Claude Code MCP integration. |
| `vela uninstall` | Remove selected agent integrations while preserving project indexes. |
| `vela purge` | Refuse single-project purge without `--confirm` or `--force`; current single-project mode preserves `.vela/graph.db` even when confirmed. Use `vela purge --all --confirm` or `--force` to delete tracked project indexes. |
| `vela hooks install/status/uninstall <path>` | Manage Vela-managed Git hook blocks. |
| `vela compatibility` | Show language compatibility evidence levels. |
| `vela version` | Print version information. |

Common flags:

```bash
vela build ./repo --language go --driver scip-go --out-dir ./repo/.vela
vela lookup "AuthService" --graph ./repo/.vela/graph.json --limit 5
vela explain "AuthService" --graph ./repo/.vela/graph.json --format json
```

## MCP Surface

`vela serve` starts a stdio MCP server by default. It is meant for MCP clients and does not print a terminal UI.

```bash
vela serve --graph ./my-repo/.vela/graph.json
```

Registered MCP tools use unprefixed names:

| Tool | Purpose |
| --- | --- |
| `explore` | Resolve broad graph context requests into graph-backed candidates. |
| `lookup` | Resolve a term into exact graph node candidates. |
| `dependencies` | Return direct outgoing dependencies for a node. |
| `reverse_dependencies` | Return direct incoming dependencies and users for a node. |
| `impact` | Return reverse-impact facts. |
| `path` | Return graph-backed path evidence between two nodes. |
| `explain` | Explain a node with evidence-bearing graph facts. |
| `status` | Report runtime graph freshness/status. |

Some MCP clients display these with a server prefix such as `vela_explore` or `vela_explain`, but the registered tool names above are the current source of truth.

Legacy HTTP debug mode is still available:

```bash
vela serve --http --port 7700 --graph ./my-repo/.vela/graph.json
```

HTTP debug endpoints are `GET /graph`, `GET /query?kind=dependencies&subject=AuthService&limit=5`, and `GET /health`.

## Agent Guidance

Treat Vela as structural graph truth, not keyword search.

1. Start broad product or architecture prompts with discovery.
2. Identify concrete files, symbols, DTOs, services, repositories, or modules.
3. Use `vela lookup` if the exact node label is unclear.
4. Run structural queries only after choosing exact subjects or path endpoints.
5. Treat stale or unavailable graph diagnostics as blockers for trusted graph answers.

Do not send bag-of-words prompts or whole feature descriptions directly to `vela search`.

## Relevant Docs

Older design docs and reports are historical snapshots. Use this README and the MCP/CLI source for current command and tool names.

| File | Why read it |
| --- | --- |
| `AGENTS.md` | Runtime guidance for agents using Vela structural queries. |
| `docs/VELA_ARCHITECTURE.md` | Active graph-truth architecture model and layer boundaries. |
| `docs/VELA_LOOKUP_AND_AMBIGUITY_SPEC.md` | Historical design context for the lookup/search split and ambiguity handling. |
| `docs/GRAPH_DOMAIN_SCHEMAS.md` | Domain boundaries for graph views and truth models. |
| `reports/VELA_V0_4_ARCHIVE.md` | Historical v0.4 archive scope, gate evidence, and known hardening follow-ups. |
| `reports/SCN-025-real-workspace-smoke.md` | Redacted real-workspace smoke proof. |

## Current Boundaries

- Full `go test ./...` is not the active gate because malformed detect fixture packages are intentionally present under `tests/fixtures/detect/**`.
- `vela search` is structural-only. Use `explore`, `lookup`, grep, or file reads for discovery and raw text lookup.
- Repo-local trusted query answers require a usable `.vela/graph.db`; stale or missing runtime storage should be fixed with `vela update` or `vela build` before relying on answers.
- Vela's active product surface is local graph build/query plus MCP. Durable memory writes belong to Ancora, not Vela.
