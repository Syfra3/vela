# Skill Registry — Vela

Generated: 2026-04-12
Project: vela (github.com/Syfra3/vela)

## Available Skills

### SDD Workflow
| Skill | Trigger |
|-------|---------|
| `sdd-init` | `/sdd-init` — initialize SDD context |
| `sdd-explore` | `/sdd-explore <topic>` — investigate codebase or idea |
| `sdd-propose` | `/sdd-propose <change>` — create change proposal |
| `sdd-spec` | `/sdd-spec` — write detailed specs |
| `sdd-design` | `/sdd-design` — create technical design |
| `sdd-tasks` | `/sdd-tasks` — break down into implementation tasks |
| `sdd-apply` | `/sdd-apply` — implement tasks |
| `sdd-verify` | `/sdd-verify` — validate implementation |
| `sdd-archive` | `/sdd-archive` — finalize and archive change |

### Testing
| Skill | Trigger |
|-------|---------|
| `go-testing` | When writing Go tests, using `testing` package, table-driven tests |

### Process
| Skill | Trigger |
|-------|---------|
| `prd` | `create a prd`, `plan this feature`, `requirements for` |
| `judgment-day` | `judgment day`, `dual review`, `adversarial review` |
| `issue-creation` | Creating a GitHub issue, reporting a bug |
| `branch-pr` | Creating a pull request |
| `skill-creator` | Creating a new skill |

## Project Conventions (AGENTS.md / CLAUDE.md)
- None found yet (bootstrap phase)

## Stack
- Go 1.26.1
- Bubbletea / Lipgloss (TUI)
- Cobra (CLI)
- yaml.v3 (config)
- Python 3.14 (subprocess only — Leiden clustering via graspologic)
