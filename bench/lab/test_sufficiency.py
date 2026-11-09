import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

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
