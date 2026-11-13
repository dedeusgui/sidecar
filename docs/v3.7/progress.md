# V3.7 — progresso experimental (checkpoint P0 + P1)

> **Estado:** `SIDECAR_ENGINE=v3.7` implementada como **opt-in** sobre `main@8550eaf`; **V1 continua padrão**. Passou nos portões locais **P0** e **P1** sem regressões frente à `v3` e à `v3.6` nos cortes avaliados. **P3 (piloto agentic pago no `bench/lab/lab.py`) ainda não foi executado** e `golangci-lint` reporta 3 sugestões `staticcheck` descritas abaixo. Não há comprovação de ganho end-to-end com agentes.

Arquitetura e pipeline: [architecture.md](architecture.md). Plano original: [research-plan.md](research-plan.md). Critérios de promoção: [engine-progression.md](../engine-progression.md).

## P0 — Portão imediato local (`$0`)

Executado sobre a árvore de código atual da V3.7:

| Verificação | Comando | Resultado observado |
|---|---|---|
| Testes Go completos | `go test -count=1 ./...` | PASS (`15.34s`, todos os pacotes ok) |
| Detector de race | `go test -race -count=1 ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/` | PASS (`36.52s`) |
| Testes Python do harness/bench | `python3 -m unittest discover -s bench -p 'test_*.py'` | PASS (`50` testes em `0.129s`) |
| Consistência do registro de benchmarks | `python3 bench/consolidate.py --check` | PASS (`0.08s`) |
| Manifesto congelado V3.5 | `python3 bench/v3.5/freeze.py verify` | PASS (`0.11s`, coortes E1/E2 e baseline plan conferem) |
| Lint estático Go | `golangci-lint run ./...` | `3 issues` (`staticcheck`: `QF1008` em `internal/explore/core/core.go:613,621` e `S1011` em `internal/extract/extract.go:204`) |

*(Nota: P0 será revalidado após os commits de documentação finais antes do push.)*

## P1 — Scorecard offline de entrega (`$0`)

Todos os recortes P1 abaixo foram executados localmente sobre o mesmo código Go da V3.7 e compilados em um painel Observatory HTML único (`sidecar studio`, `905.618` bytes). **Omissão:** os 21 casos `holdout` de `bench/cases.json` permanecem omitidos por padrão pelo `sidecar-eval`.

### 1. Fixtures internas (`bench/cases.json`: 40 positivos, 10 negativos; 21 holdout omitidos)

| Métrica | V1 | V2 | V3 | V3.5 | V3.6 | V3.7 |
|---|---:|---:|---:|---:|---:|---:|
| Caminho gold entregue (40 pos) | 38 | 35 | 29 | 3 | 27 | **30** (`eval` 12/20, `tune` 18/20) |
| Símbolo gold entregue (40 pos) | 37 | 33 | 29 | 1 | 26 | **27** |
| Gold nas 3 primeiras linhas | 34 | 34 | 28 | 3 | 27 | **29** |
| Apenas orientação (`orient`) | — | — | — | 6 | — | **0** |
| Rótulos conflitantes (`confl`) | 0 | 1 | 0 | — | 0 | **0** |
| Tokens médios por positivo | 264 | 263 | 135 | 15 | 159 | **164** (não-determinismo: **0**) |
| Saída em negativos (10 neg) | 0/10 | 0/10 | 0/10 | 0/10 | 0/10 | **0/10** |
| Latência V3.7 (single run) | — | — | — | — | — | `p50 23.0ms`, `p95 31.4ms` |

Gate automático `fast_gate`: `GATE v3.7 vs v3: OK`, `GATE v3.7 vs v3.6: OK`.

### 2. Dez repositórios externos + negativos difíceis (`ten-repos` 40 pos + `hard_negatives` 180 neg + 10 neg)

| Métrica | V1 | V2 | V3 | V3.5 | V3.6 | V3.7 |
|---|---:|---:|---:|---:|---:|---:|
| Caminho gold nos positivos (40) | 37 | 37 | 35 | 8 | 36 | **39/40** (`direct` 19/20, `messy_pt` **10/10**, `voice` 10/10) |
| Gold nas 3 primeiras linhas | 32 | 35 | 35 | — | 34 | **34** (`direct` 18, `messy_pt` 10, `voice` **6**) |
| Apenas orientação nos positivos | — | — | — | 18 | — | **0** |
| Rótulos conflitantes nos positivos | 0 | 6 | 3 | 2 | 0 | **0** |
| Tokens médios nos positivos | 403 | 413 | 317 | 94 | 380 | **390** (não-determinismo: **0**) |
| Negativos difíceis não vazios (`180`) | 72 | 30 | 14 | 9 (`orient` 6, `confl` 1) | 15 (`confl` 1) | **12/180** (`confl` **0**, `18` tok méd.) |
| Saída em negativos simples (`10`) | 0/10 | 0/10 | 0/10 | 0/10 | 0/10 | **0/10** |
| Latência V3.7 (single run) | — | — | — | — | — | pos `p50 41.7ms` / `p95 406.8ms`; hard_neg `p50 12.9ms` / `p95 71.7ms` |

Gate automático `fast_gate`: `GATE v3.7 vs v3: OK`, `GATE v3.7 vs v3.6: OK`. Limitação: `ten-repos` e `hard_negatives` são checkouts locais temporários já inspecionados (split dev/validação, não holdout cego), e a queda em `gold_in_first_3_lines` do estrato `voice_mode` (6/10 na V3.7 vs 10/10 na V3) decorre de snippets multi-linha que empurram o item #2/#3 além das 3 primeiras linhas.

### 3. Coorte E1 de prompts curtos de domínio e negativos (`30` pos: `15 EN + 15 PT`; `40` neg)

| Métrica | V1 | V2 | V3 | V3.5 | V3.6 | V3.7 |
|---|---:|---:|---:|---:|---:|---:|
| Caminho gold nos positivos (`30`) | 20 (`12 EN, 8 PT`) | 20 (`12 EN, 8 PT`) | 20 (`12 EN, 8 PT`) | 2 (`2 EN, 0 PT`) | 22 (`12 EN, 10 PT`) | **23/30** (`13 EN, 10 PT`) |
| Gold nas 3 primeiras linhas | 17 | 17 | 17 | — | 21 | **21** |
| Apenas orientação (`orient`) | — | — | — | 20 | — | **0** |
| Tokens médios nos positivos | 381 | 418 | 270 | 64 | 291 | **302** (`confl` **0**) |
| Negativos com saída (`40`) | 28 | 21 | 19 | 13 (`orient` 11) | 18 | **16/40** (`105` tok méd.) |
| Latência V3.7 (single run) | — | — | — | — | — | pos `p50 32.8ms` / `p95 104.3ms`; neg `p50 26.9ms` / `p95 112.7ms` |

Gate automático `fast_gate`: `GATE v3.7 vs v3: OK`, `GATE v3.7 vs v3.6: OK`. Embora `16/40` negativos em E1 seja o melhor número entre V1–V3.7 (exceto V3.5 onde 11/13 eram apenas orientação e apenas 2/30 positivos tinham caminho gold), 40% de vazamento em negativos E1 ainda é alto e segue como alvo aberto.

### 4. Comparativo offline E2 (`e2_offline.py` vs `@colbymchenry/codegraph@1.6.0` em `bench/lab/corpus-nav`: 7 tarefas / 26 fatos)

| Braço | Fatos presentes (26) | Tarefas completas (7) | Volume de saída (chars) | Notas observadas |
|---|---:|---:|---:|---|
| Hook `v2` | 11/26 | 0/7 | 12.781 | — |
| Hook `v3` | 10/26 | 0/7 | 8.487 | — |
| Hook `v3.6` | 8/26 | 0/7 | 7.704 | — |
| Hook `v3.7` | **13/26** | 0/7 | 13.388 | Maior cobertura entre os hooks testados nesta rodada. |
| `sidecar explore` | **23/26** | **5/7** | 84.255 | Completa `NAV-CHI-01` (4/4), `NAV-EXP-01` (3/3), `NAV-HON-01` (3/3), `NAV-MON-01` (4/4), `NAV-SIN-01` (4/4). Faltam: `NAV-CLK-01` (3/4, falta `parse_args`) e `NAV-SER-01` (2/4, faltam `deserializer\|from_trait` e `deserialize_any\|deserialize`). |
| `codegraph explore` (`1.6.0`) | **24/26** | **5/7** | 114.230 | Faltam: `NAV-CLK-01` (3/4, falta `make_context\|main`) e `NAV-SIN-01` (3/4, falta `route!`). |

*(Limitação: `corpus-nav` tem N=7 tarefas escritas internamente para diagnóstico de navegação; presença textual do fato no output não comprova sucesso do agente nem economia de tokens end-to-end.)*

## Gaps abertos e próximos passos obrigatórios

1. **`sidecar explore` vs `codegraph` no E2:** fechar a diferença residual de 1 fato (`23/26` vs `24/26`) nos casos `NAV-CLK-01` (`parse_args`) e `NAV-SER-01` (`deserializer|from_trait`, `deserialize_any|deserialize`).
2. **`voice_mode` nas 3 primeiras linhas e `E1` negativos:** melhorar ordenação compacta nos primeiros itens em prompts de voz (`6/10` → manter `10/10` de top3 como na V3 sem perder os snippets úteis) e reduzir os `16/40` falsos positivos na coorte `E1`.
3. **Lint (`3` itens `staticcheck`):** resolver `QF1008` em `internal/explore/core/core.go:613,621` e `S1011` em `internal/extract/extract.go:204` na próxima iteração de código.
4. **P2 e P3 pendentes:** validar contrato real no cliente Antigravity/Claude/Codex (`sidecar doctor` + captura real de `hook.jsonl`) e, só depois de fechar os itens 1–3, rodar o piloto agentic pareado (`bench/lab/lab.py`) comparando `OFF`, `V1`, `V3`, `V3.7` e `CG` com `--max-total-usd` explícito.
