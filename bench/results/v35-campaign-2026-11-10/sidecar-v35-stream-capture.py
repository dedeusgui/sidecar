import hashlib, json, pathlib, sys, tempfile
sys.path.insert(0, '/home/dedeusgui/orca/sidecar/bench/v3.5')
import capture_baselines as c

def parse(raw):
    report = json.loads(raw)
    evidence = [{'path': x.get('path', ''), 'lines': x.get('lines', '')} for x in (report.get('evidence') or [])]
    return str(report.get('context') or ''), evidence, report

c._context_from_explain = parse
def safe_hash(home):
    digest = hashlib.sha256()
    for path in sorted(home.glob('repos/*/index.db*')):
        if path.name.endswith('.lock'):
            continue
        try:
            data = path.read_bytes()
        except FileNotFoundError:
            continue
        digest.update(path.relative_to(home).as_posix().encode())
        digest.update(b'\0')
        digest.update(data)
        digest.update(b'\0')
    return digest.hexdigest()
c._tree_sha256 = safe_hash
c.freeze.verify_sources(pathlib.Path('/tmp/sidecar-v35-repos'))
out = pathlib.Path('/tmp/sidecar-v35-e1e2-rows.jsonl')
done = set()
if out.exists():
    done = {(r['suite'], r['case_id'], r['engine'], r['surface'], r['cache_state']) for r in (json.loads(s) for s in out.read_text().splitlines())}
with tempfile.TemporaryDirectory(prefix='sidecar-v35-stream-') as temp, out.open('a') as f:
    for suite in ('e1', 'e2'):
        _, cases = c._load_cases(suite)
        for n, case in enumerate(cases, 1):
            for engine in ('v3', 'v3.5'):
                for surface in c.SURFACES:
                    if all((suite, case['id'], engine, surface, cache) in done for cache in c.CACHE_STATES):
                        continue
                    rows = c._capture_case(pathlib.Path('/tmp/sidecar-v35-bench'), pathlib.Path('/tmp/sidecar-v35-repos') / case['repo'], case, engine, surface, pathlib.Path(temp))
                    for row in rows:
                        row['suite'] = suite
                        f.write(json.dumps(row) + '\n')
                    f.flush()
            print(f'{suite}: {n}/{len(cases)}', flush=True)
