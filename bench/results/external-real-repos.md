# Relatório sidecar-eval

Ambiente: 2026-11-07T02:18:54Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

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
| textual (substring) | 16.7% | 44.4% | 50.0% | 0.290 | 6.2% | 16.7% | 50.0% | 1.52 | 2.44 | 80 |
| bm25 (fts5, raw terms) | 33.3% | 61.1% | 61.1% | 0.423 | 8.6% | 11.1% | 50.0% | 2.54 | 4.71 | 320 |
| bm25 + traducao/normalizacao | 55.6% | 94.4% | 100.0% | 0.738 | 12.5% | 0.0% | 100.0% | 3.19 | 5.63 | 379 |
| lexical + grafo | 61.1% | 94.4% | 100.0% | 0.766 | 12.5% | 0.0% | 0.0% | 10.11 | 14.42 | 417 |
| lexical + grafo + git | 61.1% | 94.4% | 100.0% | 0.766 | 12.5% | 0.0% | 0.0% | 11.10 | 15.11 | 471 |
| v2.1 (intent router) | 61.1% | 94.4% | 100.0% | 0.766 | 12.5% | 0.0% | 0.0% | 5.33 | 13.26 | 427 |
| v2 (intent + contract + packer) | 61.1% | 94.4% | 100.0% | 0.766 | 12.5% | 0.0% | 0.0% | 10.75 | 22.80 | 450 |
| v2 sem intent | 61.1% | 94.4% | 100.0% | 0.766 | 12.5% | 0.0% | 0.0% | 15.12 | 21.32 | 490 |
| v2 sem contract | 61.1% | 94.4% | 100.0% | 0.766 | 12.5% | 0.0% | 0.0% | 7.50 | 16.57 | 427 |
| v2 sem packer | 61.1% | 94.4% | 100.0% | 0.766 | 12.5% | 0.0% | 0.0% | 9.35 | 18.73 | 450 |

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

### Comparação com baseline

Baseline: 2026-11-07T01:51:04Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

Δ = atual − baseline (Δ em % são pontos percentuais). Em Falso Positivo, P50 e Tokens, Δ positivo é pior; nos demais, positivo é melhor.

| Split | Método | Δ Hit@5 | Δ Recall@10 | Δ MRR | Δ Precisão | Δ Falso Positivo | Δ P50 ms | Δ Tokens |
|---|---|---|---|---|---|---|---|---|
| tune | textual (substring) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | textual (substring) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | textual (substring) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| tune | bm25 (fts5, raw terms) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | bm25 (fts5, raw terms) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | bm25 (fts5, raw terms) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| tune | bm25 + traducao/normalizacao | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | bm25 + traducao/normalizacao | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | bm25 + traducao/normalizacao | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| tune | lexical + grafo | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | lexical + grafo | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | lexical + grafo | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| tune | lexical + grafo + git | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | lexical + grafo + git | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | lexical + grafo + git | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| tune | v2.1 (intent router) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | v2.1 (intent router) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | v2.1 (intent router) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| tune | v2 (intent + contract + packer) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | v2 (intent + contract + packer) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | v2 (intent + contract + packer) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| tune | v2 sem intent | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | v2 sem intent | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | v2 sem intent | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| tune | v2 sem contract | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | v2 sem contract | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | v2 sem contract | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| tune | v2 sem packer | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| eval | v2 sem packer | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |
| holdout | v2 sem packer | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.00 | +0 |

#### Regressões (0)

Nenhuma regressão.

#### Melhorias (64)

- bm25 + traducao/normalizacao · caso PC03 (real): Hit@5 falso → verdadeiro
- bm25 + traducao/normalizacao · caso PC08 (real): Hit@5 falso → verdadeiro
- bm25 + traducao/normalizacao · caso CH02 (real): Hit@5 falso → verdadeiro
- bm25 + traducao/normalizacao · caso CH06 (real): Hit@5 falso → verdadeiro
- lexical + grafo · caso PC02 (real): Hit@5 falso → verdadeiro
- lexical + grafo · caso PC03 (real): Hit@5 falso → verdadeiro
- lexical + grafo · caso PC08 (real): Hit@5 falso → verdadeiro
- lexical + grafo · caso PC10 (real): falso positivo removido
- lexical + grafo · caso CH02 (real): Hit@5 falso → verdadeiro
- lexical + grafo · caso CH06 (real): Hit@5 falso → verdadeiro
- lexical + grafo · caso CH07 (real): Hit@5 falso → verdadeiro
- lexical + grafo · caso CH10 (real): falso positivo removido
- lexical + grafo + git · caso PC02 (real): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso PC03 (real): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso PC08 (real): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso PC10 (real): falso positivo removido
- lexical + grafo + git · caso CH02 (real): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso CH06 (real): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso CH07 (real): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso CH10 (real): falso positivo removido
- v2.1 (intent router) · caso PC02 (real): Hit@5 falso → verdadeiro
- v2.1 (intent router) · caso PC03 (real): Hit@5 falso → verdadeiro
- v2.1 (intent router) · caso PC08 (real): Hit@5 falso → verdadeiro
- v2.1 (intent router) · caso PC10 (real): falso positivo removido
- v2.1 (intent router) · caso CH02 (real): Hit@5 falso → verdadeiro
- v2.1 (intent router) · caso CH06 (real): Hit@5 falso → verdadeiro
- v2.1 (intent router) · caso CH07 (real): Hit@5 falso → verdadeiro
- v2.1 (intent router) · caso CH10 (real): falso positivo removido
- v2 (intent + contract + packer) · caso PC02 (real): Hit@5 falso → verdadeiro
- v2 (intent + contract + packer) · caso PC03 (real): Hit@5 falso → verdadeiro
- v2 (intent + contract + packer) · caso PC08 (real): Hit@5 falso → verdadeiro
- v2 (intent + contract + packer) · caso PC10 (real): falso positivo removido
- v2 (intent + contract + packer) · caso CH01 (real): Hit@5 falso → verdadeiro
- v2 (intent + contract + packer) · caso CH02 (real): Hit@5 falso → verdadeiro
- v2 (intent + contract + packer) · caso CH05 (real): Hit@5 falso → verdadeiro
- v2 (intent + contract + packer) · caso CH06 (real): Hit@5 falso → verdadeiro
- v2 (intent + contract + packer) · caso CH07 (real): Hit@5 falso → verdadeiro
- v2 (intent + contract + packer) · caso CH10 (real): falso positivo removido
- v2 sem intent · caso PC02 (real): Hit@5 falso → verdadeiro
- v2 sem intent · caso PC03 (real): Hit@5 falso → verdadeiro
- v2 sem intent · caso PC08 (real): Hit@5 falso → verdadeiro
- v2 sem intent · caso PC10 (real): falso positivo removido
- v2 sem intent · caso CH01 (real): Hit@5 falso → verdadeiro
- v2 sem intent · caso CH02 (real): Hit@5 falso → verdadeiro
- v2 sem intent · caso CH05 (real): Hit@5 falso → verdadeiro
- v2 sem intent · caso CH06 (real): Hit@5 falso → verdadeiro
- v2 sem intent · caso CH07 (real): Hit@5 falso → verdadeiro
- v2 sem intent · caso CH10 (real): falso positivo removido
- v2 sem contract · caso PC02 (real): Hit@5 falso → verdadeiro
- v2 sem contract · caso PC03 (real): Hit@5 falso → verdadeiro
- v2 sem contract · caso PC08 (real): Hit@5 falso → verdadeiro
- v2 sem contract · caso PC10 (real): falso positivo removido
- v2 sem contract · caso CH02 (real): Hit@5 falso → verdadeiro
- v2 sem contract · caso CH06 (real): Hit@5 falso → verdadeiro
- v2 sem contract · caso CH07 (real): Hit@5 falso → verdadeiro
- v2 sem contract · caso CH10 (real): falso positivo removido
- v2 sem packer · caso PC02 (real): Hit@5 falso → verdadeiro
- v2 sem packer · caso PC03 (real): Hit@5 falso → verdadeiro
- v2 sem packer · caso PC08 (real): Hit@5 falso → verdadeiro
- v2 sem packer · caso PC10 (real): falso positivo removido
- v2 sem packer · caso CH02 (real): Hit@5 falso → verdadeiro
- v2 sem packer · caso CH06 (real): Hit@5 falso → verdadeiro
- v2 sem packer · caso CH07 (real): Hit@5 falso → verdadeiro
- v2 sem packer · caso CH10 (real): falso positivo removido
