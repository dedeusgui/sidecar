# Roadmap de implementação — Sidecar V2 (produto completo)

**Atualizado em 2026-11-06.** Fonte de verdade para execução e aceitação do **produto inteiro**. Não é um MVP; trabalhos serão integrados em PRs incrementais com gates reais até que **todas** as capacidades tenham integração E2E validada. A especificação de destino está em [v2-product-spec.md](v2-product-spec.md); reuso/estudo obrigatório de soluções existentes em [open-source-references.md](open-source-references.md).

## North Star e limites

**North Star:** menor **custo e tempo por tarefa corretamente concluída** em Claude Code, Codex e (onde compatível) Gemini CLI, usando modelos econômicos (Haiku/Luna/Flash), com menos rounds, buscas redundantes, conflito de fontes e intervenção humana. Não vender “mais inteligência do modelo”. Medir impacto negativo, não apenas acertos.

**Produto esperado:** one-time install, sem config por repositório, execução local determinística para descoberta/seleção, fail-open, proteção de dados, compatibilidade de clientes por adaptador, LSP opcional assistido, observação de sessão, contratos e decisão, injeção concisa por contexto, diagnóstico, benchmark e distribuição com rollback.

**Regra de implementação:** não criar outro CodeGraph ou orquestrador: consulta upstream e aprova reuso/integração onde compensa; código protegido e licenças respeitados. Reaproveitar FTS5, Git, Tree-sitter, adapter e testes já escritos. Por PR, garantir teste/dogfood/documentação. **Técnica que piora resultado é isolada/desativada e substituída com evidência**, não mantida por apego ao design.

## Estado atual da base v1 (inspecionado na main em 2026-11-06)

| Base existente | Evidência de estado |
|---|---|
| Go CLI, `install/uninstall/status/doctor/explain`, SQLite FTS5/BM25, tradução PT/EN, AST, relações simples, Git, RRF, índice incremental | Código em `cmd/sidecar`, `internal/*` |
| Hooks Claude Code e Codex para `UserPromptSubmit` | Adaptador/instalador implementados; E2E Claude registrado, **E2E Codex real ainda pendente** |
| CI, GoReleaser, fixtures e `sidecar-eval` | Workflow Linux/macOS/Windows, cross-build e bench; resultado `docs/benchmark.md` não comprova ganho agentic |
| Engenharia e ADRs | `AGENTS.md`, `docs/development.md`, engine v2, LSP e feedback especificados |
| Ausências relevantes | Ainda sem IntentRouter V2, ContextPacker V2, SessionObserver multiponto, ContractResolver, DecisionMemory, cliente/setup LSP, agente Gemini, benchmark agentic comparativo e release validada |

**Pendência administrativa:** branch padrão do GitHub ainda era `ccr-b6db8870-kl90n6` no último check, embora as mudanças estejam em `main`. Alterar via configuração do repositório. Licença do próprio Sidecar ainda não escolhida; **não incorporar código externo enquanto isso não for resolvido**.

## Contratos de entrega comuns a todos os PRs

- **Investigar antes de programar:** buscar 1–3 implementações específicas na [matriz de open source](open-source-references.md), em SHA pinado; listar tradeoffs, licença por componente, decisão e testes existentes. Não copiar código de outros projetos sem revisão.
- **Dogfood:** executar `sidecar explain` no Sidecar e caso real apropriado; repetir queries PT/EN, de busca simples e de conflito; anexar diff da avaliação.
- **Testar:** regressão unitária, teste de contrato (hooks/schema), integração real quando pertinente, adversarial e cross-platform; manter baseline v1 e feature flags.
- **Medir custo:** latência de hook frio/quente, tokens extras, memória/CPU/disco e sucesso de tarefa onde houver agente; não celebrar melhoria de Hit@5 isoladamente.
- **Documentar:** atualizar ADR, README, roadmap, `AGENTS.md` e resultados, inclusive falhas/inconclusivos, no próprio PR; rollback explícito.
- **Segurança:** sem executar o repo, capturar prompt bruto, chamar modelo/rede por padrão, sobrescrever hook/config alheio ou instalar LSP silenciosamente.

## Trilha A — Fundamentação, instalação e runtime confiável (P0)

**Entrega A1 — Governança/distribuição**
- [ ] Alterar default branch para `main`; decidir e registrar LICENSE; definir formato dos notices de terceiros.
- [ ] Resolver contract de binário gerenciado x binário copiado, PATH, update, idempotência, backup/uninstall Windows (exe em uso).
- [ ] E2E de hooks Claude e Codex reais com consentimento/trust; `doctor` diferencia `configured` de `verified`.
- [ ] Release real GoReleaser com checksums, guia e smoke Linux/macOS/Windows; sem dependências de Node/daemon obrigatórias.

**Aceitação:** instalação e remoção repetidas deixam configs de terceiros intactas; binários e hooks de ambas as plataformas realmente usados; release reproduzível, update/rollback testados; nenhum false-success ao remover arquivo.

## Trilha B — Observador e telemetria local confiável (P0)

**Entrega B1 — Traces passivos antes de alterar inferência**
- [ ] Instrumentar trace e contadores do pipeline v1: candidatos, origem, motivo de seleção, orçamento, dedup, timings (sem mudar ranking).
- [ ] Especificar suporte por cliente/versão: `UserPromptSubmit`, tool-start/end, stop, compact e limites; adicionar adaptadores verificáveis.
- [ ] `SessionObserver` captura metadados redigidos: search/read/edit/test/lsp, caminhos permitidos como hashes, IDs e cobertura `observed/unobserved`.
- [ ] Estado `repo/worktree/agent/session` com TTL, locks, invalidação e opção opt-in; não guardar prompts/output bruto.
- [ ] Diagnóstico local `activity/status --json` (CLI futuro) e teste de observação sem injeção.

**Aceitação:** coleta não atrasa significativamente hook, não perde controle do agente, não vaza prompt nem confunde evento não observado com zero calls. Testar oito+ sessões concorrentes, event duplication, reboot/compaction, timestamps e erro de permissão. Benchmarks OFF/ON usam observador simétrico.

## Trilha C — Motor de contexto por intenção e custo de tokens (P0)

**Entrega C1 — Recuperação / packing**
- [ ] `IntentRouter` determinístico multiintenção `locate/impact/debug/extend/test/explain/general` com trace explicável e fallback v1.
- [ ] Aproveitar `retrieve` / BM25+tradução, índices/exatos/AST/Git/testes como fontes; ablação por componente e por intenção.
- [ ] `Evidence` tipada com caminho/linhas/revisão/fonte/nível de prova; gate abstention para `no_evidence`.
- [ ] `ContextPacker`: diversidade, novelty, custo de tokens, links testes↔código, controle de redundância; heurística sob budget mensurável.
- [ ] Orçamentos graduais experimentais (0 quando nada; 100–500 tokens em muitos prompts, ampliado apenas com evidência); contar overhead e tokens reais quando possível.
- [ ] Rollout com flags `v1` / `v2`, compatível com `explain --json`; novos campos versionados, sem quebrar CLI.

**Aceitação:** dados pinados + Gold, Hit@1/5, Recall, MRR, BCY@budget, abstention, falsos positivos, p50/p95 warm+cold, tokens extras. Nenhuma regressão silenciosa; resultado PT/EN comparado em cada classe.

## Trilha D — Reconciliação de contratos e memória técnica (P0/P1)

**Entrega D1 — Contract Evidence**
- [ ] Indexar docs, código, testes, rotas, enums, regras explícitas, manifests e histórico como evidências com proveniência e hash.
- [ ] Extrair claims **comparáveis** (assinatura, rota, estado, limites, validação) e relações; distinguir `mismatch_verified / potential_mismatch / stale / not_comparable`.
- [ ] `ContractResolver` aponta divergência código↔teste↔docs sem escolher automaticamente o comportamento desejado; não promover qualquer arquivo a instrução superior.
- [ ] `DecisionMemory` técnica e mínima, somente com consentimento e prova de origem/escopo/commit, busca contextual e invalidação de decisões obsoletas.
- [ ] Queries vagas PT/EN buscam candidatos válidos via sessão, docs, diffs e estrutura; quando desconhecido, indicam incerteza.

**Aceitação:** fixtures com conflitos e sem conflitos, documentos desatualizados, mesmos nomes em módulos diferentes, fontes inseguras, PT/EN vagos; medir **falso conflito** e impacto sobre patch/tests. [Detalhes](contract-evidence.md).

## Trilha E — LSP e detecção de capacidades do ambiente (P1)

**Entrega E1 — Env discovery + LSP**
- [ ] Reconhecer linguagens/manifests/versões/compiladores/test runners instalados sem executar scripts de projeto.
- [ ] Detectar LSP disponível e integrar-se corretamente com plugins nativos de Claude; não tentar acessar sessões privadas sem API.
- [ ] Cliente LSP próprio **opt-in**, JSON-RPC e cancelamento, `initialize`/capabilities, timeouts, limites de processos, cleanup e cache; fallback sem LSP.
- [ ] Setup seguro e versionado (download/instalação verificada, consentimento explícito, sem hook bloqueante); `lsp doctor/setup` somente quando implementado.
- [ ] Comparar custo/qualidade sem LSP, LSP do agente e LSP do Sidecar, mantendo ferramentas idênticas no braço de controle.

**Aceitação:** E2E Java+Go (e matriz Rust/TS/C# conforme suporte), workspaces múltiplos, timeout, rede, warmup, server missing, Windows/Linux, rollback e nenhum subprocesso órfão. [Detalhes](lsp-strategy.md).

## Trilha F — Intervenção adaptativa e feedback (P1)

**Entrega F1 — Contexto durante a tarefa**
- [ ] `SessionSignals` usa evidências já enviadas, arquivos pesquisados, erros e progresso para priorizar informação realmente nova; nunca presume que arquivo aberto = evidência correta.
- [ ] `ToolResponseContext` injeta apenas em eventos/clientes que **comprovadamente** entregam additionalContext ao modelo; outros eventos só observam.
- [ ] Cooldown, TTL, hash, limite de tokens por sessão, `no-op` e supressão de loops; replay da compactação preserva apenas fatos válidos, se suportado.
- [ ] Feedback pós-tarefa estruturado **opt-in, fora da tarefa avaliada**, com enums e evidência de utilidade; autorrelato é **sinal fraco**, não gold/treinamento online.
- [ ] Mostrar atividade em statusline/mod opcional (Claude) ou CLI; nunca exigido para recuperar contexto.

**Aceitação:** não aumenta loops em testes adversariais; `tool_call_id` opcional, eventos duplicados/ausentes tratados, nenhum tool blocking. Comparar tool calls, rounds, TTFE e custo com e sem intervenções.

## Trilha G — Benchmarks e comprovação da entrega completa (P0 até release)

**Entrega G1 — Harness de evidência**
- [ ] Reusar **Agent Retrieval Bench** com corpus fixo, licença e SHA, ferramenta oficial de scoring onde compatível; depois ContextBench como teste externo de contexto. Não reinventar gabaritos.
- [ ] Executar baseline `Sidecar v1`, `V2`, `BM25+tradução`, `rg`, grafo estrutural e concorrente CodeGraph conforme matriz de recursos; relatórios por intenção e PT/EN.
- [ ] Montar pequeno conjunto **próprio e privado** de tarefas reais: identificar contrato vago, docs↔código divergentes, testes/regras, trabalho pós-compactação, repo desconhecido e mudanças de múltiplos módulos.
- [ ] A/B agentic: `OFF`, `ON-v1`, `ON-v2`; mesmo commit, ambiente, ferramentas, modelo, permissions e estado inicial. Repetir modelos Haiku/Luna/Flash com limites de custo e ordem balanceada.
- [ ] Métricas: sucesso/passing oracle, TTFE **apresentado versus utilizado**, tool calls e repetição, rounds, intervenções humanas, tokens reais quando expostos, tempo de CPU/indexação/hook, custo e falhas/timeout.
- [ ] Feedback diagnostica, mas benchmark de qualidade exige gold, testes, revisão externa e intervalos de incerteza; nenhuma alegação determinística sobre respostas do modelo.
- [ ] Comparar também contra agente+CodeGraph e `Sidecar + CodeGraph` se integração fizer sentido, sem diferenciar ferramentas disponíveis injustamente.
- [ ] Publicar relatório completo de resultados favoráveis, negativos e inconclusivos em `docs/benchmark.md` ou relatórios versionados, jamais sobrescrevendo histórico.

**Metas de produto experimentais:** buscar **≥15% menos tempo até evidência correta utilizada** e **≥20% menos buscas/leitura até evidência**, sem reduzir resolução correta e com p95 warm de hook **≤150ms** onde mensurável. Metas não são resultados. Custo **total** importa mais que redução de tokens do contexto. Para resultados instáveis ou com amostra pequena, classificar `inconclusivo` e aumentar amostra só dentro do orçamento.

## Trilha H — UX, qualidade, documentação e release V2 (P0 final)

**Entrega H1 — Produto completo**
- [ ] Default usable com `sidecar install` uma única vez e diagnóstico claro; nenhuma skill grande injetada, config por repo ou LLM/embedding obrigatório.
- [ ] Matriz real de agentes/OS/hook modes, cobertura parcial de LSP e comportamento offline; onboarding e uninstall/upgrade testados.
- [ ] Interface/diagnóstico opcional mostra `injected/skipped/failed`, evidências e latência sem prompt bruto ou falsas afirmações.
- [ ] Testes funcionais, integração, segurança (prompt injection, arquivos sensíveis, path traversal, Symlink, scripts e rede), concorrência, migração, fallback e performance rodando em CI e smoke manual.
- [ ] Dogfooding Sidecar no próprio repo e em pelo menos dois repositórios externos de linguagens diferentes, com comparativo e fallback.
- [ ] Publicar release V2 com checksums/notices/versionamento, documentação consolidada e rollback. **Não marcar V2 completo antes das trilhas A–H validadas.**

## Dependências e execução em paralelo

```text
A (instalação confiável) ──────────────────────┬───────────> H (release)
B (observador passivo) ──┬─> F (intervenções) ┤
                         └─> G (agentic)      │
C (intent + packing) ────┬─> D (contracts) ───┤
                         ├─> E (LSP) ────────┤
                         └─> G (retrieval) ───┘
```

**PRs recomendados (não etapas descartáveis):** `01 benchmark/trace & baseline`; `02 observer+schemas`; `03 intent router`; `04 packer/gate`; `05 contract evidence`; `06 decision/session memory`; `07 LSP client+opt-in setup`; `08 adaptive tool context/compaction`; `09 benchmark agentic e competitor`; `10 release, UX e matriz E2E`. A (higiene de instalação/licença) é prioridade imediata e pode avançar em paralelo. Cada PR funcional entrega parte da arquitetura final e tem tests/ADR/documentação.

## Testes pendentes: check-list consolidado

| Categoria | Casos/gates mínimos |
|---|---|
| **Runtime v1** | `go test -count=1 ./...`, race nos pacotes concorrentes, lint, benchmark existente, CI Linux/Windows/macOS; corrigir fragilidade de assert de timing `0s` em Windows |
| **Hooks** | Cliente real Claude+Codex+Gemini conforme suporte declarado; schema/evento, trust/approval, múltiplos hooks, no-output, falha/timeout, upgrade/uninstall e cloud gaps |
| **Indexação** | cold/warm, dirty files, HEAD change, renames, worktrees, no Git, sem parser, megarepo, Unicode, arquivos gerados, index corruption, limites de tempo |
| **Retrieval v2** | intenções PT/EN, negativo/no-evidence, relações reais, score explainável, packing 4k/8k/16k, duplicates, budget, regressão v1, false positives |
| **Contracts** | conflitos verificáveis, ambiguidade, documentos antigos, testes inconsistentes, decisão inválida, prompt injection |
| **LSP** | mock JSON-RPC, capacidades, timeout, confiabilidade, processos e rede, instalação consentida e rollback, trabalho em multi-language |
| **Observer** | hooks ausentes/duplicados, sessões concorrentes, redaction, TTL, perda/compactação, observabilidade parcial, no-op |
| **Agentic** | mesmo modelo/ambiente/branch ON/OFF, PT/EN, cada condição e n, oracle independente, custo/rounds/tokens, adversos e incerteza |
| **Distribuição** | licença/notices, checksum, binários e path, upgrade repetido, release, instalação e desinstalação isoladas por SO |

**Regra de status:** `[x]` significa teste/implementação efetivamente demonstrados; texto planejado não equivale a pronto. Não executar rodadas pagas em CI por default. Se algo foi observado apenas em uma plataforma, não declarar cobertura universal.

## Critério final de release V2

Somente `READY` quando: A–H completas; todas as capacidades habilitadas são realmente suportadas e reversíveis; gates e mitigação de risco passaram; docs e código concordam; benchmarks reprodutíveis mostram qualidade e custo **com honestidade**; e o Sidecar apresenta comportamento útil sem obrigar o usuário a alterar a estrutura de seus repositórios.

Se o teste provar que uma funcionalidade **não** ajuda, registrar o resultado e simplificar/remover a técnica mediante decisão registrada, sem inflar o escopo. Produto completo significa **fluxo integrado e confiável**, não colecionar funcionalidades.

## Documentos por responsabilidade

- [v2-product-spec.md](v2-product-spec.md) — contrato de experiência, arquitetura, diferenciação e Definition of Done.
- [context-engine-v2.md](context-engine-v2.md) — decisões de retrieval/packing/estado e rollback.
- [contract-evidence.md](contract-evidence.md) — inferência vs conflito objetivo, memória com proveniência.
- [agent-feedback.md](agent-feedback.md) — contratos de eventos e feedback.
- [lsp-strategy.md](lsp-strategy.md) — descoberta/cliente/setup LSP seguro.
- [evaluation-plan.md](evaluation-plan.md) — datasets, métricas e execução A/B.
- [open-source-references.md](open-source-references.md) — **leitura de referências existentes obrigatória por módulo**; licenças e SHAs.
- [benchmark.md](benchmark.md), [release.md](release.md) — números históricos e entrega.
