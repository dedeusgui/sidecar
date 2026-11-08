#!/usr/bin/env python3
"""Expande bench/negatives/hard_negatives.json para o formato do avaliador externo.

Cada prompt negativo é executado contra cada repositório fixado de bench/ten_repos_cases.json,
porque o vocabulário genérico precisa colidir com código real. O split vem do arquivo de origem.
Uso: python3 bench/negatives/make_manifest.py [--checkout DIR] [saida.json]
DIR é a pasta criada por bench/agentic/setup_10_repos.sh (ela recebe o caminho de cada repositório).
"""
import hashlib
import json
import pathlib
import sys

HERE = pathlib.Path(__file__).resolve().parent
SRC = HERE / "hard_negatives.json"
REPOS = HERE.parent / "ten_repos_cases.json"


def build(checkout=None):
    src = json.loads(SRC.read_text())
    repos = json.loads(REPOS.read_text())["repos"]
    if checkout:
        # Same layout as bench/agentic/setup_10_repos.sh: one folder per pinned repository.
        for name, repo in repos.items():
            repo["path"] = str(pathlib.Path(checkout) / name)
    cases = []
    for split, prompts in src["splits"].items():
        for i, prompt in enumerate(prompts, 1):
            for repo in repos:
                cases.append({"id": f"HN-{split}-{i:02d}@{repo}", "split": split, "repo": repo,
                              "category": "negative", "no_gold": True, "prompt": prompt, "relevant": []})
    return {"repos": repos, "cases": cases}


def holdout_sha256():
    prompts = json.loads(SRC.read_text())["splits"]["holdout"]
    return hashlib.sha256("\n".join(prompts).encode()).hexdigest()


if __name__ == "__main__":
    args = sys.argv[1:]
    checkout = None
    if args[:1] == ["--checkout"]:
        checkout, args = args[1], args[2:]
    out = pathlib.Path(args[0]) if args else HERE / "hard_negatives_external.json"
    manifest = build(checkout)
    out.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(f"wrote {out}: {len(manifest['cases'])} cases; holdout prompts sha256 {holdout_sha256()}")
