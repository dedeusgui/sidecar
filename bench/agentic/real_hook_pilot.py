#!/usr/bin/env python3
"""Piloto E2E com Claude Code real e hook UserPromptSubmit real (sem prefixo de prompt).

Diferença para bench/agent/run_agents.py: o braço ON registra o hook por --settings, o cliente o dispara
e o contexto chega ao modelo como saída do hook. A entrega é verificada no fluxo de eventos
(--include-hook-events): hook_started, hook_response e o campo additionalContext. Nada disso é inferido
do disparo sozinho.

Isolamento por execução: checkout git descartável de um commit fixado (clone --shared), verificação de
árvore limpa antes e depois, SIDECAR_HOME próprio, --setting-sources project,local (configurações globais
e de usuário não carregam), sem MCP, sem slash commands, Edit/Write/NotebookEdit bloqueados, modo plan.
O oráculo é o gold de arquivo do caso, que nunca entra no checkout.

Limite conhecido: não há prova de que plugins/skills globais estejam fora do processo (apenas que
configurações globais não entram). Os resultados são um piloto de instrumento, não uma comparação.

Gasta dinheiro real. Sem --yes-spend só planeja. Use IDs de modelo explícitos (ex.: claude-haiku-5-5).

Exemplo:
  python3 bench/agentic/real_hook_pilot.py --repo . --commit HEAD --sidecar /tmp/sidecar \\
      --cases bench/agentic/real_hook_pilot_cases.json --reps 2 --yes-spend --out-dir /tmp/pilot
"""
import argparse
import hashlib
import json
import math
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import time
import uuid

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent / "agent"))
import run_agents as ra  # noqa: E402  (reutiliza checkout, julgamento, relatório e validação de modelo)

CLAUDE_FLAGS = ["--output-format", "stream-json", "--verbose", "--include-hook-events",
                "--no-session-persistence", "--permission-mode", "plan",
                "--setting-sources", "project,local", "--strict-mcp-config", "--disable-slash-commands",
                "--disallowedTools", "Edit", "Write", "NotebookEdit"]


def hook_settings_json(sidecar):
    """Registra o hook real do Sidecar para UserPromptSubmit, só nesta execução."""
    cmd = f"{json.dumps(str(sidecar))} hook --agent claude" if " " in str(sidecar) else f"{sidecar} hook --agent claude"
    return json.dumps({"hooks": {"UserPromptSubmit": [{"hooks": [{"type": "command", "command": cmd}]}]}})


def build_claude_cmd(model, prompt, max_budget, settings=None):
    cmd = ["claude", "-p", prompt, "--model", model, *CLAUDE_FLAGS, "--max-budget-usd", str(max_budget)]
    if settings is not None:
        cmd += ["--settings", settings]
    return cmd


BUILTIN_PLUGINS = {"cc-plugin-agents-md", "cc-plugin-telemetry", "cc-plugin-privacy", "cc-plugin-plugin-authoring"}


def parse_stream(text):
    """Lê o stream-json: contexto entregue pelo hook, respostas, ferramentas, custo, modelo servido e o
    que foi carregado no processo (init), para detectar contaminação de configurações globais."""
    out = {"hook_started": False, "hook_delivered": False, "hook_context": "", "hook_names": [],
           "result": None, "is_error": None, "subtype": None, "cost_usd": None, "tokens_in": None,
           "tokens_out": None, "input_uncached": None, "cache_read": None, "cache_create": None, "output": None,
           "tool_result_tokens": None, "tool_calls": 0, "turns": None, "served_models": None, "init": None,
           "saw_result": False}
    texts = []
    for line in text.splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            d = json.loads(line)
        except ValueError:
            continue
        t, st = d.get("type"), d.get("subtype")
        if t == "system" and st == "init" and out["init"] is None:  # the stream has one init, at the start
            out["init"] = {
                "plugins": sorted({p.get("name", "?") for p in (d.get("plugins") or []) if isinstance(p, dict)
                                   and p.get("source", "").split("@")[-1] != "builtin"}),
                "plugin_names": sorted({p.get("name", "?") for p in (d.get("plugins") or []) if isinstance(p, dict)}),
                "mcp_servers": d.get("mcp_servers") or [],
                "skills": len(d.get("skills") or []),
            }
        if t == "system" and st in ("hook_started", "hook_response"):
            name = d.get("hook_name") or d.get("hook_event")
            if name and name not in out["hook_names"]:
                out["hook_names"].append(name)
        if t == "system" and st == "hook_started" and d.get("hook_event") == "UserPromptSubmit":
            out["hook_started"] = True
        if t == "system" and st == "hook_response" and d.get("hook_event") == "UserPromptSubmit":
            try:
                ctx = json.loads(d.get("output") or "")["hookSpecificOutput"]["additionalContext"]
            except (ValueError, KeyError, TypeError):
                ctx = ""
            if isinstance(ctx, str) and ctx:
                out["hook_delivered"] = True
                out["hook_context"] = ctx
        if t == "assistant":
            for block in (d.get("message") or {}).get("content") or []:
                if isinstance(block, dict) and block.get("type") == "tool_use":
                    out["tool_calls"] += 1
                if isinstance(block, dict) and block.get("type") == "text":
                    texts.append(block.get("text", ""))
        if t == "result":
            out["saw_result"] = True
            out["result"] = d.get("result") if isinstance(d.get("result"), str) else None
            out["is_error"] = d.get("is_error")
            out["subtype"] = st or d.get("subtype")
            out["cost_usd"] = _nonnegative_number(d.get("total_cost_usd"))
            out["turns"] = d.get("num_turns")
            usage = d.get("usage") or {}
            if isinstance(usage, dict):
                out["input_uncached"] = _token_count(usage, "input_tokens")
                out["cache_read"] = _token_count(usage, "cache_read_input_tokens")
                out["cache_create"] = _token_count(usage, "cache_creation_input_tokens")
                out["output"] = _token_count(usage, "output_tokens")
            input_counts = (out["input_uncached"], out["cache_read"], out["cache_create"])
            out["tokens_in"] = sum(input_counts) if all(v is not None for v in input_counts) else None
            out["tokens_out"] = out["output"]
            models = d.get("modelUsage")
            if isinstance(models, dict) and models:
                out["served_models"] = sorted(models)
    if out["result"] is None and texts:
        out["result"] = texts[-1]
    if not out["saw_result"]:
        out["tool_calls"] = None
    return out


def _token_count(usage, key):
    value = usage.get(key)
    return value if isinstance(value, int) and not isinstance(value, bool) and value >= 0 else None


def _nonnegative_number(value):
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value) or value < 0:
        return None
    return value


def contamination(parsed, canary_fired, requested_model):
    """Motivos pelos quais a execução não é comparável. Lista vazia = comparável."""
    reasons = []
    init = parsed.get("init")
    if init is None:
        reasons.append("stream sem evento init: não é possível verificar o que foi carregado")
    else:
        if init["plugins"]:
            reasons.append("plugins não embutidos carregados: " + ", ".join(init["plugins"]))
        if init["mcp_servers"]:
            reasons.append("servidores MCP carregados")
    if canary_fired:
        reasons.append("canário de configuração global disparou")
    served = parsed.get("served_models")
    if not parsed.get("saw_result"):
        reasons.append("stream incompleto: sem evento result")
    elif not served:
        reasons.append("modelo servido ausente em modelUsage")
    elif served != [requested_model]:
        reasons.append(f"modelo servido {served} difere do solicitado {requested_model}")
    return reasons


def clean(path):
    p = subprocess.run(["git", "-C", str(path), "status", "--porcelain", "--untracked-files=all"],
                       capture_output=True, text=True, check=True)
    return p.stdout.strip() == ""


def make_config_dir(credentials, root):
    """Diretório de configuração isolado: só as credenciais e um canário que dispara se for lido."""
    cfg = tempfile.mkdtemp(prefix="hookpilot-cfg-", dir=root)
    if credentials:
        shutil.copy(credentials, os.path.join(cfg, ".credentials.json"))
        os.chmod(os.path.join(cfg, ".credentials.json"), 0o600)
    marker = os.path.join(cfg, "canary-fired.txt")
    canary = {"hooks": {"UserPromptSubmit": [{"hooks": [{"type": "command", "command":
        f"sh -c 'echo fired >> {marker}'"}]}]}}
    with open(os.path.join(cfg, "settings.json"), "w") as f:
        json.dump(canary, f)
    return cfg, marker


def run_one(args, case, arm, rep, repo_path, commit, model, cfg_root):
    work = tempfile.mkdtemp(prefix="hookpilot-")
    home = tempfile.mkdtemp(prefix="hookpilot-home-")
    cfg = None
    try:
        dest = os.path.join(work, "checkout")
        ra.checkout({"path": repo_path, "commit": commit}, dest)
        if not clean(dest):
            raise RuntimeError("checkout descartável não está limpo")
        if os.path.exists(os.path.join(dest, ".claude")):
            raise RuntimeError("o checkout contém .claude/: configuração de projeto não controlada")
        cfg, marker = make_config_dir(args.credentials, cfg_root)  # uma configuração nova por execução
        env = dict(os.environ, SIDECAR_HOME=home, SIDECAR_ENGINE=args.engine, CLAUDE_CONFIG_DIR=cfg)
        for key in ("ORCA_AGENT_HOOK_PORT", "ORCA_AGENT_HOOK_TOKEN", "ORCA_PANE_KEY"):
            env.pop(key, None)
        settings = None
        hook_probe_ms = None
        if arm == "ON":
            subprocess.run([args.sidecar, "index", "--root", dest], env=env, capture_output=True, check=True, timeout=300)
            settings = hook_settings_json(args.sidecar)
        cmd = build_claude_cmd(model, case["prompt"] + ra.SUFFIX, args.max_budget, settings)
        t0 = time.perf_counter()
        timed_out = False
        try:
            p = subprocess.run(cmd, cwd=dest, capture_output=True, text=True, timeout=args.timeout,
                               stdin=subprocess.DEVNULL, env=env)
            out, err, code = p.stdout, p.stderr, p.returncode
        except subprocess.TimeoutExpired as e:
            out = e.stdout.decode(errors="replace") if isinstance(e.stdout, bytes) else (e.stdout or "")
            err = e.stderr.decode(errors="replace") if isinstance(e.stderr, bytes) else (e.stderr or "")
            code, timed_out = -1, True
        wall = time.perf_counter() - t0
        parsed = parse_stream(out)
        if arm == "ON":
            hook_probe_ms = probe_hook_ms(args.sidecar, home, dest, case["prompt"])
        files = ra.answer_files(parsed["result"] or "")
        edited = not clean(dest)
        canary_fired = os.path.exists(marker)
        reasons = contamination(parsed, canary_fired, model)
        ctx = parsed["hook_context"]
        rec = {
            "agent": "claude", "model": model, "arm": arm, "rep": rep, "case": case["id"],
            "category": case["category"], "repo": case.get("repo", "sidecar"), "engine": args.engine,
            "transport": "real_hook" if arm == "ON" else "none",
            "hook_fired": parsed["hook_started"] if arm == "ON" else None,
            "hook_delivered": parsed["hook_delivered"] if arm == "ON" else None,
            "hook_names": parsed["hook_names"], "hook_probe_ms": hook_probe_ms,
            "wall_s": round(wall, 2), "sidecar_ms": None, "injected_chars": len(ctx),
            "exit": code, "timed_out": timed_out, "files": files, "edited_files": edited,
            "tokens_in": parsed["tokens_in"], "tokens_out": parsed["tokens_out"], "cost_usd": parsed["cost_usd"],
            "tool_calls": parsed["tool_calls"], "turns": parsed["turns"], "served_models": parsed["served_models"],
            "init": parsed["init"], "canary_fired": canary_fired,
            "comparable": not reasons, "incomparable_reasons": reasons,
            "stream_error": parsed["is_error"] is True or parsed["subtype"] not in (None, "success"),
            "input_uncached": parsed["input_uncached"], "cache_read": parsed["cache_read"],
            "cache_create": parsed["cache_create"], "output": parsed["output"],
            "tool_result_tokens": parsed["tool_result_tokens"], "resident_context_tokens": None,
            **ra.judge(case, files), **ra.attention(case, files, ctx, dest),
        }
        if arm == "OFF":
            # Sem hook não há contexto: as coincidências com o contexto não se aplicam a este braço.
            rec["gold_in_ctx"] = rec["gold_ignored"] = rec["followed_noise"] = None
        # Ausência de contexto é comportamento correto quando não há evidência (sem evidência = sem injeção):
        # não é falha do agente, por isso hook_delivered não entra no critério de sucesso.
        if code != 0 or timed_out or edited or rec["stream_error"]:
            rec["success"] = False
            for metric in ("hit_at_1", "hit_at_5"):
                if rec[metric] is not None:
                    rec[metric] = False
        if err.strip():
            rec["stderr_tail_chars"] = len(err)
        return rec
    finally:
        shutil.rmtree(work, ignore_errors=True)
        shutil.rmtree(home, ignore_errors=True)
        if cfg:
            shutil.rmtree(cfg, ignore_errors=True)


def probe_hook_ms(sidecar, home, workdir, prompt, repeats=3):
    """Tempo do binário do hook sobre o mesmo payload, medido FORA do loop do agente (mediana)."""
    payload = json.dumps({"hook_event_name": "UserPromptSubmit", "prompt": prompt, "cwd": workdir,
                          "session_id": uuid.uuid4().hex})
    times = []
    for _ in range(repeats):
        t0 = time.perf_counter()
        p = subprocess.run([sidecar, "hook", "--agent", "claude"], input=payload, capture_output=True, text=True,
                           env=dict(os.environ, SIDECAR_HOME=home), timeout=60)
        if p.returncode != 0:
            return None
        times.append((time.perf_counter() - t0) * 1000)
    return round(sorted(times)[len(times) // 2], 1)


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--repo", required=True, help="repositório git local (origem somente leitura)")
    ap.add_argument("--commit", default="HEAD")
    ap.add_argument("--cases", required=True)
    ap.add_argument("--sidecar", required=True)
    ap.add_argument("--engine", choices=("v1", "v2"), default="v2")
    ap.add_argument("--model", default="claude-haiku-5-5")
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--timeout", type=int, default=300)
    ap.add_argument("--max-budget", type=float, default=0.25, help="teto por execução (US$)")
    ap.add_argument("--total-budget", type=float, default=1.0, help="teto total reservado (US$)")
    ap.add_argument("--out-dir", required=True)
    ap.add_argument("--yes-spend", action="store_true")
    ap.add_argument("--credentials", default=os.path.expanduser("~/.claude/.credentials.json"),
                    help="credenciais copiadas para a configuração isolada de cada execução")
    args = ap.parse_args()
    try:
        ra.validate_model("claude", args.model)
    except ValueError as exc:
        ap.error(str(exc))
    if args.reps < 1 or args.max_budget <= 0 or args.total_budget <= 0:
        ap.error("reps e orçamentos devem ser positivos")
    if not shutil.which("claude"):
        sys.exit("claude não encontrado no PATH")
    cases = json.loads(pathlib.Path(args.cases).read_text())
    args.repo = os.path.abspath(args.repo)
    commit = subprocess.run(["git", "-C", args.repo, "rev-parse", "--verify", args.commit + "^{commit}"],
                            check=True, capture_output=True, text=True).stdout.strip()
    if not clean(args.repo):
        sys.exit("repositório de origem tem alterações; use um checkout limpo")
    # Pares adjacentes com ordem AB/BA alternada, para que o efeito de posição não recaia sempre sobre um braço.
    jobs = []
    for k, (c, rep) in enumerate((c, rep) for c in cases for rep in range(args.reps)):
        arms = ("OFF", "ON") if k % 2 == 0 else ("ON", "OFF")
        jobs += [(c, arm, rep) for arm in arms]
    reserved = len(jobs) * args.max_budget
    print(f"plano: {len(cases)} casos × 2 braços × {args.reps} rep = {len(jobs)} execuções; "
          f"modelo {args.model}; teto reservado US$ {reserved:.2f} (limite total US$ {args.total_budget:.2f})")
    if reserved > args.total_budget:
        ap.error("teto reservado excede --total-budget")
    if not args.yes_spend:
        print("dry run: nada executado. Use --yes-spend para gastar dinheiro real.")
        return
    out = pathlib.Path(args.out_dir)
    out.mkdir(parents=True, exist_ok=True)
    runs = out / "runs.jsonl"
    if runs.exists():
        sys.exit(f"{runs} já existe; use outro --out-dir")
    version = subprocess.run(["claude", "--version"], capture_output=True, text=True, timeout=30).stdout.strip()
    meta = {"repo_commit": commit, "claude_version": version, "model": args.model, "engine": args.engine,
            "sidecar_sha256": sha256(args.sidecar), "pilot_sha256": sha256(__file__),
            "runner_sha256": sha256(ra.__file__), "cases_sha256": sha256(args.cases), "reps": args.reps,
            "max_budget_usd_per_run": args.max_budget, "reserved_usd": reserved,
            "isolation": ["clone --shared descartável por execução", "SIDECAR_HOME por execução",
                          "--setting-sources project,local", "--strict-mcp-config", "--disable-slash-commands",
                          "Edit/Write/NotebookEdit bloqueados, modo plan"],
            "jobs": [{"case": c["id"], "arm": a, "rep": r} for c, a, r in jobs]}
    with (out / "run-metadata.json").open("x") as f:
        json.dump(meta, f, ensure_ascii=False, indent=2)
    spent = 0.0
    cfg_root = tempfile.mkdtemp(prefix="hookpilot-cfgroot-")
    for i, (case, arm, rep) in enumerate(jobs, 1):
        print(f"[{i}/{len(jobs)}] {arm} {case['id']} rep{rep}", flush=True)
        rec = run_one(args, case, arm, rep, args.repo, commit, args.model, cfg_root)
        spent += rec["cost_usd"] or 0.0
        with runs.open("a") as f:
            f.write(json.dumps(rec, ensure_ascii=False) + "\n")
        if not rec["comparable"]:
            print("execução não comparável (registrada, fora das médias): " + "; ".join(rec["incomparable_reasons"]), file=sys.stderr)
        if rec["exit"] != 0 or rec["timed_out"] or rec["edited_files"]:
            print("rodada interrompida: falha, timeout ou checkout alterado", file=sys.stderr)
            break
        if spent > args.total_budget:
            print(f"interrompido: custo medido US$ {spent:.4f} excede o teto", file=sys.stderr)
            break
    shutil.rmtree(cfg_root, ignore_errors=True)
    print(f"custo medido: US$ {spent:.4f}")
    ra.report(str(runs), str(out / "agent-benchmark.md"))


if __name__ == "__main__":
    main()
