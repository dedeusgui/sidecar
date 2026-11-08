"""Testes do placar: negativos, ausência, duplicatas, baseline incompatível e agentes reais."""

import contextlib
import io
import json
import pathlib
import tempfile
import unittest

import scorecard as sc

HW = {"cpu_model": "cpu", "cpus": 4, "go": "go1.24", "goos": "linux", "goarch": "amd64", "date": "2026-11-08"}
CASES = {"cases": [
    {"id": "p1", "split": "dev", "prompt": "a"},
    {"id": "p2", "split": "dev", "prompt": "b"},
    {"id": "n1", "split": "dev", "negative": True, "prompt": "c"},
]}


def out(method, case, negative=False, hit5=False, empty=True, failed=None, latency=10.0, tokens=100):
    o = {"method": method, "case": case, "split": "dev", "negative": negative, "hit_at_5": hit5,
         "empty": empty, "latency_ms": latency, "context_tokens": tokens}
    if failed is not None:
        o["failed_checks"] = failed
    return o


def base_outcomes(method="m1"):
    return [out(method, "p1", hit5=True, latency=10.0, tokens=100),
            out(method, "p2", failed=["gold: 0/1 no top 10"], latency=30.0, tokens=300),
            out(method, "n1", negative=True, empty=False, latency=20.0, tokens=200)]


def agent_rec(**kw):
    rec = {"agent": "claude", "model": "haiku", "arm": "OFF", "case": "R01-1", "rep": 0, "success": True,
           "timed_out": False, "wall_s": 10.0, "cost_usd": 0.02, "transport": "none"}
    rec.update(kw)
    return rec


class Base(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.dir = pathlib.Path(tmp.name)

    def write(self, name, content):
        path = self.dir / name
        path.write_text(content if isinstance(content, str) else json.dumps(content), encoding="utf-8")
        return str(path)

    def cases_file(self, name="cases.json", obj=CASES):
        return self.write(name, obj)

    def results_file(self, name, outcomes, methods=("m1",), hardware=HW):
        return self.write(name, {"hardware": hardware, "methods": list(methods), "outcomes": outcomes})

    def jsonl(self, name, records):
        return self.write(name, "".join(json.dumps(r) + "\n" for r in records))

    def load(self, outcomes, methods=("m1",), cases_obj=CASES):
        index, _ = sc.load_cases(self.cases_file(obj=cases_obj))
        return sc.load_results(self.results_file("r.json", outcomes, methods), index)

    def assert_load_error(self, pattern, outcomes, methods=("m1",), cases_obj=CASES):
        with self.assertRaisesRegex(sc.DataError, pattern):
            self.load(outcomes, methods, cases_obj)

    def run_cli(self, argv):
        out_buf, err_buf = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out_buf), contextlib.redirect_stderr(err_buf):
            code = sc.main(argv)
        return code, out_buf.getvalue(), err_buf.getvalue()


class MetricTests(Base):
    def test_counts_checks_hits_and_false_positives(self):
        m = sc.metrics(self.load(base_outcomes())["groups"][("m1", "dev")])
        self.assertEqual((m["n_pos"], m["n_neg"]), (2, 1))
        self.assertEqual(m["passed"], (2, 3))  # n1 sem failed_checks conta como aprovado
        self.assertEqual(m["hit5"], (1, 2))
        self.assertEqual(m["fp"], (1, 1))
        self.assertEqual(m["latency"], 20.0)
        self.assertEqual(m["tokens"], 200)

    def test_negative_with_empty_context_is_not_false_positive(self):
        outs = base_outcomes()
        outs[2]["empty"] = True
        m = sc.metrics(self.load(outs)["groups"][("m1", "dev")])
        self.assertEqual(m["fp"], (0, 1))

    def test_no_gold_marks_negative_as_in_external_loader(self):
        cases = {"cases": [CASES["cases"][0], CASES["cases"][1], {"id": "n1", "split": "dev", "no_gold": True}]}
        m = sc.metrics(self.load(base_outcomes(), cases_obj=cases)["groups"][("m1", "dev")])
        self.assertEqual((m["n_pos"], m["n_neg"]), (2, 1))
        self.assertEqual(m["fp"], (1, 1))

    def test_missing_negative_cases_give_nd_not_zero(self):
        outs = [o for o in base_outcomes() if not o["negative"]]
        index, _ = sc.load_cases(self.cases_file(obj={"cases": CASES["cases"][:2]}))
        src = sc.load_results(self.results_file("r.json", outs), index)
        m = sc.metrics(src["groups"][("m1", "dev")])
        self.assertIsNone(m["fp"])
        self.assertEqual(sc.fmt_frac(m["fp"]), "n/d")


class ValidationTests(Base):
    def test_missing_or_null_latency_is_error_not_zero(self):
        outs = base_outcomes()
        del outs[0]["latency_ms"]
        self.assert_load_error("latency_ms", outs)
        outs = base_outcomes()
        outs[0]["latency_ms"] = None
        self.assert_load_error("latency_ms", outs)

    def test_negative_latency_is_error(self):
        outs = base_outcomes()
        outs[0]["latency_ms"] = -1
        self.assert_load_error("latency_ms", outs)

    def test_outcome_without_negative_flag_is_error(self):
        outs = base_outcomes()
        del outs[2]["negative"]
        self.assert_load_error("'negative'", outs)

    def test_polarity_divergent_from_cases_is_error(self):
        outs = base_outcomes()
        outs[2]["negative"] = False
        self.assert_load_error("polaridade", outs)

    def test_no_gold_case_recorded_as_positive_is_error(self):
        # Resultado gerado por --cases ignoraria no_gold; a divergência precisa aparecer, não virar zero.
        cases = {"cases": [CASES["cases"][0], CASES["cases"][1], {"id": "n1", "split": "dev", "no_gold": True}]}
        outs = base_outcomes()
        outs[2]["negative"] = False
        self.assert_load_error("polaridade", outs, cases_obj=cases)

    def test_non_boolean_polarity_flag_is_error(self):
        cases = {"cases": CASES["cases"][:2] + [{"id": "n1", "split": "dev", "negative": "sim"}]}
        self.assert_load_error("'negative'", base_outcomes(), cases_obj=cases)

    def test_method_with_missing_case_is_error(self):
        self.assert_load_error("incompletos", base_outcomes()[:2])

    def test_unknown_case_is_error(self):
        self.assert_load_error("ausente de CASES_JSON", base_outcomes() + [out("m1", "zz")])

    def test_failed_checks_must_be_list_of_text(self):
        outs = base_outcomes()
        outs[1]["failed_checks"] = "gold: 0/1"
        self.assert_load_error("failed_checks", outs)

    def test_missing_hardware_key_is_error(self):
        index, _ = sc.load_cases(self.cases_file())
        hw = {k: v for k, v in HW.items() if k != "goos"}
        with self.assertRaisesRegex(sc.DataError, "hardware sem"):
            sc.load_results(self.results_file("r.json", base_outcomes(), hardware=hw), index)

    def test_invalid_json_is_error(self):
        index, _ = sc.load_cases(self.cases_file())
        with self.assertRaisesRegex(sc.DataError, "JSON inválido"):
            sc.load_results(self.write("r.json", "{"), index)

    def test_cases_without_split_or_empty_list_is_error(self):
        with self.assertRaisesRegex(sc.DataError, "'split'"):
            sc.load_cases(self.cases_file(obj={"cases": [{"id": "p1"}]}))
        with self.assertRaisesRegex(sc.DataError, "sem casos"):
            sc.load_cases(self.cases_file(obj={"cases": []}))


class DuplicateTests(Base):
    def test_duplicate_outcome_is_error(self):
        self.assert_load_error("duplicata", base_outcomes() + [base_outcomes()[0]])

    def test_duplicate_case_id_is_error(self):
        with self.assertRaisesRegex(sc.DataError, "id duplicado"):
            sc.load_cases(self.cases_file(obj={"cases": CASES["cases"] + [CASES["cases"][0]]}))

    def test_duplicate_method_name_is_error(self):
        self.assert_load_error("únicos", base_outcomes(), methods=("m1", "m1"))

    def test_same_agent_run_in_two_files_is_error(self):
        seen = set()
        sc.load_agent_runs(self.jsonl("a.jsonl", [agent_rec()]), seen)
        with self.assertRaisesRegex(sc.DataError, "duplicada"):
            sc.load_agent_runs(self.jsonl("b.jsonl", [agent_rec()]), seen)

    def test_duplicate_label_is_cli_error(self):
        r, c = self.results_file("r.json", base_outcomes()), self.cases_file()
        with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
            sc.main(["--suite", "s", r, c, "--baseline", "s", r, c])


class AgentRunTests(Base):
    def test_unknown_cost_is_explicit_and_mean_uses_only_informed(self):
        path = self.jsonl("a.jsonl", [
            agent_rec(case="A", cost_usd=0.02, wall_s=1.0),
            agent_rec(case="B", cost_usd=0.04, wall_s=2.0, success=False),
            agent_rec(case="C", cost_usd=None, wall_s=3.0, success=False, timed_out=True),
        ])
        records, _ = sc.load_agent_runs(path, set())
        (row,) = sc.summarize_agents(records)
        self.assertEqual((row["n"], row["success"], row["timeouts"]), (3, 1, 1))
        self.assertEqual(row["wall_mean"], 2.0)
        self.assertEqual((row["cost_known"], row["cost_unknown"]), (2, 1))
        self.assertAlmostEqual(row["cost_mean"], 0.03)

    def test_all_unknown_cost_renders_nd(self):
        records, _ = sc.load_agent_runs(self.jsonl("a.jsonl", [agent_rec(cost_usd=None)]), set())
        row = sc.summarize_agents(records)[0]
        self.assertIsNone(row["cost_mean"])
        self.assertIn("| 0 | n/d | 1 |", "\n".join(sc.agent_table([row])))

    def test_missing_verdict_is_not_a_task_failure(self):
        records, _ = sc.load_agent_runs(self.jsonl("a.jsonl", [agent_rec(success=None)]), set())
        row = sc.summarize_agents(records)[0]
        self.assertEqual((row["success"], row["judged"], row["n"]), (0, 0, 1))
        self.assertIn("| n/d | 1 |", "\n".join(sc.agent_table([row])))
        self.assertIsNone(row["wall_per_success"])
        self.assertIsNone(row["cost_per_success"])

    def test_legacy_infrastructure_error_is_unknown_but_invalid_patch_is_failure(self):
        rows = [agent_rec(case="infra", success=False, exit=1, oracle={"status": "not_run"}),
                agent_rec(case="scope", success=False, exit=0, oracle={"status": "not_run"})]
        records, _ = sc.load_agent_runs(self.jsonl("a.jsonl", rows), set())
        self.assertIsNone(records[0]["success"])
        self.assertIs(records[1]["success"], False)

    def test_per_success_counts_failures_and_refuses_incomplete_costs(self):
        records, _ = sc.load_agent_runs(self.jsonl("a.jsonl", [
            agent_rec(case="a", wall_s=10, cost_usd=0.2),
            agent_rec(case="b", success=False, wall_s=20, cost_usd=0.1),
        ]), set())
        row = sc.summarize_agents(records)[0]
        self.assertEqual(row["wall_per_success"], 30)
        self.assertAlmostEqual(row["cost_per_success"], 0.3)
        records[1]["cost_usd"] = None
        self.assertIsNone(sc.summarize_agents(records)[0]["cost_per_success"])

    def test_missing_cost_key_is_error(self):
        rec = agent_rec()
        del rec["cost_usd"]
        with self.assertRaisesRegex(sc.DataError, "'cost_usd'"):
            sc.load_agent_runs(self.jsonl("a.jsonl", [rec]), set())

    def test_cost_must_be_number_or_null(self):
        with self.assertRaisesRegex(sc.DataError, "'cost_usd'"):
            sc.load_agent_runs(self.jsonl("a.jsonl", [agent_rec(cost_usd="0.02")]), set())

    def test_invalid_arm_is_error(self):
        with self.assertRaisesRegex(sc.DataError, "'arm'"):
            sc.load_agent_runs(self.jsonl("a.jsonl", [agent_rec(arm="X")]), set())

    def test_broken_json_line_reports_line_number(self):
        path = self.write("a.jsonl", json.dumps(agent_rec()) + "\n{bad\n")
        with self.assertRaisesRegex(sc.DataError, r":2: JSON inválido"):
            sc.load_agent_runs(path, set())

    def test_empty_file_is_error(self):
        with self.assertRaisesRegex(sc.DataError, "sem registros"):
            sc.load_agent_runs(self.write("a.jsonl", "\n"), set())

    def test_agent_section_comes_first_and_is_optional(self):
        code, text, _ = self.run_cli(["--agent-runs", self.jsonl("a.jsonl", [agent_rec()])])
        self.assertEqual(code, 0)
        self.assertLess(text.index("## 1. Agentes reais"), text.index("## 2. Retrieval offline"))
        self.assertIn("Nenhum `--suite` informado", text)

    def test_agent_rounds_are_not_mixed_even_with_same_case(self):
        first = self.jsonl("a.jsonl", [agent_rec(success=True)])
        second = self.jsonl("b.jsonl", [agent_rec(success=False)])
        code, text, _ = self.run_cli(["--agent-runs", first, "--agent-runs", second])
        self.assertEqual(code, 0)
        self.assertEqual(text.count("### Rodada"), 2)
        self.assertIn("1/1 (100%)", text)
        self.assertIn("0/1 (0%)", text)


class BaselineTests(Base):
    def test_different_cases_file_is_incompatible_and_shows_no_delta(self):
        ra = self.results_file("ra.json", base_outcomes())
        rb = self.results_file("rb.json", base_outcomes())
        ca = self.cases_file("ca.json", CASES)
        cb = self.cases_file("cb.json", {**CASES, "nota": "outra cópia"})
        code, text, _ = self.run_cli(["--suite", "nova", ra, ca, "--baseline", "antiga", rb, cb])
        self.assertEqual(code, 0)
        self.assertIn("Incompatível: nenhum delta calculado", text)
        self.assertIn("CASES_JSON difere", text)
        self.assertNotIn("| Grupo | Métrica", text)

    def test_different_method_sets_are_incompatible(self):
        ra = self.results_file("ra.json", base_outcomes("m1"))
        rb = self.results_file("rb.json", base_outcomes("m1") + base_outcomes("m2"), methods=("m1", "m2"))
        c = self.cases_file()
        _, text, _ = self.run_cli(["--suite", "nova", rb, c, "--baseline", "antiga", ra, c])
        self.assertIn("grupos (método, split) diferentes", text)
        self.assertNotIn("| Grupo | Métrica", text)

    def test_compatible_baseline_reports_per_case_changes(self):
        after = base_outcomes()
        after[0]["failed_checks"] = ["gold: 0/1 no top 10"]  # p1: aprovado -> reprovado
        del after[1]["failed_checks"]                         # p2: reprovado -> aprovado
        ra = self.results_file("ra.json", base_outcomes())
        rb = self.results_file("rb.json", after)
        c = self.cases_file()
        code, text, _ = self.run_cli(["--suite", "nova", rb, c, "--baseline", "antiga", ra, c])
        self.assertEqual(code, 0)
        self.assertNotIn("Incompatível", text)
        self.assertIn("Casos com check alterado: 2 de 3.", text)
        self.assertIn("| m1 / dev | p1 | piorou | gold: 0/1 no top 10 | - |", text)
        self.assertIn("| m1 / dev | p2 | melhorou | - | gold: 0/1 no top 10 |", text)
        self.assertIn("| Grupo | Métrica", text)

    def test_compare_flags_divergent_ids_and_polarity_even_with_same_hash(self):
        def row(case, negative=False):
            return {"case": case, "negative": negative, "failed": [], "hit5": False,
                    "empty": True, "latency": 1.0, "tokens": 1}

        base = {"cases_sha": "same", "groups": {("m1", "dev"): [row("p1"), row("p2")]}}
        other_ids = {"cases_sha": "same", "groups": {("m1", "dev"): [row("p1"), row("p3")]}}
        other_pol = {"cases_sha": "same", "groups": {("m1", "dev"): [row("p1"), row("p2", negative=True)]}}
        self.assertEqual(sc.compare(base, base), [])
        self.assertTrue(any("IDs diferentes" in r for r in sc.compare(base, other_ids)))
        self.assertTrue(any("polaridade diferente" in r for r in sc.compare(base, other_pol)))


class CliTests(Base):
    def test_no_inputs_is_cli_error(self):
        with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
            sc.main([])

    def test_baseline_without_suite_is_cli_error(self):
        r, c = self.results_file("r.json", base_outcomes()), self.cases_file()
        with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
            sc.main(["--baseline", "b", r, c])

    def test_out_cannot_overwrite_an_input(self):
        r, c = self.results_file("r.json", base_outcomes()), self.cases_file()
        before = pathlib.Path(r).read_bytes()
        with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
            sc.main(["--suite", "s", r, c, "--out", r])
        self.assertEqual(pathlib.Path(r).read_bytes(), before)

    def test_data_error_returns_1_with_empty_stdout(self):
        code, text, err = self.run_cli(["--suite", "s", self.write("bad.json", "{"), self.cases_file()])
        self.assertEqual(code, 1)
        self.assertEqual(text, "")
        self.assertTrue(err.startswith("erro:"))

    def test_out_file_matches_stdout(self):
        target = self.dir / "report.md"
        code, text, _ = self.run_cli(["--agent-runs", self.jsonl("a.jsonl", [agent_rec()]), "--out", str(target)])
        self.assertEqual(code, 0)
        self.assertEqual(target.read_text(encoding="utf-8"), text)


if __name__ == "__main__":
    unittest.main()
