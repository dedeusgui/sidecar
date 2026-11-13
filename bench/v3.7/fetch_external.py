#!/usr/bin/env python3
"""Fetch the pinned external benchmark repositories used by the V3.7 cohorts.

Each repository is a shallow fetch of the exact commit named in its manifest, read anonymously from its
public URL. Nothing is pushed or written upstream. Use --verify-only to check existing checkouts against the
pinned commits without touching the network.

Defaults: manifests under bench/ (ten-repos and E1) and checkouts under ~/bench-external.
"""
from __future__ import annotations

import argparse
import json
import os
import pathlib
import subprocess
import sys

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
MANIFESTS = (
    REPO_ROOT / "bench" / "ten_repos_cases.json",
    REPO_ROOT / "bench" / "v3.5" / "e1_cases.json",
)


def pinned_repos() -> dict[str, dict]:
    repos: dict[str, dict] = {}
    for path in MANIFESTS:
        for slug, meta in json.loads(path.read_text())["repos"].items():
            repos.setdefault(slug, meta)
    return repos


def head_of(path: pathlib.Path) -> str:
    result = subprocess.run(["git", "-C", str(path), "rev-parse", "HEAD"], capture_output=True, text=True)
    return result.stdout.strip() if result.returncode == 0 else ""


def fetch(slug: str, meta: dict, dst: pathlib.Path) -> bool:
    env = dict(os.environ, GIT_LFS_SKIP_SMUDGE="1", GIT_TERMINAL_PROMPT="0")
    dst.mkdir(parents=True, exist_ok=True)
    if not (dst / ".git").exists():
        subprocess.run(["git", "init", "-q", str(dst)], check=True)
        subprocess.run(["git", "-C", str(dst), "remote", "add", "origin", meta["source"]], check=True)
    result = subprocess.run(["git", "-C", str(dst), "fetch", "--depth", "1", "origin", meta["commit"]],
                            env=env, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"FAILED {slug}: {result.stderr.strip()[-300:]}", flush=True)
        return False
    subprocess.run(["git", "-C", str(dst), "checkout", "-q", "FETCH_HEAD"], env=env, check=True)
    return True


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--root", default=str(pathlib.Path.home() / "bench-external"), help="checkout directory")
    parser.add_argument("--verify-only", action="store_true", help="compare existing HEADs with pins; no network")
    args = parser.parse_args()
    root = pathlib.Path(args.root)
    failures = 0
    for slug, meta in sorted(pinned_repos().items()):
        dst = root / slug
        if args.verify_only:
            head = head_of(dst)
            ok = head == meta["commit"]
            print(("ok      " if ok else "MISMATCH"), slug, head[:12] or "(missing)", "pin", meta["commit"][:12], flush=True)
            failures += 0 if ok else 1
            continue
        if head_of(dst) == meta["commit"]:
            print("ok (cached)", slug, flush=True)
            continue
        print("fetching", slug, meta["source"], meta["commit"][:12], flush=True)
        if not fetch(slug, meta, dst):
            failures += 1
    print("DONE" if not failures else f"DONE with {failures} problem(s)", flush=True)
    return 0 if not failures else 1


if __name__ == "__main__":
    sys.exit(main())
