# Benchmark First-Loop Targets

## Scope

- Subject under test: real `vela` built from `/home/geen/Documents/personal/vela`
- Required build step: `make dev`
- Required binary under test: `/home/geen/Documents/personal/vela/bin/vela`
- Benchmark comparators: `vela`, `vela-next-sim`, `graphify`
- Full benchmark 1: `dep-eval run --adapter vela,vela-next-sim,graphify --corpus vela`
- Full benchmark 2: `dep-eval run --adapter vela,vela-next-sim,graphify --corpus stock-chef`
- Query-only benchmark: `dep-eval query-run --adapter vela,vela-next-sim,graphify --suite code-v1`

The loop contract uses both corpora deliberately:

- `vela` measures behavior on the product's own repository and catches self-hosting regressions.
- `stock-chef` is the larger stress corpus and exposes scaling, ranking, and path-quality weaknesses that the smaller repo can hide.
- `code-v1` graph-query suite already spans both corpora, so the query-only run remains one combined benchmark.

## Locked Baseline

Use these two dep-eval runs as the approval baseline for loop 1.

- Full benchmark baseline for `vela` corpus:
  `/home/geen/Documents/personal/dep-eval/results/2026-04-22T19-47-51-916Z-vela-vela-vs-vela-next-sim-vs-graphify/report.json`
- Full benchmark baseline for `stock-chef` corpus:
  `/home/geen/Documents/personal/dep-eval/results/2026-04-22T21-01-05-592Z-stock-chef-vela-vs-vela-next-sim-vs-graphify/report.json`
- Query benchmark baseline:
  `/home/geen/Documents/personal/dep-eval/results/2026-04-22T19-47-51-911Z-graph-query-code-v1-vela-vs-vela-next-sim-vs-graphify/graph-query-report.json`

## Immediate Gates For Loop 1

Loop 1 is about hardening reverse lookup and path quality without wrecking the real binary.

### Must Hold

1. `make dev` succeeds.
2. `/home/geen/Documents/personal/vela/bin/vela` is the binary benchmarked by dep-eval.
3. `vela` remains in the comparison set with `vela-next-sim` and `graphify` only.
4. Full benchmark `build_success` stays `true`.
5. Full benchmark `build_time_ms` for `vela` stays `<= 2000` on the `vela` corpus.
6. Full benchmark `file_coverage_ratio` for `vela` stays `>= 0.9898` on the `vela` corpus.
7. Full benchmark `symbol_coverage_ratio` for `vela` stays `1.0` on the `vela` corpus.
8. Full benchmark `unresolved_reference_rate` for `vela` stays `0` on the `vela` corpus.
9. Full benchmark `layer1_weighted_score` for `vela` does not drop below `0.4992` on the `vela` corpus.
10. Full benchmark `layer2_weighted_score` for `vela` does not drop below `0.868` on the `stock-chef` corpus.
11. Full benchmark `layer1_weighted_score` for `vela` does not drop below `0.350` on the `stock-chef` corpus.
12. Query benchmark `weighted_score` for `vela` does not drop below `0.7585`.
13. Query benchmark `error_rate` equivalent stays clean: no adapter failure for `vela` in either full run or the query run.

### Must Improve

At least one reverse-lookup metric and one path-quality metric must improve for real `vela` versus the locked baseline.

1. Reverse lookup baseline:
   `mean_precision = 0.325`, `mean_recall = 0.800`, `mean_f1 = 0.4533`
2. Path quality baseline in graph-query overall:
   `path_node_precision = 0.5667`, `path_node_recall = 0.5378`
3. Path-tracing family baseline:
   `path_node_recall = 0.8333`
4. A loop does not pass if only aggregate score moves while both targeted families stay flat.

## Preferred Pass Condition

Treat loop 1 as a clean win only if all of these are true:

1. Reverse lookup `mean_f1` rises above `0.4533`.
2. Query benchmark `path_node_precision` rises above `0.5667`.
3. Query benchmark `path_node_recall` rises above `0.5378`.
4. Full benchmark `build_time_ms` stays `<= 2000` on the `vela` corpus.
5. Full benchmark `layer1_weighted_score` stays `>= 0.4992` on the `vela` corpus.
6. Full benchmark `layer2_weighted_score` stays `>= 0.868` on the `stock-chef` corpus.

## Runner

Use `scripts/eval-loop.sh` to run the actual improvement campaign.

- It keeps persistent phase state in `scripts/phase-state.json` so the loop can resume safely.
- It runs bounded campaign phases in order instead of a single one-shot hypothesis pass.
- Each phase uses the existing `scripts/run-benchmark-loop.sh` as the inner execution engine.
- It preserves the best result seen in each phase and advances only when that phase clears its goals.
- It stops for human direction if a previously approved floor regresses in a later phase.

If you want the inner execution engine to auto-apply changes, point each phase in `scripts/eval-loop.json` at a hypotheses file using the schema documented in `docs/BENCHMARK_LOOP_HYPOTHESES.md`.
