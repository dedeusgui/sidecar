import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest
from unittest import mock
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
        (self.corpus / "manifest.json").write_text(json.dumps({"schema": "lab-manifest/1", "tasks": [],
            "holdout_prompts_sha256": lab.sha256_bytes(b"")}))
        self.holdout_prompts = {}
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
        manifest_path = self.corpus / "manifest.json"
        manifest = json.loads(manifest_path.read_text())
        entry = {"id": tid, "split": task["split"], "class": task["class"], "repo": task["repo"],
                 "task_sha256": lab.sha256_file(d / "task.json"), "oracle_sha256": lab.tree_sha256(d / "oracle"),
                 "reference_sha256": lab.sha256_file(d / "reference.patch") if (d / "reference.patch").is_file() else None,
                 "validated": False}
        manifest["tasks"] = [t for t in manifest["tasks"] if t["id"] != tid] + [entry]
        if split == "holdout":
            self.holdout_prompts[tid] = task["prompt"]
        manifest["holdout_prompts_sha256"] = lab.sha256_bytes("\n".join(sorted(self.holdout_prompts.values())).encode())
        manifest_path.write_text(json.dumps(manifest))
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
    @unittest.skipUnless(lab.shutil.which("bwrap") and sys.platform == "linux", "requer bwrap em Linux")
    def test_agent_sandbox_hides_host_paths_and_limits_path(self):
        with tempfile.TemporaryDirectory() as root:
            root = pathlib.Path(root)
            work = root / "work"
            work.mkdir()
            secret = root / "host-only.txt"
            secret.write_text("do not read")
            isolated_tmp = root / "isolated-tmp"
            isolated_tmp.mkdir()
            fake_bin = root / "fake-bin"
            fake_bin.mkdir()
            for name in ("sidecar", "codegraph"):
                fake = fake_bin / name
                fake.write_text("#!/bin/sh\nexit 0\n")
                fake.chmod(0o755)
            agent_home = root / "agent-home"
            agent_home.mkdir()
            state = root / "sidecar-home"
            state.mkdir()
            bin_host = root / "agent-bin"
            with mock.patch.dict(os.environ, {"PATH": str(fake_bin) + os.pathsep + os.environ["PATH"]}):
                bin_host, runtime_mounts, runtime_env = lab.agent_runtime_bin(bin_host)
            command = lab.agent_sandbox_command(
                ["sh", "-c", f"test ! -e {secret} && test ! -e {lab.HERE.parent.parent} && "
                 "! command -v sidecar >/dev/null && ! command -v codegraph >/dev/null && "
                 "printf sandbox-ok > /work/writable"],
                work, readonly_paths=runtime_mounts + [(bin_host, "/agent-bin")],
                writable_paths=[(isolated_tmp, "/tmp"), (agent_home, "/home/agent"), (state, "/sidecar-home")],
                sandbox_env=runtime_env)
            result = subprocess.run(command, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((work / "writable").read_text(), "sandbox-ok")

    def test_agent_sandbox_fails_closed_without_bwrap(self):
        with mock.patch.object(lab.shutil, "which", return_value=None):
            with self.assertRaisesRegex(lab.LabError, "filesystem sandbox"):
                lab.agent_sandbox_command(["true"], "/tmp/work", [], [])

    def test_mounts_cannot_contain_protected_data(self):
        with self.assertRaises(lab.LabError):
            lab.reject_sensitive_mounts(["/usr"], ["/usr/local/benchmark"])

    @unittest.skipUnless(lab.shutil.which("bwrap") and sys.platform == "linux", "requer bwrap em Linux")
    def test_venv_python_and_pytest_work_after_workspace_relocation(self):
        with tempfile.TemporaryDirectory() as root:
            root = pathlib.Path(root)
            work = root / "work"
            work.mkdir()
            subprocess.run([sys.executable, "-m", "venv", "--without-pip", str(work / ".venv")], check=True)
            site = work / ".venv" / "lib" / f"python{sys.version_info.major}.{sys.version_info.minor}" / "site-packages"
            (site / "pytest.py").write_text("print('venv-ok')\n")
            bin_host, mounts, sandbox_env = lab.agent_runtime_bin(root / "bin", work)
            tmp = root / "tmp"
            tmp.mkdir()
            command = lab.agent_sandbox_command(["pytest"], work, mounts + [(bin_host, "/agent-bin")],
                                                [(tmp, "/tmp")], sandbox_env)
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("venv-ok", result.stdout)

    def test_failed_agent_cannot_pass_even_if_oracle_passes(self):
        d = self.make_task()
        task = json.loads((d / "task.json").read_text())
        class Failed(lab.ScriptedAgent):
            def run(self, *args):
                result = super().run(*args)
                result["exit"] = 1
                return result
        with tempfile.TemporaryDirectory() as home:
            rec = lab.execute_one(Failed("reference"), task, d, {"name": "OFF", "engine": None, "env": {}}, 0, None, home)
        self.assertEqual(rec["oracle"]["tests"], "pass")
        self.assertFalse(rec["success"])

    def test_language_tool_allowlists_cover_each_repository_runner(self):
        expected = {
            "go": "Bash(go test:*)",
            "python": "Bash(pytest:*)",
            "javascript": "Bash(npm test:*)",
            "ruby": "Bash(bundle exec:*)",
            "rust": "Bash(cargo test:*)",
            "java": "Bash(mvn test:*)",
            "php": "Bash(phpunit:*)",
        }
        for language, runner in expected.items():
            with self.subTest(language=language):
                allowed = lab.tools_allow_for({"language": language})
                self.assertIn(runner, allowed)
                self.assertIn("Read", allowed)
                self.assertEqual(allowed, list(dict.fromkeys(allowed)))

    def test_explicit_tool_allowlist_replaces_language_defaults(self):
        self.assertEqual(lab.tools_allow_for({"language": "go"}, ["Read", "Bash(custom:*)", "Read"]),
                         ["Read", "Bash(custom:*)"])

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
        self.assertTrue(all(row["tools_allow"] == ["Read"] for row in rows))
        meta = json.loads((pathlib.Path(args.out_dir) / "meta.json").read_text())
        self.assertEqual(meta["tools_allow_by_repo"], {"toy": ["Read"]})
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

    def test_spend_cap_reserves_the_next_execution(self):
        self.make_task()
        args = self.run_args(max_total_usd=0.09, max_budget=0.1)
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
        self.assertEqual([e["kind"] for e in ev], ["read", "search", "edit", "test", "search", "read"])
        self.assertEqual(ev[3]["err"], "fail line #")
        self.assertTrue(ev[3]["completed"])
        self.assertEqual(ev[3]["tool_use_id"], "4")

    def test_tool_events_keep_order_thread_and_result_correlation(self):
        use = {"type": "assistant", "uuid": "msg-a", "parent_tool_use_id": "agent-7", "timestamp": "t1",
               "message": {"content": [
                   {"type": "tool_use", "id": "read-1", "name": "Read", "input": {"file_path": "a.go"}},
                   {"type": "tool_use", "id": "edit-1", "name": "Edit", "input": {"file_path": "a.go"}}]}}
        results = {"type": "user", "uuid": "msg-b", "parent_tool_use_id": "agent-7", "message": {"content": [
            {"type": "tool_result", "tool_use_id": "edit-1", "is_error": True, "content": "denied"},
            {"type": "tool_result", "tool_use_id": "read-1", "content": "source text"}]}}
        events = lab.tool_events(self.stream(use, results))
        self.assertEqual([e["order"] for e in events], sorted(e["order"] for e in events))
        self.assertTrue(all(e["thread_id"] == "agent-7" and e["message_id"] == "msg-a" for e in events))
        by_id = {e["tool_use_id"]: e for e in events}
        self.assertTrue(by_id["read-1"]["completed"])
        self.assertIsNone(by_id["read-1"]["err"])
        self.assertEqual(by_id["edit-1"]["err"], "denied")

    def test_native_question_is_ephemeral_and_requires_successful_tool_result(self):
        use = {"type": "assistant", "uuid": "q-msg", "message": {"content": [{"type": "tool_use", "id": "q1",
               "name": "AskUserQuestion", "input": {"questions": [{"question": "Which output style?", "options": [
                   {"label": "compact output"}, {"label": "pretty output"}]}]}}]}}
        result = {"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "q1", "content": "compact"}]}}
        stream = self.stream(use, result)
        persisted = lab.tool_events(stream)[0]
        self.assertEqual(persisted["kind"], "clarify")
        self.assertTrue(persisted["completed"])
        self.assertNotIn("question", persisted)
        self.assertNotIn("compact output", json.dumps(persisted))
        ephemeral = lab.tool_events(stream, include_question_text=True)[0]
        self.assertIn("compact output", ephemeral["question"])
        task = {"underspec": {"canned_answer": "Use compact output.", "options": [
            {"label": "compact output"}, {"label": "pretty output"}]}}
        self.assertEqual(lab.followup_for(task, {"assistant_questions": [], "tool_events": [ephemeral]}),
                         "Use compact output.")
        failed = self.stream(use, {"type": "user", "message": {"content": [
            {"type": "tool_result", "tool_use_id": "q1", "is_error": True, "content": "cancelled"}]}})
        failed_turn = {"assistant_questions": [], "tool_events": lab.tool_events(failed, include_question_text=True)}
        self.assertEqual(lab.followup_for(task, failed_turn), lab.DEFAULT_FOLLOWUP)

    def test_post_context_requires_a_delivered_hook_response(self):
        tool = lambda ident: {"type": "assistant", "uuid": ident, "message": {"content": [
            {"type": "tool_use", "id": ident, "name": "Read", "input": {"file_path": ident + ".go"}}]}}
        hook = {"type": "system", "subtype": "hook_response", "hook_event": "UserPromptSubmit",
                "output": json.dumps({"hookSpecificOutput": {"additionalContext": "context"}})}
        before, after = lab.tool_events(self.stream(tool("before"), hook, tool("after")))
        self.assertFalse(before["after_context"])
        self.assertTrue(after["after_context"])
        with mock.patch.object(lab, "run", return_value=(0, json.dumps({"events": 1, "labels": [], "redundant": 0,
                                                                            "exploratory": 1, "novelty_ratio": 1,
                                                                            "stagnant_windows": 0, "longest_stagnation": 0}), "", False)) as run:
            self.assertIsNotNone(lab.progress_report("/sidecar", [before, after]))
            payload = run.call_args.kwargs["input_text"]
            self.assertNotIn("before.go", payload)
            self.assertIn("after.go", payload)
        unobserved = lab.tool_events(self.stream(tool("unobserved")))[0]
        self.assertIsNone(unobserved["after_context"])
        self.assertIsNone(lab.progress_report("/sidecar", [unobserved]))

    def test_tool_events_tolerates_unexpected_stream_shapes(self):
        s = self.stream({"type": "system", "message": "text instead of an object"}, {"type": "user", "message": {"content": "plain string"}},
                        {"type": "assistant", "message": {"content": ["not a block", None]}})
        self.assertEqual(lab.tool_events(s), [])

    def test_persisted_trace_redacts_command_and_error_text(self):
        event = {"kind": "other", "key": "secret", "cmd": "cat secret", "err": "secret error"}
        redacted = lab.redact_tool_events([event])[0]
        self.assertNotIn("secret", json.dumps(redacted))
        self.assertNotIn("cmd", redacted)

    def test_summarize_reports_redundancy_only_when_progress_exists(self):
        base = {"comparable": True, "success": True, "cost_usd": 0.1, "wall_s": 1.0, "tool_calls": 1, "injected_chars": 0,
                "served_models": ["m"], "verdict": "pass", "oracle": {"scope_ok": True, "invariants": [], "tests": "pass", "outside_scope": []}}
        self.assertIsNone(lab.summarize([dict(base)])["redundant_calls_mean"])
        with_p = dict(base, progress={"redundant": 4, "stagnant_windows": 1})
        s = lab.summarize([dict(base, progress={"redundant": 2, "stagnant_windows": 0}), with_p])
        self.assertEqual(s["redundant_calls_mean"], 3)
        self.assertEqual(s["stagnant_runs"], 1)

    def test_summary_totals_and_means_stay_unknown_when_any_counter_is_missing(self):
        base = {"comparable": True, "success": True, "cost_usd": 0.1, "wall_s": 1.0, "tool_calls": 2,
                "injected_chars": 10, "served_models": ["m"], "verdict": "pass",
                "oracle": {"scope_ok": True, "invariants": [], "tests": "pass", "outside_scope": []}}
        summary = lab.summarize([base, dict(base, cost_usd=None, wall_s=None, tool_calls=None, injected_chars=None)])
        self.assertIsNone(summary["cost_total"])
        self.assertIsNone(summary["wall_p95"])
        self.assertIsNone(summary["tool_calls_mean"])
        self.assertIsNone(summary["injected_chars_mean"])
        self.assertEqual(summary["cost_unknown_runs"], 1)

    def test_optional_metrics_render_only_when_fully_measured(self):
        row = {"comparable": True, "success": True, "cost_usd": 0.1, "wall_s": 1.0, "tool_calls": 2,
               "injected_chars": 10, "cache_read": 7, "arm": "V2", "task": "T1", "rep": 0,
               "evidence": "agentic", "split": "development", "class": "edit", "progress": None,
               "served_models": ["m"], "verdict": "pass",
               "oracle": {"scope_ok": True, "invariants": [], "tests": "pass", "outside_scope": []}}
        md = lab.render_md(lab.build_report([row]))
        self.assertIn("Contadores e tempos observados", md)
        self.assertIn("cache lido", md)
        unknown = lab.summarize([dict(row, cache_read=None)])
        self.assertNotIn("measured", unknown)


class HookMetricTests(unittest.TestCase):
    def test_reads_only_metadata_timings_appended_after_offset(self):
        with tempfile.TemporaryDirectory() as d:
            path = pathlib.Path(d) / "hook.jsonl"
            path.write_text(json.dumps({"total_ms": 99, "sync_ms": 90, "retrieve_ms": 8}) + "\n")
            offset = lab.hook_log_offset(path)
            with path.open("a") as f:
                f.write(json.dumps({"prompt": "private", "total_ms": 12.5, "sync_ms": 4, "retrieve_ms": 3.5}) + "\n")
                f.write(json.dumps({"total_ms": 7.5, "sync_ms": 2, "retrieve_ms": 1}) + "\n")
            self.assertEqual(lab.hook_metrics_since(path, offset), {"hook_ms": 20.0, "sync_ms": 6, "retrieve_ms": 4.5})

    def test_missing_log_stage_counter_stays_unknown(self):
        with tempfile.TemporaryDirectory() as d:
            path = pathlib.Path(d) / "hook.jsonl"
            path.write_text('{"total_ms": 12, "sync_ms": 3}\n')
            self.assertEqual(lab.hook_metrics_since(path, 0), {"hook_ms": 12, "sync_ms": 3, "retrieve_ms": None})
            self.assertTrue(all(v is None for v in lab.hook_metrics_since(path, path.stat().st_size).values()))


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

    def test_context_delivery_carries_over_and_event_order_spans_turns(self):
        first = self.turn(tool_events=[{"kind": "read", "key": "a", "order": 20, "after_context": True}])
        second = self.turn(hook_delivered=False, tool_events=[{"kind": "read", "key": "b", "order": 1, "after_context": None}])
        events = lab.merge_turns([first, second])["tool_events"]
        self.assertEqual([e["order"] for e in events], [0, 1])
        self.assertEqual([e["turn"] for e in events], [1, 2])
        self.assertTrue(events[1]["after_context"])

    def test_one_contaminated_or_timed_out_turn_taints_the_run(self):
        m = lab.merge_turns([self.turn(), self.turn(incomparable_reasons=["modelo servido ausente"], timed_out=True)])
        self.assertTrue(m["timed_out"])
        self.assertEqual(m["incomparable_reasons"], ["modelo servido ausente"])

    def test_unknown_cost_in_one_turn_does_not_become_zero_and_notes_are_presence_only(self):
        m = lab.merge_turns([self.turn(cost_usd=None, hook_context="Open decision (underspecified): x"), self.turn(cost_usd=None, hook_context="Change check on uncommitted edits")])
        self.assertIsNone(m["cost_usd"])
        self.assertEqual(m["notas"], {"groom": True, "impact": True, "brief": False})
        self.assertNotIn("hook_context", m)

    def test_partial_turn_metrics_remain_unknown_in_either_order(self):
        for turns in (
            [self.turn(cost_usd=0.01), self.turn(cost_usd=None)],
            [self.turn(cost_usd=None), self.turn(cost_usd=0.01)],
        ):
            with self.subTest(turns=[turn["cost_usd"] for turn in turns]):
                self.assertIsNone(lab.merge_turns(turns)["cost_usd"])

    def test_missing_aggregate_metrics_are_not_reported_as_zero(self):
        for key in ("wall_s", "tokens_in", "tokens_out", "tool_calls", "turns", "injected_chars"):
            with self.subTest(key=key):
                merged = lab.merge_turns([self.turn(**{key: 1}), self.turn(**{key: None})])
                self.assertIsNone(merged[key])

    def test_usage_and_timing_fields_sum_only_when_every_turn_is_measured(self):
        first = self.turn(input_uncached=10, cache_read=20, cache_create=30, output=4,
                          index_ms=5.0, hook_ms=2.0, sync_ms=1.0, retrieve_ms=0.5)
        second = self.turn(input_uncached=2, cache_read=3, cache_create=4, output=1,
                           index_ms=0.0, hook_ms=3.0, sync_ms=2.0, retrieve_ms=1.5)
        merged = lab.merge_turns([first, second])
        self.assertEqual((merged["input_uncached"], merged["cache_read"], merged["cache_create"], merged["output"]),
                         (12, 23, 34, 5))
        self.assertEqual((merged["index_ms"], merged["hook_ms"], merged["sync_ms"], merged["retrieve_ms"]),
                         (5.0, 5.0, 3.0, 2.0))
        self.assertIsNone(lab.merge_turns([first, self.turn(index_ms=None)]) ["index_ms"])

    def test_followup_is_defined_by_the_task_not_the_arm(self):
        self.assertEqual(lab.followup_for({"followup": "faça X"}), "faça X")
        amb = {"class": "ambiguous", "underspec": {
            "canned_answer": "Use compact output.",
            "options": [{"label": "compact output"}, {"label": "pretty output"}],
        }}
        self.assertEqual(lab.followup_for(amb), lab.DEFAULT_FOLLOWUP)
        self.assertEqual(lab.followup_for({"class": "edit"}), lab.DEFAULT_FOLLOWUP)

    def test_ambiguous_answer_requires_relevant_question_before_any_edit(self):
        task = {"class": "ambiguous", "underspec": {
            "canned_answer": "Use compact output.",
            "options": [{"label": "compact output"}, {"label": "pretty output"}],
        }}
        no_question = {"assistant_questions": [], "tool_events": []}
        irrelevant = {"assistant_questions": [{"text": "Should I rename the package?", "order": 2}], "tool_events": []}
        relevant_text_question = {"assistant_questions": [
            {"text": "Should output be compact output or pretty output?", "order": 2}], "tool_events": []}
        asked_after_edit = {"assistant_questions": [
            {"text": "Should output be compact output or pretty output?", "order": 5}],
            "tool_events": [{"kind": "edit", "order": 3}]}
        relevant_tool_question = {"assistant_questions": [], "tool_events": [
            {"kind": "clarify", "key": "Should output be compact output or pretty output?", "order": 2,
             "completed": True, "err": None}]}

        for turn in (no_question, irrelevant, asked_after_edit):
            with self.subTest(turn=turn):
                self.assertEqual(lab.followup_for(task, turn), lab.DEFAULT_FOLLOWUP)
        self.assertEqual(lab.followup_for(task, relevant_text_question), "Use compact output.")
        self.assertEqual(lab.followup_for(task, relevant_tool_question), "Use compact output.")

    def test_duplicate_clarification_gets_one_answer_and_expired_session_gets_none(self):
        task = {"class": "ambiguous", "underspec": {
            "canned_answer": "Use compact output.",
            "options": [{"label": "compact output"}, {"label": "pretty output"}],
        }}
        question = {"text": "Should output be compact output or pretty output?", "order": 1}
        duplicate = {"assistant_questions": [question, dict(question, order=2)], "tool_events": []}
        self.assertEqual(lab.followup_for(task, duplicate), "Use compact output.")
        self.assertEqual(lab.followup_for(task, duplicate, session_available=False), lab.DEFAULT_FOLLOWUP)
        self.assertEqual(lab.clarification_status(task, duplicate, session_available=False), "expired")
        self.assertEqual(lab.followup_for(task, {"clarification_status": "answered"}), "Use compact output.")
        self.assertEqual(lab.followup_for(task, {"clarification_status": "answered"}, session_available=False), lab.DEFAULT_FOLLOWUP)

    def test_claude_text_question_is_supported_without_a_native_question_tool(self):
        task = {"class": "ambiguous", "underspec": {
            "canned_answer": "Use compact output.",
            "options": [{"label": "compact output"}, {"label": "pretty output"}],
        }}
        stream = json.dumps({"type": "assistant", "uuid": "message-1", "message": {"content": [
            {"type": "text", "text": "Should output be compact output or pretty output?"}]}})
        turn = {"assistant_questions": lab.assistant_questions(stream), "tool_events": []}
        self.assertEqual(lab.followup_for(task, turn), "Use compact output.")

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
    def test_real_final_requires_freeze_before_reading_holdout(self):
        self.make_task("T-EDIT-H1", split="holdout")
        args = self.run_args(agent="claude", model="claude-haiku-5-5", sidecar=sys.executable,
                             splits="holdout", final=True, yes_spend=True)
        with mock.patch.object(lab, "build_manifest", side_effect=AssertionError("holdout opened")):
            with self.assertRaisesRegex(lab.LabError, "frozen-revision"):
                lab.cmd_run(args)

    def test_development_manifest_never_opens_holdout_task_or_oracle(self):
        self.make_task()
        held = self.make_task("T-EDIT-H1", split="holdout")
        original_load, original_tree = lab.load_json, lab.tree_sha256
        def guarded_load(path):
            if held in pathlib.Path(path).parents:
                raise AssertionError("holdout was opened")
            return original_load(path)
        def guarded_tree(path):
            if pathlib.Path(path) == held / "oracle":
                raise AssertionError("holdout oracle was opened")
            return original_tree(path)
        with mock.patch.object(lab, "load_json", side_effect=guarded_load), mock.patch.object(lab, "tree_sha256", side_effect=guarded_tree):
            manifest = lab.build_manifest()
        self.assertEqual(manifest["counts"]["holdout"], 1)

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
