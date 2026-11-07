# Relatório sidecar-eval

Ambiente: 2026-11-07T03:16:08Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

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
| v2 sem intent | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem contract | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem packer | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |

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
| v2 sem intent | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem contract | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem packer | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |

### Split: holdout

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| bm25 (fts5, raw terms) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| bm25 + traducao/normalizacao | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| lexical + grafo | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| lexical + grafo + git | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2.1 (intent router) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 (intent + contract + packer) | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem intent | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem contract | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |
| v2 sem packer | 0.0% | 0.0% | 0.0% | 0.000 | 0.0% | 0.0% | 0.0% | 0.00 | 0.00 | 0 |

### Split: development

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 12.5% | 27.5% | 32.5% | 0.189 | 16.8% | 22.5% | 70.0% | 6.31 | 31.29 | 56 |
| bm25 (fts5, raw terms) | 30.0% | 45.0% | 52.5% | 0.382 | 20.7% | 20.0% | 10.0% | 2.10 | 5.77 | 260 |
| bm25 + traducao/normalizacao | 70.0% | 90.0% | 95.0% | 0.780 | 32.5% | 0.0% | 90.0% | 3.14 | 7.31 | 392 |
| lexical + grafo | 47.5% | 97.5% | 100.0% | 0.667 | 22.1% | 0.0% | 0.0% | 8.96 | 13.12 | 385 |
| lexical + grafo + git | 47.5% | 97.5% | 100.0% | 0.667 | 22.1% | 0.0% | 0.0% | 9.82 | 13.55 | 398 |
| v2.1 (intent router) | 47.5% | 97.5% | 100.0% | 0.667 | 25.3% | 0.0% | 0.0% | 5.79 | 11.84 | 378 |
| v2 (intent + contract + packer) | 47.5% | 97.5% | 100.0% | 0.667 | 25.3% | 0.0% | 0.0% | 14.40 | 35.35 | 425 |
| v2 sem intent | 47.5% | 97.5% | 100.0% | 0.667 | 22.8% | 0.0% | 0.0% | 17.25 | 35.65 | 442 |
| v2 sem contract | 47.5% | 97.5% | 100.0% | 0.667 | 25.3% | 0.0% | 0.0% | 8.68 | 14.81 | 378 |
| v2 sem packer | 47.5% | 97.5% | 100.0% | 0.667 | 25.3% | 0.0% | 0.0% | 11.85 | 26.43 | 425 |

### Cenários (checks mecânicos)

- textual (substring): 16/50 passam · falhas: R01-1 (gold: 0/1 no top 10) · R01-2 (gold: 0/1 no top 10) · R01-3 (gold: 0/1 no top 10) · R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R03-1 (gold: 0/1 no top 10) · R03-3 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R03-5 (no_evidence: contexto injetado) · R04-3 (gold: 0/1 no top 10) · R04-4 (gold: 0/1 no top 10) · R04-5 (no_evidence: contexto injetado) · R05-1 (gold: 0/1 no top 10) · R05-2 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R06-1 (gold: 0/1 no top 10) · R06-2 (gold: 0/1 no top 10) · R06-3 (gold: 0/1 no top 10) · R06-4 (gold: 0/1 no top 10) · R06-5 (no_evidence: contexto injetado) · R07-3 (gold: 0/1 no top 10) · R07-5 (no_evidence: contexto injetado) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-5 (no_evidence: contexto injetado) · R09-1 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-3 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-4 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- bm25 (fts5, raw terms): 30/50 passam · falhas: R01-2 (gold: 0/1 no top 10) · R01-3 (gold: 0/1 no top 10) · R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R03-3 (gold: 0/1 no top 10) · R04-3 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R06-2 (gold: 0/1 no top 10) · R06-3 (gold: 0/1 no top 10) · R06-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-1 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-3 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10)
- bm25 + traducao/normalizacao: 39/50 passam · falhas: R01-5 (no_evidence: contexto injetado) · R02-5 (no_evidence: contexto injetado) · R03-5 (no_evidence: contexto injetado) · R04-3 (gold: 0/1 no top 10) · R04-5 (no_evidence: contexto injetado) · R06-5 (no_evidence: contexto injetado) · R07-5 (no_evidence: contexto injetado) · R08-1 (gold: 0/1 no top 10) · R08-5 (no_evidence: contexto injetado) · R09-5 (no_evidence: contexto injetado) · R10-5 (no_evidence: contexto injetado)
- lexical + grafo: 50/50 passam · falhas: nenhuma
- lexical + grafo + git: 50/50 passam · falhas: nenhuma
- v2.1 (intent router): 50/50 passam · falhas: nenhuma
- v2 (intent + contract + packer): 50/50 passam · falhas: nenhuma
- v2 sem intent: 50/50 passam · falhas: nenhuma
- v2 sem contract: 50/50 passam · falhas: nenhuma
- v2 sem packer: 50/50 passam · falhas: nenhuma
