import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest
from types import SimpleNamespace

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import lab  # noqa: E402

IDENT = ["-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"]
ORACLE = """import sys
sys.path.insert(0, '.')
from calc import add
ok = add(2, 3) == 5
print('ORACLE-PASS' if ok else 'ORACLE-FAIL')
sys.exit(0 if ok else 1)
"""
REF = """--- a/calc.py
+++ b/calc.py
@@ -1,2 +1,2 @@
 def add(a, b):
-    return a - b
+    return a + b
"""


def git(cwd, *a):
    return subprocess.run(["git", *IDENT, *a], cwd=cwd, check=True, capture_output=True, text=True).stdout.strip()


class LabCase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        t = pathlib.Path(self.tmp.name)
        src = t / "src"
        src.mkdir()
        git(src, "init", "-q")
        (src / "calc.py").write_text("def add(a, b):\n    return a - b\n")
        (src / "other.py").write_text("X = 1\n")
        git(src, "add", "-A")
        git(src, "commit", "-q", "-m", "c1")
        self.commit = git(src, "rev-parse", "HEAD")
        self.corpus = t / "corpus"
        (self.corpus / "tasks").mkdir(parents=True)
        (self.corpus / "repos.json").write_text(json.dumps({"repos": {"toy": {
            "commit": self.commit, "source": "file://" + str(src), "license": "MIT"}}}))
        self.old = (lab.CORPUS, lab.TASKS, lab.REPOS_JSON, lab.LEDGER)
        lab.CORPUS, lab.TASKS = self.corpus, self.corpus / "tasks"
        lab.REPOS_JSON, lab.LEDGER = self.corpus / "repos.json", self.corpus / "holdout_ledger.jsonl"
        os.environ["SIDECAR_LAB_CACHE"] = str(t / "cache")
        self.addCleanup(lambda: setattr(lab, "CORPUS", self.old[0]) or setattr(lab, "TASKS", self.old[1])
                        or setattr(lab, "REPOS_JSON", self.old[2]) or setattr(lab, "LEDGER", self.old[3]))
        self.addCleanup(self.tmp.cleanup)

    def make_task(self, tid="T-EDIT-01", split="development", cls="edit", patch=True, **over):
        d = lab.TASKS / tid
        (d / "oracle").mkdir(parents=True)
        (d / "oracle" / "o.py").write_text(ORACLE)
        task = {"schema": "lab-task/1", "id": tid, "split": split, "class": cls, "repo": "toy", "commit": self.commit,
                "prompt": "A função add subtrai; corrija para somar.",
                "oracle": {"kind": "tests", "files": {"zz_oracle.py": "o.py"}, "command": ["python3", "zz_oracle.py"],
                           "require_output": ["ORACLE-(PASS|FAIL)"], "timeout_s": 60},
                "scope": {"allowed_paths": ["calc.py"],
                          "invariants": [{"type": "regex_present", "path": "calc.py", "pattern": "def add\\(a, b\\)"}]}}
        task.update(over)
        (d / "task.json").write_text(json.dumps(task))
        if patch:
            (d / "reference.patch").write_text(REF)
        return d

    def run_args(self, **kw):
        base = dict(agent="scripted", scripted_behaviour="reference", arms="OFF,V1,V2", tasks=None,
                    splits="development,validation", reps=1, seed=3, out_dir=str(pathlib.Path(self.tmp.name) / "out"),
                    model=None, sidecar=None, credentials=None, max_budget=0.1, max_total_usd=None, timeout=60,
                    tools_allow="Read", yes_spend=False, final=False, allow_unvalidated=True)
        base.update(kw)
        return SimpleNamespace(**base)


class SchemaTests(LabCase):
    def test_valid_task_has_no_errors(self):
        d = self.make_task()
        self.assertEqual(lab.validate_task_schema(json.loads((d / "task.json").read_text()), d), [])

    def test_rejects_unpinned_commit_missing_oracle_file_and_vacuous_oracle(self):
        d = self.make_task(commit="0" * 40)
        task = json.loads((d / "task.json").read_text())
        task["oracle"]["require_output"] = []
        task["oracle"]["files"]["zz.py"] = "ausente.py"
        errs = lab.validate_task_schema(task, d)
        self.assertTrue(any("commit" in e for e in errs))
        self.assertTrue(any("ausente" in e for e in errs))
        self.assertTrue(any("require_output" in e for e in errs))

    def test_unsafe_oracle_destination_and_ambiguous_without_underspec(self):
        d = self.make_task(cls="ambiguous")
        task = json.loads((d / "task.json").read_text())
        task["oracle"]["files"] = {"../escape.py": "o.py"}
        errs = lab.validate_task_schema(task, d)
        self.assertTrue(any("inseguro" in e for e in errs))
        self.assertTrue(any("underspec" in e for e in errs))

    def test_requires_reference_patch_for_edit(self):
        d = self.make_task(patch=False)
        errs = lab.validate_task_schema(json.loads((d / "task.json").read_text()), d)
        self.assertTrue(any("reference.patch" in e for e in errs))


class OracleTests(LabCase):
    def test_validate_task_proves_fail_before_pass_after(self):
        self.make_task()
        r = lab.validate_task("T-EDIT-01")
        self.assertTrue(r["ok"], r)
        self.assertEqual(r["base_verdict"], "fail")
        self.assertEqual(lab.verdict(r["reference"]), "pass")

    def test_validate_task_fails_when_oracle_already_passes_on_base(self):
        d = self.make_task()
        (d / "oracle" / "o.py").write_text("print('ORACLE-PASS')\n")  # oráculo vacuamente verdadeiro
        r = lab.validate_task("T-EDIT-01")
        self.assertFalse(r["ok"])

    def test_oracle_without_marker_is_infrastructure_error_not_fail(self):
        d = self.make_task()
        (d / "oracle" / "o.py").write_text("import sys\nsys.exit(1)\n")  # nunca imprime o marcador
        r = lab.validate_task("T-EDIT-01")
        self.assertEqual(r["base"]["tests"], "error")
        self.assertEqual(r["base_verdict"], "error")
        self.assertFalse(r["ok"])

    def test_invariant_and_scope_violations_fail_verdict(self):
        task_d = self.make_task()
        task = json.loads((task_d / "task.json").read_text())
        with tempfile.TemporaryDirectory() as w:
            work = lab.prepare_workspace(task, pathlib.Path(w) / "w")
            lab.apply_patch(work, task_d / "reference.patch")
            (work / "other.py").write_text("X = 2\n")
            res = lab.run_oracle(task, task_d, work)
            self.assertEqual(res["tests"], "pass")
            self.assertEqual(res["outside_scope"], ["other.py"])
            self.assertEqual(lab.verdict(res), "fail")

    def test_oracle_files_removed_after_run(self):
        d = self.make_task()
        task = json.loads((d / "task.json").read_text())
        with tempfile.TemporaryDirectory() as w:
            work = lab.prepare_workspace(task, pathlib.Path(w) / "w")
            lab.run_oracle(task, d, work)
            self.assertFalse((work / "zz_oracle.py").exists())

    def test_answer_oracle(self):
        self.make_task("T-LOC-01", cls="locate", patch=False,
                       oracle={"kind": "answer", "answer_must_mention": ["calc.py"]},
                       scope={"allowed_paths": []})
        d = lab.TASKS / "T-LOC-01"
        task = json.loads((d / "task.json").read_text())
        with tempfile.TemporaryDirectory() as w:
            work = lab.prepare_workspace(task, pathlib.Path(w) / "w")
            self.assertEqual(lab.run_oracle(task, d, work, "está em CALC.PY")["tests"], "pass")
            self.assertEqual(lab.run_oracle(task, d, work, "não sei")["tests"], "fail")
        self.assertTrue(lab.validate_task("T-LOC-01")["ok"])

    def test_setup_artifacts_are_not_scope_violations(self):
        self.make_task(setup=[["python3", "-c", "import pathlib; pathlib.Path('deps').mkdir(); pathlib.Path('deps/x').write_text('1')"]])
        d = lab.TASKS / "T-EDIT-01"
        task = json.loads((d / "task.json").read_text())
        with tempfile.TemporaryDirectory() as w:
            work = lab.prepare_workspace(task, pathlib.Path(w) / "w")
            self.assertEqual(lab.changed_paths(work), [])
            (work / "deps" / "y").write_text("2")
            (work / "stray.txt").write_text("x")
            self.assertEqual(lab.changed_paths(work), ["stray.txt"])


class RunnerTests(LabCase):
    def test_no_upstream_history_or_remote_in_checkout(self):
        d = self.make_task()
        task = json.loads((d / "task.json").read_text())
        with tempfile.TemporaryDirectory() as w:
            work = lab.prepare_workspace(task, pathlib.Path(w) / "w")
            self.assertEqual(git(work, "rev-list", "--count", "HEAD"), "1")
            self.assertEqual(git(work, "remote"), "")

    def test_agent_never_sees_oracle_files(self):
        d = self.make_task()
        seen = {}

        class Spy(lab.ScriptedAgent):
            def run(self, task, tdir, work, arm, env, prompt):
                seen["files"] = sorted(p.name for p in pathlib.Path(work).rglob("*") if ".git" not in p.parts)
                seen["env"] = sorted(env)
                return super().run(task, tdir, work, arm, env, prompt)

        task = json.loads((d / "task.json").read_text())
        with tempfile.TemporaryDirectory() as home:
            rec = lab.execute_one(Spy("reference"), task, d, {"name": "OFF", "engine": None, "env": {}}, 0, None, home)
        self.assertNotIn("zz_oracle.py", seen["files"])
        self.assertNotIn("reference.patch", seen["files"])
        self.assertEqual(rec["verdict"], "pass")
        self.assertEqual(rec["evidence"], "simulated")

    def test_behaviours_noop_scope_crash(self):
        d = self.make_task()
        task = json.loads((d / "task.json").read_text())
        arm = {"name": "V2", "engine": "v2", "env": {}}
        with tempfile.TemporaryDirectory() as home:
            noop = lab.execute_one(lab.ScriptedAgent("noop"), task, d, arm, 0, None, home)
            stray = lab.execute_one(lab.ScriptedAgent("scope_violation"), task, d, arm, 0, None, home)
            crash = lab.execute_one(lab.ScriptedAgent("crash"), task, d, arm, 0, None, home)
        self.assertEqual(noop["verdict"], "fail")
        self.assertFalse(stray["oracle"]["scope_ok"])
        self.assertFalse(crash["comparable"])
        self.assertIn("RuntimeError", crash["agent_error"])
        self.assertFalse(crash["success"])

    def test_schedule_is_balanced_and_deterministic(self):
        arms = lab.parse_arms("OFF,V1,V2")
        ids = [f"T{i}" for i in range(9)]
        o1, o2 = lab.schedule(ids, arms, 1, 5), lab.schedule(ids, arms, 1, 5)
        self.assertEqual(o1, o2)
        firsts = [o1[i][1]["name"] for i in range(0, len(o1), 3)]
        self.assertEqual({firsts.count(a) for a in ("OFF", "V1", "V2")}, {3})
        self.assertEqual(len(o1), 27)

    def test_feature_arms_set_sidecar_features(self):
        arms = lab.parse_arms("OFF,V2,V2+brief,V2+adaptive+brief")
        self.assertEqual(arms[2]["env"], {"SIDECAR_FEATURES": "brief"})
        self.assertEqual(arms[3]["env"], {"SIDECAR_FEATURES": "adaptive,brief"})
        self.assertEqual(arms[3]["engine"], "v2")
        self.assertEqual(lab.parse_arms("V2+groom+impact")[0]["env"], {"SIDECAR_FEATURES": "groom,impact"})
        with self.assertRaises(lab.LabError):
            lab.parse_arms("V2+nope")

    def test_unknown_arm_and_alias_model_rejected(self):
        with self.assertRaises(lab.LabError):
            lab.parse_arms("OFF,V9")
        self.make_task()
        with self.assertRaises(lab.LabError):
            lab.cmd_run(self.run_args(agent="claude", model="haiku", sidecar=sys.executable, yes_spend=True))

    def test_full_scripted_run_resume_and_report(self):
        self.make_task()
        self.make_task("T-EDIT-02")
        args = self.run_args()
        lab.cmd_run(args)
        runs = pathlib.Path(args.out_dir) / "runs.jsonl"
        rows = [json.loads(l) for l in runs.read_text().splitlines()]
        self.assertEqual(len(rows), 6)
        lab.cmd_run(args)  # retomada: nada é repetido
        self.assertEqual(len(runs.read_text().splitlines()), 6)
        rep = lab.build_report(rows)
        self.assertEqual(list(rep["sections"]), ["simulated"])  # simulado nunca vira "agentic"
        self.assertEqual(rep["sections"]["simulated"]["arms"]["OFF"]["pass_rate"], 1.0)
        self.assertIn("inconclusivo", rep["sections"]["simulated"]["paired_vs_OFF"]["V2"]["conclusion"])
        self.assertIn("simulated", lab.render_md(rep))

    def test_spend_cap_stops_run(self):
        self.make_task()
        self.make_task("T-EDIT-02")
        args = self.run_args(max_total_usd=0.0)
        lab.cmd_run(args)
        self.assertFalse((pathlib.Path(args.out_dir) / "runs.jsonl").exists())

    def test_unvalidated_task_blocked(self):
        self.make_task()
        with self.assertRaises(lab.LabError):
            lab.cmd_run(self.run_args(allow_unvalidated=False))


class IsolationTests(unittest.TestCase):
    def test_deny_flags_cover_read_search_list_and_write_with_absolute_paths(self):
        flags = lab.deny_flags(["/data/bench/lab", "/data/out"])
        self.assertEqual(flags[0], "--disallowedTools")
        for tool in ("Read", "Grep", "Glob", "Edit", "Write"):
            self.assertIn(f"{tool}(//data/bench/lab/**)", flags)
            self.assertIn(f"{tool}(//data/out/**)", flags)
        self.assertEqual(lab.deny_flags([]), [])

    def test_touching_benchmark_data_is_detected_even_when_denied(self):
        deny = ["/data/bench/lab"]
        self.assertTrue(lab.touched_benchmark_data([{"kind": "read", "key": "/data/bench/lab/corpus/tasks/T/oracle/o.go"}], deny))
        self.assertTrue(lab.touched_benchmark_data([{"kind": "search", "key": "grep -r Test reference.patch"}], deny))
        self.assertTrue(lab.touched_benchmark_data([{"kind": "other", "key": "cat /x/oracle/a.py"}], deny))
        self.assertFalse(lab.touched_benchmark_data([{"kind": "read", "key": "/tmp/lab-run-1/work/main.go"}], deny))

    def test_run_refuses_a_deny_path_that_would_block_the_workspace(self):
        with self.assertRaises(lab.LabError):
            args = SimpleNamespace(agent="claude", model="claude-haiku-5-5", sidecar=sys.executable, yes_spend=True,
                                   out_dir=tempfile.gettempdir(), tasks=None, splits="development,validation", arms="OFF",
                                   allow_unvalidated=True, reps=1, seed=1, credentials=None, max_budget=0.1, timeout=10,
                                   tools_allow="Read", final=False, max_total_usd=None, scripted_behaviour="reference")
            lab.cmd_run(args)

class ToolTraceTests(unittest.TestCase):
    def stream(self, *blocks):
        return "\n".join(json.dumps(b) for b in blocks)

    def test_tool_events_maps_tools_and_failures(self):
        use = lambda i, name, inp: {"type": "assistant", "message": {"content": [{"type": "tool_use", "id": i, "name": name, "input": inp}]}}
        s = self.stream(use("1", "Read", {"file_path": "a.go"}), use("2", "Grep", {"pattern": "foo bar"}),
                        use("3", "Edit", {"file_path": "a.go"}), use("4", "Bash", {"command": "go test ./..."}),
                        use("5", "Bash", {"command": "rg timeout"}), use("6", "Bash", {"command": "cat b.go"}),
                        {"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "4", "is_error": True, "content": "FAIL line 42"}]}})
        ev = lab.tool_events(s)
        self.assertEqual([e["kind"] for e in ev], ["read", "search", "edit", "test", "search", "read", "other"])
        self.assertEqual(ev[-1]["err"], "fail line #")

    def test_tool_events_tolerates_unexpected_stream_shapes(self):
        s = self.stream({"type": "system", "message": "text instead of an object"}, {"type": "user", "message": {"content": "plain string"}},
                        {"type": "assistant", "message": {"content": ["not a block", None]}})
        self.assertEqual(lab.tool_events(s), [])

    def test_summarize_reports_redundancy_only_when_progress_exists(self):
        base = {"comparable": True, "success": True, "cost_usd": 0.1, "wall_s": 1.0, "tool_calls": 1, "injected_chars": 0,
                "served_models": ["m"], "verdict": "pass", "oracle": {"scope_ok": True, "invariants": [], "tests": "pass", "outside_scope": []}}
        self.assertIsNone(lab.summarize([dict(base)])["redundant_calls_mean"])
        with_p = dict(base, progress={"redundant": 4, "stagnant_windows": 1})
        s = lab.summarize([dict(base, progress={"redundant": 2, "stagnant_windows": 0}), with_p])
        self.assertEqual(s["redundant_calls_mean"], 3)
        self.assertEqual(s["stagnant_runs"], 1)


class ReviewFixTests(LabCase):
    def test_glob_star_does_not_cross_directories_and_deleted_file_fails_regex_absent(self):
        self.assertTrue(lab.in_scope("src/x.go", ["src/*.go"]))
        self.assertFalse(lab.in_scope("src/deep/x.go", ["src/*.go"]))
        self.assertTrue(lab.in_scope("src/deep/x.go", ["src/**"]))
        self.assertTrue(lab.in_scope("src/deep/x.go", ["src/"]))  # prefixo de diretório continua valendo
        d = self.make_task(scope={"allowed_paths": ["calc.py"], "invariants": [{"type": "regex_absent", "path": "calc.py", "pattern": "TODO"}]})
        task = json.loads((d / "task.json").read_text())
        with tempfile.TemporaryDirectory() as w:
            work = lab.prepare_workspace(task, pathlib.Path(w) / "w")
            self.assertTrue(lab.check_invariants(task, work)[0]["ok"])
            (work / "calc.py").unlink()
            self.assertFalse(lab.check_invariants(task, work)[0]["ok"])

    def test_setup_baseline_lives_outside_the_workspace_so_the_agent_cannot_hide_changes(self):
        d = self.make_task()
        task = json.loads((d / "task.json").read_text())
        with tempfile.TemporaryDirectory() as w:
            work = lab.prepare_workspace(task, pathlib.Path(w) / "w")
            self.assertFalse(str(lab.setup_paths_file(work)).startswith(str(work) + os.sep))
            (work / "STRAY.txt").write_text("x")
            (work / ".git" / "lab_setup_paths.json").write_text(json.dumps(["STRAY.txt"]))  # tentativa de esconder
            self.assertEqual(lab.changed_paths(work), ["STRAY.txt"])

    def test_oracle_source_and_dest_are_confined(self):
        d = self.make_task()
        task = json.loads((d / "task.json").read_text())
        for files in ({"zz.py": "../../../../etc/hostname"}, {"zz.py": "/etc/hostname"}, {".git/hooks/pre-commit": "o.py"}):
            task["oracle"]["files"] = files
            self.assertTrue(any("insegur" in e for e in lab.validate_task_schema(task, d)), files)

    def test_timeout_stays_in_the_rates_as_a_failure(self):
        row = lambda ok, cost, to: {"comparable": True, "success": ok and not to, "cost_usd": cost, "wall_s": 1.0, "tool_calls": 1,
                                    "injected_chars": 0, "served_models": ["m"], "verdict": "pass",
                                    "oracle": {"scope_ok": True, "invariants": [], "tests": "pass", "outside_scope": []}}
        s = lab.summarize([row(True, 1.0, False), row(True, 5.0, True)])
        self.assertEqual(s["pass_rate"], 0.5)
        self.assertEqual(s["cost_per_success"], 6.0)

    def test_unknown_cost_counts_as_the_per_run_budget_in_the_cap(self):
        self.make_task()
        args = self.run_args(max_budget=0.25, max_total_usd=100.0)
        lab.cmd_run(args)
        rows = [json.loads(l) for l in (pathlib.Path(args.out_dir) / "spend.jsonl").read_text().splitlines()]
        self.assertTrue(rows and all("estimated" in r for r in rows))

    def test_holdout_final_is_single_shot(self):
        self.make_task("T-EDIT-H1", split="holdout")
        lab.cmd_run(self.run_args(splits="holdout", final=True, arms="OFF"))
        with self.assertRaises(lab.LabError):
            lab.cmd_run(self.run_args(splits="holdout", final=True, arms="OFF", out_dir=str(pathlib.Path(self.tmp.name) / "out2")))
        lab.cmd_run(self.run_args(splits="holdout", final=True, arms="OFF", allow_repeat_final=True, out_dir=str(pathlib.Path(self.tmp.name) / "out3")))

    def test_agent_written_code_that_reads_the_benchmark_is_detected(self):
        d = self.make_task()
        task = json.loads((d / "task.json").read_text())
        with tempfile.TemporaryDirectory() as w:
            work = lab.prepare_workspace(task, pathlib.Path(w) / "w")
            (work / "leak_test.go").write_text('os.ReadFile("/data/bench/lab/corpus/tasks/T/reference.patch")')
            self.assertEqual(lab.workspace_mentions_benchmark(work, ["/data/bench/lab"]), "leak_test.go")
            (work / "leak_test.go").write_text("package x")
            self.assertIsNone(lab.workspace_mentions_benchmark(work, ["/data/bench/lab"]))
        self.assertTrue(lab.touched_benchmark_data([{"kind": "test", "key": "test", "cmd": "go test -exec 'cat /data/bench/lab/x' ./..."}], ["/data/bench/lab"]))


class ComparisonTests(unittest.TestCase):
    def row(self, task, arm, rep, ok, calls, tin, tout, cost, wall):
        return {"task": task, "arm": arm, "rep": rep, "comparable": True, "success": ok, "cost_usd": cost, "wall_s": wall, "tool_calls": calls,
                "tokens_in": tin, "tokens_out": tout, "evidence": "agentic", "split": "development", "class": "edit", "verdict": "pass" if ok else "fail",
                "injected_chars": 0, "served_models": ["m"], "oracle": {"scope_ok": True, "invariants": [], "tests": "pass", "outside_scope": []}}

    def test_delta_percent_is_paired_by_task_and_flags_noise(self):
        rows = []
        for i in range(12):
            rows.append(self.row(f"T{i}", "OFF", 0, i % 2 == 0, 10, 1000, 100, 0.10, 20.0))
            rows.append(self.row(f"T{i}", "V2", 0, i % 2 == 0, 8, 800, 100, 0.08, 15.0))
        c = lab.compare_arms(rows, "V2", "OFF")
        self.assertEqual(c["pairs"], 12)
        self.assertEqual(c["metrics"]["tool_calls"]["delta_pct"], -20.0)
        self.assertEqual(c["metrics"]["tokens"]["delta_pct"], -18.2)
        self.assertEqual(c["metrics"]["pass_rate"]["delta_pct"], 0.0)
        self.assertEqual(c["metrics"]["tool_calls"]["reading"], "melhor/menor")  # a constant -20% has a CI that excludes zero
        self.assertEqual(c["metrics"]["pass_rate"]["reading"], "inconclusivo")

    def test_missing_metrics_and_few_tasks_are_inconclusive_not_invented(self):
        rows = [self.row("T1", "OFF", 0, True, 10, None, None, None, 5.0), self.row("T1", "V2", 0, True, 9, None, None, None, 5.0)]
        c = lab.compare_arms(rows, "V2", "OFF")
        self.assertIsNone(c["metrics"]["tokens"])
        self.assertIsNone(c["metrics"]["cost_usd"])
        self.assertEqual(c["metrics"]["tool_calls"]["reading"], "inconclusivo")
        self.assertIsNone(c["metrics"]["tool_calls"]["ci95_pct"])

    def test_only_pairs_where_both_arms_are_comparable_count(self):
        rows = [self.row("T1", "OFF", 0, True, 10, 1, 1, 1, 1), self.row("T2", "OFF", 0, True, 10, 1, 1, 1, 1), self.row("T1", "V2", 0, True, 5, 1, 1, 1, 1)]
        self.assertEqual(lab.compare_arms(rows, "V2", "OFF")["pairs"], 1)

    def test_comparison_plan_has_no_inverse_or_self_pairs(self):
        plan = lab.comparison_plan(["OFF", "V1", "V2", "V2+adaptive", "V2+brief"])
        self.assertNotIn(("OFF", "V1"), plan)
        self.assertIn(("V1", "OFF"), plan)  # V1 contra a referência é legítimo; o inverso não
        self.assertTrue(all(a != b for a, b in plan))
        self.assertIn(("V2", "OFF"), plan)
        self.assertIn(("V2", "V1"), plan)
        self.assertIn(("V2+brief", "V2"), plan)
        self.assertEqual(len(plan), len(set(plan)))

    def test_report_renders_the_percentage_table(self):
        rows = [self.row(f"T{i}", a, 0, True, 10 if a == "OFF" else 8, 100, 10, 0.1, 1.0) for i in range(6) for a in ("OFF", "V2")]
        md = lab.render_md(lab.build_report(rows))
        self.assertIn("Δ% por métrica", md)
        self.assertIn("V2 vs OFF", md)


class TwoTurnTests(unittest.TestCase):
    def turn(self, **kw):
        base = {"wall_s": 10.0, "result": "ok", "cost_usd": 0.01, "tokens_in": 100, "tokens_out": 10, "tool_calls": 5, "turns": 3,
                "served_models": ["m"], "exit": 0, "timed_out": False, "hook_delivered": True, "injected_chars": 300,
                "incomparable_reasons": [], "stream_error": False, "hook_names": ["UserPromptSubmit"], "tool_events": [{"kind": "read", "key": "a"}],
                "hook_context": ""}
        base.update(kw)
        return base

    def test_merge_sums_cost_tokens_calls_and_keeps_last_result(self):
        m = lab.merge_turns([self.turn(result="first"), self.turn(result="last", cost_usd=0.02, tool_calls=7)])
        self.assertEqual((m["cost_usd"], m["tokens_in"], m["tool_calls"], m["n_voltas"]), (0.03, 200, 12, 2))
        self.assertEqual(m["result"], "last")
        self.assertEqual(len(m["tool_events"]), 2)
        self.assertEqual(m["volta_injetada"], [300, 300])

    def test_one_contaminated_or_timed_out_turn_taints_the_run(self):
        m = lab.merge_turns([self.turn(), self.turn(incomparable_reasons=["modelo servido ausente"], timed_out=True)])
        self.assertTrue(m["timed_out"])
        self.assertEqual(m["incomparable_reasons"], ["modelo servido ausente"])

    def test_unknown_cost_in_one_turn_does_not_become_zero_and_notes_are_presence_only(self):
        m = lab.merge_turns([self.turn(cost_usd=None, hook_context="Open decision (underspecified): x"), self.turn(cost_usd=None, hook_context="Change check on uncommitted edits")])
        self.assertIsNone(m["cost_usd"])
        self.assertEqual(m["notas"], {"groom": True, "impact": True, "brief": False})
        self.assertNotIn("hook_context", m)

    def test_followup_is_defined_by_the_task_not_the_arm(self):
        self.assertEqual(lab.followup_for({"followup": "faça X"}), "faça X")
        amb = lab.followup_for({"underspec": {"canned_answer": "usar as opções declaradas"}})
        self.assertIn("usar as opções declaradas", amb)
        self.assertEqual(lab.followup_for({"class": "edit"}), lab.DEFAULT_FOLLOWUP)

    def test_session_id_is_read_from_the_stream(self):
        self.assertEqual(lab.session_id_of('noise\n{"type":"system","session_id":"abc-123"}\n'), "abc-123")
        self.assertIsNone(lab.session_id_of('{"type":"x"}'))

    def test_command_persists_the_session_only_for_two_turns_and_resumes(self):
        one = lab.ClaudeAgent("claude-haiku-5-5", "/bin/true", None, 0.1, 10, ["Read"], deny_paths=["/data/x"], turns=1)
        two = lab.ClaudeAgent("claude-haiku-5-5", "/bin/true", None, 0.1, 10, ["Read"], deny_paths=["/data/x"], turns=2)
        self.assertIn("--no-session-persistence", one.command("p", None))
        c = two.command("p", None, resume="sid-1")
        self.assertNotIn("--no-session-persistence", c)
        self.assertEqual(c[c.index("--resume") + 1], "sid-1")
        self.assertIn("--disallowedTools", c)  # a negação do gabarito vale também na segunda volta


class HoldoutTests(LabCase):
    def test_holdout_sealed_without_final_and_ledgered_with_final(self):
        self.make_task("T-EDIT-H1", split="holdout")
        self.assertFalse(lab.holdout_burned())
        with self.assertRaises(lab.LabError):
            lab.cmd_run(self.run_args(splits="holdout"))
        self.assertFalse(lab.holdout_burned())
        lab.cmd_run(self.run_args(splits="holdout", final=True, arms="OFF"))
        self.assertTrue(lab.holdout_burned())
        entry = json.loads(lab.LEDGER.read_text().splitlines()[0])
        self.assertEqual(entry["tasks"], ["T-EDIT-H1"])

    def test_manifest_hashes_and_counts(self):
        self.make_task()
        self.make_task("T-EDIT-H1", split="holdout")
        m = lab.build_manifest()
        self.assertEqual(m["counts"], {"development": 1, "validation": 0, "holdout": 1})
        self.assertEqual(len(m["tasks"][0]["oracle_sha256"]), 64)
        self.assertEqual(len(m["holdout_prompts_sha256"]), 64)


class StatsTests(unittest.TestCase):
    def test_percentiles_and_bootstrap_guard(self):
        self.assertEqual(lab.pct([1, 2, 3, 4, 5], 0.5), 3)
        self.assertEqual(lab.pct([1, 2, 3, 4, 5], 0.95), 5)
        self.assertIsNone(lab.pct([], 0.5))
        self.assertIsNone(lab.boot_ci([1, 0, 1]))  # N < 5: sem IC

    def test_test_files_outside_scope_are_not_counted_as_source_violations(self):
        r = {"oracle": {"outside_scope": ["tests/test_x.py", "middleware/a_test.go", "src/app.py", "CHANGES.md"]}}
        self.assertEqual(lab.non_test_outside(r), ["src/app.py", "CHANGES.md"])

    def test_cost_per_success_requires_complete_cost_data(self):
        def row(c, ok=True):
            return {"comparable": True, "success": ok, "cost_usd": c, "wall_s": 1.0, "tool_calls": 1, "injected_chars": 0,
                    "served_models": ["m"], "verdict": "pass", "oracle": {"scope_ok": True, "invariants": [], "tests": "pass", "outside_scope": []}}
        self.assertIsNone(lab.summarize([row(0.1), row(None)])["cost_per_success"])
        self.assertEqual(lab.summarize([row(0.1), row(0.3)])["cost_per_success"], 0.2)


if __name__ == "__main__":
    unittest.main()
