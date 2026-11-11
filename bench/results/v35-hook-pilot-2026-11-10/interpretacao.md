# Piloto V3.5 com hook real — 2026-11-10

No commit `9096b39` da main, cinco tarefas históricas de localização foram executadas uma vez em cada braço OFF e V3.5, com Claude Code e `claude-haiku-5-5`. O hook `UserPromptSubmit` real entregou contexto nas cinco execuções ON. As dez execuções foram consideradas comparáveis pelo instrumento, que confirmou o modelo servido. O custo informado pelo cliente foi US$ 0,0579.

| Métrica | OFF | V3.5 |
|---|---:|---:|
| Sucesso segundo o oráculo de arquivos | 4/5 | 5/5 |
| Tempo médio | 16,7 s | 15,7 s |
| Tokens de entrada médios | 83.522 | 110.575 |
| Chamadas de ferramentas médias | 3,0 | 3,6 |
| Custo médio | US$ 0,0054 | US$ 0,0062 |

A diferença de sucesso veio de um único caso negativo; os quatro casos positivos passaram nos dois braços. O hook entregou algum contexto inclusive no negativo. O gold apareceu no contexto em 2 dos 4 casos positivos. A amostra é pequena, usa tarefas internas e um oráculo de arquivos: não demonstra ganho de qualidade, tempo ou custo, nem avalia edição, fatos de navegação ou generalização. As comparações históricas com V2 em `../real-hook-pilot-isolated-2026-11-08/` usam outro commit do repositório e outra versão do cliente; não são um A/B pareado entre versões.

## Controle V2 no mesmo commit

Um segundo bloco usou o mesmo binário, commit, tarefas, modelo, isolamento, orçamento e número de repetições, com `--engine v2`. As dez execuções foram comparáveis e custaram US$ 0,0617 no total. Resultados ON: V2 3/5 versus V3.5 5/5; V2 falhou em H04-model e N01-negative, enquanto V3.5 passou em ambos. V2 teve 16,4 s, 104.091 tokens de entrada, 3,6 chamadas de ferramentas e US$ 0,0060 por execução ON, contra 15,7 s, 110.575 tokens, 3,6 chamadas e US$ 0,0062 na V3.5. Os respectivos braços OFF obtiveram 5/5 e 4/5, evidenciando variabilidade mesmo sem Sidecar. Os blocos foram sequenciais, sem ordem balanceada entre engines; nenhuma diferença deve ser atribuída causalmente à versão com essa amostra. Dados e relatório em `v2-control/`.

## Reprodução e limites do instrumento

Foi compilado `go build -o /tmp/sidecar-v35-bench ./cmd/sidecar` no commit acima. Usou-se `bench/agentic/real_hook_pilot.py` copiado para `/tmp`, com alteração local exclusiva para aceitar `--engine v3.5`, e o runner `bench/agent/run_agents.py` também copiado. O checkout de origem era um clone limpo do mesmo commit. A variável `CLAUDE_CODE_PLUGIN_DIRS` foi removida do ambiente do piloto para impedir a carga do plugin Orca; demais detalhes, hashes e ordem estão em `run-metadata.json`. O comando teve `--reps 1 --max-budget 0.25 --total-budget 2.5 --yes-spend`; usou `bench/agentic/real_hook_pilot_cases.json`. Os dados e o relatório automático estão nesta pasta.

**Não usar `hook_probe_ms` para comparar latência V3.5:** a sonda separada do instrumento não propaga `SIDECAR_ENGINE` e mede o default. O hook real do braço ON recebeu `SIDECAR_ENGINE=v3.5`. O tempo de parede inclui cliente e modelo.

Uma tentativa anterior foi interrompida por carregar `orca-agent-view`: a primeira execução OFF custou US$ 0,00886396 e foi marcada não comparável. Uma chamada ON foi interrompida em andamento e seu custo é desconhecido, limitado pela configuração de US$ 0,25 por execução, sujeito à semântica do cliente. Não incluí-la nas médias. O custo observado conhecido dos dois blocos e da tentativa anterior é aproximadamente US$ 0,1285; o total efetivo é desconhecido.

Não foi executado Codex/Luna: o runner disponível não suporta V3.5 e não estabelece um teto agregado de custo para esse cliente. As coortes externas E1/E2, edição e holdout também não foram executados. `python3 bench/v3.5/freeze.py verify` passou; `python3 -m unittest discover -s bench/v3.5 -p 'test_*.py'` passou com 11 testes, um ignorado por falta dos checkouts externos. As coortes são de autoria acoplada e ainda requerem revisão semântica independente.
