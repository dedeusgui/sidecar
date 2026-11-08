# Scorecard do laboratório

## Evidência: agentic

| braço | exec. | comparáveis | pass | taxa | ocultos ok | escopo (não-teste) | invariantes | custo/correta | p50 s/correta | tool calls | redundantes | injetado |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| OFF | 45 | 45 | 35 | 0.778 | 45 | 9 (1) | 1 | 0.0101 | 26.56 | 11.07 | 0.79 | 0 |
| V1 | 45 | 45 | 33 | 0.733 | 45 | 11 (2) | 2 | 0.0107 | 26.48 | 10.96 | 0.38 | 1268.0 |
| V2 | 45 | 45 | 36 | 0.8 | 45 | 8 (2) | 1 | 0.0096 | 25.12 | 10.8 | 0.64 | 1541.4 |
| V2+adaptive | 45 | 45 | 35 | 0.778 | 45 | 9 (2) | 3 | 0.0103 | 26.1 | 10.89 | 0.4 | 1527.0 |
| V2+brief | 45 | 45 | 38 | 0.844 | 45 | 6 (0) | 1 | 0.0089 | 26.02 | 10.53 | 0.4 | 1368.9 |

- V1 vs OFF: 45 pares, Δpass -0.044 IC95 (-0.15555555555555556, 0.06666666666666667), Δcusto -0.0 → **ver IC**
- V2 vs OFF: 45 pares, Δpass 0.022 IC95 (-0.13333333333333333, 0.15555555555555556), Δcusto -0.0002 → **ver IC**
- V2+adaptive vs OFF: 45 pares, Δpass 0 IC95 (-0.13333333333333333, 0.13333333333333333), Δcusto 0.0001 → **ver IC**
- V2+brief vs OFF: 45 pares, Δpass 0.067 IC95 (-0.044444444444444446, 0.17777777777777778), Δcusto -0.0003 → **ver IC**

## Δ% por métrica (agentic): braço contra referência, pareado por tarefa e repetição

Negativo em tool calls/tokens/custo/tempo/redundantes = menos. `inconclusivo` = IC95 cruza zero ou menos de 10 tarefas.

### V1 vs OFF (45 pares, 15 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.7778 | 0.7333 | -5.7 | [-28.9, 14.7] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 11.0667 | 10.9556 | -1.0 | [-10.2, 9.4] | inconclusivo |
| tokens (entrada+saída) | 284714.6667 | 281150.8889 | -1.3 | [-11.6, 8.6] | inconclusivo |
| custo US$ | 0.0078 | 0.0078 | -0.4 | [-7.4, 6.6] | inconclusivo |
| tempo s | 33.0582 | 39.7727 | 20.3 | [1.0, 39.8] | maior |
| chamadas redundantes | 0.7857 | 0.381 | -51.5 | [-76.0, 0.0] | inconclusivo |

### V2 vs OFF (45 pares, 15 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.7778 | 0.8 | 2.9 | [-18.9, 32.3] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 11.0667 | 10.8 | -2.4 | [-13.7, 7.9] | inconclusivo |
| tokens (entrada+saída) | 284714.6667 | 277148.5778 | -2.7 | [-13.6, 10.4] | inconclusivo |
| custo US$ | 0.0078 | 0.0077 | -2.3 | [-11.3, 8.1] | inconclusivo |
| tempo s | 33.0582 | 35.7942 | 8.3 | [-5.3, 18.8] | inconclusivo |
| chamadas redundantes | 0.7857 | 0.6429 | -18.2 | [-48.9, 70.0] | inconclusivo |

### V2+adaptive vs OFF (45 pares, 15 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.7778 | 0.7778 | 0.0 | [-17.5, 24.1] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 11.0667 | 10.8889 | -1.6 | [-15.0, 12.8] | inconclusivo |
| tokens (entrada+saída) | 284714.6667 | 290061.7111 | 1.9 | [-9.8, 14.9] | inconclusivo |
| custo US$ | 0.0078 | 0.008 | 1.7 | [-6.8, 11.4] | inconclusivo |
| tempo s | 33.0582 | 34.1536 | 3.3 | [-5.7, 14.3] | inconclusivo |
| chamadas redundantes | 0.7857 | 0.4048 | -48.5 | [-73.3, -0.0] | inconclusivo |

### V2+brief vs OFF (45 pares, 15 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.7778 | 0.8444 | 8.6 | [-4.7, 28.6] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 11.0667 | 10.5333 | -4.8 | [-15.2, 5.2] | inconclusivo |
| tokens (entrada+saída) | 284714.6667 | 259613.0 | -8.8 | [-20.4, 2.1] | inconclusivo |
| custo US$ | 0.0078 | 0.0075 | -4.0 | [-12.1, 3.7] | inconclusivo |
| tempo s | 33.0582 | 32.8973 | -0.5 | [-10.5, 6.9] | inconclusivo |
| chamadas redundantes | 0.7857 | 0.4048 | -48.5 | [-71.9, 6.3] | inconclusivo |

### V2 vs V1 (45 pares, 15 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.7333 | 0.8 | 9.1 | [-10.0, 41.7] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 10.9556 | 10.8 | -1.4 | [-9.1, 7.8] | inconclusivo |
| tokens (entrada+saída) | 281150.8889 | 277148.5778 | -1.4 | [-10.2, 10.8] | inconclusivo |
| custo US$ | 0.0078 | 0.0077 | -2.0 | [-7.6, 5.1] | inconclusivo |
| tempo s | 39.7727 | 35.7942 | -10.0 | [-23.3, 0.7] | inconclusivo |
| chamadas redundantes | 0.381 | 0.6429 | 68.7 | [8.0, 233.3] | maior |

### V2+adaptive vs V1 (45 pares, 15 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.7333 | 0.7778 | 6.1 | [-12.8, 37.5] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 10.9556 | 10.8889 | -0.6 | [-9.1, 9.5] | inconclusivo |
| tokens (entrada+saída) | 281150.8889 | 290061.7111 | 3.2 | [-4.8, 13.7] | inconclusivo |
| custo US$ | 0.0078 | 0.008 | 2.1 | [-3.2, 8.0] | inconclusivo |
| tempo s | 39.7727 | 34.1536 | -14.1 | [-24.1, 2.2] | inconclusivo |
| chamadas redundantes | 0.381 | 0.4048 | 6.3 | [-33.3, 84.6] | inconclusivo |

### V2+brief vs V1 (45 pares, 15 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.7333 | 0.8444 | 15.2 | [0.0, 45.8] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 10.9556 | 10.5333 | -3.9 | [-12.8, 5.7] | inconclusivo |
| tokens (entrada+saída) | 281150.8889 | 259613.0 | -7.7 | [-14.5, 0.2] | inconclusivo |
| custo US$ | 0.0078 | 0.0075 | -3.7 | [-9.1, 1.3] | inconclusivo |
| tempo s | 39.7727 | 32.8973 | -17.3 | [-29.9, -5.0] | melhor/menor |
| chamadas redundantes | 0.381 | 0.4048 | 6.3 | [-54.2, 162.5] | inconclusivo |

### V2+adaptive vs V2 (45 pares, 15 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.8 | 0.7778 | -2.8 | [-13.9, 12.1] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 10.8 | 10.8889 | 0.8 | [-7.0, 9.1] | inconclusivo |
| tokens (entrada+saída) | 277148.5778 | 290061.7111 | 4.7 | [-3.4, 12.0] | inconclusivo |
| custo US$ | 0.0077 | 0.008 | 4.1 | [-1.8, 9.1] | inconclusivo |
| tempo s | 35.7942 | 34.1536 | -4.6 | [-13.7, 7.6] | inconclusivo |
| chamadas redundantes | 0.6429 | 0.4048 | -37.0 | [-65.4, -6.9] | melhor/menor |

### V2+brief vs V2 (45 pares, 15 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.8 | 0.8444 | 5.6 | [-10.5, 26.7] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 10.8 | 10.5333 | -2.5 | [-9.3, 4.7] | inconclusivo |
| tokens (entrada+saída) | 277148.5778 | 259613.0 | -6.3 | [-16.1, 3.0] | inconclusivo |
| custo US$ | 0.0077 | 0.0075 | -1.7 | [-8.1, 5.8] | inconclusivo |
| tempo s | 35.7942 | 32.8973 | -8.1 | [-12.8, -2.9] | melhor/menor |
| chamadas redundantes | 0.6429 | 0.4048 | -37.0 | [-64.3, 3.7] | inconclusivo |

## Limites

- N pequeno é descritivo; sem significância não há ganho comprovado.
- Execuções com evidence=simulated validam a infraestrutura, não o produto.
- Retrieval offline não é resultado de desenvolvimento; ver seção offline separadamente.
- Custo ausente (None) não é zero; cost_per_success só aparece quando todas as execuções comparáveis têm custo.
