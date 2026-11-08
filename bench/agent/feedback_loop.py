#!/usr/bin/env python3
"""Protótipo de laço de feedback; execução com agentes está bloqueada até qualificação.

Para cada alvo (um repositório e o gold de arquivo de um caso), em um SIDECAR_HOME isolado:
  1. Turno 1 (prompt A, braço ON): o bloco do Sidecar traz uma tabela de evidências E1..En. O agente
     responde {"files": [...], "helpful": ["E1"], "misleading": ["E3"]}.
  2. O runner deriva o feedback com a função PURA `derive_feedback` (sem oráculo, sem modelo, mesma
     entrada dá mesma saída) e o grava com o binário real: `sidecar feedback record`.
  3. Turno 2 (prompts irmãos B do mesmo alvo): mede a posição do gold na evidência ANTES (cópia do
     índice sem feedback = controle) e DEPOIS (índice com feedback). É medição de recuperação,
     sem modelo. Com --agent-turn2, o agente real também resolveria B com e sem feedback.
  3b. Verificação de determinismo: o mesmo relatório gravado em outra cópia produz os mesmos pesos.

O gold só é usado para AVALIAR (rank, sucesso), nunca para gerar o feedback, que usa apenas o que um
agente em produção veria: as evidências injetadas e a resposta dele. Custo do feedback: tokens extras
da tabela E1..En e o tempo do `feedback record`.

Sem --yes-spend, só planeja. A execução paga está desabilitada até validar isolamento,
tratamento de falhas, orçamento e comparação equilibrada dos braços.

  python3 bench/agent/feedback_loop.py --manifest /tmp/sidecar-real-bench/ten-repos-manifest.json \\
      --agents claude:haiku --targets 6 --out-dir /tmp/fb-loop --yes-spend
  python3 bench/agent/feedback_loop.py --report /tmp/fb-loop/loop.jsonl --report-md bench/results/feedback-loop.md
"""
import argparse
import hashlib
import json
import os
import pathlib
import shutil
import statistics
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, str(pathlib.Path(__file__).parent))
import run_agents as ra  # noqa: E402

SUFFIX_FB = (
    "\n\n---\nTarefa de localização, somente leitura: NÃO edite, crie nem apague arquivos. "
    "Indique onde está o código relevante para o pedido acima. Termine com uma única linha JSON, sem markdown: "
    '{"files": ["caminho/relativo", ...], "helpful": ["E1"], "misleading": ["E3"]} '
    "com no máximo 5 caminhos. 'helpful' = itens E do bloco do Sidecar que você usou; 'misleading' = itens E "
    "que apontavam para o lugar errado ou atrapalharam. Use [] quando não houver. "
    'Se nada se relaciona ao pedido, responda {"files": [], "helpful": [], "misleading": []}.'
)


def explain(sidecar, home, root, prompt):
    p = subprocess.run([sidecar, "explain", "--root", root, "--engine", "v2", "--json"], input=prompt, capture_output=True, text=True,
                       env=dict(os.environ, SIDECAR_HOME=home), timeout=120, check=True)
    return json.loads(p.stdout)


def gold_rank(evidence, rel):
    for i, e in enumerate(evidence, 1):
        if e["path"] in rel:
            return i
    return None


def evidence_table(evidence):
    return "\n".join(f"E{i} {e['id']}" for i, e in enumerate(evidence, 1))


def parse_report(text):
    """Lê o JSON do agente; devolve (files, helpful, misleading) ou None se malformado."""
    for raw in reversed(ra.re.findall(r'\{[^{}]*"files"[^{}]*\}', text or "", ra.re.S)):
        try:
            d = json.loads(raw)
            fields = [d.get(k) for k in ("files", "helpful", "misleading")]
            if any(not isinstance(v, list) or any(not isinstance(x, str) for x in v) for v in fields):
                return None
            if len(fields[0]) > 5:
                return None
            return tuple(fields)
        except ValueError:
            continue
    return None


def derive_feedback(evidence, report):
    """Função pura: (evidências injetadas, relatório do agente) -> argumentos de `feedback record`.

    - helpful: evidências cujo caminho o agente escolheu na resposta, mais as que ele marcou como úteis.
    - misleading: SOMENTE o que o agente marcou explicitamente (e que não escolheu): ausência de uso não
      é prova de erro, então não penaliza por omissão.
    - confiança 'high' só com JSON válido e ao menos um item útil; caso contrário 'low'.
    """
    if report is None:
        return None
    files, helpful_tok, mis_tok = report
    by_tok = {f"E{i}": e for i, e in enumerate(evidence, 1)}
    chosen = {f.strip().lstrip("./") for f in files}
    helpful = [e["id"] for e in evidence if e["path"] in chosen]
    helpful += [by_tok[t]["id"] for t in helpful_tok if t in by_tok]
    helpful = list(dict.fromkeys(helpful))
    mis = [by_tok[t]["id"] for t in mis_tok if t in by_tok and by_tok[t]["id"] not in helpful]
    mis = list(dict.fromkeys(mis))
    if not helpful and not mis:
        return None
    return {"helpful": helpful, "misleading": mis, "confidence": "high" if helpful else "low",
            "reason": "noisy_neighbor" if mis else "exact_match", "completed": bool(files)}


def record(sidecar, home, root, fb):
    cmd = [sidecar, "feedback", "record", "--root", root, "-confidence", fb["confidence"], "-reason", fb["reason"]]
    if fb["completed"]:
        cmd.append("-completed")
    for i in fb["helpful"]:
        cmd += ["-helpful", i]
    for i in fb["misleading"]:
        cmd += ["-misleading", i]
    t0 = time.perf_counter()
    p = subprocess.run(cmd, capture_output=True, text=True, env=dict(os.environ, SIDECAR_HOME=home))
    return (time.perf_counter() - t0) * 1000, p.returncode == 0, p.stdout


def listing(sidecar, home, root):
    p = subprocess.run([sidecar, "feedback", "list", "--root", root], capture_output=True, text=True, env=dict(os.environ, SIDECAR_HOME=home))
    return hashlib.sha256(p.stdout.encode()).hexdigest()[:12], p.stdout


def agent_turn(args, repo, agent, model, prompt):
    work = tempfile.mkdtemp(prefix="fbloop-")
    try:
        ra.checkout(repo, work)
        cmd, _ = ra.build_cmd(agent, model, work, prompt, args)
        t0 = time.perf_counter()
        try:
            p = subprocess.run(cmd, cwd=work, capture_output=True, text=True, timeout=args.timeout, stdin=subprocess.DEVNULL)
            out = p.stdout
        except subprocess.TimeoutExpired as e:
            out = (e.stdout or b"").decode() if isinstance(e.stdout, bytes) else (e.stdout or "")
        wall = time.perf_counter() - t0
        parsed = ra.parse_output(agent, out)
        return parsed, wall
    finally:
        shutil.rmtree(work, ignore_errors=True)


def targets(cases, n):
    groups = {}
    for c in cases:
        if c["category"] == "negative" or not c["relevant"]:
            continue
        # Prompts são irmãos apenas quando repositório e conjunto gold coincidem.
        key = (c["repo"], tuple(sorted(r["path"] for r in c["relevant"])))
        groups.setdefault(key, []).append(c)
    out = []
    for gid, cs in groups.items():
        cs = sorted(cs, key=lambda c: c["id"])
        if len(cs) >= 2:
            out.append((cs[0], cs[1:3]))
    return out[:n]


def run(args):
    m = json.loads(pathlib.Path(args.manifest).read_text())
    repos = m["repos"]
    tgts = targets(m["cases"], args.targets)
    specs = [a.split(":", 1) for a in args.agents]
    print(f"plano: {len(tgts)} alvos × {len(specs)} agentes; turno 1 com agente real; turno 2 por recuperação"
          + (" + agente" if args.agent_turn2 else ""))
    for a, bs in tgts[:3]:
        print(f"  ex.: A={a['id']} {a['prompt'][:60]!r} -> B={[b['id'] for b in bs]}")
    if not args.yes_spend:
        print("dry run: nada executado. A execução com agentes está temporariamente indisponível.")
        return
    raise SystemExit("execução com agentes indisponível: o experimento de feedback ainda não foi qualificado")
    out = pathlib.Path(args.out_dir)
    out.mkdir(parents=True, exist_ok=True)
    for agent, model in specs:
        for A, Bs in tgts:
            repo = repos[A["repo"]]
            rel = {r["path"] for r in A["relevant"]}
            home = tempfile.mkdtemp(prefix="fbloop-home-")
            subprocess.run([args.sidecar, "index", "--root", repo["path"]], env=dict(os.environ, SIDECAR_HOME=home), capture_output=True)
            ctrl, chk = home + "-ctrl", home + "-chk"
            shutil.copytree(home, ctrl)
            shutil.copytree(home, chk)
            ev_a = explain(args.sidecar, home, repo["path"], A["prompt"])
            evidence = ev_a.get("evidence") or []
            table = evidence_table(evidence)
            prompt = (ev_a.get("context") or "") + "\n\nIDs das evidências acima:\n" + table + "\n\n" + A["prompt"] + SUFFIX_FB
            print(f"[{agent}] turno 1 {A['id']}", flush=True)
            parsed, wall = agent_turn(args, repo, agent, model, prompt)
            rep = parse_report(parsed["text"])
            fb = derive_feedback(evidence, rep)
            rec = {"agent": agent, "model": model, "target": A["id"], "repo": A["repo"], "turn1_wall_s": round(wall, 2),
                   "turn1_tokens_in": parsed["tokens_in"], "turn1_tokens_out": parsed["tokens_out"], "turn1_cost_usd": parsed["cost_usd"],
                   "turn1_files": rep[0] if rep else None, "turn1_success": bool(rep and set(rep[0]) & rel),
                   "malformed": rep is None, "table_chars": len(table), "evidence_n": len(evidence),
                   "evidence_noise": (sum(1 for e in evidence if e["path"] not in rel) / len(evidence)) if evidence else None,
                   "feedback": fb, "probes": []}
            if fb:
                ms, ok, _ = record(args.sidecar, home, repo["path"], fb)
                ms2, ok2, _ = record(args.sidecar, chk, repo["path"], fb)  # mesma entrada, outra cópia
                h1, _ = listing(args.sidecar, home, repo["path"])
                h2, _ = listing(args.sidecar, chk, repo["path"])
                rec.update({"record_ms": round(ms, 1), "record_ok": ok and ok2, "deterministic": (h1 == h2) if ok and ok2 else None})
            for B in Bs:
                relB = {r["path"] for r in B["relevant"]}
                before = explain(args.sidecar, ctrl, repo["path"], B["prompt"])
                after = explain(args.sidecar, home, repo["path"], B["prompt"])
                eb, ea = before.get("evidence") or [], after.get("evidence") or []
                probe = {"case": B["id"], "category": B["category"], "rank_before": gold_rank(eb, relB), "rank_after": gold_rank(ea, relB),
                         "top1_before": eb[0]["path"] if eb else None, "top1_after": ea[0]["path"] if ea else None}
                if args.agent_turn2:
                    for label, ev in (("ctrl", before), ("fb", after)):
                        pr = (ev.get("context") or "") + "\n\n" + B["prompt"] + ra.SUFFIX
                        p2, w2 = agent_turn(args, repo, agent, model, pr)
                        files = ra.answer_files(p2["text"])
                        probe[label] = {"success": bool(files and set(files) & relB), "wall_s": round(w2, 2),
                                        "tokens_in": p2["tokens_in"], "cost_usd": p2["cost_usd"]}
                rec["probes"].append(probe)
            with (out / "loop.jsonl").open("a") as f:
                f.write(json.dumps(rec, ensure_ascii=False) + "\n")
            for d in (home, ctrl, chk):
                shutil.rmtree(d, ignore_errors=True)
    report(str(out / "loop.jsonl"), str(out / "feedback-loop.md"))


def mrr(rank):
    return 0.0 if not rank else 1.0 / rank


def report(path, md):
    rows = [json.loads(l) for l in pathlib.Path(path).read_text().splitlines() if l.strip()]
    L = ["# Experimento de feedback com agentes", "",
         f"Fonte: `{path}` · alvos×agentes: {len(rows)}. O feedback é derivado só do que o agente viu e respondeu "
         "(`derive_feedback`, função pura); o gold serve apenas para avaliar. Este relatório não estabelece ROI.", "",
         "## Resultado por agente", "",
         "| Agente / modelo | Alvos | Relatório válido | Com feedback gravado | Pesos repetidos | Ruído da evidência | Tokens extras estimados | `feedback record` (ms) |",
         "|---|---:|---:|---:|---:|---:|---:|---:|"]
    keys = sorted({(r["agent"], r["model"]) for r in rows})
    for a, m in keys:
        rs = [r for r in rows if r["agent"] == a and r["model"] == m]
        rec = [r for r in rs if r.get("record_ok") is True]
        noise = [r["evidence_noise"] for r in rs if r.get("evidence_noise") is not None]
        L.append(f"| {a} / {m} | {len(rs)} | {sum(not r['malformed'] for r in rs)}/{len(rs)} | {len(rec)}/{len(rs)} | "
                 f"{sum(bool(r.get('deterministic')) for r in rec)}/{len(rec)} | "
                 f"{('n/d' if not noise else f'{100*statistics.mean(noise):.0f}%')} | "
                 f"{statistics.mean(r['table_chars'] for r in rs)/4:.0f} | "
                 f"{('n/d' if not rec else f'{statistics.mean(r.get("record_ms", 0) for r in rec):.1f}')} |")
    L += ["", "## Efeito no turno 2 (prompts irmãos do mesmo alvo): posição do gold na evidência", "",
          "| Agente / modelo | Sondas | Hit@1 antes | Hit@1 depois | MRR antes | MRR depois | Melhorou | Piorou | Igual |",
          "|---|---:|---:|---:|---:|---:|---:|---:|---:|"]
    for a, m in keys:
        ps = [p for r in rows if r["agent"] == a and r["model"] == m and r.get("record_ok") is True for p in r["probes"]]
        if not ps:
            L.append(f"| {a} / {m} | 0 | n/d | n/d | n/d | n/d | n/d | n/d | n/d |")
            continue
        h = lambda k: sum(1 for p in ps if p[k] == 1)  # noqa: E731
        up = sum(1 for p in ps if mrr(p["rank_after"]) > mrr(p["rank_before"]))
        dn = sum(1 for p in ps if mrr(p["rank_after"]) < mrr(p["rank_before"]))
        L.append(f"| {a} / {m} | {len(ps)} | {h('rank_before')}/{len(ps)} | {h('rank_after')}/{len(ps)} | "
                 f"{statistics.mean(mrr(p['rank_before']) for p in ps):.3f} | {statistics.mean(mrr(p['rank_after']) for p in ps):.3f} | "
                 f"{up} | {dn} | {len(ps)-up-dn} |")
    t2 = [(r, p) for r in rows if r.get("record_ok") is True for p in r["probes"] if "fb" in p]
    if t2:
        L += ["", "## Turno 2 com agente real: com feedback × controle", "",
              "| Agente / modelo | Sondas | Sucesso controle | Sucesso com feedback | Tokens in controle | Tokens in com feedback | Tempo controle (s) | Tempo com feedback (s) |",
              "|---|---:|---:|---:|---:|---:|---:|---:|"]
        for a, m in keys:
            ps = [p for r, p in t2 if r["agent"] == a and r["model"] == m]
            if not ps:
                continue
            avg = lambda k, f: statistics.mean([p[k][f] for p in ps if p[k].get(f) is not None] or [float("nan")])  # noqa: E731
            L.append(f"| {a} / {m} | {len(ps)} | {sum(p['ctrl']['success'] for p in ps)}/{len(ps)} | {sum(p['fb']['success'] for p in ps)}/{len(ps)} | "
                     f"{avg('ctrl','tokens_in'):.0f} | {avg('fb','tokens_in'):.0f} | {avg('ctrl','wall_s'):.1f} | {avg('fb','wall_s'):.1f} |")
    L += ["", "## Limites", "",
          "- Poucos alvos e uma repetição: sem intervalo de confiança, nada aqui é conclusão.",
          "- A estimativa de tokens usa caracteres divididos por quatro; autorrelato e gold podem ser incompletos.",
          "- Prompts irmãos do mesmo autor compartilham alvo e vocabulário; isso favorece a reutilização do feedback.",
          "- Feedback depende de `file_hash`: editar o arquivo o invalida (testado em `internal/store/feedback_test.go`).",
          "- O ROI líquido exige somar o custo do turno 1 extra (tabela + relatório) contra a economia no turno 2 com agente real."]
    pathlib.Path(md).write_text("\n".join(L) + "\n")
    print(f"wrote {md}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--manifest")
    ap.add_argument("--sidecar", default=os.path.expanduser("~/.local/bin/sidecar"))
    ap.add_argument("--agents", nargs="+", default=["claude:haiku"])
    ap.add_argument("--targets", type=int, default=6)
    ap.add_argument("--timeout", type=int, default=300)
    ap.add_argument("--max-budget", type=float, default=0.5)
    ap.add_argument("--agent-turn2", action="store_true", help="o agente real também resolve os prompts irmãos, com e sem feedback")
    ap.add_argument("--out-dir", default="/tmp/fb-loop")
    ap.add_argument("--yes-spend", action="store_true")
    ap.add_argument("--report")
    ap.add_argument("--report-md", default="bench/results/feedback-loop.md")
    args = ap.parse_args()
    if args.report:
        report(args.report, args.report_md)
        return
    if not args.manifest:
        ap.error("--manifest é obrigatório")
    run(args)


if __name__ == "__main__":
    main()
