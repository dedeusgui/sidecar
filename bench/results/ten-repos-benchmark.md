# Relatório sidecar-eval

**Leitura do experimento:** somente o split `development` contém casos (40 positivos e 10 negativos); as tabelas `tune`, `eval` e `holdout` abaixo são impressas automaticamente com denominador zero e não são resultados. Corpus autoral exploratório, não cego, com gold de arquivo; uma execução por caso. A descrição, o setup e as limitações estão em [feedback-and-voice-roi.md](feedback-and-voice-roi.md) e [ten_repos_cases.json](../ten_repos_cases.json). SHA-256 do manifesto versionado: `d1bcc0d8fc1484d3a10150a3bdacdc7245bda27f60c62ddd7c8a11f6b9d9fba9`. Clone e gere o manifesto local com `bench/agentic/setup_10_repos.sh`; rode `go run ./cmd/sidecar-eval --external-cases /tmp/sidecar-real-bench/ten-repos-manifest.json --out /tmp/sidecar-real-bench/ten-repos-results.json --report-md bench/results/ten-repos-benchmark.md`. O relatório gerado por esse comando sobrescreve esta nota.

Ambiente: 2026-11-07T02:25:14Z · go1.27.0 · AMD Ryzen 7 7735HS with Radeon Graphics

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
| textual (substring) | 7.5% | 22.5% | 27.5% | 0.146 | 12.3% | 22.5% | 70.0% | 6.47 | 31.38 | 56 |
| bm25 (fts5, raw terms) | 25.0% | 40.0% | 47.5% | 0.325 | 18.2% | 20.0% | 20.0% | 2.36 | 18.37 | 254 |
| bm25 + traducao/normalizacao | 27.5% | 50.0% | 60.0% | 0.376 | 21.1% | 5.0% | 70.0% | 3.00 | 21.52 | 328 |
| lexical + grafo | 30.0% | 62.5% | 65.0% | 0.422 | 16.8% | 5.0% | 20.0% | 8.21 | 26.23 | 328 |
| lexical + grafo + git | 30.0% | 62.5% | 65.0% | 0.422 | 16.8% | 5.0% | 20.0% | 9.09 | 27.18 | 347 |
| v2.1 (intent router) | 30.0% | 62.5% | 65.0% | 0.422 | 17.2% | 5.0% | 20.0% | 4.96 | 24.45 | 326 |
| v2 (intent + contract + packer) | 30.0% | 62.5% | 65.0% | 0.422 | 17.2% | 5.0% | 20.0% | 15.57 | 40.26 | 353 |
| v2 sem intent | 30.0% | 62.5% | 65.0% | 0.422 | 16.8% | 5.0% | 20.0% | 19.68 | 38.10 | 374 |
| v2 sem contract | 30.0% | 62.5% | 65.0% | 0.422 | 17.2% | 5.0% | 20.0% | 8.05 | 30.14 | 326 |
| v2 sem packer | 30.0% | 62.5% | 65.0% | 0.422 | 17.2% | 5.0% | 20.0% | 12.90 | 34.82 | 353 |

### Cenários (checks mecânicos)

- textual (substring): 14/50 passam · falhas: R01-1 (gold: 0/1 no top 10) · R01-2 (gold: 0/1 no top 10) · R01-3 (gold: 0/1 no top 10) · R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R03-1 (gold: 0/1 no top 10) · R03-3 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R03-5 (no_evidence: contexto injetado) · R04-1 (gold: 0/1 no top 10) · R04-2 (gold: 0/1 no top 10) · R04-3 (gold: 0/1 no top 10) · R04-4 (gold: 0/1 no top 10) · R04-5 (no_evidence: contexto injetado) · R05-1 (gold: 0/1 no top 10) · R05-2 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R06-1 (gold: 0/1 no top 10) · R06-2 (gold: 0/1 no top 10) · R06-3 (gold: 0/1 no top 10) · R06-4 (gold: 0/1 no top 10) · R06-5 (no_evidence: contexto injetado) · R07-3 (gold: 0/1 no top 10) · R07-5 (no_evidence: contexto injetado) · R08-1 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-5 (no_evidence: contexto injetado) · R09-1 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-3 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-4 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- bm25 (fts5, raw terms): 27/50 passam · falhas: R01-2 (gold: 0/1 no top 10) · R01-3 (gold: 0/1 no top 10) · R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R03-3 (gold: 0/1 no top 10) · R04-3 (gold: 0/1 no top 10) · R04-4 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R06-2 (gold: 0/1 no top 10) · R06-3 (gold: 0/1 no top 10) · R06-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R07-4 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-1 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-3 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- bm25 + traducao/normalizacao: 27/50 passam · falhas: R01-4 (gold: 0/1 no top 10) · R01-5 (no_evidence: contexto injetado) · R02-3 (gold: 0/1 no top 10) · R02-5 (no_evidence: contexto injetado) · R03-3 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R04-4 (gold: 0/1 no top 10) · R04-5 (no_evidence: contexto injetado) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R06-5 (no_evidence: contexto injetado) · R07-3 (gold: 0/1 no top 10) · R07-4 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R08-5 (no_evidence: contexto injetado) · R09-1 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- lexical + grafo: 34/50 passam · falhas: R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R02-4 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R07-4 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- lexical + grafo + git: 34/50 passam · falhas: R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R02-4 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R07-4 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- v2.1 (intent router): 34/50 passam · falhas: R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R02-4 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R07-4 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- v2 (intent + contract + packer): 34/50 passam · falhas: R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R02-4 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R07-4 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- v2 sem intent: 34/50 passam · falhas: R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R02-4 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R07-4 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- v2 sem contract: 34/50 passam · falhas: R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R02-4 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R07-4 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
- v2 sem packer: 34/50 passam · falhas: R01-4 (gold: 0/1 no top 10) · R02-3 (gold: 0/1 no top 10) · R02-4 (gold: 0/1 no top 10) · R03-4 (gold: 0/1 no top 10) · R05-3 (gold: 0/1 no top 10) · R05-4 (gold: 0/1 no top 10) · R07-3 (gold: 0/1 no top 10) · R07-4 (gold: 0/1 no top 10) · R08-2 (gold: 0/1 no top 10) · R08-3 (gold: 0/1 no top 10) · R08-4 (gold: 0/1 no top 10) · R09-2 (gold: 0/1 no top 10) · R09-4 (gold: 0/1 no top 10) · R09-5 (no_evidence: contexto injetado) · R10-3 (gold: 0/1 no top 10) · R10-5 (no_evidence: contexto injetado)
