# PRD: Benchmark-Driven Improvement Loop for Real Vela

**Project**: Vela
**Repo**: `github.com/Syfra3/vela`
**Status**: Ready for loop 1
**Strategy**: Balanced
**Primary Target**: Improve the real `vela` binary, not the simulator
**Primary Build/Run Constraint**: Use `make dev` to build/run the real binary and benchmark `/home/geen/Documents/personal/vela/bin/vela`

## 1. Overview

Vela already answers real dependency questions far better than `graphify`, but it still loses the balanced benchmark narrative because its Layer 1 operational profile is weak and its Layer 2 query quality still has visible gaps in reverse lookup and path quality.

The current comparison is useful but not yet product-driving. `vela-next-sim` proves that better answer quality is possible when scanner and semantic edges are fused aggressively and answers are served from memory, but it also proves that this approach can become operationally unacceptable when prepare latency explodes. `graphify` proves the opposite: excellent build and incremental metrics alone do not matter if the tool cannot answer real dependency questions.

This PRD defines a benchmark-driven improvement loop for the real `vela` product so the team can raise query quality without regressing operational viability. The loop treats `vela-next-sim` as the internal quality ceiling and `graphify` as the external operational floor to beat selectively, while keeping the shipped target as the real `vela` binary. The loop must validate against both the `vela` corpus and the larger `stock-chef` corpus so small-repo wins do not hide large-repo weaknesses.

## 2. Problem Statement

Today, real `vela` has three product problems:

1. It underperforms on balanced benchmark scoring because Layer 1 metrics penalize build cost, graph size, and unsupported incremental behavior.
2. Its Layer 2 answers are good enough to stay competitive, but reverse lookup and path quality are still inconsistent enough to block trust for architecture navigation.
3. The team lacks a disciplined improvement loop that turns benchmark evidence into small, validated product iterations against the real binary.

Evidence from the current benchmark set:

- Layer 1 overall: `vela` scored `0.499`, `vela-next-sim` scored `0.705`, `graphify` scored `0.792`.
- Layer 1 build time: `vela` `1546 ms`, `vela-next-sim` `262693 ms`, `graphify` `200 ms`.
- Layer 1 incremental support: `vela` scored `0` because the current adapter contract rejects temporary fixture roots.
- Layer 2 overall: `vela` scored `0.658`, `vela-next-sim` scored `0.683`, `graphify` scored `0.200`.
- Graph-query benchmark overall: `vela` weighted score `0.759`, `vela-next-sim` `0.782`, `graphify` `0.339`.
- Query-quality gap concentration: reverse lookup is weak for both Vela variants, and path node quality is still not strong enough.
- Larger-corpus evidence matters: on `stock-chef`, real `vela` scored `0.350` on Layer 1 and `0.868` on Layer 2, while `vela-next-sim` reached `0.950` / `0.928` and `graphify` reached `0.699` / `0.200`.

## 3. Target Users and Jobs To Be Done

### Primary Users

- Vela maintainers improving extraction and query quality without destroying build/runtime cost.
- Developers using Vela to answer real codebase dependency questions from the CLI or service layer.

### Secondary Users

- Benchmark maintainers comparing Vela against internal and external baselines.
- Reviewers deciding whether a change is a real product improvement or just benchmark gaming.

### Jobs To Be Done

1. As a Vela maintainer, I want every improvement to be validated against the real binary so I do not optimize a simulator and fool myself.
2. As a developer querying a codebase, I want reverse dependency and path answers to be accurate enough to trust during architecture exploration.
3. As a benchmark maintainer, I want explicit baselines and cadence so performance wins do not hide quality regressions, and quality wins do not hide operational collapse.
4. As a reviewer, I want each loop to produce comparable before/after evidence so approval is based on measured deltas, not vibes.

## 4. Goals

- Raise the balanced benchmark position of real `vela` while preserving its advantage over `graphify` on real dependency answering.
- Close the quality gap between `vela` and `vela-next-sim` on Layer 2, especially reverse lookup and path quality.
- Improve Layer 1 operational behavior enough that real `vela` is no longer the obvious weak option on build/incremental metrics.
- Establish a repeatable improvement loop that ships only changes validated against benchmark artifacts produced from `/home/geen/Documents/personal/vela/bin/vela`.

## 5. Baselines and Comparison Targets

### Primary Subject

- `vela`: the real shipped implementation under `/home/geen/Documents/personal/vela`

### Internal Baseline

- `vela-next-sim`: use as the comparison point for query-quality ideas and fusion behavior
- Interpretation: quality ceiling and design probe, not a deploy target

### External Baseline

- `graphify`: use as the comparison point for operational cost, build speed, and incremental practicality
- Interpretation: operational benchmark, not the quality target

### Source Artifacts

- `dep-eval/results/2026-04-22T19-47-51-916Z-vela-vela-vs-vela-next-sim-vs-graphify/report.json`
- `dep-eval/results/2026-04-22T21-01-05-592Z-stock-chef-vela-vs-vela-next-sim-vs-graphify/report.json`
- `dep-eval/results/2026-04-22T19-47-51-911Z-graph-query-code-v1-vela-vs-vela-next-sim-vs-graphify/graph-query-report.md`

## 6. Success Metrics

The Balanced strategy means Vela must improve across both layers. A change is not successful if it only wins one layer by sacrificing the other.

### Layer 1: Operational and Build Metrics

For each benchmark loop, record raw values and delta versus prior approved baseline for both the `vela` corpus and the `stock-chef` corpus where applicable.

1. `build_success`
   Requirement: remain `true` for real `vela` in all benchmark runs.
2. `build_time_ms`
   Baseline: `1546 ms`
   Near-term target: do not regress above `2000 ms`
   Mid-term target: reduce below `1000 ms`
3. `incremental_update_score`
   Baseline: `0`
   Near-term target: support the benchmark contract well enough to score above `0`
   Mid-term target: beat `0.25`
4. `file_coverage_ratio`
   Baseline: `0.9898`
   Requirement: keep `>= 0.99`
5. `symbol_coverage_ratio`
   Baseline: `1.0`
   Requirement: keep `1.0`
6. `unresolved_reference_rate`
   Baseline: `0`
   Requirement: keep `0`
7. `layer1_weighted_score`
   Baseline: `0.499`
   Near-term target: exceed `0.60`
   Mid-term target: exceed `0.70`

### Layer 2: Query Correctness and Trust Metrics

1. `layer2_weighted_score`
   Baseline: `0.658`
   Near-term target: exceed `0.70`
   Mid-term target: exceed `0.75`
2. `graph-query weighted score`
   Baseline: `0.759`
   Near-term target: match or exceed `0.782` from `vela-next-sim`
3. `reverse_dependency mean_f1`
   Baseline in Layer 2 task set: `0.833`
   Baseline in graph-query family: `0.453`
   Requirement: improve in both suites; no regression allowed in either benchmark family
4. `path mean_f1`
   Baseline in Layer 2 task set: `0.667`
   Requirement: improve to `>= 0.80`
5. `path node precision` and `path node recall`
   Baseline in graph-query report: `0.567` / `0.538`
   Requirement: each should improve by at least `0.10` before the loop can be called complete
6. `tasks_with_evidence_rate`
   Baseline: `0.875`
   Requirement: improve to `1.0`
7. `error_rate`
   Requirement: remain `0`

### Balanced Release Gate

A loop is considered successful only if all of the following are true:

1. Layer 2 quality improves on the targeted family or metric.
2. Layer 1 does not materially regress against the approved baseline.
3. The real `vela` binary remains the benchmarked subject.
4. The benchmark write-up explains whether the change borrowed an idea from `vela-next-sim`, defended against an operational failure seen in `vela-next-sim`, or closed a gap exposed by `graphify`.

## 7. User Stories

### US-001: Establish the real-binary benchmark contract
**Description:** As a maintainer, I want every loop to benchmark the real `vela` binary so results reflect the shipped product rather than an internal simulation.

**Acceptance Criteria:**
- [ ] Benchmark instructions explicitly require `make dev` before benchmark execution
- [ ] Benchmark instructions explicitly require `/home/geen/Documents/personal/vela/bin/vela` as the binary under test
- [ ] Comparison set explicitly includes `vela`, `vela-next-sim`, and `graphify`
- [ ] Output artifacts record the exact run IDs and result paths used for comparison

### US-002: Track balanced benchmark scorecards
**Description:** As a reviewer, I want each loop to report both Layer 1 and Layer 2 deltas so we stop accepting one-sided wins.

**Acceptance Criteria:**
- [ ] Each loop captures Layer 1 raw metrics and weighted score
- [ ] Each loop captures Layer 2 raw metrics and weighted score
- [ ] Each loop compares results against the last approved baseline, not only the current branch state
- [ ] Each loop includes a pass/fail statement for the balanced release gate

### US-003: Prioritize reverse lookup and path quality
**Description:** As a developer using Vela for architecture exploration, I want reverse dependency and path answers to become materially more reliable so I can trust dependency navigation.

**Acceptance Criteria:**
- [ ] Each improvement cycle names one primary query-quality target family
- [ ] The first two cycles prioritize reverse lookup and path quality
- [ ] Benchmark evidence includes before/after examples for affected query families
- [ ] No cycle can claim success from latency improvements alone when the targeted quality family does not improve

### US-004: Convert simulator lessons into real-product hypotheses
**Description:** As a maintainer, I want to borrow good ideas from `vela-next-sim` without inheriting its catastrophic prepare latency.

**Acceptance Criteria:**
- [ ] Each hypothesis states which `vela-next-sim` behavior inspired it
- [ ] Each hypothesis states the expected operational risk to watch in real `vela`
- [ ] Each loop records whether the idea improved answer quality, harmed Layer 1, or both
- [ ] Changes that reproduce simulator-like prepare blowups are rejected

### US-005: Use graphify as an operational guardrail, not a product direction
**Description:** As a reviewer, I want `graphify` included as an operational comparator so Vela does not ignore build and incremental ergonomics while still optimizing for real dependency answering.

**Acceptance Criteria:**
- [ ] Each benchmark loop compares `vela` against `graphify` on Layer 1 metrics
- [ ] Each benchmark loop compares `vela` against `graphify` on Layer 2 to confirm Vela still wins where it matters
- [ ] Review notes explicitly distinguish operational lessons from retrieval-quality lessons
- [ ] No work item is approved if it makes Vela more like `graphify` on quality-critical families

## 8. Functional Requirements

1. FR-1: The benchmark loop must use the real `vela` binary built via `make dev`.
2. FR-2: The loop must benchmark `vela`, `vela-next-sim`, and `graphify` together on the same suites.
3. FR-3: The loop must treat `vela-next-sim` as the internal quality baseline and `graphify` as the external operational baseline.
4. FR-4: Every cycle must declare one primary improvement hypothesis and one explicit risk of regression.
5. FR-5: Every cycle must produce a before/after scorecard covering Layer 1, Layer 2, and graph-query family breakdowns.
6. FR-6: The first execution phase must target reverse lookup and path quality before broader benchmark expansion.
7. FR-7: The loop must reject changes that improve Layer 2 only by causing unacceptable prepare/build regressions.
8. FR-8: The loop must reject changes that improve Layer 1 while regressing direct dependency, reverse lookup, or path-tracing usefulness.
9. FR-9: Benchmark reports must preserve links or paths to the source artifacts used for review.
10. FR-10: Validation cadence must include a fast local cycle for hypothesis testing and a full comparison cycle before approval.
11. FR-11: The full comparison cycle must run `dep-eval run` on both `vela` and `stock-chef`, plus the combined `code-v1` graph-query suite.

## 9. Constraints

- The improvement target is the real `vela` project, not `vela-next-sim`.
- The real Vela binary must be built/run through `make dev`.
- The benchmarked binary path is `/home/geen/Documents/personal/vela/bin/vela`.
- Existing evidence from the 2026-04-22 benchmark runs is the starting baseline.
- This PRD defines process and validation artifacts only. It does not authorize product-code changes by itself.
- Changes must stay consistent with Vela’s graph-truth architecture in `docs/VELA_ARCHITECTURE.md`.

## 10. Non-Goals

- Replacing real `vela` with `vela-next-sim`
- Chasing the absolute lowest build time at the expense of dependency answer quality
- Copying simulator memory-answer behavior directly without proving operational viability
- Using `graphify` as the design target for query semantics
- Expanding the benchmark suite before the current reverse/path gaps are under control
- Implementing code changes as part of this PRD itself

## 11. Design and Technical Considerations

- Vela’s architecture is explicit: route first, retrieve deeply second. Improvements should strengthen graph-truth query behavior, not bypass it with opaque shortcuts.
- `vela-next-sim` suggests that fused scanner and semantic evidence can help quality, but its `261114 ms` prepare latency on the `vela` corpus is a warning, not a success story.
- `graphify` proves that fast preparation and incremental behavior are valuable, but its failure clusters show major gaps in positive dependency recovery and reverse dependency expansion.
- Unsupported incremental benchmarking in real `vela` is currently a product and benchmark-contract problem. It must be treated as a real gap, not waved away as tooling noise.

## 12. Phased Improvement Loop

### Phase 0: Baseline Lock

Objective: freeze the current benchmark evidence as the approval baseline.

Outputs:
- baseline metric snapshot for Layer 1 and Layer 2
- named comparison artifacts from the 2026-04-22 runs
- approved target thresholds for the next loop
- runnable loop assets in `docs/BENCHMARK_FIRST_LOOP_TARGETS.md` and `scripts/run-benchmark-loop.sh`

### Phase 1: Hypothesis Selection

Objective: pick one narrow, testable improvement target.

Rules:
- choose one primary family only
- define the exact metric expected to move
- define one acceptable regression budget and one unacceptable regression condition

Initial priority order:
1. reverse lookup precision/recall
2. path node precision/recall
3. evidence completeness
4. incremental benchmark contract support
5. build-time and graph-size optimization

### Phase 2: Minimal Product Change

Objective: implement the smallest real-Vela change that tests the chosen hypothesis.

Rules:
- no broad refactors unless the loop specifically justifies them
- no simulator-only changes counted as product progress
- preserve the real binary workflow through `make dev`

### Phase 3: Fast Validation

Objective: reject bad ideas cheaply before the full benchmark run.

Validation:
- rebuild the real binary with `make dev`
- run the smallest relevant benchmark slice or focused scenario against `/home/geen/Documents/personal/vela/bin/vela`
- verify the targeted family changed in the expected direction

### Phase 4: Full Comparative Benchmark

Objective: compare `vela` vs `vela-next-sim` vs `graphify` on the standard suites.

Validation:
- capture Layer 1 report
- capture Layer 2 report
- capture graph-query family breakdown
- record whether the change passed the balanced release gate

### Phase 5: Review and Decision

Objective: convert benchmark output into a product decision.

Decision outcomes:
- approve and keep
- reject and revert in next loop
- keep behind an experiment branch until the operational cost is fixed

### Phase 6: Cadence Repeat

Objective: maintain a steady improvement rhythm.

Cadence:
- per change: fast validation
- per approval candidate: full comparative benchmark
- per 2 to 3 accepted loops: refresh threshold targets and reprioritize the next weakest family

## 13. Validation Cadence

### Per Loop

1. Build real Vela with `make dev`.
2. Run focused validation for the targeted metric family.
3. If the focused check passes, run the full comparison suite.
4. Write a benchmark delta summary with pass/fail against the balanced release gate.

### Weekly or Milestone Review

1. Compare the current approved baseline against the prior one.
2. Re-rank the weakest families across reverse lookup, path quality, evidence completeness, and incremental behavior.
3. Decide whether the next loop should stay quality-first or spend one cycle paying down Layer 1 operational debt.

## 14. Open Questions

1. Should incremental benchmark-contract support be treated as a top-two priority even before path quality, since it currently zeros out a Layer 1 dimension?
2. Do we want one canonical benchmark summary file in `vela/docs/` for approved runs, or should approved summaries live in `dep-eval/results/` only?
3. Should the balanced release gate use absolute thresholds only, or require minimum delta improvements against the last approved baseline as well?

## 15. Recommended First Execution Loop After Approval

**Loop Theme:** Reverse lookup and path-quality hardening in real `vela`

**Why first:**
- It directly attacks the most obvious trust gap in the Vela family.
- It improves the part of the product users actually feel when asking architecture questions.
- It leverages the useful lesson from `vela-next-sim` without starting with the most dangerous operational experiment.

**First-loop objective:**
- improve reverse lookup precision/recall and path node precision/recall in real `vela`
- do so without regressing `build_time_ms` above `2000 ms` or lowering `layer1_weighted_score`

**First-loop validation steps:**
1. Run `make dev` in `/home/geen/Documents/personal/vela`.
2. Benchmark the real binary at `/home/geen/Documents/personal/vela/bin/vela`.
3. Compare against `vela-next-sim` and `graphify` using the same benchmark suites.
4. Review family-level deltas for reverse lookup and path-tracing before looking at aggregate scores.
5. Approve only if the balanced release gate passes.

**Loop 1 execution assets:**
- benchmark targets: `docs/BENCHMARK_FIRST_LOOP_TARGETS.md`
- repeatable runner: `scripts/run-benchmark-loop.sh`
- per-run outputs: `/home/geen/Documents/personal/dep-eval/results/vela-benchmark-loops/<timestamp>/`

## 16. Reference Inputs

- `docs/VELA_ARCHITECTURE.md`
- `docs/BENCHMARK_FIRST_LOOP_TARGETS.md`
- `scripts/run-benchmark-loop.sh`
- `/home/geen/Documents/personal/dep-eval/results/2026-04-22T19-47-51-916Z-vela-vela-vs-vela-next-sim-vs-graphify/report.json`
- `/home/geen/Documents/personal/dep-eval/results/2026-04-22T19-47-51-911Z-graph-query-code-v1-vela-vs-vela-next-sim-vs-graphify/graph-query-report.md`
