# E2 offline — fatos do oráculo presentes no que cada braço entrega (sem modelo)

| tarefa | split | fatos | V2 hook | V3 hook (ação) | explore | chars V2 / V3 / explore | faltam no explore |
|---|---|---:|---:|---:|---:|---|---|
| NAV-CHI-01 | development | 4 | 3 | 3 (EXPLORE_AVAILABLE) | 4 | 2192 / 1391 / 4237 | — |
| NAV-CLK-01 | development | 4 | 2 | 2 (EXPLORE_AVAILABLE) | 3 | 2529 / 1671 / 9836 | parse_args |
| NAV-EXP-01 | development | 3 | 0 | 0 (EXPLORE_AVAILABLE) | 3 | 229 / 229 / 5667 | — |
| NAV-HON-01 | development | 3 | 2 | 2 (EXPLORE_AVAILABLE) | 3 | 1665 / 1083 / 5850 | — |
| NAV-MON-01 | validation | 4 | 2 | 1 (EXPLORE_AVAILABLE) | 2 | 2254 / 1472 / 4161 | handle(|->handle|handle , abstractprocessinghandler|write( |
| NAV-SER-01 | validation | 4 | 4 | 4 (EXPLORE_AVAILABLE) | 4 | 1842 / 1209 / 2625 | — |
| NAV-SIN-01 | validation | 4 | 1 | 1 (EXPLORE_AVAILABLE) | 3 | 1639 / 1081 / 6479 | route! |

| braço | fatos cobertos | tarefas com todos os fatos | caracteres totais |
|---|---:|---:|---:|
| v2 | 14/26 | 1/7 | 12350 |
| v3 | 13/26 | 1/7 | 8136 |
| explore | 22/26 | 4/7 | 38855 |

Leitura: cobertura de fatos na evidência é condição para economizar exploração, não prova de resposta correta nem de economia end-to-end.
