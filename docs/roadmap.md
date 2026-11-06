# Roadmap consolidado — Sidecar V2, produto completo

**Atualização:** 2026-11-06 · **Estado:** plano de implementação, NÃO prova de funcionalidades concluídas.
**Fonte arquitetural:** [product-v2.md](product-v2.md) · **Reuso e licenças:** [reference-implementations.md](reference-implementations.md) · **Medição:** [evaluation-plan.md](evaluation-plan.md).

## 0. Direção irrevogável do produto

Construir e entregar **a V2 integrada**, e não um MVP com capacidades essenciais ausentes. A implementação continua **iterativa**, em PRs pequenos, reversíveis, com testes e documentação; a conclusão só é declarada quando todos os módulos e integrações comprometidos na arquitetura passaram pelos gates. Não confundir “produto completo” com ativar de uma vez todas as features sem evidência.

**Proposta de valor para uso pessoal:** diminuir tempo, tokens, buscas repetidas e necessidade de intervenção humana **por tarefa corretamente concluída** usando Claude Code, Codex e, onde suportado, Gemini CLI e modelos econômicos. Zero configuração por repositório; instalação única; fallback universal; runtime local e sem LLM/rede obrigatórios.

**Diferencial a testar contra CodeGraph:** não ser “outro grafo” mais pobre, mas uma **camada econômica de entrega e manutenção de evidências** antes e durante a navegação do agente. A injeção inicial **não é exclusiva do Sidecar**: CodeGraph também a utiliza. O posicionamento exige benchmark competitivo.

## 1. Inventário auditado: temos vs falta

**Implementado antes deste planejamento:** binário Go e CLI; indexação incremental e worktrees; SQLite/FTS5; BM25 e normalização PT↔EN; tree-sitter em parte das linguagens; identificadores, estrutura, Git/RRF; contexto por budget; hook de prompt Claude e adaptador Codex; `doctor`, `status`, `explain`, install/uninstall; testes e CI multiplataforma; avaliador offline com fixtures. Documentação e AGENTS já existem.

**Não implementado / não comprovado:** IntentRouter por tarefa, pacote de evidências por utilidade marginal, fingerprint arquitetural, reconciliação de contratos código/docs/testes, observação de tool calls, detecção de redescoberta, memória de decisões entre sessões, cliente/instalador LSP gerenciado, provider Context7, Gemini adapter, feedback parametrizado, suíte A/B agentic confiável e prova de ganho real. A existência de interfaces de hook no cliente não prova E2E do Sidecar.

**Pendências operacionais P0:** licença do Sidecar; GitHub default branch ainda `ccr-b6db8870-kl90n6` em vez de `main`; primeira release inexistente; PATH/upgrade/ownership; E2E Codex real; testes Windows de tempo (falha histórica intermitente em asserção `0s`, embora último CI tenha passado); testes de rollout/rollback e instalação por SO.

**Evidência hoje:** `docs/benchmark.md` é retrieval offline, não melhora de tempo/rounds; sinais de grafo/Git ainda não demonstraram ganho consistente frente a BM25 + tradução. Nenhuma cifra de economia agentic pode ser publicada como resultado Sidecar.

## 2. Componentes do produto e prioridade

| Workstream | Prioridade | Produto final comprometido | Dependências |
|---|---|---|---|
| W0 — Engenharia, licenças e distribuição | P0 | Instalação por usuário, rollback, binários verificados, Claude/Codex E2E, projeto licenciável | Nenhuma |
| W1 — Evidência, corpus e observabilidade baseline | P0 | Trace sem alterar ranking, gold público pinado, dashboard local de métricas, dogfooding | W0 parcial |
| W2 — RepoFingerprint + IntentRouter | P0 | Descoberta de arquitetura, intenção multirrótulo, fallback, testes PT/EN e repositórios diversos | W1 |
| W3 — ContractEvidence/ConflictResolver | P0 | Evidência correlacionada de código, testes e docs, conflitos verificáveis e proveniência | W1, W2 |
| W4 — EvidencePacker + InjectionPolicy | P0 | Contexto de alta utilidade por token, abstinência, limites, deduplicação e gates | W1, W2, W3 |
| W5 — SessionObserver + LoopDiagnostics | P0 | Traces por adapter, busca repetida, continuidade e intervenção seletiva segura | W1, W4, adapter hooks |
| W6 — DecisionMemory | P1 | Memória opcional de decisões técnicas verificáveis, TTL, invalidação e purge | W3, W5 |
| W7 — LSP + Documentation Providers | P1 | Detecção, integração local opt-in, setup confiável e rollback; docs por versão como complemento | W0, W2, W3 |
| W8 — Adaptadores, UX e integrações | P1 | Claude/Codex/Gemini onde suportado, status local e UI opcional, zero config de repo | W0, W5 |
| W9 — Avaliação agentic e qualidade de entrega | P0 contínuo | OFF vs v1 vs v2 e concorrentes; oráculos independentes, métricas de custo/qualidade | W1 e cada módulo |
| W10 — Hardening e entrega V2 integrada | P0 final | E2E, perf, segurança, compatibilidade, docs e release verificadas | W0–W9 |

P0 não significa “construir todos os módulos simultaneamente”. Ordem deve respeitar dependências e usar benchmarks para decidir políticas/defaults. W6/W7/W8 são partes da V2 completa, **não são descartados** por terem prioridade relativa P1.

## 3. Plano de PRs executáveis — rodadas de engenharia

Cada PR deve atualizar `AGENTS.md`/docs pertinentes, adicionar testes, anotar upstream e SHA consultados (se houver), explicar rollback e publicar comparação contra baseline. Não reproduzir grafo/LSP se um componente existente de licença compatível ou um protocolo resolver a necessidade.

### Rodada A — tornar o produto testável e instalável

- **A1 — Reparo de distribuição:** mudar default branch no GitHub; decidir/commitar LICENSE; fix de PATH/upgrade/uninstall Win; build release e smoke de instalação em Linux/macOS/Windows; teste de preservação de configuração externa.
- **A2 — E2E de adapters:** validar hook real Claude e Codex (consentimento/trust), perf e fail-open, configs existentes; observar eventos de ferramenta disponíveis e limitações por versão/superfície sem alegar cobertura universal.
- **A3 — Observabilidade baseline:** fortalecer `explain --json` com razões de escolha/supressão/latência, IDs e token estimates; **nenhuma alteração no score/ranking**; resolver o teste de tempo com asserção robusta cross-platform.
- **A4 — Corpus e harness:** adaptador offline do Agent Retrieval Bench e depois ContextBench, SHA/manifest/license checks, no-gold, gabaritos de arquivo/símbolo, resultados reprodutíveis em CI offline. Não baixar grandes corpora sempre no CI.

**Gate A:** CI verde, install/uninstall/upgrade reversível, hooks reais reproduzidos nos clientes disponíveis, benchmark v1 congelado. Pendências de cliente indisponível são rotuladas bloqueio externo, nunca “aprovado”.

### Rodada B — inteligência de contexto integrada

- **B1 — RepoFingerprint:** inventário de frameworks, manifests, testes, padrões locais, sem executar repo; fontes e confiança; invalidar por commit/hash.
- **B2 — IntentRouter:** intenções `locate / debug / impact / extend / test / explain / contract / general` + `confidence` e fallback v1; comparar intenção e recuperação em PT/EN e queries vagas.
- **B3 — ContractEvidence:** ligar implementação, testes, docs e decisões; detectar constantes, estados, endpoints, assinaturas e assertions; retornar `IMPLEMENTED / TEST_EXPECTED / DOCUMENTED / INFERRED / CONFLICT_CANDIDATE`, sem “resolver” ambiguidade sozinho.
- **B4 — EvidencePacker e InjectPolicy:** utilidade incremental, diversidade, novidade de sessão e orçamento; `no evidence = no injection`; snippets curtos, IDs, linhas e proveniência; comparar contexto correto **usado**, não apenas apresentado.
- **B5 — Integração ponta a ponta:** combinar fingerprint + intenção + contratos + packing em hook real, com ablações e rollback para v1; impedir que doc inconsistente seja injetado como regra superior.

**Gate B:** não degrada sucesso/precisão em avaliação externa congelada; falsos conflitos e falsas “evidências verificadas” revisados; relatório de latência, tokens e cobertura. Se falhar, corrigir ou substituir técnica antes de promover default.

### Rodada C — acompanhar o agente com baixo custo

- **C1 — SessionObserver passivo:** normalizar eventos suportados de Claude, Codex e Gemini por adapter; IDs opacos, retenção curta, redaction, distinção `unobserved` vs zero; modo OFF possui instrumentação igual no benchmark.
- **C2 — LoopDiagnostics:** detectar repetição de buscas/leituras sem novidade e mudanças de stage observáveis; testar falso alarme em loops legítimos; não interferir no agente nesta etapa.
- **C3 — Continuidade/SessionSignals:** memória efêmera por sessão, hash de evidências, mudança de branch e compactação; retomar sem republicar contexto inteiro.
- **C4 — Intervenção seletiva:** apenas quando cliente suportar entrega de evidência em evento real e houver **nova evidência verificada**; alternativa é aguardar próximo UserPromptSubmit. Nunca cancelar arbitrariamente tool calls.
- **C5 — DecisionMemory:** guardar decisões técnicas com origem/versão e invalidação; opt-in para persistência entre sessões; diagnóstico e purge.

**Gate C:** mede repetição, custo de hook, cobertura de eventos e utilidade das intervenções; nenhuma coleta de prompts/tool output; tempo e qualidade não podem piorar silenciosamente.

### Rodada D — providers e usabilidade completa

- **D1 — LSP discovery:** language/workspace/servidor instalado/capabilities/status; coexistir com plugin LSP do Claude; fallback universal sem dependência.
- **D2 — LSP retrieval:** cliente próprio JSON-RPC somente com servidor confiável e autorizado, timeouts/recursos/locks, definitions/references/symbols e proveniência; testar Java/Go e cross-platform.
- **D3 — LSP install:** comando/UX assistidos, com origem, versão, checksum, licença, preview e rollback; **nunca instalar durante prompt/hook**.
- **D4 — Dependency docs:** detectar versões do repo e adapter externo opcional (Context7, documentação oficial); consentimento e cache; rede **nunca** requerida no core.
- **D5 — Gemini + status UX:** validar hooks reais para Gemini CLI; status local baixo custo, UI/mod Claude opcional; reverter configs com segurança.

**Gate D:** instalação/desinstalação e confiança verificáveis; sem LSP/docs online ainda há produto funcional; adapter por OS/superfície com cobertura efetivamente testada.

### Rodada E — qualidade integrada, benchmarking e release V2

- **E1 — Matriz de tarefas:** benchmark público mais 10–15 casos de uso próprio (prompt vago, contrato implícito, docs divergentes, repetição de busca, retomada e alteração com testes); gold e critérios de aceitação definidos **antes**.
- **E2 — A/B agentic:** agentes baratos Haiku/Luna/Flash com IDs exatos; braços OFF/v1/v2, ordem AB/BA e repetições. Para 4 tarefas × 3 modelos × 3 braços = **36 execuções por repetição**; definir teto e realizar smoke menor para qualificar o harness, **não chamar de teste do produto incompleto**.
- **E3 — Benchmark competitivo:** incluir CodeGraph em configuração documentada, tooling equivalente em todos os braços e orçamento de contexto equivalente; medir por tarefa correta, latência, tokens, chamadas, rounds e intervenções.
- **E4 — Hardening final:** dados não confiáveis, prompt injection, paths, SQLite corrupção/locks, limites de CPU/memória, Windows/macOS/Linux, monorepos, sem Git, symlinks, Unicode, áreas sensíveis e transições de versão de storage.
- **E5 — Distribuição:** documentação sem promessas falsas, LICENSE/THIRD_PARTY_NOTICES conforme aplicável, release com checksums, instalação simples, rollback/backup e relatório público honesto de limitações.

**Gate E:** a V2 só é considerada concluída quando o conjunto estiver integrado e verificável, com falhas/efeitos negativos publicados e caminho de recuperação. Metas de produto para amostra robusta são reduzir **≥15% tempo até evidência correta usada** e **≥20% buscas/leitura** sem degradar taxa de solução; são hipóteses, **não resultados nem gatilhos de fingir significância no piloto**.

## 4. Testes obrigatórios e rastreabilidade de status

| Área | Testes de aceite mínimos | Status |
|---|---|---|
| CLI/distribuição | build + checksums; install→reinstall→upgrade→uninstall→rollback; PATH e ownership; 3 SO | **Pendente E2E abrangente** |
| Hooks | schema por versão, aprovação Codex, Claude real, Gemini real, no evidence/failure/safe stdout, coexistência | **Parcial** |
| Ranking/intenção | baselines v1, ablações, PT/EN, tie-break estável, negativas, mudanças de repo | **V2 pendente** |
| Contratos | código/docs/testes de acordo, conflito explícito, falso conflito, fonte desatualizada, alteração de commit | **Pendente** |
| SessionObserver | repetição real/legítima, concorrência, TTL, privacy, compactação, unobserved, session IDs | **Pendente** |
| Memória | decisão com hash, stale/invalidation, purge e opt-in, não misturar worktrees | **Pendente** |
| LSP/docs | handshake, Java/Go, absent, slow, untrusted, windows, cancelamento, checksum/rollback | **Pendente** |
| Desempenho | cold/warm, p50/p95/p99, overhead do processo, memória, tokens, indexação grande | **Parcial v1** |
| Segurança | entrada não confiável, injeção de instruções, symlinks, segredos, paths e logs sem conteúdo sensível | **Parcial v1** |
| Agentes reais | OFF/v1/v2, sucesso, TTFE utilizado, tool calls, rounds, total tokens, walltime, interferência | **Não executado** |
| Concorrentes | CodeGraph e busca nativa; mesmo modelo, tarefas, permissions, ferramentas e budget | **Não executado** |
| Publicação | versão, changelog, licença, comparações, limitações, guia rápido, rollback | **Pendente** |

**Invariante de validação:** unit test verde não é substituto de E2E real; cross-compile não prova plugin funcionando; offline ranking não prova economia agentic; ausência de evento não é ausência de ferramenta usada; autoavaliação do modelo não é gold.

## 5. Governança de implementação e referências OSS

**Antes de qualquer PR de código:** consultar [reference-implementations.md](reference-implementations.md), os dois upstreams mais pertinentes, seus testes, licenças **por componente**, versões/SHAs, e justificar `integrar / adaptar / implementar independentemente`. Ex.: CodeGraph (duas implementações), SolidLSP/Serena, Context7, Repomix, RTK-OSS, Headroom, Claude-Mem, Superpowers, Agent Retrieval Bench.

- Não recriar mecanismos maduros só para “ter código próprio”, mas preservar simplicidade e sem dependência obrigatória do repo do usuário.
- Nada de GPL/BUSL importado sem revisão/licença compatível e decisão explícita. Context7 **não** publica o backend privado de coleta/parser.
- Ordem de descoberta ⇒ testes contratados ⇒ engenharia mínima ⇒ ablação ⇒ E2E ⇒ docs no mesmo PR. Nenhuma melhoria com benchmark omitido.
- Não usar skill extensa em every prompt. Sempre conservar **runtime local, fail-open e sem rede/modelo obrigatórios**.
- A base de documentação da V2 é [product-v2.md](product-v2.md), [context-engine-v2.md](context-engine-v2.md), [agent-feedback.md](agent-feedback.md), [lsp-strategy.md](lsp-strategy.md), [evaluation-plan.md](evaluation-plan.md), [reference-implementations.md](reference-implementations.md), [release.md](release.md) e [development.md](development.md).

## 6. Regra de promoção à próxima etapa

Cada feature só muda o default após um PR que contenha: hipótese → referência upstream e licença → diff → testes locais/CI → métricas **antes e depois** → regressões adversas → documentação → reversão. Se um experimento falhar, substituí-lo sem abrir mão da capacidade essencial correspondente do produto.

**V2 pronta** significa produto **integrado, instalável, testado e auditável**, não a soma de módulos que compilaram isoladamente.
