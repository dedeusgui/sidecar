import copy
import json
import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import consolidate as c  # noqa: E402

REG = json.loads((pathlib.Path(__file__).resolve().parent / "registry.json").read_text())


class RegistryTests(unittest.TestCase):
    def test_committed_registry_is_valid(self):
        self.assertEqual(c.validate(REG), [])

    def test_missing_field_unknown_level_and_missing_artifact_are_errors(self):
        bad = copy.deepcopy(REG)
        bad["benchmarks"][0]["caveat"] = ""
        bad["benchmarks"][1]["level"] = "magico"
        bad["benchmarks"][2]["artifacts"] = ["nao/existe.md"]
        errs = " | ".join(c.validate(bad))
        self.assertIn("caveat", errs)
        self.assertIn("nível de evidência desconhecido", errs)
        self.assertIn("artefato inexistente", errs)

    def test_only_agentic_benchmarks_may_claim_gain(self):
        bad = copy.deepcopy(REG)
        bad["benchmarks"][0]["headline"] = "ganho de 20%"
        self.assertTrue(any("só benchmark agentic" in e for e in c.validate(bad)))

    def test_leads_must_cite_existing_benchmarks(self):
        bad = copy.deepcopy(REG)
        bad["improvement_leads"][0]["source"] = ["B99"]
        self.assertTrue(any("inexistente" in e for e in c.validate(bad)))

    def test_render_has_every_benchmark_and_lead(self):
        text = c.render(REG)
        for b in REG["benchmarks"]:
            self.assertIn(b["id"], text)
        for l in REG["improvement_leads"]:
            self.assertIn(l["id"], text)
        self.assertIn("Não edite à mão", text)

    def test_pre_fix_agentic_pilots_are_not_marked_as_reference(self):
        for b in REG["benchmarks"]:
            if b["id"] in ("B10-lab-pilot-dev1", "B11-lab-pilot-r3"):
                self.assertIn("SUPERADO", b["status"])
                self.assertIn("contaminado", b["caveat"])


if __name__ == "__main__":
    unittest.main()
