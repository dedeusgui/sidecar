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
import hashlib
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
import uuid

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
    if per_category < 1:
        raise ValueError("--per-category deve ser positivo")
    cases = [c for c in m["cases"] if not categories or c["category"] in categories]
    category_order = list(dict.fromkeys(c["category"] for c in cases))
    repo_order = list(dict.fromkeys(c["repo"] for c in cases))
    picked, offset = [], 0
    for category in category_order:
        by_repo = {repo: [c for c in cases if c["repo"] == repo and c["category"] == category]
                   for repo in repo_order}
        count = 0
        while count < per_category and any(by_repo.values()):
            repo = repo_order[offset % len(repo_order)]
            offset += 1
            if by_repo[repo]:
                picked.append(by_repo[repo].pop(0))
                count += 1
    return m["repos"], picked


def validate_repos(repos, cases):
    """O índice ON e o clone do agente devem partir do mesmo commit limpo."""
    for name in sorted({case["repo"] for case in cases}):
        repo = repos[name]
        root = repo["path"]
        try:
            expected = subprocess.run(["git", "-C", root, "rev-parse", "--verify", repo["commit"] + "^{commit}"],
                                      check=True, capture_output=True, text=True).stdout.strip()
            actual = subprocess.run(["git", "-C", root, "rev-parse", "HEAD"],
                                    check=True, capture_output=True, text=True).stdout.strip()
            dirty = subprocess.run(["git", "-C", root, "status", "--porcelain", "--untracked-files=all"],
                                   check=True, capture_output=True, text=True).stdout.strip()
        except (OSError, subprocess.CalledProcessError) as exc:
            raise ValueError(f"{name}: não foi possível verificar o checkout: {exc}") from exc
        if actual != expected:
            raise ValueError(f"{name}: HEAD difere do commit fixado no manifesto")
        if dirty:
            raise ValueError(f"{name}: checkout possui alterações; não indexar corpus sujo")
        for rel in (".claude/settings.json", ".claude/settings.local.json"):
            if pathlib.Path(root, rel).exists():
                raise ValueError(f"{name}: {rel} pode carregar hooks no cliente Claude")


def checkout(repo, dest):
    subprocess.run(["git", "clone", "-q", "--shared", "--no-checkout", repo["path"], dest], check=True)
    subprocess.run(["git", "-C", dest, "checkout", "-q", "--detach", repo["commit"]], check=True)


def sidecar_context(sidecar, home, workdir, prompt, engine=None):
    payload = json.dumps({"hook_event_name": "UserPromptSubmit", "prompt": prompt, "cwd": workdir, "session_id": uuid.uuid4().hex})
    env = dict(os.environ, SIDECAR_HOME=home)
    if engine is not None:
        if engine not in ("v1", "v2"):
            raise ValueError("engine deve ser v1 ou v2")
        env["SIDECAR_ENGINE"] = engine
    t0 = time.perf_counter()
    p = subprocess.run([sidecar, "hook", "--agent", "claude"], input=payload, capture_output=True, text=True, env=env, timeout=60)
    ms = (time.perf_counter() - t0) * 1000
    if p.returncode != 0:
        raise RuntimeError(f"sidecar hook falhou (exit {p.returncode})")
    if not p.stdout.strip():
        return "", ms
    try:
        data = json.loads(p.stdout)
        output = data["hookSpecificOutput"]
        ctx = output["additionalContext"]
        if output["hookEventName"] != "UserPromptSubmit" or not isinstance(ctx, str) or not ctx:
            raise ValueError("hookSpecificOutput inválido")
    except (ValueError, TypeError, KeyError) as exc:
        raise RuntimeError("sidecar hook retornou JSON inválido") from exc
    return ctx, ms


def validate_model(agent, model):
    """Claude Code aliases ('haiku') were observed serving a different model (claude-sonnet-5-5, about
    20x the cost of claude-haiku-5-5). Only explicit IDs are accepted for Claude."""
    if agent == "claude" and not model.startswith("claude-"):
        raise ValueError(f"claude:{model}: use o ID explícito (ex.: claude-haiku-5-5); o alias '{model}' pode ser servido por outro modelo")


def hook_settings(sidecar, agent, event):
    return {event: [{"hooks": [{"type": "command", "command": f"{sidecar} hook --agent {agent}"}]}]}


def build_cmd(agent, model, workdir, prompt, args, real_hook=False):
    if agent == "claude":
        cmd = ["claude", "-p", prompt, "--output-format", "json", "--model", model, "--no-session-persistence",
               "--permission-mode", "plan", "--setting-sources", "project,local", "--strict-mcp-config",
               "--disable-slash-commands", "--disallowedTools", "Edit", "Write", "NotebookEdit",
               "--max-budget-usd", str(args.max_budget)]
        if real_hook:
            cmd += ["--settings", json.dumps({"hooks": hook_settings(args.sidecar, "claude", "UserPromptSubmit")})]
        return cmd, None
    if agent == "codex":
        return ["codex", "exec", "--json", "--ephemeral", "--ignore-user-config", "--skip-git-repo-check",
                "-s", "read-only", "-m", model, "-C", workdir, prompt], None
    if agent == "agy":
        # Headless não pede permissão: libera só comandos de leitura, por sessão (--settings nunca é salvo).
        allow = ["command(%s)" % c for c in ("grep", "rg", "cat", "ls", "find", "head", "tail", "sed", "wc", "git")]
        settings = {"permissions": {"allow": allow}}
        if real_hook:
            settings["hooks"] = hook_settings(args.sidecar, "antigravity", "PreInvocation")
        return ["agy", "--print", prompt, "--output-format", "json", "--model", model, "--mode", "plan",
                "--settings", json.dumps(settings)], None
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
    res = {"text": stdout, "tokens_in": None, "tokens_out": None, "cost_usd": None, "tool_calls": None, "turns": None,
           "served_models": None}
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
    for d in docs:
        usage = d.get("modelUsage")
        if isinstance(usage, dict) and usage:
            res["served_models"] = sorted(usage)
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
            files = json.loads(raw)["files"]
            if not isinstance(files, list) or len(files) > 5:
                return None
            result = []
            for f in files:
                if not isinstance(f, str):
                    return None
                f = f.strip()
                while f.startswith("./"):
                    f = f[2:]
                if not f or f.startswith("/") or ".." in f.split("/"):
                    return None
                result.append(f)
            return result
        except ValueError:
            continue
    return None


def judge(case, files):
    rel = {r["path"] for r in case.get("relevant", [])}
    if case["category"] == "negative" or not rel:
        ok = files is not None and len(files) == 0
        return {"success": ok, "hit_at_1": None, "hit_at_5": None, "malformed": files is None}
    if files is None:
        return {"success": False, "hit_at_1": False, "hit_at_5": False, "malformed": True}
    h1 = bool(files[:1]) and files[0] in rel
    h5 = any(f in rel for f in files[:5])
    return {"success": h5, "hit_at_1": h1, "hit_at_5": h5, "malformed": False}


CTX_PATH_RE = re.compile(r"\(([^()\s:]+\.[A-Za-z0-9]+):\d+-\d+\)|\bfile ([^\s()]+\.[A-Za-z0-9]+)")


def ctx_paths(ctx):
    """Caminhos de arquivo citados no bloco injetado pelo Sidecar."""
    return {a or b for a, b in CTX_PATH_RE.findall(ctx or "")}


def hook_runs(sidecar, home):
    p = subprocess.run([sidecar, "activity", "--json", "--limit", "10000"], capture_output=True, text=True,
                       env=dict(os.environ, SIDECAR_HOME=home))
    try:
        return int(json.loads(p.stdout).get("runs", 0))
    except Exception:
        return 0


def attention(case, files, ctx, workdir):
    """Coincidências entre caminhos da resposta, gold e contexto; não inferem causalidade."""
    rel = {r["path"] for r in case.get("relevant", [])}
    cp = ctx_paths(ctx)
    ans = set(files or [])
    return {
        "ctx_paths": len(cp),
        "gold_in_ctx": bool(rel & cp) if rel else None,
        "gold_ignored": bool(rel & cp) and not (rel & ans) if rel else None,   # estava no contexto e o agente não respondeu
        "followed_noise": bool((ans & cp) - rel) if cp else None,              # respondeu um caminho do contexto que não é gold
        "hallucinated": sum(1 for f in ans if not os.path.exists(os.path.join(workdir, f))),
        "answer_n": len(ans),
    }


def run_one(args, repos, case, agent, model, arm, rep, home):
    repo = repos[case["repo"]]
    if args.real_hooks:
        raise ValueError("--real-hooks indisponível: isolamento e medição ainda não validados")
    real = False
    work = tempfile.mkdtemp(prefix="agentbench-")
    try:
        if not real:
            checkout(repo, work)
        ctx_ms, ctx = 0.0, ""
        prompt = case["prompt"]
        if arm == "ON" and not real:
            ctx, ctx_ms = sidecar_context(args.sidecar, home, repo["path"], case["prompt"], engine=args.engine)  # índice quente do checkout fixado
            if ctx:
                prompt = ctx + "\n\n" + prompt
        prompt += SUFFIX
        before = hook_runs(args.sidecar, home) if real else 0
        cmd, _ = build_cmd(agent, model, work, prompt, args, real_hook=real)
        env = {key: value for key, value in os.environ.items()
               if key not in ("ORCA_AGENT_HOOK_PORT", "ORCA_AGENT_HOOK_TOKEN", "ORCA_PANE_KEY")}
        if real:
            env["SIDECAR_HOME"] = home
        t0 = time.perf_counter()
        timed_out = False
        try:
            p = subprocess.run(cmd, cwd=work, capture_output=True, text=True, timeout=args.timeout, stdin=subprocess.DEVNULL, env=env)
            out, err, code = p.stdout, p.stderr, p.returncode
        except subprocess.TimeoutExpired as e:
            out, err, code, timed_out = (e.stdout or ""), (e.stderr or ""), -1, True
            out = out.decode(errors="replace") if isinstance(out, bytes) else out
            err = err.decode(errors="replace") if isinstance(err, bytes) else err
        wall = time.perf_counter() - t0 + ctx_ms / 1000.0
        hook_fired = None
        if real:
            hook_fired = hook_runs(args.sidecar, home) > before
            if hook_fired:  # o contexto real não aparece no JSON do cliente; reconstitui o mesmo bloco só para diagnóstico
                ctx, ctx_ms = sidecar_context(args.sidecar, home, work, case["prompt"], engine=args.engine)
        parsed = parse_output(agent, out)
        files = answer_files(parsed["text"])
        dirty = subprocess.run(["git", "-C", work, "status", "--porcelain"], capture_output=True, text=True, check=True).stdout.strip()
        rec = {
            "agent": agent, "model": model, "arm": arm, "rep": rep, "case": case["id"], "category": case["category"],
            "repo": case["repo"], "engine": args.engine, "transport": ("real_hook" if real else ("prompt_prefix" if arm == "ON" else "none")),
            "hook_fired": hook_fired, "wall_s": round(wall, 2), "sidecar_ms": round(ctx_ms, 1), "injected_chars": len(ctx),
            "exit": code, "timed_out": timed_out, "files": files, "edited_files": bool(dirty),
            "tokens_in": parsed["tokens_in"], "tokens_out": parsed["tokens_out"], "cost_usd": parsed["cost_usd"],
            "tool_calls": parsed["tool_calls"], "turns": parsed["turns"], "served_models": parsed["served_models"],
            **judge(case, files),
            **attention(case, files, ctx, work),
        }
        if code != 0 or timed_out or dirty:
            rec["success"] = False
            for metric in ("hit_at_1", "hit_at_5"):
                if rec[metric] is not None:
                    rec[metric] = False
        if getattr(args, "save_raw", False):
            raw_dir = pathlib.Path(args.out_dir) / "raw"
            raw_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
            if raw_dir.is_symlink() or raw_dir.stat().st_mode & 0o077:
                raise ValueError(f"{raw_dir}: diretório raw deve ser privado (0700)")
            raw_path = raw_dir / f"{agent}-{arm}-{case['id']}-{rep}.txt"
            fd = os.open(raw_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, "w") as raw:
                raw.write(out + "\n--- stderr ---\n" + err[-2000:])
        return rec
    finally:
        if not real:
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


def paired_runs(rows):
    """Pareia por caso e repetição; rejeita duplicatas em vez de sobrescrevê-las."""
    idx = {}
    for row in rows:
        key = (row["case"], row["rep"], row["arm"])
        if key in idx:
            raise ValueError(f"execução duplicada: {key}")
        if row["arm"] not in ("ON", "OFF"):
            raise ValueError(f"braço inválido: {row['arm']}")
        idx[key] = row
    groups = {(case, rep) for case, rep, _ in idx}
    pairs = [(idx[(case, rep, "OFF")], idx[(case, rep, "ON")])
             for case, rep in sorted(groups) if (case, rep, "OFF") in idx and (case, rep, "ON") in idx]
    for off, on in pairs:
        if any(off.get(field) != on.get(field) for field in ("repo", "category")):
            raise ValueError(f"par com metadados inconsistentes: {off['case']} rep {off['rep']}")
    return pairs, len(groups) - len(pairs)


def report(path, md):
    rows = [json.loads(l) for l in pathlib.Path(path).read_text().splitlines() if l.strip()]
    lines = ["# Benchmark de agentes reais: Sidecar ON × OFF", "",
             f"Fonte: `{path}` · execuções: {len(rows)}. O oráculo é o gold de arquivo do manifesto (o agente não o vê). "
             "Tokens, custo e chamadas vêm do próprio cliente e podem faltar (`n/d`). ON injeta o contexto do binário "
             "`sidecar hook` por prefixo de prompt quando há contexto; não usa o hook instalado do cliente.", ""]
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
    lines += ["", "## Diferença pareada ON − OFF", "",
              "Cada tarefa tem o mesmo peso: calculamos a média das repetições pareadas por tarefa e "
              "bootstrap sobre tarefas (IC 95% apenas com 3+ tarefas). Pares incompletos são reportados, não imputados.", "",
              "| Agente / modelo | Pares completos | Tarefas | Sem par | Δ tempo (s) | Δ tokens in | Δ custo (US$) | Δ sucesso (pontos) |",
              "|---|---:|---:|---:|---|---|---|---|"]
    for a, m in keys:
        pairs, incomplete = paired_runs([r for r in rows if r["agent"] == a and r["model"] == m])
        if not pairs:
            lines.append(f"| {a} / {m} | 0 | 0 | {incomplete} | n/d | n/d | n/d | n/d |")
            continue

        def d(field, scale=1):
            groups = {}
            for off, on in pairs:
                if on[field] is not None and off[field] is not None:
                    groups.setdefault(off["case"], []).append(scale * (on[field] - off[field]))
            xs = [statistics.mean(values) for values in groups.values()]
            if not xs:
                return "n/d"
            ci = boot_ci(xs)
            return f"{statistics.mean(xs):+.2f} (n={len(xs)} tarefas)" + (f" [{ci[0]:+.2f}, {ci[1]:+.2f}]" if ci else " (poucas tarefas)")
        tasks = len({off["case"] for off, _ in pairs})
        lines.append(f"| {a} / {m} | {len(pairs)} | {tasks} | {incomplete} | {d('wall_s')} | "
                     f"{d('tokens_in')} | {d('cost_usd')} | {d('success', 100)} |")
    lines += ["", "## Coincidências entre resposta e contexto (braço ON)", "",
              "| Agente / modelo | N ON | Contexto vazio | Hook disparou | Gold no contexto | Gold ausente da resposta | Outros caminhos em comum | Caminhos inexistentes | Resposta malformada | Transporte |",
              "|---|---:|---:|---:|---:|---:|---:|---:|---:|---|"]
    for a, m in keys:
        rs = [r for r in rows if r["agent"] == a and r["model"] == m and r["arm"] == "ON"]
        if not rs:
            continue

        def rate(field, base=None):
            xs = [r[field] for r in rs if r.get(field) is not None and (base is None or base(r))]
            return "n/d" if not xs else f"{sum(bool(x) for x in xs)}/{len(xs)}"
        fired = [r["hook_fired"] for r in rs if r.get("hook_fired") is not None]
        lengths = [r["injected_chars"] for r in rs if r.get("injected_chars") is not None]
        empty = "n/d" if not lengths else f"{sum(n == 0 for n in lengths)}/{len(lengths)}"
        lines.append(f"| {a} / {m} | {len(rs)} | {empty} | {('n/a (prefixo)' if not fired else f'{sum(fired)}/{len(fired)}')} | "
                     f"{rate('gold_in_ctx')} | {rate('gold_ignored', lambda r: r.get('gold_in_ctx'))} | {rate('followed_noise')} | "
                     f"{sum(r.get('hallucinated') or 0 for r in rs)} | {sum(r['malformed'] for r in rs)} | "
                     f"{', '.join(sorted({r.get('transport', '?') for r in rs}))} |")
    lines += ["", "Contexto vazio significa que nenhum prefixo foi entregue; diferenças entre braços ainda podem ocorrer por variabilidade. "
              "Estas coincidências não demonstram uso, atenção ou causalidade; o gold de arquivo pode estar incompleto. "
              "Caminhos inexistentes são contados no checkout.", ""]
    lines += ["", "## Limites", "",
              "- Tarefa de localização somente leitura; não mede edição de código nem testes.",
              "- Poucos casos por categoria e repetições: sem amostra adequada não há conclusão.",
              "- Os prompts de voz do manifesto compartilham um molde de preenchimento; não são transcrições independentes.",
              "- Tempo de parede inclui a latência do modelo e variação de rede; ON inclui o tempo do `sidecar hook` (campo `sidecar_ms`).",
              "- Preços e versões dos clientes mudam; registre-os junto do relatório."]
    pathlib.Path(md).write_text("\n".join(lines) + "\n")
    print(f"wrote {md}")


def schedule(cases, specs, reps, seed=11):
    """Pares adjacentes e ordem AB/BA alternada, com semente registrada."""
    pairs = [(c, a, m, rep) for c in cases for a, m in specs for rep in range(reps)]
    random.Random(seed).shuffle(pairs)
    return [(c, a, m, arm, rep) for i, (c, a, m, rep) in enumerate(pairs)
            for arm in (("OFF", "ON") if i % 2 == 0 else ("ON", "OFF"))]


def file_sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--manifest")
    ap.add_argument("--sidecar", default=os.path.expanduser("~/.local/bin/sidecar"))
    ap.add_argument("--engine", choices=("v1", "v2"), default="v2", help="engine Sidecar dos benchmarks (padrão: v2)")
    ap.add_argument("--agents", nargs="+", default=["claude:claude-haiku-5-5", "codex:gpt-6-luna"])
    ap.add_argument("--categories", nargs="*", default=["direct", "messy_pt", "voice_mode", "negative"])
    ap.add_argument("--per-category", type=int, default=2)
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--timeout", type=int, default=300)
    ap.add_argument("--max-budget", type=float, default=0.3, help="teto em US$ por execução (Claude Code)")
    ap.add_argument("--total-budget", type=float, default=10.0, help="teto agregado reservado para chamadas Claude; outros clientes podem não informar custo")
    ap.add_argument("--out-dir", default="/tmp/agent-bench")
    ap.add_argument("--real-hooks", action="store_true", help="indisponível até validar isolamento e medição")
    ap.add_argument("--save-raw", action="store_true", help="salva stdout/stderr em diretório privado; pode conter código e prompts")
    ap.add_argument("--yes-spend", action="store_true", help="executa os agentes de verdade (custa dinheiro)")
    ap.add_argument("--report")
    ap.add_argument("--report-md", default="bench/results/agent-benchmark.md")
    args = ap.parse_args()
    if args.report:
        report(args.report, args.report_md)
        return
    if args.real_hooks:
        ap.error("--real-hooks indisponível: isolamento e medição ainda não validados")
    if not args.manifest:
        ap.error("--manifest é obrigatório")
    try:
        repos, cases = load_cases(args.manifest, args.categories, args.per_category)
        if not cases:
            raise ValueError("nenhum caso selecionado")
        validate_repos(repos, cases)
    except (ValueError, KeyError) as exc:
        sys.exit(str(exc))
    specs = [a.split(":", 1) for a in args.agents]
    try:
        for a, m in specs:
            validate_model(a, m)
    except ValueError as exc:
        ap.error(str(exc))
    for a, _ in specs:
        if not shutil.which(a):
            sys.exit(f"cliente '{a}' não encontrado no PATH")
    total = len(cases) * len(specs) * 2 * args.reps
    print(f"plano: {len(cases)} casos em {len({c['repo'] for c in cases})} repositórios × {len(specs)} agentes × 2 braços × {args.reps} rep = {total} execuções; engine {args.engine}")
    for c in cases[:3]:
        print(f"  ex.: {c['id']} [{c['category']}] {c['prompt'][:80]!r}")
    if args.reps < 1 or args.timeout < 1 or args.max_budget <= 0 or args.total_budget <= 0:
        ap.error("reps, timeout e orçamentos devem ser positivos")
    reserved = len(cases) * 2 * args.reps * sum(a == "claude" for a, _ in specs) * args.max_budget
    print(f"teto reservado Claude: US$ {reserved:.2f}; custo dos outros clientes pode ser indisponível")
    if reserved > args.total_budget:
        ap.error("teto reservado para Claude excede --total-budget")
    if not args.yes_spend:
        print("dry run: nada executado. Use --yes-spend para rodar os agentes (gasta tokens/dinheiro).")
        return
    if any(a == "agy" for a, _ in specs):
        ap.error("Antigravity ainda não tem isolamento de configuração/hook qualificado para piloto")
    out = pathlib.Path(args.out_dir)
    out.mkdir(parents=True, exist_ok=True)
    runs = out / "runs.jsonl"
    if runs.exists():
        sys.exit(f"{runs}: já existe; use outro --out-dir para evitar misturar rodadas")
    jobs = schedule(cases, specs, args.reps)
    versions = {}
    for agent, _ in specs:
        proc = subprocess.run([agent, "--version"], capture_output=True, text=True, timeout=20)
        versions[agent] = proc.stdout.strip() if proc.returncode == 0 else None
    metadata = {"manifest_sha256": file_sha256(args.manifest), "sidecar_sha256": file_sha256(args.sidecar),
                "runner_sha256": file_sha256(__file__),
                "clients": versions, "models": args.agents, "engine": args.engine, "reps": args.reps, "timeout_s": args.timeout,
                "max_budget_usd_claude": args.max_budget, "total_budget_usd": args.total_budget,
                "claude_reserved_usd": reserved, "seed": 11, "raw_saved": args.save_raw,
                "repos": {name: repos[name]["commit"] for name in sorted({c["repo"] for c in cases})},
                "jobs": [{"case": c["id"], "agent": a, "model": m, "arm": arm, "rep": rep}
                         for c, a, m, arm, rep in jobs]}
    with (out / "run-metadata.json").open("x") as f:
        json.dump(metadata, f, ensure_ascii=False, indent=2)
    with tempfile.TemporaryDirectory(prefix="sidecar-agent-home-") as home:
        for name in sorted({c["repo"] for c in cases}):
            subprocess.run([args.sidecar, "index", "--root", repos[name]["path"]],
                           env=dict(os.environ, SIDECAR_HOME=home, SIDECAR_ENGINE=args.engine), capture_output=True, check=True)
        aborted = False
        for i, (c, a, m, arm, rep) in enumerate(jobs, 1):
            print(f"[{i}/{len(jobs)}] {a}:{m} {arm} {c['id']} rep{rep}", flush=True)
            rec = run_one(args, repos, c, a, m, arm, rep, home)
            with runs.open("a") as f:
                f.write(json.dumps(rec, ensure_ascii=False) + "\n")
            if rec["exit"] != 0 or rec["timed_out"] or rec["malformed"] or rec["edited_files"]:
                print("rodada interrompida: falha, timeout, resposta malformada ou checkout alterado", file=sys.stderr)
                aborted = True
                break
        report(str(runs), str(out / "agent-benchmark.md"))
        if aborted:
            sys.exit(1)



if __name__ == "__main__":
    main()
