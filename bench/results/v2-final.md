# Relatório sidecar-eval

Ambiente: 2026-11-07T01:37:48Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

### Split: tune

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 10.0% | 15.0% | 15.0% | 0.113 | 13.2% | 40.0% | 0.0% | 0.18 | 0.31 | 12 |
| bm25 (fts5, raw terms) | 5.0% | 20.0% | 20.0% | 0.085 | 7.0% | 35.0% | 0.0% | 0.71 | 1.11 | 70 |
| bm25 + traducao/normalizacao | 50.0% | 70.0% | 87.5% | 0.588 | 13.6% | 0.0% | 0.0% | 1.46 | 2.48 | 253 |
| lexical + grafo | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 6.69 | 7.97 | 306 |
| lexical + grafo + git | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 7.67 | 12.38 | 356 |
| v2.1 (intent router) | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 4.71 | 8.71 | 347 |
| v2 (intent + contract + packer) | 55.0% | 80.0% | 80.0% | 0.629 | 12.7% | 0.0% | 0.0% | 8.25 | 14.23 | 368 |
| v2 sem intent | 55.0% | 80.0% | 80.0% | 0.629 | 12.7% | 0.0% | 0.0% | 11.15 | 13.46 | 376 |
| v2 sem contract | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 6.84 | 12.01 | 347 |
| v2 sem packer | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 8.35 | 12.54 | 371 |

### Split: eval

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 10.0% | 10.0% | 10.0% | 0.100 | 25.0% | 80.0% | 0.0% | 0.18 | 0.19 | 6 |
| bm25 (fts5, raw terms) | 15.0% | 20.0% | 20.0% | 0.163 | 37.5% | 70.0% | 0.0% | 0.56 | 0.90 | 27 |
| bm25 + traducao/normalizacao | 50.0% | 90.0% | 90.0% | 0.626 | 20.2% | 5.0% | 0.0% | 1.13 | 1.39 | 188 |
| lexical + grafo | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 4.21 | 7.26 | 197 |
| lexical + grafo + git | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 4.79 | 8.06 | 226 |
| v2.1 (intent router) | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 4.33 | 8.03 | 222 |
| v2 (intent + contract + packer) | 50.0% | 85.0% | 85.0% | 0.642 | 24.9% | 5.0% | 0.0% | 7.75 | 11.87 | 227 |
| v2 sem intent | 50.0% | 85.0% | 85.0% | 0.642 | 24.9% | 5.0% | 0.0% | 7.76 | 11.87 | 230 |
| v2 sem contract | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 6.39 | 10.85 | 222 |
| v2 sem packer | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 7.20 | 10.87 | 231 |

### Split: holdout

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 11.8% | 11.8% | 11.8% | 0.118 | 25.0% | 76.5% | 0.0% | 0.18 | 0.20 | 7 |
| bm25 (fts5, raw terms) | 17.6% | 17.6% | 17.6% | 0.176 | 29.2% | 64.7% | 0.0% | 0.64 | 0.92 | 30 |
| bm25 + traducao/normalizacao | 47.1% | 88.2% | 88.2% | 0.625 | 22.5% | 5.9% | 0.0% | 1.10 | 1.70 | 182 |
| lexical + grafo | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 4.00 | 7.23 | 201 |
| lexical + grafo + git | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 4.29 | 8.18 | 225 |
| v2.1 (intent router) | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 3.71 | 7.96 | 217 |
| v2 (intent + contract + packer) | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 6.29 | 11.68 | 228 |
| v2 sem intent | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 7.30 | 12.11 | 234 |
| v2 sem contract | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 5.89 | 10.64 | 217 |
| v2 sem packer | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 6.80 | 11.71 | 228 |

### Comparação com baseline

Baseline: 2026-11-05T02:41:23Z · go1.24.7 · Intel(R) Xeon(R) Processor @ 2.10GHz

Δ = atual − baseline (Δ em % são pontos percentuais). Em Falso Positivo, P50 e Tokens, Δ positivo é pior; nos demais, positivo é melhor.

| Split | Método | Δ Hit@5 | Δ Recall@10 | Δ MRR | Δ Precisão | Δ Falso Positivo | Δ P50 ms | Δ Tokens |
|---|---|---|---|---|---|---|---|---|
| tune | textual (substring) | +0.0 | +0.0 | -0.004 | +0.0 | +0.0 | -0.03 | +0 |
| eval | textual (substring) | +0.0 | +0.0 | +0.025 | +0.0 | +0.0 | -0.02 | +0 |
| holdout | textual (substring) | +0.0 | +0.0 | +0.029 | +0.0 | +0.0 | -0.01 | +0 |
| tune | bm25 (fts5, raw terms) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.16 | +0 |
| eval | bm25 (fts5, raw terms) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.15 | +1 |
| holdout | bm25 (fts5, raw terms) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.10 | +1 |
| tune | bm25 + traducao/normalizacao | +0.0 | +5.0 | +0.031 | +0.6 | +0.0 | +0.18 | +5 |
| eval | bm25 + traducao/normalizacao | +5.0 | +5.0 | +0.054 | -3.2 | +0.0 | +0.18 | +14 |
| holdout | bm25 + traducao/normalizacao | +17.6 | +11.8 | +0.137 | +1.7 | +0.0 | +0.22 | +17 |
| tune | lexical + grafo | +0.0 | +0.0 | +0.025 | +0.0 | +0.0 | -0.86 | +3 |
| eval | lexical + grafo | +10.0 | +10.0 | +0.075 | +1.6 | +0.0 | -0.16 | +12 |
| holdout | lexical + grafo | +17.6 | +17.6 | +0.176 | +3.7 | +0.0 | +0.10 | +16 |
| tune | lexical + grafo + git | +0.0 | +0.0 | +0.025 | +0.0 | +0.0 | -0.41 | +2 |
| eval | lexical + grafo + git | +10.0 | +10.0 | +0.075 | +1.6 | +0.0 | -1.60 | +12 |
| holdout | lexical + grafo + git | +17.6 | +17.6 | +0.176 | +3.7 | +0.0 | -0.60 | +10 |

Métodos sem baseline (não comparados): [v2.1 (intent router) v2 (intent + contract + packer) v2 sem intent v2 sem contract v2 sem packer]

#### Regressões (0)

Nenhuma regressão.

#### Melhorias (14)

- bm25 + traducao/normalizacao · caso E11 (eval): Hit@5 falso → verdadeiro
- bm25 + traducao/normalizacao · caso H03 (holdout): Hit@5 falso → verdadeiro
- bm25 + traducao/normalizacao · caso H08 (holdout): Hit@5 falso → verdadeiro
- bm25 + traducao/normalizacao · caso H11 (holdout): Hit@5 falso → verdadeiro
- lexical + grafo · caso E11 (eval): Hit@5 falso → verdadeiro
- lexical + grafo · caso E14 (eval): Hit@5 falso → verdadeiro
- lexical + grafo · caso H03 (holdout): Hit@5 falso → verdadeiro
- lexical + grafo · caso H08 (holdout): Hit@5 falso → verdadeiro
- lexical + grafo · caso H11 (holdout): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso E11 (eval): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso E14 (eval): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso H03 (holdout): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso H08 (holdout): Hit@5 falso → verdadeiro
- lexical + grafo + git · caso H11 (holdout): Hit@5 falso → verdadeiro
