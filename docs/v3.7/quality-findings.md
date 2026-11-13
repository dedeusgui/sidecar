# V3.7: achados de qualidade da entrega

O que a V3.7 entrega de errado ou deixa de entregar, com a causa rastreada caso a caso. Complementa o [backlog de latência](optimization-backlog.md). Nada aqui foi corrigido: é diagnóstico para decidir o próximo trabalho.

## Como foi levantado

- `sidecar-eval --delivery --delivery-contexts` nos fixtures internos (`bench/cases.json`, splits `tune` e `eval`; holdout não lido) e no ten-repos (50 casos, todos no split `development`), motores V1, V3 e V3.7, sobre o código do PR #30.
- Para cada caso ruim, `sidecar explain --json --engine v3.7` na raiz do fixture: termos, traduções, sinais, candidatos com o motivo de exclusão, plano e contexto final.
- **A coorte E1 não foi aberta caso a caso.** Ela está marcada `frozen-before-tuning`; olhar os erros dela para decidir ajuste a transformaria em conjunto de desenvolvimento. Os números agregados do E1 seguem os de [progress.md](progress.md).
- Os fixtures internos foram escritos pelo autor do código. Um padrão só entra aqui como achado quando a causa está no código, não só no caso.

## Resumo

| # | Achado | Evidência | Onde |
|---|---|---|---|
| 1 | O packer descarta o símbolo certo e mantém vizinhos do mesmo arquivo ou ruído de outros arquivos | 4 casos internos: caminho gold entregue, símbolo gold não (V3 entrega os 4) | `retrieve.Pack`, `novelty`, `diversity` |
| 2 | O gate não mede símbolo gold | V3.7 31 símbolos contra 35 da V3, com "zero regressões" | `bench/fast_gate.py` |
| 3 | Constantes são penalizadas mesmo quando a pergunta pede um limite | T02 ("quantos ... ao mesmo tempo"), a constante é a resposta | `packState.value` |
| 4 | Palavras de estrutura contam como termos do prompt e derrubam a confiança | T02, T08, E06 | `v37FillerTerms`, sinais de `engine` |
| 5 | Cognatos e palavras muito comuns sem tradução | T08, E12, E05, T06, E20 | `internal/lexical` |
| 6 | Modo ambíguo (2 itens) gasta um item com o teste do mesmo símbolo | E20 | `routeV37`, seleção LEAN |
| 7 | Linha `[UNKNOWN] ... no test file` ocupa a primeira linha do contexto e está errada em pelo menos um caso | ten-repos: gold cai da linha 1 para a 2 em 6 casos; em R04-1 o teste existe | `brief/task.go` |
| 8 | Sobrecargas do mesmo método ocupam todos os itens | R09-1 perdido, R09-2 e R09-4 com gold na linha 4 | seleção e render |
| 9 | Caminho do arquivo vai para o fim da linha, depois de assinatura e docstring | todos os itens da V3.7 | `v37EvidenceItem`, `DescribeBare` |
| 10 | Comentários XML de documentação renderizados crus | repositório C# (dapper) | render |

## 1. O packer descarta o símbolo certo

É o achado de maior efeito. Em quatro casos internos a V3.7 entrega o arquivo gold mas não o símbolo gold, e a V3 entrega os dois:

| Caso | Prompt | Símbolo gold | Posição no ranking | O que a V3.7 entregou |
|---|---|---|---|---|
| T02 | quantos caminhões podem ser pesados ao mesmo tempo na balança? | `MaxWeighingSlots` | 2º, confiança 0,75 | `ReleaseTruck`, `ConfirmWeight` |
| T03 | onde um caminhão entra na fila de espera? | `TicketQueueService` (e `Enqueue`) | 3º e 4º, confiança 0,75 | `Length`, a seção do doc, `admit_vehicle` (Python), `advanceVehicle` (Java), `route GET /api/trucks` |
| T06 | levantar a cancela para o veículo que foi admitido | `open_barrier` | 2º (1º no FTS) | `admit_vehicle`, `GateController` |
| E04 | devolver ao estoque a reserva que expirou | `release_stock` | 3º, confiança 0,75 | `StockLedger`, `reserve_stock`, `PlaceOrder` (Go, outro módulo), dois testes |

Em todos, o motivo registrado no candidato é `packer: not worth its tokens: marginal utility per token below threshold`. Em T06 o packer V2 usou 227 de 1.500 tokens, então não foi falta de orçamento.

**Mecanismo (lido no código, `internal/retrieve/packer.go`):**
- `novelty` vale `0,3 + 0,7 × termos ainda não cobertos ÷ termos do item`. Depois que um item cobre os conceitos do prompt, um segundo item com os mesmos conceitos cai para 0,3.
- `diversity` vale `1 ÷ (1 + itens já escolhidos do mesmo arquivo)`: o segundo item do arquivo vale metade, o terceiro um terço.
- Com `PreferImplTieBreak` (só V3.7), a ordem de escolha multiplica por `2,2^termos novos`, então o item de maior cobertura sai primeiro e os irmãos do mesmo arquivo ficam com novidade 0,3 e diversidade 0,5 ou menos. O teste "vale os tokens" usa esse valor e reprova.
- Um item de **outro arquivo** com um único termo ainda não coberto passa: novidade e diversidade cheias. É por isso que T03 e E04 recebem métodos de outras linguagens e módulos no lugar do símbolo gold.

O efeito é o oposto do desejado em perguntas de localização: os melhores candidatos costumam estar no mesmo arquivo e casar os mesmos termos.

**Direções a testar (não testadas):** não aplicar o corte "vale os tokens" a um item com confiança igual à do melhor selecionado; trocar a penalidade por arquivo por uma que só comece depois de N itens; exigir de um item de outro arquivo mais de um termo casado para entrar por novidade. Cada uma precisa de ablação no P1 com símbolo gold medido (achado 2).

## 2. O gate não mede símbolo gold

`bench/fast_gate.py` compara `gold_path_delivered` e `gold_in_first_3_lines`, mas não `gold_symbol_delivered`, que o `sidecar-eval` já calcula. Nos fixtures internos:

| Motor | Caminho gold | Símbolo gold |
|---|---:|---:|
| V1 | 39 | 39 |
| V3 | 35 | 35 |
| V3.7 | 35 | **31** |

O gate reporta V3.7 contra V3 sem regressão. Entregar o arquivo certo com o símbolo errado ainda manda o agente ler o arquivo, mas a resposta direta (uma constante, um método) não está no contexto. No ten-repos e no E1 o gold é só por caminho (coluna de símbolo em 0), então essa perda não é mensurável fora dos fixtures internos hoje.

**Sugestão:** incluir `gold_symbol_delivered` nas métricas do gate quando o corte tem gold de símbolo.

## 3. Constantes penalizadas em perguntas de quantidade

Em `packState.value`, com `PreferImplTieBreak`, um item `constant` que não é match exato é multiplicado por 0,35. O comentário diz que é para constantes não tomarem o lugar de código. Em T02 a pergunta é "quantos ... ao mesmo tempo" e a resposta é a constante `MaxWeighingSlots = 4`, cujo docstring diz literalmente isso. A V3 entrega a constante; a V3.7 entrega dois métodos.

T08 e E06 têm a mesma forma (gold `PAYMENT_GRACE_DAYS` e `MAX_RETRY_ATTEMPTS`), mas lá a perda vem antes, por abstenção (achados 4 e 5).

**Direção a testar:** não aplicar a penalidade quando o prompt pede quantidade ou limite (`quantos`, `quantas`, `how many`, `limite`, `máximo`, `prazo`).

## 4. Palavras de estrutura contam como termos do prompt

Termos extraídos nos casos ruins:

- T02: `quanto, caminhao, podem, pesado, mesmo, tempo, balanca` (7 termos, 3 com conceito). `present_ratio` 0,43 → relevância `ambiguous` → LEAN com 200 tokens e 2 itens.
- T08: `quanto, dia, carencia, existem, pagamento, boleto` (6 termos, 1 conceito). O candidato certo, `PAYMENT_GRACE_DAYS`, é o 1º do ranking com cobertura 1, mas a confiança fica em 0,45, abaixo do gate de bloco (0,5) → abstenção.
- E06: `quanta, tentativa, cobranca, ante, desistir`. `MAX_RETRY_ATTEMPTS` é o 1º do ranking, confiança 0,52, e o gate de relevância abstém ("one generic term matches").

`v37FillerTerms` (`internal/retrieve/retrieve.go`) só tem palavras em inglês. `quanto`, `quanta`, `podem`, `mesmo`, `existem`, `ante` e similares entram no denominador de cobertura e de `present_ratio`.

**Já tentado e revertido** (ver progress.md): ignorar essas palavras no denominador da confiança deu +1 gold (E06) mas subiu o custo por gold para 218 e reprovou o gate. O que não foi tentado: tirar as palavras só do `present_ratio` (que decide `ambiguous`), mantendo o denominador de confiança, e medir junto com o achado 1, já que parte do custo extra vinha de blocos maiores com itens errados.

## 5. Cognatos e palavras comuns sem tradução

Traduções que faltaram nos casos ruins (campo `translations` do explain):

| Caso | Sem tradução | O que o código usa |
|---|---|---|
| T08 | `pagamento`, `dia`, `boleto` | `PAYMENT_GRACE_DAYS` |
| E12 | `marcar`, `concluida` | `mark_delivered` |
| E05 | `estornar`, `pagamento`, `cobrado` | (falha em todos os motores) |
| T06 | `levantar`, `admitido` | `open_barrier`, docstring "Raise the barrier so the admitted truck can pass" |
| E20 | `registro`, `unico` | tabela `customers` com email único |

Vários são cognatos regulares que `CognateCandidates` não gera: `pagamento → payment`, `marcar → mark`, `admitido → admitted`, `unico → unique`. Somar entradas ao léxico a partir destes casos é ajuste em amostra (já registrado como limitação). A alternativa que generaliza é ampliar as regras morfológicas (`-mento → -ment`, particípio `-ado/-ido → -ed`, `-ico → -ic/-ique`, infinitivo `-ar → raiz`) e validar no `messy_pt` e `voice_mode` do ten-repos, onde a V3.7 já está em 10/10 de gold e qualquer aumento de falso positivo aparece nos negativos.

## 6. Modo ambíguo gasta um dos dois itens com o teste

Em E20 a relevância é `ambiguous`, o que dá 200 tokens e 2 itens. Os dois itens foram `findByEmail` e `findByEmailReturnsNullForUnknown`, o teste do mesmo método. O gold (`customers` em `db/migrations.sql`) estava no ranking e foi cortado por "item limit for LEAN". A V3.7 já exibe `called_by:` do teste na linha do método, então o segundo item repete informação.

**Direção a testar:** com limite de 2 itens, não selecionar um teste cujo alvo já foi selecionado; usar a vaga para outro arquivo.

## 7. A linha `[UNKNOWN]` de teste ausente ocupa o topo

Quando o TaskBrief entra, a primeira linha do contexto é:

```
TASK BRIEF
- [UNKNOWN] unknown: no test file in the index references cors; behaviour is not covered by a known test
```

No ten-repos isso empurra o gold da linha 1 (V3) para a linha 2 (V3.7) em R04-1, R04-2, R04-4, R10-1, R10-2 e R10-4. Ainda conta como top-3, mas é a primeira coisa que o agente lê, custa ~25 tokens e é uma afirmação negativa sobre o índice. **A afirmação está errada em R04-1.** O alvo é o middleware `cors` do hono e a linha diz que nenhum arquivo de teste do índice referencia `cors`. O checkout pinado tem `src/middleware/cors/index.test.ts` ao lado do arquivo, com 17 ocorrências de `cors`. O índice não liga esse teste ao símbolo (a causa na extração de referências TypeScript não foi investigada), e o hook transforma essa lacuna do índice em uma afirmação sobre o repositório, na primeira linha do contexto.

Origem: `internal/brief/task.go` (linha do `brief|notest|`).

**Direção a testar:** não emitir a linha (ausência no índice não prova ausência no repositório), ou reescrevê-la como lacuna do índice e movê-la para depois do código. Em paralelo, investigar por que o teste vizinho `index.test.ts` não é ligado.

## 8. Sobrecargas do mesmo método ocupam todos os itens

Repositório dapper (C#), gold `Dapper/SqlMapper.Async.cs`:

- R09-1: os 3 itens de código são três sobrecargas de `QueryAsync` do mesmo arquivo `Dapper.Rainbow/Database.Async.cs`. O gold não é entregue (a V1 entrega, na linha 6, porque manda 8 itens).
- R09-2 e R09-4: quatro `QueryAsync` antes do primeiro item do arquivo gold, que fica na linha 4 (V3: linha 2). São os dois casos que tiram o top-3 de 39 para 37.
- R03-3 (click): dois `raw_terminal` do mesmo arquivo, um por plataforma.

Aqui a penalidade por arquivo do achado 1 não segurou, porque itens com match exato de nome recebem multiplicador 8.

**Direção a testar:** agrupar itens de mesmo nome e mesmo arquivo em uma linha só (nome, número de sobrecargas, faixa de linhas) e liberar as vagas.

## 9. O caminho do arquivo fica no fim da linha

Formato da V3: `- method open_barrier (gate/gate_controller.py:16-18) [INFERRED] — def open_barrier(...)`.
Formato da V3.7: `- method open_barrier — def open_barrier(...) · docstring... | called_by:... [VERIFIED] (source: gate/gate_controller.py:16-18)`.

Na V3.7 o caminho vem depois da assinatura, do docstring e da relação. Em linhas longas (assinaturas genéricas do dapper passam de 200 caracteres) o localizador, que é o que o agente usa para abrir o arquivo, é a última coisa da linha. O efeito no agente **não foi medido**; só o P3 responde. Fica registrado porque é uma diferença de formato entre V3 e V3.7 que nenhum gate offline enxerga.

## 10. Comentários XML renderizados crus

No dapper os docstrings chegam como `<summary> Execute a query asynchronously using Task. </summary> <param name="cnn">The connection...</param> <typeparam name="T">...`. As tags gastam tokens sem informação. Vale para C# (e provavelmente Javadoc com `@param`, não verificado).

**Direção a testar:** no render, manter só o texto do `<summary>` e cortar as demais tags.

## O que está bom e não deve regredir

- Negativos: 0 de 36 negativos difíceis internos e 0 de 10 negativos do ten-repos com saída (V1: 7 de 36).
- `messy_pt` do ten-repos: V3.7 10/10 de gold e 10/10 no top-3 (V1 8 e 7, V3 7 e 7). R04-3, R08-1 e R08-3 são casos que só a V3.7 acerta.
- Rótulos conflitantes: 0 (V3 tem 3 no ten-repos).

## Não investigado

- Negativos do E1 com saída (16/40) e os 7 positivos perdidos do E1: coorte congelada, não aberta por caso.
- Negativos difíceis externos (`bench/negatives/hard_negatives_external.json`): a execução que tentei não produziu linhas (formato de entrada diferente do que usei); os 12/180 de progress.md não foram revisitados.
- Fatos faltantes do E2 no `explore` (`parse_args`, `deserializer|from_trait`).
- E05 (`estornar um pagamento que já foi cobrado`): falha em todos os motores; além da tradução (achado 5), o único candidato é uma seção de doc.

## Ordem sugerida

1. **Achado 2 primeiro:** medir símbolo gold no gate. Sem isso, os próximos ajustes não têm como provar ganho nem regressão.
2. **Achado 1:** maior efeito esperado (4 símbolos internos, e menos ruído de outros arquivos). Ablação por variante no P1.
3. **Achados 7 e 8:** mexem no top-3 do ten-repos, que é onde a V3.7 perde para a V3 (`voice_mode` 9 contra 10).
4. **Achados 3, 4 e 6:** regras pequenas, cada uma com um caso interno que a justifica; validar nos cortes externos antes de aceitar.
5. **Achado 5:** regras morfológicas de cognato, com atenção aos negativos.
6. **Achados 9 e 10:** formato; só o P3 mede o efeito do 9.
