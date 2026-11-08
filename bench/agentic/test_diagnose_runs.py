import unittest

import diagnose_runs as d


def rec(**kw):
    base = {"agent": "claude", "model": "claude-haiku-5-5", "arm": "ON", "rep": 0, "case": "c", "category": "direct",
            "exit": 0, "timed_out": False, "stream_error": False, "edited_files": False, "malformed": False,
            "success": False, "files": ["x.go"], "hook_delivered": True, "gold_in_ctx": True, "gold_ignored": False,
            "followed_noise": False}
    base.update(kw)
    return base


class DiagnoseTests(unittest.TestCase):
    def test_success_has_no_causes(self):
        self.assertEqual(d.causes(rec(success=True)), [])

    def test_infra_failures_are_not_blamed_on_retrieval(self):
        self.assertEqual(d.causes(rec(timed_out=True)), ["infra_failure"])
        self.assertEqual(d.causes(rec(edited_files=True)), ["infra_failure"])

    def test_malformed_answer_is_separate_from_wrong_answer(self):
        self.assertEqual(d.causes(rec(malformed=True, files=None)), ["malformed_answer"])

    def test_missing_gold_in_delivered_context_is_missing_evidence(self):
        self.assertEqual(d.causes(rec(gold_in_ctx=False)), ["missing_evidence"])

    def test_gold_in_context_but_ignored_is_ranking_or_use(self):
        self.assertEqual(d.causes(rec(gold_ignored=True)), ["ranking_or_use"])

    def test_no_context_on_non_negative_case_is_injection_candidate_not_proof(self):
        self.assertEqual(d.causes(rec(hook_delivered=False, gold_in_ctx=False)), ["injection_failure", "missing_evidence"])

    def test_negative_case_with_answer_is_false_positive(self):
        self.assertEqual(d.causes(rec(category="negative", files=["a.go"], hook_delivered=False)), ["false_positive"])

    def test_context_on_negative_case_is_flagged_even_when_answer_is_right(self):
        self.assertEqual(d.causes(rec(category="negative", files=[], success=True, hook_delivered=True)),
                         ["false_positive_injection"])
        self.assertEqual(d.causes(rec(category="negative", files=[], success=True, arm="OFF", hook_delivered=None)), [])

    def test_unexplained_failure_stays_unknown(self):
        self.assertEqual(d.causes(rec(arm="OFF", gold_in_ctx=None, files=[])), ["unknown"])


if __name__ == "__main__":
    unittest.main()
