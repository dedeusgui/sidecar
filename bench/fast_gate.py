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


def check_delivery_gate(summaries, candidate, baseline):
    """Compare candidate vs baseline across delivery summary strata without hidden regressions."""
    by_key = {(s["stratum"], s["engine"]): s for s in summaries}
    failures = []
    strata = sorted({s["stratum"] for s in summaries})
    for stratum in strata:
        cand = by_key.get((stratum, candidate))
        base = by_key.get((stratum, baseline))
        if not cand or not base:
            continue
        if cand.get("non_deterministic", 0) > 0:
            failures.append(f"{stratum}: {candidate} non_deterministic={cand['non_deterministic']}")
        if cand.get("trust_label_conflicts", 0) > base.get("trust_label_conflicts", 0):
            failures.append(
                f"{stratum}: {candidate} label conflicts {cand['trust_label_conflicts']} > {baseline} {base['trust_label_conflicts']}"
            )
        if stratum in ("negative", "hard_negative"):
            if cand.get("non_empty", 0) > base.get("non_empty", 0):
                failures.append(
                    f"{stratum}: {candidate} negative leaks {cand['non_empty']} > {baseline} {base['non_empty']}"
                )
        else:
            if cand.get("gold_path_delivered", 0) < base.get("gold_path_delivered", 0):
                failures.append(
                    f"{stratum}: {candidate} gold_path_delivered {cand['gold_path_delivered']} < {baseline} {base['gold_path_delivered']}"
                )
            if cand.get("orientation_only", 0) > base.get("orientation_only", 0):
                failures.append(
                    f"{stratum}: {candidate} orientation_only {cand['orientation_only']} > {baseline} {base['orientation_only']}"
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
    args = ap.parse_args(argv)

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
    run_step(
        "P1 delivery (bench/cases.json)",
        [str(eval_bin), "--cases", "bench/cases.json", "--delivery", "--delivery-engines", args.engines, "--out", str(cases_json)],
    )
    studio_inputs.append(str(cases_json))

    gate_failures = []
    if args.candidate:
        data = json.loads(cases_json.read_text())
        gate_failures.extend(f"cases.json · {f}" for f in check_delivery_gate(data.get("summary", []), args.candidate, args.baseline))

    if args.external_manifest:
        ext_json = out_dir / "delivery-external.json"
        cmd = [str(eval_bin), "--external-cases", args.external_manifest, "--delivery", "--delivery-engines", args.engines, "--out", str(ext_json)]
        if args.negatives and (ROOT / args.negatives).is_file():
            cmd += ["--delivery-negatives", args.negatives]
        run_step("P1 delivery (external manifest)", cmd)
        studio_inputs.append(str(ext_json))
        if args.candidate:
            ext_data = json.loads(ext_json.read_text())
            gate_failures.extend(f"external · {f}" for f in check_delivery_gate(ext_data.get("summary", []), args.candidate, args.baseline))

    if args.checkout:
        checkout = pathlib.Path(args.checkout)
        e1_manifest = out_dir / "e1-manifest.json"
        run_step("P1 build E1 manifest", [sys.executable, "bench/v3.5/make_e1_manifest.py", "--checkout", str(checkout), "--out", str(e1_manifest)])
        e1_json = out_dir / "delivery-e1.json"
        run_step("P1 delivery (E1 cohorts)", [str(eval_bin), "--external-cases", str(e1_manifest), "--delivery", "--delivery-engines", args.engines, "--out", str(e1_json)])
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
