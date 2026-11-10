# V3.5 Stage 0 freeze

This folder freezes fresh E1 and E2 cohorts before V3.5 retrieval tuning. The E1 set has 30 short positive prompts and 40 no-gold negatives in both PT-BR and English, across five previously unused pinned repositories and five languages. E2 has 20 navigation questions across those repositories and languages. Every case, gold path/span, repository commit, license, and source URL is covered by `freeze_manifest.json` and checked by `freeze.py`.

The cohorts are **author-coupled**: this worker authored the prompts and gold while implementing Stage 0. The repository revisions are pinned, and the E2 source spans are checked by SHA-256, but semantic gold still needs review by someone who did not author or tune Sidecar. Treat these as lower-confidence validation until that review is recorded; do not present them as independent evidence.

## Verify the freeze

```bash
python3 bench/v3.5/freeze.py verify
python3 -m unittest discover -s bench/v3.5 -p 'test_*.py'
SIDECAR_V35_CHECKOUT=/path/to/pinned-repositories python3 -m unittest discover -s bench/v3.5 -p 'test_*.py'
```

`freeze.py` reads only the declared Stage 0 artifacts, the published `bench/lab/corpus/manifest.json` hash metadata, and its own freeze manifest. It does not open or enumerate any sealed task, oracle, reference patch, or holdout file. `holdout_prompts_sha256` is copied from the already-published corpus manifest and included only as a guard reference.

To validate gold against checkouts at the exact pinned commits, put each checkout under one directory using the repository slug as its folder name, then run:

```bash
python3 bench/v3.5/freeze.py verify-sources --checkout /path/to/pinned-repositories
```

The checkout roots used when this cohort was authored were fetched from the URLs and commits recorded in the JSON files. The verifier reads only positively-golded E1 files and E2 line spans from those checkouts; it runs no repository code.

To expand E1 for the existing `sidecar-eval --external-cases` interface, run:

```bash
python3 bench/v3.5/make_e1_manifest.py \
  --checkout /path/to/pinned-repositories \
  --out /tmp/sidecar-v35-e1-cases.json
```

This verifies the freeze and source hashes, then writes the 70 validation cases; it does not run an evaluator.

## Baseline snapshots

`baseline_plan.json` records the required V1/V2/V3 fields and the capture command. Output snapshots, cold/warm timings, and index-store hashes are **not captured** in this commit: the task forbids running the benchmark. Once a later operator authorizes that local snapshot run and has the pinned repositories and Sidecar binary available, use:

```bash
git worktree add /tmp/sidecar-v35-baseline-src 828c3c3268791b81295dc32b3f2dfcd6782d7351
(cd /tmp/sidecar-v35-baseline-src && go build -o /tmp/sidecar-v35-baseline ./cmd/sidecar)
python3 bench/v3.5/capture_baselines.py \
  --sidecar /tmp/sidecar-v35-baseline \
  --checkout /path/to/pinned-repositories \
  --suite e1,e2 \
  --out /tmp/sidecar-v35-baseline.json \
  --run
```

The capture command runs offline V1/V2/V3 `explain --json` and a Codex-shaped prompt hook on each frozen case, in isolated Sidecar homes for cold and warm runs. It writes output strings only to the requested local artifact; that artifact retains output hashes, lengths, evidence paths/lines, elapsed time, index-store hashes, and E2 gold-term coverage. That term coverage is diagnostic and does not score answer correctness. It does not run a paid agent, a model, a network call, `sidecar-eval`, or any holdout case. Do not run it as part of this Stage 0 task.
