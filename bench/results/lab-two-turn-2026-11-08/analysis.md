# Análise dos runs do lab (30 execuções, 6 tarefas, 1 repetição(ões), braços: OFF, V2, V2+brief, V2+groom, V2+impact)

## Distribuição das falhas por braço

| causa | OFF | V2 | V2+brief | V2+groom | V2+impact |
|---|---:|---:|---:|---:|---:|
| escopo_so_testes_extras | 1 | 1 | 0 | 0 | 0 |
| invariante_quebrado | 0 | 0 | 1 | 0 | 1 |
| ok | 5 | 5 | 5 | 5 | 4 |
| teste_oculto_falhou | 0 | 0 | 0 | 1 | 1 |

## Tarefas que nenhum braço resolve

nenhuma

Se há tarefas aqui, verifique primeiro oráculo e escopo (defeito do corpus) antes de culpar o contexto.

## Tarefas que dividem os braços (onde o contexto muda o resultado)

| tarefa | classe | taxa por braço |
|---|---|---|
| CHI-LOC-01 | locate | {'OFF': 1.0, 'V2': 1.0, 'V2+brief': 1.0, 'V2+groom': 0.0, 'V2+impact': 0.0} |
| CHI-MF-01 | multi-file | {'OFF': 0.0, 'V2': 0.0, 'V2+brief': 0.0, 'V2+groom': 1.0, 'V2+impact': 0.0} |

## Por classe de tarefa

### debug

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 3 | 1.0 | 13.333 | 0.018 | 379223 |
| V2 | 3 | 1.0 | 14 | 0.022 | 431273 |
| V2+brief | 3 | 1.0 | 13.333 | 0.019 | 441012.333 |
| V2+groom | 3 | 1.0 | 13.667 | 0.022 | 493342.667 |
| V2+impact | 3 | 1.0 | 12 | 0.019 | 411626.333 |

### edit

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 1 | 1.0 | 11 | 0.014 | 274795 |
| V2 | 1 | 1.0 | 10 | 0.013 | 250059 |
| V2+brief | 1 | 1.0 | 10 | 0.013 | 276871 |
| V2+groom | 1 | 1.0 | 9 | 0.011 | 232391 |
| V2+impact | 1 | 1.0 | 12 | 0.012 | 319672 |

### locate

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 1 | 1.0 | 3 | 0.005 | 95602 |
| V2 | 1 | 1.0 | 2 | 0.005 | 75803 |
| V2+brief | 1 | 1.0 | 4 | 0.007 | 150914 |
| V2+groom | 1 | 0.0 | 2 | 0.005 | 101732 |
| V2+impact | 1 | 0.0 | 3 | 0.006 | 127572 |

### multi-file

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 1 | 0.0 | 26 | 0.048 | 748963 |
| V2 | 1 | 0.0 | 20 | 0.037 | 528164 |
| V2+brief | 1 | 0.0 | 31 | 0.049 | 753117 |
| V2+groom | 1 | 1.0 | 26 | 0.041 | 837258 |
| V2+impact | 1 | 0.0 | 30 | 0.045 | 945166 |

## Onde está o gasto e a repetição

- tarefas mais caras (custo médio US$): [('CHI-MF-01', 0.044), ('CHI-DBG-01', 0.027), ('SIN-DBG-01', 0.021), ('CHI-EDIT-01', 0.013), ('CLK-DBG-01', 0.012)]
- chamadas redundantes médias por braço: {'OFF': 1.5, 'V2': 0.833, 'V2+brief': 1.333, 'V2+impact': 0.667, 'V2+groom': 0.833}
- execuções com estagnação: {'OFF': 0, 'V2': 0, 'V2+brief': 0, 'V2+groom': 1, 'V2+impact': 0}
- contexto injetado médio (chars): {'OFF': 0, 'V2': 1831.5, 'V2+brief': 1754.333, 'V2+groom': 2003, 'V2+impact': 2014.667}
- % das execuções em que o contexto foi entregue: {'V2': 100.0, 'V2+brief': 100.0, 'V2+groom': 100.0, 'V2+impact': 100.0}

