# Relatório agentic (A/B pareado)

Fonte: `bench/agentic` · execuções: 90 · tarefas no manifesto: 10 (verificadas: 6)

## Braços

| Braço | Tarefas | Execuções | Censuradas | Resolução | TTFE usado (ms) | Δ TTFE vs OFF (ms, IC 95%) | Δ % | Tool calls até gold | Rounds até gold | Fidelidade contrato | Ineficiência de loop | Tokens da tarefa | Δ tokens |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| OFF | 6 | 30 | 0 | 100.0% | 40 | — | — | 18.7 | 3.0 | — | 0.4 | 18435 | — |
| ON-v1 | 6 | 30 | 0 | 100.0% | 56 | +16 [-1, +31] | +39.7% | 9.8 | 2.3 | 1.0 | 0.2 | 8960 | -9475 |
| ON-v2 | 6 | 30 | 0 | 100.0% | 57 | +17 [+1, +32] | +43.0% | 9.8 | 2.3 | 1.0 | 0.2 | 8925 | -9510 |

Censuradas: execuções em que a evidência correta nunca apareceu; entram no TTFE com o tempo de parede (cota inferior), nunca como zero.

## Veredito por tarefa (braço contra OFF)

| Tarefa | Braço | Oráculo | Pares | Δ TTFE ms [IC 95%] | Δ resolução [IC 95%] | Veredito | Motivo |
|---|---|---|---|---|---|---|---|
| R01 | ON-v1 | verificado | 5 | -9 [-10, -8] | +0.00 [+0.00, +0.00] | Sidecar ajudou | evidência correta usada mais cedo, sem perda de resolução |
| R02 | ON-v1 | verificado | 5 | +36 [+34, +38] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R03 | ON-v1 | verificado | 5 | -8 [-9, -7] | +0.00 [+0.00, +0.00] | Sidecar ajudou | evidência correta usada mais cedo, sem perda de resolução |
| R04 | ON-v1 | verificado | 5 | +5 [+4, +6] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R05 | ON-v1 | verificado | 5 | +36 [+35, +38] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R06 | ON-v1 | verificado | 5 | +36 [+35, +37] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R01 | ON-v2 | verificado | 5 | -7 [-12, +2] | +0.00 [+0.00, +0.00] | inconclusivo | IC 95% cruza zero |
| R02 | ON-v2 | verificado | 5 | +38 [+36, +40] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R03 | ON-v2 | verificado | 5 | -11 [-12, -10] | +0.00 [+0.00, +0.00] | Sidecar ajudou | evidência correta usada mais cedo, sem perda de resolução |
| R04 | ON-v2 | verificado | 5 | +16 [+4, +40] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R05 | ON-v2 | verificado | 5 | +33 [+31, +34] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |
| R06 | ON-v2 | verificado | 5 | +35 [+31, +40] | +0.00 [+0.00, +0.00] | Sidecar prejudicou | evidência mais tardia ou resolução menor, com IC fora de zero |

Regra fixada antes das execuções: `Sidecar ajudou` exige oráculo verificado, ao menos 3 pares, IC 95% do Δ TTFE abaixo de zero e IC 95% do Δ resolução não negativo. `Sidecar prejudicou` exige IC 95% do Δ TTFE acima de zero ou do Δ resolução abaixo de zero. Os pares são por repetição: com 3 repetições, o IC só exclui zero quando todas as diferenças têm o mesmo sinal.
