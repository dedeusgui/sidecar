# Sidecar V3.5 — progresso de implementação

> Estado: **implementada, opt-in, gates de avaliação pendentes.** Referências `arquivo:linha` são do commit da branch `feat/v3.5-unified-engine` na data desta escrita; revalide ao mudar o código. Experimentos posteriores com agentes foram executados, sem comprovação de ganho generalizável; veja a campanha abaixo. Visão e contratos: [README](README.md), [architecture.md](architecture.md), [implementation-plan.md](implementation-plan.md), [evaluation.md](evaluation.md).

## Entregue por componente

| Componente | Onde |
|---|---|
| Contratos (Scope, TaskContext, EvidenceRef/Registry, GuidanceProposal, OutputPlan, VerificationProof) | `internal/engine/contract/types.go:66,95,184,206,238,260` |
| Planner determinístico | `internal/engine/planner.go:27` (`Planner.Plan`) |
| Packer global sob um orçamento | `internal/ctxbuild/packer.go:44` (`Pack`) |
| MicroBrief (orientação curta junto do código) | `internal/brief/task.go:209` (`MicroBriefProposal`) |
| QuickExpand (limites: 2 arquivos, 512 tokens, 500 ms) e DeepExplore (6 arquivos, 4000 tokens, 2 s), 1 execução concorrente | `internal/explore/service/service.go:21-36,77,147` |
| Expansão de cadeia de chamadas limitada no Explore | `internal/explore/core/core.go:458` (`expandCallChain`) |
| Propostas Groom / Impact / Memory / Session | `internal/groom/groom_proposals.go:33`, `internal/app/groom_proposals.go:60`, `internal/impact/impact_proposals.go:32`, `internal/app/memory_proposals.go:29`, `internal/brief/session_proposals.go:37` |
| Fluxo único (fail-open: erro vira contexto vazio) | `internal/app/v35.go:37` (`runV35Safe`), `:48` (`runV35`); chamada pelo hook em `internal/app/app.go:174-200` |
| Matriz de capacidades por cliente | `internal/adapter/adapter.go:47` (`CapabilitiesFor`); `ExploreCapability` segue `unverified` para clientes sem caminho nativo testado |
| Coortes congeladas E1 (30 positivos + 40 negativos, PT-BR/EN) e E2 (20 navegação) | `bench/v3.5/` (`freeze_manifest.json`, `freeze.py`) |
| `sidecar-eval` omite holdout por padrão; `--include-holdout` só para relatório final | `cmd/sidecar-eval/main.go:115,164` |

Também no ramo: ranking com grafo preservando a ordem lexical (commit `0a7b179`) e normalização de âncoras de transcrição por voz em `internal/lexical` (commit `670f627`).

## Ativação

Opt-in; o default continua V1. Mecanismo real:

- Hook: variável de ambiente `SIDECAR_ENGINE=v3.5` (`internal/app/app.go:285`). Qualquer outro valor desconhecido ou ausente mantém V1.
- CLI: `sidecar explain --engine v3.5` (`cmd/sidecar/main.go:100,306-311`); o default do flag vem de `SIDECAR_ENGINE` (`main.go:293`).
- Desativar: remover `SIDECAR_ENGINE` ou usar `v1`/`v2`/`v3`. Não existe outra flag.

## Limitações conhecidas

- Coortes E1/E2 são **author-coupled** (prompts e gold escritos pelo mesmo autor que implementou) e foram executadas posteriormente contra V3 e V3.5; revisão semântica independente pendente.
- Capacidade de Explore nativo por cliente não verificada (`unverified`).
- Validação offline local (2026-11-09, fixture, sem agente): testes E2E do hook para Claude Code e Codex (injeção, Explore embutido, sessão em várias voltas, abstenção, saída idêntica com V3.5 desligada, readiness em `doctor`/`status`) em `internal/app/v35_test.go` e `e2e_cli_test.go`; `go test -count=1 ./...` e `-race` nos pacotes concorrentes passam. Latência do hook em `BenchmarkHookPathLatency` (p50, 20 iterações, uma máquina): quente V2 11,7 ms, V3 13,0 ms, V3.5 17,8 ms (p95 20,9); frio V2 39,3 ms, V3 40,0 ms, V3.5 45,1 ms. **Na data dessa validação não havia sido feita** comparação de V1/V2/V3 contra um binário da `main` nem E2E com cliente real; as referências `arquivo:linha` deste documento não foram reconferidas após os últimos commits.
- O packer e o planner usam heurísticas determinísticas sem modelo; a campanha posterior investigou prompts curtos nas coortes novas e encontrou perda de entrega de caminhos na V3.5.

## Resultados offline obtidos (e ressalvas)

| Resultado | Números | Ressalva |
|---|---|---|
| Ranking com grafo | tune Hit@5 13/20, MRR .483→.553; eval 15/20→16/20, MRR .637→.683; latência total P50 8,40→2,77 ms | Fixtures internas (portão mínimo); latência de amostra local única, não é benchmark de máquina. |
| Tradução / Voice Mode | tune 13/20→20/20 | Medido no **mesmo split usado para ajustar**; eval não reportado depois. **Não confirmado.** |
| Explore | development 10/14→11/14 fatos; 23.308→29.071 caracteres | Mais fatos custou mais caracteres. Alvo 24/26 (CodeGraph) **não verificado**. |

## Campanha posterior e lacunas

A [campanha de 2026-11-10](../../bench/results/v35-campaign-2026-11-10/relatorio.md) executou E1/E2 contra V3 e V3.5. Em E1, com cache quente e `explain`, o caminho esperado apareceu em 21/30 e 2/30 positivos, respectivamente; houve saída em 19/40 e 13/40 negativos. A chamada direta ao hook com payload Codex não equivale ao cliente Codex real; E2 usa comparação literal sem avaliação de equivalência semântica. Essas coortes são de autoria acoplada, e a latência não foi controlada.

Em localização com Claude, OFF e V3.5 acertaram 49/50 cada, com uma execução por braço; ali o contexto foi inserido como prefixo, não pelo hook real. Em nove tarefas de edição/validação com hook real, o veredito estrito foi OFF 9/9, V2 9/9 e V3.5 7/9; as duas falhas envolveram restrição de arquivos e uma também uma verificação textual de invariante. Amostra pequena, uma repetição e limitações do oráculo não estabelecem causalidade nem ganho generalizável.

Permanecem pendentes validação independente, comparação controlada e repetida de custo/tempo por tarefa correta, cobertura de clientes reais e avaliação final cega sob o protocolo aplicável. Não confundir os dados já observados com holdout cego, nem interpretar essas medições como aceleração comprovada do agente.

## Proveniência (reuso de código aberto)

| Upstream | SHA | Licença | Uso |
|---|---|---|---|
| CodeGraph `colbymchenry/codegraph` v1.6.2 | `6560052a6f856855d3f71eee838fd66ccfa4285d` | MIT | Estudo de mecanismo (expansão de relações/cadeia de chamadas) |
| Aider `Aider-AI/aider` | `5dc9490bb35f9729ef2c95d00a19ccd30c26339c` | Apache-2.0 | Estudo de mecanismo (mapa de repositório/ranking por grafo) |

- Decisão: **implementação independente em Go, sem cópia de código**. Não é clean room: os mecanismos foram estudados antes de implementar.
- Adotado: expansão de cadeia de chamadas limitada no Explore (`core.go:458`), sob limites explícitos de arquivos, tokens e tempo.
- Apenas candidato (não adotado): demais ideias dos dois projetos, incluindo ranking global por grafo estilo Aider além do já feito, e qualquer integração de biblioteca ou serviço.
- Justificativa: preserva execução local sem modelo/rede e sem dependência nova de runtime.
