# Scorecard do laboratório

## Evidência: agentic

| braço | exec. | comparáveis | pass | taxa | ocultos ok | escopo (não-teste) | invariantes | custo/correta | p50 s/correta | tool calls | redundantes | injetado |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| OFF | 9 | 9 | 9 | 1.0 | 9 | 0 (0) | 0 | 0.0066 | 63.58 | 10.22 | None | 0 |
| V2 | 9 | 9 | 9 | 1.0 | 9 | 0 (0) | 0 | 0.0071 | 64.7 | 11.89 | 0.62 | 1455.7 |
| V35 | 9 | 9 | 7 | 0.778 | 9 | 2 (0) | 1 | 0.0091 | 43.95 | 12 | 0.5 | 627.6 |

### Contadores e tempos observados

| métrica | braço | total nas execuções comparáveis |
|---|---|---:|
| entrada sem cache | OFF | 162 |
| entrada sem cache | V2 | 168 |
| entrada sem cache | V35 | 172 |
| cache lido | OFF | 1688874 |
| cache lido | V2 | 1862454 |
| cache lido | V35 | 1864830 |
| cache criado | OFF | 106697 |
| cache criado | V2 | 103767 |
| cache criado | V35 | 104050 |
| saída | OFF | 43112 |
| saída | V2 | 49263 |
| saída | V35 | 48883 |
| indexação ms | V2 | 13798.252 |
| indexação ms | V35 | 13513.153 |
| hook ms | V2 | 976.287 |
| hook ms | V35 | 1347.065 |
| sync ms | V2 | 56.75 |
| sync ms | V35 | 72.825 |
| retrieval ms | V2 | 555.358 |

### Suficiência após cada entrega do Sidecar (próximo ato por thread; NÃO é correção)

| braço | entregas | moved_on | read_returned | read_named | read_not_returned | search | explore_again | errored | releituras redundantes | leitura após edição | sem efeito |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| OFF | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 |
| V2 | 9 | 4 | 3 | 0 | 0 | 2 | 0 | 0 | 6 | 1 | 2 |
| V35 | 8 | 3 | 1 | 0 | 1 | 3 | 0 | 0 | 4 | 1 | 1 |

- V2 vs OFF: 9 pares, Δpass 0 IC95 (0, 0), Δcusto 0.0005 → **inconclusivo**
- V35 vs OFF: 9 pares, Δpass -0.222 IC95 (-0.5555555555555556, 0), Δcusto 0.0005 → **inconclusivo**

## Δ% por métrica (agentic): braço contra referência, pareado por tarefa e repetição

Negativo em tool calls/tokens/custo/tempo/redundantes = menos. `inconclusivo` = IC95 cruza zero ou menos de 10 tarefas.

### V2 vs OFF (9 pares, 9 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 10.2222 | 11.8889 | 16.3 | [-8.6, 41.7] | inconclusivo |
| tokens (entrada+saída) | 204316.1111 | 223961.3333 | 9.6 | [-18.0, 47.1] | inconclusivo |
| custo US$ | 0.0066 | 0.0071 | 7.1 | [-16.6, 33.8] | inconclusivo |
| tempo s | 52.1478 | 67.8633 | 30.1 | [0.2, 62.5] | inconclusivo |

### V35 vs OFF (9 pares, 9 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 1.0 | 0.7778 | -22.2 | [-55.6, 0.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 10.2222 | 12 | 17.4 | [-11.0, 55.9] | inconclusivo |
| tokens (entrada+saída) | 204316.1111 | 224215 | 9.7 | [-9.7, 45.6] | inconclusivo |
| custo US$ | 0.0066 | 0.0071 | 6.9 | [-15.5, 41.4] | inconclusivo |
| tempo s | 52.1478 | 66.18 | 26.9 | [-15.3, 80.9] | inconclusivo |

### V35 vs V2 (9 pares, 9 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 1.0 | 0.7778 | -22.2 | [-55.6, 0.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 11.8889 | 12 | 0.9 | [-17.9, 25.3] | inconclusivo |
| tokens (entrada+saída) | 223961.3333 | 224215 | 0.1 | [-25.4, 28.1] | inconclusivo |
| custo US$ | 0.0071 | 0.0071 | -0.2 | [-19.7, 22.7] | inconclusivo |
| tempo s | 67.8633 | 66.18 | -2.5 | [-40.0, 31.4] | inconclusivo |
| chamadas redundantes | 0.625 | 0.5 | -20.0 | [-100.0, 200.0] | inconclusivo |

## Limites

- N pequeno é descritivo; sem significância não há ganho comprovado.
- Execuções com evidence=simulated validam a infraestrutura, não o produto.
- Retrieval offline não é resultado de desenvolvimento; ver seção offline separadamente.
- Custo ausente (None) não é zero; cost_per_success só aparece quando todas as execuções comparáveis têm custo.
