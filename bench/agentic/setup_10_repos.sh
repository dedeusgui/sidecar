#!/usr/bin/env bash
# Download the pinned public repositories and materialize the external evaluator manifest.
set -euo pipefail
base=${1:-/tmp/sidecar-real-bench}
project=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
mkdir -p "$base"
base=$(cd "$base" && pwd)
python3 - "$project/bench/ten_repos_cases.json" "$base" <<'PY'
import json, pathlib, subprocess, sys
manifest = json.loads(pathlib.Path(sys.argv[1]).read_text())
base = pathlib.Path(sys.argv[2])
for name, repo in manifest['repos'].items():
    path = base / name
    if not path.exists():
        subprocess.run(['git', 'clone', '--no-checkout', '--depth', '1', repo['source'], str(path)], check=True)
        subprocess.run(['git', '-C', str(path), 'fetch', '--depth', '1', 'origin', repo['commit']], check=True)
        subprocess.run(['git', '-C', str(path), 'checkout', '--detach', repo['commit']], check=True)
    else:
        head = subprocess.check_output(['git', '-C', str(path), 'rev-parse', 'HEAD'], text=True).strip()
        dirty = subprocess.check_output(['git', '-C', str(path), 'status', '--porcelain'], text=True)
        if head != repo['commit'] or dirty:
            raise SystemExit(f'{path}: checkout differs from pin or is dirty; refusing to modify it')
    repo['path'] = str(path)
out = base / 'ten-repos-manifest.json'
out.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
print(out)
PY
