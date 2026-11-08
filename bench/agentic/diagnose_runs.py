#!/usr/bin/env python3
"""Diagnóstico automático de falhas de runs.jsonl (piloto ou run_agents).

Classifica cada execução com causas candidatas. São hipóteses para revisão, não veredictos: uma causa
só é afirmada quando o registro a sustenta, e a falta de evidência é marcada como `unknown`.

Causas candidatas:
  infra_failure      processo com erro, timeout, erro de stream ou checkout alterado
  malformed_answer   resposta sem JSON {"files": [...]} válido; o oráculo não foi alcançado
  injection_failure  braço ON sem contexto entregue em caso não negativo (pode ser abstenção legítima)
  missing_evidence   gold ausente do contexto entregue, em caso não negativo
  ranking_or_use     gold estava no contexto, mas a resposta não o usou
  false_positive     resposta com arquivos em caso negativo, ou resposta seguiu ruído do contexto
  false_positive_injection  contexto entregue em caso sem evidência relevante (abstenção falhou), mesmo com resposta correta
  wrong_answer       resposta válida, sem gold, sem ruído identificável

Uso: python3 bench/agentic/diagnose_runs.py runs.jsonl [--md saida.md]
"""
import argparse
import json
import pathlib


def causes(rec):
    out = []
    # Contexto entregue para um caso sem evidência relevante: falha de abstenção mesmo se a resposta acertar.
    if rec.get("category") == "negative" and rec.get("arm") == "ON" and rec.get("hook_delivered"):
        out.append("false_positive_injection")
    if rec.get("success") is True:
        return out
    if rec.get("exit") not in (0, None) or rec.get("timed_out") or rec.get("stream_error") or rec.get("edited_files"):
        return out + ["infra_failure"]
    if rec.get("malformed"):
        return out + ["malformed_answer"]
    if rec.get("category") == "negative":
        return out + (["false_positive"] if rec.get("files") else ["unknown"])
    arm = rec.get("arm")
    if arm == "ON" and rec.get("hook_delivered") is False:
        out.append("injection_failure")
    if arm == "ON" and rec.get("gold_in_ctx") is False:
        out.append("missing_evidence")
    if arm == "ON" and rec.get("gold_ignored"):
        out.append("ranking_or_use")
    if arm == "ON" and rec.get("followed_noise"):
        out.append("false_positive")
    if not out:
        out.append("wrong_answer" if rec.get("files") else "unknown")
    return out


def diagnose(rows):
    return [{"run": f"{r.get('arm')}/{r.get('case')}/rep{r.get('rep')}", "agent": r.get("agent"),
             "model": r.get("model"), "category": r.get("category"), "success": r.get("success"),
             "causes": causes(r), "files": r.get("files"), "gold_in_ctx": r.get("gold_in_ctx"),
             "injected_chars": r.get("injected_chars"), "source_line": i + 1}
            for i, r in enumerate(rows)]


def render(items, source):
    lines = ["# Diagnóstico de falhas (automático, hipóteses)", "",
             f"Fonte: `{source}` · execuções: {len(items)} · falhas: {sum(1 for d in items if d['success'] is not True)}", "",
             "Causas são candidatas para revisão, não verdades. Reproduza uma execução com o caso e o braço indicados.", "",
             "| Execução | Modelo | Categoria | Sucesso | Causas candidatas | Respostas | Gold no contexto | Linha |",
             "|---|---|---|---|---|---|---|---|"]
    for d in items:
        lines.append(f"| {d['run']} | {d['model']} | {d['category']} | {d['success']} | "
                     f"{', '.join(d['causes']) or '—'} | {', '.join(d['files'] or []) or '—'} | "
                     f"{d['gold_in_ctx']} | {d['source_line']} |")
    return "\n".join(lines) + "\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("runs")
    ap.add_argument("--md")
    args = ap.parse_args()
    rows = [json.loads(line) for line in pathlib.Path(args.runs).read_text().splitlines() if line.strip()]
    items = diagnose(rows)
    text = render(items, args.runs)
    if args.md:
        pathlib.Path(args.md).write_text(text)
        print(f"wrote {args.md}")
    else:
        print(text)


if __name__ == "__main__":
    main()
