# Benchmark Loop Hypotheses File

`scripts/run-benchmark-loop.sh` can auto-apply benchmark hypotheses when you provide `--hypotheses-file <path>`.

The file must be a JSON array. Each hypothesis needs enough metadata for ranking, focused validation, and safe revert.

## Required Fields

- `id`: stable hypothesis identifier
- `title`: short human-readable description
- `family`: one of the current loop priorities such as `reverse-lookup`, `path-quality`, or `path-tracing`
- `inspired_by`: the simulator lesson or operational gap this hypothesis borrows from
- `risk`: the regression to watch while testing the idea
- `apply`: shell command that makes the minimal code change
- `revert`: shell command that undoes the exact change if the focused gate fails
- `focused_gate.compare`: `any-improve` or `all-improve`
- `focused_gate.metrics`: query metrics to compare against the last accepted query benchmark

## Supported Focused Gate Metrics

- `summary.weighted_score`
- `summary.path_node_precision`
- `summary.path_node_recall`
- `family.reverse-lookup.mean_f1`
- `family.reverse-lookup.mean_precision`
- `family.reverse-lookup.mean_recall`
- `family.path-tracing.path_node_recall`

The focused gate is strict on safety even when the target metric improves:

- query `weighted_score` must not regress
- `success_rate` must stay `1`
- `unsupported_rate` must stay `0`

## Example

```json
[
  {
    "id": "reverse-lookup-import-expansion",
    "title": "Expand reverse lookup ranking with import edge evidence",
    "family": "reverse-lookup",
    "inspired_by": "vela-next-sim keeps more fused import evidence available during reverse expansion",
    "risk": "Could increase query latency by widening candidate sets.",
    "apply": "git apply scripts/hypotheses/reverse-lookup-import-expansion.patch",
    "revert": "git apply -R scripts/hypotheses/reverse-lookup-import-expansion.patch",
    "focused_gate": {
      "compare": "any-improve",
      "metrics": [
        "family.reverse-lookup.mean_f1"
      ]
    }
  },
  {
    "id": "path-quality-index-hops",
    "title": "Prefer index hop nodes in path reconstruction",
    "family": "path-quality",
    "inspired_by": "vela-next-sim resolves intermediate export hops more consistently",
    "risk": "Could overfit path reconstruction and hide missing graph truth.",
    "apply": "git apply scripts/hypotheses/path-quality-index-hops.patch",
    "revert": "git apply -R scripts/hypotheses/path-quality-index-hops.patch",
    "focused_gate": {
      "compare": "any-improve",
      "metrics": [
        "summary.path_node_precision",
        "summary.path_node_recall",
        "family.path-tracing.path_node_recall"
      ]
    }
  }
]
```

## Safety Notes

- The runner refuses to auto-apply hypotheses in a dirty git worktree unless you pass `--allow-dirty`.
- Keep each hypothesis minimal. One idea, one reversible change.
- If a hypothesis cannot be reverted with confidence, do not run it through the loop.
