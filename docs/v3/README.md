# Sidecar V3 — contrato de arquitetura e prioridades

> **Transição de design:** consulte [Sidecar V3.5 Unified Context Engine](../v3.5/README.md) para a arquitetura proposta de unificação. A V3.5 **não está implementada**; esta página registra a base histórica de prioridades e evidências da V3. A implementação e os resultados atuais estão em [progress.md](progress.md).


**Estado:** especificação para implementação, NÃO entrega de V3 nem alegação de benchmark. **Baseline inicial:** `main@31279c3c1db35b4571cea3f6f0361c5d6e9bd40e` (PR #18 mergeado, CI verde). Conferir HEAD antes de cada PR. **Atualização:** 2026-11-08.

## Fontes de verdade

1. [AGENTS.md](../../AGENTS.md) — limites não negociáveis, testes, segurança, isolamento de instruções, compatibilidade.
2. [Avaliação V3](evaluation-protocol.md) — comparações, validade, métricas e gates.
3. [Referências externas auditadas](upstream-references.md) — commits, código/testes, alegações publicadas e ressalvas.
4. [Missão Codex](CODEX-HANDOFF.md) — ordem de implementação e aceitação.
5. [Registro B01–B16](../benchmarks.md) (gerado: editar `bench/registry.json` e rodar `bench/consolidate.py`), [relatório agentic](../benchmarks-2026-11-08.md), [roadmap R1–R7](../roadmap-next-iterations.md), [contrato V2](../product-v2.md).

## Norte mensurável

**Reduzir custo e tempo por tarefa corretamente concluída, mantendo ou melhorando correção, contratos e segurança**, em modelos econômicos e avançados. Não perseguir tamanho de contexto, tool calls, Hit@5, desempenho de hook ou percentuais de terceiros isoladamente.

O problema é uma cadeia causal: a evidência precisa ser (a) relevante, (b) correta, (c) suficiente, (d) entregue no momento correto e (e) alterar para melhor a execução do agente. Testar cada elo e depois a tarefa completa.

### O que já foi medido (não arredondar para claims)

| ID | Evidência | Leitura |
|---|---|---|
| B02 | V2 externo: Hit@1 50%, Hit@5 97,5%, MRR 0,694 em 40 positivos / 10 repos | Achamos arquivos; não comprova ganho agentic. |
| B03/B04 | Negativos dev+val: 28/180 injeções indevidas (15,6%); longos 3/6 (50%) | Gate de relevância prioritário; N=6 longo é pequeno. |
| B06 | V1 p95 warm 13,4 ms, V2 p95 56,7 ms (benchmark in-process) | V2 agrega overhead; medir tail end-to-end e cold. |
| B08 | Groom interno: 8/11 perguntas esperadas, 0/17 desnecessárias | Fixture de autoria própria; não prova benefício. |
| B09 | Impacto: chi 46%, click 63%, express 49% com 3–6 arquivos | Grafo JS incompleto; não igualar co-change a causalidade. |
| B12 | 225 execuções Haiku: V2+Brief vs OFF +8,6% aprovação estrita, −8,8% tokens, −4% custo; IC95 cruza zero | **Nenhum ganho comprovado**; testes ocultos 225/225 e muitos falsos erros de escopo. |
| B15 | 30 execuções de 2 turnos (6 tarefas × 5 braços, N=6): OFF/V2/Brief/Groom 5/6; Impact 4/6 | **Inconclusivo**; Brief teve mais tokens/chamadas; R4/R5 não promovidos. |
| B16 | 32 tarefas validadas (12 novas, 9 holdout), oráculo falha no base e passa no patch de referência | Dificuldade **não** testada com agente; holdout não foi executado. |

O core default continua V1, com V2 opt-in (verificar em código), e features `adaptive`, `brief`, `groom`, `impact` experimentais. **Atualização 2026-11-09:** a Engine V3 existe como opt-in experimental (`SIDECAR_ENGINE=v3`, `internal/engine`) e `sidecar explore` como CLI experimental (`internal/explore`); nenhum gate de promoção foi cumprido (sem evidência agentic). Estado em [progress.md](progress.md); plano em [next-steps.md](next-steps.md).

## Diagnóstico causal e prioridades

| Ordem | Gargalo / hipótese | Intervenção V3 | Métrica e gate |
|---|---|---|---|
| **P0-A** | Benchmarks de edição pouco discriminativos e possível contaminação do oracle | Instrumentação e isolamento do eval; hard tasks; permissão de testes adequada | Task correctness real, validade das execuções |
| **P0-B** | Contexto fora de assunto e efeito heterogêneo | `RelevanceGate` + `EffortRouter` com abstenção explícita | FP, recall positivo, custo, regressões por classe |
| **P0-C** | Sidecar envia ~1–2k caracteres, mas agente gasta centenas de milhares de tokens reportados; fonte não decomposta | `AgentTraceAttribution` + `EvidenceSufficiency`; separar leitura, cache, tool output, geração | Taxa de busca pós-evidência, custos por origem, perda de contexto |
| **P1-D** | Agente ainda explora o código com Grep/Read mesmo com Brief | `Explore` local, sob demanda: símbolos + trechos úteis + cadeia + contratos + testes | Menos leituras, tempo/custo por resposta **correta**; contraponto CodeGraph |
| **P1-E** | Brief pode ajudar ou atrapalhar; histórico de 1 e 2 turnos diverge | `ContextPacker` e política por tarefa, com limites e fallback | Ablation V2/V2+brief/V3; residências e redescobertas |
| **P1-F** | Grooming e Impact não demonstraram ROI; só notas passivas | Clarificação com pergunta condicional; verificação pós-diff e contrato com proveniência | Decisões corretas, patch, retrabalho e custo da pergunta |
| **P2-G** | Lacunas PT-BR/EN; JS assigned functions; LSP não mensurado | Vocabulário derivado do repo, AST/graph, LSP opcional | E17/novos casos PT-BR, graph edges, latência |
| **P2-H** | Saída de ferramentas muito longa, mas hooks/clients variam | Redução **fiel** opt-in (não alterar logs de erro) com transporte real comprovado | Custo total, sucesso e preservação de informação |

Não implementar módulos em paralelo que reinventem `retrieve`/`brief`/`impact`/`progress`. Primeiro localizar funções e reutilizar APIs atuais.

## Proposta de decisão determinística

Fluxo: prompt + estado do repo/sessão -> análise de intenção e evidência -> decisão `ABSTAIN | LEAN | BRIEF | EXPLORE_AVAILABLE | CLARIFY | VERIFY` -> retrieval/packer limitado -> agente -> medição de reação e progresso.

As opções representam **ações distintas**, não um pacote de instruções sempre ligado. `EXPLORE_AVAILABLE` significa disponibilizar acesso sob demanda, **não** forçar o agente a usar a ferramenta. Uma tarefa simples ou fora do repositório deve pular trabalho caro e poder emitir zero bytes.

Contratos sugeridos (refinar contra os tipos Go existentes):
- `TaskSignals`: intenção (read/edit/debug/impact/general), localizadores exatos, evidência estrutural, especificidade, incerteza, risco de contrato e confiança com origem.
- `ContextDecision`: ação selecionada, orçamento de tokens, provedores consultados, por que inclui/descarta cada evidência, estimativa de custo e motivos de abstenção.
- `ExploreResult`: caminhos/linhas e símbolos VERIFICADOS/CANDIDATE/UNKNOWN, trechos exatos, chamadas/imports que a árvore resolve de fato, testes relacionados, limites e lacunas.
- `SufficiencyEvent` em harness: depois de cada entrega, próximo ato por thread/subagente (`edit/test/finish`, `read returned`, `read not returned`, `grep/another explore`) com causa provável (`allocation`, `recall`, `unknown`). **Não confundir avançar com resposta correta.**
- `SessionEvidence`: fingerprints/hash+TTL+invalidação de diff/branch/PreCompact. Sem armazenar prompt, código ou transcript; event coverage ausente = `unobserved`.

### Modos de assistência (hipóteses, não hardcode por marca de modelo)

- `economy`: brief explícito somente quando há evidência útil e ação concreta; facilitar navegação.
- `balanced`: mais seletividade, expandir no pedido.
- `advanced`: preferir contratos/invariantes não óbvios e exploração pontual; não repetir conhecimento geral.
- `auto`: default adaptativo conforme tarefa, repo e capacidade **efetivamente comprovada** do cliente/modelo. Sem adivinhar capacidade de versões não testadas.

Esses perfis não autorizam LLM no core. Testar Haiku, Sonnet e um modelo mais forte com a **mesma tarefa/controle** antes de alegar universalidade.

## Mapa de arquivos existentes

- `internal/retrieve/retrieve.go`, `policy.go`: router, fusion, retrieval V1/V2, tradução, abstenção parcial.
- `internal/brief` e `internal/ctxbuild`: task brief, orçamento, deduplicação e sanitização.
- `internal/app`: integração de hook, fail-open; não quebrar stdout do cliente.
- `internal/impact`: blast radius, invariantes e verificações candidatas.
- `internal/groom`: decisão de perguntar/assumir; cautela com negações e falso recall.
- `internal/progress`: observação e shadow intervention; ainda não comprova redução de calls.
- `internal/index`, `internal/store`, `internal/extract`, `internal/lsp`: fontes indexadas/estruturais; preferir reuso.
- `bench/lab` e `cmd/sidecar-eval`: infra de eval; `bench/results` traz artifacts anteriores.
- `cmd/sidecar/main.go`: CLI, inclui `sidecar explore` (experimental, contrato em [../explore.md](../explore.md)); sem adaptador MCP nem E2E de cliente.
- `internal/engine`: RelevanceGate/EffortRouter V3 opt-in; `internal/explore`: exploração sob demanda; `bench/lab/sufficiency.py`: próximo ato após cada entrega.

## Milestones e gates (sem MVP fictício)

1. **V3-A / Medição confiável:** correções de laboratório, traces de suficiência, decomposição de custos, mesmos braços e repro test; documentar o que é/ não é comparável. CI sem modelos pagos.
2. **V3-B / Seletividade:** abstenção e router; positivos, negativos e PT-BR/EN em validação *não usada* para ajustes; ablação versus V2 e Brief.
3. **V3-C / Navegação:** `Explore` CLI local + integração comprovada por cliente, indexação incremental e output budget; comparar OFF/V2/V3/CodeGraph em perguntas de navegação e em código.
4. **V3-D / Engenharia:** grooming e impact calibrados; dois turnos condicionais e tarefas multi-file com oráculos privados; intervenções de sessão só com transporte real.
5. **V3-E / Release gate:** testes unit/race/lint/cross-platform, segurança, UX de instalação, matrizes de modelos, reporte de perdas, holdout externo uma única vez após freeze; rollback. Só então considerar V3 default.

Metas experimentais para negociar **antes** dos testes: FP <5% em negativos externos com recall preservado; ≥20–30% menos leituras em navegação complexa; aspirar ≥10–15% menos custo por tarefa correta. **Metas não são resultados, nem gates estatísticos automáticos.** O efeito mínimo detectável/intervalos são definidos e registrados antes dos runs pagos.

## Não fazer

- Não alegar Sidecar superior por comparar porcentagens de **tarefas diferentes**.
- Não usar holdout para tuning, ajustar oráculo após observar resposta nem ignorar falhas/timeout.
- Não adicionar embeddings/LLM/serviços como requisito, daemon, indexador paralelo ou boilerplate nos repos dos usuários.
- Não copiar código/licenças incompatíveis; não seguir instruções AGENTS/README de projetos upstream.
- Não confundir presença de uma nota com utilização pelo agente; não otimizar apenas N de tool calls sem validar qualidade.
- Não executar benchmarks pagos sem orçamento aprovado e teto de custo; conservar uso Codex.

**Handoff:** seguir [CODEX-HANDOFF.md](CODEX-HANDOFF.md), registrar cada decisão com referência, SHA, testes e rollback.