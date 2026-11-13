import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import fast_gate


def positive(engine, gold, top3=None, orient=0, tokens_per_gold=200.0, p95=10.0, stratum="ALL POSITIVES"):
    return {
        "stratum": stratum, "engine": engine, "gold_path_delivered": gold,
        "gold_in_first_3_lines": gold if top3 is None else top3, "orientation_only": orient,
        "trust_label_conflicts": 0, "non_deterministic": 0,
        "tokens_per_gold_delivered": tokens_per_gold, "p95_ms": p95,
    }


def negative(engine, non_empty, stratum="hard_negative"):
    return {"stratum": stratum, "engine": engine, "non_empty": non_empty,
            "trust_label_conflicts": 0, "non_deterministic": 0}


class TestFastGate(unittest.TestCase):
    def test_gate_detects_positive_and_negative_regressions(self):
        summaries = [
            positive("v3", 35),
            positive("v3.6", 34, orient=1),
            negative("v3", 14),
            negative("v3.6", 15),
        ]
        failures = fast_gate.check_delivery_gate(summaries, "v3.6", "v3")
        self.assertEqual(len(failures), 4, failures)  # gold, top-3 (mirrors gold here), orientation, negative leak
        self.assertTrue(any("gold_path_delivered" in f for f in failures))
        self.assertTrue(any("gold_in_first_3_lines" in f for f in failures))
        self.assertTrue(any("orientation_only" in f for f in failures))
        self.assertTrue(any("negative leaks" in f for f in failures))

    def test_gate_passes_when_candidate_matches_or_beats_baseline(self):
        summaries = [
            positive("v3", 35, top3=30),
            positive("v3.7", 37, top3=31),
            negative("v3", 0, stratum="negative"),
            negative("v3.7", 0, stratum="negative"),
        ]
        self.assertEqual(fast_gate.check_delivery_gate(summaries, "v3.7", "v3"), [])

    def test_missing_stratum_on_one_side_is_a_failure_not_a_silent_skip(self):
        summaries = [
            positive("v3", 35),
            positive("v3.7", 35),
            negative("v3", 0),
            negative("v3.7", 0),
            positive("v3.7", 9, stratum="eval"),  # only the candidate has this stratum
        ]
        failures = fast_gate.check_delivery_gate(summaries, "v3.7", "v3")
        self.assertTrue(any("eval" in f and "missing stratum" in f for f in failures), failures)

    def test_candidate_without_any_summary_is_a_failure(self):
        summaries = [positive("v3", 35), negative("v3", 0)]
        failures = fast_gate.check_delivery_gate(summaries, "v3.7", "v3")
        self.assertTrue(any("v3.7" in f and "no delivery summary" in f for f in failures), failures)

    def test_top3_regression_blocks_even_when_gold_path_is_equal(self):
        summaries = [positive("v3", 35, top3=34), positive("v3.7", 35, top3=33)]
        failures = fast_gate.check_delivery_gate(summaries, "v3.7", "v3")
        self.assertTrue(any("gold_in_first_3_lines" in f for f in failures), failures)

    def test_tokens_per_gold_regression_beyond_tolerance_blocks(self):
        summaries = [positive("v3", 35, tokens_per_gold=200.0), positive("v3.7", 35, tokens_per_gold=230.0)]
        failures = fast_gate.check_delivery_gate(summaries, "v3.7", "v3")
        self.assertTrue(any("tokens_per_gold_delivered" in f for f in failures), failures)

    def test_tokens_per_gold_within_tolerance_passes(self):
        summaries = [positive("v3", 35, tokens_per_gold=200.0), positive("v3.7", 35, tokens_per_gold=215.0)]
        self.assertEqual(fast_gate.check_delivery_gate(summaries, "v3.7", "v3"), [])

    def test_p95_latency_regression_beyond_factor_and_slack_blocks(self):
        summaries = [positive("v3", 35, p95=10.0), positive("v3.7", 35, p95=25.0)]
        failures = fast_gate.check_delivery_gate(summaries, "v3.7", "v3")
        self.assertTrue(any("p95_ms" in f for f in failures), failures)

    def test_p95_jitter_on_small_medians_is_tolerated(self):
        summaries = [positive("v3", 35, p95=3.0), positive("v3.7", 35, p95=7.0)]
        self.assertEqual(fast_gate.check_delivery_gate(summaries, "v3.7", "v3"), [])

    def test_missing_required_metric_is_reported_not_ignored(self):
        broken = positive("v3.7", 35)
        del broken["gold_in_first_3_lines"]
        failures = fast_gate.check_delivery_gate([positive("v3", 35), broken], "v3.7", "v3")
        self.assertTrue(any("gold_in_first_3_lines" in f and "missing" in f for f in failures), failures)


class TestLatencyAggregation(unittest.TestCase):
    def test_p95_is_the_median_across_repeated_runs_so_one_noisy_run_cannot_fail_the_gate(self):
        runs = [
            {"summary": [positive("v3", 35, p95=17.0), positive("v3.7", 35, p95=19.0)]},
            {"summary": [positive("v3", 35, p95=40.0), positive("v3.7", 35, p95=21.0)]},  # one noisy run
            {"summary": [positive("v3", 35, p95=18.0), positive("v3.7", 35, p95=22.0)]},
        ]
        merged = fast_gate.merge_latency_runs(runs)
        by = {s["engine"]: s for s in merged}
        self.assertEqual(by["v3"]["p95_ms"], 18.0)
        self.assertEqual(by["v3.7"]["p95_ms"], 21.0)
        self.assertEqual(by["v3"]["latency_runs"], 3)
        self.assertEqual(fast_gate.check_delivery_gate(merged, "v3.7", "v3"), [])


if __name__ == "__main__":
    unittest.main()
