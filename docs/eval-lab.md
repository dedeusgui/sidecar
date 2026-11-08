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

```bash
python3 bench/lab/lab.py validate-corpus                     # prova fail-before/pass-after de cada tarefa
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

Correção (`ClaudeAgent`): regras `--disallowedTools` para `Read/Grep/Glob/Edit/Write` no repositório e no diretório de saída (verificado: Read, Grep e até `ls` no caminho são negados); detecção pós-execução: qualquer evento de ferramenta que cite dados do benchmark (mesmo negado), e qualquer arquivo alterado pelo agente que os referencie, tornam a execução **não comparável**; a linha de base do `setup` fica fora do workspace; `oracle.files` não pode sair de `oracle/` nem escrever em `.git`.

**Risco residual, sem sandbox de SO (sem `bwrap` aqui):** código que o agente escreve e roda com `go test` lê qualquer arquivo do usuário. A detecção por varredura do workspace pega o caso comum, não um agente que monte o caminho em tempo de execução. Isolamento forte exige contêiner/usuário separado.

Outras correções da mesma revisão: timeout agora fica nas taxas como falha (antes sumia do `pass_rate` e do custo); custo desconhecido conta como o teto por execução no limite de gasto; `*` de glob não cruza `/`; `regex_absent` falha se o arquivo foi apagado; o holdout só pode ser avaliado uma vez (`--allow-repeat-final`); `validate-task` não consome o holdout porque nenhum resultado de agente é observado; `--replay` exclui o holdout (`--include-holdout` para um relatório final).

## 5c. Protocolo de duas voltas e corpus v2

`lab.py run --turns 2` retoma a sessão do cliente (`--resume`) e envia a fala de um usuário simulado, **definida pela tarefa e igual para todos os braços**: tarefa ambígua recebe `underspec.canned_answer`; as demais, uma revisão genérica (é nela que há edições não commitadas para a nota de impacto). A execução soma as duas voltas; qualquer volta contaminada ou sem modelo servido invalida tudo. Braços de funcionalidade: `V2+brief|adaptive|groom|impact`. O corpus v2 (12 tarefas, escopo com testes permitidos por glob, oráculos de 6 a 13 casos de borda) está validado mas **não foi medido com agente**.

## 6. Lacunas conhecidas (R1)

- `--tools-allow` padrão libera só `go test`/`ls`: agentes de click/express/sinatra não conseguem rodar a suíte própria da linguagem. Igual nos três braços, mas limita a qualidade absoluta.
- ROI de feedback (`OFF` / `ON-no-feedback` / `ON-historical-feedback`): o feedback é indexado por raiz do repositório; o protocolo exige workspace de caminho estável por repositório e semeadura por fase de histórico. **Não implementado**; não declarado.
- Isolamento de rede do cliente do agente: o oráculo roda sem rede, mas o `claude` precisa de rede para a API; não há `bwrap` neste ambiente cloud, então a proteção de escrita fora do workspace depende do `acceptEdits` e de `--allowedTools` restritos, não de um sandbox de SO.
- Amostras `cold` são poucas (uma por repositório+engine por execução do perf); aumentar exige reabrir índices.
