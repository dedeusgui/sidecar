#!/usr/bin/env python3
"""Valida bench/registry.json e gera docs/benchmarks.md (índice dos benchmarks e pistas de melhoria).

Regras que impedem relatório enganoso: todo benchmark declara nível de evidência, dados e ressalva; só `agentic` pode ter
status de referência para alegação de ganho; artefatos citados precisam existir; toda pista cita benchmarks que existem.
Só lê e escreve texto. Stdlib. Uso: python3 bench/consolidate.py [--check] [--out docs/benchmarks.md]
"""
import argparse
import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
REQUIRED = ("id", "level", "question", "data", "command", "artifacts", "headline", "status", "caveat")


def validate(reg, root=ROOT):
    errs, ids = [], set()
    levels = reg.get("evidence_levels", {})
    for b in reg["benchmarks"]:
        for k in REQUIRED:
            if k not in b or (k != "artifacts" and not str(b[k]).strip()):
                errs.append(f"{b.get('id', '?')}: campo ausente ou vazio: {k}")
        if b.get("level") not in levels:
            errs.append(f"{b.get('id')}: nível de evidência desconhecido {b.get('level')!r}")
        if b.get("id") in ids:
            errs.append(f"id repetido: {b.get('id')}")
        ids.add(b.get("id"))
        for a in b.get("artifacts", []):
            if not (root / a).exists():
                errs.append(f"{b['id']}: artefato inexistente: {a}")
        if b.get("level") != "agentic" and "ganho" in b.get("headline", "").lower() and "sem" not in b.get("headline", "").lower():
            errs.append(f"{b['id']}: só benchmark agentic pode alegar ganho")
    prefixes = {i.split("-")[0] for i in ids}
    for lead in reg.get("improvement_leads", []):
        for s in lead["source"]:
            if s not in prefixes:
                errs.append(f"{lead['id']}: cita benchmark inexistente {s}")
        for k in ("problem", "hypothesis_v3", "confirm_with", "priority"):
            if not lead.get(k):
                errs.append(f"{lead['id']}: campo ausente: {k}")
    return errs


def render(reg):
    L = ["# Benchmarks do Sidecar: índice, nível de evidência e pistas de melhoria", "",
         f"Gerado de `bench/registry.json` ({reg['updated']}) por `python3 bench/consolidate.py`. **Não edite à mão**: edite o registro e regenere.", "",
         "## Níveis de evidência", ""] + [f"- **{k}**: {v}" for k, v in reg["evidence_levels"].items()]
    L += ["", "## Benchmarks", "", "| id | nível | pergunta | resultado | status | ressalva |", "|---|---|---|---|---|---|"]
    for b in reg["benchmarks"]:
        L.append(f"| {b['id']} | {b['level']} | {b['question']} | {b['headline']} | {b['status']} | {b['caveat']} |")
    L += ["", "### Dados e comandos", ""]
    for b in reg["benchmarks"]:
        arts = ", ".join(f"`{a}`" for a in b["artifacts"]) or "—"
        L += [f"**{b['id']}** — dados: {b['data']}  ", f"comando: `{b['command']}`  ", f"artefatos: {arts}", ""]
    L += ["## Pistas de melhoria (insumos do V3)", "", "Cada pista aponta o benchmark que a sustenta e o que mediria a confirmação. São hipóteses, não decisões.", "",
          "| id | origem | problema observado | hipótese para o V3 | como confirmar | prioridade |", "|---|---|---|---|---|---|"]
    for l in reg["improvement_leads"]:
        L.append(f"| {l['id']} | {', '.join(l['source'])} | {l['problem']} | {l['hypothesis_v3']} | {l['confirm_with']} | {l['priority']} |")
    return "\n".join(L) + "\n"


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--check", action="store_true", help="só valida (sai 1 se houver erro)")
    ap.add_argument("--out", default=str(ROOT / "docs" / "benchmarks.md"))
    ap.add_argument("--registry", default=str(ROOT / "bench" / "registry.json"))
    a = ap.parse_args(argv)
    reg = json.loads(pathlib.Path(a.registry).read_text())
    errs = validate(reg)
    for e in errs:
        print("erro:", e, file=sys.stderr)
    if errs:
        return 1
    if not a.check:
        pathlib.Path(a.out).write_text(render(reg))
        print(f"wrote {a.out}: {len(reg['benchmarks'])} benchmarks, {len(reg['improvement_leads'])} pistas")
    return 0


if __name__ == "__main__":
    sys.exit(main())
