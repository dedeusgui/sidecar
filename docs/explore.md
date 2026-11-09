# `sidecar explore` — exploração estrutural sob demanda (contrato `sidecar-explore/1`)

**Estado:** experimental, opt-in, local. Implementado em `internal/explore` sobre o índice existente (`internal/index`, `internal/store`) e a recuperação V2/V3 (`internal/retrieve`, `internal/engine`). Sem daemon, MCP, rede, modelo ou embeddings. Não executa código do repositório. **Nenhuma economia end-to-end foi medida ainda** (ver [progresso V3](v3/progress.md)).

## Uso

```bash
sidecar explore --query "How does res.send decide the Content-Type and ETag?" [--root DIR] [--json] \
                [--max-files N] [--max-chars N] [--impact]
```

- Sem `--json`: texto para um agente ou terminal. Cada arquivo com código começa com ``**`caminho`**``; linhas de código vêm numeradas como `cat -n` (`N<TAB>código`) para o agente citar `arquivo:linha` sem reler o arquivo.
- `--json`: contrato abaixo. Saída só em stdout; erros em stderr; uso inválido sai com código 2, erro de execução com 1.
- O índice é sincronizado com limite de 60 s (com histórico Git). Sincronização incompleta vira lacuna explícita.

## O que é retornado

| Campo | Conteúdo | Status |
|---|---|---|
| `relevance`, `class`, `intent` | Veredito do RelevanceGate V3, classe de esforço e intenção. O explore responde mesmo se o gate disser `irrelevant`, porque foi pedido; nesse caso registra a lacuna | — |
| `entry_points` | Símbolos sem chamador indexado no topo da cadeia que leva aos candidatos | sempre `CANDIDATE` |
| `files[]` | Arquivos candidatos (até `budget.max_files`), papel (`candidate`/`test`), símbolos com veredito/fonte/confiança/motivo, trechos numerados, `stale` | do índice |
| `relations[]` | Chamadas (`calls`) e imports resolvidos | `VERIFIED` = mesmo arquivo ou import resolvido; `CANDIDATE` = nome definido uma única vez |
| `consumers`, `dependencies` | Quem chama/importa os candidatos e o que eles chamam/importam | idem |
| `tests` | Arquivos de teste que referenciam os símbolos ou casaram a consulta | `VERIFIED` (referência no índice) |
| `contracts` | Ligações de contrato da V2 (doc/teste/valor), no máximo 8 | status da V2 |
| `impact` | `impact.Analyze` sobre os arquivos mostrados quando a tarefa é edição/contrato/multiarquivo ou `--impact` | status do impact |
| `not_shown` | Arquivos ranqueados que não couberam: apontados, não entregues | — |
| `gaps` | Limites da análise: lacunas de gramática, arquivos alterados desde a indexação, histórico ausente, despacho dinâmico, entry points heurísticos | — |
| `budget` | `max_files`, `max_chars`, `used_chars`, `repo_files` | — |

**Trechos de corpos longos:** um símbolo com mais de 60 linhas mantém as 12 primeiras, até 8 janelas (±3 linhas) nas linhas que mencionam termos da consulta, priorizando termos raros dentro do corpo, e a linha final. Lacunas aparecem como `⋯ lines A-B not shown`.

**Orçamento** (caracteres de código, escala com o repositório): <200 arquivos: 3 arquivos/6 000; <2 000: 4/9 000; <15 000: 5/12 000; acima: 6/16 000. É teto, não meta; relevância decide o que entra.

## O que NÃO afirma

- Chamadas são ligadas por nome no mesmo arquivo, por import resolvido ou nome único: despacho dinâmico, reflexão, callbacks e código gerado não são resolvidos.
- `entry_points` são candidatos: "sem chamador indexado" não prova ponto de entrada (frameworks, test runners, outros repositórios).
- Testes listados *referenciam* símbolos; o explore não executa testes nem afirma que passam.
- Sem gramática, símbolos vêm de regras léxicas e são marcados como lacuna.

## Integração com agentes

- **Hook V3** (`SIDECAR_ENGINE=v3`): quando a decisão é `EXPLORE_AVAILABLE` ou `VERIFY`, o bloco ganha uma linha `ON DEMAND: <executável> explore --query "..."`. Não força o uso; não intercepta Read/Grep.
- **Laboratório:** braço `V3` coloca `sidecar` no PATH do sandbox e permite `Bash(sidecar explore:*)`; braço `V3-hook` não. A diferença é registrada em `meta.json` (`explore_arms`).
- Não há adaptador MCP nem suporte nativo Codex/Antigravity declarado; não existe E2E com cliente real ainda.

## Referência consultada

CodeGraph `f72c9d2` (MIT): `docs/benchmarks/explore-sufficiency.md` e `src/mcp/tools.ts` (orçamento adaptativo por tamanho do projeto, seções por arquivo com marcador único, linhas numeradas para citar sem reler, lista de não mostrados). Implementação do Sidecar independente, em Go, sobre o índice SQLite existente; nenhum código copiado.
