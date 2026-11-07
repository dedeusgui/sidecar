#!/usr/bin/env python3
"""Single illustrative feedback experiment; usage: feedback_chi.py SIDECAR CHI EMPTY_DIR."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time

if len(sys.argv) != 4:
    raise SystemExit(__doc__)
binary, repo, directory = (Path(p).resolve() for p in sys.argv[1:])
head = subprocess.check_output(['git', '-C', str(repo), 'rev-parse', 'HEAD'], text=True).strip()
if head != '9f8adb1aabaa8a91180a821111d3f8e668d1c0bf':
    raise SystemExit('chi checkout is not at the pinned experiment commit')
if subprocess.check_output(['git', '-C', str(repo), 'status', '--porcelain'], text=True).strip():
    raise SystemExit('chi checkout is dirty')
if directory.exists() and any(directory.iterdir()):
    raise SystemExit('experiment directory must be empty')
directory.mkdir(parents=True, exist_ok=True)
env = dict(os.environ, HOME=str(directory), XDG_CONFIG_HOME=str(directory / 'config'),
           XDG_CACHE_HOME=str(directory / 'cache'))

def explain(prompt):
    result = subprocess.run([str(binary), 'explain', '--root', str(repo), '--engine', 'v2', '--json'],
                            input=prompt, text=True, capture_output=True, env=env, check=True)
    return json.loads(result.stdout)

turn1 = explain('Qual middleware aplica timeout a cada requisição?')
related = 'Onde fica o middleware de timeout para requisições?'
cold = explain(related)
expected = ['middleware/throttle_test.go#constant:timeout@292',
            'middleware/timeout.go#function:Timeout@32']
if [e['id'] for e in turn1['evidence'][:2]] != expected:
    raise SystemExit('turn 1 evidence differs from the scenario; no feedback recorded')
report = {'schema_version': 1, 'task_completed': True,
          'helpful_evidence_ids': ['middleware/timeout.go#function:Timeout@32'],
          'misleading_evidence_ids': ['middleware/throttle_test.go#constant:timeout@292'],
          'reason_code': 'noisy_neighbor', 'confidence': 'high', 'additional_search_needed': True}
raw = json.dumps(report, ensure_ascii=False, separators=(',', ':'))
start = time.perf_counter()
result = subprocess.run([str(binary), 'feedback', 'record', '--root', str(repo), '--json'],
                        input=raw, text=True, capture_output=True, env=env, check=True)
elapsed = (time.perf_counter() - start) * 1000
warm = explain(related)
output = {'turn1': turn1, 'cold_related': cold, 'warm_related': warm, 'report': report,
          'report_utf8_bytes': len(raw.encode('utf-8')), 'record_cli_ms': elapsed,
          'records': json.loads(result.stdout)}
(directory / 'feedback-experiment.json').write_text(json.dumps(output, ensure_ascii=False, indent=2) + '\n')
print(f'record CLI: {elapsed:.3f} ms; report: {len(raw.encode("utf-8"))} bytes')
for label, data in [('cold', cold), ('warm', warm)]:
    print(label, [(e['path'], e['name']) for e in data['evidence'][:5]])
