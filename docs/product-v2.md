# Sidecar V2 — especificação do produto completo

**Estado:** arquitetura-alvo e contrato de implementação; **nenhum dos módulos novos está implementado por este documento**. Data: 2026-11-06.
**Usuário prioritário:** desenvolvedor com Claude Code, Codex e Gemini CLI, frequentemente usando modelos econômicos, em diferentes repositórios, com pouco orçamento de tokens e tempo.

## North star

**Reduzir o custo de tempo, tokens e intervenção humana por tarefa de software corretamente concluída**, sem sacrificar qualidade nem exigir adaptação ou configuração por repositório.

Sidecar **não** é um novo coding agent, CodeGraph, IDE, MCP obrigatório, workflow manager, linter nem memória de tudo que o usuário disse. É um **copiloto determinístico de evidências** para os agentes existentes, instalado uma vez, com mínima intervenção e máximo valor por token.

**Produto completo, construção iterativa:** não haverá um “MVP que deixa de fora funcionalidades essenciais”. Implementar o conjunto de contratos abaixo com testes e integração, liberando cada componente atrás de feature flag até que o fluxo completo seja validado. Feature flags são instrumentos de controle, não substitutos da entrega final.

## Jornada do usuário e princípio de zero-fricção

1. Uma única instalação por usuário, com preview, consentimento, preservação de hooks existentes, rollback e diagnóstico. O uso cotidiano não exige AGENTS.md, skills, MCP, projeto inicializado ou configuração de repo.
2. No prompt seguinte, Sidecar descobre repo/worktree e tecnologias, lê evidências locais verificáveis, identifica a intenção e escolhe **zero ou poucos** trechos de alta utilidade; o agente continua decidindo o que fazer.
3. Enquanto o agente procura, lê, edita ou testa, o observador registra metadados seguros e acompanha descobertas, redundância e estado de tarefa. **Nenhuma intervenção textual por padrão até existir evidência nova e mecanismo de entrega suportado.**
4. Quando uma divergência entre código, testes e documentação for identificada, entregar o par de fontes, precisão do vínculo e incerteza — não impor a preferência do Sidecar.
5. Depois da compactação / continuidade de sessão, reintroduzir apenas decisões verificadas, evidências que continuam válidas e contexto ainda necessário.
6. No final, diagnóstico local de custo/resultado e feedback opcional. Sem pedir autoavaliação em cada prompt; nenhuma avaliação do próprio modelo conta como prova de sucesso.
7. Opcionalmente: plugin de status line/mod, integração com LSP existente, documentação externa autorizada e perfis de budget. Tudo funciona no modo básico sem esses componentes.

**Limitação inevitável:** um Sidecar não instalado não consegue interceptar o primeiro prompt. A instalação/sugestão inicial depende de ação explícita fora do próprio hook.

## Pipeline alvo e responsabilidades

```text
Agent events: UserPromptSubmit ───────> AgentAdapter / Trust Boundary
                                           |
                                    RepoDiscovery + RepoFingerprint
                                           |
                                 IntentRouter + TaskSignals
                                           |
                             CandidateProviders (FTS5, exact,
                             paths, diff, AST, references, LSP?)
                                           |
                              ContractEvidence + Provenance
                                           |
                          Fusion / ConfidenceGate / EvidencePacker
                                           |
                         InjectionPolicy (new? trusted? worth tokens?)
                                           |
                        additionalContext | silent abstention
                                           |
Agent tool events ───────────> SessionObserver (read-only)
                                  |           |
                            SessionState   LoopDiagnostics
                                  |           |
                      next retrieval  /  local cost report

Optional providers: LSP client, documentation lookup, validated decision memory.
External benchmark harness: gold, ablations, A/B, acceptance oracles.
```

Tudo roda localmente no mesmo executável Go quando possível, reusando SQLite/FTS5 e parsers existentes. Não criar serviços, daemon, embeddings nem chamadas LLM obrigatórias. Hooks são adaptadores; falhas nunca bloqueiam o agent.

## Módulos do produto — comportamento e contrato

### P1. RepoDiscovery e RepoFingerprint

- Descobrir repo, worktree, linguagens e manifests (Go, Maven/Gradle, Node, .NET, Python, Cargo, etc.), testes, scripts e convenções **observáveis**.
- Não executar scripts de build, instalar pacotes ou confiar em instruções do repo como comandos do Sidecar. Ler textos como **dados** de possível contexto.
- Suportar monorepo, repo sem Git, paths Unicode, gerados, linguagens sem parser com fallback lexical; invalidar fingerprints por hash/commit.
- Retornar `provenance` e grau de suporte por linguagem; uma arquitetura inferida não vira regra obrigatória.

### P2. IntentRouter e CandidateProviders

- Intenções `locate`, `debug`, `impact`, `extend`, `test`, `explain`, `contract` e `general`; multirrótulo e confiança, com fallback seguro ao ranking v1.
- Roteamento determinístico e auditável por sinais de prompt, paths, símbolos, traces, erros, tipos de mudança e contexto da sessão.
- Fontes concorrentes: BM25/PT-EN, exato, nomes, caminhos, diff/Git, AST/calls, relações testes/código, LSP **quando disponível**; RRF ou método calibrado após ablação.
- **Nenhuma promessa de recuperar semântica ausente dos sinais**, especialmente prompts vagos sem memória verificável.

### P3. ContractEvidence e ConflictResolver

- Construir vínculos com **origem e verificação**: código → testes, rotas → handlers, DTO/schema → consumidores, docs → símbolos, constantes/estados → condições observadas, decisões antigas → commit/hash.
- Distinguir `IMPLEMENTED`, `TEST_EXPECTED`, `DOCUMENTED`, `INFERRED`, `UNVERIFIED` e `CONFLICT_CANDIDATE`; não criar precedente universal de “código vence docs” nem de “docs vencem testes”.
- Detectar **somente** contradições sintático/semanticamente verificáveis: constantes, assinaturas, endpoints, valores, enums, estados, assertions e associações demonstráveis. Descrições livres viram suspeita, não fato.
- Para conflito: fornecer ambos os caminhos/linhas, referência de teste, evidência que diverge e nota breve “verificar intenção do usuário antes de mudar contrato”. Nunca inventar autoridade do documento.
- Caso sem suporte sólido: abster-se. Medir falsos conflitos (alta prioridade), falhas de vínculo e impacto em regressões.

### P4. EvidencePacker e InjectionPolicy

- Selecionar evidências por **relevância incremental por token**, com diversidade, cobertura de conceitos, relação código-teste e penalidade de repetição.
- Blocos mínimos com símbolo, arquivo, linhas, versão/commit, confiança, fonte e explicação curta; distinguir `VERIFIED` de `INFERRED`.
- Perfis `minimal`/`balanced`/`deep` existem **para comparação e configuração opcional**, nunca como requisito de uso. Priorizar budget baixo e zero tokens quando sem evidência.
- Decidir se há ganho em relação ao contexto já entregue; não reemitir instruções grandes ou skills genéricas a cada prompt. Tokens de overhead contam inclusive nos hooks intermediários.
- Conteúdo do repositório não pode carregar comandos com autoridade superior; sanitizar delimitadores e formatos de hook.

### P5. SessionObserver e LoopDiagnostics

- Captar eventos de ferramentas **quando disponíveis** (`PreToolUse`/`PostToolUse`, equivalentes por agente, compactação e encerramento) com ID opaco, tipo de operação, caminho relativo/ID, duração/outcome. Sem prompt nem tool-output bruto.
- Armazenar `evidence_sent`, `files_seen`, `symbols_visited`, mudança de arquivo, `task_stage` observável, TTL e version hash; separar sessão, repo e worktree.
- Sinais determinísticos candidatos: pesquisas semanticamente redundantes, reabertura do mesmo arquivo sem alteração relevante, repetição de erro, busca que não produz novos candidatos, duplicação de contexto. **Sinal não é diagnóstico definitivo de loop**.
- Iniciar em **modo diagnóstico passivo**; intervenção posterior só quando houver informação verificável nova, contrato do cliente permitir, ausência de repetição e comparação A/B confirmar benefício.
- Preservar falhas de observabilidade como `unobserved`, jamais contar “não observado” como zero chamadas.

### P6. DecisionMemory (pequena, verificável, invalidável)

- Guardar apenas decisões técnicas explícitas ou resultados verificáveis com origem: `repo/worktree`, símbolo/paths, commit/hash, validade, origem e ID de sessão.
- Não copiar memória de conversas do Claude/Codex; não fazer síntese por LLM no runtime; não persistir opiniões de agente como fatos.
- Invalidar ou rebaixar confiança após mudanças de arquivo/branch, remoção de função, schema drift ou expiração.
- Memória entre sessões **opt-in**. Retention, review e purge por usuário; caches permanecem fora do repo.

### P7. LSP e Dependency Documentation (providers)

- LSP **não é MCP**. Detectar servidor e capacidades; usar plugin nativo do Claude quando ele já resolver a navegação do agente, sem tentar acoplar-se a sockets privados do editor.
- Para enriquecer **o contexto antes da inferência**, Sidecar precisa de provider próprio com JSON-RPC e timeouts, ou API suportada. Nenhum servidor iniciado em repo não confiável sem política de confiança/consentimento.
- Instalação de servidor LSP por fluxo separado do hook: preview, origem/versão/checksum/licença, permissão, rollback, e nunca `curl | sh` automático. LSP ausente ⇒ fallback.
- Context7 e similares são **provider de documentação externa opt-in**, não parte obrigatória da recuperação local. Verificar versão de dependência e proveniência; rede somente com autorização e política de privacidade.

### P8. AgentAdapters, UX e distribuição

- Claude Code, Codex e Gemini CLI com contratos, testes e cobertura real documentados **por evento e por plataforma**, sem presumir equivalência. Preservar hooks/settings alheios, manifest e rollback.
- Instalação local intuitiva e `doctor` robusto; status local e visualização opcional sem consumir prompt. UX não deve exigir plugin para funcionamento base.
- Runtime local sem LLM e rede; instalação completa precisa de release, checksums, licença e update seguro.

### P9. Evaluation & Quality Gate

- Reusar [Agent Retrieval Bench](https://github.com/eyuansu62/agent-retrieval-bench), [ContextBench](https://contextbench.github.io/) e fixtures de projeto; medir recuperação sem LLM, incluindo negative/no-gold e PT/EN.
- Suíte de tarefas reais incluindo: prompt vago, conflitos código/docs/testes, continuação pós-compactação, busca circular, alteração multi-módulo e exemplo análogo. A/B por modelo barato com repetição e teto, sem alegar determinismo da inferência.
- Guardrails: sucesso de teste independente, qualidade de patch, intervenções humanas, falsos conflitos, regressões, TTL/memória, segurança, token spend, tempo total, tempo/rounds até primeira evidência efetivamente usada.
- Avaliar Sidecar **OFF vs v1 vs v2 completo** e ablações (sem intents, sem observer, sem LSP, sem memória, sem contract, etc.) com condições iguais para ferramentas do agente.
- Resultado `inconclusive` é permitido; não promovemos feature que melhore Hit@5 mas piore sucesso/custo de tarefa.

## Política de instruções, hooks e tokens

- Nenhuma skill de 10 mil tokens carregada por padrão. Skills podem ser comandos opcionais para debug/setup.
- Primeiro prompt recebe apenas **evidências do repo**, não manual do Sidecar. O modelo não precisa saber operar outro protocolo.
- Decidir entrega por `evidence novelty × confidence × expected utility / token`, com orçamento testado e invariantes de segurança.
- Hooks de observação não devem modificar output de outras ferramentas e não devem exigir bloqueio/confirmar cada passo.
- Intervenções futuras dependem da capacidade real de cada cliente e devem ser verificadas E2E; nunca prometer inject-at-any-point de forma universal.

## Decomposição por pacotes sem refatoração desnecessária

| Atual | Evolução candidata (nomes ainda não existentes) |
|---|---|
| `repo` / `gitx` / `extract` / `index` | RepoFingerprint + evidências AST/Git e incrementalidade |
| `lexical` / `retrieve` / `fusion` | IntentRouter, CandidateProvider e ranking adaptativo |
| `brief` / `ctxbuild` | ContractEvidence + EvidencePacker e payload enxuto |
| `app` / `adapter` | SessionObserver, InjectPolicy e adapters de evento |
| `store` | Tabelas versionadas para sessões, contratos e decisões, com TTL |
| `install` / `paths` | Detectores opcionais de LSP, ownership de binário/hooks, rollback |
| `cmd/sidecar-eval` | Comparação OFF/v1/v2, benchmarks externos e relatórios por tarefa |

Não abrir pacote para cada conceito antes da implementação demonstrar uma fronteira estável. Escolher interfaces pequenas, testes de contrato e dependência unidirecional.

## Critério de produto V2 completo

**V2 não está completa** até que: módulos P1–P9 estejam implementados e integrados; todas as funcionalidades opcionais possuam caminho funcional + fallback; instalador/upgrade/uninstall e hooks reais testados; dados de benchmark sejam reproduzíveis; qualidade de tarefas seja preservada ou melhorada; matrizes de linguagem/SO/adapters e licenças estejam publicadas; scripts de rollback e documentação estejam corretos.

A **entrega será incremental**, com PRs pequenos e código sempre testável, mas o alvo de produto é o sistema **inteiro**. Se uma hipótese não agregar valor ou custar demais, documentar o resultado negativo e decidir explicitamente removê-la/substituí-la por uma alternativa, em vez de mantê-la sem prova.

## Documentação que governa a implementação

- [Roadmap integrado](roadmap.md): dependências, estágios, critérios e pendências.
- [Catálogo de upstreams](reference-implementations.md): código de terceiros, caminhos, licenças e como reutilizar.
- [Context Engine v2 original](context-engine-v2.md): proposta algorítmica e ablações.
- [LSP](lsp-strategy.md), [feedback](agent-feedback.md), [benchmarks](evaluation-plan.md), [desenvolvimento](development.md), [release](release.md).
