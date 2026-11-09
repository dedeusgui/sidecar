import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest
import unittest.mock

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import lab  # noqa: E402
import sufficiency as S  # noqa: E402


def ev(order, kind, key="", thread="main", msg=None, cmd=None, **kw):
    e = {"order": order, "kind": kind, "key": key, "thread_id": thread, "message_id": msg or f"m{order}",
         "completed": True, "err": None}
    if cmd:
        e["cmd"] = cmd
    e.update(kw)
    return e


def delivery(order, returned, named=(), thread="main", msg=None, source="explore", errored=False):
    return {"order": order, "returned": list(returned), "named": list(named), "thread_id": thread,
            "message_id": msg, "source": source, "errored": errored}


class ClassifyTests(unittest.TestCase):
    def bucket(self, events, deliveries):
        return [c["bucket"] for c in S.classify(events, deliveries)]

    def test_read_of_returned_file_is_allocation(self):
        events = [ev(10, "read", "/work/lib/a.go")]
        self.assertEqual(self.bucket(events, [delivery(0, ["lib/a.go"], source="hook")]), ["read_returned"])

    def test_read_of_named_and_of_unknown_file_are_recall(self):
        events = [ev(10, "read", "lib/b.go")]
        self.assertEqual(self.bucket(events, [delivery(0, ["lib/a.go"], named=["lib/b.go"])]), ["read_named"])
        self.assertEqual(self.bucket(events, [delivery(0, ["lib/a.go"])]), ["read_not_returned"])

    def test_same_message_calls_are_concurrent_not_a_verdict(self):
        events = [ev(5, "explore", msg="m1"), ev(6, "read", "lib/z.go", msg="m1"), ev(20, "edit", "lib/a.go", msg="m2")]
        res = S.classify(events, [delivery(5, ["lib/a.go"], msg="m1")])
        self.assertEqual(res[0]["bucket"], "moved_on")
        self.assertEqual(res[0]["concurrent"], 1)

    def test_subagent_calls_do_not_judge_the_parent(self):
        events = [ev(10, "search", "grep x", thread="toolu_sub"), ev(11, "edit", "lib/a.go")]
        self.assertEqual(self.bucket(events, [delivery(0, ["lib/a.go"])]), ["moved_on"])

    def test_delegation_is_judged_by_the_subagent_first_call(self):
        events = [ev(10, "delegate", tool_use_id="toolu_sub"), ev(11, "bookkeeping", "TodoWrite", thread="toolu_sub"),
                  ev(12, "search", "rg y", thread="toolu_sub")]
        self.assertEqual(self.bucket(events, [delivery(0, ["lib/a.go"])]), ["search"])

    def test_explore_again_errored_and_final_answer(self):
        self.assertEqual(self.bucket([ev(10, "explore")], [delivery(0, ["a"])]), ["explore_again"])
        self.assertEqual(self.bucket([], [delivery(0, ["a"], errored=True)]), ["errored"])
        self.assertEqual(self.bucket([], [delivery(0, ["a"])]), ["moved_on"])

    def test_shell_read_counts_as_read(self):
        events = [ev(10, "read", "sed -n 1,20p lib/a.go", cmd="sed -n '1,20p' lib/a.go")]
        self.assertEqual(self.bucket(events, [delivery(0, ["lib/a.go"])]), ["read_returned"])

    def test_file_shipped_by_an_earlier_delivery_is_still_allocation(self):
        events = [ev(10, "explore"), ev(30, "read", "lib/a.go")]
        res = self.bucket(events, [delivery(0, ["lib/a.go"]), delivery(10, ["lib/b.go"], msg="m10")])
        self.assertEqual(res, ["explore_again", "read_returned"])


class SessionSignalTests(unittest.TestCase):
    def test_redundant_stale_and_no_effect(self):
        events = [ev(1, "read", "a.go"), ev(2, "read", "a.go"), ev(3, "edit", "a.go"), ev(4, "read", "a.go")]
        sig = S.session_signals(events, [delivery(0, ["a.go"]), delivery(0, ["z.go"])])
        self.assertEqual(sig["redundant_rereads"], 1)
        self.assertEqual(sig["stale_reads_after_edit"], 1)
        self.assertEqual(sig["deliveries_without_effect"], 1)

    def test_token_origins_keep_unknown(self):
        o = S.token_origins({"input_uncached": 10, "cache_read": None, "injected_chars": 400, "explore_chars": None})
        self.assertEqual(o["measured"]["cache_read"], None)
        self.assertEqual(o["estimated_from_chars"]["sidecar_hook"], 100)
        self.assertIsNone(o["estimated_from_chars"]["sidecar_explore"])


class StreamTests(unittest.TestCase):
    def stream(self, *events):
        return "\n".join(json.dumps(e) for e in events)

    def test_explore_call_and_returned_files_are_parsed_from_the_stream(self):
        out = "[SIDECAR EXPLORE] x\n\n**`lib/response.js`** — candidate\n126\tres.send = ...\n\nConsumers:\n- [CANDIDATE] function html (examples/users.js:5) — calls send\n"
        text = self.stream(
            {"type": "assistant", "uuid": "a1", "message": {"content": [{"type": "tool_use", "id": "t1", "name": "Bash",
                                                                       "input": {"command": "/opt/sidecar explore --query 'res.send'"}}]}},
            {"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "t1", "content": out}]}},
            {"type": "assistant", "uuid": "a2", "message": {"content": [{"type": "tool_use", "id": "t2", "name": "Read",
                                                                       "input": {"file_path": "/work/examples/users.js"}}]}},
        )
        events = lab.tool_events(text)
        self.assertEqual(events[0]["kind"], "explore")
        self.assertEqual(events[0]["returned"], ["lib/response.js"])
        self.assertIn("examples/users.js", events[0]["named"])
        self.assertEqual(events[0]["result_chars"], len(out))
        rep = lab.sufficiency_report(events, "", False)
        self.assertEqual(rep["read_named"], 1)
        self.assertEqual(rep["explore_chars"], len(out))
        persisted = lab.redact_tool_events(events)
        self.assertTrue(all(p.startswith("sha256:") for p in persisted[0]["returned"]))

    def test_hook_delivery_paths_come_from_the_context_block(self):
        ctx = "[SIDECAR CONTEXT]\n- function Throttle (middleware/throttle.go:35-37) [INFERRED]\n[END SIDECAR CONTEXT]"
        self.assertEqual(S.paths_in_hook_context(ctx), ["middleware/throttle.go"])


class ArmTests(unittest.TestCase):
    def test_v3_arm_exposes_explore_only_to_itself(self):
        arms = {a["name"]: a for a in lab.parse_arms("OFF,V2,V3,V3-hook,V3+brief")}
        self.assertTrue(arms["V3"]["explore"])
        self.assertFalse(arms["V3-hook"]["explore"])
        self.assertTrue(arms["V3+brief"]["explore"])
        self.assertFalse(arms["V2"].get("explore", False))
        self.assertIsNone(arms["OFF"]["engine"])

    def test_v3_is_compared_with_v2_and_its_ablations_with_v3(self):
        plan = lab.comparison_plan(["OFF", "V2", "V3", "V3-hook"])
        self.assertIn(("V3", "V2"), plan)
        self.assertIn(("V3-hook", "V3"), plan)
        self.assertIn(("V3", "OFF"), plan)
        self.assertNotIn(("V3", "V3"), plan)

    def test_summary_aggregates_sufficiency_and_keeps_unknown(self):
        base = {"comparable": True, "success": True, "cost_usd": 0.1, "wall_s": 1.0, "tool_calls": 3, "injected_chars": 10,
                "served_models": ["m"], "oracle": {"tests": "pass", "scope_ok": True, "invariants": [], "outside_scope": []},
                "verdict": "pass"}
        r1 = dict(base, sufficiency={"moved_on": 1, "read_returned": 0, "total": 1, "redundant_rereads": 0})
        r2 = dict(base, sufficiency={"moved_on": 0, "read_returned": 2, "total": 2, "redundant_rereads": None})
        s = lab.summarize([r1, r2])["sufficiency"]
        self.assertEqual((s["moved_on"], s["read_returned"], s["total"], s["runs_with_report"]), (1, 2, 3, 2))
        self.assertIsNone(s["redundant_rereads"])
        self.assertEqual(s["moved_on_and_passed"], 1)

    @unittest.skipUnless(lab.shutil.which("bwrap") and sys.platform == "linux", "requer bwrap em Linux")
    def test_intended_sidecar_link_survives_competitor_masking(self):
        with tempfile.TemporaryDirectory() as root:
            root = pathlib.Path(root)
            fake = root / "sidecar-bin"
            fake.write_text("#!/bin/sh\necho explore-ok\n")
            fake.chmod(0o755)
            bindir = root / "bin"
            bindir.mkdir()
            (bindir / "sidecar").symlink_to("/opt/sidecar")
            (bindir / "sh").symlink_to("/usr/bin/sh")
            work = root / "work"
            work.mkdir()
            cmd = lab.agent_sandbox_command(["sidecar"], work, [(fake, "/opt/sidecar"), (bindir, "/agent-bin")], [])
            res = subprocess.run(cmd, capture_output=True, text=True)
            self.assertEqual(res.returncode, 0, res.stderr)
            self.assertEqual(res.stdout.strip(), "explore-ok")


if __name__ == "__main__":
    unittest.main()


FAKE_CLAUDE = r'''#!/bin/sh
# Fake Claude Code client for no-cost lab tests: records argv, runs one explore through the arm's tool, prints stream-json.
printf '%s\n' "$@" > /tmp/.fake_args
out=""
if command -v codegraph >/dev/null 2>&1; then out=$(cd /work && codegraph explore "ping" 2>/dev/null | head -c 2000); fi
mcp=$(grep -qx -- '--mcp-config' /tmp/.fake_args && echo '[{"name":"codegraph","status":"connected"}]' || echo '[]')
[ -n "$out" ] || { echo "codegraph explore failed" >&2; exit 3; }
echo "{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"s1\",\"plugins\":[],\"mcp_servers\":$mcp}"
echo '{"type":"result","subtype":"success","is_error":false,"result":"ok","num_turns":1,"total_cost_usd":0.0,"usage":{"input_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"output_tokens":1},"modelUsage":{"claude-haiku-5-5":{}}}'
'''


class CodeGraphArmTests(unittest.TestCase):
    def test_cg_arms_parse_and_get_only_their_tools(self):
        arms = {a["name"]: a for a in lab.parse_arms("OFF,CG,CG-mcp,V3")}
        self.assertEqual(arms["CG"]["codegraph"], "cli")
        self.assertIsNone(arms["CG"]["engine"])
        self.assertEqual(lab.arm_extra_tools(arms["CG-mcp"]), ["mcp__codegraph"])
        self.assertEqual(lab.arm_extra_tools(arms["OFF"]), [])
        self.assertIn(("V3", "CG"), lab.comparison_plan(["OFF", "CG", "V3"]))

    def test_only_the_codegraph_server_is_allowed_and_only_in_cg_mcp(self):
        reason = "servidores MCP carregados"
        parsed = {"init": {"mcp_servers": [{"name": "codegraph", "status": "connected"}]}}
        self.assertEqual(lab.allow_arm_mcp([reason], parsed, {"codegraph": "mcp"}), [])
        self.assertEqual(lab.allow_arm_mcp([reason], parsed, {"codegraph": "cli"}), [reason])
        other = {"init": {"mcp_servers": [{"name": "codegraph"}, {"name": "github"}]}}
        self.assertEqual(lab.allow_arm_mcp([reason], other, {"codegraph": "mcp"}), [reason])

    def test_codegraph_mcp_tool_counts_as_an_explore_delivery(self):
        text = "\n".join(json.dumps(e) for e in [
            {"type": "assistant", "uuid": "a1", "message": {"content": [{"type": "tool_use", "id": "t1", "name": "mcp__codegraph__codegraph_explore", "input": {"query": "x"}}]}},
            {"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "t1", "content": "**`lib/a.js`** — x\n1\tcode\n"}]}},
        ])
        ev = lab.tool_events(text)
        self.assertEqual((ev[0]["kind"], ev[0]["returned"]), ("explore", ["lib/a.js"]))

    @unittest.skipUnless(os.environ.get("LAB_CODEGRAPH_DIR") and lab.shutil.which("bwrap") and sys.platform == "linux",
                         "requer LAB_CODEGRAPH_DIR (pacote CodeGraph instalado) e bwrap")
    def test_cg_arm_runs_codegraph_in_the_sandbox_with_a_fake_client(self):
        with tempfile.TemporaryDirectory() as root:
            root = pathlib.Path(root)
            fakebin = root / "fakebin"
            fakebin.mkdir()
            (fakebin / "claude").write_text(FAKE_CLAUDE)
            (fakebin / "claude").chmod(0o755)
            work = root / "work"
            work.mkdir()
            (work / "app.js").write_text("function ping() { return pong() }\nfunction pong() { return 1 }\nmodule.exports = { ping }\n")
            subprocess.run("git init -q && git add -A && git -c user.email=a@b -c user.name=a commit -qm base", shell=True, cwd=work, check=True)
            lab.setup_paths_file(work).write_text("[]")
            sh = root / "sidecar-home"
            sh.mkdir()
            with unittest.mock.patch.dict(os.environ, {"PATH": str(fakebin) + os.pathsep + os.environ["PATH"]}):
                agent = lab.ClaudeAgent("claude-haiku-5-5", "/bin/true", None, 0.1, 120, ["Read"], deny_paths=[str(root / "deny")],
                                        codegraph=os.environ["LAB_CODEGRAPH_DIR"])
                (root / "deny").mkdir()
                for mode in ("cli", "mcp"):
                    arm = {"name": "CG" if mode == "cli" else "CG-mcp", "engine": None, "env": {}, "codegraph": mode,
                           "tools_allow": ["Read"] + lab.arm_extra_tools({"codegraph": mode})}
                    res = agent.run({"id": "t", "underspec": None}, root, work, arm, {"SIDECAR_HOME": str(sh)}, "prompt")
                    self.assertEqual(res["incomparable_reasons"], [], (mode, res["incomparable_reasons"]))
                    self.assertIsNotNone(res["index_ms"])
                    self.assertEqual(res["exit"], 0, "codegraph explore must run inside the arm sandbox")
            self.assertTrue((work / ".codegraph").is_dir())
            self.assertIn(".codegraph/", json.loads(lab.setup_paths_file(work).read_text()))
            self.assertNotIn(".codegraph/codegraph.db", lab.changed_paths(work))


ENGINE_ECHO_CLAUDE = r'''#!/bin/sh
# Fake client: reports the engine, features and tools the sandbox exposes, as the run result.
seen="engine=${SIDECAR_ENGINE:-none};features=${SIDECAR_FEATURES:-};sidecar=$(command -v sidecar >/dev/null 2>&1 && echo yes || echo no)"
echo '{"type":"system","subtype":"init","session_id":"s1","plugins":[],"mcp_servers":[]}'
echo "{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"$seen\",\"num_turns\":1,\"total_cost_usd\":0.0,\"usage\":{\"input_tokens\":1,\"cache_read_input_tokens\":0,\"cache_creation_input_tokens\":0,\"output_tokens\":1},\"modelUsage\":{\"claude-haiku-5-5\":{}}}"
'''


class ArmEnvironmentTests(unittest.TestCase):
    @unittest.skipUnless(lab.shutil.which("bwrap") and sys.platform == "linux", "requer bwrap em Linux")
    def test_each_arm_sees_its_own_engine_and_tools_in_the_sandbox(self):
        # Regression: an edit moved SIDECAR_ENGINE into the CodeGraph branch, so V2/V3 arms ran the V1 hook.
        sidecar = pathlib.Path(tempfile.mkdtemp(prefix="lab-sidecar-bin-")) / "sidecar"
        build = subprocess.run(["go", "build", "-o", str(sidecar), "./cmd/sidecar"], cwd=lab.HERE.parent.parent,
                               capture_output=True, text=True)
        if build.returncode != 0:
            self.skipTest("go build falhou: " + build.stderr[-200:])
        with tempfile.TemporaryDirectory() as root:
            root = pathlib.Path(root)
            fakebin = root / "fakebin"
            fakebin.mkdir()
            (fakebin / "claude").write_text(ENGINE_ECHO_CLAUDE)
            (fakebin / "claude").chmod(0o755)
            work = root / "work"
            work.mkdir()
            (work / "a.go").write_text("package a\n\nfunc A() int { return 1 }\n")
            subprocess.run("git init -q && git add -A && git -c user.email=a@b -c user.name=a commit -qm base", shell=True, cwd=work, check=True)
            lab.setup_paths_file(work).write_text("[]")
            (root / "deny").mkdir()
            expected = {"OFF": "engine=none;features=;sidecar=no", "V2": "engine=v2;features=;sidecar=no",
                        "V3": "engine=v3;features=;sidecar=yes", "V3-hook": "engine=v3;features=;sidecar=no",
                        "V2+brief": "engine=v2;features=brief;sidecar=no"}
            with unittest.mock.patch.dict(os.environ, {"PATH": str(fakebin) + os.pathsep + os.environ["PATH"]}):
                agent = lab.ClaudeAgent("claude-haiku-5-5", str(sidecar), None, 0.1, 120, ["Read"], deny_paths=[str(root / "deny")])
                for arm in lab.parse_arms(",".join(expected)):
                    sh = root / ("home-" + arm["name"].replace("+", "_"))
                    sh.mkdir()
                    arm = dict(arm, tools_allow=["Read"] + lab.arm_extra_tools(arm))
                    res = agent.run({"id": "t", "underspec": None}, root, work, arm, {"SIDECAR_HOME": str(sh)}, "prompt")
                    self.assertEqual(res["result"], expected[arm["name"]], arm["name"])
