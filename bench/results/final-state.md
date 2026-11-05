
### Split: tune

| Método | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 15.0% | 15.0% | 0.117 | 13.2% | 40.0% | 0.0% | 0.36 | 0.41 | 12 |
| bm25 (fts5, raw terms) | 20.0% | 20.0% | 0.085 | 7.0% | 35.0% | 0.0% | 0.60 | 1.35 | 70 |
| bm25 + traducao/normalizacao | 70.0% | 82.5% | 0.557 | 13.0% | 0.0% | 0.0% | 1.87 | 3.05 | 248 |
| lexical + grafo | 80.0% | 80.0% | 0.604 | 12.6% | 0.0% | 0.0% | 9.16 | 11.90 | 304 |
| lexical + grafo + git | 80.0% | 80.0% | 0.604 | 12.6% | 0.0% | 0.0% | 8.17 | 9.53 | 354 |

### Split: eval

| Método | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 10.0% | 10.0% | 0.075 | 25.0% | 80.0% | 0.0% | 0.32 | 0.43 | 6 |
| bm25 (fts5, raw terms) | 20.0% | 20.0% | 0.163 | 37.5% | 70.0% | 0.0% | 0.50 | 0.86 | 26 |
| bm25 + traducao/normalizacao | 85.0% | 85.0% | 0.572 | 23.4% | 10.0% | 0.0% | 0.92 | 1.41 | 175 |
| lexical + grafo | 75.0% | 75.0% | 0.567 | 23.1% | 10.0% | 0.0% | 4.12 | 7.45 | 185 |
| lexical + grafo + git | 75.0% | 75.0% | 0.567 | 23.1% | 10.0% | 0.0% | 4.76 | 9.27 | 214 |

### Split: holdout

| Método | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 11.8% | 11.8% | 0.088 | 25.0% | 76.5% | 0.0% | 0.21 | 0.27 | 7 |
| bm25 (fts5, raw terms) | 17.6% | 17.6% | 0.176 | 29.2% | 64.7% | 0.0% | 0.49 | 0.87 | 29 |
| bm25 + traducao/normalizacao | 70.6% | 76.5% | 0.487 | 20.8% | 17.6% | 0.0% | 0.93 | 1.46 | 165 |
| lexical + grafo | 64.7% | 64.7% | 0.490 | 20.2% | 17.6% | 0.0% | 3.97 | 7.63 | 185 |
| lexical + grafo + git | 64.7% | 64.7% | 0.490 | 20.2% | 17.6% | 0.0% | 7.36 | 15.23 | 215 |
