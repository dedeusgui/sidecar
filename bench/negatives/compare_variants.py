#!/usr/bin/env python3
"""Compara variantes do sidecar-eval em conjuntos congelados, sem chamar modelo.

Lê saídas de `sidecar-eval --out` (uma por variante) e reporta, para o método indicado:
- falsos positivos em negativos difíceis, por split (dev e validation; holdout só com --include-holdout);
- Hit@1, Hit@5 e MRR nos positivos e abstenções (casos sem evidência) dos conjuntos externos.

Não seleciona nada sozinho: a escolha é do pesquisador, sobre os splits de desenvolvimento e validação.
O holdout fica excluído por padrão; usá-lo para escolher parâmetros invalida a avaliação.

Uso:
  python3 bench/negatives/compare_variants.py --method "v2 (intent + contract + packer)" \
      base=/tmp/hn-base.json:/tmp/pos-base.json new=/tmp/hn-new.json:/tmp/pos-new.json
"""
import argparse
import collections
import json
import pathlib
import sys


def load(path, method):
    outs = [o for o in json.loads(pathlib.Path(path).read_text())["outcomes"] if o["method"] == method]
    return outs


def summarize(hard_negatives, positives, include_holdout):
    r = collections.Counter()
    for o in hard_negatives:
        split = o["split"]
        if split == "holdout" and not include_holdout:
            continue
        if o["negative"]:
            r[f"fp_{split}_n"] += 1
            r[f"fp_{split}_fp"] += 0 if o["empty"] else 1
    for o in positives:
        if o["negative"]:
            r["abst_n"] += 1
            r["abst_fp"] += 0 if o["empty"] else 1
            continue
        r["pos_n"] += 1
        r["hit1"] += int(bool(o["hit_at_1"]))
        r["hit5"] += int(bool(o["hit_at_5"]))
        r["rr"] += o["reciprocal_rank"]
        r["empty"] += int(bool(o["empty"]))
    return r


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--method", default="v2 (intent + contract + packer)")
    ap.add_argument("--include-holdout", action="store_true")
    ap.add_argument("variants", nargs="+", help="tag=hard_negatives.json:positives.json")
    args = ap.parse_args()
    print(f"{'variante':10s} {'FP dev+val':>14s} {'pos':>4s} {'Hit@1':>6s} {'Hit@5':>6s} {'MRR':>6s} {'sem evid.':>9s}")
    for spec in args.variants:
        tag, files = spec.split("=", 1)
        hn_path, pos_path = files.split(":", 1)
        s = summarize(load(hn_path, args.method), load(pos_path, args.method), args.include_holdout)
        n = s["fp_dev_n"] + s["fp_validation_n"]
        fp = s["fp_dev_fp"] + s["fp_validation_fp"]
        pn = s["pos_n"] or 1
        print(f"{tag:10s} {fp:4d}/{n:<4d} ({100*fp/max(n,1):4.1f}%) {s['pos_n']:4d} {s['hit1']:6d} {s['hit5']:6d} "
              f"{s['rr']/pn:6.3f} {s['empty']:9d}")
        if args.include_holdout:
            print(f"           holdout FP {s['fp_holdout_fp']}/{s['fp_holdout_n']} (já observado: não usar para escolha)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
