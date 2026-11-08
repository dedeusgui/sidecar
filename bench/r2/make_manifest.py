#!/usr/bin/env python3
"""Expande bench/r2/long_prompts.json para o formato do avaliador externo (--external-cases).
Uso: python3 bench/r2/make_manifest.py --checkout DIR [saida.json]   (DIR de bench/agentic/setup_10_repos.sh)"""
import json, pathlib, sys
HERE = pathlib.Path(__file__).resolve().parent
repos_all = json.loads((HERE.parent / "ten_repos_cases.json").read_text())["repos"]
src = json.loads((HERE / "long_prompts.json").read_text())
args = sys.argv[1:]
checkout = None
if args[:1] == ["--checkout"]:
    checkout, args = args[1], args[2:]
used = {c["repo"] for c in src["positives"] + src["negatives"]}
repos = {}
for n in used:
    r = dict(repos_all[n])
    if checkout:
        r["path"] = str(pathlib.Path(checkout) / n)
    repos[n] = r
cases = [{"id": c["id"], "split": c["split"], "repo": c["repo"], "category": c["category"], "prompt": c["prompt"],
          "relevant": [{"path": p} for p in c["relevant"]]} for c in src["positives"]]
cases += [{"id": c["id"], "split": c["split"], "repo": c["repo"], "category": "negative_long", "no_gold": True,
           "prompt": c["prompt"], "relevant": []} for c in src["negatives"]]
out = pathlib.Path(args[0]) if args else HERE / "long_prompts_external.json"
out.write_text(json.dumps({"repos": repos, "cases": cases}, ensure_ascii=False, indent=2) + "\n")
print(f"wrote {out}: {len(cases)} cases")
