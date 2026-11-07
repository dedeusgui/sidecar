# Avisos de terceiros

Este arquivo lista o código de terceiros que o Sidecar **compila ou distribui** e as referências arquiteturais que **consultamos**, com a licença de cada item e a decisão de uso. O Sidecar é distribuído sob a licença MIT (`LICENSE`).

Atualizado em 2026-11-07, com base em `go.mod` e nos arquivos de licença presentes no cache de módulos do Go nesta máquina. Ao mudar `go.mod`, atualize este arquivo no mesmo PR.

## 1. Dependências Go (compiladas no binário)

Licenças conferidas pelo texto do arquivo `LICENSE` de cada módulo na versão fixada em `go.mod`.

| Módulo | Versão | Licença | Uso |
|---|---|---|---|
| `modernc.org/sqlite` | v1.45.0 | BSD-3-Clause | Armazenamento SQLite/FTS5 local (`internal/store`). O motor SQLite embutido é código de domínio público; o pacote Go é BSD-3-Clause. |
| `modernc.org/libc` | v1.67.6 | BSD-3-Clause | Dependência transitiva de `modernc.org/sqlite`. |
| `modernc.org/mathutil` | v1.7.1 | BSD-3-Clause | Dependência transitiva de `modernc.org/sqlite`. |
| `modernc.org/memory` | v1.11.0 | BSD-3-Clause | Dependência transitiva de `modernc.org/sqlite`. |
| `github.com/odvcencio/gotreesitter` | v0.56.0 | MIT | Análise sintática por tree-sitter (`internal/extract`). As gramáticas embutidas seguem licenças dos projetos tree-sitter de origem; **não auditadas nesta rodada** (pendência). |
| `github.com/dustin/go-humanize` | v1.0.1 | MIT | Dependência transitiva. |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause | Dependência transitiva. |
| `github.com/mattn/go-isatty` | v0.0.20 | MIT | Dependência transitiva. |
| `github.com/ncruces/go-strftime` | v1.0.0 | MIT | Dependência transitiva de `modernc.org/sqlite`. |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause | Dependência transitiva. |
| `golang.org/x/exp` | v0.0.0-20251023183803-a4bb9ffd2546 | BSD-3-Clause | Dependência transitiva. |
| `golang.org/x/sys` | v0.37.0 | BSD-3-Clause | Dependência transitiva. |

Nenhum código desses módulos foi copiado para o repositório. Os avisos de licença acima são os exigidos por BSD-3-Clause e MIT na redistribuição de binários; o pacote de distribuição deve incluir este arquivo.

## 2. Referências arquiteturais consultadas

Fonte: [`docs/reference-implementations.md`](docs/reference-implementations.md). As licenças abaixo são as registradas no catálogo, **verificadas nele em 2026-11-06 e não reverificadas nesta rodada**. Nesta rodada, nenhum código de referência foi lido, copiado ou traduzido; a decisão é de implementação independente em Go, conforme a tabela.

| Referência | Licença registrada | Decisão no Sidecar |
|---|---|---|
| [CodeGraph (codegraph-ai)](https://github.com/codegraph-ai/CodeGraph) | Apache-2.0 | Concorrente. Comparação prevista em E3; sem código incorporado. |
| [colbymchenry/codegraph](https://github.com/colbymchenry/codegraph) | MIT | Concorrente de injeção por hook. Comparação prevista em E3; sem código incorporado. |
| [oraios/serena](https://github.com/oraios/serena) — `solidlsp` | MIT (`solidlsp`); aplicação GPL-3.0-or-later | Não integrado. A aplicação GPL não é importada. LSP do Sidecar é cliente próprio (`internal/lsp`). |
| [upstash/context7](https://github.com/upstash/context7) | MIT (código publicado); backend não aberto | Não integrado. Provedor local de documentação (`internal/docprovider`); sem rede obrigatória. |
| [yamadashy/repomix](https://github.com/yamadashy/repomix) | MIT | Referência de filtros e empacotamento. Sem código incorporado. |
| [rtk-ai/rtk](https://github.com/rtk-ai/rtk) | `rtk-oss` Apache-2.0; `engine/` e `proxy/` BUSL-1.1 | Não integrado. O motor BUSL não é usado nem copiado. |
| [headroomlabs-ai/headroom](https://github.com/headroomlabs-ai/headroom) | Apache-2.0 (OSS) | Referência de avaliação de compressão. Sem código incorporado. |
| [thedotmack/claude-mem](https://github.com/thedotmack/claude-mem) | Apache-2.0 (documentada) | Referência de eventos e retenção para `SessionObserver`. Não lido nesta rodada; sem código incorporado. |
| [gastownhall/beads](https://github.com/gastownhall/beads) | MIT | Referência de modelo de estado. Sem código incorporado. |
| [obra/superpowers](https://github.com/obra/superpowers) | MIT | Referência de contratos de verificação. Sem código incorporado. |

### Referências de avaliação (não são runtime)

| Referência | Licença | Situação |
|---|---|---|
| [Agent Retrieval Bench](https://github.com/eyuansu62/agent-retrieval-bench) | Código avaliador e metadados MIT; **arquivos-fonte dos corpora mantêm licenças upstream** | `sidecar-eval --external-cases` lê um manifesto **normalizado** com commits fixados. **Nenhum corpus foi baixado nem incorporado**, e o conversor do formato oficial do ARB ainda não existe (o esquema não foi verificado; ver `docs/benchmark.md`). |
| [ContextBench](https://contextbench.github.io/) | **Não declarada** na página consultada | Não incorporado. Nenhum dado redistribuído. |
| [SWE-bench](https://github.com/SWE-bench/SWE-bench) | Ver repositório upstream | Citado como referência; não usado nesta rodada. |
| [Aider RepoMap](https://aider.chat/docs/repomap.html) | Ver documentação upstream | Citado como referência; não usado nesta rodada. |

## 3. Dados e fixtures

- `bench/testdata/fixtures/` (inclui `v2/`) contém código escrito para este repositório, sem código de terceiros copiado.
- `bench/cases.json`, `bench/v2_scenarios.json` e `bench/agentic/manifest.jsonl` são casos e gabaritos escritos para este repositório.
- Nenhum corpus externo está versionado. Para reproduzir um corpus externo, baixe-o fora do repositório, fixe o commit e respeite a licença dele.

## 4. Pendências

- Auditar as licenças das gramáticas embutidas em `gotreesitter` (cada uma vem de um projeto tree-sitter-* distinto).
- Reverificar, antes de cada release, as licenças listadas na seção 2 nos SHAs consultados.
- Confirmar a licença de dados do ContextBench antes de qualquer redistribuição.
