# Relatório sidecar-eval

Ambiente: 2026-11-07T01:51:04Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

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
| textual (substring) | 16.7% | 44.4% | 50.0% | 0.290 | 6.2% | 16.7% | 50.0% | 1.61 | 2.17 | 80 |
| bm25 (fts5, raw terms) | 33.3% | 61.1% | 61.1% | 0.423 | 8.6% | 11.1% | 50.0% | 2.43 | 3.53 | 320 |
| bm25 + traducao/normalizacao | 27.8% | 72.2% | 72.2% | 0.415 | 9.0% | 0.0% | 100.0% | 3.03 | 4.06 | 361 |
| lexical + grafo | 33.3% | 61.1% | 61.1% | 0.423 | 7.6% | 0.0% | 100.0% | 9.27 | 11.69 | 406 |
| lexical + grafo + git | 33.3% | 61.1% | 61.1% | 0.423 | 7.6% | 0.0% | 100.0% | 10.24 | 12.80 | 454 |
| v2.1 (intent router) | 33.3% | 61.1% | 61.1% | 0.428 | 7.6% | 0.0% | 100.0% | 5.04 | 12.89 | 409 |
| v2 (intent + contract + packer) | 27.8% | 50.0% | 50.0% | 0.354 | 6.5% | 0.0% | 100.0% | 9.82 | 18.89 | 411 |
| v2 sem intent | 27.8% | 50.0% | 50.0% | 0.354 | 6.8% | 0.0% | 100.0% | 14.96 | 18.89 | 440 |
| v2 sem contract | 33.3% | 61.1% | 61.1% | 0.428 | 7.8% | 0.0% | 100.0% | 7.38 | 15.79 | 401 |
| v2 sem packer | 33.3% | 61.1% | 61.1% | 0.428 | 7.6% | 0.0% | 100.0% | 8.90 | 19.01 | 432 |

### Cenários (checks mecânicos)

- textual (substring): 10/20 passam · falhas: PC01 (gold: 0/1 no top 10) · PC02 (gold: 0/1 no top 10) · PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · CH01 (gold: 0/1 no top 10) · CH02 (gold: 0/1 no top 10) · CH05 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- bm25 (fts5, raw terms): 12/20 passam · falhas: PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC06 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · CH02 (gold: 0/1 no top 10) · CH05 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- bm25 + traducao/normalizacao: 13/20 passam · falhas: PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · PC10 (no_evidence: contexto injetado) · CH02 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- lexical + grafo: 11/20 passam · falhas: PC02 (gold: 0/1 no top 10) · PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · PC10 (no_evidence: contexto injetado) · CH02 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH07 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- lexical + grafo + git: 11/20 passam · falhas: PC02 (gold: 0/1 no top 10) · PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · PC10 (no_evidence: contexto injetado) · CH02 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH07 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- v2.1 (intent router): 11/20 passam · falhas: PC02 (gold: 0/1 no top 10) · PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · PC10 (no_evidence: contexto injetado) · CH02 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH07 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- v2 (intent + contract + packer): 9/20 passam · falhas: PC02 (gold: 0/1 no top 10) · PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · PC10 (no_evidence: contexto injetado) · CH01 (gold: 0/1 no top 10) · CH02 (gold: 0/1 no top 10) · CH05 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH07 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- v2 sem intent: 9/20 passam · falhas: PC02 (gold: 0/1 no top 10) · PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · PC10 (no_evidence: contexto injetado) · CH01 (gold: 0/1 no top 10) · CH02 (gold: 0/1 no top 10) · CH05 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH07 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- v2 sem contract: 11/20 passam · falhas: PC02 (gold: 0/1 no top 10) · PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · PC10 (no_evidence: contexto injetado) · CH02 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH07 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)
- v2 sem packer: 11/20 passam · falhas: PC02 (gold: 0/1 no top 10) · PC03 (gold: 0/1 no top 10) · PC04 (gold: 0/1 no top 10) · PC08 (gold: 0/1 no top 10) · PC10 (no_evidence: contexto injetado) · CH02 (gold: 0/1 no top 10) · CH06 (gold: 0/1 no top 10) · CH07 (gold: 0/1 no top 10) · CH10 (no_evidence: contexto injetado)

### Comparação com baseline

Baseline: 2026-11-07T01:48:25Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

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

#### Regressões (8)

- bm25 + traducao/normalizacao · caso PC10 (real): falso positivo novo (passou a injetar contexto)
- lexical + grafo · caso PC10 (real): falso positivo novo (passou a injetar contexto)
- lexical + grafo + git · caso PC10 (real): falso positivo novo (passou a injetar contexto)
- v2.1 (intent router) · caso PC10 (real): falso positivo novo (passou a injetar contexto)
- v2 (intent + contract + packer) · caso PC10 (real): falso positivo novo (passou a injetar contexto)
- v2 sem intent · caso PC10 (real): falso positivo novo (passou a injetar contexto)
- v2 sem contract · caso PC10 (real): falso positivo novo (passou a injetar contexto)
- v2 sem packer · caso PC10 (real): falso positivo novo (passou a injetar contexto)

#### Melhorias (14)

- bm25 + traducao/normalizacao · caso PC06 (real): Hit@5 falso → verdadeiro
- bm25 + traducao/normalizacao · caso CH05 (real): Hit@5 falso → verdadeiro
- lexical + grafo · caso PC06 (real): Hit@5 falso → verdadeiro
- lexical + grafo · caso CH05 (real): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso PC06 (real): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso CH05 (real): Hit@5 falso → verdadeiro
- v2.1 (intent router) · caso PC06 (real): Hit@5 falso → verdadeiro
- v2.1 (intent router) · caso CH05 (real): Hit@5 falso → verdadeiro
- v2 (intent + contract + packer) · caso PC06 (real): Hit@5 falso → verdadeiro
- v2 sem intent · caso PC06 (real): Hit@5 falso → verdadeiro
- v2 sem contract · caso PC06 (real): Hit@5 falso → verdadeiro
- v2 sem contract · caso CH05 (real): Hit@5 falso → verdadeiro
- v2 sem packer · caso PC06 (real): Hit@5 falso → verdadeiro
- v2 sem packer · caso CH05 (real): Hit@5 falso → verdadeiro
