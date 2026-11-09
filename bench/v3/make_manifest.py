#!/usr/bin/env python3
"""Expande bench/v3/e1_independent.json para o avaliador externo (--external-cases).
Uso: python3 bench/v3/make_manifest.py --checkout DIR SAIDA.json   (DIR de bench/agentic/setup_10_repos.sh)"""
import hashlib
import json
import pathlib
import sys

HERE = pathlib.Path(__file__).resolve().parent


def main(argv):
    if argv[:1] != ["--checkout"] or len(argv) != 3:
        raise SystemExit(__doc__)
    checkout, out = pathlib.Path(argv[1]), pathlib.Path(argv[2])
    raw = (HERE / "e1_independent.json").read_bytes()
    src = json.loads(raw)
    repos_all = json.loads((HERE.parent / "ten_repos_cases.json").read_text())["repos"]
    used = {c["repo"] for c in src["positives"] + src["negatives"]}
    repos = {n: dict(repos_all[n], path=str(checkout / n)) for n in sorted(used)}
    cases = [{"id": c["id"], "split": "independent", "repo": c["repo"], "category": c["category"], "prompt": c["prompt"],
              "relevant": [{"path": p} for p in c["relevant"]]} for c in src["positives"]]
    cases += [{"id": c["id"], "split": "independent", "repo": c["repo"], "category": c["category"], "no_gold": True,
               "prompt": c["prompt"], "relevant": []} for c in src["negatives"]]
    for c in cases:
        for g in c["relevant"]:
            if not (checkout / c["repo"] / g["path"]).is_file():
                raise SystemExit(f"{c['id']}: gold ausente no checkout: {g['path']}")
    out.write_text(json.dumps({"repos": repos, "cases": cases}, ensure_ascii=False, indent=2) + "\n")
    print(f"wrote {out}: {len(cases)} cases; e1_independent.json sha256 {hashlib.sha256(raw).hexdigest()}")


if __name__ == "__main__":
    main(sys.argv[1:])
