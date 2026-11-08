#!/usr/bin/env python3
"""Simulação instrumentada do A/B: OFF (grep + leitura) contra OFF + bloco do Sidecar V1 ou V2.

Não é um agente de linguagem nem uma medição end-to-end. A política de busca e leitura é fixa e
igual nos três braços; só muda a fonte de candidatos (o bloco de `sidecar explain`). Tempo, chamadas
e tokens vêm de execuções reais de grep, leitura de arquivo e `sidecar explain`. O tempo de modelo
fica de fora. Tokens usam a regra de internal/ctxbuild.EstimateTokens: (caracteres + 3) / 4.

Política (igual nos braços; ON começa pelo bloco do Sidecar):
  1. ON: uma chamada `sidecar explain --json` (SIDECAR_ENGINE=v1|v2). Gabarito presente no bloco
     (caminho e símbolo) conta como identificado sem leitura.
  2. Se faltar gabarito: um grep por termo do prompt e do dicionário PT→EN (até 14 termos).
     Ordenação dos arquivos: soma de idf(termo); termo no caminho vale o dobro. Sem julgamento semântico.
  3. Leitura de até 12 arquivos, 3 por rodada, sem reler arquivo já identificado. Para ao
     identificar todo o gabarito (caminho e símbolo presentes no conteúdo lido).
Limite conhecido: o OFF é uma política de grep, não um agente de linguagem. Não mede a força de um agente real.

Saídas em --out-dir:
  runs.jsonl       registro estrito lido por `sidecar-eval --agentic-report` (campos do benchagent);
  real_runs.jsonl  o mesmo registro, com qualidade, chamadas totais e itens encontrados.

Exemplo:
  python3 -I bench/agentic/instrumented_sim.py --sidecar BIN --home DIR_NOVO \
    --dict internal/lexical/dictionary.go --manifest bench/agentic/manifest.jsonl \
    --repo spring-petclinic=/tmp/sidecar-real-bench/spring-petclinic --repo chi=/tmp/sidecar-real-bench/chi \
    --reps 5 --out-dir bench/agentic --scores-out bench/results/real-repos-scores.json
"""

import argparse
import json
import math
import os
import pathlib
import re
import statistics
import subprocess
import sys
import time
import unicodedata

ARMS = ["OFF", "ON-v1", "ON-v2"]
READ_BATCH = 3       # arquivos lidos por rodada (leituras emitidas em paralelo)
READ_BUDGET = 12     # leituras por execução, iguais em todos os braços
GREP_TERM_CAP = 14   # termos buscados por execução
WEIGHTS = (0.45, 0.30, 0.25)  # qualidade, velocidade, economia de custo e buscas
HTTP_VERBS = ("GET ", "POST ", "PUT ", "DELETE ", "PATCH ", "HEAD ", "OPTIONS ")
STOPWORDS = set(
    "the a an of to for in on and or which what where who how is are does do that this with when "
    "from by it its be as at qual quais onde como que um uma de da do dos das e ou para com sem no "
    "na nos nas se ao aos pelo pela pelos pelas sao".split()
)


def fold(s):
    return "".join(c for c in unicodedata.normalize("NFKD", s) if not unicodedata.combining(c)).lower()


def est_tokens(s):
    return (len(s) + 3) // 4


def load_dictionary(path):
    """Lê o léxico PT→EN de internal/lexical/lexicon_pt_en.tsv (ou dictionary.go legado)."""
    p = pathlib.Path(path)
    if p.name == "dictionary.go":
        tsv_candidate = p.parent / "lexicon_pt_en.tsv"
        if tsv_candidate.exists():
            p = tsv_candidate
    txt = p.read_text(encoding="utf-8")
    fwd = {}
    if "var ptToEn = map[string][]string{" in txt:
        block = txt.split("var ptToEn = map[string][]string{", 1)[1].split("\n}\n", 1)[0]
        for m in re.finditer(r'"([^"]+)":\s*\{([^}]*)\}', block):
            fwd[fold(m.group(1))] = [fold(v) for v in re.findall(r'"([^"]+)"', m.group(2))]
    else:
        for line in txt.splitlines():
            line = line.strip()
            if not line or line.startswith("#") or "\t" not in line:
                continue
            pt, en_csv = line.split("\t", 1)
            fwd[fold(pt.strip())] = [fold(v.strip()) for v in en_csv.split(",") if v.strip()]
    rev = {}
    for k, vs in fwd.items():
        for v in vs:
            rev.setdefault(v, []).append(k)
    return fwd, rev


def vocabulary(prompt, fwd, rev):
    """Termos de busca do agente sem Sidecar: palavras do prompt e suas traduções do dicionário."""
    toks = [t for t in re.findall(r"[a-z0-9]+", fold(prompt)) if len(t) >= 3 and t not in STOPWORDS]
    terms = []

    def add(t):
        if t not in terms:
            terms.append(t)

    for t in toks:
        add(t)
    for t in toks:
        for k in [t] + ([t[:-1]] if t.endswith("s") else []):
            for v in fwd.get(k, []) + rev.get(k, []):
                add(v)
    return terms[:GREP_TERM_CAP]


def grep_files(term, repo):
    t0 = time.perf_counter()
    p = subprocess.run(
        ["grep", "-rIil", "-F", "-i", "--exclude-dir=.git", "--", term, repo],
        capture_output=True, text=True, errors="replace",
    )
    dt = (time.perf_counter() - t0) * 1000
    if p.returncode not in (0, 1):
        raise RuntimeError(f"grep falhou em {repo}: {p.stderr.strip()}")
    files = [os.path.relpath(line, repo) for line in p.stdout.splitlines() if line]
    return files, p.stdout, dt


def read_file(rel, repo):
    t0 = time.perf_counter()
    text = (pathlib.Path(repo) / rel).read_bytes().decode("utf-8", "replace")
    return text, (time.perf_counter() - t0) * 1000


def explain(sidecar, home, repo, engine, prompt):
    env = {k: v for k, v in os.environ.items() if k != "SIDECAR_PROFILE"}
    env.update(SIDECAR_HOME=home, SIDECAR_ENGINE=engine)
    t0 = time.perf_counter()
    p = subprocess.run(
        [sidecar, "explain", "--root", repo, "--json"],
        input=prompt, capture_output=True, text=True, env=env,
    )
    dt = (time.perf_counter() - t0) * 1000
    if p.returncode != 0:
        raise RuntimeError(f"sidecar explain ({engine}) em {repo}: {p.stderr.strip()}")
    return json.loads(p.stdout), dt


def symbol_in(name, text):
    if name.startswith(HTTP_VERBS):  # rotas trazem o verbo; o código só tem o caminho
        name = name.split(" ", 1)[1]
    # Lookaround em vez de \b: rotas começam com "/", e \b não casa depois de caractere não alfanumérico.
    return re.search(rf"(?<![A-Za-z0-9_]){re.escape(name)}(?![A-Za-z0-9_])", text) is not None


def verify_evidence(evidence, repo, cache):
    """Fidelidade: fração dos símbolos citados que aparecem nas linhas citadas. Fora do relógio."""
    ok = total = 0
    for e in evidence:
        name = e.get("name") or ""
        if not name:
            continue
        total += 1
        if e["path"] not in cache:
            p = pathlib.Path(repo) / e["path"]
            cache[e["path"]] = p.read_text(encoding="utf-8", errors="replace") if p.is_file() else ""
        lines = cache[e["path"]].splitlines()
        m = re.fullmatch(r"(\d+)-(\d+)", e.get("lines") or "")
        span = lines[int(m[1]) - 1:int(m[2])] if m else lines
        if symbol_in(name, "\n".join(span)):
            ok += 1
    return ok / total if total else None


def run_arm(arm, task, repo, sidecar, home, fwd, rev, cache, n_files):
    gold = task["gold"]
    vocab = vocabulary(task["prompt"], fwd, rev)
    st = {"calls": [], "clock": 0.0, "round": 0, "ident": {}, "pres": {}, "seen": set(), "read": set(), "reads": 0}

    def call(kind, dt, tokens, new, target):
        st["clock"] += dt
        st["calls"].append({"kind": kind, "target": target, "ms": round(dt, 3), "tokens": tokens, "new": new})
        return st["clock"]

    def mark_pres(paths, at):
        for i, g in enumerate(gold):
            if g["path"] in paths:
                st["pres"].setdefault(i, at)

    def mark_ident(i, at):
        st["ident"].setdefault(i, (at, len(st["calls"]), st["round"]))

    def done():
        return len(st["ident"]) == len(gold)

    fidelity = None
    if arm != "OFF":
        rep, dt = explain(sidecar, home, repo, ARM_ENGINE[arm], task["prompt"])
        st["round"] = 1
        ev = rep.get("evidence") or []
        t = call("explain", dt, est_tokens(rep.get("context") or ""), True, "sidecar explain --json")
        mark_pres({e["path"] for e in ev}, t)
        for i, g in enumerate(gold):
            if any(e["path"] == g["path"] and e["name"] == g["name"] for e in ev):
                mark_ident(i, t)
        fidelity = verify_evidence(ev, repo, cache)

    fallback = not done()
    if fallback:
        # Sem o bloco (ou com itens faltando), o agente segue a mesma política do braço OFF.
        st["round"] += 1
        hits, df = {}, {}
        for term in vocab:
            files, out, dt = grep_files(term, repo)
            new = any(f not in st["seen"] for f in files)
            st["seen"].update(files)
            df[term] = len(files)
            for f in files:
                hits.setdefault(f, []).append(term)
            mark_pres(set(files), call("grep", dt, est_tokens(out), new, term))

        def score(f):
            # idf(termo) soma por arquivo; termo que aparece no caminho vale o dobro.
            return sum(math.log(1 + n_files / df[t]) * (2.0 if t in f.lower() else 1.0) for t in hits[f])

        # Não relê arquivo cujos símbolos do gabarito já foram identificados (pelo bloco ou por leitura).
        done_paths = {g["path"] for i, g in enumerate(gold)
                      if all(j in st["ident"] for j, h in enumerate(gold) if h["path"] == g["path"])}
        pending = [f for f in sorted(hits, key=lambda f: (-score(f), f))
                   if f not in st["read"] and f not in done_paths]
        k = 0
        while not done() and k < len(pending) and st["reads"] < READ_BUDGET:
            st["round"] += 1
            for f in pending[k:k + READ_BATCH]:
                if done() or st["reads"] >= READ_BUDGET:
                    break
                text, dt = read_file(f, repo)
                t = call("read", dt, est_tokens(text), True, f)
                st["reads"] += 1
                st["read"].add(f)
                mark_pres({f}, t)
                for i, g in enumerate(gold):
                    if g["path"] == f and i not in st["ident"] and (not g["name"] or symbol_in(g["name"], text)):
                        mark_ident(i, t)
            k += READ_BATCH

    resolved = done()
    found = list(st["ident"].values())
    n = len(st["calls"])
    return {
        "resolved": resolved,
        "presented_ms": max(st["pres"].values()) if len(st["pres"]) == len(gold) else None,
        "used_ms": max(v[0] for v in found) if resolved else None,
        "wall_ms": st["clock"],
        "tool_calls_to_gold": max(v[1] for v in found) if resolved else None,
        "rounds_to_gold": max(v[2] for v in found) if resolved else None,
        "contract_fidelity": fidelity,
        "loop_inefficiency": sum(1 for c in st["calls"] if not c["new"]) / n,
        "tokens": sum(c["tokens"] for c in st["calls"]),
        "tool_calls": n,
        "quality": 100.0 * len(st["ident"]) / len(gold),
        "identified": len(st["ident"]),
        "gold_total": len(gold),
        "fallback": fallback,
        "trace": st["calls"],
    }


ARM_ENGINE = {"ON-v1": "v1", "ON-v2": "v2"}


def check_inputs(tasks, repos):
    """Gabarito e commit precisam bater com o checkout antes de qualquer execução."""
    for t in tasks:
        repo = repos[t["repo"]]
        head = subprocess.run(["git", "-C", repo, "rev-parse", "HEAD"], capture_output=True, text=True).stdout.strip()
        if head != t["commit"]:
            raise SystemExit(f"{t['id']}: {repo} está em {head}, manifesto pede {t['commit']}")
        for g in t["gold"]:
            p = pathlib.Path(repo) / g["path"]
            if not p.is_file() or not symbol_in(g["name"], p.read_text(encoding="utf-8", errors="replace")):
                raise SystemExit(f"{t['id']}: gabarito {g['path']}:{g['name']} não existe no commit")


def mean(xs):
    return sum(xs) / len(xs)


def compose(recs, tasks):
    """Score 0–100 por tarefa, normalizado pelo melhor braço da mesma tarefa (ver o relatório)."""
    per_task = []
    agg = {a: [] for a in ARMS}
    for t in tasks:
        stats = {}
        for a in ARMS:
            rs = [r for r in recs if r["task"] == t["id"] and r["arm"] == a]
            if not rs:
                continue
            # Sem evidência correta, o tempo de parede é usado: é cota inferior, nunca zero.
            ttfe = [r["used_ms"] if r["resolved"] else r["wall_ms"] for r in rs]
            stats[a] = {
                "quality": mean([r["quality"] for r in rs]),
                "ttfe_ms": mean(ttfe),
                "ttfe_sd_ms": statistics.stdev(ttfe) if len(ttfe) > 1 else 0.0,
                "wall_ms": mean([r["wall_ms"] for r in rs]),
                "calls": mean([r["tool_calls"] for r in rs]),
                "tokens": mean([r["total_task_burden_tokens"] for r in rs]),
                "resolved": sum(r["resolved"] for r in rs) / len(rs),
            }
        best_t = min(s["ttfe_ms"] for s in stats.values())
        best_k = min(s["tokens"] for s in stats.values())
        best_c = min(s["calls"] for s in stats.values())
        for a, s in stats.items():
            s["speed"] = 100 * best_t / s["ttfe_ms"]
            s["cost"] = 100 * (0.5 * best_k / s["tokens"] + 0.5 * best_c / s["calls"])
            s["score"] = WEIGHTS[0] * s["quality"] + WEIGHTS[1] * s["speed"] + WEIGHTS[2] * s["cost"]
            agg[a].append(s)
        per_task.append({"task": t["id"], "repo": t["repo"], "arms": stats})
    summary = {}
    for a, rows in agg.items():
        if rows:
            summary[a] = {k: mean([r[k] for r in rows]) for k in rows[0]}
    return per_task, summary


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--sidecar", required=True)
    ap.add_argument("--home", required=True, help="diretório novo para o índice (isolado do cache do usuário)")
    ap.add_argument("--dict", required=True)
    ap.add_argument("--manifest", required=True)
    ap.add_argument("--repo", action="append", required=True, help="nome=caminho")
    ap.add_argument("--reps", type=int, default=5)
    ap.add_argument("--out-dir", required=True)
    ap.add_argument("--scores-out", required=True)
    args = ap.parse_args()

    if os.path.exists(args.home) and os.listdir(args.home):
        raise SystemExit("--home deve ser um diretório novo ou vazio")
    os.makedirs(args.home, exist_ok=True)
    repos = dict(r.split("=", 1) for r in args.repo)
    tasks = []
    for line in open(args.manifest, encoding="utf-8"):
        if line.strip():
            t = json.loads(line)
            if t["status"] == "verified" and t["repo"] in repos:
                tasks.append(t)
    if not tasks:
        raise SystemExit("nenhuma tarefa verificada para os repositórios informados")
    check_inputs(tasks, repos)
    fwd, rev = load_dictionary(args.dict)
    cache = {}

    # Índice frio: primeira chamada por repositório. As execuções medidas usam o índice aquecido.
    setup = {}
    for name, repo in repos.items():
        _, dt = explain(args.sidecar, args.home, repo, "v1", "setup")
        setup[name] = dt

    # Tamanho do repositório (arquivos rastreados) para o idf; contado uma vez, fora das execuções.
    sizes = {}
    for name, repo in repos.items():
        out = subprocess.run(["git", "-C", repo, "ls-files"], capture_output=True, text=True).stdout
        sizes[name] = len(out.splitlines())

    # Aquecimento não registrado: leva os arquivos para o cache do sistema operacional.
    for t in tasks:
        run_arm("OFF", t, repos[t["repo"]], args.sidecar, args.home, fwd, rev, cache, sizes[t["repo"]])

    recs, detail, trace = [], [], []
    for rep in range(1, args.reps + 1):
        rot = ARMS[(rep - 1) % len(ARMS):] + ARMS[:(rep - 1) % len(ARMS)]
        for t in tasks:
            for arm in rot:
                out = run_arm(arm, t, repos[t["repo"]], args.sidecar, args.home, fwd, rev, cache, sizes[t["repo"]])
                # AB: OFF rodou antes do braço nesta repetição (o campo do avaliador diz qual veio primeiro).
                if arm == "OFF":
                    order = "AB" if rot[0] == "OFF" else "BA"
                else:
                    order = "AB" if rot.index("OFF") < rot.index(arm) else "BA"
                base = {
                    "task": t["id"], "arm": arm, "model": "instrumented-sim", "rep": rep, "order": order,
                    "resolved": out["resolved"], "wall_ms": round(out["wall_ms"], 3),
                    "total_task_burden_tokens": out["tokens"],
                    "loop_inefficiency": round(out["loop_inefficiency"], 4),
                }
                for k in ("presented_ms", "used_ms", "tool_calls_to_gold", "rounds_to_gold"):
                    if out[k] is not None:
                        base[k] = round(out[k], 3) if isinstance(out[k], float) else out[k]
                if out["contract_fidelity"] is not None:
                    base["contract_fidelity"] = round(out["contract_fidelity"], 4)
                recs.append(base)
                detail.append({**base, "repo": t["repo"], "quality": round(out["quality"], 2),
                               "identified": out["identified"], "gold_total": out["gold_total"],
                               "tool_calls": out["tool_calls"], "fallback": out["fallback"]})
                trace.append({"task": t["id"], "arm": arm, "rep": rep, "calls": out["trace"]})

    os.makedirs(args.out_dir, exist_ok=True)
    with open(os.path.join(args.out_dir, "runs.jsonl"), "w", encoding="utf-8") as f:
        for r in recs:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
    with open(os.path.join(args.out_dir, "real_runs.jsonl"), "w", encoding="utf-8") as f:
        for r in detail:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
    with open(os.path.join(args.out_dir, "real_trace.jsonl"), "w", encoding="utf-8") as f:
        for r in trace:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")

    per_task, summary = compose(detail, tasks)
    with open(args.scores_out, "w", encoding="utf-8") as f:
        json.dump({"setup_index_cold_ms": setup, "reps": args.reps, "weights": WEIGHTS,
                   "per_task": per_task, "summary": summary}, f, ensure_ascii=False, indent=2)
    print(f"runs={len(recs)} tasks={len(tasks)} reps={args.reps}")
    print("setup (índice frio, ms):", {k: round(v, 1) for k, v in setup.items()})


if __name__ == "__main__":
    sys.exit(main())
