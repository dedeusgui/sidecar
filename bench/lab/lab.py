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
import unicodedata

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import sufficiency  # noqa: E402

HERE = pathlib.Path(__file__).resolve().parent
# LAB_CORPUS selects another corpus (e.g. bench/lab/corpus-nav for the E2 navigation suite). The holdout
# protections that key on the default corpus path keep applying to it.
CORPUS = pathlib.Path(os.environ["LAB_CORPUS"]).resolve() if os.environ.get("LAB_CORPUS") else HERE / "corpus"
TASKS = CORPUS / "tasks"
REPOS_JSON = CORPUS / "repos.json"
LEDGER = CORPUS / "holdout_ledger.jsonl"
SCHEMA = "lab-task/1"
RUN_SCHEMA = "lab-run/1"
SPLITS = ("development", "validation", "holdout")
CLASSES = ("locate", "debug", "edit", "multi-file", "contract-change", "negative", "ambiguous", "voice", "navigation")
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
            if dest.startswith("/") or ".." in pathlib.PurePosixPath(dest).parts or ".git" in pathlib.PurePosixPath(dest).parts:
                errs.append(f"destino inseguro do oráculo: {dest}")
            if str(src).startswith("/") or ".." in pathlib.PurePosixPath(str(src)).parts:
                errs.append(f"origem insegura do oráculo (fora de oracle/): {src}")
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


def build_manifest(include_holdout=False):
    if include_holdout and CORPUS.resolve() == (HERE / "corpus").resolve():
        raise LabError("atualização do holdout real está bloqueada enquanto o experimento não estiver autorizado")
    repos = load_repos()
    tasks = []
    sealed_path = CORPUS / "manifest.json"
    sealed = load_json(sealed_path) if sealed_path.is_file() and not include_holdout else None
    sealed_tasks = {t["id"]: t for t in (sealed or {}).get("tasks", [])}
    sealed_holdout = [t for t in sealed_tasks.values() if t.get("split") == "holdout"]
    if not include_holdout and sealed is None:
        raise LabError("manifesto selado ausente; atualização de inventário exige --include-holdout explícito")
    if not include_holdout:
        current_ids = {p.name for p in task_dirs()}
        expected_non_holdout = {tid for tid, t in sealed_tasks.items() if t.get("split") != "holdout"}
        if current_ids - set(sealed_tasks) or expected_non_holdout - current_ids:
            raise LabError("inventário difere do manifesto selado; recusei ler tarefas fora do inventário congelado")
        tasks.extend(sealed_holdout)
    for tdir in task_dirs():
        if not include_holdout:
            prior = sealed_tasks[tdir.name]
            if prior.get("split") == "holdout":
                continue
            if sha256_file(tdir / "task.json") != prior.get("task_sha256"):
                raise LabError(f"tarefa {tdir.name} mudou após o manifesto selado; atualizar exige revisão explícita do inventário")
        task = load_json(tdir / "task.json")
        errs = validate_task_schema(task, tdir)
        if errs:
            raise LabError(f"{tdir.name}: " + "; ".join(errs))
        if not include_holdout and task["split"] == "holdout":
            raise LabError(f"tarefa {tdir.name} mudou para holdout; atualização explícita do manifesto necessária")
        val = tdir / "validation.json"
        tasks.append({
            "id": task["id"], "split": task["split"], "class": task["class"], "repo": task["repo"],
            "task_sha256": sha256_file(tdir / "task.json"),
            "oracle_sha256": tree_sha256(tdir / "oracle") if (tdir / "oracle").is_dir() else None,
            "reference_sha256": sha256_file(tdir / "reference.patch") if (tdir / "reference.patch").is_file() else None,
            "validated": bool(val.is_file() and load_json(val).get("ok")),
        })
    by_split = {s: [t for t in tasks if t["split"] == s] for s in SPLITS}
    holdout_prompt_hash = (sealed or {}).get("holdout_prompts_sha256")
    if include_holdout:
        holdout_prompts = sorted(load_json(TASKS / t["id"] / "task.json")["prompt"] for t in by_split["holdout"])
        holdout_prompt_hash = sha256_bytes("\n".join(holdout_prompts).encode())
    if by_split["holdout"] and not holdout_prompt_hash:
        raise LabError("hash do holdout ausente no manifesto selado")
    return {
        "schema": "lab-manifest/1", "repos": repos, "tasks": tasks,
        "counts": {s: len(v) for s, v in by_split.items()},
        "classes": {c: sum(1 for t in tasks if t["class"] == c) for c in CLASSES},
        "holdout_prompts_sha256": holdout_prompt_hash,
        "note": "holdout selado: use --final; acessos ficam em holdout_ledger.jsonl",
    }


def holdout_guard(tasks, final, purpose, repeat=False):
    """Bloqueia holdout fora de --final e registra qualquer uso no ledger (append-only)."""
    held = [t for t in tasks if t["split"] == "holdout"]
    if not held:
        return
    if not final:
        raise LabError("holdout selado: use --final (uma única avaliação final, sem tuning depois)")
    if holdout_burned() and not repeat:
        raise LabError("o holdout já foi consumido (ver holdout_ledger.jsonl); repetir exige --allow-repeat-final e invalida o nome 'cego'")
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


def setup_paths_file(work):
    """Linha de base dos artefatos de setup. Fica FORA do workspace: um arquivo dentro de .git poderia ser reescrito
    pelo agente para esconder alterações do cálculo de escopo."""
    work = pathlib.Path(work)
    return work.parent / (work.name + ".lab_setup_paths.json")


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
    base_file = setup_paths_file(work)
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
    setup_paths_file(work).write_text(json.dumps(sorted(entries)))
    return work


def sandbox_prefix():
    """Prefixo de isolamento de rede para o oráculo. Retorna (argv_prefix, nome_do_isolamento)."""
    if shutil.which("bwrap"):
        return ["bwrap", "--unshare-net", "--unshare-pid", "--die-with-parent", "--dev-bind", "/", "/"], "bwrap-net"
    if shutil.which("unshare") and subprocess.run(["unshare", "-rn", "true"], capture_output=True).returncode == 0:
        return ["unshare", "-rn"], "unshare-net"
    return [], "none"


def _bind_specs(paths):
    specs = []
    for item in paths:
        host, target = item if isinstance(item, (tuple, list)) else (item, item)
        host = pathlib.Path(host).resolve()
        target = pathlib.PurePosixPath(str(target))
        if not host.exists() or not target.is_absolute() or ".." in target.parts:
            raise LabError(f"agent filesystem sandbox recebeu mount inválido: {host} -> {target}")
        specs.append((str(host), target.as_posix()))
    return specs


TOOL_MOUNTS = ("/opt/sidecar", "/opt/codegraph")  # where an arm mounts the tool it is allowed to use
COMPETITOR_TOOL_NAME = re.compile(r"^(@colbymchenry|codegraph|sidecar)([-_.].*)?$", re.I)


def competitor_tool_paths(mounts, max_depth=5):
    """Arquivos/pastas de Sidecar ou CodeGraph dentro de prefixos montados (ex.: `npm -g` em /opt/node). O controle OFF
    não pode alcançá-los por caminho absoluto; o sandbox os mascara e o braço habilitado monta seu binário à parte."""
    found = []
    for host, target in mounts:
        host = pathlib.Path(host)
        if not host.is_dir():
            continue
        base_depth = len(host.parts)
        for dirpath, dirnames, filenames in os.walk(host):
            depth = len(pathlib.Path(dirpath).parts) - base_depth
            for name in list(dirnames) + filenames:
                if COMPETITOR_TOOL_NAME.match(name):
                    rel = (pathlib.Path(dirpath) / name).relative_to(host)
                    found.append(((pathlib.Path(dirpath) / name), (pathlib.PurePosixPath(target) / rel.as_posix()).as_posix()))
            dirnames[:] = [d for d in dirnames if depth + 1 < max_depth and not COMPETITOR_TOOL_NAME.match(d)
                           and not (pathlib.Path(dirpath) / d).is_symlink()]
    return found


def agent_sandbox_command(argv, work, readonly_paths=(), writable_paths=(), sandbox_env=None):
    """Executa o agente em uma raiz vazia: somente sistema, checkout e mounts explicitamente permitidos."""
    bwrap = shutil.which("bwrap")
    if sys.platform != "linux" or not bwrap:
        raise LabError("agent filesystem sandbox indisponível: Linux com bwrap é obrigatório")
    work = pathlib.Path(work).resolve()
    if not work.is_dir():
        raise LabError("agent filesystem sandbox exige checkout existente")
    readonly = _bind_specs(readonly_paths)
    writable = _bind_specs(writable_paths)
    targets = [dest for _, dest in readonly + writable] + ["/work", "/agent-bin", "/home/agent"]
    dirs = set()
    for target in targets:
        parent = pathlib.PurePosixPath(target).parent
        while parent != pathlib.PurePosixPath("/"):
            dirs.add(parent.as_posix())
            parent = parent.parent
    cmd = [bwrap, "--die-with-parent", "--unshare-pid", "--ro-bind", "/usr", "/usr",
           "--symlink", "usr/bin", "/bin", "--symlink", "usr/lib", "/lib",
           "--symlink", "usr/lib64", "/lib64", "--ro-bind", "/etc", "/etc",
           "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/run"]
    for directory in sorted(dirs, key=lambda p: (p.count("/"), p)):
        cmd.extend(["--dir", directory])
    cmd.extend(["--bind", str(work), "/work"])
    for host, target in writable:
        cmd.extend(["--bind", host, target])
    for host, target in readonly:
        cmd.extend(["--ro-bind", host, target])
    intended = {dest for _, dest in readonly}
    tool_roots = [dest for _, dest in readonly if dest in TOOL_MOUNTS]
    in_tool = lambda p: any(p == r or p.startswith(r.rstrip("/") + "/") for r in tool_roots)
    for host, target in competitor_tool_paths([(h, d) for h, d in readonly if d not in TOOL_MOUNTS]):
        link = pathlib.Path(host)
        if target in intended or link.is_symlink() and in_tool(os.readlink(link)):
            continue  # montado de propósito (binário do braço habilitado)
        cmd.extend(["--tmpfs", target] if pathlib.Path(host).is_dir() else ["--ro-bind", "/dev/null", target])
    cmd.extend(["--clearenv", "--setenv", "PATH", "/agent-bin", "--setenv", "HOME", "/home/agent",
                "--setenv", "CLAUDE_CONFIG_DIR", "/agent-config", "--setenv", "SIDECAR_HOME", "/sidecar-home",
                "--setenv", "TMPDIR", "/tmp", "--setenv", "LANG", "C.UTF-8", "--setenv", "CI", "1",
                "--setenv", "GOTOOLCHAIN", "local", "--setenv", "GOPROXY", "off", "--setenv", "GOFLAGS", "-mod=mod",
                "--chdir", "/work"])
    for key, value in sorted((sandbox_env or {}).items()):
        cmd.extend(["--setenv", key, str(value)])
    cmd.extend(["--", *argv])
    return cmd


def verify_agent_sandbox():
    """Preflight the actual bwrap mount setup before a paid agent run is accepted."""
    with tempfile.TemporaryDirectory(prefix="lab-sandbox-check-") as work:
        command = agent_sandbox_command(["/usr/bin/true"], work)
        result = subprocess.run(command, capture_output=True, text=True, timeout=10)
        if result.returncode != 0:
            detail = (result.stderr or result.stdout).strip()[-300:]
            raise LabError(f"agent filesystem sandbox falhou no preflight: {detail}")
    return "bwrap-restricted"


def oracle_env(task):
    env = {k: os.environ[k] for k in ("PATH", "HOME", "LANG") if k in os.environ}
    env.update({"GOFLAGS": "-mod=mod", "GOPROXY": "off", "GOTOOLCHAIN": "local", "CGO_ENABLED": "0",
                "GOCACHE": str(cache_dir() / "gocache"), "NPM_CONFIG_OFFLINE": "true", "CI": "1"})
    env.update(task["oracle"].get("env") or {})
    return env


# --------------------------------------------------------------------------- oracle

def glob_re(g):
    """Glob com `*` dentro de um segmento, `**` entre segmentos e `?` de um caractere (fnmatch deixaria `*` cruzar `/`)."""
    out, i = "", 0
    while i < len(g):
        if g.startswith("**", i):
            out, i = out + ".*", i + 2
        elif g[i] == "*":
            out, i = out + "[^/]*", i + 1
        elif g[i] == "?":
            out, i = out + "[^/]", i + 1
        else:
            out, i = out + re.escape(g[i]), i + 1
    return re.compile("^" + out + "$")


def in_scope(path, allowed):
    return any(glob_re(g).match(path) or path == g.rstrip("/") or path.startswith(g.rstrip("/") + "/") for g in allowed)


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
                ok = text is not None and re.search(inv["pattern"], text, re.M) is None  # arquivo apagado não prova ausência
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
        # Each item is a required fact; a list item is satisfied by any one of its alternatives.
        missing = [m if isinstance(m, str) else "|".join(m) for m in o["answer_must_mention"]
                   if not any(alt.lower() in text for alt in ([m] if isinstance(m, str) else m))]
        forbidden = [m for m in o.get("answer_must_not_mention") or [] if m.lower() in text]
        res["tests"] = "pass" if not missing and not forbidden else "fail"
        res["detail"] = ("faltando: " + ", ".join(missing)) if missing else ""
        if forbidden:
            res["detail"] += " afirmação incorreta: " + ", ".join(forbidden)
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
    if CORPUS.resolve() == (HERE / "corpus").resolve():
        sealed = load_json(CORPUS / "manifest.json")
        if any(t.get("id") == tid and t.get("split") == "holdout" for t in sealed.get("tasks", [])):
            raise LabError("validação do holdout real está bloqueada enquanto o experimento não estiver autorizado")
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
            answer = " ".join(m if isinstance(m, str) else m[0] for m in o.get("answer_must_mention") or [])
            report["reference"] = run_oracle(task, tdir, refwork, answer_text=answer)
            report["ok"] = base_v == "fail" and verdict(report["reference"]) == "pass"
        report["base_verdict"] = base_v
    for r in ("base", "reference"):
        if report[r]:
            report[r]["detail"] = (report[r].get("detail") or "")[-300:]
    (tdir / "validation.json").write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")
    return report


# --------------------------------------------------------------------------- agents

AGENT_BIN_COMMANDS = (
    "claude", "git", "bash", "sh", "env", "ls", "cat", "head", "tail", "sed", "grep", "rg", "find", "timeout",
    "go", "gofmt", "python", "python3", "pytest", "npm", "node", "yarn", "pnpm", "ruby", "bundle", "rake",
    "cargo", "rustc", "mvn", "gradle", "php", "phpunit", "composer", "mkdir", "cp", "mv", "rm", "touch", "pwd",
    "dirname", "basename", "readlink", "realpath", "uname", "wc", "sort", "uniq", "tr", "cut", "xargs",
)


PROXY_ENV_KEYS = ("HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy")
CA_ENV_KEYS = ("NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE")


def sandbox_network_env(environ=None):
    """Proxy/CA do host, iguais para todos os braços: `--clearenv` remove o proxy e o cliente não alcança a API em
    redes que exigem proxy (reproduzido em 2026-11-09). Só variáveis de rede e o arquivo de CA são repassados."""
    environ = os.environ if environ is None else environ
    env, mounts = {}, []
    for key in PROXY_ENV_KEYS:
        if environ.get(key):
            env[key] = environ[key]
    for key in CA_ENV_KEYS:
        value = environ.get(key)
        if value and pathlib.Path(value).is_file():
            target = f"/agent-ca/{key.lower()}.pem"
            mounts.append((pathlib.Path(value).resolve(), target))
            env[key] = target
    return env, mounts


def verify_agent_auth(credentials):
    """Preflight gratuito (sem inferência): o cliente dentro do sandbox enxerga as credenciais isoladas."""
    with tempfile.TemporaryDirectory(prefix="lab-auth-check-") as root:
        root = pathlib.Path(root)
        work, cfg, tmp, home = (root / n for n in ("work", "cfg", "tmp", "home"))
        for d in (work, cfg, tmp, home):
            d.mkdir()
        if credentials and pathlib.Path(credentials).is_file():
            shutil.copy(credentials, cfg / ".credentials.json")
            os.chmod(cfg / ".credentials.json", 0o600)
        bin_dir, mounts, env = agent_runtime_bin(root / "bin", work)
        net_env, net_mounts = sandbox_network_env()
        command = agent_sandbox_command(["claude", "auth", "status"], work,
                                        mounts + net_mounts + [(bin_dir, "/agent-bin")],
                                        [(cfg, "/agent-config"), (home, "/home/agent"), (tmp, "/tmp")],
                                        {**env, **net_env})
        code, out, err, timed_out = run(command, timeout=60)
        try:
            status = json.loads(out)
        except ValueError:
            status = {}
        if timed_out or status.get("loggedIn") is not True:
            raise LabError("cliente Claude no sandbox não está autenticado (claude auth status: loggedIn != true); "
                           "nenhuma execução paga foi iniciada")
        if status.get("configDirectory") != "/agent-config":
            raise LabError("cliente Claude no sandbox não usa a configuração isolada")
        return status.get("authMethod") or "unknown"


def _inside(path, root):
    try:
        pathlib.Path(path).relative_to(root)
        return True
    except ValueError:
        return False


def reject_sensitive_mounts(sources, forbidden):
    for source in sources:
        source = pathlib.Path(source).resolve()
        for blocked in forbidden:
            blocked = pathlib.Path(blocked).resolve()
            if source == blocked or source in blocked.parents or blocked in source.parents:
                raise LabError(f"agent filesystem sandbox: mount sobrepõe dados protegidos: {source}")


UNSAFE_RUNTIME_PREFIXES = {"/", "/root", "/home", "/opt", "/usr", "/var", "/tmp", "/etc"}


def _install_prefix(path):
    """`/opt/x/bin/tool` -> `/opt/x`; outros layouts usam o diretório do executável."""
    path = pathlib.Path(path)
    for parent in path.parents:
        if parent.name == "bin":
            return parent.parent
    return path.parent


def _shebang_interpreter(path):
    try:
        with open(path, "rb") as f:
            head = f.read(256)
    except OSError:
        return None
    if not head.startswith(b"#!"):
        return None
    parts = head[2:].split(b"\n", 1)[0].decode(errors="replace").split()
    if not parts or parts[0] == "/usr/bin/env":
        return None  # resolvido pelo PATH filtrado do sandbox
    return pathlib.Path(parts[0])


def _elf_library_dirs(path):
    try:
        with open(path, "rb") as f:
            if f.read(4) != b"\x7fELF":
                return []
    except OSError:
        return []
    ldd = shutil.which("ldd")
    if not ldd:
        return []
    code, out, _, timed_out = run([ldd, str(path)], timeout=10)
    if code != 0 or timed_out:
        return []
    dirs = []
    for line in out.splitlines():
        m = re.search(r"=>\s+(/\S+)", line)
        if m:
            dirs.append(pathlib.Path(m.group(1)).resolve().parent)
    return dirs


def runtime_prefixes(executable, _depth=0):
    """Prefixos mínimos (fora de /usr) para um runtime funcionar no sandbox: prefixo do executável, prefixo das
    bibliotecas dinâmicas e do interpretador do shebang. Prefixos amplos (/, /root, $HOME) são recusados."""
    executable = pathlib.Path(executable).resolve()
    out = []
    candidates = [_install_prefix(executable)]
    candidates += [_install_prefix(d / "x") if d.name != "lib" else d.parent for d in _elf_library_dirs(executable)]
    interp = _shebang_interpreter(executable)
    if interp is not None and _depth < 2:
        resolved = interp.resolve() if interp.exists() else interp
        if not _inside(resolved, "/usr"):
            candidates.append(_install_prefix(interp))
            candidates += runtime_prefixes(resolved, _depth + 1)
    home = pathlib.Path.home().resolve()
    for prefix in candidates:
        prefix = pathlib.Path(prefix).resolve()
        if _inside(prefix, "/usr") or not prefix.is_dir():
            continue
        if str(prefix) in UNSAFE_RUNTIME_PREFIXES or prefix == home or prefix in home.parents:
            raise LabError(f"runtime {executable} exige montar prefixo amplo demais: {prefix}")
        if prefix not in out:
            out.append(prefix)
    return out


def agent_runtime_bin(root, work=None):
    """Create a PATH with only named runtime/test commands; never include Sidecar or CodeGraph."""
    root = pathlib.Path(root)
    root.mkdir(parents=True, exist_ok=True)
    mounts, env = [], {}
    for name in AGENT_BIN_COMMANDS:
        found = shutil.which(name)
        if not found:
            continue
        executable = pathlib.Path(found).resolve()
        if not executable.is_file():
            continue
        if _inside(executable, "/usr"):
            target = str(executable)
        elif name == "claude":
            package = next((p for p in (executable.parent, *executable.parents) if (p / "package.json").is_file()), None)
            if package:
                target = "/agent-runtime/claude-package/" + executable.relative_to(package).as_posix()
                mounts.append((package, "/agent-runtime/claude-package"))
            else:
                target = "/agent-runtime/claude"
                mounts.append((executable, target))
        elif name == "go":
            code, goroot, _, timed_out = run([str(executable), "env", "GOROOT"], timeout=10)
            if code == 0 and not timed_out and pathlib.Path(goroot.strip()).is_dir():
                target = "/agent-runtime/go-root/bin/go"
                mounts.append((pathlib.Path(goroot.strip()), "/agent-runtime/go-root"))
                env["GOROOT"] = "/agent-runtime/go-root"
            else:
                target = "/agent-runtime/go"
                mounts.append((executable, target))
        else:
            # Runtimes fora de /usr (rbenv, uv tools, node em /opt) dependem de bibliotecas, shebangs e pacotes do
            # próprio prefixo: montar só o arquivo quebra ruby/npm/pytest (reproduzido em 2026-11-09 com bwrap real).
            target = str(executable)
            try:
                prefixes = runtime_prefixes(executable)
            except LabError:
                continue  # comando indisponível para todos os braços, nunca um mount amplo do host
            for prefix in prefixes:
                if not any(_inside(prefix, existing) for existing, _ in mounts):
                    mounts.append((prefix, str(prefix)))
        link = root / name
        if not link.exists() and not link.is_symlink():
            link.symlink_to(target)
    if work and (pathlib.Path(work) / ".venv" / "bin" / "python").exists():
        cfg = pathlib.Path(work) / ".venv" / "pyvenv.cfg"
        if cfg.is_file():
            for line in cfg.read_text(errors="replace").splitlines():
                key, sep, value = line.partition("=")
                if sep and key.strip() == "home":
                    runtime = pathlib.Path(value.strip()).resolve().parent
                    if runtime.is_dir() and not _inside(runtime, "/usr"):
                        mounts.append((runtime, str(runtime)))
                    break
        for name, suffix in (("python", ""), ("python3", ""), ("pytest", "-m pytest ")):
            link = root / name
            if link.exists() or link.is_symlink():
                link.unlink()
            link.write_text(f'#!/bin/sh\nexec /work/.venv/bin/python {suffix}"$@"\n')
            link.chmod(0o755)
    return root, mounts, env


BASE_AGENT_TOOLS = ["Read", "Grep", "Glob", "Edit", "Write", "Bash(ls:*)"]
EXPLORE_TOOLS = ["Bash(sidecar explore:*)", "Bash(/opt/sidecar explore:*)"]  # only arms with "explore": True
CODEGRAPH_CLI_TOOLS = ["Bash(codegraph explore:*)", "Bash(codegraph node:*)", "Bash(codegraph query:*)", "Bash(codegraph context:*)"]
CODEGRAPH_MCP_TOOLS = ["mcp__codegraph"]


def arm_extra_tools(arm):
    """Tools an arm adds on top of the equal base list: its own intervention, recorded in meta."""
    if arm.get("explore"):
        return list(EXPLORE_TOOLS)
    if arm.get("codegraph") == "cli":
        return list(CODEGRAPH_CLI_TOOLS)
    if arm.get("codegraph") == "mcp":
        return list(CODEGRAPH_MCP_TOOLS)
    return []
TEST_RUNNER_TOOLS = {
    "go": ["Bash(go test:*)"],
    "python": ["Bash(pytest:*)", "Bash(python -m pytest:*)", "Bash(python3 -m pytest:*)"],
    "javascript": ["Bash(npm test:*)", "Bash(npm run test:*)", "Bash(node --test:*)", "Bash(yarn test:*)", "Bash(pnpm test:*)"],
    "ruby": ["Bash(bundle exec:*)", "Bash(rake test:*)"],
    "rust": ["Bash(cargo test:*)"],
    "java": ["Bash(mvn test:*)", "Bash(gradle test:*)", "Bash(./gradlew test:*)"],
    "php": ["Bash(phpunit:*)", "Bash(vendor/bin/phpunit:*)", "Bash(composer test:*)"],
}


def tools_allow_for(task, requested=None):
    """Stable effective tool list, equal across arms; explicit --tools-allow replaces defaults."""
    if requested is not None:
        values = requested.split(",") if isinstance(requested, str) else requested
    else:
        language = task.get("language") or load_repos().get(task.get("repo"), {}).get("language")
        values = BASE_AGENT_TOOLS + TEST_RUNNER_TOOLS.get(language, [])
    return list(dict.fromkeys(v.strip() for v in values if v and v.strip()))

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
            text = " ".join(m if isinstance(m, str) else m[0] for m in task["oracle"].get("answer_must_mention") or []) or "feito"
        elif self.behaviour == "scope_violation":
            (pathlib.Path(work) / "STRAY_FILE.txt").write_text("fora do escopo\n")
        elif self.behaviour == "crash":
            raise RuntimeError("agente simulado falhou")
        return {"wall_s": round(time.perf_counter() - t0, 3), "result": text, "cost_usd": 0.0, "tokens_in": 0,
                "tokens_out": 0, "tool_calls": 0, "turns": 1, "served_models": ["scripted"], "exit": 0,
                "timed_out": False, "hook_delivered": None, "injected_chars": 0, "incomparable_reasons": [],
                "stream_error": False, "first_edit_s": None, "tool_events": []}


def deny_flags(paths):
    """Regras de negação do Claude Code para dados do benchmark (oráculo, reference.patch, resultados dos outros braços).
    `//` marca caminho absoluto. Cobre leitura, busca, listagem e escrita; NÃO cobre código executado por `go test`."""
    rules = []
    for p in paths:
        for tool in ("Read", "Grep", "Glob", "Edit", "Write"):
            rules.append(f"{tool}(/{p}/**)")
    return ["--disallowedTools", *rules] if rules else []


def touched_benchmark_data(events, deny_paths):
    """True se algum evento de ferramenta menciona dados do benchmark. Uma tentativa negada também conta:
    o braço sondou o gabarito, então a execução deixa de ser comparável."""
    needles = [str(pathlib.Path(p).resolve()) for p in deny_paths] + ["reference.patch", "/oracle/", "validation.json", "holdout_ledger"]
    return any(n in (e.get(f) or "") for e in events for n in needles for f in ("key", "cmd"))


def workspace_mentions_benchmark(work, needles):
    """Código escrito pelo agente (ex.: um teste Go) pode ler o oráculo mesmo com Read negado, porque `go test` roda como o
    mesmo usuário. Procura nos arquivos alterados qualquer referência ao caminho do benchmark."""
    for rel in changed_paths(work):
        p = pathlib.Path(work) / rel
        try:
            if p.is_file() and p.stat().st_size < 1 << 20:
                text = p.read_text(errors="replace")
                if any(n in text for n in needles):
                    return rel
        except OSError:
            continue
    return None


DEFAULT_FOLLOWUP = ("Revise o que você alterou: confirme que o pedido foi atendido por completo e corrija o que ficou faltando. "
                    "Responda em uma frase.")


def assistant_questions(stream_text):
    """Extract ephemeral, ordered assistant questions from stream-json; callers must not persist their text."""
    questions = []
    for line_order, line in enumerate(stream_text.splitlines()):
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if event.get("type") != "assistant":
            continue
        message_id = event.get("uuid") or event.get("message_id") or f"message-{line_order}"
        message = event.get("message") or {}
        for block_order, block in enumerate(message.get("content") or []):
            if not isinstance(block, dict) or block.get("type") != "text":
                continue
            text = block.get("text", "")
            if "?" in text:
                questions.append({"text": text, "order": line_order * 1000 + block_order,
                                  "message_id": message_id})
    return questions


def _normalized_text(value):
    return unicodedata.normalize("NFKD", str(value)).encode("ascii", "ignore").decode("ascii").lower()


def _option_labels(task):
    labels = task.get("underspec", {}).get("options") or []
    out = set()
    for option in labels:
        values = [option] if isinstance(option, str) else [option.get(k) for k in ("label", "value", "name", "choice") if isinstance(option, dict)]
        for value in values:
            if isinstance(value, str) and value.strip():
                out.add(" ".join(_normalized_text(value).split()))
    return out


def _relevant_question(task, question):
    if "?" not in question:
        return False
    text = " " + " ".join(_normalized_text(question).split()) + " "
    matched = 0
    for label in _option_labels(task):
        if label and re.search(rf"(?<![a-z0-9]){re.escape(label)}(?![a-z0-9])", text):
            matched += 1
    return matched >= 2


def _question_events(first_turn):
    questions = list(first_turn.get("assistant_questions") or [])
    for event in first_turn.get("tool_events") or []:
        if event.get("kind") == "clarify" and event.get("completed") is True and not event.get("err"):
            text = event.get("question") or event.get("key") or ""
            if text:
                questions.append({"text": text, "order": event.get("order"), "message_id": event.get("message_id")})
    return questions


def _asked_before_edit(first_turn, question):
    edits = [e for e in first_turn.get("tool_events", []) if e.get("kind") == "edit"]
    if not edits:
        return True
    order = question.get("order")
    if order is None:
        return False
    for edit in edits:
        edit_order = edit.get("order")
        if edit_order is None or edit_order <= order:
            return False
        if question.get("message_id") and question.get("message_id") == edit.get("message_id"):
            return False
    return True


def clarification_status(task, first_turn=None, session_available=True):
    """Classify the simulator response without storing the question or exposing an unasked decision."""
    if not isinstance(task.get("underspec"), dict):
        return "not_applicable"
    if (first_turn or {}).get("unattributed_edits"):
        return "uncertain_order"
    questions = _question_events(first_turn or {})
    asked = [q for q in questions if "?" in q.get("text", "")]
    relevant = any(_relevant_question(task, q.get("text", "")) and _asked_before_edit(first_turn or {}, q) for q in asked)
    if not session_available:
        return "expired" if relevant else "irrelevant" if asked else "not_asked"
    return "answered" if relevant else "irrelevant" if asked else "not_asked"


def followup_for(task, first_turn=None, session_available=True):
    """Return a neutral follow-up unless a relevant clarification was asked before editing."""
    if isinstance(task.get("underspec"), dict):
        if session_available and ((first_turn or {}).get("clarification_status") == "answered" or
                                  clarification_status(task, first_turn, session_available) == "answered"):
            return task["underspec"]["canned_answer"]
        return DEFAULT_FOLLOWUP
    if task.get("followup"):
        return task["followup"]
    return DEFAULT_FOLLOWUP


def session_id_of(stream_text):
    for line in stream_text.splitlines():
        line = line.strip()
        if line.startswith("{"):
            try:
                sid = json.loads(line).get("session_id")
            except ValueError:
                continue
            if isinstance(sid, str) and sid:
                return sid
    return None


def merge_turns(turns):
    """Soma as voltas de uma execução: custo, tokens, chamadas, tempo e eventos; o resultado é o da última volta; qualquer
    volta não comparável, com timeout ou sem modelo servido contamina a execução inteira."""
    def total(key):
        vals = [t.get(key) for t in turns]
        return sum(vals) if vals and all(v is not None for v in vals) else None
    last = turns[-1]
    models = sorted({m for t in turns for m in (t["served_models"] or [])}) or None
    reasons = [r for t in turns for r in t["incomparable_reasons"]]
    delivered = [t.get("hook_delivered") for t in turns]
    hook_delivered = True if any(v is True for v in delivered) else False if all(v is not None for v in delivered) else None
    client_wall = total("wall_s")
    clarification = next((t.get("clarification_status") for t in turns if t.get("clarification_status")), None)
    optional_metrics = {key: total(key) for key in ("input_uncached", "cache_read", "cache_create", "output",
                                                     "tool_result_tokens", "index_ms", "hook_ms", "sync_ms",
                                                     "retrieve_ms", "resident_context_tokens")}
    wall = client_wall
    if wall is not None and optional_metrics["index_ms"] is not None:
        wall += optional_metrics["index_ms"] / 1000
    events, context_seen = [], False
    for turn_index, turn in enumerate(turns):
        for event in turn["tool_events"]:
            event = dict(event, turn=turn_index + 1, stream_order=event.get("order"), order=len(events))
            if context_seen:
                event["after_context"] = True
            events.append(event)
        context_seen = context_seen or turn.get("hook_delivered") is True
    return {"wall_s": round(wall, 2) if wall is not None else None, "client_wall_s": client_wall, "result": last["result"], "cost_usd": total("cost_usd"), "tokens_in": total("tokens_in"),
            "tokens_out": total("tokens_out"), "tool_calls": total("tool_calls"), "turns": total("turns"), "served_models": models,
            "exit": last["exit"], "timed_out": any(t["timed_out"] for t in turns), "hook_delivered": hook_delivered,
            "injected_chars": total("injected_chars"), "incomparable_reasons": reasons,
            "stream_error": any(t["stream_error"] for t in turns), "hook_names": sorted({n for t in turns for n in t["hook_names"]}),
            "clarification_status": clarification,
            "first_edit_s": None, "tool_events": events, "volta_injetada": [t["injected_chars"] for t in turns],
            "n_voltas": len(turns),
            # O que o Sidecar disse de fato (só presença; o texto não é guardado): permite saber se R4/R5 foram exercidos.
            "sufficiency": merge_sufficiency([t.get("sufficiency") for t in turns]),
            "token_origins": sufficiency.token_origins({**optional_metrics, "injected_chars": total("injected_chars"),
                                                       "explore_chars": total("explore_chars"),
                                                       "tool_result_chars": total("tool_result_chars")}),
            "notas": {"groom": any("Open decision" in t.get("hook_context", "") for t in turns),
                      "impact": any("Change check" in t.get("hook_context", "") for t in turns),
                      "brief": any("TASK BRIEF" in t.get("hook_context", "") for t in turns)},
            **optional_metrics}


def codegraph_shim(package_dir):
    """Path of the codegraph entry point inside an installed package dir (npm --prefix DIR), relative to it."""
    root = pathlib.Path(package_dir)
    for rel in ("node_modules/@colbymchenry/codegraph/npm-shim.js", "node_modules/@colbymchenry/codegraph/dist/bin/codegraph.js"):
        if (root / rel).is_file():
            return rel
    raise LabError(f"codegraph não encontrado em {root} (instale com npm install --prefix DIR @colbymchenry/codegraph@VERSÃO)")


def codegraph_version(package_dir):
    try:
        return load_json(pathlib.Path(package_dir) / "node_modules/@colbymchenry/codegraph/package.json").get("version")
    except (OSError, ValueError, LabError):
        return None


def allow_arm_mcp(reasons, parsed, arm):
    """MCP loaded is contamination, except the CodeGraph server in the CG-mcp arm (and nothing else)."""
    if arm.get("codegraph") != "mcp":
        return reasons
    servers = (parsed.get("init") or {}).get("mcp_servers") or []
    names = {s.get("name") if isinstance(s, dict) else str(s) for s in servers}
    if names == {"codegraph"}:
        return [r for r in reasons if r != "servidores MCP carregados"]
    return list(reasons) + ([] if names else ["braço CG-mcp sem servidor codegraph carregado"])


def merge_sufficiency(parts):
    """Sum per-turn sufficiency counts; unknown (None) when a turn has no report."""
    if not parts or any(p is None for p in parts):
        return None
    out = {}
    for p in parts:
        for k, v in p.items():
            if k == "by_delivery":
                out.setdefault(k, []).extend(v)
            elif isinstance(v, (int, float)) and not isinstance(v, bool):
                out[k] = out.get(k, 0) + v
            elif v is None:
                out[k] = None
    return out


class ClaudeAgent:
    """Claude Code real: configuração isolada por execução, canário, verificação de init e modelo servido."""
    simulated = False

    def __init__(self, model, sidecar, credentials, max_budget, timeout, tools_allow, deny_paths=(), turns=1, codegraph=None):
        self.codegraph = str(pathlib.Path(codegraph).resolve()) if codegraph else None
        sys.path.insert(0, str(HERE.parent / "agentic"))
        import real_hook_pilot as rhp
        self.rhp, self.model, self.sidecar, self.turns = rhp, model, sidecar, turns
        self.credentials, self.max_budget, self.timeout, self.tools_allow = credentials, max_budget, timeout, tools_allow
        # O agente roda como o mesmo usuário e `Read` aceita caminho absoluto: sem estas regras ele LÊ o oráculo
        # oculto. Medido (2026-11-08): sem a regra a leitura funciona; com ela Read, Grep e `ls` são negados.
        self.deny_paths = [str(pathlib.Path(p).resolve()) for p in deny_paths]

    def command(self, prompt, settings, resume=None, tools_allow=None, deny_paths=None, max_budget=None, mcp_config=None):
        persist = self.turns > 1  # uma segunda volta precisa da sessão do cliente
        tools_allow = (self.tools_allow or []) if tools_allow is None else tools_allow
        flags = ["--output-format", "stream-json", "--verbose", "--include-hook-events", *([] if persist else ["--no-session-persistence"]),
                 *(["--resume", resume] if resume else []), "--permission-mode", "acceptEdits", "--setting-sources", "project,local", "--strict-mcp-config",
                 "--disable-slash-commands", "--allowedTools", *tools_allow,
                 *deny_flags(self.deny_paths if deny_paths is None else deny_paths)]
        cmd = ["claude", "-p", prompt, "--model", self.model, *flags, "--max-budget-usd", str(self.max_budget if max_budget is None else max_budget)]
        cmd += ["--mcp-config", mcp_config] if mcp_config else []
        return cmd + (["--settings", settings] if settings else [])

    def run(self, task, tdir, work, arm, env, prompt):
        rhp = self.rhp
        tools_allow = arm["tools_allow"] if "tools_allow" in arm else tools_allow_for(task, self.tools_allow)
        cfg, marker = rhp.make_config_dir(self.credentials, tempfile.gettempdir())
        cfg_dir = pathlib.Path(cfg)
        sandbox_home, sandbox_tmp = cfg_dir / "home", cfg_dir / "tmp"
        sandbox_home.mkdir()
        sandbox_tmp.mkdir()
        (cfg_dir / "settings.json").write_text((cfg_dir / "settings.json").read_text().replace(marker, "/agent-config/canary-fired.txt"))
        bin_dir, runtime_mounts, sandbox_env = agent_runtime_bin(cfg_dir / "runtime-bin", work)
        writable_mounts = [(cfg_dir, "/agent-config"), (sandbox_home, "/home/agent"),
                           (sandbox_tmp, "/tmp"), (pathlib.Path(env["SIDECAR_HOME"]), "/sidecar-home")]
        net_env, net_mounts = sandbox_network_env()
        sandbox_env.update(net_env)
        readonly_mounts = list(runtime_mounts) + list(net_mounts) + [(bin_dir, "/agent-bin")]
        if arm["engine"] is not None:
            readonly_mounts.append((self.sidecar, "/opt/sidecar"))
            if arm.get("explore"):
                (bin_dir / "sidecar").symlink_to("/opt/sidecar")
        if arm.get("codegraph"):
            if not self.codegraph:
                raise LabError("braço CodeGraph exige --codegraph (diretório do pacote instalado)")
            shim = codegraph_shim(self.codegraph)
            readonly_mounts.append((pathlib.Path(self.codegraph), "/opt/codegraph"))
            (bin_dir / "codegraph").symlink_to("/opt/codegraph/" + shim)
            sandbox_env["CODEGRAPH_TELEMETRY"] = "0"
            sandbox_env["SIDECAR_ENGINE"] = arm["engine"]
            sandbox_env.update({k: v for k, v in arm.get("env", {}).items() if k in ("SIDECAR_FEATURES", "SIDECAR_PROFILE")})
        try:
            reject_sensitive_mounts(["/usr", "/etc", work, cfg_dir, bin_dir, *[p for p, _ in runtime_mounts],
                                     *[p for p, _ in writable_mounts]], self.deny_paths)
            env = dict(env, CLAUDE_CONFIG_DIR=cfg)
            for k in ("ORCA_AGENT_HOOK_PORT", "ORCA_AGENT_HOOK_TOKEN", "ORCA_PANE_KEY", "ANTHROPIC_API_KEY"):
                env.pop(k, None)
            settings = None
            index_ms = None
            if arm["engine"] is not None:
                index_started = time.perf_counter()
                index_cmd = agent_sandbox_command(["/opt/sidecar", "index", "--root", "/work"], work,
                                                  readonly_mounts, writable_mounts, sandbox_env)
                code, _, err, timed_out = run(index_cmd, cwd=work, env=env, timeout=300)
                if code != 0 or timed_out:
                    raise LabError(f"indexação isolada falhou: {err[-300:]}")
                index_ms = round((time.perf_counter() - index_started) * 1000, 3)
                settings = rhp.hook_settings_json("/opt/sidecar")
            mcp_config = None
            if arm.get("codegraph"):
                index_started = time.perf_counter()
                init_cmd = agent_sandbox_command(["codegraph", "init", "/work"], work, readonly_mounts, writable_mounts, sandbox_env)
                code, _, err, timed_out = run(init_cmd, cwd=work, env=env, timeout=900)
                if code != 0 or timed_out:
                    raise LabError(f"codegraph init isolado falhou: {err[-300:]}")
                index_ms = round((time.perf_counter() - index_started) * 1000, 3)
                baseline = setup_paths_file(work)
                entries = set(json.loads(baseline.read_text())) if baseline.is_file() else set()
                baseline.write_text(json.dumps(sorted(entries | {".codegraph/"})))  # the index is setup, not the agent's change
                if arm["codegraph"] == "mcp":
                    (cfg_dir / "mcp.json").write_text(json.dumps({"mcpServers": {"codegraph": {
                        "command": "/agent-bin/codegraph", "args": ["serve", "--mcp"], "env": {"CODEGRAPH_TELEMETRY": "0"}}}}))
                    mcp_config = "/agent-config/mcp.json"
            hook_log = pathlib.Path(env["SIDECAR_HOME"]) / "logs" / "hook.jsonl"
            turns, session, remaining_budget = [], None, self.max_budget
            for n in range(self.turns):
                text = prompt if n == 0 else followup_for(task, turns[0], session_available=bool(session))
                hook_offset = hook_log_offset(hook_log)
                t0 = time.perf_counter()
                denied = self.deny_paths + ["/agent-config/.credentials.json"]
                sandboxed = agent_sandbox_command(self.command(text, settings, resume=session, tools_allow=tools_allow,
                                                               deny_paths=denied, max_budget=remaining_budget, mcp_config=mcp_config),
                                                  work, readonly_mounts, writable_mounts, sandbox_env)
                code, out, err, to = run(sandboxed, cwd=work, env=env, timeout=self.timeout)
                wall = time.perf_counter() - t0
                parsed = rhp.parse_stream(out)
                hook_metrics = hook_metrics_since(hook_log, hook_offset)
                session = session or session_id_of(out)
                reasons = rhp.contamination(parsed, os.path.exists(marker), self.model)
                reasons = allow_arm_mcp(reasons, parsed, arm)
                raw_events = tool_events(out)
                question_tool_events = tool_events(out, include_question_text=True) if n == 0 and self.turns > 1 else raw_events
                if touched_benchmark_data(raw_events, self.deny_paths):
                    reasons = list(reasons) + ["o agente tentou acessar dados do benchmark (oráculo, reference.patch ou resultados)"]
                suff = sufficiency_report(raw_events, parsed["hook_context"], parsed["hook_delivered"] is True)
                events = redact_tool_events(raw_events)
                if n == 0 and self.turns > 1 and not session:
                    reasons = list(reasons) + ["sessão do cliente ausente: a segunda volta não pôde ser retomada"]
                turn = {"wall_s": round(wall, 2), "result": parsed["result"] or "", "cost_usd": parsed["cost_usd"],
                              "tokens_in": parsed["tokens_in"], "tokens_out": parsed["tokens_out"],
                              "input_uncached": parsed["input_uncached"], "cache_read": parsed["cache_read"],
                              "cache_create": parsed["cache_create"], "output": parsed["output"],
                              "tool_result_tokens": parsed["tool_result_tokens"], "resident_context_tokens": None,
                              "index_ms": index_ms if n == 0 else 0.0 if index_ms is not None else None,
                              "hook_ms": hook_metrics["hook_ms"], "sync_ms": hook_metrics["sync_ms"],
                              "retrieve_ms": hook_metrics["retrieve_ms"],
                              "tool_calls": parsed["tool_calls"], "turns": parsed["turns"], "served_models": parsed["served_models"],
                              "exit": code, "timed_out": to, "hook_delivered": parsed["hook_delivered"] if arm["engine"] else None,
                              "injected_chars": (len(parsed["hook_context"]) if parsed["hook_delivered"] is not None else None)
                              if arm["engine"] else 0, "incomparable_reasons": reasons,
                              "stream_error": parsed["is_error"] is True or parsed["subtype"] not in (None, "success"),
                              "hook_names": parsed["hook_names"], "first_edit_s": None, "tool_events": events,
                              "assistant_questions": assistant_questions(out), "hook_context": parsed["hook_context"],
                              "sufficiency": suff, "explore_chars": suff["explore_chars"],
                              "tool_result_chars": suff["tool_result_chars"]}
                if n == 0 and self.turns > 1:
                    turn["unattributed_edits"] = bool(changed_paths(work)) and (
                        any("cmd" in e for e in raw_events) or not any(e.get("kind") == "edit" for e in raw_events))
                    status_turn = dict(turn, tool_events=question_tool_events)
                    turn["clarification_status"] = clarification_status(
                        task, status_turn, session_available=bool(session) and code == 0 and not to)
                turns.append(turn)
                if parsed["cost_usd"] is None:
                    remaining_budget = 0
                else:
                    remaining_budget = max(0.0, remaining_budget - parsed["cost_usd"])
                if n + 1 < self.turns and remaining_budget <= 0:
                    turn["incomparable_reasons"] = list(turn["incomparable_reasons"]) + ["orçamento da execução esgotado antes da próxima volta"]
                    break
                if to or code != 0 or not session and self.turns > 1:
                    break  # sem resposta da primeira volta não há o que continuar
            leaked = workspace_mentions_benchmark(work, self.deny_paths + ["reference.patch", "holdout_ledger"])
            merged = merge_turns(turns)
            if leaked:
                merged["incomparable_reasons"] = list(merged["incomparable_reasons"]) + [f"arquivo escrito pelo agente referencia dados do benchmark: {leaked}"]
            return merged
        finally:
            shutil.rmtree(cfg, ignore_errors=True)


# --------------------------------------------------------------------------- tool traces (R6)

TEST_CMD = re.compile(r"^\s*(go test\b|pytest\b|python3? -m pytest\b|npm (run )?test\b|pnpm test\b|yarn test\b|node --test\b|bundle exec\b|ruby -[IiT]|rake (test|spec)\b|cargo test\b|mvn test\b|gradle test\b|(?:\./)?gradlew test\b|phpunit\b|vendor/bin/phpunit\b|composer test\b)")
SEARCH_CMD = re.compile(r"^\s*(grep|rg|find|ag|ack|git grep)\b")
READ_CMD = re.compile(r"^\s*(cat|head|tail|sed -n|less|nl)\b")


def tool_events(stream_text, include_question_text=False):
    """Return ordered, correlated tool calls. Clarification text is ephemeral unless explicitly requested."""
    events, pending, context_orders = [], {}, []
    for line_order, line in enumerate(stream_text.splitlines()):
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            d = json.loads(line)
        except ValueError:
            continue
        order_base = line_order * 1000
        if d.get("type") == "system" and d.get("subtype") == "hook_response" and d.get("hook_event") == "UserPromptSubmit":
            try:
                output = json.loads(d.get("output") or "")
                context = output.get("hookSpecificOutput", {}).get("additionalContext")
                if isinstance(context, str) and context:
                    context_orders.append(order_base)
            except (ValueError, TypeError, AttributeError):
                pass
        msg = d.get("message")
        if not isinstance(msg, dict):
            continue
        content = msg.get("content")
        for block_order, block in enumerate(content if isinstance(content, list) else []):
            if not isinstance(block, dict):
                continue
            if d.get("type") == "assistant" and block.get("type") == "tool_use":
                name, inp = block.get("name", ""), block.get("input") or {}
                ev = None
                if name == "Read":
                    ev = {"kind": "read", "key": inp.get("file_path", "")}
                elif name in ("Grep", "Glob"):
                    ev = {"kind": "search", "key": " ".join(str(inp.get(k, "")) for k in ("pattern", "path", "glob") if inp.get(k))}
                elif name in ("Edit", "Write", "MultiEdit", "NotebookEdit"):
                    ev = {"kind": "edit", "key": inp.get("file_path", "")}
                elif name == "Bash":
                    cmd = inp.get("command", "")
                    kind = ("explore" if sufficiency.EXPLORE_CMD.search(cmd) else "test" if TEST_CMD.match(cmd) else
                            "search" if SEARCH_CMD.match(cmd) else "read" if READ_CMD.match(cmd) else "other")
                    ev = {"kind": kind, "key": cmd if kind != "test" else "test", "cmd": cmd}
                elif name.startswith("mcp__codegraph__"):
                    ev = {"kind": "explore", "key": name}  # CodeGraph MCP tools deliver code like an explore
                elif name in ("Agent", "Task"):
                    ev = {"kind": "delegate", "key": ""}
                elif name in ("ToolSearch", "TodoWrite"):
                    ev = {"kind": "bookkeeping", "key": name}
                elif name == "AskUserQuestion":
                    question_items = inp.get("questions")
                    question_items = question_items if isinstance(question_items, list) else [inp]
                    labels = [o.get("label", "") for item in question_items if isinstance(item, dict)
                              for o in item.get("options") or [] if isinstance(o, dict) and o.get("label")]
                    question = " ".join(str(item.get("question") or item.get("text") or "")
                                         for item in question_items if isinstance(item, dict))
                    if labels:
                        question += " Options: " + "; ".join(labels)
                    ev = {"kind": "clarify", "key": ""}
                    if include_question_text:
                        ev["question"] = question
                if ev is not None:
                    ev.update({"order": order_base + block_order, "thread_id": d.get("parent_tool_use_id") or msg.get("parent_tool_use_id") or "main",
                               "message_id": d.get("uuid") or d.get("message_id") or msg.get("id") or f"message-{line_order}",
                               "tool_use_id": block.get("id"), "timestamp": d.get("timestamp") or msg.get("timestamp"),
                               "completed": None, "err": None})
                    events.append(ev)
                    pending[block.get("id")] = ev
            elif d.get("type") == "user" and block.get("type") == "tool_result":
                ev = pending.get(block.get("tool_use_id"))
                if ev is None:
                    continue
                ev["completed"] = True
                ev["result_order"] = order_base + block_order
                content = block.get("content")
                result_text = content if isinstance(content, str) else " ".join(
                    c.get("text", "") for c in content or [] if isinstance(c, dict))
                ev["result_chars"] = len(result_text or "")
                if ev.get("kind") == "explore" and not block.get("is_error"):
                    # Paths only (never code): which files the explore shipped and which it only named.
                    ev["returned"], ev["named"] = sufficiency.paths_in_explore_output(result_text)
                if block.get("is_error"):
                    text = result_text
                    first = (text or "").strip().splitlines()[0].lower() if (text or "").strip() else "error"
                    ev["err"] = re.sub(r"\d+", "#", first)[:200]
    context_order = min(context_orders) if context_orders else None
    for ev in events:
        ev["after_context"] = None if context_order is None else ev["order"] > context_order
    return events


def deliveries_of(events, hook_context, hook_delivered):
    """Sidecar deliveries of one turn, in stream order: the hook context, then each completed explore call."""
    out = []
    if hook_delivered and hook_context:
        first = min((e["order"] for e in events), default=0)
        out.append({"source": "hook", "order": first - 1, "thread_id": "main", "message_id": None,
                    "returned": sufficiency.paths_in_hook_context(hook_context), "named": []})
    for e in events:
        if e.get("kind") == "explore" and e.get("completed"):
            out.append({"source": "explore", "order": e["order"], "thread_id": e.get("thread_id", "main"),
                        "message_id": e.get("message_id"), "returned": e.get("returned") or [],
                        "named": e.get("named") or [], "errored": bool(e.get("err"))})
    return out


def sufficiency_report(events, hook_context, hook_delivered):
    """Bucket counts and session signals for one turn; computed on raw events, persisted without paths."""
    deliveries = deliveries_of(events, hook_context, hook_delivered)
    classified = sufficiency.classify(events, deliveries)
    summary = sufficiency.summarize(classified)
    summary.update(sufficiency.session_signals(events, deliveries))
    summary["by_delivery"] = [{k: c.get(k) for k in ("source", "bucket", "next", "concurrent", "thread_id")} for c in classified]
    summary["explore_chars"] = sum(e.get("result_chars") or 0 for e in events if e.get("kind") == "explore") or 0
    tool_chars = [e.get("result_chars") for e in events if e.get("completed")]
    summary["tool_result_chars"] = sum(tool_chars) if tool_chars and all(c is not None for c in tool_chars) else (0 if not tool_chars else None)
    return summary


def redact_tool_events(events):
    """Keep repeatable identities without persisting shell commands, file contents or error text."""
    out = []
    for event in events:
        clean = dict(event)
        clean.pop("cmd", None)
        clean.pop("question", None)
        for field in ("returned", "named"):
            if clean.get(field):
                clean[field] = ["sha256:" + sha256_bytes(str(p).encode()) for p in clean[field]]
        for field in ("key", "err"):
            if clean.get(field):
                clean[field] = "sha256:" + sha256_bytes(str(clean[field]).encode())
        out.append(clean)
    return out


def hook_log_offset(path):
    try:
        return pathlib.Path(path).stat().st_size
    except OSError:
        return 0


def hook_metrics_since(path, offset):
    """Read only numeric stage timings from appended local hook diagnostics; never return prompt/code fields."""
    try:
        with pathlib.Path(path).open("rb") as f:
            f.seek(offset)
            data = f.read().decode("utf-8", errors="replace")
    except OSError:
        return {"hook_ms": None, "sync_ms": None, "retrieve_ms": None}
    records = []
    for line in data.splitlines():
        try:
            rec = json.loads(line)
        except ValueError:
            continue
        if isinstance(rec, dict):
            records.append(rec)
    out = {}
    for output_key, source_key in (("hook_ms", "total_ms"), ("sync_ms", "sync_ms"), ("retrieve_ms", "retrieve_ms")):
        values = [r.get(source_key) for r in records]
        known = bool(values) and all(isinstance(v, (int, float)) and not isinstance(v, bool) and math.isfinite(v) and v >= 0
                                     for v in values)
        out[output_key] = round(sum(values), 3) if known else None
    return out


def progress_report(sidecar, events):
    """Classifica os eventos com o MESMO código do observador (sidecar progress). None se indisponível."""
    observed = [e for e in events if e.get("after_context") is True]
    if not sidecar or not observed:
        return None
    payload = "\n".join(json.dumps(e) for e in observed) + "\n"
    code, out, err, to = run([sidecar, "progress"], input_text=payload, timeout=30)
    if code != 0 or to:
        return None
    try:
        r = json.loads(out)
    except ValueError:
        return None
    return {k: r.get(k) for k in ("events", "labels", "redundant", "exploratory", "novelty_ratio", "stagnant_windows", "longest_stagnation")} | {"shadow_decisions": len(r.get("shadow_decisions") or [])}


# --------------------------------------------------------------------------- runner

KNOWN_FEATURES = {"adaptive", "brief", "groom", "impact"}  # nomes aceitos em SIDECAR_FEATURES


def parse_arms(spec):
    """OFF | V1 | V2 | NAME=engine[,ENV=val...]; engine None = sem hook (OFF)."""
    arms = []
    for item in spec.split(","):
        if item == "OFF":
            arms.append({"name": "OFF", "engine": None, "env": {}})
        elif item in ("V1", "V2"):
            arms.append({"name": item, "engine": item.lower(), "env": {}})
        elif item == "V3":
            # Hook V3 + `sidecar explore` on PATH: exploration is the intervention, recorded in meta.
            arms.append({"name": item, "engine": "v3", "env": {}, "explore": True})
        elif item == "V3-hook":
            arms.append({"name": item, "engine": "v3", "env": {}, "explore": False})
        elif item in ("CG", "CG-mcp"):
            # Competitor arm: no Sidecar hook; CodeGraph indexed in setup and reachable only by this arm.
            arms.append({"name": item, "engine": None, "env": {}, "codegraph": "mcp" if item == "CG-mcp" else "cli"})
        elif item.startswith(("V2+", "V3+")):
            feats = item[3:].split("+")
            if not feats or any(f not in KNOWN_FEATURES for f in feats):
                raise LabError(f"feature desconhecida em {item}: use {sorted(KNOWN_FEATURES)}")
            engine = item[:2].lower()
            arms.append({"name": item, "engine": engine, "env": {"SIDECAR_FEATURES": ",".join(feats)}, "explore": engine == "v3"})
        else:
            raise LabError(f"braço desconhecido: {item} (use OFF, V1, V2, V3, V3-hook, V2+<feature>, V3+<feature>, CG, CG-mcp)")
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


def verify_final_freeze(args):
    """Require an explicitly identified clean revision and binary before opening the real holdout."""
    revision = getattr(args, "frozen_revision", None)
    binary_hash = getattr(args, "frozen_binary_sha256", None)
    if not args.yes_spend or not revision or not binary_hash or not args.sidecar:
        raise LabError("holdout final exige --yes-spend, --frozen-revision e --frozen-binary-sha256")
    root = HERE.parent.parent
    code, head, _, _ = run(["git", "rev-parse", "HEAD"], cwd=root)
    status_code, status, _, _ = run(["git", "status", "--porcelain"], cwd=root)
    if code != 0 or status_code != 0 or status.strip() or head.strip() != revision:
        raise LabError("holdout final exige revisão congelada igual ao HEAD e checkout limpo")
    if not pathlib.Path(args.sidecar).is_file() or sha256_file(args.sidecar) != binary_hash:
        raise LabError("hash do binário não corresponde ao candidato congelado")
    verify_agent_sandbox()


def execute_one(agent, task, tdir, arm, rep, args, home):
    with tempfile.TemporaryDirectory(prefix="lab-run-") as tmp:
        work = prepare_workspace(task, pathlib.Path(tmp) / "work")
        sidecar_home = pathlib.Path(tmp) / "sidecar-home"
        sidecar_home.mkdir()
        if tdir.resolve() in pathlib.Path(work).resolve().parents or str(tdir.resolve()) in str(work):
            raise LabError("oráculo dentro do workspace do agente")
        before = changed_paths(work)
        if before:
            raise LabError("checkout não está limpo")
        env = {k: os.environ[k] for k in ("PATH", "HOME", "LANG") if k in os.environ}
        env.update({"SIDECAR_HOME": str(sidecar_home), "GOPROXY": "off", "GOFLAGS": "-mod=mod"})
        if arm["engine"]:
            env["SIDECAR_ENGINE"] = arm["engine"]
        env.update(arm["env"])
        err = None
        try:
            r = agent.run(task, tdir, work, arm, env, task["prompt"])
        except Exception as e:  # falha do agente é registrada, não escondida
            r = {"wall_s": None, "result": "", "cost_usd": None, "tokens_in": None, "tokens_out": None, "tool_calls": None,
                 "turns": None, "served_models": None, "exit": -1, "timed_out": False, "hook_delivered": None,
                 "injected_chars": None, "incomparable_reasons": [], "stream_error": True, "first_edit_s": None, "tool_events": []}
            err = f"{type(e).__name__}: {e}"
        t0 = time.perf_counter()
        orc = run_oracle(task, tdir, work, answer_text=r["result"])
        v = verdict(orc)
        # Timeout é falha do braço, não contaminação: continua nas taxas (com custo possivelmente desconhecido).
        comparable = not r["incomparable_reasons"] and err is None
        effective_tools = arm.get("tools_allow", tools_allow_for(task, getattr(agent, "tools_allow", None)))
        return {
            "schema": RUN_SCHEMA, "task": task["id"], "split": task["split"], "class": task["class"], "repo": task["repo"],
            "commit": task["commit"], "arm": arm["name"], "rep": rep, "model": getattr(agent, "model", "scripted"),
            "tools_allow": effective_tools,
            "evidence": "simulated" if agent.simulated else "agentic", "verdict": v, "success": v == "pass" and comparable and not r["timed_out"] and r["exit"] == 0 and not r["stream_error"],
            "comparable": comparable, "agent_error": err, "oracle": {k: orc[k] for k in
                ("tests", "scope_ok", "outside_scope", "invariants", "changed", "isolation", "detail")},
            "oracle_s": round(time.perf_counter() - t0, 2), "task_sha256": sha256_file(tdir / "task.json"),
            "n_voltas": r.get("n_voltas", 1), "notas": r.get("notas"), "clarification_status": r.get("clarification_status"),
            "volta_injetada": r.get("volta_injetada"), "tool_events": r.get("tool_events", []), "progress": progress_report(args.sidecar if args else None, r.get("tool_events", [])),
            "sufficiency": r.get("sufficiency"), "token_origins": r.get("token_origins"),
            **{k: r.get(k) for k in ("wall_s", "cost_usd", "tokens_in", "tokens_out", "tool_calls", "turns",
               "served_models", "exit", "timed_out", "hook_delivered", "injected_chars", "incomparable_reasons",
               "stream_error", "first_edit_s", "input_uncached", "cache_read", "cache_create", "output",
               "tool_result_tokens", "index_ms", "hook_ms", "sync_ms", "retrieve_ms", "resident_context_tokens", "client_wall_s")},
        }


def cmd_run(args):
    splits = set(args.splits.split(","))
    if "holdout" in splits and not args.final:
        raise LabError("holdout selado: use --final (uma única avaliação final, sem tuning depois)")
    if "holdout" in splits and args.agent == "scripted" and CORPUS.resolve() == (HERE / "corpus").resolve():
        raise LabError("agente simulado não pode abrir o holdout real")
    if "holdout" in splits and args.agent == "claude":
        verify_final_freeze(args)
    manifest = build_manifest()
    wanted = set(args.tasks.split(",")) if args.tasks else {t["id"] for t in manifest["tasks"]}
    tasks = [t for t in manifest["tasks"] if t["id"] in wanted and t["split"] in splits]
    if not tasks:
        raise LabError("nenhuma tarefa selecionada")
    unvalidated = [t["id"] for t in tasks if not t["validated"]]
    if unvalidated and not args.allow_unvalidated:
        raise LabError("tarefas sem validation.json ok (rode validate-task): " + ", ".join(unvalidated))
    arms = parse_arms(args.arms)
    requested_tools = args.tools_allow.split(",") if args.tools_allow is not None else None
    offset = getattr(args, "rep_offset", 0)
    order = [(t, a, r + offset) for t, a, r in schedule([t["id"] for t in tasks], arms, args.reps, args.seed + offset)]
    n = len(order)
    out_dir = pathlib.Path(args.out_dir)
    agent_auth = None
    if args.agent == "scripted":
        agent = ScriptedAgent(args.scripted_behaviour)
    else:
        if args.agent != "claude":
            raise LabError("agentes: claude | scripted")
        if not args.model or not re.match(r"^claude-[a-z0-9.-]+$", args.model):
            raise LabError("--model deve ser um ID explícito (ex.: claude-haiku-5-5), nunca um alias")
        if args.yes_spend and args.max_total_usd is None:
            raise LabError("--max-total-usd é obrigatório para execução paga")
        for need in ("claude", "git"):
            if not shutil.which(need):
                raise LabError(f"comando ausente: {need}")
        agent_isolation = verify_agent_sandbox()
        if not args.sidecar or not pathlib.Path(args.sidecar).is_file():
            raise LabError("--sidecar (binário) obrigatório para o agente claude")
        deny = [HERE.parent.parent, out_dir]
        reject_sensitive_mounts(["/usr", "/etc"], deny)
        tmp_root = pathlib.Path(tempfile.gettempdir()).resolve()
        for d in deny:
            if d.resolve() == tmp_root or d.resolve() in tmp_root.parents or d.resolve() == pathlib.Path("/"):
                raise LabError(f"--out-dir/repositório não pode ser {d}: bloquearia o workspace do agente. Use um subdiretório")
        if any(a.get("codegraph") for a in arms) and not getattr(args, "codegraph", None):
            raise LabError("braços CG/CG-mcp exigem --codegraph DIR (pacote fixado, ex.: @colbymchenry/codegraph@1.6.2)")
        if getattr(args, "codegraph", None):
            codegraph_shim(args.codegraph)
            reject_sensitive_mounts([args.codegraph], deny)
        agent = ClaudeAgent(args.model, str(pathlib.Path(args.sidecar).resolve()), args.credentials, args.max_budget,
                            args.timeout, requested_tools, deny_paths=deny, turns=getattr(args, "turns", 1),
                            codegraph=getattr(args, "codegraph", None))
        est = n * args.max_budget
        if args.yes_spend:
            agent_auth = verify_agent_auth(args.credentials)
        if not args.yes_spend:
            print(f"Dry run: {n} execuções ({len(tasks)} tarefas × {len(arms)} braços × {args.reps}); "
                  f"teto teórico US$ {est:.2f}; modelo {args.model}. Use --yes-spend para executar.")
            return 0
    holdout_guard(tasks, args.final, "run:" + args.arms, getattr(args, "allow_repeat_final", False))
    out_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    runs, spend = out_dir / "runs.jsonl", out_dir / "spend.jsonl"
    meta = {"schema": "lab-meta/1", "manifest": manifest, "arms": [a["name"] for a in arms], "seed": args.seed,
            "model_requested": getattr(agent, "model", None), "order": [[t, a["name"], r] for t, a, r in order],
            "tools_allow_by_repo": {task["repo"]: tools_allow_for(task, requested_tools) for task in tasks},
            "explore_arms": {a["name"]: arm_extra_tools(a) for a in arms if arm_extra_tools(a)},
            "codegraph": {"package": str(pathlib.Path(args.codegraph).resolve()) if getattr(args, "codegraph", None) else None,
                          "version": codegraph_version(args.codegraph) if getattr(args, "codegraph", None) else None,
                          "telemetry": "disabled (CODEGRAPH_TELEMETRY=0)"},
            "isolation": sandbox_prefix()[1], "oracle_network_isolation": sandbox_prefix()[1],
            "agent_filesystem_isolation": agent_isolation if args.agent == "claude" else "simulated-unrestricted",
            "agent_auth_method": agent_auth if args.agent == "claude" else None,
            "agent_network_env": sorted(sandbox_network_env()[0]) if args.agent == "claude" else [],
            "sidecar_sha256": sha256_file(args.sidecar) if args.sidecar else None,
            "frozen_revision": getattr(args, "frozen_revision", None),
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
            if args.max_total_usd is not None and spend_total(spend) + args.max_budget > args.max_total_usd + 1e-9:
                print("teto de custo atingido; parando", file=sys.stderr)
                break
            task, tdir = load_task(tid)
            if task["split"] == "holdout":
                sealed_task = next(t for t in manifest["tasks"] if t["id"] == tid)
                actual = (sha256_file(tdir / "task.json"), tree_sha256(tdir / "oracle") if (tdir / "oracle").is_dir() else None,
                          sha256_file(tdir / "reference.patch") if (tdir / "reference.patch").is_file() else None)
                expected = (sealed_task.get("task_sha256"), sealed_task.get("oracle_sha256"), sealed_task.get("reference_sha256"))
                if actual != expected:
                    raise LabError("holdout difere do manifesto selado; execução interrompida")
            arm = dict(arm, tools_allow=tools_allow_for(task, requested_tools) + arm_extra_tools(arm))
            rec = execute_one(agent, task, tdir, arm, rep, args, home)
            with runs.open("a") as f:
                f.write(json.dumps(rec, ensure_ascii=False) + "\n")
            with spend.open("a") as f:
                known = rec["cost_usd"] is not None
                # Custo desconhecido (timeout, exceção) conta como o teto por execução: o teto total é conservador.
                f.write(json.dumps({"task": tid, "arm": arm["name"], "model": rec["model"],
                                    "cost_usd": rec["cost_usd"] if known else args.max_budget, "estimated": not known}) + "\n")
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
    complete = lambda key: bool(ok) and all(r.get(key) is not None for r in ok)
    complete_success_wall = bool(succ) and all(r.get("wall_s") is not None for r in succ)
    progress = [r["progress"] for r in ok if r.get("progress")]
    summary = {
        "runs": len(rows), "comparable": len(ok), "excluded": len(rows) - len(ok), "passed": len(succ),
        "pass_rate": round(len(succ) / len(ok), 3) if ok else None,
        "hidden_tests_passed": sum(1 for r in ok if r["oracle"]["tests"] == "pass"),
        "scope_violations": sum(1 for r in ok if not r["oracle"]["scope_ok"]),
        "scope_violations_non_test": sum(1 for r in ok if non_test_outside(r)),
        "invariant_violations": sum(1 for r in ok for i in r["oracle"]["invariants"] if not i["ok"]),
        "infra_errors": sum(1 for r in rows if r["verdict"] == "error"),
        "cost_total": round(sum(cost), 4) if complete("cost_usd") else None,
        "cost_per_success": round(sum(cost) / len(succ), 4) if succ and cost and len(cost) == len(ok) else None,
        "wall_per_success_p50": pct([r["wall_s"] for r in succ], 0.5) if complete_success_wall else None,
        "wall_p95": pct(wall, 0.95) if complete("wall_s") else None,
        "tool_calls_mean": round(statistics.mean([r["tool_calls"] for r in ok]), 2) if complete("tool_calls") else None,
        "injected_chars_mean": round(statistics.mean([r["injected_chars"] for r in ok]), 1) if complete("injected_chars") else None,
        "models_served": sorted({m for r in rows for m in (r["served_models"] or [])}),
        "cost_unknown_runs": sum(1 for r in ok if r["cost_usd"] is None),
        "progress_runs": len(progress),
        "redundant_calls_mean": round(statistics.mean([p["redundant"] for p in progress]), 2)
        if progress and all(p.get("redundant") is not None for p in progress) else None,
        "stagnant_runs": sum(1 for r in ok if r.get("progress") and r["progress"]["stagnant_windows"] > 0),
    }
    suff = [r["sufficiency"] for r in ok if r.get("sufficiency")]
    if suff:
        # Proxy of allocation/recall only: "moved_on" is not correctness; read it next to pass_rate.
        keys = list(sufficiency.BUCKETS) + ["total", "concurrent", "redundant_rereads", "stale_reads_after_edit",
                                            "deliveries_without_effect"]
        summary["sufficiency"] = {k: (sum(x.get(k) or 0 for x in suff) if all(x.get(k) is not None for x in suff) else None)
                                  for k in keys}
        summary["sufficiency"]["runs_with_report"] = len(suff)
        summary["sufficiency"]["moved_on_and_passed"] = sum(1 for r in ok if r.get("sufficiency") and r["success"]
                                                            and (r["sufficiency"].get("moved_on") or 0) > 0)
    optional = ("input_uncached", "cache_read", "cache_create", "output", "tool_result_tokens", "index_ms", "hook_ms",
                "sync_ms", "retrieve_ms", "resident_context_tokens")
    measured = [key for key in optional if any(r.get(key) is not None for r in ok)]
    if measured:
        summary["measured"] = {key: round(sum(r[key] for r in ok), 3) if complete(key) else None for key in measured}
    return summary


METRICS = (("pass_rate", "taxa de aprovação (estrita: teste+escopo+invariantes)", "pass"), ("hidden_pass", "testes ocultos passaram (sem escopo)", "hid"), ("tool_calls", "chamadas de ferramenta", "calls"), ("tokens", "tokens (entrada+saída)", "tok"),
           ("cost_usd", "custo US$", "cost"), ("wall_s", "tempo s", "wall"), ("redundant", "chamadas redundantes", "red"))


def run_metrics(r):
    prog = r.get("progress") or {}
    tin, tout = r.get("tokens_in"), r.get("tokens_out")
    return {"pass_rate": 1.0 if r["success"] else 0.0, "hidden_pass": 1.0 if (r.get("oracle") or {}).get("tests") == "pass" else 0.0,
            "tool_calls": r.get("tool_calls"),
            "tokens": (tin + tout) if isinstance(tin, int) and isinstance(tout, int) else None,
            "cost_usd": r.get("cost_usd"), "wall_s": r.get("wall_s"), "redundant": prog.get("redundant")}


def compare_arms(rows, arm, base, n_boot=2000, seed=7):
    """Δ% de `arm` contra `base`, pareando por (tarefa, repetição) entre execuções comparáveis dos dois braços.
    IC95 por bootstrap sobre as TAREFAS (as repetições de uma tarefa não são independentes). Métrica sem dado em
    algum lado do par fica de fora, e a contagem de pares é reportada."""
    idx = {}
    for r in rows:
        if r["comparable"]:
            idx.setdefault(r["arm"], {})[(r["task"], r["rep"])] = r
    pairs = [(k, idx[arm][k], idx[base][k]) for k in idx.get(arm, {}) if k in idx.get(base, {})]
    out = {"arm": arm, "base": base, "pairs": len(pairs), "tasks": len({k[0] for k, _, _ in pairs}), "metrics": {}}
    rnd = random.Random(seed)
    for key, label, _ in METRICS:
        by_task = {}
        for k, a, b in pairs:
            va, vb = run_metrics(a)[key], run_metrics(b)[key]
            if va is not None and vb is not None:
                by_task.setdefault(k[0], []).append((va, vb))
        tasks = list(by_task)
        if not tasks:
            out["metrics"][key] = None
            continue
        ma = lambda ts: statistics.mean(statistics.mean(x for x, _ in by_task[t]) for t in ts)
        mb = lambda ts: statistics.mean(statistics.mean(y for _, y in by_task[t]) for t in ts)
        base_mean, arm_mean = mb(tasks), ma(tasks)
        entry = {"label": label, "base": round(base_mean, 4), "arm": round(arm_mean, 4), "n_tasks": len(tasks),
                 "delta_pct": round(100 * (arm_mean - base_mean) / base_mean, 1) if base_mean else None, "ci95_pct": None}
        if len(tasks) >= 5 and base_mean:
            boots = []
            for _ in range(n_boot):
                sample = rnd.choices(tasks, k=len(tasks))
                bm = mb(sample)
                if bm:
                    boots.append(100 * (ma(sample) - bm) / bm)
            boots.sort()
            if boots:
                entry["ci95_pct"] = [round(boots[int(0.025 * len(boots))], 1), round(boots[int(0.975 * len(boots)) - 1], 1)]
        lo_hi = entry["ci95_pct"]
        entry["reading"] = "inconclusivo" if (lo_hi is None or (lo_hi[0] <= 0 <= lo_hi[1]) or len(tasks) < 10) else ("melhor/menor" if entry["delta_pct"] < 0 else "maior")
        out["metrics"][key] = entry
    return out


def comparison_plan(arms):
    """Pares (braço, referência) que respondem a uma pergunta: tudo contra OFF; V2 e seus componentes contra V1; componentes
    contra o V2 puro. Nunca OFF como braço (seria o inverso de outro par) nem um braço contra si mesmo."""
    plan = []
    for a in arms:
        if a != "OFF" and "OFF" in arms:
            plan.append((a, "OFF"))
    for a in arms:
        if a.startswith("V2") and "V1" in arms:
            plan.append((a, "V1"))
    for a in arms:
        if a.startswith("V2+") and "V2" in arms:
            plan.append((a, "V2"))
    for a in arms:
        if a.startswith("V3") and "CG" in arms:
            plan.append((a, "CG"))  # competitive question: same tasks, same base tools
        if a.startswith("V3") and "CG-mcp" in arms:
            plan.append((a, "CG-mcp"))
    for a in arms:
        if a.startswith("V3") and "V2" in arms:
            plan.append((a, "V2"))  # the V3 promotion question is V3 against V2, not only against OFF
        if a.startswith("V3") and a != "V3" and "V3" in arms:
            plan.append((a, "V3"))
    return plan


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
        sec["comparisons"] = [compare_arms(sel, a, b) for a, b in comparison_plan(list(sec["arms"]))]
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
        L += ["| braço | exec. | comparáveis | pass | taxa | ocultos ok | escopo (não-teste) | invariantes | custo/correta | p50 s/correta | tool calls | redundantes | injetado |",
              "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"]
        for a, s in sec["arms"].items():
            L.append(f"| {a} | {s['runs']} | {s['comparable']} | {s['passed']} | {s['pass_rate']} | {s['hidden_tests_passed']} | {s['scope_violations']} ({s['scope_violations_non_test']}) | "
                     f"{s['invariant_violations']} | {s['cost_per_success']} | {s['wall_per_success_p50']} | {s['tool_calls_mean']} | {s['redundant_calls_mean']} | {s['injected_chars_mean']} |")
        L.append("")
        measured_labels = {"input_uncached": "entrada sem cache", "cache_read": "cache lido", "cache_create": "cache criado",
                           "output": "saída", "tool_result_tokens": "tokens de resultados de ferramentas",
                           "index_ms": "indexação ms", "hook_ms": "hook ms", "sync_ms": "sync ms",
                           "retrieve_ms": "retrieval ms", "resident_context_tokens": "contexto residente tokens"}
        measured_keys = [key for key in measured_labels if any(s.get("measured", {}).get(key) is not None for s in sec["arms"].values())]
        if measured_keys:
            L += ["### Contadores e tempos observados", "", "| métrica | braço | total nas execuções comparáveis |", "|---|---|---:|"]
            for key in measured_keys:
                for arm, summary in sec["arms"].items():
                    if key in summary.get("measured", {}):
                        L.append(f"| {measured_labels[key]} | {arm} | {summary['measured'][key]} |")
            L.append("")
        suff_arms = {a: s["sufficiency"] for a, s in sec["arms"].items() if s.get("sufficiency")}
        if suff_arms:
            L += ["### Suficiência após cada entrega do Sidecar (próximo ato por thread; NÃO é correção)", "",
                  "| braço | entregas | moved_on | read_returned | read_named | read_not_returned | search | explore_again | errored | releituras redundantes | leitura após edição | sem efeito |",
                  "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"]
            for a, x in suff_arms.items():
                L.append(f"| {a} | {x['total']} | {x['moved_on']} | {x['read_returned']} | {x['read_named']} | {x['read_not_returned']} | "
                         f"{x['search']} | {x['explore_again']} | {x['errored']} | {x['redundant_rereads']} | {x['stale_reads_after_edit']} | {x['deliveries_without_effect']} |")
            L.append("")
        for a, p in sec["paired_vs_OFF"].items():
            L.append(f"- {a} vs OFF: {p['pairs']} pares, Δpass {p['d_pass_mean']} IC95 {p['d_pass_ci95']}, Δcusto {p['d_cost_mean']} → **{p['conclusion']}**")
        L.append("")
    for ev, sec in rep["sections"].items():
        if ev == "offline_retrieval" or not sec.get("comparisons"):
            continue
        L += [f"## Δ% por métrica ({ev}): braço contra referência, pareado por tarefa e repetição", "",
              "Negativo em tool calls/tokens/custo/tempo/redundantes = menos. `inconclusivo` = IC95 cruza zero ou menos de 10 tarefas.", ""]
        for c in sec["comparisons"]:
            L += [f"### {c['arm']} vs {c['base']} ({c['pairs']} pares, {c['tasks']} tarefas)", "",
                  "| métrica | referência | braço | Δ% | IC95 Δ% | leitura |", "|---|---:|---:|---:|---|---|"]
            for key, label, _ in METRICS:
                m = c["metrics"].get(key)
                if m:
                    L.append(f"| {m['label']} | {m['base']} | {m['arm']} | {m['delta_pct']} | {m['ci95_pct']} | {m['reading']} |")
            L.append("")
    L += ["## Limites", ""] + [f"- {x}" for x in rep["limits"]]
    return "\n".join(L) + "\n"


def cmd_report(args):
    rows = [json.loads(l) for path in args.runs for l in pathlib.Path(path).read_text().splitlines() if l.strip()]
    keys = [(r["task"], r["arm"], r["rep"]) for r in rows]
    if len(keys) != len(set(keys)):
        raise LabError("runs repetidos entre os arquivos (mesma tarefa, braço e repetição): use --rep-offset distintos")
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
    p = sub.add_parser("validate-corpus"); p.add_argument("--include-holdout", action="store_true")
    p = sub.add_parser("manifest"); p.add_argument("--out", default=str(CORPUS / "manifest.json"))
    p.add_argument("--include-holdout", action="store_true", help="refresh sealed holdout metadata explicitly")
    p = sub.add_parser("run")
    p.add_argument("--agent", default="claude"); p.add_argument("--model"); p.add_argument("--sidecar")
    p.add_argument("--arms", default="OFF,V1,V2"); p.add_argument("--tasks"); p.add_argument("--splits", default="development,validation")
    p.add_argument("--codegraph", help="installed CodeGraph package dir (npm --prefix DIR) for the CG / CG-mcp arms")
    p.add_argument("--reps", type=int, default=1); p.add_argument("--seed", type=int, default=11)
    p.add_argument("--out-dir", required=True); p.add_argument("--credentials", default=str(pathlib.Path.home() / ".claude" / ".credentials.json"))
    p.add_argument("--max-budget", type=float, default=0.5, help="teto por execução (US$)")
    p.add_argument("--max-total-usd", type=float, default=None); p.add_argument("--timeout", type=int, default=600)
    p.add_argument("--tools-allow", default=None, help="lista explícita que substitui a política padrão por linguagem")
    p.add_argument("--yes-spend", action="store_true"); p.add_argument("--final", action="store_true")
    p.add_argument("--frozen-revision"); p.add_argument("--frozen-binary-sha256")
    p.add_argument("--allow-unvalidated", action="store_true"); p.add_argument("--allow-repeat-final", action="store_true"); p.add_argument("--turns", type=int, choices=(1, 2), default=1, help="2 = protocolo de duas voltas com usuário simulado (R4/R5)"); p.add_argument("--rep-offset", type=int, default=0, help="índice da primeira repetição; permite processos paralelos com repetições distintas"); p.add_argument("--scripted-behaviour", default="reference")
    p = sub.add_parser("report"); p.add_argument("runs", nargs="+", help="um ou mais runs.jsonl (repetições de processos diferentes são juntadas)"); p.add_argument("--md"); p.add_argument("--json"); p.add_argument("--offline")
    a = ap.parse_args(argv)
    try:
        if a.cmd == "validate-task":
            r = validate_task(a.id)
            print(json.dumps({k: r[k] for k in ("id", "ok", "base_verdict")}))
            return 0 if r["ok"] else 1
        if a.cmd == "validate-corpus":
            bad = 0
            held = set()
            if a.include_holdout and CORPUS.resolve() == (HERE / "corpus").resolve():
                raise LabError("validação do holdout real está bloqueada enquanto o experimento não estiver autorizado")
            if not a.include_holdout:
                sealed_path = CORPUS / "manifest.json"
                if not sealed_path.is_file():
                    raise LabError("manifesto selado ausente; validação segura do corpus requer inventário congelado")
                inventory = load_json(sealed_path).get("tasks", [])
                known = {t["id"] for t in inventory}
                if {d.name for d in task_dirs()} - known:
                    raise LabError("inventário difere do manifesto selado; recusei validar tarefas desconhecidas")
                held = {t["id"] for t in inventory if t.get("split") == "holdout"}
            for d in task_dirs():
                if d.name in held:
                    continue
                r = validate_task(d.name)
                print(f"{d.name:28s} {'OK' if r['ok'] else 'FALHOU'} base={r['base_verdict']}")
                bad += 0 if r["ok"] else 1
            return 1 if bad else 0
        if a.cmd == "manifest":
            m = build_manifest(include_holdout=a.include_holdout)
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
