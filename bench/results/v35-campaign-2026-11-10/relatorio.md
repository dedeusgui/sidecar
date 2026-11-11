# Campanha V3.5 — 2026-11-10

Commit do Sidecar: `9096b39e173d73dd97fff727bcdda2c9f1411fb3`. Os dez repositórios históricos foram obtidos nos commits de `bench/ten_repos_cases.json`. Dados anteriores já foram inspecionados: esta é uma avaliação de regressão, não um holdout cego nem uma medida independente de produtividade.

## Contexto efetivamente entregue: dez repositórios

O script local `sidecar-context-campaign.py` executou `sidecar explain --engine v3` e `--engine v3.5` para os 50 casos históricos e para os 180 negativos difíceis dos splits dev e validation. Uma presença positiva significa que o caminho literal do gold aparece em `context`, não na lista interna de candidatos. Esse teste é diagnóstico de entrega, não Hit@5 nem resposta correta de agente. Linhas e hashes de contexto estão em `sidecar-context-campaign.jsonl`; o código-fonte recuperado não foi gravado nesse arquivo. O script usa caminhos locais `/tmp` e deve ser ajustado em outra máquina.

| Casos | V3: gold no contexto | V3.5: gold no contexto |
|---|---:|---:|
| Diretos | 18/20 | 1/20 |
| PT imperfeito | 7/10 | 1/10 |
| Voz | 10/10 | 10/10 |

Nos 180 negativos difíceis, houve contexto não vazio em 14 casos V3 e 8 V3.5; nos dez negativos históricos, ambos retornaram contexto vazio. Por exemplo, em `R02-1`, a V3.5 listou `middleware/timeout.go` como evidência interna, mas seu `context` entregou apenas uma orientação sobre teste desconhecido, sem esse caminho. Uma chamada adicional do hook Claude com o mesmo prompt confirmou a mesma saída. Isso justifica investigação; não prova sozinho que o agente falhará, pois ele ainda pode pesquisar.

O avaliador histórico completo para os dez repositórios foi executado separadamente; seus dados estão em `sidecar-ten-main.json` e `sidecar-ten-main.md`. Ele não implementa V3.5 e não deve ser apresentado como avaliação dela. As medições ocorreram junto com outras tarefas; não usar a latência como comparação controlada.

## Coortes congeladas E1/E2

Foram coletadas 720 medições: 70 casos E1 e 20 E2, duas engines, `explain` e chamada direta do hook com payload de formato Codex, e cache frio/quente. O hook foi chamado diretamente, não por um cliente Codex real. Os dados sem o texto de código estão em `e1e2-rows.jsonl`. Os cinco repositórios e gold tiveram commits e hashes conferidos, mas as coortes são de autoria acoplada e não receberam revisão semântica independente.

Para E1 com cache quente, a presença literal de caminho gold no contexto de `explain` foi V3 21/30 e V3.5 2/30; contexto não vazio nos negativos foi 19/40 e 13/40. Na chamada direta do hook, os números correspondentes foram 9/30 e 4/30, com 9/40 e 11/40 negativos não vazios. Presença literal não é Hit@5 nem sucesso do agente.

Para E2 com cache quente, o diagnóstico automático de termos literais do gabarito em `explain` marcou V3 0/20 e V3.5 1/20; no hook direto, 0/20 para ambas. A regra aceita uma alternativa textual por fato, mas não avalia equivalentes semânticos: não é uma nota de resposta correta. A coleta foi retomada após dois problemas do instrumento: `evidence: null` e um arquivo de lock que desapareceu durante o hash do índice. O script de retomada foi preservado; hashes de índice foram omitidos dos dados publicados porque o tratamento de locks mudou durante a execução. Não há medição válida de latência controlada por causa de carga concorrente.

## Localização com agente real

`claude-haiku-5-5`, 50 tarefas × OFF e V3.5, uma repetição por braço, nos dez repositórios fixados: 100/100 execuções comparáveis segundo o runner. A V3.5 foi acrescentada à lista de engines em uma cópia temporária do runner, preservada junto dos resultados em `haiku-ten-repos/`. O braço ON usa o contexto do hook como **prefixo ao prompt**, não o hook do cliente. O runner removeu `CLAUDE_CODE_PLUGIN_DIRS` do ambiente da campanha. O modelo servido foi `claude-haiku-5-5` em todas as execuções.

| Métrica | OFF | V3.5 |
|---|---:|---:|
| Sucesso (gold de arquivo) | 49/50 | 49/50 |
| Tempo médio | 12,1 s | 11,2 s |
| Tokens de entrada médios | 64.399 | 56.879 |
| Custo médio | US$ 0,0038 | US$ 0,0035 |

As falhas foram o negativo `R10-5` OFF e `R01-5` ON. O custo total informado foi US$ 0,36848144. O runner não observa chamadas de ferramenta neste formato de saída Claude. Trata-se de localização em tarefas do autor e com quase todos os casos acertados, sem teste de edição ou evidência de ganho generalizável. O intervalo pareado de tempo do relatório inclui zero; a máquina executava outras coletas. A linha automática `Gold no contexto 0/40` usa regex incompatível com o formato V3.5; não interpretar esse campo como medida válida. A verificação literal independente acima encontrou 12/40. As duas medições foram realizadas em coletas separadas e não provam que o agente tenha usado o contexto.

## Piloto de edição com hook real

Em `CHI-EDIT-01`, OFF, V2 e V3.5 passaram no oráculo; todos os braços foram comparáveis e ambos os hooks ON tiveram entrega confirmada. É uma tarefa com uma repetição: não demonstra vantagem. Dados em `lab-pilot/`.

A primeira tentativa, em `lab-attempt/`, retornou sem modelo servido e foi interrompida no segundo braço. O primeiro registro tem `verdict: fail`, mas não é um resultado válido do agente. O custo desses dois braços não é conhecido; cada um foi configurado com limite de US$ 0,10. Identificou-se uma hipótese de DNS: `/etc/resolv.conf` aponta para um arquivo sob `/run`, que o sandbox deixa vazio. Uma sonda sem modelo confirmou resolução com esse arquivo montado; a repetição com essa montagem produziu os três resultados comparáveis acima. A causa original não foi provada, pois o runner não preservou stderr dessa tentativa.

## Laboratório: nove tarefas sem instaladores

Incluindo o piloto, nove tarefas de desenvolvimento/validação foram executadas com OFF, V2 e V3.5, uma vez por braço: 27 registros comparáveis. O veredito estrito foi OFF 9/9, V2 9/9 e V3.5 7/9. Os testes ou veredictos comportamentais passaram nos 27 registros; as falhas estritas da V3.5 ocorreram em `CHI-DBG-01` e `CHI-MF-01`, por criação de arquivos de teste fora da lista permitida. Em `CHI-MF-01` também falhou a invariante por regex de preservação de normalização de barra invertida contra open redirect. Essa verificação textual merece análise, mas não demonstra por si só uma vulnerabilidade. Os intervalos e detalhes estão em `sidecar-v35-lab-combined.md`; nove tarefas e uma repetição não estabelecem causalidade ou diferença generalizável.

As tarefas do Click e Express rodaram em uma segunda rodada simultânea para reduzir a espera. A rodada inicial foi interrompida depois de concluir os braços Chi; ela ainda registrou dois braços extras duplicados do Click e interrompeu outro em andamento. Eles estão em `lab-rest/`, mas foram excluídos do consolidado, que usa os três braços completos de `lab-extra/`. O custo informado dos 27 registros consolidados foi US$ 0,18774358; uma chamada interrompida adicional tem custo desconhecido e limite configurado de US$ 0,10.

Os 14 casos restantes do laboratório exigem preparação com instalação de dependências que o runner atual executa fora do sandbox do agente, com acesso ao ambiente do operador. Não foram executados nesta rodada; precisam de preparação isolada e validação antes de rodar. Codex/Luna e Antigravity não foram executados: o primeiro não informa custo monetário nem permite impor o teto em dólares; o segundo permanece bloqueado pelo runner por isolamento não qualificado. O usuário autorizou Codex e manteve teto total de US$ 10; a campanha não comprovou uma forma de garantir esse teto para o cliente. Os demais benchmarks consolidados não adaptados ao caminho V3.5 e o holdout selado permanecem pendentes.

O custo monetário informado em todas as execuções desta campanha e no piloto anterior é aproximadamente US$ 0,6876, incluindo duplicatas registradas. Além disso, há chamadas interrompidas e uma chamada sem modelo servido cujo custo é desconhecido; cada uma tinha teto configurado entre US$ 0,10 e US$ 0,25, sujeito à semântica do cliente. Valores desconhecidos não foram somados como zero.

## Integridade

`python3 bench/v3.5/freeze.py verify-sources --checkout /tmp/sidecar-v35-repos` confirmou os cinco commits e os hashes de gold das coortes E1/E2. `env TMPDIR=/var/tmp/sidecar-tests-S2onYe go test -count=1 ./...` passou. A primeira execução de `go test -count=1 ./...` falhou porque testes temporários detectaram `/tmp` como raiz Git e tentaram ler diretórios protegidos; o resultado isolado não reproduziu essas falhas.

Dogfood isolado com `sidecar explain --engine v3.5 --root . --json` para uma pergunta sobre os benchmarks retornou `bench/lab/test_lab.py`, `bench/agentic/run_edit_pilot.py` e `bench/lab/lab.py` entre as evidências internas; o contexto entregue tinha 395 caracteres. É uma única consulta e não demonstra benefício ao agente.
