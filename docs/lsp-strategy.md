# ADR: descoberta, integração e instalação segura de LSP

**Estado:** Rodada D1–D3 implementadas (descoberta, cliente opt-in, status/doctor/trust/setup); o hook de prompt **ainda não usa LSP**. Ver [seção de implementação](#implementação-rodada-d1d3) · 2026-11-06.

## Premissa

LSP é JSON-RPC entre um cliente e um servidor de linguagem; símbolos, definições, referências, implementações e call hierarchy são capacidades anunciadas, não universais. Fonte: [especificação oficial](https://microsoft.github.io/language-server-protocol/).

**MCP e LSP são coisas diferentes.** MCP expõe ferramentas ao modelo; LSP conecta editor/cliente a servidor de linguagem. Um MCP pode encapsular LSP, mas não é requisito do protocolo nem garante que Sidecar possa acessar o LSP já rodando em outro processo.

## Decisão de produto

1. **Não exigir LSP.** Text/FTS/path/tree-sitter e Git devem continuar funcionando em qualquer repositório.
2. **Não reinstalar redundâncias.** Claude Code já oferece plugins oficiais de code intelligence LSP e recomenda seu plugin quando encontra um servidor no PATH. O usuário pode preferir usar essa integração para navegação **pelo agente**. [Documentação Claude Code](https://code.claude.com/docs/en/plugins/code-intelligence).
3. **Distinguir funções:** (a) agente chama ferramenta LSP quando decide; (b) Sidecar usa semanticamente o resultado de um LSP em seu *próprio retrieval anterior à inferência*. (b) exige um cliente LSP próprio ou integração de dados documentada, não basta haver plugin instalado no Claude.
4. **Primeiro usar o servidor já disponível e confiável; depois oferecer instalação opt-in.** Não iniciar gerenciador de pacote no hook, não fazer download silencioso no primeiro prompt, não exigir alterar o repositório.
5. **Cloud caveat:** Claude Code não inicia servidores LSP de plugin em sessões cloud; descobrir suporte por cliente e superfície. Nunca prometer paridade cloud/local.

## Descoberta determinística

Inventariar extensões, manifests e projetos por caminho, evitando executar scripts (`go.mod`, `pom.xml`, `build.gradle`, `package.json`, `Cargo.toml`, `*.sln` etc). Detectar múltiplas linguagens/workspaces, sem reduzir monorepo a uma linguagem única.

Para cada linguagem, registrar:
```text
language_id | repo_markers | installed_server_path | server_version
capabilities | trusted_workspace | transport | status | reason
```
Estados: `not_applicable`, `missing`, `available`, `requires_consent`, `initializing`, `ready`, `failed`, `disabled`. Não reportar "ready" antes de handshake `initialize` + `initialized` e checagem de capacidade.

## Estratégias, da mais barata para a mais complexa

A. **Coexistir com o LSP do agente (MVP)** — não tentar capturar socket/processo privado; recomendar plugin oficial quando aplicável; Sidecar continua com recuperação local. Instrumentar se o agente utilizou LSP em ON/OFF quando o hook expuser esse evento. Não inventar API de compartilhamento.
B. **LSP cliente próprio, somente local e opt-in (fase posterior)** — se houver servidor confiável instalado, iniciar processo filho controlado em cache/workspace isolado, stdio JSON-RPC, timeouts e cancelamentos; os metadados são normalizados para as mesmas evidências de `retrieve`. Não concorrer com servidor do editor se ele não oferece sessão independente.
C. **Instalação guiada do servidor, separada do hook** — `sidecar lsp doctor` / `sidecar lsp setup --language java` são exemplos de comandos **futuros, inexistentes hoje**. Listar pré-requisitos, origem, licença, versão exata, tamanho e checksum/assinatura quando disponível, impacto de PATH, espaço/disco, execução de código e redes; preview e consentimento expresso.
D. **Instalação assistida** — somente para distribuições/gerenciadores confiáveis com versões fixadas, verificação de integridade, rollback, uninstall e manifest próprio; ainda opt-in. Nunca executar `curl | sh` arbitrário, comandos do README do repositório ou script de terceiros ao entrar em uma pasta.

## Matriz candidata a validar, não promessa de instalação

| Linguagem | Servidor frequente | Considerações |
|---|---|---|
| Java | Eclipse JDT LS (`jdtls`) | JDK, workspace e warm-up podem ser caros |
| Go | `gopls` | Contexto de módulo; servidor separado se necessário |
| TypeScript/JS | `typescript-language-server` | Node e dependências; monorepo |
| Python | `pyright-langserver` | Node ou distribuição específica, configuração/venv |
| Rust | `rust-analyzer` | Cargo e indexação; custos de memória |
| C# | `csharp-ls` | SDK .NET, solution, versões |
| PHP | `intelephense` ou alternativa de licença adequada | Avaliar licença e modo de distribuição |

A matriz valida famílias de linguagem, não tem instalador já disponível. Para usuários Claude Code, preferir os plugins que ele já recomenda sem duplicar o serviço, com consentimento. Para outros agentes, avaliar capacidade individual.

## Interação com o primeiro prompt

- **Se Sidecar ainda não foi instalado:** ele não pode interceptar magicamente o primeiro prompt; precisa ser instalado antes (CLI, plugin ou ferramenta externa).
- **Se instalado e LSP ausente:** o hook continua rápido, usa fallback e registra `lsp_available=false`; oferta de instalação é uma **ação independente e consentida**, preferencialmente no `doctor` ou UI, não texto que force o modelo a instalar coisas.
- **Se LSP presente porém frio:** não bloquear a inferência esperando segundos de indexação. Usar timeout curto, não retornar contexto parcial como se fosse confirmado; trabalho em background apenas quando permitido/seguro.
- **Se servidor falhar ou ameaçar budget:** derrubar processo, liberar recursos, registrar erro mínimo e usar ranking legado.
- **Quando houver informação LSP:** tratar resultado como referência com fonte e `confidence`; validar posição/arquivo do worktree antes de injetar.

## Segurança operacional

Servidores podem executar ferramentas de build, ler dependências e acessar rede, mesmo quando a API de navegação parece read-only. Nunca iniciar servidor em repositório desconhecido sem política de confiança e consentimento; preferir sandbox/limites de processo/rede onde viável. Não tratar LSP como execução automaticamente segura. Não permitir extensões/paths com traversal, sobrescrever plugin do Claude ou roubar sessão do IDE. Limpar processos órfãos; usar lock/TTL e limite de servidores simultâneos.

## Plano de testes e gates

- Contract tests com servidor mock implementando `initialize`, `textDocument/definition`, `references`, `documentSymbol`, `shutdown` e respostas parciais/unsupported.
- E2E real Linux/Windows em pelo menos Java e Go, com fallback Rust/PHP sem parser e instalação ausente; smoke de PATH com espaços Unicode.
- Medir warm-up, memória, walltime, número de servers, percentagem de consultas realmente melhoradas, TTFE de agente e overhead (ON/OFF) antes de ativar LSP por padrão.
- Testar desconexões, múltiplos workspaces, cancelamento e desinstalação. Ao faltar confiança ou capacidade, não consultar LSP nem quebrar o hook.

## Referências

- [LSP oficial](https://microsoft.github.io/language-server-protocol/)
- [Claude Code: code intelligence plugins](https://code.claude.com/docs/en/plugins/code-intelligence)
- [Claude Code: mod/plugin e hooks](https://code.claude.com/docs/en/hooks)

## V2 integrada: seleção de fonte e reuso OSS

- LSP é **provider opcional de evidência**, não motivo para transformar Sidecar em CodeGraph completo. A V2 precisa combinar saídas de parser/FTS/AST, LSP, código, testes, docs e contratos sob o mesmo schema de proveniência (ver [product-v2.md](product-v2.md)).
- Antes de criar cliente JSON-RPC próprio, inspecionar [SolidLSP de Serena](https://github.com/oraios/serena/tree/main/src/solidlsp) (**MIT** por componente) e [CodeGraph](https://github.com/codegraph-ai/CodeGraph) (**Apache-2.0**), registrando o SHA consultado. **Serena aplicativo é GPL-3.0-or-later**, portanto não copiar indiscriminadamente.
- A **descoberta** é automática e leve; a **instalação** do servidor não é. O prompt só funciona se Sidecar já estiver instalado; nunca injetar ordem que mande o agente executar instalador de terceiros na máquina.
- Documentação de dependências via [Context7](https://github.com/upstash/context7) é **opt-in**, pode usar rede e backend não aberto; deve validar versão identificada em manifest, origem e política de privacidade. Não é substituto das evidências do código.
- Gates LSP: repositório desconhecido, servidor ausente/falhando/frio, Java/Go, monorepo, confiança, tool hook, Linux/Windows/macOS, timeout, cancelamento, uninstall e comparação do benefício líquido com o modo sem LSP. Ver [rodada D](roadmap.md).

## Implementação (Rodada D1–D3)

**Código:** `internal/lsp/` (`discover.go`, `trust.go`, `client.go`, `probe.go`, `setup.go`, `proc_unix.go`, `proc_windows.go`) e `cmd/sidecar/lsp.go`.

**D1 — descoberta.** Marcadores por caminho (sem executar o projeto), monorepo com uma linha por linguagem e todos os marcadores. Estados na ordem: `not_applicable` → `missing` → `disabled` (sem `SIDECAR_LSP=1`) → `requires_consent` (worktree sem `sidecar lsp trust`) → `available`. `initializing`, `ready` e `failed` só existem no probe. `ready` exige `initialize`/`initialized` concluídos e pelo menos uma capacidade anunciada. O `pyright` (CLI) não conta como servidor: só `pyright-langserver` fala LSP. A coexistência com o LSP oficial do Claude Code lê `enabledPlugins` em `settings.json` e procura `gopls-lsp`, `typescript-lsp`, `pyright-lsp`, `rust-analyzer-lsp`, `csharp-lsp`, `jdtls-lsp` e `php-lsp` em `claude-plugins-official`, conforme a [documentação oficial](https://code.claude.com/docs/en/plugins/code-intelligence). Quando o plugin existe, o Sidecar mantém processo próprio e diz isso no motivo.

**D2 — cliente JSON-RPC.** stdio com `Content-Length`, um leitor concorrente e correlação por `id`. Limites: cabeçalho de 64 KiB, corpo de 16 MiB, `didOpen` de até 512 KiB, timeout de 5 s por requisição e de 15 s no `initialize`. Métodos: `initialize`, `initialized`, `textDocument/didOpen`, `documentSymbol`, `definition`, `references`, `shutdown` e `exit`. Cada resultado passa por `filepath.IsLocal`, `EvalSymlinks` e checagem de prefixo da worktree; caminho fora dela é descartado, nunca injetado. `Span.Evidence()` gera `retrieve.Evidence` com `Source: "lsp"` e `Verdict: "VERIFIED"`. No Unix o servidor roda em grupo de processos e `Close` mata a árvore; no Windows só o processo principal é morto (limite conhecido, veja `proc_windows.go`).

**Opt-in e confiança.** `Start` recusa sem `SIDECAR_LSP=1` e sem a raiz canônica em `lsp-trusted-roots.json` (dentro de `SIDECAR_CONFIG_HOME`). A confiança nunca é inferida do conteúdo do repositório. Quem concede é o usuário, com `sidecar lsp trust` (pergunta ou `--yes`). Esse subcomando não estava na lista do brief; foi adicionado porque sem ele não há como cumprir "workspace confiável".

**D2 — fallback de linguagens (decisão).** O brief pedia `internal/extract/fallback.go` para Rust, PHP, Ruby e Kotlin, por supor que não há gramática embutida. A premissa está errada: `gotreesitter` v0.56.0 traz gramáticas e consultas de tags para as quatro (verificado por teste, `internal/extract/embedded_langs_test.go`). A lacuna real era o `repo.Classify`, que não mapeava essas extensões. Foram mapeadas (`.rs`, `.php`, `.rb`, `.kt`) e `extractVersion` subiu para 2, para que índices antigos sejam reprocessados em lotes. Um fallback por regex seria código morto e menos preciso, então **não foi criado**.

**D3 — `sidecar lsp setup`.** Só entram comandos fixados e verificados nesta máquina: `go install golang.org/x/tools/gopls@v0.24.0`, `npm install -g typescript-language-server@6.0.3 typescript@7.0.2`, `npm install -g pyright@1.1.414` e `rustup component add rust-analyzer`. Os comandos rodam como argv, sem shell, e só depois de confirmação explícita. `csharp`, `java` e `php` estão **bloqueados** com o motivo: sem versão fixada verificada, sem gerenciador oficial confiável, ou licença proprietária (intelephense). A licença de `gopls` (BSD-3-Clause, `golang.org/x/tools`) está declarada no plano, mas o arquivo `LICENSE` do módulo não foi reconferido nesta rodada.

**Verificação executada (2026-11-06, Linux, gopls v0.24.0, go 1.27):**
- `go test -count=1 ./...` e `go test -race` em `internal/lsp`, `store`, `index`, `app`, `gitx`: passam.
- Contract tests com servidor mock (o próprio binário de teste, via `SIDECAR_LSP_MOCK`): navegação dentro da worktree, descarte de URI externa e de symlink que aponta para fora, timeout em servidor lento com `Close` limitado, erro do servidor, capacidade não anunciada, e recusa sem opt-in ou sem confiança.
- E2E real com `gopls`: `TestGoplsEndToEnd` confere símbolos, definição e referências com linhas exatas.
- Probe manual pela CLI: `sidecar lsp doctor --probe` reportou `go ready` com `gopls` 0.24.0 numa worktree confiável de teste.

**Pendências, sem afirmar o que não foi testado:**
- O hook de prompt não consulta LSP. Integrar exige medir latência de cold start (o hook tem 1,5 s de orçamento) e custo total, conforme [roadmap](roadmap.md) C4/D2.
- Probe real só para Go. TypeScript, Python e Rust têm plano e detecção, mas o handshake não foi executado nesta rodada.
- Windows e macOS: compilação cruzada e `go vet` passam; nenhum teste de execução foi feito nesses sistemas.
- Coexistência com o plugin oficial numa sessão real do Claude Code: não testada. Só a leitura de `enabledPlugins` tem teste.
- `sidecar lsp setup` com execução real não foi rodado nesta máquina (instalaria software). Há teste de recusa sem consentimento e de bloqueio, e o caminho de execução é só `exec` de argv.
- Context7 (provider remoto): não implementado. O `DependencyDocsProvider` tem apenas o cache local com proveniência (`internal/docprovider`).

**Upstream consultado.** Serena em `ede329aaf0c824bea75455a2926f01420d7aa620` (`main` no momento da consulta): só o layout de `src/solidlsp` foi listado. Nenhum arquivo foi lido nem copiado. O `SolidLSP` é Python e o cliente é Go, então a referência usada foi a [especificação LSP](https://microsoft.github.io/language-server-protocol/). A licença MIT declarada no catálogo para `src/solidlsp` não foi reverificada no arquivo. Isso fica registrado como lacuna, porque o catálogo pedia a consulta antes da implementação, e ela não ocorreu nessa ordem.
