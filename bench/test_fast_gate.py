import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import fast_gate


class TestFastGate(unittest.TestCase):
    def test_gate_detects_positive_and_negative_regressions(self):
        summaries = [
            {"stratum": "ALL POSITIVES", "engine": "v3", "gold_path_delivered": 35, "orientation_only": 0, "trust_label_conflicts": 0, "non_deterministic": 0},
            {"stratum": "ALL POSITIVES", "engine": "v3.6", "gold_path_delivered": 34, "orientation_only": 1, "trust_label_conflicts": 0, "non_deterministic": 0},
            {"stratum": "hard_negative", "engine": "v3", "non_empty": 14, "trust_label_conflicts": 0, "non_deterministic": 0},
            {"stratum": "hard_negative", "engine": "v3.6", "non_empty": 15, "trust_label_conflicts": 0, "non_deterministic": 0},
        ]
        failures = fast_gate.check_delivery_gate(summaries, "v3.6", "v3")
        self.assertEqual(len(failures), 3)
        self.assertTrue(any("gold_path_delivered" in f for f in failures))
        self.assertTrue(any("orientation_only" in f for f in failures))
        self.assertTrue(any("negative leaks" in f for f in failures))

    def test_gate_passes_when_candidate_matches_or_beats_baseline(self):
        summaries = [
            {"stratum": "ALL POSITIVES", "engine": "v3", "gold_path_delivered": 35, "orientation_only": 0, "trust_label_conflicts": 5, "non_deterministic": 0},
            {"stratum": "ALL POSITIVES", "engine": "v3.7", "gold_path_delivered": 37, "orientation_only": 0, "trust_label_conflicts": 0, "non_deterministic": 0},
            {"stratum": "negative", "engine": "v3", "non_empty": 0, "trust_label_conflicts": 0, "non_deterministic": 0},
            {"stratum": "negative", "engine": "v3.7", "non_empty": 0, "trust_label_conflicts": 0, "non_deterministic": 0},
        ]
        self.assertEqual(fast_gate.check_delivery_gate(summaries, "v3.7", "v3"), [])


if __name__ == "__main__":
    unittest.main()
