# Auditoria e estado do laboratório de avaliação (2026-11-08)

Base: `main` = `1ef08c729e2db170208347132453920c83a2df9f` (igual ao branch de trabalho no início). Trabalho desta rodada: branch `ccr-015682a3-u078zn`.

Este documento registra o que está implementado, o que é só simulação, o que foi testado com agente real, o que está desatualizado e o que segue bloqueado. Não declara conclusão de workstreams que não foram executados.

## 1. Linha de base

| Verificação | Antes das mudanças | Depois |
|---|---|---|
| `go test -count=1 ./...` | ok em todos os pacotes (EXIT 0) | ok (EXIT 0) |
| `go test -race` (store, index, app, gitx) | não executado nesta rodada | ok |
| `go vet ./...` | não executado | ok |
| `bench/agent` (unittest) | 22 ok | 24 ok (+2 de identidade de modelo) |
| `bench/agentic` (unittest) | 6 ok | 22 ok (6 existentes + 7 do piloto + 9 do diagnosticador) |
| `bench/test_scorecard.py` | 42 ok | 42 ok |

## 2. Inventário: implementado × simulado × testado com agente real

| Componente | Estado antes desta rodada | Evidência real com agente |
|---|---|---|
| Recuperação offline (`sidecar-eval`, 10 repositórios, voz) | implementada e testada; retrieval só | não se aplica |
| Simulação agêntica (`instrumented_sim.py`, política fixa) | simulação | não |
| Runner de agentes reais `run_agents.py` (OFF/ON por prefixo) | implementado, testado com registros sintéticos; smoke de 1 caso | smoke antigo, **com alias `haiku` que foi servido por Sonnet**; ver achado 1 |
| Hook real (`--real-hooks`) | **desativado** | não |
| Feedback pós-tarefa (`sidecar feedback`) | implementado com testes unitários | **nenhuma** |
| `SessionObserver` / `sidecar observe` | unitário e E2E do binário | **nenhuma**; custo não medido |
| Piloto E2E com hook real (Claude Code) | não existia | **sim, 20 execuções**: [agent-pilot-2026-11-08.md](agent-pilot-2026-11-08.md) |
| Codex e Antigravity | adaptadores com testes de parsing | não executados nesta rodada |
| CodeGraph e busca nativa | não executados | não |

## 3. Achados e correções

| # | Achado | Impacto | Estado |
|---|---|---|---|
| 1 | O alias `claude --model haiku` foi servido por `claude-sonnet-5-5`. A mesma chamada trivial custou US$ 0,163 com o alias e US$ 0,0083 com `claude-haiku-5-5`. | Os números de Claude de `handoff-agent-benchmark.md` (seções 3 e 6) **não são de Haiku**. O alias é um risco de custo e de validade. | **Corrigido:** `validate_model()` recusa aliases de Claude; o padrão é `claude-haiku-5-5`; o `modelUsage` é registrado por execução (`served_models`). Doc anotada. |
| 2 | `ComputeReconciledWeight` usava `task_completed` como oráculo quando `oracle_passed` faltava. Um autorrelato recebia o peso de 0,45 reservado ao resultado independente. | Feedback não verificado influenciava o ranking com o peso do oráculo. | **Corrigido** (commit `a805fdd`): `oracle_status` tri-state; `unknown` não contribui; `task_completed` vira `agent_reported_completion`. |
| 3 | O CLI `--completed` tinha padrão `false`, e um registro por flags sem oráculo contava como **falha** do oráculo. | Registros por flags eram penalizados sem evidência. | **Corrigido:** `--oracle` (padrão `unknown`) e `--oracle-provenance`. |
| 4 | Um item marcado como enganoso e nunca lido recebia −1 no rastro. | Ausência de leitura era tratada como prova de inutilidade, o que a especificação proíbe. | **Corrigido:** rastro sem observação não pesa; só o elogio sem rastro é descontado. |
| 5 | Todo feedback alterava o ranking, inclusive autorrelato sem edição. | Feedback fraco criava ajuste global sozinho. | **Corrigido:** níveis forte/médio/fraco; só forte (×1,0) e médio (×0,5) são aplicados. Fraco é armazenado e não altera o ranking. |
| 6 | Registros antigos com oráculo do autorrelato. | Dados anteriores podiam continuar influenciando o ranking. | **Migração v5→v6** (versionada, com teste): registros antigos viram `weak`, proveniência `legacy`, e deixam de ser aplicados. |
| 7 | `docs/agent-feedback.md` dizia "decisão em aberto" sobre o oráculo de autorrelato, e a fórmula estava desatualizada. | Documentação contradizia o código. | **Corrigido:** seção reescrita; `development.md` atualizado. |
| 8 | O braço ON do `run_agents.py` usa prefixo de prompt, não o hook do cliente. O caminho real estava atrás de `--real-hooks`, que não tem isolamento provado. | Ambiguidade sobre o que foi medido. | `--real-hooks` **continua desativado**. O hook real foi exercitado em `real_hook_pilot.py`, com entrega verificada no stream. |
| 9 | **Falha de abstenção:** em um caso sem evidência ("pão de fermentação natural"), o hook entregou 1.307 caracteres de contexto ruidoso (confiança 0,95), por casar a palavra comum "natural". | Custo de tokens e risco de seguir ruído. Não mudou o desfecho desta amostra. | **Aberto.** Diagnosticado e reproduzível; a correção mexe no retrieval e exige A/B com `sidecar-eval`. Ver [agent-pilot-2026-11-08.md](agent-pilot-2026-11-08.md). |
| 10 | As confianças de `explain --json` (0,95, 0,90) são constantes heurísticas, não calibradas. | Leitor pode interpretar como probabilidade. | Já documentado no código; **sem alteração** nesta rodada. |
| 11 | `internal/lexical/cognates.go` não está gofmt-limpo no HEAD. | Ruído de formatação. | **Não tocado** (fora do escopo). |

## 4. Estado das workstreams da missão

| Workstream | Estado nesta rodada |
|---|---|
| **A. Harness E2E** | Parcial. Caminho real com hook do Claude Code validado (20 execuções, entrega verificada). Isolamento **parcial**: configurações de usuário excluídas; plugins, skills e MCP globais não provados; `CLAUDE_CONFIG_DIR` não usado. `--real-hooks` segue bloqueado. Codex e Antigravity não executados. |
| **B. Corpus e oráculos** | **Não iniciado.** Requer repositórios externos pinados, oráculos independentes, splits e holdout congelado. Os 5 casos do piloto são de desenvolvimento e não substituem o corpus. |
| **C. Feedback confiável** | **Concluído na parte de correção.** Sinais separados, tiers, migração, testes de regressão para as três situações pedidas (feedback enganoso, evidência entregue sem leitura, evidência lida sem utilidade comprovada). `evidence_presented` **pendente**, pois exige registro de apresentação por sessão. |
| **D. Diagnosticador** | **Versão 0.** `bench/agentic/diagnose_runs.py` classifica causas candidatas (incluindo `false_positive_injection`) a partir de `runs.jsonl`. Não cobre `explain` por execução nem latência por fase. |
| **E. Replay e tuning** | **Não iniciado.** Não há replay offline das variantes de feedback nem braço `NoFeedback` exposto no `sidecar-eval`. |
| **F. ROI do feedback** | **Não executado.** Requer tarefas relacionadas e pares com feedback histórico, fora do escopo do piloto. |
| **G. Desempenho** | **Não iniciado.** Sem p50/p95/p99 por fase nesta rodada. |
| **H. Placar** | **Não consolidado.** Os números existentes continuam em relatórios separados. |

## 5. Critérios de aceitação da missão

| Critério | Situação |
|---|---|
| Testes existentes passam | **Sim** (Go e Python). |
| Feedback desconhecido não é confundido com oráculo aprovado | **Sim** (`unknown` = 0; testes). |
| Regressões: feedback enganoso, entrega sem leitura, leitura sem utilidade | **Sim** para as duas primeiras. A terceira está coberta pelo teste de tiers (leitura é `inferred`, peso ≤ 0,5). |
| Runners E2E com Git descartável e isolamento | **Parcial**: clone descartável e árvore verificada sim; isolamento global não provado. |
| Corpus ampliado com oráculos, hashes e splits | **Não** (bloqueado por escopo; ver B). |
| Diagnóstico reproduzível de falsos positivos e perdas | **Parcial**: diagnóstico de um caso real (abstenção) e de perda de gold; reprodução documentada. |
| Replay offline comparando variantes sem modelo | **Não**. |
| Tuning respeitando validação/holdout e rollback | **Não** (não houve tuning). |
| Scorecards com sucesso, custo, latência e falhas com denominadores | **Parcial**: piloto com denominadores (9/10, 10/10; Hit@1 com 8 casos). |
| Piloto E2E com Claude Code documentado | **Sim**, com limitações explícitas. |
| Testes de integração e regressão verdes | **Sim** (ver seção 1). |
| Documentação e roadmap refletem o estado real | **Parcial**: feedback, handoff e este documento atualizados; roadmap atualizado com o estado desta rodada. |

## 6. Custos e limites

- Gasto real medido nesta sessão: **≈ US$ 0,29** no total (smoke com alias que virou Sonnet, US$ 0,163; smoke com Haiku, US$ 0,008; sonda do hook, US$ 0,009; piloto, US$ 0,108). Teto da missão: US$ 3.
- O custo é o reportado pelo cliente (`total_cost_usd`), não cobrança.
- Sem benchmark pago em CI. `--real-hooks` segue desativado.
- **Não foram alteradas** configurações globais do usuário: a instalação de hooks em `~/.claude` **não** foi feita. Por isso o `AGENTS.md` pede `sidecar install` para autodogfooding e o registro com `sidecar feedback record` ao fim da tarefa **não foi executado**. Fica pendente com consentimento explícito.

## 7. Próximos experimentos, em ordem

1. Calibrar a abstenção para termos genéricos (caso `N01` e casos negativos adicionais em `bench/cases.json`), com A/B e ablação no `sidecar-eval` e holdout intocado.
2. Provar isolamento: `CLAUDE_CONFIG_DIR` com canário de configuração global e inventário de plugins e MCP no evento `init`.
3. Corpus externo com oráculo independente (`--external-cases`), prompts realistas e repetições suficientes.
4. Replay offline das variantes de feedback (tiers, fatores 1,0 e 0,5) sobre os registros de sessões, com a regra de rollback.
5. Medir feedback com tarefas relacionadas (workstream F) só depois dos itens 1 a 3.
