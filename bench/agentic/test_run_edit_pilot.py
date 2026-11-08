import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import run_edit_pilot as pilot


class PatchScopeTests(unittest.TestCase):
    def test_snapshot_detects_new_ignored_files_and_symlinks(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp)
            (repo / ".gitignore").write_text("*.tmp\n")
            before = pilot.snapshot(repo)
            (repo / "ignored.tmp").write_text("change")
            (repo / "link").symlink_to("ignored.tmp")
            after = pilot.snapshot(repo)
            self.assertEqual(set(after) - set(before), {"ignored.tmp", "link"})
            self.assertEqual(after["link"][0], "link")


class OracleTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name)
        (self.repo / "middleware").mkdir()

    def event(self, action, test=None):
        return json.dumps({"Action": action, "Package": "github.com/go-chi/chi/v5/middleware", "Test": test})

    def test_pass_requires_both_test_and_package_events(self):
        out = self.event("pass", "TestBenchmarkRequestSizeKnownLength") + "\n" + self.event("pass")
        with patch.object(pilot, "run_limited", return_value=(0, out, "", False)):
            self.assertEqual(pilot.score(self.repo, 1)["status"], "pass")

    def test_missing_test_is_an_infrastructure_error_not_a_pass(self):
        with patch.object(pilot, "run_limited", return_value=(0, self.event("pass"), "", False)):
            self.assertEqual(pilot.score(self.repo, 1)["status"], "error")

    def test_failed_oracle_is_a_failure_and_timeout_is_unknown(self):
        out = self.event("fail", "TestBenchmarkRequestSizeKnownLength") + "\n" + self.event("fail")
        with patch.object(pilot, "run_limited", return_value=(1, out, "", False)):
            self.assertEqual(pilot.score(self.repo, 1)["status"], "fail")
        (self.repo / "middleware/sidecar_benchmark_oracle_test.go").unlink()
        with patch.object(pilot, "run_limited", return_value=(-9, "", "", True)):
            self.assertEqual(pilot.score(self.repo, 1)["status"], "error")

    def test_preexisting_oracle_path_is_not_overwritten(self):
        target = self.repo / "middleware/sidecar_benchmark_oracle_test.go"
        target.write_text("keep")
        with self.assertRaises(ValueError):
            pilot.score(self.repo, 1)
        self.assertEqual(target.read_text(), "keep")

    def test_sandbox_has_no_network_and_readonly_checkout(self):
        cmd = pilot.sandbox_command(self.repo)
        self.assertIn("--unshare-net", cmd)
        pos = cmd.index(str(self.repo))
        self.assertEqual((cmd[pos - 1], cmd[pos + 1]), ("--ro-bind", "/work"))
        self.assertIn("GOPROXY", cmd)
        self.assertEqual(cmd[cmd.index("GOPROXY") + 1], "off")


if __name__ == "__main__":
    unittest.main()
