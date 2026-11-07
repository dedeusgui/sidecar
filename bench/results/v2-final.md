# Relatório sidecar-eval

Ambiente: 2026-11-07T01:19:04Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

### Split: tune

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 10.0% | 15.0% | 15.0% | 0.117 | 13.2% | 40.0% | 0.0% | 0.15 | 0.29 | 12 |
| bm25 (fts5, raw terms) | 5.0% | 20.0% | 20.0% | 0.085 | 7.0% | 35.0% | 0.0% | 0.73 | 1.16 | 70 |
| bm25 + traducao/normalizacao | 45.0% | 70.0% | 82.5% | 0.557 | 13.0% | 0.0% | 0.0% | 1.36 | 2.28 | 248 |
| lexical + grafo | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 6.37 | 7.84 | 304 |
| lexical + grafo + git | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 7.60 | 8.89 | 354 |
| v2.1 (intent router) | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 4.61 | 9.91 | 345 |
| v2 (intent + contract + packer) | 55.0% | 80.0% | 80.0% | 0.629 | 12.7% | 0.0% | 0.0% | 5.79 | 10.45 | 365 |
| v2 sem intent | 55.0% | 80.0% | 80.0% | 0.629 | 12.7% | 0.0% | 0.0% | 8.72 | 10.44 | 373 |
| v2 sem contract | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 4.79 | 9.00 | 345 |
| v2 sem packer | 55.0% | 80.0% | 80.0% | 0.629 | 12.6% | 0.0% | 0.0% | 5.62 | 10.21 | 369 |

### Split: eval

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 10.0% | 10.0% | 10.0% | 0.100 | 25.0% | 80.0% | 0.0% | 0.14 | 0.33 | 6 |
| bm25 (fts5, raw terms) | 15.0% | 20.0% | 20.0% | 0.163 | 37.5% | 70.0% | 0.0% | 0.58 | 0.91 | 26 |
| bm25 + traducao/normalizacao | 50.0% | 90.0% | 90.0% | 0.626 | 20.2% | 5.0% | 0.0% | 1.14 | 1.36 | 186 |
| lexical + grafo | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 4.17 | 7.09 | 194 |
| lexical + grafo + git | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 4.52 | 8.14 | 223 |
| v2.1 (intent router) | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 4.41 | 8.07 | 220 |
| v2 (intent + contract + packer) | 50.0% | 85.0% | 85.0% | 0.642 | 24.9% | 5.0% | 0.0% | 5.45 | 9.18 | 224 |
| v2 sem intent | 50.0% | 85.0% | 85.0% | 0.642 | 24.9% | 5.0% | 0.0% | 5.48 | 9.00 | 227 |
| v2 sem contract | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 4.46 | 8.74 | 220 |
| v2 sem packer | 50.0% | 85.0% | 85.0% | 0.642 | 24.6% | 5.0% | 0.0% | 5.19 | 8.81 | 228 |

### Split: holdout

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 11.8% | 11.8% | 11.8% | 0.118 | 25.0% | 76.5% | 0.0% | 0.12 | 0.13 | 7 |
| bm25 (fts5, raw terms) | 17.6% | 17.6% | 17.6% | 0.176 | 29.2% | 64.7% | 0.0% | 0.61 | 0.89 | 29 |
| bm25 + traducao/normalizacao | 47.1% | 88.2% | 88.2% | 0.625 | 22.5% | 5.9% | 0.0% | 1.05 | 1.61 | 178 |
| lexical + grafo | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 3.97 | 7.19 | 198 |
| lexical + grafo + git | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 4.37 | 8.34 | 222 |
| v2.1 (intent router) | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 3.61 | 8.14 | 214 |
| v2 (intent + contract + packer) | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 4.34 | 9.11 | 225 |
| v2 sem intent | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 5.29 | 9.20 | 231 |
| v2 sem contract | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 3.43 | 8.03 | 214 |
| v2 sem packer | 52.9% | 82.4% | 82.4% | 0.667 | 23.9% | 5.9% | 0.0% | 4.21 | 8.88 | 225 |

### Comparação com baseline

Baseline: 2026-11-05T02:41:23Z · go1.24.7 · Intel(R) Xeon(R) Processor @ 2.10GHz

Δ = atual − baseline (Δ em % são pontos percentuais). Em Falso Positivo, P50 e Tokens, Δ positivo é pior; nos demais, positivo é melhor.

| Split | Método | Δ Hit@5 | Δ Recall@10 | Δ MRR | Δ Precisão | Δ Falso Positivo | Δ P50 ms | Δ Tokens |
|---|---|---|---|---|---|---|---|---|
| tune | textual (substring) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | -0.06 | +0 |
| eval | textual (substring) | +0.0 | +0.0 | +0.025 | +0.0 | +0.0 | -0.07 | +0 |
| holdout | textual (substring) | +0.0 | +0.0 | +0.029 | +0.0 | +0.0 | -0.08 | +0 |
| tune | bm25 (fts5, raw terms) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.17 | +0 |
| eval | bm25 (fts5, raw terms) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.17 | +0 |
| holdout | bm25 (fts5, raw terms) | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.08 | +0 |
| tune | bm25 + traducao/normalizacao | +0.0 | +0.0 | +0.000 | +0.0 | +0.0 | +0.08 | +0 |
| eval | bm25 + traducao/normalizacao | +5.0 | +5.0 | +0.054 | -3.2 | +0.0 | +0.19 | +11 |
| holdout | bm25 + traducao/normalizacao | +17.6 | +11.8 | +0.137 | +1.7 | +0.0 | +0.17 | +12 |
| tune | lexical + grafo | +0.0 | +0.0 | +0.025 | +0.0 | +0.0 | -1.17 | +0 |
| eval | lexical + grafo | +10.0 | +10.0 | +0.075 | +1.6 | +0.0 | -0.20 | +9 |
| holdout | lexical + grafo | +17.6 | +17.6 | +0.176 | +3.7 | +0.0 | +0.08 | +13 |
| tune | lexical + grafo + git | +0.0 | +0.0 | +0.025 | +0.0 | +0.0 | -0.49 | +0 |
| eval | lexical + grafo + git | +10.0 | +10.0 | +0.075 | +1.6 | +0.0 | -1.87 | +9 |
| holdout | lexical + grafo + git | +17.6 | +17.6 | +0.176 | +3.7 | +0.0 | -0.52 | +8 |

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
