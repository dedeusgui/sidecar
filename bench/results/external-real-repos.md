# Relatório sidecar-eval

Ambiente: 2026-11-07T02:31:27Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

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

### Split: real

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 22.2% | 44.4% | 50.0% | 0.315 | 6.2% | 16.7% | 50.0% | 1.38 | 2.22 | 80 |
| bm25 (fts5, raw terms) | 33.3% | 61.1% | 61.1% | 0.423 | 8.6% | 11.1% | 50.0% | 2.39 | 3.63 | 320 |
| bm25 + traducao/normalizacao | 55.6% | 94.4% | 100.0% | 0.738 | 12.5% | 0.0% | 100.0% | 3.28 | 4.30 | 381 |
| lexical + grafo | 55.6% | 94.4% | 100.0% | 0.748 | 12.5% | 0.0% | 0.0% | 9.14 | 13.14 | 421 |
| lexical + grafo + git | 55.6% | 94.4% | 100.0% | 0.748 | 12.5% | 0.0% | 0.0% | 10.29 | 14.19 | 474 |
| v2.1 (intent router) | 55.6% | 94.4% | 100.0% | 0.748 | 12.5% | 0.0% | 0.0% | 5.30 | 11.59 | 429 |
| v2 (intent + contract + packer) | 55.6% | 94.4% | 100.0% | 0.748 | 12.5% | 0.0% | 0.0% | 9.04 | 18.50 | 452 |
| v2 sem intent | 55.6% | 94.4% | 100.0% | 0.748 | 12.5% | 0.0% | 0.0% | 13.81 | 20.87 | 495 |
| v2 sem contract | 55.6% | 94.4% | 100.0% | 0.748 | 12.5% | 0.0% | 0.0% | 7.30 | 14.09 | 429 |
| v2 sem packer | 55.6% | 94.4% | 100.0% | 0.748 | 12.5% | 0.0% | 0.0% | 9.57 | 18.05 | 452 |

### Cenários (checks mecânicos)

- textual (substring): 10/20 passam · falhas: PC01 (gold: 0/1 no top 10) · PC02 (gold: 0/1 no top 10) · PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · CH01 (gold: 0/1 no top 10) · CH02 (gold: 0/1 no top 10) · CH05 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- bm25 (fts5, raw terms): 12/20 passam · falhas: PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC06 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · CH02 (gold: 0/1 no top 10) · CH05 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- bm25 + traducao/normalizacao: 18/20 passam · falhas: PC10 (no_evidence: contexto injetado) · CH10 (no_evidence: contexto injetado)
- lexical + grafo: 20/20 passam · falhas: nenhuma
- lexical + grafo + git: 20/20 passam · falhas: nenhuma
- v2.1 (intent router): 20/20 passam · falhas: nenhuma
- v2 (intent + contract + packer): 20/20 passam · falhas: nenhuma
- v2 sem intent: 20/20 passam · falhas: nenhuma
- v2 sem contract: 20/20 passam · falhas: nenhuma
- v2 sem packer: 20/20 passam · falhas: nenhuma
