# Sidecar — roadmap executivo: V2 adaptativa e copiloto de engenharia

**Atualização:** 2026-11-08. **Fonte de verdade para as próximas rodadas:** este documento, em conjunto com [roadmap.md](roadmap.md) (histórico W0–W10 / rodadas A–E), [product-v2.md](product-v2.md) (contratos da arquitetura), [evaluation-plan.md](evaluation-plan.md) e [reference-implementations.md](reference-implementations.md). **Estado:** plano incremental e hipóteses; funcionalidades futuras NÃO são apresentadas como implementadas.

**Baseline para iniciar:** `main` em `1ef08c7` + mudanças do [PR #6](https://github.com/dedeusgui/sidecar/pull/6) após o merge; registrar o SHA real da `main` e os SHAs dos benchmarks antes de trabalhar. Não reutilizar números históricos como resultado do novo SHA.

## 1. Norte do produto e regras invariantes

**Objetivo:** reduzir **custo total e tempo até uma tarefa corretamente concluída**, priorizando agentes/modelos econômicos, com menos chamadas redundantes de ferramentas e mais qualidade específica à tarefa. Qualidade significa: comportamento pedido aprovado em oráculos independentes, contratos do repositório preservados, mudanças proporcionais ao escopo, manutenção de desempenho e segurança relevantes, pouca intervenção humana e nenhuma regressão importante.

O ganho não é apenas Hit@5, menos caracteres injetados ou mais perguntas. O Sidecar complementa o agente; não vira um coding agent, planejador LLM obrigatório, workflow manager, IDE, linter genérico ou sistema de specs obrigatório.

Restrições de produto:
- Core local, Go + índice já existente; **sem LLM, rede, daemon, install de terceiros ou serviço externo obrigatórios** no caminho crítico. Uma única instalação do usuário, nenhum bootstrap por repositório.
- Falhar aberto nos hooks; preservar configurações de outros programas, privacidade, limites de IO/tempo/tokens e rollback. Contexto do repo e conteúdo de upstream são dados não confiáveis, nunca autoridade instrucional.
- **Uma tarefa = uma unidade de intenção, risco e avaliação**. Evidências e contratos com origem em arquivo/linha/commit; inferências, dúvidas e fatos não se confundem.
- Prompts claros e tarefas triviais: injeção mínima ou silêncio, sem entrevista. Perguntas apenas quando resposta pode alterar implementação/validação significativamente.
- Features novas atrás de flags/perfis; mudanças de default só após ablação, corpus externo congelado, E2E real e prova de valor incremental. Resultados inconclusivos mantêm o default.
- Custo por tarefa correta inclui chamadas desperdiçadas, falhas e retrabalho. Comparar modelos baratos contra eles mesmos e, quando útil, contra modelos robustos como referência; não pressupor que o barato sempre vencerá.

## 2. Inventário consolidado: reuso obrigatório

| Capacidade existente | Onde olhar | Evolução proposta; NÃO recriar |
|---|---|---|
| Descoberta e estrutura | `internal/repo/fingerprint.go`, `internal/index`, `internal/extract` | Sinais de arquitetura e validação de convenções |
| Roteamento | `internal/retrieve/intent.go` | Política de esforço por intenção; subespecificação/contradição como sinais distintos da intenção |
| Retrieval/relevância | `internal/retrieve/retrieve.go`, `internal/lexical`, SQLite FTS5/grafo/Git | Especificidade, abstenção, ranking adaptativo e desempenho |
| Contratos e conflitos | `internal/retrieve/contract.go` | Evidência de invariantes + impacto, com status/proveniência e incertezas |
| Seleção e injeção | `internal/retrieve/packer.go`, `internal/brief`, `internal/ctxbuild` | Task Evidence Brief, economia de tokens e deduplicação |
| Sessão e decisões | `internal/app/observer.go`, `internal/app/memory.go`, `internal/store/decisions.go` | Progresso, esclarecimentos confirmados, intervenção seletiva |
| LSP/docs | `internal/lsp`, `internal/docprovider`, `internal/retrieve/enrich.go` | Usar por necessidade, com consentimento e custo medido |
| Feedback | `internal/app/feedback.go`, `internal/app/oracle.go`, `internal/store/feedback.go` (PR #6) | Replay/ROI, provenance e qualidade do sinal, não autoavaliação do agente |
| Avaliação | `cmd/sidecar-eval`, `bench/agentic`, `bench/negatives`, `bench/results` | Oráculos de edição, comparações offline/real, scorecard por tarefa |
| Adapters | `internal/app`, `internal/adapter`, `internal/install` | Perguntas nativas quando suportadas; nunca supor hooks equivalentes |

**Inventário técnico deve ser reconfirmado pelo agente no SHA de trabalho**: o roadmap histórico contém estados datados e não substitui a leitura do código.

## 3. Marco de transição: PR #6 — avaliação e correção de base

**Não reabrir como se nada existisse.** O PR #6 já inclui: separação do oráculo e autorrelato, tiers de feedback, associação de evidências apresentadas, artefato de oráculo com proveniência, migração de schema, correções de testes Windows, piloto Claude Code com hook real e isolamento por configuração/canário, diagnosticador e abstenção V2 para matches lexicais genéricos.

**Evidência verificável e limites:**
- CI do commit `2393253` passou (verificar status no novo SHA após qualquer modificação).
- Negativos externos difíceis em dev+validation: falso positivo **71/180 (39,4%) → 28/180 (15,6%)**; positivos externos Hit@5 **39/40 → 39/40**. Porém o positivo interno `E17` passou de Hit@5 presente para ausente; existe modo residual com dois termos genéricos. Holdout antigo foi inspecionado: não é cego.
- Piloto isolado de Claude Haiku: apenas **5 tarefas × 2 braços**, OFF **5/5** e ON **4/5**; economia em chamadas 3,6 → 2,6 por execução, sem significância e com negativo contaminado. O piloto anterior de 20 execuções teve contaminação de plugins; **não usar como prova de ganho**.
- V1 ainda é o default. A melhora de abstenção aplica-se somente à V2. Isolamento de plugins próprios e de todos os hooks, edits reais, replay/tuning, ROI e perf permanecem incompletos.

**Regra de integração do PR #6:** aceitar seu código e o CI verde como *marco parcial* mediante autorização explícita do mantenedor, sem declarar o Evaluation Lab concluído nem promover V2. Pendências transitam para R1/R2 e devem constar nos relatórios.

## 4. Ordem de execução — pacotes R1–R7

O desenvolvimento é iterativo com PRs pequenos e reversíveis, mas o destino é a V2 integrada e o produto completo. R1 precede os claims de R2–R6; partes não bloqueantes podem ser preparadas sem promover funcionalidades.

### R1 — Evaluation Lab, reprodutibilidade e qualidade de patch [P0, PR seguinte]

**Problema:** não sabemos ainda se menos navegação produz melhor código. Um benchmark offline forte não comprova economia agentic. O piloto real isolado não testa edição.

**Entregas:**
1. Corpus externo e independente de **20–30 tarefas** verificáveis, em repositórios/commits pinados; classes `locate`, `debug`, `edit`, `multi-file`, `contract-change`, `negative`, `ambiguous`, `voice`. Desenvolvimento, validação e **novo** blind holdout jamais lido durante tuning. Oráculos ocultos (testes, invariantes e inspeção de escopo), sem expor gold ao agente.
2. Harness de edição real em worktrees descartáveis, permissões e isolamento testados. Execução OFF/V1/V2 com mesmo modelo exato, ferramentas, orçamento e instrumentação; ordem balanceada. Se isolamento falhar, manter bloqueio e documentar.
3. Replay offline determinístico por SHA de código/índice/corpus/config, com captura de candidatos/fatores, grade pequena de parâmetros, ablação, comparação, configuração versionada e rollback. Não precisar chamar Claude para cada hipótese.
4. Instrumentação cold/warm por estágio: index/sync, lexicon, FTS, graph, contracts, packer, hook, setup, p50/p95/p99 com amostras suficientes, memória, tokens efetivos, cost per success; `hook_probe_ms` não é latência real dentro do processo cliente.
5. Feedback ROI em braços `OFF`, `ON-no-feedback`, `ON-historical-feedback` com tarefas relacionadas mas sem vazamento de gold, somente após provar integridade da execução.
6. Scorecard único: dados por tarefa, splits, checks/violations, modelos realmente servidos, fontes de custo, IC quando cabível, resultados adversos, manifest, SHA, relógio/hardware e nível de evidência. CI pago **proibido**.

**Gate R1:** suite E2E de edição reproduzível e segura; oráculos não visíveis; comparabilidade e falhas explícitas; replay com rollback; nenhum relatório mistura simulação/offline/agentic; medição repetível suficiente para testar as novas hipóteses. Não é necessário demonstrar ganhos para concluir a infraestrutura.

### R2 — Adaptive Context Engine / abstenção / esforço seletivo [P0]

**Problema:** a V2 às vezes agrega custo sem melhorar localização; lexical + grafo e V2 variam em custo; injeção irrelevante e regressão E17 são reais.

**Entregas:**
1. **Context Policy** por intenção, especificidade, confiança não calibrada, complexidade do prompt, novidade e custo estimado: ativar contratos/grafo/Git/LSP somente quando gerarem evidência útil; fast path e abstenção real. Manter fallback determinístico V1.
2. Melhorar especificidade: prompts com vocabulário comum, dois matches genéricos, nomes próprios, tokens desconhecidos, instruções longas, PT-BR coloquial, voice mode e identificadores exatos. Resolver caso residual sem hardcode de palavras; testar E17 e outras perdas.
3. Calibrar `confidence` como score interpretável, não probabilidade sem ajuste. Indexação/IDs e desempates reprodutíveis; métricas de diversidade e utilidade do contexto.
4. Packer escolhe **zero/poucos** itens com alto valor marginal. Perfis mínimos/profundos são **hipóteses a medir**, não limites arbitrários de resposta.

**Gate R2:** reduzir falsos positivos em conjunto novo sem perda relevante de recuperação/sucesso, latência p95 e custo de contexto controlados; comparação com V2 sem módulos e concorrentes. Rollback se qualidade cair.

### R3 — Task Evidence Brief / contexto operacional [P0]

**Problema:** entregar símbolos corretos ainda obriga o agente barato a montar sozinho mapa de dependências, contratos e testes.

**Entregas:**
1. Sem novo motor: a saída do `ContractEvidence` + `RepoFingerprint` + `IntentRouter` + `EvidencePacker` vira um brief **compacto, rastreável e orientado pela tarefa**.
2. Campos opcionais: `goal`, `relevant_entrypoints`, `confirmed_contracts`, `related_tests`, `change_surface`, `uncertainties`, `verification_hints`. Qualquer afirmação deve ter caminho/linha/commit ou classificação de hipótese; nunca inventar regra de negócio.
3. Seleção por utilidade: um brief não duplica snippets e só substitui/acompanha o contexto quando economiza descoberta; links e detalhes sob demanda. Falhar silenciosamente se não houver evidência suficiente.
4. Medir tokens extras do brief, tempo até primeira edição correta, tool calls, cobertura de requisitos, regressões e testes ocultos.

**Gate R3:** em tarefas de edição de contratos, `V2+Brief` melhora ou preserva qualidade e reduz esforço total versus V2, no corpus externo; em tarefas triviais o custo adicional é nulo ou marginal.

### R4 — Task Grooming / Clarification Policy [P0 após R3]

**Problema:** pedidos vagos ou contraditórios causam implementação errada e retrabalho; perguntar tudo também custa tempo e tokens.

**Entregas:**
1. `TaskUncertainty` como etapa leve no pipeline, **não novo agente**: separar intenção da tarefa de seu grau de especificação. Estados: `clear`, `underspecified`, `ambiguous`, `contradictory`, `contract_conflict`; aceitar `unknown`. Múltiplos estados podem coexistir.
2. Evidências por lacuna: qual decisão falta, quais alternativas são compatíveis com o repo, qual consequência concreta de errar, como afeta patch/testes. Dados do prompt e de fontes confirmadas separados.
3. Política de pergunta por **impacto × incerteza × custo do retrabalho**, descontando custo de interação/espera; perguntar **apenas** quando resposta mudar materialmente comportamento, arquitetura, autorização, estados, persistência ou critérios de aceite. Preferir **uma pergunta de alto valor por vez**; não perguntar sobre algo que o repo já responde. Limite experimental pequeno de perguntas, adaptado à urgência declarada.
4. Injeção compacta orientando o **agente principal** a usar ferramenta nativa de questionamento, se existir; não assumir que `UserPromptSubmit` consegue chamar a ferramenta nem interromper execução. Fallback textual, ou premissa explícita se o risco permitir. Não bloquear tarefas claras, não forçar entrevista em CI/cloud não interativa.
5. Registrar respostas do usuário como decisões explícitas e invalidáveis por sessão/tarefa; não misturar com inferências do agente. Continuar execução com o mínimo de reenvio.
6. **Benchmark de esclarecimento:** tarefas intencionalmente incompletas, contraditórias e plenamente especificadas; respostas padrão do usuário/oráculos definidos antes; medir qualidade de perguntas (necessária/relevante/evitou erro), perguntas desnecessárias, custo de espera, tempo de implementação, retrabalho, sucesso oculto e regressão.

**Gate R4:** benefício por tarefa correta após contabilizar perguntas e espera; nenhum aumento sistemático de perguntas irrelevantes; suportes e limites documentados por Claude/Codex/Antigravity. Incerteza sem evidência não vira veto automático.

### R5 — Change Impact, Invariants e Verification Guidance [P1]

**Problema:** o modelo pode localizar código correto, mas deixar consumidores, contratos, testes e riscos fora do patch.

**Entregas:**
1. Estender somente relações existentes de código/teste/docs/Git/LSP com evidência para blast radius por alteração, invariantes locais e cenários negativos relevantes; distinguir `confirmed`, `candidate`, `unknown`.
2. Diff-aware: avaliar o escopo proposto/alterado e sugerir testes existentes + lacunas de verificação com custo proporcional. Sem executar comandos arbitrários automaticamente.
3. Antes da edição: fornecer dependências críticas, estados e verificações. Depois: conferir evidências de regressão, escopo e falhas de testes. Nenhum texto genérico de "boas práticas" sem vínculo na tarefa.
4. Comparar com `V2+Brief` e `V2+Brief+Impact` em tarefas multi-arquivo de risco controlado.

**Gate R5:** mais testes/invariantes preservados e menos alterações desnecessárias por tarefa corretamente resolvida, sem perda de velocidade total no corpus relevante.

### R6 — Session Progress / Loop Guardian [P1]

**Problema:** modelos econômicos repetem busca, releitura e tentativa sem evidência nova.

**Entregas:**
1. Evoluir o `SessionObserver` já existente para detectar estagnação apenas por eventos observados: buscas repetidas, arquivos revisitados, erros iguais, ausência de nova evidência; não interpretar ausência de eventos como prova de ausência de trabalho.
2. Modo passivo inicialmente: métricas de novidade, redundância e mudança efetiva de hipótese. Distinguir iteração útil, correção de teste e loop improdutivo.
3. Intervenções somente quando houver **nova evidência verificável e mecanismo real de entrega**; limitar frequência; evitar cancelar ações, reescrever saídas de ferramentas ou bloquear o agente. Fallback para próximo prompt.
4. Avaliar em tarefas longas, retomada pós-compactação, debugging e sessões multiarquivo. Não generalizar ganhos de simulação para LLM.

**Gate R6:** menos tool calls repetidos sem prejudicar resolução; custo de observação inferior ao ganho; falsos alarmes baixos, medidos.

### R7 — Studio, adapters, integração, distribuição [P1/P2]

**Entregas:**
1. **Sidecar Studio estático/local:** consumidor dos mesmos JSON/traces de R1–R6; visualizar intenção, candidatos, scores, exclusões, evidências entregues, decisões, perguntas, testes, linhas de tempo, custos e diff de variantes. Não inserir HTML no prompt nem criar daemon obrigatório.
2. Adapters reais Claude/Codex/Antigravity CLI com matriz de suporte por evento, ferramenta de perguntas, cloud/local, permissão e fallback. Verificar hooks de tool use e PreCompact antes de prometer comportamento.
3. LSP/docs opt-in: medição de custo/ganho, consentimento, trust, versão exata de bibliotecas e rollback. Não ativar por padrão só porque há cliente pronto.
4. Instalação/upgrade/desinstalação cross-platform, segurança, licença, release/checksums e documentação atualizada; comparação justa contra CodeGraph e buscas nativas.

**Gate R7:** produto instalável/reversível, sem perda de funcionalidade sem serviços opcionais; UX de inspeção útil e comparabilidade de benchmarks auditada.

## 5. Critérios universais para cada PR

**Obrigatório:** (a) hipótese operacional e classe de tarefa, (b) inventário de reuso, (c) duas referências externas no máximo como aprofundamento por tema com SHA/licença/testes, (d) implementação mínima e flags, (e) testes e baseline por SHA, (f) ablação + casos negativos + latência/custo, (g) execução real quando hipótese é agentic, (h) efeitos adversos e rollback, (i) docs/roadmap atualizados.

**Métricas primárias:** pass@1 em oráculo independente; **custo por tarefa correta**; **tempo por tarefa correta**; mudanças fora do escopo/regressões. Secundárias: primeira evidência efetivamente usada, calls/rounds, tokens por etapa, latência do hook, índice, falsos positivos, falsas perguntas, uso do contexto, incidência de loops e intervenção humana. A não significância/baixo N é resultado inconclusivo, jamais "ganho comprovado".

Sequência científica: **reproduzir defeito → hipótese → baseline congelada → pequena mudança → replay + ablação → avaliação externa → piloto agentic pareado → decisão de promover/reverter**. Nunca ajustar no holdout já visto; gerar outro blind holdout e versioná-lo sem revelar o gold ao agente.

## 6. Referências OSS prioritárias por experimento

Reusar [reference-implementations.md](reference-implementations.md) para CodeGraph (ambas as implementações), Serena/SolidLSP, Context7, Repomix, Headroom, RTK, Claude-Mem, Beads, Superpowers, Agent Retrieval Bench, ContextBench e Aider RepoMap. Além disso, investigar **antes da R4**, com SHAs pinados e licenças verificadas por arquivo:

| Referência | Partes a estudar | Uso proposto |
|---|---|---|
| [GSD](https://github.com/gsd-build/get-shit-done) | `get-shit-done/workflows/discuss-phase-assumptions.md`, `agents/gsd-assumptions-analyzer.md` | Evidência antes de questionar, premissas graduadas, custo concreto de erro; **não** importar o workflow todo |
| [GitHub Spec Kit](https://github.com/github/spec-kit) | `templates/commands/clarify.md` | Taxonomia de lacunas, perguntas de alto impacto, integração com requisitos; sem gerar specs extensas por default |
| [OpenSpec](https://github.com/Fission-AI/OpenSpec) | `src/core/templates/workflows/verify-change.ts` | Verificar implementação contra escopo/contratos sem inventar cobertura; sem obrigar artefatos novos por tarefa |
| [Ask-or-Assume](https://github.com/nedwards99/ask-or-assume) | Código, datasets e protocolo após verificar acesso/licença | Hipóteses e métricas de perguntar vs assumir; não confiar em métricas de terceiro como próprias |

**Não copiar código indiscriminadamente.** Anotar URL, SHA, arquivos, licença, decisão de integração/adaptação ou implementação independente e testes comparativos. Atenção a componentes GPL/BUSL/mistos.

## 7. Próxima tarefa concreta para o agente

1. Após concluir o PR #6, **não reimplementar** o que ele entregou.
2. Abrir R1 numa branch/PR próprio: qualificar edição real isolada; novo corpus e oráculos; replay offline e perf; consolidar scorecard. Resolver pendências P0 da V2 conhecidas no contexto da mensuração.
3. No final de R1, publicar baseline para R2 e R3 com reproduções, custos e regressões. Não promover V2 nem Task Grooming antes de avaliação apropriada.
4. Após R2, implementar R3 e R4 como **incrementos sobre os pacotes atuais**; comparar OFF / V2 / V2+Brief / V2+Brief+Grooming em tarefas e modelos equivalentes.
5. Documentar qualquer desvio de escopo e dependência antes de mudar o plano. Roadmap descreve intenção; Git, testes e scorecards descrevem realidade.

**Definição de pronto final:** todos os contratos essenciais da V2 integrados, adapters compatíveis validados, recuperação seletiva e segura, grooming útil em tarefas ambíguas, qualidade de patch demonstrada por oráculos, observabilidade e distribuição reversível. Nenhuma promessa de ganhos quantitativos sem experimento reproduzível.


## 8. Transição para a V3 (após PR #18)

O plano acima cobre a V2 e os milestones R1–R7. A campanha V3 **não é substituição retroativa nem resultado implementado**. Baseline da transição: PR #18 mergeado em `main` no commit `31279c3c1db35b4571cea3f6f0361c5d6e9bd40e`, corpus de 32 tarefas com 9 holdout seladas, piloto de duas voltas N=6 inconclusivo. O holdout não foi rodado; as 12 tarefas novas não têm medição agentic. Consulte:

- [Contrato/arquitetura e prioridades V3](v3/README.md)
- [Referências externas auditadas, SHAs, métricas e caveats](v3/upstream-references.md)
- [Evals competitivos, anti-contaminação e gates de promoção](v3/evaluation-protocol.md)
- [Instruções executáveis para Codex](v3/CODEX-HANDOFF.md)

Não marcar a V3 default sem melhora de qualidade e custo/tempo por tarefa correta com testes end-to-end válidos. Atualizar os benchmarks através de `bench/registry.json` + `bench/consolidate.py`, não editar o índice gerado à mão.
