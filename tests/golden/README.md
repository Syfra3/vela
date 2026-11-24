# Offline golden evaluation

`TMPDIR=/dev/shm make eval-golden > report.json` builds the current production CLI
with the local Go module/build cache and networking disabled. It copies the four
synthetic fixtures into a disposable directory, runs a real MCP stdio server,
indexes only those copies, mutates them, and rebuilds them. The Go-only bound
fixture has no module marker, avoiding selection of an external SCIP driver;
its lifecycle asserts fresh → stale → fresh in the original session. The
inventory fixture creates an ignored `node_modules/go.mod` symlink to its
benign `main.go`, with no root module configuration, and checks the manifest's
exact source-file list. HOME, XDG paths and an empty runtime PATH are isolated;
no driver or toolchain is installed. Go cache discovery and compilation disable
network access, Go environment configuration, workspace mode, and automatic
toolchain selection; only compilation receives a system-tool PATH. A warm local Go
module cache and Go toolchain are required. Set TMPDIR to another writable,
executable filesystem if `/dev/shm` is unavailable.

For comparisons, build each revision independently and run the *same* harness
and corpus: `python3 scripts/golden_eval.py --binary /absolute/path/to/vela --output base.json`
(and likewise for the candidate), then
`python3 scripts/golden_eval.py --compare base.json candidate.json`.
The comparator rejects different versioned recipe hashes, case IDs or labels,
and reports lacking a binary SHA-256. The recipe hash covers corpus bytes and
both harness scripts, including the generated-fixture recipe. Binary hashes
identify the compared artifacts and may differ. Reports contain per-case expected/actual tuples, set scores, errors and
seconds, plus micro-aggregated counts. Timing is observational, never a gate.
A successful runner exits zero even if cases fail: inspect `summary` and each
case, with per-fixture and per-category summaries. Fixture build and MCP
initialization failures are recorded per case; independent fixtures continue.
If graphless initialization fails, one post-publish session is attempted for
quality cases only: lifecycle cases never use that replacement. CLI compilation
and report-writing infrastructure failures exit nonzero. Status and freshness cases
are exact expectations, not assertions that every revision implements them.
Text-only tool responses are reported as unscorable failures rather than
inventing typed graph facts. Path tuples preserve edge order and edge type.

The tiny dev and holdout fixtures are independent hand-written examples, not
real repositories; their labels in `cases.json` are source-derived expectations,
not generated from Vela. The ambiguity case requires both distinct IDs from
`alpha.py` and `beta.py` and an ambiguous status; label duplication alone is
not sufficient. This initial corpus is intentionally small and does
not establish production accuracy or performance.
