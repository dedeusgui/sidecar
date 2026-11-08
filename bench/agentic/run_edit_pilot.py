#!/usr/bin/env python3
"""Small, isolated Codex editing pilot for a pinned public chi checkout.

The benchmark is a development experiment, not a blind or general productivity test.
The oracle is added only after the agent exits and runs without network in bubblewrap.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import random
import shutil
import signal
import stat
import subprocess
import sys
import tarfile
import tempfile
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "agent"))
import run_agents  # noqa: E402


ROOT = Path(__file__).resolve().parents[2]
COMMIT = "9f8adb1aabaa8a91180a821111d3f8e668d1c0bf"
TARGET = "middleware/request_size.go"
ORACLE = ROOT / "bench/agentic/oracles/chi_request_size_test.go.txt"
PROMPT = (
    "Atualize o middleware RequestSize do chi: quando o Content-Length declarado for "
    "maior que o limite em bytes, responda HTTP 413 imediatamente, sem executar o "
    "próximo handler. Para tamanho igual ou menor, mantenha o comportamento atual. "
    "Quando o tamanho for desconhecido, preserve a limitação por leitura existente. "
    "Mantenha a API pública e altere somente middleware/request_size.go. "
    "Não acesse a rede. Ao terminar, descreva brevemente o que fez."
)


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def snapshot(repo):
    """Snapshot files, directories and symlinks, including ignored/untracked paths."""
    result = {}
    for root, dirs, files in os.walk(repo, followlinks=False):
        for name in dirs + files:
            path = Path(root) / name
            rel = path.relative_to(repo).as_posix()
            mode = path.lstat().st_mode
            if stat.S_ISLNK(mode):
                result[rel] = ("link", os.readlink(path))
            elif stat.S_ISREG(mode):
                result[rel] = ("file", stat.S_IMODE(mode), sha256(path))
            elif stat.S_ISDIR(mode):
                result[rel] = ("dir", stat.S_IMODE(mode))
            else:
                result[rel] = ("special", mode)
    return result


def checkout_archive(source, destination):
    """Export exactly the pinned tree; do not expose Git objects, refs or history."""
    destination.mkdir()
    archive = destination.parent / (destination.name + ".tar")
    with archive.open("xb") as out:
        subprocess.run(["git", "-C", str(source), "archive", "--format=tar", COMMIT], stdout=out, check=True)
    with tarfile.open(archive) as data:
        data.extractall(destination, filter="data")
    archive.unlink()


def agent_sandbox(work, model, prompt):
    node = Path(shutil.which("node") or "").resolve()
    codex = Path(shutil.which("codex") or "").resolve()
    node_root = node.parent.parent
    if not node.is_file() or not codex.is_file() or not codex.is_relative_to(node_root):
        raise ValueError("Codex and Node must be installed in the same Node distribution")
    auth = Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex"))) / "auth.json"
    if not auth.is_file():
        raise ValueError("Codex auth.json not found for isolated client")
    resolver = Path("/etc/resolv.conf").resolve()
    resolver_mount = []
    if resolver.is_relative_to("/run"):
        parents = list(reversed([p for p in resolver.parents if p == Path('/run') or p.is_relative_to('/run')]))
        for parent in parents:
            resolver_mount += ["--dir", str(parent)]
        resolver_mount += ["--ro-bind", str(resolver), str(resolver)]
    elif not resolver.is_relative_to("/etc"):
        raise ValueError("unsupported resolver path for isolated client")
    return ["bwrap", "--die-with-parent", "--unshare-pid", "--new-session", "--clearenv", "--ro-bind", "/usr", "/usr",
            "--ro-bind", "/lib", "/lib", "--ro-bind", "/lib64", "/lib64", "--ro-bind", "/bin", "/bin",
            "--ro-bind", "/etc", "/etc", "--ro-bind", str(node_root), "/opt/node",
            *resolver_mount,
            "--bind", str(work), "/work", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
            "--dir", "/tmp/home", "--dir", "/codex", "--ro-bind", str(auth), "/codex/auth.json",
            "--setenv", "HOME", "/tmp/home", "--setenv", "CODEX_HOME", "/codex",
            "--setenv", "PATH", "/opt/node/bin:/usr/local/go/bin:/usr/bin:/bin", "--chdir", "/work",
            "/opt/node/bin/codex", "exec", "--json", "--ephemeral", "--ignore-user-config", "--skip-git-repo-check",
            "-c", 'approval_policy="never"', "-s", "workspace-write", "-m", model, "-C", "/work", prompt]


def sandbox_command(repo):
    return ["bwrap", "--unshare-net", "--unshare-pid", "--die-with-parent", "--clearenv",
            "--ro-bind", "/usr", "/usr", "--ro-bind", "/lib", "/lib",
            "--ro-bind", "/lib64", "/lib64", "--ro-bind", "/bin", "/bin",
            "--ro-bind", str(repo), "/work", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
            "--setenv", "HOME", "/tmp", "--setenv", "PATH", "/usr/local/go/bin:/usr/bin:/bin",
            "--setenv", "GOCACHE", "/tmp/go-cache", "--setenv", "GOPATH", "/tmp/go",
            "--setenv", "GOPROXY", "off", "--setenv", "GOTOOLCHAIN", "local", "--setenv", "CGO_ENABLED", "0",
            "--chdir", "/work", "/usr/local/go/bin/go", "test", "-json", "./middleware",
            "-run", "^TestBenchmarkRequestSizeKnownLength$", "-count=1"]


def preflight_agent_sandbox(work):
    """Check the nested tool sandbox without making a model request."""
    cmd = agent_sandbox(work, "unused", "unused")
    cli = cmd.index("/opt/node/bin/codex")
    probe = (
        "import pathlib,socket,subprocess\n"
        "assert not pathlib.Path('/home/dedeusgui/orca/sidecar').exists()\n"
        "assert not pathlib.Path('/tmp/sidecar-real-bench').exists()\n"
        "assert subprocess.run(['git','-C','/work','rev-parse','--is-inside-work-tree'],"
        "stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode != 0\n"
        "p=pathlib.Path('/work/.sidecar-sandbox-probe'); p.write_text('ok'); p.unlink()\n"
        "try:\n s=socket.socket(); s.settimeout(2); result=s.connect_ex(('1.1.1.1',443)); s.close()\n"
        "except OSError:\n result=-1\n"
        "assert result != 0\n"
    )
    command = cmd[:cli] + ["/opt/node/bin/codex", "-c", 'sandbox_mode="workspace-write"',
                           "sandbox", "/usr/bin/python3", "-c", probe]
    code, _out, _err, timed_out = run_limited(command, work, 30)
    if code != 0 or timed_out:
        raise RuntimeError("Codex nested sandbox preflight failed")
    dns = cmd[:cli] + ["/usr/bin/python3", "-c", "import socket; socket.getaddrinfo('chatgpt.com',443)"]
    code, _out, _err, timed_out = run_limited(dns, work, 15)
    if code != 0 or timed_out:
        raise RuntimeError("Codex client DNS preflight failed")


def run_limited(cmd, cwd, timeout, env=None):
    proc = subprocess.Popen(cmd, cwd=cwd, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, start_new_session=True)
    try:
        out, err = proc.communicate(timeout=timeout)
        return proc.returncode, out, err, False
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)
        out, err = proc.communicate()
        return proc.returncode, out, err, True


def score(repo, timeout):
    """Return pass/fail only if go test actually emits the expected package result."""
    test_path = repo / "middleware/sidecar_benchmark_oracle_test.go"
    if test_path.exists() or test_path.is_symlink():
        raise ValueError("oracle path already exists")
    shutil.copyfile(ORACLE, test_path)
    code, out, _err, timed_out = run_limited(sandbox_command(repo), repo, timeout)
    if timed_out:
        return {"status": "error", "reason": "oracle timeout"}
    events = []
    for line in out.splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    terminal = [e for e in events if e.get("Package") == "github.com/go-chi/chi/v5/middleware"
                and not e.get("Test") and e.get("Action") in ("pass", "fail")]
    tests = [e for e in events if e.get("Package") == "github.com/go-chi/chi/v5/middleware"
             and e.get("Test") == "TestBenchmarkRequestSizeKnownLength" and e.get("Action") in ("pass", "fail")]
    if len(terminal) != 1 or len(tests) != 1 or terminal[0]["Action"] != tests[0]["Action"] or (terminal[0]["Action"] == "pass") != (code == 0):
        return {"status": "error", "reason": "oracle infrastructure or build failed"}
    return {"status": terminal[0]["Action"], "reason": None}


def execute(args):
    source = Path(args.repo).resolve()
    sidecar = str(Path(args.sidecar).resolve())
    run_agents.validate_repos({"chi": {"path": str(source), "commit": COMMIT}}, [{"repo": "chi"}])
    if not Path(sidecar).is_file() or not ORACLE.is_file():
        raise ValueError("sidecar binary or oracle missing")
    for cmd in ("codex", "bwrap", "git"):
        if not shutil.which(cmd):
            raise ValueError(f"required command missing: {cmd}")
    if args.timeout < 1 or args.oracle_timeout < 1:
        raise ValueError("timeouts must be positive")
    if not args.yes_spend:
        print("Dry run: 1 tarefa de edição × 2 braços Codex; custo monetário indisponível. Use --yes-spend para executar.")
        return
    out = Path(args.out_dir)
    out.mkdir(mode=0o700, parents=True, exist_ok=False)
    version = subprocess.run(["codex", "--version"], capture_output=True, text=True, check=True).stdout.strip()
    order = ["OFF", "ON"]
    random.Random(11).shuffle(order)
    meta = {"task": "CHI-EDIT-01", "repo_commit": COMMIT, "prompt_sha256": hashlib.sha256(PROMPT.encode()).hexdigest(),
            "oracle_sha256": sha256(ORACLE), "sidecar_sha256": sha256(sidecar), "runner_sha256": sha256(__file__),
            "agent_helpers_sha256": sha256(run_agents.__file__),
            "codex_version": version, "model": args.model, "engine": args.engine, "order": order, "timeout_s": args.timeout,
            "oracle_timeout_s": args.oracle_timeout, "transport": "prompt_prefix", "estimated_cost_usd": None,
            "raw_saved": args.save_raw}
    (out / "run-metadata.json").write_text(json.dumps(meta, indent=2) + "\n")
    with tempfile.TemporaryDirectory(prefix="sidecar-edit-pilot-") as temp:
        probe_work = Path(temp) / "preflight"
        checkout_archive(source, probe_work)
        preflight_agent_sandbox(probe_work)
        home = str(Path(temp) / "sidecar-home")
        subprocess.run([sidecar, "index", "--root", str(source)], env=dict(os.environ, SIDECAR_HOME=home, SIDECAR_ENGINE=args.engine),
                       capture_output=True, check=True)
        for arm in order:
            work = Path(temp) / arm
            checkout_archive(source, work)
            before = snapshot(work)
            context, hook_ms = ("", 0.0)
            if arm == "ON":
                context, hook_ms = run_agents.sidecar_context(sidecar, home, str(source), PROMPT, engine=args.engine)
            prompt = (context + "\n\n" if context else "") + PROMPT
            cmd = agent_sandbox(work, args.model, prompt)
            print(f"Executando {arm}...", flush=True)
            started = time.monotonic()
            code, stdout, stderr, timed_out = run_limited(cmd, work, args.timeout)
            elapsed = time.monotonic() - started + hook_ms / 1000
            if args.save_raw:
                raw = out / "raw"
                raw.mkdir(mode=0o700, exist_ok=True)
                if raw.is_symlink() or raw.stat().st_mode & 0o077:
                    raise ValueError("raw directory must be private")
                fd = os.open(raw / f"codex-{arm}.txt", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(fd, "w") as f:
                    f.write(stdout + "\n--- stderr ---\n" + stderr)
            parsed = run_agents.parse_output("codex", stdout)
            after = snapshot(work)
            changed = sorted(path for path in before.keys() | after.keys() if before.get(path) != after.get(path))
            target = work / TARGET
            valid_patch = changed == [TARGET] and target.is_file() and not target.is_symlink()
            oracle = score(work, args.oracle_timeout) if code == 0 and not timed_out and valid_patch else {"status": "not_run", "reason": "agent failure or patch outside scope"}
            rec = {"agent": "codex", "model": args.model, "arm": arm, "case": "CHI-EDIT-01", "repo": "chi",
                   "category": "edit", "rep": 0, "wall_s": round(elapsed, 3), "sidecar_ms": round(hook_ms, 1),
                   "transport": "prompt_prefix" if arm == "ON" else "none",
                   "injected_chars": len(context), "exit": code, "timed_out": timed_out, "changed_files": changed,
                   "oracle": oracle, "success": (None if code != 0 or timed_out or oracle["status"] == "error"
                                                    else oracle["status"] == "pass"), "tokens_in": parsed["tokens_in"],
                   "tokens_out": parsed["tokens_out"], "cost_usd": parsed["cost_usd"], "tool_calls": parsed["tool_calls"]}
            with (out / "runs.jsonl").open("a") as f:
                f.write(json.dumps(rec, ensure_ascii=False) + "\n")
            print(json.dumps(rec, ensure_ascii=False), flush=True)
            if code != 0 or timed_out or oracle["status"] == "error":
                raise RuntimeError("pilot stopped after agent or oracle infrastructure error")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--repo", required=True)
    p.add_argument("--sidecar", required=True)
    p.add_argument("--out-dir", required=True)
    p.add_argument("--model", default="gpt-6-luna")
    p.add_argument("--engine", choices=("v1", "v2"), default="v2")
    p.add_argument("--timeout", type=int, default=180)
    p.add_argument("--oracle-timeout", type=int, default=120)
    p.add_argument("--save-raw", action="store_true", help="save private agent output for audit")
    p.add_argument("--yes-spend", action="store_true")
    args = p.parse_args()
    try:
        execute(args)
    except (ValueError, RuntimeError, OSError, subprocess.SubprocessError) as exc:
        p.exit(1, f"error: {exc}\n")


if __name__ == "__main__":
    main()
