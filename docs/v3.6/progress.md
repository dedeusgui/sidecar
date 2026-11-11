# V3.6: progresso experimental

A implementação está em desenvolvimento e é opt-in por `SIDECAR_ENGINE=v3.6`. Não há demonstração de ganho em tarefas de agentes nem evidência suficiente para promovê-la. A V3.5 permanece disponível.

O trabalho procura corrigir divergências entre seleção, orçamento e entrega: estimativa pelo conteúdo renderizado, alocação por seção e ordem, snippets de código e rótulos de confiança. Os testes cobrem casos pontuais, mas a bateria de regressão e a revisão da implementação ainda não estão completas.

## Avaliação offline em desenvolvimento

Em 2026-11-10, `go run ./cmd/sidecar-eval --external-cases /tmp/sidecar-real-bench/ten-repos-manifest.json --delivery --delivery-negatives bench/negatives/hard_negatives.json --out /tmp/sidecar-codex-v36-delivery.json` executou hooks sem modelo em dez repositórios externos fixados. O manifesto e os checkouts estão em diretório temporário local e não constituem um artefato reproduzível distribuído. A medida conta presença textual do caminho gold no bloco entregue; não comprova que o agente usou a evidência ou resolveu a tarefa. Latências de uma execução não são comparação controlada.

| Métrica | V3 | V3.5 | V3.6 |
|---|---:|---:|---:|
| Positivos com caminho gold entregue | 35/40 | 7/40 | 36/40 |
| Tokens médios por positivo | 317 | 86 | 355 |
| Rótulos conflitantes, positivos | 5 | 6 | 0 |
| Negativos difíceis com saída não vazia | 14/180 | 8/180 | 15/180 |

Os negativos difíceis indicam regressão de abstenção em relação à V3.5. Não se deve afirmar ausência de regressões. A métrica de símbolos não era informativa nesse corpus (zero nos três motores). É necessário investigar casos sem ajustar regras aos exemplos e repetir avaliação independente, incluindo custo total e comparações por caso.

Na continuação, um teste sintético reproduziu perda da evidência alternativa quando o planner rejeitava uma proposta de snippet por limite de itens; a V3.6 agora preserva a alternativa nesse caso. Um teste cobre também o snippet aceito e o limite de tokens. A comparação externa foi repetida com `go run ./cmd/sidecar-eval --external-cases /tmp/sidecar-real-bench/ten-repos-manifest.json --delivery --delivery-negatives bench/negatives/hard_negatives.json --out /tmp/sidecar-v36-fallback-eval.json`: os totais de caminhos positivos (35/7/36) e negativos difíceis não vazios (14/8/15) permaneceram iguais para V3/V3.5/V3.6. Não se investigou ainda a possibilidade de uma proposta alocada ser posteriormente descartada no packing; alocação não é prova de entrega.

O avaliador omitiu o holdout na execução. Porém, durante a inspeção manual foi exibido acidentalmente o arquivo completo de negativos, inclusive o texto do split holdout. Esse split de negativos não é mais cego para esta sessão; não foi executado nem usado para ajustar a implementação. É necessário um novo holdout independente antes de reivindicar avaliação cega.

## Comparação das cinco engines

Após corrigir o scorecard para não contar prefixos de caminhos ou associar símbolo e arquivo em linhas diferentes, executou-se `go run ./cmd/sidecar-eval --external-cases /tmp/sidecar-real-bench/ten-repos-manifest.json --delivery --delivery-engines v1,v2,v3,v3.5,v3.6 --delivery-negatives bench/negatives/hard_negatives.json --out /tmp/sidecar-v36-all-engines.json`. Os dez checkouts fixados e o manifesto são locais e temporários; veja as limitações acima. Nenhum dos 1.150 resultados repetidos apresentou diferença de bytes. Não houve execução de holdout.

| Métrica | V1 | V2 | V3 | V3.5 | V3.6 |
|---|---:|---:|---:|---:|---:|
| Caminho gold nos positivos | 37/40 | 37/40 | 35/40 | 7/40 | 36/40 |
| Tokens médios nos positivos | 398 | 400 | 313 | 86 | 355 |
| Negativos difíceis com saída | 72/180 | 30/180 | 14/180 | 8/180 | 15/180 |
| Rótulos conflitantes nos positivos | 0 | 9 | 5 | 6 | 0 |

Nos casos positivos, V3.6 recuperou `R04-3` que V1–V3.5 não entregaram; deixou de entregar `R05-3` e `R09-1`, entregues por V1 e V2. O diagnóstico de caminhos não estabelece que o contexto era suficiente ou correto; a medição de símbolos foi zero em todas as engines e não permite conclusão. Em negativos difíceis, cinco das oito saídas V3.5 eram apenas orientação; comparar saída não vazia com abstenção do gate exige inspeção da decisão e do contexto. Latências de uma única execução, com cache e ordem não balanceados, não são prova de superioridade. A V3.6 não atende ao critério de substituição descrito em [engine-progression.md](../engine-progression.md).

Nas fixtures internas não-holdout, `go run ./cmd/sidecar-eval --cases bench/cases.json --delivery --delivery-engines v1,v2,v3,v3.5,v3.6 --out /tmp/sidecar-v36-fixture-delivery.json` entregou caminho gold em 38/40, 35/40, 29/40, 3/40 e 27/40 positivos, respectivamente. As cinco engines ficaram vazias nos dez negativos dessas fixtures. A V3.6 perde dois positivos para V3 e onze para V1 nesse conjunto; as fixtures compartilham autoria com a implementação e não demonstram utilidade externa. Foram omitidos 21 casos holdout.

Para investigar `R05-3` e `R09-1`, `sidecar explain --engine v3.6 --root <checkout> --json` com cache isolado mostrou decisão `LEAN` e priorização, respectivamente, de outros símbolos de JSON e de métodos `QueryAsync` de um wrapper. Os arquivos gold não figuraram entre as primeiras escolhas inspecionadas. É um indício de problema de recuperação/ordenação, não prova de que o packing causou essas perdas; `explain` usa exploração mais ampla que o hook e não reproduz integralmente o scorecard. Não foi adicionada regra específica para esses casos.

Na investigação de um negativo de validação em `chi`, o `explain` V3.5 registrou `ABSTAIN` e ausência de âncora de código, enquanto a V3.6 registrou `LEAN` com relevância ambígua e retornou símbolos `Route`. O gate V3.6 aceita traduções como âncoras; a coincidência lexical é uma hipótese plausível para a divergência, ainda sem ablação que prove causalidade. Não houve ajuste usando esse caso de validação.

Validação local em Linux: `go test -count=1 ./...`, `go test -race -count=1 ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/`, `golangci-lint run ./...` e testes offline Python do workflow CI passaram (um teste Python ignorado). O teste de binário exercita os contratos simulados de Claude e Codex e falha de índice; não é E2E em clientes reais. A próxima fase deve investigar os casos de perda e a divergência entre gate e entrega, testar descarte após alocação, e comparar custos e resultados reais antes de qualquer promoção.
