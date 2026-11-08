# Relatório sidecar-eval

Ambiente: 2026-11-08T14:50:30Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

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
| textual (substring) | 12.5% | 25.0% | 32.5% | 0.187 | 16.8% | 22.5% | 70.0% | 6.91 | 36.18 | 56 |
| bm25 (fts5, raw terms) | 30.0% | 45.0% | 52.5% | 0.382 | 20.7% | 20.0% | 10.0% | 1.95 | 5.74 | 259 |
| bm25 + traducao/normalizacao | 70.0% | 90.0% | 95.0% | 0.780 | 32.5% | 0.0% | 90.0% | 3.09 | 6.88 | 394 |
| lexical + grafo | 50.0% | 97.5% | 100.0% | 0.676 | 24.7% | 0.0% | 0.0% | 7.12 | 12.29 | 374 |
| lexical + grafo + git | 50.0% | 97.5% | 100.0% | 0.676 | 24.7% | 0.0% | 0.0% | 8.21 | 14.23 | 374 |
| v2.1 (intent router) | 50.0% | 97.5% | 100.0% | 0.676 | 26.4% | 0.0% | 0.0% | 4.79 | 11.21 | 370 |
| v2 (intent + contract + packer) | 50.0% | 97.5% | 100.0% | 0.676 | 26.4% | 0.0% | 0.0% | 13.22 | 32.99 | 417 |
| v2 sem intent | 50.0% | 97.5% | 100.0% | 0.676 | 24.7% | 0.0% | 0.0% | 16.90 | 34.34 | 421 |
| v2 sem contract | 50.0% | 97.5% | 100.0% | 0.676 | 26.4% | 0.0% | 0.0% | 8.34 | 13.61 | 370 |
| v2 sem packer | 50.0% | 97.5% | 100.0% | 0.676 | 26.4% | 0.0% | 0.0% | 11.10 | 25.22 | 417 |

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
