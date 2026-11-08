#!/usr/bin/env python3
"""Análise dos runs do lab para decidir o que melhorar (insumos do V3).

Junta vários runs.jsonl (repetições em processos diferentes) e responde, com dados e sem inventar causa:
- quais tarefas NENHUM braço resolve (corpus difícil ou defeito de oráculo/escopo?);
- quais tarefas dividem os braços (onde o contexto pode estar ajudando ou atrapalhando);
- como as falhas se distribuem (teste oculto, escopo, invariante, timeout, erro de agente, não comparável);
- onde está o gasto (tokens, custo, chamadas) por classe de tarefa e por braço;
- onde há repetição de trabalho (chamadas redundantes) e contexto injetado.
Só lê arquivos; não executa agentes. Stdlib.

Uso: python3 bench/lab/analyze.py RUNS.jsonl [RUNS.jsonl ...] [--md saida.md] [--json saida.json]
"""
import argparse
import collections
import json
import pathlib
import statistics
import sys


def load(paths):
    rows = []
    for p in paths:
        for line in pathlib.Path(p).read_text().splitlines():
            if line.strip():
                rows.append(json.loads(line))
    return rows


def failure_kind(r):
    """Uma única causa por execução, na ordem em que invalida ou explica o resultado."""
    if not r["comparable"]:
        return "nao_comparavel: " + ("; ".join(r.get("incomparable_reasons") or []) or r.get("agent_error") or "?")[:70]
    if r.get("timed_out"):
        return "timeout"
    if r["success"]:
        return "ok"
    o = r["oracle"]
    if o["tests"] == "error":
        return "erro_de_infra_do_oraculo"
    if o["tests"] != "pass":
        return "teste_oculto_falhou"
    if any(not i["ok"] for i in o["invariants"]):
        return "invariante_quebrado"
    if not o["scope_ok"]:
        outside = o.get("outside_scope") or []
        only_tests = outside and all(("test" in p.lower() or "spec" in p.lower()) for p in outside)
        return "escopo_so_testes_extras" if only_tests else "escopo_fora_do_codigo_permitido"
    return "outro"


def mean(xs):
    xs = [x for x in xs if x is not None]
    return round(statistics.mean(xs), 3) if xs else None


def analyze(rows):
    arms = sorted({r["arm"] for r in rows})
    by_task = collections.defaultdict(list)
    for r in rows:
        by_task[r["task"]].append(r)
    meta = {t: (v[0]["class"], v[0]["split"], v[0]["repo"]) for t, v in by_task.items()}
    out = {"runs": len(rows), "arms": arms, "tasks": len(by_task), "reps": len({r["rep"] for r in rows}), "failure_taxonomy": {},
           "unsolved_by_all": [], "solved_by_all": [], "split_arms": [], "by_class": {}, "hot_spots": {}, "per_task": {}}

    tax = collections.defaultdict(collections.Counter)
    for r in rows:
        tax[r["arm"]][failure_kind(r)] += 1
    out["failure_taxonomy"] = {a: dict(c.most_common()) for a, c in tax.items()}

    for t, rs in sorted(by_task.items()):
        comp = [r for r in rs if r["comparable"]]
        rate = {a: (sum(1 for r in comp if r["arm"] == a and r["success"]), sum(1 for r in comp if r["arm"] == a)) for a in arms}
        total_ok, total_n = sum(v[0] for v in rate.values()), sum(v[1] for v in rate.values())
        entry = {"class": meta[t][0], "split": meta[t][1], "repo": meta[t][2], "pass": {a: f"{ok}/{n}" for a, (ok, n) in rate.items()},
                 "failure_kinds": dict(collections.Counter(failure_kind(r) for r in rs if not r["success"]).most_common(3)),
                 "tokens_mean": mean([(r.get("tokens_in") or 0) + (r.get("tokens_out") or 0) for r in comp if r.get("tokens_in") is not None]),
                 "tool_calls_mean": mean([r.get("tool_calls") for r in comp])}
        out["per_task"][t] = entry
        if total_n and total_ok == 0:
            out["unsolved_by_all"].append(t)
        elif total_n and total_ok == total_n:
            out["solved_by_all"].append(t)
        else:
            fr = {a: ok / n for a, (ok, n) in rate.items() if n}
            if fr and max(fr.values()) - min(fr.values()) > 0:
                out["split_arms"].append({"task": t, "class": meta[t][0], "rates": {a: round(v, 2) for a, v in fr.items()}})

    classes = sorted({m[0] for m in meta.values()})
    for c in classes:
        out["by_class"][c] = {}
        for a in arms:
            sel = [r for r in rows if meta[r["task"]][0] == c and r["arm"] == a and r["comparable"]]
            if sel:
                out["by_class"][c][a] = {"n": len(sel), "pass_rate": round(sum(1 for r in sel if r["success"]) / len(sel), 2),
                                         "tool_calls": mean([r.get("tool_calls") for r in sel]), "cost_usd": mean([r.get("cost_usd") for r in sel]),
                                         "tokens": mean([(r["tokens_in"] + r["tokens_out"]) for r in sel if isinstance(r.get("tokens_in"), int)])}

    comp = [r for r in rows if r["comparable"]]
    costly = sorted(by_task, key=lambda t: -(mean([r.get("cost_usd") for r in by_task[t] if r["comparable"]]) or 0))[:5]
    redundant = collections.defaultdict(list)
    for r in comp:
        if r.get("progress"):
            redundant[r["arm"]].append(r["progress"]["redundant"])
    out["hot_spots"] = {
        "tarefas_mais_caras": [(t, mean([r.get("cost_usd") for r in by_task[t] if r["comparable"]])) for t in costly],
        "redundantes_por_braco": {a: mean(v) for a, v in redundant.items()},
        "execucoes_com_estagnacao": {a: sum(1 for r in comp if r["arm"] == a and r.get("progress") and r["progress"]["stagnant_windows"] > 0) for a in arms},
        "contexto_injetado_medio_chars": {a: mean([r.get("injected_chars") for r in comp if r["arm"] == a]) for a in arms},
        "contexto_entregue_pct": {a: round(100 * sum(1 for r in comp if r["arm"] == a and r.get("hook_delivered")) / max(1, sum(1 for r in comp if r["arm"] == a)), 0) for a in arms if a != "OFF"},
    }
    return out


def render(a):
    L = [f"# Análise dos runs do lab ({a['runs']} execuções, {a['tasks']} tarefas, {a['reps']} repetição(ões), braços: {', '.join(a['arms'])})", "",
         "## Distribuição das falhas por braço", "", "| causa | " + " | ".join(a["arms"]) + " |", "|---|" + "---:|" * len(a["arms"])]
    kinds = sorted({k for v in a["failure_taxonomy"].values() for k in v})
    for k in kinds:
        L.append(f"| {k} | " + " | ".join(str(a["failure_taxonomy"].get(arm, {}).get(k, 0)) for arm in a["arms"]) + " |")
    L += ["", "## Tarefas que nenhum braço resolve", "", ", ".join(a["unsolved_by_all"]) or "nenhuma",
          "", "Se há tarefas aqui, verifique primeiro oráculo e escopo (defeito do corpus) antes de culpar o contexto.", "",
          "## Tarefas que dividem os braços (onde o contexto muda o resultado)", ""]
    if a["split_arms"]:
        L += ["| tarefa | classe | taxa por braço |", "|---|---|---|"] + [f"| {s['task']} | {s['class']} | {s['rates']} |" for s in a["split_arms"]]
    else:
        L.append("nenhuma")
    L += ["", "## Por classe de tarefa", ""]
    for c, v in a["by_class"].items():
        L += [f"### {c}", "", "| braço | n | aprovação | tool calls | custo US$ | tokens |", "|---|---:|---:|---:|---:|---:|"]
        L += [f"| {arm} | {m['n']} | {m['pass_rate']} | {m['tool_calls']} | {m['cost_usd']} | {m['tokens']} |" for arm, m in v.items()]
        L.append("")
    h = a["hot_spots"]
    L += ["## Onde está o gasto e a repetição", "", f"- tarefas mais caras (custo médio US$): {h['tarefas_mais_caras']}", f"- chamadas redundantes médias por braço: {h['redundantes_por_braco']}",
          f"- execuções com estagnação: {h['execucoes_com_estagnacao']}", f"- contexto injetado médio (chars): {h['contexto_injetado_medio_chars']}",
          f"- % das execuções em que o contexto foi entregue: {h['contexto_entregue_pct']}", ""]
    return "\n".join(L) + "\n"


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("runs", nargs="+")
    ap.add_argument("--md")
    ap.add_argument("--json")
    a = ap.parse_args(argv)
    rows = load(a.runs)
    if not rows:
        print("sem execuções", file=sys.stderr)
        return 1
    res = analyze(rows)
    if a.json:
        pathlib.Path(a.json).write_text(json.dumps(res, indent=2, ensure_ascii=False) + "\n")
    text = render(res)
    if a.md:
        pathlib.Path(a.md).write_text(text)
    else:
        print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
