# Sidecar Benchmark Observatory — painel HTML e base de evidências

> **Estado: especificação de produto, não dashboard implementado.** O objetivo é tornar comparações entre engines e concorrentes visíveis, auditáveis e de execução barata. Os relatórios atuais permanecem fonte até a ingestão automatizada. Não apresentar um gráfico ilustrativo como medição.

## Objetivo

O Observatory deverá responder: **o Sidecar ajuda o agente a concluir tarefas corretamente, mais rápido e com menor custo do que estar sem Sidecar e do que as alternativas?** Além disso, **qual fase perdeu informação ou gastou recursos?**

Aspiramos superar todos os concorrentes relevantes nos benchmarks econômicos que pudermos executar. Comparações devem ser justas, com as mesmas oportunidades e permissões; superar um baseline de recuperação não significa superar um agente real. Conservar as derrotas e os resultados inconclusivos, e jamais otimizar contra um holdout já aberto.

## Fluxo de dados

```text
bench manifests + corpus por commit + runner/adapter versões
       |
       +-- testes unitários / offline (CI, gratuitos)
       +-- hooks reais E2E (local/CI de integração quando disponível)
       +-- agentic piloto pago (opt-in, teto e consentimento)
       +-- concorrentes pinados (mesmo escopo, licença e superfície)
       |
       v
run.json (schema e proveniência) + cases.jsonl + raw references (quando seguros)
       |
       v
validate / normalize / aggregate / compare (sem inventar valores)
       |
       +-- relatório Markdown (histórico e CI)
       +-- relatório HTML estático, local e offline
       +-- JSON machine-readable / artefatos de CI
```

O HTML deve ser gerado a partir de dados versionados e revisados, nunca de números copiados manualmente. Sem dependência de APIs externas para visualizar. Não incluir prompts, código do repositório, tokens secretos ou arquivos privados no HTML público.

## Contrato mínimo de um run

- `run_id`, `recorded_at`, `git_sha`, `engine_version`, `sidecar_binary_sha256`, `runner_sha256`, `benchmark_id`, `benchmark_version` e `case_manifest_sha256`.
- `level`: `infra`, `offline-retrieval`, `offline-delivery`, `integration`, `agentic`. Nunca somar métricas entre níveis incompatíveis.
- `arm`: `OFF`, `sidecar:vX`, `competitor:<name>@<ref>`; registrar configuração, feature flags, cliente/modelo, ferramentas disponíveis e transporte (hook real, simulação ou prompt prefix).
- Contexto: repositório/commit/licença, linguagem, split `development|validation|holdout`, família de tarefas, critérios de exclusão, orçamento, cold/warm, isolamento, ambiente e limites.
- Métricas por caso (com denominadores): gold recuperado, gold **entregue**, abstenção, erro de evidência, sucesso de tarefa/oráculo, escopo/invariantes, tool calls, buscas/releituras redundantes, tokens observados versus estimados, tempo de indexação/hook/agente, memória e custo **observado**. Ausente = `null/unknown`, nunca 0.
- Proveniência: caminho de artefato, hash, status de comparabilidade, contaminação do corpus, validação do oráculo, falhas de execução, notas e intervalo de confiança quando aplicável.

**Evitar métrica única de ranking geral:** mostram-se dimensões, trade-offs e uma decisão de promoção que depende de gates pré-registrados.

## Vistas obrigatórias do HTML

1. **Overview**: versão, referência, abrangência de testes, status dos gates, últimos resultados e nível de confiança.
2. **Timeline 1 → 4**: milestones, funcionalidade introduzida, ganhos/perdas comprovados, documentação e experimentos pendentes.
3. **Engine vs Engine**: linhas de tendência e comparação pareada V1/V2/V3/V3.5/V3.6/... por classe/repo/idioma. Diferenciar offline e agentic.
4. **Competitive Arena**: Sidecar, OFF e alternativas como CodeGraph/Aider RepoMap; benchmark e orçamento equivalentes, pin de concorrente, disponibilidade de ferramentas, custo/resultado correto, IC/limites. Não ocultar concorrentes sem dados: mostrar `não medido`.
5. **Delivery Pipeline**: candidato → evidência registrada → planejada → alocada → empacotada → entregue; taxas de perda e razões de descarte, casos com snippet sem fallback.
6. **Failures & Deltas**: matrizes de regressão por caso, negativos, falso positivo, arquivo stale, confiança e contratos, com links para reprodução.
7. **Cost & Performance**: p50/p95/p99 de hook e sync, frio/quente, tokens, leitura de cache, custo por tarefa correta e sobrecarga de índice.
8. **Sources / Run Inspector**: selecionar qualquer ponto e abrir ambiente, SHA, runner, manifesto, hipótese, critérios e artefatos; mostrar o que foi efetivamente observado.

Filtros por engine, baseline, commit, runner, benchmark, categoria, language, repo, nível de evidência, modelo, data, transport e cold/warm; valores sem evidência devem permanecer explicitamente vazios.

## Pipeline de testes barato

**P0 — em todo PR, automático e sem LLM:** testes Go e Python, segurança, determinismo, compilação, hooks simulados e invariantes de packing. Falha bloqueia merge.

**P1 — offline, por mudança de engine e execução agendada:** scorecard de entrega e ablações em corpus pinado; negativos, casos difíceis e pequenos repositórios externos. Fazer cache dos checkouts com hash, sem baixar corpus grande a cada CI. Gerar HTML e delta contra baseline com defeitos por caso.

**P2 — cliente real, teste de integração:** prova de que hook instalado disparou e entregou byte correto dentro de Claude/Codex, conforme suporte, sem exigir tarefa longa paga. Marcar capacidades não verificadas.

**P3 — agentic econômico, só quando necessário:** piloto discriminativo pequeno com oráculos e comparabilidade previamente validados; ordem balanceada, pareamento, custos reais e teto; ampliar somente se houver sinal; autorização explícita. Medir sucesso/custo, não só localização ou contexto. Uma campanha incompleta deve dizer *incompleta*, nunca sucesso.

## Critérios de qualidade do Observatory

- Os números do HTML reconciliam com JSON e Markdown do mesmo run; teste automatizado de totais, denominadores e nulos.
- Resultado com falha de runner ou modelo indisponível não entra na média; exclusões continuam visíveis.
- Nunca comparar latência de máquinas/caches diferentes como ganho causal.
- Corrigir e registrar corpus contaminado (holdout negativo já exposto; não reutilizar como cego).
- Atualizar o painel em cada PR/milestone sem rodar benchmarks pagos.
- Preservar snapshots versionados e links para execução; link quebrado deve aparecer como dado indisponível.
- Qualquer claims de vitória deve apontar para a configuração equivalente, método e incerteza. Separar benchmark feito do benchmark desejado.

## Primeira slice de implementação recomendada

Conectar o registro existente (`bench/registry.json`, `bench/consolidate.py`, `docs/benchmarks.md`) e o avaliador de entrega (`cmd/sidecar-eval --delivery`) a um exportador canônico de resultados. Gerar um **HTML estático com Overview, Engine Comparison, Failures e Sources** a partir de artefatos já existentes, com destaque das ressalvas. Adicionar as outras vistas em incrementos pequenos. Não mudar ranking da engine nesta slice.

Priorizar HTML local sem serviços ou modelos, com fallback acessível sem JavaScript e exportação CSV/JSON quando houver dados. Não prometer um dashboard concluído antes de existir arquivo, build e verificação visual.

Referências: [roadmap de versões](product-milestones.md), [benchmarks consolidados](benchmarks.md), [progresso V3.6](v3.6/progress.md), [evaluation plan](evaluation-plan.md), [pesquisa V3.7](v3.7/research-plan.md).
