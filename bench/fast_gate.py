#!/usr/bin/env python3
"""Fast zero-cost P0+P1 iterative gate and Studio Observatory generator for Sidecar.

Runs Go/Python unit gates (P0), offline delivery scorecard (P1), optional external/CodeGraph
comparisons when local pinned checkouts are provided, checks non-regression between engines,
and compiles one self-contained `sidecar studio` HTML page for instant inspection.
"""
import argparse
import json
import os
import pathlib
import statistics
import subprocess
import sys
import tempfile
import time

ROOT = pathlib.Path(__file__).resolve().parents[1]


def run_step(name, cmd, cwd=ROOT, env=None):
    t0 = time.perf_counter()
    res = subprocess.run(cmd, cwd=cwd, env=env, text=True)
    dt = time.perf_counter() - t0
    if res.returncode != 0:
        raise SystemExit(f"[FAIL] {name} ({dt:.2f}s) exited {res.returncode}")
    print(f"[PASS] {name} ({dt:.2f}s)")
    return dt


# Thresholds for the delivery gate. They are deliberately explicit: a change to any of them is a
# visible change to what counts as a regression. Gold path and top-3 gates are strict (no drop at all).
NEGATIVE_STRATA = ("negative", "hard_negative")
POSITIVE_METRICS = ("gold_path_delivered", "gold_in_first_3_lines", "orientation_only", "trust_label_conflicts",
                    "non_deterministic", "tokens_per_gold_delivered", "p95_ms")
NEGATIVE_METRICS = ("non_empty", "trust_label_conflicts", "non_deterministic")
TOKEN_TOLERANCE = 0.10      # tokens per delivered gold may grow at most 10% over the baseline
LATENCY_FACTOR = 1.5        # p95 may be at most 1.5x the baseline ...
LATENCY_SLACK_MS = 5.0      # ... plus 5 ms, so jitter on sub-10 ms medians is not a regression


def merge_latency_runs(runs):
    """Median p95 per (stratum, engine) across repeated delivery runs; other fields come from the first run.

    A single run's p95 over a few dozen cases swings by tens of percent on a loaded machine, so the gate
    compares medians. `latency_runs` records how many samples each median used.
    """
    samples = {}
    for run in runs:
        for row in run["summary"]:
            samples.setdefault((row["stratum"], row["engine"]), []).append(row["p95_ms"])
    merged = []
    for row in runs[0]["summary"]:
        out = dict(row)
        values = samples[(row["stratum"], row["engine"])]
        out["p95_ms"] = statistics.median(values)
        out["latency_runs"] = len(values)
        merged.append(out)
    return merged


def run_delivery(name, base_cmd, out_json, runs):
    """Run one delivery scorecard `runs` times and write it to out_json with median latency."""
    if runs <= 1:
        run_step(name, base_cmd + ["--out", str(out_json)])
        return
    paths = []
    for i in range(runs):
        path = out_json.with_name(f"{out_json.stem}.run{i + 1}.json")
        run_step(f"{name} run {i + 1}/{runs}", base_cmd + ["--out", str(path)])
        paths.append(path)
    data = json.loads(paths[0].read_text())
    data["summary"] = merge_latency_runs([json.loads(path.read_text()) for path in paths])
    out_json.write_text(json.dumps(data, ensure_ascii=False, indent=2))


def check_delivery_gate(summaries, candidate, baseline, token_tolerance=TOKEN_TOLERANCE,
                        latency_factor=LATENCY_FACTOR, latency_slack_ms=LATENCY_SLACK_MS):
    """Compare candidate vs baseline across delivery strata; any gap is a failure, never a silent skip.

    A stratum or engine missing on either side means the comparison is incomplete, so it fails rather
    than reporting GATE OK on partial evidence. Latency is gated with a tolerance because it is noisy.
    """
    by_key = {(s["stratum"], s["engine"]): s for s in summaries}
    engines = {s["engine"] for s in summaries}
    failures = []
    for engine in (candidate, baseline):
        if engine not in engines:
            failures.append(f"{engine}: no delivery summary; the gate cannot compare it")
    if failures:
        return failures
    strata = sorted({s["stratum"] for s in summaries})
    for stratum in strata:
        cand = by_key.get((stratum, candidate))
        base = by_key.get((stratum, baseline))
        if cand is None or base is None:
            side = candidate if cand is None else baseline
            failures.append(f"{stratum}: missing stratum for {side}; partial results are not a gate pass")
            continue
        negative = stratum in NEGATIVE_STRATA
        required = NEGATIVE_METRICS if negative else POSITIVE_METRICS
        absent = [m for m in required if m not in cand or m not in base]
        if absent:
            failures.append(f"{stratum}: missing metric(s) {', '.join(absent)} for {candidate} or {baseline}")
            continue
        if cand["non_deterministic"] > 0:
            failures.append(f"{stratum}: {candidate} non_deterministic={cand['non_deterministic']}")
        if cand["trust_label_conflicts"] > base["trust_label_conflicts"]:
            failures.append(
                f"{stratum}: {candidate} label conflicts {cand['trust_label_conflicts']} > {baseline} {base['trust_label_conflicts']}"
            )
        if negative:
            if cand["non_empty"] > base["non_empty"]:
                failures.append(f"{stratum}: {candidate} negative leaks {cand['non_empty']} > {baseline} {base['non_empty']}")
            continue
        if cand["gold_path_delivered"] < base["gold_path_delivered"]:
            failures.append(
                f"{stratum}: {candidate} gold_path_delivered {cand['gold_path_delivered']} < {baseline} {base['gold_path_delivered']}"
            )
        if cand["gold_in_first_3_lines"] < base["gold_in_first_3_lines"]:
            failures.append(
                f"{stratum}: {candidate} gold_in_first_3_lines {cand['gold_in_first_3_lines']} < {baseline} {base['gold_in_first_3_lines']}"
            )
        if cand["orientation_only"] > base["orientation_only"]:
            failures.append(
                f"{stratum}: {candidate} orientation_only {cand['orientation_only']} > {baseline} {base['orientation_only']}"
            )
        base_tpg = base["tokens_per_gold_delivered"]
        if base_tpg > 0 and cand["tokens_per_gold_delivered"] > base_tpg * (1 + token_tolerance):
            failures.append(
                f"{stratum}: {candidate} tokens_per_gold_delivered {cand['tokens_per_gold_delivered']:.1f} > "
                f"{baseline} {base_tpg:.1f} x{1 + token_tolerance:.2f}"
            )
        if cand["p95_ms"] > base["p95_ms"] * latency_factor + latency_slack_ms:
            failures.append(
                f"{stratum}: {candidate} p95_ms {cand['p95_ms']:.1f} > {baseline} {base['p95_ms']:.1f} x{latency_factor} + {latency_slack_ms}ms"
            )
    return failures


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--engines", default="v1,v2,v3,v3.5,v3.6,v3.7", help="comma-separated delivery engines")
    ap.add_argument("--out-dir", default="/tmp/sidecar-fast-gate", help="directory for JSON/HTML outputs")
    ap.add_argument("--checkout", help="optional directory containing pinned external repos for E1/E2")
    ap.add_argument("--external-manifest", help="optional pinned external-cases manifest JSON")
    ap.add_argument("--negatives", default="bench/negatives/hard_negatives.json", help="negatives file for --external-manifest")
    ap.add_argument("--codegraph", help="optional codegraph CLI path for e2_offline.py")
    ap.add_argument("--candidate", help="candidate engine for non-regression comparison (e.g. v3.6)")
    ap.add_argument("--baseline", default="v3", help="baseline engine for comparison (default: v3)")
    ap.add_argument("--enforce-gate", action="store_true", help="exit 1 if candidate regresses vs baseline")
    ap.add_argument("--race", action="store_true", help="also run Go race detector on concurrent packages")
    ap.add_argument("--skip-tests", action="store_true", help="skip P0 unit tests and run P1+Studio only")
    ap.add_argument("--latency-runs", type=int, default=3,
                    help="repeat each delivery run N times and gate on the median p95 (1 = single run, noisy)")
    args = ap.parse_args(argv)
    if args.latency_runs < 1:
        ap.error("--latency-runs must be at least 1")

    out_dir = pathlib.Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    bin_dir = out_dir / "bin"
    bin_dir.mkdir(parents=True, exist_ok=True)
    sidecar_bin = bin_dir / "sidecar"
    eval_bin = bin_dir / "sidecar-eval"

    t_total = time.perf_counter()
    if not args.skip_tests:
        run_step("P0 go test", ["go", "test", "-count=1", "./..."])
        if args.race:
            run_step("P0 go test -race", ["go", "test", "-race", "-count=1", "./internal/store/", "./internal/index/", "./internal/app/", "./internal/gitx/"])
        run_step("P0 python bench tests", [sys.executable, "-m", "unittest", "discover", "-s", "bench", "-p", "test_*.py"])
        run_step("P0 registry check", [sys.executable, "bench/consolidate.py", "--check"])
        run_step("P0 v3.5 freeze verify", [sys.executable, "bench/v3.5/freeze.py", "verify"])

    run_step("Build sidecar & sidecar-eval", ["go", "build", "-o", str(sidecar_bin), "./cmd/sidecar"])
    run_step("Build sidecar-eval", ["go", "build", "-o", str(eval_bin), "./cmd/sidecar-eval"])

    studio_inputs = []
    cases_json = out_dir / "delivery-cases.json"
    run_delivery(
        "P1 delivery (bench/cases.json)",
        [str(eval_bin), "--cases", "bench/cases.json", "--delivery", "--delivery-engines", args.engines],
        cases_json,
        args.latency_runs,
    )
    studio_inputs.append(str(cases_json))

    gate_failures = []
    if args.candidate:
        data = json.loads(cases_json.read_text())
        gate_failures.extend(f"cases.json · {f}" for f in check_delivery_gate(data.get("summary", []), args.candidate, args.baseline))

    if args.external_manifest:
        ext_json = out_dir / "delivery-external.json"
        cmd = [str(eval_bin), "--external-cases", args.external_manifest, "--delivery", "--delivery-engines", args.engines]
        if args.negatives and (ROOT / args.negatives).is_file():
            cmd += ["--delivery-negatives", args.negatives]
        run_delivery("P1 delivery (external manifest)", cmd, ext_json, args.latency_runs)
        studio_inputs.append(str(ext_json))
        if args.candidate:
            ext_data = json.loads(ext_json.read_text())
            gate_failures.extend(f"external · {f}" for f in check_delivery_gate(ext_data.get("summary", []), args.candidate, args.baseline))

    if args.checkout:
        checkout = pathlib.Path(args.checkout)
        e1_manifest = out_dir / "e1-manifest.json"
        run_step("P1 build E1 manifest", [sys.executable, "bench/v3.5/make_e1_manifest.py", "--checkout", str(checkout), "--out", str(e1_manifest)])
        e1_json = out_dir / "delivery-e1.json"
        run_delivery("P1 delivery (E1 cohorts)", [str(eval_bin), "--external-cases", str(e1_manifest), "--delivery", "--delivery-engines", args.engines], e1_json, args.latency_runs)
        studio_inputs.append(str(e1_json))
        if args.candidate:
            e1_data = json.loads(e1_json.read_text())
            gate_failures.extend(f"e1 · {f}" for f in check_delivery_gate(e1_data.get("summary", []), args.candidate, args.baseline))

        e2_json = out_dir / "e2-offline.json"
        e2_cmd = [sys.executable, "bench/v3/e2_offline.py", "--sidecar", str(sidecar_bin), "--checkout", str(checkout), "--engines", args.engines, "--json", str(e2_json)]
        if args.codegraph:
            e2_cmd += ["--codegraph", args.codegraph]
        run_step("P1 E2 offline navigation", e2_cmd)
        studio_inputs.append(str(e2_json))

    html_out = out_dir / "observatory.html"
    run_step("Studio Observatory HTML", [str(sidecar_bin), "studio", "--out", str(html_out), *studio_inputs])

    if args.candidate:
        if gate_failures:
            print(f"\n[GATE REGRESSIONS] {args.candidate} vs {args.baseline}:")
            for f in gate_failures:
                print(" -", f)
            if args.enforce_gate:
                return 1
        else:
            print(f"\n[GATE OK] {args.candidate} had zero regressions vs {args.baseline}")

    print(f"\nDone in {time.perf_counter() - t_total:.2f}s -> {html_out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
