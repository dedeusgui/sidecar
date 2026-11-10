#!/usr/bin/env python3
"""Freeze and verify V3.5 E1/E2 manifests without inspecting sealed benchmark files."""

from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import re
import subprocess
import sys
from typing import Any


HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]
E1_PATH = HERE / "e1_cases.json"
E2_PATH = HERE / "e2_navigation.json"
PLAN_PATH = HERE / "baseline_plan.json"
FREEZE_PATH = HERE / "freeze_manifest.json"
PUBLISHED_HOLDOUT_PATH = ROOT / "bench" / "lab" / "corpus" / "manifest.json"
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
COMMIT_RE = re.compile(r"^[0-9a-f]{40}$")


class FreezeError(ValueError):
    """A cohort cannot be frozen or verified as declared."""


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def canonical_sha256(value: Any) -> str:
    encoded = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return sha256_bytes(encoded)


def read_json(path: pathlib.Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def safe_relative_path(value: str) -> pathlib.PurePosixPath:
    path = pathlib.PurePosixPath(value)
    if not value or path.is_absolute() or any(part in ("", ".", "..") for part in path.parts):
        raise FreezeError(f"caminho de gold inválido: {value!r}")
    return path


def _validate_repos(repos: dict[str, Any], label: str) -> None:
    if not repos:
        raise FreezeError(f"{label}: nenhum repositório declarado")
    for slug, repo in repos.items():
        if not isinstance(slug, str) or not slug or not isinstance(repo, dict):
            raise FreezeError(f"{label}: metadados de repositório inválidos")
        if not COMMIT_RE.fullmatch(str(repo.get("commit", ""))):
            raise FreezeError(f"{label}/{slug}: commit precisa ser SHA-1 completo")
        if not str(repo.get("source", "")).startswith("https://"):
            raise FreezeError(f"{label}/{slug}: fonte HTTPS ausente")
        if not repo.get("language") or not repo.get("license"):
            raise FreezeError(f"{label}/{slug}: linguagem/licença ausente")


def validate_cohorts(e1: dict[str, Any], e2: dict[str, Any], plan: dict[str, Any]) -> None:
    if e1.get("schema") != "sidecar-v35-e1-cases/1" or e2.get("schema") != "sidecar-v35-e2-navigation/1":
        raise FreezeError("schema de coorte E1/E2 inválido")
    if e1.get("freeze_status") != "frozen-before-tuning" or e2.get("freeze_status") != "frozen-before-tuning":
        raise FreezeError("coortes devem declarar freeze anterior ao tuning")
    if e2.get("split") != "validation":
        raise FreezeError("E2 deve ser uma partição de validation")
    _validate_repos(e1.get("repos", {}), "E1")
    _validate_repos(e2.get("repos", {}), "E2")
    if e1["repos"] != e2["repos"]:
        raise FreezeError("E1/E2 devem usar a mesma proveniência de repositórios nesta versão")

    positives = e1.get("positives", [])
    negatives = e1.get("negatives", [])
    if len(positives) < 30 or len(negatives) < 40:
        raise FreezeError("E1 requer pelo menos 30 positivos e 40 negativos")
    e1_cases = positives + negatives
    e2_tasks = e2.get("tasks", [])
    if len(e2_tasks) < 20:
        raise FreezeError("E2 requer pelo menos 20 tarefas")
    if len({case.get("language") for case in e1_cases}) < 5 or len({task.get("language") for task in e2_tasks}) < 5:
        raise FreezeError("E1 e E2 devem cobrir pelo menos cinco linguagens")
    positive_languages = {case.get("category", "").rsplit("_", 1)[-1] for case in positives}
    if not {"en", "pt"}.issubset(positive_languages):
        raise FreezeError("E1 positivos devem conter pedidos curtos em inglês e PT-BR")

    for cases, label in ((e1_cases, "E1"), (e2_tasks, "E2")):
        ids = [case.get("id") for case in cases]
        if any(not case_id for case_id in ids) or len(set(ids)) != len(ids):
            raise FreezeError(f"{label}: IDs ausentes ou duplicados")
        if any("holdout" in str(case_id).lower() for case_id in ids):
            raise FreezeError(f"{label}: IDs de holdout não podem ser incluídos")
        for case in cases:
            if case.get("repo") not in e1["repos"]:
                raise FreezeError(f"{label}/{case.get('id')}: repositório sem provenance")
            if case.get("language") != e1["repos"][case["repo"]]["language"]:
                raise FreezeError(f"{label}/{case.get('id')}: linguagem não bate com a fonte")
            if "holdout" in str(case.get("split", "")).lower():
                raise FreezeError(f"{label}/{case.get('id')}: split de holdout proibido")
            if len(str(case.get("prompt", ""))) > 500:
                raise FreezeError(f"{label}/{case.get('id')}: prompt excede limite do Stage 0")
    if any(case.get("prompt_language") not in {"en", "pt-BR"} for case in e1_cases):
        raise FreezeError("cada prompt E1 deve declarar en ou pt-BR")
    if any({case.get("prompt_language") for case in cases} != {"en", "pt-BR"} for cases in (positives, negatives)):
        raise FreezeError("E1 positivos e negativos devem conter inglês e PT-BR")

    for case in positives:
        relevant = case.get("relevant", [])
        if not relevant:
            raise FreezeError(f"{case['id']}: positivo sem gold")
        for gold in relevant:
            safe_relative_path(gold.get("path", ""))
            if not SHA256_RE.fullmatch(str(gold.get("sha256", ""))):
                raise FreezeError(f"{case['id']}: hash do arquivo gold ausente")
    for case in negatives:
        if case.get("no_gold") is not True or case.get("category") not in {"near_miss", "unrelated"}:
            raise FreezeError(f"{case.get('id')}: negativo precisa ser no_gold near_miss/unrelated")

    for task in e2_tasks:
        oracle = task.get("oracle", {})
        facts = oracle.get("facts", [])
        fact_checks = oracle.get("answer_must_mention", [])
        if not facts or not oracle.get("spans") or len(facts) != len(fact_checks):
            raise FreezeError(f"{task['id']}: oracle de navegação sem fatos/span")
        if any(not check or any(not isinstance(value, str) or not value for value in check) for check in fact_checks):
            raise FreezeError(f"{task['id']}: termos verificáveis de fato inválidos")
        for span in oracle["spans"]:
            safe_relative_path(span.get("path", ""))
            start, end = span.get("line_start"), span.get("line_end")
            if not isinstance(start, int) or not isinstance(end, int) or start < 1 or end < start:
                raise FreezeError(f"{task['id']}: intervalo de span inválido")
            if not SHA256_RE.fullmatch(str(span.get("sha256", ""))):
                raise FreezeError(f"{task['id']}: hash de span ausente")

    if plan.get("schema") != "sidecar-v35-baseline-plan/1" or plan.get("status") != "not-captured":
        raise FreezeError("baseline deve permanecer explicitamente não capturado no Stage 0")
    if plan.get("engines") != ["v1", "v2", "v3"]:
        raise FreezeError("baseline deve cobrir V1/V2/V3")
    if plan.get("extract_version") != 3 or not COMMIT_RE.fullmatch(str(plan.get("sidecar_commit", ""))):
        raise FreezeError("versão/commit da baseline ausente ou inválido")
    if "--run" not in str(plan.get("capture_command", "")):
        raise FreezeError("comando de captura precisa exigir --run explicitamente")


def _case_records(cases: list[dict[str, Any]]) -> list[dict[str, str]]:
    return [{"id": case["id"], "sha256": canonical_sha256(case)} for case in sorted(cases, key=lambda item: item["id"])]


def published_holdout_hash(path: pathlib.Path = PUBLISHED_HOLDOUT_PATH) -> str:
    """Read only the already-published hash field from the lab manifest, never task files."""
    manifest = read_json(path)
    digest = manifest.get("holdout_prompts_sha256")
    if not SHA256_RE.fullmatch(str(digest or "")):
        raise FreezeError(f"hash de holdout publicado ausente em {path}")
    return digest


def build_manifest(root: pathlib.Path = ROOT) -> dict[str, Any]:
    e1_path = root / "bench" / "v3.5" / "e1_cases.json"
    e2_path = root / "bench" / "v3.5" / "e2_navigation.json"
    plan_path = root / "bench" / "v3.5" / "baseline_plan.json"
    readme_path = root / "bench" / "v3.5" / "README.md"
    freeze_tool_path = root / "bench" / "v3.5" / "freeze.py"
    capture_tool_path = root / "bench" / "v3.5" / "capture_baselines.py"
    converter_tool_path = root / "bench" / "v3.5" / "make_e1_manifest.py"
    test_path = root / "bench" / "v3.5" / "test_freeze.py"
    e1 = read_json(e1_path)
    e2 = read_json(e2_path)
    plan = read_json(plan_path)
    validate_cohorts(e1, e2, plan)

    source_files: dict[str, dict[str, str]] = {slug: {} for slug in sorted(e1["repos"])}
    source_spans: dict[str, dict[str, str]] = {slug: {} for slug in sorted(e1["repos"])}
    for case in e1["positives"]:
        for gold in case["relevant"]:
            source_files[case["repo"]][gold["path"]] = gold["sha256"]
    for task in e2["tasks"]:
        for span in task["oracle"]["spans"]:
            key = f"{span['path']}:{span['line_start']}-{span['line_end']}"
            prior = source_spans[task["repo"]].setdefault(key, span["sha256"])
            if prior != span["sha256"]:
                raise FreezeError(f"hashes inconsistentes para {task['repo']}/{key}")

    def artifact(path: pathlib.Path, root_path: pathlib.Path) -> dict[str, str]:
        return {"path": path.relative_to(root_path).as_posix(), "sha256": sha256_bytes(path.read_bytes())}

    return {
        "schema": "sidecar-v35-stage0-freeze/1",
        "cohort_freeze": "2026-11-09",
        "baseline_commit": plan["sidecar_commit"],
        "extract_version": plan["extract_version"],
        "artifacts": {
            "e1": artifact(e1_path, root),
            "e2": artifact(e2_path, root),
            "baseline_plan": artifact(plan_path, root),
            "readme": artifact(readme_path, root),
            "freeze_tool": artifact(freeze_tool_path, root),
            "capture_tool": artifact(capture_tool_path, root),
            "e1_converter": artifact(converter_tool_path, root),
            "tests": artifact(test_path, root),
        },
        "cohorts": {
            "e1": {
                "id": e1["cohort_id"],
                "positive_count": len(e1["positives"]),
                "negative_count": len(e1["negatives"]),
                "languages": sorted({case["language"] for case in e1["positives"] + e1["negatives"]}),
                "prompt_languages": sorted({case["prompt_language"] for case in e1["positives"] + e1["negatives"]}),
                "independence": e1["authoring"]["independence"],
                "case_hashes": _case_records(e1["positives"] + e1["negatives"]),
            },
            "e2": {
                "id": e2["cohort_id"],
                "split": e2["split"],
                "task_count": len(e2["tasks"]),
                "languages": sorted({task["language"] for task in e2["tasks"]}),
                "independence": e2["authoring"]["independence"],
                "gold_status": e2["authoring"]["gold_status"],
                "case_hashes": _case_records(e2["tasks"]),
            },
        },
        "repositories": {
            slug: {
                **metadata,
                "gold_file_hashes": dict(sorted(source_files[slug].items())),
                "gold_span_hashes": dict(sorted(source_spans[slug].items())),
            }
            for slug, metadata in sorted(e1["repos"].items())
        },
        "baseline_snapshots": {
            "status": plan["status"],
            "required_engines": plan["engines"],
            "required_surfaces": plan["surfaces"],
            "required_cache_states": plan["cache_states"],
            "capture_command": plan["capture_command"],
        },
        "sealed_holdout_guard": {
            "metadata_source": "bench/lab/corpus/manifest.json",
            "published_holdout_prompts_sha256": published_holdout_hash(root / "bench" / "lab" / "corpus" / "manifest.json"),
            "read_policy": "hash metadata only; never enumerate or open task/oracle/reference files",
        },
    }


def verify_manifest(root: pathlib.Path = ROOT) -> tuple[bool, str]:
    manifest_path = root / "bench" / "v3.5" / "freeze_manifest.json"
    if not manifest_path.is_file():
        return False, "freeze_manifest.json ausente; rode freeze.py freeze"
    expected = read_json(manifest_path)
    actual = build_manifest(root)
    if expected != actual:
        return False, "hashes/proveniência diferem do freeze_manifest.json"
    return True, "coortes E1/E2 e baseline plan conferem com o manifesto congelado"


def _repo_head(repo_root: pathlib.Path) -> str:
    result = subprocess.run(
        ["git", "-C", str(repo_root), "rev-parse", "HEAD"],
        check=True,
        capture_output=True,
        text=True,
        timeout=30,
    )
    return result.stdout.strip()


def _file_bytes(repo_root: pathlib.Path, relative: str) -> bytes:
    path = safe_relative_path(relative)
    result = subprocess.run(
        ["git", "-C", str(repo_root), "show", f"HEAD:{path.as_posix()}"],
        check=False,
        capture_output=True,
        timeout=30,
    )
    if result.returncode != 0:
        raise FreezeError(f"gold file ausente no commit pinado: {relative}")
    return result.stdout


def source_span_hash(content: bytes, line_start: int, line_end: int) -> str:
    lines = content.splitlines(keepends=True)
    if line_end > len(lines):
        raise FreezeError(f"span fora do arquivo: linhas {line_start}-{line_end}, arquivo tem {len(lines)} linhas")
    return sha256_bytes(b"".join(lines[line_start - 1 : line_end]))


def verify_sources(checkout: pathlib.Path, root: pathlib.Path = ROOT) -> list[str]:
    e1 = read_json(root / "bench" / "v3.5" / "e1_cases.json")
    e2 = read_json(root / "bench" / "v3.5" / "e2_navigation.json")
    messages: list[str] = []
    repos = e1["repos"]
    for slug, metadata in repos.items():
        repo_root = checkout / slug
        actual_commit = _repo_head(repo_root)
        if actual_commit != metadata["commit"]:
            raise FreezeError(f"{slug}: checkout {actual_commit} != commit congelado {metadata['commit']}")
        messages.append(f"{slug}: {actual_commit}")
    for case in e1["positives"]:
        repo_root = checkout / case["repo"]
        for gold in case["relevant"]:
            actual = sha256_bytes(_file_bytes(repo_root, gold["path"]))
            if actual != gold["sha256"]:
                raise FreezeError(f"{case['id']}: hash do arquivo {gold['path']} não confere")
    for task in e2["tasks"]:
        repo_root = checkout / task["repo"]
        for span in task["oracle"]["spans"]:
            actual = source_span_hash(_file_bytes(repo_root, span["path"]), span["line_start"], span["line_end"])
            if actual != span["sha256"]:
                raise FreezeError(f"{task['id']}: hash do span {span['path']}:{span['line_start']}-{span['line_end']} não confere")
    return messages


def refresh_source_hashes(checkout: pathlib.Path, root: pathlib.Path = ROOT) -> None:
    e1_path = root / "bench" / "v3.5" / "e1_cases.json"
    e2_path = root / "bench" / "v3.5" / "e2_navigation.json"
    e1 = read_json(e1_path)
    e2 = read_json(e2_path)
    for slug, metadata in e1["repos"].items():
        actual_commit = _repo_head(checkout / slug)
        if actual_commit != metadata["commit"]:
            raise FreezeError(f"{slug}: checkout {actual_commit} != commit congelado {metadata['commit']}")
    for case in e1["positives"]:
        repo_root = checkout / case["repo"]
        for gold in case["relevant"]:
            gold["sha256"] = sha256_bytes(_file_bytes(repo_root, gold["path"]))
    for task in e2["tasks"]:
        repo_root = checkout / task["repo"]
        for span in task["oracle"]["spans"]:
            span["sha256"] = source_span_hash(
                _file_bytes(repo_root, span["path"]), span["line_start"], span["line_end"]
            )
    e1_path.write_text(json.dumps(e1, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    e2_path.write_text(json.dumps(e2, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def write_manifest(root: pathlib.Path = ROOT) -> None:
    manifest = build_manifest(root)
    path = root / "bench" / "v3.5" / "freeze_manifest.json"
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("freeze", help="recalcula e grava os hashes congelados")
    sub.add_parser("verify", help="verifica os hashes sem alterar arquivos")
    for name in ("verify-sources", "refresh-source-hashes"):
        command = sub.add_parser(name)
        command.add_argument("--checkout", required=True, type=pathlib.Path, help="diretório com checkouts nos commits pinados")
    args = parser.parse_args(argv)
    try:
        if args.command == "freeze":
            write_manifest()
            print(f"manifesto congelado: {FREEZE_PATH.relative_to(ROOT)}")
        elif args.command == "verify":
            ok, message = verify_manifest()
            print(message)
            return 0 if ok else 1
        elif args.command == "verify-sources":
            for message in verify_sources(args.checkout):
                print(message)
            print("gold E1/E2 confere com os commits e hashes pinados")
        elif args.command == "refresh-source-hashes":
            refresh_source_hashes(args.checkout)
            print("hashes de arquivos E1 e spans E2 atualizados; rode freeze.py freeze em seguida")
    except (FreezeError, OSError, json.JSONDecodeError, subprocess.CalledProcessError) as exc:
        print(f"freeze: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
