import unittest

from golden_score import aggregate, compare, score_set


class ScoreTests(unittest.TestCase):
    def test_typed_facts_and_ordered_paths(self):
        expected = [("uses", "A", "B"), ("path", "A", "B", "C")]
        actual = [("calls", "A", "B"), ("path", "C", "B", "A")]
        s = score_set(expected, actual)
        self.assertEqual((s["tp"], s["fp"], s["fn"]), (0, 2, 2))
        self.assertFalse(s["exact"])

    def test_empty_and_duplicate_semantics(self):
        s = score_set([], [])
        self.assertTrue(s["exact"])
        self.assertIsNone(s["precision"])
        self.assertIsNone(s["recall"])
        s = score_set([("x",)], [("x",), ("x",), ("y",)])
        self.assertEqual((s["tp"], s["fp"], s["fn"]), (1, 1, 0))
        self.assertEqual(s["precision"], 0.5)
        self.assertEqual(s["recall"], 1.0)

    def test_micro_aggregation(self):
        s = aggregate([score_set([("a",)], [("a",)]),
                       score_set([("b",), ("c",)], [])])
        self.assertEqual((s["tp"], s["fp"], s["fn"]), (1, 0, 2))
        self.assertEqual(s["recall"], 1 / 3)

    def test_comparison_rejects_incomparable_reports(self):
        base = {"corpus": "sha256-v2:abc", "binary_sha256": "a" * 64, "cases": [{"id": "x", "expected": ["a"],
                 "pass": True, "seconds": 1.0}]}
        candidate = {"corpus": "sha256-v2:abc", "binary_sha256": "b" * 64, "cases": [{"id": "x", "expected": ["a"],
                      "pass": False, "seconds": 2.5}]}
        self.assertEqual(compare(base, candidate)[0]["delta_seconds"], 1.5)
        candidate["cases"][0]["expected"] = ["b"]
        with self.assertRaisesRegex(ValueError, "label mismatch"):
            compare(base, candidate)
        candidate["corpus"] = "v2"
        with self.assertRaisesRegex(ValueError, "corpus mismatch"):
            compare(base, candidate)
        candidate["corpus"] = "sha256-v2:abc"
        del candidate['binary_sha256']
        with self.assertRaisesRegex(ValueError, 'binary'):
            compare(base, candidate)
        candidate['binary_sha256'] = 'b' * 64
        candidate["cases"] *= 2
        with self.assertRaisesRegex(ValueError, "duplicate"):
            compare(base, candidate)


if __name__ == "__main__":
    unittest.main()
