"""Pure scoring primitives for a future offline golden evaluation runner.

Labels are explicit sets of typed tuples; ordered paths are tuples whose order is
significant. This module does not infer expected answers from Vela output.
"""

from collections import Counter


def score_set(expected, actual):
    """Score exact set membership; duplicates are ignored, not rewarded."""
    expected, actual = set(map(tuple, expected)), set(map(tuple, actual))
    tp = len(expected & actual)
    fp = len(actual - expected)
    fn = len(expected - actual)
    return {
        "tp": tp, "fp": fp, "fn": fn,
        "precision": tp / (tp + fp) if tp + fp else None,
        "recall": tp / (tp + fn) if tp + fn else None,
        "exact": expected == actual,
    }


def aggregate(results):
    """Micro-average case-level counts; undefined denominators remain null."""
    counts = Counter()
    for result in results:
        counts.update({k: result[k] for k in ("tp", "fp", "fn")})
    tp, fp, fn = (counts[k] for k in ("tp", "fp", "fn"))
    return {
        "tp": tp, "fp": fp, "fn": fn,
        "precision": tp / (tp + fp) if tp + fp else None,
        "recall": tp / (tp + fn) if tp + fn else None,
    }


def compare(base, candidate):
    """Compare only reports with the same corpus and matching unique case IDs."""
    identity = base.get("corpus")
    if not isinstance(identity, str) or not identity.startswith('sha256-v2:') or identity != candidate.get("corpus"):
        raise ValueError("corpus mismatch or unsupported recipe identity")
    for report in (base, candidate):
        digest = report.get('binary_sha256')
        if not isinstance(digest, str) or len(digest) != 64 or any(c not in '0123456789abcdef' for c in digest):
            raise ValueError('missing or invalid binary SHA-256')
    def cases(report):
        values = report["cases"]
        by_id = {case["id"]: case for case in values}
        if len(by_id) != len(values):
            raise ValueError("duplicate case ID")
        return by_id
    before, after = cases(base), cases(candidate)
    if before.keys() != after.keys():
        raise ValueError("case ID mismatch")
    changes = []
    for case_id in sorted(before):
        a, b = before[case_id], after[case_id]
        if a["expected"] != b["expected"]:
            raise ValueError("label mismatch: " + case_id)
        changes.append({"id": case_id, "base_pass": a["pass"],
                        "candidate_pass": b["pass"],
                        "delta_seconds": b["seconds"] - a["seconds"]})
    return changes
