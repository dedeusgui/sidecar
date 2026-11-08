#!/usr/bin/env python3
"""Benchmark de agentes reais (Claude Code, Codex, Antigravity CLI), ON × OFF, em repositórios reais.

Cada execução: checkout descartável do commit fixado, um prompt realista (direto, PT imperfeito,
voz de ~200 palavras ou negativo), agente em modo somente leitura, resposta final em JSON com os
arquivos que ele aponta. O oráculo é o gold de arquivo de bench/ten_repos_cases.json, que o agente
nunca vê e não pode editar. Mede sucesso, tempo de parede, tokens, custo e chamadas de ferramenta
relatados pelo próprio cliente.

Braço ON: o contexto vem do binário real `sidecar hook --agent X`, alimentado com o mesmo payload
UserPromptSubmit que o cliente enviaria, e é prefixado ao prompt. É o mesmo canal nos três clientes
(comparação justa), mas NÃO é o hook instalado do cliente: o que muda é só o transporte. Não toca em
settings do usuário.

Gasta dinheiro real. Sem --yes-spend, só faz plano e validação (dry run).

Exemplo:
  bash bench/agentic/setup_10_repos.sh /tmp/sidecar-real-bench
  python3 bench/agent/run_agents.py --manifest /tmp/sidecar-real-bench/ten-repos-manifest.json \\
      --sidecar ~/.local/bin/sidecar --agents claude:haiku codex:gpt-6-luna agy:gemini-3.8-flash-low \\
      --categories direct messy_pt voice_mode negative --per-category 2 --reps 1 --out-dir /tmp/agent-bench --yes-spend
  python3 bench/agent/run_agents.py --report /tmp/agent-bench/runs.jsonl --report-md bench/results/agent-benchmark.md
"""
import argparse
import json
import os
import pathlib
import random
import re
import shutil
import statistics
import subprocess
import sys
import tempfile
import time

SUFFIX = (
    "\n\n---\nTarefa de localização, somente leitura: NÃO edite, crie nem apague arquivos. "
    "Explore o repositório atual e indique onde está o código relevante para o pedido acima. "
    "Termine sua resposta com uma única linha JSON, sem markdown, no formato "
    '{"files": ["caminho/relativo", ...]} com no máximo 5 caminhos, do mais para o menos relevante. '
    'Se nada no repositório se relaciona com o pedido, responda {"files": []}.'
)
JSON_RE = re.compile(r'\{\s*"files"\s*:\s*\[.*?\]\s*\}', re.S)


def load_cases(manifest, categories, per_category):
    m = json.loads(pathlib.Path(manifest).read_text())
    picked, count = [], {}
    for c in m["cases"]:
        if categories and c["category"] not in categories:
            continue
        if count.get(c["category"], 0) >= per_category:
            continue
        count[c["category"]] = count.get(c["category"], 0) + 1
        picked.append(c)
    return m["repos"], picked


def checkout(repo, dest):
    subprocess.run(["git", "clone", "-q", "--shared", "--no-checkout", repo["path"], dest], check=True)
    subprocess.run(["git", "-C", dest, "checkout", "-q", "--detach", repo["commit"]], check=True)


def sidecar_context(sidecar, home, workdir, prompt):
    payload = json.dumps({"hook_event_name": "UserPromptSubmit", "prompt": prompt, "cwd": workdir, "session_id": "bench"})
    env = dict(os.environ, SIDECAR_HOME=home)
    t0 = time.perf_counter()
    p = subprocess.run([sidecar, "hook", "--agent", "claude"], input=payload, capture_output=True, text=True, env=env, timeout=60)
    ms = (time.perf_counter() - t0) * 1000
    try:
        ctx = json.loads(p.stdout)["hookSpecificOutput"]["additionalContext"]
    except Exception:
        ctx = ""
    return ctx, ms


def build_cmd(agent, model, workdir, prompt, args):
    if agent == "claude":
        return ["claude", "-p", prompt, "--output-format", "json", "--model", model, "--no-session-persistence",
                "--permission-mode", "plan", "--disallowedTools", "Edit", "Write", "NotebookEdit",
                "--max-budget-usd", str(args.max_budget)], None
    if agent == "codex":
        return ["codex", "exec", "--json", "--ephemeral", "--ignore-user-config", "--skip-git-repo-check",
                "-s", "read-only", "-m", model, "-C", workdir, prompt], None
    if agent == "agy":
        # Headless não pede permissão: libera só comandos de leitura, por sessão (--settings nunca é salvo).
        allow = ["command(%s)" % c for c in ("grep", "rg", "cat", "ls", "find", "head", "tail", "sed", "wc", "git")]
        return ["agy", "--print", prompt, "--output-format", "json", "--model", model, "--mode", "plan",
                "--settings", json.dumps({"permissions": {"allow": allow}})], None
    raise SystemExit(f"agente desconhecido: {agent}")


def walk(o):
    if isinstance(o, dict):
        yield o
        for v in o.values():
            yield from walk(v)
    elif isinstance(o, list):
        for v in o:
            yield from walk(v)


def parse_output(agent, stdout):
    """Extrai texto final, tokens, custo e chamadas de ferramenta, tolerando mudanças de esquema."""
    res = {"text": stdout, "tokens_in": None, "tokens_out": None, "cost_usd": None, "tool_calls": None, "turns": None}
    docs = []
    for line in stdout.splitlines():
        line = line.strip()
        if line.startswith("{"):
            try:
                docs.append(json.loads(line))
            except ValueError:
                pass
    if not docs:
        try:
            docs = [json.loads(stdout)]
        except ValueError:
            return res
    tin = tout = 0
    saw_usage = False
    tools = 0
    texts = []
    for d in docs:
        for n in walk(d):
            if n.get("type") == "result" and isinstance(n.get("result"), str):
                texts.append(n["result"])
            if agent == "agy" and isinstance(n.get("response"), str):
                texts.append(n["response"])
            if n.get("type") == "item.completed" and isinstance(n.get("item"), dict):
                it = n["item"]
                if it.get("type") in ("command_execution", "mcp_tool_call", "file_change", "web_search"):
                    tools += 1
                if it.get("type") == "agent_message" and isinstance(it.get("text"), str):
                    texts.append(it["text"])
            if "total_cost_usd" in n and isinstance(n["total_cost_usd"], (int, float)):
                res["cost_usd"] = n["total_cost_usd"]
            if "num_turns" in n and isinstance(n["num_turns"], int):
                res["turns"] = n["num_turns"]
        if agent == "claude":
            u = d.get("usage") or {}
            if u:
                saw_usage = True
                tin = sum(u.get(k, 0) or 0 for k in ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens"))
                tout = u.get("output_tokens", 0) or 0
        else:
            for n in walk(d):
                if n.get("type") == "turn.completed" and isinstance(n.get("usage"), dict):
                    saw_usage = True
                    u = n["usage"]
                    tin += (u.get("input_tokens", 0) or 0)
                    tout += (u.get("output_tokens", 0) or 0)
                elif agent == "agy" and isinstance(n.get("usage"), dict):
                    u = n["usage"]
                    tin = u.get("input_tokens", u.get("prompt_tokens", tin)) or tin
                    tout = u.get("output_tokens", u.get("completion_tokens", tout)) or tout
                    saw_usage = True
    if saw_usage:
        res["tokens_in"], res["tokens_out"] = tin, tout
    if agent == "claude" and tools == 0:
        for d in docs:
            if isinstance(d.get("tool_calls"), int):
                tools = d["tool_calls"]
    res["tool_calls"] = tools if (tools or agent == "codex") else None
    if texts:
        res["text"] = texts[-1]
    return res


def answer_files(text):
    ms = JSON_RE.findall(text or "")
    for raw in reversed(ms):
        try:
            return [f.strip().lstrip("./") for f in json.loads(raw)["files"] if isinstance(f, str)]
        except ValueError:
            continue
    return None


def judge(case, files):
    rel = {r["path"] for r in case["relevant"]}
    if case["category"] == "negative" or not rel:
        ok = files is not None and len(files) == 0
        return {"success": ok, "hit_at_1": None, "hit_at_5": None, "malformed": files is None}
    if files is None:
        return {"success": False, "hit_at_1": False, "hit_at_5": False, "malformed": True}
    h1 = bool(files[:1]) and files[0] in rel
    h5 = any(f in rel for f in files[:5])
    return {"success": h5, "hit_at_1": h1, "hit_at_5": h5, "malformed": False}


def run_one(args, repos, case, agent, model, arm, rep, home):
    repo = repos[case["repo"]]
    work = tempfile.mkdtemp(prefix="agentbench-")
    try:
        checkout(repo, work)
        ctx_ms, ctx = 0.0, ""
        prompt = case["prompt"]
        if arm == "ON":
            ctx, ctx_ms = sidecar_context(args.sidecar, home, repo["path"], case["prompt"])  # índice quente do checkout fixado
            if ctx:
                prompt = ctx + "\n\n" + prompt
        prompt += SUFFIX
        cmd, _ = build_cmd(agent, model, work, prompt, args)
        t0 = time.perf_counter()
        timed_out = False
        try:
            p = subprocess.run(cmd, cwd=work, capture_output=True, text=True, timeout=args.timeout, stdin=subprocess.DEVNULL)
            out, err, code = p.stdout, p.stderr, p.returncode
        except subprocess.TimeoutExpired as e:
            out, err, code, timed_out = (e.stdout or ""), (e.stderr or ""), -1, True
            out = out.decode() if isinstance(out, bytes) else out
        wall = time.perf_counter() - t0
        parsed = parse_output(agent, out)
        files = answer_files(parsed["text"])
        dirty = subprocess.run(["git", "-C", work, "status", "--porcelain"], capture_output=True, text=True).stdout.strip()
        rec = {
            "agent": agent, "model": model, "arm": arm, "rep": rep, "case": case["id"], "category": case["category"],
            "repo": case["repo"], "wall_s": round(wall, 2), "sidecar_ms": round(ctx_ms, 1), "injected_chars": len(ctx),
            "exit": code, "timed_out": timed_out, "files": files, "edited_files": bool(dirty),
            "tokens_in": parsed["tokens_in"], "tokens_out": parsed["tokens_out"], "cost_usd": parsed["cost_usd"],
            "tool_calls": parsed["tool_calls"], "turns": parsed["turns"], **judge(case, files),
        }
        raw_dir = pathlib.Path(args.out_dir) / "raw"
        raw_dir.mkdir(parents=True, exist_ok=True)
        (raw_dir / f"{agent}-{arm}-{case['id']}-{rep}.txt").write_text(out + "\n--- stderr ---\n" + err[-2000:])
        return rec
    finally:
        shutil.rmtree(work, ignore_errors=True)


def boot_ci(deltas, n=2000, seed=7):
    if len(deltas) < 3:
        return None
    rnd = random.Random(seed)
    means = sorted(statistics.mean(rnd.choices(deltas, k=len(deltas))) for _ in range(n))
    return means[int(0.025 * n)], means[int(0.975 * n)]


def fmt(v, d=1):
    return "n/d" if v is None else f"{v:.{d}f}"


def mean(xs):
    xs = [x for x in xs if x is not None]
    return statistics.mean(xs) if xs else None


def report(path, md):
    rows = [json.loads(l) for l in pathlib.Path(path).read_text().splitlines() if l.strip()]
    lines = ["# Benchmark de agentes reais: Sidecar ON × OFF", "",
             f"Fonte: `{path}` · execuções: {len(rows)}. O oráculo é o gold de arquivo do manifesto (o agente não o vê). "
             "Tokens, custo e chamadas vêm do próprio cliente e podem faltar (`n/d`). ON injeta o contexto do binário "
             "`sidecar hook` por prefixo de prompt, não pelo hook instalado do cliente.", ""]
    keys = sorted({(r["agent"], r["model"]) for r in rows})
    lines += ["## Resumo por agente", "",
              "| Agente / modelo | Braço | N | Sucesso | Hit@1 | Tempo médio (s) | Tokens in | Tokens out | Custo médio (US$) | Tool calls | Resposta malformada | Editou arquivo |",
              "|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"]
    for a, m in keys:
        for arm in ("OFF", "ON"):
            rs = [r for r in rows if r["agent"] == a and r["model"] == m and r["arm"] == arm]
            if not rs:
                continue
            h1 = [r["hit_at_1"] for r in rs if r["hit_at_1"] is not None]
            lines.append(f"| {a} / {m} | {arm} | {len(rs)} | {100*sum(r['success'] for r in rs)/len(rs):.0f}% | "
                         f"{('n/d' if not h1 else f'{100*sum(h1)/len(h1):.0f}%')} | {fmt(mean([r['wall_s'] for r in rs]))} | "
                         f"{fmt(mean([r['tokens_in'] for r in rs]), 0)} | {fmt(mean([r['tokens_out'] for r in rs]), 0)} | "
                         f"{fmt(mean([r['cost_usd'] for r in rs]), 4)} | {fmt(mean([r['tool_calls'] for r in rs]))} | "
                         f"{sum(r['malformed'] for r in rs)} | {sum(r['edited_files'] for r in rs)} |")
    lines += ["", "## Por categoria de prompt (sucesso)", "", "| Agente / modelo | Categoria | OFF | ON |", "|---|---|---:|---:|"]
    for a, m in keys:
        for cat in ("direct", "messy_pt", "voice_mode", "negative"):
            cells = []
            for arm in ("OFF", "ON"):
                rs = [r for r in rows if r["agent"] == a and r["model"] == m and r["arm"] == arm and r["category"] == cat]
                cells.append("n/d" if not rs else f"{sum(r['success'] for r in rs)}/{len(rs)}")
            if cells != ["n/d", "n/d"]:
                lines.append(f"| {a} / {m} | {cat} | {cells[0]} | {cells[1]} |")
    lines += ["", "## Diferença pareada ON − OFF (mesmo caso e repetição; IC 95% por bootstrap, só com 3+ pares)", "",
              "| Agente / modelo | Pares | Δ tempo (s) | Δ tokens in | Δ custo (US$) | Δ sucesso (pontos) |", "|---|---:|---|---|---|---|"]
    for a, m in keys:
        idx = {(r["case"], r["rep"], r["arm"]): r for r in rows if r["agent"] == a and r["model"] == m}
        pairs = [(idx[(c, rp, "OFF")], idx[(c, rp, "ON")]) for (c, rp, arm) in idx if arm == "OFF" and (c, rp, "ON") in idx]
        if not pairs:
            continue

        def d(field):
            xs = [on[field] - off[field] for off, on in pairs if on[field] is not None and off[field] is not None]
            if not xs:
                return "n/d"
            ci = boot_ci(xs)
            return f"{statistics.mean(xs):+.2f}" + (f" [{ci[0]:+.2f}, {ci[1]:+.2f}]" if ci else " (poucos pares)")
        succ = [100 * (int(on["success"]) - int(off["success"])) for off, on in pairs]
        ci = boot_ci(succ)
        lines.append(f"| {a} / {m} | {len(pairs)} | {d('wall_s')} | {d('tokens_in')} | {d('cost_usd')} | "
                     f"{statistics.mean(succ):+.0f}" + (f" [{ci[0]:+.0f}, {ci[1]:+.0f}]" if ci else " (poucos pares)") + " |")
    lines += ["", "## Limites", "",
              "- Tarefa de localização somente leitura; não mede edição de código nem testes.",
              "- Poucos casos por categoria e repetições: leia o IC; sem IC, não há conclusão.",
              "- Os prompts de voz do manifesto compartilham um molde de preenchimento; não são transcrições independentes.",
              "- Tempo de parede inclui a latência do modelo e variação de rede; ON inclui o tempo do `sidecar hook` (campo `sidecar_ms`).",
              "- Preços e versões dos clientes mudam; registre-os junto do relatório."]
    pathlib.Path(md).write_text("\n".join(lines) + "\n")
    print(f"wrote {md}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--manifest")
    ap.add_argument("--sidecar", default=os.path.expanduser("~/.local/bin/sidecar"))
    ap.add_argument("--agents", nargs="+", default=["claude:haiku", "codex:gpt-6-luna", "agy:gemini-3.8-flash-low"])
    ap.add_argument("--categories", nargs="*", default=["direct", "messy_pt", "voice_mode", "negative"])
    ap.add_argument("--per-category", type=int, default=2)
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--timeout", type=int, default=300)
    ap.add_argument("--max-budget", type=float, default=0.5, help="teto em US$ por execução (Claude Code)")
    ap.add_argument("--out-dir", default="/tmp/agent-bench")
    ap.add_argument("--yes-spend", action="store_true", help="executa os agentes de verdade (custa dinheiro)")
    ap.add_argument("--report")
    ap.add_argument("--report-md", default="bench/results/agent-benchmark.md")
    args = ap.parse_args()
    if args.report:
        report(args.report, args.report_md)
        return
    if not args.manifest:
        ap.error("--manifest é obrigatório")
    repos, cases = load_cases(args.manifest, args.categories, args.per_category)
    for name, r in repos.items():
        if not pathlib.Path(r["path"], ".git").exists():
            sys.exit(f"{name}: checkout ausente em {r['path']}; rode bench/agentic/setup_10_repos.sh")
    specs = [a.split(":", 1) for a in args.agents]
    for a, _ in specs:
        if not shutil.which(a):
            sys.exit(f"cliente '{a}' não encontrado no PATH")
    total = len(cases) * len(specs) * 2 * args.reps
    print(f"plano: {len(cases)} casos × {len(specs)} agentes × 2 braços × {args.reps} rep = {total} execuções")
    for c in cases[:3]:
        print(f"  ex.: {c['id']} [{c['category']}] {c['prompt'][:80]!r}")
    if not args.yes_spend:
        print("dry run: nada executado. Use --yes-spend para rodar os agentes (gasta tokens/dinheiro).")
        return
    out = pathlib.Path(args.out_dir)
    out.mkdir(parents=True, exist_ok=True)
    home = tempfile.mkdtemp(prefix="sidecar-agent-home-")
    for r in repos.values():  # índice quente fora das medições
        subprocess.run([args.sidecar, "index", "--root", r["path"]], env=dict(os.environ, SIDECAR_HOME=home), capture_output=True)
    rnd = random.Random(11)
    jobs = [(c, a, m, arm, rep) for c in cases for a, m in specs for arm in ("OFF", "ON") for rep in range(args.reps)]
    rnd.shuffle(jobs)  # ordem aleatória reduz viés de deriva de carga e cache
    runs = out / "runs.jsonl"
    for i, (c, a, m, arm, rep) in enumerate(jobs, 1):
        print(f"[{i}/{len(jobs)}] {a}:{m} {arm} {c['id']} rep{rep}", flush=True)
        rec = run_one(args, repos, c, a, m, arm, rep, home)
        with runs.open("a") as f:
            f.write(json.dumps(rec, ensure_ascii=False) + "\n")
    report(str(runs), str(out / "agent-benchmark.md"))


if __name__ == "__main__":
    main()
