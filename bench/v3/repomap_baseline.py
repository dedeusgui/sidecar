#!/usr/bin/env python3
"""Aider RepoMap como baseline offline de recuperação (E1), sem modelo e sem rede.

Executa o RepoMap do Aider (Apache-2.0; instalado à parte, ex. `uv pip install aider-chat==0.86.2` num venv) sobre
os mesmos manifestos externos do `sidecar-eval --external-cases`. Para cada caso, as palavras do prompt viram
`mentioned_idents` (como o Aider faz com a mensagem do usuário), nenhum arquivo está no chat e todo o repositório é
candidato. A ordem dos arquivos nas tags ranqueadas é a ordem de recuperação; o mapa renderizado com `--map-tokens`
dá o custo em tokens. O RepoMap não se abstém: qualquer mapa entregue a um negativo conta como falso positivo.

Saída no formato `outcomes` do sidecar-eval (método "aider repomap"), para bench/v3/e1_report.py.

Uso: AIDER_PY=venv/bin/python; $AIDER_PY bench/v3/repomap_baseline.py MANIFEST.json OUT.json [--map-tokens 1024]
Nenhum código do Aider é copiado para o Sidecar; ele é chamado como ferramenta externa de avaliação.
"""
import argparse
import json
import pathlib
import re
import subprocess
import sys
import time

METHOD = "aider repomap"


class _IO:
    """Silent stand-in for aider.io.InputOutput: RepoMap only reads files and reports warnings."""

    def read_text(self, fname, silent=False):
        try:
            return pathlib.Path(fname).read_text(errors="replace")
        except OSError:
            return None

    def tool_output(self, *a, **k):
        pass

    def tool_warning(self, *a, **k):
        pass

    def tool_error(self, *a, **k):
        pass


class _Model:
    def token_count(self, text):
        return len(text) // 4


def tracked_files(root):
    out = subprocess.run(["git", "-C", str(root), "ls-files"], capture_output=True, text=True, check=True).stdout
    return [str(pathlib.Path(root) / p) for p in out.splitlines() if p]


def main(argv=None):
    from aider.repomap import RepoMap  # imported here so the module loads without aider for --help

    ap = argparse.ArgumentParser()
    ap.add_argument("manifest")
    ap.add_argument("out")
    ap.add_argument("--map-tokens", type=int, default=1024)
    a = ap.parse_args(argv)
    man = json.loads(pathlib.Path(a.manifest).read_text())
    maps, files = {}, {}
    outcomes = []
    for c in man["cases"]:
        repo = man["repos"][c["repo"]]
        root = pathlib.Path(repo["path"]).resolve()
        if c["repo"] not in maps:
            maps[c["repo"]] = RepoMap(map_tokens=a.map_tokens, root=str(root), main_model=_Model(), io=_IO(), verbose=False)
            files[c["repo"]] = tracked_files(root)
        rm = maps[c["repo"]]
        idents = set(re.findall(r"[A-Za-z_][A-Za-z0-9_]{2,}", c["prompt"]))
        t0 = time.perf_counter()
        tags = rm.get_ranked_tags([], files[c["repo"]], set(), idents) or []
        text = rm.get_ranked_tags_map([], files[c["repo"]], max_map_tokens=a.map_tokens, mentioned_idents=idents) or ""
        latency = (time.perf_counter() - t0) * 1000
        order = []
        for tag in tags:
            rel = tag[0] if isinstance(tag, tuple) else getattr(tag, "rel_fname", None)
            if rel and rel not in order:
                order.append(rel)
        gold = [g["path"] for g in c.get("relevant") or []]
        negative = bool(c.get("no_gold")) or not gold
        top = order[:10]
        ranks = [order.index(g) + 1 for g in gold if g in order]
        rr = 1 / min(ranks) if ranks else 0.0
        found = sum(1 for g in gold if g in top)
        outcomes.append({"method": METHOD, "case": c["id"], "split": c.get("split", ""), "category": c.get("category"),
                         "negative": negative, "top": ["file:" + p for p in top], "gold_found_top10": found,
                         "gold_total": len(gold), "hit_at_1": bool(ranks and min(ranks) == 1),
                         "hit_at_5": bool(ranks and min(ranks) <= 5), "recall_at_10": found / len(gold) if gold else 0.0,
                         "reciprocal_rank": rr, "precision": (found / len(top)) if top else 0.0, "empty": not text.strip(),
                         "latency_ms": round(latency, 2), "context_tokens": len(text) // 4})
        print(f"{c['id']:28s} rr={rr:.2f} tokens={len(text) // 4}", file=sys.stderr)
    pathlib.Path(a.out).write_text(json.dumps({"methods": [METHOD], "outcomes": outcomes}, indent=1) + "\n")


if __name__ == "__main__":
    main()
