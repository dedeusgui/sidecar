#!/usr/bin/env python3
"""E2 offline (sem modelo): cobertura dos fatos do oráculo de navegação no que cada braço ENTREGA.

Para cada tarefa de bench/lab/corpus-nav: roda o hook V2 e V3 (via `sidecar explain --json`) e `sidecar explore`
no checkout fixado e mede quais fatos de `oracle.answer_must_mention` aparecem no texto entregue, e quantos caracteres
custou. Isso NÃO é resultado agentic: mede se a evidência necessária chegou, não se o agente respondeu certo.

Uso: python3 bench/v3/e2_offline.py --sidecar BIN --checkout DIR [--json OUT] [--md OUT]
"""
import argparse
import json
import os
import pathlib
import subprocess
import tempfile

HERE = pathlib.Path(__file__).resolve().parent
NAV = HERE.parent / "lab" / "corpus-nav"


def covered(text, facts):
    t = (text or "").lower()
    hit = [f if isinstance(f, str) else "|".join(f) for f in facts if any(a.lower() in t for a in ([f] if isinstance(f, str) else f))]
    return hit


def run(cmd, stdin=None, env=None):
    r = subprocess.run(cmd, input=stdin, capture_output=True, text=True, timeout=600, env=env)
    if r.returncode != 0:
        raise SystemExit(f"{cmd[:3]} falhou: {r.stderr[-400:]}")
    return r.stdout


def main(argv=None):
    p = argparse.ArgumentParser()
    p.add_argument("--sidecar", required=True)
    p.add_argument("--checkout", required=True)
    p.add_argument("--codegraph", help="codegraph CLI (ex.: node_modules/.bin/codegraph @1.6.2); adds its explore as an arm")
    p.add_argument("--json")
    p.add_argument("--md")
    a = p.parse_args(argv)
    arms = ("v2", "v3", "explore") + (("codegraph",) if a.codegraph else ())
    rows = []
    with tempfile.TemporaryDirectory(prefix="e2-offline-") as home:
        env = dict(os.environ, SIDECAR_HOME=home, SIDECAR_FEATURES="")
        for tdir in sorted((NAV / "tasks").iterdir()):
            task = json.loads((tdir / "task.json").read_text())
            if task["split"] == "holdout":
                continue
            root = str(pathlib.Path(a.checkout) / task["repo"])
            facts = task["oracle"]["answer_must_mention"]
            row = {"task": task["id"], "split": task["split"], "repo": task["repo"], "facts": len(facts)}
            for arm in ("v2", "v3"):
                rep = json.loads(run([a.sidecar, "explain", "--engine", arm, "--root", root, "--json"], task["prompt"], env))
                ctx = rep.get("context") or ""
                row[arm] = {"facts": len(covered(ctx, facts)), "chars": len(ctx),
                            "action": (rep.get("v3_decision") or {}).get("action")}
            out = run([a.sidecar, "explore", "--root", root, "--query", task["prompt"]], None, env)
            row["explore"] = {"facts": len(covered(out, facts)), "chars": len(out), "missing": [
                f if isinstance(f, str) else "|".join(f) for f in facts if (f if isinstance(f, str) else "|".join(f)) not in covered(out, facts)]}
            if a.codegraph:
                # Same question on a throwaway clone: `codegraph init` writes .codegraph/ into the tree. Telemetry off.
                with tempfile.TemporaryDirectory(prefix="e2-cg-") as clone:
                    subprocess.run(["git", "clone", "-q", root, clone], check=True)
                    cg_env = dict(env, CODEGRAPH_TELEMETRY="0")
                    run([a.codegraph, "init", clone], None, cg_env)
                    r = subprocess.run([a.codegraph, "explore", task["prompt"]], cwd=clone, capture_output=True, text=True,
                                       timeout=600, env=cg_env)
                    cg = r.stdout if r.returncode == 0 else ""
                row["codegraph"] = {"facts": len(covered(cg, facts)), "chars": len(cg), "missing": [
                    f if isinstance(f, str) else "|".join(f) for f in facts if (f if isinstance(f, str) else "|".join(f)) not in covered(cg, facts)]}
            rows.append(row)
    tot = lambda arm, k: sum(r[arm][k] for r in rows)
    nf = sum(r["facts"] for r in rows)
    summary = {arm: {"facts_covered": tot(arm, "facts"), "facts_total": nf, "chars_total": tot(arm, "chars"),
                     "tasks_all_facts": sum(1 for r in rows if r[arm]["facts"] == r["facts"])} for arm in arms}
    lines = ["# E2 offline — fatos do oráculo presentes no que cada braço entrega (sem modelo)", "",
             "| tarefa | split | fatos | V2 hook | V3 hook (ação) | explore | chars V2 / V3 / explore | faltam no explore |", "|---|---|---:|---:|---:|---:|---|---|"]
    if a.codegraph:
        lines[2] = "| tarefa | split | fatos | V2 hook | V3 hook (ação) | sidecar explore | codegraph explore | chars V2 / V3 / explore / codegraph | faltam (sidecar · codegraph) |"
        lines[3] = "|---|---|---:|---:|---:|---:|---:|---|---|"
    for r in rows:
        if a.codegraph:
            lines.append(f"| {r['task']} | {r['split']} | {r['facts']} | {r['v2']['facts']} | {r['v3']['facts']} ({r['v3']['action']}) | "
                         f"{r['explore']['facts']} | {r['codegraph']['facts']} | {r['v2']['chars']} / {r['v3']['chars']} / {r['explore']['chars']} / {r['codegraph']['chars']} | "
                         f"{', '.join(r['explore']['missing']) or '—'} · {', '.join(r['codegraph']['missing']) or '—'} |")
            continue
        lines.append(f"| {r['task']} | {r['split']} | {r['facts']} | {r['v2']['facts']} | {r['v3']['facts']} ({r['v3']['action']}) | "
                     f"{r['explore']['facts']} | {r['v2']['chars']} / {r['v3']['chars']} / {r['explore']['chars']} | {', '.join(r['explore']['missing']) or '—'} |")
    lines += ["", "| braço | fatos cobertos | tarefas com todos os fatos | caracteres totais |", "|---|---:|---:|---:|"]
    for arm, s in summary.items():
        lines.append(f"| {arm} | {s['facts_covered']}/{s['facts_total']} | {s['tasks_all_facts']}/{len(rows)} | {s['chars_total']} |")
    lines += ["", "Leitura: cobertura de fatos na evidência é condição para economizar exploração, não prova de resposta correta nem de economia end-to-end."]
    text = "\n".join(lines) + "\n"
    print(text)
    if a.md:
        pathlib.Path(a.md).write_text(text)
    if a.json:
        pathlib.Path(a.json).write_text(json.dumps({"rows": rows, "summary": summary}, indent=2) + "\n")


if __name__ == "__main__":
    main()
