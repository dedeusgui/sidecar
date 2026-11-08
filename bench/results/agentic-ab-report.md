# Relatório agentic (A/B pareado)

Fonte: `bench/agentic` · execuções: 90 · tarefas no manifesto: 10 (verificadas: 6)

## Braços

| Braço | Tarefas | Execuções | Censuradas | Resolução | TTFE usado (ms) | Δ TTFE vs OFF (ms, IC 95%) | Δ % | Tool calls até gold | Rounds até gold | Fidelidade contrato | Ineficiência de loop | Tokens da tarefa | Δ tokens |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| OFF | 6 | 30 | 5 | 83.3% | 96 | — | — | 18.0 | 2.8 | — | 0.4 | 17772 | — |
| ON-v1 | 6 | 30 | 5 | 83.3% | 144 | +48 [+11, +80] | +50.3% | 11.0 | 2.4 | 1.0 | 0.3 | 11523 | -6249 |
| ON-v2 | 6 | 30 | 5 | 83.3% | 142 | +46 [+8, +76] | +47.9% | 11.0 | 2.4 | 1.0 | 0.3 | 11517 | -6255 |

Censuradas: execuções em que a evidência correta nunca apareceu; entram no TTFE com o tempo de parede (cota inferior), nunca como zero.

## Veredito por tarefa (braço contra OFF)

| Tarefa | Braço | Oráculo | Pares | Δ TTFE ms [IC 95%] | Δ resolução [IC 95%] | Veredito | Motivo |
|---|---|---|---|---|---|---|---|
| R01 | ON-v1 | verificado | 5 | +76 [+64, +86] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R02 | ON-v1 | verificado | 5 | +79 [+65, +91] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R03 | ON-v1 | verificado | 5 | -32 [-38, -26] | +0.00 [+0.00, +0.00] | Sidecar ajudou | evidência correta usada mais cedo, sem perda de resolução |
| R04 | ON-v1 | verificado | 5 | +3 [-1, +8] | +0.00 [+0.00, +0.00] | inconclusivo | IC 95% cruza zero |
| R05 | ON-v1 | verificado | 5 | +86 [+67, +104] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R06 | ON-v1 | verificado | 5 | +79 [+66, +92] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R01 | ON-v2 | verificado | 5 | +79 [+57, +102] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R02 | ON-v2 | verificado | 5 | +76 [+63, +88] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R03 | ON-v2 | verificado | 5 | -37 [-45, -28] | +0.00 [+0.00, +0.00] | Sidecar ajudou | evidência correta usada mais cedo, sem perda de resolução |
| R04 | ON-v2 | verificado | 5 | +11 [+2, +25] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R05 | ON-v2 | verificado | 5 | +76 [+61, +90] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R06 | ON-v2 | verificado | 5 | +72 [+63, +81] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |

Regra fixada antes das execuções: `Sidecar ajudou` exige oráculo verificado, ao menos 3 pares, IC 95% do Δ TTFE abaixo de zero e IC 95% do Δ resolução não negativo. `Sidecar prejudicou` exige IC 95% do Δ TTFE acima de zero ou do Δ resolução abaixo de zero. Os pares são por repetição: com 3 repetições, o IC só exclui zero quando todas as diferenças têm o mesmo sinal.
