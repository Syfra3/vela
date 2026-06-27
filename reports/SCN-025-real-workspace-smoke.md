# SCN-025 Real Workspace Smoke Report

workspace: <REAL_WORKSPACE>
redaction_policy: no secrets
status: complete

## Commands and Sanitized Results

1. `vela build <REAL_WORKSPACE>`
   - result: pass
   - graph_json: `<REAL_WORKSPACE>/.vela/graph.json`
   - graph_db: present
   - manifest: present
   - files: 2911
   - nodes: 12923
   - edges: 30820

2. `vela status --graph <REAL_WORKSPACE>/.vela/graph.json --baseline ""`
   - result: pass
   - freshness: fresh
   - manifest: present, full rebuild, 2911 tracked files
   - report: present
   - quality: 100.00% resolution rate, 0 broken edges, 0 self-loops, 0 duplicate edges

3. `vela lookup "ExecuteAugusteTool" --graph <REAL_WORKSPACE>/.vela/graph.json --limit 5`
   - result: pass
   - graph-backed candidates: ExecuteAugusteToolDto, ExecuteAugusteToolInput, ExecuteAugusteToolUseCase

4. `vela explain "ExecuteAugusteToolUseCase" --graph <REAL_WORKSPACE>/.vela/graph.json --format json`
   - result: pass
   - status: ok
   - resolved subject: ExecuteAugusteToolUseCase
   - fact: source file contains ExecuteAugusteToolUseCase
   - evidence-bearing: yes
   - evidence: layer repo, type filesystem, confidence declared, source artifact redacted to repository-relative path
   - freshness: fresh

5. `VELA_SCN025_WORKSPACE=<REAL_WORKSPACE> go test ./cmd/vela -run TestSCN025_RealWorkspaceSmokeHarness -count=1`
   - result: pass
   - automated harness rebuilt the real workspace, verified `.vela/graph.db`, checked status freshness, executed evidence-bearing CLI explain JSON, started `vela serve --mcp`, and called `MCP tool call: vela_explain` with structured `query.Result` content.
   - MCP evidence-bearing: yes

## Secret and Redaction Check

- secret scan: pass
- No environment files, tokens, credentials, private keys, or secret-like values were copied into this report.
- Absolute user/workspace paths are redacted as `<REAL_WORKSPACE>`.
- Report content keeps only repository-relative source artifact names needed to prove evidence behavior.

## Risk Notes

- The smoke intentionally uses one maintainer-selected real workspace and one known evidence-bearing subject; it is release smoke coverage, not exhaustive corpus validation.
- The real workspace generated artifacts live outside this repository under `<REAL_WORKSPACE>/.vela/` and are not committed here.
