# Sidecar

Contexto de repositório para Claude Code e Codex, sem modelo, sem rede e sem configuração por projeto.

Depois de instalado uma vez, cada prompt enviado ao agente passa por um hook. O Sidecar identifica o
repositório, mantém um índice local, encontra símbolos, testes e relações ligados ao pedido e acrescenta
um bloco curto `[SIDECAR CONTEXT]` ao prompt. O prompt original nunca é reescrito, e o agente continua
decidindo como trabalhar.

O Sidecar **não** é um agente, um orquestrador, uma memória de conversa nem um substituto do Claude ou do Codex.

## Instalação

**Estado atual:** ainda não há release pública publicada. A descrição a seguir é o fluxo pretendido
**depois** da primeira release. Para desenvolvimento, veja [docs/release.md](docs/release.md) antes de
executar `sidecar install` (o comando altera configurações dos agentes do usuário).

1. Após a publicação de uma release, baixe o binário da plataforma e confira o `checksums.txt` (SHA-256).
2. Execute:

   ```sh
   sidecar install
   ```

   O instalador mostra exatamente o que vai alterar e pede confirmação. Para aplicar sem perguntas, use `--yes`.
   Ele copia o binário para `~/.local/bin` (Windows: `%LOCALAPPDATA%\Programs\sidecar`), adiciona um hook
   `UserPromptSubmit` nas configurações do Claude Code (`~/.claude/settings.json`) e, se o Codex estiver
   instalado, em `~/.codex/hooks.json`. Preserva os demais hooks e configurações, faz um backup do arquivo
   original na primeira alteração e pode ser repetido sem duplicar nada.

3. Verifique:

   ```sh
   sidecar doctor
   ```

Para desfazer: `sidecar uninstall` remove somente as entradas e o binário que o Sidecar criou. Use `--purge`
para apagar também o cache de índices.

## Uso

Não há uso manual: abra um repositório e envie um prompt normalmente. Para entender uma decisão:

```sh
echo "qual o limite de tickets ativos na portaria?" | sidecar explain
```

`explain` mostra os termos extraídos, as traduções usadas, os candidatos com seus fatores, as relações
estruturais, o orçamento de tokens, os tempos por fase e o contexto final. Use `--json` para integração.

| Comando | Para quê |
|---|---|
| `sidecar install [--yes] [--agent claude\|codex\|antigravity] [--statusline]` | configura os hooks, com autorização; `--statusline` (opt-in) adiciona a linha de status do Claude Code |
| `sidecar memory list\|add\|purge [--root DIR] [--json]` | decisões técnicas verificáveis, opt-in: `add --target ARQUIVO\|SIMBOLO --summary TEXTO`; `purge` remove as invalidadas (`--all` remove todas) |
| `sidecar uninstall [--purge]` | remove apenas o que o Sidecar instalou (inclusive a linha de status, se ainda for a do Sidecar) |
| `sidecar status [--root DIR] [--json]` | resumo do índice do repositório |
| `sidecar doctor` | instalação, binário, SQLite/FTS5, integridade do índice, falhas recentes |
| `sidecar explain [--root DIR] [--json]` | rastro completo de um prompt (lê stdin) |
| `sidecar activity [--json] [--limit N]` | últimas execuções do hook: `injected`, `no_evidence`, `skipped`, `partial`, `failed`, com latência p50/p95; nunca mostra o prompt |
| `sidecar statusline` | uma linha para a status line, p. ex. `Sidecar: injected · 4 refs · 18ms` ou `Sidecar: no match` |
| `sidecar lsp status\|doctor\|trust\|setup` | servidores de linguagem (opt-in); veja abaixo |

`hook` e `index` são internos: o agente e o indexador em segundo plano os chamam.

### Servidores de linguagem (opt-in)

O Sidecar descobre servidores LSP instalados (`gopls`, `typescript-language-server`, `pyright-langserver`, `rust-analyzer`, `csharp-ls`, `jdtls`, `intelephense`) sem executar código do projeto. O cliente próprio só inicia um servidor quando **os dois** critérios valem: `SIDECAR_LSP=1` no ambiente e a worktree autorizada com `sidecar lsp trust`. `sidecar lsp setup --language go --dry-run` mostra o plano de instalação (comando fixado, origem, licença, impacto no PATH e rollback); sem `--dry-run`, ele exige confirmação e nunca roda durante um prompt. `sidecar lsp doctor --probe` faz o handshake real. O hook de prompt ainda não usa LSP.

## Como funciona

1. **Descoberta.** A raiz vem do `.git` (ou da pasta, fora do Git). Pastas sensíveis, como a home, nunca são indexadas.
2. **Índice incremental.** Dentro de um repositório Git, o `git status` e a diferença entre o HEAD indexado e o atual
   dizem o que reler. Fora do Git, a comparação é por tamanho e data. Arquivos sensíveis (`.env`, chaves,
   `credentials.json`) têm só o nome registrado; binários e arquivos acima de 512 KB não são lidos; symlinks não
   são seguidos. Cada worktree tem seu próprio índice.
3. **Extração.** Gramáticas tree-sitter (runtime puro Go, `gotreesitter`) para Go, Python, TypeScript/TSX/JavaScript,
   Java e C#. Além de definições e chamadas, são extraídos constantes com valor numérico ou booleano (nunca
   valores de string), imports, rotas de frameworks comuns, tabelas SQL e títulos Markdown.
4. **Recuperação.** Análise bilíngue do prompt (acentos, plural, sufixo `-mente`, camelCase e snake_case), tradução
   PT→EN por dicionário offline e só quando o termo existe no repositório, BM25 do SQLite FTS5, identificadores
   exatos, cobertura de conceitos ponderada por raridade, vizinhos estruturais (membros, chamadas com o tipo de
   vínculo, testes que referenciam o nome), fusão RRF e diversificação por arquivo.
5. **Contexto.** Bloco pequeno com caminhos e linhas, veredito `VERIFIED` / `INFERRED` / `UNKNOWN`,
   seções e relações. Um prompt sem evidência não recebe nada. Cada sessão lembra o que já enviou e só reenvia
   quando o conteúdo muda ou depois de 15 prompts.

**Falhas nunca bloqueiam o agente.** Qualquer erro vira um registro em `~/.cache/sidecar/logs/hook.jsonl`
(sem o texto do prompt) e uma saída vazia. Sincronizações têm um orçamento de 1,5 s; o que sobra vai para um
indexador em segundo plano, com lock e cooldown.

**Git.** Co-alteração anota um item que já casou com o prompt; não cria resultados por conta própria. Alteração
não commitada só reforça símbolos cujo nome contém a palavra do prompt, e aparece na seção `GIT EVIDENCE`.

## Privacidade e segurança

- Nenhuma chamada de rede e nenhum modelo. O binário não executa código do repositório; o Git roda com
  `--no-optional-locks` e `core.fsmonitor=false`.
- Nada é gravado dentro do repositório. Índices e logs ficam no cache do usuário.
- O código do repositório é tratado como dado: o contexto declara que é evidência, não instruções, e marcadores
  de bloco vindos do código são neutralizados.

## Agentes

| Agente | Estado |
|---|---|
| **Claude Code** | Verificado de ponta a ponta nesta versão: instalação com `--yes`, hook real invocado pelo `claude -p`, o modelo respondeu usando o contexto injetado. Formato: `hookSpecificOutput.additionalContext`. |
| **Antigravity CLI** | Adaptador e instalador conforme a documentação oficial (`~/.gemini/config/hooks.json`, grupo `sidecar`, evento `PreInvocation`, contexto em `injectSteps`). Contexto só na primeira chamada de cada turno (`invocationNum` 0; campo assumido, não verificado). **Não executado** contra o CLI real. |
| **Codex** | Adaptador e `hooks.json` conforme o schema de hooks do Codex (`UserPromptSubmit`, `additionalContext`, campos `type`/`command`/`timeout`). **Não executado**: o Codex não está disponível neste ambiente. Precisa de teste local. O Codex só roda hooks que você aprovou na revisão de hooks dele; `sidecar doctor` avisa quando a aprovação falta. |

Plataformas: o código compila sem CGo para Linux, macOS e Windows (amd64 e arm64). O CI da branch
`main` passou em testes Go em Linux, macOS e Windows e em cross-builds nas seis combinações em 2026-11-06.
Isso **não** comprova instalação/hook E2E em todos os sistemas; Codex CLI real permanece pendente.

## Limitações

- **Vocabulário.** O dicionário PT→EN tem 64 termos (com tradução reversa e uma expressão composta). Palavras fora dele (`encomenda`, `cupom`,
  `dono`, `visita`, `controlador`) não são traduzidas, e o Sidecar não recupera conceitos ausentes do prompt.
- **Grafo e Git não provados.** Nos casos medidos, o grafo estrutural não superou a tradução sozinha, e os sinais de
  Git não mudaram os resultados. Veja `docs/benchmark.md`.
- **Idioma.** Análise morfológica simples; não há lematização completa.
- **Escala.** Medido em repositórios pequenos e no PetClinic. Repositórios muito grandes dependem do indexador em
  segundo plano para convergir.
- **Concorrência.** Testada com oito hooks simultâneos sobre o mesmo repositório (sem erros, com contexto parcial
  quando o orçamento acaba). Não há daemon.
- **Histórico.** Commits de formatação em massa são ignorados para co-alteração apenas pelo tamanho; não há detecção
  de mudanças só de espaço em branco.

## Direção de produto: Sidecar V2

O projeto segue funcional na versão atual. A **V2 completa ainda está em desenvolvimento**: uma camada local que antecipa contexto relevante, acompanha navegação dos agentes, reconcilia evidências de código/testes/docs, mantém decisões técnicas verificáveis e reduz busca/rounds/tokens com mínimo de intervenção. **Não é outro CodeGraph nem exige MCP/skills do usuário.**

- [Visão e contratos do produto V2](docs/product-v2.md)
- [Roadmap integral e matriz de todos os testes pendentes](docs/roadmap.md)
- [Implementações open source de referência, caminhos de código e licenças](docs/reference-implementations.md)

As funções novas são **planejadas, não entregues**. Toda alegação de ganho precisará de benchmark offline e A/B agentic, incluindo efeitos negativos.

## Engenharia, benchmarks e roadmap

- [AGENTS.md](AGENTS.md): contrato dos agentes que desenvolvem o Sidecar (não exige regras no repositório do usuário).
- [CLAUDE.md](CLAUDE.md): entrada de instruções Claude Code, reutilizando AGENTS.md.
- [docs/development.md](docs/development.md): mapa do código, testes, depuração e dogfooding.
- [docs/context-engine-v2.md](docs/context-engine-v2.md): arquitetura proposta de recuperação por intenção e contexto por token.
- [docs/lsp-strategy.md](docs/lsp-strategy.md): suporte LSP, coexistência com plugins e instalação segura.
- [docs/agent-feedback.md](docs/agent-feedback.md): observabilidade do loop e feedback parametrizado sem autoaprendizado.
- [docs/evaluation-plan.md](docs/evaluation-plan.md): A/B com modelos baratos, corpus diverso, rounds,
  tempo para evidência, custo e protocolo contra viés de benchmark.
- [docs/landscape.md](docs/landscape.md): Aider, Serena, Continue e fundamentação científica.
- [docs/integrations.md](docs/integrations.md): contratos de hooks e visibilidade UI opcional.
- [docs/release.md](docs/release.md): build, instalação, update e gates de release.
- [docs/roadmap.md](docs/roadmap.md): milestones, prioridades e critérios de aceite.
- [docs/benchmark.md](docs/benchmark.md): **resultados existentes** de recuperação offline.

**Atenção:** os benchmarks atuais não demonstram redução de rounds, tempo ou tokens de um
agente. Essas são hipóteses a testar em experimentos pareados, sem confundir qualidade de
retrieval com resultado de engenharia.

## Desenvolvimento

```sh
go test ./...
go test -race ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/
go run ./cmd/sidecar-eval --cases bench/cases.json
go build -tags 'grammar_subset grammar_subset_go grammar_subset_python grammar_subset_typescript grammar_subset_tsx grammar_subset_javascript grammar_subset_java grammar_subset_c_sharp' ./cmd/sidecar
```

Estrutura: `cmd/sidecar` (CLI), `cmd/sidecar-eval` (benchmark), `internal/` com `repo`, `gitx`, `extract`, `index`,
`store` (SQLite/FTS5), `lexical`, `retrieve`, `fusion`, `brief`, `ctxbuild`, `adapter`, `app`, `install`, `paths`.
Releases: `.goreleaser.yaml` e `.github/workflows/`.

Licença: MIT (veja [LICENSE](LICENSE)).
