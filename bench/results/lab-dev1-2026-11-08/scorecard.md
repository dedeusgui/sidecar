# Scorecard do laboratório

## Evidência: agentic

| braço | exec. | comparáveis | pass | taxa | ocultos ok | escopo (não-teste) | invariantes | custo/correta | p50 s/correta | tool calls | injetado |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| OFF | 8 | 8 | 6 | 0.75 | 7 | 2 (1) | 0 | 0.0107 | 27.21 | 11 | 0 |
| V1 | 8 | 8 | 5 | 0.625 | 8 | 3 (0) | 0 | 0.0113 | 26.58 | 9.5 | 1285.1 |
| V2 | 8 | 8 | 6 | 0.75 | 8 | 2 (0) | 0 | 0.0094 | 22.22 | 9.62 | 1602.1 |

- V1 vs OFF: 8 pares, Δpass -0.125 IC95 (-0.375, 0), Δcusto -0.0009 → **inconclusivo**
- V2 vs OFF: 8 pares, Δpass 0 IC95 (0, 0), Δcusto -0.0009 → **inconclusivo**

## Limites

- N pequeno é descritivo; sem significância não há ganho comprovado.
- Execuções com evidence=simulated validam a infraestrutura, não o produto.
- Retrieval offline não é resultado de desenvolvimento; ver seção offline separadamente.
- Custo ausente (None) não é zero; cost_per_success só aparece quando todas as execuções comparáveis têm custo.
