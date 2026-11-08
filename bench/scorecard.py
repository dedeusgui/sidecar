#!/usr/bin/env python3
"""Placar do Sidecar: execuções reais de agentes primeiro, retrieval offline depois.

Somente leitura e stdlib: não executa agentes, modelos, rede nem comandos destrutivos.
CASES_JSON é fornecido pelo usuário; SHA256 igual não prova a proveniência do resultado.
Dado ausente ou inválido é erro (exit 1), nunca zero.
"""

import argparse
import hashlib
import json
import math
import os
import pathlib
import statistics
import sys

ARMS = ("OFF", "ON")
HARDWARE_KEYS = ("cpu_model", "cpus", "go", "goos", "goarch")
LIMITS = (
    "CASES_JSON é fornecido pelo usuário; fixtures internas não são prova de utilidade externa.",
    "Deltas são diferenças observadas em uma execução: sem teste de significância, intervalo de confiança ou inferência causal.",
    "Checks são identificados pelo texto exato de `failed_checks`; mudança de contagem na mensagem (ex.: `gold: 0/1` → `gold: 1/2`) aparece como resolvido + novo.",
    "Execuções de agentes: N pequeno é descritivo; não há comparação estatística entre braços. Tempo médio inclui timeouts.",
    "Cada rodada pode usar tarefa, engine e transporte diferentes; consulte seu protocolo e metadados. Prefixo não é hook real.",
    "Tempo e custo por correta incluem apenas o que cada instrumento mediu; setup e avaliação podem estar ausentes.",
    "Nada é executado: sem agentes, modelos, rede ou comandos destrutivos.",
)


class DataError(ValueError):
    """Entrada ausente, inválida ou inconsistente."""


# --- leitura e validação ---------------------------------------------------

def is_str(v):
    return isinstance(v, str) and v.strip() != ""


def is_bool(v):
    return isinstance(v, bool)


def is_int(v):
    return isinstance(v, int) and not isinstance(v, bool) and v >= 0


def is_num(v):
    return isinstance(v, (int, float)) and not isinstance(v, bool) and math.isfinite(v) and v >= 0


def is_list(v):
    return isinstance(v, list)


def is_dict(v):
    return isinstance(v, dict)


def field(obj, key, ok, where, what):
    if not isinstance(obj, dict) or key not in obj:
        raise DataError(f"{where}: campo ausente '{key}'")
    if not ok(obj[key]):
        raise DataError(f"{where}: '{key}' deve ser {what}, veio {obj[key]!r}")
    return obj[key]


def read(path):
    try:
        raw = pathlib.Path(path).read_bytes()
    except OSError as e:
        raise DataError(f"{path}: {e.strerror or e}") from e
    return raw, hashlib.sha256(raw).hexdigest()


def parse_json(raw, path):
    try:
        return json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as e:
        raise DataError(f"{path}: JSON inválido: {e}") from e


def load_cases(path):
    raw, sha = read(path)
    items = field(parse_json(raw, path), "cases", is_list, path, "lista")
    if not items:
        raise DataError(f"{path}: sem casos")
    cases = {}
    for i, c in enumerate(items):
        where = f"{path} cases[{i}]"
        cid = field(c, "id", is_str, where, "texto não vazio")
        if cid in cases:
            raise DataError(f"{where}: id duplicado {cid!r}")
        # Mesma regra do sidecar-eval: `negative` (--cases) ou `no_gold` (--external-cases) marca negativo.
        # Campos omitidos = positivo; divergência com o resultado é erro em load_results.
        flags = [field(c, k, is_bool, where, "booleano") for k in ("negative", "no_gold") if k in c]
        negative = any(flags)
        cases[cid] = {"split": field(c, "split", is_str, where, "texto não vazio"), "negative": negative}
    return cases, sha


def load_results(path, cases):
    raw, sha = read(path)
    doc = parse_json(raw, path)
    methods = field(doc, "methods", is_list, path, "lista")
    if not all(is_str(m) for m in methods) or len(set(methods)) != len(methods):
        raise DataError(f"{path}: 'methods' deve ter nomes de texto únicos")
    hardware = field(doc, "hardware", is_dict, path, "objeto")
    missing = [k for k in HARDWARE_KEYS if k not in hardware]
    if missing:
        raise DataError(f"{path}: hardware sem {missing}")
    outcomes = field(doc, "outcomes", is_list, path, "lista")
    rows, groups = {}, {}
    for i, o in enumerate(outcomes):
        where = f"{path} outcomes[{i}]"
        method = field(o, "method", is_str, where, "texto não vazio")
        case = field(o, "case", is_str, where, "texto não vazio")
        split = field(o, "split", is_str, where, "texto não vazio")
        negative = field(o, "negative", is_bool, where, "booleano")
        hit5 = field(o, "hit_at_5", is_bool, where, "booleano")
        empty = field(o, "empty", is_bool, where, "booleano")
        latency = field(o, "latency_ms", is_num, where, "número finito >= 0")
        tokens = field(o, "context_tokens", is_int, where, "inteiro >= 0")
        failed = o.get("failed_checks", [])  # omitido = nenhum check falhou (omitempty do sidecar-eval)
        if not is_list(failed) or not all(is_str(f) for f in failed):
            raise DataError(f"{where}: 'failed_checks' deve ser lista de textos")
        if method not in methods:
            raise DataError(f"{where}: método {method!r} ausente de 'methods'")
        if case not in cases:
            raise DataError(f"{where}: caso {case!r} ausente de CASES_JSON")
        if cases[case]["split"] != split or cases[case]["negative"] != negative:
            raise DataError(f"{where}: split ou polaridade de {case!r} diverge de CASES_JSON")
        if (method, case) in rows:
            raise DataError(f"{where}: duplicata de ({method!r}, {case!r})")
        row = {"case": case, "negative": negative, "hit5": hit5, "empty": empty,
               "latency": float(latency), "tokens": tokens, "failed": list(failed)}
        rows[(method, case)] = row
        groups.setdefault((method, split), []).append(row)
    for method in methods:
        ids = {c for m, c in rows if m == method}
        if ids != set(cases):
            raise DataError(f"{path}: método {method!r} com casos incompletos (faltam {len(set(cases) - ids)})")
    return {"results_sha": sha, "methods": methods, "hardware": hardware, "groups": groups}


def load_agent_runs(path, seen):
    raw, sha = read(path)
    try:
        lines = raw.decode("utf-8").splitlines()
    except UnicodeDecodeError as e:
        raise DataError(f"{path}: não é UTF-8: {e}") from e
    records = []
    for lineno, line in enumerate(lines, 1):
        if not line.strip():
            continue
        where = f"{path}:{lineno}"
        try:
            r = json.loads(line)
        except json.JSONDecodeError as e:
            raise DataError(f"{where}: JSON inválido: {e}") from e
        rec = {
            "agent": field(r, "agent", is_str, where, "texto não vazio"),
            "model": field(r, "model", is_str, where, "texto não vazio"),
            "arm": field(r, "arm", lambda v: v in ARMS, where, "OFF ou ON"),
            "case": field(r, "case", is_str, where, "texto não vazio"),
            "rep": field(r, "rep", is_int, where, "inteiro >= 0"),
            "success": field(r, "success", lambda v: v is None or is_bool(v), where, "booleano ou null"),
            "timed_out": field(r, "timed_out", is_bool, where, "booleano"),
            "wall_s": field(r, "wall_s", is_num, where, "número finito >= 0"),
            "cost_usd": field(r, "cost_usd", lambda v: v is None or is_num(v), where, "número >= 0 ou null"),
            "transport": field(r, "transport", is_str, where, "texto não vazio"),
        }
        # Older edit-pilot records used false for client/oracle infrastructure
        # errors. Preserve the row, but do not score those errors as task failures.
        oracle = r.get("oracle")
        if isinstance(oracle, dict) and (oracle.get("status") == "error" or
                                         (oracle.get("status") == "not_run" and
                                          (r.get("exit") != 0 or rec["timed_out"]))):
            rec["success"] = None
        key = (rec["agent"], rec["model"], rec["arm"], rec["case"], rec["rep"])
        if key in seen:
            raise DataError(f"{where}: execução duplicada {key}")
        seen.add(key)
        records.append(rec)
    if not records:
        raise DataError(f"{path}: sem registros")
    return records, sha


# --- métricas ----------------------------------------------------------------

def frac(k, n):
    return None if n == 0 else (k, n)


def rate(f):
    return None if f is None else f[0] / f[1]


def fmt_frac(f):
    return "n/d" if f is None else f"{f[0]}/{f[1]} ({100 * f[0] / f[1]:.1f}%)"


def fmt_num(v, d=1):
    return "n/d" if v is None else f"{v:.{d}f}"


def delta_pp(a, b):
    return "n/d" if rate(a) is None or rate(b) is None else f"{100 * (rate(a) - rate(b)):+.1f} pp"


def delta_num(a, b):
    return "n/d" if a is None or b is None else f"{a - b:+.1f}"


def metrics(rows):
    pos = [r for r in rows if not r["negative"]]
    neg = [r for r in rows if r["negative"]]
    return {
        "n_pos": len(pos),
        "n_neg": len(neg),
        "passed": frac(sum(not r["failed"] for r in rows), len(rows)),
        "hit5": frac(sum(r["hit5"] for r in pos), len(pos)),
        "fp": frac(sum(not r["empty"] for r in neg), len(neg)),
        "latency": statistics.median(r["latency"] for r in rows),
        "tokens": statistics.fmean(r["tokens"] for r in rows),
    }


def group_keys(src):
    order = {m: i for i, m in enumerate(src["methods"])}
    return sorted(src["groups"], key=lambda k: (order[k[0]], k[1]))


def summarize_agents(records):
    groups = {}
    for r in records:
        groups.setdefault((r["agent"], r["model"], r["arm"]), []).append(r)
    out = []
    for key in sorted(groups, key=lambda k: (k[0], k[1], ARMS.index(k[2]))):
        rs = groups[key]
        costs = [r["cost_usd"] for r in rs if r["cost_usd"] is not None]
        success = sum(r["success"] is True for r in rs)
        judged = sum(r["success"] is not None for r in rs)
        out.append({
            "agent": key[0], "model": key[1], "arm": key[2], "n": len(rs),
            "success": success,
            "judged": judged,
            "timeouts": sum(r["timed_out"] for r in rs),
            "wall_mean": statistics.fmean(r["wall_s"] for r in rs),
            "wall_per_success": sum(r["wall_s"] for r in rs) / success if success and judged == len(rs) else None,
            "cost_per_success": sum(costs) / success if success and judged == len(rs) and len(costs) == len(rs) else None,
            "cost_known": len(costs),
            "cost_unknown": len(rs) - len(costs),
            "cost_mean": statistics.fmean(costs) if costs else None,
        })
    return out


# --- comparação com baseline ---------------------------------------------------

def compare(base, suite):
    """Motivos de incompatibilidade; lista vazia = comparável."""
    reasons = []
    if base["cases_sha"] != suite["cases_sha"]:
        reasons.append(f"CASES_JSON difere (sha256 {base['cases_sha'][:12]} × {suite['cases_sha'][:12]})")
    bk, sk = set(base["groups"]), set(suite["groups"])
    if bk != sk:
        reasons.append(f"grupos (método, split) diferentes: só no baseline: {fmt_list(f'{m} / {s}' for m, s in bk - sk)}; "
                       f"só na suite: {fmt_list(f'{m} / {s}' for m, s in sk - bk)}")
    for key in sorted(bk & sk):
        bi = {r["case"]: r["negative"] for r in base["groups"][key]}
        si = {r["case"]: r["negative"] for r in suite["groups"][key]}
        if set(bi) != set(si):
            reasons.append(f"{key[0]} / {key[1]}: IDs diferentes (só no baseline: {fmt_list(set(bi) - set(si))}; "
                           f"só na suite: {fmt_list(set(si) - set(bi))})")
        elif bi != si:
            reasons.append(f"{key[0]} / {key[1]}: polaridade diferente em {fmt_list(i for i in bi if bi[i] != si[i])}")
    return reasons


def case_changes(base, suite):
    changes = []
    for key in group_keys(base):
        before = {r["case"]: r for r in base["groups"][key]}
        for r in suite["groups"][key]:
            bf, sf = set(before[r["case"]]["failed"]), set(r["failed"])
            if bf == sf:
                continue
            kind = "piorou" if not bf else "melhorou" if not sf else "mudou"
            changes.append((key, r["case"], kind, sorted(sf - bf), sorted(bf - sf)))
    return changes


def hw_key(src):
    return tuple(src["hardware"][k] for k in ("cpu_model", "cpus", "goos", "goarch", "go"))


# --- relatório -----------------------------------------------------------------

def cell(s):
    return str(s).replace("|", "\\|")


def fmt_list(items, limit=5):
    items = sorted(str(i) for i in items)
    shown = ", ".join(items[:limit]) or "nenhum"
    return shown + (f" (+{len(items) - limit})" if len(items) > limit else "")


def agent_table(rows):
    lines = ["| Agente | Modelo | Braço | N | Sucesso | Sem veredito | Timeouts | Tempo médio (s) | Tempo por correta (s) | Custo por correta (US$) | Custo informado (n) | Custo médio informado (US$) | Custo desconhecido (n) |",
             "|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"]
    for r in rows:
        usd = "n/d" if r["cost_mean"] is None else f"{r['cost_mean']:.4f}"
        cost_per_success = "n/d" if r["cost_per_success"] is None else f"{r['cost_per_success']:.4f}"
        success = "n/d" if not r["judged"] else f"{r['success']}/{r['judged']} ({100 * r['success'] / r['judged']:.0f}%)"
        lines.append(f"| {cell(r['agent'])} | {cell(r['model'])} | {r['arm']} | {r['n']} | "
                     f"{success} | {r['n'] - r['judged']} | {r['timeouts']} | "
                     f"{r['wall_mean']:.1f} | {fmt_num(r['wall_per_success'])} | {cost_per_success} | {r['cost_known']} | {usd} | {r['cost_unknown']} |")
    return lines


def metrics_table(offline):
    lines = ["| Papel | Rótulo | Método | Split | N pos | N neg | Checks aprovados | Hit@5 pos | Falsos positivos (neg) | Latência mediana (ms) | Tokens médios |",
             "|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|"]
    for s in offline:
        for key in group_keys(s):
            m = metrics(s["groups"][key])
            lines.append(f"| {s['role']} | {cell(s['label'])} | {cell(key[0])} | {cell(key[1])} | {m['n_pos']} | {m['n_neg']} | "
                         f"{fmt_frac(m['passed'])} | {fmt_frac(m['hit5'])} | {fmt_frac(m['fp'])} | "
                         f"{fmt_num(m['latency'])} | {fmt_num(m['tokens'], 0)} |")
    return lines


def delta_lines(base, suite):
    lines = []
    for key in group_keys(base):
        b, s = metrics(base["groups"][key]), metrics(suite["groups"][key])
        g = cell(f"{key[0]} / {key[1]}")
        lines += [
            f"| {g} | checks aprovados | {fmt_frac(b['passed'])} | {fmt_frac(s['passed'])} | {delta_pp(s['passed'], b['passed'])} |",
            f"| {g} | Hit@5 positivos | {fmt_frac(b['hit5'])} | {fmt_frac(s['hit5'])} | {delta_pp(s['hit5'], b['hit5'])} |",
            f"| {g} | falsos positivos (neg) | {fmt_frac(b['fp'])} | {fmt_frac(s['fp'])} | {delta_pp(s['fp'], b['fp'])} |",
            f"| {g} | latência mediana (ms) | {fmt_num(b['latency'])} | {fmt_num(s['latency'])} | {delta_num(s['latency'], b['latency'])} |",
            f"| {g} | tokens médios | {fmt_num(b['tokens'], 0)} | {fmt_num(s['tokens'], 0)} | {delta_num(s['tokens'], b['tokens'])} |",
        ]
    return lines


def comparison_block(base, suite, reasons):
    head = [f"#### Baseline `{cell(base['label'])}` × suite `{cell(suite['label'])}`", ""]
    if reasons:
        return head + ["**Incompatível: nenhum delta calculado.**", ""] + [f"- {r}" for r in reasons] + [""]
    out = list(head)
    if hw_key(base) != hw_key(suite):
        out += ["Atenção: hardware difere entre as execuções; latência não é comparável diretamente.", ""]
    out += ["| Grupo | Métrica | Baseline | Suite | Δ (suite − baseline) |", "|---|---|---:|---:|---:|"]
    out += delta_lines(base, suite)
    changes = case_changes(base, suite)
    total = sum(len(rows) for rows in base["groups"].values())
    out += ["", f"Casos com check alterado: {len(changes)} de {total}.", ""]
    if changes:
        out += ["| Grupo | Caso | Mudança | Checks novos | Checks resolvidos |", "|---|---|---|---|---|"]
        for key, case, kind, new, gone in changes:
            out.append(f"| {cell(key[0])} / {cell(key[1])} | {cell(case)} | {kind} | "
                       f"{cell('; '.join(new)) or '-'} | {cell('; '.join(gone)) or '-'} |")
        out.append("")
    return out


def report(agent_sources, offline, comparisons):
    lines = ["# Placar Sidecar", "",
             "Ordem: 1) execuções reais de agentes, por rodada; 2) retrieval offline (secundário).", "",
             "## 1. Agentes reais", ""]
    if agent_sources:
        for path, _, records in agent_sources:
            lines += [f"### Rodada `{cell(path)}`", ""] + agent_table(summarize_agents(records))
            transports = ", ".join(f"`{t}`" for t in sorted({r["transport"] for r in records}))
            lines += ["", f"Transporte(s) registrado(s): {transports}. Tempo/custo por correta = soma das execuções dividida pelo número de acertos; "
                          "custo desconhecido ou veredito ausente torna a respectiva razão indisponível. "
                          "A média de custo usa só valores informados. Descritivo: sem inferência estatística ou causal.", ""]
    else:
        lines += ["_Nenhum `--agent-runs` informado (n/d)._", ""]
    lines += ["## 2. Retrieval offline (secundário)", "",
              "Medição offline do sidecar-eval, não é resultado end-to-end com agentes.", ""]
    if offline:
        lines += metrics_table(offline) + [""]
        if comparisons:
            lines += ["### Comparações com baseline", ""]
            for base, suite, reasons in comparisons:
                lines += comparison_block(base, suite, reasons)
    else:
        lines += ["_Nenhum `--suite` informado._", ""]
    lines += ["## 3. Arquivos e hashes", "", "| Papel | Rótulo | Tipo | Caminho | SHA256 |", "|---|---|---|---|---|"]
    for path, sha, _ in agent_sources:
        lines.append(f"| agente | - | JSONL | `{path}` | `{sha}` |")
    for s in offline:
        lines.append(f"| {s['role']} | {cell(s['label'])} | resultados | `{s['path']}` | `{s['results_sha']}` |")
        lines.append(f"| {s['role']} | {cell(s['label'])} | CASES_JSON | `{s['cases_path']}` | `{s['cases_sha']}` |")
    lines += ["", "CASES_JSON é fornecido pelo usuário: SHA256 igual indica o mesmo arquivo de casos, "
                  "não prova de que o resultado foi gerado a partir dele.", "", "## Limites", ""]
    lines += [f"- {item}" for item in LIMITS]
    return "\n".join(lines) + "\n"


def build(args):
    agent_sources = []
    for path in args.agent_runs:
        records, sha = load_agent_runs(path, set())
        agent_sources.append((path, sha, records))
    cases_cache, offline = {}, []
    for role, entries in (("suite", args.suite), ("baseline", args.baseline)):
        for label, results, cases in entries:
            if cases not in cases_cache:
                cases_cache[cases] = load_cases(cases)
            index, cases_sha = cases_cache[cases]
            src = load_results(results, index)
            src.update(label=label, role=role, path=results, cases_path=cases, cases_sha=cases_sha)
            offline.append(src)
    suites = [s for s in offline if s["role"] == "suite"]
    baselines = [s for s in offline if s["role"] == "baseline"]
    comparisons = [(b, s, compare(b, s)) for b in baselines for s in suites]
    return report(agent_sources, offline, comparisons)


def build_parser():
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("--agent-runs", action="append", default=[], metavar="JSONL",
                   help="registros de execução de agentes (repetível)")
    p.add_argument("--suite", nargs=3, action="append", default=[], metavar=("LABEL", "RESULTS_JSON", "CASES_JSON"),
                   help="resultado offline do sidecar-eval (repetível)")
    p.add_argument("--baseline", nargs=3, action="append", default=[], metavar=("LABEL", "RESULTS_JSON", "CASES_JSON"),
                   help="resultado de referência comparado a cada --suite (repetível)")
    p.add_argument("--out", metavar="PATH", help="também grava o relatório Markdown neste caminho")
    return p


def check_args(parser, args):
    if not args.suite and not args.agent_runs:
        parser.error("informe ao menos --agent-runs ou --suite")
    if args.baseline and not args.suite:
        parser.error("--baseline exige ao menos um --suite")
    labels = [e[0] for e in args.suite + args.baseline]
    if any(not label.strip() for label in labels):
        parser.error("LABEL não pode ser vazio")
    dups = sorted({label for label in labels if labels.count(label) > 1})
    if dups:
        parser.error(f"LABEL duplicado: {dups}")
    if args.out:
        inputs = args.agent_runs + [e[1] for e in args.suite + args.baseline] + [e[2] for e in args.suite + args.baseline]
        if os.path.realpath(args.out) in {os.path.realpath(p) for p in inputs}:
            parser.error("--out não pode apontar para um arquivo de entrada")


def main(argv=None):
    parser = build_parser()
    args = parser.parse_args(argv)
    check_args(parser, args)
    try:
        text = build(args)
        if args.out:
            pathlib.Path(args.out).write_text(text, encoding="utf-8")
    except (DataError, OSError) as e:
        print(f"erro: {e}", file=sys.stderr)
        return 1
    sys.stdout.write(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
