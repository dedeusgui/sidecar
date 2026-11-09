#!/usr/bin/env python3
"""Expand the frozen E1 cohort into cmd/sidecar-eval --external-cases format."""

from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys
from typing import Any

import freeze


HERE = pathlib.Path(__file__).resolve().parent


def external_manifest(checkout: pathlib.Path) -> dict[str, Any]:
    data = freeze.read_json(HERE / "e1_cases.json")
    repos = {
        slug: {
            **metadata,
            "path": str((checkout / slug).resolve()),
        }
        for slug, metadata in sorted(data["repos"].items())
    }
    cases = []
    for item in data["positives"]:
        cases.append({
            "id": item["id"],
            "split": "v35-validation",
            "repo": item["repo"],
            "category": item["category"],
            "prompt": item["prompt"],
            "relevant": [{"path": gold["path"]} for gold in item["relevant"]],
        })
    for item in data["negatives"]:
        cases.append({
            "id": item["id"],
            "split": "v35-validation",
            "repo": item["repo"],
            "category": item["category"],
            "no_gold": True,
            "prompt": item["prompt"],
            "relevant": [],
        })
    return {"repos": repos, "cases": cases}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--checkout", required=True, type=pathlib.Path, help="diretório com os cinco repositórios pinados")
    parser.add_argument("--out", required=True, type=pathlib.Path, help="manifesto --external-cases de saída")
    args = parser.parse_args(argv)
    try:
        ok, message = freeze.verify_manifest()
        if not ok:
            raise freeze.FreezeError(message)
        freeze.verify_sources(args.checkout)
        manifest = external_manifest(args.checkout)
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    except (freeze.FreezeError, OSError, json.JSONDecodeError, subprocess.CalledProcessError) as exc:
        print(f"make_e1_manifest: {exc}", file=sys.stderr)
        return 1
    print(f"E1 externo congelado gravado: {args.out} ({len(manifest['cases'])} casos)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
