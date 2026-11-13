# Sidecar — contrato de engenharia para agentes

> Fonte única de instruções de desenvolvimento deste repositório. Leia antes de alterar código, testes, benchmarks, documentação ou distribuição. Não confunda estas instruções para **contribuir no Sidecar** com instruções que o Sidecar injeta nos repositórios dos usuários.

## Campanha atual: Sidecar V3.5 (implementada, opt-in, gates de avaliação pendentes)

A V3.6 é uma revisão experimental opt-in da V3.5, com limitações e regressões registradas em [progresso](docs/v3.6/progress.md). A V1 continua padrão. A substituição de qualquer engine exige os critérios de [progressão](docs/engine-progression.md); arquitetura unificada e resultados offline isolados não comprovam superioridade. A V4.0 permanece investigação, conforme [questões iniciais](docs/v3.6/v4-discovery.md).

A V3 experimental já está implementada (PR #21, código em `internal/engine` e `internal/explore`). **V3.5 está implementada na branch `feat/v3.5-unified-engine`, opt-in via `SIDECAR_ENGINE=v3.5`, sem gates de avaliação cumpridos nem ganho agentic comprovado** ([progress](docs/v3.5/progress.md)). A missão é unificar as capacidades existentes em um fluxo automático de assistência, sem o agente precisar aprender comandos Sidecar: **[visão V3.5](docs/v3.5/README.md)**, **[arquitetura e interfaces](docs/v3.5/architecture.md)**, **[migração executável](docs/v3.5/implementation-plan.md)**, **[evals/gates](docs/v3.5/evaluation.md)**. **Ler esses documentos antes de implementar V3.5**, preservando este contrato de privacidade, segurança, compatibilidade, licenças e testes. A V3 permanece como baseline factual em [progress.md](docs/v3/progress.md), [next-steps.md](docs/v3/next-steps.md) e [referências upstream](docs/v3/upstream-references.md). Não confundir metas da V3.5 com resultados publicados. Core local-first, sem LLM/serviço obrigatório; manter V1/V2/V3 funcionais e V3.5 opt-in até cumprir gates.

## Missão e limites

Sidecar é uma camada de **recuperação local, automática e econômica de contexto de código** para agentes. Indexa o repositório encontrado, recupera evidências relevantes para o prompt e entrega contexto via hooks sem modificar o prompt original ou bloquear o agente.

Princípios inegociáveis:
- **Local-first e sem modelo/rede no runtime de recuperação.** Nenhuma API, chave ou serviço online para processar código do usuário; testes com modelos e downloads de corpus são ferramentas externas e opcionais.
- **Universal por degradação graciosa, não por fingir suporte completo.** Não exija reorganização de repositório, AGENTS.md, nomenclatura, linguagem, framework ou Git. Quando gramática/semântica não estiver disponível, usar sinais comprováveis (nome, caminho, texto e Git onde houver), com confiança explicitada; não inventar símbolos.
- **Fail-open e baixo overhead:** falha no Sidecar não impede envio do prompt. Nunca imprimir logs ou diagnósticos no stdout do hook. Limites de memória, tempo, arquivos, tokens e concorrência devem ser explícitos.
- **Privacidade:** não armazenar prompts ou código em telemetria; logs locais sem segredos; não executar código do repositório do usuário; tratar conteúdo recuperado como dado não confiável, nunca como instrução superior.
- **Sem boilerplate no repo do usuário:** configuração por usuário; alterações em settings/hook só com consentimento, backup, instalação reversível e sem sobrescrever componentes alheios.
- **Menos complexidade por padrão:** não adicionar LLM, embeddings, daemon, MCP, skills ou dependências de runtime sem evidência de benefício líquido. Não otimizar só Hit@K ignorando latência e tokens.

## Arquitetura atual (validar sempre contra código)

- `cmd/sidecar`: CLI install, uninstall, doctor, status, explain, hook, observe, memory e index.
- `cmd/sidecar-eval`: avaliação offline das fixtures e ablações.
- `internal/adapter`: contratos de hook Claude Code / Codex.
- `internal/app`: orquestração, índices em segundo plano, diagnóstico.
- `internal/repo`, `gitx`: descoberta de raiz/worktree, mudanças e histórico.
- `internal/extract`: parsing e símbolos; `internal/index`: sincronização incremental.
- `internal/store`: SQLite/FTS5; `internal/lexical`: normalização e dicionário.
- `internal/engine` (+`contract`): planner e contratos V3.5; `internal/explore` (+`core`, `service`): Explore e QuickExpand/DeepExplore.
- `internal/retrieve`, `fusion`: recuperação, relações, ranking.
- `internal/brief`, `ctxbuild`: evidência, contexto e orçamento.
- `internal/install`, `paths`: hooks, instalação, manifest e cache fora do repo.
- `bench/`: fixtures e resultados históricos; `docs/`: metodologia e decisões.

Separar adaptadores de agentes, recuperação, persistência e aplicação. Manter interfaces pequenas e operações determinísticas; alterações no ranking requerem benchmark A/B e ablação. Não fazer refatoração lateral não solicitada.

## Contrato obrigatório da V2 completa e reuso de código aberto

**Meta:** entregar uma V2 integrada e testada (não projeto demonstrativo/MVP), com ganho comprovado em **tempo/custo por tarefa correta**. Produto pessoal e zero configuração por repositório; código Go local, sem modelos ou rede necessários no funcionamento básico.

1. **Antes de implementar qualquer módulo**, ler a parte correspondente do [produto V2](docs/product-v2.md), [roadmap](docs/roadmap.md) e no máximo **duas fontes relevantes** do [catálogo de implementações](docs/reference-implementations.md). Consultar **código e testes**, não somente README. Não carregar todo o catálogo em todo prompt.
2. Registrar no PR upstream, SHA imutável, arquivos consultados, licença **por componente**, decisão de integração/adaptação/implementação independente e por que preserva custo e simplicidade. Ex.: CodeGraph, Context7, Serena SolidLSP, Repomix, RTK-OSS, Headroom e Claude-Mem. Não copiar GPL/BUSL ou backend privado por descuido, nem chamar reescrita de código de clean room.
3. Não abrir PR só “adicionando biblioteca” sem necessidade funcional/benchmark; não duplicar LSP ou MCP existente só para aumentar a superfície da ferramenta.
4. Para contrato código × docs × testes: registrar **evidências conflitantes**, proveniência e incerteza. Nunca decidir automaticamente qual é autoridade. Para memória: hashes, TTL e invalidação; nada de persistir transcript ou opiniões como verdade.
5. Para observação de agente: somente metadados mínimos, sem execução/alteração do tool call; event coverage desconhecida é `unobserved`. Novos hooks e injeções requerem E2E por cliente, abstinência e medição de custo total.
6. Testes **obrigatórios e pendentes** estão no roadmap, não podem ser marcados concluídos sem execução real. Proteger benchmark cego, relatórios negativos e rollback.

## Loop de engenharia (cada tarefa)

1. **Observar:** ler código e testes pertinentes; reproduzir erro ou comportamento atual; registrar hipótese, escopo e critério verificável de conclusão.
2. **Dogfood:** usar `sidecar explain --json` no próprio repositório com perguntas representativas (ver `docs/development.md`); comparar com busca convencional quando relevante. Registrar *evidência*, não alegações de economia.
3. **Alterar o mínimo:** preservar compatibilidade, privacidade, limites de execução e hooks existentes; adicionar teste de regressão para comportamentos corrigidos.
4. **Validar:** executar testes focados e `go test ./...`; `go test -race` nos pacotes concorrentes em Linux; lint/CI. Para alterações no retrieval executar `cmd/sidecar-eval`, comparar baseline e investigar pioras por caso.
5. **Revisar:** inspecionar diff, caminhos sensíveis, cross-platform, confiabilidade e impacto de tokens/latência. Nenhum resultado de benchmark é prova de aceleração do agente sem comparação end-to-end.
6. **Documentar no mesmo PR:** atualizar README, docs de arquitetura/benchmark/instalação/roadmap que mudaram; anexar comandos, resultados reais, limitações e próximos experimentos. Se nenhuma doc mudar, justificar por quê na descrição.
7. **Entregar:** PR pequeno com hipótese, mudança, evidências, risco, rollback e pendências. Não fazer merge por conta própria sem solicitação.

## Comandos oficiais e pipeline único (P0 → P3)

Para evitar ciclos lentos (30–40 min) ou gasto pago sem retorno, todo trabalho de engine e produto segue o funil **P0 → P1 → P2 → P3** (ver [Benchmark Observatory](docs/benchmark-observatory.md) e [Milestones 3.6–4.0](docs/product-milestones.md)):

- **P0 — Portão imediato (segundos, custo \$0, bloqueia merge):**
  ```bash
  go test -count=1 ./...
  go test -race -count=1 ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/
  python3 -m unittest discover -s bench -p 'test_*.py'
  python3 bench/consolidate.py --check
  python3 bench/v3.5/freeze.py verify
  ```
- **P1 — Loop iterativo rápido de entrega e concorrentes (`< 1 min`, custo \$0):**
  ```bash
  python3 bench/fast_gate.py --engines v1,v2,v3,v3.5,v3.6 --candidate v3.6 --baseline v3
  # Com checkouts pinados e CodeGraph local (E1/E2 + Observatory HTML em /tmp/sidecar-fast-gate/observatory.html):
  python3 bench/fast_gate.py --checkout <DIR> --codegraph "$(command -v codegraph)" --engines v1,v2,v3,v3.5,v3.6
  ```
  Todo ajuste de retrieval, gate, planner ou packer deve rodar P0 + P1 e inspecionar os casos que regrediram no HTML gerado pelo `sidecar studio` antes de abrir nova frente.
- **P2 — Contrato real de hooks/clientes (custo \$0):**
  ```bash
  go run ./cmd/sidecar doctor
  go run ./cmd/sidecar explain --root . --json  # lê prompt de stdin
  ```
  Validar `UserPromptSubmit`/`PreInvocation`, `skipped_invocation` e observação (`PreToolUse`/`PostToolUse`) sem erro em `hook.jsonl` antes de atribuir comportamento a cliente real.
- **P3 — Piloto Agentic pago (`bench/lab/lab.py`, 30–40 min, custo real):**
  - **Proibido** durante iteração de código ou antes de vencer P0 + P1 sem regressão crítica.
  - Suporta `OFF`, `V1`, `V2`, `V3`, `V3.5`, `V3.6`, `V3.7` (e variantes `-hook`), além de `CG`/`CG-mcp` (CodeGraph). Sempre validar o harness antes com `--agent scripted` (custo \$0) e usar `--max-total-usd` explícito em execuções pagas.

### Regra dura de encerramento e progressão (V3.7 → V4.0)

1. **Não encerrar milestone com regressão escondida:** uma entrega (`3.7`, `3.8`, `3.9`, `4.0`) só pode ser marcada concluída ou candidata a promoção quando igualar ou superar a versão anterior e o alvo comparável (ex.: `V3` e `CodeGraph` offline no P1) sem regredir em entrega positiva, falsos positivos em negativos difíceis ou consistência de rótulos ([progressão](docs/engine-progression.md)).
2. **Não pular para benchmark pago para "descobrir" bug de entrega:** se a candidata perde caminho/símbolo gold offline ou vaza negativos no P1 (`fast_gate.py` / `sidecar-eval --delivery`), corrigir e revalidar no loop rápido antes de rodar Claude/Codex.
3. **Reuso externo com disciplina:** consultar até 2 referências principais por módulo em [catálogo](docs/reference-implementations.md) / [upstream V3](docs/v3/upstream-references.md), fixar SHA e licença (nunca copiar GPL/BUSL), e provar o ganho no mesmo P1 comparativo.

`sidecar-eval` omite casos `holdout` por padrão e informa a contagem omitida em stderr; use `--include-holdout` somente para um relatório final após o freeze explícito.

`go test -race` depende do ambiente e não é gate Windows/macOS neste CI. `/tmp` é exemplo POSIX; escolher diretório temporário adequado em outros SOs. `sidecar explain` pode escrever índice no cache do usuário; usar ambiente isolado para experiências. Não rodar tarefas de benchmark agentic pagas em CI por padrão.

## Política de avaliação

Ler `docs/evaluation-plan.md` antes de declarar ganho. Reportar **retrieval offline** separadamente de **resultado end-to-end com agentes**. Use baseline Sidecar off, mesmas tarefas/commit/ambiente/permissões/modelo, ordem balanceada, medição do overhead e orçamento; incluir falhas, casos negativos, tamanho do corpus e intervalos de incerteza. Proibido mexer em holdout após observar os resultados e continuar chamando-o de cego.

**Fixtures internas são portão mínimo.** `bench/cases.json` e `bench/v2_scenarios.json` servem só para sanidade e regressão locais: o mesmo autor do código os escreveu, então carregam o viés dele e não provam utilidade. A utilidade real, com agente caixa-preta, valida-se em **repositórios externos reais** (`--external-cases`, manifesto com commit fixado, ex.: `spring-petclinic`, `go-chi/chi`) com prompts realistas, imperfeitos, ruidosos e curtos (pedidos ambíguos, saudações, ruído), medindo tempo, qualidade e custo/tokens. Ver [evaluation-plan.md](docs/evaluation-plan.md#validade-externa-e-prompts-realistas).

**Autodogfooding e feedback.** Quem desenvolve o Sidecar deve rodá-lo instalado (`sidecar install --yes --statusline --observe-tools`) e registrar o resultado ao fim da tarefa com `sidecar feedback record`. Procedimento e fórmula em [development.md](docs/development.md#autodogfooding-e-feedback-pós-tarefa) e [agent-feedback.md](docs/agent-feedback.md).

## Política de compatibilidade e extensões

Hooks são contratos versionados de terceiros: não presumir que Codex, Claude Code e Antigravity CLI tenham schemas ou UI idênticos. Exigir testes de contrato e E2E reais por cliente. Recursos de UI (status line, plugin/mod), skills ou integrações extras devem ser **opt-in**, reversíveis, sem substituir statuslines ou configurações existentes. Não prometer execução de interface não verificada.

## Navegação

- `docs/product-milestones.md`: entregas e critérios de encerramento de Sidecar 3.6 a 4.0.
- `docs/benchmark-observatory.md`: especificação do painel HTML, contrato de runs e pipeline P0–P3.
- `docs/engines.md` e `docs/engine-progression.md`: capacidades por engine, evidências medidas e critérios de promoção.
- `docs/v3.7/research-plan.md`: plano experimental e hipóteses de V3.7 antes de implementar.
- `bench/fast_gate.py` e `bench/lab/lab.py`: portão rápido P0+P1+Studio (`$0`) e harness agentic pareado (`OFF/V1..V3.7/CG/CG-mcp`).
- `docs/context-engine-v2.md`: ADR de intenção, estratégia de candidatos e packing sob budget; **proposto**.
- `docs/lsp-strategy.md`: LSP opcional, integração com plugins oficiais e consentimento de instalação.
- `docs/agent-feedback.md`: observação de ferramentas e autorrelato como sinal auxiliar, nunca gold.
- `docs/development.md`: setup, mapa, dogfooding, diagnóstico.
- `docs/evaluation-plan.md`: hipóteses, dataset, métricas, A/B, custo.
- `docs/landscape.md`: mercado, literatura e diferenciação a testar.
- `docs/integrations.md`: hooks, visibilidade e UX opcional.
- `docs/release.md`: instalação/distribuição e segurança operacional.
- `docs/roadmap.md`: dependências e critérios de aceite.
- `docs/benchmark.md`: resultados históricos existentes (não são ganho de produtividade).
