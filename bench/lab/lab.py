#!/usr/bin/env python3
"""Sidecar Evaluation Lab: harness de tarefas de edição real com oráculos independentes (R1).

Contrato (ver docs/eval-lab.md):
- Corpus: bench/lab/corpus/tasks/<ID>/task.json + oracle/ (arquivos ocultos) + reference.patch.
- Cada execução recebe um checkout descartável do commit fixado (git archive + um único commit-base local,
  sem histórico/refs/remotes do upstream) e uma CLAUDE_CONFIG_DIR nova. O oráculo NUNCA entra no checkout
  durante a execução do agente; é copiado depois, executado sem rede e removido.
- Split `holdout` fica selado: só roda com --final, e todo acesso é gravado em corpus/holdout_ledger.jsonl.
- Gasta dinheiro só com --yes-spend, com teto total (--max-total-usd) e registro em spend.jsonl.
- `scripted` é um agente SIMULADO para testar a infraestrutura; suas execuções recebem evidence=simulated
  e nunca entram em relatórios de resultado agentic.

Stdlib apenas. Subcomandos: validate-task, validate-corpus, manifest, run, report.
"""
import argparse
import fnmatch
import hashlib
import json
import math
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

HERE = pathlib.Path(__file__).resolve().parent
CORPUS = HERE / "corpus"
TASKS = CORPUS / "tasks"
REPOS_JSON = CORPUS / "repos.json"
LEDGER = CORPUS / "holdout_ledger.jsonl"
SCHEMA = "lab-task/1"
RUN_SCHEMA = "lab-run/1"
SPLITS = ("development", "validation", "holdout")
CLASSES = ("locate", "debug", "edit", "multi-file", "contract-change", "negative", "ambiguous", "voice")
ORACLE_KINDS = ("tests", "answer", "no_change")
BASE_IDENT = ["-c", "user.name=lab", "-c", "user.email=lab@invalid", "-c", "commit.gpgsign=false"]


class LabError(Exception):
    pass


# --------------------------------------------------------------------------- helpers

def sha256_bytes(b):
    return hashlib.sha256(b).hexdigest()


def sha256_file(p):
    return sha256_bytes(pathlib.Path(p).read_bytes())


def tree_sha256(root):
    """Hash estável de uma pasta (nomes + conteúdos), para selar oráculos e reference patches."""
    h = hashlib.sha256()
    root = pathlib.Path(root)
    for p in sorted(root.rglob("*")):
        if p.is_file():
            h.update(p.relative_to(root).as_posix().encode() + b"\0" + p.read_bytes() + b"\0")
    return h.hexdigest()


def run(cmd, cwd=None, env=None, timeout=300, input_text=None):
    """Executa sem shell; mata o grupo inteiro no timeout. Retorna (code, stdout, stderr, timed_out)."""
    import signal
    proc = subprocess.Popen(cmd, cwd=cwd, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                            stdin=subprocess.PIPE if input_text is not None else subprocess.DEVNULL,
                            start_new_session=True)
    try:
        out, err = proc.communicate(input_text, timeout=timeout)
        return proc.returncode, out, err, False
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)
        out, err = proc.communicate()
        return -1, out or "", err or "", True


def load_json(p):
    try:
        return json.loads(pathlib.Path(p).read_text())
    except (OSError, ValueError) as e:
        raise LabError(f"{p}: {e}")


# --------------------------------------------------------------------------- corpus

def load_repos():
    return load_json(REPOS_JSON)["repos"]


def task_dirs():
    return sorted(p for p in TASKS.iterdir() if (p / "task.json").is_file()) if TASKS.is_dir() else []


def validate_task_schema(task, tdir):
    """Erros de esquema. Lista vazia = válido."""
    errs = []
    req = ("schema", "id", "split", "class", "repo", "commit", "prompt", "oracle", "scope")
    for k in req:
        if k not in task:
            errs.append(f"campo ausente: {k}")
    if errs:
        return errs
    if task["schema"] != SCHEMA:
        errs.append(f"schema {task['schema']!r} != {SCHEMA}")
    if task["id"] != tdir.name:
        errs.append("id difere do nome da pasta")
    if task["split"] not in SPLITS:
        errs.append(f"split inválido: {task['split']}")
    if task["class"] not in CLASSES:
        errs.append(f"class inválida: {task['class']}")
    repos = load_repos()
    if task["repo"] not in repos:
        errs.append(f"repo desconhecido: {task['repo']}")
    elif repos[task["repo"]]["commit"] != task["commit"]:
        errs.append("commit difere do pin em repos.json")
    if not isinstance(task["prompt"], str) or len(task["prompt"].strip()) < 8:
        errs.append("prompt vazio")
    o = task["oracle"]
    if o.get("kind") not in ORACLE_KINDS:
        errs.append(f"oracle.kind inválido: {o.get('kind')}")
    if o.get("kind") == "tests":
        if not (isinstance(o.get("command"), list) and o["command"] and all(isinstance(a, str) for a in o["command"])):
            errs.append("oracle.command deve ser lista argv")
        if not o.get("files"):
            errs.append("oracle.files vazio (teste oculto necessário)")
        for dest, src in (o.get("files") or {}).items():
            if dest.startswith("/") or ".." in pathlib.PurePosixPath(dest).parts:
                errs.append(f"destino inseguro do oráculo: {dest}")
            if not (tdir / "oracle" / src).is_file():
                errs.append(f"arquivo do oráculo ausente: oracle/{src}")
        if not o.get("require_output"):
            errs.append("oracle.require_output: padrões que provam que o teste rodou (anti-vacuidade)")
    if o.get("kind") == "answer" and not o.get("answer_must_mention"):
        errs.append("oracle.answer_must_mention vazio")
    sc = task["scope"]
    if not isinstance(sc.get("allowed_paths"), list):
        errs.append("scope.allowed_paths deve ser lista (vazia = nenhuma edição)")
    if task["class"] in ("negative", "locate", "ambiguous", "voice") and o.get("kind") == "tests":
        pass  # voice/ambiguous podem exigir edição; não restringimos
    if task["class"] != "negative" and o.get("kind") != "no_change" and task["class"] not in ("locate",):
        if o.get("kind") == "tests" and not (tdir / "reference.patch").is_file():
            errs.append("reference.patch ausente (prova que o oráculo é satisfazível)")
    if task["class"] == "ambiguous" and not isinstance(task.get("underspec"), dict):
        errs.append("tarefa ambiguous exige 'underspec' {decision, options, canned_answer}")
    return errs


def load_task(tid):
    tdir = TASKS / tid
    task = load_json(tdir / "task.json")
    errs = validate_task_schema(task, tdir)
    if errs:
        raise LabError(f"{tid}: " + "; ".join(errs))
    return task, tdir


def build_manifest():
    repos = load_repos()
    tasks = []
    for tdir in task_dirs():
        task = load_json(tdir / "task.json")
        errs = validate_task_schema(task, tdir)
        if errs:
            raise LabError(f"{tdir.name}: " + "; ".join(errs))
        val = tdir / "validation.json"
        tasks.append({
            "id": task["id"], "split": task["split"], "class": task["class"], "repo": task["repo"],
            "task_sha256": sha256_file(tdir / "task.json"),
            "oracle_sha256": tree_sha256(tdir / "oracle") if (tdir / "oracle").is_dir() else None,
            "reference_sha256": sha256_file(tdir / "reference.patch") if (tdir / "reference.patch").is_file() else None,
            "validated": bool(val.is_file() and load_json(val).get("ok")),
        })
    by_split = {s: [t for t in tasks if t["split"] == s] for s in SPLITS}
    holdout_prompts = sorted(load_json(TASKS / t["id"] / "task.json")["prompt"] for t in by_split["holdout"])
    return {
        "schema": "lab-manifest/1", "repos": repos, "tasks": tasks,
        "counts": {s: len(v) for s, v in by_split.items()},
        "classes": {c: sum(1 for t in tasks if t["class"] == c) for c in CLASSES},
        "holdout_prompts_sha256": sha256_bytes("\n".join(holdout_prompts).encode()),
        "note": "holdout selado: use --final; acessos ficam em holdout_ledger.jsonl",
    }


def holdout_guard(tasks, final, purpose):
    """Bloqueia holdout fora de --final e registra qualquer uso no ledger (append-only)."""
    held = [t for t in tasks if t["split"] == "holdout"]
    if not held:
        return
    if not final:
        raise LabError("holdout selado: use --final (uma única avaliação final, sem tuning depois)")
    with LEDGER.open("a") as f:
        f.write(json.dumps({"ts": time.time(), "purpose": purpose, "tasks": [t["id"] for t in held]}) + "\n")


def holdout_burned():
    return LEDGER.is_file() and bool(LEDGER.read_text().strip())


# --------------------------------------------------------------------------- sandbox / checkout

def cache_dir():
    return pathlib.Path(os.environ.get("SIDECAR_LAB_CACHE", pathlib.Path.home() / ".cache" / "sidecar-lab"))


def ensure_repo_cache(name):
    """Clone nu cacheado com o commit fixado buscado. Rede só aqui (fase de preparação)."""
    repo = load_repos()[name]
    dest = cache_dir() / "repos" / name
    if not dest.exists():
        dest.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(["git", "init", "-q", "--bare", str(dest)], check=True)
        subprocess.run(["git", "-C", str(dest), "remote", "add", "origin", repo["source"]], check=True)
    have = subprocess.run(["git", "-C", str(dest), "cat-file", "-e", repo["commit"] + "^{commit}"],
                          capture_output=True).returncode == 0
    if not have:
        subprocess.run(["git", "-C", str(dest), "fetch", "-q", "--depth", "1", "origin", repo["commit"]], check=True)
    return dest


def materialize(task, dest):
    """Árvore exata do commit fixado, com um único commit-base local. Sem remotes, refs ou histórico upstream."""
    dest = pathlib.Path(dest)
    dest.mkdir(parents=True)
    bare = ensure_repo_cache(task["repo"])
    tarball = dest.parent / (dest.name + ".tar")
    with tarball.open("wb") as out:
        subprocess.run(["git", "-C", str(bare), "archive", "--format=tar", task["commit"]], stdout=out, check=True)
    import tarfile
    with tarfile.open(tarball) as t:
        t.extractall(dest, filter="data")
    tarball.unlink()
    for cmd in (["git", "init", "-q"], ["git", "add", "-A", "-f"],
                ["git", *BASE_IDENT, "commit", "-q", "-m", "base", "--allow-empty"]):
        subprocess.run(cmd, cwd=dest, check=True, capture_output=True)
    if (dest / ".claude").exists():
        raise LabError("o checkout contém .claude/: configuração de projeto não controlada")
    return dest


def status_paths(work):
    out = subprocess.run(["git", "status", "--porcelain", "--untracked-files=all", "--ignored"],
                         cwd=work, capture_output=True, text=True, check=True).stdout
    paths = set()
    for line in out.splitlines():
        p = line[3:].strip().strip('"')
        if " -> " in p:
            p = p.split(" -> ")[1]
        if p:
            paths.add(p)
    return paths


def changed_paths(work):
    """Caminhos alterados contra o commit-base, incluindo untracked e ignorados.
    Artefatos criados pela fase `setup` (dependências) são a linha de base e não contam como alteração."""
    base_file = pathlib.Path(work) / ".git" / "lab_setup_paths.json"
    setup = set(json.loads(base_file.read_text())) if base_file.is_file() else set()
    return sorted(p for p in status_paths(work) if not any(p == s or p.startswith(s) for s in setup))


def prepare_workspace(task, dest):
    """materialize + fase `setup` (única com rede; instala dependências) + linha de base de artefatos."""
    work = materialize(task, dest)
    cmds = task.get("setup") or []
    if cmds:
        # A fase de setup usa rede: repassa só o necessário para proxy/CA, nunca credenciais de modelo.
        keep = ("PATH", "HOME", "LANG", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy",
                "SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE", "GIT_SSL_CAINFO")
        env = {k: os.environ[k] for k in keep if k in os.environ}
        env.update(task.get("setup_env") or {})
        for argv in cmds:
            code, out, err, to = run(argv, cwd=work, env=env, timeout=task.get("setup_timeout_s", 900))
            if code != 0:
                raise LabError(f"setup falhou ({' '.join(argv)}): {(err or out)[-300:]}")
    entries = set()
    for p in status_paths(work):
        top = p.split("/")[0]
        tracked = subprocess.run(["git", "ls-files", "--", top], cwd=work, capture_output=True, text=True).stdout.strip()
        entries.add(top + "/" if "/" in p and not tracked else p)  # pasta nova do setup (node_modules, .venv...) inteira
    (work / ".git" / "lab_setup_paths.json").write_text(json.dumps(sorted(entries)))
    return work


def sandbox_prefix():
    """Prefixo de isolamento de rede para o oráculo. Retorna (argv_prefix, nome_do_isolamento)."""
    if shutil.which("bwrap"):
        return ["bwrap", "--unshare-net", "--unshare-pid", "--die-with-parent", "--dev-bind", "/", "/"], "bwrap-net"
    if shutil.which("unshare") and subprocess.run(["unshare", "-rn", "true"], capture_output=True).returncode == 0:
        return ["unshare", "-rn"], "unshare-net"
    return [], "none"


def oracle_env(task):
    env = {k: os.environ[k] for k in ("PATH", "HOME", "LANG") if k in os.environ}
    env.update({"GOFLAGS": "-mod=mod", "GOPROXY": "off", "GOTOOLCHAIN": "local", "CGO_ENABLED": "0",
                "GOCACHE": str(cache_dir() / "gocache"), "NPM_CONFIG_OFFLINE": "true", "CI": "1"})
    env.update(task["oracle"].get("env") or {})
    return env


# --------------------------------------------------------------------------- oracle

def in_scope(path, allowed):
    return any(fnmatch.fnmatch(path, g) or path == g.rstrip("/") or path.startswith(g.rstrip("/") + "/")
               for g in allowed)


def check_invariants(task, work):
    results = []
    for inv in task["scope"].get("invariants") or []:
        kind, path = inv["type"], pathlib.Path(work) / inv["path"]
        ok, detail = False, ""
        try:
            text = path.read_text(errors="replace") if path.exists() else None
            if kind == "file_unchanged":
                ok = subprocess.run(["git", "diff", "--quiet", "--", inv["path"]], cwd=work).returncode == 0 and path.exists()
            elif kind == "regex_present":
                ok = text is not None and re.search(inv["pattern"], text, re.M) is not None
            elif kind == "regex_absent":
                ok = text is None or re.search(inv["pattern"], text, re.M) is None
            else:
                detail = f"tipo desconhecido {kind}"
        except re.error as e:
            detail = f"regex inválida: {e}"
        results.append({"type": kind, "path": inv["path"], "ok": ok, "detail": detail or inv.get("why", "")})
    return results


def run_oracle(task, tdir, work, answer_text=""):
    """Executa o oráculo oculto DEPOIS da execução do agente. Retorna veredito estruturado."""
    work = pathlib.Path(work)
    o, sc = task["oracle"], task["scope"]
    changed_before_oracle = changed_paths(work)
    outside = [p for p in changed_before_oracle if not in_scope(p, sc["allowed_paths"])]
    res = {"kind": o["kind"], "changed": changed_before_oracle, "outside_scope": outside,
           "scope_ok": not outside, "tests": None, "invariants": [], "isolation": None, "detail": ""}
    res["invariants"] = check_invariants(task, work)
    if o["kind"] == "no_change":
        res["tests"] = "pass" if not changed_before_oracle else "fail"
    elif o["kind"] == "answer":
        text = (answer_text or "").lower()
        missing = [m for m in o["answer_must_mention"] if m.lower() not in text]
        res["tests"] = "pass" if not missing else "fail"
        res["detail"] = ("faltando: " + ", ".join(missing)) if missing else ""
        if o.get("must_not_edit", True) and changed_before_oracle:
            res["tests"] = "fail"
            res["detail"] += " edição indevida"
    else:
        placed = []
        try:
            for dest, src in o["files"].items():
                target = work / dest
                if target.exists() or target.is_symlink():
                    res["tests"], res["detail"] = "error", f"destino do oráculo já existe: {dest}"
                    return res
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(tdir / "oracle" / src, target)
                placed.append(target)
            prefix, iso = sandbox_prefix()
            res["isolation"] = iso
            code, out, err, to = run(prefix + o["command"], cwd=work, env=oracle_env(task), timeout=o.get("timeout_s", 300))
            combined = out + err
            if to:
                res["tests"], res["detail"] = "error", "timeout do oráculo"
            elif any(re.search(p, combined) is None for p in o["require_output"]):
                res["tests"], res["detail"] = "error", "oráculo não provou execução (require_output ausente): " + combined[-300:]
            else:
                res["tests"] = "pass" if code == 0 else "fail"
                if code != 0:
                    res["detail"] = combined[-400:]
        finally:
            for t in placed:
                try:
                    t.unlink()
                except OSError:
                    pass
    return res


def verdict(res):
    """pass exige teste, invariantes e escopo. error = infraestrutura, nunca falha do agente."""
    if res["tests"] == "error":
        return "error"
    ok = res["tests"] == "pass" and all(i["ok"] for i in res["invariants"]) and res["scope_ok"]
    return "pass" if ok else "fail"


# --------------------------------------------------------------------------- validate-task

def apply_patch(work, patch):
    code, out, err, _ = run(["git", "apply", "--whitespace=nowarn", str(patch)], cwd=work)
    if code != 0:
        raise LabError(f"reference.patch não aplica: {err[-300:]}")


def validate_task(tid, keep=False):
    """Prova: o oráculo FALHA na base, PASSA com o reference.patch, e o patch respeita o escopo."""
    task, tdir = load_task(tid)
    o = task["oracle"]
    report = {"id": tid, "ok": False, "base": None, "reference": None, "ts": time.time()}
    with tempfile.TemporaryDirectory(prefix="lab-val-") as tmp:
        base = prepare_workspace(task, pathlib.Path(tmp) / "base")
        report["base"] = run_oracle(task, tdir, base, answer_text="")
        base_v = verdict(report["base"])
        ref = tdir / "reference.patch"
        if not ref.is_file():
            # locate/negative/no_change: sem patch. A base deve falhar com resposta vazia (exceto no_change).
            report["reference"] = None
            ok = (base_v == "fail") if o["kind"] == "answer" else (base_v == "pass" and o["kind"] == "no_change")
            report["ok"] = bool(ok)
        else:
            refwork = prepare_workspace(task, pathlib.Path(tmp) / "ref")
            apply_patch(refwork, ref)
            answer = " ".join(o.get("answer_must_mention") or [])
            report["reference"] = run_oracle(task, tdir, refwork, answer_text=answer)
            report["ok"] = base_v == "fail" and verdict(report["reference"]) == "pass"
        report["base_verdict"] = base_v
    for r in ("base", "reference"):
        if report[r]:
            report[r]["detail"] = (report[r].get("detail") or "")[-300:]
    (tdir / "validation.json").write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")
    return report


# --------------------------------------------------------------------------- agents

class ScriptedAgent:
    """Agente SIMULADO para testar a infraestrutura. Nunca é evidência de resultado agentic."""
    simulated = True

    def __init__(self, behaviour):
        self.behaviour = behaviour  # "reference" | "noop" | "scope_violation" | "crash"

    def run(self, task, tdir, work, arm, env, prompt):
        t0 = time.perf_counter()
        text = "feito"
        if self.behaviour == "reference":
            ref = tdir / "reference.patch"
            if ref.is_file():
                apply_patch(work, ref)
            text = " ".join(task["oracle"].get("answer_must_mention") or []) or "feito"
        elif self.behaviour == "scope_violation":
            (pathlib.Path(work) / "STRAY_FILE.txt").write_text("fora do escopo\n")
        elif self.behaviour == "crash":
            raise RuntimeError("agente simulado falhou")
        return {"wall_s": round(time.perf_counter() - t0, 3), "result": text, "cost_usd": 0.0, "tokens_in": 0,
                "tokens_out": 0, "tool_calls": 0, "turns": 1, "served_models": ["scripted"], "exit": 0,
                "timed_out": False, "hook_delivered": None, "injected_chars": 0, "incomparable_reasons": [],
                "stream_error": False, "first_edit_s": None}


class ClaudeAgent:
    """Claude Code real: configuração isolada por execução, canário, verificação de init e modelo servido."""
    simulated = False

    def __init__(self, model, sidecar, credentials, max_budget, timeout, tools_allow):
        sys.path.insert(0, str(HERE.parent / "agentic"))
        import real_hook_pilot as rhp
        self.rhp, self.model, self.sidecar = rhp, model, sidecar
        self.credentials, self.max_budget, self.timeout, self.tools_allow = credentials, max_budget, timeout, tools_allow

    def command(self, prompt, settings):
        flags = ["--output-format", "stream-json", "--verbose", "--include-hook-events", "--no-session-persistence",
                 "--permission-mode", "acceptEdits", "--setting-sources", "project,local", "--strict-mcp-config",
                 "--disable-slash-commands", "--allowedTools", *self.tools_allow]
        cmd = ["claude", "-p", prompt, "--model", self.model, *flags, "--max-budget-usd", str(self.max_budget)]
        return cmd + (["--settings", settings] if settings else [])

    def run(self, task, tdir, work, arm, env, prompt):
        rhp = self.rhp
        cfg, marker = rhp.make_config_dir(self.credentials, tempfile.gettempdir())
        try:
            env = dict(env, CLAUDE_CONFIG_DIR=cfg)
            for k in ("ORCA_AGENT_HOOK_PORT", "ORCA_AGENT_HOOK_TOKEN", "ORCA_PANE_KEY", "ANTHROPIC_API_KEY"):
                env.pop(k, None)
            settings = None
            if arm["engine"] is not None:
                subprocess.run([self.sidecar, "index", "--root", str(work)], env=env, capture_output=True, check=True, timeout=300)
                settings = rhp.hook_settings_json(self.sidecar)
            t0 = time.perf_counter()
            code, out, err, to = run(self.command(prompt, settings), cwd=work, env=env, timeout=self.timeout)
            wall = time.perf_counter() - t0
            parsed = rhp.parse_stream(out)
            reasons = rhp.contamination(parsed, os.path.exists(marker), self.model)
            return {"wall_s": round(wall, 2), "result": parsed["result"] or "", "cost_usd": parsed["cost_usd"],
                    "tokens_in": parsed["tokens_in"], "tokens_out": parsed["tokens_out"],
                    "tool_calls": parsed["tool_calls"], "turns": parsed["turns"], "served_models": parsed["served_models"],
                    "exit": code, "timed_out": to, "hook_delivered": parsed["hook_delivered"] if arm["engine"] else None,
                    "injected_chars": len(parsed["hook_context"]), "incomparable_reasons": reasons,
                    "stream_error": parsed["is_error"] is True or parsed["subtype"] not in (None, "success"),
                    "hook_names": parsed["hook_names"], "first_edit_s": None}
        finally:
            shutil.rmtree(cfg, ignore_errors=True)


# --------------------------------------------------------------------------- runner

def parse_arms(spec):
    """OFF | V1 | V2 | NAME=engine[,ENV=val...]; engine None = sem hook (OFF)."""
    arms = []
    for item in spec.split(","):
        if item == "OFF":
            arms.append({"name": "OFF", "engine": None, "env": {}})
        elif item in ("V1", "V2"):
            arms.append({"name": item, "engine": item.lower(), "env": {}})
        else:
            raise LabError(f"braço desconhecido: {item} (use OFF, V1, V2)")
    return arms


def schedule(task_ids, arms, reps, seed):
    """Ordem balanceada: por tarefa/repetição, os braços entram em rotação (quadrado latino) com embaralho seeded."""
    rnd = random.Random(seed)
    order = []
    for rep in range(reps):
        ids = list(task_ids)
        rnd.shuffle(ids)
        for i, tid in enumerate(ids):
            k = (i + rep) % len(arms)
            for arm in arms[k:] + arms[:k]:
                order.append((tid, arm, rep))
    return order


def spend_total(path):
    if not path.is_file():
        return 0.0
    return sum((json.loads(l).get("cost_usd") or 0.0) for l in path.read_text().splitlines() if l.strip())


def execute_one(agent, task, tdir, arm, rep, args, home):
    with tempfile.TemporaryDirectory(prefix="lab-run-") as tmp:
        work = prepare_workspace(task, pathlib.Path(tmp) / "work")
        if tdir.resolve() in pathlib.Path(work).resolve().parents or str(tdir.resolve()) in str(work):
            raise LabError("oráculo dentro do workspace do agente")
        before = changed_paths(work)
        if before:
            raise LabError("checkout não está limpo")
        env = {k: os.environ[k] for k in ("PATH", "HOME", "LANG") if k in os.environ}
        env.update({"SIDECAR_HOME": home, "GOPROXY": "off", "GOFLAGS": "-mod=mod"})
        if arm["engine"]:
            env["SIDECAR_ENGINE"] = arm["engine"]
        env.update(arm["env"])
        err = None
        try:
            r = agent.run(task, tdir, work, arm, env, task["prompt"])
        except Exception as e:  # falha do agente é registrada, não escondida
            r = {"wall_s": None, "result": "", "cost_usd": None, "tokens_in": None, "tokens_out": None, "tool_calls": None,
                 "turns": None, "served_models": None, "exit": -1, "timed_out": False, "hook_delivered": None,
                 "injected_chars": None, "incomparable_reasons": [], "stream_error": True, "first_edit_s": None}
            err = f"{type(e).__name__}: {e}"
        t0 = time.perf_counter()
        orc = run_oracle(task, tdir, work, answer_text=r["result"])
        v = verdict(orc)
        comparable = not r["incomparable_reasons"] and err is None and not r["timed_out"]
        return {
            "schema": RUN_SCHEMA, "task": task["id"], "split": task["split"], "class": task["class"], "repo": task["repo"],
            "commit": task["commit"], "arm": arm["name"], "rep": rep, "model": getattr(agent, "model", "scripted"),
            "evidence": "simulated" if agent.simulated else "agentic", "verdict": v, "success": v == "pass" and comparable,
            "comparable": comparable, "agent_error": err, "oracle": {k: orc[k] for k in
                ("tests", "scope_ok", "outside_scope", "invariants", "changed", "isolation", "detail")},
            "oracle_s": round(time.perf_counter() - t0, 2), "task_sha256": sha256_file(tdir / "task.json"),
            **{k: r[k] for k in ("wall_s", "cost_usd", "tokens_in", "tokens_out", "tool_calls", "turns",
               "served_models", "exit", "timed_out", "hook_delivered", "injected_chars", "incomparable_reasons",
               "stream_error", "first_edit_s")},
        }


def cmd_run(args):
    manifest = build_manifest()
    wanted = set(args.tasks.split(",")) if args.tasks else {t["id"] for t in manifest["tasks"]}
    splits = set(args.splits.split(","))
    tasks = [t for t in manifest["tasks"] if t["id"] in wanted and t["split"] in splits]
    if not tasks:
        raise LabError("nenhuma tarefa selecionada")
    unvalidated = [t["id"] for t in tasks if not t["validated"]]
    if unvalidated and not args.allow_unvalidated:
        raise LabError("tarefas sem validation.json ok (rode validate-task): " + ", ".join(unvalidated))
    arms = parse_arms(args.arms)
    order = schedule([t["id"] for t in tasks], arms, args.reps, args.seed)
    n = len(order)
    out_dir = pathlib.Path(args.out_dir)
    if args.agent == "scripted":
        agent = ScriptedAgent(args.scripted_behaviour)
    else:
        if args.agent != "claude":
            raise LabError("agentes: claude | scripted")
        if not args.model or not re.match(r"^claude-[a-z0-9.-]+$", args.model):
            raise LabError("--model deve ser um ID explícito (ex.: claude-haiku-5-5), nunca um alias")
        for need in ("claude", "git"):
            if not shutil.which(need):
                raise LabError(f"comando ausente: {need}")
        if not args.sidecar or not pathlib.Path(args.sidecar).is_file():
            raise LabError("--sidecar (binário) obrigatório para o agente claude")
        agent = ClaudeAgent(args.model, str(pathlib.Path(args.sidecar).resolve()), args.credentials, args.max_budget,
                            args.timeout, args.tools_allow.split(","))
        est = n * args.max_budget
        if not args.yes_spend:
            print(f"Dry run: {n} execuções ({len(tasks)} tarefas × {len(arms)} braços × {args.reps}); "
                  f"teto teórico US$ {est:.2f}; modelo {args.model}. Use --yes-spend para executar.")
            return 0
    holdout_guard(tasks, args.final, "run:" + args.arms)
    out_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    runs, spend = out_dir / "runs.jsonl", out_dir / "spend.jsonl"
    meta = {"schema": "lab-meta/1", "manifest": manifest, "arms": [a["name"] for a in arms], "seed": args.seed,
            "model_requested": getattr(agent, "model", None), "order": [[t, a["name"], r] for t, a, r in order],
            "isolation": sandbox_prefix()[1], "sidecar_sha256": sha256_file(args.sidecar) if args.sidecar else None,
            "claude_version": run(["claude", "--version"])[1].strip() if args.agent == "claude" else None,
            "hardware": {"cpus": os.cpu_count(), "platform": sys.platform}, "cost_cap_usd": args.max_total_usd,
            "started": time.time()}
    (out_dir / "meta.json").write_text(json.dumps(meta, indent=2) + "\n")
    done = {(json.loads(l)["task"], json.loads(l)["arm"], json.loads(l)["rep"]) for l in runs.read_text().splitlines()
            if l.strip()} if runs.is_file() else set()
    with tempfile.TemporaryDirectory(prefix="lab-home-") as home:
        for tid, arm, rep in order:
            if (tid, arm["name"], rep) in done:
                continue
            if args.max_total_usd is not None and spend_total(spend) >= args.max_total_usd:
                print("teto de custo atingido; parando", file=sys.stderr)
                break
            task, tdir = load_task(tid)
            rec = execute_one(agent, task, tdir, arm, rep, args, home)
            with runs.open("a") as f:
                f.write(json.dumps(rec, ensure_ascii=False) + "\n")
            with spend.open("a") as f:
                f.write(json.dumps({"task": tid, "arm": arm["name"], "model": rec["model"], "cost_usd": rec["cost_usd"]}) + "\n")
            print(f"{tid:24s} {arm['name']:4s} r{rep} {rec['verdict']:5s} wall={rec['wall_s']}s cost={rec['cost_usd']}")
    return 0


# --------------------------------------------------------------------------- report

def boot_ci(deltas, n=2000, seed=7):
    if len(deltas) < 5:
        return None
    rnd = random.Random(seed)
    means = sorted(statistics.mean(rnd.choices(deltas, k=len(deltas))) for _ in range(n))
    return means[int(0.025 * n)], means[int(0.975 * n)]


def pct(xs, q):
    if not xs:
        return None
    s = sorted(xs)
    return s[min(len(s) - 1, math.ceil(q * len(s)) - 1)]


TEST_PATH = re.compile(r"(^|/)(tests?|spec|__tests__)/|(_test\.go|_spec\.rb|\.test\.[jt]sx?|(^|/)test_[^/]*\.py)$")


def non_test_outside(r):
    return [p for p in r["oracle"]["outside_scope"] if not TEST_PATH.search(p)]


def summarize(rows):
    ok = [r for r in rows if r["comparable"]]
    succ = [r for r in ok if r["success"]]
    cost = [r["cost_usd"] for r in ok if r["cost_usd"] is not None]
    wall = [r["wall_s"] for r in ok if r["wall_s"] is not None]
    scc = [r["cost_usd"] for r in succ if r["cost_usd"] is not None]
    return {
        "runs": len(rows), "comparable": len(ok), "excluded": len(rows) - len(ok), "passed": len(succ),
        "pass_rate": round(len(succ) / len(ok), 3) if ok else None,
        "hidden_tests_passed": sum(1 for r in ok if r["oracle"]["tests"] == "pass"),
        "scope_violations": sum(1 for r in ok if not r["oracle"]["scope_ok"]),
        "scope_violations_non_test": sum(1 for r in ok if non_test_outside(r)),
        "invariant_violations": sum(1 for r in ok for i in r["oracle"]["invariants"] if not i["ok"]),
        "infra_errors": sum(1 for r in rows if r["verdict"] == "error"),
        "cost_total": round(sum(cost), 4) if cost else None,
        "cost_per_success": round(sum(cost) / len(succ), 4) if succ and cost and len(cost) == len(ok) else None,
        "wall_per_success_p50": pct([r["wall_s"] for r in succ if r["wall_s"] is not None], 0.5),
        "wall_p95": pct(wall, 0.95), "tool_calls_mean": round(statistics.mean(
            [r["tool_calls"] for r in ok if r["tool_calls"] is not None]), 2) if any(r["tool_calls"] is not None for r in ok) else None,
        "injected_chars_mean": round(statistics.mean([r["injected_chars"] or 0 for r in ok]), 1) if ok else None,
        "models_served": sorted({m for r in rows for m in (r["served_models"] or [])}),
        "cost_unknown_runs": sum(1 for r in ok if r["cost_usd"] is None),
    }


def build_report(rows, offline=None):
    """Scorecard unificado. Evidências simuladas/offline/agentic ficam em seções separadas, nunca somadas."""
    rep = {"schema": "lab-scorecard/1", "sections": {}, "limits": [
        "N pequeno é descritivo; sem significância não há ganho comprovado.",
        "Execuções com evidence=simulated validam a infraestrutura, não o produto.",
        "Retrieval offline não é resultado de desenvolvimento; ver seção offline separadamente.",
        "Custo ausente (None) não é zero; cost_per_success só aparece quando todas as execuções comparáveis têm custo.",
    ]}
    for ev in ("agentic", "simulated"):
        sel = [r for r in rows if r["evidence"] == ev]
        if not sel:
            continue
        sec = {"arms": {}, "by_split": {}, "by_class": {}, "paired_vs_OFF": {}}
        for arm in sorted({r["arm"] for r in sel}):
            sec["arms"][arm] = summarize([r for r in sel if r["arm"] == arm])
            for key, field in (("by_split", "split"), ("by_class", "class")):
                for v in sorted({r[field] for r in sel}):
                    sec[key].setdefault(v, {})[arm] = summarize([r for r in sel if r["arm"] == arm and r[field] == v])
        idx = {(r["task"], r["rep"], r["arm"]): r for r in sel if r["comparable"]}
        for arm in sec["arms"]:
            if arm == "OFF":
                continue
            d_pass, d_cost = [], []
            for (t, rp, a), r in idx.items():
                if a != arm or (t, rp, "OFF") not in idx:
                    continue
                o = idx[(t, rp, "OFF")]
                d_pass.append(int(r["success"]) - int(o["success"]))
                if r["cost_usd"] is not None and o["cost_usd"] is not None:
                    d_cost.append(r["cost_usd"] - o["cost_usd"])
            sec["paired_vs_OFF"][arm] = {
                "pairs": len(d_pass), "d_pass_mean": round(statistics.mean(d_pass), 3) if d_pass else None,
                "d_pass_ci95": boot_ci(d_pass), "d_cost_mean": round(statistics.mean(d_cost), 4) if d_cost else None,
                "d_cost_ci95": boot_ci(d_cost),
                "conclusion": "inconclusivo" if len(d_pass) < 20 else "ver IC"}
        rep["sections"][ev] = sec
    if offline:
        rep["sections"]["offline_retrieval"] = {"source": "sidecar-eval", "note": "não é resultado agentic", **offline}
    return rep


def render_md(rep):
    L = ["# Scorecard do laboratório", ""]
    for ev, sec in rep["sections"].items():
        L += [f"## Evidência: {ev}", ""]
        if ev == "offline_retrieval":
            L += ["```json", json.dumps({k: v for k, v in sec.items()}, ensure_ascii=False, indent=1)[:4000], "```", ""]
            continue
        L += ["| braço | exec. | comparáveis | pass | taxa | ocultos ok | escopo (não-teste) | invariantes | custo/correta | p50 s/correta | tool calls | injetado |",
              "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"]
        for a, s in sec["arms"].items():
            L.append(f"| {a} | {s['runs']} | {s['comparable']} | {s['passed']} | {s['pass_rate']} | {s['hidden_tests_passed']} | {s['scope_violations']} ({s['scope_violations_non_test']}) | "
                     f"{s['invariant_violations']} | {s['cost_per_success']} | {s['wall_per_success_p50']} | {s['tool_calls_mean']} | {s['injected_chars_mean']} |")
        L.append("")
        for a, p in sec["paired_vs_OFF"].items():
            L.append(f"- {a} vs OFF: {p['pairs']} pares, Δpass {p['d_pass_mean']} IC95 {p['d_pass_ci95']}, Δcusto {p['d_cost_mean']} → **{p['conclusion']}**")
        L.append("")
    L += ["## Limites", ""] + [f"- {x}" for x in rep["limits"]]
    return "\n".join(L) + "\n"


def cmd_report(args):
    rows = [json.loads(l) for l in pathlib.Path(args.runs).read_text().splitlines() if l.strip()]
    offline = load_json(args.offline)["summary"] if args.offline and "summary" in load_json(args.offline) else (
        {"file": args.offline} if args.offline else None)
    rep = build_report(rows, offline)
    if args.json:
        pathlib.Path(args.json).write_text(json.dumps(rep, indent=2, ensure_ascii=False) + "\n")
    md = render_md(rep)
    if args.md:
        pathlib.Path(args.md).write_text(md)
    else:
        print(md)
    return 0


# --------------------------------------------------------------------------- cli

def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("validate-task"); p.add_argument("id")
    sub.add_parser("validate-corpus")
    p = sub.add_parser("manifest"); p.add_argument("--out", default=str(CORPUS / "manifest.json"))
    p = sub.add_parser("run")
    p.add_argument("--agent", default="claude"); p.add_argument("--model"); p.add_argument("--sidecar")
    p.add_argument("--arms", default="OFF,V1,V2"); p.add_argument("--tasks"); p.add_argument("--splits", default="development,validation")
    p.add_argument("--reps", type=int, default=1); p.add_argument("--seed", type=int, default=11)
    p.add_argument("--out-dir", required=True); p.add_argument("--credentials", default=str(pathlib.Path.home() / ".claude" / ".credentials.json"))
    p.add_argument("--max-budget", type=float, default=0.5, help="teto por execução (US$)")
    p.add_argument("--max-total-usd", type=float, default=None); p.add_argument("--timeout", type=int, default=600)
    p.add_argument("--tools-allow", default="Read,Grep,Glob,Edit,Write,Bash(go test:*),Bash(ls:*)")
    p.add_argument("--yes-spend", action="store_true"); p.add_argument("--final", action="store_true")
    p.add_argument("--allow-unvalidated", action="store_true"); p.add_argument("--scripted-behaviour", default="reference")
    p = sub.add_parser("report"); p.add_argument("runs"); p.add_argument("--md"); p.add_argument("--json"); p.add_argument("--offline")
    a = ap.parse_args(argv)
    try:
        if a.cmd == "validate-task":
            r = validate_task(a.id)
            print(json.dumps({k: r[k] for k in ("id", "ok", "base_verdict")}))
            return 0 if r["ok"] else 1
        if a.cmd == "validate-corpus":
            bad = 0
            for d in task_dirs():
                r = validate_task(d.name)
                print(f"{d.name:28s} {'OK' if r['ok'] else 'FALHOU'} base={r['base_verdict']}")
                bad += 0 if r["ok"] else 1
            return 1 if bad else 0
        if a.cmd == "manifest":
            m = build_manifest()
            pathlib.Path(a.out).write_text(json.dumps(m, indent=2, ensure_ascii=False) + "\n")
            print(f"{a.out}: {len(m['tasks'])} tarefas {m['counts']}")
            return 0
        if a.cmd == "run":
            return cmd_run(a)
        if a.cmd == "report":
            return cmd_report(a)
    except LabError as e:
        print(f"erro: {e}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
