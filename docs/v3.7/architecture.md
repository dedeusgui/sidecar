# V3.7 — arquitetura e infraestrutura P0–P3 (opt-in experimental)

> **Estado:** implementada como **opt-in** via `SIDECAR_ENGINE=v3.7` (sobre base `main@8550eaf`, com V1 ainda como padrão). Resultados medidos em P0 e P1 offline constam em [progress.md](progress.md). Nenhum resultado **P3** (agentic pago) foi executado ainda; portanto a V3.7 **não** substitui a V1 nem encerra os critérios finais de promoção em [engine-progression.md](../engine-progression.md).

## Objetivo arquitetural

A V3.7 combina o gate seletivo e o roteador de esforço inspirados na V3 com o pipeline unificado da V3.5/V3.6 (planner, seleção por seções, fallback entre alternativas, snippets e limpeza de rótulos de confiança) e melhorias em extração, resolução de símbolos, consulta ao grafo estrutural (`internal/store`) e `explore`. O foco desta entrega foi:

1. Eliminar a degradação vista na V3.5/V3.6 entre recuperação, planejamento e entrega final (zero conflitos de rótulos, zero saídas apenas de orientação nos benchmarks P1, snippets prioritários de código de produção).
2. Fechar o ciclo de desenvolvimento P0→P3 no próprio repositório ([AGENTS.md](../../AGENTS.md)) com o portão rápido `$0` (`bench/fast_gate.py`), suporte do Studio HTML (`internal/studio`) aos scorecards de entrega (`delivery/1` e `e2-offline/1`) e preparo do `bench/lab/lab.py` para `V3.6`/`V3.7` e `@colbymchenry/codegraph`.

## Fluxo da Engine V3.7

```text
Hook/Explain (SIDECAR_ENGINE=v3.7)
  ├─► internal/engine (Version37 = "v3.7")
  │     ├─ relevanceGateV37 + hasCodeAnchorV37 (ignora âncoras em test/bench)
  │     └─ routeV37 (preferImplementationEvidenceV37, ro.MaxItems = 60)
  │
  ├─► internal/retrieve (V37Tuning)
  │     ├─ RaritySupport + SplitMemberKeys + filterV37Filler
  │     └─ PreferImplTieBreak (2.2^tier, Exact*8.0, prod 2.0x vs test/example/const/doc, perFile decay)
  │
  ├─► internal/extract + internal/index + internal/store + internal/explore
  │     ├─ extract.go / resolve.go: imports, rotas, decorators PHP/Ruby/Python e resolução por sufixo
  │     ├─ store/query.go: SubtypesOf, SubtypeMethods e ordenação CalleesOf/CallersOf (prod antes de test)
  │     └─ explore (+core/+service): withExplorePool (MaxItems: 60, V37Tuning), expandCallChain com subtipos
  │
  └─► internal/app/v37.go + internal/ctxbuild
        └─ v37Plan / v37Choices / v37EvidenceItem + sanitizeTrustLabels (sem rótulos duplicados/conflitantes)
```

### Componentes alterados ou adicionados

| Camada | Arquivos principais | Responsabilidade na V3.7 |
|---|---|---|
| **Orquestração V3.7** | `internal/app/v37.go`, `internal/app/v37_test.go`, `internal/app/v35.go`, `internal/app/diagnostics.go`, `internal/app/activity.go` | Fluxo `v37Plan` / `v37Choices` / `v37EvidenceItem`, integração em diagnóstico e atividade do engine `v3.7`. |
| **Gate e Roteador** | `internal/engine/engine.go`, `internal/engine/planner.go`, `internal/engine/signals.go`, `internal/engine/v37_test.go` | `Version37 = "v3.7"`, `relevanceGateV37`, `hasCodeAnchorV37` (ignora caminhos `test`/`bench`), `preferImplementationEvidenceV37`, `routeV37` e `ro.MaxItems = 60`. |
| **Recuperação e Tuning** | `internal/retrieve/retrieve.go`, `internal/retrieve/packer.go`, `internal/retrieve/tuning.go`, `internal/retrieve/v37_test.go` | `V37Tuning()`, `RaritySupport`, `SplitMemberKeys`, `filterV37Filler` e `PreferImplTieBreak` priorizando código de produção (`Exact`, `VERIFIED`) sobre constantes, exemplos, testes e documentação narrativa. |
| **Contexto / Trust Labels** | `internal/ctxbuild/packer.go`, `internal/ctxbuild/packer_test.go` | `sanitizeTrustLabels` garantindo ausência de conflitos de rótulos de confiança no texto emitido. |
| **Extração, Índice e Grafo** | `internal/extract/extract.go`, `internal/index/index.go`, `internal/index/resolve.go`, `internal/store/query.go` | Suporte ampliado a imports/rotas/decorators (PHP/Ruby/Python), resolução por sufixo/Ruby e métodos `SubtypesOf`/`SubtypeMethods` + ordenação de `CalleesOf`/`CallersOf` em `store/query.go` priorizando código de produção e `same_file`. |
| **Explore** | `internal/explore/explore.go`, `internal/explore/core/core.go`, `internal/explore/service/service.go` | `withExplorePool` (`MaxItems: 60`, `PreferImplementation`, `PreferImplTieBreak`, `RaritySupport`, `SplitMemberKeys`, `V37Tuning`) e `expandCallChain` com travessia de subtipos e filtragem pré-slice de testes. |
| **CLI, Eval e Adaptador** | `cmd/sidecar/main.go`, `cmd/sidecar-eval/{delivery,external,main}.go`, `internal/adapter/tool.go` | Seleção explícita de `--engine v3.7`, campo `language` em manifesto externo, resumo do gate de entrega e correspondência case-insensitive de `HookEventName` (`strings.EqualFold`) em eventos de hook (ex.: `PreInvocation`). |

## Infraestrutura P0–P3 e Observatory HTML

- **`bench/fast_gate.py` (`bench/test_fast_gate.py`):** portão rápido de custo `$0` que executa P0 (`go test`, `unittest`, opcionalmente `-race`) + P1 (`sidecar-eval --delivery` em `bench/cases.json` e opcionalmente `--external-manifest`, E1/E2 offline em `--checkout`, comparação `--candidate` vs `--baseline` com `--enforce-gate`) e gera o painel HTML único do Studio. O gate reprova (não pula em silêncio) estrato ou motor ausente, regressão de gold ou top-3, orientação só, vazamento em negativos, custo por caso com gold acima de +10% (`tokens_per_gold_delivered`) e p95 acima de 1,5× a base + 5 ms; a latência usa a mediana de `--latency-runs` (padrão 3) execuções.
- **`internal/studio` (`studio.go`, `template.html`):** reconhece os esquemas `"delivery/1"` e `"e2-offline/1"` para renderizar o Observatory HTML comparativo (incluindo comparativo Sidecar vs CodeGraph).
- **`bench/v3/e2_offline.py`:** passou a aceitar `--engines` separado por vírgulas (ex.: `v2,v3,v3.6,v3.7`) e emitir o cabeçalho `"schema": "e2-offline/1"`.
- **`bench/lab/lab.py` (`bench/lab/test_lab.py`):** estendido com suporte aos braços `V3.6` e `V3.7` (`EXPLORE_ENGINES = {"V3", "V3.5", "V3.6", "V3.7"}`) e descoberta de `@colbymchenry/codegraph` em instalações nvm (`lib/node_modules/@colbymchenry/codegraph`). Nenhuma execução paga P3 foi disparada nesta etapa.

## Referência externa consultada (clean-room em Go)

Conforme o contrato de engenharia do repositório ([AGENTS.md](../../AGENTS.md)):

| Componente upstream | Versão / Licença | Arquivos consultados (`.d.ts`) | Decisão e justificativa |
|---|---|---|---|
| `@colbymchenry/codegraph` (`https://github.com/colbymchenry/codegraph.git`) | `1.6.0` / **MIT** | `dist/context/{formatter,index,markers}.d.ts`; `dist/extraction/{index,function-ref,grammars}.d.ts`; `dist/search/{identifier-segments,query-parser,query-paths,query-utils}.d.ts`; `dist/resolution/{name-matcher,path-aliases,types}.d.ts` | Consultadas apenas interfaces/declarações TypeScript de extração multi-linguagem, segmentação de identificadores, resolução de caminhos e formatação de contexto para orientar a **implementação independente em Go** em `internal/extract`, `internal/index`, `internal/store`, `internal/retrieve`, `internal/engine` e `internal/explore`. Nenhuma dependência Node/TS foi adicionada ao runtime Go (preserva local-first, single binary e zero serviços externos). |

## Rollback

A V1 permanece padrão. Para desativar a V3.7 a qualquer momento, basta remover ou alterar a variável de ambiente:

```bash
unset SIDECAR_ENGINE
```
