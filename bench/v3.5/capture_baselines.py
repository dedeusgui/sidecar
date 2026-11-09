#!/usr/bin/env python3
"""Capture local V1/V2/V3 output snapshots for the frozen E1/E2 cohorts.

This tool is intentionally inert unless ``--run`` is passed. It never loads the
legacy lab task corpus, paid-agent traces, or any holdout files.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
from typing import Any, Iterable

import freeze


HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]
ENGINES = ("v1", "v2", "v3")
SURFACES = ("explain-json", "codex-prompt-hook")
CACHE_STATES = ("cold", "warm")


def _load_cases(suite: str) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    if suite == "e1":
        data = freeze.read_json(HERE / "e1_cases.json")
        cases = data["positives"] + data["negatives"]
        return data, cases
    if suite == "e2":
        data = freeze.read_json(HERE / "e2_navigation.json")
        return data, data["tasks"]
    raise ValueError(f"suite desconhecida: {suite}")


def planned_invocations(suites: Iterable[str]) -> int:
    count = 0
    for suite in suites:
        _, cases = _load_cases(suite)
        count += len(cases) * len(ENGINES) * len(SURFACES) * len(CACHE_STATES)
    return count


def _tree_sha256(home: pathlib.Path) -> str:
    """Hash only Sidecar index-store files from the isolated cache home."""
    digest = hashlib.sha256()
    files = sorted(
        path for path in home.glob("repos/*/index.db*")
        if path.is_file()
    )
    for path in files:
        digest.update(path.relative_to(home).as_posix().encode("utf-8"))
        digest.update(b"\0")
        digest.update(path.read_bytes())
        digest.update(b"\0")
    return digest.hexdigest()


def _context_from_explain(raw: str) -> tuple[str, list[dict[str, str]], dict[str, Any]]:
    report = json.loads(raw)
    evidence = [
        {"path": item.get("path", ""), "lines": item.get("lines", "")}
        for item in report.get("evidence", [])
    ]
    return str(report.get("context", "")), evidence, report


def _context_from_hook(raw: str) -> str:
    if not raw.strip():
        return ""
    payload = json.loads(raw)
    return str(((payload.get("hookSpecificOutput") or {}).get("additionalContext")) or "")


def fact_coverage(context: str, oracle: dict[str, Any]) -> tuple[int, int, list[str]]:
    """Return diagnostic source-term coverage; this is not an answer-correctness score."""
    needles = oracle.get("answer_must_mention", [])
    facts = oracle.get("facts", [])
    normalized = context.lower()
    missing = [
        facts[index]
        for index, alternatives in enumerate(needles)
        if not any(term.lower() in normalized for term in alternatives)
    ]
    return len(needles) - len(missing), len(needles), missing


def _invoke(
    sidecar: pathlib.Path,
    repo_root: pathlib.Path,
    prompt: str,
    engine: str,
    surface: str,
    home: pathlib.Path,
) -> tuple[str, list[dict[str, str]] | None, dict[str, Any] | None, float]:
    env = os.environ.copy()
    env.update({
        "SIDECAR_HOME": str(home),
        "SIDECAR_CONFIG_HOME": str(home / "config"),
        "SIDECAR_ENGINE": engine,
        "SIDECAR_FEATURES": "",
    })
    if surface == "explain-json":
        command = [str(sidecar), "explain", "--engine", engine, "--root", str(repo_root), "--json"]
        input_text = prompt
    elif surface == "codex-prompt-hook":
        command = [str(sidecar), "hook", "--agent", "codex"]
        input_text = json.dumps({
            "session_id": "stage0-baseline",
            "cwd": str(repo_root),
            "prompt": prompt,
            "hook_event_name": "UserPromptSubmit",
        })
    else:
        raise ValueError(f"surface desconhecida: {surface}")
    started = time.perf_counter()
    result = subprocess.run(command, input=input_text, capture_output=True, text=True, env=env, timeout=600)
    elapsed_ms = round((time.perf_counter() - started) * 1000, 3)
    if result.returncode != 0:
        raise RuntimeError(f"{command[1:3]} falhou ({result.returncode}): {result.stderr[-400:]}")
    if surface == "explain-json":
        context, evidence, report = _context_from_explain(result.stdout)
        return context, evidence, report, elapsed_ms
    context = _context_from_hook(result.stdout)
    return context, None, None, elapsed_ms


def _capture_case(
    sidecar: pathlib.Path,
    repo_root: pathlib.Path,
    case: dict[str, Any],
    engine: str,
    surface: str,
    temp_root: pathlib.Path,
) -> list[dict[str, Any]]:
    home = pathlib.Path(tempfile.mkdtemp(prefix="sidecar-v35-", dir=temp_root))
    rows = []
    for cache_state in CACHE_STATES:
        context, evidence, report, elapsed_ms = _invoke(
            sidecar, repo_root, case["prompt"], engine, surface, home
        )
        index_hash = _tree_sha256(home)
        row = {
            "case_id": case["id"],
            "engine": engine,
            "surface": surface,
            "cache_state": cache_state,
            "repo_commit": freeze._repo_head(repo_root),
            "elapsed_ms": elapsed_ms,
            "index_store_sha256": index_hash,
            "context_sha256": freeze.sha256_bytes(context.encode("utf-8")),
            "context_chars": len(context),
            "evidence_paths_and_lines": evidence,
            "facts_hit": None,
            "facts_total": None,
            "facts_missing": None,
            "output": context,
        }
        if case.get("oracle"):
            covered, total, missing = fact_coverage(context, case["oracle"])
            row["facts_hit"] = covered
            row["facts_total"] = total
            row["facts_missing"] = missing
        if report is not None:
            row["engine_report"] = report
            row["reported_timings_ms"] = report.get("timings_ms", {})
            row["sync"] = report.get("sync", {})
        rows.append(row)
    return rows


def capture(sidecar: pathlib.Path, checkout: pathlib.Path, suites: list[str], out: pathlib.Path) -> dict[str, Any]:
    ok, message = freeze.verify_manifest()
    if not ok:
        raise freeze.FreezeError(message)
    freeze.verify_sources(checkout)
    if not sidecar.is_file():
        raise freeze.FreezeError(f"binário Sidecar ausente: {sidecar}")
    binary_sha = freeze.sha256_bytes(sidecar.read_bytes())
    plan = freeze.read_json(HERE / "baseline_plan.json")
    rows: list[dict[str, Any]] = []
    with tempfile.TemporaryDirectory(prefix="sidecar-v35-snapshot-") as temp_name:
        temp_root = pathlib.Path(temp_name)
        for suite in suites:
            _, cases = _load_cases(suite)
            for case in cases:
                repo_root = checkout / case["repo"]
                for engine in ENGINES:
                    for surface in SURFACES:
                        rows.extend(_capture_case(sidecar, repo_root, case, engine, surface, temp_root))
    artifact = {
        "schema": "sidecar-v35-baseline-snapshot/1",
        "captured_at_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "baseline_commit_declared": plan["sidecar_commit"],
        "extract_version": plan["extract_version"],
        "binary_sha256": binary_sha,
        "cohort_manifest_sha256": freeze.sha256_bytes((HERE / "freeze_manifest.json").read_bytes()),
        "suites": suites,
        "rows": rows,
        "limitations": [
            "Offline deterministic output snapshot only; not an agentic task result.",
            "Timing values are local diagnostics and require a controlled machine for comparison.",
            "The declared source commit for the Sidecar binary must be verified by the operator when building it.",
        ],
    }
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(artifact, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return artifact


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sidecar", required=True, type=pathlib.Path)
    parser.add_argument("--checkout", required=True, type=pathlib.Path)
    parser.add_argument("--suite", default="e1,e2", help="lista separada por vírgula: e1,e2")
    parser.add_argument("--out", required=True, type=pathlib.Path)
    parser.add_argument("--run", action="store_true", help="executa as capturas locais explicitamente")
    args = parser.parse_args(argv)
    suites = [name.strip() for name in args.suite.split(",") if name.strip()]
    if not suites or any(name not in ("e1", "e2") for name in suites):
        parser.error("--suite aceita e1 e/ou e2")
    if not args.run:
        print(f"nenhuma captura executada; plano: {planned_invocations(suites)} invocações locais; acrescente --run após autorização")
        return 0
    try:
        artifact = capture(args.sidecar.resolve(), args.checkout.resolve(), suites, args.out.resolve())
    except (freeze.FreezeError, OSError, json.JSONDecodeError, subprocess.CalledProcessError, RuntimeError) as exc:
        print(f"capture: {exc}", file=sys.stderr)
        return 1
    print(f"snapshot local gravado: {args.out} ({len(artifact['rows'])} medições)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
