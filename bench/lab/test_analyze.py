import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import analyze  # noqa: E402


def row(task, arm, rep=0, ok=True, comparable=True, tests="pass", scope=True, outside=(), invariants=(), timed_out=False, cls="edit", tokens=(100, 10),
        calls=5, redundant=1, delivered=None):
    return {"task": task, "arm": arm, "rep": rep, "class": cls, "split": "development", "repo": "r", "comparable": comparable, "success": ok,
            "timed_out": timed_out, "incomparable_reasons": ["x"] if not comparable else [], "agent_error": None, "cost_usd": 0.01,
            "tokens_in": tokens[0], "tokens_out": tokens[1], "tool_calls": calls, "injected_chars": 0 if arm == "OFF" else 500, "hook_delivered": delivered,
            "oracle": {"tests": tests, "scope_ok": scope, "outside_scope": list(outside), "invariants": [{"ok": i} for i in invariants]},
            "progress": {"redundant": redundant, "stagnant_windows": 0}}


class AnalyzeTests(unittest.TestCase):
    def test_failure_kind_has_one_cause_in_priority_order(self):
        self.assertEqual(analyze.failure_kind(row("t", "a")), "ok")
        self.assertTrue(analyze.failure_kind(row("t", "a", ok=False, comparable=False)).startswith("nao_comparavel"))
        self.assertEqual(analyze.failure_kind(row("t", "a", ok=False, timed_out=True)), "timeout")
        self.assertEqual(analyze.failure_kind(row("t", "a", ok=False, tests="fail")), "teste_oculto_falhou")
        self.assertEqual(analyze.failure_kind(row("t", "a", ok=False, tests="error")), "erro_de_infra_do_oraculo")
        self.assertEqual(analyze.failure_kind(row("t", "a", ok=False, invariants=[False])), "invariante_quebrado")
        self.assertEqual(analyze.failure_kind(row("t", "a", ok=False, scope=False, outside=["tests/test_x.py"])), "escopo_so_testes_extras")
        self.assertEqual(analyze.failure_kind(row("t", "a", ok=False, scope=False, outside=["CHANGES.md"])), "escopo_fora_do_codigo_permitido")

    def test_unsolved_solved_and_split_tasks(self):
        rows = []
        for arm in ("OFF", "V2"):
            rows += [row("solved", arm), row("hopeless", arm, ok=False, tests="fail")]
        rows += [row("split", "OFF", ok=False, tests="fail"), row("split", "V2")]
        a = analyze.analyze(rows)
        self.assertEqual(a["unsolved_by_all"], ["hopeless"])
        self.assertEqual(a["solved_by_all"], ["solved"])
        self.assertEqual([s["task"] for s in a["split_arms"]], ["split"])
        self.assertEqual(a["split_arms"][0]["rates"], {"OFF": 0.0, "V2": 1.0})

    def test_non_comparable_runs_never_count_as_solved_or_unsolved(self):
        a = analyze.analyze([row("t", "OFF", ok=False, comparable=False), row("t", "V2", ok=False, comparable=False)])
        self.assertEqual(a["unsolved_by_all"], [])
        self.assertEqual(a["solved_by_all"], [])

    def test_hot_spots_and_class_breakdown(self):
        rows = [row("a", "OFF", redundant=4, delivered=None), row("a", "V2", redundant=1, delivered=True), row("b", "V2", cls="locate", delivered=False)]
        a = analyze.analyze(rows)
        self.assertEqual(a["hot_spots"]["redundantes_por_braco"], {"OFF": 4, "V2": 1})
        self.assertEqual(a["hot_spots"]["contexto_entregue_pct"], {"V2": 50.0})
        self.assertEqual(set(a["by_class"]), {"edit", "locate"})
        self.assertIn("Distribuição das falhas", analyze.render(a))

    def test_unsolved_task_is_flagged_for_oracle_review_in_the_text(self):
        text = analyze.render(analyze.analyze([row("hopeless", "OFF", ok=False, tests="fail")]))
        self.assertIn("hopeless", text)
        self.assertIn("verifique primeiro oráculo e escopo", text)


if __name__ == "__main__":
    unittest.main()
