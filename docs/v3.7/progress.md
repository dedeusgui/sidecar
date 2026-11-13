# V3.7 — progresso experimental (checkpoint P0 + P1)

> **Estado:** `SIDECAR_ENGINE=v3.7` implementada como **opt-in** sobre `main@8550eaf`; **V1 continua padrão**. Passa nos portões **P0** (testes, race, lint, registro e freeze) e no **P1 contra V3 e V3.6** (zero regressões; ver [checkpoint](#checkpoint-de-hardening-rodada-atual)). **Contra V1 ainda reprova** (recall e latência nos cortes internos). **P3 (piloto agentic pago) não foi executado** e o encerramento do marco depende dele e dos cortes externos. As afirmações "GATE OK" das seções abaixo vieram do gate anterior, que ignorava estratos ausentes e não comparava top-3, tokens ou latência. Não há comprovação de ganho end-to-end com agentes.

Arquitetura e pipeline: [architecture.md](architecture.md). Plano original: [research-plan.md](research-plan.md). Critérios de promoção: [engine-progression.md](../engine-progression.md).

> **Passagem para outro agente:** [docs/v3.7/handoff.md](handoff.md) (estado, bloqueio do P3, números, decisões revertidas e comandos).

## Latência do hook: três causas isoladas e corrigidas (sem mudança de saída)

Perfil de CPU do hook quente (`app.Hook` em processo, índice completo) neste repositório e nos checkouts pinados do ten-repos. As três correções abaixo não alteram ranking nem texto entregue: os 150 contextos do ten-repos (V1, V3 e V3.7, positivos e negativos) e os 172 dos fixtures internos são idênticos byte a byte antes e depois (só muda o caminho do binário na dica `ON DEMAND`).

1. **`store.TestFilesReferencing` (caminho compartilhado, inclusive V1).** A consulta juntava cada referência ao arquivo e ao símbolo e só então aplicava `DISTINCT`. Para um nome muito referenciado em testes (`get`, `it`, `assert_equal` no sinatra, ~1.200 referências) custava ~11,7 ms por chamada no SQLite em C e é chamada por semente e por definição. Reescrita com `f.id IN (SELECT r.file_id ...)`: ~0,31 ms, mesmo resultado, sem mudança de schema. Era ~75% do tempo do hook no sinatra.
2. **`lexical.normalizeRomanceOrthography`.** Construía um `strings.NewReplacer` a cada palavra, para cada candidato. Em prompts longos (voz, ~1.100 caracteres) era ~30% do hook. Agora é construído uma vez.
3. **`retrieve.ContractEvidence`.** Lia o mesmo arquivo de teste uma vez por definição e varria todo Markdown linha a linha para cada definição. Agora lê cada arquivo uma vez por chamada e pula arquivo ou definição cujo nome não aparece no texto.

| Medida | Motor | Antes | Depois |
|---|---|---:|---:|
| ten-repos, 40 positivos, p95 ms (3 execuções antes, 2 depois) | V1 | 119 a 131 | 25 a 38 |
| | V3 | 229 a 273 | 51 a 52 |
| | V3.7 | 221 a 240 | 60 a 70 |
| ten-repos, p50 ms | V1 / V3 / V3.7 | 11 / 23 / 33 | 11 a 13 / 22 a 30 / 29 a 33 |
| sinatra, 4 prompts, média ms | V1 / V3.7 | 81 / 167 | 22 / 32 |
| hono, prompt de voz, média ms | V1 / V3.7 | 22 / 109 | 22 / 67 |
| este repositório, 4 prompts, estágio do motor V3.7, ms | V3.7 | 43 | 32 (só o item 3) |

Gold, top-3 e tokens por gold do ten-repos não mudaram (V3.7 39 / 37 / 320). As medidas são de uma máquina de 4 CPUs com carga variável; a segunda execução "depois" rodou junto com outros processos. A recompilação de SQLite (`sqlite3Reprepare`) apareceu com 1 a 2% nestes perfis, não com os 26 a 30% registrados abaixo: não foi a causa da cauda.

**O que continua aberto em latência (medido, não corrigido):** a V3.7 ainda fica em ~2,5× a V1 no p50 do ten-repos. O que sobra no perfil: `ContractEvidence` roda antes do gate de relevância (prompts que terminam em abstenção pagam por ele) e ainda faz uma varredura completa de `symbols.doc` por definição (`DocstringOwners`, até 12 por prompt); `lexical.Translations` é recalculada por candidato × termo sem memoização (~20% em prompt de voz); `index.Sync` carrega `FileRecords` duas vezes por prompt; a V3.7 empacota três vezes (packer V2, `engine.pack`, `PackWith`) sobre uma janela de 60 itens. Qualquer uma dessas mexe em ordem de estágios ou em resultado e precisa de P1 próprio.

## Checkpoint de hardening (rodada atual)

Medido com `python3 bench/fast_gate.py --race --engines v1,v2,v3,v3.5,v3.6,v3.7 --candidate v3.7 --baseline v3` (P0 + P1 + Studio, `$0`). "Antes" é o código de `56088bf`. O holdout nunca foi lido. Nenhuma execução agentic paga (P3).

### Resultado (fixtures internos, 40 positivos; negativos difíceis dev + validation, 36 casos)

| Motor | Caminho gold antes → depois | Top-3 antes → depois | Tokens por gold antes → depois |
|---|---:|---:|---:|
| V1 (padrão) | 38 → 39 | 34 → 34 | 278 → 288 |
| V3 | 29 → 35 | 28 → 33 | 187 → 192 |
| V3.6 | 27 → 33 | 27 → 32 | 236 → 243 |
| **V3.7** | **30 → 35** | **29 → 33** | **218 → 195** |

- **Gate V3.7 contra V3 e contra V3.6: zero regressões.** Cobre caminho gold, top-3, orientação, conflitos de rótulo, determinismo, tokens por gold (+10%), latência (p95 mediano, 1,5× + 5 ms) e falsos positivos nos negativos difíceis. Resultado: V3.7 deixa 0 de 36 negativos difíceis com saída; a V1 deixa 7.
- **Gate V3.7 contra V1: ainda reprova.** Caminho gold 35 contra 39, top-3 33 contra 34 nos positivos, e p95 de ~21 ms contra ~10 ms da V1 (estrato `tune`). A V1 segue padrão. A V3.7 não supera a V1 em recall nem em latência nestes cortes.

### O que mudou

1. **Lint:** as 3 issues de `staticcheck` foram corrigidas (`QF1008` em `internal/explore/core/core.go` e `S1011` em `internal/extract/extract.go`). `golangci-lint run ./...` → 0 issues.
2. **Gate endurecido (`bench/fast_gate.py`):** estrato ou motor ausente falha (antes era pulado em silêncio). Top-3 não pode cair. `tokens_per_gold_delivered` pode subir no máximo 10% sobre a base. p95 precisa ficar em até 1,5× a base + 5 ms, com a mediana de `--latency-runs` (padrão 3) execuções, porque o p95 de uma só execução variou de ~12 a ~25 ms sem mudança de código. Os negativos difíceis dev + validation entram no gate, para que um vazamento reprove sem depender de uma execução manual. O holdout continua fora.
3. **Dois defeitos em `internal/app/v37.go`, reproduzidos por teste antes da correção** (`internal/app/v37_review_test.go`):
   - uma escolha `contract` que não era teste suprimia a orientação de teste da TaskBrief; agora só suprime quando o contrato aponta um arquivo de teste;
   - um trecho grande que a V3.6 pulou por orçamento nunca era oferecido compactado; agora a compactação usa o orçamento que sobra e pula trechos já entregues. Nos fixtures a saída não muda (nenhum trecho grande é disparado): é correção validada por teste, não ganho medido.
4. **Relações renderizadas pela V3.7** (`v37ActionableRelations`): no máximo 1 por item, na ordem `tested_by` > `called_by` > `calls`. Contenção (`member_of`, `contains`) e co-change (`cochange`) deixam de ser exibidos. O gold não depende disso (está no rótulo `source:`), e a ordem por caminho de teste não mudou nenhum gold. Motivo do corte: relações eram ~20% dos tokens entregues pela V3.7, e `cochange` é a anotação mais frequente, sinal histórico que aponta para o mesmo teste que `tested_by` mostra estaticamente. **Trade-off:** o valor dessas relações para o agente não foi medido. Só o piloto P3 responde isso.
5. **Léxico PT-BR→EN (`internal/lexical/lexicon_pt_en.tsv`):** +47 entradas gerais desde `56088bf` (42 de comércio, cobrança, logística e prazos, na `b7cc3f8`, e 5 flexões: `cupons`, `unidade`/`unidades`, `quantidade`/`quantidades`, nesta rodada). Essas palavras vieram das falhas dos fixtures internos, então o ganho ali é ajuste em amostra.

### Testado e revertido (medido, não entrou)

- **Ignorar palavras de estrutura em PT no denominador da confiança** (`quanto`, `existem`, `ante`, só na V3.7): +1 gold (E06), mas o custo por gold subiu para 218 e reprovou o gate. Reverter também evitou crescimento do bloco em T02 (167 → 293 tokens, sem ganho). Revertido.
- **Deduplicar relações repetidas dentro do mesmo bloco:** são 9% das relações e ~1% dos tokens. Não compensa a complexidade. Não feito.
- **Ordem de relações com `cochange`:** mantido fora (ver item 4). Antes de sair, o estrato `tune` ficou em 221 tokens por gold contra 217 de limite; sem `cochange` ficou em 206.

### Recall restante (V1 acerta, V3.7 não)

- **E06** `quantas tentativas de cobrança antes de desistir`: abstenção. Só um termo forte (`cobrança`) casa, abaixo do gate de bloco.
- **E12** `marcar a entrega como concluída`: abstenção. `concluída` não tem tradução presente no repositório.
- **T08** `quantos dias de carência existem para pagamento do boleto?`: nenhuma evidência passa pelos gates V2 (`best_support 0`); é limitação de recuperação em nível de símbolo.
- **E20** `registro de clientes com email único`: entrega contexto errado (modo LEAN, item de ordenação).
- **E05** falha em todas as engines (V1 inclusive).

### Latência do hook quente (perfil de CPU, `$0`)

Benchmark local do caminho do hook com índice aquecido (uma consulta, 4 CPUs): V1 ~4,8 ms por execução, V3.7 ~8,9 ms. A diferença de p95 contra a V1 nos cortes internos vem deste mesmo trecho.

- **V3.7:** cerca de 37% das amostras de CPU estão em `explore/service.QuickExpand` (`explore/core.Build`, `expandCallChain`). É a expansão automática que a V3.7 roda no hook e a V1 não roda. Custo do desenho, não defeito.
- **Ambos os motores:** cerca de 26% (V3.7) e 30% (V1) das amostras estão na recompilação de comandos SQLite (`sqlite3Reprepare` chamado por `sqlite3_step`). Afeta o caminho compartilhado de armazenamento, inclusive o motor padrão.
- **Descartado:** mudança de schema durante a consulta. `PRAGMA schema_version` ficou em 41 em todas as consultas de um hook quente.
- **Não identificado:** a causa da recompilação. Hipótese descartada em microbenchmark isolado: consultas FTS5 cacheadas custam ~18,7 µs contra ~20,7 µs sem cache, e consultas em tabela comum cacheadas custam ~2,4 µs sem sinal de recompilação. A recompilação do perfil real não se reproduz fora do hook; a causa continua aberta. Corrigir isso muda o caminho de desempenho do motor padrão e exige PR próprio, com benchmark antes e depois.

### Cortes externos (medidos nesta rodada, com checkouts pinados)

Os repositórios são públicos e foram obtidos por clone raso no commit fixado de cada manifesto (`bench/ten_repos_cases.json`, `bench/v3.5/e1_cases.json`, `bench/lab/corpus-nav`). Rodada completa: `fast_gate.py --race --external-manifest ... --checkout ... --codegraph ... --candidate v3.7 --baseline v3`.

| Corte | Motor | Gold | Top-3 | Tokens/gold | p95 ms |
|---|---|---:|---:|---:|---:|
| ten-repos (40 pos.) | V1 | 37 | 32 | 436 | 102 |
| | V3 | 35 | 35 | 367 | 180 |
| | V3.6 | 36 | 34 | 422 | 179 |
| | **V3.7** | **39** | **37** | **320** | 205 |
| E1 (30 pos. / 40 neg.) | V1 | 20 | 17 | 572 | 47 |
| | V3 | 20 | 17 | 407 | 66 |
| | V3.6 | 22 | 21 | 397 | 96 |
| | **V3.7** | **23** | **22** | **350** | 92 |

- **E1, negativos com saída:** V3.7 16/40 (V1 28, V3 19, V3.6 18). Os números de gold 23/30 e 16/40 batem com os que este documento já registrava.
- **Gate V3.7 contra V3.6 nos dois cortes:** zero regressões. Contra V3 no E1: zero regressões. Contra V3 no ten-repos: duas, `messy_pt` p95 (41 contra limite de 38 ms, em 10 casos) e `voice_mode` top-3 (9 contra 10, uma posição).
- **Contra V1:** gold do ten-repos e do E1 já é igual ou maior. O que reprova é latência (p95 ~2× a V1 no ten-repos e no E1) e o `voice_mode` do ten-repos (top-3 e custo).
- **E2 (navegação, corpus-nav, 7 tarefas / 26 fatos, CodeGraph 1.6.0 instalado):** V3.7 hook cobre 14 fatos (V1 11, V3 10). `sidecar explore` cobre 23 e CodeGraph 24. Os faltantes são os mesmos já listados: `parse_args` (NAV-CLK-01) e `deserializer|from_trait` (NAV-SER-01). Nenhuma tarefa cobre todos os fatos em nenhum braço.

**Dois achados de ablação que mudaram o motor (ver commit `9e95f03`):**

- Compactar trechos que a V3.6 pulou por orçamento, ao lado dos trechos inteiros da V3.6, gastava o orçamento do item gold: `voice_mode` do ten-repos caiu de 10 para 7. Revertido. A correção estava coberta por teste unitário, mas o benchmark externo mostrou o efeito oposto.
- A expansão automática (`QuickExpand`) da V3.7 no hook piorava o `voice_mode` (top-3 de 9 com a expansão desligada, contra 6 com ela ligada) e o custo (320 contra 394 tokens por gold no ten-repos), sem ganho líquido de gold. Passou a ser opt-in (`SIDECAR_FEATURES=quickexpand`). V3.5 e V3.6 continuam como antes.

### Limitações que continuam

- **P3 (piloto agentic pago): não executado.** Preparei o harness: o `--agent scripted` validou a infraestrutura com custo zero, e instalei o `bwrap` que o sandbox exige. Quando tentei o piloto real, o preflight de autenticação do sandbox reportou `loggedIn: false` (`authMethod: none`), mesmo com o token copiado para a configuração isolada. Nenhuma execução paga foi iniciada e nenhum gasto foi registrado. Para rodar, é preciso uma credencial que o harness aceite (por exemplo, uma chave de API fornecida como segredo do ambiente) e um teto de gasto explícito.
- **Latência da V3.7 contra a V1:** o p95 é cerca de 2× o da V1 nos cortes externos. Não atribuo à expansão automática (o ablation com ela desligada não reduziu o p95 de forma clara). A causa não foi isolada.
- **`voice_mode` top-3:** 9 contra 10 da V3, e 9 contra 9 da V1 nesse corte. Não há correção validada ainda; a ordenação "resumo antes do trecho" foi tentada e revertida (ganho de voz compensado por perda em `direct`).
- **Vocabulário:** o ganho nos fixtures internos é de ajuste em amostra. Os cortes externos são a validação fora dessa amostra.
- **Negativos E1 com saída (16/40):** ainda é o maior vazamento da V3.7, mas é menor que o da V1 (28).

### Próximos passos

1. Fornecer uma credencial aceita pelo harness e um teto de gasto para rodar o P3.
2. Investigar a latência da V3.7 contra a V1 nos cortes externos (perfil por estágio em repositórios grandes).
3. Investigar `voice_mode` top-3 contra V3 (ordenação e seleção de itens em prompts de voz).
4. Investigar os fatos faltantes do E2 (`parse_args`, `deserializer|from_trait`) no `explore`.

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
