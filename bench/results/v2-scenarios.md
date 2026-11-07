# Relatório sidecar-eval

Ambiente: 2026-11-07T01:19:04Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

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
| textual (substring) | 15.4% | 38.5% | 34.6% | 0.237 | 23.5% | 30.8% | 0.0% | 0.12 | 0.16 | 19 |
| bm25 (fts5, raw terms) | 30.8% | 46.2% | 42.3% | 0.349 | 13.1% | 30.8% | 0.0% | 0.84 | 3.00 | 117 |
| bm25 + traducao/normalizacao | 46.2% | 61.5% | 57.7% | 0.526 | 15.0% | 7.7% | 0.0% | 1.08 | 3.17 | 149 |
| lexical + grafo | 46.2% | 61.5% | 53.8% | 0.538 | 27.4% | 7.7% | 0.0% | 3.85 | 9.70 | 130 |
| lexical + grafo + git | 46.2% | 61.5% | 53.8% | 0.538 | 27.4% | 7.7% | 0.0% | 4.38 | 10.37 | 135 |
| v2.1 (intent router) | 46.2% | 61.5% | 53.8% | 0.538 | 27.4% | 7.7% | 0.0% | 2.44 | 8.47 | 130 |
| v2 (intent + contract + packer) | 46.2% | 53.8% | 50.0% | 0.500 | 23.3% | 7.7% | 0.0% | 3.17 | 9.00 | 134 |
| v2 sem intent | 46.2% | 53.8% | 50.0% | 0.500 | 23.3% | 7.7% | 0.0% | 4.84 | 11.38 | 139 |
| v2 sem contract | 46.2% | 53.8% | 50.0% | 0.500 | 23.3% | 7.7% | 0.0% | 2.42 | 8.39 | 125 |
| v2 sem packer | 46.2% | 61.5% | 53.8% | 0.538 | 27.4% | 7.7% | 0.0% | 3.20 | 8.82 | 138 |

### Cenários (checks mecânicos)

- textual (substring): 4/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 0/2 no top 10) · V07 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V09 (gold: 0/2 no top 10) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go) · V13 (gold: 0/1 no top 10)
- bm25 (fts5, raw terms): 5/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V09 (gold: 0/2 no top 10) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go) · V13 (gold: 0/1 no top 10)
- bm25 + traducao/normalizacao: 7/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go)
- lexical + grafo: 6/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 1/2 no top 10) · V07 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go)
- lexical + grafo + git: 6/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 1/2 no top 10) · V07 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go)
- v2.1 (intent router): 6/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 1/2 no top 10) · V07 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go)
- v2 (intent + contract + packer): 7/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 0/2 no top 10) · V07 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go)
- v2 sem intent: 7/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 0/2 no top 10) · V07 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go)
- v2 sem contract: 6/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V03 (conflict: nota CONFLICT_CANDIDATE ausente) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 0/2 no top 10) · V07 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go)
- v2 sem packer: 7/15 passam · falhas: V01 (gold: 0/1 no top 10) · V02 (gold: 0/1 no top 10) · V04 (gold: 0/1 no top 10; contract: TEST_EXPECTED ausente) · V05 (gold: 1/2 no top 10) · V07 (gold: 1/2 no top 10) · V08 (gold: 0/1 no top 10; seen: 1ª passada sem todo o gold) · V10 (gold: 0/1 no top 10) · V12 (must_not: gen/schema_gen.go)
