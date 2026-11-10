import contextlib
import io
import json
import os
import pathlib
import shutil
import tempfile
import unittest
from unittest import mock

import capture_baselines
import freeze
import make_e1_manifest


ROOT = pathlib.Path(__file__).resolve().parents[2]
HERE = pathlib.Path(__file__).resolve().parent


class FreezeTests(unittest.TestCase):
    def test_checked_in_manifest_matches_frozen_cohorts(self):
        ok, message = freeze.verify_manifest()
        self.assertTrue(ok, message)

    def test_cohort_sizes_languages_and_provenance_are_declared(self):
        e1 = freeze.read_json(HERE / "e1_cases.json")
        e2 = freeze.read_json(HERE / "e2_navigation.json")
        plan = freeze.read_json(HERE / "baseline_plan.json")
        freeze.validate_cohorts(e1, e2, plan)
        self.assertEqual((len(e1["positives"]), len(e1["negatives"])), (30, 40))
        self.assertEqual(len(e2["tasks"]), 20)
        self.assertEqual(len({task["language"] for task in e2["tasks"]}), 5)
        self.assertEqual({case["prompt_language"] for case in e1["positives"]}, {"en", "pt-BR"})
        self.assertEqual({case["prompt_language"] for case in e1["negatives"]}, {"en", "pt-BR"})
        self.assertEqual(e1["authoring"]["independence"], "author-coupled")
        self.assertIsNone(e2["authoring"]["independent_reviewer"])

    def test_published_holdout_hash_is_metadata_only(self):
        published = json.loads((ROOT / "bench/lab/corpus/manifest.json").read_text(encoding="utf-8"))
        self.assertEqual(freeze.published_holdout_hash(), published["holdout_prompts_sha256"])
        self.assertEqual(
            freeze.build_manifest()["sealed_holdout_guard"]["published_holdout_prompts_sha256"],
            published["holdout_prompts_sha256"],
        )

    def test_manifest_generation_never_opens_sealed_task_files(self):
        original_open = pathlib.Path.open

        def guarded_open(path, *args, **kwargs):
            normalized = path.as_posix()
            if "/bench/lab/corpus/tasks/" in normalized:
                raise AssertionError(f"sealed task file access attempted: {normalized}")
            return original_open(path, *args, **kwargs)

        with mock.patch.object(pathlib.Path, "open", guarded_open):
            manifest = freeze.build_manifest()
        self.assertEqual(manifest["sealed_holdout_guard"]["read_policy"],
                         "hash metadata only; never enumerate or open task/oracle/reference files")

    def test_modified_case_is_detected_against_freeze_manifest(self):
        with tempfile.TemporaryDirectory(prefix="v35-freeze-test-") as tmp:
            root = pathlib.Path(tmp)
            for relative in (
                "bench/v3.5/e1_cases.json",
                "bench/v3.5/e2_navigation.json",
                "bench/v3.5/baseline_plan.json",
                "bench/v3.5/README.md",
                "bench/v3.5/freeze.py",
                "bench/v3.5/capture_baselines.py",
                "bench/v3.5/make_e1_manifest.py",
                "bench/v3.5/test_freeze.py",
                "bench/v3.5/freeze_manifest.json",
                "bench/lab/corpus/manifest.json",
            ):
                target = root / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / relative, target)
            e1_path = root / "bench/v3.5/e1_cases.json"
            e1 = json.loads(e1_path.read_text(encoding="utf-8"))
            e1["positives"][0]["prompt"] += " changed"
            e1_path.write_text(json.dumps(e1, ensure_ascii=False), encoding="utf-8")
            ok, message = freeze.verify_manifest(root)
        self.assertFalse(ok)
        self.assertIn("diferem", message)

    def test_holdout_split_is_rejected(self):
        e1 = freeze.read_json(HERE / "e1_cases.json")
        e2 = freeze.read_json(HERE / "e2_navigation.json")
        plan = freeze.read_json(HERE / "baseline_plan.json")
        e1["positives"][0]["split"] = "holdout"
        with self.assertRaises(freeze.FreezeError):
            freeze.validate_cohorts(e1, e2, plan)

    def test_source_span_hash_is_line_exact_and_bounded(self):
        content = b"one\r\ntwo\r\nthree"
        self.assertEqual(freeze.source_span_hash(content, 2, 2), freeze.sha256_bytes(b"two\r\n"))
        with self.assertRaises(freeze.FreezeError):
            freeze.source_span_hash(content, 2, 4)

    @unittest.skipUnless(os.environ.get("SIDECAR_V35_CHECKOUT"), "checkouts externos pinados não disponíveis")
    def test_gold_hashes_match_pinned_source_checkouts(self):
        messages = freeze.verify_sources(pathlib.Path(os.environ["SIDECAR_V35_CHECKOUT"]))
        self.assertEqual(len(messages), 5)

    def test_capture_plan_is_inert_without_run_flag(self):
        stdout = io.StringIO()
        with mock.patch("capture_baselines.subprocess.run", side_effect=AssertionError("must not execute Sidecar")):
            with contextlib.redirect_stdout(stdout):
                code = capture_baselines.main([
                    "--sidecar", "/missing/sidecar",
                    "--checkout", "/missing/repos",
                    "--suite", "e1,e2",
                    "--out", "/tmp/unused-v35-baseline.json",
                ])
        self.assertEqual(code, 0)
        self.assertIn("1080 invocações locais", stdout.getvalue())
        self.assertFalse(pathlib.Path("/tmp/unused-v35-baseline.json").exists())

    def test_e1_converter_preserves_frozen_gold_without_holdout(self):
        manifest = make_e1_manifest.external_manifest(pathlib.Path("/external-checkouts"))
        self.assertEqual(len(manifest["cases"]), 70)
        self.assertEqual(sum(bool(case["relevant"]) for case in manifest["cases"]), 30)
        self.assertEqual(sum(case.get("no_gold", False) for case in manifest["cases"]), 40)
        self.assertEqual({case["split"] for case in manifest["cases"]}, {"v35-validation"})
        self.assertTrue(all("holdout" not in case["id"].lower() for case in manifest["cases"]))

    def test_e2_term_coverage_is_diagnostic_and_matches_any_gold_alternative(self):
        oracle = {"facts": ["route registration"], "answer_must_mention": [["path_router.route(", "route_service("]]}
        self.assertEqual(capture_baselines.fact_coverage("this.path_router.route(path, method)", oracle), (1, 1, []))
        self.assertEqual(capture_baselines.fact_coverage("no route output", oracle), (0, 1, ["route registration"]))


if __name__ == "__main__":
    unittest.main()
