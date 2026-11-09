import json
import unittest

import real_hook_pilot as pilot

CTX = "[SIDECAR CONTEXT]\nRELEVANT CODE\n- function Release (release/handler.go:4-4) [INFERRED]\n[END SIDECAR CONTEXT]"


def stream(hook_output=None):
    """Mirrors the event shapes observed in a real claude -p --output-format stream-json probe."""
    events = [
        {"type": "system", "subtype": "init"},
        {"type": "system", "subtype": "hook_started", "hook_name": "UserPromptSubmit", "hook_event": "UserPromptSubmit"},
    ]
    if hook_output is not None:
        events.append({"type": "system", "subtype": "hook_response", "hook_name": "UserPromptSubmit",
                       "hook_event": "UserPromptSubmit", "output": hook_output})
    events += [
        {"type": "assistant", "message": {"content": [{"type": "tool_use", "name": "Grep", "input": {}}]}},
        {"type": "assistant", "message": {"content": [{"type": "text", "text": "release/handler.go"}]}},
        {"type": "result", "subtype": "success", "is_error": False, "result": '{"files": ["release/handler.go"]}',
         "num_turns": 2, "total_cost_usd": 0.0083,
         "usage": {"input_tokens": 2, "cache_creation_input_tokens": 41533, "cache_read_input_tokens": 0, "output_tokens": 66},
         "modelUsage": {"claude-haiku-5-5": {"inputTokens": 2}}},
    ]
    return "\n".join(json.dumps(e) for e in events)


class StreamTests(unittest.TestCase):
    def test_delivery_is_read_from_hook_response_not_from_firing(self):
        out = json.dumps({"hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": CTX}})
        p = pilot.parse_stream(stream(out))
        self.assertTrue(p["hook_started"])
        self.assertTrue(p["hook_delivered"])
        self.assertEqual(p["hook_context"], CTX)

    def test_fired_hook_without_context_is_not_delivery(self):
        p = pilot.parse_stream(stream(None))
        self.assertTrue(p["hook_started"])
        self.assertFalse(p["hook_delivered"])
        self.assertEqual(p["hook_context"], "")

    def test_malformed_hook_output_is_not_delivery(self):
        p = pilot.parse_stream(stream("not json"))
        self.assertFalse(p["hook_delivered"])

    def test_cost_tools_and_served_model_come_from_the_result(self):
        p = pilot.parse_stream(stream(None))
        self.assertEqual(p["cost_usd"], 0.0083)
        self.assertEqual(p["tool_calls"], 1)
        self.assertEqual(p["served_models"], ["claude-haiku-5-5"])
        self.assertEqual(p["tokens_in"], 2 + 41533)
        self.assertEqual(p["tokens_out"], 66)
        self.assertEqual(p["result"], '{"files": ["release/handler.go"]}')

    def test_usage_counters_keep_uncached_cache_read_cache_create_and_output_separate(self):
        p = pilot.parse_stream(stream(None))
        self.assertEqual((p["input_uncached"], p["cache_read"], p["cache_create"], p["output"]), (2, 0, 41533, 66))

    def test_explicit_zero_usage_is_zero_but_missing_or_malformed_stays_unknown(self):
        events = [json.loads(line) for line in stream(None).splitlines()]
        result = events[-1]
        result["usage"] = {"input_tokens": 0, "cache_creation_input_tokens": 0,
                           "cache_read_input_tokens": 0, "output_tokens": 0}
        zero = pilot.parse_stream("\n".join(json.dumps(e) for e in events))
        self.assertEqual((zero["input_uncached"], zero["cache_read"], zero["cache_create"], zero["output"], zero["tokens_in"]),
                         (0, 0, 0, 0, 0))

        result["usage"] = {"input_tokens": 2}
        missing = pilot.parse_stream("\n".join(json.dumps(e) for e in events))
        self.assertEqual(missing["input_uncached"], 2)
        self.assertIsNone(missing["cache_read"])
        self.assertIsNone(missing["tokens_in"])
        self.assertIsNone(missing["tokens_out"])

        result["usage"] = {"input_tokens": "bad", "cache_creation_input_tokens": -1}
        malformed = pilot.parse_stream("\n".join(json.dumps(e) for e in events))
        self.assertIsNone(malformed["input_uncached"])
        self.assertIsNone(malformed["cache_create"])
        self.assertIsNone(malformed["tokens_in"])

    def test_missing_usage_object_does_not_become_zero(self):
        events = [json.loads(line) for line in stream(None).splitlines()]
        del events[-1]["usage"]
        parsed = pilot.parse_stream("\n".join(json.dumps(e) for e in events))
        for key in ("input_uncached", "cache_read", "cache_create", "output", "tokens_in", "tokens_out"):
            self.assertIsNone(parsed[key], key)

    def test_truncated_stream_does_not_claim_hook_was_not_delivered(self):
        parsed = pilot.parse_stream('{"type":"system","subtype":"hook_started","hook_event":"UserPromptSubmit"}')
        self.assertIsNone(parsed["hook_delivered"])
        self.assertIsNone(parsed["tool_calls"])


class CommandTests(unittest.TestCase):
    def test_off_arm_registers_no_hook(self):
        cmd = pilot.build_claude_cmd("claude-haiku-5-5", "pergunta", 0.25, None)
        self.assertNotIn("--settings", cmd)
        self.assertIn("--include-hook-events", cmd)
        self.assertEqual(cmd[cmd.index("--setting-sources") + 1], "project,local")
        self.assertNotIn("--real-hooks", cmd)

    def test_on_arm_registers_only_the_user_prompt_submit_hook(self):
        settings = pilot.hook_settings_json("/opt/sidecar")
        cmd = pilot.build_claude_cmd("claude-haiku-5-5", "pergunta", 0.25, settings)
        self.assertEqual(cmd[cmd.index("--settings") + 1], settings)
        hooks = json.loads(settings)["hooks"]
        self.assertEqual(list(hooks), ["UserPromptSubmit"])
        self.assertEqual(hooks["UserPromptSubmit"][0]["hooks"][0]["command"], "/opt/sidecar hook --agent claude")

    def test_model_alias_is_rejected_before_any_spend(self):
        import run_agents
        with self.assertRaises(ValueError):
            run_agents.validate_model("claude", "haiku")


def init_event(plugins=(), mcp=()):
    return json.dumps({"type": "system", "subtype": "init", "plugins": [
        {"name": n, "source": f"{n}@synced" if n not in pilot.BUILTIN_PLUGINS else f"{n}@builtin"} for n in
        list(plugins) + sorted(pilot.BUILTIN_PLUGINS)], "mcp_servers": list(mcp), "skills": []})


class ComparabilityTests(unittest.TestCase):
    def comparable_stream(self, served=("claude-haiku-5-5",), extra_init=()):
        base = stream(None).splitlines()
        return "\n".join([init_event(extra_init)] + base).replace(
            '"claude-haiku-5-5"', json.dumps(list(served)[0]) if served else "null")

    def test_builtin_only_init_with_requested_model_is_comparable(self):
        p = pilot.parse_stream(self.comparable_stream())
        self.assertEqual(pilot.contamination(p, False, "claude-haiku-5-5"), [])

    def test_synced_global_plugin_makes_run_incomparable(self):
        p = pilot.parse_stream(self.comparable_stream(extra_init=["superpowers"]))
        reasons = pilot.contamination(p, False, "claude-haiku-5-5")
        self.assertTrue(any("superpowers" in r for r in reasons), reasons)

    def test_mcp_server_makes_run_incomparable(self):
        p = pilot.parse_stream(init_event(mcp=["x"]) + "\n" + stream(None))
        self.assertIn("servidores MCP carregados", pilot.contamination(p, False, "claude-haiku-5-5"))

    def test_firing_canary_makes_run_incomparable(self):
        p = pilot.parse_stream(self.comparable_stream())
        self.assertTrue(any("canário" in r for r in pilot.contamination(p, True, "claude-haiku-5-5")))

    def test_unexpected_served_model_is_recorded_and_incomparable(self):
        p = pilot.parse_stream(self.comparable_stream(served=("claude-sonnet-5-5",)))
        reasons = pilot.contamination(p, False, "claude-haiku-5-5")
        self.assertTrue(any("difere do solicitado" in r for r in reasons), reasons)

    def test_stream_without_result_or_init_is_incomparable(self):
        p = pilot.parse_stream(stream(None).split("\n", 1)[1])  # drop the first event entirely
        reasons = pilot.contamination(p, False, "claude-haiku-5-5")
        self.assertTrue(any("sem evento init" in r for r in reasons))
        truncated = pilot.parse_stream('{"type": "system", "subtype": "init", "plugins": [], "mcp_servers": []}')
        self.assertTrue(any("incompleto" in r for r in pilot.contamination(truncated, False, "claude-haiku-5-5")))

    def test_missing_cost_is_not_zero(self):
        raw = stream(None).replace('"total_cost_usd": 0.0083', '"x": 1')
        p = pilot.parse_stream(raw)
        self.assertIsNone(p["cost_usd"])


if __name__ == "__main__":
    unittest.main()
