# Roadmap consolidado — Sidecar V2, produto completo

**Atualização:** 2026-11-06 · **Estado:** plano de implementação, NÃO prova de funcionalidades concluídas.
**Fonte arquitetural:** [product-v2.md](product-v2.md) · **Reuso e licenças:** [reference-implementations.md](reference-implementations.md) · **Medição:** [evaluation-plan.md](evaluation-plan.md).

## 0. Direção irrevogável do produto

Construir e entregar **a V2 integrada**, e não um MVP com capacidades essenciais ausentes. A implementação continua **iterativa**, em PRs pequenos, reversíveis, com testes e documentação; a conclusão só é declarada quando todos os módulos e integrações comprometidos na arquitetura passaram pelos gates. Não confundir “produto completo” com ativar de uma vez todas as features sem evidência.

**Proposta de valor para uso pessoal:** diminuir tempo, tokens, buscas repetidas e necessidade de intervenção humana **por tarefa corretamente concluída** usando Claude Code, Codex e, onde suportado, Antigravity CLI e modelos econômicos. Zero configuração por repositório; instalação única; fallback universal; runtime local e sem LLM/rede obrigatórios.

**Diferencial a testar contra CodeGraph:** não ser “outro grafo” mais pobre, mas uma **camada econômica de entrega e manutenção de evidências** antes e durante a navegação do agente. A injeção inicial **não é exclusiva do Sidecar**: CodeGraph também a utiliza. O posicionamento exige benchmark competitivo.

## 1. Inventário auditado: temos vs falta

**Implementado antes deste planejamento:** binário Go e CLI; indexação incremental e worktrees; SQLite/FTS5; BM25 e normalização PT↔EN; tree-sitter em parte das linguagens; identificadores, estrutura, Git/RRF; contexto por budget; hook de prompt Claude e adaptador Codex; `doctor`, `status`, `explain`, install/uninstall; testes e CI multiplataforma; avaliador offline com fixtures. Documentação e AGENTS já existem.

**Não implementado / não comprovado:** IntentRouter por tarefa, pacote de evidências por utilidade marginal, fingerprint arquitetural, reconciliação de contratos código/docs/testes, observação de tool calls, detecção de redescoberta, memória de decisões entre sessões, cliente/instalador LSP gerenciado, provider Context7, Antigravity CLI adapter, feedback parametrizado, suíte A/B agentic confiável e prova de ganho real. A existência de interfaces de hook no cliente não prova E2E do Sidecar.

**Pendências operacionais P0:** LICENSE MIT criado na raiz em 2026-11-06 (ainda não commitado); GitHub default branch `main` verificada em 2026-11-06 (`gh repo view`); primeira release inexistente; PATH/upgrade/ownership; E2E Codex real; testes Windows de tempo (falha histórica intermitente em asserção `0s`, embora último CI tenha passado); testes de rollout/rollback e instalação por SO.

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
| W8 — Adaptadores, UX e integrações | P1 | Claude/Codex/Antigravity CLI onde suportado, status local e UI opcional, zero config de repo | W0, W5 |
| W9 — Avaliação agentic e qualidade de entrega | P0 contínuo | OFF vs v1 vs v2 e concorrentes; oráculos independentes, métricas de custo/qualidade | W1 e cada módulo |
| W10 — Hardening e entrega V2 integrada | P0 final | E2E, perf, segurança, compatibilidade, docs e release verificadas | W0–W9 |

P0 não significa “construir todos os módulos simultaneamente”. Ordem deve respeitar dependências e usar benchmarks para decidir políticas/defaults. W6/W7/W8 são partes da V2 completa, **não são descartados** por terem prioridade relativa P1.

## 3. Plano de PRs executáveis — rodadas de engenharia

Cada PR deve atualizar `AGENTS.md`/docs pertinentes, adicionar testes, anotar upstream e SHA consultados (se houver), explicar rollback e publicar comparação contra baseline. Não reproduzir grafo/LSP se um componente existente de licença compatível ou um protocolo resolver a necessidade.

### Rodada A — tornar o produto testável e instalável

- **A1 — Reparo de distribuição:** default branch `main` já verificada no GitHub (2026-11-06); LICENSE MIT criado (2026-11-06, ainda não commitado); fix de PATH/upgrade/uninstall Win; build release e smoke de instalação em Linux/macOS/Windows; teste de preservação de configuração externa.
- **A2 — E2E de adapters:** validar hook real Claude e Codex (consentimento/trust), perf e fail-open, configs existentes; observar eventos de ferramenta disponíveis e limitações por versão/superfície sem alegar cobertura universal.
- **A3 — Observabilidade baseline:** fortalecer `explain --json` com razões de escolha/supressão/latência, IDs e token estimates; **nenhuma alteração no score/ranking**; resolver o teste de tempo com asserção robusta cross-platform.
- **A4 — Corpus e harness:** adaptador offline do Agent Retrieval Bench e depois ContextBench, SHA/manifest/license checks, no-gold, gabaritos de arquivo/símbolo, resultados reprodutíveis em CI offline. Não baixar grandes corpora sempre no CI.

**Estado em 2026-11-06 (A3/A4 parcial, sem alterar ranking):** `sidecar-eval --compare` e `--report-md` implementados; `bench/dogfood_cases.json` com 6 consultas pinadas e `TestDogfoodQueries`; testes de risco de install→doctor→hook→reinstall→uninstall, caminhos com espaço/Unicode, concorrência e corrupção de SQLite. Detalhes e limites em [development.md](development.md).

**Gate A:** CI verde, install/uninstall/upgrade reversível, hooks reais reproduzidos nos clientes disponíveis, benchmark v1 congelado. Pendências de cliente indisponível são rotuladas bloqueio externo, nunca “aprovado”.

### Rodada B — inteligência de contexto integrada

- **B1 — RepoFingerprint:** inventário de frameworks, manifests, testes, padrões locais, sem executar repo; fontes e confiança; invalidar por commit/hash.
- **B2 — IntentRouter:** intenções `locate / debug / impact / extend / test / explain / contract / general` + `confidence` e fallback v1; comparar intenção e recuperação em PT/EN e queries vagas.
- **B3 — ContractEvidence:** ligar implementação, testes, docs e decisões; detectar constantes, estados, endpoints, assinaturas e assertions; retornar `IMPLEMENTED / TEST_EXPECTED / DOCUMENTED / INFERRED / CONFLICT_CANDIDATE`, sem “resolver” ambiguidade sozinho.
- **B4 — EvidencePacker e InjectPolicy:** utilidade incremental, diversidade, novidade de sessão e orçamento; `no evidence = no injection`; snippets curtos, IDs, linhas e proveniência; comparar contexto correto **usado**, não apenas apresentado.
- **B5 — Integração ponta a ponta:** combinar fingerprint + intenção + contratos + packing em hook real, com ablações e rollback para v1; impedir que doc inconsistente seja injetado como regra superior.

**Gate B:** não degrada sucesso/precisão em avaliação externa congelada; falsos conflitos e falsas “evidências verificadas” revisados; relatório de latência, tokens e cobertura. Se falhar, corrigir ou substituir técnica antes de promover default.

**Estado em 2026-11-06 (A3, B1 e B2 implementados; Gate B aberto):**
- **A3:** `explain --json` traz `id`, `source`, `confidence` (heurística fixa, não calibrada), `token_estimate`, `reason_selected` e `reason_suppressed` por candidato e por evidência, além de `intent`, `intent_routed`, `token_estimate` e `repo_fingerprint`. Ranking v1 inalterado. A asserção de tempo de `TestTraceExplainsFactorsAndTimings` agora exige apenas presença e valor não negativo, porque um estágio rápido pode medir 0 em relógio grosso.
- **B1:** `internal/repo/fingerprint.go` (`Discover`): linguagens e contagens por código/teste/doc/config, grau de suporte, manifests com dependências declaradas (Go, npm, Maven, Gradle, Cargo, Python, NuGet, Composer), convenções de teste, entrypoints, monorepo, arquivos gerados e `hash` de invalidação (HEAD mais conteúdo dos manifests). Não há cache persistido: é recalculado em `explain`. `Classify`, contrato do indexador, não mudou; as linguagens novas entram por `LanguageOf`, então indexação e retrieval permanecem idênticos.
- **B2:** `internal/retrieve/intent.go` (`ClassifyIntent`): regras nomeadas PT/EN sobre texto normalizado, formatos de stack trace e identificadores de código; `Signals` lista cada regra disparada. Confiança < 0,55 cai no v1. Com `EngineVersion: "v2"` ou `IntentRouting`, a intenção só restringe a expansão estrutural. Só o primeiro trecho e o último de 2 KB de cada prompt são lidos, para custo fixo.
- **Eval** (`bench/cases.json`, 71 casos): 11 prompts roteiam; 59 caem em `general`, porque são consultas vagas sem vocabulário de intenção. v2.1 iguala v1 em Hit@5, Recall@10, MRR, precisão e falsos positivos em todos os splits e reduz tokens médios em 2–4% (354→345, 214→210, 215→206). Isso não demonstra ganho de qualidade nem ganho end-to-end com agente, que não foi medido.
- **Pendências do Gate B:** rotulagem de intenção para comparar intenção × recuperação em PT/EN e queries vagas; cache de fingerprint; calibração de `confidence` contra o eval; conjunto de casos com divergência real (código × doc × teste).

**Estado em 2026-11-06 (B3, B4 e B5 implementados atrás de `SIDECAR_ENGINE=v2`; padrão continua v1):**
- **B3:** `internal/retrieve/contract.go`. Vincula cada definição (função, método, constante, rota, struct/classe) a testes que a referenciam (índice; para constantes, varredura de texto), seções Markdown e docstrings. Classifica `IMPLEMENTED`, `TEST_EXPECTED`, `DOCUMENTED`, `INFERRED`, `UNVERIFIED` e `CONFLICT_CANDIDATE`. Conflito só com valor literal ou método HTTP **explícito** em uma linha, sem outro identificador de código, sem comparação e sem aritmética; cada lado vai com caminho e linha, e a nota proíbe escolher um vencedor. **Limite:** assinaturas de função não são comparadas (exige parser de parâmetros por linguagem); a classe `UNVERIFIED` cobre valor que não pode ser atribuído.
- **B4:** `internal/retrieve/packer.go`. Seleção gulosa por utilidade por token dentro do top-8 da v1, com `InjectionPolicy` (nova na sessão? confiável? abstenção sem item de confiança ≥ 0,5). Perfis `minimal`/`balanced`/`deep` (300/800/1500). Não reduz tokens nesta suíte (ver `benchmark.md`); a diferença real do packer é a memória da sessão, que o bench não mede.
- **B5:** `Options.EngineVersion`, `--engine v1|v2` em `sidecar explain`, `SIDECAR_ENGINE` e `SIDECAR_PROFILE` no hook; ablações `NoIntent`, `NoContract`, `NoPacker` em `sidecar-eval`. Testes unitários e `go test -race` nos pacotes concorrentes passam. Hook com v2 **sem** E2E com Claude Code ou Codex: pendente.
- **Pendências B3–B5:** casos de divergência real no bench; medição de latência do hook com v2 em repositório grande (a varredura de docs e testes lê até 300 arquivos por prompt); E2E por cliente; medição agêntica.
- **TestDogfoodQueries (decisão aplicada):** no HEAD só D01 falhava (docs superavam `app.Hook`). Com os arquivos de teste novos, D02–D04 também saíram do top-5, porque o repositório vivo mudou o vocabulário e o IDF do próprio índice. A decisão foi congelar o corpus: o teste agora indexa um snapshot de produção (`dogfoodSnapshot`). Nesse snapshot todos os casos passam, mas `Hook` (D01) ficou no 5.º lugar, na borda do top-5. **Isso não corrige a qualidade do repositório vivo:** a consulta D01 continua errada se o `explain` rodar sobre o repositório inteiro. O snapshot muda o que o teste mede; o ranking v1 não mudou.
- **Pré-existente, não corrigido:** o método `textual` e alguns desempates de `lexical` variam entre execuções do mesmo código, porque os IDs de símbolo dependem da ordem da indexação paralela. Comparações com diferença de centésimos de MRR precisam de repetição.

### Rodada C — acompanhar o agente com baixo custo

**Estado (2026-11-06, rodadas C1–C5 e D5 parcial):** implementados com testes unitários executados e `go test -race` nos pacotes concorrentes. Não implementado ou não verificado, e portanto **não** concluído: (1) nenhum hook de `PreToolUse`/`PostToolUse` é instalado por padrão, `sidecar observe` existe mas precisa de E2E por cliente e medição de custo antes de ser ligado; (2) `PreCompact` é aceito por `sidecar observe` (zera a deduplicação da sessão), mas nenhum instalador registra esse evento ainda; (3) Claude Code e Codex não documentam campo de falha em `PostToolUse` (resultado `unknown`), então `repeated_error` só dispara com Antigravity; (4) C4 é apenas a política passiva (`InjectionAllowed`), sem injeção por evento de ferramenta; (5) `invocationNum` = 0 como primeira chamada do turno é assumido, não verificado; (6) os upstreams Claude-Mem e Beads **não foram lidos** e nenhum SHA foi registrado, exigência do AGENTS.md ainda aberta; (7) D5 sem E2E com o Antigravity CLI real.

- **C1 — SessionObserver passivo:** normalizar eventos suportados de Claude, Codex e Antigravity CLI por adapter; IDs opacos, retenção curta, redaction, distinção `unobserved` vs zero; modo OFF possui instrumentação igual no benchmark.
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
- **D5 — Antigravity CLI + status UX:** validar hooks reais para Antigravity CLI; status local baixo custo, UI/mod Claude opcional; reverter configs com segurança.

**Status da Rodada D (2026-11-06):** D1 (descoberta) e D2 (cliente opt-in) implementados e testados; D3 (`sidecar lsp status|doctor|trust|setup`) implementado, com setup bloqueado para csharp, java e php; D4 tem `DependencyDocsProvider` local com proveniência, sem adaptador de rede; D5 tem `status --json`, `activity`, `statusline` e `install --statusline` com rollback. Divergências do brief, todas em [lsp-strategy.md](lsp-strategy.md): sem `fallback.go` (Rust, PHP, Ruby e Kotlin já têm gramática embutida), `pyright` CLI não conta como servidor, e o subcomando `lsp trust` foi acrescentado. O hook de prompt não usa LSP ainda.

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
| CLI/distribuição | build + checksums; install→reinstall→upgrade→uninstall→rollback; PATH e ownership; 3 SO | **Parcial: E2E de binário em Linux (`TestCLIEndToEndWithAllAdapters`: install com três adapters e flags, doctor, schemas de hook, observe, status e uninstall --purge restaurando configs); 3 SO, upgrade e rollback pendentes** |
| Hooks | schema por versão, aprovação Codex, Claude real, Antigravity CLI real, no evidence/failure/safe stdout, coexistência | **Parcial** |
| Ranking/intenção | baselines v1, ablações, PT/EN, tie-break estável, negativas, mudanças de repo | **V2 pendente** |
| Contratos | código/docs/testes de acordo, conflito explícito, falso conflito, fonte desatualizada, alteração de commit | **Pendente** |
| SessionObserver | repetição real/legítima, concorrência, TTL, privacy, compactação, unobserved, session IDs | **Parcial: unitários; concorrência (`TestObserverConcurrentWrites`, com `-race`); compactação (`TestObserverPreCompactResetsSessionSeen`); E2E do binário com `observe` PreToolUse/PostToolUse. Custo com agente real não medido; `PreCompact` não é instalado por nenhum instalador** |
| Memória | decisão com hash, stale/invalidation, purge e opt-in, não misturar worktrees | **Parcial: decisão ACTIVE injetada em v2 e decisão editada não injetada (`TestDecisionMemoryIsInjectedInV2AndNeverWhenStale`); sem E2E com agente real nem medição de custo** |
| LSP/docs | handshake, Java/Go, absent, slow, untrusted, windows, cancelamento, checksum/rollback | **Parcial**: handshake, absent, slow, untrusted, erro e cancelamento de processo testados em Linux; `gopls` real só para Go; Java, Windows, macOS e checksum/rollback de instalação de LSP pendentes |
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
