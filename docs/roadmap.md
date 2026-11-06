# Roadmap verificável — Sidecar

**Atualizado: 2026-11-06.** Este é um plano de execução; itens propostos **não** estão implementados só por constarem aqui. Prioridades: validade da integração → ferramenta reproduzível → evidência de valor → ampliação de cobertura. Não adicionar features para mascarar resultados ruins.

## Estado observado em main (`f5820b1`)

- **Existe:** CLI em Go, indexação, SQLite/FTS5, extração para linguagens indicadas no README, pipeline lexical/grafo/Git, hooks Claude/Codex, install/uninstall, doctor/explain, testes, benchmark de fixtures, CI cross-platform, GoReleaser.
- **Evidência limitada:** README relata E2E Claude Code; Codex ainda não verificado via CLI real. Dados de `docs/benchmark.md` mostram que grafo/Git ainda não provam ganho versus BM25 com tradução. Não há benchmark end-to-end de tempo/rounds de agente.
- **Distribuição:** GoReleaser configurado, **sem release publicada** ao avaliar. Licença não definida.
- **Governança:** a branch padrão do GitHub era `ccr-b6db8870-kl90n6`, enquanto `main` estava um commit à frente; atualizar configuração do GitHub separadamente. Evitar links sem `/blob/main/` até ajuste.

## M0 — Base de engenharia e instalação [P0]

**Objetivo:** qualquer contribuinte consegue inspecionar, compilar e validar, e um usuário consegue instalar com rollback sem conhecimento do código.

| Trabalho | Entrega / aceite |
|---|---|
| Guia de agentes + desenvolvimento + docs navegáveis | `AGENTS.md`, `CLAUDE.md`, `docs/development.md` e links no README; documentação atualizada no PR |
| Corrigir default branch e definir licença | `main` é padrão; licença escolhida pelo mantenedor e refletida nos releases |
| Documentar instalação e atualização | `docs/release.md` com matriz Linux/macOS/Windows, PATH, consentimento, backup, rollback |
| Testar smoke em binário real | install / doctor / prompt / reinstall / uninstall e arquivos do usuário intactos; Windows incluído |
| Publicar release de desenvolvimento | tag, artefatos, checksums e verificação de download; não publicar antes dos gates anteriores |

**Gates:** testes + lint + cross-build verdes; nenhum overwrite de hook/binário alheio; documentação sem afirmação de release inexistente.

## M1 — Harness leve e dogfooding [P0/P1]

**Objetivo:** Sidecar ajuda a desenvolver o próprio Sidecar sem criar uma ferramenta maior que o produto.

| Trabalho | Entrega / aceite |
|---|---|
| Contrato agentic E2E | instalação e contexto real testados em Claude e Codex; capturar versões, aprovação do Codex e evidência de injeção |
| Dogfooding repetível | 4+ queries pinadas do próprio repo, traces redigidos, regressões por símbolo e baseline |
| Reporter de benchmark offline | ler resultado `sidecar-eval`, diferenciar commits/variantes e apontar regressões por caso; sem custo de modelos |
| CI de qualidade | teste, lint e benchmark offline na mesma pipeline; não bloquear por ruído de tempo em runner compartilhado |
| Testes de risco | corrupção/reconciliação de índice, indexadores concorrentes, paths sensíveis, instalação/upgrade, falha de hook |

**Gates:** relatório compara antes/depois com entradas e versões; nenhum benchmark paga tokens no PR; docs sincronizadas.

## M1.5 — Context Engine v2 incremental [P1]

**Objetivo:** nova estratégia orientada à intenção sem substituir de imediato BM25 + tradução; benchmark e rollout por feature flag. Especificação: [context-engine-v2.md](context-engine-v2.md).

| Trabalho | Entrega / aceite |
|---|---|
| Instrumentar pipeline v1 | Trace de candidates/ablation, decisões de supressão, ids de evidência, orçamento e timings **sem mudar ranking** |
| IntentRouter determinístico | `locate/impact/debug/extend/test/explain/general`, explicável, fallback ao v1; medir por intenção com labels públicos |
| ContextPacker sob budget | Cobertura incremental/novidade/redundância e gate de confiança; medir evidência correta por token, falsos positivos e latência |
| SessionSignals local e opt-in | IDs de evidência/arquivos já consultados, TTL/invalidação, sem gravação de conversa; adaptar sem forçar modelos |
| Ablação e rollback | Comparação OFF vs v1 vs v2 e por componente; sem ganho replicado, não promover default |

**Gate:** resultado offline reproduzível em corpora públicos pinados, sem regressões graves e com métricas de overhead; `AGENTS.md` e docs mantidos no PR.

## M2 — Benchmark de resultado real e econômico [P1]

**Objetivo:** medir se a injeção reduz *tempo para evidência*, rounds e buscas sem diminuir qualidade.

| Trabalho | Entrega / aceite |
|---|---|
| Corpus público pinado como base | Agent Retrieval Bench (427 casos/25 repos, inclui no-gold) primeiro; ContextBench (1.136 tarefas/66 repos) depois; reusar labels sem reanotar; licença, SHA, manifests e blind-test isolado |
| Instrumentação mínima | braços OFF / ON-v1 / ON-v2, mesmo modelo/harness/permissões; TTFE apresentado vs utilizado, tool calls, rounds, sucesso, tokens, custo, overhead e eventos não observados |
| Piloto manual | **24 execuções**, 4 tarefas × 3 modelos (Haiku/Luna/Flash) × 2 braços; diagnosticar protocolo, não alegar ganho |
| Amostra adicional | repetições randomizadas, intervalos de confiança por tarefa, casos com efeito adverso reportados |
| Comparação de métodos | lexical vs tradução vs grafo vs Git vs intenção vs packer vs LSP; preservar capacidades de ferramentas do agente nos dois braços |
| Qualidade downstream | bugs/fixes pequenos com testes oracle; separar “achou código” de “resolveu corretamente” |

**Gates:** custos/quotas e timeout registrados, sem contaminação de holdout; resultados incluem negativos, falhas e informação não observada. Repetibilidade **determinística apenas no nível offline**; agentes exigem repetição e incerteza. Aceitamos conclusão **inconclusiva**. Protocolo: `docs/evaluation-plan.md` e `docs/agent-feedback.md`.

## M3 — Universalidade por fallback e integrações [P1/P2]

**Objetivo:** funcionar sem exigir que repositórios se adaptem.

| Trabalho | Entrega / aceite |
|---|---|
| Matriz por linguagem/escala | publicar suporte efetivo: parser, lexical fallback, caminhos, repo sem Git, tamanho e limitações |
| Cobertura desconhecida | testar Rust, PHP, arquivos gerados, monorepos, symlinks, paths Unicode e projetos não convencionais; nunca inventar símbolos |
| LSP interoperável e opcional | detectar linguagem/servidor confiável; aproveitar plugin nativo do Claude; cliente LSP próprio somente após benefício medido; setup com consentimento, fallback e rollback; ver `docs/lsp-strategy.md` |
| Visibilidade opcional | API local de atividade e status line/mod Claude Code **opt-in**, coexistente e reversível |
| Avaliar plugins/mods | Claude Code já documenta mods em JS/TS e LSP plugins oficiais; usar extensão opt-in com E2E sem modificar o fluxo core |
| Distribuição conveniente | script ou gerenciador de pacotes somente depois do contrato de update/uninstall; npm apenas se houver benefício concreto |

**Gates:** nenhum requer config de projeto; fallback e falhas não bloqueiam agente; UI não vaza prompt ou código.

## M4 — Evolução guiada por dados [P2; condicional]

- Melhorar léxico para queries sem sobreposição; medir ganhos em `blind-test`.
- Experimentar semântica/embeddings **locais e opcionais** apenas se recuperarem casos relevantes sem custo excessivo.
- Podar sinais de grafo/Git se aumentarem tokens/latência sem alterar qualidade.
- Expandir agentes por adaptador com contrato e E2E individual.

## Ordem de execução sugerida

1. Concluir documentação e alinhar GitHub default branch.
2. Reproduzir instalação/upgrade/E2E; publicar primeiro release de desenvolvimento.
3. Instrumentar benchmark sem LLM e dogfood no próprio repo.
4. Preparar corpus e gold independente; executar piloto Haiku/Luna/Flash com teto.
5. Rodar ARB/ContextBench offline em subset fixo; habilitar roteamento por intenção e packing por feature flag com ablações.
6. Instrumentar hooks de ferramenta em modo observação, com feedback opcional pós-tarefa (sem autoajuste por autorrelato).
7. Avaliar LSP existente e suporte Claude; só depois implementar cliente/instalador LSP opt-in.
8. Rodar pilotos agentic Haiku/Luna/Flash com teto e repetir pares; adicionar UI/mod opcional depois de comprovado o valor.

**Definição de pronto para qualquer PR:** escopo pequeno, testes apropriados realmente executados, impacto documentado, risco de privacidade/cross-platform tratado e `docs/` atualizadas (ou justificativa objetiva).
