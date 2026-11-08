# Análise dos runs do lab (225 execuções, 15 tarefas, 3 repetição(ões), braços: OFF, V1, V2, V2+adaptive, V2+brief)

## Distribuição das falhas por braço

| causa | OFF | V1 | V2 | V2+adaptive | V2+brief |
|---|---:|---:|---:|---:|---:|
| escopo_fora_do_codigo_permitido | 1 | 2 | 2 | 2 | 0 |
| escopo_so_testes_extras | 8 | 8 | 6 | 5 | 6 |
| invariante_quebrado | 1 | 2 | 1 | 3 | 1 |
| ok | 35 | 33 | 36 | 35 | 38 |

## Tarefas que nenhum braço resolve

nenhuma

Se há tarefas aqui, verifique primeiro oráculo e escopo (defeito do corpus) antes de culpar o contexto.

## Tarefas que dividem os braços (onde o contexto muda o resultado)

| tarefa | classe | taxa por braço |
|---|---|---|
| CHI-CC-01 | contract-change | {'OFF': 0.33, 'V1': 1.0, 'V2': 1.0, 'V2+adaptive': 1.0, 'V2+brief': 1.0} |
| CHI-DBG-01 | debug | {'OFF': 1.0, 'V1': 0.0, 'V2': 0.33, 'V2+adaptive': 0.67, 'V2+brief': 0.67} |
| CHI-MF-01 | multi-file | {'OFF': 0.33, 'V1': 0.0, 'V2': 0.67, 'V2+adaptive': 0.33, 'V2+brief': 0.67} |
| CLK-AMB-01 | ambiguous | {'OFF': 0.0, 'V1': 0.0, 'V2': 0.67, 'V2+adaptive': 0.33, 'V2+brief': 0.0} |
| CLK-CC-01 | contract-change | {'OFF': 1.0, 'V1': 1.0, 'V2': 0.33, 'V2+adaptive': 0.33, 'V2+brief': 1.0} |
| CLK-EDIT-01 | edit | {'OFF': 0.0, 'V1': 0.0, 'V2': 0.0, 'V2+adaptive': 0.33, 'V2+brief': 0.33} |
| SIN-DBG-01 | debug | {'OFF': 1.0, 'V1': 1.0, 'V2': 1.0, 'V2+adaptive': 0.67, 'V2+brief': 1.0} |

## Por classe de tarefa

### ambiguous

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 3 | 0.0 | 14 | 0.011 | 404622.667 |
| V1 | 3 | 0.0 | 12.333 | 0.009 | 360787.333 |
| V2 | 3 | 0.67 | 15.667 | 0.011 | 397935 |
| V2+adaptive | 3 | 0.33 | 16.333 | 0.011 | 429082.667 |
| V2+brief | 3 | 0.0 | 14.667 | 0.01 | 399485.333 |

### contract-change

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 9 | 0.78 | 12.333 | 0.008 | 294361.667 |
| V1 | 9 | 1.0 | 14.889 | 0.009 | 324187.222 |
| V2 | 9 | 0.78 | 13.556 | 0.009 | 313893.333 |
| V2+adaptive | 9 | 0.78 | 16.444 | 0.01 | 370147.222 |
| V2+brief | 9 | 1.0 | 13.333 | 0.009 | 305779.222 |

### debug

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 12 | 1.0 | 13.667 | 0.009 | 359058.333 |
| V1 | 12 | 0.75 | 12.667 | 0.008 | 305830.333 |
| V2 | 12 | 0.83 | 11.333 | 0.008 | 310491.333 |
| V2+adaptive | 12 | 0.83 | 11 | 0.008 | 295266.667 |
| V2+brief | 12 | 0.92 | 10.667 | 0.007 | 258651.25 |

### edit

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 9 | 0.67 | 10.333 | 0.006 | 247029.889 |
| V1 | 9 | 0.67 | 10.222 | 0.007 | 261252.889 |
| V2 | 9 | 0.67 | 11 | 0.007 | 274548.111 |
| V2+adaptive | 9 | 0.78 | 10.333 | 0.007 | 282268.222 |
| V2+brief | 9 | 0.78 | 10.222 | 0.007 | 234387.889 |

### locate

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 6 | 1.0 | 3.167 | 0.003 | 88742.167 |
| V1 | 6 | 1.0 | 1.667 | 0.002 | 61599.333 |
| V2 | 6 | 1.0 | 2.5 | 0.003 | 79666 |
| V2+adaptive | 6 | 1.0 | 1.667 | 0.002 | 66566 |
| V2+brief | 6 | 1.0 | 2.333 | 0.002 | 70184.333 |

### multi-file

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 3 | 0.33 | 23 | 0.019 | 605409.667 |
| V1 | 3 | 0.0 | 22.667 | 0.02 | 730254.333 |
| V2 | 3 | 0.67 | 22.333 | 0.017 | 569113.333 |
| V2+adaptive | 3 | 0.33 | 19.333 | 0.02 | 627005.667 |
| V2+brief | 3 | 0.67 | 25.333 | 0.02 | 675776.333 |

### negative

| braço | n | aprovação | tool calls | custo US$ | tokens |
|---|---:|---:|---:|---:|---:|
| OFF | 3 | 1.0 | 0 | 0.001 | 22795.333 |
| V1 | 3 | 1.0 | 0 | 0.001 | 23381.333 |
| V2 | 3 | 1.0 | 0 | 0.001 | 23558.667 |
| V2+adaptive | 3 | 1.0 | 0 | 0.001 | 23392.333 |
| V2+brief | 3 | 1.0 | 0 | 0.001 | 23458.333 |

## Onde está o gasto e a repetição

- tarefas mais caras (custo médio US$): [('CHI-MF-01', 0.019), ('CLK-AMB-01', 0.011), ('CHI-CC-01', 0.011), ('CHI-DBG-01', 0.011), ('SIN-DBG-01', 0.009)]
- chamadas redundantes médias por braço: {'OFF': 0.786, 'V1': 0.381, 'V2': 0.643, 'V2+adaptive': 0.405, 'V2+brief': 0.405}
- execuções com estagnação: {'OFF': 0, 'V1': 0, 'V2': 0, 'V2+adaptive': 0, 'V2+brief': 0}
- contexto injetado médio (chars): {'OFF': 0, 'V1': 1268.022, 'V2': 1541.422, 'V2+adaptive': 1526.956, 'V2+brief': 1368.933}
- % das execuções em que o contexto foi entregue: {'V1': 100.0, 'V2': 100.0, 'V2+adaptive': 100.0, 'V2+brief': 100.0}

