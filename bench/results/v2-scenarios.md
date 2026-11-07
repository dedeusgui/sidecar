# Relatório sidecar-eval

Ambiente: 2026-11-07T01:37:49Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

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

### Split: v2

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 23.1% | 38.5% | 34.6% | 0.276 | 23.5% | 30.8% | 0.0% | 0.20 | 0.36 | 19 |
| bm25 (fts5, raw terms) | 30.8% | 46.2% | 42.3% | 0.349 | 13.1% | 30.8% | 0.0% | 0.76 | 2.19 | 121 |
| bm25 + traducao/normalizacao | 76.9% | 100.0% | 100.0% | 0.859 | 22.3% | 0.0% | 0.0% | 1.44 | 2.78 | 227 |
| lexical + grafo | 76.9% | 100.0% | 100.0% | 0.872 | 35.7% | 0.0% | 0.0% | 6.20 | 8.20 | 210 |
| lexical + grafo + git | 76.9% | 100.0% | 100.0% | 0.872 | 35.7% | 0.0% | 0.0% | 7.06 | 9.00 | 215 |
| v2.1 (intent router) | 76.9% | 100.0% | 100.0% | 0.872 | 35.7% | 0.0% | 0.0% | 3.70 | 7.62 | 209 |
| v2 (intent + contract + packer) | 76.9% | 100.0% | 100.0% | 0.872 | 35.7% | 0.0% | 0.0% | 6.79 | 15.31 | 225 |
| v2 sem intent | 76.9% | 100.0% | 100.0% | 0.872 | 35.7% | 0.0% | 0.0% | 9.86 | 11.84 | 230 |
| v2 sem contract | 76.9% | 100.0% | 100.0% | 0.872 | 35.7% | 0.0% | 0.0% | 5.90 | 10.16 | 209 |
| v2 sem packer | 76.9% | 100.0% | 100.0% | 0.872 | 35.7% | 0.0% | 0.0% | 6.65 | 11.45 | 225 |

### Cenários (checks mecânicos)

- textual (substring): 4/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 0/2 no top 10) · V07 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V09 (gold: 0/2 no top 10) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go) · V13 (gold: 0/1 no top 10)
- bm25 (fts5, raw terms): 5/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V09 (gold: 0/2 no top 10) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go) · V13 (gold: 0/1 no top 10)
- bm25 + traducao/normalizacao: 11/15 passam · falhas: V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (contract: TEST_EXPECTED ausente) · V08 (seen: gold repetido após envio) · V12 (must_not: gen/schema_gen.go)
- lexical + grafo: 12/15 passam · falhas: V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (contract: TEST_EXPECTED ausente) · V08 (seen: gold repetido após envio)
- lexical + grafo + git: 12/15 passam · falhas: V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (contract: TEST_EXPECTED ausente) · V08 (seen: gold repetido após envio)
- v2.1 (intent router): 12/15 passam · falhas: V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (contract: TEST_EXPECTED ausente) · V08 (seen: gold repetido após envio)
- v2 (intent + contract + packer): 15/15 passam · falhas: nenhuma
- v2 sem intent: 15/15 passam · falhas: nenhuma
- v2 sem contract: 13/15 passam · falhas: V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (contract: TEST_EXPECTED ausente)
- v2 sem packer: 14/15 passam · falhas: V08 (seen: gold repetido após envio)
