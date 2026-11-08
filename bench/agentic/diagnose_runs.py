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
import os
import pathlib
import shutil
import subprocess
import tempfile


CLASSES = ("experiment_violation", "infra_failure", "oracle_failure", "retrieval_failure", "task_failure",
           "false_positive_injection", "false_positive", "unknown")


def causes(rec):
    """Classes candidatas. Cada classe é uma hipótese com a evidência do próprio registro.

    experiment_violation  edição não autorizada no checkout ou execução contaminada (não é falha do agente)
    infra_failure         processo com erro, timeout ou erro de stream
    oracle_failure        o oráculo não conseguiu ler a resposta (malformada), logo não há julgamento
    retrieval_failure     gold ausente do contexto entregue, ou contexto não entregue em caso com gold
    task_failure          resposta válida e errada apesar de o gold estar disponível (contexto ou OFF)
    false_positive_injection  contexto entregue em caso sem evidência relevante, mesmo com resposta correta
    false_positive        resposta com arquivos em caso negativo, ou resposta seguiu ruído do contexto
    """
    out = []
    if rec.get("category") == "negative" and rec.get("arm") == "ON" and rec.get("hook_delivered"):
        out.append("false_positive_injection")
    if rec.get("success") is True:
        return out
    if rec.get("edited_files") or rec.get("comparable") is False:
        return out + ["experiment_violation"]
    if rec.get("exit") not in (0, None) or rec.get("timed_out") or rec.get("stream_error"):
        return out + ["infra_failure"]
    if rec.get("malformed"):
        return out + ["oracle_failure"]
    if rec.get("category") == "negative":
        return out + (["false_positive"] if rec.get("files") else ["unknown"])
    arm = rec.get("arm")
    if arm == "ON" and (rec.get("hook_delivered") is False or rec.get("gold_in_ctx") is False):
        return out + ["retrieval_failure"]
    if arm == "ON" and rec.get("followed_noise"):
        out.append("false_positive")
    if rec.get("files"):
        out.append("task_failure")
    else:
        out.append("unknown")
    return out


def summarize_explain(doc, gold_paths):
    """Resumo do `sidecar explain --json` de uma execução, para diagnosticar a recuperação.

    Diz onde o gold apareceu entre os candidatos, o que ficou acima dele e as razões registradas. Não afirma
    causalidade: são os registros do próprio explain.
    """
    cands = doc.get("candidates") or []
    evidence = doc.get("evidence") or []
    pos = {}
    for rank, c in enumerate(cands, 1):
        path = c.get("Path") or ""
        if path in gold_paths and path not in pos:
            pos[path] = rank
    above = []
    for c in cands[:5]:
        if (c.get("Path") or "") not in gold_paths:
            above.append({"id": c.get("id"), "score": c.get("Score"), "coverage": c.get("Coverage"),
                          "exact": c.get("Exact"), "reason": c.get("reason_selected") or c.get("Reason")})
    intent = doc.get("intent") or {}
    timings = doc.get("timings_ms") or {}
    return {
        "intent": intent.get("kinds"), "intent_confidence": intent.get("confidence"),
        "candidates": len(cands), "evidence_delivered": len(evidence),
        "gold_rank_in_candidates": pos or None,
        "above_top5": above,
        "evidence_ids": [e.get("id") for e in evidence],
        "budget_tokens": doc.get("budget_tokens"), "token_estimate": doc.get("token_estimate"),
        "latency_ms": timings,
    }


def diagnose(rows):
    return [{"run": f"{r.get('arm')}/{r.get('case')}/rep{r.get('rep')}", "agent": r.get("agent"),
             "model": r.get("model"), "category": r.get("category"), "success": r.get("success"),
             "causes": causes(r), "files": r.get("files"), "gold_in_ctx": r.get("gold_in_ctx"),
             "injected_chars": r.get("injected_chars"), "source_line": i + 1}
            for i, r in enumerate(rows)]


def run_explain(sidecar, clone, home, prompt):
    env = dict(os.environ, SIDECAR_HOME=home, SIDECAR_ENGINE="v2")
    p = subprocess.run([sidecar, "explain", "--root", clone, "--json"], input=prompt, capture_output=True,
                       text=True, env=env, timeout=120)
    if p.returncode != 0:
        return None
    return json.loads(p.stdout)


def prompt_literal_paths(clone, prompt):
    """Arquivos do checkout que contêm o texto do prompt. Se houver algum, o caso negativo não é limpo:
    a recuperação casa com o próprio texto da pergunta, e não com vocabulário genérico."""
    p = subprocess.run(["git", "-C", clone, "grep", "-l", "-i", "-F", prompt], capture_output=True, text=True)
    return [line for line in p.stdout.splitlines() if line]


def attach_explain(items, rows, cases, sidecar, repo, commit):
    """Associa a cada execução com falha (e a cada negativo com contexto) o `explain --json` do caso,
    no checkout descartável do commit fixado e com índice próprio. Não afirma causalidade."""
    prompts = {c["id"]: c for c in cases}
    work = tempfile.mkdtemp(prefix="diag-explain-")
    try:
        clone = os.path.join(work, "repo")
        subprocess.run(["git", "clone", "-q", "--shared", "--no-checkout", repo, clone], check=True)
        subprocess.run(["git", "-C", clone, "checkout", "-q", "--detach", commit], check=True)
        home = os.path.join(work, "home")
        os.makedirs(home)
        subprocess.run([sidecar, "index", "--root", clone], env=dict(os.environ, SIDECAR_HOME=home, SIDECAR_ENGINE="v2"),
                       capture_output=True, check=True, timeout=600)
        for item, row in zip(items, rows):
            case = prompts.get(row.get("case"))
            if not case:
                continue
            if row.get("category") == "negative":
                item["prompt_in_repo"] = prompt_literal_paths(clone, case["prompt"])
            if item["success"] is True and not (row.get("category") == "negative" and row.get("arm") == "ON"):
                continue
            doc = run_explain(sidecar, clone, home, case["prompt"])
            if doc is not None:
                item["explain"] = summarize_explain(doc, {r["path"] for r in case.get("relevant", [])})
    finally:
        shutil.rmtree(work, ignore_errors=True)


def render(items, source):
    lines = ["# Diagnóstico de falhas (automático, hipóteses)", "",
             f"Fonte: `{source}` · execuções: {len(items)} · falhas: {sum(1 for d in items if d['success'] is not True)}", "",
             "Classes: experiment_violation (edição ou contaminação: não é falha do agente), infra_failure, oracle_failure "
             "(resposta ilegível pelo oráculo), retrieval_failure (gold fora do contexto entregue), task_failure (resposta "
             "errada com o gold disponível), false_positive, false_positive_injection. Causas são candidatas para revisão, "
             "não verdades; `explain` mostra registros do ranking, não causalidade.", "",
             "| Execução | Modelo | Categoria | Sucesso | Classes | Respostas | Gold no contexto | Linha |",
             "|---|---|---|---|---|---|---|---|"]
    for d in items:
        lines.append(f"| {d['run']} | {d['model']} | {d['category']} | {d['success']} | "
                     f"{', '.join(d['causes']) or '—'} | {', '.join(d['files'] or []) or '—'} | "
                     f"{d['gold_in_ctx']} | {d['source_line']} |")
    detailed = [d for d in items if d.get("explain")]
    if detailed:
        lines += ["", "## Explain associado (candidatos e posição do gold)", ""]
        for d in detailed:
            e = d["explain"]
            lines.append(f"### {d['run']} — classes: {', '.join(d['causes']) or '—'}")
            lines.append(f"- intenção: {e['intent']} (confiança {e['intent_confidence']}); candidatos: {e['candidates']}; "
                         f"entregues: {e['evidence_delivered']}; orçamento: {e['budget_tokens']} tokens; "
                         f"estimado: {e['token_estimate']} tokens")
            lines.append(f"- posição do gold entre candidatos: {e['gold_rank_in_candidates'] or 'ausente'}")
            for c in e["above_top5"]:
                lines.append(f"  - não-gold entre os 5 primeiros: `{c['id']}` score {c['score']}, cobertura {c['coverage']}, "
                             f"exato {c['exact']}, razão: {c['reason']}")
            lines.append(f"- latência por fase (ms): {e['latency_ms']}")
            lines.append("")
        contaminated = [d for d in items if d.get("prompt_in_repo")]
        if contaminated:
            lines += ["## Casos negativos não limpos", "",
                      "O texto do prompt aparece no checkout; nesses casos a recuperação pode casar com a própria pergunta:", ""]
            for d in contaminated:
                lines.append(f"- {d['run']}: {', '.join(d['prompt_in_repo'])}")
            lines.append("")
    return "\n".join(lines) + "\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("runs")
    ap.add_argument("--md")
    ap.add_argument("--sidecar", help="binário sidecar: associa o `explain --json` de cada falha")
    ap.add_argument("--repo", help="repositório git de origem (com --commit e --cases)")
    ap.add_argument("--commit")
    ap.add_argument("--cases", help="arquivo de casos com os prompts")
    args = ap.parse_args()
    rows = [json.loads(line) for line in pathlib.Path(args.runs).read_text().splitlines() if line.strip()]
    items = diagnose(rows)
    if args.sidecar and args.repo and args.commit and args.cases:
        cases = json.loads(pathlib.Path(args.cases).read_text())
        cases = cases["cases"] if isinstance(cases, dict) else cases
        attach_explain(items, rows, cases, args.sidecar, args.repo, args.commit)
    text = render(items, args.runs)
    if args.md:
        pathlib.Path(args.md).write_text(text)
        print(f"wrote {args.md}")
    else:
        print(text)


if __name__ == "__main__":
    main()
