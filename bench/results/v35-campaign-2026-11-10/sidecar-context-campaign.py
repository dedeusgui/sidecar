import hashlib, json, os, pathlib, subprocess, time

root = pathlib.Path('/home/dedeusgui/orca/sidecar')
base = pathlib.Path('/tmp/sidecar-real-bench')
m = json.loads((base / 'ten-repos-manifest.json').read_text())
cases = list(m['cases'])
neg = json.loads((root / 'bench/negatives/hard_negatives.json').read_text())
for split in ('dev', 'validation'):
    if split not in neg['splits']:
        continue
    for i, prompt in enumerate(neg['splits'][split]):
        for repo in m['repos']:
            cases.append({'id': f'HN-{split}-{i+1}@{repo}', 'repo': repo, 'prompt': prompt, 'category': 'negative', 'relevant': []})
out = pathlib.Path('/tmp/sidecar-context-campaign.jsonl')
with out.open('x') as f:
    for case in cases:
        for engine in ('v3', 'v3.5'):
            env = dict(os.environ, SIDECAR_HOME='/tmp/sidecar-context-campaign-home')
            cmd = ['/tmp/sidecar-v35-bench', 'explain', '--engine', engine, '--root', m['repos'][case['repo']]['path'], '--json']
            t = time.monotonic()
            p = subprocess.run(cmd, input=case['prompt'], text=True, capture_output=True, env=env, timeout=120)
            if p.returncode:
                raise RuntimeError(f"{case['id']} {engine}: {p.stderr[-500:]}")
            d = json.loads(p.stdout)
            if d.get('engine') != engine:
                raise RuntimeError(f"unexpected engine: {d.get('engine')}")
            ctx = d.get('context') or ''
            gold = [r['path'] for r in case.get('relevant', [])]
            row = {'case':case['id'], 'repo':case['repo'], 'category':case['category'], 'engine':engine,
                   'context_chars':len(ctx), 'context_sha256':hashlib.sha256(ctx.encode()).hexdigest(),
                   'gold_in_context':any(path in ctx for path in gold) if gold else None,
                   'elapsed_ms':round((time.monotonic()-t)*1000, 2)}
            f.write(json.dumps(row)+'\n'); f.flush()
