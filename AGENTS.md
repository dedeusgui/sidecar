# Sidecar — contrato de engenharia para agentes

> Fonte única de instruções de desenvolvimento deste repositório. Leia antes de alterar código, testes, benchmarks, documentação ou distribuição. Não confunda estas instruções para **contribuir no Sidecar** com instruções que o Sidecar injeta nos repositórios dos usuários.

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

- `cmd/sidecar`: CLI install, uninstall, doctor, status, explain, hook e index.
- `cmd/sidecar-eval`: avaliação offline das fixtures e ablações.
- `internal/adapter`: contratos de hook Claude Code / Codex.
- `internal/app`: orquestração, índices em segundo plano, diagnóstico.
- `internal/repo`, `gitx`: descoberta de raiz/worktree, mudanças e histórico.
- `internal/extract`: parsing e símbolos; `internal/index`: sincronização incremental.
- `internal/store`: SQLite/FTS5; `internal/lexical`: normalização e dicionário.
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

## Comandos oficiais

```bash
go test -count=1 ./...
go test -race -count=1 ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/
go run ./cmd/sidecar-eval --cases bench/cases.json --out /tmp/sidecar-eval.json
go run ./cmd/sidecar explain --root . --json  # lê prompt de stdin
go run ./cmd/sidecar doctor
```

`go test -race` depende do ambiente e não é gate Windows/macOS neste CI. `/tmp` é exemplo POSIX; escolher diretório temporário adequado em outros SOs. `sidecar explain` pode escrever índice no cache do usuário; usar ambiente isolado para experiências. Não rodar tarefas de benchmark agentic pagas em CI por padrão.

## Política de avaliação

Ler `docs/evaluation-plan.md` antes de declarar ganho. Reportar **retrieval offline** separadamente de **resultado end-to-end com agentes**. Use baseline Sidecar off, mesmas tarefas/commit/ambiente/permissões/modelo, ordem balanceada, medição do overhead e orçamento; incluir falhas, casos negativos, tamanho do corpus e intervalos de incerteza. Proibido mexer em holdout após observar os resultados e continuar chamando-o de cego.

## Política de compatibilidade e extensões

Hooks são contratos versionados de terceiros: não presumir que Codex, Claude e Gemini tenham schemas ou UI idênticos. Exigir testes de contrato e E2E reais por cliente. Recursos de UI (status line, plugin/mod), skills ou integrações extras devem ser **opt-in**, reversíveis, sem substituir statuslines ou configurações existentes. Não prometer execução de interface não verificada.

## Navegação

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
