# Scorecard do laboratório

## Evidência: agentic

| braço | exec. | comparáveis | pass | taxa | ocultos ok | escopo (não-teste) | invariantes | custo/correta | p50 s/correta | tool calls | redundantes | injetado |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| OFF | 6 | 6 | 5 | 0.833 | 6 | 1 (0) | 0 | 0.0238 | 33.98 | 13.33 | 1.5 | 0 |
| V2 | 6 | 6 | 5 | 0.833 | 6 | 1 (0) | 0 | 0.0242 | 40.71 | 12.33 | 0.83 | 1831.5 |
| V2+brief | 6 | 6 | 5 | 0.833 | 6 | 0 (0) | 1 | 0.0254 | 37.61 | 14.17 | 1.33 | 1754.3 |
| V2+groom | 6 | 6 | 5 | 0.833 | 5 | 0 (0) | 0 | 0.0247 | 44.78 | 13 | 0.83 | 2003 |
| V2+impact | 6 | 6 | 4 | 0.667 | 5 | 0 (0) | 1 | 0.03 | 38.94 | 13.5 | 0.67 | 2014.7 |

- V2 vs OFF: 6 pares, Δpass 0 IC95 (0, 0), Δcusto 0.0003 → **inconclusivo**
- V2+brief vs OFF: 6 pares, Δpass 0 IC95 (0, 0), Δcusto 0.0013 → **inconclusivo**
- V2+groom vs OFF: 6 pares, Δpass 0 IC95 (-0.5, 0.5), Δcusto 0.0008 → **inconclusivo**
- V2+impact vs OFF: 6 pares, Δpass -0.167 IC95 (-0.5, 0), Δcusto 0.0002 → **inconclusivo**

## Δ% por métrica (agentic): braço contra referência, pareado por tarefa e repetição

Negativo em tool calls/tokens/custo/tempo/redundantes = menos. `inconclusivo` = IC95 cruza zero ou menos de 10 tarefas.

### V2 vs OFF (6 pares, 6 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.8333 | 0.8333 | 0.0 | [0.0, 0.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 13.3333 | 12.3333 | -7.5 | [-20.3, 6.1] | inconclusivo |
| tokens (entrada+saída) | 376171.5 | 357974.1667 | -4.8 | [-24.0, 23.0] | inconclusivo |
| custo US$ | 0.0198 | 0.0202 | 1.8 | [-17.4, 34.3] | inconclusivo |
| tempo s | 52.4 | 54.76 | 4.5 | [-2.6, 10.2] | inconclusivo |
| chamadas redundantes | 1.5 | 0.8333 | -44.4 | [-83.3, 75.0] | inconclusivo |

### V2+brief vs OFF (6 pares, 6 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.8333 | 0.8333 | 0.0 | [0.0, 0.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 13.3333 | 14.1667 | 6.2 | [-4.2, 16.5] | inconclusivo |
| tokens (entrada+saída) | 376171.5 | 417323.1667 | 10.9 | [-3.3, 31.6] | inconclusivo |
| custo US$ | 0.0198 | 0.0212 | 6.8 | [0.0, 17.5] | inconclusivo |
| tempo s | 52.4 | 60.28 | 15.0 | [2.3, 21.8] | inconclusivo |
| chamadas redundantes | 1.5 | 1.3333 | -11.1 | [-50.0, 80.0] | inconclusivo |

### V2+groom vs OFF (6 pares, 6 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.8333 | 0.8333 | 0.0 | [-50.0, 100.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 0.8333 | -16.7 | [-50.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 13.3333 | 13 | -2.5 | [-14.1, 6.0] | inconclusivo |
| tokens (entrada+saída) | 376171.5 | 441901.5 | 17.5 | [-8.0, 42.4] | inconclusivo |
| custo US$ | 0.0198 | 0.0206 | 3.9 | [-13.3, 29.3] | inconclusivo |
| tempo s | 52.4 | 53.7983 | 2.7 | [-22.5, 23.5] | inconclusivo |
| chamadas redundantes | 1.5 | 0.8333 | -44.4 | [-100.0, 9.1] | inconclusivo |

### V2+impact vs OFF (6 pares, 6 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.8333 | 0.6667 | -20.0 | [-60.0, 0.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 0.8333 | -16.7 | [-50.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 13.3333 | 13.5 | 1.2 | [-15.8, 14.4] | inconclusivo |
| tokens (entrada+saída) | 376171.5 | 437881.5 | 16.4 | [2.1, 27.2] | inconclusivo |
| custo US$ | 0.0198 | 0.02 | 1.0 | [-7.7, 15.7] | inconclusivo |
| tempo s | 52.4 | 53.3767 | 1.9 | [-22.1, 20.7] | inconclusivo |
| chamadas redundantes | 1.5 | 0.6667 | -55.6 | [-90.9, -14.3] | inconclusivo |

### V2+brief vs V2 (6 pares, 6 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.8333 | 0.8333 | 0.0 | [0.0, 0.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 1.0 | 0.0 | [0.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 12.3333 | 14.1667 | 14.9 | [-7.1, 44.4] | inconclusivo |
| tokens (entrada+saída) | 357974.1667 | 417323.1667 | 16.6 | [2.9, 37.0] | inconclusivo |
| custo US$ | 0.0202 | 0.0212 | 4.9 | [-14.3, 25.7] | inconclusivo |
| tempo s | 54.76 | 60.28 | 10.1 | [-1.9, 16.8] | inconclusivo |
| chamadas redundantes | 0.8333 | 1.3333 | 60.0 | [-28.6, 400.0] | inconclusivo |

### V2+groom vs V2 (6 pares, 6 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.8333 | 0.8333 | 0.0 | [-50.0, 100.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 0.8333 | -16.7 | [-50.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 12.3333 | 13 | 5.4 | [-7.5, 20.2] | inconclusivo |
| tokens (entrada+saída) | 357974.1667 | 441901.5 | 23.4 | [4.3, 41.9] | inconclusivo |
| custo US$ | 0.0202 | 0.0206 | 2.1 | [-8.9, 11.8] | inconclusivo |
| tempo s | 54.76 | 53.7983 | -1.8 | [-23.5, 19.8] | inconclusivo |
| chamadas redundantes | 0.8333 | 0.8333 | 0.0 | [-100.0, 240.0] | inconclusivo |

### V2+impact vs V2 (6 pares, 6 tarefas)

| métrica | referência | braço | Δ% | IC95 Δ% | leitura |
|---|---:|---:|---:|---|---|
| taxa de aprovação (estrita: teste+escopo+invariantes) | 0.8333 | 0.6667 | -20.0 | [-60.0, 0.0] | inconclusivo |
| testes ocultos passaram (sem escopo) | 1.0 | 0.8333 | -16.7 | [-50.0, 0.0] | inconclusivo |
| chamadas de ferramenta | 12.3333 | 13.5 | 9.5 | [-19.6, 40.7] | inconclusivo |
| tokens (entrada+saída) | 357974.1667 | 437881.5 | 22.3 | [-12.7, 60.9] | inconclusivo |
| custo US$ | 0.0202 | 0.02 | -0.8 | [-24.7, 19.8] | inconclusivo |
| tempo s | 54.76 | 53.3767 | -2.5 | [-21.6, 10.6] | inconclusivo |
| chamadas redundantes | 0.8333 | 0.6667 | -20.0 | [-83.3, 150.0] | inconclusivo |

## Limites

- N pequeno é descritivo; sem significância não há ganho comprovado.
- Execuções com evidence=simulated validam a infraestrutura, não o produto.
- Retrieval offline não é resultado de desenvolvimento; ver seção offline separadamente.
- Custo ausente (None) não é zero; cost_per_success só aparece quando todas as execuções comparáveis têm custo.
