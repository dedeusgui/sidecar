# Evaluation Lab (R1)

Infraestrutura para medir **tarefas de edição reais** com oráculos independentes, replay offline e latência por estágio. Complementa (não substitui) `bench/agent`, `bench/agentic` e `cmd/sidecar-eval`. Estado e evidências: [mission-progress.md](mission-progress.md).

## 1. Harness de edição — `bench/lab/lab.py`

| Garantia | Como |
|---|---|
| Checkout descartável | `git archive` do commit fixado + um único commit-base local; sem remotes, refs nem histórico upstream. Alterações medidas por `git status` contra esse commit (inclui untracked/ignorados) |
| Oráculo oculto | `tasks/<ID>/oracle/` nunca entra no workspace durante a execução. Depois do agente é copiado, roda **sem rede** (`bwrap --unshare-net` ou `unshare -rn`; o isolamento efetivo é gravado em cada execução) e é removido |
| Oráculo não vacuoso | `oracle.require_output` (regex) prova que o teste executou; sem ele o resultado é `error` (infra), nunca `fail` do agente |
| Oráculo satisfazível | `validate-task`: o oráculo **falha na base**, **passa com `reference.patch`**, dentro do escopo e dos invariantes |
| Escopo e contratos | `scope.allowed_paths` + `scope.invariants` (`file_unchanged`, `regex_present`, `regex_absent`). Veredito `pass` exige teste + invariantes + escopo |
| Dependências | fase `setup` (única com rede) antes do agente; artefatos criados nela são linha de base e não contam como alteração |
| Isolamento do cliente | `ClaudeAgent`: `CLAUDE_CONFIG_DIR` novo por execução, canário, `--setting-sources project,local`, sem MCP/slash, verificação do `init`, **modelo servido deve ser igual ao ID solicitado** (alias é recusado) |
| Braços balanceados | `OFF`, `V1`, `V2` em rotação (quadrado latino) com embaralho por semente; retomada sem repetir execuções |
| Holdout selado | split `holdout` só com `--final`; todo uso é anexado a `corpus/holdout_ledger.jsonl`. Ledger não vazio = holdout consumido |
| Custo | só com `--yes-spend`; teto por execução (`--max-budget`) e total (`--max-total-usd`) lido de `spend.jsonl`; sem CI pago |
| Simulação isolada | `ScriptedAgent` testa a infraestrutura; seus registros têm `evidence: simulated` e nunca entram na seção `agentic` |

O caminho normal usa o inventário selado de `manifest.json` sem abrir tarefas ou oráculos de holdout. O holdout real exige, antes da leitura, `--final`, `--yes-spend`, `--frozen-revision` igual ao HEAD limpo, `--frozen-binary-sha256` e preflight do sandbox; os hashes são conferidos após o registro no ledger. Atualização do manifesto e validação direta do holdout real ficam bloqueadas durante esta etapa.

```bash
python3 bench/lab/lab.py validate-corpus                     # development/validation; preserva holdout selado
python3 bench/lab/lab.py manifest                            # corpus/manifest.json: hashes, splits, classes
python3 bench/lab/lab.py run --agent claude --model claude-haiku-5-5 --sidecar /tmp/sidecar \
    --arms OFF,V1,V2 --reps 1 --out-dir /tmp/lab-run          # dry-run; --yes-spend executa
python3 bench/lab/lab.py report /tmp/lab-run/runs.jsonl --md /tmp/lab-run/scorecard.md
python3 -m unittest bench.lab.test_lab                       # testes do harness (sem rede nem modelo)
```

## 2. Corpus — `bench/lab/corpus/`

Repositórios e commits em `repos.json` (mesmos pins de `bench/ten_repos_cases.json`). Cada tarefa tem `task.json` (`lab-task/1`), `oracle/`, `reference.patch` e `validation.json` (gerado). Classes: `locate`, `debug`, `edit`, `multi-file`, `contract-change`, `negative`, `ambiguous` (com `underspec.canned_answer`, base do R4), `voice`. Splits `development`/`validation`/`holdout`. O holdout desta rodada é **novo**, jamais lido por tuning.

Limite: os autores das tarefas e do `reference.patch` são subagentes e o mesmo time que escreveu o Sidecar; os oráculos testam comportamento, mas o viés de autoria permanece. Veja a contagem real e o estado de validação em [mission-progress.md](mission-progress.md).

## 3. Replay offline — `sidecar-eval --replay`

`bench/replay/params.json` (`replay-config/1`) versiona `retrieve.Tuning`: `active` (espelho de `DefaultTuning()`, guardado por teste e por `--replay-check`), `variants` explícitas e uma `grid` limitada (≤ 32 variantes). O replay roda o pipeline V2 sobre os casos congelados para cada variante, sem modelo nem rede; grava `run_key` (SHA dos casos, da config, do índice por repositório, do `git HEAD`, Go), candidatos com fatores/confiança por caso, regressões e melhorias contra `active`, e **prova determinismo** rodando `active` duas vezes.

```bash
go run ./cmd/sidecar-eval --cases bench/cases.json --replay bench/replay/params.json --out /tmp/replay.json
go run ./cmd/sidecar-eval --external-cases /tmp/manifest.json --replay bench/replay/params.json --out /tmp/replay-ext.json
go run ./cmd/sidecar-eval --replay-check bench/replay/params.json
```

**Promoção/rollback:** alterar `active` e os defaults juntos num PR com a saída do replay; rollback é `git revert` desse PR. Nada é ajustado em holdout.

## 4. Latência por estágio — `sidecar-eval --perf N`

Mede o pipeline em processo (V1 e V2) por estágio de `Trace.Timings` (`analyze`, `lexical`, `exact`, `structural`, `contract_pack`, `fusion`, `total`), separando **cold** (primeira chamada por repositório+engine) de **warm**, com p50/p95/p99, máximo e contagem de amostras. Não inclui processo do hook, IO JSON nem cliente; `hook_probe_ms` (real_hook_pilot) é outra medida e não se mistura.

## 5. Scorecard — `lab.py report`

Seções **separadas** por nível de evidência (`agentic`, `simulated`, `offline_retrieval`), por braço, split e classe: taxa de aprovação, violações de escopo/invariantes, erros de infra, custo e tempo por tarefa correta (só quando todo custo comparável é conhecido), chamadas, caracteres injetados, modelos servidos e comparação pareada com OFF com IC bootstrap só com ≥ 5 pares (`inconclusivo` abaixo de 20). Retrieval offline nunca é somado a resultado agentic.

## 5b. Isolamento do agente em relação ao gabarito (corrigido após revisão)

Falha encontrada em 2026-11-08, **depois** dos primeiros pilotos: o agente roda como o mesmo usuário e `Read` aceita caminho absoluto, então ele podia **ler o oráculo oculto e o `reference.patch`** (reproduzido: sem regra, a leitura funciona). Os pilotos `lab-dev1` e o piloto R3 foram executados **antes** da correção e sem traces de ferramentas: não há como provar que nenhum agente leu o gabarito. Tratar seus números como **possivelmente contaminados** (a probabilidade é baixa, porque os prompts não citam o caminho, mas não está medida).

O harness atual exige Linux com `bwrap` para o agente real e falha antes de uma execução paga se o preflight falhar. A raiz do agente expõe o checkout descartável, runtimes de sistema, configuração e estado temporários e, nos braços habilitados, o binário Sidecar. O PATH é filtrado; o repositório do benchmark e o diretório de resultados não são montados. Um teste de processo verifica acesso negado a um arquivo externo e ausência de Sidecar/CodeGraph no PATH. O oráculo mantém isolamento de rede separado. A integração com o cliente real ainda precisa de validação em ambiente com Claude CLI; os pilotos anteriores continuam com seu risco histórico de contaminação.

Outras correções da mesma revisão: timeout agora fica nas taxas como falha (antes sumia do `pass_rate` e do custo); custo desconhecido conta como o teto por execução no limite de gasto; `*` de glob não cruza `/`; `regex_absent` falha se o arquivo foi apagado; o holdout só pode ser avaliado uma vez (`--allow-repeat-final`); `validate-task` não consome o holdout porque nenhum resultado de agente é observado; `--replay` exclui o holdout (`--include-holdout` para um relatório final).

## 5c. Protocolo de duas voltas e corpus v2

`lab.py run --turns 2` retoma a sessão (`--resume`). Em tarefa ambígua, a resposta predefinida só é enviada após uma pergunta relevante, observada antes de editar e com sessão disponível; caso contrário usa a revisão neutra. A pergunta em si não é gravada no registro. Métricas ausentes em uma volta tornam o total desconhecido. A trace correlaciona resultados com chamadas e só classifica ações após uma resposta de hook que entregou contexto.

## 5d. Auditoria de integração real do P0-A (2026-11-09, sem custo)

Executada com `bwrap` 0.9.0 e o binário nativo `claude` 2.1.319 deste ambiente, **sem credenciais e sem prompt ao modelo** (nenhuma chamada paga é possível). O que foi exercido de verdade e o que **não** foi:

| Item | Resultado |
|---|---|
| `claude` dentro do sandbox, indexação e hook do Sidecar no filesystem isolado | **Verificado**: `claude --version` roda; `sidecar index` leva ~60–80 ms; o hook `UserPromptSubmit` dispara dentro do sandbox e injeta contexto (277 caracteres na sonda); o braço OFF não tem hook |
| Falha de autenticação | **Verificado**: `Not logged in` vira `exit=1`, `stream_error`, execução não comparável; não conta como sucesso |
| Suítes de teste do projeto no sandbox | **Falhavam em 3 de 4 linguagens, corrigido**: Click (`pip install -e .` grava o caminho absoluto do host no venv), Express (`npm` montado como arquivo único) e Sinatra (`bundle` com shebang para outra árvore de Ruby). Após a correção, no sandbox: chi (`go test`), Click (109 passed), Express (1290 passing), Sinatra (273 runs) |
| Controle OFF e caminhos absolutos | **Falhava, corrigido**: com `PATH` filtrado, `/usr/local/bin/sidecar`, `/usr/local/bin/codegraph` e um pacote `@colbymchenry/codegraph` global continuavam executáveis por caminho absoluto. Agora são mascarados por mount (`forbidden_tool_masks`) |
| Gabarito via código executado | Oráculo e `reference.patch` **não** estão montados: `cat`, `python` e `ls` por caminho absoluto falham com ENOENT. **Canal residual**: o corpus também existe num repositório público do GitHub e o sandbox precisa de rede para a API; um teste escrito pelo agente poderia baixá-lo. O harness **não bloqueia** esse canal; marca a execução como não comparável se o agente ou o código que ele escreveu menciona `dedeusgui/sidecar` ou `bench/lab/corpus` |
| Subagentes e chamadas paralelas | **Corrigido**: `message_id` agora é o id da mensagem da API (as linhas do stream de uma mesma mensagem compartilham `message.id`, não `uuid`), então pergunta e `Edit` emitidos juntos não contam como "perguntou antes de editar". O contexto do hook só é atribuído à thread principal: ações de subagente recebem `context_scope=subagent` e `after_context=false` |
| **Não verificado** | volta autenticada, `--resume` da segunda volta, contadores reais de custo/cache do `result`, `AskUserQuestion` no modo headless, modelo servido real. Exigem credencial e orçamento autorizado |

Mudanças de harness correspondentes: o checkout é montado também no caminho absoluto do host (`/work` continua existindo); runtimes fora de `/usr` montam a árvore de instalação inteira (`/opt/<x>`, rbenv, nvm, pyenv...) mais o interpretador do shebang, com wrapper `exec` em vez de symlink; `verify_agent_runtimes` roda um comando de fumaça por linguagem no sandbox real **antes de qualquer gasto** e recusa a execução se algum falhar; `--agent-network-env` (opt-in) repassa proxy e CA, nunca credenciais de modelo.

## 6. Lacunas conhecidas (R1)

- A lista padrão de ferramentas inclui runners por linguagem do repositório, igual entre braços e gravada nos registros; `--tools-allow` substitui essa lista. A disponibilidade dos runtimes deve ser conferida no ambiente de cada experimento.
- ROI de feedback (`OFF` / `ON-no-feedback` / `ON-historical-feedback`): o feedback é indexado por raiz do repositório; o protocolo exige workspace de caminho estável por repositório e semeadura por fase de histórico. **Não implementado**; não declarado.
- O cliente precisa de rede para a API; o isolamento de rede do oráculo e o isolamento de filesystem do agente são registrados separadamente. A execução real com o sandbox novo ainda não foi exercida nesta sessão.
- Amostras `cold` são poucas (uma por repositório+engine por execução do perf); aumentar exige reabrir índices.
