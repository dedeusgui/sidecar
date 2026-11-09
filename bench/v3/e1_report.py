#!/usr/bin/env python3
"""E1 offline: compara métodos do `sidecar-eval --out` por suíte, sem modelo nem rede.

Para cada suíte (arquivo de saída) e método: falsos positivos em negativos por split, Hit@1/Hit@5/MRR/Recall@10,
positivos sem evidência (falsos negativos de abstenção), tokens e latência p50/p95; e a lista de regressões/melhorias
caso a caso entre dois métodos (`--base` e `--new`). O holdout fica fora por padrão (--include-holdout para relatório final).

Uso: python3 bench/v3/e1_report.py --base "v2 (intent + contract + packer)" --new "v3 (relevance gate + effort router)" \
         ten=OUT1.json hn=OUT2.json long=OUT3.json [--md report.md] [--json report.json]
"""
import argparse
import collections
import json
import pathlib
import statistics


def pct(xs, q):
    xs = sorted(xs)
    if not xs:
        return None
    k = (len(xs) - 1) * q
    lo, hi = int(k), min(int(k) + 1, len(xs) - 1)
    return round(xs[lo] + (xs[hi] - xs[lo]) * (k - lo), 2)


def summarize(outs, include_holdout):
    rows = [o for o in outs if include_holdout or o["split"] != "holdout"]
    neg = [o for o in rows if o["negative"]]
    pos = [o for o in rows if not o["negative"]]
    fp_by_split = collections.defaultdict(lambda: [0, 0])
    for o in neg:
        fp_by_split[o["split"]][0] += 0 if o["empty"] else 1
        fp_by_split[o["split"]][1] += 1
    n = len(pos)
    return {
        "negatives": len(neg), "false_positives": sum(0 if o["empty"] else 1 for o in neg),
        "fp_by_split": {k: {"fp": v[0], "n": v[1]} for k, v in sorted(fp_by_split.items())},
        "positives": n,
        "hit1": round(sum(o["hit_at_1"] for o in pos) / n, 4) if n else None,
        "hit5": round(sum(o["hit_at_5"] for o in pos) / n, 4) if n else None,
        "recall10": round(statistics.mean(o["recall_at_10"] for o in pos), 4) if n else None,
        "mrr": round(statistics.mean(o["reciprocal_rank"] for o in pos), 4) if n else None,
        "false_negatives_empty": sum(o["empty"] for o in pos),
        "tokens_mean_pos": round(statistics.mean(o["context_tokens"] for o in pos), 1) if n else None,
        "tokens_mean_neg": round(statistics.mean(o["context_tokens"] for o in neg), 1) if neg else None,
        "latency_p50": pct([o["latency_ms"] for o in rows], .5), "latency_p95": pct([o["latency_ms"] for o in rows], .95),
    }


def per_case(base, new, include_holdout):
    b = {o["case"]: o for o in base if include_holdout or o["split"] != "holdout"}
    changes = []
    for o in new:
        if o["case"] not in b:
            continue
        x = b[o["case"]]
        if o["negative"]:
            if x["empty"] != o["empty"]:
                changes.append({"case": o["case"], "split": o["split"], "kind": "negative",
                                "change": "improved" if o["empty"] else "regressed",
                                "detail": f"FP {not x['empty']} -> {not o['empty']}"})
        else:
            if (x["hit_at_5"], x["empty"], round(x["reciprocal_rank"], 3)) != (o["hit_at_5"], o["empty"], round(o["reciprocal_rank"], 3)):
                worse = (o["hit_at_5"] < x["hit_at_5"]) or (o["empty"] and not x["empty"]) or (
                    o["hit_at_5"] == x["hit_at_5"] and o["reciprocal_rank"] < x["reciprocal_rank"])
                changes.append({"case": o["case"], "split": o["split"], "kind": "positive",
                                "change": "regressed" if worse else "improved",
                                "detail": f"hit5 {x['hit_at_5']}->{o['hit_at_5']} rr {x['reciprocal_rank']:.3f}->{o['reciprocal_rank']:.3f} empty {x['empty']}->{o['empty']}"})
    return changes


def main(argv=None):
    p = argparse.ArgumentParser()
    p.add_argument("suites", nargs="+", help="nome=arquivo.json")
    p.add_argument("--base", required=True)
    p.add_argument("--new", required=True)
    p.add_argument("--include-holdout", action="store_true")
    p.add_argument("--md")
    p.add_argument("--json")
    a = p.parse_args(argv)
    report = {"base": a.base, "new": a.new, "holdout_included": a.include_holdout, "suites": {}}
    for spec in a.suites:
        name, path = spec.split("=", 1)
        outs = json.loads(pathlib.Path(path).read_text())["outcomes"]
        by = collections.defaultdict(list)
        for o in outs:
            by[o["method"]].append(o)
        if a.base not in by or a.new not in by:
            raise SystemExit(f"{path}: métodos ausentes ({a.base!r} / {a.new!r})")
        report["suites"][name] = {"base": summarize(by[a.base], a.include_holdout),
                                  "new": summarize(by[a.new], a.include_holdout),
                                  "changes": per_case(by[a.base], by[a.new], a.include_holdout)}
    lines = [f"# E1 — {a.new} vs {a.base}", "", "Holdout incluído: " + ("sim" if a.include_holdout else "não"), ""]
    lines += ["| Suíte | Método | FP/neg | Hit@1 | Hit@5 | MRR | Positivos vazios | Tokens pos | Tokens neg | p50 ms | p95 ms |",
              "|---|---|---|---|---|---|---|---|---|---|---|"]
    for name, s in report["suites"].items():
        for label in ("base", "new"):
            m = s[label]
            f = lambda v: "—" if v is None else (f"{v * 100:.1f}%" if isinstance(v, float) and v <= 1 else str(v))
            lines.append(f"| {name} | {a.base if label == 'base' else a.new} | {m['false_positives']}/{m['negatives']} | "
                         f"{f(m['hit1'])} | {f(m['hit5'])} | {m['mrr'] if m['mrr'] is not None else '—'} | "
                         f"{m['false_negatives_empty']}/{m['positives']} | {m['tokens_mean_pos']} | {m['tokens_mean_neg']} | "
                         f"{m['latency_p50']} | {m['latency_p95']} |")
    for name, s in report["suites"].items():
        reg = [c for c in s["changes"] if c["change"] == "regressed"]
        imp = [c for c in s["changes"] if c["change"] == "improved"]
        lines += ["", f"## {name}: {len(imp)} melhorias, {len(reg)} regressões"]
        for c in reg + imp:
            lines.append(f"- {c['change']} `{c['case']}` ({c['split']}, {c['kind']}): {c['detail']}")
    text = "\n".join(lines) + "\n"
    print(text)
    if a.md:
        pathlib.Path(a.md).write_text(text)
    if a.json:
        pathlib.Path(a.json).write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")


if __name__ == "__main__":
    main()
