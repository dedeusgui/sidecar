# Relatório sidecar-eval

Ambiente: 2026-11-10T23:14:37Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

### Split: tune

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| bm25 (fts5, raw terms) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| bm25 + traducao/normalizacao | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| lexical + grafo | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| lexical + grafo + git | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2.1 (intent router) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 (intent + contract + packer) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 adaptive (policy) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 + task brief | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem intent | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem contract | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem packer | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v3 (relevance gate + effort router) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |

### Split: eval

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| bm25 (fts5, raw terms) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| bm25 + traducao/normalizacao | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| lexical + grafo | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| lexical + grafo + git | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2.1 (intent router) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 (intent + contract + packer) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 adaptive (policy) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 + task brief | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem intent | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem contract | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem packer | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v3 (relevance gate + effort router) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |

### Split: development

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 5.0% | 25.0% | 35.0% | 0.131 | 15.2% | 22.5% | 70.0% | 11.31 | 56.47 | 56 |
| bm25 (fts5, raw terms) | 35.0% | 50.0% | 52.5% | 0.424 | 20.3% | 20.0% | 20.0% | 3.35 | 9.28 | 259 |
| bm25 + traducao/normalizacao | 72.5% | 90.0% | 95.0% | 0.792 | 33.4% | 0.0% | 90.0% | 5.00 | 10.58 | 394 |
| lexical + grafo | 47.5% | 87.5% | 92.5% | 0.651 | 30.7% | 0.0% | 0.0% | 6.13 | 13.89 | 398 |
| lexical + grafo + git | 47.5% | 87.5% | 92.5% | 0.651 | 30.7% | 0.0% | 0.0% | 5.44 | 11.93 | 398 |
| v2.1 (intent router) | 52.5% | 87.5% | 92.5% | 0.676 | 32.7% | 0.0% | 0.0% | 4.63 | 9.54 | 387 |
| v2 (intent + contract + packer) | 52.5% | 87.5% | 92.5% | 0.676 | 32.3% | 2.5% | 0.0% | 17.74 | 60.74 | 400 |
| v2 adaptive (policy) | 52.5% | 87.5% | 92.5% | 0.676 | 32.6% | 2.5% | 0.0% | 10.70 | 50.86 | 384 |
| v2 + task brief | 52.5% | 87.5% | 92.5% | 0.676 | 32.3% | 2.5% | 0.0% | 14.87 | 60.01 | 370 |
| v2 sem intent | 47.5% | 87.5% | 92.5% | 0.651 | 29.8% | 2.5% | 0.0% | 16.35 | 57.50 | 415 |
| v2 sem contract | 52.5% | 87.5% | 92.5% | 0.676 | 33.1% | 2.5% | 0.0% | 10.36 | 44.84 | 367 |
| v2 sem packer | 52.5% | 87.5% | 92.5% | 0.676 | 32.7% | 0.0% | 0.0% | 14.91 | 59.42 | 429 |
| v3 (relevance gate + effort router) | 52.5% | 87.5% | 87.5% | 0.669 | 34.5% | 2.5% | 0.0% | 11.02 | 53.43 | 301 |

### Cenários (checks mecânicos)

- textual (substring): 17/50 passam · falhas: R01-1 (gold: 0/1 no top 10) · R01-2 (gold: 0/1 no top 10) · R01-3 (gold: 0/1 no top 10) · R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R03-1 (gold: 0/1 no top 10) · R03-3 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R03-5 (no_evidence: contexto injetado) · R04-1 (gold: 0/1 no top 10) · R04-2 (gold: 0/1 no top 10) · R04-3 (gold: 0/1 no top 10) · R04-4 (gold: 0/1 no top 10) · R04-5 (no_evidence: contexto injetado) · R05-1 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R06-1 (gold: 0/1 no top 10) · R06-2 (gold: 0/1 no top 10) · R06-3 (gold: 0/1 no top 10) · R06-5 (no_evidence: contexto injetado) · R07-3 (gold: 0/1 no top 10) · R07-5 (no_evidence: contexto injetado) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-5 (no_evidence: contexto injetado) · R09-1 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-3 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-4 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- bm25 (fts5, raw terms): 29/50 passam · falhas: R01-2 (gold: 0/1 no top 10) · R01-3 (gold: 0/1 no top 10) · R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R03-3 (gold: 0/1 no top 10) · R04-3 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R06-2 (gold: 0/1 no top 10) · R06-3 (gold: 0/1 no top 10) · R06-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-1 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-3 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- bm25 + traducao/normalizacao: 39/50 passam · falhas: R01-5 (no_evidence: contexto injetado) · R02-5 (no_evidence: contexto injetado) · R03-5 (no_evidence: contexto injetado) · R04-3 (gold: 0/1 no top 10) · R04-5 (no_evidence: contexto injetado) · R06-5 (no_evidence: contexto injetado) · R07-5 (no_evidence: contexto injetado) · R08-1 (gold: 0/1 no top 10) · R08-5 (no_evidence: contexto injetado) · R09-5 (no_evidence: contexto injetado) · R10-5 (no_evidence: contexto injetado)
- lexical + grafo: 47/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10)
- lexical + grafo + git: 47/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10)
- v2.1 (intent router): 47/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10)
- v2 (intent + contract + packer): 47/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10)
- v2 adaptive (policy): 47/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10)
- v2 + task brief: 47/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10)
- v2 sem intent: 47/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10)
- v2 sem contract: 47/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10)
- v2 sem packer: 47/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10)
- v3 (relevance gate + effort router): 45/50 passam · falhas: R04-3 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R09-1 (gold: 0/1 no top 10)
