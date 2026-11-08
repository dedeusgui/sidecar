import unittest

import diagnose_runs as d


def rec(**kw):
    base = {"agent": "claude", "model": "claude-haiku-5-5", "arm": "ON", "rep": 0, "case": "c", "category": "direct",
            "exit": 0, "timed_out": False, "stream_error": False, "edited_files": False, "malformed": False,
            "success": False, "comparable": True, "files": ["x.go"], "hook_delivered": True, "gold_in_ctx": True,
            "gold_ignored": False, "followed_noise": False}
    base.update(kw)
    return base


class ClassTests(unittest.TestCase):
    def test_success_has_no_failure_class(self):
        self.assertEqual(d.causes(rec(success=True)), [])

    def test_unauthorized_edit_is_an_experiment_violation_not_infra(self):
        self.assertEqual(d.causes(rec(edited_files=True)), ["experiment_violation"])

    def test_contaminated_run_is_an_experiment_violation(self):
        self.assertEqual(d.causes(rec(comparable=False)), ["experiment_violation"])

    def test_timeout_and_stream_errors_are_infra(self):
        self.assertEqual(d.causes(rec(timed_out=True)), ["infra_failure"])
        self.assertEqual(d.causes(rec(stream_error=True)), ["infra_failure"])

    def test_unreadable_answer_is_an_oracle_failure(self):
        self.assertEqual(d.causes(rec(malformed=True, files=None)), ["oracle_failure"])

    def test_gold_missing_from_delivered_context_is_retrieval_failure(self):
        self.assertEqual(d.causes(rec(gold_in_ctx=False)), ["retrieval_failure"])

    def test_no_context_delivered_on_case_with_gold_is_retrieval_failure(self):
        self.assertEqual(d.causes(rec(hook_delivered=False, gold_in_ctx=False)), ["retrieval_failure"])

    def test_wrong_answer_with_gold_in_context_is_task_failure(self):
        self.assertEqual(d.causes(rec(gold_ignored=True)), ["task_failure"])

    def test_off_arm_wrong_answer_is_task_failure(self):
        self.assertEqual(d.causes(rec(arm="OFF", hook_delivered=None, gold_in_ctx=None)), ["task_failure"])

    def test_negative_case_with_answer_is_false_positive(self):
        self.assertEqual(d.causes(rec(category="negative", files=["a.go"], hook_delivered=False)), ["false_positive"])

    def test_context_on_negative_case_is_flagged_even_when_answer_is_right(self):
        self.assertEqual(d.causes(rec(category="negative", files=[], success=True, hook_delivered=True)),
                         ["false_positive_injection"])
        self.assertEqual(d.causes(rec(category="negative", files=[], success=True, arm="OFF", hook_delivered=None)), [])

    def test_unexplained_failure_stays_unknown(self):
        self.assertEqual(d.causes(rec(arm="OFF", gold_in_ctx=None, files=[], hook_delivered=None)), ["unknown"])


class ExplainTests(unittest.TestCase):
    def test_gold_position_and_displacers_are_reported_without_causal_claims(self):
        doc = {"intent": {"kinds": ["locate"], "confidence": 0.6},
               "candidates": [{"Path": "other/a.go", "id": "other/a.go#Foo", "Score": 0.9, "Coverage": 0.5,
                               "Exact": False, "reason_selected": "own text matches"},
                              {"Path": "docs/guide.md", "id": "docs/guide.md#s", "Score": 0.7, "Coverage": 0.25,
                               "Exact": False, "Reason": "kept"},
                              {"Path": "H04/gold.py", "id": "H04/gold.py#f", "Score": 0.5, "Coverage": 0.1,
                               "Exact": False}],
               "evidence": [{"id": "other/a.go#Foo"}], "budget_tokens": 800, "token_estimate": 120,
               "timings_ms": {"lexical": 1.2}}
        s = d.summarize_explain(doc, {"H04/gold.py"})
        self.assertEqual(s["gold_rank_in_candidates"], {"H04/gold.py": 3})
        self.assertEqual([c["id"] for c in s["above_top5"]], ["other/a.go#Foo", "docs/guide.md#s"])  # non-gold in top 5
        self.assertEqual(s["evidence_ids"], ["other/a.go#Foo"])

    def test_missing_gold_is_reported_as_absent(self):
        s = d.summarize_explain({"candidates": [{"Path": "x.go"}]}, {"H04/gold.py"})
        self.assertIsNone(s["gold_rank_in_candidates"])


if __name__ == "__main__":
    unittest.main()
