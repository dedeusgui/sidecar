# V3.7 — progresso experimental (checkpoint P0 + P1)

> **Estado:** `SIDECAR_ENGINE=v3.7` implementada como **opt-in** sobre `main@8550eaf`; **V1 continua padrão**. Passa nos portões **P0** (testes, race, lint, registro e freeze). No **P1**, o gate endurecido nesta rodada (ver [checkpoint](#checkpoint-de-hardening-rodada-atual)) **bloqueia** o custo por caminho gold entregue frente à `v3`: as afirmações "GATE OK" das seções abaixo vieram do gate anterior, que ignorava estratos ausentes e não comparava top-3, tokens ou latência. **P3 (piloto agentic pago no `bench/lab/lab.py`) ainda não foi executado.** Não há comprovação de ganho end-to-end com agentes.

Arquitetura e pipeline: [architecture.md](architecture.md). Plano original: [research-plan.md](research-plan.md). Critérios de promoção: [engine-progression.md](../engine-progression.md).

## Checkpoint de hardening (rodada atual)

Medido com `python3 bench/fast_gate.py --race --candidate v3.7 --baseline v3` (P0 + P1 + Studio, `$0`). Nenhuma execução agentic paga (P3). Antes → depois refere-se ao código de `56088bf`.

### O que mudou

1. **Lint:** `staticcheck` em `internal/explore/core/core.go` (`QF1008`, uso de `r.Path` no campo embutido `SymbolHit`) e `internal/extract/extract.go` (`S1011`, `append(..., slice...)`). `golangci-lint run ./...` → `0 issues`.
2. **Gate de entrega endurecido (`bench/fast_gate.py`, `bench/test_fast_gate.py`):** estrato ou motor ausente em um dos lados agora **falha** (antes era pulado em silêncio); top-3 (`gold_in_first_3_lines`) não pode cair; `tokens_per_gold_delivered` pode subir no máximo 10%; p95 precisa ficar em no máximo 1,5× a base + 5 ms. Latência usa a **mediana de 3 execuções** (`--latency-runs`): o p95 de uma única execução variou de ~12 a ~25 ms entre rodadas sem mudança de código.
3. **Dois defeitos em `internal/app/v37.go`, reproduzidos por teste antes da correção** (`internal/app/v37_review_test.go`):
   - uma escolha `contract` que **não é teste** suprimia a orientação de teste da TaskBrief. Agora só suprime quando o contrato aponta um arquivo de teste;
   - um trecho grande que a V3.6 pulou por orçamento nunca era oferecido em forma compactada. Agora a compactação usa o orçamento que sobra e pula trechos já entregues. Nos fixtures internos a saída não muda (nenhum trecho grande é disparado): é correção validada por teste unitário, não ganho medido.
4. **Vocabulário base PT-BR→EN (`internal/lexical/lexicon_pt_en.tsv`):** +42 entradas gerais de comércio, cobrança, logística e prazos (p.ex. `reserva`, `expirar`, `desconto`, `despachar`, `recorrente`, `tentativa`, `unico`). Ver limitação de validação abaixo.

### Medições (fixtures internos; holdout nunca usado)

| Motor | Caminho gold (40) antes → depois | Top-3 antes → depois | Tokens/gold antes → depois |
|---|---:|---:|---:|
| V1 | 38 → 39 | 34 → 34 | 278 → 286 |
| V3 | 29 → 33 | 28 → 31 | 187 → 197 |
| V3.6 | 27 → 32 | 27 → 31 | 236 → 243 |
| **V3.7** | **30 → 33** | **29 → 32** | 218 → 231 |

- **Sentinela de falsos positivos** (`--delivery-negatives bench/negatives/hard_negatives.json`, splits dev + validation, 36 casos nos 2 repositórios internos): V3.7 mantém **0** saídas indevidas, antes e depois da mudança de vocabulário; V1 entrega 7.
- **Gate atual, V3.7 contra V3:** reprova em `tokens_per_gold_delivered` nos 3 estratos positivos (+17%, limite 10%). Não há regressão de gold, top-3, abstenção, determinismo ou latência (p95 mediano 18,0 ms contra 15,0 ms da V3).
- O mesmo gate contra V1 mostra a perda de recall que a V1 não sofre (30 contra 38 antes da mudança; 33 contra 39 depois). V1 continua a referência de recall, não o baseline do gate.

### Diagnóstico das 8 perdas da V3.7 em que a V1 acerta

Rastreado até a etapa com o trace de `sidecar explain` sobre repositórios reconstruídos com o mesmo histórico do harness:

- **7 de 8 são abstenções**: nenhum item chega ao gate de bloco (`MinBlockConfidence` 0,5). A causa é `ownTextConfidence` (V2+): todo termo do prompt entra no denominador, e termos sem tradução no léxico (`reserva`, `expirou`) contam contra o match. Ex.: `release_stock` tem `support=1` sobre `promptTerms=4`, confiança 0,45. A V1 usa cobertura sobre os conceitos do repositório e dá 0,70. O piso de resgate por raridade não dispara porque `stock` não é raro neste fixture (símbolos acima de `max(6, total/50)`).
- **1 de 8 entrega contexto errado** (E20: `db/migrations.sql` no lugar de um item de clientes).
- O vocabulário acima recuperou 3 abstenções (E04, E10, T10). Os outros 4 dependem de traduções que o léxico ainda não tem.

### Trade-off aberto: custo por caso com gold

Relações (`cochange:`, `called_by:`, `member_of:`, `contains:`, `tested_by:`) ocupam ~20% dos tokens entregues pela V3.7 (contra ~5% na V3) e explicam quase toda a diferença de ~29 tokens por caso. Elas não carregam o gold (o gold está no rótulo `source:`), então cortá-las reduziria o custo sem mudar a métrica de entrega. Mas são o sinal estrutural que a V3.6/V3.7 acrescentou, e o benchmark de entrega não mede se o agente as usa. **Não foram removidas nesta rodada**: a política de relações precisa de avaliação end-to-end (P3). Repetições da mesma relação dentro do mesmo bloco são 9% das relações (ganho pequeno, ~3 tokens por caso).

### O que não foi medido aqui (limitações)

- **ten-repos (39/40), E1 (23/30 positivos e 16/40 negativos) e top-3 de `voice_mode`** dependem de checkouts pinados de repositórios externos. Eles não estão nesta máquina, e obtê-los exigiria clonar repositórios fora do escopo permitido deste ambiente. Nenhum número desses cortes foi alterado nem reproduzido.
- **Explore (`NAV-CLK-01`, `NAV-SER-01`)** depende do `corpus-nav` com os mesmos repositórios externos e do CodeGraph. Não foi tocado.
- **O vocabulário novo é ajuste em amostra.** Ele foi escolhido a partir das palavras que falhavam nos fixtures internos, então o ganho de recall ali não prova generalização. A generalização só se mede nos cortes externos, ainda pendentes.
- **Holdout** (`bench/cases.json`, 21 casos) não foi usado. O corte `hard_negatives` do holdout (6 prompts) também permanece congelado.
- **P3 (pago):** não executado.

### Próximos passos

1. Decidir a política de relações (limitar, agrupar ou manter) com um piloto agentic pareado, comparando custo por tarefa concluída.
2. Rodar `ten-repos`, E1 e E2 quando os checkouts pinados estiverem disponíveis, e medir top-3 de `voice_mode` nesse corte.
3. Investigar `NAV-CLK-01` (`parse_args`) e `NAV-SER-01` (`deserializer|from_trait`) no `corpus-nav` com os checkouts.

## P0 — Portão imediato local (`$0`)

Executado sobre a árvore de código atual da V3.7:

| Verificação | Comando | Resultado observado |
|---|---|---|
| Testes Go completos | `go test -count=1 ./...` | PASS (`15.34s`, todos os pacotes ok) |
| Detector de race | `go test -race -count=1 ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/` | PASS (`36.52s`) |
| Testes Python do harness/bench | `python3 -m unittest discover -s bench -p 'test_*.py'` | PASS (`50` testes em `0.129s`) |
| Consistência do registro de benchmarks | `python3 bench/consolidate.py --check` | PASS (`0.08s`) |
| Manifesto congelado V3.5 | `python3 bench/v3.5/freeze.py verify` | PASS (`0.11s`, coortes E1/E2 e baseline plan conferem) |
| Lint estático Go | `golangci-lint run ./...` | `0 issues` nesta rodada (eram 3 `staticcheck`: `QF1008` em `internal/explore/core/core.go:613,621` e `S1011` em `internal/extract/extract.go:204`, corrigidos) |

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
3. ~~**Lint (`3` itens `staticcheck`):**~~ resolvido no checkpoint de hardening: `golangci-lint` em `0 issues`.
4. **P2 e P3 pendentes:** validar contrato real no cliente Antigravity/Claude/Codex (`sidecar doctor` + captura real de `hook.jsonl`) e, só depois de fechar os itens 1–3, rodar o piloto agentic pareado (`bench/lab/lab.py`) comparando `OFF`, `V1`, `V3`, `V3.7` e `CG` com `--max-total-usd` explícito.
