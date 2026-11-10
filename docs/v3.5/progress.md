# Sidecar V3.5 — progresso de implementação

> Estado: **implementada, opt-in, gates de avaliação pendentes.** Referências `arquivo:linha` são do commit da branch `feat/v3.5-unified-engine` na data desta escrita; revalide ao mudar o código. Nenhum resultado end-to-end com agentes foi medido. Visão e contratos: [README](README.md), [architecture.md](architecture.md), [implementation-plan.md](implementation-plan.md), [evaluation.md](evaluation.md).

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

- Coortes E1/E2 são **author-coupled** (prompts e gold escritos pelo mesmo autor que implementou) e **ainda não executadas** contra a V3.5; revisão semântica independente pendente.
- Capacidade de Explore nativo por cliente não verificada (`unverified`).
- Validação E2E por cliente, latência do fluxo completo e readiness em `doctor`/`status`: **pendente de validação** (trabalho em andamento fora desta documentação).
- O packer e o planner usam heurísticas determinísticas sem modelo; regressões em prompts curtos de domínio documentadas na V3 não foram reavaliadas nas coortes novas.

## Resultados offline obtidos (e ressalvas)

| Resultado | Números | Ressalva |
|---|---|---|
| Ranking com grafo | tune Hit@5 13/20, MRR .483→.553; eval 15/20→16/20, MRR .637→.683; latência total P50 8,40→2,77 ms | Fixtures internas (portão mínimo); latência de amostra local única, não é benchmark de máquina. |
| Tradução / Voice Mode | tune 13/20→20/20 | Medido no **mesmo split usado para ajustar**; eval não reportado depois. **Não confirmado.** |
| Explore | development 10/14→11/14 fatos; 23.308→29.071 caracteres | Mais fatos custou mais caracteres. Alvo 24/26 (CodeGraph) **não verificado**. |

## O que NÃO foi medido

Ganho agentic (tempo/custo/correção por tarefa) com Sidecar on vs off; holdout (nenhum aberto); coortes E1/E2 de `bench/v3.5`; tokens e latência do hook V3.5 ponta a ponta; abstinência e E2E em Codex/Antigravity. Nada acima deve ser lido como aceleração do agente.

## Proveniência (reuso de código aberto)

| Upstream | SHA | Licença | Uso |
|---|---|---|---|
| CodeGraph `colbymchenry/codegraph` v1.6.2 | `6560052a6f856855d3f71eee838fd66ccfa4285d` | MIT | Estudo de mecanismo (expansão de relações/cadeia de chamadas) |
| Aider `Aider-AI/aider` | `5dc9490bb35f9729ef2c95d00a19ccd30c26339c` | Apache-2.0 | Estudo de mecanismo (mapa de repositório/ranking por grafo) |

- Decisão: **implementação independente em Go, sem cópia de código**. Não é clean room: os mecanismos foram estudados antes de implementar.
- Adotado: expansão de cadeia de chamadas limitada no Explore (`core.go:458`), sob limites explícitos de arquivos, tokens e tempo.
- Apenas candidato (não adotado): demais ideias dos dois projetos, incluindo ranking global por grafo estilo Aider além do já feito, e qualquer integração de biblioteca ou serviço.
- Justificativa: preserva execução local sem modelo/rede e sem dependência nova de runtime.
